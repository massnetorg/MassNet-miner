package mining

import (
	"errors"
	"sync/atomic"

	"github.com/massnetorg/mass-core/massutil"
	_ "massnet.org/mass/poc/engine.v2/pocminer/miner"
	_ "massnet.org/mass/poc/engine/pocminer/miner"
	"massnet.org/mass/poc/wallet/keystore"
)

var ErrNotImplemented = errors.New("not implemented")

type PoCMiner interface {
	Start() error
	Stop() error
	Started() bool
	Type() string
	SetPayoutAddresses(addresses []massutil.Address) error
}

type MockedPoCMiner struct{}

func NewMockedPoCMiner() *MockedPoCMiner {
	return &MockedPoCMiner{}
}

func (m *MockedPoCMiner) Start() error {
	return nil
}

func (m *MockedPoCMiner) Stop() error {
	return nil
}

func (m *MockedPoCMiner) Started() bool {
	return false
}

func (m *MockedPoCMiner) Type() string {
	return "pocminer.mock"
}

func (m *MockedPoCMiner) SetPayoutAddresses(addresses []massutil.Address) error {
	return ErrNotImplemented
}

type PoCWallet interface {
	IsLocked() bool
	Unlock(privPassphrase []byte) error
	Lock()
	ChangePrivPassphrase(oldPrivPass, newPrivPass []byte, scryptConfig *keystore.ScryptOptions) error
	ChangePubPassphrase(oldPubPass, newPubPass []byte, scryptConfig *keystore.ScryptOptions) error
	ExportKeystore(accountID string, privPassphrase []byte) ([]byte, error)
	GetManagedAddrManager() []*keystore.AddrManager
	ImportKeystore(keystoreJson []byte, oldPrivPass, newPrivPass []byte) (string, string, error)
}

type MockedPoCWallet struct {
	locked int32 // atomic
}

func NewMockedPoCWallet() *MockedPoCWallet {
	return &MockedPoCWallet{}
}

func (m *MockedPoCWallet) IsLocked() bool {
	return atomic.LoadInt32(&m.locked) == 1
}

func (m *MockedPoCWallet) Unlock(privPassphrase []byte) error {
	atomic.StoreInt32(&m.locked, 0)
	return nil
}

func (m *MockedPoCWallet) Lock() {
	atomic.StoreInt32(&m.locked, 1)
}

func (m *MockedPoCWallet) ChangePrivPassphrase(oldPrivPass, newPrivPass []byte, scryptConfig *keystore.ScryptOptions) error {
	return ErrNotImplemented
}

func (m *MockedPoCWallet) ChangePubPassphrase(oldPubPass, newPubPass []byte, scryptConfig *keystore.ScryptOptions) error {
	return ErrNotImplemented
}

func (m *MockedPoCWallet) ExportKeystore(accountID string, privPassphrase []byte) ([]byte, error) {
	return nil, ErrNotImplemented
}

func (m *MockedPoCWallet) GetManagedAddrManager() []*keystore.AddrManager {
	return []*keystore.AddrManager{}
}

func (m *MockedPoCWallet) ImportKeystore(keystoreJson []byte, oldPrivPass, newPrivPass []byte) (string, string, error) {
	return "", "", ErrNotImplemented
}
