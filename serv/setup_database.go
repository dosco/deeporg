package serv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/dosco/graphjin/auth/v3"
	"github.com/dosco/graphjin/core/v3"
	"gopkg.in/yaml.v3"
)

// The database step of setup. The admin gives a connection; the server tests
// it, saves it in the stored configuration and comes back up connected.
//
// It exists only on an install that keeps its configuration in the store. An
// install run from config files is changed by editing those files.

const (
	routeSetupDatabase = "/api/v1/setup/database"

	// setupConnectTimeout bounds the connection test. Long enough for a cold
	// cloud database, short enough that a wrong host name is reported while
	// the person is still looking at the form.
	setupConnectTimeout = 12 * time.Second
)

// OptionSetConfigApplier supplies how a changed stored configuration takes
// effect. The CLI restarts the process in place; a test rebuilds the service.
func OptionSetConfigApplier(apply func() error) Option {
	return func(s *graphjinService) error {
		s.configApplier = apply
		return nil
	}
}

// OptionSetSampleData supplies how the sample database is built. It returns
// the SQLite file to connect. Without it the setup page offers no sample data.
func OptionSetSampleData(build func(context.Context) (string, error)) Option {
	return func(s *graphjinService) error {
		s.sampleData = build
		return nil
	}
}

// storeBacked reports whether this install keeps its configuration in the
// store, which is what makes the setup steps after the admin available.
func (s *graphjinService) storeBacked() bool {
	return s != nil && s.conf != nil && s.conf.storePath != "" && s.accounts != nil
}

// setupNeedsDatabase reports whether no database has been connected yet.
func (s *graphjinService) setupNeedsDatabase() bool {
	if !s.storeBacked() {
		return false
	}
	for _, source := range s.conf.Core.Sources {
		if strings.EqualFold(source.Kind, "database") || source.Kind == "" {
			return false
		}
	}
	return true
}

type setupDatabaseRequest struct {
	// Sample asks for the built-in sample company's data in place of a
	// connection of the caller's own.
	Sample   bool   `json:"sample"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	URL      string `json:"url"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
	Path     string `json:"path"`
}

var setupSourceNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

// source turns what the form sent into a source for the configuration.
func (req setupDatabaseRequest) source() (map[string]any, core.DatabaseConfig, error) {
	name := strings.ToLower(strings.TrimSpace(req.Name))
	if name == "" {
		name = "main"
	}
	if !setupSourceNameRe.MatchString(name) {
		return nil, core.DatabaseConfig{}, errors.New("the name can use lowercase letters, digits and underscores, and must start with a letter")
	}
	source := map[string]any{"name": name, "kind": "database"}
	dbConf := core.DatabaseConfig{PingTimeout: setupConnectTimeout}

	if raw := strings.TrimSpace(req.URL); raw != "" {
		dbType, err := databaseTypeFromURL(raw)
		if err != nil {
			return nil, core.DatabaseConfig{}, err
		}
		if dbType == "sqlite" {
			path := strings.TrimPrefix(strings.TrimPrefix(raw, "sqlite://"), "sqlite:")
			source["type"], source["path"] = dbType, path
			dbConf.Type, dbConf.Path = dbType, path
			return source, dbConf, nil
		}
		source["type"], source["connection_string"] = dbType, raw
		dbConf.Type, dbConf.ConnString = dbType, raw
		return source, dbConf, nil
	}

	dbType := strings.ToLower(strings.TrimSpace(req.Type))
	if dbType == "" {
		return nil, core.DatabaseConfig{}, errors.New("paste a connection URL, or choose the kind of database")
	}
	if err := core.ValidateDBType(dbType); err != nil {
		return nil, core.DatabaseConfig{}, err
	}
	source["type"] = dbType
	dbConf.Type = dbType
	if dbType == "sqlite" {
		if strings.TrimSpace(req.Path) == "" {
			return nil, core.DatabaseConfig{}, errors.New("give the path to the SQLite file")
		}
		source["path"], dbConf.Path = strings.TrimSpace(req.Path), strings.TrimSpace(req.Path)
		return source, dbConf, nil
	}
	if strings.TrimSpace(req.Host) == "" || strings.TrimSpace(req.Database) == "" {
		return nil, core.DatabaseConfig{}, errors.New("give the host and the database name")
	}
	source["host"], dbConf.Host = strings.TrimSpace(req.Host), strings.TrimSpace(req.Host)
	source["dbname"], dbConf.DBName = strings.TrimSpace(req.Database), strings.TrimSpace(req.Database)
	if req.Port > 0 {
		source["port"], dbConf.Port = req.Port, req.Port
	}
	if strings.TrimSpace(req.User) != "" {
		source["user"], dbConf.User = strings.TrimSpace(req.User), strings.TrimSpace(req.User)
	}
	if req.Password != "" {
		source["password"], dbConf.Password = req.Password, req.Password
	}
	return source, dbConf, nil
}

func databaseTypeFromURL(raw string) (string, error) {
	lower := strings.ToLower(raw)
	for prefix, dbType := range map[string]string{
		"postgres://": "postgres", "postgresql://": "postgres",
		"mysql://":     "mysql",
		"sqlserver://": "mssql", "mssql://": "mssql",
		"mongodb://": "mongodb", "mongodb+srv://": "mongodb",
		"snowflake://": "snowflake",
		"bigquery://":  "bigquery",
		"sqlite://":    "sqlite", "sqlite:": "sqlite",
	} {
		if strings.HasPrefix(lower, prefix) {
			return dbType, nil
		}
	}
	return "", errors.New("that does not look like a connection URL; it should start with something like postgres:// or mysql://")
}

// testSetupDatabase opens the candidate database and closes it again.
func (s *graphjinService) testSetupDatabase(dbConf core.DatabaseConfig) error {
	dbType := strings.ToLower(dbConf.Type)
	conf := configuredDatabaseServiceConfig(dbType, dbConf)
	fs := s.fs
	if fs == nil {
		fs = core.NewOsFS("")
	}
	driver, err := initDBDriver(conf, dbConf.ConnString == "", false, fs)
	if err != nil {
		return err
	}
	db, err := connectConfiguredDatabase(driver, conf.DB, setupConnectTimeout)
	if err != nil {
		return err
	}
	return db.Close()
}

// databaseConnectAdvice turns a driver error into what went wrong and what to
// do about it, in words a newcomer can act on. The driver's own message is
// returned beside it for whoever wants the detail.
func databaseConnectAdvice(err error, raw string) string {
	msg := strings.ToLower(err.Error())
	host := databaseHostForAdvice(raw)
	switch {
	case strings.Contains(msg, "no such host") || strings.Contains(msg, "hostname resolving error") || strings.Contains(msg, "server misbehaving"):
		return fmt.Sprintf("We could not find the host%s. Check the host name for typos.", host)
	case strings.Contains(msg, "connection refused"):
		return fmt.Sprintf("We reached%s but nothing is accepting connections on that port. Check the port, and that the database is running.", host)
	case strings.Contains(msg, "password authentication failed") || strings.Contains(msg, "access denied for user") || strings.Contains(msg, "login failed") || strings.Contains(msg, "authentication failed"):
		return "The database did not accept that user name and password. Check both."
	case strings.Contains(msg, "does not exist") && strings.Contains(msg, "database"), strings.Contains(msg, "unknown database"):
		return "The server was reached, but it has no database with that name. Check the database name."
	case strings.Contains(msg, "no pg_hba.conf entry") || strings.Contains(msg, "ssl is required") || strings.Contains(msg, "sslmode") || strings.Contains(msg, "requires ssl") || strings.Contains(msg, "insecure transport"):
		return "This database only accepts encrypted connections. Add ?sslmode=require to the end of the connection URL and try again."
	case strings.Contains(msg, "certificate"):
		return "The encrypted connection could not be verified. If you trust this database, use sslmode=require instead of verify-full."
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded") || strings.Contains(msg, "i/o timeout"):
		return fmt.Sprintf("We could not reach%s in time. Check the host and port, and that this server is allowed through the database's firewall.", host)
	case strings.Contains(msg, "placeholder"):
		return "The connection URL still has a ${...} placeholder in it. Paste the URL itself."
	case strings.Contains(msg, "unable to open database file") || strings.Contains(msg, "no such file"):
		return "We could not open that file. Check the path."
	}
	return "We could not connect to that database."
}

func databaseHostForAdvice(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" {
		return ""
	}
	return " " + u.Hostname()
}

// callerIsAdmin reports whether the request context belongs to an admin.
func (s *graphjinService) callerIsAdmin(ctx context.Context) bool {
	if ctx == nil || ctx.Value(core.UserIDKey) == nil {
		return false
	}
	role := runtimeRoleClass(ctx)
	for _, admin := range s.conf.Core.EffectiveIdentityConfig().AdminRoles {
		if strings.EqualFold(strings.TrimSpace(admin), role) {
			return true
		}
	}
	return false
}

// setupAdminContext checks the request is from a signed-in admin and writes
// the refusal when it is not.
func (s *graphjinService) setupAdminContext(w http.ResponseWriter, r *http.Request, ah auth.HandlerFunc) (context.Context, bool) {
	if !consoleSameOrigin(r) {
		writeJSONStatus(w, http.StatusForbidden, map[string]any{"error": "cross-origin setup requests are refused"})
		return nil, false
	}
	ctx, err := withConsoleSession(ah)(w, r)
	if err != nil || ctx == nil {
		writeJSONStatus(w, http.StatusUnauthorized, map[string]any{"error": "sign in as an admin first"})
		return nil, false
	}
	ctx = s.applyIdentityContext(ctx)
	if !s.callerIsAdmin(ctx) {
		writeJSONStatus(w, http.StatusForbidden, map[string]any{"error": "only an admin can change this server's setup"})
		return nil, false
	}
	return ctx, true
}

func (s *graphjinService) setupConnectDatabase(w http.ResponseWriter, r *http.Request, ah auth.HandlerFunc) {
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
	var req setupDatabaseRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
		return
	}
	if req.Sample {
		if s.sampleData == nil {
			writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "sample data is not available on this server"})
			return
		}
		path, err := s.sampleData(ctx)
		if err != nil {
			if s.log != nil {
				s.log.Errorf("setup: sample data: %s", err)
			}
			writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "The sample data could not be prepared. Connect your own database, or see the server log for why."})
			return
		}
		req = setupDatabaseRequest{Name: "sample", Type: "sqlite", Path: path}
	}
	source, dbConf, err := req.source()
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if err := s.testSetupDatabase(dbConf); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{
			"error":  databaseConnectAdvice(err, req.URL),
			"detail": redactRuntimeError(err),
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
	updated, err := storedConfigWithSource(doc, source)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if err := writeStoredConfig(ctx, s.accounts.store, s.conf.storeSealKey, updated); err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "could not save the connection"})
		return
	}
	if s.log != nil {
		s.log.Infof("setup: connected database %q (%s)", source["name"], source["type"])
	}
	// The connection is saved. It takes effect when the server comes back up
	// with it, which the console waits for.
	writeJSONStatus(w, http.StatusOK, map[string]any{"name": source["name"], "type": source["type"], "restarting": s.configApplier != nil, "boot": setupBootID})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	s.applyStoredConfigSoon()
}

// applyStoredConfigSoon makes the saved configuration take effect once the
// response that reported it has had time to leave.
func (s *graphjinService) applyStoredConfigSoon() {
	apply := s.configApplier
	if apply == nil {
		return
	}
	go func() {
		time.Sleep(400 * time.Millisecond)
		// Version based, like the check every copy runs, so a change is
		// applied once whichever of the two notices it first.
		s.applyStoredConfig(context.Background())
	}()
}

// storedConfigWithSource adds the source to the document, replacing one of
// the same name. The first database becomes the default.
func storedConfigWithSource(doc string, source map[string]any) (string, error) {
	settings, err := parseStoredConfig(doc)
	if err != nil {
		return "", err
	}
	var sources []any
	if existing, ok := settings["sources"].([]any); ok {
		sources = existing
	}
	hasDefault, replaced := false, false
	for i, item := range sources {
		current, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if strings.EqualFold(fmt.Sprint(current["name"]), fmt.Sprint(source["name"])) {
			if isDefault, _ := current["default"].(bool); isDefault {
				source["default"] = true
			}
			// Connecting a database again under the same name changes where
			// it is, not who may read its tables.
			if access, ok := current["access"]; ok {
				if _, set := source["access"]; !set {
					source["access"] = access
				}
			}
			sources[i] = source
			replaced = true
			current = source
		}
		if isDefault, _ := current["default"].(bool); isDefault {
			hasDefault = true
		}
	}
	if !replaced {
		if !hasDefault {
			source["default"] = true
		}
		sources = append(sources, source)
	}
	settings["sources"] = sources
	out, err := yaml.Marshal(settings)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
