package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// An agent such as Claude Code is given the server's address and nothing
// else. It has to be able to find out where to sign in, send the person to
// the console to allow it, and come away with a token for that person's
// account, without anyone copying a secret by hand.
func TestAnAgentSignsInWithAnAccount(t *testing.T) {
	if testing.Short() {
		t.Skip("embedded service integration")
	}
	const setupToken = "connect-setup-token"
	t.Setenv("DEEPORG_SETUP_TOKEN", setupToken)
	base, secret, _ := bootGeneratedAgentic(t, "")
	mcpURL := base + "/api/v1/mcp"

	status, body, cookie := accountsPost(t, base+"/api/v1/setup/admin", "", map[string]any{"token": setupToken, "email": "ada@example.com", "password": "a long enough password"})
	if status != http.StatusOK || cookie == "" {
		t.Fatalf("creating the admin answered %d: %s", status, body)
	}

	// The console is told the address to hand to an agent.
	if page := accountsGet(t, base+"/api/v1/console/bootstrap", cookie); !strings.Contains(page, `"mcp_url":"`+mcpURL+`"`) {
		t.Fatalf("the console was not told how to connect an agent: %s", clip(page))
	}

	// Refused, the agent is told where sign-in is described.
	res := firstBootRequest(t, mcpURL, "", "", firstBootInitialize())
	res.Body.Close() //nolint:errcheck
	challenge := res.Header.Get("WWW-Authenticate")
	if res.StatusCode != http.StatusUnauthorized || !strings.Contains(challenge, `resource_metadata="`) {
		t.Fatalf("an agent with no token got %d and the challenge %q", res.StatusCode, challenge)
	}
	metadataURL := strings.TrimSuffix(strings.SplitN(challenge, `resource_metadata="`, 2)[1], `"`)

	var resource struct {
		Resource             string   `json:"resource"`
		AuthorizationServers []string `json:"authorization_servers"`
	}
	connectJSON(t, accountsGet(t, metadataURL, ""), &resource)
	if resource.Resource != mcpURL || len(resource.AuthorizationServers) != 1 {
		t.Fatalf("the resource metadata is wrong: %+v", resource)
	}
	var server struct {
		AuthorizationEndpoint string   `json:"authorization_endpoint"`
		TokenEndpoint         string   `json:"token_endpoint"`
		RegistrationEndpoint  string   `json:"registration_endpoint"`
		Methods               []string `json:"code_challenge_methods_supported"`
		MetadataDocuments     bool     `json:"client_id_metadata_document_supported"`
	}
	connectJSON(t, accountsGet(t, resource.AuthorizationServers[0]+"/.well-known/oauth-authorization-server", ""), &server)
	if server.AuthorizationEndpoint == "" || server.TokenEndpoint == "" || server.RegistrationEndpoint == "" || len(server.Methods) != 1 || server.Methods[0] != "S256" {
		t.Fatalf("the authorization server metadata is incomplete: %+v", server)
	}
	// Claude Code skips registering and uses a web address as its id when a
	// server says it accepts one. This server takes registered clients only,
	// so saying so got Claude Code refused at the next step.
	if server.MetadataDocuments {
		t.Fatal("the server says it accepts a client id that is a web address, and it does not")
	}

	// The agent registers itself, naming where it will listen for the answer.
	const redirect = "http://localhost:53124/callback"
	status, body, _ = accountsPost(t, server.RegistrationEndpoint, "", map[string]any{"client_name": "Claude Code", "redirect_uris": []string{redirect}})
	var client struct {
		ClientID string `json:"client_id"`
	}
	connectJSON(t, body, &client)
	if status != http.StatusCreated || client.ClientID == "" {
		t.Fatalf("registering the agent answered %d: %s", status, body)
	}

	const verifier = "a-code-verifier-that-is-long-enough-to-be-allowed-0123456789"
	sum := sha256.Sum256([]byte(verifier))
	authorize := func() string {
		t.Helper()
		q := url.Values{
			"response_type":         {"code"},
			"client_id":             {client.ClientID},
			"redirect_uri":          {redirect},
			"state":                 {"agent-state"},
			"code_challenge":        {base64.RawURLEncoding.EncodeToString(sum[:])},
			"code_challenge_method": {"S256"},
			"resource":              {mcpURL},
		}
		noFollow := &http.Client{Timeout: firstBootClient.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		res, err := noFollow.Get(server.AuthorizationEndpoint + "?" + q.Encode())
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close() //nolint:errcheck
		to, err := url.Parse(res.Header.Get("Location"))
		if res.StatusCode != http.StatusFound || err != nil || to.Path != "/connect" || to.Query().Get("request") == "" {
			t.Fatalf("the browser was sent to %q with status %d, not to the console to approve", res.Header.Get("Location"), res.StatusCode)
		}
		return to.Query().Get("request")
	}
	request := authorize()
	consent := base + "/api/v1/oauth/consent"

	// Nobody but a person signed in to the console can see or answer it.
	if status, body, _ := accountsPost(t, consent, "", map[string]any{"request": request, "allow": true}); status != http.StatusUnauthorized {
		t.Fatalf("allowing without signing in answered %d: %s", status, body)
	}
	// A token made by hand has no account behind it and cannot allow anything.
	res = firstBootRequest(t, consent, signFirstBootToken(t, secret, "sam", "admin"), "", map[string]any{"request": request, "allow": true})
	res.Body.Close() //nolint:errcheck
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("allowing with a hand-made token answered %d", res.StatusCode)
	}

	// The person is shown who is asking and where it will send them.
	shown := accountsGet(t, consent+"?request="+url.QueryEscape(request), cookie)
	for _, want := range []string{`"client_name":"Claude Code"`, `"destination":"localhost:53124"`, `"on_this_computer":true`, `"email":"ada@example.com"`} {
		if !strings.Contains(shown, want) {
			t.Fatalf("the approval screen is missing %s: %s", want, shown)
		}
	}

	// Saying no sends the agent away with nothing.
	status, body, _ = accountsPost(t, consent, cookie, map[string]any{"request": request, "allow": false})
	if status != http.StatusOK || !strings.Contains(body, "error=access_denied") || strings.Contains(body, "code=") {
		t.Fatalf("refusing answered %d: %s", status, body)
	}
	// An answered request cannot be answered again.
	if status, body, _ := accountsPost(t, consent, cookie, map[string]any{"request": request, "allow": true}); status != http.StatusNotFound {
		t.Fatalf("answering a request twice gave %d: %s", status, body)
	}

	// Saying yes hands the agent a one-time code.
	request = authorize()
	status, body, _ = accountsPost(t, consent, cookie, map[string]any{"request": request, "allow": true})
	var answer struct {
		Redirect string `json:"redirect"`
	}
	connectJSON(t, body, &answer)
	back, err := url.Parse(answer.Redirect)
	if status != http.StatusOK || err != nil || back.Host != "localhost:53124" || back.Query().Get("state") != "agent-state" || back.Query().Get("code") == "" {
		t.Fatalf("allowing answered %d: %s", status, body)
	}
	code := back.Query().Get("code")

	token := func(form url.Values) (int, map[string]any) {
		t.Helper()
		res, err := firstBootClient.PostForm(server.TokenEndpoint, form)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close() //nolint:errcheck
		raw, _ := io.ReadAll(res.Body)
		out := map[string]any{}
		_ = json.Unmarshal(raw, &out)
		return res.StatusCode, out
	}
	exchange := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {client.ClientID}, "redirect_uri": {redirect}, "resource": {mcpURL}}

	// The code is useless without the secret the agent kept to itself, and
	// trying uses it up.
	exchange.Set("code_verifier", "not the verifier the agent started with, at all")
	if status, out := token(exchange); status == http.StatusOK {
		t.Fatalf("a code was exchanged with the wrong verifier: %v", out)
	}
	exchange.Set("code_verifier", verifier)
	if status, out := token(exchange); status == http.StatusOK {
		t.Fatalf("a code was exchanged a second time: %v", out)
	}

	request = authorize()
	_, body, _ = accountsPost(t, consent, cookie, map[string]any{"request": request, "allow": true})
	connectJSON(t, body, &answer)
	back, _ = url.Parse(answer.Redirect)
	exchange.Set("code", back.Query().Get("code"))
	status, out := token(exchange)
	access, _ := out["access_token"].(string)
	refresh, _ := out["refresh_token"].(string)
	if status != http.StatusOK || access == "" || refresh == "" {
		t.Fatalf("exchanging the code answered %d: %v", status, out)
	}

	// The agent now acts as the admin through the MCP tools.
	got := firstBootMCPCall(t, base, access, "query_catalog", map[string]any{"search": "customers and their orders"})
	if !strings.Contains(got, "cards") {
		t.Fatalf("the signed-in agent could not search the catalog: %s", clip(got))
	}
	// The role comes from the account, not from anything the token says.
	if role := regexp.MustCompile(`role_class\W+(\w+)`).FindStringSubmatch(got); len(role) != 2 || role[1] != "admin" {
		t.Fatalf("the agent did not get its account's admin role: %v in %s", role, clip(got))
	}
	// Its token is good at the MCP endpoint and nowhere else: what a
	// connected agent can do is what the MCP tools let it do.
	for _, path := range []string{"/api/v1/graphql", "/api/v1/console/bootstrap", "/api/v1/agent/status"} {
		res := firstBootRequest(t, base+path, access, "", map[string]any{"query": `query { gj_config(id: "current") { id } }`})
		raw, _ := io.ReadAll(res.Body)
		res.Body.Close() //nolint:errcheck
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("the agent's token was accepted at %s (%d): %s", path, res.StatusCode, clip(string(raw)))
		}
	}
	// A token the agent holds cannot be used to allow another agent.
	request = authorize()
	res = firstBootRequest(t, consent, access, "", map[string]any{"request": request, "allow": true})
	res.Body.Close() //nolint:errcheck
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("an agent's own token allowed another request: %d", res.StatusCode)
	}

	// The short-lived token is renewed without asking the person again.
	status, out = token(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {client.ClientID}, "resource": {mcpURL}})
	if renewed, _ := out["access_token"].(string); status != http.StatusOK || renewed == "" {
		t.Fatalf("renewing the token answered %d: %v", status, out)
	}

	// None of this is readable through GraphQL.
	assertAccountsNotReadable(t, base, map[string]string{"the admin": cookie})
}

func connectJSON(t *testing.T, raw string, into any) {
	t.Helper()
	if err := json.Unmarshal([]byte(raw), into); err != nil {
		t.Fatalf("not JSON: %s", clip(raw))
	}
}
