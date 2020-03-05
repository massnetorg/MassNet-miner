package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"massnet.org/mass/poc"
	"massnet.org/mass/pocec"
)

// ---------------------------------------
//
// Messages for WorkSpace state
//
// ---------------------------------------

var (
	// ErrInvalidState indicates invalid WorkSpaceState
	ErrInvalidState = errors.New("invalid state")

	// ErrNoneStateFlags indicates none WorkSpaceStateFlags
	ErrNoneStateFlags = errors.New("workspace state flag is none")
)

// Here lists the states of a WorkSpace.
const (
	// Registered indicates SpaceKeeper has registered a (PK, BitLength),
	// no matter whether it has been loaded or plotted
	Registered WorkSpaceState = iota

	// Plotting indicates a (PK, BitLength) instance is plotting or loading
	Plotting

	// Ready indicates a (PK, BitLength) instance is loaded (also plotted),
	// thus the space is ready for mining
	Ready

	// Mining indicates a (PK, BitLength) instance is used for mining
	Mining

	// FirstState hints the first state
	FirstState = Registered

	// LastState hints the last state
	LastState = Mining
)

// Here lists the state flags of a WorkSpace.
const (
	// SFRegistered corresponds to Registered
	SFRegistered WorkSpaceStateFlags = 1 << iota

	// SFPlotting corresponds to Plotting
	SFPlotting

	// SFReady corresponds to Ready
	SFReady

	// SFMining corresponds to Mining
	SFMining

	// FirstStateFlags hints the value of first StateFlag
	FirstStateFlags = SFRegistered

	// LastStateFlags hints the value of last StateFlag
	LastStateFlags = SFMining

	// SFAll is a convenient way of setting all flags
	SFAll = SFRegistered | SFPlotting | SFReady | SFMining
)

// WorkSpaceState represents the state of a WorkSpace according to its ability
type WorkSpaceState uint32

// WorkSpaceStateFlags represents the flags of a WorkSpace state
type WorkSpaceStateFlags WorkSpaceState

// wsStateString map WorkSpaceState to string
var wsStateString = map[WorkSpaceState]string{
	Registered: "registered",
	Plotting:   "plotting",
	Ready:      "ready",
	Mining:     "mining",
}

// wsStateFlagsString map WorkSpaceState to string
var wsStateFlagsString = map[WorkSpaceStateFlags]string{
	SFRegistered: "SFRegistered",
	SFPlotting:   "SFPlotting",
	SFReady:      "SFReady",
	SFMining:     "SFMining",
}

// IsValid reports whether current state is defined or not
func (s WorkSpaceState) IsValid() bool {
	return s >= FirstState && s <= LastState
}

// Flag converts current state to corresponding WorkSpaceStateFlags
func (s WorkSpaceState) Flag() WorkSpaceStateFlags {
	return 1 << s
}

func (s WorkSpaceState) String() string {
	str, ok := wsStateString[s]
	if !ok {
		str = fmt.Sprintf("invalid(%d)", s)
	}
	return str
}

// IsNone reports whether none of a defined flag is set or not
func (f WorkSpaceStateFlags) IsNone() bool {
	return f&SFAll == 0
}

// Contains reports whether current flags contains given flags
func (f WorkSpaceStateFlags) Contains(flag WorkSpaceStateFlags) bool {
	return (f & flag) == flag
}

// States returns a slice of WorkSpaceStates contained in current flags by ascending order
func (f WorkSpaceStateFlags) States() []WorkSpaceState {
	states := make([]WorkSpaceState, 0, LastState-FirstState+1)
	for s := FirstState; s <= LastState; s++ {
		if f.Contains(s.Flag()) {
			states = append(states, s)
		}
	}
	return states
}

// Format as "SFAll", "SFAll|SFReady" and "SFNone"
func (f WorkSpaceStateFlags) String() string {
	if f.IsNone() {
		return "SFNone"
	}

	states := f.States()
	strList := make([]string, 0, len(states))
	for _, state := range states {
		strList = append(strList, wsStateFlagsString[state.Flag()])
	}
	return strings.Join(strList, "|")
}

// ---------------------------------------
//
// Messages for WorkSpace action
//
// ---------------------------------------

var (
	// ErrInvalidAction indicates invalid action
	ErrInvalidAction = errors.New("invalid action")

	// ErrUnImplementedAction indicates unimplemented action
	ErrUnImplementedAction = errors.New("unimplemented action")
)

const (
	// UnknownOrdinal indicates that the ordinal of MassDB is unknown or non-existent
	UnknownOrdinal int64 = -1
)

// ActionType represents the action acted on WorkSpace
type ActionType uint8

const (
	// Plot should make WorkSpace state conversion happen like:
	// registered -> plotting -> ready
	// registered -> ready
	// plotting -> ready
	// ready -> ready
	// mining -> mining
	// WorkSpace is not allowed to plot unless its state is registered, plotting, ready or mining
	Plot ActionType = iota

	// Mine should make WorkSpace state conversion happen like:
	// registered -> plotting -> mining
	// plotting   -> mining
	// ready      -> mining
	// mining     -> mining
	// WorkSpace is not allowed to mine unless its state is registered, plotting, ready or mining
	Mine

	// Stop should make WorkSpace state conversion happen like:
	// registered -> registered
	// plotting   -> registered
	// ready      -> ready
	// mining     -> ready
	// WorkSpace is not allowed to stop unless its state is registered, plotting, ready or mining
	Stop

	// Remove should remove index of WorkSpace
	// WorkSpace is not allowed to remove unless its state is registered or ready
	Remove

	// Delete should delete both index and data of WorkSpace
	// WorkSpace is not allowed to delete unless its state is registered or ready
	Delete

	// FirstAction hints the value of first ActionType
	FirstAction = Plot

	// LastAction hints the value of last ActionType
	LastAction = Delete
)

// actionTypeString map ActionType to string
var actionTypeString = map[ActionType]string{
	Plot:   "plot",
	Mine:   "mine",
	Stop:   "stop",
	Remove: "remove",
	Delete: "delete",
}

// IsValid reports whether current action is defined or not
func (act ActionType) IsValid() bool {
	return act >= FirstAction && act <= LastAction
}

func (act ActionType) String() string {
	str, ok := actionTypeString[act]
	if !ok {
		str = fmt.Sprintf("invalid(%d)", act)
	}
	return str
}

// ---------------------------------------
//
// Messages between Miner and SpaceKeeper
//
// ---------------------------------------

// WorkSpaceProof contains poc.Proof along with other elements helping miner,
// Proof is nil when Error is not nil
type WorkSpaceProof struct {
	SpaceID   string
	Proof     *poc.Proof
	PublicKey *pocec.PublicKey
	Ordinal   int64
	Error     error
}

// WorkSpaceInfo represents the general info of a WorkSpace
type WorkSpaceInfo struct {
	SpaceID   string
	PublicKey *pocec.PublicKey
	Ordinal   int64
	BitLength int
	Progress  float64
	State     WorkSpaceState
}

// ProofReader is the interface that wraps the basic Read-Proof method.
//
// An instance of this general case is that a ProofReader returning
// a non-nil WorkSpaceProof at the end of the input stream may
// return either err == io.EOF or err == nil. The next Read should
// return nil, io.EOF.
type ProofReader interface {
	Read() (*WorkSpaceProof, error)
}

var (
	// ErrProofIOTimeout indicates proof read/write operation timeout
	ErrProofIOTimeout = errors.New("proof i/o timeout")
)

// ProofRW implements ProofReader and provides a way that servers can writes WorkSpaceProofs.
type ProofRW struct {
	l      sync.Mutex
	ctx    context.Context
	ch     chan *WorkSpaceProof
	closed bool
}

// NewProofRW generates a new ProofRW instance.
func NewProofRW(ctx context.Context, bufSize int) *ProofRW {
	prw := &ProofRW{
		ctx: ctx,
		ch:  make(chan *WorkSpaceProof, bufSize),
	}
	go func() {
		<-prw.ctx.Done()
		prw.Close()
	}()
	return prw
}

// Read implements ProofReader.
func (prw *ProofRW) Read() (*WorkSpaceProof, error) {
	wsp, ok := <-prw.ch
	var err error
	if !ok && wsp == nil {
		err = io.EOF
	}
	return wsp, err
}

// Write provides a way that servers can writes WorkSpaceProofs.
//
// Write is concurrent safe, and returns an error ErrProofIOTimeout if p.ctx is Done.
func (prw *ProofRW) Write(wsp *WorkSpaceProof) error {
	prw.l.Lock()
	defer prw.l.Unlock()
	if prw.closed {
		return ErrProofIOTimeout
	}
	prw.ch <- wsp
	return nil
}

// Close closes ProofRW.
//
// Any call to Write would get ErrProofIOTimeout.
func (prw *ProofRW) Close() {
	prw.l.Lock()
	if !prw.closed {
		prw.closed = true
		close(prw.ch)
	}
	prw.l.Unlock()
}
