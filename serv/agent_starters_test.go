package serv

import (
	"reflect"
	"testing"

	"github.com/dosco/graphjin/core/v3"
)

// Which table is the child is read from the foreign keys, not from how the
// engine labels a relationship: with those labels the sample data's first
// question was "Which invoices have the most accounts?".
func TestStarterTablesReadParentsFromForeignKeys(t *testing.T) {
	tables := newStarterTables([]*core.TableSchema{
		{Name: "accounts", Columns: []core.ColumnInfo{{Name: "id"}, {Name: "created_at", Type: "text"}}},
		{Name: "invoices", Columns: []core.ColumnInfo{{Name: "account_id", ForeignKey: "main.accounts.id"}, {Name: "issued_at", Type: "timestamp"}}},
		{Name: "users", Columns: []core.ColumnInfo{{Name: "account_id", ForeignKey: "accounts.id"}, {Name: "manager_id", ForeignKey: "main.users.id"}, {Name: "created_on", Type: "date"}}},
		// A key to a table that is not connected is not a link to follow.
		{Name: "payments", Columns: []core.ColumnInfo{{Name: "ledger_id", ForeignKey: "main.ledgers.id"}}},
	})
	got := map[string]starterTable{}
	for _, table := range tables {
		got[table.name] = table
	}
	if want := []string{"invoices", "users"}; !reflect.DeepEqual(got["accounts"].children, want) {
		t.Fatalf("accounts has children %q, want %q", got["accounts"].children, want)
	}
	if len(got["invoices"].children) != 0 || len(got["users"].children) != 0 || len(got["payments"].children) != 0 {
		t.Fatalf("a child table was given children: %+v", got)
	}
	if !got["accounts"].dated || got["invoices"].dated || !got["users"].dated {
		t.Fatalf("the tables that record when a row was made were misread: %+v", got)
	}
	questions := starterQuestions(tables)
	if questions[0] != "How many accounts are there?" || questions[1] != "Which accounts have the most invoices?" {
		t.Fatalf("got %q", questions)
	}
}

func TestStarterQuestionsComeFromTheConnectedTables(t *testing.T) {
	tables := []starterTable{
		{name: "invoices", dated: true},
		{name: "support_tickets", dated: true},
		{name: "accounts", children: []string{"invoices", "users"}},
		{name: "users", children: []string{"support_tickets"}},
	}
	got := starterQuestions(tables)
	want := []string{
		"How many accounts are there?",
		"Which accounts have the most invoices?",
		// invoices is already asked about, so the newest rows come from elsewhere.
		"What are the 5 newest support tickets?",
		"What data can I ask about?",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestStarterQuestionsWithLittleToGoOn(t *testing.T) {
	// One table with no links and no dates still gets a question of its own.
	got := starterQuestions([]starterTable{{name: "Line_Items"}})
	if want := []string{"How many line items are there?", "What data can I ask about?"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	// No tables, no suggestions: the page keeps its own.
	if got := starterQuestions(nil); got != nil {
		t.Fatalf("got %q for no tables", got)
	}
}

func TestStarterTableNameLeavesOutTheServersOwnTables(t *testing.T) {
	for name, want := range map[string]bool{
		"accounts":                true,
		"_graphjin_auth_sessions": false,
		"gj_artifacts":            false,
		"sqlite_sequence":         false,
		"":                        false,
	} {
		if got := starterTableName(name); got != want {
			t.Errorf("starterTableName(%q) = %t, want %t", name, got, want)
		}
	}
}
