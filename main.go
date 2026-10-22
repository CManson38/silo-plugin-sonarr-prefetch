package main

import (
	_ "embed"

	"github.com/CManson38/silo-plugin-requests-prefetch/prefetch"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtime"
)

var version string

//go:embed manifest.json
var manifestJSON []byte

func main() {
	s := prefetch.NewServer(nil)
	runtime.ServeManifestWithOptions(manifestJSON, version,
		runtime.CapabilityServers{EventConsumer: s, HttpRoutes: s}, runtime.WithConfigure(s.Configure))
}
