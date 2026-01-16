#!/usr/bin/env bash
#
# Gas Town Bootstrap Script
# Sets up Gas Town from scratch for codebase analysis and rebuild experiments
#
# Usage:
#   ./bootstrap-gastown.sh
#   ./bootstrap-gastown.sh --repo https://github.com/org/repo.git --name myproject
#   ./bootstrap-gastown.sh --skip-prereqs  # Skip prerequisite installation
#

set -euo pipefail

# =============================================================================
# Configuration
# =============================================================================

GT_HOME="${GT_HOME:-$HOME/gt}"
GO_VERSION_MIN="1.21"
GIT_VERSION_MIN="2.20"
TMUX_VERSION_MIN="3.0"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# =============================================================================
# Helper Functions
# =============================================================================

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[OK]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_step() {
    echo -e "\n${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${CYAN}  $1${NC}"
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}\n"
}

command_exists() {
    command -v "$1" &> /dev/null
}

version_gte() {
    # Returns 0 if $1 >= $2
    printf '%s\n%s\n' "$2" "$1" | sort -V -C
}

get_os() {
    case "$(uname -s)" in
        Darwin*) echo "macos" ;;
        Linux*)  echo "linux" ;;
        *)       echo "unknown" ;;
    esac
}

get_linux_distro() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        echo "$ID"
    else
        echo "unknown"
    fi
}

# =============================================================================
# Parse Arguments
# =============================================================================

REPO_URL=""
RIG_NAME=""
SKIP_PREREQS=false
CREW_NAME="${USER:-developer}"

while [[ $# -gt 0 ]]; do
    case $1 in
        --repo)
            REPO_URL="$2"
            shift 2
            ;;
        --name)
            RIG_NAME="$2"
            shift 2
            ;;
        --crew)
            CREW_NAME="$2"
            shift 2
            ;;
        --gt-home)
            GT_HOME="$2"
            shift 2
            ;;
        --skip-prereqs)
            SKIP_PREREQS=true
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --repo URL       Git repository URL to add as a rig"
            echo "  --name NAME      Name for the rig (derived from URL if not specified)"
            echo "  --crew NAME      Your crew member name (default: \$USER)"
            echo "  --gt-home PATH   Gas Town home directory (default: ~/gt)"
            echo "  --skip-prereqs   Skip prerequisite installation"
            echo "  --help, -h       Show this help message"
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            exit 1
            ;;
    esac
done

# =============================================================================
# Banner
# =============================================================================

echo -e "${CYAN}"
cat << 'EOF'
   ____            _____
  / ___| __ _ ___ |_   _|____      ___ __
 | |  _ / _` / __|  | |/ _ \ \ /\ / / '_ \
 | |_| | (_| \__ \  | | (_) \ V  V /| | | |
  \____|\__,_|___/  |_|\___/ \_/\_/ |_| |_|

  Multi-Agent Orchestration System
  Bootstrap Script v1.0
EOF
echo -e "${NC}"

log_info "Gas Town Home: $GT_HOME"
log_info "Operating System: $(get_os)"
[ -n "$REPO_URL" ] && log_info "Repository: $REPO_URL"

# =============================================================================
# Step 1: Check and Install Prerequisites
# =============================================================================

log_step "Step 1: Checking Prerequisites"

install_go() {
    local os=$(get_os)
    log_info "Installing Go..."

    if [ "$os" = "macos" ]; then
        if command_exists brew; then
            brew install go
        else
            log_error "Homebrew not found. Please install Go manually: https://go.dev/dl/"
            exit 1
        fi
    elif [ "$os" = "linux" ]; then
        local distro=$(get_linux_distro)
        case "$distro" in
            ubuntu|debian)
                # Use official Go installer for latest version
                log_info "Downloading Go from official source..."
                local go_version="1.24.2"
                local go_tar="go${go_version}.linux-amd64.tar.gz"
                curl -LO "https://go.dev/dl/${go_tar}"
                sudo rm -rf /usr/local/go
                sudo tar -C /usr/local -xzf "$go_tar"
                rm "$go_tar"
                export PATH=$PATH:/usr/local/go/bin
                ;;
            fedora|rhel|centos)
                sudo dnf install -y golang
                ;;
            arch)
                sudo pacman -S --noconfirm go
                ;;
            *)
                log_error "Unknown distro. Please install Go manually: https://go.dev/dl/"
                exit 1
                ;;
        esac
    fi
}

install_git() {
    local os=$(get_os)
    log_info "Installing Git..."

    if [ "$os" = "macos" ]; then
        brew install git
    elif [ "$os" = "linux" ]; then
        local distro=$(get_linux_distro)
        case "$distro" in
            ubuntu|debian)
                sudo apt-get update && sudo apt-get install -y git
                ;;
            fedora|rhel|centos)
                sudo dnf install -y git
                ;;
            arch)
                sudo pacman -S --noconfirm git
                ;;
        esac
    fi
}

install_tmux() {
    local os=$(get_os)
    log_info "Installing tmux..."

    if [ "$os" = "macos" ]; then
        brew install tmux
    elif [ "$os" = "linux" ]; then
        local distro=$(get_linux_distro)
        case "$distro" in
            ubuntu|debian)
                sudo apt-get update && sudo apt-get install -y tmux
                ;;
            fedora|rhel|centos)
                sudo dnf install -y tmux
                ;;
            arch)
                sudo pacman -S --noconfirm tmux
                ;;
        esac
    fi
}

if [ "$SKIP_PREREQS" = false ]; then
    # Check Go
    if command_exists go; then
        go_version=$(go version | grep -oE '[0-9]+\.[0-9]+(\.[0-9]+)?' | head -1)
        if version_gte "$go_version" "$GO_VERSION_MIN"; then
            log_success "Go $go_version installed"
        else
            log_warn "Go $go_version is below minimum $GO_VERSION_MIN, upgrading..."
            install_go
        fi
    else
        log_warn "Go not found, installing..."
        install_go
    fi

    # Check Git
    if command_exists git; then
        git_version=$(git --version | grep -oE '[0-9]+\.[0-9]+(\.[0-9]+)?' | head -1)
        if version_gte "$git_version" "$GIT_VERSION_MIN"; then
            log_success "Git $git_version installed"
        else
            log_warn "Git $git_version is below minimum $GIT_VERSION_MIN, upgrading..."
            install_git
        fi
    else
        log_warn "Git not found, installing..."
        install_git
    fi

    # Check tmux
    if command_exists tmux; then
        tmux_version=$(tmux -V | grep -oE '[0-9]+\.[0-9]+' | head -1)
        if version_gte "$tmux_version" "$TMUX_VERSION_MIN"; then
            log_success "tmux $tmux_version installed"
        else
            log_warn "tmux $tmux_version is below minimum $TMUX_VERSION_MIN, upgrading..."
            install_tmux
        fi
    else
        log_warn "tmux not found, installing..."
        install_tmux
    fi
else
    log_info "Skipping prerequisite checks (--skip-prereqs)"
fi

# Ensure Go bin is in PATH
if [[ ":$PATH:" != *":$HOME/go/bin:"* ]]; then
    export PATH="$PATH:$HOME/go/bin"
    log_info "Added \$HOME/go/bin to PATH for this session"

    # Add to shell config
    shell_rc=""
    if [ -f "$HOME/.zshrc" ]; then
        shell_rc="$HOME/.zshrc"
    elif [ -f "$HOME/.bashrc" ]; then
        shell_rc="$HOME/.bashrc"
    fi

    if [ -n "$shell_rc" ]; then
        if ! grep -q 'go/bin' "$shell_rc" 2>/dev/null; then
            echo 'export PATH="$PATH:$HOME/go/bin"' >> "$shell_rc"
            log_info "Added Go bin to $shell_rc"
        fi
    fi
fi

# Also add /usr/local/go/bin if it exists
if [ -d "/usr/local/go/bin" ] && [[ ":$PATH:" != *":/usr/local/go/bin:"* ]]; then
    export PATH="$PATH:/usr/local/go/bin"
fi

# =============================================================================
# Step 2: Install Gas Town and Beads
# =============================================================================

log_step "Step 2: Installing Gas Town and Beads"

# Install/update Gas Town
if command_exists gt; then
    log_info "Gas Town already installed, checking for updates..."
fi
log_info "Installing Gas Town CLI..."
go install github.com/steveyegge/gastown/cmd/gt@latest
log_success "Gas Town CLI installed: $(gt version 2>/dev/null || echo 'installed')"

# Install/update Beads
if command_exists bd; then
    log_info "Beads already installed, checking for updates..."
fi
log_info "Installing Beads CLI..."
go install github.com/steveyegge/beads/cmd/bd@latest
log_success "Beads CLI installed: $(bd version 2>/dev/null || echo 'installed')"

# =============================================================================
# Step 3: Create Gas Town Workspace
# =============================================================================

log_step "Step 3: Creating Gas Town Workspace"

if [ -d "$GT_HOME" ] && [ -f "$GT_HOME/mayor/town.json" ]; then
    log_info "Gas Town workspace already exists at $GT_HOME"
    log_info "Skipping workspace creation"
else
    log_info "Creating Gas Town workspace at $GT_HOME..."
    gt install "$GT_HOME" --git
    log_success "Workspace created at $GT_HOME"
fi

cd "$GT_HOME"

# =============================================================================
# Step 4: Run Health Check
# =============================================================================

log_step "Step 4: Running Health Check"

log_info "Running gt doctor..."
if gt doctor --fix; then
    log_success "Health check passed"
else
    log_warn "Some issues found, attempting auto-fix..."
    gt doctor --fix || true
fi

# =============================================================================
# Step 5: Add Repository as Rig (if provided)
# =============================================================================

if [ -n "$REPO_URL" ]; then
    log_step "Step 5: Adding Repository as Rig"

    # Derive rig name from URL if not provided
    if [ -z "$RIG_NAME" ]; then
        RIG_NAME=$(basename "$REPO_URL" .git | tr '[:upper:]' '[:lower:]' | tr '-' '_' | tr '.' '_')
    fi

    # Derive prefix (first two chars of each word, max 4 chars)
    PREFIX=$(echo "$RIG_NAME" | sed 's/_/ /g' | awk '{for(i=1;i<=NF;i++) printf substr($i,1,2)}' | tr '[:upper:]' '[:lower:]' | cut -c1-4)

    log_info "Rig name: $RIG_NAME"
    log_info "Beads prefix: $PREFIX"

    # Check if rig already exists
    if gt rig list 2>/dev/null | grep -q "$RIG_NAME"; then
        log_info "Rig '$RIG_NAME' already exists, skipping..."
    else
        log_info "Adding rig '$RIG_NAME' from $REPO_URL..."
        gt rig add "$RIG_NAME" "$REPO_URL" --prefix "$PREFIX"
        log_success "Rig '$RIG_NAME' added"
    fi

    # Add crew member
    log_info "Adding crew member '$CREW_NAME'..."
    if gt crew add "$CREW_NAME" --rig "$RIG_NAME" 2>/dev/null; then
        log_success "Crew member '$CREW_NAME' added"
    else
        log_info "Crew member may already exist, continuing..."
    fi

    # Boot the rig
    log_info "Booting rig (starting Witness and Refinery)..."
    gt rig boot "$RIG_NAME" || log_warn "Rig boot had issues, may need manual intervention"

    # Create analysis issues
    log_step "Step 6: Creating Analysis Issues"

    log_info "Creating analysis task issues..."

    # Store issue IDs
    ISSUE_IDS=""

    create_issue() {
        local title="$1"
        local id
        id=$(bd create --title="$title" --type=task --prefix="$PREFIX" 2>&1 | grep -oE "${PREFIX}-[0-9]+" | head -1)
        if [ -n "$id" ]; then
            log_success "Created: $id - $title"
            ISSUE_IDS="$ISSUE_IDS $id"
        else
            log_warn "Could not create issue: $title"
        fi
    }

    create_issue "Extract and document all API specifications, endpoints, and contracts"
    create_issue "Document data models, schemas, relationships, and invariants"
    create_issue "Extract business rules, domain logic, and validation requirements"
    create_issue "Map all integrations, external dependencies, and auth flows"
    create_issue "Catalog all features, user flows, and UI components"
    create_issue "Analyze test coverage, identify edge cases and gaps"

    # Create convoy
    if [ -n "$ISSUE_IDS" ]; then
        log_info "Creating analysis convoy..."
        # shellcheck disable=SC2086
        gt convoy create "${RIG_NAME} Full Analysis" $ISSUE_IDS 2>/dev/null || \
            log_warn "Convoy creation had issues, you can create it manually"
        log_success "Analysis convoy created"
    fi

    # Show summary
    log_step "Setup Complete!"

    echo -e "${GREEN}"
    cat << EOF
Gas Town is ready for your codebase analysis experiment!

Workspace:     $GT_HOME
Rig:           $RIG_NAME
Your workspace: $GT_HOME/rigs/$RIG_NAME/crew/$CREW_NAME

Next steps:
  1. Open VS Code in your crew workspace:
     code $GT_HOME/rigs/$RIG_NAME/crew/$CREW_NAME

  2. View your analysis issues:
     bd list --prefix=$PREFIX

  3. Dispatch polecats to analyze (run from $GT_HOME):
     cd $GT_HOME
EOF

    # List issues for dispatch commands
    for id in $ISSUE_IDS; do
        echo "     gt sling $id $RIG_NAME"
    done

    cat << EOF

  4. Monitor progress:
     gt convoy list
     gt polecat list $RIG_NAME

  5. Attach to Mayor for coordination:
     gt mayor attach

EOF
    echo -e "${NC}"

else
    # No repo provided
    log_step "Setup Complete!"

    echo -e "${GREEN}"
    cat << EOF
Gas Town workspace is ready!

Workspace: $GT_HOME

To add your repository, run:
  cd $GT_HOME
  gt rig add <name> <git-url> --prefix <prefix>
  gt crew add <yourname> --rig <name>
  gt rig boot <name>

Or re-run this script with your repo:
  $0 --repo https://your-repo-url.git --name your_project

EOF
    echo -e "${NC}"
fi

# =============================================================================
# Step 7: VS Code Integration Tips
# =============================================================================

log_step "VS Code Integration Tips"

cat << 'EOF'
For the best VS Code experience:

1. Install the "Remote - SSH" extension if working remotely

2. Open your workspace directly:
   code ~/gt/rigs/<your-rig>/crew/<yourname>

3. Recommended extensions:
   - GitLens (for git history)
   - GitHub Copilot (complementary AI)
   - Todo Tree (track TODOs)

4. Add to your VS Code settings.json for terminal integration:
   {
     "terminal.integrated.env.linux": {
       "PATH": "${env:PATH}:${env:HOME}/go/bin"
     },
     "terminal.integrated.env.osx": {
       "PATH": "${env:PATH}:${env:HOME}/go/bin"
     }
   }

5. Create a VS Code task (.vscode/tasks.json) for common commands:
   {
     "version": "2.0.0",
     "tasks": [
       {
         "label": "GT Status",
         "type": "shell",
         "command": "gt status",
         "problemMatcher": []
       },
       {
         "label": "GT Convoy List",
         "type": "shell",
         "command": "gt convoy list",
         "problemMatcher": []
       }
     ]
   }

EOF

log_success "Bootstrap complete! Happy orchestrating!"
