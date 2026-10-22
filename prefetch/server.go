package prefetch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtime"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtimehost"
)

type Server struct {
	pluginv1.UnimplementedEventConsumerServer
	pluginv1.UnimplementedHttpRoutesServer
	mu          sync.Mutex
	gate        chan struct{}
	config      Config
	processor   *Processor
	lastEvent   string
	lastError   string
	lastResults []Result
	hostInfo    func(context.Context) (*runtimehost.HostInfo, error)
}

func NewServer(client *http.Client) *Server {
	return &Server{
		gate: make(chan struct{}, 1), config: defaultConfig(), processor: NewProcessor(client),
		hostInfo: func(ctx context.Context) (*runtimehost.HostInfo, error) {
			host := runtime.Host()
			if host == nil {
				return nil, errors.New("host broker unavailable")
			}
			return host.GetHostInfo(ctx)
		},
	}
}

func (s *Server) Configure(ctx context.Context, entries []*pluginv1.ConfigEntry) error {
	cfg := defaultConfig()
	for _, entry := range entries {
		if entry.GetKey() != "prefetch" {
			continue
		}
		data, err := json.Marshal(entry.GetValue().AsMap())
		if err != nil {
			return errors.New("cannot decode Prefetch settings")
		}
		cfg, err = DecodeConfig(data)
		if err != nil {
			return errors.New("cannot decode Prefetch settings; check field names and types")
		}
	}
	select {
	case s.gate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-s.gate }()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = cfg
	s.processor = NewProcessor(s.processor.Client)
	s.lastError = ""
	return nil
}

func (s *Server) snapshot() Config { s.mu.Lock(); defer s.mu.Unlock(); return s.config }

// Resolve after startup: the host binds its broker separately from Configure.
// Keep discovery transient so host reloads take effect.
func (s *Server) runtimeConfig(ctx context.Context, cfg Config) (Config, error) {
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	info, err := s.hostInfo(ctx)
	if err != nil || info == nil || validURL(info.InternalBaseURL) != nil {
		return cfg, errors.New("cannot discover Silo's internal API URL; the host connection may be unavailable or this Silo build may need updating")
	}
	cfg.SiloURL = info.InternalBaseURL
	return cfg, nil
}

func (s *Server) HandleEvent(ctx context.Context, req *pluginv1.HandleEventRequest) (*pluginv1.HandleEventResponse, error) {
	if req.GetEventName() != "playback_sessions_changed" && req.GetEventName() != "sessions.replaced" {
		return &pluginv1.HandleEventResponse{}, nil
	}
	select {
	case s.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-s.gate }()
	cfg := s.snapshot()
	if !cfg.Enabled {
		return &pluginv1.HandleEventResponse{}, nil
	}
	cfg, err := s.runtimeConfig(ctx, cfg)
	if err != nil {
		s.record(nil, err)
		return nil, err
	}
	sessions, err := Sessions(ctx, s.processor.Client, cfg)
	if err != nil {
		s.record(nil, err)
		return nil, err
	}
	var results []Result
	var failures []error
	for _, session := range sessions {
		r, err := s.processor.Process(ctx, cfg, session)
		results = append(results, r...)
		if err != nil {
			failures = append(failures, err)
		}
	}
	err = errors.Join(failures...)
	s.record(results, err)
	if err != nil {
		return nil, err
	}
	return &pluginv1.HandleEventResponse{}, nil
}

func (s *Server) record(results []Result, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastEvent = time.Now().UTC().Format(time.RFC3339)
	s.lastError = ""
	if err != nil {
		s.lastError = err.Error()
		slog.Warn("Prefetch could not complete session event", "error", err)
	}
	if len(results) > 0 {
		s.lastResults = append([]Result(nil), results...)
		for _, r := range results {
			slog.Info("Prefetch", "instance", r.Instance, "series", r.Series, "result", r.Reason, "episodes", r.Searched)
		}
	}
}

type CheckResult struct {
	Service string
	OK      bool
	Message string
}

func (s *Server) Checks(ctx context.Context) []CheckResult {
	cfg := s.snapshot()
	cfg, err := s.runtimeConfig(ctx, cfg)
	if err != nil {
		return []CheckResult{{Service: "Configuration", Message: err.Error()}}
	}
	client := NewHTTPClient()
	var checks []CheckResult
	_, err = Sessions(ctx, client, cfg)
	checks = append(checks, checkResult("Silo sessions", err))
	for _, instance := range instances(cfg, client) {
		checks = append(checks, checkResult(instance.Name, instance.Check(ctx)))
	}
	return checks
}

func checkResult(name string, err error) CheckResult {
	if err != nil {
		return CheckResult{Service: name, Message: err.Error()}
	}
	return CheckResult{Service: name, OK: true, Message: "Connected"}
}

var page = template.Must(template.New("status").Parse(`<!doctype html>
<html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Prefetch</title><style>:root{color-scheme:light dark;font:16px system-ui}body{max-width:800px;margin:40px auto;padding:0 24px;line-height:1.6}table{border-collapse:collapse;width:100%}td,th{text-align:left;padding:8px;border-bottom:1px solid #888}a{color:light-dark(#164cca,#8bbcff)}.error{color:light-dark(#a20b0b,#ffabab)}</style>
<h1>Prefetch</h1><p>{{if .Enabled}}Prefetch is enabled.{{else}}Prefetch is disabled. Enable it in this plugin's settings after checking connections.{{end}}</p>
{{if .MinWatchedPercent}}<p>Prefetch waits until playback reaches {{.MinWatchedPercent}}% of the episode.</p>{{end}}
{{if .ConfigError}}<p class="error">{{.ConfigError}}</p>{{end}}
<p><a href="check">Test connections</a> · <a href="status">Refresh status</a></p>
{{if .Checks}}<h2>Connection checks</h2><table><tr><th>Service</th><th>Result</th></tr>{{range .Checks}}<tr><td>{{.Service}}</td><td {{if not .OK}}class="error"{{end}}>{{.Message}}</td></tr>{{end}}</table><p>These checks only read Silo and Sonarr; they do not change monitoring or request searches.</p>{{end}}
<p>Last session event: {{if .LastEvent}}{{.LastEvent}} (UTC){{else}}Waiting for playback{{end}}</p>
{{if .LastError}}<p class="error">{{.LastError}}</p>{{end}}
{{if .Results}}<h2>Recent results</h2><table><tr><th>Instance</th><th>Series</th><th>Result</th></tr>{{range .Results}}<tr><td>{{.Instance}}</td><td>{{.Series}}</td><td>{{.Reason}}{{if .Searched}} (episode IDs: {{.Searched}}){{end}}</td></tr>{{end}}</table>{{end}}
<p>Configure Sonarr, users, tags, and episodes ahead in Silo's plugin settings. Both Sonarr instances are checked when configured. Series must already exist in Sonarr. Exclusions take precedence; lists accept commas or one entry per line.</p></html>`))

func (s *Server) Handle(ctx context.Context, req *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	path := strings.TrimRight(req.GetPath(), "/")
	if req.GetMethod() != "GET" {
		return &pluginv1.HandleHTTPResponse{StatusCode: 405, Headers: map[string]string{"Allow": "GET"}}, nil
	}
	if path != "/status" && path != "/check" {
		return &pluginv1.HandleHTTPResponse{StatusCode: 404}, nil
	}
	var checks []CheckResult
	if path == "/check" {
		checks = s.Checks(ctx)
	}
	s.mu.Lock()
	data := struct {
		Enabled                           bool
		MinWatchedPercent                 int
		ConfigError, LastEvent, LastError string
		Results                           []Result
		Checks                            []CheckResult
	}{Enabled: s.config.Enabled, MinWatchedPercent: s.config.MinWatchedPercent, LastEvent: s.lastEvent, LastError: s.lastError, Results: append([]Result(nil), s.lastResults...), Checks: checks}
	if err := s.config.Validate(); err != nil {
		data.ConfigError = err.Error()
	}
	s.mu.Unlock()
	var body bytes.Buffer
	if err := page.Execute(&body, data); err != nil {
		return nil, errors.New("render Prefetch status")
	}
	return &pluginv1.HandleHTTPResponse{StatusCode: 200, Headers: map[string]string{"Content-Type": "text/html; charset=utf-8", "Cache-Control": "no-store"}, Body: body.Bytes()}, nil
}
