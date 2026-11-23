package main

import (
	"bytes"
	"strings"
	"testing"

	gjeval "github.com/dosco/graphjin/agent/v3/eval"
	"github.com/spf13/cobra"
)

// TestQuickSuiteIsASliceOfThePublicSuite fails when the public suite is
// refrozen without a fresh quick pick, so the stale list is caught here and not
// at the start of a paid run.
func TestQuickSuiteIsASliceOfThePublicSuite(t *testing.T) {
	quick, err := loadQuickEvalSuite()
	if err != nil {
		t.Fatal(err)
	}
	public, err := loadPublicEvalSuite()
	if err != nil {
		t.Fatal(err)
	}
	if len(quick.Tasks) != 25 {
		t.Errorf("quick check has %d tasks; the noise figures in its notice assume 25", len(quick.Tasks))
	}
	byID := map[string]gjeval.Task{}
	families := map[gjeval.Category]bool{}
	for _, task := range public.Tasks {
		byID[task.ID] = task
		families[task.Category] = true
	}
	covered := map[gjeval.Category]int{}
	for _, task := range quick.Tasks {
		frozen, ok := byID[task.ID]
		if !ok {
			t.Fatalf("quick task %s is not in the public suite", task.ID)
		}
		if frozen.Prompt != task.Prompt || frozen.Slug != task.Slug {
			t.Errorf("quick task %s differs from its frozen public copy", task.ID)
		}
		covered[task.Category]++
	}
	// Every family keeps at least two tasks so a family that breaks shows up.
	for family := range families {
		if covered[family] < 2 {
			t.Errorf("family %s has %d quick tasks, want at least 2", family, covered[family])
		}
	}
	if gjeval.SuiteFingerprint(*quick) == gjeval.PublicBenchmark().SuiteFingerprint {
		t.Fatal("quick suite shares the public fingerprint, so publish could not tell them apart")
	}
	if !strings.Contains(quick.Name, "Quick Check") {
		t.Errorf("quick suite name = %q", quick.Name)
	}
}

// TestResumeCommandForQuickRunParses replays the printed resume command: a
// resume without --quick would load the full suite and refuse the run.
func TestResumeCommandForQuickRunParses(t *testing.T) {
	spec := gjeval.PublicBenchmark()
	opts := &evalCLIOptions{Demo: true, Quick: true}
	recorded := evalInvocationArgs(opts, "/tmp/project", spec.Scale, spec.Seed, true)
	manifest := gjeval.RunManifest{Intent: gjeval.RunIntentBench, RunID: "run-1", InvocationArgs: recorded}
	printed := manifest.ResumeCommand()
	if !strings.Contains(printed, "--quick") {
		t.Fatalf("resume command drops --quick: %s", printed)
	}
	bench := benchCommand(t)
	if err := bench.ParseFlags(shellSplit(t, append(recorded, "--resume", "run-1", "--yes"))); err != nil {
		t.Fatalf("printed resume command does not parse: %v (%s)", err, printed)
	}
	if !bench.Flags().Changed("quick") {
		t.Errorf("bench command did not see --quick: %s", printed)
	}
}

// A quick check has 2-4 tasks per family, so the full-suite floors ("8 of 10
// refusals") can never be met; the report must not print them as failures.
func TestPrintEvalReportSkipsGatesForQuickCheck(t *testing.T) {
	command := &cobra.Command{}
	output := new(bytes.Buffer)
	command.SetOut(output)
	printEvalReport(command, &evalCLIOptions{}, &gjeval.Report{
		RunID: "quick", RunStatus: gjeval.RunStatusComplete,
		SuiteFingerprint: quickEvalSuiteFingerprint(),
		Provenance:       gjeval.RunProvenance{Repeats: quickEvalRepeats},
		Metrics:          gjeval.Metrics{TaskCount: 25, EpisodeCount: 25},
	}, nil)
	if !strings.Contains(output.String(), "Publication gates: not applied to a quick check.") {
		t.Fatalf("quick report does not say gates are skipped: %s", output.String())
	}
	if strings.Contains(output.String(), "NOT met") {
		t.Fatalf("quick report prints full-suite gates: %s", output.String())
	}
}
