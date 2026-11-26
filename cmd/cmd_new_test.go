package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dosco/graphjin/serv/v3"
	"github.com/spf13/afero"
	"go.uber.org/zap"
)

func TestCmdNewWritesAgenticAndSourcesTemplates(t *testing.T) {
	log = zap.NewNop().Sugar()
	dbURL = ""
	t.Chdir(t.TempDir())

	cmdNew(nil, []string{"myapp"})

	for _, name := range []string{"dev.yml", "prod.yml", "agentic.yml"} {
		p := filepath.Join("myapp", name)
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		content := string(b)
		// Every scaffolded config declares its database under sources. A
		// config file without them is no longer read.
		if !strings.Contains(content, "\nsources:\n") {
			t.Fatalf("%s does not use sources config:\n%s", name, content)
		}
		if strings.Contains(content, "kind: graphjin") || strings.Contains(content, "kind: workflow") {
			t.Fatalf("%s contains a removed internal source kind:\n%s", name, content)
		}
		if strings.Contains(content, "\ndatabases:\n") || strings.Contains(content, "\ndatabase:\n") {
			t.Fatalf("%s contains active legacy database config:\n%s", name, content)
		}
		// dev.yml and prod.yml are an API service. Settings for the agent,
		// the catalog and MCP do nothing there, so they are not written.
		agentSettings := strings.Contains(content, "\ndiscovery_cache:\n") && strings.Contains(content, "\ncatalog_search:\n")
		if name == "agentic.yml" && !agentSettings {
			t.Fatalf("%s missing discovery cache or semantic catalog config:\n%s", name, content)
		}
		if name != "agentic.yml" {
			if !strings.Contains(content, "\nmode: api\n") {
				t.Fatalf("%s is not an API service config:\n%s", name, content)
			}
			for _, inert := range []string{"\nmcp:\n", "\ndiscovery_cache:\n", "\ncatalog_search:\n", "\nagent:\n"} {
				if strings.Contains(content, inert) {
					t.Fatalf("%s carries %q, which does nothing in an API service:\n%s", name, strings.TrimSpace(inert), content)
				}
			}
		}
	}

	dev, err := os.ReadFile(filepath.Join("myapp", "dev.yml"))
	if err != nil {
		t.Fatalf("read dev.yml: %v", err)
	}
	resolvedDev, err := serv.NewConfig(string(dev), "yaml")
	if err != nil {
		t.Fatalf("resolve generated dev.yml: %v", err)
	}
	assertAPIServiceDefaults(t, "dev", resolvedDev)
	if resolvedDev.Serv.Production {
		t.Fatal("generated dev.yml is locked to saved queries; it should save them while building")
	}

	agentic, err := os.ReadFile(filepath.Join("myapp", "agentic.yml"))
	if err != nil {
		t.Fatalf("read agentic.yml: %v", err)
	}
	if !strings.Contains(string(agentic), "mode: agentic") {
		t.Fatalf("agentic.yml missing mode: agentic:\n%s", string(agentic))
	}
	if strings.Contains(string(agentic), "allow_raw_graphql") {
		t.Fatalf("agentic.yml ships removed agent knob allow_raw_graphql:\n%s", string(agentic))
	}
	for _, redundant := range []string{"\nagent:\n", "sampling:", "http_stateful:", "include_tools_with_agent:", "artifacts:\n  enabled:", "watches:\n  enabled:"} {
		if strings.Contains(string(agentic), redundant) {
			t.Fatalf("agentic.yml contains redundant default %q:\n%s", redundant, string(agentic))
		}
	}
	resolved, err := serv.NewConfig(string(agentic), "yaml")
	if err != nil {
		t.Fatalf("resolve generated agentic.yml: %v", err)
	}
	assertGeneratedRuntimeDefaults(t, "agentic", resolved)
}

// assertAPIServiceDefaults fails if anything agentic is on in an API service
// config.
func assertAPIServiceDefaults(t *testing.T, name string, conf *serv.Config) {
	t.Helper()
	if !conf.Core.IsAPIService() {
		t.Fatalf("generated %s config is not an API service: mode %q", name, conf.Core.Mode)
	}
	if conf.Agent.Enabled || conf.MCP.IncludeToolsWithAgent || conf.Core.Artifacts.Enabled || conf.Core.Watches.Enabled {
		t.Fatalf("generated %s config turned on something agentic: agent=%+v mcp=%+v artifacts=%+v watches=%+v", name, conf.Agent, conf.MCP, conf.Core.Artifacts, conf.Core.Watches)
	}
}

func assertGeneratedRuntimeDefaults(t *testing.T, mode string, conf *serv.Config) {
	t.Helper()
	if !conf.Agent.Enabled || !conf.MCP.IncludeToolsWithAgent || !conf.Core.Artifacts.Enabled || !conf.Core.Watches.Enabled || conf.Core.Watches.Runner != "all" {
		t.Fatalf("generated %s config did not resolve complete runtime: agent=%+v mcp=%+v artifacts=%+v watches=%+v", mode, conf.Agent, conf.MCP, conf.Core.Artifacts, conf.Core.Watches)
	}
}

func TestTemplatesDecodeAsConfig(t *testing.T) {
	tmpl := newTempl(map[string]any{
		"AppName":     "MyApp",
		"AppNameSlug": "myapp",
		"DBType":      "postgres",
		"DBHost":      "db",
		"DBPort":      "5432",
		"DBUser":      "postgres",
		"DBPass":      "postgres",
		"DBName":      "myapp",
	})
	for _, name := range []string{"dev.yml", "prod.yml", "agentic.yml"} {
		b, err := tmpl.get(name)
		if err != nil {
			t.Fatalf("render %s: %v", name, err)
		}
		if _, err := serv.NewConfig(string(b), "yaml"); err != nil {
			t.Fatalf("decode %s: %v", name, err)
		}
	}
}

func TestRenderedTemplatesReadWithInheritance(t *testing.T) {
	tmpl := newTempl(map[string]any{
		"AppName":     "MyApp",
		"AppNameSlug": "myapp",
		"DBType":      "postgres",
		"DBHost":      "db",
		"DBPort":      "5432",
		"DBUser":      "postgres",
		"DBPass":      "postgres",
		"DBName":      "myapp",
	})
	fs := afero.NewMemMapFs()
	for _, name := range []string{"dev.yml", "prod.yml", "agentic.yml"} {
		b, err := tmpl.get(name)
		if err != nil {
			t.Fatalf("render %s: %v", name, err)
		}
		if err := afero.WriteFile(fs, "/"+name, b, 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	for _, name := range []string{"dev.yml", "prod.yml", "agentic.yml"} {
		conf, err := serv.ReadInConfigFS("/"+name, fs)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if err := validateCoreOffline(conf); err != nil {
			t.Fatalf("validate %s: %v", name, err)
		}
		seen := map[string]bool{}
		for _, source := range conf.Core.Sources {
			if seen[source.Name] {
				t.Fatalf("%s has duplicate source %q after inheritance: %+v", name, source.Name, conf.Core.Sources)
			}
			seen[source.Name] = true
		}
		if name == "agentic.yml" {
			assertGeneratedRuntimeDefaults(t, name, conf)
		} else {
			assertAPIServiceDefaults(t, name, conf)
			// dev saves queries as you build; prod serves only the saved ones.
			if locked := conf.Serv.Production; locked != (name == "prod.yml") {
				t.Fatalf("generated %s has production=%t", name, locked)
			}
		}
	}
}

func TestCmdNewUsesDatabaseURLInSourceTemplates(t *testing.T) {
	log = zap.NewNop().Sugar()
	oldDBURL := dbURL
	dbURL = "postgres://alice:p%40ss@db.example:5544/acme"
	t.Cleanup(func() { dbURL = oldDBURL })
	t.Chdir(t.TempDir())

	cmdNew(nil, []string{"myapp"})

	for _, name := range []string{"prod.yml", "agentic.yml"} {
		b, err := os.ReadFile(filepath.Join("myapp", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if strings.Contains(string(b), "${GJ_DATABASE_") {
			t.Fatalf("%s contains an unresolved database placeholder:\n%s", name, string(b))
		}

		resolved, err := serv.NewConfig(string(b), "yaml")
		if err != nil {
			t.Fatalf("resolve %s: %v", name, err)
		}
		var sourceFound bool
		for _, source := range resolved.Core.Sources {
			if source.Name != "default" {
				continue
			}
			sourceFound = true
			if source.Type != "postgres" || source.Host != "db.example" || source.Port != 5544 || source.DBName != "acme" || source.User != "alice" || source.Password != "p@ss" {
				t.Fatalf("%s did not preserve --db-url values: %+v", name, source)
			}
		}
		if !sourceFound {
			t.Fatalf("%s missing default database source", name)
		}
	}
}

func TestCmdNewUsesMySQLURLDefaults(t *testing.T) {
	log = zap.NewNop().Sugar()
	oldDBURL := dbURL
	dbURL = "mysql://db.example/inventory"
	t.Cleanup(func() { dbURL = oldDBURL })
	t.Chdir(t.TempDir())

	cmdNew(nil, []string{"myapp"})

	b, err := os.ReadFile(filepath.Join("myapp", "agentic.yml"))
	if err != nil {
		t.Fatalf("read agentic.yml: %v", err)
	}
	resolved, err := serv.NewConfig(string(b), "yaml")
	if err != nil {
		t.Fatalf("resolve agentic.yml: %v", err)
	}
	for _, source := range resolved.Core.Sources {
		if source.Name == "default" {
			if source.Type != "mysql" || source.Host != "db.example" || source.Port != 3306 || source.DBName != "inventory" || source.User != "root" || source.Password != "" {
				t.Fatalf("agentic.yml did not apply MySQL URL defaults: %+v", source)
			}
			return
		}
	}
	t.Fatal("agentic.yml missing default database source")
}

func TestSetupCreatesTemplateSet(t *testing.T) {
	log = zap.NewNop().Sugar()
	oldConf := conf
	oldCpath := cpath
	defer func() {
		conf = oldConf
		cpath = oldCpath
	}()
	conf = nil
	t.Setenv("GO_ENV", "dev")

	dir := filepath.Join(t.TempDir(), "config")
	setup(dir)

	for _, name := range []string{"dev.yml", "prod.yml", "agentic.yml"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("expected setup to create %s: %v", name, err)
		}
	}
	// The files it writes are an API service project.
	assertAPIServiceDefaults(t, "setup dev", conf)
}

func TestTemplatesRenderSourcesMode(t *testing.T) {
	tmpl := newTempl(map[string]any{
		"AppName":     "MyApp",
		"AppNameSlug": "myapp",
		"DBType":      "postgres",
		"DBHost":      "db",
		"DBPort":      "5432",
		"DBUser":      "postgres",
		"DBPass":      "postgres",
		"DBName":      "myapp",
	})

	for _, name := range []string{"dev.yml", "prod.yml", "agentic.yml"} {
		b, err := tmpl.get(name)
		if err != nil {
			t.Fatalf("render %s: %v", name, err)
		}
		content := string(b)
		if !strings.Contains(content, "\nsources:\n") {
			t.Fatalf("%s missing sources:\n%s", name, content)
		}
		if strings.Contains(content, "kind: graphjin") || strings.Contains(content, "kind: workflow") {
			t.Fatalf("%s contains a removed internal source kind:\n%s", name, content)
		}
		if strings.Contains(content, "\ndatabases:\n") || strings.Contains(content, "\ndatabase:\n") {
			t.Fatalf("%s contains active legacy database config:\n%s", name, content)
		}
	}
}
