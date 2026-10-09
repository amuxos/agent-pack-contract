// Package initialization generates a minimal Agent Pack repository with public CI.
package initialization

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/amuxos/agent-pack-contract/go/internal/contract"
	"github.com/amuxos/agent-pack-contract/go/internal/errs"
	"github.com/amuxos/agent-pack-contract/go/internal/jsonx"
	"github.com/amuxos/agent-pack-contract/go/internal/version"
)

// InitializeAgentPack returns the absolute root of a newly initialized Pack.
func InitializeAgentPack(target string) (string, error) {
	repoRoot := target
	if strings.HasPrefix(repoRoot, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			switch {
			case repoRoot == "~":
				repoRoot = home
			case strings.HasPrefix(repoRoot, "~/"):
				repoRoot = filepath.Join(home, repoRoot[2:])
			default:
				// "~user" is not supported; keep repoRoot as-is.
			}
		}
	}
	if !filepath.IsAbs(repoRoot) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		// Resolve the physical working directory before joining a relative target.
		if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
			cwd = resolved
		}
		repoRoot = filepath.Join(cwd, repoRoot)
	}
	repoRoot = filepath.Clean(repoRoot)

	if info, err := os.Lstat(repoRoot); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", errs.New("%s: target must not be a symlink", repoRoot)
		}
		if !info.IsDir() {
			return "", errs.New("%s: target must be a directory", repoRoot)
		}
	}

	packName := strings.TrimSpace(filepath.Base(repoRoot))
	if packName == "" {
		return "", errs.New("Agent Pack name must be a non-empty string")
	}

	generatedFiles := map[string]string{
		filepath.Join("agent-pack.toml"): strings.Join([]string{
			`schema_version = "v2"`,
			"name = " + jsonx.Quote(packName),
			"namespace = " + jsonx.Quote(packName),
			`description = "Empty Agent Pack."`,
			"owners = []",
			"",
		}, "\n"),
		filepath.Join(".gitignore"): strings.Join([]string{
			"out/",
			"",
		}, "\n"),
		filepath.Join("ci", "check.sh"): strings.Join([]string{
			"#!/usr/bin/env bash",
			"set -euo pipefail",
			"",
			`cd "$(dirname "$0")/.."`,
			"",
			`EXPECTED_VERSION="` + version.Version + `"`,
			"# Pin the public release so checks remain reproducible.",
			`RELEASE_URL="${AGENT_PACK_CONTRACT_SCM_URL:-https://github.com/amuxos/agent-pack-contract-releases/releases/download/v${EXPECTED_VERSION}/agent-pack-contract-scm-latest.tar.gz}"`,
			`INSTALL_DIR="$(mktemp -d)"`,
			`trap 'rm -rf "$INSTALL_DIR"' EXIT`,
			"",
			`curl -fsSL "$RELEASE_URL" -o "$INSTALL_DIR/latest.tar.gz"`,
			`tar -xzf "$INSTALL_DIR/latest.tar.gz" -C "$INSTALL_DIR"`,
			`"$INSTALL_DIR/install.sh" --bin-dir "$INSTALL_DIR/bin" --expected-version "$EXPECTED_VERSION"`,
			"",
			`"$INSTALL_DIR/bin/agent-pack-contract" check --repo-root .`,
			"git diff --exit-code -- dist/agent-pack.manifest.json",
			"",
		}, "\n"),
		filepath.Join(".github", "workflows", "check.yml"): strings.Join([]string{
			"name: Check",
			"on: [push, pull_request]",
			"permissions:",
			"  contents: read",
			"",
			"jobs:",
			"  check:",
			"    runs-on: ubuntu-latest",
			"    steps:",
			"      - uses: actions/checkout@v4",
			"      - run: ci/check.sh",
			"",
		}, "\n"),
	}

	managedPaths := make([]string, 0, len(generatedFiles)+1)
	for rel := range generatedFiles {
		managedPaths = append(managedPaths, filepath.Join(repoRoot, filepath.FromSlash(rel)))
	}
	managedPaths = append(managedPaths, filepath.Join(repoRoot, "dist", "agent-pack.manifest.json"))

	for _, p := range managedPaths {
		if _, err := os.Lstat(p); err == nil {
			return "", errs.New("%s: managed file already exists", p)
		}
		parent := filepath.Dir(p)
		for parent != filepath.Dir(repoRoot) {
			if info, err := os.Lstat(parent); err == nil {
				if info.Mode()&os.ModeSymlink != 0 {
					return "", errs.New("%s: managed directory must not be a symlink", parent)
				}
				if !info.IsDir() {
					return "", errs.New("%s: managed directory path is not a directory", parent)
				}
			}
			if parent == filepath.Dir(parent) {
				break
			}
			parent = filepath.Dir(parent)
		}
	}

	var createdDirs, createdFiles []string
	ensureDir := func(path string) error {
		var missing []string
		cur := path
		for {
			if _, err := os.Stat(cur); err == nil {
				break
			}
			missing = append(missing, cur)
			parent := filepath.Dir(cur)
			if parent == cur {
				break
			}
			cur = parent
		}
		for i := len(missing) - 1; i >= 0; i-- {
			if err := os.Mkdir(missing[i], 0o755); err != nil {
				return err
			}
			createdDirs = append(createdDirs, missing[i])
		}
		return nil
	}

	rollback := func() {
		for i := len(createdFiles) - 1; i >= 0; i-- {
			if info, err := os.Lstat(createdFiles[i]); err == nil && (info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
				os.Remove(createdFiles[i])
			}
		}
		for i := len(createdDirs) - 1; i >= 0; i-- {
			if info, err := os.Stat(createdDirs[i]); err == nil && info.IsDir() {
				os.Remove(createdDirs[i])
			}
		}
	}

	if err := ensureDir(repoRoot); err != nil {
		return "", err
	}
	for rel, content := range generatedFiles {
		p := filepath.Join(repoRoot, filepath.FromSlash(rel))
		if err := ensureDir(filepath.Dir(p)); err != nil {
			rollback()
			return "", err
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			rollback()
			return "", err
		}
		createdFiles = append(createdFiles, p)
	}
	os.Chmod(filepath.Join(repoRoot, "ci", "check.sh"), 0o755)

	manifestPath := filepath.Join(repoRoot, "dist", "agent-pack.manifest.json")
	if err := ensureDir(filepath.Dir(manifestPath)); err != nil {
		rollback()
		return "", err
	}
	manifest, err := contract.BuildAgentPackManifest(repoRoot)
	if err != nil {
		rollback()
		return "", err
	}
	if err := contract.WriteJSON(manifestPath, manifest); err != nil {
		rollback()
		return "", err
	}
	createdFiles = append(createdFiles, manifestPath)
	return repoRoot, nil
}
