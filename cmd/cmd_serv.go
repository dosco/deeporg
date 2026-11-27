package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dosco/graphjin/serv/v3"
	"github.com/spf13/cobra"
)

var (
	// deployActive  bool
	servDemoMode bool
	servDBFlags  []string
)

// ANSI color codes
const (
	colorCyan    = "\033[36m"
	colorMagenta = "\033[35m"
	colorYellow  = "\033[33m"
	colorGray    = "\033[90m"
	colorReset   = "\033[0m"
)

// bannerLetters are the letters of the startup banner, six rows each.
var bannerLetters = map[rune][6]string{
	'D': {"██████╗ ", "██╔══██╗", "██║  ██║", "██║  ██║", "██████╔╝", "╚═════╝ "},
	'E': {"███████╗", "██╔════╝", "█████╗  ", "██╔══╝  ", "███████╗", "╚══════╝"},
	'P': {"██████╗ ", "██╔══██╗", "██████╔╝", "██╔═══╝ ", "██║     ", "╚═╝     "},
	'O': {" ██████╗ ", "██╔═══██╗", "██║   ██║", "██║   ██║", "╚██████╔╝", " ╚═════╝ "},
	'R': {"██████╗ ", "██╔══██╗", "██████╔╝", "██╔══██╗", "██║  ██║", "╚═╝  ╚═╝"},
	'G': {" ██████╗ ", "██╔════╝ ", "██║  ███╗", "██║   ██║", "╚██████╔╝", " ╚═════╝ "},
}

// bannerText draws DeepORG in block letters, DEEP in one color and ORG in
// another. With color off it is plain text.
func bannerText(color bool) string {
	first, second, reset := "", "", ""
	if color {
		first, second, reset = colorCyan, colorMagenta, colorReset
	}
	var b strings.Builder
	b.WriteString("\n")
	for row := 0; row < 6; row++ {
		b.WriteString(" " + first)
		for _, letter := range "DEEP" {
			b.WriteString(bannerLetters[letter][row])
		}
		b.WriteString(reset + second)
		for _, letter := range "ORG" {
			b.WriteString(bannerLetters[letter][row])
		}
		b.WriteString(reset + "\n")
	}
	b.WriteString("\n")
	return b.String()
}

// printBanner prints the banner on startup. NO_COLOR turns the colors off,
// for CI and for terminals that do not show them.
func printBanner() {
	fmt.Print(bannerText(os.Getenv("NO_COLOR") == ""))
}

// servCmd is the cobra CLI command for the serve subcommand
func servCmd() *cobra.Command {
	c := &cobra.Command{
		Use:     "serve",
		Aliases: []string{"serv"},
		Short:   "Run the GraphJin service",
		Long: `Run the GraphJin HTTP service.

Demo mode (--demo):
  deeporg serve --demo                            # built-in demo (SQLite, no containers)
  deeporg serve --demo --path examples/webshop
  deeporg serve --demo --path examples/coffee-roastery

Without --path the built-in saas-ops demo is extracted to
./graphjin-demo and served from there; delete that directory for a
fresh copy. Demo state is stored under <path>/demo. Delete that
folder to reset the data.`,
		Run: cmdServ,
	}
	// c.Flags().BoolVar(&deployActive, "deploy-active", false, "Deploy active config")
	c.Flags().BoolVar(&servDemoMode, "demo", false, "Run a curated local demo (built-in example when --path is unset) with state under <path>/demo")
	c.Flags().StringArrayVar(&servDBFlags, "db", nil, "Database type override(s) (requires --demo)")

	// Server-side lifecycle subcommands. These used to live at the top level
	// (the db, new, and test commands) but were moved under
	// `serve` since they all operate on the same server-side config and DB.
	c.AddCommand(newCmd())
	c.AddCommand(dbCmd())
	c.AddCommand(testCmd())
	return c
}

// cmdServ is the handler for the serve subcommand
func cmdServ(cmd *cobra.Command, args []string) {
	printBanner()

	// Check that --db requires --demo
	if !servDemoMode && len(servDBFlags) > 0 {
		log.Fatal("--db flags require --demo")
	}

	if servDemoMode {
		pathSet := cmd.Flags().Changed("path") || cmd.Flags().Changed("config")
		dpath, err := resolveDemoPath(pathSet, os.Stdout)
		if err != nil {
			log.Fatalf("Failed to prepare demo: %s", err)
		}
		cpath = dpath
		if err := loadDemoEnv(cpath, os.Stdout); err != nil {
			log.Fatalf("Failed to load demo .env: %s", err)
		}
	}

	// A new install has no config directory: it starts from a start config
	// and reads everything else from its store. setup() leaves conf alone
	// once it is set, so the legacy flow below is untouched.
	var start *startConfig
	if !servDemoMode {
		if file, ok := usesStartConfig(cmd); ok {
			var err error
			if start, err = loadStartConfig(file); err == nil {
				err = start.storeWritable()
			}
			if err == nil {
				err = start.findListen()
			}
			if err != nil {
				// These messages tell the reader what to do next; print them
				// as they are, without a log prefix.
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			if conf, err = loadStartFlowConfig(start); err != nil {
				log.Fatalf("%s", err)
			}
			if start.created {
				fmt.Printf("Created %s\n\n", start.path)
			}
			if advice := start.unreachableListenAdvice(); advice != "" {
				fmt.Fprintf(os.Stderr, "%s\n\n", advice)
			}
		}
	}

	setup(cpath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var demo *DemoRuntime

	// Start demo containers if --demo is set
	if servDemoMode {
		var err error
		demo, err = StartDemo(ctx, servDBFlags, os.Stdout)
		if err != nil {
			log.Fatalf("Failed to start demo: %s", err)
		}
	}

	var opt []serv.Option
	if demo != nil && len(demo.Databases) != 0 {
		opt = append(opt,
			serv.OptionSetDatabases(demo.Databases),
			serv.OptionSetRuntimeSchemaDDLDir(demoRuntimeSchemaDDLDir()),
		)
	}
	if seedOpt, ok := demoWatchSeedOption(demo, cpath); ok {
		opt = append(opt, seedOpt)
	}
	if signIn := demoSignInOption(demo, cpath); signIn != nil {
		opt = append(opt, signIn)
	}
	if start != nil {
		// A setup step saves to the store; the server then comes back up
		// reading it.
		opt = append(opt, serv.OptionSetConfigApplier(restartInPlace))
		opt = append(opt, serv.OptionSetSampleData(sampleDataBuilder(start)))
		// A new install says only what its owner has to act on. The start
		// config's log_level opens it up for someone who wants the detail.
		opt = append(opt, serv.OptionSetMinLogLevel(startLogLevel(start)))
	}
	// if deployActive {
	// 	opt = append(opt, serv.OptionDeployActive())
	// }

	gj, err := serv.NewGraphJinService(conf, opt...)
	if err != nil {
		log.Fatalf("%s", err)
	}
	if demo != nil {
		demo.Status.Emit("GraphJin", "ready", "service initialized")
	}

	// Setup graceful shutdown for demo mode
	if demo != nil && len(demo.Cleanups) > 0 {
		done := make(chan struct{})
		go func() {
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
			<-sigCh

			log.Info("Shutting down...")
			cancel()

			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer shutdownCancel()

			// Stop the HTTP server first so gj.Start() returns and the process
			// can exit. Draining here also lets in-flight requests finish
			// before the demo databases/containers are torn down.
			if err := gj.Shutdown(shutdownCtx); err != nil {
				log.Warnf("HTTP shutdown error: %s", err)
			}

			// Then tear down the demo containers.
			cleanupAll(shutdownCtx, demo.Cleanups)
			log.Info("Container(s) terminated")

			close(done)
		}()

		if err := gj.Start(); err != nil {
			log.Fatalf("%s", err)
		}

		<-done
	} else {
		if start != nil {
			announceSetup(gj, start)
		}
		if err := gj.Start(); err != nil {
			log.Fatalf("%s", err)
		}
	}
}
