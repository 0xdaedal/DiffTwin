package credentials

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/zalando/go-keyring"
)

const service = "DiffBench" // Preserve access to credentials saved by the previous app name.
const chunkSize = 1800
const maxCredentialSize = 64 * 1024
const chunkManifestPrefix = "difftwin-keyring-chunks:v1:"

var ErrTooLarge = errors.New("credentials exceed the supported size of the system keyring")

type Store interface {
	Get(projectID string) (string, error)
	Set(projectID, value string) error
	Delete(projectID string) error
}

type keyringBackend interface {
	Get(service, user string) (string, error)
	Set(service, user, password string) error
	Delete(service, user string) error
}

type systemKeyring struct{}

func (systemKeyring) Get(service, user string) (string, error) {
	return keyring.Get(service, user)
}

func (systemKeyring) Set(service, user, password string) error {
	return keyring.Set(service, user, password)
}

func (systemKeyring) Delete(service, user string) error {
	return keyring.Delete(service, user)
}

type KeyringStore struct {
	backend keyringBackend
}

func (s KeyringStore) keyring() keyringBackend {
	if s.backend != nil {
		return s.backend
	}
	return systemKeyring{}
}

func IsNotFound(err error) bool {
	return errors.Is(err, keyring.ErrNotFound)
}

func IsDataTooBig(err error) bool {
	return errors.Is(err, keyring.ErrSetDataTooBig)
}

func (s KeyringStore) Get(projectID string) (string, error) {
	return s.getWithBackend(s.keyring(), projectID)
}

func (s KeyringStore) getWithBackend(backend keyringBackend, projectID string) (string, error) {
	value, err := backend.Get(service, projectID)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", err
		}
		return "", fmt.Errorf("read credentials from system keyring: %w", err)
	}
	if !strings.HasPrefix(value, chunkManifestPrefix) {
		return value, nil
	}
	manifest, err := parseChunkManifest(value)
	if err != nil {
		return "", fmt.Errorf("read credentials from system keyring: %w", err)
	}
	chunks := make([]byte, 0, manifest.size)
	for index := 0; index < manifest.count; index++ {
		chunk, err := backend.Get(service, chunkUser(projectID, manifest.generation, index))
		if err != nil {
			return "", fmt.Errorf("read credential chunk %d from system keyring: %w", index+1, err)
		}
		decodedChunk, err := base64.StdEncoding.DecodeString(chunk)
		if err != nil {
			return "", fmt.Errorf("decode credential chunk %d from system keyring: %w", index+1, err)
		}
		chunks = append(chunks, decodedChunk...)
		if len(chunks) > maxCredentialSize {
			return "", errors.New("credential chunks exceed the supported size")
		}
	}
	if len(chunks) != manifest.size || sha256.Sum256(chunks) != manifest.checksum {
		return "", errors.New("credential chunks failed integrity verification")
	}
	return string(chunks), nil
}

func (s KeyringStore) Set(projectID, value string) error {
	backend := s.keyring()
	if len(value) > maxCredentialSize {
		return fmt.Errorf("save credentials to system keyring: %w (%d KiB limit)", ErrTooLarge, maxCredentialSize/1024)
	}

	oldValue, err := backend.Get(service, projectID)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("read existing credentials from system keyring: %w", err)
	}
	var oldManifest chunkManifest
	if err == nil && strings.HasPrefix(oldValue, chunkManifestPrefix) {
		oldManifest, err = parseChunkManifest(oldValue)
		if err != nil {
			return fmt.Errorf("read existing credentials from system keyring: %w", err)
		}
	}

	if err := backend.Set(service, projectID, value); err == nil {
		if oldManifest.count > 0 {
			if err := deleteChunks(backend, projectID, oldManifest); err != nil {
				return fmt.Errorf("remove replaced credential chunks from system keyring: %w", err)
			}
		}
		return nil
	} else if !errors.Is(err, keyring.ErrSetDataTooBig) {
		return fmt.Errorf("save credentials to system keyring: %w", err)
	}

	if err := s.setChunked(backend, projectID, value, oldManifest); err != nil {
		if IsDataTooBig(err) {
			return fmt.Errorf("save credentials to system keyring: %w", ErrTooLarge)
		}
		return err
	}
	return nil
}

type chunkManifest struct {
	generation string
	count      int
	size       int
	checksum   [sha256.Size]byte
}

func (s KeyringStore) setChunked(backend keyringBackend, projectID, value string, oldManifest chunkManifest) error {
	var generationBytes [16]byte
	if _, err := rand.Read(generationBytes[:]); err != nil {
		return fmt.Errorf("create credential chunk identifier: %w", err)
	}
	generation := hex.EncodeToString(generationBytes[:])
	count := (len(value) + chunkSize - 1) / chunkSize
	written := chunkManifest{generation: generation, count: count}
	cleanupWritten := func() {
		_ = deleteChunks(backend, projectID, written)
	}
	for index, start := 0, 0; start < len(value); index, start = index+1, start+chunkSize {
		end := min(start+chunkSize, len(value))
		chunk := base64.StdEncoding.EncodeToString([]byte(value[start:end]))
		if err := backend.Set(service, chunkUser(projectID, generation, index), chunk); err != nil {
			cleanupWritten()
			return fmt.Errorf("save credential chunk %d to system keyring: %w", index+1, err)
		}
	}
	checksum := sha256.Sum256([]byte(value))
	manifest := fmt.Sprintf("%s%s:%d:%d:%s", chunkManifestPrefix, generation, count, len(value), hex.EncodeToString(checksum[:]))
	if err := backend.Set(service, projectID, manifest); err != nil {
		cleanupWritten()
		return fmt.Errorf("save credential index to system keyring: %w", err)
	}
	if oldManifest.count > 0 {
		if err := deleteChunks(backend, projectID, oldManifest); err != nil {
			return fmt.Errorf("remove replaced credential chunks from system keyring: %w", err)
		}
	}
	return nil
}

func chunkUser(projectID, generation string, index int) string {
	return projectID + ":difftwin:v1:" + generation + ":" + strconv.Itoa(index)
}

func parseChunkManifest(value string) (chunkManifest, error) {
	parts := strings.Split(strings.TrimPrefix(value, chunkManifestPrefix), ":")
	if len(parts) != 4 {
		return chunkManifest{}, errors.New("invalid credential chunk index")
	}
	count, err := strconv.Atoi(parts[1])
	if err != nil || count < 1 || count > maxCredentialSize/chunkSize+1 {
		return chunkManifest{}, errors.New("invalid credential chunk count")
	}
	size, err := strconv.Atoi(parts[2])
	if err != nil || size < 1 || size > maxCredentialSize {
		return chunkManifest{}, errors.New("invalid credential chunk size")
	}
	if count != (size+chunkSize-1)/chunkSize {
		return chunkManifest{}, errors.New("invalid credential chunk count")
	}
	checksumBytes, err := hex.DecodeString(parts[3])
	if err != nil || len(checksumBytes) != sha256.Size {
		return chunkManifest{}, errors.New("invalid credential chunk checksum")
	}
	generationBytes, err := hex.DecodeString(parts[0])
	if err != nil || len(generationBytes) != 16 {
		return chunkManifest{}, errors.New("invalid credential chunk identifier")
	}
	var checksum [sha256.Size]byte
	copy(checksum[:], checksumBytes)
	return chunkManifest{generation: parts[0], count: count, size: size, checksum: checksum}, nil
}

func deleteChunks(backend keyringBackend, projectID string, manifest chunkManifest) error {
	for index := 0; index < manifest.count; index++ {
		err := backend.Delete(service, chunkUser(projectID, manifest.generation, index))
		if err != nil && !errors.Is(err, keyring.ErrNotFound) {
			return fmt.Errorf("delete credential chunk %d: %w", index+1, err)
		}
	}
	return nil
}

func (s KeyringStore) Delete(projectID string) error {
	backend := s.keyring()
	value, err := backend.Get(service, projectID)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("read credentials from system keyring: %w", err)
	}
	var manifest chunkManifest
	if err == nil && strings.HasPrefix(value, chunkManifestPrefix) {
		manifest, err = parseChunkManifest(value)
		if err != nil {
			return fmt.Errorf("read credentials from system keyring: %w", err)
		}
	}
	if err := backend.Delete(service, projectID); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("delete credentials from system keyring: %w", err)
	}
	if manifest.count > 0 {
		if err := deleteChunks(backend, projectID, manifest); err != nil {
			return fmt.Errorf("delete credentials from system keyring: %w", err)
		}
	}
	return nil
}
