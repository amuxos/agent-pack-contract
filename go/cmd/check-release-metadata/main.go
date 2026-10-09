// Command check-release-metadata validates the canonical Go release version,
// CHANGELOG entry, and published Git tags.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/amuxos/agent-pack-contract/go/internal/version"
)

var semverPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

type semver struct {
	major int
	minor int
	patch int
}

type publishedVersionPolicy int

const (
	allowPublishedCurrent publishedVersionPolicy = iota
	requireUnpublished
)

func parseSemver(value, context string) (semver, error) {
	match := semverPattern.FindStringSubmatch(value)
	if match == nil {
		return semver{}, fmt.Errorf("%s: expected SemVer X.Y.Z, got %q", context, value)
	}
	parts := make([]int, 3)
	for i := range parts {
		parsed, err := strconv.Atoi(match[i+1])
		if err != nil {
			return semver{}, fmt.Errorf("%s: invalid SemVer %q: %w", context, value, err)
		}
		parts[i] = parsed
	}
	return semver{major: parts[0], minor: parts[1], patch: parts[2]}, nil
}

func compareSemver(left, right semver) int {
	leftParts := [...]int{left.major, left.minor, left.patch}
	rightParts := [...]int{right.major, right.minor, right.patch}
	for i := range leftParts {
		if leftParts[i] < rightParts[i] {
			return -1
		}
		if leftParts[i] > rightParts[i] {
			return 1
		}
	}
	return 0
}

func git(repoRoot string, args ...string) (string, error) {
	commandArgs := append([]string{"-C", repoRoot}, args...)
	command := exec.Command("git", commandArgs...)
	output, err := command.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			detail = err.Error()
		}
		return "", errors.New(detail)
	}
	return strings.TrimSpace(string(output)), nil
}

func localTags(repoRoot string) (map[string]string, error) {
	output, err := git(repoRoot, "tag", "--list", "v*")
	if err != nil {
		return nil, fmt.Errorf("cannot query local release tags: %w", err)
	}
	tags := make(map[string]string)
	for _, tag := range strings.Fields(output) {
		commit, err := git(repoRoot, "rev-list", "-n", "1", tag)
		if err != nil {
			return nil, fmt.Errorf("cannot query local release tags: %w", err)
		}
		tags[tag] = commit
	}
	return tags, nil
}

func remoteTags(repoRoot string) (map[string]string, error) {
	output, err := git(repoRoot, "ls-remote", "--tags", "origin", "refs/tags/v*")
	if err != nil {
		return nil, fmt.Errorf("cannot query remote release tags: %w", err)
	}
	tags := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			return nil, fmt.Errorf("cannot query remote release tags: malformed ls-remote line %q", scanner.Text())
		}
		const prefix = "refs/tags/"
		if !strings.HasPrefix(fields[1], prefix) {
			continue
		}
		tag := strings.TrimPrefix(fields[1], prefix)
		peeled := strings.HasSuffix(tag, "^{}")
		tag = strings.TrimSuffix(tag, "^{}")
		if _, exists := tags[tag]; !exists || peeled {
			tags[tag] = fields[0]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("cannot query remote release tags: %w", err)
	}
	return tags, nil
}

func checkReleaseMetadata(repoRoot, tagSource string, policy publishedVersionPolicy) error {
	current, err := parseSemver(version.Version, "Go release version")
	if err != nil {
		return err
	}

	changelog, err := os.ReadFile(filepath.Join(repoRoot, "CHANGELOG.md"))
	if err != nil {
		return fmt.Errorf("missing CHANGELOG.md: %w", err)
	}
	heading := regexp.MustCompile(`(?m)^##\s+\[?` + regexp.QuoteMeta(version.Version) + `\]?(?:\s+-|\s*$)`)
	if !heading.Match(changelog) {
		return fmt.Errorf("CHANGELOG.md has no entry for version %s", version.Version)
	}

	var tags map[string]string
	switch tagSource {
	case "local":
		tags, err = localTags(repoRoot)
	case "remote":
		tags, err = remoteTags(repoRoot)
	default:
		return fmt.Errorf("invalid tag source %q", tagSource)
	}
	if err != nil {
		return err
	}

	for tag := range tags {
		tagVersion, err := parseSemver(strings.TrimPrefix(tag, "v"), tag)
		if err != nil {
			continue
		}
		switch compareSemver(tagVersion, current) {
		case 1:
			return fmt.Errorf("release version %s is older than existing tag %s", version.Version, tag)
		case 0:
			if policy == requireUnpublished {
				return fmt.Errorf("release version %s is already published by tag %s; bump it", version.Version, tag)
			}
		}
	}
	return nil
}

func main() {
	repoRoot := flag.String("repo-root", ".", "repository root")
	tagSource := flag.String("tag-source", "local", "release tag source: local or remote")
	requireUnpublishedVersion := flag.Bool("require-unpublished", false, "require the current version to be newer than every published tag")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "ERROR: unexpected argument(s): %s\n", strings.Join(flag.Args(), " "))
		os.Exit(2)
	}
	absoluteRoot, err := filepath.Abs(*repoRoot)
	if err == nil {
		policy := allowPublishedCurrent
		if *requireUnpublishedVersion {
			policy = requireUnpublished
		}
		err = checkReleaseMetadata(absoluteRoot, *tagSource, policy)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("OK: Contract release metadata is valid (%s)\n", version.Version)
}
