package fractal

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/massnetorg/mass-core/logging"
	"massnet.org/mass/fractal/connection"
)

const (
	defaultMaxCollector  = 2000
	defaultListenNetwork = "tcp"
)

type collectorPoolOptions struct {
	maxCollector  uint32
	listenAddress string
	listenNetwork string
	connOptions   []connection.Option
}

func defaultCollectorPoolOptions() *collectorPoolOptions {
	return &collectorPoolOptions{
		maxCollector:  defaultMaxCollector,
		listenNetwork: defaultListenNetwork,
		connOptions: []connection.Option{
			connection.CopyMsg(false),
		},
	}
}

type CollectorPoolOption interface {
	apply(options *collectorPoolOptions)
}

type collectorPoolOptionFunc struct {
	fn func(options *collectorPoolOptions)
}

func (of *collectorPoolOptionFunc) apply(o *collectorPoolOptions) {
	of.fn(o)
}

func newCollectorPoolOptionFunc(fn func(options *collectorPoolOptions)) *collectorPoolOptionFunc {
	return &collectorPoolOptionFunc{fn: fn}
}

func CollectorPoolMaxCollector(max uint32) CollectorPoolOption {
	return newCollectorPoolOptionFunc(func(options *collectorPoolOptions) {
		if max > 0 {
			options.maxCollector = max
		}
	})
}

func CollectorPoolListenAddress(listenAddress string) CollectorPoolOption {
	return newCollectorPoolOptionFunc(func(options *collectorPoolOptions) {
		options.listenAddress = listenAddress
	})
}

type CollectorPool struct {
	l            sync.RWMutex
	wg           sync.WaitGroup
	ctx          context.Context
	ctxCanceller context.CancelFunc
	stopping     int32 // atomic
	stopped      int32 // atomic
	superior     Superior
	listener     *Listener
	opts         *collectorPoolOptions
	collectors   map[uuid.UUID]Collector
	cancellers   map[uuid.UUID]context.CancelFunc
}

func NewCollectorPool(ctx context.Context, superior Superior, opt ...CollectorPoolOption) (*CollectorPool, context.CancelFunc, error) {
	cCtx, cancel := context.WithCancel(ctx)
	opts := defaultCollectorPoolOptions()
	for _, o := range opt {
		o.apply(opts)
	}
	opts.connOptions = append(opts.connOptions, connection.WithContext(cCtx))
	listener, err := NewListener(opts.listenNetwork, opts.listenAddress, opts.connOptions...)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	pool := &CollectorPool{
		ctx:          cCtx,
		ctxCanceller: cancel,
		superior:     superior,
		listener:     listener,
		opts:         opts,
		collectors:   make(map[uuid.UUID]Collector),
		cancellers:   make(map[uuid.UUID]context.CancelFunc),
	}
	return pool, pool.run(), nil
}

func (pool *CollectorPool) run() context.CancelFunc {
	pool.wg.Add(1)
	go pool.listenRoutine()

	return func() {
		pool.waitStop()
	}
}

func (pool *CollectorPool) waitStop() {
	if atomic.LoadInt32(&pool.stopped) == 1 {
		return
	}
	if !atomic.CompareAndSwapInt32(&pool.stopping, 0, 1) {
		pool.wg.Wait()
		return
	}

	pool.ctxCanceller()
	pool.wg.Wait()
	atomic.StoreInt32(&pool.stopped, 1)
}

func (pool *CollectorPool) Count() uint32 {
	pool.l.RLock()
	defer pool.l.RUnlock()
	return uint32(len(pool.collectors))
}

func (pool *CollectorPool) listenRoutine() {
	defer pool.wg.Done()

routine:
	for {
		conn, cancel, err := pool.listener.Accept()
		if err != nil {
			logging.CPrint(logging.WARN, "fail to accept inbound collector connection", logging.LogFormat{"err": err})
			select {
			case <-pool.ctx.Done():
				logging.CPrint(logging.INFO, "leave collector_pool routine")
				return
			default:
			}
			continue routine
		}
		if pool.Count() >= pool.opts.maxCollector {
			logging.CPrint(logging.WARN, "inbound collector connection rejected", logging.LogFormat{
				"err":           "too many collector connections",
				"max_collector": pool.opts.maxCollector,
				"local_addr":    conn.LocalAddr(),
				"remote_addr":   conn.RemoteAddr(),
			})
			cancel()
			continue routine
		}
		pool.addCollectorWithConn(conn, cancel)
	}
}

func (pool *CollectorPool) addCollectorWithConn(conn *connection.Conn, connCanceller context.CancelFunc) {
	writer := NewRemoteRequestWriter(pool.ctx, conn)
	reader := NewRemoteReportReader(pool.ctx, conn)
	rc, cancel := NewRemoteCollector(pool.ctx, pool.superior, writer, reader, pool.onCollectorStopped)
	if err := pool.addCollector(rc, func() {
		connCanceller()
		cancel()
	}); err != nil {
		logging.CPrint(logging.INFO, "inbound collector connection rejected", logging.LogFormat{
			"err":          err,
			"local_addr":   conn.LocalAddr(),
			"remote_addr":  conn.RemoteAddr(),
			"collector_id": rc.ID(),
		})
	} else {
		logging.CPrint(logging.INFO, "inbound collector connection accepted", logging.LogFormat{
			"local_addr":   conn.LocalAddr(),
			"remote_addr":  conn.RemoteAddr(),
			"collector_id": rc.ID(),
		})
	}
}

func (pool *CollectorPool) addCollector(c Collector, canceller func()) error {
	pool.l.Lock()
	defer pool.l.Unlock()
	if _, ok := pool.collectors[c.ID()]; ok {
		return errors.New("collector already exists")
	}
	pool.collectors[c.ID()] = c
	pool.cancellers[c.ID()] = canceller
	return nil
}

func (pool *CollectorPool) removeCollector(cid uuid.UUID) {
	pool.l.Lock()
	defer pool.l.Unlock()
	if _, ok := pool.collectors[cid]; !ok {
		return
	}
	if canceller := pool.cancellers[cid]; canceller != nil {
		canceller()
	}
	delete(pool.collectors, cid)
	delete(pool.cancellers, cid)
}

func (pool *CollectorPool) onCollectorStopped(c Collector) {
	pool.removeCollector(c.ID())
}
