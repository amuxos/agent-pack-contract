package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amuxos/agent-pack-contract/go/internal/version"
)

func TestCheckReleaseMetadataAcceptsCanonicalGoVersion(t *testing.T) {
	repo := releaseFixture(t)
	if err := checkReleaseMetadata(repo, "local", allowPublishedCurrent); err != nil {
		t.Fatal(err)
	}
}

func TestCheckReleaseMetadataRejectsNewerPublishedTag(t *testing.T) {
	repo := releaseFixture(t)
	current, err := parseSemver(version.Version, "test version")
	if err != nil {
		t.Fatal(err)
	}
	newer := fmt.Sprintf("v%d.%d.%d", current.major, current.minor, current.patch+1)
	runGit(t, repo, "tag", newer)

	err = checkReleaseMetadata(repo, "local", allowPublishedCurrent)
	if err == nil || !strings.Contains(err.Error(), "older than existing tag "+newer) {
		t.Fatalf("expected newer-tag rejection, got %v", err)
	}
}

func TestCheckReleaseMetadataAcceptsEqualPublishedTagForDevelopment(t *testing.T) {
	repo := releaseFixture(t)
	tag := "v" + version.Version
	runGit(t, repo, "tag", tag)

	if err := checkReleaseMetadata(repo, "local", allowPublishedCurrent); err != nil {
		t.Fatalf("development metadata check should accept published current version: %v", err)
	}
}

func TestCheckReleaseMetadataRejectsEqualPublishedTagForRelease(t *testing.T) {
	repo := releaseFixture(t)
	tag := "v" + version.Version
	runGit(t, repo, "tag", tag)

	err := checkReleaseMetadata(repo, "local", requireUnpublished)
	if err == nil || !strings.Contains(err.Error(), "already published by tag "+tag) {
		t.Fatalf("release metadata check should reject a published version, got %v", err)
	}
}

func releaseFixture(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(repo, "CHANGELOG.md"),
		[]byte("# Changelog\n\n## "+version.Version+" - today\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "contract-test@example.test")
	runGit(t, repo, "config", "user.name", "Contract Test")
	runGit(t, repo, "add", "CHANGELOG.md")
	runGit(t, repo, "commit", "-m", "fixture")
	return repo
}

func runGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"-C", repo}, args...)
	output, err := exec.Command("git", commandArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
}
