package serv

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dosco/graphjin/auth/v3"
)

const peopleTestPassword = "a long enough password"

// A server must never be left with nobody in charge: an admin cannot take
// admin rights from their own account, and nobody can take them from the
// last admin.
func TestChangingRolesNeverLeavesAServerWithoutAnAdmin(t *testing.T) {
	s := newAccountsTestService(t)
	ctx := context.Background()
	ada, err := s.accounts.create(ctx, "ada@example.com", peopleTestPassword, accountRoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	sam, err := s.accounts.create(ctx, "sam@example.com", peopleTestPassword, accountRoleUser)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.accounts.setRole(ctx, "ada@example.com", accountRoleUser, ada.ID); !errors.Is(err, errOwnAccount) {
		t.Fatalf("an admin demoted their own account: %v", err)
	}
	if err := s.accounts.remove(ctx, "Ada@Example.com", ada.ID); !errors.Is(err, errOwnAccount) {
		t.Fatalf("an admin removed their own account: %v", err)
	}
	// Someone else, such as whoever runs a server set up from config files,
	// still cannot take the last admin away.
	if _, err := s.accounts.setRole(ctx, "ada@example.com", accountRoleUser, ""); !errors.Is(err, errLastAdmin) {
		t.Fatalf("the last admin was demoted: %v", err)
	}
	if err := s.accounts.remove(ctx, "ada@example.com", ""); !errors.Is(err, errLastAdmin) {
		t.Fatalf("the last admin was removed: %v", err)
	}
	if _, err := s.accounts.setRole(ctx, "nobody@example.com", accountRoleAdmin, ada.ID); !errors.Is(err, errNoSuchAccount) {
		t.Fatalf("changing the role of nobody: %v", err)
	}

	// Sam becomes an admin. The search for an admin reads the role from the
	// row's lookup key, so it has to follow the change.
	got, err := s.accounts.setRole(ctx, "sam@example.com", accountRoleAdmin, ada.ID)
	if err != nil || got.Role != accountRoleAdmin {
		t.Fatalf("promote: %+v %v", got, err)
	}
	if n, err := s.accounts.otherAdmins(ctx, ada.ID); err != nil || n != 1 {
		t.Fatalf("other admins after the promotion = %d, %v", n, err)
	}
	row, err := s.accounts.store.getBy(ctx, accountKind, "lookup_key", accountRoleUser)
	if err != nil || row != nil {
		t.Fatalf("an account is still filed as a user after everyone became an admin: %+v %v", row, err)
	}
	// With two admins, one can be demoted, by the other.
	if _, err := s.accounts.setRole(ctx, "ada@example.com", accountRoleUser, sam.ID); err != nil {
		t.Fatalf("an admin could not demote another admin: %v", err)
	}
	if _, err := s.accounts.setRole(ctx, "sam@example.com", accountRoleUser, ada.ID); !errors.Is(err, errLastAdmin) {
		t.Fatalf("the last admin was demoted by a user: %v", err)
	}
	// The password did not change when the role did.
	if got, _ := s.accounts.verify(ctx, "ada@example.com", peopleTestPassword); got == nil || got.Role != accountRoleUser {
		t.Fatalf("after a role change the account is %+v", got)
	}
}

// Removing a person stops their session at once, and choosing a new password
// stops every session made before it.
func TestRemovingAPersonOrChangingAPasswordEndsTheirSessions(t *testing.T) {
	s := newAccountsTestService(t)
	ctx := context.Background()
	ada, err := s.accounts.create(ctx, "ada@example.com", peopleTestPassword, accountRoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	sam, err := s.accounts.create(ctx, "sam@example.com", peopleTestPassword, accountRoleUser)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := auth.NewAuthHandlerFunc(s.conf.Auth)
	if err != nil {
		t.Fatal(err)
	}
	ah := s.withLocalAccounts(plain)
	signedIn := func(token string) error {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		_, err := ah(httptest.NewRecorder(), req)
		return err
	}
	samSession, _, err := s.mintAccountSession(sam)
	if err != nil {
		t.Fatal(err)
	}
	if err := signedIn(samSession); err != nil {
		t.Fatalf("a new session was refused: %v", err)
	}

	// A password link names the account and works until it is used.
	if _, _, err := s.accounts.resetLink(ctx, "nobody@example.com"); !errors.Is(err, errNoSuchAccount) {
		t.Fatalf("a link was made for nobody: %v", err)
	}
	first, _, err := s.accounts.resetLink(ctx, "Sam@Example.com")
	if err != nil {
		t.Fatal(err)
	}
	token, expires, err := s.accounts.resetLink(ctx, "sam@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if left := time.Until(expires); left < 23*time.Hour || left > 25*time.Hour {
		t.Fatalf("a password link lasts %s", left)
	}
	if reset, err := s.accounts.pendingReset(ctx, first); err != nil || reset != nil {
		t.Fatalf("a replaced password link still works: %+v %v", reset, err)
	}
	reset, err := s.accounts.pendingReset(ctx, token)
	if err != nil || reset == nil || reset.Email != "sam@example.com" {
		t.Fatalf("the password link reads as %+v, %v", reset, err)
	}
	// An invite token is not a password link and the other way round.
	if invite, err := s.accounts.pendingInvite(ctx, token); err != nil || invite != nil {
		t.Fatalf("a password link was read as an invite: %+v %v", invite, err)
	}

	if _, err := s.accounts.setPassword(ctx, "sam@example.com", "short"); err == nil {
		t.Fatal("a short new password was accepted")
	}
	// The session was signed this second. Move the change one second on, as
	// it would be for any session older than the password.
	if _, err := s.accounts.setPassword(ctx, "sam@example.com", "a brand new password"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.accounts.change(ctx, "sam@example.com", func(account *localAccount) error {
		account.SessionsAfter = time.Now().Unix() + 1
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := signedIn(samSession); !errors.Is(err, auth.Err401) {
		t.Fatalf("a session from before the new password still works: %v", err)
	}
	if got, _ := s.accounts.verify(ctx, "sam@example.com", peopleTestPassword); got != nil {
		t.Fatal("the old password still signs in")
	}
	if got, _ := s.accounts.verify(ctx, "sam@example.com", "a brand new password"); got == nil {
		t.Fatal("the new password does not sign in")
	}

	// Removing the account ends its sessions and its password link.
	adaSession, _, err := s.mintAccountSession(ada)
	if err != nil {
		t.Fatal(err)
	}
	link, _, err := s.accounts.resetLink(ctx, "sam@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.accounts.remove(ctx, "sam@example.com", ada.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got, _ := s.accounts.verify(ctx, "sam@example.com", "a brand new password"); got != nil {
		t.Fatal("a removed account still signs in")
	}
	if reset, _ := s.accounts.pendingReset(ctx, link); reset != nil {
		t.Fatal("a removed account's password link still works")
	}
	if err := s.accounts.remove(ctx, "sam@example.com", ada.ID); !errors.Is(err, errNoSuchAccount) {
		t.Fatalf("removing someone twice: %v", err)
	}
	if err := signedIn(adaSession); err != nil {
		t.Fatalf("the admin's own session stopped working: %v", err)
	}
}

func TestIssuedBefore(t *testing.T) {
	now := time.Now().Unix()
	for name, tc := range map[string]struct {
		claims map[string]any
		after  int64
		want   bool
	}{
		"no password change":         {map[string]any{"iat": float64(now - 100)}, 0, false},
		"signed after the change":    {map[string]any{"iat": float64(now)}, now - 10, false},
		"signed in the same second":  {map[string]any{"iat": float64(now)}, now, false},
		"signed before the change":   {map[string]any{"iat": float64(now - 10)}, now, true},
		"no issue time, with change": {map[string]any{}, now, true},
		"no issue time, no change":   {map[string]any{}, 0, false},
	} {
		if got := issuedBefore(tc.claims, tc.after); got != tc.want {
			t.Errorf("%s: issuedBefore = %t, want %t", name, got, tc.want)
		}
	}
}

// Wrong guesses are counted against whoever made them. Behind a web server
// that passes requests on, that is the address the web server saw, and never
// one the client wrote itself.
func TestClientAddressBehindAWebServer(t *testing.T) {
	for name, tc := range map[string]struct {
		remote    string
		forwarded []string
		want      string
	}{
		"straight from a public address":         {"203.0.113.9:4000", nil, "addr:203.0.113.9"},
		"public address claiming to be another":  {"203.0.113.9:4000", []string{"198.51.100.1"}, "addr:203.0.113.9"},
		"from a web server on this machine":      {"127.0.0.1:4000", []string{"198.51.100.1"}, "addr:198.51.100.1"},
		"from a web server on a private network": {"10.0.0.5:4000", []string{"198.51.100.1"}, "addr:198.51.100.1"},
		"the client added an address of its own": {"127.0.0.1:4000", []string{"192.0.2.77, 198.51.100.1"}, "addr:198.51.100.1"},
		"two header lines, the last one counts":  {"127.0.0.1:4000", []string{"192.0.2.77", "198.51.100.1"}, "addr:198.51.100.1"},
		"this machine, nothing in front":         {"127.0.0.1:4000", nil, "addr:127.0.0.1"},
		"a header that is not an address":        {"127.0.0.1:4000", []string{"unknown"}, "addr:127.0.0.1"},
		"IPv6 on this machine":                   {"[::1]:4000", []string{"2001:db8::7"}, "addr:2001:db8::7"},
	} {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.RemoteAddr = tc.remote
		for _, value := range tc.forwarded {
			req.Header.Add("X-Forwarded-For", value)
		}
		if got := clientAddress(req); got != tc.want {
			t.Errorf("%s: clientAddress = %q, want %q", name, got, tc.want)
		}
	}
}

// Connecting a database again under the same name changes where it is, not
// who may read its tables.
func TestReconnectingADatabaseKeepsWhoCanReadItsTables(t *testing.T) {
	doc, err := storedConfigWithSource("", map[string]any{"name": "shop", "kind": "database", "type": "postgres", "host": "old"})
	if err != nil {
		t.Fatal(err)
	}
	if doc, err = storedConfigWithTableAccess(doc, "Shop", []string{"salaries"}, []string{"audit_log"}); err != nil {
		t.Fatal(err)
	}
	if _, err := storedConfigWithTableAccess(doc, "warehouse", nil, nil); err == nil {
		t.Fatal("a database that is not in the document was accepted")
	}
	doc, err = storedConfigWithSource(doc, map[string]any{"name": "shop", "kind": "database", "type": "postgres", "host": "new"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"host: new", "admin_tables:", "- salaries", "blocked_tables:", "- audit_log", "default: true"} {
		if !strings.Contains(doc, want) {
			t.Fatalf("after reconnecting, the document is missing %q:\n%s", want, doc)
		}
	}
	// Opening every table again removes the setting.
	if doc, err = storedConfigWithTableAccess(doc, "shop", nil, nil); err != nil || strings.Contains(doc, "access") {
		t.Fatalf("the access setting was left behind (%v):\n%s", err, doc)
	}
}
