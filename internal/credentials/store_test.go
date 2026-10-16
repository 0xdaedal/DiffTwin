package credentials

import (
	"errors"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

const testCredentialLimit = 2560

type memoryKeyring struct {
	values          map[string]string
	failChunkWrites bool
}

func newMemoryKeyring() *memoryKeyring {
	return &memoryKeyring{values: make(map[string]string)}
}

func (m *memoryKeyring) Get(service, user string) (string, error) {
	value, ok := m.values[service+":"+user]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return value, nil
}

func (m *memoryKeyring) Set(service, user, password string) error {
	if m.failChunkWrites && strings.Contains(user, ":difftwin:v1:") {
		return errors.New("simulated keyring write failure")
	}
	if len(password) > testCredentialLimit {
		return keyring.ErrSetDataTooBig
	}
	m.values[service+":"+user] = password
	return nil
}

func (m *memoryKeyring) Delete(service, user string) error {
	key := service + ":" + user
	if _, ok := m.values[key]; !ok {
		return keyring.ErrNotFound
	}
	delete(m.values, key)
	return nil
}

func TestKeyringStoreChunksValuesBeyondWindowsCredentialLimit(t *testing.T) {
	backend := newMemoryKeyring()
	store := KeyringStore{backend: backend}
	value := strings.Repeat("a", chunkSize-1) + "é" + strings.Repeat("credential-data-", 500)

	if err := store.Set("project", value); err != nil {
		t.Fatal(err)
	}
	if got := len(backend.values); got < 2 {
		t.Fatalf("expected a manifest and credential chunks, stored %d entries", got)
	}
	for key, storedValue := range backend.values {
		if len(storedValue) > testCredentialLimit {
			t.Fatalf("keyring entry %q has %d bytes, above the Windows limit", key, len(storedValue))
		}
	}
	got, err := store.Get("project")
	if err != nil {
		t.Fatal(err)
	}
	if got != value {
		t.Fatal("reassembled credentials differ from the stored value")
	}
}

func TestKeyringStoreReadsLegacyValuesAndReplacesChunkedValues(t *testing.T) {
	backend := newMemoryKeyring()
	store := KeyringStore{backend: backend}
	backend.values[service+":legacy-project"] = `{"primary":{"token":"legacy"}}`

	legacy, err := store.Get("legacy-project")
	if err != nil || legacy != `{"primary":{"token":"legacy"}}` {
		t.Fatalf("legacy credentials = %q, error = %v", legacy, err)
	}

	large := strings.Repeat("token", 1000)
	if err := store.Set("project", large); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("project", "small"); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Get("project"); err != nil || got != "small" {
		t.Fatalf("credentials after replacement = %q, error = %v", got, err)
	}
	if len(backend.values) != 2 {
		t.Fatalf("expected only the legacy and replaced project entries, got %d", len(backend.values))
	}
}

func TestKeyringStoreDeletesChunkedValues(t *testing.T) {
	backend := newMemoryKeyring()
	store := KeyringStore{backend: backend}
	if err := store.Set("project", strings.Repeat("token", 1000)); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete("project"); err != nil {
		t.Fatal(err)
	}
	if len(backend.values) != 0 {
		t.Fatalf("credential chunks remain after deletion: %d", len(backend.values))
	}
}

func TestKeyringStorePreservesExistingValueWhenChunkWriteFails(t *testing.T) {
	backend := newMemoryKeyring()
	store := KeyringStore{backend: backend}
	if err := store.Set("project", "previous"); err != nil {
		t.Fatal(err)
	}
	backend.failChunkWrites = true

	err := store.Set("project", strings.Repeat("token", 1000))
	if err == nil {
		t.Fatal("Set succeeded despite the simulated keyring failure")
	}
	if got, err := store.Get("project"); err != nil || got != "previous" {
		t.Fatalf("credentials after failed replacement = %q, error = %v", got, err)
	}
	if len(backend.values) != 1 {
		t.Fatalf("failed replacement left partial credential chunks: %d entries", len(backend.values))
	}
}

func TestKeyringStoreRejectsValuesBeyondSupportedLimit(t *testing.T) {
	backend := newMemoryKeyring()
	store := KeyringStore{backend: backend}
	err := store.Set("project", strings.Repeat("x", maxCredentialSize+1))
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("Set error = %v, want ErrTooLarge", err)
	}
	if len(backend.values) != 0 {
		t.Fatalf("oversized credentials were partially stored: %d entries", len(backend.values))
	}
}
