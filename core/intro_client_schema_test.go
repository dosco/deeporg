package core

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/dosco/graphjin/core/v3/internal/sdata"
)

// The console's Workbench hands this introspection result to GraphiQL, which
// calls graphql-js buildClientSchema. That call refuses the whole schema if a
// single field, argument or input field names a type the result never
// defines, so one dangling name blanks the editor for every table.

func TestIntrospectionBuildsClientSchema(t *testing.T) {
	result := introspectRaw(t, sdata.GetTestDBInfo(), nil)
	if err := checkClientSchema(result); err != nil {
		t.Fatal(err)
	}
}

// An API source joined onto a table (the saas-ops demo's account_health) and
// a top-level API list are remote tables with no where type. Their fields
// used to take includeIf/skipIf typed as account_healthWhereInput anyway.
func TestIntrospectionWithAPIRemotesBuildsClientSchema(t *testing.T) {
	di := sdata.GetTestDBInfo()

	joined := sdata.NewDBTable("public", "account_health", "remote", []sdata.DBColumn{
		{Schema: "public", Table: "account_health", Name: "account_id", Type: "bigint"},
		{Schema: "public", Table: "account_health", Name: "health", Type: "text"},
		{Schema: "public", Table: "account_health", Name: "open_risks", Type: "json"},
	})
	idcol := sdata.DBColumn{
		PrimaryKey: true, Schema: "public", Table: "account_health",
		Name: "__account_health_id", Type: "bigint",
		FKeySchema: "public", FKeyTable: "users", FKeyCol: "id",
	}
	joined.PrimaryCols = []sdata.DBColumn{idcol}
	joined.PrimaryCol = idcol
	joined.StrictColumns = true
	di.AddTable(joined)

	list := sdata.NewDBTable("public", "incidents_api", "remote", []sdata.DBColumn{
		{Schema: "public", Table: "incidents_api", Name: "id", Type: "text"},
		{Schema: "public", Table: "incidents_api", Name: "severity", Type: "text"},
	})
	list.Args = []sdata.DBColumn{
		{Schema: "public", Table: "incidents_api", Name: "status", Type: "text"},
	}
	di.AddTable(list)

	mutation := sdata.NewDBTable("public", "create_incident", "openapi_mutation", []sdata.DBColumn{
		{Schema: "public", Table: "create_incident", Name: "id", Type: "text"},
	})
	di.AddTable(mutation)

	result := introspectRaw(t, di, map[string]bool{"create_incident": true})
	if err := checkClientSchema(result); err != nil {
		t.Fatal(err)
	}

	var intro IntroResult
	if err := json.Unmarshal(result, &intro); err != nil {
		t.Fatal(err)
	}
	// The fix must not cost the remote tables their fields or their roots.
	health := requireIntroType(t, intro, "account_health")
	requireIntroField(t, health, "health")
	query := requireIntroType(t, intro, "Query")
	requireIntroField(t, query, "account_health")
	requireIntroField(t, query, "incidents_api")
	mut := requireIntroType(t, intro, "Mutation")
	requireIntroField(t, mut, "create_incident")
}

// The control store's internal role is in the role map but is not a valid
// enum value; neither is a role name with a dash. Valid roles stay listed.
func TestIntrospectionRolesEnumBuildsClientSchema(t *testing.T) {
	schema, err := sdata.NewDBSchema(sdata.GetTestDBInfo(), nil)
	if err != nil {
		t.Fatal(err)
	}
	gj := &graphjinEngine{
		conf: &Config{DBType: "postgres"},
		roles: map[string]*Role{
			"user":                      {Name: "user"},
			"anon":                      {Name: "anon"},
			"__graphjin_internal_store": {Name: "__graphjin_internal_store"},
			"support-agent":             {Name: "support-agent"},
		},
		defaultDB: "default",
		databases: map[string]*dbContext{
			"default": {name: "default", schema: schema},
		},
	}
	result, err := gj.introQuery()
	if err != nil {
		t.Fatal(err)
	}
	if err := checkClientSchema(result); err != nil {
		t.Fatal(err)
	}
	var intro IntroResult
	if err := json.Unmarshal(result, &intro); err != nil {
		t.Fatal(err)
	}
	roles := requireIntroType(t, intro, "rolesEnum")
	var got []string
	for _, v := range roles.EnumValues {
		got = append(got, v.Name)
	}
	if strings.Join(got, ",") != "anon,user" {
		t.Fatalf("rolesEnum values = %v, want [anon user]", got)
	}
}

// A filesystem remote defines a where type, so its fields keep includeIf.
func TestIntrospectionFilesystemRemoteKeepsFieldConditions(t *testing.T) {
	di := sdata.GetTestDBInfo()
	fs := sdata.NewDBTable("public", "avatars", "remote", []sdata.DBColumn{
		{Schema: "public", Table: "avatars", Name: "key", Type: "text"},
		{Schema: "public", Table: "avatars", Name: "url", Type: "text"},
	})
	fs.Args = []sdata.DBColumn{
		{Schema: "public", Table: "avatars", Name: "prefix", Type: "text"},
		{Schema: "public", Table: "avatars", Name: "inline_data", Type: "boolean"},
	}
	di.AddTable(fs)

	result := introspectRaw(t, di, nil)
	if err := checkClientSchema(result); err != nil {
		t.Fatal(err)
	}
	var intro IntroResult
	if err := json.Unmarshal(result, &intro); err != nil {
		t.Fatal(err)
	}
	avatars := requireIntroType(t, intro, "avatars")
	key := requireIntroField(t, avatars, "key")
	if !hasIntroArg(key.Args, "includeIf") {
		t.Fatal("avatars.key lost its includeIf argument")
	}
}

func introspectRaw(t *testing.T, di *sdata.DBInfo, visibleMutations map[string]bool) json.RawMessage {
	t.Helper()
	schema, err := sdata.NewDBSchema(di, nil)
	if err != nil {
		t.Fatal(err)
	}
	gj := &graphjinEngine{
		conf:      &Config{DBType: "postgres"},
		roles:     make(map[string]*Role),
		defaultDB: "default",
		databases: map[string]*dbContext{
			"default": {name: "default", schema: schema},
		},
	}
	if visibleMutations == nil {
		visibleMutations = map[string]bool{}
	}
	result, err := gj.introQueryWithVisibleOpenAPIMutations(visibleMutations)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func hasIntroArg(args []InputValue, name string) bool {
	for _, a := range args {
		if a.Name == name {
			return true
		}
	}
	return false
}

// checkClientSchema applies the checks graphql-js buildClientSchema makes
// before it will build a schema from an introspection result: every named
// type reference resolves to a defined type, argument and input-field
// positions name input types, field positions name output types, and the
// root operation types are defined objects. It decodes the raw JSON so it
// sees exactly what GraphiQL receives.
func checkClientSchema(raw json.RawMessage) error {
	var res struct {
		Schema struct {
			Types []struct {
				Kind   string `json:"kind"`
				Name   string `json:"name"`
				Fields []struct {
					Name string           `json:"name"`
					Args []clientInputVal `json:"args"`
					Type *clientTypeRef   `json:"type"`
				} `json:"fields"`
				InputFields []clientInputVal `json:"inputFields"`
				EnumValues  []struct {
					Name string `json:"name"`
				} `json:"enumValues"`
				Interfaces    []clientTypeRef `json:"interfaces"`
				PossibleTypes []clientTypeRef `json:"possibleTypes"`
			} `json:"types"`
			QueryType        *struct{ Name string } `json:"queryType"`
			MutationType     *struct{ Name string } `json:"mutationType"`
			SubscriptionType *struct{ Name string } `json:"subscriptionType"`
			Directives       []struct {
				Name string           `json:"name"`
				Args []clientInputVal `json:"args"`
			} `json:"directives"`
		} `json:"__schema"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return err
	}

	kinds := make(map[string]string, len(res.Schema.Types))
	for _, ty := range res.Schema.Types {
		if ty.Name == "" {
			return fmt.Errorf("a %s type has no name", ty.Kind)
		}
		if _, dup := kinds[ty.Name]; dup {
			return fmt.Errorf("type %q is defined twice", ty.Name)
		}
		kinds[ty.Name] = ty.Kind
	}

	var problems []string
	check := func(where string, tr *clientTypeRef, input bool) {
		name, err := tr.namedType()
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", where, err))
			return
		}
		kind, ok := kinds[name]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s: unknown type %s", where, name))
			return
		}
		switch {
		case input && kind != "SCALAR" && kind != "ENUM" && kind != "INPUT_OBJECT":
			problems = append(problems, fmt.Sprintf("%s: %s is %s, not an input type", where, name, kind))
		case !input && kind == "INPUT_OBJECT":
			problems = append(problems, fmt.Sprintf("%s: %s is an input type in an output position", where, name))
		}
	}

	// validateSchema (which GraphiQL runs right after buildClientSchema)
	// rejects names outside the grammar and names in the reserved "__"
	// namespace, which only the built-in introspection types may use.
	checkName := func(where, name string) {
		if !clientNameRe.MatchString(name) {
			problems = append(problems, fmt.Sprintf("%s: %q is not a valid GraphQL name", where, name))
		} else if strings.HasPrefix(name, "__") {
			problems = append(problems, fmt.Sprintf("%s: %q uses the reserved \"__\" prefix", where, name))
		}
	}

	for _, ty := range res.Schema.Types {
		if !strings.HasPrefix(ty.Name, "__") {
			checkName("type", ty.Name)
		}
		for _, f := range ty.Fields {
			if !strings.HasPrefix(ty.Name, "__") {
				checkName(ty.Name, f.Name)
			}
			check(ty.Name+"."+f.Name, f.Type, false)
			for _, a := range f.Args {
				checkName(ty.Name+"."+f.Name, a.Name)
				check(fmt.Sprintf("%s.%s(%s)", ty.Name, f.Name, a.Name), a.Type, true)
			}
		}
		for _, f := range ty.InputFields {
			checkName(ty.Name, f.Name)
			check(ty.Name+"."+f.Name, f.Type, true)
		}
		for _, v := range ty.EnumValues {
			if !strings.HasPrefix(ty.Name, "__") {
				checkName(ty.Name, v.Name)
			}
			if v.Name == "true" || v.Name == "false" || v.Name == "null" {
				problems = append(problems, fmt.Sprintf("%s: enum value %q is not allowed", ty.Name, v.Name))
			}
		}
		for i := range ty.Interfaces {
			check(ty.Name+" implements", &ty.Interfaces[i], false)
		}
		for i := range ty.PossibleTypes {
			check(ty.Name+" possible type", &ty.PossibleTypes[i], false)
		}
	}
	for _, d := range res.Schema.Directives {
		for _, a := range d.Args {
			check(fmt.Sprintf("@%s(%s)", d.Name, a.Name), a.Type, true)
		}
	}

	roots := []struct {
		op  string
		ref *struct{ Name string }
	}{
		{"query", res.Schema.QueryType},
		{"mutation", res.Schema.MutationType},
		{"subscription", res.Schema.SubscriptionType},
	}
	for _, r := range roots {
		if r.ref == nil {
			if r.op == "query" {
				problems = append(problems, "no query root type")
			}
			continue
		}
		if kinds[r.ref.Name] != "OBJECT" {
			problems = append(problems, fmt.Sprintf("%s root %q is not a defined object type", r.op, r.ref.Name))
		}
	}

	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	if len(problems) > 20 {
		problems = append(problems[:20], fmt.Sprintf("... and %d more", len(problems)-20))
	}
	msg := "introspection result cannot build a client schema:"
	for _, p := range problems {
		msg += "\n  " + p
	}
	return fmt.Errorf("%s", msg)
}

var clientNameRe = regexp.MustCompile(`^[_A-Za-z][_0-9A-Za-z]*$`)

type clientInputVal struct {
	Name string         `json:"name"`
	Type *clientTypeRef `json:"type"`
}

type clientTypeRef struct {
	Kind   string         `json:"kind"`
	Name   *string        `json:"name"`
	OfType *clientTypeRef `json:"ofType"`
}

// namedType unwraps LIST and NON_NULL down to the named type, the way
// graphql-js getNamedType does when it resolves a reference.
func (tr *clientTypeRef) namedType() (string, error) {
	for depth := 0; tr != nil; depth++ {
		if depth > 8 {
			return "", fmt.Errorf("type reference nests too deep")
		}
		switch tr.Kind {
		case "LIST", "NON_NULL":
			if tr.OfType == nil {
				return "", fmt.Errorf("%s wrapper has no ofType", tr.Kind)
			}
			tr = tr.OfType
		default:
			if tr.Name == nil || *tr.Name == "" {
				return "", fmt.Errorf("named type reference has no name")
			}
			return *tr.Name, nil
		}
	}
	return "", fmt.Errorf("missing type reference")
}
