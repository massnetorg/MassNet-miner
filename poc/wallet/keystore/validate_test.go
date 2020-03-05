package keystore_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"massnet.org/mass/poc/wallet/keystore"
)

func TestValidatePassphrase(t *testing.T) {
	//true
	assert.Equal(t, true, keystore.ValidatePassphrase([]byte("a23456")))
	assert.Equal(t, true, keystore.ValidatePassphrase([]byte("A23456")))
	assert.Equal(t, true, keystore.ValidatePassphrase([]byte("123456")))
	assert.Equal(t, true, keystore.ValidatePassphrase([]byte("12345678901234567890")))
	assert.Equal(t, true, keystore.ValidatePassphrase([]byte("0123456#")))
	assert.Equal(t, true, keystore.ValidatePassphrase([]byte("#0123456")))
	assert.Equal(t, true, keystore.ValidatePassphrase([]byte("@#$%^&")))

	// false
	assert.Equal(t, false, keystore.ValidatePassphrase([]byte("12345")))
	assert.Equal(t, false, keystore.ValidatePassphrase([]byte("12345678901234567890123456789012345678901")))
	assert.Equal(t, false, keystore.ValidatePassphrase([]byte("")))
	assert.Equal(t, false, keystore.ValidatePassphrase([]byte("*123456")))
	assert.Equal(t, false, keystore.ValidatePassphrase([]byte("1234567)")))
	assert.Equal(t, false, keystore.ValidatePassphrase([]byte(`(@#$%^&`)))
	assert.Equal(t, false, keystore.ValidatePassphrase([]byte(`@#$%^&*`)))
}
