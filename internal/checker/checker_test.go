package checker

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestStatusesAndOrder(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/slow":
			time.Sleep(30 * time.Millisecond)
			w.WriteHeader(204)
		case "/bad":
			w.WriteHeader(503)
		case "/redirect":
			http.Redirect(w, r, "/ok", 302)
		default:
			w.WriteHeader(200)
		}
	}))
	defer s.Close()
	urls := []string{s.URL + "/slow", s.URL + "/bad", s.URL + "/redirect"}
	got, err := Check(context.Background(), urls, time.Second, 3)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []int{204, 503, 200} {
		if got[i].URL != urls[i] || got[i].StatusCode != want || got[i].OK != (want < 400) || got[i].DurationMS < 0 {
			t.Fatalf("result %d: %+v", i, got[i])
		}
	}
	if got[1].Error == "" {
		t.Fatal("HTTP failure lacks explanation")
	}
}

func TestValidationBeforeRequests(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer s.Close()
	for _, bad := range []string{"", "example.com", "ftp://example.com", "http:///missing", "https://u:p@example.com", "http://host:bad"} {
		if _, err := Check(context.Background(), []string{s.URL, bad}, time.Second, 2); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	for _, tc := range []struct {
		urls    []string
		timeout time.Duration
		workers int
	}{{nil, time.Second, 1}, {[]string{s.URL}, 0, 1}, {[]string{s.URL}, time.Second, 0}} {
		if _, err := Check(context.Background(), tc.urls, tc.timeout, tc.workers); err == nil {
			t.Error("invalid options accepted")
		}
	}
	if calls.Load() != 0 {
		t.Fatal("validation allowed partial requests")
	}
}

func TestTimeoutAndCancellation(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer s.Close()
	got, err := Check(context.Background(), []string{s.URL}, 20*time.Millisecond, 1)
	if err != nil || got[0].OK || got[0].StatusCode != 0 || got[0].Error == "" {
		t.Fatalf("timeout: %+v %v", got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Check(ctx, []string{s.URL}, time.Second, 1); err != context.Canceled {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestConcurrencyLimit(t *testing.T) {
	var active, peak atomic.Int32
	entered := make(chan struct{}, 8)
	release := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old; old = peak.Load() {
			if peak.CompareAndSwap(old, n) {
				break
			}
		}
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer s.Close()
	urls := make([]string, 8)
	for i := range urls {
		urls[i] = fmt.Sprintf("%s/%d", s.URL, i)
	}
	done := make(chan error, 1)
	go func() { _, err := Check(context.Background(), urls, time.Second, 2); done <- err }()
	for range 2 {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			close(release)
			t.Fatal("workers did not start")
		}
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if peak.Load() != 2 {
		t.Fatalf("peak=%d, want 2", peak.Load())
	}
}

func TestCancellationDuringRequest(t *testing.T) {
	entered := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done() }))
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := Check(ctx, []string{s.URL}, time.Minute, 1); done <- err }()
	<-entered
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation hung")
	}
}
