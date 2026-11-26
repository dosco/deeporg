package serv

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/dosco/graphjin/auth/v3"
)

// What an admin does after first-run setup: see what is connected, connect
// another database, change the model, and bring teammates in.
//
// Teammates join by invite. The admin names an email and gets a link to send;
// whoever opens it chooses a password and is signed in. There is no way to
// sign up without one. A link works once and for a week, and only its hash is
// kept, so the store cannot be read for links that still work.

const (
	routeSetupDetails = "/api/v1/setup/details"
	routeSetupInvites = "/api/v1/setup/invites"
	routeSetupInvite  = "/api/v1/setup/invite"

	// consoleInvitePath is the console page an invite link opens.
	consoleInvitePath = "/invite"

	inviteKind = "invite"
	inviteTTL  = 7 * 24 * time.Hour

	// setupListLimit bounds the people and invites one reply lists.
	setupListLimit = 500
)

// setupBootID changes each time the process starts. A setup step that
// restarts the server reports it, and the console waits for a different one,
// which is how it knows the server it is talking to is the new one.
var setupBootID = newSetupBootID()

func newSetupBootID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return time.Now().Format(time.RFC3339Nano)
	}
	return hex.EncodeToString(b)
}

type storedInvite struct {
	Email     string `json:"email"`
	Role      string `json:"role"`
	InvitedBy string `json:"invited_by,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

// setupDetails tells an admin what is connected and who has an account.
func (s *graphjinService) setupDetails(w http.ResponseWriter, r *http.Request, ah auth.HandlerFunc) {
	ctx, ok := s.setupAdminContext(w, r, ah)
	if !ok {
		return
	}
	out := map[string]any{"store_backed": s.storeBacked()}

	tables := map[string]int{}
	for _, table := range s.connectedTables() {
		tables[strings.ToLower(table.Database)]++
	}
	databases := []map[string]any{}
	for _, source := range s.conf.Core.Sources {
		if source.Kind != "" && !strings.EqualFold(source.Kind, "database") {
			continue
		}
		databases = append(databases, map[string]any{
			"name":   source.Name,
			"type":   source.Type,
			"tables": tables[strings.ToLower(source.Name)],
		})
	}
	out["databases"] = databases

	agent := agentStatusFromConfig(agentConfigFromService(s.conf), nil, s.agentClientFactory != nil)
	out["model"] = map[string]any{"provider": agent.Provider, "model": agent.Model, "ready": agent.Ready}

	people, err := s.accounts.listPeople(ctx)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "could not read the account store"})
		return
	}
	out["people"] = people
	invites, err := s.accounts.listInvites(ctx)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "could not read the account store"})
		return
	}
	out["invites"] = invites
	writeJSONStatus(w, http.StatusOK, out)
}

func (a *accountService) listPeople(ctx context.Context) ([]map[string]any, error) {
	rows, err := a.store.list(ctx, accountKind, setupListLimit)
	if err != nil {
		return nil, err
	}
	people := []map[string]any{}
	for _, row := range rows {
		var account localAccount
		if json.Unmarshal(row.data, &account) != nil {
			continue
		}
		people = append(people, map[string]any{"email": account.Email, "role": account.Role, "disabled": account.Disabled, "created_at": account.CreatedAt})
	}
	sort.SliceStable(people, func(i, j int) bool { return people[i]["email"].(string) < people[j]["email"].(string) })
	return people, nil
}

func (a *accountService) listInvites(ctx context.Context) ([]map[string]any, error) {
	rows, err := a.store.list(ctx, inviteKind, setupListLimit)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	invites := []map[string]any{}
	for _, row := range rows {
		var invite storedInvite
		if row.expired(now) || json.Unmarshal(row.data, &invite) != nil {
			continue
		}
		invites = append(invites, map[string]any{"email": invite.Email, "role": invite.Role, "expires_at": row.expiresAt / 1000})
	}
	return invites, nil
}

// invite makes a link for this email. An earlier invite for the same email
// stops working, so a link sent to the wrong place can be replaced.
func (a *accountService) invite(ctx context.Context, email, role, invitedBy string) (token string, expires time.Time, err error) {
	if !validAccountEmail(email) {
		return "", time.Time{}, errors.New("enter a valid email address")
	}
	if role != accountRoleAdmin {
		role = accountRoleUser
	}
	if existing, err := a.store.get(ctx, accountKind, accountID(email)); err != nil {
		return "", time.Time{}, err
	} else if existing != nil {
		return "", time.Time{}, errAccountExists
	}
	if err := a.cancelInvite(ctx, email); err != nil {
		return "", time.Time{}, err
	}
	if token, err = randomURLToken(32); err != nil {
		return "", time.Time{}, err
	}
	expires = time.Now().Add(inviteTTL)
	invite := storedInvite{Email: normalizeEmail(email), Role: role, InvitedBy: invitedBy, CreatedAt: time.Now().Unix()}
	if err := a.store.insert(ctx, inviteKind, authStoreKey(token), invite.Email, invite, expires); err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

func (a *accountService) cancelInvite(ctx context.Context, email string) error {
	for {
		row, err := a.store.getBy(ctx, inviteKind, "lookup_key", normalizeEmail(email))
		if err != nil || row == nil {
			return err
		}
		if _, err := a.store.del(ctx, inviteKind, row.id); err != nil {
			return err
		}
	}
}

// pendingInvite reads an invite by its link token without using it up.
func (a *accountService) pendingInvite(ctx context.Context, token string) (*storedInvite, error) {
	row, err := a.store.get(ctx, inviteKind, authStoreKey(strings.TrimSpace(token)))
	if err != nil || row == nil || row.expired(time.Now()) {
		return nil, err
	}
	var invite storedInvite
	if err := json.Unmarshal(row.data, &invite); err != nil {
		return nil, err
	}
	return &invite, nil
}

// setupInvites lets an admin invite a teammate (POST) or withdraw an invite
// (DELETE).
func (s *graphjinService) setupInvites(w http.ResponseWriter, r *http.Request, ah auth.HandlerFunc) {
	ctx, ok := s.setupAdminContext(w, r, ah)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodPost:
		var req struct {
			Email string `json:"email"`
			Role  string `json:"role"`
		}
		if !decodeSetupJSON(w, r, &req) {
			return
		}
		by, _ := ctx.Value(localAccountKey{}).(string)
		token, expires, err := s.accounts.invite(ctx, req.Email, strings.ToLower(strings.TrimSpace(req.Role)), by)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, errAccountExists) {
				status = http.StatusConflict
			}
			writeJSONStatus(w, status, map[string]any{"error": err.Error()})
			return
		}
		if s.log != nil {
			s.log.Infof("setup: invited %s", normalizeEmail(req.Email))
		}
		writeJSONStatus(w, http.StatusOK, map[string]any{
			"email":      normalizeEmail(req.Email),
			"link":       publicBaseURL(r) + consoleInvitePath + "?token=" + token,
			"expires_at": expires.Unix(),
		})
	case http.MethodDelete:
		if err := s.accounts.cancelInvite(ctx, r.URL.Query().Get("email")); err != nil {
			writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "could not withdraw the invite"})
			return
		}
		writeJSONStatus(w, http.StatusOK, map[string]any{"withdrawn": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

const inviteGone = "This invite link is not valid any more. Ask whoever invited you for a new one."

// setupInvite is the teammate's side: GET says who the invite is for, POST
// chooses a password, creates the account and signs it in.
func (s *graphjinService) setupInvite(w http.ResponseWriter, r *http.Request) {
	if !consoleSameOrigin(r) {
		writeJSONStatus(w, http.StatusForbidden, map[string]any{"error": "cross-origin requests are refused"})
		return
	}
	accounts := s.accounts
	// A link is a long random secret, but guessing at them is still limited.
	who := "invite:" + clientAddress(r)
	if !accounts.failures.allow(who) {
		writeJSONStatus(w, http.StatusTooManyRequests, map[string]any{"error": "too many attempts; wait a minute and try again"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		invite, err := accounts.pendingInvite(r.Context(), r.URL.Query().Get("token"))
		if err != nil {
			writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "could not read the invite"})
			return
		}
		if invite == nil {
			accounts.failures.fail(who)
			writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": inviteGone})
			return
		}
		writeJSONStatus(w, http.StatusOK, map[string]any{"email": invite.Email})
	case http.MethodPost:
		var req struct {
			Token    string `json:"token"`
			Password string `json:"password"`
		}
		if !decodeSetupJSON(w, r, &req) {
			return
		}
		invite, err := accounts.pendingInvite(r.Context(), req.Token)
		if err != nil {
			writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "could not read the invite"})
			return
		}
		if invite == nil {
			accounts.failures.fail(who)
			writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": inviteGone})
			return
		}
		// A password that is refused leaves the invite in place to try again.
		account, err := accounts.create(r.Context(), invite.Email, req.Password, invite.Role)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, errAccountExists) {
				status = http.StatusConflict
			}
			writeJSONStatus(w, status, map[string]any{"error": err.Error()})
			return
		}
		_ = accounts.cancelInvite(r.Context(), invite.Email)
		if s.log != nil {
			s.log.Infof("setup: %s joined by invite", account.Email)
		}
		s.finishAccountSignIn(w, r, account)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// decodeSetupJSON reads a small JSON body, answering for itself when it
// cannot.
func decodeSetupJSON(w http.ResponseWriter, r *http.Request, into any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeJSONStatus(w, http.StatusUnsupportedMediaType, map[string]any{"error": "send a JSON body"})
		return false
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(into); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
		return false
	}
	return true
}
