package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
)

func runCrewRemove(cmd *cobra.Command, args []string) error {
	name := args[0]
	// Parse rig/name format (e.g., "beads/emma" -> rig=beads, name=emma)
	if rig, crewName, ok := parseRigSlashName(name); ok {
		if crewRig == "" {
			crewRig = rig
		}
		name = crewName
	}

	crewMgr, r, err := getCrewManager(crewRig)
	if err != nil {
		return err
	}

	// Check for running session (unless forced)
	if !crewForce {
		t := tmux.NewTmux()
		sessionID := crewSessionName(r.Name, name)
		hasSession, _ := t.HasSession(sessionID)
		if hasSession {
			return fmt.Errorf("session '%s' is running (use --force to kill and remove)", sessionID)
		}
	}

	// Kill session if it exists
	t := tmux.NewTmux()
	sessionID := crewSessionName(r.Name, name)
	if hasSession, _ := t.HasSession(sessionID); hasSession {
		if err := t.KillSession(sessionID); err != nil {
			return fmt.Errorf("killing session: %w", err)
		}
		fmt.Printf("Killed session %s\n", sessionID)
	}

	// Remove the crew workspace
	if err := crewMgr.Remove(name, crewForce); err != nil {
		if err == crew.ErrCrewNotFound {
			return fmt.Errorf("crew workspace '%s' not found", name)
		}
		if err == crew.ErrHasChanges {
			return fmt.Errorf("crew workspace has uncommitted changes (use --force to remove anyway)")
		}
		return fmt.Errorf("removing crew workspace: %w", err)
	}

	fmt.Printf("%s Removed crew workspace: %s/%s\n",
		style.Bold.Render("✓"), r.Name, name)
	return nil
}

func runCrewRefresh(cmd *cobra.Command, args []string) error {
	name := args[0]
	// Parse rig/name format (e.g., "beads/emma" -> rig=beads, name=emma)
	if rig, crewName, ok := parseRigSlashName(name); ok {
		if crewRig == "" {
			crewRig = rig
		}
		name = crewName
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

	t := tmux.NewTmux()
	sessionID := crewSessionName(r.Name, name)

	// Check if session exists
	hasSession, _ := t.HasSession(sessionID)

	// Create handoff message
	handoffMsg := crewMessage
	if handoffMsg == "" {
		handoffMsg = fmt.Sprintf("Context refresh for %s. Check mail and beads for current work state.", name)
	}

	// Send handoff mail to self
	mailDir := filepath.Join(worker.ClonePath, "mail")
	if _, err := os.Stat(mailDir); os.IsNotExist(err) {
		if err := os.MkdirAll(mailDir, 0755); err != nil {
			return fmt.Errorf("creating mail dir: %w", err)
		}
	}

	// Create and send mail
	mailbox := mail.NewMailbox(mailDir)
	msg := &mail.Message{
		From:    fmt.Sprintf("%s/%s", r.Name, name),
		To:      fmt.Sprintf("%s/%s", r.Name, name),
		Subject: "🤝 HANDOFF: Context Refresh",
		Body:    handoffMsg,
	}
	if err := mailbox.Append(msg); err != nil {
		return fmt.Errorf("sending handoff mail: %w", err)
	}
	fmt.Printf("Sent handoff mail to %s/%s\n", r.Name, name)

	// Kill existing session if running
	if hasSession {
		if err := t.KillSession(sessionID); err != nil {
			return fmt.Errorf("killing old session: %w", err)
		}
		fmt.Printf("Killed old session %s\n", sessionID)
	}

	// Start new session
	if err := t.NewSession(sessionID, worker.ClonePath); err != nil {
		return fmt.Errorf("creating session: %w", err)
	}

	// Set environment
	_ = t.SetEnvironment(sessionID, "GT_RIG", r.Name)
	_ = t.SetEnvironment(sessionID, "GT_CREW", name)

	// Wait for shell to be ready
	if err := t.WaitForShellReady(sessionID, 5*time.Second); err != nil {
		return fmt.Errorf("waiting for shell: %w", err)
	}

	// Start claude (refresh uses regular permissions, reads handoff mail)
	if err := t.SendKeys(sessionID, "claude"); err != nil {
		return fmt.Errorf("starting claude: %w", err)
	}

	fmt.Printf("%s Refreshed crew workspace: %s/%s\n",
		style.Bold.Render("✓"), r.Name, name)
	fmt.Printf("Attach with: %s\n", style.Dim.Render(fmt.Sprintf("gt crew at %s", name)))

	return nil
}

func runCrewRestart(cmd *cobra.Command, args []string) error {
	name := args[0]
	// Parse rig/name format (e.g., "beads/emma" -> rig=beads, name=emma)
	if rig, crewName, ok := parseRigSlashName(name); ok {
		if crewRig == "" {
			crewRig = rig
		}
		name = crewName
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

	t := tmux.NewTmux()
	sessionID := crewSessionName(r.Name, name)

	// Kill existing session if running
	if hasSession, _ := t.HasSession(sessionID); hasSession {
		if err := t.KillSession(sessionID); err != nil {
			return fmt.Errorf("killing old session: %w", err)
		}
		fmt.Printf("Killed session %s\n", sessionID)
	}

	// Start new session
	if err := t.NewSession(sessionID, worker.ClonePath); err != nil {
		return fmt.Errorf("creating session: %w", err)
	}

	// Set environment
	t.SetEnvironment(sessionID, "GT_RIG", r.Name)
	t.SetEnvironment(sessionID, "GT_CREW", name)

	// Apply rig-based theming (uses config if set, falls back to hash)
	theme := getThemeForRig(r.Name)
	_ = t.ConfigureGasTownSession(sessionID, theme, r.Name, name, "crew")

	// Wait for shell to be ready
	if err := t.WaitForShellReady(sessionID, 5*time.Second); err != nil {
		return fmt.Errorf("waiting for shell: %w", err)
	}

	// Start claude with skip permissions (crew workers are trusted)
	if err := t.SendKeys(sessionID, "claude --dangerously-skip-permissions"); err != nil {
		return fmt.Errorf("starting claude: %w", err)
	}

	// Wait for Claude to start, then prime it
	shells := []string{"bash", "zsh", "sh", "fish", "tcsh", "ksh"}
	if err := t.WaitForCommand(sessionID, shells, 15*time.Second); err != nil {
		fmt.Printf("Warning: Timeout waiting for Claude to start: %v\n", err)
	}
	// Give Claude time to initialize after process starts
	time.Sleep(500 * time.Millisecond)
	if err := t.SendKeys(sessionID, "gt prime"); err != nil {
		// Non-fatal: Claude started but priming failed
		fmt.Printf("Warning: Could not send prime command: %v\n", err)
	}

	// Send crew resume prompt after prime completes
	// Use NudgeSession (the canonical way to message Claude) with longer pre-delay
	// to ensure gt prime has finished processing
	time.Sleep(5 * time.Second)
	crewPrompt := "Read your mail, act on anything urgent, else await instructions."
	if err := t.NudgeSession(sessionID, crewPrompt); err != nil {
		fmt.Printf("Warning: Could not send resume prompt: %v\n", err)
	}

	fmt.Printf("%s Restarted crew workspace: %s/%s\n",
		style.Bold.Render("✓"), r.Name, name)
	fmt.Printf("Attach with: %s\n", style.Dim.Render(fmt.Sprintf("gt crew at %s", name)))

	return nil
}
