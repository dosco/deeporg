package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// Sample data lets someone with no database to hand still get to an answer.
// It is the built-in demo company's database: the same tables and rows
// `deeporg serve --demo` serves, as one SQLite file.
//
// Building it runs the demo's own schema and seed code, which works on this
// package's globals. A running server cannot call that in its own process, so
// it runs this binary again with the hidden `sample-data` command and reads
// back where the file is.

// sampleDataFile is where the demo's database lands inside the demo project.
var sampleDataFile = filepath.Join("demo", "databases", "app", "graphjin_demo.db")

const sampleDataTimeout = 3 * time.Minute

func sampleDataCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "sample-data <dir>",
		Short:  "Build the sample database in a directory and print its path",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			path, err := buildSampleData(args[0])
			if err != nil {
				log.Fatalf("%s", err)
			}
			fmt.Println(path)
		},
	}
}

// buildSampleData extracts the built-in demo project into dir, creates its
// database and seeds it, and returns the database file. A directory that
// already holds the file is left as it is.
func buildSampleData(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	dbPath := filepath.Join(dir, sampleDataFile)
	if info, err := os.Stat(dbPath); err == nil && info.Size() > 0 {
		return dbPath, nil
	}
	if err := extractDefaultDemo(dir); err != nil {
		return "", fmt.Errorf("extract the sample project: %w", err)
	}

	// The demo project is a config directory in the legacy flow.
	if err := os.Setenv("GO_ENV", "dev"); err != nil {
		return "", err
	}
	cpath = dir
	conf = nil
	setup(cpath)

	ctx, cancel := context.WithTimeout(context.Background(), sampleDataTimeout)
	defer cancel()
	demo, err := StartDemo(ctx, nil, io.Discard)
	if err != nil {
		return "", fmt.Errorf("build the sample database: %w", err)
	}
	cleanupAll(ctx, demo.Cleanups)

	if info, err := os.Stat(dbPath); err != nil || info.Size() == 0 {
		return "", errors.New("the sample database was not created")
	}
	if err := dropStoreTablesFromSample(dbPath); err != nil {
		return "", err
	}
	return dbPath, nil
}

// dropStoreTablesFromSample removes the tables the demo keeps its own state
// in. The install that connects this file has a store of its own; left in,
// they would show up as if they were the sample company's data.
func dropStoreTablesFromSample(dbPath string) error {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close() //nolint:errcheck
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name LIKE '\_graphjin\_%' ESCAPE '\'`)
	if err != nil {
		return err
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close() //nolint:errcheck
			return err
		}
		names = append(names, name)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, name := range names {
		if _, err := db.Exec(`DROP TABLE IF EXISTS "` + strings.ReplaceAll(name, `"`, `""`) + `"`); err != nil {
			return err
		}
	}
	return nil
}

// sampleDataBuilder returns the function a running server calls to get the
// sample database: it runs this binary's sample-data command and returns the
// path it prints.
func sampleDataBuilder(sc *startConfig) func(context.Context) (string, error) {
	home := filepath.Dir(sc.Store)
	kept := filepath.Join(home, "sample.db")
	return func(ctx context.Context) (string, error) {
		// Built once. Asking again connects the same file.
		if info, err := os.Stat(kept); err == nil && info.Size() > 0 {
			return kept, nil
		}
		binary, err := os.Executable()
		if err != nil {
			return "", err
		}
		// The demo project is only needed to build the database. Build in a
		// scratch directory and keep the one file.
		dir, err := os.MkdirTemp(home, "sample-build-")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(dir) //nolint:errcheck
		ctx, cancel := context.WithTimeout(ctx, sampleDataTimeout)
		defer cancel()
		child := exec.CommandContext(ctx, binary, "sample-data", dir)
		// The child is a separate, legacy-flow run. It must not inherit this
		// install's start config, or its secrets.
		child.Env = sampleDataEnv(os.Environ())
		var stderr strings.Builder
		child.Stderr = &stderr
		out, err := child.Output()
		if err != nil {
			detail := strings.TrimSpace(stderr.String())
			if len(detail) > 600 {
				detail = detail[len(detail)-600:]
			}
			return "", fmt.Errorf("building the sample data failed: %w: %s", err, detail)
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		path := strings.TrimSpace(lines[len(lines)-1])
		if _, err := os.Stat(path); err != nil {
			return "", errors.New("the sample database was not created")
		}
		if err := os.Rename(path, kept); err != nil {
			return "", err
		}
		if err := os.Chmod(kept, 0o600); err != nil {
			return "", err
		}
		return kept, nil
	}
}

func sampleDataEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "DEEPORG_") || strings.HasPrefix(name, "GJ_") || name == "GO_ENV" {
			continue
		}
		out = append(out, entry)
	}
	return out
}
