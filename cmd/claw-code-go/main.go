package main

import (
	"claw-code-go/internal/auth"
	"claw-code-go/internal/commands"
	"claw-code-go/internal/compat"
	"claw-code-go/internal/permissions"
	"claw-code-go/internal/runtime"
	"claw-code-go/internal/serve"
	"claw-code-go/internal/tui"
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	// Route diagnostic subcommands before flag parsing.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "dump-manifests":
			compat.RunDumpManifests(os.Args[2:])
			return
		case "bootstrap-plan":
			compat.RunBootstrapPlan(os.Args[2:])
			return
		case "print-system-prompt":
			compat.RunPrintSystemPrompt(os.Args[2:])
			return
		case "resume-session":
			compat.RunResumeSession(os.Args[2:])
			return
		case "serve":
			runServe(os.Args[2:])
			return
		}
	}

	promptFlag := flag.String("prompt", "", "Run a single prompt and exit")
	modelFlag := flag.String("model", "", "Override the model to use")
	replFlag := flag.Bool("repl", false, "Run in interactive REPL mode (default when no --prompt)")
	sessionFlag := flag.String("session", "", "Session ID to load")
	sessionDirFlag := flag.String("session-dir", "", "Directory to store sessions")
	permModeFlag := flag.String("permission-mode", "default", "Permission mode: default, accept-edits, bypass, plan")
	_ = replFlag

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: claw-code-go [subcommand] [options]\n\n")
		fmt.Fprintf(os.Stderr, "Subcommands:\n")
		fmt.Fprintf(os.Stderr, "  dump-manifests [--src <dir>] [--json]   List tools, slash commands, and source manifest\n")
		fmt.Fprintf(os.Stderr, "  bootstrap-plan [--json]                 Print the ordered startup phase plan\n")
		fmt.Fprintf(os.Stderr, "  print-system-prompt [--cwd] [--date]    Render the full system prompt\n")
		fmt.Fprintf(os.Stderr, "  resume-session <file> [commands...]     Replay a saved session file\n")
		fmt.Fprintf(os.Stderr, "  serve [--addr] [--session-root]         Minimal dispatch server (kata 84) - loopback only, no auth\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nEnvironment variables:\n")
		fmt.Fprintf(os.Stderr, "  ANTHROPIC_API_KEY        Anthropic API key (takes precedence over stored credentials)\n")
		fmt.Fprintf(os.Stderr, "  OPENAI_API_KEY           OpenAI API key (takes precedence over stored credentials)\n")
		fmt.Fprintf(os.Stderr, "  ANTHROPIC_MODEL          Model to use (default: %s)\n", runtime.DefaultModel)
		fmt.Fprintf(os.Stderr, "  ANTHROPIC_BASE_URL       Base URL for the Anthropic API\n")
		fmt.Fprintf(os.Stderr, "  CLAUDE_CODE_USE_BEDROCK  Set to 1 to use AWS Bedrock (env-var fallback)\n")
		fmt.Fprintf(os.Stderr, "  CLAUDE_CODE_USE_VERTEX   Set to 1 to use Google Vertex AI (env-var fallback)\n")
		fmt.Fprintf(os.Stderr, "  CLAUDE_CODE_USE_FOUNDRY  Set to 1 to use Azure AI Foundry (env-var fallback)\n")
	}

	flag.Parse()

	cfg := runtime.LoadConfig()

	if *modelFlag != "" {
		cfg.Model = *modelFlag
	}
	if *sessionDirFlag != "" {
		cfg.SessionDir = *sessionDirFlag
	}

	// Resolve credentials using the multi-provider credential store.
	// Env vars take precedence (ANTHROPIC_API_KEY, OPENAI_API_KEY).
	// Falls back gracefully so the TUI can start and prompt the user to /login.
	provider, token, authMethod, credErr := auth.ResolveCredentials()
	if credErr == nil {
		cfg.ProviderName = provider
		cfg.AuthMethod = authMethod
		if authMethod == "oauth" {
			cfg.OAuthToken = token
		} else {
			cfg.APIKey = token
		}
	} else if cfg.APIKey != "" && cfg.BaseURL != "" {
		// ResolveCredentials only ever looks at ANTHROPIC_API_KEY/OPENAI_API_KEY env vars and
		// the stored /login credential store - it has no idea a custom OpenAI-compatible
		// endpoint with its own key came from the global config file
		// (~/.config/claw-code-go/config.json) or a CLI flag. That's a real, sufficient
		// credential on its own (cfg.ProviderName is already "openai" here, via
		// detectProvider(cfg.BaseURL) in LoadConfig) - without this branch, a configured local
		// gateway/kronk endpoint could never be used without ALSO exporting OPENAI_API_KEY by
		// hand on every single invocation.
		cfg.AuthMethod = "api_key"
	} else {
		// No credentials found — start with NoAuthClient so the TUI still opens.
		// The user can run /login inside the TUI.
		fmt.Fprintf(os.Stderr, "Note: no credentials found (%v).\n", credErr)
		fmt.Fprintln(os.Stderr, "      Use /login in the TUI to authenticate.")
	}

	// Create the provider client (or a no-auth placeholder).
	realClient, clientErr := runtime.NewProviderClient(cfg)
	if clientErr != nil {
		fmt.Fprintf(os.Stderr, "Note: could not create %s client: %v\n", cfg.ProviderName, clientErr)
		fmt.Fprintln(os.Stderr, "      Use /login in the TUI to authenticate.")
		realClient = runtime.NewNoAuthClient()
	}

	loop := runtime.NewConversationLoop(cfg, realClient)

	// Wire up the permission manager (Phase 11).
	// CLI --permission-mode flag overrides the config-file value when set to a
	// non-default value. cfg.PermissionMode comes from the layered settings files.
	resolvedPermMode := cfg.PermissionMode
	if *permModeFlag != "default" {
		resolvedPermMode = *permModeFlag
	}
	permMode, err := permissions.ParsePermissionMode(resolvedPermMode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v; using default mode\n", err)
		permMode = permissions.ModeDefault
	}
	cfg.PermissionMode = permMode.String() // normalise back into Config

	ruleset, rErr := permissions.LoadRuleset(".claude/settings.json")
	if rErr != nil {
		ruleset = &permissions.Ruleset{}
	}
	// Merge allowedTools/blockedTools from layered config into the ruleset.
	if len(cfg.AllowedTools) > 0 || len(cfg.BlockedTools) > 0 {
		extra := permissions.RulesetFromLists(cfg.AllowedTools, cfg.BlockedTools)
		ruleset.Rules = append(ruleset.Rules, extra.Rules...)
	}
	loop.PermManager = permissions.NewManager(permMode, ruleset)

	// Connect to MCP servers defined in config (non-fatal errors printed inside).
	loop.InitMCPFromConfig(context.Background())

	if *sessionFlag != "" {
		sess, err := runtime.LoadSession(cfg.SessionDir, *sessionFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not load session %s: %v\n", *sessionFlag, err)
		} else {
			loop.Session = sess
			fmt.Printf("Loaded session: %s\n", sess.ID)
		}
	}

	// Single prompt (non-interactive) mode — no TUI, plain stdout streaming.
	if *promptFlag != "" {
		// Check cfg directly, not credErr: credErr only reflects ResolveCredentials' own
		// sources (env vars, stored /login credentials) and knows nothing about a custom
		// endpoint's key coming from the global config file instead (see the credErr handling
		// above).
		if cfg.APIKey == "" && cfg.OAuthToken == "" {
			fmt.Fprintln(os.Stderr, "Error: cannot use --prompt without valid credentials.")
			fmt.Fprintln(os.Stderr, "Set ANTHROPIC_API_KEY or OPENAI_API_KEY, or run the TUI and use /login.")
			os.Exit(1)
		}
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			fmt.Fprintln(os.Stdout, "\nInterrupted. Saving session...")
			saveSessionSilent(cfg.SessionDir, loop)
			// A distinct exit code, not 0 - kata 84 Test 3 found live that an
			// interrupted dispatch and a naturally-completed one were
			// indistinguishable at the process level (both exited 0), leaving
			// internal/serve's classifyStatus to guess from message_count alone.
			// See exitCodeInterrupted's own doc comment for why this exact value.
			os.Exit(serve.ExitCodeInterrupted)
		}()

		ctx := context.Background()
		if err := loop.SendMessage(ctx, *promptFlag); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		saveSessionSilent(cfg.SessionDir, loop)
		return
	}

	// Interactive TUI mode.
	runTUI(cfg, loop)
}

// runTUI starts the Bubble Tea TUI for interactive use.
func runTUI(cfg *runtime.Config, loop *runtime.ConversationLoop) {
	// Register slash commands (available for future non-TUI REPL mode).
	registry := commands.NewRegistry()
	commands.RegisterAuthCommands(registry)
	commands.RegisterMCPCommand(registry)
	_ = registry

	// Save session on SIGTERM (Ctrl+C is handled by Bubble Tea itself).
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM)
	go func() {
		<-sigCh
		saveSessionSilent(cfg.SessionDir, loop)
		os.Exit(0)
	}()

	model := tui.NewModel(cfg, loop)
	// WithMouseCellMotion requests real mouse reporting from the terminal - without it, the
	// terminal has no reason to report wheel events at all, and most substitute synthesized
	// Up/Down key presses for wheel scroll instead, which the TUI reads as input-history
	// navigation rather than scrolling the conversation.
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		os.Exit(1)
	}

	// Save session after the TUI exits (covers Ctrl+C via tea.Quit).
	saveSessionSilent(cfg.SessionDir, loop)
}

// saveSessionSilent saves the session and prints its ID + resume command on the way out - the
// whole reason this exists is a real incident: exiting the TUI (accidentally or not) left no
// visible trail back to that conversation, and finding it again meant grepping session files by
// mtime. Prints to stderr on failure, matching the name's original "silent unless something's
// wrong" contract for errors; the resume hint on success is new, not silent by design.
func saveSessionSilent(dir string, loop *runtime.ConversationLoop) {
	if err := runtime.SaveSession(dir, loop.Session); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not save session: %v\n", err)
		return
	}
	fmt.Printf("Session saved: %s\n", loop.Session.ID)
	fmt.Printf("Resume with: claw-code-go --session %s\n", loop.Session.ID)
}

// runServe starts the minimal dispatch server (kata 84 Test 1) - see
// internal/serve's own package doc for the real design decision behind it
// (process-per-dispatch, not an in-process session registry).
func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	// Loopback by default: this has no authentication and spawns child processes
	// with -permission-mode bypass (bash auto-allowed). Widening this to 0.0.0.0
	// (e.g. for Prometheus, which runs in a container and needs host.docker.internal
	// to reach a BARE HOST process like this one - unlike kronk's own debug server,
	// there is no Docker network isolation containing the exposure here) is a real,
	// understood tradeoff to opt into per-session, not a default to change quietly.
	addr := fs.String("addr", "127.0.0.1:4097", "address to listen on - loopback by default, no authentication; widen deliberately (e.g. 0.0.0.0:4097) only when you want Prometheus/Grafana visibility for this session")
	sessionRoot := fs.String("session-root", "", "parent directory for per-dispatch session directories (default: ~/.claw-code/dispatches)")
	fs.Parse(args)

	binaryPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: resolve own binary path: %v\n", err)
		os.Exit(1)
	}

	root := *sessionRoot
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "serve: resolve home dir: %v\n", err)
			os.Exit(1)
		}
		root = filepath.Join(home, ".claw-code", "dispatches")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "serve: create session root: %v\n", err)
		os.Exit(1)
	}

	reg := serve.NewRegistry(binaryPath, root)
	srv := serve.NewServer(reg)

	fmt.Fprintf(os.Stderr, "serve: listening on %s, dispatch sessions under %s\n", *addr, root)
	if err := http.ListenAndServe(*addr, srv.Mux()); err != nil {
		fmt.Fprintf(os.Stderr, "serve: %v\n", err)
		os.Exit(1)
	}
}
