package safetype

import (
	"errors"
	"math"
	"math/big"
)

const (
	// Uint128Bytes the number of bytes Uint128 type will take.
	Uint128Bytes = 16
	// Uint128Bits defines the number of bits for Uint128 type.
	Uint128Bits = 128
)

var (
	// ErrUint128Overflow indicates the value is greater than maximum of 2^128-1.
	ErrUint128Overflow = errors.New("Uint128: overflow")

	// ErrUint128Underflow indicates the value is less than minimum of 0.
	ErrUint128Underflow = errors.New("Uint128: underflow")

	// ErrUint128InvalidBytes indicates the bytes size is not equal to Uint128Bytes.
	ErrUint128InvalidBytes = errors.New("Uint128: invalid bytes")

	// ErrUint128InvalidString indicates the string is not valid when converted to Uint128.
	ErrUint128InvalidString = errors.New("Uint128: invalid string")

	// ErrUint128DividedByZero indicates a division-by-zero error occurs
	ErrUint128DividedByZero = errors.New("Uint128: divided by zero")
)

var (
	maxUint64 = NewUint128FromUint(math.MaxUint64)
	maxInt64  = NewUint128FromUint(math.MaxInt64)
)

// Uint128 defines a uint128 type with safe arithmetic operations implemented, based on big.Int.
//
// Supported arithmetic operations:
//			 u1 = u1.Add(u2)
//			 u1 = u1.Sub(u2)
//			 u1 = u1.Mul(u2)
//			 u1 = u1.Div(u2)
//			 u1 = u1.Mod(u2)
//			 u1 = u1.Exp(u2)
//			 etc.
type Uint128 struct {
	value *big.Int
}

// Validate returns error if u is not a valid Uint128, otherwise returns nil.
func (u *Uint128) Validate() error {
	if u.value.Sign() < 0 {
		return ErrUint128Underflow
	}
	if u.value.BitLen() > Uint128Bits {
		return ErrUint128Overflow
	}
	return nil
}

// NewUint128 returns a new Uint128 struct with default value 0.
func NewUint128() *Uint128 {
	return &Uint128{big.NewInt(0)}
}

// NewUint128FromString returns a new Uint128 struct with given value and value check.
func NewUint128FromString(str string) (*Uint128, error) {
	b, success := new(big.Int).SetString(str, 10)
	if !success {
		return nil, ErrUint128InvalidString
	}
	obj := &Uint128{b}
	if err := obj.Validate(); err != nil {
		return nil, err
	}
	return obj, nil
}

// NewUint128FromUint returns a new Uint128 with given value
func NewUint128FromUint(i uint64) *Uint128 {
	return &Uint128{new(big.Int).SetUint64(i)}
}

// NewUint128FromInt returns a new Uint128 struct with given value and value check.
func NewUint128FromInt(i int64) (*Uint128, error) {
	obj := &Uint128{big.NewInt(i)}
	if err := obj.Validate(); err != nil {
		return nil, err
	}
	return obj, nil
}

// NewUint128FromBigInt returns a new Uint128 struct with given value and value check.
func NewUint128FromBigInt(i *big.Int) (*Uint128, error) {
	obj := &Uint128{i}
	if err := obj.Validate(); err != nil {
		return nil, err
	}
	return obj, nil
}

// NewUint128FromBytes converts big-endian byte slice to Uint128, len(bytes) must be not greater than Uint128Bytes.
func NewUint128FromBytes(bytes []byte) (*Uint128, error) {
	l := len(bytes)
	if l > Uint128Bytes {
		return nil, ErrUint128InvalidBytes
	}
	u := NewUint128()
	for i := 0; i < l; i++ {
		if bytes[i] != 0 {
			u.value.SetBytes(bytes[i:])
			break
		}
	}
	return u, nil
}

// Bytes returns the value of u as a big-endian byte array.
func (u *Uint128) Bytes() [Uint128Bytes]byte {
	var ret [Uint128Bytes]byte
	bs := u.value.Bytes()
	l := len(bs)
	if l > 0 {
		copy(ret[Uint128Bytes-l:], bs)
	}
	return ret
}

//RawBytes returns the underlying byte slice of u with prefix 0(byte) removed.
func (u *Uint128) AbsBytes() []byte {
	return u.value.Bytes()
}

// String returns the string representation of x.
func (u *Uint128) String() string {
	return u.value.Text(10)
}

// UintValue returns the uint64 representation of u.
// Panic if u is greater than maximum uint64
func (u *Uint128) UintValue() uint64 {
	if u.Gt(maxUint64) {
		panic("too big to be converted to uint64: " + u.String())
	}
	return u.value.Uint64()
}

// IntValue returns the int64 representation of u.
// Panic if u is greater than maximum int64
func (u *Uint128) IntValue() int64 {
	if u.Gt(maxInt64) {
		panic("too big to be converted to int64: " + u.String())
	}
	return u.value.Int64()
}

func (u *Uint128) Float64() float64 {
	f, _ := new(big.Float).SetInt(u.value).Float64()
	return f
}

//Add returns u + x
func (u *Uint128) Add(x *Uint128) (*Uint128, error) {
	obj := &Uint128{new(big.Int).Add(u.value, x.value)}
	if err := obj.Validate(); err != nil {
		return nil, err
	}
	return obj, nil
}

func (u *Uint128) AddInt(i int64) (*Uint128, error) {
	x, err := NewUint128FromInt(i)
	if err != nil {
		return nil, err
	}
	return u.Add(x)
}

func (u *Uint128) AddUint(i uint64) (*Uint128, error) {
	return u.Add(NewUint128FromUint(i))
}

//Sub returns u - x
func (u *Uint128) Sub(x *Uint128) (*Uint128, error) {
	obj := &Uint128{new(big.Int).Sub(u.value, x.value)}
	if err := obj.Validate(); err != nil {
		return nil, err
	}
	return obj, nil
}

func (u *Uint128) SubUint(x uint64) (*Uint128, error) {
	return u.Sub(NewUint128FromUint(x))
}

//Mul returns u * x
func (u *Uint128) Mul(x *Uint128) (*Uint128, error) {
	obj := &Uint128{new(big.Int).Mul(u.value, x.value)}
	if err := obj.Validate(); err != nil {
		return nil, err
	}
	return obj, nil
}

func (u *Uint128) MulInt(i int64) (*Uint128, error) {
	x, err := NewUint128FromInt(i)
	if err != nil {
		return nil, err
	}
	return u.Mul(x)
}

//Div returns u / x.
func (u *Uint128) Div(x *Uint128) (*Uint128, error) {
	if x.IsZero() {
		return nil, ErrUint128DividedByZero
	}
	obj := &Uint128{new(big.Int).Div(u.value, x.value)}
	if err := obj.Validate(); err != nil {
		return nil, err
	}
	return obj, nil
}

func (u *Uint128) DivInt(i int64) (*Uint128, error) {
	x, err := NewUint128FromInt(i)
	if err != nil {
		return nil, err
	}
	return u.Div(x)
}

//Mod returns u % x
func (u *Uint128) Mod(x *Uint128) (*Uint128, error) {
	if x.IsZero() {
		return nil, ErrUint128DividedByZero
	}
	obj := &Uint128{new(big.Int).Mod(u.value, x.value)}
	if err := obj.Validate(); err != nil {
		return nil, err
	}
	return obj, nil
}

//Exp returns u^x
func (u *Uint128) Exp(x *Uint128) (*Uint128, error) {
	obj := &Uint128{new(big.Int).Exp(u.value, x.value, nil)}
	if err := obj.Validate(); err != nil {
		return nil, err
	}
	return obj, nil
}

// Rsh returns u>>n
func (u *Uint128) Rsh(n uint) *Uint128 {
	return &Uint128{new(big.Int).Rsh(u.value, n)}
}

//DeepCopy returns a deep copy of u
func (u *Uint128) DeepCopy() *Uint128 {
	return &Uint128{new(big.Int).Set(u.value)}
}

// Cmp compares u and x and returns:
//
//   -1 if u <  x
//    0 if u == x
//   +1 if u >  x
func (u *Uint128) Cmp(x *Uint128) int {
	return u.value.Cmp(x.value)
}

// Gt returns whether u is greater than x
func (u *Uint128) Gt(x *Uint128) bool {
	return u.Cmp(x) > 0
}

// Gt returns whether u is greater than or equal to x
func (u *Uint128) Ge(x *Uint128) bool {
	return u.Cmp(x) >= 0
}

// Lt returns whether u is less than x
func (u *Uint128) Lt(x *Uint128) bool {
	return u.Cmp(x) < 0
}

// Le returns whether u is less than or equal to x
func (u *Uint128) Le(x *Uint128) bool {
	return u.Cmp(x) <= 0
}

// Eq returns whether u is equal to x
func (u *Uint128) Eq(x *Uint128) bool {
	return u.Cmp(x) == 0
}

func (u *Uint128) IsZero() bool {
	return u.value.Sign() == 0
}

func (u *Uint128) BigValue() *big.Int {
	return new(big.Int).Set(u.value)
}
