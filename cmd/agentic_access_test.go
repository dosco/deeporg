package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dosco/graphjin/serv/v3"
	"go.uber.org/zap/zapcore"
)

// On a server set up in the browser everyone who signs in reads every table.
// An admin narrows that one table at a time: for everyone, for admins only,
// or for nobody. The choice is kept in the store and holds after a restart.
func TestAdminChoosesWhoCanReadEachTable(t *testing.T) {
	if testing.Short() {
		t.Skip("embedded service integration")
	}
	_, home := inEmptyDir(t)
	const setupToken = "access-setup-token"
	t.Setenv("DEEPORG_SETUP_TOKEN", setupToken)

	file, ok := usesStartConfig(nil)
	if !ok {
		t.Fatal("an empty directory did not take the new flow")
	}
	sc, err := loadStartConfig(file)
	if err != nil {
		t.Fatal(err)
	}
	appPath := filepath.Join(home, "app.sqlite3")
	seedFirstBootDatabase(t, appPath)
	doc := fmt.Sprintf("sources:\n  - name: shop\n    kind: database\n    default: true\n    type: sqlite\n    path: %q\n", appPath)
	if err := serv.WriteStoredConfig(context.Background(), sc.Store, sc.sealKey(), doc); err != nil {
		t.Fatalf("store the configuration: %v", err)
	}

	boot := func() (base string, stop func()) {
		t.Helper()
		conf, err := loadStartFlowConfig(sc)
		if err != nil {
			t.Fatalf("load from the store: %v", err)
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

	base, stop := boot()
	status, body, ada := accountsPost(t, base+"/api/v1/setup/admin", "", map[string]any{"token": setupToken, "email": "ada@example.com", "password": "a long enough password"})
	if status != http.StatusOK || ada == "" {
		t.Fatalf("creating the admin answered %d: %s", status, body)
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

	const ordersQuery = `query { orders(order_by: { id: asc }) { id total } }`
	const customersQuery = `query { customers(order_by: { id: asc }) { id name } }`
	reads := func(base, cookie, query, field string) bool {
		t.Helper()
		return strings.Contains(accountsGraphQL(t, base, cookie, query), field)
	}
	// To begin with everyone reads both tables, and the admin's page says so.
	if !reads(base, sam, ordersQuery, `"total"`) || !reads(base, sam, customersQuery, `"name"`) {
		t.Fatal("a teammate could not read the tables before any were hidden")
	}
	details := accountsGet(t, base+"/api/v1/setup/details", ada)
	if !strings.Contains(details, `"table_list":[{"access":"everyone","name":"customers"},{"access":"everyone","name":"orders"}]`) {
		t.Fatalf("the tables are not listed as open to everyone: %s", clip(details))
	}

	// The address changes with each restart, so it is worked out when used.
	const accessPath = "/api/v1/setup/access"
	access := base + accessPath
	// Only an admin changes this, and only for tables that exist.
	if status, body, _ := accountsPost(t, access, sam, map[string]any{"database": "shop", "tables": map[string]string{"customers": "nobody"}}); status != http.StatusForbidden {
		t.Fatalf("a teammate changed who can read a table (%d): %s", status, body)
	}
	for name, tc := range map[string]struct {
		body map[string]any
		want int
	}{
		"a database that is not connected": {map[string]any{"database": "warehouse", "tables": map[string]string{"orders": "admins"}}, http.StatusNotFound},
		"a table that does not exist":      {map[string]any{"database": "shop", "tables": map[string]string{"invoices": "admins"}}, http.StatusBadRequest},
		"a choice that does not exist":     {map[string]any{"database": "shop", "tables": map[string]string{"orders": "managers"}}, http.StatusBadRequest},
	} {
		if status, body, _ := accountsPost(t, access, ada, tc.body); status != tc.want {
			t.Fatalf("%s answered %d, want %d: %s", name, status, tc.want, body)
		}
	}

	// Orders become admins only. The change is saved and holds after a restart.
	if status, body, _ := accountsPost(t, access, ada, map[string]any{"database": "Shop", "tables": map[string]string{"orders": "admins"}}); status != http.StatusOK {
		t.Fatalf("hiding a table answered %d: %s", status, body)
	}
	stop()
	base, stop = boot()
	if reads(base, sam, ordersQuery, `"total"`) {
		t.Fatal("a teammate still reads a table that is for admins only")
	}
	if !reads(base, ada, ordersQuery, `"total"`) {
		t.Fatal("the admin cannot read a table that is for admins only")
	}
	if !reads(base, sam, customersQuery, `"name"`) {
		t.Fatal("hiding one table hid another")
	}
	details = accountsGet(t, base+"/api/v1/setup/details", ada)
	if !strings.Contains(details, `{"access":"everyone","name":"customers"},{"access":"admins","name":"orders"}`) {
		t.Fatalf("the admin's page does not show the change: %s", clip(details))
	}
	// A hidden table is still counted and listed for the admin, so it can be
	// opened again.
	if !strings.Contains(details, `"tables":2`) {
		t.Fatalf("a hidden table is no longer counted: %s", clip(details))
	}

	// For nobody means the admin too.
	if status, body, _ := accountsPost(t, base+accessPath, ada, map[string]any{"database": "shop", "tables": map[string]string{"orders": "nobody"}}); status != http.StatusOK {
		t.Fatalf("closing a table answered %d: %s", status, body)
	}
	stop()
	base, stop = boot()
	if reads(base, ada, ordersQuery, `"total"`) || reads(base, sam, ordersQuery, `"total"`) {
		t.Fatal("a table that is for nobody can still be read")
	}
	if details := accountsGet(t, base+"/api/v1/setup/details", ada); !strings.Contains(details, `{"access":"nobody","name":"orders"}`) {
		t.Fatalf("the admin's page does not show a closed table: %s", clip(details))
	}

	// Leaving every table out opens them all again, and nothing about access
	// is left in the stored configuration.
	if status, body, _ := accountsPost(t, base+accessPath, ada, map[string]any{"database": "shop", "tables": map[string]string{}}); status != http.StatusOK {
		t.Fatalf("opening the tables again answered %d: %s", status, body)
	}
	stop()
	base, stop = boot()
	defer stop()
	if !reads(base, sam, ordersQuery, `"total"`) {
		t.Fatal("a teammate cannot read a table that was opened again")
	}
	stored, err := serv.ReadStoredConfig(context.Background(), sc.Store, sc.sealKey())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "access") || strings.Contains(stored, "admin_tables") || strings.Contains(stored, "blocked_tables") {
		t.Fatalf("an access setting was left behind:\n%s", stored)
	}
}
