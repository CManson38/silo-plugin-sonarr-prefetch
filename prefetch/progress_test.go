package prefetch

import (
	"context"
	"math"
	"testing"
)

func TestProgressThreshold(t *testing.T) {
	for _, tc := range []struct {
		name      string
		threshold int
		position  float64
		duration  *int
		want      bool
	}{
		{"disabled", 0, 0, nil, true},
		{"below", 10, 99.9, ptr(1000), false},
		{"boundary", 10, 100, ptr(1000), true},
		{"resumed past threshold", 10, 500, ptr(1000), true},
		{"completed", 100, 1000, ptr(1000), true},
		{"position exceeds duration", 100, 1001, ptr(1000), true},
		{"unknown duration", 10, 500, nil, false},
		{"zero duration", 10, 500, ptr(0), false},
		{"negative duration", 10, 500, ptr(-1), false},
		{"negative position", 10, -1, ptr(1000), false},
		{"invalid position", 10, math.NaN(), ptr(1000), false},
		{"infinite position", 10, math.Inf(1), ptr(1000), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session := Session{PositionSeconds: tc.position, FileDuration: tc.duration}
			if got := session.reachedProgress(tc.threshold); got != tc.want {
				t.Fatalf("reachedProgress = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestProgressDoesNotMonitorOrSearchUntilThreshold(t *testing.T) {
	f := newSonarr(t)
	p := NewProcessor(nil)
	cfg := Config{Enabled: true, EpisodesAhead: 2, MinWatchedPercent: 10, SonarrURL: f.server.URL + "/sonarr", SonarrAPIKey: "sonarr-secret"}
	session := playing()
	session.FileDuration = ptr(1000)
	session.PositionSeconds = 99
	results, err := p.Process(context.Background(), cfg, session)
	if err != nil || len(results) != 0 || len(f.monitor) != 0 || len(f.commands) != 0 || f.seriesWrites != 0 || len(p.processed) != 0 {
		t.Fatalf("work performed below threshold: %+v %v", results, err)
	}
	session.PositionSeconds = 100
	results, err = p.Process(context.Background(), cfg, session)
	if err != nil || len(results) != 1 || len(f.commands) != 1 || len(f.monitor) != 1 {
		t.Fatalf("did not process threshold crossing: %+v %v", results, err)
	}
	if _, err = p.Process(context.Background(), cfg, session); err != nil || len(f.commands) != 1 {
		t.Fatal("repeated progress triggered another search")
	}
}

func TestProgressConfiguration(t *testing.T) {
	cfg, err := DecodeConfig([]byte(`{"silo_api_key":"key","sonarr_url":"http://sonarr","sonarr_api_key":"key"}`))
	if err != nil || cfg.MinWatchedPercent != 10 {
		t.Fatal("omitted threshold should default to 10%")
	}
	explicit, err := DecodeConfig([]byte(`{"min_watched_percent":0}`))
	if err != nil || explicit.MinWatchedPercent != 0 {
		t.Fatal("explicit zero should preserve immediate prefetch")
	}
	for _, value := range []int{-1, 101} {
		cfg.MinWatchedPercent = value
		if cfg.Validate() == nil {
			t.Errorf("accepted threshold %d", value)
		}
	}
	for _, value := range []int{0, 10, 100} {
		cfg.MinWatchedPercent = value
		if err := cfg.Validate(); err != nil {
			t.Errorf("rejected threshold %d: %v", value, err)
		}
	}
}
