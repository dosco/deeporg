package serv

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dosco/graphjin/auth/v3"
	"github.com/dosco/graphjin/auth/v3/issuer"
	"github.com/dosco/graphjin/core/v3"
	jwt "github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Local accounts let a person sign in to an agentic server with an email and
// a password, with nothing else to set up. The first account is the admin and
// is created through a one-time setup token; the admin adds everyone else.
//
// Accounts live in the store's sign-in table, which is read with plain SQL
// and is never reachable through GraphQL. A session is a token signed by this
// server that names the account. It carries no role: the role and whether the
// account is still active are read from the store on each request, so removing
// someone takes effect at once and not when their session expires.

const (
	routeSetup      = "/api/v1/setup"
	routeSetupAdmin = "/api/v1/setup/admin"

	accountKind = "account"
	// localAccountClaim holds the account id in a session this server signed
	// for a local account.
	localAccountClaim = "gj_account"
	// agentTokenClaim marks a token given to an agent that signed in through
	// MCP, and names the client it was given to.
	agentTokenClaim = "gj_agent"

	accountRoleAdmin = "admin"
	accountRoleUser  = "user"

	// accountCacheTTL bounds how long a removed or changed account keeps
	// working on a server that did not make the change itself.
	accountCacheTTL = 15 * time.Second

	minPasswordLength = 10
	maxPasswordLength = 256 // bcrypt reads 72 bytes; refuse absurd inputs outright
	maxEmailLength    = 254

	setupTokenEnv = "DEEPORG_SETUP_TOKEN"
)

var errAccountExists = errors.New("an account with that email already exists")

type localAccount struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	PasswordHash string `json:"password_hash"`
	Role         string `json:"role"`
	Disabled     bool   `json:"disabled,omitempty"`
	CreatedAt    int64  `json:"created_at"`
	// SessionsAfter is when the password was last changed, in Unix seconds.
	// A session or agent token signed before it is refused, so choosing a
	// new password signs the account out everywhere.
	SessionsAfter int64 `json:"sessions_after,omitempty"`
}

type cachedAccount struct {
	account *localAccount // nil when the account does not exist
	at      time.Time
}

type accountService struct {
	store      *sqlAuthStore
	setupToken string

	// adminExists is sticky: once an admin account has been seen there is no
	// way back to "needs setup" through this service.
	adminExists atomic.Bool

	mu    sync.Mutex
	cache map[string]cachedAccount

	failures *signInLimiter
}

// localAccountsEnabled reports whether this server can hold accounts: agentic
// mode, sessions signed with its own secret, and a store to keep them in.
func localAccountsEnabled(conf *Config) bool {
	if conf == nil || effectiveMode(conf) != modeAgentic || !consoleSessionsEnabled(conf) {
		return false
	}
	if conf.Auth.JWT.Secret == "" || !conf.Core.Artifacts.Enabled {
		return false
	}
	switch strings.ToLower(conf.Auth.JWT.Provider) {
	case "", "generic", "other":
		return true
	}
	return false
}

// initAccounts opens the account store. It returns nil when this server does
// not hold accounts.
func (s *graphjinService) initAccounts(ctx context.Context) (*accountService, error) {
	if !localAccountsEnabled(s.conf) {
		return nil, nil
	}
	db, dbType, _, ok := s.artifactDB()
	if !ok {
		return nil, nil
	}
	schema := s.conf.Core.EffectiveArtifactsConfig().Schema
	for _, stmt := range authStoreDDL(dbType, schema) {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return nil, fmt.Errorf("create account store: %w", err)
		}
	}
	if err := migrateAuthStoreSchema(ctx, db, dbType, schema); err != nil {
		return nil, fmt.Errorf("migrate account store: %w", err)
	}
	svc := &accountService{
		store:    newSQLAuthStore(db, dbType, schema),
		cache:    make(map[string]cachedAccount),
		failures: newSignInLimiter(),
	}
	has, err := svc.hasAdmin(ctx)
	if err != nil {
		return nil, fmt.Errorf("read account store: %w", err)
	}
	if !has {
		svc.setupToken = strings.TrimSpace(os.Getenv(setupTokenEnv))
		if svc.setupToken == "" {
			if svc.setupToken, err = randomURLToken(24); err != nil {
				return nil, err
			}
		}
	}
	return svc, nil
}

func (a *accountService) setupTokenMatches(token string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.setupToken != "" && subtle.ConstantTimeCompare([]byte(strings.TrimSpace(token)), []byte(a.setupToken)) == 1
}

func (a *accountService) clearSetupToken() {
	a.mu.Lock()
	a.setupToken = ""
	a.mu.Unlock()
}

// pendingSetupToken returns the setup token while no admin exists.
func (a *accountService) pendingSetupToken() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.setupToken
}

func accountID(email string) string {
	sum := sha256.Sum256([]byte(normalizeEmail(email)))
	return hex.EncodeToString(sum[:16])
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func validAccountEmail(email string) bool {
	email = normalizeEmail(email)
	at := strings.LastIndex(email, "@")
	return len(email) <= maxEmailLength && at > 0 && at < len(email)-1 && !strings.ContainsAny(email, " \t\r\n")
}

func (a *accountService) hasAdmin(ctx context.Context) (bool, error) {
	if a.adminExists.Load() {
		return true, nil
	}
	// lookup_key holds the role, so finding an admin is one indexed read.
	row, err := a.store.getBy(ctx, accountKind, "lookup_key", accountRoleAdmin)
	if err != nil {
		return false, err
	}
	if row == nil {
		return false, nil
	}
	a.adminExists.Store(true)
	return true, nil
}

// get returns the account with this id, or nil. A short cache keeps the
// per-request lookup off the database.
func (a *accountService) get(ctx context.Context, id string) (*localAccount, error) {
	now := time.Now()
	a.mu.Lock()
	if hit, ok := a.cache[id]; ok && now.Sub(hit.at) < accountCacheTTL {
		a.mu.Unlock()
		return hit.account, nil
	}
	a.mu.Unlock()

	row, err := a.store.get(ctx, accountKind, id)
	if err != nil {
		return nil, err
	}
	var account *localAccount
	if row != nil {
		account = &localAccount{}
		if err := json.Unmarshal(row.data, account); err != nil {
			return nil, err
		}
	}
	a.mu.Lock()
	a.cache[id] = cachedAccount{account: account, at: now}
	a.mu.Unlock()
	return account, nil
}

func (a *accountService) forget(id string) {
	a.mu.Lock()
	delete(a.cache, id)
	a.mu.Unlock()
}

func (a *accountService) create(ctx context.Context, email, password, role string) (*localAccount, error) {
	if !validAccountEmail(email) {
		return nil, errors.New("enter a valid email address")
	}
	if err := checkPasswordLength(password); err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword(passwordBytes(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	account := &localAccount{
		ID:           accountID(email),
		Email:        normalizeEmail(email),
		PasswordHash: string(hash),
		Role:         role,
		CreatedAt:    time.Now().Unix(),
	}
	if existing, err := a.store.get(ctx, accountKind, account.ID); err != nil {
		return nil, err
	} else if existing != nil {
		return nil, errAccountExists
	}
	// The id is derived from the email and is the primary key, so two
	// requests for the same email cannot both insert.
	if err := a.store.insert(ctx, accountKind, account.ID, role, account, time.Time{}); err != nil {
		return nil, errAccountExists
	}
	a.forget(account.ID)
	if role == accountRoleAdmin {
		a.adminExists.Store(true)
	}
	return account, nil
}

// passwordBytes hashes a long password down so bcrypt, which reads only the
// first 72 bytes, still depends on all of it.
func passwordBytes(password string) []byte {
	if len(password) <= 72 {
		return []byte(password)
	}
	sum := sha256.Sum256([]byte(password))
	return []byte(hex.EncodeToString(sum[:]))
}

// dummyPasswordHash is compared against when no account matches, so a wrong
// email takes as long to refuse as a wrong password.
var dummyPasswordHash, _ = bcrypt.GenerateFromPassword([]byte("no such account"), bcrypt.DefaultCost)

// verify returns the active account for this email and password, or nil.
func (a *accountService) verify(ctx context.Context, email, password string) (*localAccount, error) {
	// Read past the cache: a password check must see a removed account.
	a.forget(accountID(email))
	account, err := a.get(ctx, accountID(email))
	if err != nil {
		return nil, err
	}
	if account == nil || account.Disabled || len(password) > maxPasswordLength {
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, passwordBytes(password))
		return nil, nil
	}
	if bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), passwordBytes(password)) != nil {
		return nil, nil
	}
	return account, nil
}

// mintAccountSession signs a console session that names the account and
// nothing else. The role is looked up from the store when the session is used.
func (s *graphjinService) mintAccountSession(account *localAccount) (string, time.Time, error) {
	iss, err := issuer.New(issuer.Config{
		Secret:   s.conf.Auth.JWT.Secret,
		Issuer:   s.conf.Auth.JWT.Issuer,
		Audience: s.conf.Auth.JWT.Audience,
	})
	if err != nil {
		return "", time.Time{}, err
	}
	exp := time.Now().Add(consoleSessionTTL)
	claims := localAccountClaims(account, s.conf.Core.EffectiveIdentityConfig().UserIDClaim)
	claims["exp"] = exp.Unix()
	tok, err := iss.Mint(claims)
	if err != nil {
		return "", time.Time{}, err
	}
	return tok, exp, nil
}

// localAccountClaims are what a token signed for a local account says: who it
// is, and never what it may do.
func localAccountClaims(account *localAccount, userIDClaim string) jwt.MapClaims {
	claims := jwt.MapClaims{
		"sub":             account.ID,
		"email":           account.Email,
		"name":            account.Email,
		localAccountClaim: account.ID,
	}
	if userIDClaim != "" {
		claims[userIDClaim] = account.ID
	}
	return claims
}

// withLocalAccounts resolves a session signed for a local account against the
// store. A removed or disabled account is refused, and the role comes from
// the store, never from the token.
//
// A token handed to an agent through MCP sign-in is good at the MCP endpoint
// only. Everywhere else it is refused, so what a connected agent can do is
// what the MCP tools let it do and no more.
func (s *graphjinService) withLocalAccounts(ah auth.HandlerFunc) auth.HandlerFunc {
	return s.resolveLocalAccounts(ah, false)
}

// withLocalAccountsForMCP is withLocalAccounts for the MCP endpoint, the one
// place an agent's token is accepted.
func (s *graphjinService) withLocalAccountsForMCP(ah auth.HandlerFunc) auth.HandlerFunc {
	return s.resolveLocalAccounts(ah, true)
}

func (s *graphjinService) resolveLocalAccounts(ah auth.HandlerFunc, mcpEndpoint bool) auth.HandlerFunc {
	if ah == nil || s == nil || s.accounts == nil {
		return ah
	}
	accounts := s.accounts
	return func(w http.ResponseWriter, r *http.Request) (context.Context, error) {
		ctx, err := ah(w, r)
		if err != nil || ctx == nil {
			return ctx, err
		}
		claims := auth.UserClaims(ctx)
		id, ok := claims[localAccountClaim].(string)
		if !ok || id == "" {
			return ctx, nil
		}
		if _, agent := claims[agentTokenClaim]; agent && !mcpEndpoint {
			return nil, auth.Err401
		}
		account, err := accounts.get(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("account lookup failed: %w", err)
		}
		if account == nil || account.Disabled || issuedBefore(claims, account.SessionsAfter) {
			return nil, auth.Err401
		}
		roles := []string{accountRoleUser}
		if account.Role == accountRoleAdmin {
			roles = []string{s.accountAdminRole()}
		}
		ctx = context.WithValue(ctx, core.IdentityRolesKey, roles)
		return context.WithValue(ctx, localAccountKey{}, account.ID), nil
	}
}

// localAccountKey marks a request whose role was read from the account store.
type localAccountKey struct{}

func isLocalAccountContext(ctx context.Context) bool {
	_, ok := ctx.Value(localAccountKey{}).(string)
	return ok
}

// accountAdminRole is the role name this server treats as admin.
func (s *graphjinService) accountAdminRole() string {
	for _, role := range s.conf.Core.EffectiveIdentityConfig().AdminRoles {
		if role = strings.TrimSpace(role); role != "" {
			return role
		}
	}
	return accountRoleAdmin
}

// setupNeedsAdmin reports whether the first admin still has to be created.
// A server with a demo sign-in already has a way in and needs no setup.
func (s *graphjinService) setupNeedsAdmin(ctx context.Context) bool {
	if s == nil || s.accounts == nil || s.demoSignInAvailable() {
		return false
	}
	has, err := s.accounts.hasAdmin(ctx)
	return err == nil && !has
}

// Setup serves the first-run steps. GET says which step is next; POST to
// /admin creates the first admin and signs them in.
func (s1 *HttpService) Setup(ah auth.HandlerFunc) http.Handler {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := s1.Load().(*graphjinService)
		w.Header().Set("Cache-Control", "no-store")
		if s.accounts == nil {
			writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "this server does not hold local accounts"})
			return
		}
		switch {
		case r.URL.Path == routeSetup && r.Method == http.MethodGet:
			state := map[string]any{
				"boot":           setupBootID,
				"needs_admin":    s.setupNeedsAdmin(r.Context()),
				"needs_database": s.setupNeedsDatabase(),
				"needs_model":    s.setupNeedsModel(),
				"sample_data":    s.storeBacked() && s.sampleData != nil,
			}
			// Someone signed in is also told how many tables the connected
			// databases have, so the database step can say what it found.
			if ctx, err := withConsoleSession(ah)(w, r); err == nil && auth.IsAuth(ctx) {
				state["tables"] = len(s.connectedTables())
			}
			writeJSONStatus(w, http.StatusOK, state)
		case r.URL.Path == routeSetupAdmin && r.Method == http.MethodPost:
			s.setupCreateAdmin(w, r, ah)
		case r.URL.Path == routeSetupDatabase && r.Method == http.MethodPost:
			s.setupConnectDatabase(w, r, ah)
		case r.URL.Path == routeSetupModel && r.Method == http.MethodPost:
			s.setupChooseModel(w, r, ah)
		case r.URL.Path == routeSetupDetails && r.Method == http.MethodGet:
			s.setupDetails(w, r, ah)
		case r.URL.Path == routeSetupInvites:
			s.setupInvites(w, r, ah)
		case r.URL.Path == routeSetupInvite:
			s.setupInvite(w, r)
		case r.URL.Path == routeSetupPeople:
			s.setupPeople(w, r, ah)
		case r.URL.Path == routeSetupPeopleReset:
			s.setupPeopleReset(w, r, ah)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})
	return apiV1Handler(s1, nil, h, nil)
}

func (s *graphjinService) setupCreateAdmin(w http.ResponseWriter, r *http.Request, ah auth.HandlerFunc) {
	if !consoleSameOrigin(r) {
		writeJSONStatus(w, http.StatusForbidden, map[string]any{"error": "cross-origin setup requests are refused"})
		return
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeJSONStatus(w, http.StatusUnsupportedMediaType, map[string]any{"error": "send a JSON body"})
		return
	}
	var req struct {
		Token    string `json:"token"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&req); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
		return
	}
	accounts := s.accounts
	if has, err := accounts.hasAdmin(r.Context()); err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "could not read the account store"})
		return
	} else if has {
		writeJSONStatus(w, http.StatusConflict, map[string]any{"error": "this server already has an admin; sign in instead"})
		return
	}
	who := clientAddress(r)
	if !accounts.failures.allow(who) {
		writeJSONStatus(w, http.StatusTooManyRequests, map[string]any{"error": "too many attempts; wait a minute and try again"})
		return
	}
	// The setup token is what stops the first stranger to reach a new server
	// from claiming it.
	if !accounts.setupTokenMatches(req.Token) {
		accounts.failures.fail(who)
		writeJSONStatus(w, http.StatusForbidden, map[string]any{"error": "the setup link is not valid; use the one this server printed when it started"})
		return
	}
	account, err := accounts.create(r.Context(), req.Email, req.Password, accountRoleAdmin)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errAccountExists) {
			status = http.StatusConflict
		}
		writeJSONStatus(w, status, map[string]any{"error": err.Error()})
		return
	}
	accounts.clearSetupToken()
	if s.log != nil {
		s.log.Infof("setup: admin account created for %s", account.Email)
	}
	s.finishAccountSignIn(w, r, account)
}

// finishAccountSignIn starts a console session for the account.
func (s *graphjinService) finishAccountSignIn(w http.ResponseWriter, r *http.Request, account *localAccount) {
	token, exp, err := s.mintAccountSession(account)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "could not start a session"})
		return
	}
	setConsoleSessionCookie(w, r, token, exp)
	writeJSONStatus(w, http.StatusOK, map[string]any{
		"user_id":    account.ID,
		"email":      account.Email,
		"role":       account.Role,
		"expires_at": exp.Unix(),
	})
}

// passwordSignIn handles {"method": "password"} on the session endpoint.
func (s *graphjinService) passwordSignIn(w http.ResponseWriter, r *http.Request, email, password string) {
	accounts := s.accounts
	if accounts == nil {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "password sign-in is not enabled on this server"})
		return
	}
	// Limit by address and by email, so neither one address trying many
	// accounts nor many addresses trying one account gets unlimited guesses.
	who, target := clientAddress(r), "email:"+normalizeEmail(email)
	if !accounts.failures.allow(who) || !accounts.failures.allow(target) {
		writeJSONStatus(w, http.StatusTooManyRequests, map[string]any{"error": "too many attempts; wait a minute and try again"})
		return
	}
	account, err := accounts.verify(r.Context(), email, password)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "could not read the account store"})
		return
	}
	if account == nil {
		accounts.failures.fail(who)
		accounts.failures.fail(target)
		writeJSONStatus(w, http.StatusUnauthorized, map[string]any{"error": "that email and password do not match an account"})
		return
	}
	s.finishAccountSignIn(w, r, account)
}

// clientAddress is the address attempts are counted against.
//
// Behind a web server that passes requests on, every request arrives from
// that web server, and counting by its address would let one person's wrong
// guesses lock everyone out. So when the request comes from this machine or
// a private network, the last address in X-Forwarded-For is used: the one
// the web server itself saw and wrote. Anything earlier in that header came
// from the client and is never believed. A request that arrives straight
// from a public address is counted by that address, whatever its headers say.
func clientAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "addr:" + r.RemoteAddr
	}
	if peer := net.ParseIP(host); peer != nil && (peer.IsLoopback() || peer.IsPrivate() || peer.IsLinkLocalUnicast()) {
		forwarded := r.Header.Values("X-Forwarded-For")
		if len(forwarded) != 0 {
			hops := strings.Split(forwarded[len(forwarded)-1], ",")
			if last := net.ParseIP(strings.TrimSpace(hops[len(hops)-1])); last != nil {
				return "addr:" + last.String()
			}
		}
	}
	return "addr:" + host
}

// signInLimiter counts recent failed attempts per key. It is per process:
// enough to stop guessing against one server, and a second server only
// multiplies the allowance by the number of servers.
type signInLimiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time
	limit    int
	window   time.Duration
}

func newSignInLimiter() *signInLimiter {
	return &signInLimiter{failures: make(map[string][]time.Time), limit: 5, window: time.Minute}
}

func (l *signInLimiter) recent(key string, now time.Time) []time.Time {
	kept := l.failures[key][:0]
	for _, at := range l.failures[key] {
		if now.Sub(at) < l.window {
			kept = append(kept, at)
		}
	}
	if len(kept) == 0 {
		delete(l.failures, key)
		return nil
	}
	l.failures[key] = kept
	return kept
}

func (l *signInLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.recent(key, time.Now())) < l.limit
}

func (l *signInLimiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	// Keep the map from growing without bound under a spray of keys.
	if len(l.failures) > 10000 {
		for k := range l.failures {
			l.recent(k, now)
		}
	}
	l.failures[key] = append(l.recent(key, now), now)
}
