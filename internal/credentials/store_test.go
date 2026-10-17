package credentials

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

const testCredentialLimit = 2560

type memoryKeyring struct {
	values          map[string]string
	failChunkWrites bool
	getErrors       map[string]error
	setErrors       map[string]error
	deleteErrors    map[string]error
}

func newMemoryKeyring() *memoryKeyring {
	return &memoryKeyring{values: make(map[string]string)}
}

func (m *memoryKeyring) Get(service, user string) (string, error) {
	if err := m.getErrors[service+":"+user]; err != nil {
		return "", err
	}
	value, ok := m.values[service+":"+user]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return value, nil
}

func (m *memoryKeyring) Set(service, user, password string) error {
	if err := m.setErrors[service+":"+user]; err != nil {
		return err
	}
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
	if err := m.deleteErrors[key]; err != nil {
		return err
	}
	if _, ok := m.values[key]; !ok {
		return keyring.ErrNotFound
	}
	delete(m.values, key)
	return nil
}

func TestKeyringStoreReadsAndValidatesChunkData(t *testing.T) {
	const projectID = "project"
	const generation = "0123456789abcdef0123456789abcdef"
	chunkKey := service + ":" + chunkUser(projectID, generation, 0)
	checksumX := sha256.Sum256([]byte("x"))
	makeManifest := func(value string) string {
		checksum := sha256.Sum256([]byte(value))
		return chunkManifestPrefix + generation + ":1:" + strconv.Itoa(len(value)) + ":" + hex.EncodeToString(checksum[:])
	}

	tests := []struct {
		name     string
		manifest string
		chunk    string
		want     string
		wantErr  string
	}{
		{
			name:     "missing chunk",
			manifest: chunkManifestPrefix + generation + ":1:1:" + hex.EncodeToString(checksumX[:]),
			wantErr:  "read credential chunk",
		},
		{
			name:     "invalid chunk encoding",
			manifest: chunkManifestPrefix + generation + ":1:1:" + hex.EncodeToString(checksumX[:]),
			chunk:    "not-base64",
			wantErr:  "decode credential chunk",
		},
		{
			name:     "chunk integrity mismatch",
			manifest: chunkManifestPrefix + generation + ":1:1:" + hex.EncodeToString(checksumX[:]),
			chunk:    base64.StdEncoding.EncodeToString([]byte("y")),
			wantErr:  "integrity verification",
		},
		{
			name:     "valid chunk",
			manifest: makeManifest("chunked-value"),
			chunk:    base64.StdEncoding.EncodeToString([]byte("chunked-value")),
			want:     "chunked-value",
		},
		{
			name:     "malformed manifest",
			manifest: chunkManifestPrefix + "invalid",
			wantErr:  "invalid credential chunk index",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := newMemoryKeyring()
			backend.values[service+":"+projectID] = test.manifest
			if test.chunk != "" {
				backend.values[chunkKey] = test.chunk
			}
			store := KeyringStore{backend: backend}
			got, err := store.Get(projectID)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("Get() error = %v, want substring %q", err, test.wantErr)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("Get() = %q, %v, want %q", got, err, test.want)
			}
		})
	}
}

func TestKeyringStoreWrapsBackendFailuresAndHandlesMissingDelete(t *testing.T) {
	backend := newMemoryKeyring()
	store := KeyringStore{backend: backend}
	readErr := errors.New("backend read failed")
	backend.getErrors = map[string]error{service + ":read": readErr}
	if _, err := store.Get("read"); !errors.Is(err, readErr) {
		t.Fatalf("Get() error = %v, want wrapped backend error", err)
	}

	backend.setErrors = map[string]error{service + ":write": errors.New("backend write failed")}
	if err := store.Set("write", "value"); err == nil || !strings.Contains(err.Error(), "backend write failed") {
		t.Fatalf("Set() error = %v, want backend write failure", err)
	}

	checksum := sha256.Sum256([]byte("x"))
	backend.values[service+":missing-chunk"] = chunkManifestPrefix + "0123456789abcdef0123456789abcdef:1:1:" +
		hex.EncodeToString(checksum[:])
	backend.getErrors = map[string]error{
		service + ":missing-chunk:difftwin:v1:0123456789abcdef0123456789abcdef:0": errors.New("chunk read failed"),
	}
	if _, err := store.Get("missing-chunk"); err == nil || !strings.Contains(err.Error(), "chunk read failed") {
		t.Fatalf("Get() missing chunk error = %v, want chunk read failure", err)
	}

	if err := store.Delete("never-stored"); err != nil {
		t.Fatalf("Delete() missing value error = %v, want nil", err)
	}
	backend.values[service+":delete"] = "value"
	backend.deleteErrors = map[string]error{service + ":delete": errors.New("backend delete failed")}
	if err := store.Delete("delete"); err == nil || !strings.Contains(err.Error(), "backend delete failed") {
		t.Fatalf("Delete() error = %v, want backend delete failure", err)
	}
}

func TestParseChunkManifestRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "wrong number of fields", value: chunkManifestPrefix + "id:1:1"},
		{name: "invalid count", value: chunkManifestPrefix + "0123456789abcdef0123456789abcdef:no:1:" + strings.Repeat("a", 64)},
		{name: "zero count", value: chunkManifestPrefix + "0123456789abcdef0123456789abcdef:0:1:" + strings.Repeat("a", 64)},
		{name: "zero size", value: chunkManifestPrefix + "0123456789abcdef0123456789abcdef:1:0:" + strings.Repeat("a", 64)},
		{name: "count does not match size", value: chunkManifestPrefix + "0123456789abcdef0123456789abcdef:2:1:" + strings.Repeat("a", 64)},
		{name: "invalid checksum", value: chunkManifestPrefix + "0123456789abcdef0123456789abcdef:1:1:bad"},
		{name: "invalid generation", value: chunkManifestPrefix + "not-hex:1:1:" + strings.Repeat("a", 64)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseChunkManifest(test.value); err == nil {
				t.Fatal("parseChunkManifest() accepted an invalid manifest")
			}
		})
	}
}

func TestKeyringStoreErrorClassification(t *testing.T) {
	if !IsNotFound(keyring.ErrNotFound) || IsNotFound(errors.New("other")) {
		t.Fatal("IsNotFound() did not classify keyring errors correctly")
	}
	if !IsDataTooBig(keyring.ErrSetDataTooBig) || IsDataTooBig(errors.New("other")) {
		t.Fatal("IsDataTooBig() did not classify keyring errors correctly")
	}
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
