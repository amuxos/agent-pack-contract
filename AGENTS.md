# Agent Pack Contract maintenance

- CLI name: `agent-pack-contract`; Go module: `github.com/amuxos/agent-pack-contract/go`.
- New packs use manifest v2. Existing neutral v1.1 packs remain supported.
- V2 rejects profile execution fields, top-level providers, and provider source files;
  caller-selected runtime/model/permission settings belong only to rendered output.
- Source compilation, validation, initialization and rendering are independent of
  any application or business pack. Test fixtures must be generic.
- Public workspace engines are Codex, Claude Code and Pi; legacy providers use
  standard openai-compat. Keep product-specific integrations outside this build.
- Repository and embedded JSON Schemas must remain byte-identical.
- JSON outputs use ordered serialization; TOML output is compared semantically.
- Keep init/build/check/validate/render/import-asset/new-custom-asset CLI contracts
  stable. New init output must use public, version-pinned downloads and GitHub CI.
- Do not choose a source-code license or change repository visibility implicitly.
- Before commit or push, run `ci/check.sh`. Before release, run
  `ci/check-release.sh`, which also requires a new SemVer and CHANGELOG entry.
- Verify all four release archives and checksums, native install/CLI behavior,
  anonymous public download, and consuming amux authoring before claiming delivery.
- Build/package environment variables and bundle filenames are compatibility APIs.
