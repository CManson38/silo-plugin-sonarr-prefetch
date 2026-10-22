package prefetch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func NewHTTPClient() *http.Client {
	return &http.Client{Timeout: 4 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// Errors deliberately exclude URLs, credentials, and upstream response bodies.
func request(ctx context.Context, client *http.Client, base, key, service, method, path string, body, out any) error {
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode %s request", service)
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(base, "/")+path, bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("invalid %s request", service)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if service == "Silo" {
		req.Header.Set("Authorization", "Bearer "+key)
	} else {
		req.Header.Set("X-Api-Key", key)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s request failed (connection, timeout, or TLS); check URL and reachability", service)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s returned HTTP %d", service, resp.StatusCode)
	}
	const maxBody = 16 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil || len(data) > maxBody {
		return fmt.Errorf("cannot read %s response within 16 MiB limit", service)
	}
	if out != nil && json.Unmarshal(data, out) != nil {
		return fmt.Errorf("%s returned invalid JSON", service)
	}
	return nil
}
