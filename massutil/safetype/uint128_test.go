package safetype

import (
	"fmt"
	"math"
	"math/big"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewUint128FromBytes(t *testing.T) {
	tests := []struct {
		name         string
		bytes        []byte
		expectInt    int64
		expectString string
		err          error
	}{
		{
			name:         "case 16-bytes 0",
			bytes:        []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			expectInt:    0,
			expectString: "0",
			err:          nil,
		},
		{
			name:         "case less-16-bytes 0",
			bytes:        []byte{0, 0, 0, 0},
			expectInt:    0,
			expectString: "0",
			err:          nil,
		},
		{
			name:         "case nil-bytes 0",
			bytes:        nil,
			expectInt:    0,
			expectString: "0",
			err:          nil,
		},
		{
			name:  "case too many bytes",
			bytes: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			err:   ErrUint128InvalidBytes,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			u, err := NewUint128FromBytes(test.bytes)
			assert.Equal(t, test.err, err)
			if err == nil {
				assert.Equal(t, test.expectString, u.String())
				assert.Equal(t, test.expectInt, u.IntValue())
			}
		})
	}
}

func TestNewUint128FromString(t *testing.T) {
	tests := []struct {
		name string
		str  string
		err  error
	}{
		{
			name: "empty string",
			str:  "",
			err:  ErrUint128InvalidString,
		},
		{
			name: "float string",
			str:  "0.3",
			err:  ErrUint128InvalidString,
		},
		{
			name: "negative string",
			str:  "-1",
			err:  ErrUint128Underflow,
		},
		{
			name: "zero",
			str:  "0",
			err:  nil,
		},
		{
			name: "max",
			str:  "340282366920938463463374607431768211455",
			err:  nil,
		},
		{
			name: "max+1",
			str:  "340282366920938463463374607431768211456",
			err:  ErrUint128Overflow,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewUint128FromString(test.str)
			assert.Equal(t, test.err, err)
		})
	}
}

func TestUint128(t *testing.T) {
	bigInt0 := big.NewInt(0)

	bigInt1 := big.NewInt(1)
	bigIntNeg1 := big.NewInt(-1)

	bigMaxUint8 := &big.Int{}
	bigMaxUint8.SetUint64(uint64(math.MaxUint8))

	bigMaxUint64 := &big.Int{}
	bigMaxUint64.SetUint64(^uint64(0))

	bigMaxUint64Add1 := &big.Int{}
	bigMaxUint64Add1.Add(bigMaxUint64, big.NewInt(1))

	bigUint128 := &big.Int{}
	bigUint128.Mul(bigMaxUint64, big.NewInt(100000000000000000))

	bigMaxUint128 := &big.Int{}
	bigMaxUint128.SetString(strings.Repeat("f", 32), 16)

	bigMaxUint128Add1 := &big.Int{}
	bigMaxUint128Add1.Add(bigMaxUint128, big.NewInt(1))

	tests := []struct {
		input       *big.Int // input
		expected    [16]byte // expected Big-Endian result
		expectedErr error
	}{
		{bigInt0, [16]byte{
			0, 0, 0, 0,
			0, 0, 0, 0,
			0, 0, 0, 0,
			0, 0, 0, 0}, nil},
		{bigInt1, [16]byte{
			0, 0, 0, 0,
			0, 0, 0, 0,
			0, 0, 0, 0,
			0, 0, 0, 1}, nil},
		{bigIntNeg1, [16]byte{}, ErrUint128Underflow},
		{bigMaxUint8, [16]byte{
			0, 0, 0, 0,
			0, 0, 0, 0,
			0, 0, 0, 0,
			0, 0, 0, 255}, nil},
		{bigMaxUint64, [16]byte{
			0, 0, 0, 0,
			0, 0, 0, 0,
			255, 255, 255, 255,
			255, 255, 255, 255}, nil},
		{bigMaxUint64Add1, [16]byte{
			0, 0, 0, 0,
			0, 0, 0, 1,
			0, 0, 0, 0,
			0, 0, 0, 0}, nil},
		{bigUint128, [16]byte{
			1, 99, 69, 120,
			93, 137, 255, 255,
			254, 156, 186, 135,
			162, 118, 0, 0}, nil},
		{bigMaxUint128, [16]byte{
			255, 255, 255, 255,
			255, 255, 255, 255,
			255, 255, 255, 255,
			255, 255, 255, 255}, nil},
		{bigMaxUint128Add1, [16]byte{}, ErrUint128Overflow},
	}
	for _, tt := range tests {
		u1, err := NewUint128FromBigInt(tt.input)
		assert.Equal(t, tt.expectedErr, err)
		if err != nil {
			continue
		}

		fsb := u1.Bytes()
		assert.Nil(t, u1.Validate())
		assert.Equal(t, tt.expected, fsb)

		u, err := NewUint128FromBytes(tt.expected[:])
		assert.Nil(t, err)
		fmt.Println(u.String())

		u2, err := NewUint128FromBytes(fsb[:])
		assert.Equal(t, u1.Bytes(), u2.Bytes())
	}
}

func TestUint128Operation(t *testing.T) {
	a, _ := NewUint128FromInt(10)
	b, _ := NewUint128FromInt(9)
	z, _ := NewUint128FromInt(0)
	tmp := NewUint128FromUint(uint64(1 << 63))
	assert.Equal(t, tmp.value.Bytes(), []byte{0x80, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0})

	sumExpect, _ := NewUint128FromInt(19)
	sumResult, _ := a.Add(b)
	assert.Equal(t, sumExpect.Bytes(), sumResult.Bytes())

	diffExpect, _ := NewUint128FromInt(1)
	diffResult, _ := a.Sub(b)
	assert.Equal(t, diffExpect.Bytes(), diffResult.Bytes())
	_, err := b.Sub(a)
	assert.Equal(t, ErrUint128Underflow, err)

	productExpect, _ := NewUint128FromInt(90)
	productResult, _ := a.Mul(b)
	assert.Equal(t, productExpect.Bytes(), productResult.Bytes())

	quotientExpect, _ := NewUint128FromInt(1)
	quotientResult, _ := a.Div(b)
	assert.Equal(t, quotientExpect.Bytes(), quotientResult.Bytes())
	_, err = a.Div(z)
	assert.Equal(t, err, ErrUint128DividedByZero)

	remainResult, _ := a.Mod(b)
	assert.Equal(t, quotientExpect.Bytes(), remainResult.Bytes())
	_, err = a.Mod(z)
	assert.Equal(t, ErrUint128DividedByZero, err)

	powerExpect, _ := NewUint128FromInt(1000000000)
	powerResult, _ := a.Exp(b)
	assert.Equal(t, powerExpect.Bytes(), powerResult.Bytes())
	powerResult, _ = a.Exp(z)
	assert.Equal(t, uint64(1), powerResult.UintValue())

	c := a.DeepCopy()
	c.value.SetUint64(2)
	assert.NotEqual(t, a.Bytes(), c.Bytes())

	assert.Equal(t, a.Cmp(b), 1)
	assert.Equal(t, b.Cmp(a), -1)
	assert.Equal(t, a.Cmp(a), 0)

	rshExpect := a
	rshResult := a.Rsh(0)
	assert.Equal(t, rshExpect.UintValue(), rshResult.UintValue())

	rshExpect, _ = NewUint128FromInt(5)
	rshResult = a.Rsh(1)
	assert.Equal(t, rshExpect.UintValue(), rshResult.UintValue())

	rshExpect, _ = NewUint128FromInt(0)
	rshResult = a.Rsh(100000)
	assert.Equal(t, rshExpect.UintValue(), rshResult.UintValue())
	assert.True(t, rshExpect.IsZero())

	// Float64()
	bigMaxUint128 := &big.Int{}
	bigMaxUint128.SetString(strings.Repeat("f", 32), 16)
	u, err := NewUint128FromBigInt(bigMaxUint128)
	assert.Nil(t, err)
	assert.Equal(t, 3.402823669209385e+38, u.Float64())

	// // greater than maximum uint64
	// u, err = NewUint128FromUint(math.MaxUint64).AddUint(1)
	// assert.Nil(t, err)
	// t.Log(u.UintValue())

	// // greater than maximum int64
	// u, err = NewUint128FromUint(math.MaxInt64).AddUint(1)
	// assert.Nil(t, err)
	// t.Log(u.IntValue())
}
