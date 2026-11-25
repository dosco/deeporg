package serv

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoredConfigRoundTripsThroughTheStoreFile(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nested", "store.db")

	// A new store has no document and is created on first read.
	if doc, err := ReadStoredConfig(ctx, path); err != nil || doc != "" {
		t.Fatalf("new store: doc=%q err=%v", doc, err)
	}
	const doc = "sources:\n  - name: shop\n    kind: database\n    type: sqlite\n"
	if err := WriteStoredConfig(ctx, path, doc); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got, err := ReadStoredConfig(ctx, path); err != nil || got != doc {
		t.Fatalf("read back: %q err=%v", got, err)
	}
	// Writing again replaces the document.
	if err := WriteStoredConfig(ctx, path, "sources: []\n"); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if got, _ := ReadStoredConfig(ctx, path); got != "sources: []\n" {
		t.Fatalf("after rewrite: %q", got)
	}
}

func TestStoredConfigRefusesSettingsThatBelongToTheStartConfig(t *testing.T) {
	for _, key := range storedConfigRefusedKeys {
		doc := key + ": anything\n"
		if err := ValidateStoredConfig(doc); err == nil || !strings.Contains(err.Error(), key) {
			t.Fatalf("%s was accepted in a stored configuration: %v", key, err)
		}
		if err := WriteStoredConfig(context.Background(), filepath.Join(t.TempDir(), "store.db"), doc); err == nil {
			t.Fatalf("%s was written to the store", key)
		}
	}
	if err := ValidateStoredConfig("sources: []\nlog_format: json\n"); err != nil {
		t.Fatalf("an ordinary document was refused: %v", err)
	}
	if err := ValidateStoredConfig("not: [valid"); err == nil {
		t.Fatal("invalid YAML was accepted")
	}
}

// A document already in the store may predate a rule, or have been edited by
// hand. It must not be able to move the store or change how callers are
// identified, so those settings are dropped when it is loaded.
func TestMergeStoredConfigKeepsTheBaseForRefusedSettings(t *testing.T) {
	base := "mode: agentic\nauth:\n  type: jwt\nhost_port: 127.0.0.1:8080\nsources: []\n"
	doc := "mode: dev\nauth:\n  type: none\nhost_port: 0.0.0.0:1\nartifacts:\n  source: elsewhere\nsources:\n  - name: shop\n    kind: database\n    type: sqlite\napp_name: Shop\n"
	merged, ignored, err := MergeStoredConfig(base, doc)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if got := strings.Join(ignored, ","); got != "artifacts,auth,host_port,mode" {
		t.Fatalf("ignored = %q", got)
	}
	settings, err := parseStoredConfig(merged)
	if err != nil {
		t.Fatal(err)
	}
	if settings["mode"] != "agentic" || settings["host_port"] != "127.0.0.1:8080" {
		t.Fatalf("a refused setting overrode the base: %v", settings)
	}
	if auth, _ := settings["auth"].(map[string]any); auth["type"] != "jwt" {
		t.Fatalf("auth was overridden: %v", settings["auth"])
	}
	if _, ok := settings["artifacts"]; ok {
		t.Fatal("artifacts was taken from the stored document")
	}
	if settings["app_name"] != "Shop" {
		t.Fatalf("an allowed setting was not applied: %v", settings["app_name"])
	}
	if sources, _ := settings["sources"].([]any); len(sources) != 1 {
		t.Fatalf("sources were not taken from the stored document: %v", settings["sources"])
	}
}

func TestDeriveStoreSecretSeparatesPurposes(t *testing.T) {
	session := DeriveStoreSecret("one secret key", "session")
	sealing := DeriveStoreSecret("one secret key", "sealing")
	if session == sealing {
		t.Fatal("two purposes got the same key")
	}
	if session != DeriveStoreSecret("one secret key", "session") {
		t.Fatal("the same key and purpose gave different results")
	}
	if session == DeriveStoreSecret("another secret key", "session") {
		t.Fatal("two secret keys gave the same session key")
	}
	if len(session) < 32 {
		t.Fatalf("derived key is too short: %d", len(session))
	}
}
