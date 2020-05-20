package capacity

import (
	"bytes"

	"github.com/orcaman/concurrent-map"
	"massnet.org/mass/poc/engine"
	"massnet.org/mass/poc/engine/massdb"
	"massnet.org/mass/poc/pocutil"
	"massnet.org/mass/pocec"
)

type WorkSpace struct {
	id      *SpaceID
	db      massdb.MassDB
	state   engine.WorkSpaceState
	using   bool
	rootDir string
}

// NewWorkSpace loads MassDB from given rootDir with PubKey&BitLength,
// and set proper state for WorkSpace by its progress
// To prevent accident, double check PubKey&BitLength on loaded MassDB
// If MassDB does not exist, create new MassDB.
func NewWorkSpace(dbType string, rootDir string, ordinal int64, pubKey *pocec.PublicKey, bitLength int) (*WorkSpace, error) {
	mdb, err := massdb.OpenDB(dbType, rootDir, ordinal, pubKey, bitLength)
	if err != nil {
		if err != massdb.ErrDBDoesNotExist {
			return nil, err
		}
		mdb, err = massdb.CreateDB(dbType, rootDir, ordinal, pubKey, bitLength)
		if err != nil {
			return nil, err
		}
	}

	if !bytes.Equal(pubKey.SerializeCompressed(), mdb.PubKey().SerializeCompressed()) ||
		bitLength != mdb.BitLength() {
		return nil, ErrMassDBDoesNotMatchWithName
	}

	ws := &WorkSpace{
		db:      mdb,
		state:   engine.Registered,
		id:      NewSpaceID(ordinal, mdb.PubKey(), mdb.BitLength()),
		rootDir: rootDir,
	}

	if _, plotted, _ := mdb.Progress(); plotted {
		ws.state = engine.Ready
	}

	return ws, nil
}

func (ws *WorkSpace) Info() engine.WorkSpaceInfo {
	return engine.WorkSpaceInfo{
		SpaceID:   ws.id.String(),
		PublicKey: ws.id.PubKey(),
		Ordinal:   ws.id.Ordinal(),
		BitLength: ws.id.BitLength(),
		Progress:  ws.Progress(),
		State:     ws.state,
	}
}

func (ws *WorkSpace) PubKey() *pocec.PublicKey {
	return ws.db.PubKey()
}

func (ws *WorkSpace) PubKeyHash() pocutil.Hash {
	return ws.db.PubKeyHash()
}

func (ws *WorkSpace) BitLength() int {
	return ws.db.BitLength()
}

func (ws *WorkSpace) SpaceID() *SpaceID {
	return ws.id
}

func (ws *WorkSpace) State() engine.WorkSpaceState {
	return ws.state
}

func (ws *WorkSpace) Progress() float64 {
	_, _, progress := ws.db.Progress()
	return progress
}

func (ws *WorkSpace) Plot() error {
	result := ws.db.Plot()
	return <-result
}

func (ws *WorkSpace) StopPlot() error {
	result := ws.db.StopPlot()
	return <-result
}

func (ws *WorkSpace) Delete() error {
	result := ws.db.Delete()
	return <-result
}

type WorkSpaceMap struct {
	m cmap.ConcurrentMap
}

func NewWorkSpaceMap() *WorkSpaceMap {
	return &WorkSpaceMap{
		m: cmap.New(),
	}
}

func (m *WorkSpaceMap) Get(sid string) (*WorkSpace, bool) {
	v, ok := m.m.Get(sid)
	if !ok {
		return nil, false
	}
	return v.(*WorkSpace), ok
}

func (m *WorkSpaceMap) Set(sid string, ws *WorkSpace) {
	m.m.Set(sid, ws)
}

func (m *WorkSpaceMap) Has(sid string) bool {
	return m.m.Has(sid)
}

func (m *WorkSpaceMap) Delete(sid string) {
	m.m.Remove(sid)
}

func (m *WorkSpaceMap) Items() map[string]*WorkSpace {
	mi := m.m.Items()
	mws := make(map[string]*WorkSpace)
	for sid, ws := range mi {
		mws[sid] = ws.(*WorkSpace)
	}
	return mws
}

func (m *WorkSpaceMap) Count() int {
	return m.m.Count()
}
