package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dosco/graphjin/serv/v3"
	"go.uber.org/zap/zapcore"
)

// A generated agentic config has to work the first time it is started.
//
// The demos cannot prove this: their agentic configs inherit dev.yml and never
// set `production: true`, while the generated template did. That one flag
// turned on the saved-queries-only rule, which rejected the catalog search and
// the queries the service runs against its own store, so a fresh install
// answered nothing while every demo stayed green. This test boots the template
// itself.
func TestGeneratedAgenticConfigWorksOnFirstBoot(t *testing.T) {
	if testing.Short() {
		t.Skip("embedded service integration")
	}
	for _, tc := range []struct {
		name  string
		extra string
	}{
		{name: "as generated"},
		// Older generated configs carry the flag. It must make no difference.
		{name: "with the production flag set", extra: "\nproduction: true\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			appPath := filepath.Join(dir, "app.sqlite3")
			seedFirstBootDatabase(t, appPath)

			rendered, err := newTempl(map[string]any{
				"AppName":     "First Boot",
				"AppNameSlug": "first_boot",
				"DBType":      "postgres",
				"DBHost":      "db",
				"DBPort":      "5432",
				"DBUser":      "postgres",
				"DBPass":      "postgres",
				"DBName":      "first_boot",
			}).get("agentic.yml")
			if err != nil {
				t.Fatalf("render agentic.yml: %v", err)
			}
			config := string(rendered)
			if strings.Contains(config, "\nproduction:") {
				t.Fatal("the generated agentic config must not set production")
			}

			// Keep everything the template says about mode, sign-in and MCP,
			// and point its one source at a local SQLite file so the test
			// needs no database server.
			head, _, found := strings.Cut(config, "\nsources:\n")
			if !found {
				t.Fatal("the generated agentic config has no sources block")
			}
			config = head + fmt.Sprintf("\nsources:\n  - name: default\n    kind: database\n    default: true\n    type: sqlite\n    path: %q\n", appPath) + tc.extra

			conf, err := serv.NewConfig(config, "yaml")
			if err != nil {
				t.Fatalf("load generated config: %v", err)
			}
			conf.Serv.ConfigPath = dir
			secret := conf.Auth.JWT.Secret
			if len(secret) < 32 {
				t.Fatalf("the generated config needs its own signing secret, got %d characters", len(secret))
			}

			logs := &lockedBuffer{}
			hs, err := serv.NewGraphJinService(conf, serv.OptionSetLogOutput(zapcore.AddSync(logs)))
			if err != nil {
				t.Fatalf("start service: %v", err)
			}
			defer hs.Close() //nolint:errcheck

			mux := http.NewServeMux()
			if err := hs.Attach(mux); err != nil {
				t.Fatalf("attach routes: %v", err)
			}
			server := httptest.NewServer(mux)
			defer server.Close()

			admin := signFirstBootToken(t, secret, "ada", "admin")
			member := signFirstBootToken(t, secret, "sam", "")

			// Sign-in is required: no token, no access.
			anonCatalog := firstBootMCPCall(t, server.URL, "", "query_catalog", map[string]any{"search": "customers and their orders"})
			if strings.Contains(anonCatalog, `\"cards\"`) || strings.Contains(anonCatalog, `"cards"`) {
				t.Fatalf("the catalog was readable without signing in: %s", clip(anonCatalog))
			}
			anonGraph := firstBootGraphQL(t, server.URL, "", `query { gj_catalog(limit: 1) { id } }`)
			if !strings.Contains(anonGraph, "errors") {
				t.Fatalf("the catalog was readable over GraphQL without signing in: %s", clip(anonGraph))
			}
			anonData := firstBootGraphQL(t, server.URL, "", `query { orders { id total } }`)
			if !strings.Contains(anonData, "errors") {
				t.Fatalf("a table was readable without signing in: %s", clip(anonData))
			}
			// The console still has to learn how to sign in before anyone has.
			if page := firstBootGet(t, server.URL+"/api/v1/console/bootstrap"); !strings.HasPrefix(page, "200 ") || !strings.Contains(page, `"sign_in"`) {
				t.Fatalf("the console could not load its sign-in options: %s", clip(page))
			}

			// The catalog search is the first thing an MCP client calls.
			catalog := firstBootMCPCall(t, server.URL, admin, "query_catalog", map[string]any{"search": "customers and their orders"})
			if strings.Contains(catalog, "invalid query or fragment name") {
				t.Fatalf("catalog search was rejected by the saved-queries-only rule: %s", clip(catalog))
			}
			if !strings.Contains(catalog, `"cards"`) || !strings.Contains(catalog, "orders") {
				t.Fatalf("catalog search returned no cards for the seeded tables: %s", clip(catalog))
			}

			// A signed-in user who is not an admin can read a table that has
			// no account_id column. This is a general database agent.
			body := firstBootGraphQL(t, server.URL, member, `query { orders(order_by: { id: asc }) { id total } }`)
			if !strings.Contains(body, `"total"`) || strings.Contains(body, "blocked") {
				t.Fatalf("a signed-in user could not read the orders table: %s", clip(body))
			}

			// Writes stay off until the admin turns them on.
			body = firstBootGraphQL(t, server.URL, member, `mutation { orders(insert: { id: 99, customer_id: 1, total: 5 }) { id } }`)
			if !strings.Contains(body, "errors") {
				t.Fatalf("a signed-in user could write to the orders table by default: %s", clip(body))
			}

			// The background loops read the service's own store. Give them a
			// moment, then make sure none of them was turned away.
			time.Sleep(1500 * time.Millisecond)
			if out := logs.String(); strings.Contains(out, "invalid query or fragment name") {
				t.Fatalf("the service's own store queries were rejected:\n%s", clip(out))
			}
		})
	}
}

// A connection string that cannot be read must be an error, not a crash. The
// docs showed `connection_string: ${DATABASE_URL}`, which is never filled in,
// and the server panicked on it.
func TestUnreadablePostgresConnectionStringIsAnError(t *testing.T) {
	conf, err := serv.NewConfig(`
mode: agentic
auth:
  type: jwt
  jwt:
    secret: 0123456789abcdef0123456789abcdef
sources:
  - name: default
    kind: database
    default: true
    type: postgres
    connection_string: ${DATABASE_URL}
`, "yaml")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	conf.Serv.ConfigPath = t.TempDir()

	logs := &lockedBuffer{}
	hs, err := serv.NewGraphJinService(conf, serv.OptionSetLogOutput(zapcore.AddSync(logs)))
	if hs != nil {
		defer hs.Close() //nolint:errcheck
	}
	// Agentic keeps running without the source so it can be fixed; either way
	// the reason has to be stated.
	reason := logs.String()
	if err != nil {
		reason += err.Error()
	}
	if !strings.Contains(reason, "placeholder") {
		t.Fatalf("the failure does not explain the unfilled placeholder: %q", clip(reason))
	}
}

func seedFirstBootDatabase(t *testing.T, path string) {
	t.Helper()
	app, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close() //nolint:errcheck
	for _, stmt := range []string{
		`CREATE TABLE customers (id INTEGER PRIMARY KEY, name TEXT, account_id INTEGER)`,
		// No account_id here on purpose.
		`CREATE TABLE orders (id INTEGER PRIMARY KEY, customer_id INTEGER REFERENCES customers(id), total REAL)`,
		`INSERT INTO customers (id, name, account_id) VALUES (1, 'Ada', 1), (2, 'Bo', 2)`,
		`INSERT INTO orders (id, customer_id, total) VALUES (1, 1, 10), (2, 1, 25), (3, 2, 7)`,
	} {
		if _, err := app.Exec(stmt); err != nil {
			t.Fatalf("seed %q: %v", stmt, err)
		}
	}
}

// signFirstBootToken signs an HS256 token the way an install's own sign-in
// would. An empty role leaves the claim out, which is an ordinary user.
func signFirstBootToken(t *testing.T, secret, subject, role string) string {
	t.Helper()
	claims := map[string]any{"sub": subject, "exp": time.Now().Add(time.Hour).Unix()}
	if role != "" {
		claims["role"] = role
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding
	unsigned := enc.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`)) + "." + enc.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(unsigned))
	return unsigned + "." + enc.EncodeToString(mac.Sum(nil))
}

// firstBootClient gives up on a request that stalls, so a hang is a failure
// with a name and not a test timeout.
var firstBootClient = &http.Client{Timeout: 20 * time.Second}

func firstBootRequest(t *testing.T, url, token, session string, body any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	// MCP answers over an event stream. The GraphQL endpoint would treat the
	// same header as a request to hold a stream open, so it gets plain JSON.
	if strings.HasSuffix(url, "/api/v1/mcp") {
		req.Header.Set("Accept", "application/json, text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if session != "" {
		req.Header.Set("Mcp-Session-Id", session)
	}
	res, err := firstBootClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return res
}

func firstBootInitialize() map[string]any {
	return map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "first-boot-test", "version": "0"},
		},
	}
}

// firstBootMCPCall opens an MCP session and calls one tool, returning the text
// of its result.
func firstBootMCPCall(t *testing.T, base, token, tool string, args map[string]any) string {
	t.Helper()
	url := base + "/api/v1/mcp"

	res := firstBootRequest(t, url, token, "", firstBootInitialize())
	session := res.Header.Get("Mcp-Session-Id")
	io.Copy(io.Discard, res.Body) //nolint:errcheck
	res.Body.Close()              //nolint:errcheck
	if res.StatusCode != http.StatusOK {
		t.Fatalf("MCP initialize answered %d", res.StatusCode)
	}

	res = firstBootRequest(t, url, token, session, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	io.Copy(io.Discard, res.Body) //nolint:errcheck
	res.Body.Close()              //nolint:errcheck

	res = firstBootRequest(t, url, token, session, map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": map[string]any{"name": tool, "arguments": args},
	})
	defer res.Body.Close() //nolint:errcheck
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	// The reply is either plain JSON or a server-sent event carrying it.
	text := string(raw)
	for _, line := range strings.Split(text, "\n") {
		if rest, ok := strings.CutPrefix(line, "data: "); ok {
			text = rest
		}
	}
	return text
}

func firstBootGraphQL(t *testing.T, base, token, query string) string {
	t.Helper()
	res := firstBootRequest(t, base+"/api/v1/graphql", token, "", map[string]any{"query": query})
	defer res.Body.Close() //nolint:errcheck
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func firstBootGet(t *testing.T, url string) string {
	t.Helper()
	res, err := firstBootClient.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close() //nolint:errcheck
	raw, _ := io.ReadAll(res.Body)
	return fmt.Sprintf("%d %s", res.StatusCode, raw)
}

func clip(s string) string {
	if len(s) > 1500 {
		return s[:1500] + "…"
	}
	return s
}

// lockedBuffer collects service logs written from several goroutines.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
