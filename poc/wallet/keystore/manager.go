package keystore

import (
	"sync"

	"crypto/rand"
	"fmt"

	"crypto/sha512"
	"encoding/hex"

	"bytes"

	"massnet.org/mass/config"
	"massnet.org/mass/errors"
	"massnet.org/mass/logging"
	"massnet.org/mass/poc/wallet/db"
	"massnet.org/mass/poc/wallet/keystore/hdkeychain"
	"massnet.org/mass/poc/wallet/keystore/snacl"
	"massnet.org/mass/poc/wallet/keystore/zero"
	"massnet.org/mass/pocec"
	"massnet.org/mass/wire"
)

const (
	PoCUsage    AddrUse = iota //0
	WalletUsage                //1

	ksMgrBucket     = "km"
	accountIDBucket = "aid"
	pubKeyBucket    = "pub"

	// MaxAccountNum is the maximum allowed account number.  This value was
	// chosen because accounts are hardened children and therefore must not
	// exceed the hardened child range of extended keys and it provides a
	// reserved account at the top of the range for supporting imported
	// addresses.
	MaxAccountNum = hdkeychain.HardenedKeyStart - 2 // 2^31 - 2

	// MaxAddressesPerAccount is the maximum allowed number of addresses
	// per account number.  This value is based on the limitation of the
	// underlying hierarchical deterministic key derivation.
	MaxAddressesPerAccount = hdkeychain.HardenedKeyStart - 1

	// ExternalBranch is the child number to use when performing BIP0044
	// style hierarchical deterministic key derivation for the external
	// branch.
	ExternalBranch uint32 = 0

	// InternalBranch is the child number to use when performing BIP0044
	// style hierarchical deterministic key derivation for the internal
	// branch.
	InternalBranch uint32 = 1

	// maxCoinType is the maximum allowed coin type used when structuring
	// the BIP0044 multi-account hierarchy.  This value is based on the
	// limitation of the underlying hierarchical deterministic key
	// derivation.
	maxCoinType = hdkeychain.HardenedKeyStart - 1

	// saltSize is the number of bytes of the salt used when hashing
	// private passphrase.
	saltSize = 32
)

type AddrUse uint8

type KeystoreManagerForPoC struct {
	mu               sync.Mutex
	managedKeystores map[string]*AddrManager
	params           *config.Params
	ksMgrMeta        db.BucketMeta
	accountIDMeta    db.BucketMeta
	pubPassphrase    []byte
	unlocked         bool
	db               db.DB
}

// ScryptOptions is used to hold the scrypt parameters needed when deriving new
// passphrase keys.
type ScryptOptions struct {
	N, R, P int
}

// DefaultScryptOptions is the default options used with scrypt.
var DefaultScryptOptions = ScryptOptions{
	N: 262144, // 2^18
	R: 8,
	P: 1,
}

// defaultNewSecretKey returns a new secret key.  See newSecretKey.
func defaultNewSecretKey(passphrase *[]byte, config *ScryptOptions) (*snacl.SecretKey, error) {
	return snacl.NewSecretKey(passphrase, config.N, config.R, config.P)
}

var (
	// secretKeyGen is the inner method that is executed when calling
	// newSecretKey.
	secretKeyGen = defaultNewSecretKey
)

// EncryptorDecryptor provides an abstraction on top of snacl.CryptoKey so that
// our tests can use dependency injection to force the behaviour they need.
type EncryptorDecryptor interface {
	Encrypt(in []byte) ([]byte, error)
	Decrypt(in []byte) ([]byte, error)
	Bytes() []byte
	CopyBytes([]byte)
	Zero()
}

// cryptoKey extends snacl.CryptoKey to implement EncryptorDecryptor.
type cryptoKey struct {
	snacl.CryptoKey
}

// Bytes returns a copy of this crypto key's byte slice.
func (ck *cryptoKey) Bytes() []byte {
	return ck.CryptoKey[:]
}

// CopyBytes copies the bytes from the given slice into this CryptoKey.
func (ck *cryptoKey) CopyBytes(from []byte) {
	copy(ck.CryptoKey[:], from)
}

// defaultNewCryptoKey returns a new CryptoKey.  See newCryptoKey.
func defaultNewCryptoKey() (EncryptorDecryptor, error) {
	key, err := snacl.GenerateCryptoKey()
	if err != nil {
		return nil, err
	}
	return &cryptoKey{*key}, nil
}

// newCryptoKey is used as a way to replace the new crypto key generation
// function used so tests can provide a version that fails for testing error
// paths.
var newCryptoKey = defaultNewCryptoKey

// generate seed
func GenerateSeed() ([]byte, error) {
	hdSeed, err := hdkeychain.GenerateSeed(hdkeychain.RecommendedSeedLen)
	if err != nil {
		return nil, err
	}
	return hdSeed, nil
}

// createManagerKeyScope creates a new key scoped for a target manager's scope.
// This partitions key derivation for a particular purpose+coin tuple, allowing
// multiple address derivation schems to be maintained concurrently.
func createManagerKeyScope(km db.Bucket, root *hdkeychain.ExtendedKey,
	cryptoKeyPub, cryptoKeyPriv EncryptorDecryptor, hdPath *hdPath, net *config.Params) (db.BucketMeta, error) {

	scope := Net2KeyScope[net.HDCoinType]

	accountIDBucket, err := db.GetOrCreateBucket(km, accountIDBucket)
	if err != nil {
		return nil, err
	}

	// Derive the cointype key according to the passed scope.
	coinTypeKeyPriv, err := deriveCoinTypeKey(root, scope)
	if err != nil {
		return nil, err
	}
	defer coinTypeKeyPriv.Zero()

	// Derive the account key for the first account according our
	// BIP0044-like derivation.
	acctKeyPriv, err := deriveAccountKey(coinTypeKeyPriv, hdPath.Account)
	if err != nil {
		// The seed is unusable if the any of the children in the
		// required hierarchy can't be derived due to invalid child.
		if err == hdkeychain.ErrInvalidChild {
			str := "the provided seed is unusable"
			return nil, errors.New(str)
		}

		return nil, err
	}
	defer acctKeyPriv.Zero()

	acctKeyPub, err := acctKeyPriv.Neuter()
	if err != nil {
		return nil, fmt.Errorf("failed to convert private key for account 0: %v", err)
	}
	acctEcPubKey, err := acctKeyPub.ECPubKey()
	if err != nil {
		return nil, err
	}

	accountID, err := pubKeyToAccountID(acctEcPubKey)
	if err != nil {
		return nil, err
	}

	// check for repeated seed
	value, _ := accountIDBucket.Get([]byte(accountID))
	if value != nil {
		return nil, ErrDuplicateSeed
	}

	// Ensure the branch keys can be derived for the provided seed according
	// to our BIP0044-like derivation.
	if err := checkBranchKeys(acctKeyPriv); err != nil {
		// The seed is unusable if the any of the children in the
		// required hierarchy can't be derived due to invalid child.
		if err == hdkeychain.ErrInvalidChild {
			str := "the provided seed is unusable"
			return nil, errors.New(str)
		}

		return nil, err
	}

	// put account PubKey into seed bucket
	err = putAccountID(accountIDBucket, []byte(accountID))
	if err != nil {
		return nil, err
	}
	// new account bucket
	accountBucket, err := km.NewBucket(accountID)
	if err != nil {
		return nil, err
	}

	// Encrypt the default account keys with the associated crypto keys.
	acctPubEnc, err := cryptoKeyPub.Encrypt([]byte(acctKeyPub.String()))
	if err != nil {
		return nil, fmt.Errorf("failed to  encrypt public key for account 0: %v", err)
	}
	acctPrivEnc, err := cryptoKeyPriv.Encrypt([]byte(acctKeyPriv.String()))
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt private key for account 0: %v", err)
	}

	err = putCoinType(accountBucket, scope.Coin)
	if err != nil {
		return nil, err
	}
	// Save the information for the default account to the database.
	err = putAccountInfo(accountBucket, hdPath.Account, acctPubEnc, acctPrivEnc)
	if err != nil {
		return nil, err
	}

	// new external branch and save the pubkey
	internalBranchPrivKey, err := acctKeyPriv.Child(InternalBranch)
	if err != nil {
		logging.CPrint(logging.ERROR, "new childKey failed",
			logging.LogFormat{
				"err": err,
			})
		return nil, err
	}
	defer internalBranchPrivKey.Zero()
	internalBranchPubKey, err := internalBranchPrivKey.Neuter()
	if err != nil {
		logging.CPrint(logging.ERROR, "exKey->exPubKey failed",
			logging.LogFormat{
				"err": err,
			})
		return nil, err
	}

	externalBranchPrivKey, err := acctKeyPriv.Child(ExternalBranch)
	if err != nil {
		logging.CPrint(logging.ERROR, "new childKey failed",
			logging.LogFormat{
				"err": err,
			})
		return nil, err
	}
	defer externalBranchPrivKey.Zero()
	externalBranchPubKey, err := externalBranchPrivKey.Neuter()
	if err != nil {
		logging.CPrint(logging.ERROR, "exKey->exPubKey failed",
			logging.LogFormat{
				"err": err,
			})
		return nil, err
	}

	if hdPath.InternalChildNum != 0 {
		addressInfo := make([]*unlockDeriveInfo, 0, hdPath.InternalChildNum)
		for i := uint32(0); i < hdPath.InternalChildNum; i++ {
			var nextKey *hdkeychain.ExtendedKey
			for {
				indexKey, err := internalBranchPubKey.Child(i)
				if err != nil {
					if err == hdkeychain.ErrInvalidChild {
						continue
					}
					logging.CPrint(logging.ERROR, "new childKey failed",
						logging.LogFormat{
							"err": err,
						})
					return nil, err
				}
				indexKey.SetNet(net)
				nextKey = indexKey
				break
			}
			// Now that we know this key can be used, we'll create the
			// proper derivation path so this information can be available
			// to callers.
			derivationPath := DerivationPath{
				Account: hdPath.Account,
				Branch:  InternalBranch,
				Index:   i,
			}

			// create a new managed address based on the private key.
			// Also, zero the next key after creating the managed address
			// from it.
			managedAddr, err := newManagedAddressFromExtKey(accountID, derivationPath, nextKey, net)
			if err != nil {
				logging.CPrint(logging.ERROR, "new managedAddress failed",
					logging.LogFormat{
						"err": err,
					})
				return nil, err
			}

			nextKey.Zero()

			info := unlockDeriveInfo{
				managedAddr: managedAddr,
				branch:      InternalBranch,
				index:       i,
			}
			addressInfo = append(addressInfo, &info)
		}

		err = updateChildNum(accountBucket, true, hdPath.InternalChildNum)
		if err != nil {
			logging.CPrint(logging.ERROR, "put db failed",
				logging.LogFormat{
					"err": err,
				})
			return nil, err
		}

		pkBucket, err := db.GetOrCreateBucket(accountBucket, pubKeyBucket)
		if err != nil {
			return nil, err
		}
		for _, info := range addressInfo {
			pubKeyBytes := info.managedAddr.pubKey.SerializeCompressed()
			pubKeyEnc, err := cryptoKeyPub.Encrypt(pubKeyBytes)
			if err != nil {
				return nil, err
			}
			err = putEncryptedPubKey(pkBucket, info.branch, info.index, pubKeyEnc)
			if err != nil {
				return nil, err
			}
		}
	}

	if hdPath.ExternalChildNum != 0 {
		addressInfo := make([]*unlockDeriveInfo, 0, hdPath.ExternalChildNum)
		for i := uint32(0); i < hdPath.ExternalChildNum; i++ {
			var nextKey *hdkeychain.ExtendedKey
			for {
				indexKey, err := externalBranchPubKey.Child(i)
				if err != nil {
					if err == hdkeychain.ErrInvalidChild {
						continue
					}
					logging.CPrint(logging.ERROR, "new childKey failed",
						logging.LogFormat{
							"err": err,
						})
					return nil, err
				}
				indexKey.SetNet(net)
				nextKey = indexKey
				break
			}
			// Now that we know this key can be used, we'll create the
			// proper derivation path so this information can be available
			// to callers.
			derivationPath := DerivationPath{
				Account: hdPath.Account,
				Branch:  ExternalBranch,
				Index:   i,
			}

			// create a new managed address based on the private key.
			// Also, zero the next key after creating the managed address
			// from it.
			managedAddr, err := newManagedAddressFromExtKey(accountID, derivationPath, nextKey, net)
			if err != nil {
				logging.CPrint(logging.ERROR, "new managedAddress failed",
					logging.LogFormat{
						"err": err,
					})
				return nil, err
			}

			nextKey.Zero()

			info := unlockDeriveInfo{
				managedAddr: managedAddr,
				branch:      ExternalBranch,
				index:       i,
			}
			addressInfo = append(addressInfo, &info)
		}

		err = updateChildNum(accountBucket, false, hdPath.ExternalChildNum)
		if err != nil {
			logging.CPrint(logging.ERROR, "put db failed",
				logging.LogFormat{
					"err": err,
				})
			return nil, err
		}

		pkBucket, err := db.GetOrCreateBucket(accountBucket, pubKeyBucket)
		if err != nil {
			return nil, err
		}
		for _, info := range addressInfo {
			pubKeyBytes := info.managedAddr.pubKey.SerializeCompressed()
			pubKeyEnc, err := cryptoKeyPub.Encrypt(pubKeyBytes)
			if err != nil {
				return nil, err
			}
			err = putEncryptedPubKey(pkBucket, info.branch, info.index, pubKeyEnc)
			if err != nil {
				return nil, err
			}
		}
	}

	// Encrypt the default branch keys with the associated crypto keys.
	internalBranchPubEnc, err := cryptoKeyPub.Encrypt([]byte(internalBranchPubKey.String()))
	if err != nil {
		return nil, fmt.Errorf("failed to  encrypt public key for account 0: %v", err)
	}
	externalBranchPubEnc, err := cryptoKeyPub.Encrypt([]byte(externalBranchPubKey.String()))
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt private key for account 0: %v", err)
	}

	err = putBranchPubKeys(accountBucket, internalBranchPubEnc, externalBranchPubEnc)
	if err != nil {
		logging.CPrint(logging.ERROR, "put db failed",
			logging.LogFormat{
				"err": err,
			})
		return nil, err
	}
	err = initBranchChildNum(accountBucket)
	if err != nil {
		logging.CPrint(logging.ERROR, "put db failed",
			logging.LogFormat{
				"err": err,
			})
		return nil, err
	}

	err = putLastIndex(accountBucket, hdPath.ExternalChildNum, hdPath.InternalChildNum)
	if err != nil {
		return nil, err
	}

	return accountBucket.GetBucketMeta(), nil
}

// create creates a new address manager in the given namespace.  The seed must
// conform to the standards described in hdkeychain.NewMaster and will be used
// to create the master root node from which all hierarchical deterministic
// addresses are derived.  This allows all chained addresses in the address
// manager to be recovered by using the same seed.
//
// All private and public keys and information are protected by secret keys
// derived from the provided private and public passphrases.  The public
// passphrase is required on subsequent opens of the address manager, and the
// private passphrase is required to unlock the address manager in order to
// gain access to any private keys and information.
//
// If a config structure is passed to the function, that configuration will
// override the defaults.
//
// A ManagerError with an error code of ErrAlreadyExists will be returned the
// address manager already exists in the specified namespace.
func create(dbTransaction db.DBTransaction, kmBucketMeta db.BucketMeta, pubPassphrase, privPassphrase, seed []byte,
	usage AddrUse, remark string, net *config.Params, scryptConfig *ScryptOptions) (db.BucketMeta, error) {

	if !ValidatePassphrase(privPassphrase) {
		return nil, ErrIllegalPassphrase
	}

	if !ValidatePassphrase(pubPassphrase) {
		return nil, ErrIllegalPassphrase
	}
	if bytes.Equal(privPassphrase, pubPassphrase) {
		return nil, ErrIllegalNewPrivPass
	}

	if len(seed) != 0 && !ValidateSeed(seed) {
		return nil, ErrIllegalSeed
	}

	// if seed not exist, generate one
	if len(seed) == 0 {
		var err error
		seed, err = GenerateSeed()
		if err != nil {
			return nil, err
		}
	}

	if scryptConfig == nil {
		scryptConfig = &DefaultScryptOptions
	}

	// get the km bucket created before
	kmBucket := dbTransaction.FetchBucket(kmBucketMeta)

	if kmBucket == nil {
		return nil, ErrBucketNotFound
	}

	// Generate new master keys.  These master keys are used to protect the
	// crypto keys that will be generated next.
	masterKeyPub, err := secretKeyGen(&pubPassphrase, scryptConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to generate master public key: %v", err)
	}
	masterKeyPriv, err := secretKeyGen(&privPassphrase, scryptConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to generate master private key: %v", err)
	}
	defer masterKeyPriv.Zero()

	// Generate new crypto public, private, and script keys.  These keys are
	// used to protect the actual public and private data such as addresses,
	// extended keys, and scripts.
	cryptoKeyPub, err := newCryptoKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate crypto public key: %v", err)
	}
	cryptoKeyPriv, err := newCryptoKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate crypto private key: %v", err)
	}
	defer cryptoKeyPriv.Zero()

	// Encrypt the crypto keys with the associated master keys.
	cryptoKeyPubEnc, err := masterKeyPub.Encrypt(cryptoKeyPub.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt crypto public key: %v", err)
	}
	cryptoKeyPrivEnc, err := masterKeyPriv.Encrypt(cryptoKeyPriv.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt crypto private key: %v", err)
	}

	// Save the master key params to the database.
	pubParams := masterKeyPub.Marshal()
	privParams := masterKeyPriv.Marshal()

	// Generate the BIP0044 HD key structure to ensure the provided seed
	// can generate the required structure with no issues.

	// Derive the master extended key from the seed.
	rootKey, err := hdkeychain.NewMaster(seed, net)
	if err != nil {
		return nil, fmt.Errorf("failed to derive master extended key: %v", err)
	}
	defer rootKey.Zero()
	rootPubKey, err := rootKey.Neuter()
	if err != nil {
		return nil, fmt.Errorf("failed to neuter master extended key: %v", err)
	}

	// Next, for each registers default manager scope, we'll create the
	// hardened cointype key for it, as well as the first default account.

	// Before we proceed, we'll also store the root master private key
	// within the database in an encrypted format. This is required as in
	// the future, we may need to create additional scoped key managers.
	masterHDPrivKeyEnc, err := cryptoKeyPriv.Encrypt([]byte(rootKey.String()))
	if err != nil {
		return nil, err
	}
	masterHDPubKeyEnc, err := cryptoKeyPub.Encrypt([]byte(rootPubKey.String()))
	if err != nil {
		return nil, err
	}

	hdPath := &hdPath{
		Account:          uint32(usage),
		InternalChildNum: 0,
		ExternalChildNum: 0,
	}
	acctBucketMeta, err := createManagerKeyScope(kmBucket, rootKey,
		cryptoKeyPub, cryptoKeyPriv, hdPath, net)
	if err != nil {
		return nil, err
	}

	acctBucket := dbTransaction.FetchBucket(acctBucketMeta)
	if acctBucket == nil {
		return nil, ErrUnexpecteDBError
	}

	if len(remark) > 0 {
		err = putRemark(acctBucket, []byte(remark))
		if err != nil {
			return nil, err
		}
	}

	err = putMasterKeyParams(acctBucket, pubParams, privParams)
	if err != nil {
		return nil, err
	}

	err = putMasterHDKeys(acctBucket, masterHDPrivKeyEnc, masterHDPubKeyEnc)
	if err != nil {
		return nil, err
	}

	// Save the encrypted crypto keys to the database.
	err = putCryptoKeys(acctBucket, cryptoKeyPubEnc, cryptoKeyPrivEnc)
	if err != nil {
		return nil, err
	}

	return acctBucketMeta, nil
}

func loadAddrManager(amBucket db.Bucket, pubPassphrase []byte, net *config.Params) (*AddrManager, error) {
	amBucketMeta := amBucket.GetBucketMeta()
	// fetch the coin type
	coin, err := fetchCoinType(amBucket)
	if err != nil {
		return nil, err
	}
	keyScope, ok := Net2KeyScope[coin]
	if !ok {
		return nil, ErrKeyScopeNotFound
	}
	if coin != net.HDCoinType {
		str := "the coin type doesn't match the connected network"
		return nil, errors.New(str)
	}

	remarkBytes, err := fetchRemark(amBucket)
	if err != nil {
		return nil, err
	}

	// Load the master key params from the db.
	masterKeyPubParams, masterKeyPrivParams, err := fetchMasterKeyParams(amBucket)
	if err != nil {
		return nil, err
	}

	// Load the crypto keys from the db.
	cryptoKeyPubEnc, cryptoKeyPrivEnc, err := fetchCryptoKeys(amBucket)
	if err != nil {
		return nil, err
	}

	var masterKeyPriv snacl.SecretKey
	err = masterKeyPriv.Unmarshal(masterKeyPrivParams)
	if err != nil {
		str := "failed to unmarshal master private key"
		return nil, errors.New(str)
	}

	// Derive the master public key using the serialized params and provided
	// passphrase.
	var masterKeyPub snacl.SecretKey
	if err := masterKeyPub.Unmarshal(masterKeyPubParams); err != nil {
		str := "failed to unmarshal master public key"
		return nil, errors.New(str)
	}
	if err := masterKeyPub.DeriveKey(&pubPassphrase); err != nil {
		str := "invalid passphrase for master public key"
		return nil, errors.New(str)
	}

	// Use the master public key to decrypt the crypto public key.
	cryptoKeyPub := &cryptoKey{snacl.CryptoKey{}}
	cryptoKeyPubCT, err := masterKeyPub.Decrypt(cryptoKeyPubEnc)
	if err != nil {
		str := "failed to decrypt crypto public key"
		return nil, errors.New(str)
	}
	cryptoKeyPub.CopyBytes(cryptoKeyPubCT)
	zero.Bytes(cryptoKeyPubCT)

	// Generate private passphrase salt.
	var privPassphraseSalt [saltSize]byte
	_, err = rand.Read(privPassphraseSalt[:])
	if err != nil {
		str := "failed to read random source for passphrase salt"
		return nil, errors.New(str)
	}

	// fetch the account usage
	account, err := fetchAccountUsage(amBucket)
	if err != nil {
		return nil, err
	}

	// fetch the account info
	rowInterface, err := fetchAccountInfo(amBucket, account)
	if err != nil {
		return nil, err
	}
	row, ok := rowInterface.(*dbHDAccountKey)
	if !ok {
		str := fmt.Sprintf("unsupported account type %T", row)
		return nil, errors.New(str)
	}

	// Use the crypto public key to decrypt the account public extended
	// key.
	serializedKeyPub, err := cryptoKeyPub.Decrypt(row.pubKeyEncrypted)
	if err != nil {
		str := fmt.Sprintf("failed to decrypt public key for account %d",
			account)
		return nil, errors.New(str)
	}
	acctKeyPub, err := hdkeychain.NewKeyFromString(string(serializedKeyPub))
	if err != nil {
		str := fmt.Sprintf("failed to create extended public key for "+
			"account %d", account)
		return nil, errors.New(str)
	}

	// create the new account info with the known information.  The rest of
	// the fields are filled out below.
	acctInfo := &accountInfo{
		acctType:         account,
		acctKeyEncrypted: row.privKeyEncrypted,
		acctKeyPub:       acctKeyPub,
	}

	internalBranchPubEnc, externalBranchPubEnc, err := fetchBranchPubKeys(amBucket)
	if err != nil {
		logging.CPrint(logging.ERROR, "failed to fetch branch public key in db", logging.LogFormat{"error": err})
		return nil, err
	}

	// Use the crypto public key to decrypt the external and internal branch public extended keys.
	serializedInBrKeyPub, err := cryptoKeyPub.Decrypt(internalBranchPubEnc)
	if err != nil {
		str := fmt.Sprintf("failed to decrypt internal branch public key for account %d",
			account)
		return nil, errors.New(str)
	}
	internalBranchPub, err := hdkeychain.NewKeyFromString(string(serializedInBrKeyPub))
	if err != nil {
		str := fmt.Sprintf("failed to create extended internal branch public key for "+
			"account %d", account)
		return nil, errors.New(str)
	}

	serializedExBrKeyPub, err := cryptoKeyPub.Decrypt(externalBranchPubEnc)
	if err != nil {
		str := fmt.Sprintf("failed to decrypt external branch public key for account %d",
			account)
		return nil, errors.New(str)
	}
	externalBranchPub, err := hdkeychain.NewKeyFromString(string(serializedExBrKeyPub))
	if err != nil {
		str := fmt.Sprintf("failed to create extended external branch public key for "+
			"account %d", account)
		return nil, errors.New(str)
	}

	// get child number
	internalChildNum, externalChildNum, err := fetchChildNum(amBucket)

	branchInfo := &branchInfo{
		internalBranchPub: internalBranchPub,
		externalBranchPub: externalBranchPub,
		nextInternalIndex: internalChildNum,
		nextExternalIndex: externalChildNum,
	}

	managedAddresses := make(map[string]*ManagedAddress)
	pubkeyBucket := amBucket.Bucket(pubKeyBucket)
	if pubkeyBucket != nil {
		pks, err := fetchEncryptedPubKey(pubkeyBucket)
		if err != nil {
			return nil, err
		}
		for _, pkp := range pks {
			path := DerivationPath{
				Account: account,
				Branch:  pkp.branch,
				Index:   pkp.index,
			}
			pubkeyBytes, err := cryptoKeyPub.Decrypt(pkp.pubkeyEnc)
			if err != nil {
				return nil, err
			}
			pubkey, err := pocec.ParsePubKey(pubkeyBytes, pocec.S256())
			if err != nil {
				return nil, err
			}
			managedAddress, err := newManagedAddressWithoutPrivKey(amBucketMeta.Name(), path, pubkey, net)
			if err != nil {
				return nil, err
			}
			managedAddresses[managedAddress.address] = managedAddress
		}
	}

	return &AddrManager{
		use:                    AddrUse(account),
		unlocked:               false,
		addrs:                  managedAddresses,
		keystoreName:           amBucketMeta.Name(),
		remark:                 string(remarkBytes),
		masterKeyPub:           &masterKeyPub,
		masterKeyPriv:          &masterKeyPriv,
		cryptoKeyPub:           cryptoKeyPub,
		cryptoKeyPrivEncrypted: cryptoKeyPrivEnc,
		cryptoKeyPriv:          &cryptoKey{},
		privPassphraseSalt:     privPassphraseSalt,
		hdScope:                keyScope,
		acctInfo:               acctInfo,
		branchInfo:             branchInfo,
		storage:                amBucketMeta,
	}, nil

}

func (kmc *KeystoreManagerForPoC) NewKeystore(privPassphrase, seed []byte, remark string,
	net *config.Params, scryptConfig *ScryptOptions) (string, error) {
	kmc.mu.Lock()
	defer kmc.mu.Unlock()

	if len(kmc.managedKeystores) > 0 {
		for _, addrManager := range kmc.managedKeystores {
			err := addrManager.safelyCheckPassword(privPassphrase)
			if err != nil {
				return "", ErrDifferentPrivPass
			}
			break
		}
	}

	var acctBucketMeta db.BucketMeta
	err := db.Update(kmc.db, func(dbTransaction db.DBTransaction) error {
		var err error
		// create hd key chain and init the bucket
		acctBucketMeta, err = create(dbTransaction, kmc.ksMgrMeta, kmc.pubPassphrase, privPassphrase, seed, PoCUsage, remark, net, scryptConfig)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	var addrManager *AddrManager
	err = db.View(kmc.db, func(dbTransaction db.ReadTransaction) error {
		amBucket := dbTransaction.FetchBucket(acctBucketMeta)
		if amBucket == nil {
			return ErrUnexpecteDBError
		}

		var err error
		addrManager, err = loadAddrManager(amBucket, kmc.pubPassphrase, net)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	kmc.managedKeystores[addrManager.Name()] = addrManager

	accountID := addrManager.Name()

	if kmc.unlocked {
		err := kmc.useKeystore(accountID, privPassphrase, true)
		if err != nil {
			// should not be touched
			return "", err
		}
	}

	return accountID, nil
}

func (kmc *KeystoreManagerForPoC) allocAddrMgrNamespace(dbTransaction db.DBTransaction, oldPass, newPass []byte, pubPasphrase []byte, kStore *Keystore,
	net *config.Params, scryptConfig *ScryptOptions) (db.BucketMeta, error) {
	privParamsOld, err := hex.DecodeString(kStore.Crypto.PrivParams)
	if err != nil {
		return nil, err
	}
	var masterPrivKeyOld snacl.SecretKey
	defer masterPrivKeyOld.Zero()
	err = unmarshalMasterPrivKey(&masterPrivKeyOld, oldPass, privParamsOld)
	if err != nil {
		logging.CPrint(logging.ERROR, "unmarshalMasterPrivKey failed",
			logging.LogFormat{
				"err": err,
			})
		return nil, err
	}

	cryptoPrivKeyEncOld, err := hex.DecodeString(kStore.Crypto.CryptoKeyPrivEnc)
	if err != nil {
		return nil, err
	}
	MasterHDPrivKeyEncOld, err := hex.DecodeString(kStore.Crypto.MasterHDPrivKeyEnc)
	if err != nil {
		return nil, err
	}

	cPrivKeyBytes, err := masterPrivKeyOld.Decrypt(cryptoPrivKeyEncOld)
	if err != nil {
		return nil, err
	}
	var cPrivKeyOld cryptoKey
	cPrivKeyOld.CopyBytes(cPrivKeyBytes)
	zero.Bytes(cPrivKeyBytes)
	defer cPrivKeyOld.Zero()

	mHDKeyBytes, err := cPrivKeyOld.Decrypt(MasterHDPrivKeyEncOld)
	if err != nil {
		return nil, err
	}

	defer zero.Bytes(mHDKeyBytes)

	if scryptConfig == nil {
		scryptConfig = &DefaultScryptOptions
	}

	masterKeyPriv, err := secretKeyGen(&newPass, scryptConfig)
	if err != nil {
		return nil, err
	}
	defer masterKeyPriv.Zero()

	masterKeyPub, err := secretKeyGen(&pubPasphrase, scryptConfig)
	if err != nil {
		return nil, err
	}

	pubParams := masterKeyPub.Marshal()
	privParams := masterKeyPriv.Marshal()
	cryptoKeyPriv, err := newCryptoKey()
	if err != nil {
		return nil, err
	}
	defer cryptoKeyPriv.Zero()

	cryptoKeyPub, err := newCryptoKey()
	if err != nil {
		return nil, err
	}
	cryptoKeyScript, err := newCryptoKey()
	if err != nil {
		return nil, err
	}
	defer cryptoKeyScript.Zero()

	cryptoKeyPubEnc, err := masterKeyPub.Encrypt(cryptoKeyPub.Bytes())
	if err != nil {
		return nil, err
	}
	cryptoKeyPrivEnc, err := masterKeyPriv.Encrypt(cryptoKeyPriv.Bytes())
	if err != nil {
		return nil, err
	}

	masterHDPrivKeyEnc, err := cryptoKeyPriv.Encrypt(mHDKeyBytes)
	if err != nil {
		return nil, err
	}
	masterHDPrivKey, err := hdkeychain.NewKeyFromString(string(mHDKeyBytes))
	if err != nil {
		return nil, err
	}
	masterHDPubKey, err := masterHDPrivKey.Neuter()
	if err != nil {
		return nil, err
	}
	mHDPubKeyBytes := []byte(masterHDPubKey.String())
	masterHDPubKeyEnc, err := cryptoKeyPriv.Encrypt(mHDPubKeyBytes)
	if err != nil {
		return nil, err
	}

	kmBucket := dbTransaction.FetchBucket(kmc.ksMgrMeta)
	if kmBucket == nil {
		return nil, ErrBucketNotFound
	}

	acctBucketMeta, err := createManagerKeyScope(kmBucket, masterHDPrivKey,
		cryptoKeyPub, cryptoKeyPriv, &kStore.HDpath, net)
	if err != nil {
		return nil, err
	}

	acctBucket := dbTransaction.FetchBucket(acctBucketMeta)
	if acctBucket == nil {
		return nil, ErrUnexpecteDBError
	}

	if len(kStore.Remark) > 0 {
		err = putRemark(acctBucket, []byte(kStore.Remark))
		if err != nil {
			return nil, err
		}
	}

	err = putMasterKeyParams(acctBucket, pubParams, privParams)
	if err != nil {
		return nil, err
	}

	err = putMasterHDKeys(acctBucket, masterHDPrivKeyEnc, masterHDPubKeyEnc)
	if err != nil {
		return nil, err
	}

	// Save the encrypted crypto keys to the database.
	err = putCryptoKeys(acctBucket, cryptoKeyPubEnc, cryptoKeyPrivEnc)
	if err != nil {
		return nil, err
	}

	amBucketMeta := acctBucket.GetBucketMeta()
	return amBucketMeta, nil
}

func (kmc *KeystoreManagerForPoC) ImportKeystore(keystoreJson []byte, oldPrivPass, newPrivPass []byte) (string, string, error) {
	kmc.mu.Lock()
	defer kmc.mu.Unlock()

	if len(newPrivPass) == 0 {
		newPrivPass = oldPrivPass
	}
	if !ValidatePassphrase(newPrivPass) {
		return "", "", ErrIllegalPassphrase
	}
	if bytes.Compare(newPrivPass, kmc.pubPassphrase) == 0 {
		return "", "", ErrIllegalNewPrivPass
	}

	if len(kmc.managedKeystores) > 0 {
		for _, addrManager := range kmc.managedKeystores {
			err := addrManager.safelyCheckPassword(newPrivPass)
			if err != nil {
				return "", "", ErrDifferentPrivPass
			}
			break
		}
	}

	var amBucketMeta db.BucketMeta
	var acctManager *AddrManager
	// load keystore
	kStore, err := GetKeystoreFromJson(keystoreJson)
	if err != nil {
		return "", "", err
	}
	err = db.Update(kmc.db, func(dbTransaction db.DBTransaction) error {
		// storage update
		var err error
		amBucketMeta, err = kmc.allocAddrMgrNamespace(dbTransaction, oldPrivPass, newPrivPass, kmc.pubPassphrase, kStore, kmc.params, nil)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return "", "", err
	}

	err = db.View(kmc.db, func(dbTransaction db.ReadTransaction) error {
		// load addrManager
		var err error
		amBucket := dbTransaction.FetchBucket(amBucketMeta)
		acctManager, err = loadAddrManager(amBucket, kmc.pubPassphrase, kmc.params)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		// should not be touched
		logging.CPrint(logging.FATAL, "failed to load addrManager from db", logging.LogFormat{"error": err})
		return "", "", err
	}
	kmc.managedKeystores[acctManager.keystoreName] = acctManager

	if kmc.unlocked {
		err := kmc.useKeystore(acctManager.keystoreName, newPrivPass, true)
		if err != nil {
			// should not be touched
			delete(kmc.managedKeystores, acctManager.keystoreName)
			logging.CPrint(logging.FATAL, "failed to user keystore", logging.LogFormat{"error": err})
			return "", "", err
		}
	}

	return acctManager.keystoreName, acctManager.remark, nil
}

func (kmc *KeystoreManagerForPoC) ExportKeystore(accountID string, privPassphrase []byte) ([]byte, error) {
	kmc.mu.Lock()
	defer kmc.mu.Unlock()

	keystoreBytes := make([]byte, 0)
	addrManager, found := kmc.managedKeystores[accountID]
	if found {
		err := db.View(kmc.db, func(dbTransaction db.ReadTransaction) error {
			keystore, err := addrManager.exportKeystore(dbTransaction, privPassphrase)
			if err != nil {
				return err
			}
			keystoreBytes = keystore.Bytes()
			return nil
		})
		if err != nil {
			return nil, err
		}
		return keystoreBytes, nil
	} else {
		logging.CPrint(logging.ERROR, "account not exists",
			logging.LogFormat{
				"err": ErrAccountNotFound,
			})
		return nil, ErrAccountNotFound
	}
}

func (kmc *KeystoreManagerForPoC) DeleteKeystore(accountID string, privPassphrase []byte) (bool, error) {
	kmc.mu.Lock()
	defer kmc.mu.Unlock()

	// check private pass
	addrManager, found := kmc.managedKeystores[accountID]
	if found {
		err := addrManager.safelyCheckPassword(privPassphrase)
		if err != nil {
			logging.CPrint(logging.ERROR, "wrong passphrase",
				logging.LogFormat{
					"err": err,
				})
			return false, err
		}

		err = db.Update(kmc.db, func(dbTransaction db.DBTransaction) error {
			if addrManager.destroy(dbTransaction) != nil {
				logging.CPrint(logging.ERROR, "delete account failed",
					logging.LogFormat{
						"err": err,
					})
				return err
			}

			kmBucket := dbTransaction.FetchBucket(kmc.ksMgrMeta)
			if kmBucket == nil {
				logging.CPrint(logging.ERROR, "failed to fetch keystore manager bucket")
				return ErrBucketNotFound
			}

			err = kmBucket.DeleteBucket(addrManager.keystoreName)
			if err != nil {
				logging.CPrint(logging.ERROR, "failed to delete bucket under keystore manager", logging.LogFormat{"bucket name": addrManager.keystoreName})
				return err
			}

			accountIDBucket := dbTransaction.FetchBucket(kmc.accountIDMeta)
			err = deleteAccountID(accountIDBucket, []byte(accountID))
			if err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return false, err
		}

		addrManager.clearPrivKeys()
		delete(kmc.managedKeystores, accountID)
		return true, nil

	} else {
		logging.CPrint(logging.ERROR, "account not exists",
			logging.LogFormat{
				"err": ErrAccountNotFound,
			})
		return false, ErrAccountNotFound
	}
}

func (kmc *KeystoreManagerForPoC) useKeystore(name string, privPassphrase []byte, updatePriv bool) error {
	addrManager, found := kmc.managedKeystores[name]
	if found {
		// check privPasspharse
		if err := addrManager.checkPassword(privPassphrase); err != nil {
			if err == ErrInvalidPassphrase {
				logging.CPrint(logging.ERROR, "password wrong",
					logging.LogFormat{
						"error": err,
					})
			} else {
				logging.CPrint(logging.ERROR, "failed to derive master private key",
					logging.LogFormat{
						"error": err,
					})
			}
			return err
		}

		// hash salted passphrase
		saltPassphrase := append(addrManager.privPassphraseSalt[:], privPassphrase...)
		addrManager.hashedPrivPassphrase = sha512.Sum512(saltPassphrase)
		zero.Bytes(saltPassphrase)

		if updatePriv {
			err := addrManager.updatePrivKeys()
			if err != nil {
				logging.CPrint(logging.ERROR, "update privKeys failed",
					logging.LogFormat{
						"err": err,
					})
				return err
			}
		}

	} else {
		logging.CPrint(logging.ERROR, "account not exists",
			logging.LogFormat{
				"err": ErrAccountNotFound,
			})
		return ErrAccountNotFound
	}

	return nil
}

func (kmc *KeystoreManagerForPoC) Unlock(privPassphrase []byte) error {
	kmc.mu.Lock()
	defer kmc.mu.Unlock()

	for accountID := range kmc.managedKeystores {
		err := kmc.useKeystore(accountID, privPassphrase, true)
		if err != nil {
			logging.CPrint(logging.ERROR, "failed to use keystore",
				logging.LogFormat{
					"error": err,
				})
			return err
		}
	}
	kmc.unlocked = true
	return nil
}

func (kmc *KeystoreManagerForPoC) Lock() {
	kmc.mu.Lock()
	defer kmc.mu.Unlock()

	for _, addrManager := range kmc.managedKeystores {
		addrManager.clearPrivKeys()
	}
	kmc.unlocked = false
}

func (kmc *KeystoreManagerForPoC) IsLocked() bool {
	return !kmc.unlocked
}

func (kmc *KeystoreManagerForPoC) NextAddresses(accountID string, internal bool, numAddresses uint32) ([]*ManagedAddress, error) {
	kmc.mu.Lock()
	defer kmc.mu.Unlock()

	managedAddresses := make([]*ManagedAddress, 0)
	addrManager, found := kmc.managedKeystores[accountID]
	if found {
		err := db.Update(kmc.db, func(dbTransaction db.DBTransaction) error {
			var err error
			managedAddresses, err = addrManager.nextAddresses(dbTransaction, internal, numAddresses, kmc.params)
			if err != nil {
				logging.CPrint(logging.ERROR, "new address failed",
					logging.LogFormat{
						"err": err,
					})
				return err
			}
			return nil
		})
		if err != nil {
			return nil, err
		}

		err = db.View(kmc.db, func(dbTransaction db.ReadTransaction) error {
			return addrManager.updateManagedAddress(dbTransaction, managedAddresses)
		})
		if err != nil {
			// should not be executed
			logging.CPrint(logging.FATAL, "failed to update new address", logging.LogFormat{"error": err})
			return nil, err
		}
		return managedAddresses, nil
	} else {
		logging.CPrint(logging.ERROR, "account not exists",
			logging.LogFormat{
				"err": ErrAccountNotFound,
			})
		return nil, ErrAccountNotFound
	}
}

func (kmc *KeystoreManagerForPoC) GenerateNewPublicKey() (*pocec.PublicKey, uint32, error) {
	managedAddresses := make([]*ManagedAddress, 0)
	var accountID string
	var pubkey *pocec.PublicKey
	var index uint32
	for accountID = range kmc.managedKeystores {
		break
	}
	addrManager, found := kmc.managedKeystores[accountID]
	if found {

		err := db.Update(kmc.db, func(dbTransaction db.DBTransaction) error {
			var err error
			managedAddresses, err = addrManager.nextAddresses(dbTransaction, false, 1, kmc.params)
			if err != nil {
				logging.CPrint(logging.ERROR, "new address failed",
					logging.LogFormat{
						"err": err,
					})
				return err
			}
			return nil
		})
		if err != nil {
			return nil, 0, err
		}

		err = db.View(kmc.db, func(tx db.ReadTransaction) error {
			return addrManager.updateManagedAddress(tx, managedAddresses)
		})
		if err != nil {
			return nil, 0, err
		}

		for _, managedAddr := range managedAddresses {
			var err error
			pkbytes := managedAddr.pubKey.SerializeCompressed()
			pubkey, err = pocec.ParsePubKey(pkbytes, pocec.S256())
			if err != nil {
				return nil, 0, err
			}
			index = managedAddr.derivationPath.Index
			break
		}
		return pubkey, index, nil
	} else {
		logging.CPrint(logging.ERROR, "account not exists",
			logging.LogFormat{
				"err": ErrAccountNotFound,
			})
		return nil, 0, ErrAccountNotFound
	}
}

func (kmc *KeystoreManagerForPoC) SignMessage(pubKey *pocec.PublicKey, message []byte) (*pocec.Signature, error) {
	kmc.mu.Lock()
	defer kmc.mu.Unlock()

	if pubKey == nil {
		return nil, ErrNilPointer
	}

	mHash := wire.HashH(message)
	_, address, err := newPoCAddress(pubKey, kmc.params)
	if err != nil {
		logging.CPrint(logging.ERROR, "new witness address failed",
			logging.LogFormat{
				"err": err,
			})
		return nil, err
	}
	addr := address.EncodeAddress()
	acctM, err := kmc.getAddrManager(addr)
	if err != nil {
		return nil, err
	}
	sig, err := acctM.signPocec(mHash[:], addr)
	if err != nil {
		logging.CPrint(logging.ERROR, "sign failed",
			logging.LogFormat{
				"err": err,
			})
		return nil, err
	}
	return sig, nil
}

func (kmc *KeystoreManagerForPoC) SignHash(pubKey *pocec.PublicKey, hash []byte) (*pocec.Signature, error) {
	kmc.mu.Lock()
	defer kmc.mu.Unlock()

	if pubKey == nil {
		return nil, ErrNilPointer
	}

	_, address, err := newPoCAddress(pubKey, kmc.params)
	if err != nil {
		logging.CPrint(logging.ERROR, "new witness address failed",
			logging.LogFormat{
				"err": err,
			})
		return nil, err
	}

	addr := address.EncodeAddress()
	acctM, err := kmc.getAddrManager(addr)
	if err != nil {
		return nil, err
	}

	sig, err := acctM.signPocec(hash, addr)
	if err != nil {
		logging.CPrint(logging.ERROR, "sign failed",
			logging.LogFormat{
				"err": err,
			})
		return nil, err
	}
	return sig, nil

}

func (kmc *KeystoreManagerForPoC) VerifySig(sig *pocec.Signature, hash []byte, pubKey *pocec.PublicKey) (bool, error) {
	kmc.mu.Lock()
	defer kmc.mu.Unlock()

	if pubKey == nil {
		return false, ErrNilPointer
	}

	_, address, err := newPoCAddress(pubKey, kmc.params)
	if err != nil {
		logging.CPrint(logging.ERROR, "new witness address failed",
			logging.LogFormat{
				"err": err,
			})
		return false, err
	}

	addr := address.EncodeAddress()
	acctM, err := kmc.getAddrManager(addr)
	if err != nil {
		return false, err
	}

	boolRe, err := acctM.verifySigPocec(sig, hash, pubKey)
	if err != nil {
		logging.CPrint(logging.ERROR, "sign failed",
			logging.LogFormat{
				"err": err,
			})
		return false, err
	}
	return boolRe, nil
}

func (kmc *KeystoreManagerForPoC) getAddrManager(addr string) (*AddrManager, error) {
	for acctID, acctM := range kmc.managedKeystores {
		_, ok := acctM.addrs[addr]
		if ok {
			return kmc.managedKeystores[acctID], nil
		}
	}

	return nil, ErrAccountNotFound
}

func (kmc *KeystoreManagerForPoC) ListKeystoreNames() []string {
	kmc.mu.Lock()
	defer kmc.mu.Unlock()
	list := make([]string, 0)
	for id := range kmc.managedKeystores {
		list = append(list, id)
	}

	return list
}

func (kmc *KeystoreManagerForPoC) GetManagedAddrManager() []*AddrManager {
	kmc.mu.Lock()
	defer kmc.mu.Unlock()
	ret := make([]*AddrManager, 0)
	for _, v := range kmc.managedKeystores {
		ret = append(ret, v)
	}
	return ret
}

func (kmc *KeystoreManagerForPoC) ChainParams() *config.Params {
	kmc.mu.Lock()
	defer kmc.mu.Unlock()
	return kmc.params
}

func (kmc *KeystoreManagerForPoC) GetPublicKeyOrdinal(pubKey *pocec.PublicKey) (uint32, bool) {
	kmc.mu.Lock()
	defer kmc.mu.Unlock()

	if pubKey == nil {
		return 0, false
	}

	_, address, err := newPoCAddress(pubKey, kmc.params)
	if err != nil {
		return 0, false
	}

	addr := address.EncodeAddress()
	for _, acctM := range kmc.managedKeystores {
		mAddr, ok := acctM.addrs[addr]
		if ok {
			return mAddr.derivationPath.Index, true
		}
	}

	return 0, false
}

func (kmc *KeystoreManagerForPoC) GetAddressByPubKey(pubKey *pocec.PublicKey) (string, error) {
	kmc.mu.Lock()
	defer kmc.mu.Unlock()

	if pubKey == nil {
		return "", ErrNilPointer
	}

	_, address, err := newPoCAddress(pubKey, kmc.params)
	if err != nil {
		logging.CPrint(logging.ERROR, "new witness address failed",
			logging.LogFormat{
				"err": err,
			})
		return "", err
	}

	addr := address.EncodeAddress()
	_, err = kmc.getAddrManager(addr)
	if err != nil {
		return "", err
	}
	return addr, nil
}

func (kmc *KeystoreManagerForPoC) ChangeRemark(accountID, newRemark string) error {
	kmc.mu.Lock()
	defer kmc.mu.Unlock()

	addrManager, found := kmc.managedKeystores[accountID]
	if found {
		err := db.Update(kmc.db, func(dbTransaction db.DBTransaction) error {
			err := addrManager.changeRemark(dbTransaction, newRemark)
			if err != nil {
				logging.CPrint(logging.ERROR, "failed to change remark", logging.LogFormat{"error": err})
				return err
			}
			return nil
		})
		if err != nil {
			return err
		}
		return nil
	} else {
		logging.CPrint(logging.ERROR, "account not exists",
			logging.LogFormat{
				"err": ErrAccountNotFound,
			})
		return ErrAccountNotFound
	}

}

func (kmc *KeystoreManagerForPoC) ChangePubPassphrase(oldPubPass, newPubPass []byte, scryptConfig *ScryptOptions) error {
	kmc.mu.Lock()
	defer kmc.mu.Unlock()

	if !ValidatePassphrase(newPubPass) {
		return ErrIllegalPassphrase
	}

	if bytes.Compare(oldPubPass, newPubPass) == 0 {
		return ErrSamePubpass
	}

	// should not be the same as the private pass
	for accountID := range kmc.managedKeystores {
		err := kmc.managedKeystores[accountID].safelyCheckPassword(newPubPass)
		if err == nil {
			return ErrIllegalNewPubPass
		}
		break
	}

	if scryptConfig == nil {
		scryptConfig = &DefaultScryptOptions
	}
	// Generate new master keys.  These master keys are used to protect the
	// crypto keys that will be generated next.
	newMasterKeyPub, err := secretKeyGen(&newPubPass, scryptConfig)
	if err != nil {
		return err
	}

	err = db.Update(kmc.db, func(dbTransaction db.DBTransaction) error {
		for _, addrManager := range kmc.managedKeystores {
			amBucket := dbTransaction.FetchBucket(addrManager.storage)
			if amBucket == nil {
				return ErrUnexpecteDBError
			}
			// Load the old master pubkey params from the db.
			masterKeyPubParams, _, err := fetchMasterKeyParams(amBucket)
			if err != nil {
				return err
			}
			// Derive the master public key using the serialized params and provided
			// passphrase.
			var oldMasterKeyPub snacl.SecretKey
			if err := oldMasterKeyPub.Unmarshal(masterKeyPubParams); err != nil {
				return err
			}
			if err := oldMasterKeyPub.DeriveKey(&oldPubPass); err != nil {
				return err
			}

			// Load the crypto keys from the db.
			cryptoKeyPubEnc, _, err := fetchCryptoKeys(amBucket)
			if err != nil {
				return err
			}
			// Use the master public key to decrypt the crypto public key.
			cryptoKeyPubCT, err := oldMasterKeyPub.Decrypt(cryptoKeyPubEnc)
			if err != nil {
				return err
			}

			// Save the master key params to the database.
			newPubParams := newMasterKeyPub.Marshal()

			// Encrypt the crypto keys with the associated master keys.
			newCryptoKeyPubEnc, err := newMasterKeyPub.Encrypt(cryptoKeyPubCT)
			if err != nil {
				return err
			}

			err = putMasterKeyParams(amBucket, newPubParams, nil)
			if err != nil {
				return err
			}

			err = putCryptoKeys(amBucket, newCryptoKeyPubEnc, nil)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	for _, addrManager := range kmc.managedKeystores {
		addrManager.masterKeyPub = newMasterKeyPub
	}
	kmc.pubPassphrase = newPubPass
	return nil
}

func (kmc *KeystoreManagerForPoC) ChangePrivPassphrase(oldPrivPass, newPrivPass []byte, scryptConfig *ScryptOptions) error {
	kmc.mu.Lock()
	defer kmc.mu.Unlock()

	if !ValidatePassphrase(newPrivPass) {
		return ErrIllegalPassphrase
	}
	if bytes.Compare(newPrivPass, kmc.pubPassphrase) == 0 {
		return ErrIllegalNewPrivPass
	}
	if bytes.Equal(newPrivPass, oldPrivPass) {
		return ErrSamePrivpass
	}

	if scryptConfig == nil {
		scryptConfig = &DefaultScryptOptions
	}

	// generate new master key from the privPassphrase which is used to secure
	// the actual secret keys.
	newMasterPrivKey, err := secretKeyGen(&newPrivPass, scryptConfig)
	if err != nil {
		return err
	}

	err = db.Update(kmc.db, func(dbTransaction db.DBTransaction) error {
		for _, addrManager := range kmc.managedKeystores {
			amBucket := dbTransaction.FetchBucket(addrManager.storage)
			err := addrManager.changePrivPassphrase(amBucket, oldPrivPass, newMasterPrivKey)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	for _, addrManager := range kmc.managedKeystores {
		var passphraseSalt [saltSize]byte
		_, err = rand.Read(passphraseSalt[:])
		if err != nil {
			return err
		}
		// When the manager is locked, ensure the new clear text master
		// key is cleared from memory now that it is no longer needed.
		// If unlocked, create the new passphrase hash with the new
		// passphrase and salt.
		var hashedPassphrase [sha512.Size]byte
		if addrManager.unlocked {
			saltedPassphrase := append(passphraseSalt[:],
				newPrivPass...)
			hashedPassphrase = sha512.Sum512(saltedPassphrase)
			zero.Bytes(saltedPassphrase)
		}

		var cPrivKeyEnc []byte
		err = db.View(kmc.db, func(dbTransaction db.ReadTransaction) error {
			amBucket := dbTransaction.FetchBucket(addrManager.storage)
			var err error
			_, cPrivKeyEnc, err = fetchCryptoKeys(amBucket)
			if err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return err
		}
		copy(addrManager.cryptoKeyPrivEncrypted, cPrivKeyEnc)
		addrManager.masterKeyPriv.Zero()
		addrManager.masterKeyPriv = newMasterPrivKey
		addrManager.privPassphraseSalt = passphraseSalt
		addrManager.hashedPrivPassphrase = hashedPassphrase
	}
	return nil
}

func NewKeystoreManagerForPoC(store db.DB, pubPassphrase []byte, net *config.Params) (*KeystoreManagerForPoC, error) {
	if store == nil || net == nil || pubPassphrase == nil {
		return nil, ErrNilPointer
	}

	valid := ValidatePassphrase(pubPassphrase)
	if !valid {
		logging.CPrint(logging.ERROR, "Public pass does not match the pattern. Change your public pass.",
			logging.LogFormat{
				"pattern": "The recommended length of pass is between 6 and 40, and it is allowed consist of numbers, letters and symbols(@#$%^&)",
			})
		return nil, ErrIllegalPassphrase
	}

	managed := make(map[string]*AddrManager)
	var kmBucketMeta db.BucketMeta
	var accountIDBucketMeta db.BucketMeta
	err := db.Update(store, func(tx db.DBTransaction) error {
		kmBucket, err := db.GetOrCreateTopLevelBucket(tx, ksMgrBucket)
		if err != nil {
			return err
		}
		kmBucketMeta = kmBucket.GetBucketMeta()

		// Load the account from db
		accountIDBucket, err := db.GetOrCreateBucket(kmBucket, accountIDBucket)
		if err != nil {
			return err
		}
		accountIDBucketMeta = accountIDBucket.GetBucketMeta()

		accountIDs, err := fetchAccountID(accountIDBucket)
		if err != nil {
			return err
		}
		for _, accountIDBytes := range accountIDs {
			accountID := string(accountIDBytes)
			amBucket := kmBucket.Bucket(accountID)

			addrManager, err := loadAddrManager(amBucket, pubPassphrase, net)
			if err != nil {
				return err
			}
			managed[accountID] = addrManager
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &KeystoreManagerForPoC{
		managedKeystores: managed,
		params:           net,
		ksMgrMeta:        kmBucketMeta,
		accountIDMeta:    accountIDBucketMeta,
		pubPassphrase:    pubPassphrase,
		db:               store,
	}, nil
}
