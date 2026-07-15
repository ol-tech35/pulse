package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOutputAndExitCodes(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad" {
			w.WriteHeader(500)
		}
	}))
	defer s.Close()
	for _, tc := range []struct {
		args []string
		code int
		json bool
	}{{[]string{"-json", s.URL}, 0, true}, {[]string{s.URL}, 0, false}, {[]string{"-json", s.URL + "/bad"}, 1, true}} {
		var out, stderr bytes.Buffer
		code := Run(context.Background(), tc.args, &out, &stderr)
		if code != tc.code {
			t.Fatalf("code=%d stderr=%s", code, &stderr)
		}
		if tc.json {
			var rows []struct {
				URL    string `json:"url"`
				OK     bool   `json:"ok"`
				Status int    `json:"status_code"`
			}
			if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 || rows[0].URL != tc.args[len(tc.args)-1] || rows[0].OK != (tc.code == 0) {
				t.Fatalf("rows=%+v", rows)
			}
		} else if !strings.Contains(out.String(), "200") || !strings.Contains(out.String(), s.URL) {
			t.Fatalf("table=%s", &out)
		}
	}
}

func TestInvalidArgumentsAndHelp(t *testing.T) {
	for _, args := range [][]string{nil, {"-workers", "0", "https://example.com"}, {"-timeout", "0s", "https://example.com"}, {"-timeout", "bad"}, {"-unknown"}, {"example.com"}, {"https://user:secret@example.com"}} {
		var out, stderr bytes.Buffer
		if code := Run(context.Background(), args, &out, &stderr); code != 2 {
			t.Fatalf("args=%v code=%d", args, code)
		}
		if out.Len() != 0 || stderr.Len() == 0 {
			t.Fatal("argument errors must go to stderr")
		}
	}
	var out, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"-h"}, &out, &stderr); code != 0 {
		t.Fatalf("help code=%d", code)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Fatal("missing help")
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }

func TestWriteFailure(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer s.Close()
	for _, args := range [][]string{{s.URL}, {"-json", s.URL}} {
		var stderr bytes.Buffer
		if code := Run(context.Background(), args, brokenWriter{}, &stderr); code != 1 || !strings.Contains(stderr.String(), "output unavailable") {
			t.Fatalf("code=%d stderr=%s", code, &stderr)
		}
	}
}

func TestCancelledRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if code := Run(ctx, []string{"http://127.0.0.1:1"}, io.Discard, io.Discard); code != 130 {
		t.Fatalf("code=%d", code)
	}
}

func TestHelpOutputErrors(t *testing.T) {
	var stderr bytes.Buffer
	if code := Run(context.Background(), []string{"-h"}, brokenWriter{}, &stderr); code != 1 {
		t.Fatalf("help write failure code=%d", code)
	}
	var out bytes.Buffer
	stderr.Reset()
	if code := Run(context.Background(), []string{"-h"}, &out, &stderr); code != 0 || stderr.Len() != 0 {
		t.Fatalf("help code=%d stderr=%s", code, &stderr)
	}
}
