package blockchain

type BehaviorFlags uint32

const (
	// TODO: add FastMode
	BFFastAdd BehaviorFlags = 1 << iota

	BFNoPoCCheck

	// BFNone is a convenience value to specifically indicate no flags.
	BFNone BehaviorFlags = 0
)

func (flags BehaviorFlags) isFlagSet(flag BehaviorFlags) bool {
	return (flags & flag) == flag
}
