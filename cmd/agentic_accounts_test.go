package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// A fresh agentic install has to let a person in without anyone making a
// token by hand: the first admin is created through the setup link, and from
// then on people sign in with an email and a password.
func TestFirstAdminSetupAndPasswordSignIn(t *testing.T) {
	if testing.Short() {
		t.Skip("embedded service integration")
	}
	const setupToken = "first-boot-setup-token"
	t.Setenv("DEEPORG_SETUP_TOKEN", setupToken)

	base, secret, logs := bootGeneratedAgentic(t, "")

	// Before setup the console is told a first-run step is waiting and that
	// password sign-in exists.
	page := firstBootGet(t, base+"/api/v1/console/bootstrap")
	if !strings.Contains(page, `"needs_admin":true`) || !strings.Contains(page, `"password"`) {
		t.Fatalf("the console was not told setup is waiting: %s", clip(page))
	}
	if state := firstBootGet(t, base+"/api/v1/setup"); !strings.Contains(state, `"needs_admin":true`) {
		t.Fatalf("setup state before the first admin: %s", state)
	}
	// The link is logged at a level the generated config shows, with the token.
	if out := logs.String(); !strings.Contains(out, "/setup?token="+setupToken) {
		t.Fatalf("the setup link was not logged:\n%s", clip(out))
	}

	const email, password = "Ada@Example.com", "a long enough password"

	// Without the token from the setup link nobody can claim the server.
	if status, body, _ := accountsPost(t, base+"/api/v1/setup/admin", "", map[string]any{"token": "wrong", "email": email, "password": password}); status != http.StatusForbidden {
		t.Fatalf("setup with a wrong token answered %d: %s", status, body)
	}
	// A weak password is refused with a reason.
	if status, body, _ := accountsPost(t, base+"/api/v1/setup/admin", "", map[string]any{"token": setupToken, "email": email, "password": "short"}); status != http.StatusBadRequest || !strings.Contains(body, "at least") {
		t.Fatalf("setup with a short password answered %d: %s", status, body)
	}

	status, body, cookie := accountsPost(t, base+"/api/v1/setup/admin", "", map[string]any{"token": setupToken, "email": email, "password": password})
	if status != http.StatusOK || cookie == "" || !strings.Contains(body, `"role":"admin"`) {
		t.Fatalf("creating the first admin answered %d (cookie %t): %s", status, cookie != "", body)
	}

	// Setup is done: the state says so and the link no longer works.
	if state := firstBootGet(t, base+"/api/v1/setup"); !strings.Contains(state, `"needs_admin":false`) {
		t.Fatalf("setup state after the first admin: %s", state)
	}
	if status, body, _ := accountsPost(t, base+"/api/v1/setup/admin", "", map[string]any{"token": setupToken, "email": "eve@example.com", "password": password}); status != http.StatusConflict {
		t.Fatalf("a second setup answered %d: %s", status, body)
	}

	const readConfig = `query { gj_config(id: "current") { id sources } }`

	// The session from setup is an admin session. The role comes from the
	// account store: the session token itself carries none.
	if body := accountsGraphQL(t, base, cookie, readConfig); !strings.Contains(body, `"sources"`) || strings.Contains(body, "errors") {
		t.Fatalf("the new admin could not read the configuration: %s", clip(body))
	}

	// Signing in again: a wrong password is refused, the right one works, and
	// the email is not case sensitive.
	if status, body, _ := accountsPost(t, base+"/api/v1/auth/session", "", map[string]any{"method": "password", "email": email, "password": "not the password"}); status != http.StatusUnauthorized {
		t.Fatalf("a wrong password answered %d: %s", status, body)
	}
	if status, body, _ := accountsPost(t, base+"/api/v1/auth/session", "", map[string]any{"method": "password", "email": "nobody@example.com", "password": password}); status != http.StatusUnauthorized {
		t.Fatalf("an unknown email answered %d: %s", status, body)
	}
	status, body, again := accountsPost(t, base+"/api/v1/auth/session", "", map[string]any{"method": "password", "email": "ada@example.com", "password": password})
	if status != http.StatusOK || again == "" {
		t.Fatalf("password sign-in answered %d: %s", status, body)
	}
	if body := accountsGraphQL(t, base, again, `query { orders(order_by: { id: asc }) { id total } }`); !strings.Contains(body, `"total"`) {
		t.Fatalf("the signed-in admin could not read a table: %s", clip(body))
	}

	// A token that claims to be this account but was not signed by the server
	// is refused, and one signed by hand with no account behind it stays an
	// ordinary user.
	if body := firstBootGraphQL(t, base, signFirstBootToken(t, "not the server secret at all, sorry", "x", "admin"), readConfig); !strings.Contains(body, "errors") && !strings.Contains(body, "unauthorized") {
		t.Fatalf("a token signed with another secret was accepted: %s", clip(body))
	}
	if body := firstBootGraphQL(t, base, signFirstBootToken(t, secret, "sam", ""), readConfig); !strings.Contains(body, "errors") {
		t.Fatalf("a non-admin token read the configuration: %s", clip(body))
	}

	// Guessing is limited: after a handful of failures the server stops
	// checking passwords for that email for a while.
	limited := false
	for i := 0; i < 8; i++ {
		status, _, _ := accountsPost(t, base+"/api/v1/auth/session", "", map[string]any{"method": "password", "email": "ada@example.com", "password": "guess"})
		if status == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("repeated wrong passwords were never rate limited")
	}
}

// accountsPost sends a JSON body the way the console does and returns the
// status, the body and the session cookie the server set, if any.
func accountsPost(t *testing.T, url, cookie string, body any) (int, string, string) {
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
	req.Header.Set("Accept", "application/json")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	res, err := firstBootClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer res.Body.Close() //nolint:errcheck
	out, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	session := ""
	for _, ck := range res.Cookies() {
		if ck.Value != "" {
			session = ck.Name + "=" + ck.Value
		}
	}
	return res.StatusCode, string(out), session
}

func accountsGraphQL(t *testing.T, base, cookie, query string) string {
	t.Helper()
	_, body, _ := accountsPost(t, base+"/api/v1/graphql", cookie, map[string]any{"query": query})
	return body
}
