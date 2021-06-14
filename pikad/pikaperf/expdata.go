package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type Server struct {
	Location string
}

func ReadServerInfo(expdir string) ([]Server, error) {
	var dt []Server
	f, err := os.Open(filepath.Join(expdir, "servers.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	err = dec.Decode(&dt)
	if err != nil {
		return nil, err
	}
	return dt, nil
}

func SearchServer(s []Server, loc string) int {
	q := strings.ToLower(loc)

	for idx, v := range s {
		if strings.Contains(strings.ToLower(v.Location), q) {
			return idx
		}
	}
	return -1
}
