package prefetch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Processor is serialized by its RPC owner. Each instance records success
// independently, so a failed second Sonarr never re-searches the first one.
type Processor struct {
	Client    *http.Client
	Now       func() time.Time
	processed map[string]time.Time
	recent    map[string]time.Time
}

func NewProcessor(client *http.Client) *Processor {
	if client == nil {
		client = NewHTTPClient()
	}
	return &Processor{Client: client, Now: time.Now, processed: map[string]time.Time{}, recent: map[string]time.Time{}}
}

func instances(c Config, client *http.Client) []Sonarr {
	out := []Sonarr{{Name: "Sonarr", URL: c.SonarrURL, APIKey: c.SonarrAPIKey, Client: client}}
	if c.Sonarr4KURL != "" {
		out = append(out, Sonarr{Name: "Sonarr 4K", URL: c.Sonarr4KURL, APIKey: c.Sonarr4KAPIKey, Client: client})
	}
	return out
}

func (p *Processor) Process(ctx context.Context, c Config, session Session) ([]Result, error) {
	if !c.Enabled || !c.permitsUser(session) || session.IsPaused ||
		(!strings.EqualFold(session.MediaType, "series") && !strings.EqualFold(session.MediaType, "episode")) ||
		session.SeriesName == "" || session.SeasonNumber == nil || session.EpisodeNumber == nil || *session.SeasonNumber <= 0 || *session.EpisodeNumber <= 0 {
		return nil, nil
	}
	// A below-threshold session must remain eligible for a later progress event.
	// Do this before Sonarr requests and before recording successful work.
	if !session.reachedProgress(c.MinWatchedPercent) {
		return nil, nil
	}
	now := p.Now()
	for k, v := range p.processed {
		if now.Sub(v) >= 15*time.Minute {
			delete(p.processed, k)
		}
	}
	for k, v := range p.recent {
		if now.Sub(v) >= 15*time.Minute {
			delete(p.recent, k)
		}
	}
	var results []Result
	var failures []error
	for _, s := range instances(c, p.Client) {
		key := fmt.Sprintf("%s|%s|%s|%d|%d", s.Name, session.SessionID, session.SeriesName, *session.SeasonNumber, *session.EpisodeNumber)
		if _, ok := p.processed[key]; ok {
			continue
		}
		r, err := s.Prefetch(ctx, session, c, now, p.recent)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		results = append(results, r)
		if len(p.processed) < 10000 {
			p.processed[key] = now
		}
	}
	return results, errors.Join(failures...)
}
