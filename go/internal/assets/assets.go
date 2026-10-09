// Package assets imports and customizes Agent Pack assets.
package assets

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/amuxos/agent-pack-contract/go/internal/contract"
	"github.com/amuxos/agent-pack-contract/go/internal/errs"
)

type ImportAssetOptions struct {
	RepoRoot      string
	AssetType     string
	Name          string
	OriginKind    string
	ContentFile   string
	OriginLocator string
	OriginVersion string
}

type NewCustomAssetOptions struct {
	RepoRoot   string
	AssetType  string
	SourceName string
	TargetName string
}

type sourceMetadata struct {
	SourceKind string         `toml:"source_kind"`
	AssetType  string         `toml:"asset_type"`
	Origin     originMetadata `toml:"origin"`
}

type originMetadata struct {
	Kind    string `toml:"kind"`
	Locator string `toml:"locator,omitempty"`
	Version string `toml:"version,omitempty"`
}

func ImportAsset(options ImportAssetOptions) (string, error) {
	targetDir := filepath.Join(contract.AssetRoot(options.RepoRoot, options.AssetType, "upstream"), options.Name)
	if err := os.MkdirAll(filepath.Dir(targetDir), 0o755); err != nil {
		return "", err
	}
	if err := os.Mkdir(targetDir, 0o755); err != nil {
		if os.IsExist(err) {
			return "", errs.New("%s already exists", targetDir)
		}
		return "", err
	}
	contentTarget := filepath.Join(targetDir, contract.AssetContentFilename(options.AssetType))
	if err := copyFile(options.ContentFile, contentTarget); err != nil {
		return "", err
	}
	metadata := sourceMetadata{
		SourceKind: "upstream",
		AssetType:  options.AssetType,
		Origin: originMetadata{
			Kind:    options.OriginKind,
			Locator: options.OriginLocator,
			Version: options.OriginVersion,
		},
	}
	if err := writeSourceMetadata(filepath.Join(targetDir, "source.toml"), metadata); err != nil {
		return "", err
	}
	return targetDir, nil
}

func NewCustomAsset(options NewCustomAssetOptions) (string, error) {
	sourceDir := filepath.Join(contract.AssetRoot(options.RepoRoot, options.AssetType, "upstream"), options.SourceName)
	if info, err := os.Stat(sourceDir); err != nil || !info.IsDir() {
		return "", errs.New("missing upstream asset %q", options.SourceName)
	}
	targetDir := filepath.Join(contract.AssetRoot(options.RepoRoot, options.AssetType, "custom"), options.TargetName)
	if err := copyTree(sourceDir, targetDir); err != nil {
		return "", err
	}
	data, err := contract.LoadToml(filepath.Join(targetDir, "source.toml"))
	if err != nil {
		return "", err
	}
	origin := originMetadata{Kind: "local"}
	if raw, ok := data["origin"].(map[string]any); ok {
		if value, ok := raw["kind"].(string); ok && strings.TrimSpace(value) != "" {
			origin.Kind = strings.TrimSpace(value)
		}
		if value, ok := raw["locator"].(string); ok && strings.TrimSpace(value) != "" {
			origin.Locator = strings.TrimSpace(value)
		}
		if value, ok := raw["version"].(string); ok && strings.TrimSpace(value) != "" {
			origin.Version = strings.TrimSpace(value)
		}
	}
	metadata := sourceMetadata{
		SourceKind: "custom",
		AssetType:  options.AssetType,
		Origin:     origin,
	}
	if err := writeSourceMetadata(filepath.Join(targetDir, "source.toml"), metadata); err != nil {
		return "", err
	}
	return targetDir, nil
}

func copyTree(sourceDir, targetDir string) error {
	if _, err := os.Lstat(targetDir); err == nil {
		return errs.New("%s already exists", targetDir)
	} else if !os.IsNotExist(err) {
		return err
	}
	info, err := os.Stat(sourceDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(targetDir), 0o755); err != nil {
		return err
	}
	return copyDir(sourceDir, targetDir, info.Mode().Perm())
}

func copyDir(sourceDir, targetDir string, mode os.FileMode) error {
	if err := os.Mkdir(targetDir, mode); err != nil {
		return err
	}
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		source := filepath.Join(sourceDir, entry.Name())
		target := filepath.Join(targetDir, entry.Name())
		info, err := os.Stat(source)
		if err != nil {
			return err
		}
		if info.IsDir() {
			if err := copyDir(source, target, info.Mode().Perm()); err != nil {
				return err
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return errs.New("unsupported asset entry %s", source)
		}
		if err := copyFileMode(source, target, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(source, target string) error {
	return copyFileMode(source, target, 0o644)
}

func copyFileMode(source, target string, mode os.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func writeSourceMetadata(path string, metadata sourceMetadata) error {
	data, err := toml.Marshal(metadata)
	if err != nil {
		return errs.New("marshal TOML for %s: %v", path, err)
	}
	return os.WriteFile(path, data, 0o644)
}
