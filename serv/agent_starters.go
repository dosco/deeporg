package serv

import (
	"sort"
	"strings"

	"github.com/dosco/graphjin/core/v3"
)

// First questions for the agent page, drawn from the tables that are
// connected. A newcomer who has just connected a database does not yet know
// what to ask; questions about their own tables show what the agent is for
// better than questions about the server do.

const (
	// agentStarterTableLimit bounds how many tables are looked at, so a
	// database with thousands of tables does not slow the page down.
	agentStarterTableLimit = 60

	agentStarterCatchAll = "What data can I ask about?"
)

// starterTable is what the suggestions need to know about one table.
type starterTable struct {
	name     string
	children []string // tables that point at this one
	dated    bool     // has a column that says when a row was made
}

// agentStarters returns up to four questions about the connected tables, or
// nil when there are none to ask about.
func (s *graphjinService) agentStarters() []string {
	var schemas []*core.TableSchema
	for _, info := range s.connectedTables() {
		if len(schemas) >= agentStarterTableLimit {
			break
		}
		schema, err := s.gj.GetTableSchemaForDatabase(info.Database, info.Name)
		if err != nil || schema == nil || schema.Blocked {
			continue
		}
		schemas = append(schemas, schema)
	}
	return starterQuestions(newStarterTables(schemas))
}

// connectedTables lists the tables of the databases that are connected. The
// server's own tables are left out: they are never something a person
// connected, or something to suggest a question about.
func (s *graphjinService) connectedTables() []core.TableInfo {
	if s == nil || s.gj == nil || s.conf == nil {
		return nil
	}
	sources := map[string]bool{}
	for _, source := range s.conf.Core.Sources {
		if source.Kind == "" || strings.EqualFold(source.Kind, "database") {
			sources[strings.ToLower(source.Name)] = true
		}
	}
	var tables []core.TableInfo
	for _, info := range s.gj.GetTables() {
		if !starterTableName(info.Name) || !strings.EqualFold(info.Type, "table") && info.Type != "" {
			continue
		}
		if len(sources) != 0 && info.Database != "" && !sources[strings.ToLower(info.Database)] {
			continue
		}
		tables = append(tables, info)
	}
	return tables
}

// starterTableName reports whether a table is one of the person's own.
func starterTableName(name string) bool {
	lower := strings.ToLower(name)
	return lower != "" && !strings.HasPrefix(lower, "_") && !strings.HasPrefix(lower, "gj_") && !strings.HasPrefix(lower, "sqlite_")
}

// newStarterTables works out, for each table, which tables point at it. That
// is read from the foreign keys themselves: a column of one table that names
// another makes the first a child of the second.
func newStarterTables(schemas []*core.TableSchema) []starterTable {
	known := map[string]bool{}
	for _, schema := range schemas {
		known[strings.ToLower(schema.Name)] = true
	}
	children := map[string][]string{}
	tables := make([]starterTable, 0, len(schemas))
	for _, schema := range schemas {
		t := starterTable{name: schema.Name}
		seen := map[string]bool{}
		for _, col := range schema.Columns {
			name, kind := strings.ToLower(col.Name), strings.ToLower(col.Type)
			if name == "created_at" || name == "created" || name == "inserted_at" ||
				(strings.Contains(kind, "time") || strings.Contains(kind, "date")) && strings.HasPrefix(name, "created") {
				t.dated = true
			}
			parent := strings.ToLower(foreignKeyTable(col.ForeignKey))
			if parent == "" || !known[parent] || seen[parent] || parent == strings.ToLower(schema.Name) {
				continue
			}
			seen[parent] = true
			children[parent] = append(children[parent], schema.Name)
		}
		tables = append(tables, t)
	}
	for i := range tables {
		tables[i].children = children[strings.ToLower(tables[i].name)]
		sort.Strings(tables[i].children)
	}
	return tables
}

// foreignKeyTable reads the table out of a foreign key written as
// "schema.table.column" or "table.column".
func foreignKeyTable(foreignKey string) string {
	parts := strings.Split(strings.TrimSpace(foreignKey), ".")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-2]
}

// starterQuestions writes the questions. It asks how many there are of the
// table most others point at, which of those has the most of something, and
// for the newest rows of a table that records when its rows were made.
func starterQuestions(tables []starterTable) []string {
	if len(tables) == 0 {
		return nil
	}
	sort.SliceStable(tables, func(i, j int) bool {
		if len(tables[i].children) != len(tables[j].children) {
			return len(tables[i].children) > len(tables[j].children)
		}
		return tables[i].name < tables[j].name
	})
	main := tables[0]
	questions := []string{"How many " + starterWords(main.name) + " are there?"}
	if len(main.children) != 0 {
		questions = append(questions, "Which "+starterWords(main.name)+" have the most "+starterWords(main.children[0])+"?")
	}
	for _, t := range tables {
		// Prefer a table the other questions have not already used.
		if t.dated && t.name != main.name && (len(main.children) == 0 || t.name != main.children[0]) {
			questions = append(questions, "What are the 5 newest "+starterWords(t.name)+"?")
			break
		}
	}
	return append(questions, agentStarterCatchAll)
}

// starterWords turns a table name into words: support_tickets is "support
// tickets".
func starterWords(name string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(name), func(r rune) bool { return r == '_' || r == '-' || r == '.' }), " ")
}
