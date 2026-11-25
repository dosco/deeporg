package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dosco/graphjin/serv/v3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// The CLI logs through a package-level logger that main sets up. Give the
// tests one before any of them starts: assigning it inside a test races with
// goroutines an earlier test left running.
func init() {
	if log == nil {
		log = zap.NewNop().Sugar()
	}
}

// inEmptyDir runs the test in a new empty directory with no GO_ENV and its
// own home, so nothing it does can touch a real install.
func inEmptyDir(t *testing.T) (work, home string) {
	t.Helper()
	work, home = t.TempDir(), filepath.Join(t.TempDir(), "home")
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(previous) }) //nolint:errcheck
	t.Setenv("GO_ENV", "")
	t.Setenv(startConfigDirEnv, home)
	for _, env := range []string{startConfigEnv, startStoreEnv, startSecretKeyEnv, startListenEnv, startLogLevelEnv} {
		t.Setenv(env, "")
	}
	return work, home
}

// Which flow `serve` takes must never change under an existing install.
func TestStartConfigIsOnlyUsedByNewInstalls(t *testing.T) {
	_, home := inEmptyDir(t)

	// Nothing here at all: the new flow, with the start config in the home.
	file, ok := usesStartConfig(nil)
	if !ok || file != filepath.Join(home, startConfigName) {
		t.Fatalf("empty directory: file=%q ok=%v", file, ok)
	}

	// GO_ENV marks an existing install, whatever its value.
	for _, env := range []string{"dev", "prod", "agentic"} {
		t.Setenv("GO_ENV", env)
		if _, ok := usesStartConfig(nil); ok {
			t.Fatalf("GO_ENV=%s took the new flow", env)
		}
	}
	t.Setenv("GO_ENV", "")

	// So does a config directory.
	if err := os.Mkdir("config", 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ok := usesStartConfig(nil); ok {
		t.Fatal("a directory with ./config took the new flow")
	}

	// A start config in the directory wins over the config directory: the
	// install has been moved to the new flow.
	if err := os.WriteFile(startConfigName, []byte("secret_key: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if file, ok := usesStartConfig(nil); !ok || file != startConfigName {
		t.Fatalf("./deeporg.yml: file=%q ok=%v", file, ok)
	}

	// And an explicit file wins over everything.
	t.Setenv("GO_ENV", "dev")
	t.Setenv(startConfigEnv, "/somewhere/else.yml")
	if file, ok := usesStartConfig(nil); !ok || file != "/somewhere/else.yml" {
		t.Fatalf("%s: file=%q ok=%v", startConfigEnv, file, ok)
	}
}

func TestStartConfigIsCreatedOnceAndThenReadBack(t *testing.T) {
	_, home := inEmptyDir(t)
	file := filepath.Join(home, startConfigName)

	first, err := loadStartConfig(file)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if !first.created || len(first.SecretKey) < 32 {
		t.Fatalf("first run did not create a start config with a secret key: %+v", first)
	}
	if first.Store != filepath.Join(home, "store.db") {
		t.Fatalf("store = %q, want it beside the start config", first.Store)
	}
	// A laptop install is reachable from this machine only.
	if first.Listen != startDefaultListen {
		t.Fatalf("listen = %q", first.Listen)
	}
	info, err := os.Stat(file)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("start config mode = %v err=%v, want 0600", info.Mode().Perm(), err)
	}

	second, err := loadStartConfig(file)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if second.created || second.SecretKey != first.SecretKey || second.Store != first.Store {
		t.Fatalf("second run did not read the first run's file: %+v", second)
	}

	// The environment overrides the file, for platforms with no files.
	t.Setenv(startListenEnv, "0.0.0.0:9999")
	t.Setenv(startStoreEnv, "other.db")
	third, err := loadStartConfig(file)
	if err != nil {
		t.Fatal(err)
	}
	if third.Listen != "0.0.0.0:9999" || third.Store != filepath.Join(home, "other.db") {
		t.Fatalf("environment did not override the file: %+v", third)
	}
}

func TestStartConfigProblemsSayWhatToDo(t *testing.T) {
	_, home := inEmptyDir(t)
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(home, startConfigName)

	// No secret key: the message carries one ready to paste.
	if err := os.WriteFile(file, []byte("store: store.db\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := loadStartConfig(file)
	if err == nil || !strings.Contains(err.Error(), "secret_key: ") || !strings.Contains(err.Error(), startSecretKeyEnv) {
		t.Fatalf("missing secret key: %v", err)
	}

	// A database server as the store is not supported yet; say so plainly.
	body := fmt.Sprintf("store: postgres://u:p@db/deeporg\nsecret_key: %s\n", newInstallSecret())
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadStartConfig(file); err == nil || !strings.Contains(err.Error(), "not supported yet") {
		t.Fatalf("postgres store: %v", err)
	}
}

// The newcomer's path: nothing on disk, start, create the admin, and later a
// database appears in the store and is there after a restart.
func TestStartConfigInstallBootsFromTheStore(t *testing.T) {
	if testing.Short() {
		t.Skip("embedded service integration")
	}
	work, home := inEmptyDir(t)
	const setupToken = "start-config-setup-token"
	t.Setenv("DEEPORG_SETUP_TOKEN", setupToken)

	file, ok := usesStartConfig(nil)
	if !ok {
		t.Fatal("an empty directory did not take the new flow")
	}
	sc, err := loadStartConfig(file)
	if err != nil {
		t.Fatal(err)
	}

	boot := func() (base string, hs *serv.HttpService, stop func()) {
		t.Helper()
		conf, err := loadStartFlowConfig(sc)
		if err != nil {
			t.Fatalf("load from the store: %v", err)
		}
		hs, err = serv.NewGraphJinService(conf, serv.OptionSetLogOutput(zapcore.AddSync(&lockedBuffer{})))
		if err != nil {
			t.Fatalf("start: %v", err)
		}
		mux := http.NewServeMux()
		if err := hs.Attach(mux); err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(mux)
		return server.URL, hs, func() { server.Close(); hs.Close() } //nolint:errcheck
	}

	// First start: no database, no admin. The server is up and waiting.
	base, hs, stop := boot()
	if link := hs.SetupLink(); !strings.Contains(link, "/setup?token="+setupToken) {
		t.Fatalf("setup link = %q", link)
	}
	const email, password = "ada@example.com", "a long enough password"
	status, body, cookie := accountsPost(t, base+"/api/v1/setup/admin", "", map[string]any{"token": setupToken, "email": email, "password": password})
	if status != http.StatusOK || cookie == "" {
		t.Fatalf("create admin: %d %s", status, body)
	}
	if hs.SetupLink() != "" {
		t.Fatal("the setup link was still offered after the admin was created")
	}
	// With no database connected the store is the only database the engine
	// has. Its tables must still be closed to GraphQL.
	assertAccountsNotReadable(t, base, map[string]string{"the admin, with no database connected": cookie})
	// With no database connected the admin still has a working console.
	if page := accountsGet(t, base+"/api/v1/console/bootstrap", cookie); !strings.Contains(page, `"role":"admin"`) || !strings.Contains(page, `"id":"admin"`) {
		t.Fatalf("the admin has no admin workspace before a database is connected: %s", clip(page))
	}
	stop()

	// Nothing was written to the directory the server was started in.
	if entries, err := os.ReadDir(work); err != nil || len(entries) != 0 {
		t.Fatalf("the working directory was written to: %v %v", entries, err)
	}

	// A database arrives in the store.
	appPath := filepath.Join(home, "app.sqlite3")
	seedFirstBootDatabase(t, appPath)
	doc := fmt.Sprintf("sources:\n  - name: shop\n    kind: database\n    default: true\n    type: sqlite\n    path: %q\n", appPath)
	if err := serv.WriteStoredConfig(context.Background(), sc.Store, sc.sealKey(), doc); err != nil {
		t.Fatalf("store the configuration: %v", err)
	}

	// Second start reads it: the same admin signs in and the data is there.
	base, hs, stop = boot()
	defer stop()
	if hs.SetupLink() != "" {
		t.Fatal("setup was offered again after a restart")
	}
	// The session from before the restart is still good: its signing key
	// comes from the start config's secret key, which did not change.
	if body := accountsGraphQL(t, base, cookie, `query { orders(order_by: { id: asc }) { id total } }`); !strings.Contains(body, `"total"`) {
		t.Fatalf("the admin's earlier session could not read the new database: %s", clip(body))
	}
	status, body, again := accountsPost(t, base+"/api/v1/auth/session", "", map[string]any{"method": "password", "email": email, "password": password})
	if status != http.StatusOK || again == "" {
		t.Fatalf("sign in after restart: %d %s", status, body)
	}
}

func accountsGet(t *testing.T, url, cookie string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "application/json")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	res, err := firstBootClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer res.Body.Close() //nolint:errcheck
	buf := new(strings.Builder)
	chunk := make([]byte, 4096)
	for {
		n, err := res.Body.Read(chunk)
		buf.Write(chunk[:n])
		if err != nil {
			break
		}
	}
	return buf.String()
}

// The database step of setup: the admin gives a connection, the server tests
// it, saves it encrypted in the store, and comes back up connected.
func TestSetupConnectsADatabaseAndSurvivesARestart(t *testing.T) {
	if testing.Short() {
		t.Skip("embedded service integration")
	}
	_, home := inEmptyDir(t)
	const setupToken = "database-step-setup-token"
	t.Setenv("DEEPORG_SETUP_TOKEN", setupToken)

	file, _ := usesStartConfig(nil)
	sc, err := loadStartConfig(file)
	if err != nil {
		t.Fatal(err)
	}
	var applied atomic.Int32
	boot := func() (string, func()) {
		t.Helper()
		conf, err := loadStartFlowConfig(sc)
		if err != nil {
			t.Fatalf("load from the store: %v", err)
		}
		hs, err := serv.NewGraphJinService(conf,
			serv.OptionSetLogOutput(zapcore.AddSync(&lockedBuffer{})),
			// The CLI restarts the process here. The test counts the request
			// and starts the service again itself.
			serv.OptionSetConfigApplier(func() error { applied.Add(1); return nil }),
		)
		if err != nil {
			t.Fatalf("start: %v", err)
		}
		mux := http.NewServeMux()
		if err := hs.Attach(mux); err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(mux)
		return server.URL, func() { server.Close(); hs.Close() } //nolint:errcheck
	}

	base, stop := boot()
	const email, password = "ada@example.com", "a long enough password"
	const dbURL = "/api/v1/setup/database"

	// Before there is an admin, nobody connects a database.
	if status, body, _ := accountsPost(t, base+dbURL, "", map[string]any{"url": "sqlite:/tmp/x.db"}); status != http.StatusUnauthorized {
		t.Fatalf("an anonymous caller reached the database step: %d %s", status, body)
	}
	_, _, cookie := accountsPost(t, base+"/api/v1/setup/admin", "", map[string]any{"token": setupToken, "email": email, "password": password})
	if cookie == "" {
		t.Fatal("no admin session")
	}
	if state := firstBootGet(t, base+"/api/v1/setup"); !strings.Contains(state, `"needs_admin":false`) || !strings.Contains(state, `"needs_database":true`) {
		t.Fatalf("setup state after the admin: %s", state)
	}
	if page := accountsGet(t, base+"/api/v1/console/bootstrap", cookie); !strings.Contains(page, `"needs_database":true`) {
		t.Fatalf("the console was not told a database is still to be connected: %s", clip(page))
	}

	// A connection that does not work is refused, with advice a newcomer can
	// act on, and nothing is saved.
	for _, tc := range []struct {
		body   map[string]any
		advice string
	}{
		{map[string]any{"url": "not a url"}, "does not look like a connection URL"},
		{map[string]any{"url": "postgres://u:p@no-such-host.invalid:5432/app"}, "could not find the host"},
		{map[string]any{"url": "postgres://u:p@127.0.0.1:1/app"}, "nothing is accepting connections"},
		{map[string]any{"type": "postgres"}, "host and the database name"},
		{map[string]any{}, "paste a connection URL"},
	} {
		status, body, _ := accountsPost(t, base+dbURL, cookie, tc.body)
		if status != http.StatusBadRequest || !strings.Contains(body, tc.advice) {
			t.Fatalf("%v answered %d %s, want advice containing %q", tc.body, status, body, tc.advice)
		}
	}
	if applied.Load() != 0 {
		t.Fatal("a failed connection was applied")
	}

	// A connection that works is saved.
	appPath := filepath.Join(home, "app.sqlite3")
	seedFirstBootDatabase(t, appPath)
	status, body, _ := accountsPost(t, base+dbURL, cookie, map[string]any{"url": "sqlite:" + appPath})
	if status != http.StatusOK || !strings.Contains(body, `"type":"sqlite"`) || !strings.Contains(body, `"restarting":true`) {
		t.Fatalf("connect database: %d %s", status, body)
	}
	deadline := time.Now().Add(5 * time.Second)
	for applied.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if applied.Load() != 1 {
		t.Fatalf("the saved connection was applied %d times, want once", applied.Load())
	}
	stop()

	// It is encrypted in the store: the path does not appear in the file.
	for _, name := range []string{"store.db", "store.db-wal"} {
		if raw, err := os.ReadFile(filepath.Join(home, name)); err == nil && strings.Contains(string(raw), "app.sqlite3") {
			t.Fatalf("%s holds the connection in the clear", name)
		}
	}

	// The server comes back up connected, for the same admin.
	base, stop = boot()
	defer stop()
	if state := firstBootGet(t, base+"/api/v1/setup"); !strings.Contains(state, `"needs_database":false`) {
		t.Fatalf("setup state after connecting: %s", state)
	}
	if body := accountsGraphQL(t, base, cookie, `query { orders(order_by: { id: asc }) { id total } }`); !strings.Contains(body, `"total"`) {
		t.Fatalf("the admin could not read the connected database: %s", clip(body))
	}
	// Someone who is signed in but is not an admin cannot change the setup.
	// (There is no way to make such an account yet, so this uses a token
	// signed with the session key directly.)
	conf, err := loadStartFlowConfig(sc)
	if err != nil {
		t.Fatal(err)
	}
	member := signFirstBootToken(t, conf.Auth.JWT.Secret, "sam", "")
	res := firstBootRequest(t, base+dbURL, member, "", map[string]any{"url": "sqlite:" + appPath})
	res.Body.Close() //nolint:errcheck
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("a non-admin reached the database step: %d", res.StatusCode)
	}
}
