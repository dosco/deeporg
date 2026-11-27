package main

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dosco/graphjin/serv/v3"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"go.uber.org/zap/zapcore"
)

// startStorePostgres starts a Postgres server with two databases: one for
// DeepORG's own data and one with a little data to connect. It returns the
// address of each.
func startStorePostgres(t *testing.T) (storeURL, appURL string) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := postgres.Run(ctx, "postgres:15",
		postgres.WithUsername("deeporg"),
		postgres.WithPassword("store-test-password"),
		postgres.WithDatabase("store"),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { container.Terminate(context.Background()) }) //nolint:errcheck
	storeURL, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	var db *sql.DB
	deadline := time.Now().Add(60 * time.Second)
	for {
		if db, err = sql.Open("pgx", storeURL); err == nil {
			if err = db.PingContext(ctx); err == nil {
				break
			}
			db.Close() //nolint:errcheck
		}
		if time.Now().After(deadline) {
			t.Fatalf("postgres did not become ready: %v", err)
		}
		time.Sleep(300 * time.Millisecond)
	}
	if _, err := db.ExecContext(ctx, `CREATE DATABASE app`); err != nil {
		t.Fatalf("create the data database: %v", err)
	}
	db.Close() //nolint:errcheck

	appURL = strings.Replace(storeURL, "/store?", "/app?", 1)
	if appURL == storeURL {
		t.Fatalf("could not derive the data database's address from %q", storeURL)
	}
	app, err := sql.Open("pgx", appURL)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close() //nolint:errcheck
	for _, stmt := range []string{
		`CREATE TABLE customers (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE orders (id INTEGER PRIMARY KEY, customer_id INTEGER REFERENCES customers(id), total NUMERIC)`,
		`INSERT INTO customers (id, name) VALUES (1, 'Ada'), (2, 'Bo')`,
		`INSERT INTO orders (id, customer_id, total) VALUES (1, 1, 10), (2, 1, 25), (3, 2, 7)`,
	} {
		if _, err := app.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed %q: %v", stmt, err)
		}
	}
	return storeURL, appURL
}

// A server can keep its own data in Postgres. It then needs no disk: it is
// told where the store is and what its secret key is through the
// environment, writes no file, and a new process finds everything in the
// database.
func TestStoreInPostgresNeedsNoDisk(t *testing.T) {
	if testing.Short() {
		t.Skip("needs a Postgres container")
	}
	storeURL, appURL := startStorePostgres(t)
	work, home := inEmptyDir(t)
	const setupToken = "postgres-store-setup-token"
	t.Setenv("DEEPORG_SETUP_TOKEN", setupToken)
	t.Setenv(startStoreEnv, storeURL)

	file, ok := usesStartConfig(nil)
	if !ok {
		t.Fatal("an empty directory did not take the new flow")
	}
	// Without a key the server stops and says to set one. It does not make
	// one up and keep it on a disk that may not be there next time.
	_, err := loadStartConfig(file)
	if err == nil || !strings.Contains(err.Error(), startSecretKeyEnv) || !strings.Contains(err.Error(), "same key every time") {
		t.Fatalf("starting with no secret key: %v", err)
	}
	t.Setenv(startSecretKeyEnv, "postgres-store-test-secret-0123456789abcdef")
	sc, err := loadStartConfig(file)
	if err != nil {
		t.Fatal(err)
	}
	if sc.created || sc.Store != storeURL {
		t.Fatalf("the start config is %+v", sc)
	}
	if err := sc.storeWritable(); err != nil {
		t.Fatalf("a database store was checked as a folder: %v", err)
	}
	nothingOnDisk := func(when string) {
		t.Helper()
		if _, err := os.Stat(home); !os.IsNotExist(err) {
			t.Fatalf("%s: DeepORG's folder was created: %v", when, err)
		}
		if entries, err := os.ReadDir(work); err != nil || len(entries) != 0 {
			t.Fatalf("%s: the working directory was written to: %v %v", when, entries, err)
		}
	}
	nothingOnDisk("after reading the settings")

	boot := func() (base string, stop func()) {
		t.Helper()
		conf, err := loadStartFlowConfig(sc)
		if err != nil {
			t.Fatalf("load from the store: %v", err)
		}
		if conf.Serv.ConfigPath != sc.localDir() || !strings.HasPrefix(conf.Serv.ConfigPath, os.TempDir()) {
			t.Fatalf("local files would go to %q, not the temporary folder", conf.Serv.ConfigPath)
		}
		hs, err := serv.NewGraphJinService(conf, serv.OptionSetLogOutput(zapcore.AddSync(&lockedBuffer{})))
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
	reads := func(base, cookie, query, field string) bool {
		t.Helper()
		return strings.Contains(accountsGraphQL(t, base, cookie, query), field)
	}
	const ordersQuery = `query { orders(order_by: { id: asc }) { id total customer { name } } }`

	// Setup: an admin, then a database.
	base, stop := boot()
	status, body, ada := accountsPost(t, base+"/api/v1/setup/admin", "", map[string]any{"token": setupToken, "email": "ada@example.com", "password": "a long enough password"})
	if status != http.StatusOK || ada == "" {
		t.Fatalf("creating the admin answered %d: %s", status, body)
	}
	if status, body, _ := accountsPost(t, base+"/api/v1/setup/database", ada, map[string]any{"name": "app", "url": appURL}); status != http.StatusOK {
		t.Fatalf("connecting the database answered %d: %s", status, body)
	}
	stop()

	// A new process reads the store: the session still works, the data is
	// there, and a teammate joins.
	base, stop = boot()
	if !reads(base, ada, ordersQuery, `"name":"Ada"`) {
		t.Fatalf("the admin could not read the connected database: %s", clip(accountsGraphQL(t, base, ada, ordersQuery)))
	}
	_, body, _ = accountsPost(t, base+"/api/v1/setup/invites", ada, map[string]any{"email": "sam@example.com"})
	var invite struct {
		Link string `json:"link"`
	}
	connectJSON(t, body, &invite)
	status, body, sam := accountsPost(t, base+"/api/v1/setup/invite", "", map[string]any{"token": invite.Link[strings.Index(invite.Link, "token=")+len("token="):], "password": "sam's long password"})
	if status != http.StatusOK || sam == "" {
		t.Fatalf("accepting the invite answered %d: %s", status, body)
	}
	// What the server keeps through the engine, such as a task, is in the
	// same store.
	if body := accountsGraphQL(t, base, sam, `mutation { gj_task(insert: { goal: "check the store" }) { id status } }`); !strings.Contains(body, `"status":"open"`) {
		t.Fatalf("a task could not be kept in the store: %s", clip(body))
	}
	if status, body, _ := accountsPost(t, base+"/api/v1/setup/access", ada, map[string]any{"database": "app", "tables": map[string]string{"orders": "admins"}}); status != http.StatusOK {
		t.Fatalf("hiding a table answered %d: %s", status, body)
	}
	stop()

	base, stop = boot()
	if reads(base, sam, ordersQuery, `"total"`) || !reads(base, ada, ordersQuery, `"total"`) {
		t.Fatal("a table for admins only is not kept that way after a restart")
	}
	if !reads(base, sam, `query { gj_task(limit: 5) { goal } }`, `"goal":"check the store"`) {
		t.Fatal("the task was not there after a restart")
	}

	// The commands that work on a store find it from the same settings,
	// with no start config file.
	if _, err := existingStartConfig(nil); err != nil {
		t.Fatalf("a command could not find the store: %v", err)
	}
	if link, _, err := resetPasswordLink(sc, "sam@example.com"); err != nil || !strings.Contains(link, "/reset?token=") {
		t.Fatalf("reset-password against the store: %q %v", link, err)
	}
	stored, err := serv.ReadStoredConfig(context.Background(), sc.Store, sc.sealKey())
	if err != nil || !strings.Contains(stored, "admin_tables") || !strings.Contains(stored, "name: app") {
		t.Fatalf("the stored configuration reads as (%v):\n%s", err, stored)
	}
	// Writing it back is what `deeporg config import` does.
	if err := serv.WriteStoredConfig(context.Background(), sc.Store, sc.sealKey(), stored); err != nil {
		t.Fatalf("the stored configuration could not be written back: %v", err)
	}

	// Someone connects, as data, the very database the store is in. Its
	// tables hold accounts and password hashes, and stay closed to everyone.
	if status, body, _ := accountsPost(t, base+"/api/v1/setup/database", ada, map[string]any{"name": "same", "url": storeURL}); status != http.StatusOK {
		t.Fatalf("connecting the store's database as data answered %d: %s", status, body)
	}
	stop()
	base, stop = boot()
	defer stop()
	for who, cookie := range map[string]string{"the admin": ada, "the teammate": sam} {
		for _, query := range []string{
			`query { auth_sessions @schema(name: "_graphjin") { kind id data } }`,
			`query { auth_sessions { kind id data } }`,
			`query { artifacts @schema(name: "_graphjin") { id } }`,
			`query { tasks @schema(name: "_graphjin") { id goal } }`,
		} {
			body := accountsGraphQL(t, base, cookie, query)
			if !strings.Contains(body, "errors") || strings.Contains(body, "password_hash") || strings.Contains(body, `"kind"`) || strings.Contains(body, "check the store") {
				t.Fatalf("%s read the store through a data source with %s: %s", who, query, clip(body))
			}
		}
	}
	if details := accountsGet(t, base+"/api/v1/setup/details", ada); strings.Contains(details, "auth_sessions") {
		t.Fatalf("the store's tables are listed as data: %s", clip(details))
	}
	// The data that was there is still readable beside it.
	if !reads(base, ada, ordersQuery, `"name":"Ada"`) {
		t.Fatal("the connected database stopped working beside the store's own")
	}
	// They sign in again with their password, which is in the store.
	if status, _, cookie := accountsPost(t, base+"/api/v1/auth/session", "", map[string]any{"method": "password", "email": "ada@example.com", "password": "a long enough password"}); status != http.StatusOK || cookie == "" {
		t.Fatalf("signing in from the store answered %d", status)
	}
	nothingOnDisk("at the end")
}
