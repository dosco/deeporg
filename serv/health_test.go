package serv

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dosco/graphjin/core/v3"
	"go.uber.org/zap"
)

// A new install has no database until someone connects one on the setup page.
// A host that restarts a program failing its health check would otherwise
// kill the server before anyone could finish setup. An install configured
// from config files keeps answering unhealthy with no database.
func TestHealthWhileWaitingForSetup(t *testing.T) {
	logger := zap.NewNop()
	const store = "gj_store"
	openStore := func(t *testing.T) *sql.DB {
		t.Helper()
		db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "store.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() }) //nolint:errcheck
		return db
	}
	health := func(svc *graphjinService) (int, string) {
		svc.log, svc.zlog = logger.Sugar(), logger
		hs := &HttpService{}
		hs.Store(svc)
		rec := httptest.NewRecorder()
		healthCheckHandler(hs).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, healthRoute, nil))
		return rec.Code, rec.Body.String()
	}
	// storeBacked builds the service a start config gives: its configuration
	// is in the store, and the store is the only database it has open.
	storeBacked := func(t *testing.T, sources ...core.SourceConfig) *graphjinService {
		conf := &Config{Core: core.Config{Sources: sources}}
		conf.SetStore("/somewhere/store.db", "seal")
		return &graphjinService{
			conf:              conf,
			accounts:          &accountService{},
			dbs:               map[string]*sql.DB{store: openStore(t)},
			managedArtifactDB: store,
		}
	}

	t.Run("a new install waiting for its database is healthy", func(t *testing.T) {
		code, body := health(storeBacked(t))
		if code != http.StatusOK || !strings.Contains(body, "Waiting for setup") {
			t.Fatalf("health = %d %q, want 200 and a line saying setup is not finished", code, body)
		}
	})

	t.Run("a new install whose store does not answer is not", func(t *testing.T) {
		svc := storeBacked(t)
		svc.dbs[store].Close() //nolint:errcheck
		if code, body := health(svc); code != http.StatusInternalServerError {
			t.Fatalf("health = %d %q with a closed store, want 500", code, body)
		}
	})

	t.Run("a database that was connected and is gone is still unhealthy", func(t *testing.T) {
		svc := storeBacked(t, core.SourceConfig{Name: "main", Kind: "database"})
		if code, body := health(svc); code != http.StatusServiceUnavailable {
			t.Fatalf("health = %d %q with a database configured and none open, want 503", code, body)
		}
	})

	t.Run("an install from config files with no database is unhealthy", func(t *testing.T) {
		svc := &graphjinService{conf: &Config{}, dbs: map[string]*sql.DB{}}
		if code, body := health(svc); code != http.StatusServiceUnavailable || !strings.Contains(body, "no database connection") {
			t.Fatalf("health = %d %q, want 503 as before", code, body)
		}
		// The same install with a store file of its own for artifacts: the
		// store being open does not make it a new install.
		svc = &graphjinService{conf: &Config{}, dbs: map[string]*sql.DB{store: openStore(t)}, managedArtifactDB: store}
		if code, body := health(svc); code != http.StatusServiceUnavailable {
			t.Fatalf("health = %d %q with only an artifact store open, want 503 as before", code, body)
		}
	})

	t.Run("a connected database is pinged as before", func(t *testing.T) {
		svc := storeBacked(t, core.SourceConfig{Name: "main", Kind: "database"})
		svc.dbs["main"] = openStore(t)
		if code, body := health(svc); code != http.StatusOK || body != string(healthyResponse) {
			t.Fatalf("health = %d %q with a database connected, want 200 %q", code, body, healthyResponse)
		}
	})
}
