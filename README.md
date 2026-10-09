# Agent Pack Contract

[简体中文](README.zh-CN.md)

Agent Pack Contract builds and validates portable agent content and renders local
workspaces for Codex, Claude Code, and Pi. New packs use **manifest v2**: instructions,
skills, subagents, descriptions, inheritance, and environment-variable names belong
to the pack; the caller chooses the runtime, model, and permission mode.

## Install

Download the public release bundle and run its checksum-verifying installer:

```sh
work="$(mktemp -d)"
curl -fsSL https://github.com/amuxos/agent-pack-contract-releases/releases/latest/download/agent-pack-contract-scm-latest.tar.gz -o "$work/release.tar.gz"
tar -xzf "$work/release.tar.gz" -C "$work"
"$work/install.sh" --bin-dir "$HOME/.local/bin"
agent-pack-contract --version
```

Releases provide macOS and Linux archives for amd64 and arm64, SHA-256 sidecars,
`latest.json`, and a combined bundle. The existing bundle filename and `SCM_*`
packaging variables remain stable for consumers. Source and binary downloads are public and require no account or private network.

## Create and validate a pack

```sh
agent-pack-contract init example-pack
cd example-pack
mkdir -p profiles/common instructions/common/reviewer
printf 'Review changes for correctness.\n' > instructions/common/reviewer/AGENTS.md
cat > profiles/common/reviewer.toml <<'TOML'
description = "Review code changes."
instructions = ["instructions/common/reviewer/AGENTS.md"]
skills = []
subagents = []
TOML
agent-pack-contract check --repo-root .
agent-pack-contract validate dist/agent-pack.manifest.json
agent-pack-contract render --repo-root . --profile reviewer --agent codex --out out/reviewer
```

`init` also generates a GitHub workflow and `ci/check.sh` pinned to the creating
Contract version. It never overwrites existing managed files. `render --agent`
is required for v2; optional `--model` and `--permission-mode default|plan|bypass`
select execution settings without changing the source pack. Unsupported engine
or permission combinations fail before changing output.

V2 rejects execution fields (`type`, `model`, `providers`, `supported_models`,
`permission`) even when empty, plus top-level providers and provider source files.
Existing neutral v1.1 packs remain readable; their execution defaults do not
carry over implicitly when migrating to v2. Legacy Pi provider projection supports
standard `openai-compat` connections. See [migration](docs/profile-v2-migration.md).

Generic asset maintenance uses `import-asset` and `new-custom-asset`; use
`agent-pack-contract --help` for arguments. Pack content remains owned by its authors.

## Development

```sh
ci/check.sh          # complete development gate
ci/check-release.sh  # complete gate plus unpublished-version check
./build.sh           # native platform archive
./scm_build_release.sh  # four supported platform archives and latest.json
```

CI uses Go 1.24.12. See [release procedure](docs/releasing-agent-pack-contract.md).

## License

Agent Pack Contract is licensed under the [MIT License](LICENSE). Third-party
components retain their own licenses; see [THIRD_PARTY_NOTICES](THIRD_PARTY_NOTICES).
Both files are included in each platform release archive. Pack content remains
under the license chosen by its authors.
