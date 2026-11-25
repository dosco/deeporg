package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/dosco/graphjin/serv/v3"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// The start config is the one file a new install has. It says where the store
// is and holds the secret key; everything else is read from the store. On a
// laptop the first run writes it, so a newcomer never has to.

const (
	startConfigName   = "deeporg.yml"
	startConfigDirEnv = "DEEPORG_HOME"
	startConfigEnv    = "DEEPORG_CONFIG"

	startStoreEnv     = "DEEPORG_STORE"
	startSecretKeyEnv = "DEEPORG_SECRET_KEY"
	startListenEnv    = "DEEPORG_LISTEN"
	startLogLevelEnv  = "DEEPORG_LOG_LEVEL"

	// A laptop install listens on this machine only. A server sets listen.
	startDefaultListen = "127.0.0.1:8080"
)

type startConfig struct {
	Store     string `yaml:"store"`
	SecretKey string `yaml:"secret_key"`
	Listen    string `yaml:"listen,omitempty"`
	LogLevel  string `yaml:"log_level,omitempty"`

	// path is the file this was read from or written to.
	path string
	// created is true when this run wrote the file.
	created bool
}

// usesStartConfig decides which flow `serve` takes. The legacy flow is chosen
// by anything that marks an existing install: GO_ENV, an explicit config
// directory, or a ./config directory. Only an install with none of those, or
// one that already has a start config, takes the new flow.
func usesStartConfig(cmd *cobra.Command) (file string, ok bool) {
	if file := strings.TrimSpace(os.Getenv(startConfigEnv)); file != "" {
		return file, true
	}
	if cmd != nil && (cmd.Flags().Changed("path") || cmd.Flags().Changed("config")) {
		// --path names a config directory in the legacy flow. Pointed at a
		// file, it names a start config.
		if info, err := os.Stat(cpath); err == nil && !info.IsDir() {
			return cpath, true
		}
		return "", false
	}
	if os.Getenv("GO_ENV") != "" {
		return "", false
	}
	if _, err := os.Stat(startConfigName); err == nil {
		return startConfigName, true
	}
	if info, err := os.Stat("config"); err == nil && info.IsDir() {
		return "", false
	}
	home, err := startConfigHome()
	if err != nil {
		return "", false
	}
	return filepath.Join(home, startConfigName), true
}

// startConfigHome is where a laptop install keeps its start config and store.
func startConfigHome() (string, error) {
	if dir := strings.TrimSpace(os.Getenv(startConfigDirEnv)); dir != "" {
		return filepath.Abs(dir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".deeporg"), nil
}

// loadStartConfig reads the start config at file, creating it with a new
// secret key and a local store when it does not exist. Environment variables
// override what the file says.
func loadStartConfig(file string) (*startConfig, error) {
	file, err := filepath.Abs(file)
	if err != nil {
		return nil, err
	}
	sc := &startConfig{path: file}
	raw, err := os.ReadFile(file)
	switch {
	case err == nil:
		if err := yaml.Unmarshal(raw, sc); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
	case os.IsNotExist(err):
		sc.Store = filepath.Join(filepath.Dir(file), "store.db")
		sc.SecretKey = newInstallSecret()
		sc.Listen = startDefaultListen
		if err := writeStartConfig(sc); err != nil {
			return nil, err
		}
		sc.created = true
	default:
		return nil, err
	}

	for env, target := range map[string]*string{
		startStoreEnv: &sc.Store, startSecretKeyEnv: &sc.SecretKey, startListenEnv: &sc.Listen, startLogLevelEnv: &sc.LogLevel,
	} {
		if value := strings.TrimSpace(os.Getenv(env)); value != "" {
			*target = value
		}
	}
	if strings.TrimSpace(sc.SecretKey) == "" {
		return nil, missingSecretKeyError(file)
	}
	if len(strings.TrimSpace(sc.SecretKey)) < 32 {
		return nil, fmt.Errorf("the secret key in %s is too short; it needs at least 32 characters", file)
	}
	if strings.TrimSpace(sc.Store) == "" {
		sc.Store = filepath.Join(filepath.Dir(file), "store.db")
	}
	if sc.Store, err = startStorePath(sc.Store, filepath.Dir(file)); err != nil {
		return nil, err
	}
	if strings.TrimSpace(sc.Listen) == "" {
		sc.Listen = "0.0.0.0:8080"
	}
	return sc, nil
}

// missingSecretKeyError tells whoever started the server exactly what to add,
// with a key ready to paste, so nobody has to work out how to make one.
func missingSecretKeyError(file string) error {
	return fmt.Errorf("DeepORG needs a secret key.\nAdd this to %s, or set it as %s, and start again:\n\n  secret_key: %s\n", file, startSecretKeyEnv, newInstallSecret())
}

// startStorePath resolves the store setting to a file. The store is a local
// SQLite database for now; a database server as the store comes later.
func startStorePath(store, dir string) (string, error) {
	store = strings.TrimSpace(store)
	lower := strings.ToLower(store)
	for _, scheme := range []string{"postgres://", "postgresql://", "mysql://"} {
		if strings.HasPrefix(lower, scheme) {
			return "", errors.New("the store must be a local file for now; a Postgres or MySQL store is not supported yet")
		}
	}
	for _, prefix := range []string{"sqlite://", "sqlite:"} {
		if strings.HasPrefix(lower, prefix) {
			store = store[len(prefix):]
			break
		}
	}
	if strings.HasPrefix(store, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		store = filepath.Join(home, store[2:])
	}
	if !filepath.IsAbs(store) {
		store = filepath.Join(dir, store)
	}
	return filepath.Clean(store), nil
}

func writeStartConfig(sc *startConfig) error {
	if err := os.MkdirAll(filepath.Dir(sc.path), 0o700); err != nil {
		return err
	}
	body := fmt.Sprintf(`# DeepORG start config.
#
# This file says where DeepORG keeps its own data and holds its secret key.
# Everything else is set through setup and the console, and kept in the store.

# The database DeepORG keeps its own data in.
store: %s

# Signs sign-in sessions and encrypts stored secrets. Keep it private. If it is
# lost, everyone is signed out and stored database passwords cannot be read.
secret_key: %s

# Address and port to listen on. This one is reachable from this machine only.
listen: %s
`, yamlString(sc.Store), yamlString(sc.SecretKey), yamlString(sc.Listen))
	return os.WriteFile(sc.path, []byte(body), 0o600)
}

func yamlString(value string) string {
	out, err := yaml.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%q", value)
	}
	return strings.TrimSpace(string(out))
}

// startBaseConfig is the configuration every start-config install runs with
// before the stored document is laid over it. The MCP switches match the
// generated agentic config.
func startBaseConfig(sc *startConfig) string {
	logLevel := strings.TrimSpace(sc.LogLevel)
	if logLevel == "" {
		logLevel = "warn"
	}
	base := map[string]any{
		"app_name":                "DeepORG",
		"mode":                    "agentic",
		"host_port":               sc.Listen,
		"web_ui":                  true,
		"log_level":               logLevel,
		"log_format":              "plain",
		"http_compress":           true,
		"auth_fail_block":         true,
		"enable_tracing":          false,
		"reload_on_config_change": false,
		"auth": map[string]any{
			"type": "jwt",
			"jwt":  map[string]any{"secret": serv.DeriveStoreSecret(sc.SecretKey, "session")},
		},
		"mcp": map[string]any{
			"allow_config_updates":     false,
			"allow_mutations":          true,
			"allow_raw_queries":        false,
			"allow_workflow_execution": false,
			"allow_workflow_updates":   false,
			"allow_schema_reload":      false,
			"allow_schema_updates":     false,
			"allow_dev_tools":          false,
			"legacy_discovery":         false,
		},
		"discovery_cache": map[string]any{"enabled": true, "path": ".graphjin/discovery"},
		"sources":         []any{},
	}
	out, err := yaml.Marshal(base)
	if err != nil {
		log.Fatalf("start config: %s", err)
	}
	return string(out)
}

// loadStartFlowConfig builds the running configuration for a start-config
// install: the base, with the document read from the store laid over it.
func loadStartFlowConfig(sc *startConfig) (*serv.Config, error) {
	doc, err := serv.ReadStoredConfig(context.Background(), sc.Store, sc.sealKey())
	if err != nil {
		return nil, err
	}
	merged, ignored, err := serv.MergeStoredConfig(startBaseConfig(sc), doc)
	if err != nil {
		return nil, err
	}
	if len(ignored) != 0 {
		log.Warnf("The stored configuration sets %s, which it may not; ignoring.", strings.Join(ignored, ", "))
	}
	c, err := serv.NewConfig(merged, "yaml")
	if err != nil {
		return nil, fmt.Errorf("stored configuration: %w", err)
	}
	// Relative paths, such as the discovery cache, sit beside the store.
	c.Serv.ConfigPath = filepath.Dir(sc.Store)
	c.SetStore(sc.Store, sc.sealKey())

	// The model provider's key is kept encrypted in the store. The agent
	// reads its key from an environment variable, so hand it over in one of
	// its own. The stored configuration names that variable.
	key, err := serv.ReadStoredSecret(context.Background(), sc.Store, sc.sealKey(), serv.StoredModelKey)
	if err != nil {
		return nil, fmt.Errorf("stored model key: %w", err)
	}
	if key != "" {
		if err := os.Setenv(serv.StoredModelKeyEnv, key); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// sealKey is the key the stored configuration is encrypted with. It is
// derived from the secret key, and is not the key that signs sessions.
func (sc *startConfig) sealKey() string {
	return serv.DeriveStoreSecret(sc.SecretKey, "sealing")
}

// restartInPlace replaces this process with a fresh copy of itself, which
// reads the stored configuration again. The address, the store and the
// sessions people hold all stay the same.
func restartInPlace() error {
	binary, err := os.Executable()
	if err != nil {
		return err
	}
	return syscall.Exec(binary, os.Args, os.Environ())
}

// announceSetup tells whoever started a new install how to finish setting it
// up, and on a laptop opens the page for them once the server is listening.
func announceSetup(gj *serv.HttpService, sc *startConfig) {
	link := gj.SetupLink()
	if link == "" {
		fmt.Printf("\nDeepORG is running at http://%s/\n\n", displayListen(sc.Listen))
		return
	}
	fmt.Printf("\nFinish setup in your browser:\n\n  %s\n\n", link)
	if !opensBrowser(sc) {
		return
	}
	go func() {
		// Wait for the port before opening, or the browser shows an error
		// page that the newcomer has to reload by hand.
		health := "http://" + displayListen(sc.Listen) + "/health"
		client := &http.Client{Timeout: time.Second}
		for i := 0; i < 100; i++ {
			if res, err := client.Get(health); err == nil {
				res.Body.Close() //nolint:errcheck
				_ = tryOpenBrowser(link)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()
}

// opensBrowser is true on a laptop: someone is at the terminal and the server
// listens on this machine only.
func opensBrowser(sc *startConfig) bool {
	if os.Getenv("DEEPORG_NO_BROWSER") != "" {
		return false
	}
	host, _, err := net.SplitHostPort(sc.Listen)
	if err != nil || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
		return false
	}
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func displayListen(listen string) string {
	if strings.HasPrefix(listen, "0.0.0.0:") {
		return "localhost" + listen[len("0.0.0.0"):]
	}
	return listen
}

// existingStartConfig loads the start config for commands that read or change
// a store that must already exist. It never creates one.
func existingStartConfig(cmd *cobra.Command) (*startConfig, error) {
	file, ok := usesStartConfig(cmd)
	if !ok {
		return nil, errors.New("this directory uses config files, not a start config; edit them with `deeporg config set`")
	}
	if _, err := os.Stat(file); err != nil {
		return nil, fmt.Errorf("no start config at %s; run `deeporg` once to create it", file)
	}
	return loadStartConfig(file)
}

func configImportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "import <file>",
		Short: "Load a configuration file into the store",
		Long: `Replace the configuration kept in the store with the contents of a YAML file.

The file uses the same settings as a config file, without the ones that belong
to the start config (mode, auth, host_port, artifacts, secrets). DeepORG reads
the stored configuration when it starts, so restart it afterwards.

Examples:
  deeporg config import sources.yml
  deeporg config export > backup.yml`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			sc, err := existingStartConfig(cmd)
			if err != nil {
				log.Fatalf("%s", err)
			}
			raw, err := os.ReadFile(args[0])
			if err != nil {
				log.Fatalf("%s", err)
			}
			doc := string(raw)
			if err := serv.ValidateStoredConfig(doc); err != nil {
				log.Fatalf("%s: %s", args[0], err)
			}
			// Load it the way the server will, so a document that cannot
			// start the server is refused here and not at the next restart.
			merged, _, err := serv.MergeStoredConfig(startBaseConfig(sc), doc)
			if err != nil {
				log.Fatalf("%s: %s", args[0], err)
			}
			if _, err := serv.NewConfig(merged, "yaml"); err != nil {
				log.Fatalf("%s: %s", args[0], err)
			}
			if err := serv.WriteStoredConfig(context.Background(), sc.Store, sc.sealKey(), doc); err != nil {
				log.Fatalf("%s", err)
			}
			fmt.Printf("Stored the configuration from %s. Restart DeepORG for it to take effect.\n", args[0])
		},
	}
}

func configExportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "export",
		Short: "Print the configuration kept in the store",
		Long:  "Print the configuration document kept in the store, as YAML. Redirect it to a file to keep a copy. Database passwords in it are as they were given; treat the output as a secret.",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			sc, err := existingStartConfig(cmd)
			if err != nil {
				log.Fatalf("%s", err)
			}
			doc, err := serv.ReadStoredConfig(context.Background(), sc.Store, sc.sealKey())
			if err != nil {
				log.Fatalf("%s", err)
			}
			fmt.Print(doc)
			if doc != "" && !strings.HasSuffix(doc, "\n") {
				fmt.Println()
			}
		},
	}
}
