package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"

	"github.com/amuxos/agent-pack-contract/go/internal/testutil"
)

const helperProcessEnv = "AGENT_PACK_CONTRACT_GO_TEST_HELPER"

func TestCLIHelperProcess(t *testing.T) {
	if os.Getenv(helperProcessEnv) != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"agent-pack-contract"}, os.Args[i+1:]...)
			main()
			return
		}
	}
	os.Exit(2)
}

func TestCLIParsesEqualsFormOptions(t *testing.T) {
	fixture := testutil.SamplePackRoot(t)
	out := filepath.Join(t.TempDir(), "manifest.json")

	cmd := cliCommand(t, "check", "--repo-root="+fixture, "--out="+out)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("check with equals-form options failed: %v\n%s", err, output)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("expected manifest at requested output path: %v", err)
	}
}

func TestCLIRejectsUnknownOptions(t *testing.T) {
	cmd := cliCommand(
		t,
		"build",
		"--repo-root", testutil.SamplePackRoot(t),
		"--out", filepath.Join(t.TempDir(), "manifest.json"),
		"--bogus",
	)
	output, err := cmd.CombinedOutput()
	assertExitCode(t, err, 2, output)
}

func TestCLIRejectsMissingOptionValues(t *testing.T) {
	cmd := cliCommand(
		t,
		"render",
		"--repo-root", testutil.SamplePackRoot(t),
		"--profile", "reviewer",
		"--model",
	)
	output, err := cmd.CombinedOutput()
	assertExitCode(t, err, 2, output)
}

func TestCLISubcommandHelpSucceeds(t *testing.T) {
	cmd := cliCommand(t, "check", "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("check --help failed: %v\n%s", err, output)
	}
}

func TestCLIImportAssetCreatesUpstreamSkill(t *testing.T) {
	repoRoot := t.TempDir()
	resolvedRepoRoot, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	contentFile := testutil.FixturePath(t, "asset-input", "SKILL.md")
	content, err := os.ReadFile(contentFile)
	if err != nil {
		t.Fatal(err)
	}

	cmd := cliCommand(
		t,
		"import-asset",
		"--repo-root", repoRoot,
		"--asset-type", "skill",
		"--name", "review",
		"--origin-kind", "git",
		"--origin-locator", "example-org/review",
		"--origin-version", "v1.2.3",
		"--content-file", contentFile,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("import-asset failed: %v\n%s", err, output)
	}
	target := filepath.Join(resolvedRepoRoot, "skills", "upstream", "review")
	if !strings.Contains(string(output), "OK: imported skill into "+target) {
		t.Fatalf("unexpected output: %s", output)
	}
	gotContent, err := os.ReadFile(filepath.Join(target, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotContent) != string(content) {
		t.Fatalf("SKILL.md = %q, want %q", gotContent, content)
	}
	metadata := readTOML(t, filepath.Join(target, "source.toml"))
	if got := metadata["source_kind"]; got != "upstream" {
		t.Fatalf("source_kind = %#v, want upstream", got)
	}
	if got := metadata["asset_type"]; got != "skill" {
		t.Fatalf("asset_type = %#v, want skill", got)
	}
	origin, ok := metadata["origin"].(map[string]any)
	if !ok {
		t.Fatalf("origin = %#v, want table", metadata["origin"])
	}
	if origin["kind"] != "git" || origin["locator"] != "example-org/review" || origin["version"] != "v1.2.3" {
		t.Fatalf("origin = %#v", origin)
	}
}

func TestCLIAssetCommandsRejectPathNames(t *testing.T) {
	contentFile := testutil.FixturePath(t, "asset-input", "SKILL.md")
	repoRoot := t.TempDir()
	cmd := cliCommand(
		t,
		"import-asset",
		"--repo-root", repoRoot,
		"--asset-type", "skill",
		"--name", "../escape",
		"--origin-kind", "git",
		"--content-file", contentFile,
	)
	output, err := cmd.CombinedOutput()
	assertExitCode(t, err, 2, output)
	if !strings.Contains(string(output), "--name must be a single logical name") {
		t.Fatalf("unexpected error:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "skills", "escape")); !os.IsNotExist(err) {
		t.Fatalf("path-like asset name wrote outside upstream directory: %v", err)
	}
}

func TestCLINewCustomAssetCopiesUpstreamTree(t *testing.T) {
	repoRoot := t.TempDir()
	resolvedRepoRoot, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	upstream := filepath.Join(repoRoot, "skills", "upstream", "review")
	if err := os.CopyFS(upstream, os.DirFS(testutil.FixturePath(t, "asset-upstream", "review"))); err != nil {
		t.Fatal(err)
	}

	cmd := cliCommand(
		t,
		"new-custom-asset",
		"--repo-root", repoRoot,
		"--asset-type", "skill",
		"--source-name", "review",
		"--target-name", "team-review",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("new-custom-asset failed: %v\n%s", err, output)
	}
	target := filepath.Join(resolvedRepoRoot, "skills", "custom", "team-review")
	if !strings.Contains(string(output), "OK: created custom skill at "+target) {
		t.Fatalf("unexpected output: %s", output)
	}
	guide, err := os.ReadFile(filepath.Join(target, "references", "guide.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(guide) != "guide\n" {
		t.Fatalf("guide.md = %q", guide)
	}
	metadata := readTOML(t, filepath.Join(target, "source.toml"))
	if got := metadata["source_kind"]; got != "custom" {
		t.Fatalf("source_kind = %#v, want custom", got)
	}
	origin, ok := metadata["origin"].(map[string]any)
	if !ok {
		t.Fatalf("origin = %#v, want table", metadata["origin"])
	}
	if origin["kind"] != "git" || origin["locator"] != "example-org/review" || origin["version"] != "v1.2.3" {
		t.Fatalf("origin = %#v", origin)
	}
}

func readTOML(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := toml.Unmarshal(data, &value); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return value
}

func cliCommand(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	cmdArgs := []string{"-test.run=TestCLIHelperProcess", "--"}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.Command(os.Args[0], cmdArgs...)
	cmd.Env = append(os.Environ(), helperProcessEnv+"=1")
	return cmd
}

func assertExitCode(t *testing.T, err error, want int, output []byte) {
	t.Helper()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected exit code %d, got %v\n%s", want, err, output)
	}
	if got := exitErr.ExitCode(); got != want {
		t.Fatalf("expected exit code %d, got %d\n%s", want, got, output)
	}
}

func TestCLIProfileDescriptionsSurviveBuildCheckAndInheritance(t *testing.T) {
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(testutil.SamplePackRoot(t))); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(root, "profiles", "common", "reviewer.toml")
	source, err := os.ReadFile(base)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(base, append([]byte("description = \"  审查代码变更  \"\n"), source...), 0600); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"inherited":   "extends = \"reviewer\"\n",
		"specialized": "extends = \"reviewer\"\ndescription = \"Review concurrency\"\n",
		"plain":       "type = \"codex\"\ninstructions = []\nskills = []\nsubagents = []\n",
	} {
		if err := os.WriteFile(filepath.Join(root, "profiles", "common", name+".toml"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, command := range []string{"build", "check"} {
		out := filepath.Join(t.TempDir(), "manifest.json")
		output, err := cliCommand(t, command, "--repo-root", root, "--out", out).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", command, err, output)
		}
		output, err = cliCommand(t, "validate", out).CombinedOutput()
		if err != nil {
			t.Fatalf("validate: %v\n%s", err, output)
		}
		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		var manifest struct {
			Profiles map[string]map[string]any `json:"profiles"`
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			t.Fatal(err)
		}
		for name, want := range map[string]string{"reviewer": "审查代码变更", "inherited": "审查代码变更", "specialized": "Review concurrency"} {
			if got := manifest.Profiles[name]["description"]; got != want {
				t.Fatalf("%s description=%v, want %s", name, got, want)
			}
		}
		if _, present := manifest.Profiles["plain"]["description"]; present {
			t.Fatal("omitted description changed output")
		}
	}
	for _, bad := range []string{"42", "\"\"", "\"   \""} {
		if err := os.WriteFile(base, append([]byte("description = "+bad+"\n"), source...), 0600); err != nil {
			t.Fatal(err)
		}
		output, err := cliCommand(t, "build", "--repo-root", root, "--out", filepath.Join(t.TempDir(), "manifest.json")).CombinedOutput()
		if err == nil || !strings.Contains(string(output), "description") {
			t.Fatalf("invalid description %s: err=%v output=%s", bad, err, output)
		}
	}
}
