package blockchain

import (
	"testing"

	"massnet.org/mass/pocec"
	"massnet.org/mass/wire"
)

func TestPunishmentProposal_LessThan(t *testing.T) {
	tests := []struct {
		BitLength [2]int
		Height    [2]uint64
		Size      [2]int
		LessThan  bool
	}{
		{
			BitLength: [2]int{32, 32},
			Height:    [2]uint64{10, 10},
			Size:      [2]int{9, 10},
			LessThan:  false,
		},
		{
			BitLength: [2]int{32, 32},
			Height:    [2]uint64{10, 10},
			Size:      [2]int{10, 10},
			LessThan:  false,
		},
		{
			BitLength: [2]int{32, 32},
			Height:    [2]uint64{10, 10},
			Size:      [2]int{11, 10},
			LessThan:  true,
		},
		{
			BitLength: [2]int{32, 32},
			Height:    [2]uint64{9, 10},
			Size:      [2]int{11, 10},
			LessThan:  false,
		},
		{
			BitLength: [2]int{32, 32},
			Height:    [2]uint64{11, 10},
			Size:      [2]int{9, 10},
			LessThan:  true,
		},
		{
			BitLength: [2]int{30, 32},
			Height:    [2]uint64{9, 10},
			Size:      [2]int{9, 10},
			LessThan:  true,
		},
		{
			BitLength: [2]int{34, 32},
			Height:    [2]uint64{11, 10},
			Size:      [2]int{11, 10},
			LessThan:  false,
		},
	}

	for i, test := range tests {
		p0 := &PunishmentProposal{
			BitLength: test.BitLength[0],
			Height:    test.Height[0],
			Size:      test.Size[0],
		}
		p1 := &PunishmentProposal{
			BitLength: test.BitLength[1],
			Height:    test.Height[1],
			Size:      test.Size[1],
		}
		less := p0.LessThan(p1)
		if less != test.LessThan {
			t.Errorf("mistched LessThan result, %d, expected: %v, got: %v", i, test.LessThan, less)
		}
	}
}

func tstMockPunishmentProposal(bl int, pubKey *pocec.PublicKey) *PunishmentProposal {
	h0, h1 := wire.MockHeader(), wire.MockHeader()
	fault := &wire.FaultPubKey{
		PubKey:    h0.PubKey,
		Testimony: [2]*wire.BlockHeader{h0, h1},
	}
	if pubKey != nil {
		fault.PubKey = pubKey
	}
	punish := NewPunishmentProposal(fault)
	punish.BitLength = bl
	punish.Height, punish.Size = 100, 1600
	return punish
}

func tstExtractPunishmentProposals(punishes []*PunishmentProposal, indexes []int) []*PunishmentProposal {
	if len(indexes) == 0 {
		return []*PunishmentProposal{}
	}
	results := make([]*PunishmentProposal, 0, len(indexes))
	for _, idx := range indexes {
		results = append(results, punishes[idx])
	}
	return results
}

func TestPunishmentProposalList_Insert_Order(t *testing.T) {
	tests := []struct {
		BitLengths   []int
		ExpectedList [][]int
	}{
		{
			BitLengths:   []int{24, 26, 28},
			ExpectedList: [][]int{{0}, {1, 0}, {2, 1, 0}},
		},
		{
			BitLengths:   []int{24, 24, 24},
			ExpectedList: [][]int{{0}, {0, 1}, {0, 1, 2}},
		},
		{
			BitLengths:   []int{28, 26, 24},
			ExpectedList: [][]int{{0}, {0, 1}, {0, 1, 2}},
		},
		{
			BitLengths:   []int{28, 24, 26},
			ExpectedList: [][]int{{0}, {0, 1}, {0, 2, 1}},
		},
	}

	for i, test := range tests {
		var punishes []*PunishmentProposal
		var l = newPunishmentProposalList(nil)
		for j, bl := range test.BitLengths {
			punish := tstMockPunishmentProposal(bl, nil)
			punishes = append(punishes, punish)
			l.insert(punish)
			sli := l.slice()
			// check list length
			if len(sli) != len(test.ExpectedList[j]) {
				t.Errorf("wrong list length, %d, %d, expected %d, got %d",
					i, j, len(sli), len(test.ExpectedList[j]))
			}
			// check expected order
			for k, e := range sli {
				if e != punishes[test.ExpectedList[j][k]] {
					t.Errorf("wrong list order, %d, %d, %d, expected %v, got: %v",
						i, j, k, tstExtractPunishmentProposals(punishes, test.ExpectedList[j]), sli)
				}
			}
		}
	}
}

func TestPunishmentProposalList_Insert_Duplicate(t *testing.T) {
	tests := []struct {
		BitLengths   []int
		Duplicates   [2]int
		ExpectedList [][]int
	}{
		{
			BitLengths:   []int{24, 26, 26, 28},
			Duplicates:   [2]int{0, 1},
			ExpectedList: [][]int{{0}, {1}, {1, 2}, {3, 1, 2}},
		},
		{
			BitLengths:   []int{24, 24, 24},
			Duplicates:   [2]int{0, 2},
			ExpectedList: [][]int{{0}, {0, 1}, {0, 1}},
		},
		{
			BitLengths:   []int{28, 26, 24},
			Duplicates:   [2]int{1, 2},
			ExpectedList: [][]int{{0}, {0, 1}, {0, 1}},
		},
		{
			BitLengths:   []int{30, 24, 26, 28},
			Duplicates:   [2]int{1, 3},
			ExpectedList: [][]int{{0}, {0, 1}, {0, 2, 1}, {0, 3, 2}},
		},
	}

	for i, test := range tests {
		var punishes []*PunishmentProposal
		var l = newPunishmentProposalList(nil)
		for j, bl := range test.BitLengths {
			var punish *PunishmentProposal
			if j == test.Duplicates[1] {
				punish = tstMockPunishmentProposal(bl, punishes[test.Duplicates[0]].PubKey)
			} else {
				punish = tstMockPunishmentProposal(bl, nil)
			}
			punishes = append(punishes, punish)
			l.insert(punish)
			sli := l.slice()
			// check list length
			if len(sli) != len(test.ExpectedList[j]) {
				t.Errorf("wrong list length, %d, %d, expected %d, got %d",
					i, j, len(test.ExpectedList[j]), len(sli))
			}
			// check expected order
			for k, e := range sli {
				if e != punishes[test.ExpectedList[j][k]] {
					t.Errorf("wrong list order, %d, %d, %d, expected %v, got: %v",
						i, j, k, tstExtractPunishmentProposals(punishes, test.ExpectedList[j]), sli)
				}
			}
		}
	}
}

func TestPunishmentProposalList_Remove(t *testing.T) {
	tests := []struct {
		BitLengths   []int
		Removes      []int
		ExpectedList [][]int
	}{
		{
			BitLengths:   []int{28, 26, 24},
			Removes:      []int{0, 1, 2},
			ExpectedList: [][]int{{1, 2}, {2}, {}},
		},
		{
			BitLengths:   []int{28, 26, 24},
			Removes:      []int{0, 2, 1},
			ExpectedList: [][]int{{1, 2}, {1}, {}},
		},
		{
			BitLengths:   []int{28, 26, 24},
			Removes:      []int{1, 0, 2},
			ExpectedList: [][]int{{0, 2}, {2}, {}},
		},
		{
			BitLengths:   []int{28, 26, 24},
			Removes:      []int{1, 2, 0},
			ExpectedList: [][]int{{0, 2}, {0}, {}},
		},
		{
			BitLengths:   []int{28, 26, 24},
			Removes:      []int{2, 0, 1},
			ExpectedList: [][]int{{0, 1}, {1}, {}},
		},
		{
			BitLengths:   []int{28, 26, 24},
			Removes:      []int{2, 1, 0},
			ExpectedList: [][]int{{0, 1}, {0}, {}},
		},
	}

	for i, test := range tests {
		var punishes []*PunishmentProposal
		var l = newPunishmentProposalList(nil)
		// prepare inserted list
		for _, bl := range test.BitLengths {
			punish := tstMockPunishmentProposal(bl, nil)
			punishes = append(punishes, punish)
			l.insert(punish)
		}
		// remove and check
		for j, idx := range test.Removes {
			l.remove(punishes[idx].PubKey)
			sli := l.slice()
			// check list length
			if len(sli) != len(test.ExpectedList[j]) {
				t.Errorf("wrong list length, %d, %d, expected %d, got %d",
					i, j, len(test.ExpectedList[j]), len(sli))
			}
			// check expected order
			for k, e := range sli {
				if e != punishes[test.ExpectedList[j][k]] {
					t.Errorf("wrong list order, %d, %d, %d, expected %v, got: %v",
						i, j, k, tstExtractPunishmentProposals(punishes, test.ExpectedList[j]), sli)
				}
			}
		}
	}
}
