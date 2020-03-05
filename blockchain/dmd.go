package blockchain

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"sync"

	"massnet.org/mass/database"
	"massnet.org/mass/database/storage"
	"massnet.org/mass/logging"
	"massnet.org/mass/pocec"
	"massnet.org/mass/wire"
)

type DoubleMiningDetector struct {
	sync.RWMutex
	banState       map[wire.Hash]map[string]struct{} // banned PubKey from current blockNode to banTrackNode
	banTrackNode   map[wire.Hash]*BlockNode          // certain block's banTrackNode
	appearedPubKey map[string]*wire.Hash             // appeared mining PubKey `pubKey-height-bitLength` with corresponding block
	db             database.Db                       // db interface with double-mining-detector requirements
}

func NewDoubleMiningDetector(db database.Db) *DoubleMiningDetector {
	return &DoubleMiningDetector{
		banState:       make(map[wire.Hash]map[string]struct{}),
		banTrackNode:   make(map[wire.Hash]*BlockNode),
		appearedPubKey: make(map[string]*wire.Hash),
		db:             db,
	}
}

func (dmd *DoubleMiningDetector) isPubKeyBannedTillHeight(height uint64, pk *pocec.PublicKey) (bool, error) {
	pubKeyHash := wire.DoubleHashH(pk.SerializeUncompressed())
	_, pkHeight, err := dmd.db.FetchFaultPkBySha(&pubKeyHash)
	if err != nil {
		if err == storage.ErrNotFound {
			return false, nil
		}
		return false, err
	}

	if pkHeight > uint64(height) {
		return false, nil
	}

	return true, nil
}

// recursiveCheckPubKeyBanned recursively check whether pubKey is banned or not before current node
func recursiveCheckPubKeyBanned(dmd *DoubleMiningDetector, node *BlockNode, pubKey *pocec.PublicKey) (bool, error) {
	if node.InMainChain {
		return dmd.isPubKeyBannedTillHeight(node.Height, pubKey)
	}

	pubKeyString := hex.EncodeToString(pubKey.SerializeCompressed())

	if dmd.banState[*node.Hash] != nil && dmd.banTrackNode[*node.Hash] != nil {
		// Deal with leaf node
		// check banState between self node to trackBanNode
		if _, exists := dmd.banState[*node.Hash][pubKeyString]; exists {
			return true, nil
		}
		// continue to check banTrackNode
		return recursiveCheckPubKeyBanned(dmd, dmd.banTrackNode[*node.Hash], pubKey)

	} else {
		// Deal with stem node
		// Query and expand banState
		dmd.banState[*node.Hash] = make(map[string]struct{})

		// try to find in ancestors
		for cursor := node.Parent; cursor != nil; cursor = cursor.Parent {
			dmd.banTrackNode[*node.Hash] = cursor
			breakTag := false
			// stop condition
			if cursor.InMainChain {
				return dmd.isPubKeyBannedTillHeight(cursor.Height, pubKey)
			}
			// check each pk in node banList
			for _, key := range cursor.BanList {
				strKey := hex.EncodeToString(key.SerializeCompressed())
				dmd.banState[*node.Hash][strKey] = struct{}{}
				if pubKeyString == strKey {
					breakTag = true
				}
			}
			if breakTag {
				return true, nil
			}
		}
		return false, errInvalidDmdTree
	}
}

// isPubKeyBanned returns whether pubKey is banned or not before current node
func (dmd *DoubleMiningDetector) isPubKeyBanned(node *BlockNode, pubKey *pocec.PublicKey) (bool, error) {
	dmd.Lock()
	defer dmd.Unlock()

	return recursiveCheckPubKeyBanned(dmd, node, pubKey)
}

func makeAppearedPubKeyKey(pubKey *pocec.PublicKey, height uint64, bl int) string {
	var buf bytes.Buffer
	var hb [8]byte
	binary.LittleEndian.PutUint64(hb[:], height)
	var bb [1]byte
	bb[0] = uint8(bl)

	buf.Write(pubKey.SerializeCompressed())
	buf.Write(hb[:])
	buf.Write(bb[:])

	return buf.String()
}

// applyBlockNode applies new block node on double-mining-detector,
// assuming that parent node must in banList tree
func (dmd *DoubleMiningDetector) applyBlockNode(node *BlockNode) (bool, *wire.Hash, *wire.Hash) {
	dmd.Lock()
	defer dmd.Unlock()

	parent, nodeHash, parentHash := node.Parent, *node.Hash, node.Previous

	if parent == nil {
		dmd.banState[nodeHash] = nil
		dmd.banTrackNode[nodeHash] = nil
	} else if dmd.banState[parentHash] == nil || dmd.banTrackNode[parentHash] == nil {
		dmd.banState[nodeHash] = make(map[string]struct{})
		dmd.banTrackNode[nodeHash] = parent
	} else {
		dmd.banState[nodeHash] = dmd.banState[parentHash]
		dmd.banTrackNode[nodeHash] = dmd.banTrackNode[parentHash]
		dmd.banState[parentHash] = nil
		dmd.banTrackNode[parentHash] = nil
	}

	// add newly banned pubKey into banState
	for _, pubKey := range node.BanList {
		pubKeyString := hex.EncodeToString(pubKey.SerializeCompressed())
		dmd.banState[nodeHash][pubKeyString] = struct{}{}
	}

	mKey := makeAppearedPubKeyKey(node.PubKey, node.Height, node.Proof.BitLength)
	if doubleHash, exists := dmd.appearedPubKey[mKey]; exists {
		if !node.Hash.IsEqual(doubleHash) {
			return false, node.Hash, doubleHash
		}
	}

	dmd.appearedPubKey[mKey] = node.Hash

	return true, nil, nil
}

func (chain *Blockchain) submitFaultPubKeyFromHash(hash0, hash1 *wire.Hash) error {
	var getBlockHeader = func(chain *Blockchain, hash *wire.Hash) *wire.BlockHeader {
		if blk, err := chain.db.FetchBlockBySha(hash); err == nil {
			return &blk.MsgBlock().Header
		}
		if blk, err := chain.blockCache.getBlock(hash); err == nil {
			return &blk.MsgBlock().Header
		}
		return nil
	}

	h0 := getBlockHeader(chain, hash0)
	h1 := getBlockHeader(chain, hash1)
	fpk := wire.NewEmptyFaultPubKey()
	if h0 != nil && h1 != nil {
		fpk.PubKey = h0.PubKey
		fpk.Testimony[0] = h0
		fpk.Testimony[1] = h1
	} else {
		return errFaultPubKeyGetBlockHeader
	}

	if err := checkFaultPkSanity(fpk, chain.info.chainID); err != nil {
		return err
	}

	// insert faultPk into database punishment storage, and insert it into punishPool
	if err := chain.db.InsertPunishment(fpk); err != nil {
		return err
	}
	chain.proposalPool.InsertFaultPubKey(fpk)

	logging.CPrint(logging.INFO, "captured fault pubKey with bitLength",
		logging.LogFormat{
			"fault pubKey": hex.EncodeToString(fpk.PubKey.SerializeCompressed()),
			"bitLength":    fpk.Testimony[0].Proof.BitLength,
		})

	return nil
}
