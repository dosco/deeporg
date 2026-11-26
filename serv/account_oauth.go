package serv

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dosco/graphjin/auth/v3"
	"github.com/dosco/graphjin/auth/v3/issuer"
	"github.com/dosco/graphjin/auth/v3/oidc"
)

// Signing an MCP client in with a local account.
//
// An agent such as Claude Code is given this server's address and nothing
// else. The server refuses it and says where to sign in; the client opens a
// browser there; the person, signed in to the console with their own account,
// is shown who is asking and allows it. The client is handed a short-lived
// token that names the account, and a longer-lived one to renew it with. No
// secret is ever copied by hand.
//
// The token says who the client is acting for and never what that person may
// do: the role is read from the account store on each request, so removing an
// account stops every client connected through it at once.
//
// This reuses the authorization-code and PKCE code written for single
// sign-on. The only difference is who vouches for the person: there, an
// outside identity provider; here, a console session for a local account.

const (
	routeMCPOAuthConsent = "/api/v1/oauth/consent"

	// consoleConnectPath is the console page that asks the person to allow
	// the client.
	consoleConnectPath = "/connect"

	accountIdentityIssuer = "local"

	// A client that registers and is never allowed by anyone is forgotten
	// after this long, so strangers cannot fill the store with registrations.
	unusedOAuthClientTTL = 7 * 24 * time.Hour
)

// initAccountOAuth builds the sign-in service for MCP clients on a server
// with local accounts. It returns nil when MCP sign-in is configured some
// other way, when there is no console to approve in, or when the server is
// set up to serve people who have not signed in.
func (s *graphjinService) initAccountOAuth() (*authLoginService, error) {
	conf := s.conf
	if s.accounts == nil || conf.MCP.OAuth.Enabled || conf.mcpDisabled() || conf.MCP.Only || !conf.WebUI || s.demoSignInAvailable() {
		return nil, nil
	}
	// A server that defines what people who have not signed in may do means
	// to serve them, so its MCP endpoint is left open to them.
	for _, role := range conf.Roles {
		if role.Name == "anon" {
			return nil, nil
		}
	}
	iss, err := issuer.New(issuer.Config{Secret: conf.Auth.JWT.Secret, Issuer: conf.Auth.JWT.Issuer})
	if err != nil {
		return nil, err
	}
	return &authLoginService{
		mcpOAuth: MCPOAuthConfig{
			Enabled:                   true,
			Mode:                      "builtin",
			DynamicClientRegistration: true,
		},
		issuer:      iss,
		store:       s.accounts.store,
		accounts:    s.accounts,
		userIDClaim: conf.Core.EffectiveIdentityConfig().UserIDClaim,
		jwtAudience: conf.Auth.JWT.Audience,
		registered:  &signInLimiter{failures: make(map[string][]time.Time), limit: 20, window: time.Minute},
	}, nil
}

// withAccountOAuthChallenge refuses an MCP client that has not signed in and
// tells it where it can. Without the refusal a client is let through as
// nobody: it connects, every tool turns it away, and it never learns that
// signing in was the answer.
func (s *graphjinService) withAccountOAuthChallenge(ah auth.HandlerFunc) auth.HandlerFunc {
	if ah == nil || s == nil || s.accountOAuth == nil {
		return ah
	}
	conf := s.conf
	return func(w http.ResponseWriter, r *http.Request) (context.Context, error) {
		ctx, err := ah(w, r)
		if err == nil && auth.IsAuth(ctx) {
			return ctx, nil
		}
		setMCPOAuthChallenge(w, r, conf)
		return ctx, auth.Err401
	}
}

// OAuthConsent serves the step where a signed-in person allows an MCP client
// to act as them. GET describes the request; POST allows or refuses it and
// answers with the address to send the browser back to.
func (s1 *HttpService) OAuthConsent(ah auth.HandlerFunc) http.Handler {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := s1.Load().(*graphjinService)
		w.Header().Set("Cache-Control", "no-store")
		if s.accountOAuth == nil {
			writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "this server does not sign clients in with local accounts"})
			return
		}
		s.accountOAuth.handleConsent(w, r, s, ah)
	})
	return apiV1Handler(s1, nil, h, nil)
}

const consentGone = "This request has expired or was already answered. Start again from the app you are connecting."

func (a *authLoginService) handleConsent(w http.ResponseWriter, r *http.Request, s *graphjinService, ah auth.HandlerFunc) {
	if !consoleSameOrigin(r) {
		writeJSONStatus(w, http.StatusForbidden, map[string]any{"error": "cross-origin requests are refused"})
		return
	}
	// Only a person signed in to the console in this browser can allow a
	// client. A token a client already holds cannot be used to allow another.
	ctx, err := ah(w, r)
	accountID := ""
	if err == nil && ctx != nil && usesConsoleSession(r) {
		accountID, _ = ctx.Value(localAccountKey{}).(string)
	}
	if accountID == "" {
		writeJSONStatus(w, http.StatusUnauthorized, map[string]any{"error": "sign in with your account first"})
		return
	}
	account, err := a.accounts.get(ctx, accountID)
	if err != nil || account == nil || account.Disabled {
		writeJSONStatus(w, http.StatusUnauthorized, map[string]any{"error": "sign in with your account first"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		req, err := a.state().peekOAuthRequest(ctx, r.URL.Query().Get("request"), time.Now())
		if err != nil {
			writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "could not read the request"})
			return
		}
		if req == nil || req.ConsoleNext != "" {
			writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": consentGone})
			return
		}
		host, local := redirectDestination(req.RedirectURI)
		writeJSONStatus(w, http.StatusOK, map[string]any{
			"client_name":      a.oauthClientName(ctx, req.ClientID),
			"destination":      host,
			"on_this_computer": local,
			"email":            account.Email,
			"role":             account.Role,
		})
	case http.MethodPost:
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			writeJSONStatus(w, http.StatusUnsupportedMediaType, map[string]any{"error": "send a JSON body"})
			return
		}
		var body struct {
			Request string `json:"request"`
			Allow   bool   `json:"allow"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&body); err != nil {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
			return
		}
		a.decideConsent(ctx, w, s, account, body.Request, body.Allow)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// decideConsent uses up the request, so an answer cannot be given twice.
func (a *authLoginService) decideConsent(ctx context.Context, w http.ResponseWriter, s *graphjinService, account *localAccount, id string, allow bool) {
	req, err := a.state().takeOAuthRequest(ctx, id)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "could not read the request"})
		return
	}
	if req == nil || req.ConsoleNext != "" || time.Now().After(req.ExpiresAt) {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": consentGone})
		return
	}
	back, err := url.Parse(req.RedirectURI)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "the request names an address that cannot be used"})
		return
	}
	q := back.Query()
	if req.ClientState != "" {
		q.Set("state", req.ClientState)
	}
	if !allow {
		q.Set("error", "access_denied")
		back.RawQuery = q.Encode()
		writeJSONStatus(w, http.StatusOK, map[string]any{"redirect": back.String()})
		return
	}

	code, err := randomURLToken(32)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "could not create a sign-in code"})
		return
	}
	err = a.state().putOAuthCode(ctx, code, &mcpOAuthCode{
		ClientID:    req.ClientID,
		RedirectURI: req.RedirectURI,
		Scope:       req.Scope,
		Resource:    req.Resource,
		Challenge:   req.CodeChallenge,
		Identity:    &oidc.Identity{Subject: account.ID, Email: account.Email, Name: account.Email, Issuer: accountIdentityIssuer},
		ExpiresAt:   time.Now().Add(mcpOAuthAuthCodeTTL),
	})
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "could not store the sign-in code"})
		return
	}
	// Someone has allowed this client, so keep its registration.
	client, _ := a.state().getOAuthClient(ctx, req.ClientID, time.Now())
	if client != nil {
		_ = a.state().putOAuthClient(ctx, client, time.Time{})
	}
	if s != nil && s.log != nil {
		s.log.Infof("mcp: %s allowed %s to connect", account.Email, oauthClientLabel(client))
	}
	q.Set("code", code)
	back.RawQuery = q.Encode()
	writeJSONStatus(w, http.StatusOK, map[string]any{"redirect": back.String()})
}

func (a *authLoginService) oauthClientName(ctx context.Context, clientID string) string {
	client, _ := a.state().getOAuthClient(ctx, clientID, time.Now())
	return oauthClientLabel(client)
}

// oauthClientLabel is the name a client gave itself. Anyone can register
// under any name, so it is shown beside where the client will send the
// person, never on its own.
func oauthClientLabel(client *mcpOAuthClient) string {
	name := ""
	if client != nil {
		name = strings.Join(strings.Fields(client.ClientName), " ")
	}
	if name == "" {
		return "An app"
	}
	if len(name) > 80 {
		name = name[:80]
	}
	return name
}

// redirectDestination reports the host a client sends the person back to and
// whether that is this person's own computer.
func redirectDestination(redirectURI string) (string, bool) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		return "", false
	}
	return u.Host, isLoopbackHost(u.Hostname())
}
