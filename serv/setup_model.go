package serv

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	gjagent "github.com/dosco/graphjin/agent/v3"
	"github.com/dosco/graphjin/auth/v3"
	"gopkg.in/yaml.v3"
)

// The model step of setup. The admin picks a provider and gives its API key;
// the server checks the key with one small request, keeps it encrypted in the
// store and comes back up with the built-in agent switched on.

const (
	routeSetupModel = "/api/v1/setup/model"

	// setupModelCheckTimeout bounds the live check of a key.
	setupModelCheckTimeout = 30 * time.Second
)

var setupProviderRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)

// setupNeedsModel reports whether the built-in agent has no key to run with.
func (s *graphjinService) setupNeedsModel() bool {
	return s.storeBacked() && s.conf.agentEnabled() && !s.conf.Agent.APIKeyConfigured()
}

type setupModelRequest struct {
	Provider string `json:"provider"`
	APIKey   string `json:"api_key"`
	Model    string `json:"model"`
}

func (s *graphjinService) setupChooseModel(w http.ResponseWriter, r *http.Request, ah auth.HandlerFunc) {
	if !s.storeBacked() {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "this server is set up through its config files"})
		return
	}
	ctx, ok := s.setupAdminContext(w, r, ah)
	if !ok {
		return
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeJSONStatus(w, http.StatusUnsupportedMediaType, map[string]any{"error": "send a JSON body"})
		return
	}
	var req setupModelRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
		return
	}
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	model := strings.TrimSpace(req.Model)
	key := strings.TrimSpace(req.APIKey)
	switch {
	case !setupProviderRe.MatchString(provider):
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "choose a provider"})
		return
	case key == "":
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "paste the provider's API key"})
		return
	case len(key) > 4096 || strings.ContainsAny(key, " \t\r\n"):
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "that does not look like an API key; paste the key on its own"})
		return
	}

	// Check the key against the provider before keeping it.
	candidate := s.conf.Agent
	candidate.Provider, candidate.Model = provider, model
	checkCtx, cancel := context.WithTimeout(ctx, setupModelCheckTimeout)
	err := gjagent.CheckAPIKey(checkCtx, candidate, key, s.agentClientFactory)
	cancel()
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{
			"error":  modelKeyAdvice(err, provider, model),
			"detail": redactModelKey(redactRuntimeError(err), key),
		})
		return
	}

	lock := s.configLocker()
	lock.Lock()
	defer lock.Unlock()
	doc, err := readStoredConfig(ctx, s.accounts.store, s.conf.storeSealKey)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "could not read the stored configuration"})
		return
	}
	updated, err := storedConfigWithModel(doc, provider, model)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	// The key first: a configuration that names a provider with no key would
	// leave the agent off, while a key with no configuration changes nothing.
	if err := writeStoredSecret(ctx, s.accounts.store, s.conf.storeSealKey, StoredModelKey, key); err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "could not save the key"})
		return
	}
	if err := writeStoredConfig(ctx, s.accounts.store, s.conf.storeSealKey, updated); err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "could not save the model choice"})
		return
	}
	if s.log != nil {
		s.log.Infof("setup: model provider set to %s", provider)
	}
	writeJSONStatus(w, http.StatusOK, map[string]any{"provider": provider, "model": model, "restarting": s.configApplier != nil})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	s.applyStoredConfigSoon()
}

// storedConfigWithModel sets the agent's provider and model in the document
// and points its key at the variable the stored key is handed over in.
func storedConfigWithModel(doc, provider, model string) (string, error) {
	settings, err := parseStoredConfig(doc)
	if err != nil {
		return "", err
	}
	agent, _ := settings["agent"].(map[string]any)
	if agent == nil {
		agent = map[string]any{}
	}
	agent["provider"] = provider
	agent["api_key_env"] = StoredModelKeyEnv
	delete(agent, "api_key_file")
	if model != "" {
		agent["model"] = model
	} else {
		delete(agent, "model")
	}
	settings["agent"] = agent
	out, err := yaml.Marshal(settings)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// modelKeyAdvice turns a provider error into what went wrong and what to do.
func modelKeyAdvice(err error, provider, model string) string {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "unsupported axai provider") || strings.Contains(msg, "not a usable ax deployment profile"):
		return "We do not know a provider called " + provider + "."
	case strings.Contains(msg, "401") || strings.Contains(msg, "unauthorized") || strings.Contains(msg, "invalid api key") || strings.Contains(msg, "invalid_api_key") || strings.Contains(msg, "incorrect api key") || strings.Contains(msg, "api key not valid") || strings.Contains(msg, "authentication"):
		return "The provider did not accept that key. Check that it was copied in full and belongs to this provider."
	case strings.Contains(msg, "403") || strings.Contains(msg, "permission"):
		return "The provider accepted the key but it is not allowed to use this model. Check the key's permissions."
	case strings.Contains(msg, "404") || strings.Contains(msg, "model_not_found") || strings.Contains(msg, "does not exist") || strings.Contains(msg, "not found"):
		if model != "" {
			return "The provider has no model called " + model + " for this key. Check the name, or leave it empty to use the provider's default."
		}
		return "The provider could not find a model to use with this key."
	case strings.Contains(msg, "429") || strings.Contains(msg, "quota") || strings.Contains(msg, "rate limit") || strings.Contains(msg, "insufficient") || strings.Contains(msg, "billing"):
		return "The provider accepted the key but the account has no usage left. Check its billing and limits."
	case strings.Contains(msg, "deadline exceeded") || strings.Contains(msg, "timeout") || strings.Contains(msg, "no such host") || strings.Contains(msg, "connection refused"):
		return "We could not reach the provider. Check that this server can make outbound requests."
	}
	return "The provider did not accept a test request with that key."
}

// redactModelKey removes the key from a message before it leaves the server.
// A provider or a client library may echo the key back in an error.
func redactModelKey(msg, key string) string {
	if key == "" {
		return msg
	}
	return strings.ReplaceAll(msg, key, "[key]")
}
