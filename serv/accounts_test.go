package serv

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/dosco/graphjin/auth/v3"
	"github.com/dosco/graphjin/auth/v3/issuer"
	"github.com/dosco/graphjin/core/v3"
	jwt "github.com/golang-jwt/jwt/v5"
	_ "modernc.org/sqlite"
)

const accountsTestSecret = "accounts-test-secret-0123456789abcdef"

func newAccountsTestService(t *testing.T) *graphjinService {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "store.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() }) //nolint:errcheck
	for _, stmt := range authStoreDDL("sqlite", "_graphjin") {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("create store: %v", err)
		}
	}
	conf := &Config{}
	conf.Core.Mode = modeAgentic
	conf.Core.Sources = []core.SourceConfig{{Name: "app", Kind: "database", Type: "sqlite", Default: true}}
	conf.Serv.Auth = Auth{Type: "jwt", JWT: JWTConfig{Secret: accountsTestSecret}}
	return &graphjinService{
		conf: conf,
		accounts: &accountService{
			store:    newSQLAuthStore(db, "sqlite", "_graphjin"),
			cache:    make(map[string]cachedAccount),
			failures: newSignInLimiter(),
		},
	}
}

func TestAccountsCreateAndVerify(t *testing.T) {
	s := newAccountsTestService(t)
	ctx := context.Background()

	if has, err := s.accounts.hasAdmin(ctx); err != nil || has {
		t.Fatalf("a new store reported an admin: %v %v", has, err)
	}
	if _, err := s.accounts.create(ctx, "not-an-email", "a long enough password", accountRoleAdmin); err == nil {
		t.Fatal("an invalid email was accepted")
	}
	if _, err := s.accounts.create(ctx, "ada@example.com", "short", accountRoleAdmin); err == nil {
		t.Fatal("a short password was accepted")
	}
	account, err := s.accounts.create(ctx, " Ada@Example.com ", "a long enough password", accountRoleAdmin)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if account.Email != "ada@example.com" || account.PasswordHash == "" || account.PasswordHash == "a long enough password" {
		t.Fatalf("account not stored as expected: %+v", account)
	}
	// The same person under another spelling of the email is the same account.
	if _, err := s.accounts.create(ctx, "ADA@example.com", "another long password", accountRoleUser); !errors.Is(err, errAccountExists) {
		t.Fatalf("a duplicate email was not refused: %v", err)
	}
	if has, err := s.accounts.hasAdmin(ctx); err != nil || !has {
		t.Fatalf("the admin was not found: %v %v", has, err)
	}

	if got, err := s.accounts.verify(ctx, "ada@EXAMPLE.com", "a long enough password"); err != nil || got == nil {
		t.Fatalf("the right password was refused: %v", err)
	}
	if got, _ := s.accounts.verify(ctx, "ada@example.com", "the wrong password"); got != nil {
		t.Fatal("a wrong password was accepted")
	}
	if got, _ := s.accounts.verify(ctx, "nobody@example.com", "a long enough password"); got != nil {
		t.Fatal("an unknown email was accepted")
	}

	// A password longer than bcrypt reads still has to match in full.
	long := "0123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890"
	if _, err := s.accounts.create(ctx, "bo@example.com", long, accountRoleUser); err != nil {
		t.Fatalf("create with a long password: %v", err)
	}
	if got, _ := s.accounts.verify(ctx, "bo@example.com", long[:80]+"XXXXXXXXXXX"); got != nil {
		t.Fatal("a long password matched on its first 72 bytes alone")
	}
	if got, _ := s.accounts.verify(ctx, "bo@example.com", long); got == nil {
		t.Fatal("the long password was refused")
	}
}

// The session names the account; the store decides what the account may do.
func TestLocalAccountSessionReadsRoleAndStatusFromTheStore(t *testing.T) {
	s := newAccountsTestService(t)
	ctx := context.Background()
	admin, err := s.accounts.create(ctx, "ada@example.com", "a long enough password", accountRoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	member, err := s.accounts.create(ctx, "sam@example.com", "a long enough password", accountRoleUser)
	if err != nil {
		t.Fatal(err)
	}

	plain, err := auth.NewAuthHandlerFunc(s.conf.Auth)
	if err != nil {
		t.Fatal(err)
	}
	ah := s.withLocalAccounts(plain)
	resolve := func(token string) (context.Context, error) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		got, err := ah(httptest.NewRecorder(), req)
		if err != nil || got == nil {
			return nil, err
		}
		return s.applyIdentityContext(got), nil
	}
	roles := func(c context.Context) []string {
		got, _ := c.Value(core.IdentityRolesKey).([]string)
		return got
	}

	adminToken, _, err := s.mintAccountSession(admin)
	if err != nil {
		t.Fatal(err)
	}
	got, err := resolve(adminToken)
	if err != nil {
		t.Fatalf("admin session refused: %v", err)
	}
	if r := roles(got); len(r) != 1 || r[0] != "admin" {
		t.Fatalf("admin session roles = %v", r)
	}

	// A session for an ordinary account that also claims to be an admin: the
	// claim is ignored. Only this server can sign such a token, but the store
	// is the one place a role is granted.
	iss, err := issuer.New(issuer.Config{Secret: accountsTestSecret})
	if err != nil {
		t.Fatal(err)
	}
	forged, err := iss.Mint(jwt.MapClaims{
		"sub": member.ID, "exp": time.Now().Add(time.Hour).Unix(),
		localAccountClaim: member.ID, "role": "admin", "roles": []string{"admin"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err = resolve(forged)
	if err != nil {
		t.Fatalf("member session refused: %v", err)
	}
	if r := roles(got); len(r) != 1 || r[0] != accountRoleUser {
		t.Fatalf("a role claim overrode the store: roles = %v", r)
	}

	// Disable the admin. Their session stops working as soon as this server
	// knows, not when the session expires.
	disabled := *admin
	disabled.Disabled = true
	if err := s.accounts.store.upsert(ctx, accountKind, admin.ID, &disabled, time.Time{}); err != nil {
		t.Fatal(err)
	}
	s.accounts.forget(admin.ID)
	if _, err := resolve(adminToken); !errors.Is(err, auth.Err401) {
		t.Fatalf("a disabled account's session was not refused: %v", err)
	}
	if got, _ := s.accounts.verify(ctx, "ada@example.com", "a long enough password"); got != nil {
		t.Fatal("a disabled account could still sign in")
	}

	// A session for an account that no longer exists is refused too.
	if _, err := s.accounts.store.del(ctx, accountKind, member.ID); err != nil {
		t.Fatal(err)
	}
	s.accounts.forget(member.ID)
	if _, err := resolve(forged); !errors.Is(err, auth.Err401) {
		t.Fatalf("a deleted account's session was not refused: %v", err)
	}
}

func TestSignInLimiter(t *testing.T) {
	l := newSignInLimiter()
	for i := 0; i < l.limit; i++ {
		if !l.allow("k") {
			t.Fatalf("refused after %d failures, limit is %d", i, l.limit)
		}
		l.fail("k")
	}
	if l.allow("k") {
		t.Fatal("still allowed after reaching the limit")
	}
	if !l.allow("other") {
		t.Fatal("one key's failures limited another")
	}
	l.window = time.Millisecond
	time.Sleep(5 * time.Millisecond)
	if !l.allow("k") {
		t.Fatal("failures did not expire")
	}
}
