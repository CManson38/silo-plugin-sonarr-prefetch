// update-index generates a Silo repository catalog from released platform binaries.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/manifest"
)

type platformBinary struct {
	URL      string `json:"url"`
	Checksum string `json:"checksum"`
}

type catalogPackage struct {
	Manifest     *pluginv1.PluginManifest  `json:"manifest"`
	RepoURL      string                    `json:"repo_url"`
	ChecksumsURL string                    `json:"checksums_url"`
	Binaries     map[string]platformBinary `json:"binaries"`
}

type repositoryIndex struct {
	Plugins []catalogPackage `json:"plugins"`
}

func generate(manifestPath, binaryDir string) ([]byte, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	m, err := manifest.Load(data)
	if err != nil {
		return nil, err
	}
	repoURL := m.GetPresentation().GetSourceUrl()
	if err := manifest.ValidateCatalogPresentation(m, repoURL); err != nil {
		return nil, err
	}
	// Catalogs carry platform-specific hashes instead of one manifest checksum.
	m.Checksum = ""
	baseURL := repoURL + "/releases/download/v" + m.GetVersion()
	pkg := catalogPackage{Manifest: m, RepoURL: repoURL, ChecksumsURL: baseURL + "/checksums.txt", Binaries: map[string]platformBinary{}}
	if len(m.GetSupportedPlatforms()) == 0 {
		return nil, fmt.Errorf("manifest must declare supported platforms")
	}
	for _, platform := range m.GetSupportedPlatforms() {
		key := platform.GetOs() + "/" + platform.GetArch()
		name := "plugin-" + platform.GetOs() + "-" + platform.GetArch()
		if filepath.Base(name) != name || platform.GetOs() == "" || platform.GetArch() == "" {
			return nil, fmt.Errorf("invalid platform %q", key)
		}
		if _, exists := pkg.Binaries[key]; exists {
			return nil, fmt.Errorf("duplicate platform %q", key)
		}
		binary, err := os.ReadFile(filepath.Join(binaryDir, name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		if len(binary) == 0 {
			return nil, fmt.Errorf("%s is empty", name)
		}
		sum := sha256.Sum256(binary)
		pkg.Binaries[key] = platformBinary{URL: baseURL + "/" + name, Checksum: hex.EncodeToString(sum[:])}
	}
	// Silo decodes catalogs with encoding/json, whose protobuf enum fields
	// require numeric values (e.g. admin form controls), unlike manifest files.
	output, err := json.MarshalIndent(repositoryIndex{Plugins: []catalogPackage{pkg}}, "", "  ")
	return append(output, '\n'), err
}

func main() {
	manifestPath := flag.String("manifest", "manifest.json", "source plugin manifest")
	binaryDir := flag.String("dist", "dist", "directory containing plugin-OS-ARCH binaries")
	output := flag.String("output", "index.json", "catalog output path")
	flag.Parse()
	data, err := generate(*manifestPath, *binaryDir)
	if err == nil {
		err = os.WriteFile(*output, data, 0644)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
