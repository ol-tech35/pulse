// Package cli contains Pulse's command-line interface.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"example.com/ol-tech35/pulse/internal/checker"
)

// Run writes results to stdout, diagnostics to stderr and returns a process exit code.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("pulse", flag.ContinueOnError)
	fs.SetOutput(stderr)
	timeout := fs.Duration("timeout", 5*time.Second, "timeout per URL, including redirects")
	workers := fs.Int("workers", 4, "maximum concurrent requests")
	jsonOutput := fs.Bool("json", false, "print results as JSON")
	fs.Usage = func() { fmt.Fprintln(fs.Output(), "Usage: pulse [flags] URL [URL ...]"); fs.PrintDefaults() }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.SetOutput(stdout)
			fs.Usage()
			return 0
		}
		return 2
	}
	results, err := checker.Check(ctx, fs.Args(), *timeout, *workers)
	if err != nil {
		fmt.Fprintln(stderr, "pulse:", err)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 130
		}
		return 2
	}
	code := 0
	for _, r := range results {
		if !r.OK {
			code = 1
		}
	}
	if *jsonOutput {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		err = enc.Encode(results)
	} else {
		err = writeTable(stdout, results)
	}
	if err != nil {
		fmt.Fprintln(stderr, "pulse: write output:", err)
		return 1
	}
	return code
}

func writeTable(out io.Writer, results []checker.Result) error {
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "URL\tSTATUS\tTIME (ms)\tRESULT"); err != nil {
		return err
	}
	for _, r := range results {
		status := "-"
		if r.StatusCode != 0 {
			status = fmt.Sprint(r.StatusCode)
		}
		outcome := "OK"
		if !r.OK {
			outcome = r.Error
		}
		// Keep each check on one row even when a server returns unusual error text.
		clean := func(s string) string {
			return strings.Map(func(r rune) rune {
				if r < 32 || r == 127 {
					return ' '
				}
				return r
			}, s)
		}
		if _, err := fmt.Fprintf(w, "%s\t%s\t%.2f\t%s\n", clean(r.URL), status, r.DurationMS, clean(outcome)); err != nil {
			return err
		}
	}
	return w.Flush()
}
