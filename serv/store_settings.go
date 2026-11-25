package serv

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// A start-config install keeps its configuration in the store, not in a file.
// The start config says where the store is and holds the secret key; the
// document read from the store says everything else.
//
// The document sits in the store's keyed table beside the accounts. That
// table is read with plain SQL, before there is a config to start the engine
// from, and is never reachable through GraphQL.

const (
	storedConfigKind = "config"
	storedConfigID   = "current"

	// defaultStoreSchema is the schema, or table prefix, the store uses when
	// a config does not name one. A start-config install cannot name one.
	defaultStoreSchema = "_graphjin"
)

// storedConfigRefusedKeys are settings a stored document may not carry. They
// belong to the start config, or describe the store the document is read
// from, or are how the server decides who a caller is.
var storedConfigRefusedKeys = []string{
	"mode", "production", "host_port", "auth", "auth_fail_block", "secrets", "artifacts", "config_path",
}

// storedConfigRecord is the document as it is kept: encrypted, because it
// carries database passwords.
type storedConfigRecord struct {
	Sealed string `json:"sealed"`
}

// SetStore names the file a start-config install keeps its store in, and the
// key its stored configuration is encrypted with.
func (c *Config) SetStore(path, sealKey string) {
	if c != nil {
		c.storePath = strings.TrimSpace(path)
		c.storeSealKey = sealKey
	}
}

// sealStoredConfig encrypts a document with AES-256-GCM.
func sealStoredConfig(sealKey, doc string) (string, error) {
	gcm, err := storeCipher(sealKey)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(doc), nil)), nil
}

func openStoredConfig(sealKey, sealed string) (string, error) {
	gcm, err := storeCipher(sealKey)
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil || len(raw) < gcm.NonceSize() {
		return "", errors.New("the stored configuration is damaged")
	}
	doc, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", errors.New("the stored configuration cannot be read with this secret key; it was written with a different one")
	}
	return string(doc), nil
}

func storeCipher(sealKey string) (cipher.AEAD, error) {
	if strings.TrimSpace(sealKey) == "" {
		return nil, errors.New("no key to encrypt the stored configuration with")
	}
	key := sha256.Sum256([]byte(sealKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// ValidateStoredConfig checks that doc is YAML a stored configuration may hold.
func ValidateStoredConfig(doc string) error {
	settings, err := parseStoredConfig(doc)
	if err != nil {
		return err
	}
	for _, key := range storedConfigRefusedKeys {
		if _, ok := settings[key]; ok {
			return fmt.Errorf("%q cannot be set in the stored configuration; it belongs to the start config or is fixed for this kind of install", key)
		}
	}
	return nil
}

func parseStoredConfig(doc string) (map[string]any, error) {
	settings := map[string]any{}
	if strings.TrimSpace(doc) == "" {
		return settings, nil
	}
	if err := yaml.Unmarshal([]byte(doc), &settings); err != nil {
		return nil, fmt.Errorf("the stored configuration is not valid YAML: %w", err)
	}
	return settings, nil
}

// MergeStoredConfig lays the stored document over the base configuration and
// returns the YAML to load. Settings the document may not carry are dropped
// and named in ignored, so an old or hand-edited document cannot move the
// store or change how callers are identified.
func MergeStoredConfig(base, doc string) (merged string, ignored []string, err error) {
	settings := map[string]any{}
	if err := yaml.Unmarshal([]byte(base), &settings); err != nil {
		return "", nil, fmt.Errorf("base configuration: %w", err)
	}
	stored, err := parseStoredConfig(doc)
	if err != nil {
		return "", nil, err
	}
	refused := make(map[string]bool, len(storedConfigRefusedKeys))
	for _, key := range storedConfigRefusedKeys {
		refused[key] = true
	}
	for key, value := range stored {
		if refused[key] {
			ignored = append(ignored, key)
			continue
		}
		settings[key] = value
	}
	sort.Strings(ignored)
	out, err := yaml.Marshal(settings)
	if err != nil {
		return "", nil, err
	}
	return string(out), ignored, nil
}

// ReadStoredConfig returns the configuration document kept in the store file
// at path, or "" when there is none yet. It creates the store if it is new.
func ReadStoredConfig(ctx context.Context, path, sealKey string) (string, error) {
	st, done, err := openStoreSettings(ctx, path)
	if err != nil {
		return "", err
	}
	defer done()
	return readStoredConfig(ctx, st, sealKey)
}

// WriteStoredConfig replaces the configuration document in the store file at
// path. The server reads it the next time it starts.
func WriteStoredConfig(ctx context.Context, path, sealKey, doc string) error {
	st, done, err := openStoreSettings(ctx, path)
	if err != nil {
		return err
	}
	defer done()
	return writeStoredConfig(ctx, st, sealKey, doc)
}

func readStoredConfig(ctx context.Context, st *sqlAuthStore, sealKey string) (string, error) {
	row, err := st.get(ctx, storedConfigKind, storedConfigID)
	if err != nil || row == nil {
		return "", err
	}
	var record storedConfigRecord
	if err := json.Unmarshal(row.data, &record); err != nil {
		return "", err
	}
	if record.Sealed == "" {
		return "", nil
	}
	return openStoredConfig(sealKey, record.Sealed)
}

func writeStoredConfig(ctx context.Context, st *sqlAuthStore, sealKey, doc string) error {
	if err := ValidateStoredConfig(doc); err != nil {
		return err
	}
	sealed, err := sealStoredConfig(sealKey, doc)
	if err != nil {
		return err
	}
	return st.upsert(ctx, storedConfigKind, storedConfigID, &storedConfigRecord{Sealed: sealed}, time.Time{})
}

// openStoreSettings opens the store file directly, for the short reads and
// writes that happen before the server starts. The store allows one
// connection, so the caller must call done before the server opens it.
func openStoreSettings(ctx context.Context, path string) (*sqlAuthStore, func(), error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil, errors.New("no store path")
	}
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, nil, err
		}
		path = abs
	}
	if err := prepareManagedArtifactPath(path); err != nil {
		return nil, nil, fmt.Errorf("store %s: %w", path, err)
	}
	db, err := openManagedArtifactSQLite(ctx, path)
	if err != nil {
		return nil, nil, fmt.Errorf("store %s: %w", path, err)
	}
	done := func() { db.Close() } //nolint:errcheck
	if err := os.Chmod(path, 0o600); err != nil {
		done()
		return nil, nil, fmt.Errorf("store %s: set permissions: %w", path, err)
	}
	// The server reads the same table once it is running, so this has to be
	// the schema it uses: the store's default.
	const schema = defaultStoreSchema
	for _, stmt := range authStoreDDL("sqlite", schema) {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			done()
			return nil, nil, fmt.Errorf("store %s: %w", path, err)
		}
	}
	if err := migrateAuthStoreSchema(ctx, db, "sqlite", schema); err != nil {
		done()
		return nil, nil, fmt.Errorf("store %s: %w", path, err)
	}
	return newSQLAuthStore(db, "sqlite", schema), done, nil
}

// DeriveStoreSecret turns the start config's one secret key into a key for
// one purpose, so the key that signs sessions is never the key that seals
// stored secrets.
func DeriveStoreSecret(secretKey, purpose string) string {
	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte("deeporg/" + purpose + "/v1"))
	return hex.EncodeToString(mac.Sum(nil))
}

// SetupLink returns the address that creates the first admin, or "" once an
// admin exists or this server does not hold accounts.
func (s1 *HttpService) SetupLink() string {
	s, ok := s1.Load().(*graphjinService)
	if !ok || s == nil {
		return ""
	}
	return setupLink(s)
}
