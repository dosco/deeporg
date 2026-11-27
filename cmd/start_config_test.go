package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	ax "github.com/ax-llm/ax/packages/go"
	gjagent "github.com/dosco/graphjin/agent/v3"
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
	// Settings can be overridden from the environment, and other tests in
	// this package leave such variables behind (the demo sets the agent's
	// provider and key that way). A new install has none of them.
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		for _, prefix := range []string{"GJ_", "SG_", "SJ_"} {
			if strings.HasPrefix(name, prefix) {
				t.Setenv(name, "")
			}
		}
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

// asServerImage runs the test as the binary in the server image does.
func asServerImage(t *testing.T) {
	t.Helper()
	previous := serverImage
	serverImage = true
	t.Cleanup(func() { serverImage = previous })
}

// A container that listens on this machine only is one nothing can reach. The
// server image is the one build that knows it is in a container, so a new
// install there listens on every address without being told to.
func TestServerImageListensWhereAContainerCanBeReached(t *testing.T) {
	// Only the server image's own stamp makes a binary the server image.
	for stamped, want := range map[string]bool{"": false, "server": true, " Server\n": true, "env": false} {
		if got := builtAsServerImage(stamped); got != want {
			t.Fatalf("builtAsServerImage(%q) = %v, want %v", stamped, got, want)
		}
	}

	_, home := inEmptyDir(t)
	file := filepath.Join(home, startConfigName)
	asServerImage(t)

	sc, err := loadStartConfig(file)
	if err != nil {
		t.Fatalf("first run in the image: %v", err)
	}
	if sc.Listen != "0.0.0.0:8080" {
		t.Fatalf("listen = %q in the server image, want every address", sc.Listen)
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "listen: 0.0.0.0:8080") || strings.Contains(string(raw), "this machine only") {
		t.Fatalf("the start config written in the image does not say where it listens:\n%s", raw)
	}
	if opensBrowser(sc) {
		t.Fatal("the server image tried to open a browser")
	}
	if advice := sc.unreachableListenAdvice(); advice != "" {
		t.Fatalf("a reachable address was warned about: %s", advice)
	}
	// The environment still chooses the address.
	t.Setenv(startListenEnv, "0.0.0.0:9999")
	if sc, err = loadStartConfig(file); err != nil || sc.Listen != "0.0.0.0:9999" {
		t.Fatalf("%s did not override the image's address: %+v %v", startListenEnv, sc, err)
	}
	t.Setenv(startListenEnv, "")

	// A start config written by an older image holds the laptop's address.
	// It is used as it is, since someone may have meant it, and the server
	// says how to open it.
	body := strings.Replace(string(raw), "listen: 0.0.0.0:8080", "listen: "+startDefaultListen, 1)
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if sc, err = loadStartConfig(file); err != nil || sc.Listen != startDefaultListen {
		t.Fatalf("an existing start config was not used as written: %+v %v", sc, err)
	}
	advice := sc.unreachableListenAdvice()
	for _, want := range []string{startDefaultListen, "listen: 0.0.0.0:8080", file, startListenEnv} {
		if !strings.Contains(advice, want) {
			t.Fatalf("the advice does not say %q:\n%s", want, advice)
		}
	}
	// On a laptop the same address is the right one, and nothing is said.
	serverImage = false
	if advice := sc.unreachableListenAdvice(); advice != "" {
		t.Fatalf("a laptop was warned about listening on this machine only: %s", advice)
	}
}

// A volume mounted anywhere but the image user's home belongs to root, and the
// first run cannot write to it. The error names the folder and says what to
// mount, where it used to be `open /data/deeporg.yml: permission denied`.
func TestUnwritableHomeSaysWhichFolderAndWhatToMount(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can write anywhere")
	}
	inEmptyDir(t)
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	locked := filepath.Join(t.TempDir(), "data")
	if err := os.Mkdir(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o700) }) //nolint:errcheck
	t.Setenv(startConfigDirEnv, locked)
	file, ok := usesStartConfig(nil)
	if !ok || file != filepath.Join(locked, startConfigName) {
		t.Fatalf("file=%q ok=%v", file, ok)
	}
	uid := fmt.Sprintf("user %d", os.Getuid())

	// On a laptop: which folder, who has to be able to write it, and the
	// setting that picks another folder.
	_, err := loadStartConfig(file)
	if err == nil {
		t.Fatal("a start config was written to a folder that cannot be written")
	}
	for _, want := range []string{"cannot write to " + locked, "permission denied", uid, startConfigDirEnv} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the error does not say %q:\n%s", want, err)
		}
	}
	if strings.Contains(err.Error(), "Mount the volume") {
		t.Fatalf("a laptop was told to mount a volume:\n%s", err)
	}

	// In the server image: where to mount the volume.
	asServerImage(t)
	_, err = loadStartConfig(file)
	if err == nil {
		t.Fatal("a start config was written to a folder that cannot be written")
	}
	for _, want := range []string{"cannot write to " + locked, uid, "Mount the volume at " + userHome + " itself", "leave " + startConfigDirEnv + " unset"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the error in the image does not say %q:\n%s", want, err)
		}
	}
	// A folder inside one that cannot be written fails the same way.
	nested := filepath.Join(locked, "deeporg")
	if _, err = loadStartConfig(filepath.Join(nested, startConfigName)); err == nil || !strings.Contains(err.Error(), "cannot write to "+nested) {
		t.Fatalf("a home that cannot be created: %v", err)
	}

	// A start config that is there, with its store somewhere that cannot be
	// written: caught before the store is opened, and it names that folder.
	good := filepath.Join(t.TempDir(), "good")
	sc, err := loadStartConfig(filepath.Join(good, startConfigName))
	if err != nil {
		t.Fatal(err)
	}
	if err := sc.storeWritable(); err != nil {
		t.Fatalf("a writable home was refused: %v", err)
	}
	if entries, err := os.ReadDir(good); err != nil || len(entries) != 1 {
		t.Fatalf("checking the folder left something behind: %v %v", entries, err)
	}
	t.Setenv(startStoreEnv, filepath.Join(locked, "store.db"))
	if sc, err = loadStartConfig(filepath.Join(good, startConfigName)); err != nil {
		t.Fatal(err)
	}
	if err := sc.storeWritable(); err == nil || !strings.Contains(err.Error(), "cannot write to "+locked) {
		t.Fatalf("a store in a folder that cannot be written: %v", err)
	}
	// A start config that is there and cannot be read says so by name.
	unreadable := filepath.Join(good, "unreadable.yml")
	if err := os.WriteFile(unreadable, []byte("secret_key: x\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	if _, err = loadStartConfig(unreadable); err == nil || !strings.Contains(err.Error(), "cannot read "+unreadable) || !strings.Contains(err.Error(), uid) {
		t.Fatalf("a start config that cannot be read: %v", err)
	}
	// Any other failure is passed on as it is.
	other := fmt.Errorf("disk on fire")
	if got := homeNotWritableError(locked, other); got != other {
		t.Fatalf("an unrelated error was rewritten: %v", got)
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
	// It is healthy while it waits. A host that restarts a program failing
	// its health check would otherwise kill it before setup could be done.
	healthy := func(when, want string) {
		t.Helper()
		res, err := firstBootClient.Get(base + "/health")
		if err != nil {
			t.Fatalf("health %s: %v", when, err)
		}
		defer res.Body.Close() //nolint:errcheck
		body, _ := io.ReadAll(res.Body)
		if res.StatusCode != http.StatusOK || !strings.Contains(string(body), want) {
			t.Fatalf("health %s = %d %q, want 200 %q", when, res.StatusCode, body, want)
		}
	}
	healthy("before setup", "Waiting for setup")
	const email, password = "ada@example.com", "a long enough password"
	status, body, cookie := accountsPost(t, base+"/api/v1/setup/admin", "", map[string]any{"token": setupToken, "email": email, "password": password})
	if status != http.StatusOK || cookie == "" {
		t.Fatalf("create admin: %d %s", status, body)
	}
	if hs.SetupLink() != "" {
		t.Fatal("the setup link was still offered after the admin was created")
	}
	healthy("with an admin and no database", "Waiting for setup")
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
	healthy("with a database connected", "All's Well")
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
	defer func() { stop() }()
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
	// Signed in, the state says how many tables were found, which is what
	// the setup page shows. Nobody else is told.
	signedIn := accountsGet(t, base+"/api/v1/setup", cookie)
	if !regexp.MustCompile(`"tables":[1-9]`).MatchString(signedIn) || !strings.Contains(signedIn, `"boot":"`) {
		t.Fatalf("the setup state for the admin has no table count or restart marker: %s", signedIn)
	}
	if state := firstBootGet(t, base+"/api/v1/setup"); strings.Contains(state, `"tables"`) {
		t.Fatalf("someone who has not signed in was told about the tables: %s", state)
	}
	// The agent page is given questions about those tables.
	if status := accountsGet(t, base+"/api/v1/agent/status", cookie); !strings.Contains(status, `"starters":["How many `) {
		t.Fatalf("no questions were suggested from the connected tables: %s", clip(status))
	}

	// After setup the admin connects another database under a name of its
	// own. Both are there after the restart, and the first is untouched.
	secondPath := filepath.Join(home, "second.sqlite3")
	seedFirstBootDatabase(t, secondPath)
	status, body, _ = accountsPost(t, base+dbURL, cookie, map[string]any{"name": "second", "url": "sqlite:" + secondPath})
	if status != http.StatusOK || !strings.Contains(body, `"name":"second"`) || !strings.Contains(body, `"boot":"`) {
		t.Fatalf("connect a second database: %d %s", status, body)
	}
	deadline = time.Now().Add(5 * time.Second)
	for applied.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	stop()
	base, stop = boot()
	details := accountsGet(t, base+"/api/v1/setup/details", cookie)
	if !strings.Contains(details, `"name":"main"`) || !strings.Contains(details, `"name":"second"`) || !strings.Contains(details, `"store_backed":true`) {
		t.Fatalf("the setup details do not list both databases: %s", clip(details))
	}
	if strings.Contains(details, "second.sqlite3") || strings.Contains(details, "app.sqlite3") {
		t.Fatalf("the setup details give away where a database is: %s", clip(details))
	}
	if body := accountsGraphQL(t, base, cookie, `query { orders(order_by: { id: asc }) { id total } }`); !strings.Contains(body, `"total"`) {
		t.Fatalf("the first database stopped answering after a second was connected: %s", clip(body))
	}
}

// okModelClient stands in for a model provider. It accepts every request, or
// refuses every request with the error it was given.
type okModelClient struct{ err error }

func (c okModelClient) Chat(context.Context, map[string]ax.Value, map[string]ax.Value) (ax.Value, error) {
	return ax.Object("results", ax.Array()), c.err
}

func (c okModelClient) Embed(context.Context, map[string]ax.Value, map[string]ax.Value) (ax.Value, error) {
	return nil, c.err
}

func (c okModelClient) Stream(context.Context, map[string]ax.Value, map[string]ax.Value) ([]ax.Value, error) {
	return nil, c.err
}

// The model step of setup: the admin gives a provider and its key, the server
// checks the key, keeps it encrypted, and comes back up with the agent on.
func TestSetupChoosesAModelAndKeepsItsKeyEncrypted(t *testing.T) {
	if testing.Short() {
		t.Skip("embedded service integration")
	}
	_, home := inEmptyDir(t)
	const setupToken = "model-step-setup-token"
	t.Setenv("DEEPORG_SETUP_TOKEN", setupToken)
	// A provider variable already in the environment must not be what the
	// agent ends up using, nor be changed.
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv(serv.StoredModelKeyEnv, "")

	file, _ := usesStartConfig(nil)
	sc, err := loadStartConfig(file)
	if err != nil {
		t.Fatal(err)
	}
	var providerErr error
	var applied atomic.Int32
	boot := func() (string, func()) {
		t.Helper()
		conf, err := loadStartFlowConfig(sc)
		if err != nil {
			t.Fatalf("load from the store: %v", err)
		}
		hs, err := serv.NewGraphJinService(conf,
			serv.OptionSetLogOutput(zapcore.AddSync(&lockedBuffer{})),
			serv.OptionSetConfigApplier(func() error { applied.Add(1); return nil }),
			serv.OptionSetAgentClientFactory(func(gjagent.Config) (ax.AIClient, error) { return okModelClient{err: providerErr}, nil }),
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
	const modelURL = "/api/v1/setup/model"
	const key = "sk-test-0123456789abcdefghij"

	if status, body, _ := accountsPost(t, base+modelURL, "", map[string]any{"provider": "openai", "api_key": key}); status != http.StatusUnauthorized {
		t.Fatalf("an anonymous caller reached the model step: %d %s", status, body)
	}
	_, _, cookie := accountsPost(t, base+"/api/v1/setup/admin", "", map[string]any{"token": setupToken, "email": "ada@example.com", "password": "a long enough password"})
	if cookie == "" {
		t.Fatal("no admin session")
	}
	if state := firstBootGet(t, base+"/api/v1/setup"); !strings.Contains(state, `"needs_model":true`) {
		t.Fatalf("setup state before a model is chosen: %s", state)
	}

	// What the form can get wrong is said plainly.
	for _, tc := range []struct {
		body   map[string]any
		advice string
	}{
		{map[string]any{"api_key": key}, "choose a provider"},
		{map[string]any{"provider": "openai"}, "paste the provider's API key"},
		{map[string]any{"provider": "openai", "api_key": "two words"}, "does not look like an API key"},
	} {
		if status, body, _ := accountsPost(t, base+modelURL, cookie, tc.body); status != http.StatusBadRequest || !strings.Contains(body, tc.advice) {
			t.Fatalf("%v answered %d %s, want %q", tc.body, status, body, tc.advice)
		}
	}

	// A key the provider refuses is not kept, and the refusal never repeats
	// the key back, even when the provider's own error contains it.
	providerErr = fmt.Errorf("401 Unauthorized: incorrect API key provided: %s", key)
	status, body, _ := accountsPost(t, base+modelURL, cookie, map[string]any{"provider": "openai", "api_key": key})
	if status != http.StatusBadRequest || !strings.Contains(body, "did not accept that key") {
		t.Fatalf("a refused key answered %d %s", status, body)
	}
	if strings.Contains(body, key) {
		t.Fatalf("the refusal repeated the key: %s", body)
	}
	if applied.Load() != 0 {
		t.Fatal("a refused key was applied")
	}

	// A key the provider accepts is kept.
	providerErr = nil
	status, body, _ = accountsPost(t, base+modelURL, cookie, map[string]any{"provider": "anthropic", "api_key": key, "model": "some-model"})
	if status != http.StatusOK || !strings.Contains(body, `"provider":"anthropic"`) || strings.Contains(body, key) {
		t.Fatalf("choose model: %d %s", status, body)
	}
	deadline := time.Now().Add(5 * time.Second)
	for applied.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if applied.Load() != 1 {
		t.Fatalf("the model choice was applied %d times, want once", applied.Load())
	}
	stop()

	// Encrypted at rest: the key is nowhere in the store's files.
	for _, name := range []string{"store.db", "store.db-wal"} {
		if raw, err := os.ReadFile(filepath.Join(home, name)); err == nil && strings.Contains(string(raw), key) {
			t.Fatalf("%s holds the model key in the clear", name)
		}
	}
	// And it is not in the exported configuration either.
	doc, err := serv.ReadStoredConfig(context.Background(), sc.Store, sc.sealKey())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(doc, key) || !strings.Contains(doc, "provider: anthropic") || !strings.Contains(doc, "model: some-model") {
		t.Fatalf("stored configuration after the model step:\n%s", doc)
	}

	// The server comes back up with the agent ready, using the stored key.
	base, stop = boot()
	defer stop()
	if got := os.Getenv(serv.StoredModelKeyEnv); got != key {
		t.Fatalf("the stored key was not handed to the agent: %q", got)
	}
	if os.Getenv("OPENAI_API_KEY") != "" {
		t.Fatal("a provider variable in the environment was changed")
	}
	if state := firstBootGet(t, base+"/api/v1/setup"); !strings.Contains(state, `"needs_model":false`) {
		t.Fatalf("setup state after a model is chosen: %s", state)
	}
	if status := accountsGet(t, base+"/api/v1/agent/status", cookie); !strings.Contains(status, `"provider":"anthropic"`) || !strings.Contains(status, `"api_key_configured":true`) || strings.Contains(status, key) {
		t.Fatalf("agent status after the model step: %s", clip(status))
	}
}

// The banner names the product. It used to spell out the old name.
func TestBannerSaysDeepORG(t *testing.T) {
	plain := bannerText(false)
	rows := strings.Split(strings.Trim(plain, "\n"), "\n")
	if len(rows) != 6 {
		t.Fatalf("the banner has %d rows, want 6:\n%s", len(rows), plain)
	}
	width := len([]rune(rows[0]))
	for i, row := range rows {
		if len([]rune(row)) != width {
			t.Errorf("row %d is %d wide, the first is %d: the letters are out of line", i, len([]rune(row)), width)
		}
	}
	if strings.Contains(plain, "\033[") {
		t.Error("the plain banner contains color codes")
	}
	// Seven letters: D E E P, then O R G. Each one's top row starts a new
	// block, so the top row has seven runs of full blocks.
	if runs := len(strings.Fields(strings.NewReplacer("╗", " ", "╔", " ", "═", " ").Replace(rows[0]))); runs != 7 {
		t.Errorf("the banner's top row has %d letters, want 7: %q", runs, rows[0])
	}
	colored := bannerText(true)
	if !strings.Contains(colored, colorCyan) || !strings.Contains(colored, colorMagenta) {
		t.Error("the colored banner is missing its two colors")
	}
}

// Something else on the port is nobody's mistake on a laptop: the server
// moves to the next free one. An address chosen on purpose is never changed.
func TestStartMovesToAFreePortOnALaptop(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close() //nolint:errcheck
	_, port, _ := net.SplitHostPort(busy.Addr().String())
	t.Setenv(startListenEnv, "")

	sc := &startConfig{path: "/tmp/deeporg.yml", Listen: busy.Addr().String()}
	if err := sc.findListen(); err != nil {
		t.Fatalf("a laptop install did not move off a taken port: %v", err)
	}
	if sc.Listen == busy.Addr().String() || sc.portTaken != port || !listenFree(sc.Listen) {
		t.Fatalf("moved to %q (taken port %q), which is not a free port other than %s", sc.Listen, sc.portTaken, port)
	}
	// The move is remembered for the in-place restart a setup step causes.
	if got := os.Getenv(startListenEnv); got != sc.Listen {
		t.Fatalf("%s is %q after the move, want %q", startListenEnv, got, sc.Listen)
	}

	// Chosen through the environment: used as given, or not at all.
	chosen := &startConfig{path: "/tmp/deeporg.yml", Listen: busy.Addr().String(), listenChosen: true}
	if err := chosen.findListen(); err == nil || !strings.Contains(err.Error(), "Port "+port+" is already in use") || !strings.Contains(err.Error(), startListenEnv) {
		t.Fatalf("a chosen address that is taken should say what to change, got %v", err)
	}
	if chosen.Listen != busy.Addr().String() {
		t.Fatalf("a chosen address was changed to %q", chosen.Listen)
	}

	// A free port is left alone.
	free := &startConfig{path: "/tmp/deeporg.yml", Listen: sc.Listen}
	if err := free.findListen(); err != nil || free.portTaken != "" || free.Listen != sc.Listen {
		t.Fatalf("a free port was not used as it is: %q, %q, %v", free.Listen, free.portTaken, err)
	}

	// An address that is not a host and port is explained, not guessed at.
	if err := (&startConfig{path: "/tmp/deeporg.yml", Listen: "8080"}).findListen(); err == nil || !strings.Contains(err.Error(), "host and port") {
		t.Fatalf("a bad listen address gave %v", err)
	}
}

// Variables left in a shell from other work must not replace the model the
// admin chose on the setup page. Before a model is chosen they still apply.
func TestStoredModelChoiceBeatsStrayEnvironmentVariables(t *testing.T) {
	t.Setenv("GJ_AGENT_PROVIDER", "anthropic")
	t.Setenv("SG_AGENT_MODEL", "some-other-model")
	t.Setenv("GJ_AGENT_TIMEOUT_SECONDS", "90")

	// Nothing chosen yet: the environment is left alone.
	if dropped := dropAgentChoiceFromEnv("sources: []\n"); dropped != nil {
		t.Fatalf("variables were removed with no stored choice: %v", dropped)
	}
	if os.Getenv("GJ_AGENT_PROVIDER") != "anthropic" {
		t.Fatal("GJ_AGENT_PROVIDER was removed with no stored choice")
	}

	dropped := dropAgentChoiceFromEnv("agent:\n  provider: openai\n  api_key_env: DEEPORG_MODEL_KEY\n")
	if len(dropped) != 2 || dropped[0] != "GJ_AGENT_PROVIDER" || dropped[1] != "SG_AGENT_MODEL" {
		t.Fatalf("removed %v, want the provider and the model", dropped)
	}
	if _, set := os.LookupEnv("GJ_AGENT_PROVIDER"); set {
		t.Fatal("GJ_AGENT_PROVIDER is still set")
	}
	// Settings that do not decide who is called are not touched.
	if os.Getenv("GJ_AGENT_TIMEOUT_SECONDS") != "90" {
		t.Fatal("an unrelated agent setting was removed")
	}
}

// A project made by `deeporg serve new` keeps its config files in its own
// folder. Started from inside it, those files are used. Before, the default
// of ./config was not found there, so the new-install flow started instead
// and the project's own files were ignored.
func TestProjectConfigInTheCurrentFolderIsUsed(t *testing.T) {
	inEmptyDir(t)
	saved := cpath
	t.Cleanup(func() { cpath = saved })
	cpath = "./config"

	if err := os.WriteFile("dev.yml", []byte("mode: api\nsources: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if file, ok := usesStartConfig(nil); ok || cpath != "." {
		t.Fatalf("a project's own dev.yml was not used: start config %q (%t), config path %q", file, ok, cpath)
	}

	// With a ./config folder as well, that folder is the project's config,
	// as it always was.
	cpath = "./config"
	if err := os.Mkdir("config", 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ok := usesStartConfig(nil); ok || cpath != "./config" {
		t.Fatalf("./config was not preferred over files beside it: config path %q", cpath)
	}
}
