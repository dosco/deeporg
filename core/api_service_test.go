package core

import (
	"strings"
	"testing"
)

func apiServiceSources() []SourceConfig {
	return []SourceConfig{{Name: "app", Kind: "database", Type: "sqlite", Path: "app.db", Default: true}}
}

// The API service is chosen, by name or with the production flag. A config
// that says nothing is not one.
func TestIsAPIService(t *testing.T) {
	for _, tc := range []struct {
		name string
		conf Config
		want bool
	}{
		{"mode api", Config{Mode: "api", Sources: apiServiceSources()}, true},
		{"mode prod", Config{Mode: "prod", Sources: apiServiceSources()}, true},
		{"production flag", Config{Production: true, Sources: apiServiceSources()}, true},
		{"production flag, no sources", Config{Production: true}, true},
		{"mode agentic", Config{Mode: "agentic", Sources: apiServiceSources()}, false},
		{"mode dev", Config{Mode: "dev", Sources: apiServiceSources()}, false},
		{"agentic wins over the flag", Config{Mode: "agentic", Production: true, Sources: apiServiceSources()}, false},
		{"sources and no mode", Config{Sources: apiServiceSources()}, false},
		{"nothing at all", Config{}, false},
	} {
		conf := tc.conf
		if got := conf.IsAPIService(); got != tc.want {
			t.Errorf("%s: before the mode is settled, IsAPIService = %t, want %t", tc.name, got, tc.want)
		}
		if err := conf.NormalizeMode(); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := conf.IsAPIService(); got != tc.want {
			t.Errorf("%s: after the mode is settled (%q), IsAPIService = %t, want %t", tc.name, conf.Mode, got, tc.want)
		}
	}
	if mode, err := CanonicalMode("API"); err != nil || mode != "prod" {
		t.Fatalf(`mode "api" is %q (%v), want the same mode as "prod"`, mode, err)
	}
}

// The API service declares its database under sources and limits data with
// its own role rules, as it always has. The agent server's access block has
// no meaning there, and writing one is an error rather than a rule that is
// quietly not applied.
func TestAPIServiceKeepsItsRoleRulesWithSources(t *testing.T) {
	rules := []Role{{Name: "user", Tables: []RoleTable{{Name: "orders", Query: &Query{Filters: []string{"{ user_id: { eq: $user_id } }"}}}}}}

	api := Config{Mode: "api", Sources: apiServiceSources(), Roles: rules}
	if err := api.NormalizeMode(); err != nil {
		t.Fatal(err)
	}
	if err := api.NormalizeSources(); err != nil {
		t.Fatalf("the API service refused its role rules: %v", err)
	}
	if api.generatesSourceAccess() {
		t.Fatal("the API service generates access rules")
	}
	if got := api.Sources[0].Access; !got.isUnset() {
		t.Fatalf("the API service filled in an access block nobody wrote: %+v", got)
	}
	if _, ok := api.Databases["app"]; !ok {
		t.Fatalf("the source was not turned into a database connection: %+v", api.Databases)
	}

	// Table entries written before sources existed name a database, or
	// nothing. Both still work.
	tables := Config{Mode: "api", Sources: apiServiceSources(), Roles: rules,
		Tables: []Table{{Name: "me", Table: "users"}, {Name: "shop_orders", Table: "orders", Database: "app"}}}
	_ = tables.NormalizeMode()
	if err := tables.NormalizeSources(); err != nil {
		t.Fatalf("the API service refused its table entries: %v", err)
	}
	for _, table := range tables.Tables {
		if table.Database != "app" {
			t.Fatalf("table %q was not placed in the database: %+v", table.Name, table)
		}
	}

	withAccess := Config{Mode: "api", Sources: apiServiceSources(), Roles: rules}
	withAccess.Sources[0].Access.Read = "account"
	_ = withAccess.NormalizeMode()
	if err := withAccess.NormalizeSources(); err == nil || !strings.Contains(err.Error(), "roles[].tables") {
		t.Fatalf("an access block in the API service gave %v, want an error pointing at roles[].tables", err)
	}

	// The agent server is unchanged: it generates its rules and refuses
	// hand-written ones.
	agent := Config{Mode: "agentic", Sources: apiServiceSources(), Roles: rules}
	_ = agent.NormalizeMode()
	if err := agent.NormalizeSources(); err == nil || !strings.Contains(err.Error(), "sources[].access") {
		t.Fatalf("the agent server accepted hand-written role rules: %v", err)
	}
	generated := Config{Mode: "agentic", Sources: apiServiceSources()}
	_ = generated.NormalizeMode()
	if err := generated.NormalizeSources(); err != nil || !generated.generatesSourceAccess() || generated.Sources[0].Access.Read != AccessModeAuthenticated {
		t.Fatalf("the agent server does not generate its access rules: %v, %+v", err, generated.Sources[0].Access)
	}
}
