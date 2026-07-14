// Package checker performs bounded, one-shot HTTP availability checks.
package checker

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Result describes a request up to receipt of response headers.
type Result struct {
	URL        string  `json:"url"`
	StatusCode int     `json:"status_code"`
	DurationMS float64 `json:"duration_ms"`
	OK         bool    `json:"ok"`
	Error      string  `json:"error,omitempty"`
}

// Check validates all inputs before making requests and returns results in input order.
// Individual request failures are stored in Result; cancellation aborts the batch.
func Check(ctx context.Context, urls []string, timeout time.Duration, workers int) ([]Result, error) {
	if len(urls) == 0 {
		return nil, fmt.Errorf("provide at least one URL")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("timeout must be positive")
	}
	if workers <= 0 {
		return nil, fmt.Errorf("workers must be positive")
	}
	for i, raw := range urls {
		u, err := url.Parse(raw)
		if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
			return nil, fmt.Errorf("URL %d must be an absolute http(s) URL without credentials", i+1)
		}
		if _, err := http.NewRequest(http.MethodGet, raw, nil); err != nil {
			return nil, fmt.Errorf("invalid URL %d: %w", i+1, err)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	workers = min(workers, len(urls))
	transport := http.DefaultTransport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: timeout, Transport: transport}
	results := make([]Result, len(urls))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for i := range jobs {
				results[i] = checkOne(ctx, client, urls[i])
			}
		})
	}
dispatch:
	for i := range urls {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break dispatch
		}
	}
	close(jobs)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func checkOne(ctx context.Context, client *http.Client, raw string) Result {
	result := Result{URL: raw}
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err == nil {
		req.Header.Set("User-Agent", "pulse/0.1")
		var resp *http.Response
		resp, err = client.Do(req)
		if resp != nil {
			result.StatusCode = resp.StatusCode
			resp.Body.Close()
		}
	}
	result.DurationMS = float64(time.Since(start)) / float64(time.Millisecond)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.OK = result.StatusCode >= 200 && result.StatusCode < 400
	if !result.OK {
		result.Error = fmt.Sprintf("HTTP %d %s", result.StatusCode, http.StatusText(result.StatusCode))
	}
	return result
}
