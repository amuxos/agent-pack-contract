# Release procedure

1. Update the canonical SemVer and CHANGELOG. Freeze the final source diff.
2. Run `ci/check-release.sh` with the CI Go 1.24.12 toolchain. This includes the
   full development gate and rejects an already published source version.
3. Review the source diff and release assets for non-public integrations, private
   endpoints, credentials, and unexpected files. Source repository visibility is
   managed separately from binary publication; do not change it implicitly.
4. Commit and push the tested source. Verify its CI and record its exact commit.
5. Run `scm_build_release.sh` with `CUSTOM_AGENT_PACK_CONTRACT_RELEASE_BASE_URL`
   unset. The bundle's `latest.json` must contain relative archive filenames:
   amux resolves these entries within the downloaded bundle and rejects URLs.
   Defaults build linux/amd64, linux/arm64, darwin/amd64 and darwin/arm64.
6. Package the resulting output files as `agent-pack-contract-scm-latest.tar.gz`;
   retain individual platform archives, SHA-256 files, `latest.json` and `install.sh`.
7. Create a draft release in `amuxos/agent-pack-contract-releases`, upload the exact
   validated assets and record source provenance. Verify uploaded digests before
   publishing the release as latest. Never overwrite existing version assets.
8. Download without authentication and verify native install, version, v2
   init/check/validate/render, old compatible packs, and amux managed authoring.

Use the previous version's immutable download URL to roll back. `SCM_*` build
variables, `--scm-url` and the bundle filename remain accepted compatibility names;
all defaults use public downloads.

Source is published under MIT. Preserve LICENSE and THIRD_PARTY_NOTICES in every
platform archive. When dependency or Go toolchain versions change, review their
LICENSE/NOTICE/PATENTS files and update THIRD_PARTY_NOTICES with the release.
Version 0.8.2 begins the public source history; earlier binary releases keep their
original immutable assets and provenance. Do not retag or overwrite them.
