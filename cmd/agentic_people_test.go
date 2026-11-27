package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dosco/graphjin/serv/v3"
	"go.uber.org/zap/zapcore"
)

// removePerson asks the server to remove an account, as the console does.
func removePerson(t *testing.T, base, cookie, email string) (int, string) {
	t.Helper()
	status, body, _ := accountsPost(t, base+"/api/v1/setup/people", cookie, map[string]any{"email": email, "remove": true})
	return status, body
}

// After a team has joined, an admin changes what a person may do, gives them
// a way to choose a new password, and removes them. Whoever runs the server
// can make a password link from the command line, which is the way back in
// for an admin who has lost their own password.
func TestAdminChangesRolesResetsPasswordsAndRemovesPeople(t *testing.T) {
	if testing.Short() {
		t.Skip("embedded service integration")
	}
	inEmptyDir(t)
	const setupToken = "people-setup-token"
	t.Setenv("DEEPORG_SETUP_TOKEN", setupToken)

	file, ok := usesStartConfig(nil)
	if !ok {
		t.Fatal("an empty directory did not take the new flow")
	}
	sc, err := loadStartConfig(file)
	if err != nil {
		t.Fatal(err)
	}
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
	defer func() { server.Close(); hs.Close() }() //nolint:errcheck
	base := server.URL

	signedIn := func(cookie string) bool {
		t.Helper()
		return strings.Contains(accountsGet(t, base+"/api/v1/console/bootstrap", cookie), `"authenticated":true`)
	}
	people := base + "/api/v1/setup/people"

	status, body, ada := accountsPost(t, base+"/api/v1/setup/admin", "", map[string]any{"token": setupToken, "email": "ada@example.com", "password": "a long enough password"})
	if status != http.StatusOK || ada == "" {
		t.Fatalf("creating the admin answered %d: %s", status, body)
	}
	_, body, _ = accountsPost(t, base+"/api/v1/setup/invites", ada, map[string]any{"email": "sam@example.com"})
	var invite struct {
		Link string `json:"link"`
	}
	connectJSON(t, body, &invite)
	status, body, sam := accountsPost(t, base+"/api/v1/setup/invite", "", map[string]any{"token": strings.TrimPrefix(invite.Link, base+"/invite?token="), "password": "sam's first password"})
	if status != http.StatusOK || sam == "" {
		t.Fatalf("accepting the invite answered %d: %s", status, body)
	}

	// The admin's own account is marked, so the console can leave out what
	// they may not do to it.
	details := accountsGet(t, base+"/api/v1/setup/details", ada)
	if !strings.Contains(details, `"email":"ada@example.com","role":"admin","you":true`) || !strings.Contains(details, `"email":"sam@example.com","role":"user","you":false`) {
		t.Fatalf("the people list is not as expected: %s", clip(details))
	}

	// Only an admin changes people.
	if status, body, _ := accountsPost(t, people, sam, map[string]any{"email": "ada@example.com", "role": "user"}); status != http.StatusForbidden {
		t.Fatalf("a teammate changed a role (%d): %s", status, body)
	}
	if status, body := removePerson(t, base, sam, "ada@example.com"); status != http.StatusForbidden {
		t.Fatalf("a teammate removed someone (%d): %s", status, body)
	}
	if status, body, _ := accountsPost(t, people+"/reset", sam, map[string]any{"email": "ada@example.com"}); status != http.StatusForbidden {
		t.Fatalf("a teammate made a password link (%d): %s", status, body)
	}

	// A role changes at once, with no need to sign in again.
	if status, body, _ := accountsPost(t, people, ada, map[string]any{"email": "Sam@Example.com", "role": "admin"}); status != http.StatusOK || !strings.Contains(body, `"role":"admin"`) {
		t.Fatalf("making sam an admin answered %d: %s", status, body)
	}
	if page := accountsGet(t, base+"/api/v1/console/bootstrap", sam); !strings.Contains(page, `"manage"`) {
		t.Fatalf("a new admin's console was not offered the setup page: %s", clip(page))
	}
	if status, body, _ := accountsPost(t, people, ada, map[string]any{"email": "sam@example.com", "role": "owner"}); status != http.StatusBadRequest {
		t.Fatalf("a made-up role answered %d: %s", status, body)
	}
	// Nobody takes admin rights from their own account.
	if status, body, _ := accountsPost(t, people, ada, map[string]any{"email": "ada@example.com", "role": "user"}); status != http.StatusConflict || !strings.Contains(body, "your own account") {
		t.Fatalf("an admin demoting themself answered %d: %s", status, body)
	}
	if status, body := removePerson(t, base, ada, "ada@example.com"); status != http.StatusConflict {
		t.Fatalf("an admin removing themself answered %d: %s", status, body)
	}
	if status, body, _ := accountsPost(t, people, ada, map[string]any{"email": "sam@example.com", "role": "user"}); status != http.StatusOK {
		t.Fatalf("making sam a user again answered %d: %s", status, body)
	}
	if page := accountsGet(t, base+"/api/v1/console/bootstrap", sam); strings.Contains(page, `"manage"`) {
		t.Fatalf("a demoted admin's console is still offered the setup page: %s", clip(page))
	}

	// A password link. A session is told apart from the password change by
	// the second it was signed in, so let one pass.
	time.Sleep(1100 * time.Millisecond)
	status, body, _ = accountsPost(t, people+"/reset", ada, map[string]any{"email": "sam@example.com"})
	var reset struct {
		Email string `json:"email"`
		Link  string `json:"link"`
	}
	connectJSON(t, body, &reset)
	prefix := base + "/reset?token="
	if status != http.StatusOK || reset.Email != "sam@example.com" || !strings.HasPrefix(reset.Link, prefix) {
		t.Fatalf("making a password link answered %d: %s", status, body)
	}
	token := strings.TrimPrefix(reset.Link, prefix)
	if status, body, _ := accountsPost(t, people+"/reset", ada, map[string]any{"email": "nobody@example.com"}); status != http.StatusNotFound {
		t.Fatalf("a password link for nobody answered %d: %s", status, body)
	}
	use := base + "/api/v1/setup/invite"
	if got := accountsGet(t, use+"?token="+token, ""); !strings.Contains(got, `"email":"sam@example.com"`) || !strings.Contains(got, `"reset":true`) {
		t.Fatalf("the password link did not say who it is for: %s", got)
	}
	if status, body, _ := accountsPost(t, use, "", map[string]any{"token": token, "password": "short"}); status != http.StatusBadRequest || !strings.Contains(body, "at least") {
		t.Fatalf("a short new password answered %d: %s", status, body)
	}
	if !signedIn(sam) {
		t.Fatal("making a password link signed the person out before they used it")
	}
	status, body, samAgain := accountsPost(t, use, "", map[string]any{"token": token, "password": "sam's second password"})
	if status != http.StatusOK || samAgain == "" {
		t.Fatalf("choosing a new password answered %d: %s", status, body)
	}
	if signedIn(sam) {
		t.Fatal("a session from before the new password still works")
	}
	if !signedIn(samAgain) {
		t.Fatal("choosing a new password did not sign the person in")
	}
	if status, body, _ := accountsPost(t, use, "", map[string]any{"token": token, "password": "sam's third password"}); status != http.StatusNotFound {
		t.Fatalf("a password link was used twice (%d): %s", status, body)
	}
	if status, _, _ := accountsPost(t, base+"/api/v1/auth/session", "", map[string]any{"method": "password", "email": "sam@example.com", "password": "sam's first password"}); status != http.StatusUnauthorized {
		t.Fatalf("the old password still signs in (%d)", status)
	}
	if status, _, cookie := accountsPost(t, base+"/api/v1/auth/session", "", map[string]any{"method": "password", "email": "sam@example.com", "password": "sam's second password"}); status != http.StatusOK || cookie == "" {
		t.Fatalf("the new password does not sign in (%d)", status)
	}

	// The command line makes the same kind of link while the server runs.
	if _, _, err := resetPasswordLink(sc, "nobody@example.com"); err == nil || !strings.Contains(err.Error(), "nobody has an account") {
		t.Fatalf("the command made a link for nobody: %v", err)
	}
	link, expires, err := resetPasswordLink(sc, "Ada@Example.com")
	if err != nil {
		t.Fatalf("the command could not make a link: %v", err)
	}
	const linkStart = "http://127.0.0.1:8080/reset?token="
	if !strings.HasPrefix(link, linkStart) || time.Until(expires) < 23*time.Hour {
		t.Fatalf("the command's link is %q, good until %s", link, expires)
	}
	time.Sleep(1100 * time.Millisecond)
	status, body, adaAgain := accountsPost(t, use, "", map[string]any{"token": strings.TrimPrefix(link, linkStart), "password": "ada's second password"})
	if status != http.StatusOK || adaAgain == "" || !strings.Contains(body, `"role":"admin"`) {
		t.Fatalf("the command's link answered %d: %s", status, body)
	}
	if signedIn(ada) {
		t.Fatal("the admin's old session outlived their new password")
	}

	// Removing a person ends their session and their sign-in.
	if status, body := removePerson(t, base, adaAgain, "Sam@Example.com"); status != http.StatusOK {
		t.Fatalf("removing sam answered %d: %s", status, body)
	}
	if signedIn(samAgain) {
		t.Fatal("a removed person is still signed in")
	}
	if status, _, _ := accountsPost(t, base+"/api/v1/auth/session", "", map[string]any{"method": "password", "email": "sam@example.com", "password": "sam's second password"}); status != http.StatusUnauthorized {
		t.Fatalf("a removed person can still sign in (%d)", status)
	}
	if status, body := removePerson(t, base, adaAgain, "sam@example.com"); status != http.StatusNotFound {
		t.Fatalf("removing someone twice answered %d: %s", status, body)
	}
	if details := accountsGet(t, base+"/api/v1/setup/details", adaAgain); strings.Contains(details, "sam@example.com") {
		t.Fatalf("a removed person is still listed: %s", clip(details))
	}
	// They can be invited again later.
	if status, body, _ := accountsPost(t, base+"/api/v1/setup/invites", adaAgain, map[string]any{"email": "sam@example.com"}); status != http.StatusOK {
		t.Fatalf("inviting a removed person again answered %d: %s", status, body)
	}
}
