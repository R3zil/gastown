# Town Management Design

Design for `gt install`, `gt doctor`, and federation in the Gas Town Go port.

## Overview

A **Town** is a complete Gas Town installation containing:
- Workspace config (`.gastown/` or `mayor/`)
- Rigs (project workspaces)
- Polecats (worker clones)
- Mail system
- Beads integration

## Config Files

### Workspace Root

A town is identified by a config directory (`mayor/` or `.gastown/`) containing `config.json` with `type: "workspace"`.

```
~/ai/                           # Workspace root
├── mayor/                      # or .gastown/
│   ├── config.json             # Workspace identity
│   ├── state.json              # Workspace state
│   ├── mail/                   # Mail system
│   │   └── inbox.jsonl         # Mayor's inbox
│   ├── boss/                   # Boss state
│   │   └── state.json
│   └── rigs/                   # Mayor's rig clones (for beads access)
│       ├── gastown/
│       └── beads/
├── gastown/                    # Rig
│   ├── .gastown/               # Rig config (type: "rig")
│   │   └── config.json
│   ├── refinery/               # Refinery infrastructure
│   │   ├── README.md
│   │   ├── state.json
│   │   ├── state.json.lock
│   │   └── rig/                # Refinery's git clone
│   └── <polecats>/             # Worker clones
└── beads/                      # Another rig
```

### Workspace config.json

```json
{
  "type": "workspace",
  "version": 1,
  "created_at": "2024-01-15T10:30:00Z"
}
```

### Workspace state.json

```json
{
  "version": 1,
  "projects": {}
}
```

### Rig config.json

```json
{
  "type": "rig",
  "git_url": "https://github.com/steveyegge/gastown",
  "beads_path": "~/.gastown/rigs/gastown/.beads",
  "federation": {
    "machines": ["local", "gcp-west-1"],
    "preferred_machine": "local"
  }
}
```

### Refinery state.json

```json
{
  "version": 1,
  "state": "stopped",
  "awake": false,
  "created_at": "2024-01-15T10:30:00Z",
  "last_started": null,
  "last_stopped": null,
  "last_wake": null,
  "last_sleep": null
}
```

## gt install

### Command

```bash
gt install [path]    # Default: current directory
gt install ~/ai
```

### Behavior

1. **Check if already installed**: Look for `mayor/` or `.gastown/` with `type: "workspace"`
2. **Create workspace structure**:
   - `mayor/config.json` - workspace identity
   - `mayor/state.json` - workspace state
   - `mayor/mail/` - mail directory
   - `mayor/boss/state.json` - boss state
   - `mayor/rigs/` - empty, populated when rigs are added
3. **Create .gitignore** - ignore ephemeral state, polecat clones
4. **Create CLAUDE.md** - Mayor instructions
5. **Initialize git** if not present

### Implementation (Go)

```go
// pkg/workspace/install.go
package workspace

type InstallOptions struct {
    Path string
    Force bool  // Overwrite existing
}

func Install(opts InstallOptions) (*Workspace, error) {
    path := opts.Path
    if path == "" {
        path = "."
    }

    // Resolve and validate
    absPath, err := filepath.Abs(path)
    if err != nil {
        return nil, fmt.Errorf("invalid path: %w", err)
    }

    // Check existing
    if ws, _ := Find(absPath); ws != nil && !opts.Force {
        return nil, ErrAlreadyInstalled
    }

    // Create structure
    mayorDir := filepath.Join(absPath, "mayor")
    if err := os.MkdirAll(mayorDir, 0755); err != nil {
        return nil, err
    }

    // Write config
    config := Config{
        Type:      "workspace",
        Version:   1,
        CreatedAt: time.Now().UTC(),
    }
    if err := writeJSON(filepath.Join(mayorDir, "config.json"), config); err != nil {
        return nil, err
    }

    // ... create state, mail, boss, rigs

    return &Workspace{Path: absPath, Config: config}, nil
}
```

## gt doctor

### Command

```bash
gt doctor           # Check workspace health
gt doctor --fix     # Auto-fix issues
gt doctor <rig>     # Check specific rig
```

### Checks

#### Workspace Level
1. **Workspace exists**: `mayor/` or `.gastown/` directory
2. **Valid config**: `config.json` has `type: "workspace"`
3. **State file**: `state.json` exists and is valid JSON
4. **Mail directory**: `mail/` exists
5. **Boss state**: `boss/state.json` exists
6. **Rigs directory**: `rigs/` exists

#### Per-Rig Checks
1. **Refinery directory**: `<rig>/refinery/` exists
2. **Refinery README**: `refinery/README.md` exists
3. **Refinery state**: `refinery/state.json` exists
4. **Refinery lock**: `refinery/state.json.lock` exists
5. **Refinery clone**: `refinery/rig/` has valid `.git`
6. **Boss rig clone**: `mayor/rigs/<rig>/` has valid `.git`
7. **Gitignore entries**: workspace `.gitignore` has rig patterns

### Output Format

```
$ gt doctor
Workspace: ~/ai

✓ Workspace config valid
✓ Workspace state valid
✓ Mail directory exists
✓ Boss state valid

Rig: gastown
✓ Refinery directory exists
✓ Refinery README exists
✓ Refinery state valid
✗ Missing refinery/rig/ clone
✓ Mayor rig clone exists
✓ Gitignore entries present

Rig: beads
✓ Refinery directory exists
✓ Refinery README exists
✓ Refinery state valid
✓ Refinery clone valid
✓ Mayor rig clone exists
✓ Gitignore entries present

Issues: 1 found, 0 fixed
Run with --fix to auto-repair
```

### Implementation (Go)

```go
// pkg/doctor/doctor.go
package doctor

type CheckResult struct {
    Name    string
    Status  Status  // Pass, Fail, Warn
    Message string
    Fixable bool
}

type DoctorOptions struct {
    Fix     bool
    Rig     string  // Empty = all rigs
    Verbose bool
}

func Run(ws *workspace.Workspace, opts DoctorOptions) (*Report, error) {
    report := &Report{}

    // Workspace checks
    report.Add(checkWorkspaceConfig(ws))
    report.Add(checkWorkspaceState(ws))
    report.Add(checkMailDir(ws))
    report.Add(checkBossState(ws))
    report.Add(checkRigsDir(ws))

    // Per-rig checks
    rigs, _ := ws.ListRigs()
    for _, rig := range rigs {
        if opts.Rig != "" && rig.Name != opts.Rig {
            continue
        }
        report.AddRig(rig.Name, checkRig(rig, ws, opts.Fix))
    }

    return report, nil
}

func checkRefineryHealth(rig *rig.Rig, fix bool) []CheckResult {
    var results []CheckResult

    refineryDir := filepath.Join(rig.Path, "refinery")

    // Check refinery directory
    if !dirExists(refineryDir) {
        r := CheckResult{Name: "Refinery directory", Status: Fail, Fixable: true}
        if fix {
            if err := os.MkdirAll(refineryDir, 0755); err == nil {
                r.Status = Fixed
            }
        }
        results = append(results, r)
    }

    // Check README.md
    readmePath := filepath.Join(refineryDir, "README.md")
    if !fileExists(readmePath) {
        r := CheckResult{Name: "Refinery README", Status: Fail, Fixable: true}
        if fix {
            if err := writeRefineryReadme(readmePath); err == nil {
                r.Status = Fixed
            }
        }
        results = append(results, r)
    }

    // ... more checks
    return results
}
```

## Workspace Detection

Find workspace root by walking up from current directory:

```go
// pkg/workspace/find.go
func Find(startPath string) (*Workspace, error) {
    current, _ := filepath.Abs(startPath)

    for current != filepath.Dir(current) {
        // Check both "mayor" and ".gastown" directories
        for _, dirName := range []string{"mayor", ".gastown"} {
            configDir := filepath.Join(current, dirName)
            configPath := filepath.Join(configDir, "config.json")

            if fileExists(configPath) {
                var config Config
                if err := readJSON(configPath, &config); err != nil {
                    continue
                }
                if config.Type == "workspace" {
                    return &Workspace{
                        Path:      current,
                        ConfigDir: dirName,
                        Config:    config,
                    }, nil
                }
            }
        }
        current = filepath.Dir(current)
    }
    return nil, ErrNotFound
}
```

## Minimal Federation Protocol

Federation enables work distribution across multiple machines via SSH.

### Core Abstractions

#### Connection Interface

```go
// pkg/connection/connection.go
type Connection interface {
    // Command execution
    Execute(ctx context.Context, cmd string, opts ExecOpts) (*Result, error)

    // File operations
    ReadFile(path string) ([]byte, error)
    WriteFile(path string, data []byte) error
    AppendFile(path string, data []byte) error
    FileExists(path string) (bool, error)
    ListDir(path string) ([]string, error)
    MkdirAll(path string) error

    // Tmux operations
    TmuxSend(session string, text string) error
    TmuxCapture(session string, lines int) (string, error)
    TmuxHasSession(session string) (bool, error)

    // Health
    IsHealthy() bool
}
```

#### LocalConnection

```go
// pkg/connection/local.go
type LocalConnection struct{}

func (c *LocalConnection) Execute(ctx context.Context, cmd string, opts ExecOpts) (*Result, error) {
    // Direct exec.Command
}

func (c *LocalConnection) ReadFile(path string) ([]byte, error) {
    return os.ReadFile(path)
}
```

#### SSHConnection

```go
// pkg/connection/ssh.go
type SSHConnection struct {
    Host    string
    User    string
    KeyPath string
    client  *ssh.Client
}

func (c *SSHConnection) Execute(ctx context.Context, cmd string, opts ExecOpts) (*Result, error) {
    session, err := c.client.NewSession()
    if err != nil {
        return nil, err
    }
    defer session.Close()
    // Run command via SSH
}
```

### Machine Registry

```go
// pkg/federation/registry.go
type Machine struct {
    Name        string
    Type        string  // "local", "ssh", "gcp"
    Workspace   string  // Remote workspace path
    SSHHost     string
    SSHUser     string
    SSHKeyPath  string
    GCPProject  string
    GCPZone     string
    GCPInstance string
}

type Registry struct {
    machines map[string]*Machine
    conns    map[string]Connection
}

func (r *Registry) GetConnection(name string) (Connection, error) {
    if conn, ok := r.conns[name]; ok {
        return conn, nil
    }

    machine, ok := r.machines[name]
    if !ok {
        return nil, ErrMachineNotFound
    }

    var conn Connection
    switch machine.Type {
    case "local":
        conn = &LocalConnection{}
    case "ssh", "gcp":
        conn = NewSSHConnection(machine.SSHHost, machine.SSHUser, machine.SSHKeyPath)
    }

    r.conns[name] = conn
    return conn, nil
}
```

### Extended Addressing

Polecat addresses support optional machine prefix:

```
[machine:]rig/polecat

Examples:
  beads/happy           # Local machine (default)
  gcp-west:beads/happy  # Remote machine
```

```go
// pkg/identity/address.go
type PolecatAddress struct {
    Machine string  // Default: "local"
    Rig     string
    Polecat string
}

func ParseAddress(addr string) (*PolecatAddress, error) {
    parts := strings.SplitN(addr, ":", 2)
    if len(parts) == 2 {
        // machine:rig/polecat
        machine := parts[0]
        rigPolecat := strings.SplitN(parts[1], "/", 2)
        return &PolecatAddress{Machine: machine, Rig: rigPolecat[0], Polecat: rigPolecat[1]}, nil
    }
    // rig/polecat (local default)
    rigPolecat := strings.SplitN(addr, "/", 2)
    return &PolecatAddress{Machine: "local", Rig: rigPolecat[0], Polecat: rigPolecat[1]}, nil
}
```

### Mail Routing

For federation < 50 agents, use centralized mail through Mayor's machine:

```go
// pkg/mail/router.go
type MailRouter struct {
    registry *federation.Registry
}

func (r *MailRouter) Deliver(msg *Message) error {
    addr, _ := identity.ParseAddress(msg.Recipient)
    conn, err := r.registry.GetConnection(addr.Machine)
    if err != nil {
        return err
    }

    mailboxPath := filepath.Join(addr.Rig, addr.Polecat, "mail", "inbox.jsonl")
    return conn.AppendFile(mailboxPath, msg.ToJSONL())
}
```

## Implementation Plan

### Subtasks for gt-evp2

1. **Config package** - Config, State types and JSON serialization
2. **Workspace detection** - Find() walking up directory tree
3. **gt install command** - Create workspace structure
4. **Doctor framework** - Check interface, Result types, Report
5. **Workspace doctor checks** - Config, state, mail, boss, rigs
6. **Rig doctor checks** - Refinery health, clones, gitignore
7. **Connection interface** - Define protocol for local/remote ops
8. **LocalConnection** - Local file/exec/tmux operations
9. **Machine registry** - Store and manage machine configs
10. **Extended addressing** - Parse `[machine:]rig/polecat`

### Deferred (Federation Phase 2)

- SSHConnection implementation
- GCPConnection with gcloud integration
- Cross-machine mail routing
- Remote session management
- Worker pool across machines

## CLI Commands Summary

```bash
# Installation
gt install [path]           # Install workspace at path
gt install --force          # Overwrite existing

# Diagnostics
gt doctor                   # Check workspace health
gt doctor --fix             # Auto-fix issues
gt doctor <rig>             # Check specific rig
gt doctor --verbose         # Show all checks (not just failures)

# Future (federation)
gt machine list             # List machines
gt machine add <name>       # Add machine
gt machine status           # Check all machine health
```
