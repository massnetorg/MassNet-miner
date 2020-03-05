package orphanpool_test

import (
	"crypto/rand"
	"encoding/hex"
	"reflect"
	"testing"

	"massnet.org/mass/blockchain/orphanpool"
)

type testEntry struct {
	id      string
	parents []string
}

func (entry *testEntry) OrphanPoolID() string {
	return entry.id
}

func mockID() string {
	var b32 [32]byte
	if n, _ := rand.Read(b32[:]); n != 32 {
		panic("fail to read entropy")
	}
	return hex.EncodeToString(b32[:])
}

func mockTestEntry(n int) *testEntry {
	entry := &testEntry{
		id:      mockID(),
		parents: make([]string, n),
	}
	for i := range entry.parents {
		entry.parents[i] = mockID()
	}
	return entry
}

func constructOrphanPool(count int) (*orphanpool.AbstractOrphanPool, []*testEntry) {
	aop := orphanpool.NewAbstractOrphanPool()

	orphans := make([]*testEntry, count)
	for i := range orphans {
		entry := mockTestEntry(count - i)
		orphans[count-i-1] = entry
		entry.parents[count-i-1] = orphans[count-1].parents[count-i-1]
		aop.Put(entry, entry.parents)
	}

	return aop, orphans
}

func TestNewAbstractOrphanPool(t *testing.T) {
	var count = 5
	aop, orphans := constructOrphanPool(count)

	if realCount := aop.Count(); realCount != count {
		t.Errorf("count error: expect %d, but got %d", count, realCount)
	}

	for i, orphan := range orphans {
		if _, exists := aop.Read(orphan.id); !exists {
			t.Errorf("read error in %d: expect %v, but got nothing", i, orphan)
		}
	}
}

func TestAbstractOrphanPool_Read(t *testing.T) {
	var count = 5
	aop, orphans := constructOrphanPool(count)

	testRead(t, aop, orphans)
}

func testRead(t *testing.T, aop *orphanpool.AbstractOrphanPool, orphans []*testEntry) {
	for i, orphan := range orphans {
		entry, exists := aop.Read(orphan.id)
		if !exists {
			t.Errorf("read error in %d: expect %v, but got nothing", i, orphan)
		}
		if rOrphan := entry.(*testEntry); !reflect.DeepEqual(rOrphan, orphan) {
			t.Errorf("read error in %d: expect %v, but got %v", i, orphan, rOrphan)
		}
	}
}

func TestAbstractOrphanPool_ReadSubs(t *testing.T) {
	var count = 5
	aop, orphans := constructOrphanPool(count)

	testReadSubs(t, aop, orphans)
}

func testReadSubs(t *testing.T, aop *orphanpool.AbstractOrphanPool, orphans []*testEntry) {
	for i, orphan := range orphans {
		orphanID := orphan.id

	loop:
		for j, parent := range orphan.parents {
			entries := aop.ReadSubs(parent)
			if len(entries) == 0 {
				t.Errorf("read subs error in %d - %d: expect %v, but got nothing in %v", i, j, orphan, parent)
			}

			for _, entry := range entries {
				if entry.OrphanPoolID() == orphanID {
					break loop
				}
			}
			t.Errorf("read subs error in %d - %d: expect %v, but not found in %v", i, j, orphan, parent)
		}
	}
}

func TestAbstractOrphanPool_Fetch(t *testing.T) {
	var count = 5
	aop, orphans := constructOrphanPool(count)

	for i, orphan := range orphans {
		if entry, exists := aop.Fetch(orphan.id); !exists {
			t.Errorf("fetch error in %d: expect %v, but got nothing", i, orphan)
		} else if rOrphan := entry.(*testEntry); !reflect.DeepEqual(rOrphan, orphan) {
			t.Errorf("fetch error in %d: expect %v but got %v", i, orphan, rOrphan)
		}

		if _, ok := aop.Read(orphan.id); ok {
			t.Errorf("fetch error in %d: expect removed from aop, but still got %v", i, orphan)
		}

		for j, parent := range orphan.parents {
			for k, sub := range aop.ReadSubs(parent) {
				if sub.OrphanPoolID() == orphan.id {
					t.Errorf(" fetch error in %d - %d - %d: expect removed from aop subs, but still got %v", i, j, k, orphan)
				}
			}
		}

		testRead(t, aop, orphans[i+1:])
		testReadSubs(t, aop, orphans[i+1:])
	}
}

func TestAbstractOrphanPool_FetchSubs(t *testing.T) {
	var count = 5
	aop, orphans := constructOrphanPool(count)

	aop.FetchSubs(orphans[count-1].parents[0])

	testRead(t, aop, orphans[1:count-1])
	testReadSubs(t, aop, orphans[1:count-1])
}

func TestAbstractOrphanPool_Put(t *testing.T) {
	var count = 5
	aop, orphans := constructOrphanPool(count)

	aop.FetchSubs(orphans[count-1].parents[0])
	aop.Put(orphans[0], orphans[0].parents)
	aop.Put(orphans[count-1], orphans[count-1].parents)

	testRead(t, aop, orphans[1:count-1])
	testReadSubs(t, aop, orphans[1:count-1])
}

func TestAbstractOrphanPool_IDs(t *testing.T) {
	var count = 5
	aop, _ := constructOrphanPool(count)

	if number := len(aop.IDs()); number != count {
		t.Errorf("ids error: expect %d, but got %d", count, number)
	}
}

func TestAbstractOrphanPool_Items(t *testing.T) {
	var count = 5
	aop, _ := constructOrphanPool(count)

	items := aop.Items()
	orphans := make([]*testEntry, len(items))
	var i = 0
	for _, item := range items {
		orphans[i] = item.(*testEntry)
		i++
	}

	testRead(t, aop, orphans)
}

func TestAbstractOrphanPool_Has(t *testing.T) {
	var count = 5
	aop, orphans := constructOrphanPool(count)

	for i, orphan := range orphans {
		if !aop.Has(orphan.id) {
			t.Errorf("has error in %d: expect %v, but got nothing", i, orphan)
		}
		for j, parent := range orphan.parents {
			if aop.Has(parent) {
				t.Errorf("hash error in %d - %d: expect nothing, but got %v", i, j, parent)
			}
		}
	}
}

func TestAbstractOrphanPool_ChainRelation(t *testing.T) {
	aop := orphanpool.NewAbstractOrphanPool()

	entry0 := mockTestEntry(1)
	entry1 := mockTestEntry(2)
	entry0.parents[0] = entry1.id

	aop.Put(entry1, entry1.parents)
	aop.Put(entry0, entry0.parents)
	aop.Fetch(entry1.id)

	testRead(t, aop, []*testEntry{entry0})
	testReadSubs(t, aop, []*testEntry{entry0})
}

//var showRelation = func() map[string][]int {
//	result := make(map[string][]int)
//	ids := make(map[string]int)
//
//	for i, orphan := range orphans {
//		ids[orphan.id] = i + 1
//	}
//
//	for parent, subs := range aop.priorToSubIndex {
//		arr := make([]int, len(subs))
//		for i, sub := range subs {
//			arr[i] = ids[sub.OrphanPoolID()]
//		}
//		result[parent] = arr
//	}
//
//	return result
//}
//
//for k, v := range showRelation() {
//	fmt.Println(k, v)
//}
