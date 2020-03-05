package capacity

import (
	"sync"

	"massnet.org/mass/logging"
	"massnet.org/mass/massutil/ccache"
	"massnet.org/mass/poc/engine"
	"massnet.org/mass/poc/pocutil"
)

type WsSearchResult struct {
	proof *engine.WorkSpaceProof
	error error
	wg    *sync.WaitGroup
}

type Job struct {
	ws         *WorkSpace
	cacheKey   string
	challenge  pocutil.Hash
	proofCache *ccache.CCache
	result     chan WsSearchResult
	wg         *sync.WaitGroup
}

type Worker struct {
	WorkerPoolCh chan<- *Worker
	JobCh        chan Job
	quit         <-chan struct{}
}

func (w *Worker) Run(index int, wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()

	go func() {
		for {
			w.WorkerPoolCh <- w
			select {
			case job := <-w.JobCh:
				proof, err := job.ws.db.GetProof(job.challenge)
				eProof := &engine.WorkSpaceProof{
					SpaceID:   job.ws.id.String(),
					Proof:     proof,
					PublicKey: job.ws.id.PubKey(),
					Ordinal:   job.ws.id.Ordinal(),
					Error:     err,
				}
				job.proofCache.Add(job.cacheKey, eProof)
				job.result <- WsSearchResult{proof: eProof, error: err, wg: job.wg}

			case <-w.quit:
				return
			}
		}
	}()
}

type WorkerPool struct {
	MaxWorks int
	WorkerCh chan *Worker
	JobCh    chan Job
	result   chan WsSearchResult
	stop     chan struct{}
	wg       sync.WaitGroup
}

func NewWorkerPool(maxWorks int) *WorkerPool {
	return &WorkerPool{
		MaxWorks: maxWorks,
		WorkerCh: make(chan *Worker, maxWorks),
		JobCh:    make(chan Job),
		result:   make(chan WsSearchResult, maxWorks),
	}
}

func (sk *SpaceKeeper) runWorkerPool() {
	wp := sk.workerPool
	wp.stop = make(chan struct{})
	wp.result = make(chan WsSearchResult, wp.MaxWorks)

	go wp.dispatchRoutine()
	go wp.resultRoutine()

	for i := 0; i < wp.MaxWorks; i++ {
		worker := &Worker{
			WorkerPoolCh: wp.WorkerCh,
			JobCh:        make(chan Job),
			quit:         wp.stop,
		}
		worker.Run(i, &wp.wg)
	}
}

func (wp *WorkerPool) Stop() {
	close(wp.stop)
	wp.wg.Wait()
}

func (wp *WorkerPool) AddTask(job Job) {
	job.wg.Add(1)
	wp.JobCh <- job
}

func (wp *WorkerPool) dispatchRoutine() {
	wp.wg.Add(1)
	defer wp.wg.Done()

	for {
		select {
		case job := <-wp.JobCh:
			worker := <-wp.WorkerCh
			go func(job Job, worker *Worker) {
				worker.JobCh <- job
			}(job, worker)

		case <-wp.stop:
			logging.CPrint(logging.DEBUG, "WorkerPool dispatcher done")
			return
		}
	}
}

func (wp *WorkerPool) resultRoutine() {
	wp.wg.Add(1)
	defer wp.wg.Done()

	for {
		select {
		case result := <-wp.result:
			result.wg.Done()

		case <-wp.stop:
			return
		}
	}
}
