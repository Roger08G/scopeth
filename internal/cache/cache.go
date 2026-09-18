package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Roger08G/scopeth/internal/compiler"
	"github.com/Roger08G/scopeth/internal/input"
)

const (
	schemaVersion = "1.0"
	maxCacheBytes = 192 << 20
)

type entry struct {
	SchemaVersion string          `json:"schema_version"`
	Key           string          `json:"key"`
	PayloadSHA256 string          `json:"payload_sha256"`
	Payload       json.RawMessage `json:"payload"`
}

func Key(snapshot input.Snapshot, solcVersion, mode string, remappings []string, engineVersion string) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte("scopeth-compilation-cache\x00" + schemaVersion + "\x00" + engineVersion + "\x00"))
	_, _ = hash.Write([]byte(solcVersion + "\x00" + mode + "\x00"))
	orderedRemappings := append([]string(nil), remappings...)
	sort.Strings(orderedRemappings)
	for _, remapping := range orderedRemappings {
		_, _ = hash.Write([]byte("remapping\x00" + remapping + "\n"))
	}
	for _, source := range snapshot.Sources {
		_, _ = hash.Write([]byte("source\x00" + source.LogicalPath + "\x00" + source.SHA256 + "\n"))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func Load(base, key string) (compiler.Result, bool, error) {
	directory, err := directory(base, false)
	if err != nil {
		return compiler.Result{}, false, err
	}
	if directory == "" {
		return compiler.Result{}, false, nil
	}
	path, err := cachePath(directory, key)
	if err != nil {
		return compiler.Result{}, false, err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return compiler.Result{}, false, nil
	}
	if err != nil {
		return compiler.Result{}, false, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return compiler.Result{}, false, errors.New("CACHE_CORRUPT: la entrada de caché no es un archivo regular")
	}
	if info.Size() > maxCacheBytes {
		return compiler.Result{}, false, errors.New("CACHE_CORRUPT: la entrada de caché supera 192 MiB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return compiler.Result{}, false, err
	}
	var cached entry
	if err := json.Unmarshal(data, &cached); err != nil {
		return compiler.Result{}, false, fmt.Errorf("CACHE_CORRUPT: JSON inválido: %w", err)
	}
	if cached.SchemaVersion != schemaVersion || cached.Key != key {
		return compiler.Result{}, false, errors.New("CACHE_CORRUPT: identidad o esquema de caché incorrecto")
	}
	digest := sha256.Sum256(cached.Payload)
	if !strings.EqualFold(cached.PayloadSHA256, hex.EncodeToString(digest[:])) {
		return compiler.Result{}, false, errors.New("CACHE_CORRUPT: hash del payload incorrecto")
	}
	var result compiler.Result
	if err := json.Unmarshal(cached.Payload, &result); err != nil {
		return compiler.Result{}, false, fmt.Errorf("CACHE_CORRUPT: payload inválido: %w", err)
	}
	return result, true, nil
}

func Save(base, key string, result compiler.Result) error {
	directory, err := directory(base, true)
	if err != nil {
		return err
	}
	path, err := cachePath(directory, key)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if len(payload) > maxCacheBytes {
		return errors.New("RESOURCE_LIMIT: el resultado de compilación supera el límite de caché")
	}
	digest := sha256.Sum256(payload)
	data, err := json.Marshal(entry{
		SchemaVersion: schemaVersion, Key: key, PayloadSHA256: hex.EncodeToString(digest[:]), Payload: payload,
	})
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".scopeth-cache-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		if _, statErr := os.Lstat(path); statErr == nil {
			return nil
		}
		return err
	}
	committed = true
	return nil
}

func directory(base string, create bool) (string, error) {
	abs, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	if info, statErr := os.Lstat(abs); statErr == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("UNSAFE_PATH: la base de caché no es un directorio real")
		}
	} else if errors.Is(statErr, os.ErrNotExist) {
		if !create {
			return "", nil
		}
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return "", err
		}
	} else {
		return "", statErr
	}
	current := abs
	for _, segment := range []string{".scopeth", "cache", "compilation"} {
		current = filepath.Join(current, segment)
		info, statErr := os.Lstat(current)
		if statErr == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return "", errors.New("UNSAFE_PATH: componente de caché no seguro")
			}
			continue
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return "", statErr
		}
		if !create {
			return "", nil
		}
		if err := os.Mkdir(current, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
			return "", err
		}
	}
	return current, nil
}

func cachePath(directory, key string) (string, error) {
	if len(key) != 64 {
		return "", errors.New("CACHE_CORRUPT: clave de caché inválida")
	}
	if _, err := hex.DecodeString(key); err != nil {
		return "", errors.New("CACHE_CORRUPT: clave de caché no hexadecimal")
	}
	return filepath.Join(directory, key+".json"), nil
}
