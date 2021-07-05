package chiawallet

import (
	"encoding/json"
)

type KeystoreStorage struct {
	Keys map[string]string `json:"keys"`
}

func (store *KeystoreStorage) Bytes() ([]byte, error) {
	return json.MarshalIndent(store, "", "  ")
}

func (store *KeystoreStorage) FromBytes(data []byte) error {
	return json.Unmarshal(data, store)
}
