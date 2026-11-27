package serv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dosco/graphjin/auth/v3"
	"golang.org/x/crypto/bcrypt"
)

// What an admin does to an account that already exists: change what the
// person may do, give them a way to choose a new password, or remove them.
//
// A new password is chosen by the person, the same way an invite works. The
// admin gets a link to send; whoever opens it chooses the password and is
// signed in. Nobody ever knows someone else's password. Whoever runs the
// server can make the same link from the command line, which is the way back
// in for an admin who has lost their own password.
//
// Two rules keep a server from being left with nobody in charge: an admin
// cannot remove or demote their own account, and the last admin cannot be
// removed or demoted by anyone.

const (
	routeSetupPeople      = "/api/v1/setup/people"
	routeSetupPeopleReset = "/api/v1/setup/people/reset"

	// consoleResetPath is the console page a password link opens.
	consoleResetPath = "/reset"

	resetKind = "password_reset"
	resetTTL  = 24 * time.Hour
)

var (
	errNoSuchAccount = errors.New("nobody has an account with that email")
	errOwnAccount    = errors.New("you cannot do that to your own account; ask another admin")
	errLastAdmin     = errors.New("this is the only admin; make someone else an admin first")
)

type storedReset struct {
	Email     string `json:"email"`
	CreatedAt int64  `json:"created_at"`
}

// replace writes an account row again, with the role in lookup_key where the
// search for an admin reads it. It reports false when the row changed since
// it was read.
func (st *sqlAuthStore) replace(ctx context.Context, kind string, row *authStoreRow, lookupKey string, v any) (bool, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return false, err
	}
	res, err := st.db.ExecContext(ctx,
		st.q(`UPDATE {t} SET data = ?, lookup_key = ?, version = version + 1, updated_at = ? WHERE kind = ? AND id = ? AND version = ?`),
		string(data), lookupKey, time.Now().UnixMilli(), kind, row.id, row.version)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// change reads the account for this email, applies fn and writes it back.
func (a *accountService) change(ctx context.Context, email string, fn func(*localAccount) error) (*localAccount, error) {
	id := accountID(email)
	for attempt := 0; attempt < authStoreCASAttempts; attempt++ {
		row, err := a.store.get(ctx, accountKind, id)
		if err != nil {
			return nil, err
		}
		if row == nil {
			return nil, errNoSuchAccount
		}
		var account localAccount
		if err := json.Unmarshal(row.data, &account); err != nil {
			return nil, err
		}
		if err := fn(&account); err != nil {
			return nil, err
		}
		ok, err := a.store.replace(ctx, accountKind, row, account.Role, &account)
		if err != nil {
			return nil, err
		}
		if ok {
			a.forget(id)
			return &account, nil
		}
	}
	return nil, errAuthStoreConflict
}

// otherAdmins counts the admins that can sign in, leaving one account out.
func (a *accountService) otherAdmins(ctx context.Context, exceptID string) (int, error) {
	rows, err := a.store.list(ctx, accountKind, setupListLimit)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, row := range rows {
		var account localAccount
		if json.Unmarshal(row.data, &account) != nil {
			continue
		}
		if account.ID != exceptID && account.Role == accountRoleAdmin && !account.Disabled {
			n++
		}
	}
	return n, nil
}

// guardAdmin refuses a change that would take admin rights away from the
// caller's own account or from the last admin.
func (a *accountService) guardAdmin(ctx context.Context, email, by string) error {
	id := accountID(email)
	if by != "" && id == by {
		return errOwnAccount
	}
	account, err := a.get(ctx, id)
	if err != nil {
		return err
	}
	if account == nil {
		return errNoSuchAccount
	}
	if account.Role != accountRoleAdmin {
		return nil
	}
	others, err := a.otherAdmins(ctx, id)
	if err != nil {
		return err
	}
	if others == 0 {
		return errLastAdmin
	}
	return nil
}

// setRole makes the account an admin or an ordinary user. by is the account
// making the change.
func (a *accountService) setRole(ctx context.Context, email, role, by string) (*localAccount, error) {
	if role != accountRoleAdmin {
		role = accountRoleUser
	}
	a.forget(accountID(email))
	if role != accountRoleAdmin {
		if err := a.guardAdmin(ctx, email, by); err != nil {
			return nil, err
		}
	}
	return a.change(ctx, email, func(account *localAccount) error {
		account.Role = role
		return nil
	})
}

// remove deletes the account. Every session and connected agent that names
// it is refused from then on, because each request looks the account up.
func (a *accountService) remove(ctx context.Context, email, by string) error {
	id := accountID(email)
	a.forget(id)
	if err := a.guardAdmin(ctx, email, by); err != nil {
		return err
	}
	if _, err := a.store.del(ctx, accountKind, id); err != nil {
		return err
	}
	a.forget(id)
	return a.cancelReset(ctx, email)
}

// resetLink makes a link with which the person chooses a new password. An
// earlier link for the same account stops working.
func (a *accountService) resetLink(ctx context.Context, email string) (token string, expires time.Time, err error) {
	a.forget(accountID(email))
	account, err := a.get(ctx, accountID(email))
	if err != nil {
		return "", time.Time{}, err
	}
	if account == nil {
		return "", time.Time{}, errNoSuchAccount
	}
	if err := a.cancelReset(ctx, account.Email); err != nil {
		return "", time.Time{}, err
	}
	if token, err = randomURLToken(32); err != nil {
		return "", time.Time{}, err
	}
	expires = time.Now().Add(resetTTL)
	reset := storedReset{Email: account.Email, CreatedAt: time.Now().Unix()}
	if err := a.store.insert(ctx, resetKind, authStoreKey(token), account.Email, reset, expires); err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

func (a *accountService) cancelReset(ctx context.Context, email string) error {
	for {
		row, err := a.store.getBy(ctx, resetKind, "lookup_key", normalizeEmail(email))
		if err != nil || row == nil {
			return err
		}
		if _, err := a.store.del(ctx, resetKind, row.id); err != nil {
			return err
		}
	}
}

// pendingReset reads a password link by its token without using it up.
func (a *accountService) pendingReset(ctx context.Context, token string) (*storedReset, error) {
	row, err := a.store.get(ctx, resetKind, authStoreKey(strings.TrimSpace(token)))
	if err != nil || row == nil || row.expired(time.Now()) {
		return nil, err
	}
	var reset storedReset
	if err := json.Unmarshal(row.data, &reset); err != nil {
		return nil, err
	}
	return &reset, nil
}

// setPassword gives the account a new password and signs it out everywhere:
// sessions and agent tokens made before now stop working.
func (a *accountService) setPassword(ctx context.Context, email, password string) (*localAccount, error) {
	if err := checkPasswordLength(password); err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword(passwordBytes(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	return a.change(ctx, email, func(account *localAccount) error {
		account.PasswordHash = string(hash)
		account.SessionsAfter = time.Now().Unix()
		return nil
	})
}

func checkPasswordLength(password string) error {
	if len(password) < minPasswordLength {
		return fmt.Errorf("choose a password of at least %d characters", minPasswordLength)
	}
	if len(password) > maxPasswordLength {
		return fmt.Errorf("choose a password of at most %d characters", maxPasswordLength)
	}
	return nil
}

// issuedBefore reports whether a token was signed before the account's
// password was last changed. A token with no issue time counts as older.
func issuedBefore(claims map[string]any, sessionsAfter int64) bool {
	if sessionsAfter <= 0 {
		return false
	}
	switch iat := claims["iat"].(type) {
	case float64:
		return int64(iat) < sessionsAfter
	case int64:
		return iat < sessionsAfter
	case json.Number:
		n, err := iat.Int64()
		return err != nil || n < sessionsAfter
	}
	return true
}

// peopleError picks the status for a refusal the admin can do something about.
func peopleError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, errNoSuchAccount):
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": err.Error()})
	case errors.Is(err, errOwnAccount), errors.Is(err, errLastAdmin):
		writeJSONStatus(w, http.StatusConflict, map[string]any{"error": err.Error()})
	default:
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": fallback})
	}
}

// setupPeople lets an admin change a person's role or remove them. Both are
// a POST with the email in the body, so an address never ends up in a URL
// and from there in a log.
func (s *graphjinService) setupPeople(w http.ResponseWriter, r *http.Request, ah auth.HandlerFunc) {
	ctx, ok := s.setupAdminContext(w, r, ah)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	by, _ := ctx.Value(localAccountKey{}).(string)
	var req struct {
		Email  string `json:"email"`
		Role   string `json:"role"`
		Remove bool   `json:"remove"`
	}
	if !decodeSetupJSON(w, r, &req) {
		return
	}
	if req.Remove {
		email := normalizeEmail(req.Email)
		if err := s.accounts.remove(ctx, email, by); err != nil {
			peopleError(w, err, "could not remove the account")
			return
		}
		if s.log != nil {
			s.log.Infof("setup: removed %s", email)
		}
		writeJSONStatus(w, http.StatusOK, map[string]any{"removed": true})
		return
	}
	role := strings.ToLower(strings.TrimSpace(req.Role))
	if role != accountRoleAdmin && role != accountRoleUser {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "the role is admin or user"})
		return
	}
	account, err := s.accounts.setRole(ctx, req.Email, role, by)
	if err != nil {
		peopleError(w, err, "could not change the role")
		return
	}
	if s.log != nil {
		s.log.Infof("setup: %s is now %s", account.Email, account.Role)
	}
	writeJSONStatus(w, http.StatusOK, map[string]any{"email": account.Email, "role": account.Role})
}

// setupPeopleReset makes a password link for an admin to send.
func (s *graphjinService) setupPeopleReset(w http.ResponseWriter, r *http.Request, ah auth.HandlerFunc) {
	ctx, ok := s.setupAdminContext(w, r, ah)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if !decodeSetupJSON(w, r, &req) {
		return
	}
	token, expires, err := s.accounts.resetLink(ctx, req.Email)
	if err != nil {
		peopleError(w, err, "could not make the link")
		return
	}
	if s.log != nil {
		s.log.Infof("setup: password link made for %s", normalizeEmail(req.Email))
	}
	writeJSONStatus(w, http.StatusOK, map[string]any{
		"email":      normalizeEmail(req.Email),
		"link":       publicBaseURL(r) + consoleResetPath + "?token=" + token,
		"expires_at": expires.Unix(),
	})
}

// CreatePasswordReset makes a password link token for the account with this
// email, in the store file at path. It is for whoever runs the server: the
// way back in when the only admin has lost their password. The server does
// not have to be stopped.
func CreatePasswordReset(ctx context.Context, path, email string) (token string, expires time.Time, err error) {
	st, done, err := openStoreSettings(ctx, path)
	if err != nil {
		return "", time.Time{}, err
	}
	defer done()
	accounts := &accountService{store: st, cache: make(map[string]cachedAccount)}
	return accounts.resetLink(ctx, email)
}

// IsNoSuchAccount reports whether err says the email has no account.
func IsNoSuchAccount(err error) bool {
	return errors.Is(err, errNoSuchAccount)
}

// ResetLinkPath is the path and query of the page a password link opens.
func ResetLinkPath(token string) string {
	return consoleResetPath + "?token=" + token
}
