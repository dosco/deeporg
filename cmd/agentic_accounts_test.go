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

	// Accounts and their password hashes are in the store. No role reads the
	// store's tables through GraphQL: not the admin, not anyone else.
	assertAccountsNotReadable(t, base, map[string]string{"the admin": cookie})
	member := signFirstBootToken(t, secret, "sam", "")
	if body := firstBootGraphQL(t, base, member, storeAccountsQuery); strings.Contains(body, "password_hash") || strings.Contains(body, `"kind"`) {
		t.Fatalf("a signed-in user read the account table: %s", clip(body))
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

// storeAccountsQuery asks for the store's keyed table by the name it has on
// SQLite. It holds the accounts and their password hashes.
const storeAccountsQuery = `query { _graphjin_auth_sessions { kind id data } }`

// assertAccountsNotReadable fails if any of the sessions can read the account
// table through GraphQL.
func assertAccountsNotReadable(t *testing.T, base string, sessions map[string]string) {
	t.Helper()
	for who, cookie := range sessions {
		// A blocked table answers with an error or with null, depending on
		// whether another database is connected. Either way no row comes back.
		body := accountsGraphQL(t, base, cookie, storeAccountsQuery)
		if strings.Contains(body, "password_hash") || strings.Contains(body, `"kind"`) || strings.Contains(body, "account") {
			t.Fatalf("%s read the account table through GraphQL: %s", who, clip(body))
		}
	}
}

// After setup the admin brings teammates in by invite. There is no way to
// sign up without one, a link works once, and a teammate is an ordinary user:
// they read the data and cannot change the server's setup.
func TestAdminInvitesATeammate(t *testing.T) {
	if testing.Short() {
		t.Skip("embedded service integration")
	}
	const setupToken = "invite-setup-token"
	t.Setenv("DEEPORG_SETUP_TOKEN", setupToken)
	base, _, _ := bootGeneratedAgentic(t, "")

	status, body, admin := accountsPost(t, base+"/api/v1/setup/admin", "", map[string]any{"token": setupToken, "email": "ada@example.com", "password": "a long enough password"})
	if status != http.StatusOK || admin == "" {
		t.Fatalf("creating the admin answered %d: %s", status, body)
	}
	invites := base + "/api/v1/setup/invites"

	// Only an admin invites.
	if status, body, _ := accountsPost(t, invites, "", map[string]any{"email": "sam@example.com"}); status != http.StatusUnauthorized {
		t.Fatalf("inviting without signing in answered %d: %s", status, body)
	}
	// Someone who already has an account is not invited again.
	if status, body, _ := accountsPost(t, invites, admin, map[string]any{"email": "Ada@Example.com"}); status != http.StatusConflict {
		t.Fatalf("inviting an existing account answered %d: %s", status, body)
	}
	if status, body, _ := accountsPost(t, invites, admin, map[string]any{"email": "not an email"}); status != http.StatusBadRequest {
		t.Fatalf("inviting a bad address answered %d: %s", status, body)
	}

	invite := func() string {
		t.Helper()
		status, body, _ := accountsPost(t, invites, admin, map[string]any{"email": "Sam@Example.com"})
		var out struct {
			Email string `json:"email"`
			Link  string `json:"link"`
		}
		connectJSON(t, body, &out)
		prefix := base + "/invite?token="
		if status != http.StatusOK || out.Email != "sam@example.com" || !strings.HasPrefix(out.Link, prefix) {
			t.Fatalf("inviting answered %d: %s", status, body)
		}
		return strings.TrimPrefix(out.Link, prefix)
	}
	first := invite()
	// A second invite for the same person replaces the first, so a link sent
	// to the wrong place can be cancelled by sending a new one.
	token := invite()
	if got := accountsGet(t, base+"/api/v1/setup/invite?token="+first, ""); !strings.Contains(got, "not valid any more") {
		t.Fatalf("a replaced invite link still works: %s", got)
	}

	// The admin sees who has an account and who has been invited.
	details := accountsGet(t, base+"/api/v1/setup/details", admin)
	for _, want := range []string{`"email":"ada@example.com"`, `"role":"admin"`, `"invites":[{"email":"sam@example.com"`, `"databases":[{"name":"default"`} {
		if !strings.Contains(details, want) {
			t.Fatalf("the details are missing %s: %s", want, clip(details))
		}
	}
	if strings.Contains(details, "password_hash") || strings.Contains(details, token) {
		t.Fatalf("the details give away a secret: %s", clip(details))
	}

	// The teammate opens the link, is told who it is for, and chooses a password.
	if got := accountsGet(t, base+"/api/v1/setup/invite?token="+token, ""); !strings.Contains(got, `"email":"sam@example.com"`) {
		t.Fatalf("the invite did not say who it is for: %s", got)
	}
	accept := base + "/api/v1/setup/invite"
	if status, body, _ := accountsPost(t, accept, "", map[string]any{"token": token, "password": "short"}); status != http.StatusBadRequest || !strings.Contains(body, "at least") {
		t.Fatalf("a short password answered %d: %s", status, body)
	}
	status, body, sam := accountsPost(t, accept, "", map[string]any{"token": token, "password": "another long password"})
	if status != http.StatusOK || sam == "" || !strings.Contains(body, `"role":"user"`) {
		t.Fatalf("accepting the invite answered %d: %s", status, body)
	}
	// The link is spent.
	if status, body, _ := accountsPost(t, accept, "", map[string]any{"token": token, "password": "yet another long password"}); status != http.StatusNotFound {
		t.Fatalf("an invite was accepted twice (%d): %s", status, body)
	}

	// The teammate reads the data, as everyone signed in does.
	if body := accountsGraphQL(t, base, sam, `query { orders(order_by: { id: asc }) { id total } }`); !strings.Contains(body, `"total"`) {
		t.Fatalf("the teammate could not read a table: %s", clip(body))
	}
	// They are not an admin: no invites, no details, no configuration.
	if status, body, _ := accountsPost(t, invites, sam, map[string]any{"email": "eve@example.com"}); status != http.StatusForbidden {
		t.Fatalf("a teammate invited someone (%d): %s", status, body)
	}
	if got := accountsGet(t, base+"/api/v1/setup/details", sam); strings.Contains(got, "people") {
		t.Fatalf("a teammate read the setup details: %s", clip(got))
	}
	if body := accountsGraphQL(t, base, sam, `query { gj_config(id: "current") { id } }`); !strings.Contains(body, "errors") {
		t.Fatalf("a teammate read the configuration: %s", clip(body))
	}
	if page := accountsGet(t, base+"/api/v1/console/bootstrap", sam); strings.Contains(page, `"manage"`) {
		t.Fatalf("a teammate's console was offered the admin's setup page: %s", clip(page))
	}
	if page := accountsGet(t, base+"/api/v1/console/bootstrap", admin); !strings.Contains(page, `"manage"`) {
		t.Fatalf("the admin's console was not offered the setup page: %s", clip(page))
	}
	// They sign in again with their password.
	if status, body, again := accountsPost(t, base+"/api/v1/auth/session", "", map[string]any{"method": "password", "email": "sam@example.com", "password": "another long password"}); status != http.StatusOK || again == "" {
		t.Fatalf("the teammate could not sign in again (%d): %s", status, body)
	}
	// Invites are in the store with everything else, and as unreadable.
	assertAccountsNotReadable(t, base, map[string]string{"the admin": admin, "the teammate": sam})
}
