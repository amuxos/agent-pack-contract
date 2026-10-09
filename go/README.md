# Go implementation

Module: `github.com/amuxos/agent-pack-contract/go`.

- `internal/contract`: source loading, validation and ordered manifest generation.
- `internal/schema`: embedded JSON Schema validation.
- `internal/rendering`: Codex, Claude Code and Pi workspace projection.
- `internal/provider`: standard openai-compat legacy Pi provider projection.
- `internal/initialization`: v2 packs with version-pinned public CI.
- `internal/assets`: generic imports and custom asset derivation.
- `internal/jsonx`: stable ordered JSON serialization.

Run the complete gate from the repository root with `ci/check.sh`.
Use `ci/check-release.sh` before creating a new release. Both use the canonical
version in `internal/version/version.go`; package linker overrides use the same
public module path. JSON and Markdown output is byte-stable; TOML is semantic.
