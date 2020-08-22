package blockchain

import (
	"container/list"
	"encoding/hex"
	"sync"

	"massnet.org/mass/massutil"
	"massnet.org/mass/pocec"
	"massnet.org/mass/wire"
)

type ProposalPool struct {
	sync.RWMutex
	punishments *punishmentProposalList
}

func NewProposalPool(faults []*wire.FaultPubKey) *ProposalPool {
	return &ProposalPool{
		punishments: newPunishmentProposalList(faults),
	}
}

func (pp *ProposalPool) SyncAttachBlock(block *massutil.Block) {
	pp.Lock()
	defer pp.Unlock()
	pp.punishments.removeFaultPubKeys(block.MsgBlock().Proposals.PunishmentArea...)
}

func (pp *ProposalPool) SyncDetachBlock(block *massutil.Block) {
	pp.Lock()
	defer pp.Unlock()
	pp.punishments.insertFaultPubKeys(block.MsgBlock().Proposals.PunishmentArea...)
}

func (pp *ProposalPool) InsertFaultPubKey(fpk *wire.FaultPubKey) {
	pp.Lock()
	defer pp.Unlock()
	pp.punishments.insertFaultPubKeys(fpk)
}

func (pp *ProposalPool) PunishmentProposals() []*PunishmentProposal {
	pp.RLock()
	defer pp.RUnlock()
	return pp.punishments.slice()
}

// PunishmentProposal warps wire.FaultPubKey
type PunishmentProposal struct {
	*wire.FaultPubKey
	BitLength int
	Height    uint64
	Size      int
}

func NewPunishmentProposal(fpk *wire.FaultPubKey) *PunishmentProposal {
	return new(PunishmentProposal).setFaultPubKey(fpk)
}

func (p *PunishmentProposal) setFaultPubKey(fpk *wire.FaultPubKey) *PunishmentProposal {
	p.FaultPubKey = fpk
	p.BitLength = fpk.Testimony[0].Proof.BitLength
	p.Height = fpk.Testimony[0].Height
	p.Size = fpk.PlainSize()
	return p
}

func (p *PunishmentProposal) LessThan(other *PunishmentProposal) bool {
	if p.BitLength != other.BitLength {
		return p.BitLength < other.BitLength
	}
	if p.Height != other.Height {
		return p.Height > other.Height
	}
	return p.Size > other.Size
}

type punishmentProposalList struct {
	list     *list.List               // linked list to store punishment proposals
	elements map[string]*list.Element // public_key_string -> *list.Element
}

func newPunishmentProposalList(faults []*wire.FaultPubKey) *punishmentProposalList {
	l := &punishmentProposalList{
		list:     list.New(),
		elements: map[string]*list.Element{},
	}
	l.insertFaultPubKeys(faults...)
	return l
}

func (l *punishmentProposalList) slice() []*PunishmentProposal {
	results := make([]*PunishmentProposal, 0, len(l.elements))
	for e := l.list.Front(); e != nil; e = e.Next() {
		results = append(results, e.Value.(*PunishmentProposal))
	}
	return results
}

func (l *punishmentProposalList) insertFaultPubKeys(faults ...*wire.FaultPubKey) {
	for i := range faults {
		l.insert(NewPunishmentProposal(faults[i]))
	}
}

func (l *punishmentProposalList) removeFaultPubKeys(faults ...*wire.FaultPubKey) {
	for i := range faults {
		l.remove(faults[i].PubKey)
	}
}

func (l *punishmentProposalList) insert(new *PunishmentProposal) {
	pkStr := hex.EncodeToString(new.PubKey.SerializeCompressed())
	old, exists := l.elements[pkStr]
	if exists {
		// preserve better proposal, remove duplicate pubKeys
		if !old.Value.(*PunishmentProposal).LessThan(new) {
			return
		}
		l.list.Remove(old)
		delete(l.elements, pkStr)
	}
	// insert to empty list
	if l.list.Len() == 0 {
		l.elements[pkStr] = l.list.PushFront(new)
		return
	}
	// insert to the tail of list
	if tail := l.list.Back().Value.(*PunishmentProposal); !tail.LessThan(new) {
		l.elements[pkStr] = l.list.PushBack(new)
		return
	}
	// insert to somewhere else in the list
	for e := l.list.Front(); e != nil; e = e.Next() {
		if e.Value.(*PunishmentProposal).LessThan(new) {
			l.elements[pkStr] = l.list.InsertBefore(new, e)
			break
		}
	}
}

func (l *punishmentProposalList) remove(pubKey *pocec.PublicKey) {
	pkStr := hex.EncodeToString(pubKey.SerializeCompressed())
	old, exists := l.elements[pkStr]
	if !exists {
		return
	}
	l.list.Remove(old)
	delete(l.elements, pkStr)
}
