package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/QuantumNous/new-api/common"
)

var errStatusNotFound = errors.New("updater status not found")

type fileStatusStore struct {
	path string
}

func (store fileStatusStore) Load() (updateStatus, error) {
	data, err := os.ReadFile(store.path)
	if errors.Is(err, os.ErrNotExist) {
		return updateStatus{}, errStatusNotFound
	}
	if err != nil {
		return updateStatus{}, fmt.Errorf("read updater status: %w", err)
	}
	var status updateStatus
	if err := common.Unmarshal(data, &status); err != nil {
		return updateStatus{}, fmt.Errorf("decode updater status: %w", err)
	}
	return status, nil
}

func (store fileStatusStore) Save(status updateStatus) error {
	if err := os.MkdirAll(filepath.Dir(store.path), 0o700); err != nil {
		return fmt.Errorf("create updater state directory: %w", err)
	}
	payload, err := common.Marshal(status)
	if err != nil {
		return fmt.Errorf("encode updater status: %w", err)
	}
	temporary := store.path + ".tmp"
	if err := os.WriteFile(temporary, payload, 0o600); err != nil {
		return fmt.Errorf("write updater status: %w", err)
	}
	if err := os.Rename(temporary, store.path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("replace updater status: %w", err)
	}
	return nil
}
