package serv

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/dosco/graphjin/auth/v3"
	"github.com/dosco/graphjin/core/v3"
	"gopkg.in/yaml.v3"
)

// Who can read a table.
//
// On a server set up in the browser everyone who signs in reads every table
// of the databases that are connected. An admin narrows that one table at a
// time: a table is for everyone, for admins only, or for nobody. The choice
// is kept in the stored configuration as the source's admin_tables and
// blocked_tables, which the engine already enforces for every query and
// leaves out of the catalog the agent reads. Nothing here decides access; it
// only writes the two lists.

const (
	routeSetupAccess = "/api/v1/setup/access"

	tableForEveryone = "everyone"
	tableForAdmins   = "admins"
	tableForNobody   = "nobody"
)

// tableAccessName is the name a table goes under in the two lists: its own
// name, or schema.name when another schema of the same database has a table
// of that name.
func tableAccessName(table core.TableInfo, clash map[string]int) string {
	if clash[strings.ToLower(table.Name)] > 1 && table.Schema != "" {
		return table.Schema + "." + table.Name
	}
	return table.Name
}

// tableListed reports whether a list of the stored configuration names the
// table, by its name or by schema.name.
func tableListed(list []string, table core.TableInfo) bool {
	for _, item := range list {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == strings.ToLower(table.Name) || (table.Schema != "" && item == strings.ToLower(table.Schema+"."+table.Name)) {
			return true
		}
	}
	return false
}

// databaseTables lists the tables of one connected database with who can
// read each, for the admin's setup page.
func (s *graphjinService) databaseTables(source core.SourceConfig) []map[string]any {
	var tables []core.TableInfo
	clash := map[string]int{}
	for _, table := range s.connectedTables() {
		if strings.EqualFold(table.Database, source.Name) {
			tables = append(tables, table)
			clash[strings.ToLower(table.Name)]++
		}
	}
	access := s.conf.Core.EffectiveSourceAccess(source)
	out := make([]map[string]any, 0, len(tables))
	for _, table := range tables {
		who := tableForEveryone
		switch {
		case tableListed(access.BlockedTables, table):
			who = tableForNobody
		case tableListed(access.AdminTables, table):
			who = tableForAdmins
		}
		out = append(out, map[string]any{"name": tableAccessName(table, clash), "access": who})
	}
	// A table that is for nobody is left out of the schema the engine works
	// with, so it is not among the tables found. It is still listed here from
	// the setting itself, or the admin could never open it again.
	listed := func(name string) bool {
		for _, row := range out {
			if strings.EqualFold(row["name"].(string), name) {
				return true
			}
		}
		return false
	}
	for who, names := range map[string][]string{tableForNobody: access.BlockedTables, tableForAdmins: access.AdminTables} {
		for _, name := range names {
			if name = strings.TrimSpace(name); name != "" && !listed(name) {
				out = append(out, map[string]any{"name": name, "access": who})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i]["name"].(string) < out[j]["name"].(string) })
	return out
}

// setupAccess saves who can read each table of one database. The body names
// every table that is not for everyone; a table it leaves out is for everyone.
func (s *graphjinService) setupAccess(w http.ResponseWriter, r *http.Request, ah auth.HandlerFunc) {
	if !s.storeBacked() {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "this server is set up through its config files"})
		return
	}
	ctx, ok := s.setupAdminContext(w, r, ah)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Database string            `json:"database"`
		Tables   map[string]string `json:"tables"`
	}
	if !decodeSetupJSON(w, r, &req) {
		return
	}
	var source *core.SourceConfig
	for i := range s.conf.Core.Sources {
		candidate := &s.conf.Core.Sources[i]
		if strings.EqualFold(candidate.Name, strings.TrimSpace(req.Database)) && (candidate.Kind == "" || strings.EqualFold(candidate.Kind, "database")) {
			source = candidate
		}
	}
	if source == nil {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "no database of that name is connected"})
		return
	}
	known := map[string]bool{}
	for _, table := range s.databaseTables(*source) {
		known[strings.ToLower(table["name"].(string))] = true
	}
	var admins, nobody []string
	for name, who := range req.Tables {
		name = strings.TrimSpace(name)
		if !known[strings.ToLower(name)] {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("the database %s has no table named %s", source.Name, name)})
			return
		}
		switch strings.ToLower(strings.TrimSpace(who)) {
		case tableForAdmins:
			admins = append(admins, name)
		case tableForNobody:
			nobody = append(nobody, name)
		case tableForEveryone, "":
		default:
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "a table is for everyone, admins or nobody"})
			return
		}
	}
	sort.Strings(admins)
	sort.Strings(nobody)

	lock := s.configLocker()
	lock.Lock()
	defer lock.Unlock()
	doc, err := readStoredConfig(ctx, s.accounts.store, s.conf.storeSealKey)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "could not read the stored configuration"})
		return
	}
	updated, err := storedConfigWithTableAccess(doc, source.Name, admins, nobody)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if err := writeStoredConfig(ctx, s.accounts.store, s.conf.storeSealKey, updated); err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": "could not save the change"})
		return
	}
	if s.log != nil {
		s.log.Infof("setup: database %q now has %d tables for admins only and %d for nobody", source.Name, len(admins), len(nobody))
	}
	// The rules are compiled when the server starts, so the change takes
	// effect when it comes back up, which the console waits for.
	writeJSONStatus(w, http.StatusOK, map[string]any{"database": source.Name, "restarting": s.configApplier != nil, "boot": setupBootID})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	s.applyStoredConfigSoon()
}

// storedConfigWithTableAccess sets the two table lists of one source in the
// document and leaves every other access setting of that source as it was.
func storedConfigWithTableAccess(doc, database string, admins, nobody []string) (string, error) {
	settings, err := parseStoredConfig(doc)
	if err != nil {
		return "", err
	}
	sources, _ := settings["sources"].([]any)
	found := false
	for _, item := range sources {
		source, ok := item.(map[string]any)
		if !ok || !strings.EqualFold(fmt.Sprint(source["name"]), database) {
			continue
		}
		found = true
		access, _ := source["access"].(map[string]any)
		if access == nil {
			access = map[string]any{}
		}
		setList := func(key string, names []string) {
			if len(names) == 0 {
				delete(access, key)
				return
			}
			access[key] = names
		}
		setList("admin_tables", admins)
		setList("blocked_tables", nobody)
		if len(access) == 0 {
			delete(source, "access")
		} else {
			source["access"] = access
		}
	}
	if !found {
		return "", fmt.Errorf("the stored configuration has no database named %s", database)
	}
	out, err := yaml.Marshal(settings)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
