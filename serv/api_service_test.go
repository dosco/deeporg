package serv

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jwt "github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap/zapcore"
	_ "modernc.org/sqlite"
)

const apiServiceSecret = "api-service-test-secret-0123456789"

// The API service is DeepORG with nothing agentic in it: GraphQL and REST
// over a database, limited by the rules its owner wrote for each role. While
// it is being built a query is saved under its name, and saved again when it
// changes. In production the saved queries are the only ones served, and a
// client cannot change what one of them does.
func TestAPIServiceServesSavedQueriesAndNothingAgentic(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "app.sqlite3")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id TEXT, total INTEGER)`,
		`INSERT INTO orders (id, user_id, total) VALUES (1, 'ada', 10), (2, 'bo', 20), (3, 'ada', 30)`,
		// No account or owner column: the agent server's generated rules
		// would have a view on this table. The API service has none.
		`CREATE TABLE notes (id INTEGER PRIMARY KEY, body TEXT)`,
		`INSERT INTO notes (id, body) VALUES (1, 'hello')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	db.Close() //nolint:errcheck

	boot := func(production bool) string {
		t.Helper()
		conf, err := NewConfig(fmt.Sprintf(`
mode: api
production: %t
auth:
  type: jwt
  jwt:
    secret: %s
sources:
  - name: app
    kind: database
    default: true
    type: sqlite
    path: %q
roles:
  - name: user
    tables:
      - name: orders
        query:
          filters: ["{ user_id: { eq: $user_id } }"]
`, production, apiServiceSecret, dbPath), "yaml")
		if err != nil {
			t.Fatalf("load config: %v", err)
		}
		conf.Serv.ConfigPath = dir
		hs, err := NewGraphJinService(conf, OptionSetLogOutput(zapcore.AddSync(io.Discard)))
		if err != nil {
			t.Fatalf("start: %v", err)
		}
		t.Cleanup(func() { hs.Close() }) //nolint:errcheck
		mux := http.NewServeMux()
		if err := hs.Attach(mux); err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(mux)
		t.Cleanup(server.Close)
		return server.URL
	}
	token := func(sub string) string {
		t.Helper()
		signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": sub}).SignedString([]byte(apiServiceSecret))
		if err != nil {
			t.Fatal(err)
		}
		return signed
	}
	graphql := func(base, bearer, query string) string {
		t.Helper()
		raw, _ := json.Marshal(map[string]any{"query": query})
		req, _ := http.NewRequest(http.MethodPost, base+"/api/v1/graphql", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close() //nolint:errcheck
		body, _ := io.ReadAll(res.Body)
		return string(body)
	}
	status := func(base, path string) int {
		t.Helper()
		res, err := http.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close() //nolint:errcheck
		return res.StatusCode
	}

	// While it is being built.
	dev := boot(false)

	// The role's own rule limits what it reads.
	if got := graphql(dev, token("ada"), `query myOrders { orders(order_by: { id: asc }) { id total } }`); !strings.Contains(got, `"id":1`) || !strings.Contains(got, `"id":3`) || strings.Contains(got, `"id":2`) {
		t.Fatalf("the role rule on orders was not applied: %s", got)
	}
	// A table with no rule and no account column is readable, as it always was.
	if got := graphql(dev, token("ada"), `query myNotes { notes { id body } }`); !strings.Contains(got, `"hello"`) {
		t.Fatalf("a table with no rule was not readable: %s", got)
	}
	// Nothing agentic is there: no MCP server, no agent.
	for _, path := range []string{"/api/v1/mcp", "/api/v1/agent", "/api/v1/setup"} {
		if code := status(dev, path); code != http.StatusNotFound {
			t.Errorf("GET %s answered %d in the API service, want 404", path, code)
		}
	}

	// The query was saved under its name, and saving it again with a change
	// replaces it.
	saved := filepath.Join(dir, "queries", "myOrders.gql")
	first, err := os.ReadFile(saved)
	if err != nil || !strings.Contains(string(first), "total") {
		t.Fatalf("the query was not saved while building: %v %s", err, first)
	}
	if got := graphql(dev, token("ada"), `query myOrders { orders(order_by: { id: asc }) { id } }`); strings.Contains(got, "total") || !strings.Contains(got, `"id":1`) {
		t.Fatalf("the changed query did not run: %s", got)
	}
	if second, err := os.ReadFile(saved); err != nil || strings.Contains(string(second), "total") {
		t.Fatalf("the changed query did not replace the saved one: %v %s", err, second)
	}

	// In production.
	prod := boot(true)

	// The saved query is served, and a client that sends different text
	// under its name still gets the saved one.
	if got := graphql(prod, token("ada"), `query myOrders { orders(order_by: { id: asc }) { id total user_id } }`); !strings.Contains(got, `"id":1`) || strings.Contains(got, "total") || strings.Contains(got, "user_id") || strings.Contains(got, `"id":2`) {
		t.Fatalf("a client changed what a saved query returns: %s", got)
	}
	// A query nobody saved is refused.
	if got := graphql(prod, token("ada"), `query neverSaved { notes { id body } }`); strings.Contains(got, "hello") || !strings.Contains(got, "errors") {
		t.Fatalf("an unsaved query ran in production: %s", got)
	}
	// And the saved file is as it was left.
	if after, _ := os.ReadFile(saved); strings.Contains(string(after), "user_id") || strings.Contains(string(after), "total") {
		t.Fatalf("production changed a saved query: %s", after)
	}
	for _, path := range []string{"/api/v1/mcp", "/api/v1/agent"} {
		if code := status(prod, path); code != http.StatusNotFound {
			t.Errorf("GET %s answered %d in production, want 404", path, code)
		}
	}
}

// A config file must declare its databases under sources. One that still has
// the old top-level database section is told exactly what to write.
func TestConfigFileWithoutSourcesSaysWhatToAdd(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dev.yml"), []byte(`
app_name: Old App
database:
  type: mysql
  host: db.internal
  port: 3306
  dbname: shop
  user: shop
  password: a-secret-password
`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ReadInConfig(filepath.Join(dir, "dev"))
	if err == nil {
		t.Fatal("a config file with no sources was accepted")
	}
	msg := err.Error()
	for _, want := range []string{"has no `sources:` block", "kind: database", "type: mysql", "host: db.internal", "port: 3306", "dbname: shop", "user: shop", "in place of the `database:` section"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the error does not say %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "a-secret-password") {
		t.Fatalf("the error repeats the password:\n%s", msg)
	}

	// With sources it loads, and an empty list is enough to start with none.
	if err := os.WriteFile(filepath.Join(dir, "dev.yml"), []byte("app_name: New App\nsources: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadInConfig(filepath.Join(dir, "dev")); err != nil {
		t.Fatalf("a config file with sources was refused: %v", err)
	}
}
