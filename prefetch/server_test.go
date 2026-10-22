package prefetch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtimehost"
	"google.golang.org/protobuf/types/known/structpb"
)

func configure(t *testing.T, s *Server, cfg Config) {
	t.Helper()
	if cfg.SiloURL != "" {
		address := cfg.SiloURL
		s.hostInfo = func(context.Context) (*runtimehost.HostInfo, error) {
			return &runtimehost.HostInfo{InternalBaseURL: address}, nil
		}
	}
	data, _ := json.Marshal(cfg)
	var values map[string]any
	json.Unmarshal(data, &values)
	v, err := structpb.NewStruct(values)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Configure(context.Background(), []*pluginv1.ConfigEntry{{Key: "prefetch", Value: v}}); err != nil {
		t.Fatal(err)
	}
}
func silo(t *testing.T, sessions []Session) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer silo-secret" || r.URL.Path != "/silo/api/v2/admin/sessions" || r.URL.Query().Get("limit") != "200" {
			t.Errorf("unexpected Silo request: %s %s", r.Method, r.URL)
			w.WriteHeader(401)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"items": sessions, "page": map[string]any{"next_cursor": nil}})
	}))
	t.Cleanup(s.Close)
	return s
}
func TestEventPrefetchesBothInstancesAndRetriesOnlyFailure(t *testing.T) {
	a, b := newSonarr(t), newSonarr(t)
	b.failCommand = true
	host := silo(t, []Session{playing()})
	s := NewServer(nil)
	cfg := Config{Enabled: true, EpisodesAhead: 2, SiloURL: host.URL + "/silo", SiloAPIKey: "silo-secret", SonarrURL: a.server.URL + "/sonarr", SonarrAPIKey: "sonarr-secret", Sonarr4KURL: b.server.URL + "/sonarr", Sonarr4KAPIKey: "sonarr-secret"}
	configure(t, s, cfg)
	event := &pluginv1.HandleEventRequest{EventName: "playback_sessions_changed", EventId: "event-1"}
	if _, err := s.HandleEvent(context.Background(), event); err == nil {
		t.Fatal("must report failed instance for retry")
	}
	if len(a.commands) != 1 || len(b.commands) != 0 {
		t.Fatal("first instance was not processed independently")
	}
	b.failCommand = false
	if _, err := s.HandleEvent(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if len(a.commands) != 1 || len(b.commands) != 1 {
		t.Fatal("retry repeated a successful instance")
	}
	event.EventName = "sessions.replaced"
	event.EventId = "another-event"
	if _, err := s.HandleEvent(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if len(a.commands) != 1 || len(b.commands) != 1 {
		t.Fatal("session notifications duplicated searches")
	}
	checks := s.Checks(context.Background())
	if len(checks) != 3 {
		t.Fatalf("checks: %+v", checks)
	}
	for _, c := range checks {
		if !c.OK {
			t.Fatalf("check failed: %+v", c)
		}
	}
	response, err := s.Handle(context.Background(), &pluginv1.HandleHTTPRequest{Method: "GET", Path: "/status"})
	if err != nil || response.GetStatusCode() != 200 || strings.Contains(string(response.Body), "secret") {
		t.Fatalf("unsafe status response: %v", err)
	}
}
func TestNoRequestsForDisabledOrUnrelatedEvents(t *testing.T) {
	s := NewServer(nil)
	if err := s.Configure(context.Background(), nil); err != nil {
		t.Fatal("fresh installation must start", err)
	}
	for _, event := range []string{"playback_sessions_changed", "unrelated"} {
		if _, err := s.HandleEvent(context.Background(), &pluginv1.HandleEventRequest{EventName: event}); err != nil {
			t.Fatal(err)
		}
	}
	if checks := s.Checks(context.Background()); len(checks) != 1 || checks[0].OK {
		t.Fatal("missing configuration not reported")
	}
}
func TestSkipsMoviesSpecialsPausedAndExcludedUsers(t *testing.T) {
	for _, name := range []string{"movie", "special", "paused", "excluded", "missing numbers"} {
		t.Run(name, func(t *testing.T) {
			session := playing()
			cfg := Config{Enabled: true, EpisodesAhead: 2}
			switch name {
			case "movie":
				session.MediaType = "movie"
			case "special":
				session.SeasonNumber = ptr(0)
			case "paused":
				session.IsPaused = true
			case "excluded":
				cfg.ExcludeUsers = "Viewer"
			case "missing numbers":
				session.EpisodeNumber = nil
			}
			p := NewProcessor(nil)
			if results, err := p.Process(context.Background(), cfg, session); err != nil || len(results) > 0 {
				t.Fatalf("session not skipped: %+v %v", results, err)
			}
		})
	}
}
func TestPaginationAndAuthentication(t *testing.T) {
	requests := 0
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Authorization") != "Bearer silo-secret" {
			t.Error("missing bearer auth")
		}
		if requests == 1 {
			json.NewEncoder(w).Encode(map[string]any{"items": []Session{playing()}, "page": map[string]any{"next_cursor": "a + b"}})
			return
		}
		if r.URL.Query().Get("cursor") != "a + b" {
			t.Error("cursor incorrectly escaped")
		}
		w.Write([]byte(`{"items":[],"page":{"next_cursor":null}}`))
	}))
	defer host.Close()
	all, err := Sessions(context.Background(), NewHTTPClient(), Config{SiloURL: host.URL, SiloAPIKey: "silo-secret"})
	if err != nil || len(all) != 1 || requests != 2 {
		t.Fatalf("pagination: %d %v", len(all), err)
	}
}
func TestRepeatedCursorFails(t *testing.T) {
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"items":[],"page":{"next_cursor":"same"}}`))
	}))
	defer host.Close()
	_, err := Sessions(context.Background(), NewHTTPClient(), Config{SiloURL: host.URL})
	if err == nil {
		t.Fatal("accepted repeated cursor")
	}
}

func TestMalformedSessionsResponseFailsConnectionCheck(t *testing.T) {
	for _, body := range []string{`{}`, `{"items":null}`, `{"items":"invalid"}`} {
		t.Run(body, func(t *testing.T) {
			host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
			defer host.Close()
			if _, err := Sessions(context.Background(), NewHTTPClient(), Config{SiloURL: host.URL}); err == nil {
				t.Fatal("invalid sessions response was treated as a successful connection")
			}
		})
	}
}
func TestConcurrentEvents(t *testing.T) {
	f := newSonarr(t)
	host := silo(t, []Session{playing()})
	s := NewServer(nil)
	configure(t, s, Config{Enabled: true, EpisodesAhead: 2, SiloURL: host.URL + "/silo", SiloAPIKey: "silo-secret", SonarrURL: f.server.URL + "/sonarr", SonarrAPIKey: "sonarr-secret"})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			_, err := s.HandleEvent(context.Background(), &pluginv1.HandleEventRequest{EventName: "sessions.replaced"})
			if err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if len(f.commands) != 1 {
		t.Fatal("concurrent events duplicated downloads")
	}
}
func TestConfigValidation(t *testing.T) {
	base := Config{EpisodesAhead: 2, SiloURL: "http://silo:8080", SiloAPIKey: "key", SonarrURL: "http://sonarr:8989", SonarrAPIKey: "key"}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "sonarr:8989", "http://user:secret@sonarr", "ftp://sonarr", "http://sonarr?apiKey=secret"} {
		cfg := base
		cfg.SonarrURL = bad
		if cfg.Validate() == nil {
			t.Errorf("accepted URL %q", bad)
		}
	}
	cfg := base
	cfg.Sonarr4KURL = "http://4k:8989"
	if cfg.Validate() == nil {
		t.Fatal("accepted incomplete 4k configuration")
	}
	cfg = base
	cfg.EpisodesAhead = 0
	if cfg.Validate() == nil {
		t.Fatal("accepted zero episodes")
	}
	cfg = base
	cfg.EpisodesAhead = 101
	if cfg.Validate() == nil {
		t.Fatal("accepted excessive episodes")
	}
	cfg, err := DecodeConfig([]byte(`{"enabled":true}`))
	if err != nil || cfg.EpisodesAhead != 2 {
		t.Fatal("default lost")
	}
}
func TestRPCDeadlineWhileBusy(t *testing.T) {
	s := NewServer(nil)
	s.gate <- struct{}{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if _, err := s.HandleEvent(ctx, &pluginv1.HandleEventRequest{EventName: "sessions.replaced"}); err == nil {
		t.Fatal("ignored cancellation")
	}
	<-s.gate
}

func TestHostDiscoveryFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		info *runtimehost.HostInfo
		err  error
	}{
		{name: "unavailable", err: errors.New("private upstream details")},
		{name: "nil response"},
		{name: "public URL only", info: &runtimehost.HostInfo{PublicBaseURL: "http://external.example"}},
		{name: "invalid internal URL", info: &runtimehost.HostInfo{InternalBaseURL: "http://user:secret@host"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewServer(nil)
			s.hostInfo = func(context.Context) (*runtimehost.HostInfo, error) { return tc.info, tc.err }
			configure(t, s, Config{Enabled: true, EpisodesAhead: 2, SiloAPIKey: "silo-secret", SonarrURL: "http://sonarr", SonarrAPIKey: "sonarr-secret"})
			if _, err := s.HandleEvent(context.Background(), &pluginv1.HandleEventRequest{EventName: "sessions.replaced"}); err == nil || !strings.Contains(err.Error(), "discover") || strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "secret") {
				t.Fatalf("unsafe or missing discovery error: %v", err)
			}
			checks := s.Checks(context.Background())
			if len(checks) != 1 || checks[0].OK || !strings.Contains(checks[0].Message, "discover") {
				t.Fatalf("missing discovery check error: %+v", checks)
			}
		})
	}
}

func TestDiscoveryUsesCurrentHostAndDiscardsLegacyURL(t *testing.T) {
	cfg, err := DecodeConfig([]byte(`{"silo_url":"http://old-host","silo_api_key":"key","sonarr_url":"http://sonarr","sonarr_api_key":"key"}`))
	if err != nil || cfg.SiloURL != "" || cfg.Validate() != nil {
		t.Fatalf("legacy config: %+v %v", cfg, err)
	}
	s := NewServer(nil)
	address := "http://127.0.0.1:8080"
	s.hostInfo = func(context.Context) (*runtimehost.HostInfo, error) {
		return &runtimehost.HostInfo{InternalBaseURL: address}, nil
	}
	for _, target := range []string{"http://127.0.0.1:8080", "http://127.0.0.1:9090"} {
		address = target
		resolved, err := s.runtimeConfig(context.Background(), cfg)
		if err != nil || resolved.SiloURL != target {
			t.Fatalf("discovery: %s %v", resolved.SiloURL, err)
		}
	}
	if s.snapshot().SiloURL != "" {
		t.Fatal("discovered address persisted to settings")
	}
}

func TestDiscoveryHonorsCallerDeadline(t *testing.T) {
	s := NewServer(nil)
	s.hostInfo = func(ctx context.Context) (*runtimehost.HostInfo, error) { <-ctx.Done(); return nil, ctx.Err() }
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	_, err := s.runtimeConfig(ctx, Config{EpisodesAhead: 2, SiloAPIKey: "key", SonarrURL: "http://sonarr", SonarrAPIKey: "key"})
	if err == nil || ctx.Err() == nil {
		t.Fatal("discovery ignored the caller's deadline")
	}
}
