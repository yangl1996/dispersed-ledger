package main

import (
	"time"

	"github.com/akrylysov/pogreb"
	"github.com/golang/snappy"
)

type PogrebStore struct {
	*pogreb.DB
}

func NewPogrebStore(path string) (*PogrebStore, error) {
	opts := &pogreb.Options{
		BackgroundCompactionInterval: time.Duration(30 * time.Second),
	}
	db, err := pogreb.Open(path, opts)
	return &PogrebStore{db}, err
}

func (p *PogrebStore) Put(key, value []byte) error {
	//compressedKey := snappy.Encode(nil, key)
	compressedVal := snappy.Encode(nil, value)
	return p.DB.Put(key, compressedVal)
}

func (p *PogrebStore) Get(key []byte) ([]byte, error) {
	//compressedKey := snappy.Encode(nil, key)
	val, err := p.DB.Get(key)
	if err != nil {
		return nil, err
	}
	decomVal, err := snappy.Decode(nil, val)
	return decomVal, err
}
