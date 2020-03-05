package capacity

import (
	"encoding/binary"
	"math"
	"sync"

	"gopkg.in/karalabe/cookiejar.v2/collections/prque"
	"massnet.org/mass/logging"
	"massnet.org/mass/poc/engine"
)

type queuedWorkSpace struct {
	ws          *WorkSpace
	wouldMining bool
}

// newQueuedWorkSpace creates queuedWorkSpace from an existing workSpace.
// Priority represents the priority of plotting space.
func newQueuedWorkSpace(ws *WorkSpace, wouldMining bool) *queuedWorkSpace {
	return &queuedWorkSpace{
		ws:          ws,
		wouldMining: wouldMining,
	}
}

func (qws *queuedWorkSpace) priority() float32 {
	// diff prevents workSpaces have same priority
	var diff = float32(binary.LittleEndian.Uint32(qws.ws.id.PubKeyHash().Bytes()[:4])>>1) / float32(math.MaxUint32)
	return -float32(qws.ws.id.Ordinal()) + diff
}

type plotterQueue struct {
	*prque.Prque
	sync.Mutex
	poppedItem *queuedWorkSpace
}

func newPlotterQueue() *plotterQueue {
	return &plotterQueue{
		Prque: prque.New(),
	}
}

func (pq *plotterQueue) Pop() (*queuedWorkSpace, float32) {
	pq.Lock()
	defer pq.Unlock()

	data, priority := pq.Prque.Pop()
	ws := data.(*queuedWorkSpace)
	pq.poppedItem = ws
	return ws, priority
}

func (pq *plotterQueue) PopItem() *queuedWorkSpace {
	pq.Lock()
	defer pq.Unlock()

	ws := pq.Prque.PopItem().(*queuedWorkSpace)
	pq.poppedItem = ws
	return ws
}

func (pq *plotterQueue) Delete(sid string) {
	pq.Lock()
	defer pq.Unlock()

	newQueue := prque.New()
	for !pq.Empty() {
		qws, priority := pq.Prque.Pop()
		if qws.(*queuedWorkSpace).ws.id.String() == sid {
			continue
		}
		newQueue.Push(qws, priority)
	}
	pq.Prque = newQueue
}

func (pq *plotterQueue) PoppedItem() *queuedWorkSpace {
	pq.Lock()
	defer pq.Unlock()

	return pq.poppedItem
}

func (pq *plotterQueue) Reset() {
	pq.Lock()
	defer pq.Unlock()

	pq.Prque.Reset()
	pq.poppedItem = nil
	return
}

func (sk *SpaceKeeper) spacePlotter() {
	sk.wg.Add(1)
	defer sk.wg.Done()

	var wg sync.WaitGroup

	var plotSpace = func(qws *queuedWorkSpace) {
		ws := qws.ws
		sid := ws.id.String()
		var changeState = func(old, new engine.WorkSpaceState) {
			sk.workSpaceIndex[old].Delete(sid)
			sk.workSpaceIndex[new].Set(sid, ws)
			ws.state = new
		}
		// Step 1: safely change state to plotting/mining
		sk.stateLock.Lock()
		if _, ok := sk.workSpaceIndex[engine.Registered].Get(sid); ok {
			changeState(engine.Registered, engine.Plotting)
		} else {
			if _, ok := sk.workSpaceIndex[engine.Ready].Get(sid); ok && qws.wouldMining {
				changeState(engine.Ready, engine.Mining)
			}
			sk.stateLock.Unlock()
			return
		}
		sk.stateLock.Unlock()

		// Step 2: plot space (wait for finishing)
		ws.Plot()

		// Step 3: change workSpace state
		sk.stateLock.Lock()
		if ws.Progress() < 100 {
			changeState(engine.Plotting, engine.Registered)
		} else {
			if qws.wouldMining {
				changeState(engine.Plotting, engine.Mining)
			} else {
				changeState(engine.Plotting, engine.Ready)
			}
		}
		sk.stateLock.Unlock()
	}

	var addSpaces = func(newSpace *queuedWorkSpace, newSpaceChan chan *queuedWorkSpace) {
		sk.queue.Push(newSpace, newSpace.priority())
	out:
		for {
			select {
			case qws := <-newSpaceChan:
				sk.queue.Push(qws, qws.priority())
			default:
				break out
			}
		}
	}

	var monitor = func(ws *WorkSpace, killMonitorCh chan struct{}) {
		select {
		case <-sk.quit:
			ws.StopPlot()
		case <-killMonitorCh:
		}
		wg.Done()
	}

	logging.CPrint(logging.INFO, "space plotter started", logging.LogFormat{"queue_length": sk.queue.Size()})
	defer func() {
		sk.queue.Reset()
	}()

	for {
		for !sk.queue.Empty() {
			select {
			case <-sk.quit:
				wg.Wait()
				return
			default:
			}

			qws := sk.queue.PopItem()
			killMonitorCh := make(chan struct{}, 1)
			wg.Add(1)
			go monitor(qws.ws, killMonitorCh)
			plotSpace(qws)
			close(killMonitorCh)
		}

		select {
		case <-sk.quit:
			wg.Wait()
			return
		case qws := <-sk.newQueuedWorkSpaceCh:
			addSpaces(qws, sk.newQueuedWorkSpaceCh)
		}

	}
}
