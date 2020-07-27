package blockchain

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"massnet.org/mass/config"
	"massnet.org/mass/database/ldb"
	"massnet.org/mass/database/storage"
	_ "massnet.org/mass/database/storage/ldbstorage"
	"massnet.org/mass/massutil"
	"massnet.org/mass/wire"
)

func loadBlks(path string) []*massutil.Block {
	f, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	blks := make([]*massutil.Block, 0, 50)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		buf, err := hex.DecodeString(scanner.Text())
		if err != nil {
			panic(err)
		}

		blk, err := massutil.NewBlockFromBytes(buf, wire.Packet)
		if err != nil {
			panic(err)
		}
		blks = append(blks, blk)
	}
	return blks
}

func newReorgTestChain(genesis *massutil.Block, name string) (*Blockchain, func()) {
	path := fmt.Sprintf("./reorgtest/%s/blocks.db", name)
	cachePath := fmt.Sprintf("./reorgtest/%s/blocks.dat", name)
	fi, err := os.Stat(cachePath)
	if os.IsNotExist(err) {
		os.MkdirAll(cachePath, 0700)
	}
	if fi != nil {
		os.RemoveAll(cachePath)
		os.MkdirAll(cachePath, 0700)
	}
	fi, err = os.Stat(path)
	if os.IsNotExist(err) {
		os.MkdirAll(path, 0700)
	}
	if fi != nil {
		os.RemoveAll(path)
		os.MkdirAll(path, 0700)
	}

	stor, err := storage.CreateStorage("leveldb", path, nil)
	if err != nil {
		panic(err)
	}
	db, err := ldb.NewChainDb(path, stor)
	if err != nil {
		panic(err)
	}
	if err = db.InitByGenesisBlock(genesis); err != nil {
		panic(err)
	}
	bc, err := newTestBlockchain(db, cachePath)
	if err != nil {
		panic(err)
	}

	return bc, func() {
		db.Close()
		os.RemoveAll("./reorgtest/" + name)
	}
}

func TestForkBeforeStaking(t *testing.T) {
	// height: 0(main)... -> 30(main) -> 31(main) -> 31(fork) -> 32(fork) -> 33(fork) -> 32(main) -> 33(main) -> ...49(main)
	// i:      0      ...    30          31          32          33          34          35          36          ...52
	blks := loadBlks("./data/beforestaking.dat")
	assert.Equal(t, 53, len(blks))
	assert.Equal(t, uint64(31), blks[31].Height())
	assert.Equal(t, uint64(31), blks[32].Height())
	assert.Equal(t, uint64(32), blks[33].Height())
	assert.Equal(t, uint64(33), blks[34].Height())
	assert.Equal(t, uint64(32), blks[35].Height())

	copy(config.ChainParams.GenesisHash[:], blks[0].Hash()[:])
	copy(config.ChainParams.GenesisBlock.Header.Challenge[:], blks[0].MsgBlock().Header.Challenge[:])
	copy(config.ChainParams.GenesisBlock.Header.ChainID[:], blks[0].MsgBlock().Header.ChainID[:])
	config.ChainParams.GenesisBlock.Header.Timestamp = blks[0].MsgBlock().Header.Timestamp
	config.ChainParams.GenesisBlock.Header.Target = blks[0].MsgBlock().Header.Target

	// Step 1. Just process blocks on main chain
	bc1, close1 := newReorgTestChain(blks[0], "1")
	defer close1()

	for i := 1; i < 42; i++ {
		blk := blks[i]
		if i >= 32 && i <= 34 {
			continue
		}
		isOrphan, err := bc1.processBlock(blk, BFNone)
		if err != nil {
			t.Fatal(err, i)
		}
		assert.False(t, isOrphan, i)
	}
	assert.Equal(t, uint64(38), bc1.BestBlockHeight())
	entries1 := bc1.db.TestExportDbEntries()
	checkEntries(t, entries1)

	// Step 2. Process all blocks including forks
	bc2, close2 := newReorgTestChain(blks[0], "2")
	defer close2()

	for i := 1; i < 42; i++ {
		blk := blks[i]
		isOrphan, err := bc2.processBlock(blk, BFNone)
		assert.Nil(t, err)
		assert.False(t, isOrphan)
	}
	assert.Equal(t, uint64(38), bc2.BestBlockHeight())
	entries2 := bc2.db.TestExportDbEntries()
	checkEntries(t, entries2)

	assert.Equal(t, bc1.BestBlockHash().String(), bc2.BestBlockHash().String())
	assert.True(t, len(entries1) <= len(entries2))

	sameKeyValue := 0
	diffFb := 0
	diffBlkhgt := 0
	gap := SumBlkFileSize(t, blks[31], blks[32], blks[33], blks[34])
	for k2, v2 := range entries2 {
		v1, ok := entries1[k2]
		if !ok {
			assert.Equal(t, "PUNISH", string(k2[:6]))
			continue
		}
		if bytes.Equal(v1, v2) {
			sameKeyValue++
			continue
		}
		if strings.HasPrefix(k2, "fb") {
			diffFb++
			continue
		}
		if !strings.HasPrefix(k2, "BLKHGT") {
			t.FailNow()
		}
		diffBlkhgt++
		CheckBLKHGT(t, []byte(k2), v1, v2, gap)
	}
	assert.Equal(t, len(entries1), sameKeyValue+diffFb+diffBlkhgt)
}

func TestForkBeforeStakingExpire(t *testing.T) {
	// height: 0(main)... -> 39(main) -> 40(main) ->40(fork) -> 41(fork) -> 42(fork) -> 41(main) -> 42(main) -> ...49(main)
	// i:      0      ...    39          40         41          42          43          44          45          ...52
	blks := loadBlks("./data/beforestakingexpire.dat")
	assert.Equal(t, 53, len(blks))
	assert.Equal(t, uint64(40), blks[40].Height()) // main
	assert.Equal(t, uint64(40), blks[41].Height()) // fork
	assert.Equal(t, uint64(41), blks[42].Height()) // fork
	assert.Equal(t, uint64(42), blks[43].Height()) // fork
	assert.Equal(t, uint64(41), blks[44].Height()) // main

	copy(config.ChainParams.GenesisHash[:], blks[0].Hash()[:])
	copy(config.ChainParams.GenesisBlock.Header.Challenge[:], blks[0].MsgBlock().Header.Challenge[:])
	copy(config.ChainParams.GenesisBlock.Header.ChainID[:], blks[0].MsgBlock().Header.ChainID[:])
	config.ChainParams.GenesisBlock.Header.Timestamp = blks[0].MsgBlock().Header.Timestamp
	config.ChainParams.GenesisBlock.Header.Target = blks[0].MsgBlock().Header.Target

	// Step 1. Just process blocks on main chain
	bc1, close1 := newReorgTestChain(blks[0], "1")
	defer close1()

	for i := 1; i < 47; i++ {
		blk := blks[i]
		if i >= 41 && i <= 43 {
			continue
		}
		isOrphan, err := bc1.processBlock(blk, BFNone)
		if err != nil {
			t.Fatal(err, i)
		}
		assert.False(t, isOrphan, i)
	}
	assert.Equal(t, uint64(43), bc1.BestBlockHeight())
	entries1 := bc1.db.TestExportDbEntries()
	checkEntries(t, entries1)

	// Step 2. Process all blocks including forks
	bc2, close2 := newReorgTestChain(blks[0], "2")
	defer close2()

	for i := 1; i < 47; i++ {
		blk := blks[i]
		isOrphan, err := bc2.processBlock(blk, BFNone)
		assert.Nil(t, err)
		assert.False(t, isOrphan)
	}
	assert.Equal(t, uint64(43), bc2.BestBlockHeight())
	entries2 := bc2.db.TestExportDbEntries()
	checkEntries(t, entries2)

	assert.Equal(t, bc1.BestBlockHash().String(), bc2.BestBlockHash().String())
	assert.True(t, len(entries1) <= len(entries2))

	sameKeyValue := 0
	diffFb := 0
	diffBlkhgt := 0
	gap := SumBlkFileSize(t, blks[40], blks[41], blks[42], blks[43])
	for k2, v2 := range entries2 {
		v1, ok := entries1[k2]
		if !ok {
			assert.Equal(t, "PUNISH", string(k2[:6]))
			continue
		}
		if bytes.Equal(v1, v2) {
			sameKeyValue++
			continue
		}
		if strings.HasPrefix(k2, "fb") {
			diffFb++
			continue
		}
		if !strings.HasPrefix(k2, "BLKHGT") {
			t.FailNow()
		}
		diffBlkhgt++
		CheckBLKHGT(t, []byte(k2), v1, v2, gap)
	}
	assert.Equal(t, len(entries1), sameKeyValue+diffFb+diffBlkhgt)
}

func TestForkBeforeStakingReward(t *testing.T) {
	// height: 0(main)... -> 37(main) -> 38(main) -> 38(fork) -> 39(fork) -> 40(fork) -> 39(main) -> 40(main) -> ...49(main)
	// i:      0      ...    37          38          39          40          41          42          43          ...52
	blks := loadBlks("./data/beforestakingreward.dat")
	assert.Equal(t, 53, len(blks))
	assert.Equal(t, uint64(38), blks[38].Height()) // main
	assert.Equal(t, uint64(38), blks[39].Height()) // fork
	assert.Equal(t, uint64(39), blks[40].Height()) // fork
	assert.Equal(t, uint64(40), blks[41].Height()) // fork
	assert.Equal(t, uint64(39), blks[42].Height()) // main

	copy(config.ChainParams.GenesisHash[:], blks[0].Hash()[:])
	copy(config.ChainParams.GenesisBlock.Header.Challenge[:], blks[0].MsgBlock().Header.Challenge[:])
	copy(config.ChainParams.GenesisBlock.Header.ChainID[:], blks[0].MsgBlock().Header.ChainID[:])
	config.ChainParams.GenesisBlock.Header.Timestamp = blks[0].MsgBlock().Header.Timestamp
	config.ChainParams.GenesisBlock.Header.Target = blks[0].MsgBlock().Header.Target

	// Step 1. Just process blocks on main chain
	bc1, close1 := newReorgTestChain(blks[0], "1")
	defer close1()

	for i := 1; i < 45; i++ {
		blk := blks[i]
		if i >= 39 && i <= 41 {
			continue
		}
		isOrphan, err := bc1.processBlock(blk, BFNone)
		if err != nil {
			t.Fatal(err, i)
		}
		assert.False(t, isOrphan, i)
	}
	assert.Equal(t, uint64(41), bc1.BestBlockHeight())
	entries1 := bc1.db.TestExportDbEntries()
	checkEntries(t, entries1)

	// Step 2. Process all blocks including forks
	bc2, close2 := newReorgTestChain(blks[0], "2")
	defer close2()

	for i := 1; i < 45; i++ {
		blk := blks[i]
		isOrphan, err := bc2.processBlock(blk, BFNone)
		assert.Nil(t, err)
		assert.False(t, isOrphan)
	}
	assert.Equal(t, uint64(41), bc2.BestBlockHeight())
	entries2 := bc2.db.TestExportDbEntries()
	checkEntries(t, entries2)

	assert.Equal(t, bc1.BestBlockHash().String(), bc2.BestBlockHash().String())
	assert.True(t, len(entries1) <= len(entries2))

	sameKeyValue := 0
	diffFb := 0
	diffBlkhgt := 0
	gap := SumBlkFileSize(t, blks[38], blks[39], blks[40], blks[41])
	for k2, v2 := range entries2 {
		v1, ok := entries1[k2]
		if !ok {
			assert.Equal(t, "PUNISH", string(k2[:6]))
			continue
		}
		if bytes.Equal(v1, v2) {
			sameKeyValue++
			continue
		}
		if strings.HasPrefix(k2, "fb") {
			diffFb++
			continue
		}
		if !strings.HasPrefix(k2, "BLKHGT") {
			t.FailNow()
		}
		diffBlkhgt++
		CheckBLKHGT(t, []byte(k2), v1, v2, gap)
	}
	assert.Equal(t, len(entries1), sameKeyValue+diffFb+diffBlkhgt)
}
func TestForkBeforeBinding(t *testing.T) {
	// height: 0(main)... -> 23(main) -> 24(main) -> 24(fork) -> 25(fork) -> 26(fork) -> 25(main) -> 26(main) -> ...49(main)
	// i:      0      ...    23          24          25          26          27          28          29          ...52
	blks := loadBlks("./data/beforebinding.dat")
	assert.Equal(t, 53, len(blks))
	assert.Equal(t, uint64(24), blks[24].Height()) // main
	assert.Equal(t, uint64(24), blks[25].Height()) // fork
	assert.Equal(t, uint64(25), blks[26].Height()) // fork
	assert.Equal(t, uint64(26), blks[27].Height()) // fork
	assert.Equal(t, uint64(25), blks[28].Height()) // main

	copy(config.ChainParams.GenesisHash[:], blks[0].Hash()[:])
	copy(config.ChainParams.GenesisBlock.Header.Challenge[:], blks[0].MsgBlock().Header.Challenge[:])
	copy(config.ChainParams.GenesisBlock.Header.ChainID[:], blks[0].MsgBlock().Header.ChainID[:])
	config.ChainParams.GenesisBlock.Header.Timestamp = blks[0].MsgBlock().Header.Timestamp
	config.ChainParams.GenesisBlock.Header.Target = blks[0].MsgBlock().Header.Target

	// Step 1. Just process blocks on main chain
	bc1, close1 := newReorgTestChain(blks[0], "1")
	defer close1()

	for i := 1; i < 32; i++ {
		blk := blks[i]
		if i >= 25 && i <= 27 {
			continue
		}
		isOrphan, err := bc1.processBlock(blk, BFNone)
		if err != nil {
			t.Fatal(err, i)
		}
		assert.False(t, isOrphan, i)
	}
	assert.Equal(t, uint64(28), bc1.BestBlockHeight())
	entries1 := bc1.db.TestExportDbEntries()
	checkEntries(t, entries1)

	// Step 2. Process all blocks including forks
	bc2, close2 := newReorgTestChain(blks[0], "2")
	defer close2()

	for i := 1; i < 32; i++ {
		blk := blks[i]
		isOrphan, err := bc2.processBlock(blk, BFNone)
		assert.Nil(t, err)
		assert.False(t, isOrphan)
	}
	assert.Equal(t, uint64(28), bc2.BestBlockHeight())
	entries2 := bc2.db.TestExportDbEntries()
	checkEntries(t, entries2)

	assert.Equal(t, bc1.BestBlockHash().String(), bc2.BestBlockHash().String())
	assert.True(t, len(entries1) <= len(entries2))

	sameKeyValue := 0
	diffFb := 0
	diffBlkhgt := 0
	gap := SumBlkFileSize(t, blks[24], blks[25], blks[26], blks[27])
	for k2, v2 := range entries2 {
		v1, ok := entries1[k2]
		if !ok {
			assert.Equal(t, "PUNISH", string(k2[:6]))
			continue
		}
		if bytes.Equal(v1, v2) {
			sameKeyValue++
			continue
		}
		if strings.HasPrefix(k2, "fb") {
			diffFb++
			continue
		}
		if !strings.HasPrefix(k2, "BLKHGT") {
			t.FailNow()
		}
		diffBlkhgt++
		CheckBLKHGT(t, []byte(k2), v1, v2, gap)
	}
	assert.Equal(t, len(entries1), sameKeyValue+diffFb+diffBlkhgt)
}

func checkEntries(t *testing.T, entries map[string][]byte) {
	var (
		blkFileTotal   uint32
		blkFileCounter int
		maxBlkFileNo   uint32
	)
	for key, value := range entries {
		prefix := ""
		switch {
		case strings.HasPrefix(key, "ADDRINDEXVERSION"):
			prefix = "ADDRINDEXVERSION"
		case strings.HasPrefix(key, "BLKSHA"):
			prefix = "BLKSHA"
		case strings.HasPrefix(key, "BLKHGT"):
			prefix = "BLKHGT"
		case strings.HasPrefix(key, "BANPUB"):
			prefix = "BANPUB"
		case strings.HasPrefix(key, "BANHGT"):
			prefix = "BANHGT"
		case strings.HasPrefix(key, "DBSTORAGEMETA"):
			prefix = "DBSTORAGEMETA"
		case strings.HasPrefix(key, "MBP"):
			prefix = "MBP"
		case strings.HasPrefix(key, "PUNISH"):
			prefix = "PUNISH"
		case strings.HasPrefix(key, "TXD"):
			prefix = "TXD"
		case strings.HasPrefix(key, "TXS"):
			prefix = "TXS"
		case strings.HasPrefix(key, "TXL"):
			prefix = "TXL"
		case strings.HasPrefix(key, "TXU"):
			prefix = "TXU"
		case strings.HasPrefix(key, "HTS"):
			prefix = "HTS"
		case strings.HasPrefix(key, "HTGS"):
			prefix = "HTGS"
		case strings.HasPrefix(key, "STL"):
			prefix = "STL"
		case strings.HasPrefix(key, "STG"):
			prefix = "STG"
		case strings.HasPrefix(key, "STSG"):
			prefix = "STSG"
		case strings.HasPrefix(key, "fb"):
			if key == "fblatest" {
				blkFileTotal = binary.LittleEndian.Uint32(value) + 1
			} else {
				blkFileCounter++
				no := binary.LittleEndian.Uint32([]byte(key)[2:6])
				if no > maxBlkFileNo {
					maxBlkFileNo = no
				}
			}
			continue
		case strings.HasPrefix(key, "PUBKBL"):
			prefix = "PUBKBL"
		default:
			t.Fatal(key, value)
		}
		checkEntry(t, prefix, key, value)
	}
	assert.True(t, int(blkFileTotal) == blkFileCounter)
	assert.True(t, blkFileTotal == maxBlkFileNo+1)
}

func checkEntry(t *testing.T, prefix, key string, value []byte) {
	switch prefix {
	case "ADDRINDEXVERSION":
		assert.Equal(t, 16, len(key))
		assert.Equal(t, 2, len(value))
	case "BLKSHA":
		assert.Equal(t, 38, len(key))
		assert.Equal(t, 8, len(value))
	case "BLKHGT":
		assert.Equal(t, 14, len(key))
		assert.True(t, len(value) == 52)
	case "BANPUB":
		assert.Equal(t, 38, len(key))
		assert.True(t, len(value) > 41)
	case "BANHGT":
		assert.Equal(t, 14, len(key))
		count := binary.LittleEndian.Uint16(value[0:2])
		assert.Equal(t, int(2+count*64), len(value))
	case "DBSTORAGEMETA":
		assert.Equal(t, 13, len(key))
		assert.Equal(t, 40, len(value))
	case "MBP":
		assert.Equal(t, 44, len(key))
		assert.Equal(t, 0, len(value))
	case "PUNISH":
		assert.Equal(t, 39, len(key))
		assert.True(t, len(value) > 0)
	case "TXD":
		assert.Equal(t, 35, len(key))
		assert.True(t, len(value) > 16)
	case "TXS":
		assert.Equal(t, 35, len(key))
		assert.Equal(t, 20, len(value))
	case "TXL":
		assert.Equal(t, 55, len(key))
		assert.Equal(t, 40, len(value))
	case "TXU":
		assert.Equal(t, 55, len(key))
		assert.Equal(t, 40, len(value))
	case "HTS":
		assert.Equal(t, 43, len(key))
		assert.Equal(t, 4, len(value))
	case "HTGS":
		assert.Equal(t, 32, len(key))
		assert.Equal(t, 0, len(value))
	case "STL":
		assert.Equal(t, 43, len(key))
		bitmap := binary.LittleEndian.Uint32(value[0:4])
		assert.True(t, len(value) >= 15)
		shift := 0
		cur := 4
		for bitmap != 0 {
			if bitmap&0x01 != 0 {
				index := value[cur]
				assert.True(t, int(index) == shift)
				N := binary.LittleEndian.Uint16(value[cur+1 : cur+3])
				assert.NotZero(t, N)

				// check no duplication
				offsets := make(map[uint32]bool)
				for i := 0; i < int(N); i++ {
					offset := binary.LittleEndian.Uint32(value[cur+3+8*i:])
					_, ok := offsets[offset]
					assert.False(t, ok)
					offsets[offset] = true
				}

				cur += 3 + 8*int(N)
			}
			bitmap >>= 1
			shift++
		}
		assert.True(t, cur == len(value))
	case "STG":
		assert.Equal(t, 43, len(key))
		assert.Equal(t, 0, len(value))
	case "STSG":
		assert.Equal(t, 72, len(key))
		assert.Equal(t, 0, len(value))
	case "PUBKBL":
		assert.Equal(t, 9, len(value))
	default:
		t.Fatal(prefix, key, value)
	}
}

func CheckBLKHGT(t *testing.T, key, value1, value2 []byte, gapOffset uint64) {
	height := binary.LittleEndian.Uint64(key[6:])
	var sha1, sha2 wire.Hash
	copy(sha1[:], value1[0:32])
	copy(sha2[:], value2[0:32])
	// value1
	fileNo1 := binary.LittleEndian.Uint32(value1[32:])
	offset1 := binary.LittleEndian.Uint64(value1[36:])
	blksize1 := binary.LittleEndian.Uint64(value1[44:])
	// value2(fork)
	fileNo2 := binary.LittleEndian.Uint32(value2[32:])
	offset2 := binary.LittleEndian.Uint64(value2[36:])
	blksize2 := binary.LittleEndian.Uint64(value2[44:])

	assert.Equal(t, sha1, sha2)
	assert.Equal(t, fileNo1, fileNo2, height, sha1)
	assert.Equal(t, blksize1, blksize2, height, sha1)
	assert.Equal(t, gapOffset, offset2-offset1)
}

func SumBlkFileSize(t *testing.T, blks ...*massutil.Block) (sum uint64) {
	for _, blk := range blks {
		raw, err := blk.Bytes(wire.DB)
		assert.Nil(t, err)
		sum += uint64(12 + len(raw))
	}
	return sum
}
