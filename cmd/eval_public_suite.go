package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	gjeval "github.com/dosco/graphjin/agent/v3/eval"
)

const (
	DeepContosoBenchmarkSlug             = "deepcontoso"
	DeepContosoBenchmarkSuiteFingerprint = "6d23b0e3399aba54ab9e9c9174feab09"
)

//go:embed benchmark/public-suite.json benchmark/deepcontoso-suite.json benchmark/quick-tasks.json
var publicEvalSuiteFS embed.FS

// quickEvalRepeats is one attempt per task: the quick check trades precision
// for cost (25 attempts against the full run's 339).
const quickEvalRepeats = 1

const quickEvalNotice = `
Quick check: 25 of the public suite's tasks, one attempt each, about 7% of a
full run's provider cost. Its score is expected to move about 6 points from one
run to the next (the full run moves about 2), so it catches a broken setup or a
large drop, not a small change. Quick runs are never published to the board;
run "deeporg eval bench --public" for a publishable result.
`

// quickEvalTasks is cmd/benchmark/quick-tasks.json, written by
// scripts/select-quick-tasks.py from past per-task results.
type quickEvalTasks struct {
	Suite     string   `json:"suite"`
	Repeats   int      `json:"repeats"`
	Selection string   `json:"selection"`
	Tasks     []string `json:"tasks"`
}

// loadQuickEvalSuite returns the public suite cut down to the quick-check
// tasks, in the public suite's order. The task list names the suite it was
// picked from; a refrozen public suite must get a fresh pick, because task
// pass rates measured on the old one no longer describe the new one.
func loadQuickEvalSuite() (*gjeval.Suite, error) {
	public, err := loadPublicEvalSuite()
	if err != nil {
		return nil, err
	}
	data, err := publicEvalSuiteFS.ReadFile("benchmark/quick-tasks.json")
	if err != nil {
		return nil, err
	}
	var pick quickEvalTasks
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&pick); err != nil {
		return nil, fmt.Errorf("parse quick-check task list: %w", err)
	}
	if pick.Suite != public.Name {
		return nil, fmt.Errorf("quick-check tasks were picked from %q but the public suite is %q; rerun scripts/select-quick-tasks.py --write", pick.Suite, public.Name)
	}
	if pick.Repeats != quickEvalRepeats {
		return nil, fmt.Errorf("quick-check task list says %d repeats, the quick check runs %d", pick.Repeats, quickEvalRepeats)
	}
	wanted := make(map[string]bool, len(pick.Tasks))
	for _, id := range pick.Tasks {
		if wanted[id] {
			return nil, fmt.Errorf("quick-check task %s is listed twice", id)
		}
		wanted[id] = true
	}
	quick := *public
	quick.Name = strings.Replace(public.Name, "Public Benchmark", "Quick Check", 1)
	quick.Tasks = make([]gjeval.Task, 0, len(pick.Tasks))
	for _, task := range public.Tasks {
		if wanted[task.ID] {
			quick.Tasks = append(quick.Tasks, task)
			delete(wanted, task.ID)
		}
	}
	if len(wanted) != 0 {
		missing := make([]string, 0, len(wanted))
		for id := range wanted {
			missing = append(missing, id)
		}
		sort.Strings(missing)
		return nil, fmt.Errorf("quick-check tasks not in the public suite: %s; rerun scripts/select-quick-tasks.py --write", strings.Join(missing, ", "))
	}
	if err := quick.Validate(); err != nil {
		return nil, fmt.Errorf("validate quick-check suite: %w", err)
	}
	return &quick, nil
}

// isQuickEvalReport reports whether a report came from today's quick check.
func isQuickEvalReport(report *gjeval.Report) bool {
	if report == nil || report.SuiteFingerprint == "" {
		return false
	}
	return report.SuiteFingerprint == quickEvalSuiteFingerprint()
}

// quickEvalSuiteFingerprint identifies a quick-check run's report so publish
// can refuse it outright. Empty when the quick suite cannot be built.
func quickEvalSuiteFingerprint() string {
	suite, err := loadQuickEvalSuite()
	if err != nil {
		return ""
	}
	return gjeval.SuiteFingerprint(*suite)
}

func loadPublicEvalSuite() (*gjeval.Suite, error) {
	return loadEmbeddedEvalSuite("benchmark/public-suite.json", "public")
}

func loadDeepContosoEvalSuite() (*gjeval.Suite, error) {
	return loadEmbeddedEvalSuite("benchmark/deepcontoso-suite.json", "deepcontoso")
}

func loadNamedEvalSuite(name string) (*gjeval.Suite, error) {
	switch name {
	case DeepContosoBenchmarkSlug:
		return loadDeepContosoEvalSuite()
	case "", "deeporg":
		return loadPublicEvalSuite()
	default:
		return nil, fmt.Errorf("unknown public benchmark suite: %q", name)
	}
}

func loadEmbeddedEvalSuite(filename, label string) (*gjeval.Suite, error) {
	data, err := publicEvalSuiteFS.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var suite gjeval.Suite
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&suite); err != nil {
		return nil, fmt.Errorf("parse embedded %s benchmark suite: %w", label, err)
	}
	if err := suite.Validate(); err != nil {
		return nil, fmt.Errorf("validate embedded %s benchmark suite: %w", label, err)
	}
	return &suite, nil
}
