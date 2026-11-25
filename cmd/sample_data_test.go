package main

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dosco/graphjin/serv/v3"
	"go.uber.org/zap/zapcore"
)

// The sample database is the built-in demo company's data as one SQLite file,
// with none of the demo's own state tables in it.
func TestBuildSampleDataMakesTheDemoCompanysDatabase(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the demo database")
	}
	// The demo's provisioning works on package globals; put them back.
	originalPath, originalConf, originalDB, originalOpened := cpath, conf, db, dbOpened
	defer func() { cpath, conf, db, dbOpened = originalPath, originalConf, originalDB, originalOpened }()
	inEmptyDir(t)

	dir := filepath.Join(t.TempDir(), "sample")
	path, err := buildSampleData(dir)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if path != filepath.Join(dir, sampleDataFile) {
		t.Fatalf("path = %q", path)
	}

	sample, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer sample.Close() //nolint:errcheck
	rows, err := sample.Query(`SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		t.Fatal(err)
	}
	tables := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables[name] = true
	}
	rows.Close() //nolint:errcheck
	for _, want := range []string{"accounts", "users", "subscriptions", "invoices", "support_tickets"} {
		if !tables[want] {
			t.Fatalf("the sample database has no %s table: %v", want, tables)
		}
	}
	for name := range tables {
		if strings.HasPrefix(name, "_graphjin_") {
			t.Fatalf("the sample database still holds the demo's own state table %s", name)
		}
	}
	var accounts int
	if err := sample.QueryRow(`SELECT count(*) FROM accounts`).Scan(&accounts); err != nil || accounts == 0 {
		t.Fatalf("the sample database has no accounts: %d %v", accounts, err)
	}

	// Building again into the same place reuses what is there.
	again, err := buildSampleData(dir)
	if err != nil || again != path {
		t.Fatalf("second build: %q %v", again, err)
	}
}

// The child process that builds the sample data must not see this install's
// start config or secrets, or be steered by its settings.
func TestSampleDataChildEnvironmentIsClean(t *testing.T) {
	got := sampleDataEnv([]string{
		"PATH=/usr/bin", "HOME=/home/a",
		"DEEPORG_SECRET_KEY=secret", "DEEPORG_HOME=/x", "DEEPORG_MODEL_KEY=sk-x",
		"GJ_AGENT_PROVIDER=openai", "GO_ENV=agentic",
	})
	if strings.Join(got, ",") != "PATH=/usr/bin,HOME=/home/a" {
		t.Fatalf("child environment = %v", got)
	}
}

// Choosing sample data on the database step connects it like any database.
func TestSetupConnectsSampleData(t *testing.T) {
	if testing.Short() {
		t.Skip("embedded service integration")
	}
	_, home := inEmptyDir(t)
	const setupToken = "sample-data-setup-token"
	t.Setenv("DEEPORG_SETUP_TOKEN", setupToken)

	file, _ := usesStartConfig(nil)
	sc, err := loadStartConfig(file)
	if err != nil {
		t.Fatal(err)
	}
	samplePath := filepath.Join(home, "sample.db")
	var built, applied atomic.Int32
	boot := func(withSample bool) (string, func()) {
		t.Helper()
		conf, err := loadStartFlowConfig(sc)
		if err != nil {
			t.Fatalf("load from the store: %v", err)
		}
		opts := []serv.Option{
			serv.OptionSetLogOutput(zapcore.AddSync(&lockedBuffer{})),
			serv.OptionSetConfigApplier(func() error { applied.Add(1); return nil }),
		}
		if withSample {
			// The CLI builds the demo company's database here; a small
			// seeded file stands in for it.
			opts = append(opts, serv.OptionSetSampleData(func(context.Context) (string, error) {
				built.Add(1)
				if _, err := os.Stat(samplePath); err != nil {
					seedFirstBootDatabase(t, samplePath)
				}
				return samplePath, nil
			}))
		}
		hs, err := serv.NewGraphJinService(conf, opts...)
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

	// A server that cannot build sample data does not offer it.
	base, stop := boot(false)
	_, _, cookie := accountsPost(t, base+"/api/v1/setup/admin", "", map[string]any{"token": setupToken, "email": "ada@example.com", "password": "a long enough password"})
	if cookie == "" {
		t.Fatal("no admin session")
	}
	if state := firstBootGet(t, base+"/api/v1/setup"); !strings.Contains(state, `"sample_data":false`) {
		t.Fatalf("setup state with no sample data: %s", state)
	}
	if status, body, _ := accountsPost(t, base+"/api/v1/setup/database", cookie, map[string]any{"sample": true}); status != http.StatusNotFound {
		t.Fatalf("sample data on a server without it: %d %s", status, body)
	}
	stop()

	base, stop = boot(true)
	if page := accountsGet(t, base+"/api/v1/console/bootstrap", cookie); !strings.Contains(page, `"sample_data":true`) {
		t.Fatalf("the console was not told sample data is on offer: %s", clip(page))
	}
	// Only the admin can ask for it.
	if status, body, _ := accountsPost(t, base+"/api/v1/setup/database", "", map[string]any{"sample": true}); status != http.StatusUnauthorized {
		t.Fatalf("an anonymous caller asked for sample data: %d %s", status, body)
	}
	status, body, _ := accountsPost(t, base+"/api/v1/setup/database", cookie, map[string]any{"sample": true})
	if status != http.StatusOK || !strings.Contains(body, `"name":"sample"`) || built.Load() != 1 {
		t.Fatalf("connect sample data: %d %s (built %d)", status, body, built.Load())
	}
	stop()

	// After the restart the sample company's tables answer.
	base, stop = boot(true)
	defer stop()
	if state := firstBootGet(t, base+"/api/v1/setup"); !strings.Contains(state, `"needs_database":false`) {
		t.Fatalf("setup state after sample data: %s", state)
	}
	if body := accountsGraphQL(t, base, cookie, `query { orders(order_by: { id: asc }) { id total } }`); !strings.Contains(body, `"total"`) {
		t.Fatalf("the sample data could not be read: %s", clip(body))
	}
}
