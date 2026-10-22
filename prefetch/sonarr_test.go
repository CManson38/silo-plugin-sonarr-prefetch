package prefetch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func ptr(v int) *int { return &v }
func playing() Session {
	return Session{SessionID: "session-1", MediaType: "series", SeriesName: "Example", SeasonNumber: ptr(1), EpisodeNumber: ptr(7), UserID: "10", Username: "Account", ProfileID: "profile-1", ProfileName: "Viewer"}
}
func TestWindow(t *testing.T) {
	episodes := []Episode{{ID: 4, Season: 2, Number: 2}, {ID: 1, Season: 1, Number: 7}, {ID: 9, Season: 0, Number: 1}, {ID: 3, Season: 2, Number: 1}, {ID: 2, Season: 1, Number: 8, HasFile: true}}
	got := Window(episodes, 1, 7, 2)
	if len(got) != 2 || got[0].ID != 2 || got[1].ID != 3 {
		t.Fatalf("wrong cross-season window: %+v", got)
	}
}

type fakeSonarr struct {
	server       *httptest.Server
	episodes     []Episode
	series       []map[string]any
	queued       []map[string]any
	commands     [][]int
	monitor      [][]int
	seriesWrites int
	failCommand  bool
	failTags     bool
}

func newSonarr(t *testing.T) *fakeSonarr {
	t.Helper()
	f := &fakeSonarr{series: []map[string]any{{"id": 12, "title": "Example", "monitored": false, "tags": []int{3}, "qualityProfileId": 7, "seasons": []map[string]any{{"seasonNumber": 1, "monitored": false}}}}, episodes: []Episode{
		{ID: 17, Season: 1, Number: 7, HasFile: true}, {ID: 18, Season: 1, Number: 8, HasFile: true},
		{ID: 21, Season: 2, Number: 1, AirDateUTC: "2020-01-01T00:00:00Z"}, {ID: 22, Season: 2, Number: 2, AirDateUTC: "2999-01-01T00:00:00Z"},
	}}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "sonarr-secret" {
			t.Error("missing Sonarr authentication")
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /sonarr/api/v3/system/status":
			json.NewEncoder(w).Encode(map[string]any{"version": "4"})
		case "GET /sonarr/api/v3/series":
			json.NewEncoder(w).Encode(f.series)
		case "GET /sonarr/api/v3/tag":
			if f.failTags {
				w.WriteHeader(500)
				return
			}
			json.NewEncoder(w).Encode([]map[string]any{{"id": 3, "label": "prefetch"}})
		case "GET /sonarr/api/v3/episode":
			if r.URL.Query().Get("seriesId") != "12" {
				t.Error("wrong series ID")
			}
			json.NewEncoder(w).Encode(f.episodes)
		case "PUT /sonarr/api/v3/series/12":
			var v map[string]any
			json.NewDecoder(r.Body).Decode(&v)
			if v["qualityProfileId"] != float64(7) || v["monitored"] != true || v["seasons"].([]any)[0].(map[string]any)["monitored"] != false {
				t.Errorf("series settings changed: %+v", v)
			}
			f.seriesWrites++
			w.Write([]byte(`{}`))
		case "PUT /sonarr/api/v3/episode/monitor":
			var v struct {
				EpisodeIDs []int `json:"episodeIds"`
				Monitored  bool  `json:"monitored"`
			}
			json.NewDecoder(r.Body).Decode(&v)
			if !v.Monitored {
				t.Error("must monitor")
			}
			f.monitor = append(f.monitor, v.EpisodeIDs)
			w.Write([]byte(`[]`))
		case "GET /sonarr/api/v3/queue":
			json.NewEncoder(w).Encode(map[string]any{"records": f.queued})
		case "POST /sonarr/api/v3/command":
			if f.failCommand {
				w.WriteHeader(503)
				return
			}
			var v struct {
				Name string `json:"name"`
				IDs  []int  `json:"episodeIds"`
			}
			json.NewDecoder(r.Body).Decode(&v)
			if v.Name != "EpisodeSearch" {
				t.Errorf("unexpected search %s", v.Name)
			}
			f.commands = append(f.commands, v.IDs)
			w.WriteHeader(201)
			w.Write([]byte(`{"id":1}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}
func (f *fakeSonarr) client() Sonarr {
	return Sonarr{Name: "Sonarr", URL: f.server.URL + "/sonarr/", APIKey: "sonarr-secret", Client: NewHTTPClient()}
}

func TestPrefetchMonitorsMissingAndSearchesOnlyAired(t *testing.T) {
	f := newSonarr(t)
	recent := map[string]time.Time{}
	cfg := Config{EpisodesAhead: 3, IncludeTags: "prefetch"}
	result, err := f.client().Prefetch(context.Background(), playing(), cfg, time.Now(), recent)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Monitored, []int{21, 22}) || !reflect.DeepEqual(result.Searched, []int{21}) {
		t.Fatalf("result: %+v", result)
	}
	if f.seriesWrites != 1 || len(f.commands) != 1 || !reflect.DeepEqual(f.monitor, [][]int{{21, 22}}) {
		t.Fatalf("unexpected mutations %+v", f)
	}
	if _, err = f.client().Prefetch(context.Background(), playing(), cfg, time.Now(), recent); err != nil {
		t.Fatal(err)
	}
	if len(f.commands) != 1 {
		t.Fatal("duplicate search within cooldown")
	}
}

func TestTagFilters(t *testing.T) {
	for _, tc := range []struct {
		name, include, exclude string
		fail                   bool
	}{
		{"include miss", "unknown", "", false}, {"exclude wins", "prefetch", "PREFETCH", false}, {"tag API fails", "prefetch", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newSonarr(t)
			f.failTags = tc.fail
			_, err := f.client().Prefetch(context.Background(), playing(), Config{EpisodesAhead: 3, IncludeTags: tc.include, ExcludeTags: tc.exclude}, time.Now(), map[string]time.Time{})
			if (err != nil) != tc.fail {
				t.Fatalf("unexpected error %v", err)
			}
			if len(f.monitor) > 0 || f.seriesWrites > 0 || len(f.commands) > 0 {
				t.Fatal("filtered series was modified")
			}
		})
	}
}
func TestQueuedAndAvailableEpisodesAreNotSearched(t *testing.T) {
	f := newSonarr(t)
	f.queued = []map[string]any{{"episodeId": 21}}
	_, err := f.client().Prefetch(context.Background(), playing(), Config{EpisodesAhead: 2}, time.Now(), map[string]time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.commands) != 0 {
		t.Fatal("searched queued episode")
	}
	f = newSonarr(t)
	_, err = f.client().Prefetch(context.Background(), playing(), Config{EpisodesAhead: 1}, time.Now(), map[string]time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.monitor) > 0 || f.seriesWrites > 0 {
		t.Fatal("counted missing episodes instead of the episode window")
	}
}
func TestFailedSearchCanRetry(t *testing.T) {
	f := newSonarr(t)
	f.failCommand = true
	recent := map[string]time.Time{}
	_, err := f.client().Prefetch(context.Background(), playing(), Config{EpisodesAhead: 2}, time.Now(), recent)
	if err == nil || len(recent) != 0 {
		t.Fatal("failure recorded as successful")
	}
	f.failCommand = false
	_, err = f.client().Prefetch(context.Background(), playing(), Config{EpisodesAhead: 2}, time.Now(), recent)
	if err != nil || len(f.commands) != 1 {
		t.Fatalf("retry failed: %v", err)
	}
}
func TestSeriesMatching(t *testing.T) {
	for _, name := range []string{"missing", "ambiguous", "alternate"} {
		t.Run(name, func(t *testing.T) {
			f := newSonarr(t)
			switch name {
			case "missing":
				f.series = nil
			case "ambiguous":
				f.series = append(f.series, f.series[0])
			case "alternate":
				f.series[0]["title"] = "Canonical"
				f.series[0]["alternateTitles"] = []map[string]any{{"title": "Example"}}
			}
			_, err := f.client().Prefetch(context.Background(), playing(), Config{EpisodesAhead: 2}, time.Now(), map[string]time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			if (len(f.commands) > 0) != (name == "alternate") {
				t.Fatalf("unexpected matching for %s", name)
			}
		})
	}
}
func TestUserFilters(t *testing.T) {
	for _, tc := range []struct {
		include, exclude string
		want             bool
	}{
		{"", "", true}, {"Viewer", "", true}, {"10", "", true}, {"profile-1", "", true}, {"someone", "", false}, {"Viewer", "10", false}, {"", "viewer", false},
	} {
		if got := (Config{IncludeUsers: tc.include, ExcludeUsers: tc.exclude}).permitsUser(playing()); got != tc.want {
			t.Errorf("%+v got %v", tc, got)
		}
	}
}
func TestHTTPRedactsCredentialsAndRejectsRedirect(t *testing.T) {
	var leaked bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked = true }))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, 302) }))
	defer origin.Close()
	err := request(context.Background(), NewHTTPClient(), origin.URL, "secret", "Sonarr", "GET", "/", nil, nil)
	if err == nil || leaked || strings.Contains(err.Error(), origin.URL) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("redirect or redaction failure: %v", err)
	}
}
