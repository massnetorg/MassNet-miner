package blockchain

import (
	"bytes"
	"sort"
	"sync"

	"massnet.org/mass/massutil"
	"massnet.org/mass/wire"
)

type PunishmentProposal struct {
	Size      int
	BitLength int
	Height    uint64
	Proposal  *wire.FaultPubKey
}

func (punishment *PunishmentProposal) LessThan(other *PunishmentProposal) bool {
	if punishment.BitLength < other.BitLength {
		return true
	} else if punishment.BitLength == other.BitLength {
		if punishment.Height > other.Height {
			return true
		} else if punishment.Height == other.Height {
			return punishment.Size > other.Size
		} else {
			return false
		}
	} else {
		return false
	}
}

func (punishment *PunishmentProposal) SetFromFaultPk(fpk *wire.FaultPubKey) *PunishmentProposal {
	punishment.Proposal = fpk
	punishment.Size = fpk.PlainSize()
	punishment.BitLength = fpk.Testimony[0].Proof.BitLength
	punishment.Height = fpk.Testimony[0].Height

	return punishment
}

type PunishmentProposalList []*PunishmentProposal

func (fpkList PunishmentProposalList) Len() int {
	return len(fpkList)
}

func (fpkList PunishmentProposalList) Less(i, j int) bool {
	return fpkList[i].LessThan(fpkList[j])
}

func (fpkList PunishmentProposalList) Swap(i, j int) {
	fpkList[i], fpkList[j] = fpkList[j], fpkList[i]
}

func sortPunishmentProposals(punishmentList PunishmentProposalList) {
	sort.Sort(sort.Reverse(punishmentList))
}

type ProposalPool struct {
	sync.RWMutex
	punishmentProposalList []*PunishmentProposal
}

func NewProposalPool(fpkList []*wire.FaultPubKey) *ProposalPool {
	PunishProposals := make([]*PunishmentProposal, len(fpkList))

	for i, fpk := range fpkList {
		tpp := new(PunishmentProposal)
		tpp.SetFromFaultPk(fpk)
		PunishProposals[i] = tpp
	}
	sortPunishmentProposals(PunishProposals)

	return &ProposalPool{
		punishmentProposalList: PunishProposals,
	}
}

func (pp *ProposalPool) PunishmentProposals() []*PunishmentProposal {
	return pp.punishmentProposalList
}

func (pp *ProposalPool) insertFaultPubKey(fpk *wire.FaultPubKey) {
	punishment := new(PunishmentProposal)
	punishment.SetFromFaultPk(fpk)

	var insert = func(tpp *PunishmentProposal, i int) {
		pp.punishmentProposalList = append(pp.punishmentProposalList[:i+1], pp.punishmentProposalList[i:]...)
		pp.punishmentProposalList[i] = tpp
	}

	var inserted bool
	for i, elmt := range pp.punishmentProposalList {
		if elmt.LessThan(punishment) {
			insert(punishment, i)
			inserted = true
			break
		}
	}
	if !inserted {
		pp.punishmentProposalList = append(pp.punishmentProposalList, punishment)
	}
}

func (pp *ProposalPool) InsertFaultPubKey(fpk *wire.FaultPubKey) {
	pp.insertFaultPubKey(fpk)
}

func (pp *ProposalPool) insertFaultPubKeys(fpkList []*wire.FaultPubKey) {
	for _, fpk := range fpkList {
		pp.insertFaultPubKey(fpk)
	}
}

func (pp *ProposalPool) removeFaultPubKey(fpk *wire.FaultPubKey) {
	fpkBytes := fpk.PubKey.SerializeCompressed()
	fpkBitLength := fpk.Testimony[0].Proof.BitLength

	var SameFpk = func(tpp *PunishmentProposal) bool {
		if bytes.Equal(fpkBytes, tpp.Proposal.PubKey.SerializeCompressed()) &&
			fpkBitLength == tpp.BitLength {
			return true
		} else {
			return false
		}
	}

	for i, tpp := range pp.punishmentProposalList {
		if SameFpk(tpp) {
			pp.punishmentProposalList = append(pp.punishmentProposalList[:i], pp.punishmentProposalList[i+1:]...)
			return
		}
	}
}

func (pp *ProposalPool) removeFaultPubKeys(fpkList []*wire.FaultPubKey) {
	for _, fpk := range fpkList {
		pp.removeFaultPubKey(fpk)
	}
}

func (pp *ProposalPool) SyncAttachBlock(block *massutil.Block) {
	pp.Lock()
	defer pp.Unlock()

	pp.removeFaultPubKeys(block.MsgBlock().Proposals.PunishmentArea)
}

func (pp *ProposalPool) SyncDetachBlock(block *massutil.Block) {
	pp.Lock()
	defer pp.Unlock()

	pp.insertFaultPubKeys(block.MsgBlock().Proposals.PunishmentArea)
}
