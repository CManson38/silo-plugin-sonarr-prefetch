package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/manifest"
)

func TestGenerateCatalogForSilo(t *testing.T) {
	dir := t.TempDir()
	binaries := map[string][]byte{"linux-amd64": []byte("linux amd64 binary"), "linux-arm64": []byte("linux arm64 binary"), "darwin-arm64": []byte("darwin arm64 binary")}
	for platform, data := range binaries {
		if err := os.WriteFile(filepath.Join(dir, "plugin-"+platform), data, 0755); err != nil {
			t.Fatal(err)
		}
	}
	data, err := generate("../../manifest.json", dir)
	if err != nil {
		t.Fatal(err)
	}
	// Match Silo's encoding/json catalog decoder, including numeric proto enums.
	var index repositoryIndex
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatalf("Silo catalog decoding failed: %v", err)
	}
	if len(index.Plugins) != 1 {
		t.Fatalf("expected one catalog entry: %+v", index)
	}
	pkg := index.Plugins[0]
	if err := manifest.Validate(pkg.Manifest); err != nil {
		t.Fatal(err)
	}
	if err := manifest.ValidateCatalogPresentation(pkg.Manifest, pkg.RepoURL); err != nil {
		t.Fatal(err)
	}
	if pkg.Manifest.GetChecksum() != "" || len(pkg.Binaries) != len(binaries) {
		t.Fatal("catalog must use per-platform binary checksums")
	}
	for key, binary := range pkg.Binaries {
		platform := strings.ReplaceAll(key, "/", "-")
		sum := sha256.Sum256(binaries[platform])
		if binary.Checksum != hex.EncodeToString(sum[:]) || !strings.HasSuffix(binary.URL, "/plugin-"+platform) {
			t.Fatalf("wrong download for %s: %+v", key, binary)
		}
		if !strings.HasPrefix(binary.URL, pkg.RepoURL+"/releases/download/v"+pkg.Manifest.GetVersion()+"/") {
			t.Fatal("download does not refer to the catalog version")
		}
	}
}

func TestGenerateRejectsIncompleteRelease(t *testing.T) {
	dir := t.TempDir()
	if _, err := generate("../../manifest.json", dir); err == nil {
		t.Fatal("catalog accepted missing platform binaries")
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin-linux-amd64"), nil, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := generate("../../manifest.json", dir); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("empty binary accepted: %v", err)
	}
}

func TestPublishedIndexIsSiloCompatible(t *testing.T) {
	data, err := os.ReadFile("../../index.json")
	if err != nil {
		t.Fatal(err)
	}
	var index repositoryIndex
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatal(err)
	}
	if len(index.Plugins) != 1 {
		t.Fatal("expected one published plugin")
	}
	pkg := index.Plugins[0]
	if err := manifest.Validate(pkg.Manifest); err != nil {
		t.Fatal(err)
	}
	if err := manifest.ValidateCatalogPresentation(pkg.Manifest, pkg.RepoURL); err != nil {
		t.Fatal(err)
	}
	if pkg.Manifest.GetChecksum() != "" {
		t.Fatal("catalog must use per-platform checksums")
	}
	for _, platform := range pkg.Manifest.GetSupportedPlatforms() {
		key := platform.GetOs() + "/" + platform.GetArch()
		binary, ok := pkg.Binaries[key]
		if !ok {
			t.Fatalf("missing platform %s", key)
		}
		digest, err := hex.DecodeString(binary.Checksum)
		if err != nil || len(digest) != sha256.Size {
			t.Fatalf("invalid checksum for %s", key)
		}
		expected := pkg.RepoURL + "/releases/download/v" + pkg.Manifest.GetVersion() + "/plugin-" + platform.GetOs() + "-" + platform.GetArch()
		if binary.URL != expected {
			t.Fatalf("unexpected release URL for %s", key)
		}
	}
}
