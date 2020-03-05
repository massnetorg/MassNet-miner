package massutil

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/btcsuite/btcd/btcec"
	"golang.org/x/crypto/ripemd160"
	"massnet.org/mass/config"
	"massnet.org/mass/massutil/base58"
	"massnet.org/mass/massutil/bech32"
)

// address version is resulted from:
//			(witness verison) << 2 | (witness extension version)
const (
	// witness v0 address = 0x00 << 2 | 0x00
	// StandardVersion              = 0x0000
	AddressClassWitnessV0 uint16 = 0x0000

	// witness staking address = 0x00 << 2 | 0x01
	// StakingTxVersion                  = 0x0001
	AddressClassWitnessStaking uint16 = 0x0001
)

// UnsupportedWitnessVerError describes an error where a segwit address being
// decoded has an unsupported witness version.
type UnsupportedWitnessVerError byte

func (e UnsupportedWitnessVerError) Error() string {
	return fmt.Sprintf("unsupported witness version: %d", e)
}

type UnsupportedWitnessExtVerError byte

func (e UnsupportedWitnessExtVerError) Error() string {
	return fmt.Sprintf("unsupported witness extend version: %d", e)
}

// UnsupportedWitnessProgLenError describes an error where a segwit address
// being decoded has an unsupported witness program length.
type UnsupportedWitnessProgLenError int

func (e UnsupportedWitnessProgLenError) Error() string {
	return fmt.Sprintf("unsupported witness program length: %d", e)
}

var (
	// ErrChecksumMismatch describes an error where decoding failed due
	// to a bad checksum.
	ErrChecksumMismatch = errors.New("checksum mismatch")

	// ErrUnknownAddressType describes an error where an address can not
	// decoded as a specific address type due to the string encoding
	// begining with an identifier byte unknown to any standard or
	// registered (via config.Register) network.
	ErrUnknownAddressType = errors.New("unknown address type")

	// ErrAddressCollision describes an error where an address can not
	// be uniquely determined as either a pay-to-pubkey-hash or
	// pay-to-script-hash address since the leading identifier is used for
	// describing both address kinds, but for different networks.  Rather
	// than assuming or defaulting to one or the other, this error is
	// returned and the caller must decide how to decode the address.
	ErrAddressCollision = errors.New("address collision")
)

// encodeAddress returns a human-readable payment address given a ripemd160 hash
// and netID which encodes the Mass network and address type.  It is used
// in both pay-to-pubkey-hash (P2PKH) and pay-to-script-hash (P2SH) address
// encoding.
func encodeAddress(hash160 []byte, netID byte) string {
	// Format is 1 byte for a network and address class (i.e. P2PKH vs
	// P2SH), 20 bytes for a RIPEMD160 hash, and 4 bytes of checksum.
	return base58.CheckEncode(hash160[:ripemd160.Size], netID)
}

// encodeSegWitAddress creates a bech32 encoded address string representation
// from witness version, extend version and witness program.
func encodeSegWitAddress(hrp string, witnessVersion, extendVersion byte, witnessProgram []byte) (string, error) {
	// Group the address bytes into 5 bit groups, as this is what is used to
	// encode each character in the address string.
	converted, err := bech32.ConvertBits(witnessProgram, 8, 5, true)
	if err != nil {
		return "", err
	}

	// Concatenate the witness version and program, and encode the resulting
	// bytes using bech32 encoding.
	combined := make([]byte, len(converted)+2)
	combined[0] = witnessVersion
	combined[1] = extendVersion
	copy(combined[2:], converted)
	bech, err := bech32.Encode(hrp, combined)
	if err != nil {
		return "", err
	}

	// Check validity by decoding the created address.
	version, extVer, program, err := decodeSegWitAddress(bech)
	if err != nil {
		return "", fmt.Errorf("invalid segwit address: %v", err)
	}

	if version != witnessVersion || extVer != extendVersion || !bytes.Equal(program, witnessProgram) {
		return "", fmt.Errorf("invalid segwit address")
	}

	return bech, nil
}

// Address is an interface type for any type of destination a transaction
// output may spend to.  This includes pay-to-pubkey (P2PK), pay-to-pubkey-hash
// (P2PKH), and pay-to-script-hash (P2SH).  Address is designed to be generic
// enough that other kinds of addresses may be added in the future without
// changing the decoding and encoding API.
type Address interface {
	// String returns the string encoding of the transaction output
	// destination.
	//
	// Please note that String differs subtly from EncodeAddress: String
	// will return the value as a string without any conversion, while
	// EncodeAddress may convert destination types (for example,
	// converting pubkeys to P2PKH addresses) before encoding as a
	// payment address string.
	String() string

	// EncodeAddress returns the string encoding of the payment address
	// associated with the Address value.  See the comment on String
	// for how this method differs from String.
	EncodeAddress() string

	// ScriptAddress returns the raw bytes of the address to be used
	// when inserting the address into a txout's script.
	ScriptAddress() []byte

	// IsForNet returns whether or not the address is associated with the
	// passed Mass network.
	IsForNet(*config.Params) bool
}

// DecodeAddress decodes the string encoding of an address and returns
// the Address if addr is a valid encoding for a known address type.
//
// The Mass network the address is associated with is extracted if possible.
// When the address does not encode the network, such as in the case of a raw
// public key, the address will be associated with the passed defaultNet.
func DecodeAddress(addr string, defaultNet *config.Params) (Address, error) {
	// Bech32 encoded segwit addresses start with a human-readable part
	// (hrp) followed by '1'. For Mass mainnet the hrp is "ms", and for
	// testnet it is "tb". If the address string has a prefix that matches
	// one of the prefixes for the known networks, we try to decode it as
	// a segwit address.
	oneIndex := strings.LastIndexByte(addr, '1')
	if oneIndex > 1 {
		prefix := addr[:oneIndex+1]
		if config.IsBech32SegwitPrefix(prefix) {
			witnessVer, witnessExtVer, witnessProg, err := decodeSegWitAddress(addr)
			if err != nil {
				return nil, err
			}

			// The HRP is everything before the found '1'.
			hrp := prefix[:len(prefix)-1]

			// We currently only support P2WPKH and P2WSH, which is
			// witness version 0.
			if witnessVer != 0 {
				return nil, UnsupportedWitnessVerError(witnessVer)
			}

			if witnessExtVer > 1 {
				return nil, UnsupportedWitnessExtVerError(witnessExtVer)
			}

			switch len(witnessProg) {
			case 32:
				return newAddressWitnessScriptHash(hrp, witnessExtVer, witnessProg)
			default:
				return nil, UnsupportedWitnessProgLenError(len(witnessProg))
			}

			// // stakingTx
			// switch witnessVer {
			// case 0:
			// 	return newAddressWitnessScriptHash(hrp, witnessProg)
			// case 10:
			// 	return newAddressStakingScriptHash(hrp, witnessProg)
			// default:
			// 	return nil, UnsupportedWitnessProgLenError(len(witnessProg))
			// }
		}
	}

	// Serialized public keys are either 65 bytes (130 hex chars) if
	// uncompressed/hybrid or 33 bytes (66 hex chars) if compressed.
	if len(addr) == 130 || len(addr) == 66 {
		serializedPubKey, err := hex.DecodeString(addr)
		if err != nil {
			return nil, err
		}
		return NewAddressPubKey(serializedPubKey, defaultNet)
	}

	// Switch on decoded length to determine the type.
	decoded, netID, err := base58.CheckDecode(addr)
	if err != nil {
		if err == base58.ErrChecksum {
			return nil, ErrChecksumMismatch
		}
		return nil, errors.New("decoded address is of unknown format")
	}
	switch len(decoded) {
	case ripemd160.Size: // P2PKH or P2SH
		isP2PKH := config.IsPubKeyHashAddrID(netID)
		isP2SH := config.IsScriptHashAddrID(netID)
		switch hash160 := decoded; {
		case isP2PKH && isP2SH:
			return nil, ErrAddressCollision
		case isP2PKH:
			return newAddressPubKeyHash(hash160, netID)
		case isP2SH:
			return newAddressScriptHashFromHash(hash160, netID)
		default:
			return nil, ErrUnknownAddressType
		}

	default:
		return nil, errors.New("decoded address is of unknown size")
	}
}

// decodeSegWitAddress parses a bech32 encoded segwit address string and
// returns the witness version, witness extend version and witness program byte representation.
func decodeSegWitAddress(address string) (byte, byte, []byte, error) {
	// Decode the bech32 encoded address.
	_, data, err := bech32.Decode(address)
	if err != nil {
		return 0, 0, nil, err
	}

	// The first byte of the decoded address is the witness version, it must
	// exist.
	if len(data) < 1 {
		return 0, 0, nil, fmt.Errorf("no witness version")
	}

	// ...and be <= 16.
	version := data[0]
	if version > 16 {
		return 0, 0, nil, fmt.Errorf("invalid witness version: %v", version)
	}

	extVersion := data[1]
	if extVersion > 16 {
		return 0, 0, nil, fmt.Errorf("invalid witness extend version: %v", extVersion)
	}

	// The remaining characters of the address returned are grouped into
	// words of 5 bits. In order to restore the original witness program
	// bytes, we'll need to regroup into 8 bit words.
	regrouped, err := bech32.ConvertBits(data[2:], 5, 8, false)
	if err != nil {
		return 0, 0, nil, err
	}

	// The regrouped data must be between 2 and 40 bytes.
	if len(regrouped) < 2 || len(regrouped) > 40 {
		return 0, 0, nil, fmt.Errorf("invalid data length")
	}

	// For witness version 0, address MUST be exactly 32 bytes.
	if version == 0 && len(regrouped) != 32 {
		return 0, 0, nil, fmt.Errorf("invalid data length for witness "+
			"version 0: %v", len(regrouped))
	}
	return version, extVersion, regrouped, nil
}

// AddressPubKeyHash is an Address for a pay-to-pubkey-hash (P2PKH)
// transaction.
type AddressPubKeyHash struct {
	hash  [ripemd160.Size]byte
	netID byte
}

// NewAddressPubKeyHash returns a new AddressPubKeyHash.  pkHash mustbe 20
// bytes.
func NewAddressPubKeyHash(pkHash []byte, net *config.Params) (*AddressPubKeyHash, error) {
	return newAddressPubKeyHash(pkHash, net.PubKeyHashAddrID)
}

// newAddressPubKeyHash is the internal API to create a pubkey hash address
// with a known leading identifier byte for a network, rather than looking
// it up through its parameters.  This is useful when creating a new address
// structure from a string encoding where the identifer byte is already
// known.
func newAddressPubKeyHash(pkHash []byte, netID byte) (*AddressPubKeyHash, error) {
	// Check for a valid pubkey hash length.
	if len(pkHash) != ripemd160.Size {
		return nil, errors.New("pkHash must be 20 bytes")
	}

	addr := &AddressPubKeyHash{netID: netID}
	copy(addr.hash[:], pkHash)
	return addr, nil
}

// EncodeAddress returns the string encoding of a pay-to-pubkey-hash
// address.  Part of the Address interface.
func (a *AddressPubKeyHash) EncodeAddress() string {
	return encodeAddress(a.hash[:], a.netID)
}

// ScriptAddress returns the bytes to be included in a txout script to pay
// to a pubkey hash.  Part of the Address interface.
func (a *AddressPubKeyHash) ScriptAddress() []byte {
	return a.hash[:]
}

// IsForNet returns whether or not the pay-to-pubkey-hash address is associated
// with the passed Mass network.
func (a *AddressPubKeyHash) IsForNet(net *config.Params) bool {
	return a.netID == net.PubKeyHashAddrID
}

// String returns a human-readable string for the pay-to-pubkey-hash address.
// This is equivalent to calling EncodeAddress, but is provided so the type can
// be used as a fmt.Stringer.
func (a *AddressPubKeyHash) String() string {
	return a.EncodeAddress()
}

// Hash160 returns the underlying array of the pubkey hash.  This can be useful
// when an array is more appropiate than a slice (for example, when used as map
// keys).
func (a *AddressPubKeyHash) Hash160() *[ripemd160.Size]byte {
	return &a.hash
}

// AddressScriptHash is an Address for a pay-to-script-hash (P2SH)
// transaction.
type AddressScriptHash struct {
	hash  [ripemd160.Size]byte
	netID byte
}

// NewAddressScriptHash returns a new AddressScriptHash.
func NewAddressScriptHash(serializedScript []byte, net *config.Params) (*AddressScriptHash, error) {
	scriptHash := Hash160(serializedScript)
	return newAddressScriptHashFromHash(scriptHash, net.ScriptHashAddrID)
}

// NewAddressScriptHashFromHash returns a new AddressScriptHash.  scriptHash
// must be 20 bytes.
func NewAddressScriptHashFromHash(scriptHash []byte, net *config.Params) (*AddressScriptHash, error) {
	return newAddressScriptHashFromHash(scriptHash, net.ScriptHashAddrID)
}

// newAddressScriptHashFromHash is the internal API to create a script hash
// address with a known leading identifier byte for a network, rather than
// looking it up through its parameters.  This is useful when creating a new
// address structure from a string encoding where the identifer byte is already
// known.
func newAddressScriptHashFromHash(scriptHash []byte, netID byte) (*AddressScriptHash, error) {
	// Check for a valid script hash length.
	if len(scriptHash) != ripemd160.Size {
		return nil, errors.New("scriptHash must be 20 bytes")
	}

	addr := &AddressScriptHash{netID: netID}
	copy(addr.hash[:], scriptHash)
	return addr, nil
}

// EncodeAddress returns the string encoding of a pay-to-script-hash
// address.  Part of the Address interface.
func (a *AddressScriptHash) EncodeAddress() string {
	return encodeAddress(a.hash[:], a.netID)
}

// ScriptAddress returns the bytes to be included in a txout script to pay
// to a script hash.  Part of the Address interface.
func (a *AddressScriptHash) ScriptAddress() []byte {
	return a.hash[:]
}

// IsForNet returns whether or not the pay-to-script-hash address is associated
// with the passed Mass network.
func (a *AddressScriptHash) IsForNet(net *config.Params) bool {
	return a.netID == net.ScriptHashAddrID
}

// String returns a human-readable string for the pay-to-script-hash address.
// This is equivalent to calling EncodeAddress, but is provided so the type can
// be used as a fmt.Stringer.
func (a *AddressScriptHash) String() string {
	return a.EncodeAddress()
}

// Hash160 returns the underlying array of the script hash.  This can be useful
// when an array is more appropiate than a slice (for example, when used as map
// keys).
func (a *AddressScriptHash) Hash160() *[ripemd160.Size]byte {
	return &a.hash
}

// PubKeyFormat describes what format to use for a pay-to-pubkey address.
type PubKeyFormat int

const (
	// PKFUncompressed indicates the pay-to-pubkey address format is an
	// uncompressed public key.
	PKFUncompressed PubKeyFormat = iota

	// PKFCompressed indicates the pay-to-pubkey address format is a
	// compressed public key.
	PKFCompressed

	// PKFHybrid indicates the pay-to-pubkey address format is a hybrid
	// public key.
	PKFHybrid
)

// AddressPubKey is an Address for a pay-to-pubkey transaction.
type AddressPubKey struct {
	pubKeyFormat PubKeyFormat
	pubKey       *btcec.PublicKey
	pubKeyHashID byte
}

// NewAddressPubKey returns a new AddressPubKey which represents a pay-to-pubkey
// address.  The serializedPubKey parameter must be a valid pubkey and can be
// uncompressed, compressed, or hybrid.
func NewAddressPubKey(serializedPubKey []byte, net *config.Params) (*AddressPubKey, error) {
	pubKey, err := btcec.ParsePubKey(serializedPubKey, btcec.S256())
	if err != nil {
		return nil, err
	}

	// Set the format of the pubkey.  This probably should be returned
	// from btcec, but do it here to avoid API churn.  We already know the
	// pubkey is valid since it parsed above, so it's safe to simply examine
	// the leading byte to get the format.
	pkFormat := PKFUncompressed
	switch serializedPubKey[0] {
	case 0x02, 0x03:
		pkFormat = PKFCompressed
	case 0x06, 0x07:
		pkFormat = PKFHybrid
	}
	return &AddressPubKey{
		pubKeyFormat: pkFormat,
		pubKey:       pubKey,
		pubKeyHashID: net.PubKeyHashAddrID,
	}, nil
}

// serialize returns the serialization of the public key according to the
// format associated with the address.
func (a *AddressPubKey) serialize() []byte {
	switch a.pubKeyFormat {
	default:
		fallthrough
	case PKFUncompressed:
		return a.pubKey.SerializeUncompressed()

	case PKFCompressed:
		return a.pubKey.SerializeCompressed()

	case PKFHybrid:
		return a.pubKey.SerializeHybrid()
	}
}

// EncodeAddress returns the string encoding of the public key as a
// pay-to-pubkey-hash.  Note that the public key format (uncompressed,
// compressed, etc) will change the resulting address.  This is expected since
// pay-to-pubkey-hash is a hash of the serialized public key which obviously
// differs with the format.  At the time of this writing, most Mass addresses
// are pay-to-pubkey-hash constructed from the uncompressed public key.
//
// Part of the Address interface.
func (a *AddressPubKey) EncodeAddress() string {
	return encodeAddress(Hash160(a.serialize()), a.pubKeyHashID)
}

// ScriptAddress returns the bytes to be included in a txout script to pay
// to a public key.  Setting the public key format will affect the output of
// this function accordingly.  Part of the Address interface.
func (a *AddressPubKey) ScriptAddress() []byte {
	return a.serialize()
}

// IsForNet returns whether or not the pay-to-pubkey address is associated
// with the passed Mass network.
func (a *AddressPubKey) IsForNet(net *config.Params) bool {
	return a.pubKeyHashID == net.PubKeyHashAddrID
}

// String returns the hex-encoded human-readable string for the pay-to-pubkey
// address.  This is not the same as calling EncodeAddress.
func (a *AddressPubKey) String() string {
	return hex.EncodeToString(a.serialize())
}

// Format returns the format (uncompressed, compressed, etc) of the
// pay-to-pubkey address.
func (a *AddressPubKey) Format() PubKeyFormat {
	return a.pubKeyFormat
}

// SetFormat sets the format (uncompressed, compressed, etc) of the
// pay-to-pubkey address.
func (a *AddressPubKey) SetFormat(pkFormat PubKeyFormat) {
	a.pubKeyFormat = pkFormat
}

// AddressPubKeyHash returns the pay-to-pubkey address converted to a
// pay-to-pubkey-hash address.  Note that the public key format (uncompressed,
// compressed, etc) will change the resulting address.  This is expected since
// pay-to-pubkey-hash is a hash of the serialized public key which obviously
// differs with the format.  At the time of this writing, most Mass addresses
// are pay-to-pubkey-hash constructed from the uncompressed public key.
func (a *AddressPubKey) AddressPubKeyHash() *AddressPubKeyHash {
	addr := &AddressPubKeyHash{netID: a.pubKeyHashID}
	copy(addr.hash[:], Hash160(a.serialize()))
	return addr
}

// PubKey returns the underlying public key for the address.
func (a *AddressPubKey) PubKey() *btcec.PublicKey {
	return a.pubKey
}

// AddressWitnessScriptHash is an Address for a pay-to-witness-script-hash
// (P2WSH) output. See BIP 173 for further details regarding native segregated
// witness address encoding:
// https://github.com/Mass/bips/blob/master/bip-0173.mediawiki
type AddressWitnessScriptHash struct {
	hrp            string
	witnessVersion byte
	extendVersion  byte
	witnessProgram [32]byte
}

// NewAddressWitnessScriptHash returns a new AddressWitnessPubKeyHash.
func NewAddressWitnessScriptHash(witnessProg []byte, net *config.Params) (*AddressWitnessScriptHash, error) {
	return newAddressWitnessScriptHash(net.Bech32HRPSegwit, 0, witnessProg)
}

//NewAddressStakingScriptHash returns a new Address for loctime transaction
func NewAddressStakingScriptHash(witnessProg []byte, net *config.Params) (*AddressWitnessScriptHash, error) {
	return newAddressWitnessScriptHash(net.Bech32HRPSegwit, 1, witnessProg)
}

// newAddressWitnessScriptHash is an internal helper function to create an
// AddressWitnessScriptHash with a known human-readable part, rather than
// looking it up through its parameters.
func newAddressWitnessScriptHash(hrp string, extendVersion byte, witnessProg []byte) (*AddressWitnessScriptHash, error) {
	// Check for valid program length for witness version 0, which is 32
	// for P2WSH.
	if len(witnessProg) != 32 {
		return nil, errors.New("witness program must be 32 " +
			"bytes for p2wsh")
	}

	if extendVersion > 1 {
		return nil, errors.New("unknown witness extend version: " + string(extendVersion))
	}

	addr := &AddressWitnessScriptHash{
		hrp:            strings.ToLower(hrp),
		witnessVersion: 0,
		extendVersion:  extendVersion,
	}

	copy(addr.witnessProgram[:], witnessProg)

	return addr, nil
}

// EncodeAddress returns the bech32 string encoding of an
// AddressWitnessScriptHash.
// Part of the Address interface.
func (a *AddressWitnessScriptHash) EncodeAddress() string {
	str, err := encodeSegWitAddress(a.hrp, a.witnessVersion,
		a.extendVersion, a.witnessProgram[:])
	if err != nil {
		return ""
	}
	return str
}

// ScriptAddress returns the witness program for this address.
// Part of the Address interface.
func (a *AddressWitnessScriptHash) ScriptAddress() []byte {
	return a.witnessProgram[:]
}

// IsForNet returns whether or not the AddressWitnessScriptHash is associated
// with the passed Mass network.
// Part of the Address interface.
func (a *AddressWitnessScriptHash) IsForNet(net *config.Params) bool {
	return a.hrp == net.Bech32HRPSegwit
}

// String returns a human-readable string for the AddressWitnessScriptHash.
// This is equivalent to calling EncodeAddress, but is provided so the type
// can be used as a fmt.Stringer.
// Part of the Address interface.
func (a *AddressWitnessScriptHash) String() string {
	return a.EncodeAddress()
}

// Hrp returns the human-readable part of the bech32 encoded
// AddressWitnessScriptHash.
func (a *AddressWitnessScriptHash) Hrp() string {
	return a.hrp
}

// WitnessVersion returns the witness version of the AddressWitnessScriptHash.
func (a *AddressWitnessScriptHash) WitnessVersion() byte {
	return a.witnessVersion
}

func (a *AddressWitnessScriptHash) WitnessExtendVersion() byte {
	return a.extendVersion
}

// WitnessProgram returns the witness program of the AddressWitnessScriptHash.
func (a *AddressWitnessScriptHash) WitnessProgram() []byte {
	return a.witnessProgram[:]
}

func NewAddressesFromStringList(strList []string, net *config.Params) ([]Address, error) {
	AddrList := make([]Address, 0, len(strList))
	for _, strAddr := range strList {
		addr, err := DecodeAddress(strAddr, net)
		if err != nil {
			return nil, errors.New("decode address failed to decode")
		}
		if !addr.IsForNet(net) {
			return nil, errors.New("address is on the wrong network")
		}
		AddrList = append(AddrList, addr)
	}
	return AddrList, nil
}

func IsWitnessV0Address(ad interface{}) bool {
	if ad == nil {
		return false
	}
	switch addr := ad.(type) {
	case *AddressWitnessScriptHash:
		return addr.WitnessVersion() == 0 && addr.WitnessExtendVersion() == 0
	}
	return false
}

func IsWitnessStakingAddress(ad interface{}) bool {
	if ad == nil {
		return false
	}
	switch addr := ad.(type) {
	case *AddressWitnessScriptHash:
		return addr.WitnessVersion() == 0 && addr.WitnessExtendVersion() == 1
	}
	return false
}

func IsAddressWitnessScriptHash(ad interface{}) bool {
	if ad == nil {
		return false
	}
	_, ok := ad.(*AddressWitnessScriptHash)
	return ok
}

func IsAddressPubKeyHash(ad interface{}) bool {
	if ad == nil {
		return false
	}
	_, ok := ad.(*AddressPubKeyHash)
	return ok
}

func IsValidAddressClass(class uint16) bool {
	return class == AddressClassWitnessV0 ||
		class == AddressClassWitnessStaking
}
