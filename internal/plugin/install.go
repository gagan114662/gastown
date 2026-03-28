package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const installMetadataFile = ".gastown-install.json"

// InstallMetadata records where a plugin package was installed from.
type InstallMetadata struct {
	Name              string    `json:"name"`
	Source            string    `json:"source"`
	APIVersion        string    `json:"api_version,omitempty"`
	MinGastownVersion string    `json:"min_gastown_version,omitempty"`
	InstalledAt       time.Time `json:"installed_at"`
}

// InstallPlugins installs one plugin or a source directory of plugins into targetDir.
// When pluginName is empty, every plugin in sourceDir is synced.
func InstallPlugins(sourceDir, targetDir, pluginName, sourceRef string) (*SyncResult, error) {
	if strings.TrimSpace(pluginName) == "" {
		result, err := SyncPlugins(sourceDir, targetDir, false)
		if err != nil {
			return nil, err
		}
		for _, name := range append(append([]string{}, result.Copied...), result.Skipped...) {
			if err := writeInstallMetadataForName(sourceDir, targetDir, name, sourceRef); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", name, err))
			}
		}
		return result, nil
	}

	srcPluginDir := filepath.Join(sourceDir, pluginName)
	info, err := os.Stat(srcPluginDir)
	if err != nil {
		return nil, fmt.Errorf("plugin %q not found in %s: %w", pluginName, sourceDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("plugin %q source is not a directory", pluginName)
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return nil, fmt.Errorf("creating target directory: %w", err)
	}

	result := &SyncResult{}
	dstPluginDir := filepath.Join(targetDir, pluginName)
	if dirsMatch(srcPluginDir, dstPluginDir) {
		result.Skipped = append(result.Skipped, pluginName)
	} else {
		if err := copyDir(srcPluginDir, dstPluginDir); err != nil {
			return nil, err
		}
		result.Copied = append(result.Copied, pluginName)
	}

	if err := writeInstallMetadataForName(sourceDir, targetDir, pluginName, sourceRef); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", pluginName, err))
	}
	return result, nil
}

// LoadInstallMetadata reads install metadata for an installed plugin.
func LoadInstallMetadata(pluginDir string) (*InstallMetadata, error) {
	data, err := os.ReadFile(filepath.Join(pluginDir, installMetadataFile)) //nolint:gosec // G304: plugin dir is caller-controlled
	if err != nil {
		return nil, err
	}
	var meta InstallMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func writeInstallMetadataForName(sourceDir, targetDir, pluginName, sourceRef string) error {
	plug, err := LoadPluginDir(filepath.Join(sourceDir, pluginName))
	if err != nil {
		return err
	}
	meta := &InstallMetadata{
		Name:              plug.Name,
		Source:            sourceRef,
		APIVersion:        plug.APIVersion,
		MinGastownVersion: plug.MinGastownVersion,
		InstalledAt:       time.Now().UTC(),
	}
	return writeInstallMetadata(filepath.Join(targetDir, pluginName), meta)
}

func writeInstallMetadata(pluginDir string, meta *InstallMetadata) error {
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(pluginDir, installMetadataFile), data, 0644) //nolint:gosec // metadata is non-secret
}
