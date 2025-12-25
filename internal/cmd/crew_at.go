package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

func runCrewAt(cmd *cobra.Command, args []string) error {
	var name string

	// Determine crew name: from arg, or auto-detect from cwd
	if len(args) > 0 {
		name = args[0]
		// Parse rig/name format (e.g., "beads/emma" -> rig=beads, name=emma)
		if rig, crewName, ok := parseRigSlashName(name); ok {
			if crewRig == "" {
				crewRig = rig
			}
			name = crewName
		}
	} else {
		// Try to detect from current directory
		detected, err := detectCrewFromCwd()
		if err != nil {
			return fmt.Errorf("could not detect crew workspace from current directory: %w\n\nUsage: gt crew at <name>", err)
		}
		name = detected.crewName
		if crewRig == "" {
			crewRig = detected.rigName
		}
		fmt.Printf("Detected crew workspace: %s/%s\n", detected.rigName, name)
	}

	crewMgr, r, err := getCrewManager(crewRig)
	if err != nil {
		return err
	}

	// Get the crew worker
	worker, err := crewMgr.Get(name)
	if err != nil {
		if err == crew.ErrCrewNotFound {
			return fmt.Errorf("crew workspace '%s' not found", name)
		}
		return fmt.Errorf("getting crew worker: %w", err)
	}

	// Ensure crew workspace is on main branch (persistent roles should not use feature branches)
	ensureMainBranch(worker.ClonePath, fmt.Sprintf("Crew workspace %s/%s", r.Name, name))

	// If --no-tmux, just print the path
	if crewNoTmux {
		fmt.Println(worker.ClonePath)
		return nil
	}

	// Resolve account for Claude config
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}
	accountsPath := constants.MayorAccountsPath(townRoot)
	claudeConfigDir, accountHandle, err := config.ResolveAccountConfigDir(accountsPath, crewAccount)
	if err != nil {
		return fmt.Errorf("resolving account: %w", err)
	}
	if accountHandle != "" {
		fmt.Printf("Using account: %s\n", accountHandle)
	}

	// Check if session exists
	t := tmux.NewTmux()
	sessionID := crewSessionName(r.Name, name)
	hasSession, err := t.HasSession(sessionID)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}

	if !hasSession {
		// Create new session
		if err := t.NewSession(sessionID, worker.ClonePath); err != nil {
			return fmt.Errorf("creating session: %w", err)
		}

		// Set environment
		_ = t.SetEnvironment(sessionID, "GT_RIG", r.Name)
		_ = t.SetEnvironment(sessionID, "GT_CREW", name)

		// Set CLAUDE_CONFIG_DIR for account selection
		if claudeConfigDir != "" {
			_ = t.SetEnvironment(sessionID, "CLAUDE_CONFIG_DIR", claudeConfigDir)
		}

		// Apply rig-based theming (uses config if set, falls back to hash)
		theme := getThemeForRig(r.Name)
		_ = t.ConfigureGasTownSession(sessionID, theme, r.Name, name, "crew")

		// Wait for shell to be ready after session creation
		if err := t.WaitForShellReady(sessionID, 5*time.Second); err != nil {
			return fmt.Errorf("waiting for shell: %w", err)
		}

		// Start claude with skip permissions (crew workers are trusted like Mayor)
		if err := t.SendKeys(sessionID, "claude --dangerously-skip-permissions"); err != nil {
			return fmt.Errorf("starting claude: %w", err)
		}

		// Wait for Claude to start (pane command changes from shell to node)
		shells := []string{"bash", "zsh", "sh", "fish", "tcsh", "ksh"}
		if err := t.WaitForCommand(sessionID, shells, 15*time.Second); err != nil {
			fmt.Printf("Warning: Timeout waiting for Claude to start: %v\n", err)
		}

		// Give Claude time to initialize after process starts
		time.Sleep(500 * time.Millisecond)

		// Send gt prime to initialize context
		if err := t.SendKeys(sessionID, "gt prime"); err != nil {
			// Non-fatal: Claude started but priming failed
			fmt.Printf("Warning: Could not send prime command: %v\n", err)
		}

		fmt.Printf("%s Created session for %s/%s\n",
			style.Bold.Render("✓"), r.Name, name)
	} else {
		// Session exists - check if Claude is still running
		// Uses both pane command check and UI marker detection to avoid
		// restarting when user is in a subshell spawned from Claude
		if !t.IsClaudeRunning(sessionID) {
			// Claude has exited, restart it
			fmt.Printf("Claude exited, restarting...\n")
			if err := t.SendKeys(sessionID, "claude --dangerously-skip-permissions"); err != nil {
				return fmt.Errorf("restarting claude: %w", err)
			}
			// Wait for Claude to start, then prime
			shells := []string{"bash", "zsh", "sh", "fish", "tcsh", "ksh"}
			if err := t.WaitForCommand(sessionID, shells, 15*time.Second); err != nil {
				fmt.Printf("Warning: Timeout waiting for Claude to start: %v\n", err)
			}
			// Give Claude time to initialize after process starts
			time.Sleep(500 * time.Millisecond)
			if err := t.SendKeys(sessionID, "gt prime"); err != nil {
				fmt.Printf("Warning: Could not send prime command: %v\n", err)
			}
			// Send crew resume prompt after prime completes
			// Use NudgeSession (the canonical way to message Claude) with longer pre-delay
			time.Sleep(5 * time.Second)
			crewPrompt := "Run gt prime. Check your mail and in-progress issues. Act on anything urgent, else await instructions."
			if err := t.NudgeSession(sessionID, crewPrompt); err != nil {
				fmt.Printf("Warning: Could not send resume prompt: %v\n", err)
			}
		}
	}

	// Check if we're already in the target session
	if isInTmuxSession(sessionID) {
		// We're in the session at a shell prompt - just start Claude directly
		// Pass "gt prime" as initial prompt so Claude loads context immediately
		fmt.Printf("Starting Claude in current session...\n")
		return execClaude("gt prime")
	}

	// If inside tmux (but different session), don't switch - just inform user
	if tmux.IsInsideTmux() {
		fmt.Printf("Started %s/%s. Use C-b s to switch.\n", r.Name, name)
		return nil
	}

	// Outside tmux: attach unless --detached flag is set
	if crewDetached {
		fmt.Printf("Started %s/%s. Run 'gt crew at %s' to attach.\n", r.Name, name, name)
		return nil
	}

	// Attach to session
	return attachToTmuxSession(sessionID)
}
