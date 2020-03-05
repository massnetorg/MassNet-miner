package blockchain

import (
	"math/big"
	"sync"
	"time"

	"massnet.org/mass/poc"
	"massnet.org/mass/pocec"
	"massnet.org/mass/wire"
)

const (
	// minMemoryNodes is the minimum number of consecutive nodes needed
	// in memory in order to perform all necessary validation.  It is used
	// to determine when it's safe to prune nodes from memory without
	// causing constant dynamic reloading.
	minMemoryNodes = 8000
)

type BlockNode struct {
	InMainChain     bool
	Parent          *BlockNode
	Hash            *wire.Hash
	CapSum          *big.Int
	ChainID         wire.Hash
	Version         uint64
	Height          uint64
	Timestamp       time.Time
	Previous        wire.Hash
	TransactionRoot wire.Hash
	WitnessRoot     wire.Hash
	ProposalRoot    wire.Hash
	Target          *big.Int
	Challenge       wire.Hash
	PubKey          *pocec.PublicKey
	Proof           *poc.Proof
	Quality         *big.Int
	Signature       *pocec.Signature
	BanList         []*pocec.PublicKey
}

func NewBlockNode(header *wire.BlockHeader, blockHash *wire.Hash, flags BehaviorFlags) *BlockNode {
	if flags.isFlagSet(BFNoPoCCheck) {
		return &BlockNode{
			ChainID:         header.ChainID,
			Version:         header.Version,
			Height:          header.Height,
			Timestamp:       header.Timestamp,
			Previous:        header.Previous,
			TransactionRoot: header.TransactionRoot,
			WitnessRoot:     header.WitnessRoot,
			ProposalRoot:    header.ProposalRoot,
			BanList:         header.BanList,
		}
	}
	return &BlockNode{
		Hash:            blockHash,
		CapSum:          new(big.Int).Set(header.Target),
		ChainID:         header.ChainID,
		Version:         header.Version,
		Height:          header.Height,
		Timestamp:       header.Timestamp,
		Previous:        header.Previous,
		TransactionRoot: header.TransactionRoot,
		WitnessRoot:     header.WitnessRoot,
		ProposalRoot:    header.ProposalRoot,
		Target:          header.Target,
		Challenge:       header.Challenge,
		PubKey:          header.PubKey,
		Proof:           header.Proof,
		Quality:         header.Quality(),
		Signature:       header.Signature,
		BanList:         header.BanList,
	}
}

// Ancestor returns the ancestor block node at the provided height by following
// the chain backwards from this node.  The returned block will be nil when a
// height is requested that is after the height of the passed node or is less
// than zero.
func (node *BlockNode) Ancestor(height uint64) *BlockNode {
	if height < 0 || height > node.Height {
		return nil
	}

	n := node
	for ; n != nil && n.Height != height; n = n.Parent {
		// Intentionally left blank
	}

	return n
}

func (node *BlockNode) BlockHeader() *wire.BlockHeader {
	return &wire.BlockHeader{
		ChainID:         node.ChainID,
		Version:         node.Version,
		Height:          node.Height,
		Timestamp:       node.Timestamp,
		Previous:        node.Previous,
		TransactionRoot: node.TransactionRoot,
		WitnessRoot:     node.WitnessRoot,
		ProposalRoot:    node.ProposalRoot,
		Target:          node.Target,
		Challenge:       node.Challenge,
		PubKey:          node.PubKey,
		Proof:           node.Proof,
		Signature:       node.Signature,
		BanList:         node.BanList,
	}
}

type BlockTree struct {
	sync.RWMutex
	rootNode        *BlockNode                 // root node of blockTree
	bestNode        *BlockNode                 // newest block node in main chain
	index           map[wire.Hash]*BlockNode   // hash to BlockNode
	children        map[wire.Hash][]*BlockNode // parent to children
	orphanBlockPool *OrphanBlockPool           // orphan blocks pool
}

func NewBlockTree() *BlockTree {
	return &BlockTree{
		index:           make(map[wire.Hash]*BlockNode),
		children:        make(map[wire.Hash][]*BlockNode),
		orphanBlockPool: newOrphanBlockPool(),
	}
}

func (tree *BlockTree) bestBlockNode() *BlockNode {
	return tree.bestNode
}

func (tree *BlockTree) setBestBlockNode(node *BlockNode) {
	tree.bestNode = node
}

func (tree *BlockTree) rootBlockNode() *BlockNode {
	return tree.rootNode
}

func (tree *BlockTree) setRootBlockNode(node *BlockNode) error {
	tree.Lock()
	defer tree.Unlock()

	if tree.rootNode != nil {
		return errRootNodeAlreadyExists
	}
	tree.rootNode = node
	tree.children[node.Previous] = append(tree.children[node.Previous], node)
	tree.index[*node.Hash] = node

	return nil
}

func (tree *BlockTree) getBlockNode(hash *wire.Hash) (*BlockNode, bool) {
	tree.RLock()
	defer tree.RUnlock()
	node, exists := tree.index[*hash]
	return node, exists
}

func (tree *BlockTree) blockNode(hash *wire.Hash) *BlockNode {
	tree.RLock()
	defer tree.RUnlock()
	return tree.index[*hash]
}

func (tree *BlockTree) nodeExists(hash *wire.Hash) bool {
	tree.RLock()
	defer tree.RUnlock()
	_, exists := tree.index[*hash]
	return exists
}

func (tree *BlockTree) orphanExists(hash *wire.Hash) bool {
	return tree.orphanBlockPool.isOrphanInPool(hash)
}

// attachBlockNode attaches a leaf node
func (tree *BlockTree) attachBlockNode(node *BlockNode) error {
	tree.Lock()
	defer tree.Unlock()

	parentNode, exists := tree.index[node.Previous]
	if !exists {
		return errAttachNonLeafBlockNode
	}

	node.CapSum = node.CapSum.Add(parentNode.CapSum, node.CapSum)
	node.Parent = parentNode
	tree.index[*node.Hash] = node
	tree.children[node.Previous] = append(tree.children[node.Previous], node)
	return nil
}

// removeBlockNodeFromSlice assumes that both args are valid, and removes node from nodes
func removeBlockNodeFromSlice(nodes []*BlockNode, node *BlockNode) []*BlockNode {
	for i := range nodes {
		if nodes[i].Hash.IsEqual(node.Hash) {
			copy(nodes[i:], nodes[i+1:])
			nodes[len(nodes)-1] = nil
			return nodes[:len(nodes)-1]
		}
	}
	return nodes
}

// detachBlockNode detaches a leaf node in block tree
func (tree *BlockTree) detachBlockNode(node *BlockNode) error {
	tree.RLock()
	defer tree.RUnlock()

	if _, exists := tree.children[*node.Hash]; exists {
		return errDetachParentBlockNode
	}

	tree.children[node.Previous] = removeBlockNodeFromSlice(tree.children[node.Previous], node)
	if len(tree.children[node.Previous]) == 0 {
		delete(tree.children, node.Previous)
	}

	return nil
}

// recursiveAddChildrenCapSum recursively add certain cap number to children
func recursiveAddChildrenCapSum(tree *BlockTree, hash *wire.Hash, cap *big.Int) {
	for _, childNode := range tree.children[*hash] {
		childNode.CapSum.Add(childNode.CapSum, cap)
		recursiveAddChildrenCapSum(tree, childNode.Hash, cap)
	}
}

// expandRootBlockNode expands a new node before current root
func (tree *BlockTree) expandRootBlockNode(node *BlockNode) error {
	tree.Lock()
	defer tree.Unlock()

	if _, exists := tree.index[node.Previous]; exists {
		return errExpandChildRootBlockNode
	}

	childNodes, exists := tree.children[*node.Hash]
	if !exists {
		return errExpandOrphanRootBlockNode
	}

	for _, childNode := range childNodes {
		childNode.Parent = node
		tree.children[*node.Hash] = append(tree.children[*node.Hash], childNode)
		recursiveAddChildrenCapSum(tree, node.Hash, node.CapSum)
		tree.rootNode = node
	}
	tree.index[*node.Hash] = node
	tree.children[node.Previous] = append(tree.children[node.Previous], node)

	return nil
}
