package skchia

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/massnetorg/mass-core/logging"
	"github.com/massnetorg/mass-core/massutil/service"
	"github.com/massnetorg/mass-core/poc"
	"github.com/massnetorg/mass-core/poc/chiapos"
	"github.com/massnetorg/mass-core/poc/pocutil"
	"github.com/panjf2000/ants"
	"github.com/shirou/gopsutil/disk"
	"massnet.org/mass/poc/engine.v2"
)

const (
	plotterMaxChanSize = 1024
	maxPoolWorker      = 32
	allState           = engine.LastState + 1 // allState includes all valid states
)

type PoCWallet interface {
	SignMessage(pubKey *chiapos.G1Element, hash []byte) (*chiapos.G2Element, error)
	Unlock(password []byte) error
	Lock()
	IsLocked() bool
}

type SpaceKeeper struct {
	*service.BaseService
	stateLock             sync.RWMutex
	wg                    sync.WaitGroup
	quit                  chan struct{}
	configuring           int32 // atomic
	configured            int32 // atomic
	allowGenerateNewSpace bool
	dbDirs                []string
	dbType                string
	workSpaceIndex        []*WorkSpaceMap
	workSpacePaths        map[string]*WorkSpacePath
	workSpaceList         []*WorkSpace
	queue                 *plotterQueue
	newQueuedWorkSpaceCh  chan *queuedWorkSpace
	workerPool            *ants.Pool
	generateInitialIndex  func() error
	fileWatcher           func()
}

func (sk *SpaceKeeper) OnStart() error {
	sk.quit = make(chan struct{})
	go sk.spacePlotter()
	go sk.fileWatcher()
	logging.CPrint(logging.INFO, "spaceKeeper started")
	return nil
}

func (sk *SpaceKeeper) OnStop() error {
	close(sk.quit)
	sk.wg.Wait()
	logging.CPrint(logging.INFO, "spaceKeeper stopped")
	return nil
}

func (sk *SpaceKeeper) Type() string {
	return sk.Name()
}

func (sk *SpaceKeeper) WorkSpaceIDs(flags engine.WorkSpaceStateFlags) ([]string, error) {
	sk.stateLock.RLock()
	defer sk.stateLock.RUnlock()

	if flags.Contains(engine.SFAll) {
		idList := sk.workSpaceList
		result := make([]string, len(idList))
		for i := range result {
			result[i] = idList[i].id.String()
		}
		return result, nil
	}

	items := getWsByFlags(sk.workSpaceList, flags)
	result := make([]string, 0, len(items))
	for _, ws := range items {
		result = append(result, ws.id.String())
	}
	return result, nil
}

func (sk *SpaceKeeper) WorkSpaceInfos(flags engine.WorkSpaceStateFlags) ([]engine.WorkSpaceInfo, error) {
	sk.stateLock.RLock()
	defer sk.stateLock.RUnlock()

	if flags.Contains(engine.SFAll) {
		wsList := sk.workSpaceList
		result := make([]engine.WorkSpaceInfo, len(wsList))
		for i := range result {
			result[i] = wsList[i].Info()
		}
		return result, nil
	}

	items := getWsByFlags(sk.workSpaceList, flags)
	result := make([]engine.WorkSpaceInfo, 0, len(items))
	for _, ws := range items {
		result = append(result, ws.Info())
	}
	return result, nil
}

// TODO: 1/512 plot filter

func (sk *SpaceKeeper) GetQuality(ctx context.Context, sid string, challenge pocutil.Hash) ([]*engine.WorkSpaceQuality, error) {
	if !sk.Started() {
		return nil, ErrSpaceKeeperIsNotRunning
	}

	if ws, ok := sk.workSpaceIndex[allState].Items()[sid]; ok && ws.using {
		return sk.getQuality(ws, challenge), nil
	}
	return nil, ErrWorkSpaceDoesNotExist
}

func (sk *SpaceKeeper) GetQualities(ctx context.Context, flags engine.WorkSpaceStateFlags, challenge pocutil.Hash) ([]*engine.WorkSpaceQuality, error) {
	if !sk.Started() {
		return nil, ErrSpaceKeeperIsNotRunning
	}

	items := make(map[string]*WorkSpace)
	for _, ws := range getWsByFlags(sk.workSpaceList, flags) {
		items[ws.id.String()] = ws
	}

	qualities := sk.getQualities(items, challenge)
	result := make([]*engine.WorkSpaceQuality, 0, len(qualities))
	for _, quality := range qualities {
		result = append(result, quality...)
	}
	return result, nil
}

func (sk *SpaceKeeper) GetQualityReader(ctx context.Context, sid string, challenge pocutil.Hash) (engine.QualityReader, error) {
	if !sk.Started() {
		return nil, ErrSpaceKeeperIsNotRunning
	}

	if ws, ok := sk.workSpaceIndex[allState].Items()[sid]; ok && ws.using {
		qrw := engine.NewQualityRW(ctx, 1)
		go func() {
			qualities := sk.getQuality(ws, challenge)
			for i := range qualities {
				if err := qrw.Write(qualities[i]); err != nil {
					logging.CPrint(logging.WARN, "fail to write WorkSpaceQuality to QualityRW", logging.LogFormat{"err": err, "sid": sid, "challenge": challenge})
				}
			}
			qrw.Close()
		}()
		return qrw, nil
	}
	return nil, ErrWorkSpaceDoesNotExist
}

func (sk *SpaceKeeper) GetQualitiesReader(ctx context.Context, flags engine.WorkSpaceStateFlags, challenge pocutil.Hash) (engine.QualityReader, error) {
	if !sk.Started() {
		return nil, ErrSpaceKeeperIsNotRunning
	}

	items := make(map[string]*WorkSpace)
	for _, ws := range getWsByFlags(sk.workSpaceList, flags) {
		items[ws.id.String()] = ws
	}
	qrw := engine.NewQualityRW(ctx, len(items))
	go func() {
		qualities := sk.getQualities(items, challenge)
		var err error
		for id, quality := range qualities {
			for i := range quality {
				if err = qrw.Write(quality[i]); err != nil {
					logging.CPrint(logging.WARN, "fail to write WorkSpaceQualities to QualityRW", logging.LogFormat{
						"err":       err,
						"space_id":  id,
						"count":     len(qualities),
						"flags":     flags,
						"challenge": challenge,
						"index":     quality[i].Index,
					})
					break
				}
			}
		}
		qrw.Close()
	}()
	return qrw, nil
}

func (sk *SpaceKeeper) GetProof(ctx context.Context, sid string, challenge pocutil.Hash, index uint32) (*engine.WorkSpaceProof, error) {
	if !sk.Started() {
		return nil, ErrSpaceKeeperIsNotRunning
	}

	if ws, ok := sk.workSpaceIndex[allState].Items()[sid]; ok && ws.using {
		return sk.getProof(ws, challenge, index), nil
	}
	return nil, ErrWorkSpaceDoesNotExist
}

func (sk *SpaceKeeper) GetProofs(ctx context.Context, sids []string, challenge pocutil.Hash, indexes []uint32) ([]*engine.WorkSpaceProof, error) {
	if !sk.Started() {
		return nil, ErrSpaceKeeperIsNotRunning
	}

	items := make(map[string]*WorkSpace)
	indexMap := make(map[string]uint32)
	for i := range sids {
		if ws, ok := sk.workSpaceIndex[allState].Items()[sids[i]]; ok && ws.using {
			items[sids[i]] = ws
			indexMap[sids[i]] = indexes[i]
		}
	}

	proofs := sk.getProofs(items, challenge, indexMap)
	result := make([]*engine.WorkSpaceProof, 0, len(proofs))
	for _, proof := range proofs {
		result = append(result, proof)
	}
	return result, nil
}

func (sk *SpaceKeeper) GetProofReader(ctx context.Context, sid string, challenge pocutil.Hash, index uint32) (engine.ProofReader, error) {
	if !sk.Started() {
		return nil, ErrSpaceKeeperIsNotRunning
	}

	if ws, ok := sk.workSpaceIndex[allState].Items()[sid]; ok && ws.using {
		prw := engine.NewProofRW(ctx, 1)
		go func() {
			if err := prw.Write(sk.getProof(ws, challenge, index)); err != nil {
				logging.CPrint(logging.WARN, "fail to write WorkSpaceProof to ProofRW", logging.LogFormat{"err": err, "sid": sid, "challenge": challenge})
			}
			prw.Close()
		}()
		return prw, nil
	}
	return nil, ErrWorkSpaceDoesNotExist
}

func (sk *SpaceKeeper) GetProofsReader(ctx context.Context, sids []string, challenge pocutil.Hash, indexes []uint32) (engine.ProofReader, error) {
	if !sk.Started() {
		return nil, ErrSpaceKeeperIsNotRunning
	}

	items := make(map[string]*WorkSpace)
	indexMap := make(map[string]uint32)
	for i := range sids {
		if ws, ok := sk.workSpaceIndex[allState].Items()[sids[i]]; ok && ws.using {
			items[sids[i]] = ws
			indexMap[sids[i]] = indexes[i]
		}
	}
	prw := engine.NewProofRW(ctx, len(items))
	go func() {
		proofs := sk.getProofs(items, challenge, indexMap)
		var err error
		for i, proof := range proofs {
			if err = prw.Write(proof); err != nil {
				logging.CPrint(logging.WARN, "fail to write WorkSpaceProofs to ProofRW", logging.LogFormat{
					"err":       err,
					"index":     i,
					"count":     len(proofs),
					"challenge": challenge,
				})
				break
			}
		}
		prw.Close()
	}()
	return prw, nil
}

func (sk *SpaceKeeper) ActOnWorkSpace(sid string, action engine.ActionType) (err error) {
	if !action.IsValid() {
		return engine.ErrInvalidAction
	}

	switch action {
	case engine.Plot:
		err = sk.PlotWS(sid)
	case engine.Mine:
		err = sk.MineWS(sid)
	case engine.Stop:
		err = sk.StopWS(sid)
	case engine.Remove:
		err = sk.RemoveWS(sid)
	case engine.Delete:
		err = sk.DeleteWS(sid)
	default:
		err = engine.ErrUnImplementedAction
	}

	return err
}

func (sk *SpaceKeeper) ActOnWorkSpaces(flags engine.WorkSpaceStateFlags, action engine.ActionType) (errs map[string]error, err error) {
	if !action.IsValid() {
		return nil, engine.ErrInvalidAction
	}

	switch action {
	case engine.Plot:
		errs = sk.PlotMultiWS(flags)
	case engine.Mine:
		errs = sk.MineMultiWS(flags)
	case engine.Stop:
		errs = sk.StopMultiWS(flags)
	case engine.Remove:
		errs = sk.RemoveMultiWS(flags)
	case engine.Delete:
		errs = sk.DeleteMultiWS(flags)
	default:
		return nil, engine.ErrUnImplementedAction
	}

	return errs, nil
}

func (sk *SpaceKeeper) SignHash(sid string, hash [32]byte) (*chiapos.G2Element, error) {
	if ws, ok := sk.workSpaceIndex[allState].Items()[sid]; ok {
		localSk, plotPk := ws.SpaceID().PlotInfo().LocalSk, ws.SpaceID().PlotInfo().PlotPublicKey
		return chiapos.NewAugSchemeMPL().SignPrepend(localSk, hash[:], plotPk)
	}
	return nil, ErrWorkSpaceDoesNotExist
}

// PlotWS should make workSpace state conversion happen like:
// registered -> plotting -> ready
// registered -> ready
// plotting -> ready
// ready -> ready
// mining -> mining
func (sk *SpaceKeeper) PlotWS(sid string) error {
	sk.stateLock.RLock()
	defer sk.stateLock.RUnlock()

	if ws, ok := sk.workSpaceIndex[allState].Get(sid); !ok || !ws.using {
		return ErrWorkSpaceDoesNotExist
	}

	// registered -> plotting -> ready
	// registered -> ready
	// TODO: check for existence in plotterQueue
	if ws, ok := sk.workSpaceIndex[engine.Registered].Get(sid); ok {
		sk.newQueuedWorkSpaceCh <- newQueuedWorkSpace(ws, false)
		return nil
	}

	// plotting -> ready
	if _, ok := sk.workSpaceIndex[engine.Plotting].Get(sid); ok {
		// known that there's no more than one plotting workSpace at the same time
		qws := sk.queue.PoppedItem()
		if qws.ws.id.String() != sid {
			return ErrWorkSpaceIsNotPlotting
		}
		qws.wouldMining = false
		return nil
	}

	// ready -> ready
	// mining -> mining
	return nil
}

// MineWS should make workSpace state conversion happen like:
// registered -> plotting -> mining
// plotting   -> mining
// ready      -> mining
// mining     -> mining
// For registered workSpace, simply push it into spacePlotter Queue with `wouldMining = true`
// For plotting workSpace, modify queuedWorkspace with `wouldMining = true`
// For ready workSpace, convert it to mining state
func (sk *SpaceKeeper) MineWS(sid string) error {
	sk.stateLock.Lock()
	defer sk.stateLock.Unlock()

	if ws, ok := sk.workSpaceIndex[allState].Get(sid); !ok || !ws.using {
		return ErrWorkSpaceDoesNotExist
	}

	// registered -> plotting -> mining
	// TODO: check for existence in plotterQueue
	if ws, ok := sk.workSpaceIndex[engine.Registered].Get(sid); ok {
		sk.newQueuedWorkSpaceCh <- newQueuedWorkSpace(ws, true)
		return nil
	}

	// plotting -> mining
	if _, ok := sk.workSpaceIndex[engine.Plotting].Get(sid); ok {
		// known that there's no more than one plotting workSpace at the same time
		qws := sk.queue.PoppedItem()
		if qws.ws.id.String() != sid {
			return ErrWorkSpaceIsNotPlotting
		}
		qws.wouldMining = true
		return nil
	}

	// ready -> mining
	if ws, ok := sk.workSpaceIndex[engine.Ready].Get(sid); ok {
		sk.workSpaceIndex[engine.Ready].Delete(sid)
		sk.workSpaceIndex[engine.Mining].Set(sid, ws)
		ws.state = engine.Mining
		return nil
	}

	// mining -> mining
	return nil
}

// StopWs should make workSpace state conversion happen like:
// registered -> registered
// plotting   -> registered
// ready      -> ready
// mining     -> ready
// For all states, clear workSpace out from spacePlotter Queue
// For plotting workSpace, stop plotting and modify queuedWorkspace with `wouldMining = false`
// For mining workSpace, convert it to ready state
func (sk *SpaceKeeper) StopWS(sid string) error {
	sk.stateLock.Lock()
	defer sk.stateLock.Unlock()

	if ws, ok := sk.workSpaceIndex[allState].Get(sid); !ok || !ws.using {
		return ErrWorkSpaceDoesNotExist
	}

	sk.queue.Delete(sid)

	if ws, ok := sk.workSpaceIndex[engine.Plotting].Get(sid); ok {
		// known that there's no more than one plotting workSpace at the same time
		qws := sk.queue.PoppedItem()
		if qws.ws.id.String() != sid {
			return ErrWorkSpaceIsNotPlotting
		}
		qws.wouldMining = false
		return ws.StopPlot()
	}

	if ws, ok := sk.workSpaceIndex[engine.Mining].Get(sid); ok {
		sk.workSpaceIndex[engine.Mining].Delete(sid)
		sk.workSpaceIndex[engine.Ready].Set(sid, ws)
		ws.state = engine.Ready
		return nil
	}

	return nil
}

// RemoveWS should only be applied on registered/ready workSpace
// WorkSpace in spaceKeeper workSpaceList would be removed
func (sk *SpaceKeeper) RemoveWS(sid string) error {
	sk.stateLock.Lock()
	defer sk.stateLock.Unlock()

	var ok bool
	var ws *WorkSpace
	if ws, ok = sk.workSpaceIndex[allState].Get(sid); !ok || !ws.using {
		return ErrWorkSpaceDoesNotExist
	}

	sk.queue.Delete(sid)

	if ws, ok = sk.workSpaceIndex[engine.Registered].Get(sid); !ok {
		if ws, ok = sk.workSpaceIndex[engine.Ready].Get(sid); !ok {
			return ErrWorkSpaceIsNotStill
		}
	}

	sk.disuseWorkSpace(ws)
	return nil
}

// DeleteWS should only be applied on registered/ready workSpace
// WorkSpace in spaceKeeper index and data in MassDB would be both deleted
func (sk *SpaceKeeper) DeleteWS(sid string) error {
	sk.stateLock.Lock()
	defer sk.stateLock.Unlock()

	var ok bool
	var ws *WorkSpace
	if ws, ok = sk.workSpaceIndex[allState].Get(sid); !ok || !ws.using {
		return ErrWorkSpaceDoesNotExist
	}

	sk.queue.Delete(sid)

	if ws, ok = sk.workSpaceIndex[engine.Registered].Get(sid); !ok {
		if ws, ok = sk.workSpaceIndex[engine.Ready].Get(sid); !ok {
			return ErrWorkSpaceIsNotStill
		}
	}

	sk.workSpaceIndex[ws.state].Delete(sid)
	sk.workSpaceIndex[allState].Delete(sid)
	sk.disuseWorkSpace(ws)

	return ws.Delete()
}

func (sk *SpaceKeeper) PlotMultiWS(flags engine.WorkSpaceStateFlags) map[string]error {
	result := make(map[string]error)
	for _, ws := range getWsByFlags(sk.workSpaceList, flags) {
		sid := ws.id.String()
		result[sid] = sk.PlotWS(sid)
	}
	return result
}

func (sk *SpaceKeeper) MineMultiWS(flags engine.WorkSpaceStateFlags) map[string]error {
	result := make(map[string]error)
	for _, ws := range getWsByFlags(sk.workSpaceList, flags) {
		sid := ws.id.String()
		result[sid] = sk.MineWS(sid)
	}
	return result
}

func (sk *SpaceKeeper) StopMultiWS(flags engine.WorkSpaceStateFlags) map[string]error {
	result := make(map[string]error)
	for _, ws := range getWsByFlags(sk.workSpaceList, flags) {
		sid := ws.id.String()
		result[sid] = sk.StopWS(sid)
	}
	return result
}

func (sk *SpaceKeeper) RemoveMultiWS(flags engine.WorkSpaceStateFlags) map[string]error {
	result := make(map[string]error)
	for _, ws := range getWsByFlags(sk.workSpaceList, flags) {
		sid := ws.id.String()
		result[sid] = sk.RemoveWS(sid)
	}
	return result
}

func (sk *SpaceKeeper) DeleteMultiWS(flags engine.WorkSpaceStateFlags) map[string]error {
	result := make(map[string]error)
	for _, ws := range getWsByFlags(sk.workSpaceList, flags) {
		sid := ws.id.String()
		result[sid] = sk.DeleteWS(sid)
	}
	return result
}

func (sk *SpaceKeeper) Configured() bool {
	return atomic.LoadInt32(&sk.configured) != 0
}

func (sk *SpaceKeeper) ResetDBDirs(dbDirs []string) error {
	if sk.Started() {
		return ErrSpaceKeeperIsRunning
	}

	absDirs := make([]string, len(dbDirs))
	for i := range dbDirs {
		absDir, err := filepath.Abs(dbDirs[i])
		if err != nil {
			return ErrInvalidDir
		}
		absDirs[i] = absDir
	}

	var strSliceEqual = func() bool {
		if len(absDirs) != len(sk.dbDirs) {
			return false
		}
		existsDir := make(map[string]struct{})
		for _, dir := range sk.dbDirs {
			existsDir[dir] = struct{}{}
		}
		for _, dir := range absDirs {
			if _, ok := existsDir[dir]; !ok {
				return false
			}
		}
		return true
	}

	if len(sk.dbDirs) == 0 {
		sk.dbDirs = absDirs
		if err := sk.generateInitialIndex(); err != nil {
			return err
		}
		return nil
	}

	if !strSliceEqual() {
		return ErrSpaceKeeperChangeDBDirs
	}

	return nil

}

// TODO: should consider pending workspaces
func (sk *SpaceKeeper) AvailableDiskSize() uint64 {
	info, err := disk.Usage(sk.dbDirs[0])
	if err != nil {
		return 0
	}
	return info.Free
}

// IsCapacityAvailable returns nil if given path is able to hold capacityBytes size of spaces.
func (sk *SpaceKeeper) IsCapacityAvailable(path string, capacityBytes uint64) error {
	if path == "" {
		path = sk.dbDirs[0]
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	info, err := disk.Usage(absPath)
	if err != nil {
		return err
	}
	freeBytes := info.Free

	var wsiList []engine.WorkSpaceInfo
	if p, ok := sk.workSpacePaths[absPath]; ok {
		for _, ws := range p.spaces {
			wsiList = append(wsiList, ws.Info())
		}
	} else if wsiList, err = peekMassDBInfosByDir(absPath, sk.dbType); err != nil {
		return err
	}

	var plottedBytes uint64
	for _, wsi := range wsiList {
		if wsi.State == engine.Ready || wsi.State == engine.Mining {
			plottedBytes += uint64(poc.ProofTypeChia.PlotSize(wsi.BitLength))
		}
	}

	if freeBytes+plottedBytes < capacityBytes {
		return ErrOSDiskSizeNotEnough
	}
	return nil
}

// TODO: consider more check items
func checkOSDiskSizeByPath(path string, requiredBytes int) error {
	if requiredBytes < 0 {
		return ErrInvalidRequiredBytes
	}
	info, err := disk.Usage(path)
	if err != nil {
		return err
	}
	if uint64(requiredBytes) >= info.Free {
		return ErrOSDiskSizeNotEnough
	}
	return nil
}

func (sk *SpaceKeeper) checkOSDiskSize(requiredBytes int) error {
	return checkOSDiskSizeByPath(sk.dbDirs[0], requiredBytes)
}

func usableBitLength() []int {
	return []int{24, 26, 28}
}

// getIndexedWorkSpaces get all indexed workSpace grouped by bitLength
// slice of workSpace is sorted by same priority as in queuedWorkSpace
func (sk *SpaceKeeper) getIndexedWorkSpaces() map[int][]*WorkSpace {
	queueMap := make(map[int]*plotterQueue)
	for _, ws := range sk.workSpaceIndex[allState].Items() {
		bl := ws.id.bitLength
		qws := newQueuedWorkSpace(ws, false)
		if queue, exists := queueMap[bl]; exists {
			queue.Push(qws, qws.priority())
		} else {
			queueMap[bl] = newPlotterQueue()
			queueMap[bl].Push(qws, qws.priority())
		}
	}

	resultMap := make(map[int][]*WorkSpace)
	for bl, queue := range queueMap {
		resultMap[bl] = make([]*WorkSpace, queue.Size())
		for i := range resultMap[bl] {
			resultMap[bl][i] = queue.PopItem().ws
		}
	}
	return resultMap
}

func (sk *SpaceKeeper) getQuality(ws *WorkSpace, challenge pocutil.Hash) []*engine.WorkSpaceQuality {
	qualities, err := ws.db.GetQualities(challenge)
	if err != nil {
		return nil
	}

	var results []*engine.WorkSpaceQuality
	for i := range qualities {
		results = append(results, &engine.WorkSpaceQuality{
			SpaceID:       ws.id.String(),
			PublicKey:     ws.id.PlotInfo().PlotPublicKey,
			Index:         uint32(i),
			KSize:         uint8(ws.SpaceID().BitLength()),
			Quality:       qualities[i],
			Error:         err,
			PoolPublicKey: ws.id.PlotInfo().PoolPublicKey,
			PlotID:        ws.id.PlotID(),
		})
	}

	return results
}

func (sk *SpaceKeeper) getQualities(wsMap map[string]*WorkSpace, challenge pocutil.Hash) map[string][]*engine.WorkSpaceQuality {
	result := make(map[string][]*engine.WorkSpaceQuality)
	resultCh := make(chan []*engine.WorkSpaceQuality)
	waitCh := make(chan struct{})
	go func() {
		for wsqs := range resultCh {
			if len(wsqs) > 0 {
				result[wsqs[0].SpaceID] = wsqs
			}
		}
		close(waitCh)
	}()

	var wg sync.WaitGroup
	for _, ws := range wsMap {
		ws0 := ws
		wg.Add(1)
		if err := sk.workerPool.Submit(func() {
			resultCh <- sk.getQuality(ws0, challenge)
			wg.Done()
		}); err != nil {
			// TODO: handle error?
			wg.Done()
		}
	}
	wg.Wait()
	close(resultCh)

	<-waitCh
	return result
}

func (sk *SpaceKeeper) getProof(ws *WorkSpace, challenge pocutil.Hash, index uint32) *engine.WorkSpaceProof {
	proof, err := ws.db.GetProof(challenge, index)
	result := &engine.WorkSpaceProof{
		SpaceID:   ws.id.String(),
		Proof:     proof,
		PublicKey: ws.id.PubKey(),
		Ordinal:   ws.id.Ordinal(),
		Error:     err,
	}
	return result
}

func (sk *SpaceKeeper) getProofs(wsMap map[string]*WorkSpace, challenge pocutil.Hash, indexes map[string]uint32) map[string]*engine.WorkSpaceProof {
	result := make(map[string]*engine.WorkSpaceProof)
	resultCh := make(chan *engine.WorkSpaceProof)
	waitCh := make(chan struct{})
	go func() {
		for wsp := range resultCh {
			result[wsp.SpaceID] = wsp
		}
		close(waitCh)
	}()

	var wg sync.WaitGroup
	for sid, ws := range wsMap {
		ws0 := ws
		index := indexes[sid]
		wg.Add(1)
		if err := sk.workerPool.Submit(func() {
			resultCh <- sk.getProof(ws0, challenge, index)
			wg.Done()
		}); err != nil {
			// TODO: handle error?
			wg.Done()
		}
	}
	wg.Wait()
	close(resultCh)

	<-waitCh
	return result
}

// useWorkSpace is not thread safe, should use lock in upper functions
func (sk *SpaceKeeper) useWorkSpace(ws *WorkSpace) {
	for _, e := range sk.workSpaceList {
		if e.id.String() == ws.id.String() {
			return
		}
	}
	sk.workSpaceList = append(sk.workSpaceList, ws)
	ws.using = true
}

// disuseWorkSpace is not thread safe, should use lock in upper functions
func (sk *SpaceKeeper) disuseWorkSpace(ws *WorkSpace) {
	ws.using = false
	sk.workSpaceList = deleteFromSlice(sk.workSpaceList, ws.id.String())
}

// addWorkSpaceToIndex is not thread safe, should use lock in upper functions
func (sk *SpaceKeeper) addWorkSpaceToIndex(ws *WorkSpace) {
	sid := ws.id.String()
	if _, ok := sk.workSpaceIndex[allState].Get(sid); ok {
		return
	}

	if p, ok := sk.workSpacePaths[ws.rootDir]; ok {
		p.Add(ws)
	} else {
		p = NewWorkSpacePath(ws.rootDir)
		p.Add(ws)
		sk.workSpacePaths[ws.rootDir] = p
	}

	sk.workSpaceIndex[allState].Set(sid, ws)
	sk.workSpaceIndex[ws.state].Set(sid, ws)
	return
}

// resetWorkSpaceList is not thread safe, should use lock in upper functions
func (sk *SpaceKeeper) applyConfiguredWorkSpaces(wsList []*WorkSpace, execPlot, execMine bool) ([]engine.WorkSpaceInfo, error) {
	if len(wsList) == 0 {
		return nil, ErrSpaceKeeperConfiguredNothing
	}
	// disuse current workSpaces & reset workSpaceList
	for _, ws := range sk.workSpaceList {
		ws.using = false
	}
	sk.workSpaceList = make([]*WorkSpace, 0)
	// push workSpaces to queue (for spacePlotter)
	sk.queue.Reset()
	tmpQueuedList := newPlotterQueue()
	for _, ws := range wsList {
		qws := newQueuedWorkSpace(ws, execMine)
		tmpQueuedList.Push(qws, qws.priority())
	}
	for !tmpQueuedList.Empty() {
		qws := tmpQueuedList.PopItem()
		sk.queue.Push(qws, qws.priority())
		sk.useWorkSpace(qws.ws)
	}
	if !(execMine || execPlot) {
		sk.queue.Reset()
	}
	// collect workSpaceInfos
	wsiList := make([]engine.WorkSpaceInfo, len(wsList))
	for i, ws := range wsList {
		wsiList[i] = ws.Info()
	}
	return wsiList, nil
}

func (sk *SpaceKeeper) ConfigureByFlags(flags engine.WorkSpaceStateFlags, execPlot, execMine bool) ([]engine.WorkSpaceInfo, error) {
	if sk.Started() {
		return nil, ErrSpaceKeeperIsRunning
	}
	if !atomic.CompareAndSwapInt32(&sk.configuring, 0, 1) {
		return nil, ErrSpaceKeeperIsConfiguring
	}
	defer atomic.StoreInt32(&sk.configuring, 0)
	atomic.StoreInt32(&sk.configured, 0)

	resultList := make([]*WorkSpace, 0)
	for _, state := range flags.States() {
		m := sk.workSpaceIndex[state].Items()
		for _, ws := range m {
			resultList = append(resultList, ws)
		}
	}

	wsiList, err := sk.applyConfiguredWorkSpaces(resultList, execPlot, execMine)
	if err != nil {
		logging.CPrint(logging.ERROR, "fail on ConfigureByFlags", logging.LogFormat{
			"flags":     flags,
			"exec_plot": execPlot,
			"exec_mine": execMine,
			"err":       err,
		})
		return nil, err
	}
	atomic.StoreInt32(&sk.configured, 1)
	return wsiList, nil
}

func (sk *SpaceKeeper) WorkSpaceInfosByDirs() (dirs []string, results [][]engine.WorkSpaceInfo, err error) {
	sk.stateLock.RLock()
	defer sk.stateLock.RUnlock()

	for _, dir := range sk.dbDirs {
		p, ok := sk.workSpacePaths[dir]
		if !ok {
			continue
		}
		wsList := p.WorkSpaces()
		infos := make([]engine.WorkSpaceInfo, 0, len(wsList))
		for _, ws := range wsList {
			if ws.using {
				infos = append(infos, ws.Info())
			}
		}
		dirs = append(dirs, dir)
		results = append(results, infos)
	}
	return
}

func deleteFromSlice(src []*WorkSpace, sid string) []*WorkSpace {
	if len(src) == 0 {
		return src
	}

	var idx int
	var exists bool
	for i, ws := range src {
		if ws.id.String() == sid {
			idx, exists = i, true
			break
		}
	}

	if !exists {
		return src
	}

	result := make([]*WorkSpace, len(src)-1)
	copy(result, src[:idx])
	copy(result[idx:], src[idx+1:])
	return result
}

func getWsByID(src []*WorkSpace, sid string) (*WorkSpace, bool) {
	for _, ws := range src {
		if ws.id.String() == sid {
			return ws, true
		}
	}
	return nil, false
}

func getWsByFlags(src []*WorkSpace, flags engine.WorkSpaceStateFlags) []*WorkSpace {
	states := make(map[engine.WorkSpaceState]bool)
	for _, state := range flags.States() {
		states[state] = true
	}
	result := make([]*WorkSpace, 0, len(src))
	for _, ws := range src {
		if states[ws.state] {
			result = append(result, ws)
		}
	}
	return result
}
