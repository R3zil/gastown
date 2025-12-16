# Gastown (Go)

Go port of [Gas Town](https://github.com/steveyegge/gastown-py) - a multi-agent workspace manager.

## Status

**Work in Progress** - This is the Go rewrite of the Python gastown tool.

See the [Python version](https://github.com/steveyegge/gastown-py) for current functionality.

## Goals

- Single binary installation (`gt`)
- Self-diagnosing (`gt doctor`)
- Federation support (coordinate agents across VMs)
- Performance improvements over Python version

## Development

```bash
# Build
go build -o gt ./cmd/gt

# Run
./gt --help
```

## Related

- [gastown-py](https://github.com/steveyegge/gastown-py) - Python version (current)
- [beads](https://github.com/steveyegge/beads) - Issue tracking for agents
