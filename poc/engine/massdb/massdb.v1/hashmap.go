package massdb_v1

import (
	"bytes"
	"encoding/binary"
	"os"
	"path"

	"massnet.org/mass/logging"
	"massnet.org/mass/poc/engine/massdb"
	"massnet.org/mass/poc/pocutil"
	"massnet.org/mass/pocec"
)

// File struct for massdb.v1
// | Position | Length in Bytes |     Name    |         Rule         |
// | :------: | :-------------: | :---------: | :------------------ |
// |    0     |        32       | FileCode    | Fixed hex string `DC9081169821110F139A7FC8486D74F3749653DD3F60F127F8F0F14571AE3F6D` |
// |   32     |         8       | Version     | DB version, currently use 1 |
// |   40     |         1       | BitLength   | BitLength for HashMap |
// |   41     |         1       | Type(A/B)   | HashMap(A/B), 0 for A, 1 for B |
// |   42     |         8       | Checkpoint  | Last work checkpoint, Little Endian Encoded |
// |   50     |        32       | PubKeyHash  | SHA256(SHA256(PubKey)) |
// |   82     |        33       | PubKey      | PubKey |
// |  115     |      3981       | AlignHolder | Fill `\0` to align to 4096 Bytes |
// |  4096    |       ...       | ProofData   | Proof Data, different decode/encode strategy in HashMap(A/B) |
type HashMap struct {
	data       *os.File
	bl         int
	volume     pocutil.PoCValue
	checkpoint pocutil.PoCValue
	offset     int
	step       int
	recordSize int
	pk         *pocec.PublicKey
	pkHash     pocutil.Hash
}

// ProofData struct for HashMapA
// Define `~` as "flip bits by BitLength"; Define `s = [1 << (BitLength - 1)] - 1`
// Length in Bytes = (BitLength + 7) / 8, named as L
// | Offset | Length in Bytes |      Name     |         Rule         |
// | :----: | :-------------: | :------------ | :------------------ |
// |    0   |         L       | Y(`0`) -> X   | Value X for `Y = 0` |
// |    L   |         L       | Y(`~0`) -> X  | Value X for `Y = ~0` |
// |   2L   |         L       | Y(`1`) -> X   | Value X for `Y = 1` |
// |   3L   |         L       | Y(`~1`) -> X  | Value X for `Y = ~1` |
// |   ..   |        ..       | Y(`*`) -> X   | Value X for `Y = *` |
// |   ..   |        ..       | Y(`~*`) -> X  | Value X for `Y = ~*` |
// |  2Ls   |         L       | Y(`s`) -> X   | Value X for `Y = s` |
// | 2Ls+L  |         L       | Y(`~s`) -> X  | Value X for `Y = ~s` |
type HashMapA struct {
	HashMap
	half pocutil.PoCValue
}

// ProofData struct for HashMapB
// Define `n = [1 << BitLength] - 1`
// Length in Bytes = (BitLength + 7) / 8, named as L
// | Offset | Length in Bytes |     Name     |         Rule        |
// | :----: | :-------------: | :----------- | :------------------ |
// |    0   |         L       | Z(`0`) -> X  | Value X for `Z = 0` |
// |    L   |         L       | Z(`0`) -> XP | Value XP for `Z = 0` |
// |   2L   |         L       | Z(`1`) -> X  | Value X for `Z = 1` |
// |   3L   |         L       | Z(`1`) -> XP | Value XP for `Z = 1` |
// |   ..   |        ..       | Z(`*`) -> X  | Value X for `Z = *` |
// |   ..   |        ..       | Z(`*`) -> XP | Value XP for `Z = *` |
// |  2Ln   |         L       | Z(`n`) -> X  | Value X for `Z = n` |
// | 2Ln+L  |         L       | Z(`n`) -> XP | Value XP for `Z = n` |
type HashMapB struct {
	HashMap
}

const (
	dbVersion = 1

	// Positions for meta info in file
	PosFileCode    = 0
	PosVersion     = 32
	PosBitLength   = 40
	PosType        = 41
	PosCheckpoint  = 42
	PosPubKeyHash  = 50
	PosPubKey      = 82
	PosAlignHolder = 115
	PosProofData   = 4096

	// Lengths for meta info in file
	LenFileCode    = 32
	LenVersion     = 8
	LenBitLength   = 1
	LenType        = 1
	LenCheckpoint  = 8
	LenPubKeyHash  = 32
	LenPubKey      = 33
	LenAlignHolder = 3981
	LenMetaInfo    = 4096
)

func createMapFile(filePath string, typ MapType, bl int, pubKey *pocec.PublicKey) (*os.File, error) {
	// If file already exists, return error
	_, err := os.Stat(filePath)
	if err == nil || os.IsExist(err) {
		return nil, massdb.ErrDBAlreadyExists
	}

	err = os.MkdirAll(path.Dir(filePath), os.ModePerm)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0666)
	if nil != err {
		return nil, err
	}

	var failureReturn = func(err error) (*os.File, error) {
		f.Close()
		os.Remove(filePath)
		return nil, err
	}

	// Expand file to actual size
	if err = expandMapFile(f, typ, bl); err != nil {
		return failureReturn(err)
	}

	var (
		fileHeaderBytes [LenMetaInfo]byte
		b1              [1]byte
		b8              [8]byte
		b32             [32]byte
	)

	// Write fixed file code
	copy(fileHeaderBytes[PosFileCode:], massdb.DBFileCode)
	// Write db version
	binary.LittleEndian.PutUint64(b8[:], dbVersion)
	copy(fileHeaderBytes[PosVersion:], b8[:])
	// Write bitLength
	b1[0] = byte(bl)
	copy(fileHeaderBytes[PosBitLength:], b1[:])
	// Write type
	b1[0] = byte(typ)
	copy(fileHeaderBytes[PosType:], b1[:])
	// Write checkpoint
	binary.LittleEndian.PutUint64(b8[:], 0)
	copy(fileHeaderBytes[PosCheckpoint:], b8[:])
	// Write PubKeyHash and PubKey
	pubKeyBytes := pubKey.SerializeCompressed()
	b32 = pocutil.PubKeyHash(pubKey)
	copy(fileHeaderBytes[PosPubKeyHash:], b32[:])
	copy(fileHeaderBytes[PosPubKey:], pubKeyBytes)

	if _, err = f.WriteAt(fileHeaderBytes[:], 0); err != nil {
		return failureReturn(err)
	}

	return f, nil
}

func openMapFile(filePath string) (*os.File, error) {
	_, err := os.Stat(filePath)
	if err != nil && !os.IsExist(err) {
		return nil, massdb.ErrDBDoesNotExist
	}
	f, err := os.OpenFile(filePath, os.O_RDWR, 0666)
	if nil != err {
		return nil, err
	}
	return f, nil
}

func loadHashMap(filePath string) (*HashMap, MapType, error) {
	f, err := openMapFile(filePath)
	if nil != err {
		return nil, 0, err
	}

	var failureReturn = func(err error) (*HashMap, MapType, error) {
		f.Close()
		return nil, 0, err
	}

	// read meta info
	var (
		b1  [1]byte
		b8  [8]byte
		b32 [32]byte
		b33 [33]byte
	)
	hm := &HashMap{
		data: f,
	}

	var fileHeaderBytes [LenMetaInfo]byte
	if n, err := f.ReadAt(fileHeaderBytes[:], 0); err != nil {
		logging.CPrint(logging.ERROR, "massdb fileSize is smaller than expected", logging.LogFormat{"expected": LenMetaInfo, "actual": n})
		return failureReturn(ErrDBWrongFileSize)
	}

	// Check fileCode
	copy(b32[:], fileHeaderBytes[PosFileCode:])
	if !bytes.Equal(b32[:], massdb.DBFileCode) {
		return failureReturn(ErrDBWrongFileCode)
	}
	// Check dbVersion
	copy(b8[:], fileHeaderBytes[PosVersion:])
	if ver := binary.LittleEndian.Uint64(b8[:]); ver != dbVersion {
		logging.CPrint(logging.ERROR, "massdb version not matched", logging.LogFormat{"expected": dbVersion, "actual": ver})
		return failureReturn(ErrDBWrongVersion)
	}
	// Get BitLength
	copy(b1[:], fileHeaderBytes[PosBitLength:])
	hm.bl = int(b1[0])
	// Get Type
	copy(b1[:], fileHeaderBytes[PosType:])
	typ := MapType(b1[0])
	// Get Checkpoint
	copy(b8[:], fileHeaderBytes[PosCheckpoint:])
	hm.checkpoint = pocutil.PoCValue(binary.LittleEndian.Uint64(b8[:]))
	// Get PubKeyHash
	copy(hm.pkHash[:], fileHeaderBytes[PosPubKeyHash:])
	// Get PubKey
	copy(b33[:], fileHeaderBytes[PosPubKey:])
	hm.pk, err = pocec.ParsePubKey(b33[:], pocec.S256())
	if err != nil {
		return failureReturn(err)
	}
	// check pubKey with pubKeyHash
	if hm.pkHash != pocutil.PubKeyHash(hm.pk) {
		return failureReturn(ErrDBWrongPubKeyHash)
	}

	// check typ, return err if type not valid
	offset, step, err := calcProofDataOffset(typ)
	if err != nil {
		return failureReturn(err)
	}
	hm.offset = offset
	hm.step = step
	hm.volume = 1 << uint(hm.bl)
	hm.recordSize = pocutil.RecordSize(hm.bl)

	return hm, typ, nil
}

func calcSize(typeName MapType, bl int) int {
	var recordSize = pocutil.RecordSize(bl)
	var fileSize int

	switch typeName {
	case MapTypeHashMapA:
		fileSize = LenMetaInfo + recordSize*(1<<uint(bl))
	case MapTypeHashMapB:
		fileSize = LenMetaInfo + 2*recordSize*(1<<uint(bl))
	default:
		fileSize = 0
	}
	return fileSize
}

func calcProofDataOffset(typeName MapType) (offset, step int, err error) {
	switch typeName {
	case MapTypeHashMapA:
		offset = LenMetaInfo
		step = 1
	case MapTypeHashMapB:
		offset = LenMetaInfo
		step = 2
	default:
		offset = 0
		step = 0
		err = ErrDBWrongMapType
	}
	return
}

func (hm *HashMap) Close() error {
	return hm.data.Close()
}

func (hm *HashMap) UpdateCheckpoint() error {
	var checkpointByte [LenCheckpoint]byte
	binary.LittleEndian.PutUint64(checkpointByte[:], uint64(hm.checkpoint))
	hm.data.WriteAt(checkpointByte[:], PosCheckpoint)
	return nil
}

func (hm *HashMap) ReadCheckpoint() pocutil.PoCValue {
	var checkpointByte [LenCheckpoint]byte
	hm.data.ReadAt(checkpointByte[:], PosCheckpoint)
	return pocutil.PoCValue(binary.LittleEndian.Uint64(checkpointByte[:]))
}

func (hm *HashMap) Checkpoint() pocutil.PoCValue {
	return hm.checkpoint
}

func CreateHashMap(filePath string, typ MapType, bl int, pubKey *pocec.PublicKey) error {
	f, err := createMapFile(filePath, typ, bl, pubKey)
	if err != nil {
		return err
	}
	f.Close()
	return nil
}

func LoadHashMap(filePath string) (interface{}, error) {
	hm, typ, err := loadHashMap(filePath)
	if err != nil {
		return nil, err
	}

	switch typ {
	case MapTypeHashMapA:
		return &HashMapA{HashMap: *hm, half: hm.volume / 2}, nil
	case MapTypeHashMapB:
		return &HashMapB{HashMap: *hm}, nil
	default:
		return nil, ErrDBWrongType
	}
}

func (hm *HashMapA) Progress() (bool, pocutil.PoCValue) {
	checkpoint := hm.checkpoint
	return checkpoint >= hm.volume, checkpoint
}

func (hm *HashMapA) Get(key pocutil.PoCValue) ([]byte, error) {
	if key < hm.half {
		key = key * 2
	} else {
		key = pocutil.FlipValue(key, hm.bl)*2 + 1
	}

	var recordSize = hm.recordSize
	var value [8]byte
	target := hm.offset + int(key)*recordSize
	if n, _ := hm.data.ReadAt(value[:recordSize], int64(target)); n < recordSize {
		return nil, massdb.ErrDBCorrupted
	}
	return value[:recordSize], nil
}

func (hm *HashMapA) Set(key pocutil.PoCValue, value []byte) error {
	if key < hm.half {
		key = key * 2
	} else {
		key = pocutil.FlipValue(key, hm.bl)*2 + 1
	}

	var recordSize = hm.recordSize
	target := hm.offset + int(key)*recordSize
	if n, _ := hm.data.WriteAt(value[:recordSize], int64(target)); n < recordSize {
		return massdb.ErrDBCorrupted
	}
	return nil
}

func (hm *HashMapB) Progress() (bool, pocutil.PoCValue) {
	checkpoint := hm.checkpoint
	return checkpoint >= hm.volume/2, checkpoint
}

func (hm *HashMapB) Get(key pocutil.PoCValue) ([]byte, []byte, error) {
	var recordSize = hm.recordSize
	var proof [16]byte
	target := hm.offset + int(key)*recordSize*2
	if n, _ := hm.data.ReadAt(proof[:recordSize*2], int64(target)); n < recordSize*2 {
		return nil, nil, massdb.ErrDBCorrupted
	}
	return proof[:recordSize], proof[recordSize : recordSize*2], nil
}

func (hm *HashMapB) Set(key pocutil.PoCValue, x, xp []byte) error {
	var recordSize = hm.recordSize
	var n int
	target := hm.offset + int(key)*recordSize*2
	if n, _ = hm.data.WriteAt(x[:recordSize], int64(target)); n != recordSize {
		return massdb.ErrDBCorrupted
	}
	if n, _ = hm.data.WriteAt(xp[:recordSize], int64(target+recordSize)); n < recordSize {
		return massdb.ErrDBCorrupted
	}
	return nil
}
