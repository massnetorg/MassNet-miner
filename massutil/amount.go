package massutil

import (
	"errors"
	"math"
	"strconv"

	"massnet.org/mass/consensus"
	"massnet.org/mass/massutil/safetype"
)

var (
	ErrInvalidFloat = errors.New("invalid float")
	ErrMaxAmount    = errors.New("exceed maximum amount")
)

// AmountUnit describes a method of converting an Amount to something
// other than the base unit of a Mass.  The value of the AmountUnit
// is the exponent component of the decadic multiple to convert from
// an amount in Mass to an amount counted in units.
type AmountUnit int

// These constants define various units used when describing a Mass
// monetary amount.
const (
	AmountMegaMASS  AmountUnit = 6
	AmountKiloMASS  AmountUnit = 3
	AmountMASS      AmountUnit = 0
	AmountMilliMASS AmountUnit = -3
	AmountMicroMASS AmountUnit = -6
	AmountMaxwell   AmountUnit = -8
)

// String returns the unit as a string.  For recognized units, the SI
// prefix is used, or "Maxwell" for the base unit.  For all unrecognized
// units, "1eN MASS" is returned, where N is the AmountUnit.
func (u AmountUnit) String() string {
	switch u {
	case AmountMegaMASS:
		return "MMASS"
	case AmountKiloMASS:
		return "kMASS"
	case AmountMASS:
		return "MASS"
	case AmountMilliMASS:
		return "mMASS"
	case AmountMicroMASS:
		return "μMASS"
	case AmountMaxwell:
		return "Maxwell"
	default:
		return "1e" + strconv.FormatInt(int64(u), 10) + " MASS"
	}
}

// Amount represents the base Mass monetary unit (colloquially referred
// to as a `Maxwell').  A single Amount is equal to 1e-8 of a Mass.
// The value is limited in range [0, consensus.MaxMass * consensus.MaxwellPerMass]
type Amount struct {
	value *safetype.Uint128
}

var (
	zeroAmount    = Amount{safetype.NewUint128()}
	maxAmount     Amount
	minRelayTxFee Amount
)

func init() {
	u, err := safetype.NewUint128FromUint(consensus.MaxMass).
		Mul(safetype.NewUint128FromUint(consensus.MaxwellPerMass))
	if err != nil {
		panic("init maxAmount error: " + err.Error())
	}

	maxAmount = Amount{u}

	minRelayTxFee, err = NewAmountFromUint(consensus.MinRelayTxFee)
	if err != nil {
		panic("init minRelayTxFee error: " + err.Error())
	}
}

// MaxAmount is the maximum transaction amount allowed in maxwell.
func MaxAmount() Amount {
	return maxAmount
}

func ZeroAmount() Amount {
	return zeroAmount
}

// MinRelayTxFee is the minimum fee required for relaying tx.
func MinRelayTxFee() Amount {
	return minRelayTxFee
}

// round converts a floating point number, which may or may not be representable
// as an integer, to the Amount integer type by rounding to the nearest integer.
// This is performed by adding or subtracting 0.5 depending on the sign, and
// relying on integer truncation to round the value to the nearest Amount.
func round(maxwell float64) (Amount, error) {
	var v int64
	if maxwell < 0 {
		v = int64(maxwell - 0.5)
	} else {
		v = int64(maxwell + 0.5)
	}
	u, err := safetype.NewUint128FromInt(v)
	if err != nil {
		return zeroAmount, err
	}
	return checkedAmount(u)
}

// NewAmountFromMass creates an Amount from a floating point value representing
// some value in Mass.  NewAmountFromMass errors if f is NaN or +-Infinity, but
// does not check that the amount is within the total amount of Mass
// producible as f may not refer to an amount at a single moment in time.
//
// NewAmountFromMass is for specifically for converting MASS to Maxwell.
// For creating a new Amount with an int64 value which denotes a quantity of Maxwell,
// do a simple type conversion from type int64 to Amount.
func NewAmountFromMass(f float64) (Amount, error) {
	// The amount is only considered invalid if it cannot be represented
	// as an integer type.  This may happen if f is NaN or +-Infinity.
	switch {
	case math.IsNaN(f):
		fallthrough
	case math.IsInf(f, 1):
		fallthrough
	case math.IsInf(f, -1):
		return zeroAmount, ErrInvalidFloat
	}
	return round(f * float64(consensus.MaxwellPerMass))
}

func NewAmountFromInt(i int64) (Amount, error) {
	u, err := safetype.NewUint128FromInt(i)
	if err != nil {
		return zeroAmount, err
	}
	return checkedAmount(u)
}

func NewAmountFromUint(i uint64) (Amount, error) {
	return NewAmount(safetype.NewUint128FromUint(i))
}

func NewAmount(u *safetype.Uint128) (Amount, error) {
	return checkedAmount(u)
}

// ToUnit converts a monetary amount counted in Mass base units to a
// floating point value representing an amount of Mass.
func (a Amount) ToUnit(u AmountUnit) float64 {
	return float64(a.UintValue()) / math.Pow10(int(u+8))
}

// ToMASS is the equivalent of calling ToUnit with AmountMASS.
func (a Amount) ToMASS() float64 {
	return a.ToUnit(AmountMASS)
}

// Format formats a monetary amount counted in Mass base units as a
// string for a given unit.  The conversion will succeed for any unit,
// however, known units will be formated with an appended label describing
// the units with SI notation, or "Maxwell" for the base unit.
func (a Amount) Format(u AmountUnit) string {
	units := " " + u.String()
	return strconv.FormatFloat(a.ToUnit(u), 'f', -int(u+8), 64) + units
}

// String is the equivalent of calling Format with AmountMASS.
func (a Amount) String() string {
	return a.Format(AmountMASS)
}

// MulF64 multiplies an Amount by a floating point value.  While this is not
// an operation that must typically be done by a full node or wallet, it is
// useful for services that build on top of Mass (for example, calculating
// a fee by multiplying by a percentage).
func (a Amount) MulF64(f float64) (Amount, error) {
	switch {
	case math.IsNaN(f):
		fallthrough
	case math.IsInf(f, 1):
		fallthrough
	case math.IsInf(f, -1):
		return zeroAmount, ErrInvalidFloat
	}

	return round(float64(a.UintValue()) * f)
}

func (a Amount) Add(b Amount) (Amount, error) {
	u, err := a.value.Add(b.value)
	if err != nil {
		return zeroAmount, err
	}
	return checkedAmount(u)
}

func (a Amount) AddInt(i int64) (Amount, error) {
	u, err := a.value.AddInt(i)
	if err != nil {
		return zeroAmount, err
	}
	return checkedAmount(u)
}

func (a Amount) Sub(b Amount) (Amount, error) {
	u, err := a.value.Sub(b.value)
	if err != nil {
		return zeroAmount, err
	}
	return checkedAmount(u)
}

func (a Amount) Cmp(b Amount) int {
	return a.value.Cmp(b.value)
}

func (a Amount) IsZero() bool {
	return a.value.IsZero()
}

func (a Amount) Value() *safetype.Uint128 {
	return a.value
}

func (a Amount) IntValue() int64 {
	return a.value.IntValue()
}

func (a Amount) UintValue() uint64 {
	return a.value.UintValue()
}

func checkedAmount(u *safetype.Uint128) (Amount, error) {
	if u.Gt(maxAmount.value) {
		return zeroAmount, ErrMaxAmount
	}
	return Amount{u}, nil
}

// TstSetMinRelayTxFee For testing purposes
func TstSetMinRelayTxFee(fee uint64) {
	var err error
	minRelayTxFee, err = NewAmountFromUint(fee)
	if err != nil {
		panic("TstSetMinRelayTxFee error: " + err.Error())
	}
}

// TstSetMaxAmount For testing purpose
func TstSetMaxAmount(mass float64) {
	if mass < 0 {
		panic("TstSetMaxAmount error: negative")
	}
	u, err := safetype.NewUint128FromInt(int64(mass + 0.5))
	if err != nil {
		panic("TstSetMaxAmount error: " + err.Error())
	}

	u, err = u.Mul(safetype.NewUint128FromUint(consensus.MaxwellPerMass))
	if err != nil {
		panic("TstSetMaxAmount error: " + err.Error())
	}
	maxAmount = Amount{u}
}
