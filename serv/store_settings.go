package serv

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dosco/graphjin/core/v3"
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

// SetStore names where a start-config install keeps its store, a file or a
// Postgres address, and the key its stored configuration is encrypted with.
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

// storedSecretKind marks a single secret kept in the store, such as the model
// provider's key. Each is encrypted the same way as the configuration.
const storedSecretKind = "secret"

// StoredModelKey names the model provider's key in the store, and
// StoredModelKeyEnv the environment variable a start-config install hands it
// to the agent in. A name of its own keeps it from being mistaken for, or
// overwritten by, a provider variable already in the environment.
const (
	StoredModelKey    = "model_key"
	StoredModelKeyEnv = "DEEPORG_MODEL_KEY"
)

// ReadStoredSecret returns the named secret from the store file at path, or
// "" when it has not been set.
func ReadStoredSecret(ctx context.Context, path, sealKey, name string) (string, error) {
	st, done, err := openStoreSettings(ctx, path)
	if err != nil {
		return "", err
	}
	defer done()
	return readStoredSecret(ctx, st, sealKey, name)
}

func readStoredSecret(ctx context.Context, st *sqlAuthStore, sealKey, name string) (string, error) {
	row, err := st.get(ctx, storedSecretKind, name)
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

func writeStoredSecret(ctx context.Context, st *sqlAuthStore, sealKey, name, value string) error {
	sealed, err := sealStoredConfig(sealKey, value)
	if err != nil {
		return err
	}
	return st.upsert(ctx, storedSecretKind, name, &storedConfigRecord{Sealed: sealed}, time.Time{})
}

// StoreIsPostgres reports whether a store setting names a Postgres database
// and not a local file.
func StoreIsPostgres(store string) bool {
	lower := strings.ToLower(strings.TrimSpace(store))
	return strings.HasPrefix(lower, "postgres://") || strings.HasPrefix(lower, "postgresql://")
}

// storeConnectTimeout bounds the first connection to a store on a database
// server, so a wrong address is reported and not waited on.
const storeConnectTimeout = 10 * time.Second

// openStorePostgres connects to the Postgres database a store is kept in.
func openStorePostgres(store string) (*sql.DB, error) {
	dbConf := core.DatabaseConfig{Type: "postgres", ConnString: strings.TrimSpace(store)}
	conf := configuredDatabaseServiceConfig("postgres", dbConf)
	driver, err := initDBDriver(conf, false, false, core.NewOsFS(""))
	if err != nil {
		return nil, err
	}
	return connectConfiguredDatabase(driver, conf.DB, storeConnectTimeout)
}

// storeTablesLockKey names the lock copies take while they create or change
// the store's tables. Any number will do as long as every copy uses it.
const storeTablesLockKey = 0x64656570_6f7267 // "deeporg"

// lockStoreTables makes copies of a server take turns creating the store's
// tables. Postgres does not let two sessions create the same schema or table
// at the same moment, even with IF NOT EXISTS: the one that loses fails, and
// a copy starting beside another on an empty store would stop there. The lock
// is held on a connection of its own until unlock is called. A file store has
// one process and needs no lock.
func lockStoreTables(ctx context.Context, db *sql.DB, dbType string) (unlock func(), err error) {
	if dbType != "postgres" {
		return func() {}, nil
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, int64(storeTablesLockKey)); err != nil {
		conn.Close() //nolint:errcheck
		return nil, err
	}
	return func() {
		// Closing the connection would release it too; say so plainly.
		conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, int64(storeTablesLockKey)) //nolint:errcheck
		conn.Close()                                                                                       //nolint:errcheck
	}, nil
}

// storeAddressForError is a store setting with any password taken out, for a
// message someone will read.
func storeAddressForError(store string) string {
	if !StoreIsPostgres(store) {
		return store
	}
	u, err := url.Parse(strings.TrimSpace(store))
	if err != nil || u.Host == "" {
		return "the Postgres store"
	}
	return "the Postgres store at " + u.Host + u.Path
}

// openStoreSettings opens the store directly, for the short reads and writes
// that happen before the server starts and for the commands that work on a
// store. store is a file path or a Postgres address. A file store allows one
// connection, so the caller must call done before the server opens it.
func openStoreSettings(ctx context.Context, store string) (*sqlAuthStore, func(), error) {
	store = strings.TrimSpace(store)
	if store == "" {
		return nil, nil, errors.New("no store path")
	}
	// The server reads the same table once it is running, so this has to be
	// the schema it uses: the store's default.
	const schema = defaultStoreSchema
	dbType := "sqlite"
	var db *sql.DB
	if StoreIsPostgres(store) {
		dbType = "postgres"
		var err error
		if db, err = openStorePostgres(store); err != nil {
			return nil, nil, fmt.Errorf("could not connect to %s: %s", storeAddressForError(store), redactRuntimeError(err))
		}
	} else {
		path := store
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
		var err error
		if db, err = openManagedArtifactSQLite(ctx, path); err != nil {
			return nil, nil, fmt.Errorf("store %s: %w", path, err)
		}
		if err := os.Chmod(path, 0o600); err != nil {
			db.Close() //nolint:errcheck
			return nil, nil, fmt.Errorf("store %s: set permissions: %w", path, err)
		}
	}
	done := func() { db.Close() } //nolint:errcheck
	where := storeAddressForError(store)
	unlock, err := lockStoreTables(ctx, db, dbType)
	if err != nil {
		done()
		return nil, nil, fmt.Errorf("store %s: %s", where, redactRuntimeError(err))
	}
	defer unlock()
	for _, stmt := range authStoreDDL(dbType, schema) {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			done()
			return nil, nil, fmt.Errorf("store %s: %s", where, redactRuntimeError(err))
		}
	}
	if err := migrateAuthStoreSchema(ctx, db, dbType, schema); err != nil {
		done()
		return nil, nil, fmt.Errorf("store %s: %s", where, redactRuntimeError(err))
	}
	return newSQLAuthStore(db, dbType, schema), done, nil
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
