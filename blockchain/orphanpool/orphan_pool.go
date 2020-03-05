package orphanpool

import (
	"sync"
)

type PoolEntry interface {
	OrphanPoolID() string
}

type AbstractOrphanPool struct {
	sync.RWMutex
	entryIndex      map[string]PoolEntry   // ID to Entry
	priorToSubIndex map[string][]PoolEntry // Parent to Entries
	subToPriorIndex map[PoolEntry][]string // Entry to Parents
}

func NewAbstractOrphanPool() *AbstractOrphanPool {
	return &AbstractOrphanPool{
		entryIndex:      make(map[string]PoolEntry),
		priorToSubIndex: make(map[string][]PoolEntry),
		subToPriorIndex: make(map[PoolEntry][]string),
	}
}

func (aop *AbstractOrphanPool) Read(id string) (entry PoolEntry, exists bool) {
	aop.RLock()
	defer aop.RUnlock()

	entry, exists = aop.entryIndex[id]
	return entry, exists
}

func (aop *AbstractOrphanPool) ReadSubs(id string) (entries []PoolEntry) {
	aop.RLock()
	defer aop.RUnlock()

	var exists bool
	entries, exists = aop.priorToSubIndex[id]
	if !exists {
		entries = make([]PoolEntry, 0)
	}
	return entries
}

func (aop *AbstractOrphanPool) Fetch(id string) (entry PoolEntry, exists bool) {
	aop.Lock()
	defer aop.Unlock()

	return aop.fetch(id)
}

func (aop *AbstractOrphanPool) FetchSubs(pid string) (entries []PoolEntry, exists bool) {
	aop.Lock()
	defer aop.Unlock()

	entries, exists = aop.priorToSubIndex[pid]
	if !exists {
		return entries, exists
	}

	for _, entry := range entries {
		aop.fetch(entry.OrphanPoolID())
	}
	delete(aop.priorToSubIndex, pid)

	return entries, exists
}

func (aop *AbstractOrphanPool) Put(entry PoolEntry, priors []string) {
	aop.Lock()
	defer aop.Unlock()

	var id = entry.OrphanPoolID()

	for _, pid := range priors {
		if subs, exists := aop.priorToSubIndex[pid]; exists {
			aop.priorToSubIndex[pid] = append(subs, entry)
		} else {
			aop.priorToSubIndex[pid] = []PoolEntry{entry}
		}
	}

	aop.subToPriorIndex[entry] = append([]string{}, priors...)
	aop.entryIndex[id] = entry
}

func (aop *AbstractOrphanPool) Has(id string) bool {
	aop.RLock()
	defer aop.RUnlock()

	_, exists := aop.entryIndex[id]
	return exists
}

func (aop *AbstractOrphanPool) IDs() []string {
	aop.RLock()
	defer aop.RUnlock()

	var count = len(aop.entryIndex)
	result := make([]string, count)

	var i = 0
	for id := range aop.entryIndex {
		result[i] = id
		i++
	}

	return result
}

func (aop *AbstractOrphanPool) Items() map[string]PoolEntry {
	aop.RLock()
	defer aop.RUnlock()

	result := make(map[string]PoolEntry)

	for id, entry := range aop.entryIndex {
		result[id] = entry
	}

	return result
}

func (aop *AbstractOrphanPool) Count() int {
	aop.RLock()
	defer aop.RUnlock()

	return len(aop.entryIndex)
}

func (aop *AbstractOrphanPool) fetch(id string) (entry PoolEntry, exists bool) {
	entry, exists = aop.entryIndex[id]
	if !exists {
		return entry, exists
	}

	if priors, ok := aop.subToPriorIndex[entry]; ok {
		for _, pid := range priors {
			aop.priorToSubIndex[pid] = aop.deleteFromSlice(aop.priorToSubIndex[pid], entry)
		}
		delete(aop.subToPriorIndex, entry)
	}
	delete(aop.entryIndex, id)

	return entry, exists
}

func (aop *AbstractOrphanPool) deleteFromSlice(entries []PoolEntry, entry PoolEntry) []PoolEntry {
	var index int
	for index = range entries {
		if entries[index] == entry {
			break
		}
	}

	newSlice := make([]PoolEntry, len(entries)-1)
	copy(newSlice, entries[:index])
	copy(newSlice[index:], entries[index+1:])

	return newSlice
}
