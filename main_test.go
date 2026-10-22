package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	configsdk "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/config"
	manifestsdk "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/manifest"
	runtimesdk "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtime"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
)

type testRuntimeHost struct {
	pluginv1.UnimplementedRuntimeHostServer
	address string
	calls   atomic.Int32
}

func (h *testRuntimeHost) GetHostInfo(context.Context, *pluginv1.GetHostInfoRequest) (*pluginv1.GetHostInfoResponse, error) {
	h.calls.Add(1)
	return &pluginv1.GetHostInfoResponse{InternalBaseUrl: h.address}, nil
}

func TestBuiltPluginHandshakeAndPlaybackEvent(t *testing.T) {
	// Build and launch the real executable, then use the same SDK transport as Silo.
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	binary := filepath.Join(t.TempDir(), "plugin"+ext)
	build := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"+ext), "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build plugin: %v\n%s", err, out)
	}
	output, err := exec.Command(binary, "manifest").Output()
	if err != nil {
		t.Fatal(err)
	}
	var manifest pluginv1.PluginManifest
	if err = protojson.Unmarshal(output, &manifest); err != nil {
		t.Fatal(err)
	}
	if err = manifestsdk.Validate(&manifest); err != nil {
		t.Fatal(err)
	}
	if err = manifestsdk.ValidateCatalogPresentation(&manifest, "https://github.com/CManson38/silo-plugin-sonarr-prefetch"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(binary)
	sum := sha256.Sum256(data)
	if manifest.Checksum != hex.EncodeToString(sum[:]) {
		t.Fatal("self-reported binary checksum mismatch")
	}
	if manifest.PluginId != "community.requests.prefetch" {
		t.Fatal("wrong plugin identity")
	}
	if len(manifest.SupportedPlatforms) != 3 {
		t.Fatal("missing release platform")
	}
	for _, schema := range manifest.GlobalConfigSchema {
		if !strings.Contains(schema.JsonSchema, "episodes_ahead") {
			t.Fatal("missing configuration")
		}
		var settings struct {
			Properties map[string]any `json:"properties"`
		}
		if err := json.Unmarshal([]byte(schema.JsonSchema), &settings); err != nil {
			t.Fatal(err)
		}
		if _, exists := settings.Properties["silo_url"]; exists {
			t.Fatal("Silo URL still exposed in settings")
		}
		for _, field := range schema.GetAdminForm().GetFields() {
			if field.Key == "silo_url" {
				t.Fatal("Silo URL still exposed in form")
			}
		}
	}
	var searches atomic.Int32
	var position atomic.Int64
	var active atomic.Bool
	position.Store(99)
	active.Store(true)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/api/v2/") {
			if r.Header.Get("Authorization") != "Bearer silo-key" {
				t.Error("Silo auth missing")
			}
			if !active.Load() {
				w.Write([]byte(`{"items":[],"page":{}}`))
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{{"session_id": "play-1", "user_id": "10", "username": "Viewer", "media_type": "series", "series_name": "Example", "season_number": 1, "episode_number": 1, "file_duration": 1000, "position_seconds": position.Load()}}, "page": map[string]any{}})
			return
		}
		if r.Header.Get("X-Api-Key") != "sonarr-key" {
			t.Error("Sonarr auth missing")
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v3/system/status":
			w.Write([]byte(`{"version":"4"}`))
		case "GET /api/v3/series":
			w.Write([]byte(`[{"id":1,"title":"Example","monitored":true}]`))
		case "GET /api/v3/episode":
			w.Write([]byte(`[{"id":2,"seasonNumber":1,"episodeNumber":2,"hasFile":false,"airDateUtc":"2020-01-01T00:00:00Z"}]`))
		case "PUT /api/v3/episode/monitor":
			w.Write([]byte(`[]`))
		case "GET /api/v3/queue":
			w.Write([]byte(`{"records":[]}`))
		case "POST /api/v3/command":
			searches.Add(1)
			w.WriteHeader(201)
			w.Write([]byte(`{"id":1}`))
		default:
			t.Errorf("unexpected call %s %s", r.Method, r.URL)
			w.WriteHeader(404)
		}
	}))
	defer backend.Close()
	cfg := map[string]any{"enabled": true, "silo_api_key": "silo-key", "sonarr_url": backend.URL, "sonarr_api_key": "sonarr-key", "episodes_ahead": float64(2)}
	if err = configsdk.ValidateManifestGlobalValue(&manifest, "prefetch", cfg); err != nil {
		t.Fatal(err)
	}
	value, _ := structpb.NewStruct(cfg)
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: runtimesdk.HandshakeConfig(), Plugins: runtimesdk.DefaultPluginSet(runtimesdk.CapabilityServers{}),
		Cmd: exec.Command(binary), AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC}, Logger: hclog.NewNullLogger(),
	})
	defer client.Kill()
	rpc, err := client.Client()
	if err != nil {
		t.Fatal(err)
	}
	dispensed, err := rpc.Dispense(runtimesdk.PluginSetName)
	if err != nil {
		t.Fatal(err)
	}
	sdk := dispensed.(*runtimesdk.Client)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	host := &testRuntimeHost{address: backend.URL}
	broker := sdk.Broker()
	streamID := broker.NextId()
	go broker.AcceptAndServe(streamID, func(opts []grpc.ServerOption) *grpc.Server {
		server := grpc.NewServer(opts...)
		pluginv1.RegisterRuntimeHostServer(server, host)
		return server
	})
	if _, err = sdk.Runtime().BindHostBroker(ctx, &pluginv1.BindHostBrokerRequest{BrokerId: streamID}); err != nil {
		t.Fatal(err)
	}
	// A new installation starts without any settings and can render diagnostics.
	if _, err = sdk.Runtime().Configure(ctx, &pluginv1.ConfigureRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err = sdk.Runtime().Configure(ctx, &pluginv1.ConfigureRequest{Config: []*pluginv1.ConfigEntry{{Key: "prefetch", Value: value}}}); err != nil {
		t.Fatal(err)
	}
	// Starting and stopping below 10% must not trigger a search. A later
	// progress notification for the same episode must remain eligible.
	for _, present := range []bool{true, false, true} {
		active.Store(present)
		if _, err = sdk.EventConsumer().HandleEvent(ctx, &pluginv1.HandleEventRequest{EventName: "sessions.replaced"}); err != nil {
			t.Fatal(err)
		}
		if searches.Load() != 0 {
			t.Fatal("searched before the progress threshold")
		}
	}
	position.Store(100)
	for i := 0; i < 2; i++ {
		if _, err = sdk.EventConsumer().HandleEvent(ctx, &pluginv1.HandleEventRequest{EventName: "playback_sessions_changed", EventId: "event-1"}); err != nil {
			t.Fatal(err)
		}
	}
	if searches.Load() != 1 {
		t.Fatalf("wanted one search through real RPC, got %d", searches.Load())
	}
	response, err := sdk.HttpRoutes().Handle(ctx, &pluginv1.HandleHTTPRequest{Method: "GET", Path: "/status"})
	if err != nil || response.GetStatusCode() != 200 {
		t.Fatalf("status route failed: %v", err)
	}
	if strings.Contains(string(response.Body), "silo-key") || strings.Contains(string(response.Body), "sonarr-key") {
		t.Fatal("status leaked credentials")
	}
	response, err = sdk.HttpRoutes().Handle(ctx, &pluginv1.HandleHTTPRequest{Method: "GET", Path: "/check"})
	if err != nil || !strings.Contains(string(response.GetBody()), "Connected") {
		t.Fatalf("discovered connection check failed: %v", err)
	}
	if host.calls.Load() < 3 {
		t.Fatal("playback events and connection checks did not use host discovery")
	}
	var source map[string]any
	if json.Unmarshal(manifestJSON, &source) != nil {
		t.Fatal("invalid source manifest")
	}
}
