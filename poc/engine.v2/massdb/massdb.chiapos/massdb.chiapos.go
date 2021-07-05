package massdb_chiapos

import (
	"sync"

	"github.com/massnetorg/mass-core/poc/chiapos"
	"github.com/massnetorg/mass-core/poc/pocutil"
	"massnet.org/mass/poc/engine.v2/massdb"
)

const TypeMassDBChiaPoS = "chiapos"

type MassDBChiaPoS struct {
	l  sync.Mutex
	dp *chiapos.DiskProver
}

// Get type of MassDB
func (mdb *MassDBChiaPoS) Type() string {
	return TypeMassDBChiaPoS
}

// Close MassDB
func (mdb *MassDBChiaPoS) Close() error {
	mdb.l.Lock()
	defer mdb.l.Unlock()
	return mdb.dp.Close()
}

// Is MassDB loaded and plotted
func (mdb *MassDBChiaPoS) Ready() bool {
	return true
}

// Get bitLength of MassDB
func (mdb *MassDBChiaPoS) BitLength() int {
	return int(mdb.dp.Size())
}

// Get PubKey of MassDB
func (mdb *MassDBChiaPoS) ID() [32]byte {
	return mdb.dp.ID()
}

func (mdb *MassDBChiaPoS) PlotInfo() *chiapos.PlotInfo {
	return mdb.dp.PlotInfo()
}

// Get qualities by challenge
func (mdb *MassDBChiaPoS) GetQualities(challenge pocutil.Hash) (qualities [][]byte, err error) {
	if pass := chiapos.PassPlotFilter(mdb.ID(), challenge); !pass {
		return nil, massdb.ErrNotPassingPlotFilter
	}
	posChallenge := chiapos.CalculatePosChallenge(mdb.ID(), challenge)
	return mdb.dp.GetQualitiesForChallenge(posChallenge)
}

func (mdb *MassDBChiaPoS) GetProof(challenge pocutil.Hash, index uint32) (*chiapos.ProofOfSpace, error) {
	if pass := chiapos.PassPlotFilter(mdb.ID(), challenge); !pass {
		return nil, massdb.ErrNotPassingPlotFilter
	}
	posChallenge := chiapos.CalculatePosChallenge(mdb.ID(), challenge)
	proof, err := mdb.dp.GetFullProof(posChallenge, index)
	if err != nil {
		return nil, err
	}
	info := mdb.PlotInfo()
	return &chiapos.ProofOfSpace{
		Challenge:     posChallenge,
		PoolPublicKey: info.PoolPublicKey,
		PuzzleHash:    info.PuzzleHash,
		PlotPublicKey: info.PlotPublicKey,
		KSize:         mdb.dp.Size(),
		Proof:         proof,
	}, err
}

func OpenDB(args ...interface{}) (massdb.MassDB, error) {
	filename, err := parseArgs(args...)
	if err != nil {
		return nil, err
	}
	dp, err := chiapos.NewDiskProver(filename, true)
	if err != nil {
		return nil, err
	}
	return &MassDBChiaPoS{dp: dp}, nil
}

func parseArgs(args ...interface{}) (string, error) {
	if len(args) != 1 {
		return "", massdb.ErrInvalidDBArgs
	}
	filename, ok := args[0].(string)
	if !ok {
		return "", massdb.ErrInvalidDBArgs
	}
	return filename, nil
}

func init() {
	massdb.AddDBBackend(massdb.DBBackend{
		Typ:    TypeMassDBChiaPoS,
		OpenDB: OpenDB,
	})
}
