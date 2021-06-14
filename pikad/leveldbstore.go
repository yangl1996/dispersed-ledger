package main

import (
	"github.com/syndtr/goleveldb/leveldb"
)

type LevelDBStore struct {
	*leveldb.DB
}

func (d *LevelDBStore) Put(key []byte, value []byte) error {
	return d.DB.Put(key, value, nil)
}

func (d *LevelDBStore) Get(key []byte) ([]byte, error) {
	return d.DB.Get(key, nil)
}

func NewLevelDBStore(path string) (*LevelDBStore, error) {
	handle, err := leveldb.OpenFile(path, nil)
	return &LevelDBStore{handle}, err
}
