# Pulse

A small Go CLI for checking HTTP endpoints in parallel. Useful for a quick
check after deployment or before starting a local development session.

Pulse makes one GET request per URL, measures time until response headers,
and prints a table or JSON. No external dependencies.

## Quick start

Requires Go 1.26 or newer.

```sh
go build -o bin/pulse ./cmd/pulse
./bin/pulse https://example.com https://example.org
./bin/pulse -timeout 3s -workers 2 -json https://example.com
```

Example table (timings vary):

```text
URL                  STATUS  TIME (ms)  RESULT
https://example.com  200     142.38     OK
```

JSON contains an ordered array of results:

```json
[
  {
    "url": "https://example.com",
    "status_code": 200,
    "duration_ms": 142.38,
    "ok": true
  }
]
```

Failed checks also include an `error` field. Network failures use status `0`.
All flags must appear before URLs; run `pulse -h` for help.

| Flag | Default | Meaning |
| --- | --- | --- |
| `-timeout` | `5s` | Per-URL timeout, including redirects |
| `-workers` | `4` | Maximum concurrent requests |
| `-json` | `false` | Machine-readable output |

| Exit code | Meaning |
| --- | --- |
| `0` | All checks succeeded, or help requested |
| `1` | At least one check failed, or output could not be written |
| `2` | Invalid arguments |
| `130` | Batch interrupted or context cancelled |

HTTP 200–399 is successful. Redirects follow Go's default policy (at most
10 requests in a redirect chain). Duplicate URLs are checked separately.
Results keep input order even when requests finish out of order.
Ctrl+C cancels active requests; an interrupted batch does not print partial results.

## Development

```sh
make test   # tests with the race detector
make vet    # static analysis
make build  # bin/pulse
```

Tests use local HTTP servers and need no internet access. They cover status
handling, redirects, timeouts, cancellation, bounded concurrency, validation,
output formats and CLI exit codes.

## Design

- `internal/checker`: validates a batch, feeds indexed jobs to a worker pool,
  performs requests and collects results.
- `internal/cli`: parses flags, formats output and decides exit codes.
- `cmd/pulse`: connects OS signals to a cancellation context.

Workers write to separate result slots; a wait group ensures all writes finish
before the caller reads the batch. The HTTP transport is shared within a batch
and its idle connections are closed afterwards.

## Scope and limitations

This is a one-shot availability check, not an uptime monitoring service.
There are no retries, persistent history or scheduled jobs. Response bodies
are closed without being read: the measured duration covers response headers,
not downloading or validating content. Closing unread bodies can prevent
connection reuse. Any 2xx/3xx response counts as success, including 204.

TLS certificate validation stays enabled. Go's transport honors proxy
environment variables. Check only endpoints you intend to contact; redirects
can contact other hosts. URLs with embedded credentials are rejected, but query
parameters appear in output, so avoid passing tokens in URLs.
