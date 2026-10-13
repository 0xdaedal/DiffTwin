package credentials

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

const service = "DiffBench" // Preserve access to credentials saved by the previous app name.

type Store interface {
	Get(projectID string) (string, error)
	Set(projectID, value string) error
	Delete(projectID string) error
}

type KeyringStore struct{}

func IsNotFound(err error) bool {
	return errors.Is(err, keyring.ErrNotFound)
}

func (KeyringStore) Get(projectID string) (string, error) {
	value, err := keyring.Get(service, projectID)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", err
		}
		return "", fmt.Errorf("read credentials from system keyring: %w", err)
	}
	return value, nil
}

func (KeyringStore) Set(projectID, value string) error {
	if err := keyring.Set(service, projectID, value); err != nil {
		return fmt.Errorf("save credentials to system keyring: %w", err)
	}
	return nil
}

func (KeyringStore) Delete(projectID string) error {
	if err := keyring.Delete(service, projectID); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("delete credentials from system keyring: %w", err)
	}
	return nil
}
