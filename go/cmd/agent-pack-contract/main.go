// Command agent-pack-contract is the canonical Agent Pack Contract CLI.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/amuxos/agent-pack-contract/go/internal/assets"
	"github.com/amuxos/agent-pack-contract/go/internal/contract"
	"github.com/amuxos/agent-pack-contract/go/internal/errs"
	"github.com/amuxos/agent-pack-contract/go/internal/initialization"
	"github.com/amuxos/agent-pack-contract/go/internal/rendering"
	"github.com/amuxos/agent-pack-contract/go/internal/schema"
	"github.com/amuxos/agent-pack-contract/go/internal/version"
)

const usage = `agent-pack-contract — Agent Pack contract tools

Usage:
  agent-pack-contract --version
  agent-pack-contract init [target]
  agent-pack-contract build   --repo-root <dir> [--out <path>]
  agent-pack-contract check   --repo-root <dir> [--out <path>]
  agent-pack-contract validate <manifest>
  agent-pack-contract render  --repo-root <dir> --profile <name> [--agent <engine>] [--model <ref>] [--permission-mode <mode>] [--manifest <path>] [--out <dir>]
  agent-pack-contract import-asset --repo-root <dir> --asset-type <skill|subagent> --name <name> --origin-kind <kind> --content-file <path> [--origin-locator <value>] [--origin-version <value>]
  agent-pack-contract new-custom-asset --repo-root <dir> --asset-type <skill|subagent> --source-name <name> --target-name <name>
`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(argv []string) int {
	if len(argv) < 1 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	if len(argv) == 1 && (argv[0] == "--help" || argv[0] == "-h" || argv[0] == "help") {
		fmt.Print(usage)
		return 0
	}
	if len(argv) == 1 && (argv[0] == "--version" || argv[0] == "-V" || argv[0] == "version") {
		fmt.Println(version.Version)
		return 0
	}
	cmd, args := argv[0], argv[1:]
	handlers := map[string]func([]string) error{
		"init":             runInit,
		"build":            runBuild,
		"check":            runCheck,
		"validate":         runValidate,
		"render":           runRender,
		"import-asset":     runImportAsset,
		"new-custom-asset": runNewCustomAsset,
	}
	h, ok := handlers[cmd]
	if !ok {
		fmt.Fprintf(os.Stderr, "ERROR: unknown command %q\n", cmd)
		return 2
	}
	if err := h(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Print(usage)
			return 0
		}
		var ue *usageError
		if errors.As(err, &ue) {
			fmt.Fprintf(os.Stderr, "ERROR: %s\n", ue.Error())
			return 2
		}
		var ce *errs.ContractError
		if errors.As(err, &ce) {
			fmt.Fprintf(os.Stderr, "ERROR: %s\n", ce.Error())
		} else {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		}
		return 1
	}
	return 0
}

func runImportAsset(args []string) error {
	flags := flag.NewFlagSet("import-asset", flag.ContinueOnError)
	repoRootValue := "."
	assetType := ""
	name := ""
	originKind := ""
	contentFile := ""
	originLocator := ""
	originVersion := ""
	flags.StringVar(&repoRootValue, "repo-root", repoRootValue, "Agent Pack repository root")
	flags.StringVar(&assetType, "asset-type", assetType, "asset type: skill or subagent")
	flags.StringVar(&name, "name", name, "asset logical name")
	flags.StringVar(&originKind, "origin-kind", originKind, "asset origin kind")
	flags.StringVar(&contentFile, "content-file", contentFile, "asset content file")
	flags.StringVar(&originLocator, "origin-locator", originLocator, "optional asset origin locator")
	flags.StringVar(&originVersion, "origin-version", originVersion, "optional asset origin version")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if err := rejectPositionals(flags); err != nil {
		return err
	}
	if assetType != "skill" && assetType != "subagent" {
		return &usageError{message: "import-asset: --asset-type must be skill or subagent"}
	}
	if err := validateAssetName("import-asset", "--name", name); err != nil {
		return err
	}
	if strings.TrimSpace(originKind) == "" {
		return &usageError{message: "import-asset: --origin-kind is required"}
	}
	if strings.TrimSpace(contentFile) == "" {
		return &usageError{message: "import-asset: --content-file is required"}
	}
	repoRoot, err := absRepoRoot(repoRootValue)
	if err != nil {
		return err
	}
	target, err := assets.ImportAsset(assets.ImportAssetOptions{
		RepoRoot:      repoRoot,
		AssetType:     assetType,
		Name:          name,
		OriginKind:    originKind,
		ContentFile:   contentFile,
		OriginLocator: originLocator,
		OriginVersion: originVersion,
	})
	if err != nil {
		return err
	}
	fmt.Printf("OK: imported %s into %s\n", assetType, target)
	return nil
}

func runNewCustomAsset(args []string) error {
	flags := flag.NewFlagSet("new-custom-asset", flag.ContinueOnError)
	repoRootValue := "."
	assetType := ""
	sourceName := ""
	targetName := ""
	flags.StringVar(&repoRootValue, "repo-root", repoRootValue, "Agent Pack repository root")
	flags.StringVar(&assetType, "asset-type", assetType, "asset type: skill or subagent")
	flags.StringVar(&sourceName, "source-name", sourceName, "upstream asset name")
	flags.StringVar(&targetName, "target-name", targetName, "custom asset name")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if err := rejectPositionals(flags); err != nil {
		return err
	}
	if assetType != "skill" && assetType != "subagent" {
		return &usageError{message: "new-custom-asset: --asset-type must be skill or subagent"}
	}
	if err := validateAssetName("new-custom-asset", "--source-name", sourceName); err != nil {
		return err
	}
	if err := validateAssetName("new-custom-asset", "--target-name", targetName); err != nil {
		return err
	}
	repoRoot, err := absRepoRoot(repoRootValue)
	if err != nil {
		return err
	}
	target, err := assets.NewCustomAsset(assets.NewCustomAssetOptions{
		RepoRoot:   repoRoot,
		AssetType:  assetType,
		SourceName: sourceName,
		TargetName: targetName,
	})
	if err != nil {
		return err
	}
	fmt.Printf("OK: created custom %s at %s\n", assetType, target)
	return nil
}

func validateAssetName(command, flagName, value string) error {
	if strings.TrimSpace(value) == "" {
		return &usageError{message: command + ": " + flagName + " is required"}
	}
	if value != strings.TrimSpace(value) || value == "." || value == ".." || strings.ContainsAny(value, `/\`) {
		return &usageError{message: command + ": " + flagName + " must be a single logical name"}
	}
	return nil
}

type usageError struct {
	message string
}

func (e *usageError) Error() string { return e.message }

func parseFlags(flags *flag.FlagSet, args []string) error {
	flags.SetOutput(io.Discard)
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return flag.ErrHelp
		}
		return &usageError{message: fmt.Sprintf("%s: %v", flags.Name(), err)}
	}
	return nil
}

func rejectPositionals(flags *flag.FlagSet) error {
	if flags.NArg() == 0 {
		return nil
	}
	return &usageError{message: fmt.Sprintf("%s: unexpected argument(s): %s", flags.Name(), strings.Join(flags.Args(), " "))}
}

func absRepoRoot(value string) (string, error) {
	if value == "" {
		value = "."
	}
	p, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	// Resolve symlinks so paths use their physical form (macOS /var -> /private/var).
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved, nil
	}
	return p, nil
}

func resolveOutput(repoRoot, value string) string {
	if filepath.IsAbs(value) {
		return value
	}
	return filepath.Join(repoRoot, value)
}

func runInit(args []string) error {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if flags.NArg() > 1 {
		return &usageError{message: "init: expected at most one target directory"}
	}
	target := "."
	if flags.NArg() == 1 {
		target = flags.Arg(0)
	}
	root, err := initialization.InitializeAgentPack(target)
	if err != nil {
		return err
	}
	fmt.Printf("OK: initialized empty Agent Pack at %s\n", root)
	return nil
}

func runBuild(args []string) error {
	repoRootValue, out, err := parseRepositoryFlags("build", args)
	if err != nil {
		return err
	}
	repoRoot, err := absRepoRoot(repoRootValue)
	if err != nil {
		return err
	}
	output := resolveOutput(repoRoot, out)
	manifest, err := contract.BuildAgentPackManifest(repoRoot)
	if err != nil {
		return err
	}
	if err := contract.WriteJSON(output, manifest); err != nil {
		return err
	}
	fmt.Printf("OK: wrote %s\n", output)
	return nil
}

func runCheck(args []string) error {
	repoRootValue, out, err := parseRepositoryFlags("check", args)
	if err != nil {
		return err
	}
	repoRoot, err := absRepoRoot(repoRootValue)
	if err != nil {
		return err
	}
	output := resolveOutput(repoRoot, out)
	if err := schema.ValidateRepositorySchema(repoRoot, false); err != nil {
		return err
	}
	warnings, err := contract.ValidateContract(repoRoot)
	if err != nil {
		return err
	}
	manifest, err := contract.BuildAgentPackManifest(repoRoot)
	if err != nil {
		return err
	}
	if err := contract.WriteJSON(output, manifest); err != nil {
		return err
	}
	for _, w := range warnings {
		fmt.Println(w)
	}
	fmt.Println("OK: repo contract is valid")
	return nil
}

func runValidate(args []string) error {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return &usageError{message: "validate: expected exactly one manifest path"}
	}
	manifestPath, err := absRepoRoot(flags.Arg(0))
	if err != nil {
		return err
	}
	manifest, err := contract.LoadAgentPackManifest(manifestPath)
	if err != nil {
		return err
	}
	if err := schema.Validate(manifest); err != nil {
		return err
	}
	fmt.Printf("OK: manifest is valid: %s\n", manifestPath)
	return nil
}

func runRender(args []string) error {
	flags := flag.NewFlagSet("render", flag.ContinueOnError)
	repoRootValue := "."
	manifestValue := "dist/agent-pack.manifest.json"
	profile := ""
	model := ""
	agent := ""
	permissionMode := ""
	out := ""
	flags.StringVar(&repoRootValue, "repo-root", repoRootValue, "Agent Pack repository root")
	flags.StringVar(&manifestValue, "manifest", manifestValue, "manifest path")
	flags.StringVar(&profile, "profile", profile, "profile logical name")
	flags.StringVar(&agent, "agent", agent, "execution engine (required for v2)")
	flags.StringVar(&permissionMode, "permission-mode", permissionMode, "execution permission mode: default, plan, bypass")
	flags.StringVar(&model, "model", model, "model ref")
	flags.StringVar(&out, "out", out, "output directory")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if err := rejectPositionals(flags); err != nil {
		return err
	}
	if profile == "" {
		return &usageError{message: "render: --profile is required"}
	}
	repoRoot, err := absRepoRoot(repoRootValue)
	if err != nil {
		return err
	}
	manifestPath := resolveOutput(repoRoot, manifestValue)
	if out == "" {
		out = filepath.Join("out", "local", profile)
	}
	output := resolveOutput(repoRoot, out)
	if err := rendering.RenderLocalWorkspaceWithOptions(repoRoot, profile, output, manifestPath, rendering.ExecutionOptions{Agent: agent, Model: model, PermissionMode: permissionMode}); err != nil {
		return err
	}
	fmt.Printf("OK: rendered local workspace for profile %s to %s\n", profile, output)
	return nil
}

func parseRepositoryFlags(command string, args []string) (string, string, error) {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	repoRoot := "."
	out := "dist/agent-pack.manifest.json"
	flags.StringVar(&repoRoot, "repo-root", repoRoot, "Agent Pack repository root")
	flags.StringVar(&out, "out", out, "output path")
	if err := parseFlags(flags, args); err != nil {
		return "", "", err
	}
	if err := rejectPositionals(flags); err != nil {
		return "", "", err
	}
	return repoRoot, out, nil
}
