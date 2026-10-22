package prefetch

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

func Sessions(ctx context.Context, client *http.Client, cfg Config) ([]Session, error) {
	var sessions []Session
	cursor := ""
	seen := map[string]bool{}
	for page := 0; page < 100; page++ {
		query := url.Values{"limit": {"200"}}
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		var response struct {
			Items *[]Session `json:"items"`
			Page  struct {
				NextCursor string `json:"next_cursor"`
			} `json:"page"`
		}
		if err := request(ctx, client, cfg.SiloURL, cfg.SiloAPIKey, "Silo", "GET", "/api/v2/admin/sessions?"+query.Encode(), nil, &response); err != nil {
			return nil, err
		}
		if response.Items == nil {
			return nil, fmt.Errorf("Silo sessions response is missing its items array; check the native API URL and server version")
		}
		sessions = append(sessions, (*response.Items)...)
		cursor = response.Page.NextCursor
		if cursor == "" {
			return sessions, nil
		}
		if seen[cursor] {
			return nil, fmt.Errorf("Silo returned a repeated session page cursor")
		}
		seen[cursor] = true
	}
	return nil, fmt.Errorf("Silo exceeded the 100-page session limit")
}
