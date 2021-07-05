package keystore

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/massnetorg/mass-core/errors"
	"github.com/massnetorg/mass-core/pocec"
	"github.com/massnetorg/mass-core/wire"
	"massnet.org/mass/config"
	mwdb "massnet.org/mass/poc/wallet/db"
	"massnet.org/mass/poc/wallet/keystore/hdkeychain"
	"massnet.org/mass/poc/wallet/keystore/snacl"
	"massnet.org/mass/poc/wallet/keystore/zero"
)

const (
	testDbRoot = "testDbs"
)

var (
	// seed is the master seed used throughout the tests.
	seed = []byte{
		0x2a, 0x64, 0xdf, 0x08, 0x5e, 0xef, 0xed, 0xd8, 0xbf,
		0xdb, 0xb3, 0x31, 0x76, 0xb5, 0xba, 0x2e, 0x62, 0xe8,
		0xbe, 0x8b, 0x56, 0xc8, 0x83, 0x77, 0x95, 0x59, 0x8b,
		0xb6, 0xc4, 0x40, 0xc0, 0x64,
	}
	seed2 = []byte{
		0x2a, 0x64, 0xdf, 0x08, 0x5e, 0xef, 0xed, 0xd8, 0xbf,
		0xdb, 0xb3, 0x31, 0x76, 0xb5, 0xba, 0x2e, 0x62, 0xe8,
		0xbe, 0x8b, 0x56, 0xc8, 0x83, 0x77, 0x95, 0x59, 0x8b,
		0xb6, 0xc5, 0x44, 0xc0, 0x64,
	}

	illegalSeed = []byte{
		0x2a, 0x64, 0xdf, 0x08, 0x5e, 0xef, 0xed, 0xd8, 0xbf,
		0xdb, 0xb3, 0x31, 0x76, 0xb5, 0xba, 0x2e, 0x62, 0xe8,
		0xbe, 0x8b, 0x56, 0xc8, 0x83, 0x77, 0x95, 0x59, 0x8b,
	}

	pubPassphrase   = []byte("@DJr@fL4H0O#$%0^n@V1")
	privPassphrase  = []byte("@#XXd7O9xyDIWIbXX$lj")
	pubPassphrase2  = []byte("@0NV4P@VSJBWbunw#%ZI")
	privPassphrase2 = []byte("@#$#08%68^f&5#4@%$&Y")

	pubPassphraseTooShort = []byte("123")
	illegalPrivPassphrase = []byte("qa3er2nv!a")

	samplePublicKey = "03da7e0c5c6ca123447d6106eabbe0db9bcd4ea1753b19b37e14113e5f709a7876"

	msg1 = []byte("mass")
	msg2 = []byte("abcd")

	// fastScrypt are parameters used throughout the tests to speed up the
	// scrypt operations.
	fastScrypt = &ScryptOptions{
		N: 16,
		R: 8,
		P: 1,
	}

	exportFilePath       = "./testdata/keystore"
	exportFileNamePerfix = "keystore"

	masterHDPrivStr    = "d63da8b12621813343136a5fc7ac96df41a7ae9695113fdfdcdfd80ba9bdf55c"
	masterChainCodeStr = "549d7cd80207a6ae4d3a1bbf817ea5a0d91bce7df34a7d7586d68dd2106d36fc"
)

func TestKeystoreManagerForPoC_NewKeystore(t *testing.T) {
	ldb, tearDown, err := GetDb("Tst_Manager")
	if err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	defer tearDown()

	// new poc keystore manager
	kmc, err := NewKeystoreManagerForPoC(ldb, pubPassphrase, config.ChainParams)
	if err != nil {
		t.Fatalf("failed to new keystore manager, %v", err)
	}

	// new keystore
	// error: invalid private pass
	_, err = kmc.NewKeystore(illegalPrivPassphrase, seed, "error-test1", config.ChainParams, fastScrypt)
	if err != ErrIllegalPassphrase {
		t.Fatalf("failed to catch error, expected: illegal passphrase, actual: %v", err)
	}
	t.Logf("pass test 1")

	// error: same as public pass
	_, err = kmc.NewKeystore(pubPassphrase, seed, "error-test2", config.ChainParams, fastScrypt)
	if err != ErrIllegalNewPrivPass {
		t.Fatalf("failed to catch error, expected: new private passphrase same as public passphrase, actual: %v", err)
	}
	t.Logf("pass test 2")

	// error: illegal seed
	_, err = kmc.NewKeystore(privPassphrase, illegalSeed, "error-test3", config.ChainParams, fastScrypt)
	if err != ErrIllegalSeed {
		t.Fatalf("failed to catch error, expected: illegal seed, actual: %v", err)
	}
	t.Logf("pass test 3")
}

func TestKeystoreManagerForPoC_NewKeystore_NextAddress(t *testing.T) {
	ldb, tearDown, err := GetDb("Tst_Manager")
	if err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	defer tearDown()

	t.Log("/*pocKeystoreManager*/")
	// new poc keystore manager
	kmc, err := NewKeystoreManagerForPoC(ldb, pubPassphrase, config.ChainParams)
	if err != nil {
		t.Fatalf("failed to new keystore manager, %v", err)
	}
	//new keystore
	acctID1, nerr := kmc.NewKeystore(privPassphrase, seed, "first", config.ChainParams, fastScrypt)
	if nerr != nil {
		t.Fatalf("failed to new keystore, %v", nerr)
	}
	acctID2, nerr := kmc.NewKeystore(privPassphrase2, seed2, "second", config.ChainParams, fastScrypt)
	if nerr != ErrDifferentPrivPass {
		t.Fatalf("failed to check private pass, %v", nerr)
	}
	acctID2, nerr = kmc.NewKeystore(privPassphrase, seed2, "second", config.ChainParams, fastScrypt)
	if nerr != nil {
		t.Fatalf("failed to new keystore, %v", nerr)
	}
	_, nerr = kmc.NewKeystore(privPassphrase, nil, "third", config.ChainParams, fastScrypt)
	if nerr != nil {
		t.Fatalf("failed to new keystore, %v", nerr)
	}

	uerr := kmc.Unlock(privPassphrase)
	if uerr != nil {
		t.Fatalf("failed to unlock keystore manager, %v", uerr)
	}
	acctIDs := make([]string, 0, 2)
	acctIDs = append(acctIDs, acctID1)
	acctIDs = append(acctIDs, acctID2)
	for _, acctId := range acctIDs {
		acctM, ok := kmc.managedKeystores[acctId]
		if !ok {
			t.Fatalf("can't find accountManager in managedKeystores")
		}
		t.Log()
		t.Log("acctId: ", acctId)
		t.Log("acctManagerName: ", acctM.keystoreName)
		t.Log("acctType: ", acctM.acctInfo.acctType)
		t.Log("acctPub: ", acctM.acctInfo.acctKeyPub)
		t.Log("acctPriv: ", acctM.acctInfo.acctKeyPriv)
		t.Log("externalIndex: ", acctM.branchInfo.nextExternalIndex)
		t.Log("internalIndex: ", acctM.branchInfo.nextInternalIndex)

		externalAddresses, err := kmc.NextAddresses(acctId, false, 2)
		if err != nil {
			t.Fatalf("failed to new external address, %v", err)
			return
		}

		internalAddresses, err := kmc.NextAddresses(acctId, true, 4)
		if err != nil {
			t.Fatalf("failed to new internal address, %v", err)

		}

		t.Log("exAddrNum:", acctM.branchInfo.nextExternalIndex)
		for i, addr := range externalAddresses {
			t.Logf("new external address %v:", i)
			managedAddressDetails(t, addr)
			//fmt.Printf("new external address %v: %v\n", i, acctM.addrs[addr.address].address)
			//fmt.Printf("derivation path: %v\n", acctM.addrs[addr.address].derivationPath)
			//fmt.Printf("private key: %v\n", acctM.addrs[addr.address].privKey)
		}
		t.Log("inAddrNum:", acctM.branchInfo.nextInternalIndex)
		for i, addr := range internalAddresses {
			t.Logf("new internal address %v:", i)
			managedAddressDetails(t, addr)
			//fmt.Printf("new internal address %v: %v\n", i, acctM.addrs[addr.address].address)
			//fmt.Printf("derivation path: %v\n", acctM.addrs[addr.address].derivationPath)
			//fmt.Printf("private key: %v\n", acctM.addrs[addr.address].privKey)
		}

		t.Log("after NewAddress, externalIndex: ", acctM.branchInfo.nextExternalIndex)
		t.Log("after NewAddress, internalIndex: ", acctM.branchInfo.nextInternalIndex)
	}

	return
}

func TestNewKeystoreManagerForPoC(t *testing.T) {
	ldb, tearDown, err := GetDb("Tst_Manager")
	if err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	defer tearDown()

	t.Log("/*KeystoreManagerForPoC*/")

	// new keystore manager
	// error: nil pointer
	_, err = NewKeystoreManagerForPoC(ldb, nil, config.ChainParams)
	if err != ErrNilPointer {
		t.Fatalf("failed to catch error, expected: the pointer is nil, actual: %v", err)
	}
	t.Logf("pass test 1")

	// error: illegal pass
	_, err = NewKeystoreManagerForPoC(ldb, pubPassphraseTooShort, config.ChainParams)
	if err != ErrIllegalPassphrase {
		t.Fatalf("failed to catch error, expected: illegal passphrase, actual: %v", err)
	}
	t.Logf("pass test 2")

	// new poc keystore manager
	kmc, err := NewKeystoreManagerForPoC(ldb, pubPassphrase, config.ChainParams)
	if err != nil {
		t.Fatalf("failed to new keystore manager, %v", err)
	}

	//new keystore
	_, err = kmc.NewKeystore(privPassphrase, seed, "first", config.ChainParams, fastScrypt)
	if err != nil {
		t.Fatalf("failed to new keystore, %v", err)
	}
	_, err = kmc.NewKeystore(privPassphrase, seed2, "second", config.ChainParams, fastScrypt)
	if err != nil {
		t.Fatalf("failed to new keystore, %v", err)
	}

	err = kmc.Unlock(privPassphrase)
	if err != nil {
		t.Fatalf("failed to unlock keystore manager, %v", err)
	}

	for i := 0; i < 10; i++ {
		_, _, err := kmc.GenerateNewPublicKey()
		if err != nil {
			t.Fatalf("failed to generate new public key, %v", err)
		}
	}

	for _, addrManager := range kmc.managedKeystores {
		showAddrManagerDetails(t, addrManager)
	}

	newKmc, err := NewKeystoreManagerForPoC(ldb, pubPassphrase, config.ChainParams)
	if err != nil {
		t.Fatalf("failed to new keystore manager, %v", err)
	}

	err = newKmc.Unlock(privPassphrase)
	if err != nil {
		t.Fatalf("failed to unlock keystore manager, %v", err)
	}

	for _, addrManager := range newKmc.managedKeystores {
		showAddrManagerDetails(t, addrManager)
	}
}

func TestKeystoreManagerForPoC_Unlock(t *testing.T) {
	ldb, tearDown, err := GetDb("Tst_Manager")
	if err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	defer tearDown()
	t.Log("/*pocKeystoreManager*/")

	// new poc keystore manager
	kmc, err := NewKeystoreManagerForPoC(ldb, pubPassphrase, config.ChainParams)
	if err != nil {
		t.Fatalf("failed to new keystore manager, %v", err)
	}
	//new keystore
	acctId, nerr := kmc.NewKeystore(privPassphrase, seed, "first", config.ChainParams, fastScrypt)
	if nerr != nil {
		t.Fatalf("failed to new keystore, %v", nerr)
	}

	uerr := kmc.Unlock(privPassphrase2)
	if uerr != ErrInvalidPassphrase {
		t.Fatalf("unexpected error %v", err)
	} else {
		t.Log("pass the unlock check")
	}

	uerr = kmc.Unlock(privPassphrase)
	if uerr != nil {
		t.Fatalf("failed to unlock keystore manager, %v", uerr)
	} else {
		t.Log("unlock keystore manager")
	}

	addrManager, ok := kmc.managedKeystores[acctId]
	if !ok {
		t.Fatalf("can't find addrManager, %v", ErrUnexpecteDBError)
	}
	//s := addrManager.ListAddresses()
	//t.Log("s:", s)
	//if s == nil {
	//	t.Log("s is nill")
	//}
	//if len(s) == 0 {
	//	t.Log("len(s)==0")
	//}

	externalAddresses, err := kmc.NextAddresses(acctId, false, 2)
	if err != nil {
		t.Fatalf("failed to new external address, %v", err)
		return
	}

	t.Log("exAddrNum:", addrManager.branchInfo.nextExternalIndex)
	for i, addr := range externalAddresses {
		t.Logf("new external address %v:", i)
		managedAddressDetails(t, addr)
		//fmt.Printf("new external address %v: %v\n", i, acctM.addrs[addr.address].address)
		//fmt.Printf("derivation path: %v\n", acctM.addrs[addr.address].derivationPath)
		//fmt.Printf("private key: %v\n", acctM.addrs[addr.address].privKey)
	}

	kmc.Lock()
	t.Log("lock keystore manager")

	internalAddresses, err := kmc.NextAddresses(acctId, true, 2)
	if err != nil {
		t.Fatalf("failed to new internal address, %v", err)
		return
	}

	t.Log("inAddrNum:", addrManager.branchInfo.nextInternalIndex)
	for i, addr := range internalAddresses {
		t.Logf("new internal address %v:", i)
		managedAddressDetails(t, addr)
		//fmt.Printf("new internal address %v: %v\n", i, acctM.addrs[addr.address].address)
		//fmt.Printf("derivation path: %v\n", acctM.addrs[addr.address].derivationPath)
		//fmt.Printf("private key: %v\n", acctM.addrs[addr.address].privKey)
	}

	uerr = kmc.Unlock(privPassphrase)
	if uerr != nil {
		t.Fatalf("failed to unlock keystore manager, %v", uerr)
	} else {
		t.Log("second unlock keystore manager")
	}

	internalAddresses, err = kmc.NextAddresses(acctId, true, 2)
	if err != nil {
		t.Fatalf("failed to new internal address, %v", err)
		return
	}

	t.Log("inAddrNum:", addrManager.branchInfo.nextInternalIndex)
	for i, addr := range internalAddresses {
		t.Logf("new internal address %v:", i)
		managedAddressDetails(t, addr)
		//fmt.Printf("new internal address %v: %v\n", i, acctM.addrs[addr.address].address)
		//fmt.Printf("derivation path: %v\n", acctM.addrs[addr.address].derivationPath)
		//fmt.Printf("private key: %v\n", acctM.addrs[addr.address].privKey)
	}
}

func managedAddressDetails(t *testing.T, addr *ManagedAddress) {
	t.Logf("keystoreName: %v", addr.keystoreName)
	t.Logf("address: %v", addr.address)
	t.Logf("derivation path: %v", addr.derivationPath)
	t.Logf("public key: %v", hex.EncodeToString(addr.pubKey.SerializeCompressed()))
	if addr.privKey != nil {
		t.Logf("private key: %v", hex.EncodeToString(addr.privKey.Serialize()))
	} else {
		t.Logf("private key: %v", addr.privKey)
	}
	t.Logf("scriptHash: %v", addr.scriptHash)
	t.Log()
}

func TestKeystoreManagerForPoC_DeleteKeystore(t *testing.T) {
	ldb, tearDown, err := GetDb("Tst_Manager")
	if err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	defer tearDown()
	t.Log("/*pocKeystoreManager*/")

	// new poc keystore manager
	kmc, err := NewKeystoreManagerForPoC(ldb, pubPassphrase, config.ChainParams)
	if err != nil {
		t.Fatalf("failed to new keystore manager, %v", err)
	}
	//new keystore
	accountID1, err := kmc.NewKeystore(privPassphrase, seed, "test0", config.ChainParams, fastScrypt)
	if err != nil {
		t.Fatalf("failed to new keystore, %v", err)
	}
	_, err = kmc.NewKeystore(privPassphrase, seed2, "test1", config.ChainParams, fastScrypt)
	if err != nil {
		t.Fatalf("failed to new keystore, %v", err)
	}

	t.Log("before delete")
	for index, mgr := range kmc.GetManagedAddrManager() {
		t.Logf("walletId_%v: %v", index, mgr.Name())
	}

	_, err = kmc.DeleteKeystore(accountID1, privPassphrase2)
	if err != ErrInvalidPassphrase {
		t.Fatalf("DeleteKeystore: mismatched error -- got: %v, want: %v",
			err, ErrInvalidPassphrase)
	}

	success, err := kmc.DeleteKeystore(accountID1, privPassphrase)
	if err != nil {
		t.Fatalf("failed to DeleteKeystore, %v", err)
	}
	t.Logf("delete keystore %v: %v", accountID1, success)

	_, err = kmc.DeleteKeystore(accountID1, privPassphrase)
	if err != ErrAccountNotFound {
		t.Fatalf("DeleteKeystore: mismatched error -- got: %v, want: %v",
			err, ErrAccountNotFound)
	}

	t.Log("after delete")
	for index, mgr := range kmc.GetManagedAddrManager() {
		t.Logf("walletId_%v: %v", index, mgr.Name())
	}
}

func TestKeystoreManagerForPoC_ExportKeystore_DeleteKeystore_ImportKeystore(t *testing.T) {
	//db export test
	ldb, tearDown, err := GetDb("Tst_Manager")
	if err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	defer tearDown()
	// new poc keystore manager
	t.Log("/*db*/")
	kmc, err := NewKeystoreManagerForPoC(ldb, pubPassphrase, config.ChainParams)
	if err != nil {
		t.Fatalf("failed to new keystore manager, %v", err)
	}
	// new keystore
	acctID0, cerr := kmc.NewKeystore(privPassphrase, seed, "first", config.ChainParams, fastScrypt)
	if cerr != nil {
		t.Fatalf("failed to new keystore, %v", cerr)
	}
	_, cerr = kmc.NewKeystore(privPassphrase, seed2, "second", config.ChainParams, fastScrypt)
	if cerr != nil {
		t.Fatalf("failed to new keystore, %v", cerr)
	}

	t.Log("before import-km.managedKeystores:", kmc.managedKeystores)
	for k := range kmc.managedKeystores {
		t.Log("k:", k)
	}

	managedAddresses, err := kmc.NextAddresses(acctID0, false, 2)
	for i, addr := range managedAddresses {
		t.Logf("new address %v: %v\n", i, addr.address)
		t.Logf("derivation path: %v\n", addr.derivationPath)
		t.Logf("private key: %v\n", addr.privKey)
		t.Log()
	}
	t.Logf("external address: %v, in: %v\n", kmc.managedKeystores[acctID0].branchInfo.nextExternalIndex, kmc.managedKeystores[acctID0].branchInfo.nextInternalIndex)

	internalManagedAddresses, err := kmc.NextAddresses(acctID0, true, 2)
	for i, addr := range internalManagedAddresses {
		t.Logf("new address %v: %v\n", i, addr.address)
		t.Logf("derivation path: %v\n", addr.derivationPath)
		t.Logf("private key: %v\n", addr.privKey)
		t.Log()
	}
	t.Logf("internal address: %v, in: %v\n", kmc.managedKeystores[acctID0].branchInfo.nextExternalIndex, kmc.managedKeystores[acctID0].branchInfo.nextInternalIndex)

	//for right example
	keystoreBytes, err := kmc.ExportKeystore(acctID0, privPassphrase)
	if err != nil {
		t.Fatalf("exportKeystore failed: %v", err)
	}
	//t.Log("keystoreBytes:", keystoreBytes)
	t.Log("ketstore string: ", string(keystoreBytes))

	res, err := kmc.DeleteKeystore(acctID0, privPassphrase)
	if err != nil {
		t.Fatalf("failed to delete keystore, %v", err)
	}
	if !res {
		t.Fatalf("failed to delete keystore")
	}
	t.Logf("successfully delete the keystore, %v", acctID0)
	// do not change privpass
	_, _, err = kmc.ImportKeystore(keystoreBytes, privPassphrase, privPassphrase)
	if err != nil {
		t.Fatalf("failed to import keystore, %v", err)
	}
	t.Logf("successfully import the keystore, %v", acctID0)
	accountIDs := kmc.ListKeystoreNames()
	for _, accountID := range accountIDs {
		for _, managedAddr := range kmc.managedKeystores[accountID].addrs {
			managedAddressDetails(t, managedAddr)
		}
	}
}

func TestKeystoreManagerForPoC_ChangePrivPassphrase(t *testing.T) {
	ldb, tearDown, err := GetDb("Tst_Manager")
	if err != nil {
		t.Fatalf("init db failed: %v", err)

	}
	t.Log("get db")
	defer tearDown()

	// new keystore manager
	kmc, err := NewKeystoreManagerForPoC(ldb, pubPassphrase, config.ChainParams)
	if err != nil {
		t.Fatalf("failed to new keystore manager, %v", err)
	}

	// new keystore
	accountID0, cerr := kmc.NewKeystore(privPassphrase, seed, "first", config.ChainParams, fastScrypt)
	if cerr != nil {
		t.Fatalf("failed to new keystore, %v", cerr)
	}

	err = kmc.Unlock(privPassphrase)
	if err != nil {
		t.Fatalf("failed to unlock keystore manager for poc, %v", err)
	}

	internalAddresses, err := kmc.NextAddresses(accountID0, false, 2)
	if err != nil {
		t.Fatalf("failed to new internal addresses, %v", err)
	}
	for _, addr := range internalAddresses {
		t.Logf("new internal address, %s\n", addr.address)
	}

	t.Log("before change, master privkey, ", kmc.managedKeystores[accountID0].masterKeyPriv.Key)

	// invalid new private pass
	err = kmc.ChangePrivPassphrase(privPassphrase, illegalPrivPassphrase, fastScrypt)
	if err != ErrIllegalPassphrase {
		t.Fatalf("failed to catch illegal pass, %v", err)
	}
	t.Logf("catch illegal pass")

	// new private pass same as public pass
	err = kmc.ChangePrivPassphrase(privPassphrase, pubPassphrase, fastScrypt)
	if err != ErrIllegalNewPrivPass {
		t.Fatalf("failed to catch error, new private pass same as public pass, err: %v", err)
	}
	t.Logf("catch illegal private pass same as public pass")

	// new private pass same as the original one
	err = kmc.ChangePrivPassphrase(privPassphrase, privPassphrase, fastScrypt)
	if err != ErrSamePrivpass {
		t.Fatalf("failed to catch same pass, %v", err)
	}
	t.Logf("catch same pass")

	err = kmc.ChangePrivPassphrase(privPassphrase, privPassphrase2, fastScrypt)
	if err != nil {
		t.Fatalf("failed to change passphrase, %v", err)
	}

	t.Log("after change, master privkey, ", kmc.managedKeystores[accountID0].masterKeyPriv.Key)

	externalAddresses, err := kmc.NextAddresses(accountID0, false, 2)
	if err != nil {
		t.Fatalf("failed to new external addresses, %v", err)
	}
	for _, addr := range externalAddresses {
		managedAddressDetails(t, addr)
		//t.Logf("new external address, %s\n", addr.address)
	}

	t.Log("after change, master privkey, ", kmc.managedKeystores[accountID0].masterKeyPriv)

	err = kmc.ChangePrivPassphrase(privPassphrase2, privPassphrase, nil)
	if err != nil {
		t.Fatalf("failed to change passphrase, %v", err)
	}
	a, ok := kmc.managedKeystores[accountID0]
	if !ok {
		t.Fatalf("failed to GetAddrManagerByAccountID, %v", err)
	}
	fmt.Println("a.masterKeyPriv:", a.masterKeyPriv.Key)
	err = kmc.Unlock(privPassphrase)
	if err != nil {
		t.Fatalf("failed to Unlock, %v", err)
	}
	a, ok = kmc.managedKeystores[accountID0]
	if !ok {
		t.Fatalf("failed to GetAddrManagerByAccountID, %v", err)
	}
	fmt.Println("after unlock—a.masterKeyPriv:", a.masterKeyPriv.Key)

}

func TestKeystoreManagerForPoC_ChangePubPassphrase(t *testing.T) {
	ldb, tearDown, err := GetDb("Tst_Manager")
	if err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	t.Log("get db")
	defer tearDown()

	// new keystore manager
	kmc, err := NewKeystoreManagerForPoC(ldb, pubPassphrase, config.ChainParams)
	if err != nil {
		t.Fatalf("failed to new keystore manager, %v", err)
	}

	// new keystore
	accountID0, cerr := kmc.NewKeystore(privPassphrase, seed, "first", config.ChainParams, fastScrypt)
	if cerr != nil {
		t.Fatalf("failed to new keystore, %v", cerr)
	}

	t.Logf("before change, master public pass, %v", kmc.managedKeystores[accountID0].masterKeyPub)

	err = kmc.ChangePubPassphrase(pubPassphrase, pubPassphrase2, nil)
	if err != nil {
		t.Fatalf("failed to change public passphrase, %v", err)
	}

	t.Logf("after change, master public pass, %v", kmc.managedKeystores[accountID0].masterKeyPub)

	keystoreJson, err := kmc.ExportKeystore(accountID0, privPassphrase)
	if err != nil {
		t.Fatalf("failed to export keystore, %v", err)
	}

	ldb2, tearDown2, err := GetDb("Tst_Manager_2")
	if err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	t.Log("get db")
	defer tearDown2()

	// new keystore manager
	kmc2, err := NewKeystoreManagerForPoC(ldb2, pubPassphrase, config.ChainParams)
	if err != nil {
		t.Fatalf("failed to new keystore manager, %v", err)
	}

	_, _, err = kmc2.ImportKeystore(keystoreJson, privPassphrase, privPassphrase)
	if err != nil {
		t.Fatalf("failed to import keystore, %v", err)
	}

}

func TestDeriveKey(t *testing.T) {
	ldb, tearDown, err := GetDb("Tst_Manager")
	if err != nil {
		t.Fatalf("init db failed: %v", err)

	}
	t.Log("get db")
	defer tearDown()

	// new keystore manager
	kmc, err := NewKeystoreManagerForPoC(ldb, pubPassphrase, config.ChainParams)
	if err != nil {
		t.Fatalf("failed to new keystore manager, %v", err)
	}

	// new keystore
	accountID0, cerr := kmc.NewKeystore(privPassphrase, seed, "first", config.ChainParams, fastScrypt)
	if cerr != nil {
		t.Fatalf("failed to new keystore, %v", cerr)
	}

	addrManager0 := kmc.managedKeystores[accountID0]
	err = mwdb.View(kmc.db, func(dbTransaction mwdb.ReadTransaction) error {
		amBucket := dbTransaction.FetchBucket(addrManager0.storage)
		// Load the master key params from the db.
		masterKeyPubParams, _, err := fetchMasterKeyParams(amBucket)
		if err != nil {
			return err
		}

		// Load the crypto keys from the db.
		cryptoKeyPubEnc, _, err := fetchCryptoKeys(amBucket)
		if err != nil {
			return err
		}

		// Derive the master public key using the serialized params and provided
		// passphrase.
		var masterKeyPub snacl.SecretKey
		if err := masterKeyPub.Unmarshal(masterKeyPubParams); err != nil {
			str := "failed to unmarshal master public key"
			return errors.New(str)
		}
		// no derive here
		// Use the master public key to decrypt the crypto public key.
		cryptoKeyPub := &cryptoKey{snacl.CryptoKey{}}
		cryptoKeyPubCT, err := masterKeyPub.Decrypt(cryptoKeyPubEnc)
		if err != nil {
			//str := "failed to decrypt crypto public key"
			return err
		}
		cryptoKeyPub.CopyBytes(cryptoKeyPubCT)
		zero.Bytes(cryptoKeyPubCT)
		return nil
	})
	if err == snacl.ErrDecryptFailed {
		t.Logf("first test success")
	} else {
		t.Fatalf("test failed, %v", err)
	}

	err = kmc.Unlock(privPassphrase)
	if err != nil {
		t.Fatalf("failed to unlock keystore manager, %v", err)
	}

	kmc.Lock()

	// try to decrypt cryptoKeyPrivEnc
	err = mwdb.View(kmc.db, func(dbTransaction mwdb.ReadTransaction) error {
		amBucket := dbTransaction.FetchBucket(addrManager0.storage)
		// Load the crypto keys from the db.
		_, cryptoKeyPrivEnc, err := fetchCryptoKeys(amBucket)
		if err != nil {
			return err
		}

		// no derive here
		// Use the master private key to decrypt the crypto private key.
		cryptoKeyPriv := &cryptoKey{snacl.CryptoKey{}}
		cryptoKeyPrivCT, err := addrManager0.masterKeyPriv.Decrypt(cryptoKeyPrivEnc)
		if err != nil {
			//str := "failed to decrypt crypto public key"
			return err
		}
		cryptoKeyPriv.CopyBytes(cryptoKeyPrivCT)
		zero.Bytes(cryptoKeyPrivCT)
		return nil
	})
	if err == snacl.ErrDecryptFailed {
		t.Logf("second test success")
	} else if err == nil {
		t.Fatalf("failed to catch the error without derive")
	} else {
		t.Fatalf("failed to catch the error without derive, find other error, %v", err)
	}
}

func TestKeystoreManagerForPoC_ChangeRemark(t *testing.T) {
	ldb, tearDown, err := GetDb("Tst_Manager")
	if err != nil {
		t.Fatalf("init db failed: %v", err)

	}
	defer tearDown()

	t.Log("/*pocKeystoreManager*/")
	// new poc keystore manager
	kmc, err := NewKeystoreManagerForPoC(ldb, pubPassphrase, config.ChainParams)
	if err != nil {
		t.Fatalf("failed to new keystore manager, %v", err)
	}
	//new keystore
	acctID1, err := kmc.NewKeystore(privPassphrase, seed, "", config.ChainParams, fastScrypt)
	if err != nil {
		t.Fatalf("failed to new keystore, %v", err)
	}
	acctID2, err := kmc.NewKeystore(privPassphrase, seed2, "second", config.ChainParams, fastScrypt)
	if err != nil {
		t.Fatalf("failed to new keystore, %v", err)
	}

	acctIDs := make([]string, 0, 2)
	acctIDs = append(acctIDs, acctID1)
	acctIDs = append(acctIDs, acctID2)

	for _, acctId := range acctIDs {
		acctM, ok := kmc.managedKeystores[acctId]
		if !ok {
			t.Fatalf("can't find accountManager in managedKeystores")
		}
		t.Log()
		t.Log("acctRemark: ", acctM.remark)
		t.Log("acctManagerName: ", acctM.keystoreName)
		t.Log("acctType: ", acctM.acctInfo.acctType)
		t.Log("acctPub: ", acctM.acctInfo.acctKeyPub)
		t.Log("acctPriv: ", acctM.acctInfo.acctKeyPriv)
		t.Log("externalIndex: ", acctM.branchInfo.nextExternalIndex)
		t.Log("internalIndex: ", acctM.branchInfo.nextInternalIndex)
	}

	err = kmc.ChangeRemark("wrongAccountID", "test")
	if err != ErrAccountNotFound {
		t.Fatalf("failed to catch error, wrong account id, %v", err)
	}
	t.Logf("catch wrong account id")

	err = kmc.ChangeRemark(acctID1, "first")
	if err != nil {
		t.Fatalf("failed to change remark for addrManager %v", acctID1)
	}

	err = kmc.ChangeRemark(acctID2, "new second")
	if err != nil {
		t.Fatalf("failed to change remark for addrManager %v", acctID2)
	}

	for _, acctId := range acctIDs {
		acctM, ok := kmc.managedKeystores[acctId]
		if !ok {
			t.Fatalf("can't find accountManager in managedKeystores")
		}
		t.Log()
		t.Log("acctRemark: ", acctM.remark)
		t.Log("acctManagerName: ", acctM.keystoreName)
		t.Log("acctType: ", acctM.acctInfo.acctType)
		t.Log("acctPub: ", acctM.acctInfo.acctKeyPub)
		t.Log("acctPriv: ", acctM.acctInfo.acctKeyPriv)
		t.Log("externalIndex: ", acctM.branchInfo.nextExternalIndex)
		t.Log("internalIndex: ", acctM.branchInfo.nextInternalIndex)
	}

	err = kmc.ChangeRemark(acctID1, "")
	if err != nil {
		t.Fatalf("failed to change remark for addrManager %v", acctID1)
	}

	acctM, ok := kmc.managedKeystores[acctID1]
	if !ok {
		t.Fatalf("can't find accountManager in managedKeystores")
	}
	t.Log()
	t.Log("acctRemark: ", acctM.remark)
	t.Log("acctManagerName: ", acctM.keystoreName)
	t.Log("acctType: ", acctM.acctInfo.acctType)
	t.Log("acctPub: ", acctM.acctInfo.acctKeyPub)
	t.Log("acctPriv: ", acctM.acctInfo.acctKeyPriv)
	t.Log("externalIndex: ", acctM.branchInfo.nextExternalIndex)
	t.Log("internalIndex: ", acctM.branchInfo.nextInternalIndex)
}

func TestBaseManager_GetManagedAddressByScriptHash(t *testing.T) {
	ldb, tearDown, err := GetDb("Tst_Manager")
	if err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	defer tearDown()
	t.Log("/*pocKeystoreManager*/")

	// new poc keystore manager
	kmc, err := NewKeystoreManagerForPoC(ldb, pubPassphrase, config.ChainParams)
	if err != nil {
		t.Fatalf("failed to new keystore manager, %v", err)
	}
	//new keystore
	acctID1, nerr := kmc.NewKeystore(privPassphrase, seed, "first", config.ChainParams, fastScrypt)
	if nerr != nil {
		t.Fatalf("failed to new keystore, %v", nerr)
	}
	//acctM, ok := kmc.managedKeystores[acctId]
	//if !ok {
	//	t.Fatalf("can't find accountManager in managedKeystores")
	//}
	t.Log()
	t.Log("acctId: ", acctID1)

	_, err = kmc.NextAddresses(acctID1, false, 1)
	if err != nil {
		t.Fatalf("failed to new external address, %v", err)
	}

	_, err = kmc.NextAddresses(acctID1, true, 2)
	if err != nil {
		t.Fatalf("failed to new internal address, %v", err)
	}

	//t.Log("exAddrNum:", acctM.branchInfo.nextExternalIndex)
	//t.Log("inAddrNum:", acctM.branchInfo.nextInternalIndex)

}

func TestKeystoreManagerForPoC_SignMessage(t *testing.T) {
	ldb, tearDown, err := GetDb("Tst_Manager")
	if err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	defer tearDown()

	// new poc keystore manager
	kmc, err := NewKeystoreManagerForPoC(ldb, pubPassphrase, config.ChainParams)
	if err != nil {
		t.Fatalf("failed to new keystore manager, %v", err)
	}
	//new keystore
	_, err = kmc.NewKeystore(privPassphrase, seed, "first", config.ChainParams, fastScrypt)
	if err != nil {
		t.Fatalf("failed to new keystore, %v", err)
	}

	err = kmc.Unlock(privPassphrase)
	if err != nil {
		t.Fatalf("failed to unlock, %v", err)
	}

	pk1, _, err := kmc.GenerateNewPublicKey()
	if err != nil {
		t.Fatalf("failed to generate new public key, %v", err)
	}

	// error: nil pointer
	_, err = kmc.SignMessage(nil, msg1)
	if err != ErrNilPointer {
		t.Fatalf("failed to catch error, got: %v, want: %v", err, ErrNilPointer)
	}

	// error: pk not exist
	buf, err := hex.DecodeString(samplePublicKey)
	if err != nil {
		t.Fatalf("failed to decode hex string, %v", err)
	}
	samplePk, err := pocec.ParsePubKey(buf, pocec.S256())
	if err != nil {
		t.Fatalf("failed to parse public key, %v", err)
	}
	_, err = kmc.SignMessage(samplePk, msg1)
	if err != ErrAccountNotFound {
		t.Fatalf("failed to catch error, got: %v, want: %v", err, ErrAccountNotFound)
	}

	sig, err := kmc.SignMessage(pk1, msg1)
	if err != nil {
		t.Fatalf("failed to sign message, %v", err)
	}

	msg1Hash := wire.HashH(msg1)
	_, err = kmc.VerifySig(sig, msg1Hash[:], pk1)
	if err != nil {
		t.Fatalf("failed to verify signature, %v", err)
	}
	t.Logf("pass test")
}

func TestBaseManager_SignHashPocec_VerifySigPocec(t *testing.T) {
	ldb, tearDown, err := GetDb("Tst_Manager")
	if err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	defer tearDown()

	t.Log("/*pocKeystoreManager*/")
	// new poc keystore manager
	kmc, err := NewKeystoreManagerForPoC(ldb, pubPassphrase, config.ChainParams)
	if err != nil {
		t.Fatalf("failed to new keystore manager, %v", err)
	}
	// new keystore
	acctID1, nerr := kmc.NewKeystore(privPassphrase, seed, "first", config.ChainParams, fastScrypt)
	if nerr != nil {
		t.Fatalf("failed to new keystore, %v", nerr)
	}

	err = kmc.Unlock(privPassphrase)
	if err != nil {
		t.Fatalf("failed to new  address, %v", err)
	}
	t.Log()
	t.Log("acctId: ", acctID1)
	mAddrs, err := kmc.NextAddresses(acctID1, true, 2)
	if err != nil {
		t.Fatalf("failed to new  address, %v", err)
	}
	pk := mAddrs[0].pubKey

	// get data hash 32bytes
	h1 := sha256.New()
	h1.Write(msg1)
	dataHash := h1.Sum(nil)
	sig, err := kmc.SignHash(pk, dataHash)
	if err != nil {
		t.Fatalf("failed to sign, %v", err)
	}
	t.Log("sig:", sig)
	isTrue, err := kmc.VerifySig(sig, dataHash, pk)
	if err != nil {
		t.Fatalf("failed to verify, %v", err)
	}
	t.Log("isTrue:", isTrue)

	// error test
	t.Log()
	t.Log("error test--wrong dataHash")
	// get data hash 32bytes
	h2 := sha256.New()
	h2.Write(msg2)
	dataHash2 := h2.Sum(nil)
	isTrue2, err := kmc.VerifySig(sig, dataHash2, pk)
	if err != nil {
		t.Fatalf("failed to verify, %v", err)
	}
	if isTrue2 != false {
		t.Fatalf("wrong hash but pass")
	}

	t.Log("error test--locked")
	kmc.Lock()
	_, err = kmc.VerifySig(sig, dataHash, pk)
	if err != ErrAddrManagerLocked {
		t.Fatalf("failed to catch error, want: %v, get: %v", ErrAddrManagerLocked, err)
	}
	t.Log("pass test")
}

func TestKeystoreManagerForPoC_GenerateNewPublicKeys(t *testing.T) {
	ldb, tearDown, err := GetDb("Tst_Manager")
	if err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	defer tearDown()

	t.Log("/*pocKeystoreManager*/")
	// new poc keystore manager
	kmc, err := NewKeystoreManagerForPoC(ldb, pubPassphrase, config.ChainParams)
	if err != nil {
		t.Fatalf("failed to new keystore manager, %v", err)
	}
	//new keystore
	acctID1, nerr := kmc.NewKeystore(privPassphrase, seed, "first", config.ChainParams, fastScrypt)
	if nerr != nil {
		t.Fatalf("failed to new keystore, %v", nerr)
	}
	acctID2, nerr := kmc.NewKeystore(privPassphrase, seed2, "second", config.ChainParams, fastScrypt)
	if nerr != nil {
		t.Fatalf("failed to new keystore, %v", nerr)
	}

	fmt.Printf("first: %v, second: %v\n", acctID1, acctID2)

	pk, _, err := kmc.GenerateNewPublicKey()
	if err != nil {
		t.Fatalf("failed to generate new public keys for poc, %v", err)
	}
	t.Logf("new pk: %v", pk.SerializeCompressed())

	pk, _, err = kmc.GenerateNewPublicKey()
	if err != nil {
		t.Fatalf("failed to generate new public keys for poc, %v", err)
	}
	t.Logf("new pk: %v", pk.SerializeCompressed())

	showAddrManagerDetails(t, kmc.managedKeystores[acctID1])
	showAddrManagerDetails(t, kmc.managedKeystores[acctID2])
}

func TestKeystoreManagerForPoC_ExportKeystore(t *testing.T) {
	ldb, tearDown, err := GetDb("Tst_Manager")
	if err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	defer tearDown()
	// new poc keystore manager
	t.Log("/*db*/")
	kmc, err := NewKeystoreManagerForPoC(ldb, pubPassphrase, config.ChainParams)
	if err != nil {
		t.Fatalf("failed to new keystore manager, %v", err)
	}
	// new keystore
	acctID0, cerr := kmc.NewKeystore(privPassphrase, seed, "first", config.ChainParams, fastScrypt)
	if cerr != nil {
		t.Fatalf("failed to new keystore, %v", cerr)
	}

	keystoreBytes, err := kmc.ExportKeystore(acctID0, privPassphrase)
	if err != nil {
		t.Fatalf("exportKeystore failed: %v", err)
	}
	t.Log("ketstore string: ", string(keystoreBytes))

	// write json file
	exportFileName := fmt.Sprintf("%s/%s-%s.json", exportFilePath, exportFileNamePerfix, acctID0)
	if !fileExists(exportFilePath) {
		err = os.MkdirAll(exportFilePath, 0777)
		if err != nil {
			t.Fatalf("failed to make dir, %v", err)
		}
	}

	file, err := os.OpenFile(exportFileName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		t.Fatalf("failed to open file, %v", err)
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	num, err := writer.WriteString(string(keystoreBytes))
	if err != nil {
		t.Fatalf("failed to write export file, %v", err)
	}
	t.Logf("return int, %v", num)
	writer.Flush()
}

func TestKeystoreManagerForPoC_ImportKeystore(t *testing.T) {
	acctID0 := "ac102vdc5l75yf7uytufjezpphf3m6hqk46hxp8k99"
	importFileName := fmt.Sprintf("%s/%s-%s.json", exportFilePath, exportFileNamePerfix, acctID0)
	file, err := os.Open(importFileName)
	if err != nil {
		t.Fatalf("failed to open keystore file, %v", err)
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	fileBytes, err := reader.ReadBytes('\n')
	if err != nil && err != io.EOF {
		t.Fatalf("failed to read keystore file, %v", err)
	}

	ldb, tearDown, err := GetDb("Tst_Manager")
	if err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	defer tearDown()
	// new poc keystore manager
	t.Log("/*db*/")
	kmc, err := NewKeystoreManagerForPoC(ldb, pubPassphrase, config.ChainParams)
	if err != nil {
		t.Fatalf("failed to new keystore manager, %v", err)
	}

	_, _, err = kmc.ImportKeystore(fileBytes, privPassphrase, pubPassphrase2)
	if err != nil {
		t.Fatalf("failed to import keystore, %v", err)
	}
	t.Log()
	t.Log("after import-km1.managedKeystores:", kmc.managedKeystores)
	t.Log()
	for acctId, acctM := range kmc.managedKeystores {
		t.Log()
		t.Log("acctId:", acctId)
		t.Log("acctManagerName: ", acctM.keystoreName)
		t.Log("acctType: ", acctM.acctInfo.acctType)
		t.Log("acctPub: ", acctM.acctInfo.acctKeyPub)
		t.Log("acctPriv: ", acctM.acctInfo.acctKeyPriv)
		t.Log("externalIndex: ", acctM.branchInfo.nextExternalIndex)
		t.Log("internalIndex: ", acctM.branchInfo.nextInternalIndex)

		externalAddresses, err := kmc.NextAddresses(acctId, false, 2)
		if err != nil {
			t.Fatalf("failed to new external address, %v", err)
		}

		internalAddresses, err := kmc.NextAddresses(acctId, true, 2)
		if err != nil {
			t.Fatalf("failed to new internal address, %v", err)
		}

		t.Log("addrNum:", acctM.branchInfo.nextExternalIndex)
		for i, addr := range externalAddresses {
			t.Logf("new external address %v: %v\n", i, addr.address)
			t.Logf("derivation path: %v\n", addr.derivationPath)
			t.Logf("private key: %v\n", addr.privKey)
		}

		for i, addr := range internalAddresses {
			t.Logf("new internal address %v: %v\n", i, addr.address)
			t.Logf("derivation path: %v\n", addr.derivationPath)
			t.Logf("private key: %v\n", addr.privKey)
		}

		t.Log("after NewAddress, externalIndex: ", acctM.branchInfo.nextExternalIndex)
		t.Log("after NewAddress, internalIndex: ", acctM.branchInfo.nextInternalIndex)

		for addrName, addr := range acctM.addrs {
			t.Log("address: ", addrName)
			t.Log("path: ", addr.derivationPath)
			t.Log("")
		}
	}
	t.Log("/*right example--success*/")

}

func TestCheckPassword(t *testing.T) {
	ldb, tearDown, err := GetDb("Tst_Manager")
	if err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	defer tearDown()

	t.Log("/*pocKeystoreManager*/")
	// new poc keystore manager
	kmc, err := NewKeystoreManagerForPoC(ldb, pubPassphrase, config.ChainParams)
	if err != nil {
		t.Fatalf("failed to new keystore manager, %v", err)
	}
	//new keystore
	acctID1, nerr := kmc.NewKeystore(privPassphrase, seed, "first", config.ChainParams, fastScrypt)
	if nerr != nil {
		t.Fatalf("failed to new keystore, %v", nerr)
	}

	t.Logf("before check, %v", kmc.managedKeystores[acctID1].masterKeyPriv.Key)
	err = kmc.managedKeystores[acctID1].checkPassword(privPassphrase)
	if err != nil {
		t.Fatalf("failed to check privPassphrase, %v", err)
	}

	t.Logf("after check, %v", kmc.managedKeystores[acctID1].masterKeyPriv.Key)

	kmc.managedKeystores[acctID1].masterKeyPriv.Zero()

	err = kmc.managedKeystores[acctID1].checkPassword(privPassphrase2)
	if err != nil {
		t.Logf("failed to check privPassphrase, %v", err)
	}

	t.Logf("after check, %v", kmc.managedKeystores[acctID1].masterKeyPriv.Key)
}

func TestKeystoreManagerForPoC_NextAddresses(t *testing.T) {
	ldb, tearDown, err := GetDb("Tst_Manager")
	if err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	defer tearDown()

	t.Log("/*pocKeystoreManager*/")
	// new poc keystore manager
	kmc, err := NewKeystoreManagerForPoC(ldb, pubPassphrase, config.ChainParams)
	if err != nil {
		t.Fatalf("failed to new keystore manager, %v", err)
	}
	//new keystore
	acctID1, nerr := kmc.NewKeystore(privPassphrase, seed, "first", config.ChainParams, fastScrypt)
	if nerr != nil {
		t.Fatalf("failed to new keystore, %v", nerr)
	}

	err = kmc.Unlock(privPassphrase)
	if err != nil {
		t.Fatalf("failed to unlock keystore manager for poc, %v", err)
	}

	mAddrs, err := kmc.NextAddresses(acctID1, false, 30)
	if err != nil {
		t.Fatalf("failed to new addresses, %v", err)
	}

	for _, mAddr := range mAddrs {
		fmt.Printf("\"%s\",\n", hex.EncodeToString(mAddr.privKey.Serialize()))
	}
}

func TestKeystoreManagerForPoC_Unlock_NewKeystore(t *testing.T) {
	ldb, tearDown, err := GetDb("Tst_Manager")
	if err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	defer tearDown()

	t.Log("/*pocKeystoreManager*/")
	// new poc keystore manager
	kmc, err := NewKeystoreManagerForPoC(ldb, pubPassphrase, config.ChainParams)
	if err != nil {
		t.Fatalf("failed to new keystore manager, %v", err)
	}

	acctID1, nerr := kmc.NewKeystore(privPassphrase, seed, "first", config.ChainParams, fastScrypt)
	if nerr != nil {
		t.Fatalf("failed to new keystore, %v", nerr)
	}

	err = kmc.Unlock(privPassphrase)
	if err != nil {
		t.Fatalf("failed to unlock keystore manager for poc, %v", err)
	}

	_, err = kmc.NextAddresses(acctID1, false, 2)
	if err != nil {
		t.Fatalf("failed to new addresses, account: %v, %v", acctID1, err)
	}

	acctID2, nerr := kmc.NewKeystore(privPassphrase, seed2, "second", config.ChainParams, fastScrypt)
	if nerr != nil {
		t.Fatalf("failed to new keystore, %v", nerr)
	}

	_, err = kmc.NextAddresses(acctID2, false, 4)
	if err != nil {
		t.Fatalf("failed to new addresses, account: %v, %v", acctID2, err)
	}

	showAddrManagerDetails(t, kmc.managedKeystores[acctID1])
	showAddrManagerDetails(t, kmc.managedKeystores[acctID2])
}

func TestKeystoreManagerForPoC_GetPublicKeyOrdinal(t *testing.T) {
	ldb, tearDown, err := GetDb("Tst_Manager")
	if err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	defer tearDown()

	t.Log("/*KeystoreManagerForPoC*/")
	// new poc keystore manager
	kmc, err := NewKeystoreManagerForPoC(ldb, pubPassphrase, config.ChainParams)
	if err != nil {
		t.Fatalf("failed to new keystore manager, %v", err)
	}

	//new keystore
	_, err = kmc.NewKeystore(privPassphrase, seed, "first", config.ChainParams, fastScrypt)
	if err != nil {
		t.Fatalf("failed to new keystore, %v", err)
	}

	pk, ordinal, err := kmc.GenerateNewPublicKey()
	if err != nil {
		t.Fatalf("failed to generate new public key, %v", err)
	}

	ordinalGet, exist := kmc.GetPublicKeyOrdinal(pk)
	if !exist {
		t.Fatalf("failed to find public key in wallet")
	}
	if ordinalGet != ordinal {
		t.Fatalf("failed to check pk ordinal, excepted: %v, get: %v", ordinal, ordinalGet)
	}

	// error: nil pointer
	_, exist = kmc.GetPublicKeyOrdinal(nil)
	if exist {
		t.Fatalf("failed to catch error, nil pointer")
	}

	// error: not in current wallet
	buf, err := hex.DecodeString(samplePublicKey)
	if err != nil {
		t.Fatalf("failed to decode hex string, %v", err)
	}
	samplePk, err := pocec.ParsePubKey(buf, pocec.S256())
	if err != nil {
		t.Fatalf("failed to parse public key, %v", err)
	}
	_, exist = kmc.GetPublicKeyOrdinal(samplePk)
	if exist {
		t.Fatalf("sample public key not in wallet")
	}
	t.Logf("pass test")
}

func TestKeystoreManagerForPoC_GetAddressByPubKey(t *testing.T) {
	ldb, tearDown, err := GetDb("Tst_Manager")
	if err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	defer tearDown()

	t.Log("/*KeystoreManagerForPoC*/")
	// new poc keystore manager
	kmc, err := NewKeystoreManagerForPoC(ldb, pubPassphrase, config.ChainParams)
	if err != nil {
		t.Fatalf("failed to new keystore manager, %v", err)
	}

	//new keystore
	accountId, err := kmc.NewKeystore(privPassphrase, seed, "first", config.ChainParams, fastScrypt)
	if err != nil {
		t.Fatalf("failed to new keystore, %v", err)
	}

	pk, _, err := kmc.GenerateNewPublicKey()
	if err != nil {
		t.Fatalf("failed to generate new public key, %v", err)
	}

	// error: nil pointer
	_, err = kmc.GetAddressByPubKey(nil)
	if err != ErrNilPointer {
		t.Fatalf("failed to catch error, want: nil pointer, actual: %v", err)
	}
	t.Logf("pass test 1")

	// error: not in current wallet
	buf, err := hex.DecodeString(samplePublicKey)
	if err != nil {
		t.Fatalf("failed to decode hex string, %v", err)
	}
	samplePk, err := pocec.ParsePubKey(buf, pocec.S256())
	if err != nil {
		t.Fatalf("failed to parse public key, %v", err)
	}
	_, err = kmc.GetAddressByPubKey(samplePk)
	if err != ErrAccountNotFound {
		t.Fatalf("failed to catch error, want: account not found, actual: %v", err)
	}
	t.Logf("pass test 2")

	addr, err := kmc.GetAddressByPubKey(pk)
	if err != nil {
		t.Fatalf("failed to get address by public key, %v", err)
	}

	mAddrs := kmc.managedKeystores[accountId].ManagedAddresses()
	if addr != mAddrs[0].address {
		t.Fatalf("address is different")
	}
	t.Logf("pass test")
}

func showAddrManagerDetails(t *testing.T, addrM *AddrManager) {
	t.Logf("keystore name: %v", addrM.Name())
	t.Logf("reamark: %v", addrM.Remarks())
	t.Logf("usage: %v", addrM.AddrUse())
	t.Logf("keyscope: %v", addrM.KeyScope())
	t.Logf("account info, type: %v", addrM.acctInfo.acctType)
	//t.Logf("account info, privkey: %v", addrM.acctInfo.acctKeyPriv.String())
	ex, in := addrM.CountAddresses()
	t.Logf("branch info, external index: %v, internal index: %v", addrM.branchInfo.nextExternalIndex, addrM.branchInfo.nextInternalIndex)
	t.Logf("branch info, external addresses count: %v, internal addresses count: %v", ex, in)
	t.Logf("unlocked: %v", addrM.unlocked)
	t.Logf("master pubkey: %v", addrM.masterKeyPub.Marshal())
	t.Logf("master privkey: %v", addrM.masterKeyPriv.Marshal())
	t.Logf("salt: %v", addrM.privPassphraseSalt)
	t.Logf("hash: %v", addrM.hashedPrivPassphrase)
	t.Logf("")
	for _, addr := range addrM.ManagedAddresses() {
		managedAddressDetails(t, addr)
	}
}

func TestNewPublicKey(t *testing.T) {
	masterHDPriv, err := hex.DecodeString(masterHDPrivStr)
	if err != nil {
		t.Fatalf("failed to decode masterHDPriv, error: %v", err)
	}
	masterChainCode, err := hex.DecodeString(masterChainCodeStr)
	if err != nil {
		t.Fatalf("failed to decode masterChainCode, error: %v", err)
	}

	parentFP := []byte{0x00, 0x00, 0x00, 0x00}
	rootKey := hdkeychain.NewExtendedKey(config.ChainParams.HDPrivateKeyID[:], masterHDPriv, masterChainCode,
		parentFP, 0, 0, true)

	t.Log("master", hex.EncodeToString(rootKey.PublicKey()))
	hdPath := &hdPath{
		Account:          uint32(PoCUsage),
		InternalChildNum: 0,
		ExternalChildNum: 0,
	}
	scope := Net2KeyScope[config.ChainParams.HDCoinType]
	coinTypeKeyPriv, err := deriveCoinTypeKey(rootKey, scope)
	if err != nil {
		t.Fatalf("failed to derive coinType key, error: %v", err)
	}
	t.Log("cointype", hex.EncodeToString(coinTypeKeyPriv.PublicKey()))

	acctKeyPriv, err := deriveAccountKey(coinTypeKeyPriv, hdPath.Account)
	if err != nil {
		t.Fatalf("failed to derive account key, error: %v", err)
	}
	t.Log("account", hex.EncodeToString(acctKeyPriv.PublicKey()))

	err = checkBranchKeys(acctKeyPriv)
	if err != nil {
		t.Fatalf("failed to check branch key, error: %v", err)
	}

	externalBranchPrivKey, err := acctKeyPriv.Child(ExternalBranch)
	if err != nil {
		t.Fatalf("failed to derive branch key, error: %v", err)
	}
	t.Log("branch", hex.EncodeToString(externalBranchPrivKey.PublicKey()))

	childKey, err := externalBranchPrivKey.Child(754)
	if err != nil {
		t.Fatalf("failed to derive child key, error: %v", err)
	}
	pkStr := hex.EncodeToString(childKey.PublicKey())
	t.Log("pk", pkStr)
}
