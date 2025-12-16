# Claude: Gastown Go Port

Run `bd prime` for beads context.

## Project Info

This is the **Go port** of Gas Town, a multi-agent workspace manager.

- **Issue prefix**: `gt-`
- **Python version**: ~/ai/gastown-py (reference implementation)
- **Design docs**: docs/town-design.md

## Development

```bash
go build -o gt ./cmd/gt
go test ./...
```

## Key Epics

- `gt-fqwd`: Port Gas Town to Go (main tracking epic)
- `gt-evp2`: Town & Rig Management (install, doctor, federation)
