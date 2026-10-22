package prefetch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

type Sonarr struct {
	Name, URL, APIKey string
	Client            *http.Client
}
type series struct {
	ID              int    `json:"id"`
	Title           string `json:"title"`
	AlternateTitles []struct {
		Title string `json:"title"`
	} `json:"alternateTitles"`
	Tags []int `json:"tags"`
}
type Episode struct {
	ID         int    `json:"id"`
	Season     int    `json:"seasonNumber"`
	Number     int    `json:"episodeNumber"`
	HasFile    bool   `json:"hasFile"`
	AirDateUTC string `json:"airDateUtc"`
}
type Result struct {
	Instance  string `json:"instance"`
	Series    string `json:"series"`
	Reason    string `json:"reason"`
	Monitored []int  `json:"monitored_episode_ids,omitempty"`
	Searched  []int  `json:"searched_episode_ids,omitempty"`
}

func (s Sonarr) call(ctx context.Context, method, path string, body, out any) error {
	return request(ctx, s.Client, s.URL, s.APIKey, s.Name, method, "/api/v3/"+path, body, out)
}

func (s Sonarr) Check(ctx context.Context) error {
	return s.call(ctx, "GET", "system/status", nil, &map[string]any{})
}

func (s Sonarr) Prefetch(ctx context.Context, session Session, cfg Config, now time.Time, recent map[string]time.Time) (Result, error) {
	r := Result{Instance: s.Name, Series: session.SeriesName}
	var all []json.RawMessage
	if err := s.call(ctx, "GET", "series", nil, &all); err != nil {
		return r, err
	}
	var found *series
	var resource map[string]any
	for _, raw := range all {
		var v series
		if err := json.Unmarshal(raw, &v); err != nil {
			return r, fmt.Errorf("%s returned invalid series", s.Name)
		}
		match := strings.EqualFold(strings.TrimSpace(v.Title), strings.TrimSpace(session.SeriesName))
		for _, a := range v.AlternateTitles {
			match = match || strings.EqualFold(strings.TrimSpace(a.Title), strings.TrimSpace(session.SeriesName))
		}
		if !match {
			continue
		}
		if found != nil {
			r.Reason = "ambiguous series title; skipped"
			return r, nil
		}
		found = &v
		if err := json.Unmarshal(raw, &resource); err != nil {
			return r, fmt.Errorf("decode series resource")
		}
	}
	if found == nil {
		r.Reason = "series not found"
		return r, nil
	}
	if len(list(cfg.IncludeTags))+len(list(cfg.ExcludeTags)) > 0 {
		var tags []struct {
			ID    int    `json:"id"`
			Label string `json:"label"`
		}
		if err := s.call(ctx, "GET", "tag", nil, &tags); err != nil {
			return r, err
		}
		var labels []string
		for _, t := range tags {
			for _, id := range found.Tags {
				if id == t.ID {
					labels = append(labels, t.Label)
				}
			}
		}
		if matches(list(cfg.ExcludeTags), labels...) || (len(list(cfg.IncludeTags)) > 0 && !matches(list(cfg.IncludeTags), labels...)) {
			r.Reason = "excluded by tags"
			return r, nil
		}
	}
	var episodes []Episode
	if err := s.call(ctx, "GET", fmt.Sprintf("episode?seriesId=%d", found.ID), nil, &episodes); err != nil {
		return r, err
	}
	window := Window(episodes, *session.SeasonNumber, *session.EpisodeNumber, cfg.EpisodesAhead)
	var missing []Episode
	for _, e := range window {
		if !e.HasFile {
			missing = append(missing, e)
		}
	}
	if len(missing) == 0 {
		r.Reason = "episode window already available or series ended"
		return r, nil
	}
	// Monitoring the series enables RSS for these episode flags. Preserve every
	// other series field and do not toggle whole seasons (which monitors extras).
	if monitored, _ := resource["monitored"].(bool); !monitored {
		resource["monitored"] = true
		if err := s.call(ctx, "PUT", fmt.Sprintf("series/%d", found.ID), resource, nil); err != nil {
			return r, err
		}
	}
	for _, e := range missing {
		r.Monitored = append(r.Monitored, e.ID)
	}
	if err := s.call(ctx, "PUT", "episode/monitor", map[string]any{"episodeIds": r.Monitored, "monitored": true}, nil); err != nil {
		return r, err
	}
	var queue struct {
		Records []struct {
			EpisodeID int `json:"episodeId"`
		} `json:"records"`
	}
	if err := s.call(ctx, "GET", "queue?page=1&pageSize=10000&includeUnknownSeriesItems=false", nil, &queue); err != nil {
		return r, err
	}
	queued := map[int]bool{}
	for _, q := range queue.Records {
		queued[q.EpisodeID] = true
	}
	for _, e := range missing {
		air, err := time.Parse(time.RFC3339, e.AirDateUTC)
		if err != nil || air.After(now) || queued[e.ID] || now.Sub(recent[fmt.Sprintf("%s:%d", s.Name, e.ID)]) < 15*time.Minute {
			continue
		}
		r.Searched = append(r.Searched, e.ID)
	}
	if len(r.Searched) == 0 {
		r.Reason = "missing episodes monitored; unaired, queued, or recently searched"
		return r, nil
	}
	if err := s.call(ctx, "POST", "command", map[string]any{"name": "EpisodeSearch", "episodeIds": r.Searched}, nil); err != nil {
		return r, err
	}
	for _, id := range r.Searched {
		recent[fmt.Sprintf("%s:%d", s.Name, id)] = now
	}
	r.Reason = "missing episodes monitored and search submitted"
	return r, nil
}

// Window counts the next N episode positions, including ones already on disk.
// Specials are excluded; ordering continues across season boundaries.
func Window(episodes []Episode, season, number, count int) []Episode {
	var out []Episode
	for _, e := range episodes {
		if e.ID > 0 && e.Season > 0 && e.Number > 0 && (e.Season > season || e.Season == season && e.Number > number) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Season != out[j].Season {
			return out[i].Season < out[j].Season
		}
		return out[i].Number < out[j].Number
	})
	if len(out) > count {
		out = out[:count]
	}
	return out
}
