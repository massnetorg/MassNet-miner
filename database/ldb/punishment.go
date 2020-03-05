package ldb

import (
	"massnet.org/mass/database/storage"
	"massnet.org/mass/pocec"
	"massnet.org/mass/wire"
)

var (
	punishmentPrefix = []byte("PUNISH")
)

func punishmentPubKeyToKey(pk *pocec.PublicKey) []byte {
	keyBytes := pk.SerializeCompressed()
	key := make([]byte, len(keyBytes)+len(punishmentPrefix))
	copy(key, punishmentPrefix)
	copy(key[len(punishmentPrefix):], keyBytes[:])
	return key
}

func insertPunishmentAtomic(batch storage.Batch, fpk *wire.FaultPubKey) error {
	key := punishmentPubKeyToKey(fpk.PubKey)
	data, err := fpk.Bytes(wire.DB)
	if err != nil {
		return err
	}
	return batch.Put(key, data)
}

func insertPunishments(batch storage.Batch, fpks []*wire.FaultPubKey) error {
	for _, fpk := range fpks {
		err := insertPunishmentAtomic(batch, fpk)
		if err != nil {
			return err
		}
	}
	return nil
}

func (db *ChainDb) InsertPunishment(fpk *wire.FaultPubKey) error {

	return db.insertPunishment(fpk)
}

func (db *ChainDb) insertPunishment(fpk *wire.FaultPubKey) error {
	key := punishmentPubKeyToKey(fpk.PubKey)
	data, err := fpk.Bytes(wire.DB)
	if err != nil {
		return err
	}
	return db.stor.Put(key, data)
}

func dropPunishments(batch storage.Batch, pks []*wire.FaultPubKey) error {
	for _, pk := range pks {
		key := punishmentPubKeyToKey(pk.PubKey)
		batch.Delete(key)
	}
	return nil
}

func (db *ChainDb) ExistsPunishment(pk *pocec.PublicKey) (bool, error) {
	return db.existsPunishment(pk)
}

func (db *ChainDb) existsPunishment(pk *pocec.PublicKey) (bool, error) {
	key := punishmentPubKeyToKey(pk)
	return db.stor.Has(key)
}

func (db *ChainDb) FetchAllPunishment() ([]*wire.FaultPubKey, error) {

	return db.fetchAllPunishment()
}

func (db *ChainDb) fetchAllPunishment() ([]*wire.FaultPubKey, error) {
	res := make([]*wire.FaultPubKey, 0)
	iter := db.stor.NewIterator(storage.BytesPrefix(punishmentPrefix))
	defer iter.Release()

	for iter.Next() {
		fpk, err := wire.NewFaultPubKeyFromBytes(iter.Value(), wire.DB)
		if err != nil {
			return nil, err
		}
		res = append(res, fpk)
	}
	if err := iter.Error(); err != nil {
		return nil, err
	}
	return res, nil
}
