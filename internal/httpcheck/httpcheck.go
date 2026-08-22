// Package httpcheck performs HTTP-level runtime verification of services
// that publish a host port. It distinguishes a real application response
// from connection failures and timeouts, and does NOT require HTTP 200:
// 2xx/3xx/401/403/404 are all "healthy" (the app answered); only 5xx is an
// application error. This keeps Yoink from declaring success when a
// container is marked healthy but the app itself is broken.
package httpcheck

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"yoink/internal/detector"
)

// Status is the outcome of an HTTP probe.
type Status string

const (
	// Healthy: the app responded with a non-5xx status (200, 201, 301, 302,
	// 401, 403, 404, …). The service is reachable and answering.
	Healthy Status = "healthy"
	// AppError: the app responded with 5xx — it is running but broken.
	AppError Status = "application_error"
	// Unreachable: connection refused / no listener on the port.
	Unreachable Status = "unreachable"
	// Timeout: the probe exceeded its deadline.
	Timeout Status = "timeout"
)

// Result is one service's HTTP probe outcome.
type Result struct {
	Service string
	URL     string
	Status  Status
	Code    int
	Err     string
}

// Healthy reports whether the probe got a non-5xx response.
func (r Result) Healthy() bool { return r.Status == Healthy }

// Check performs a GET against url with the given timeout. It tolerates any
// non-5xx status as "the app answered" (per the Yoink verification policy).
func Check(ctx context.Context, url string, timeout time.Duration) Result {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, url, nil)
	if err != nil {
		return Result{URL: url, Status: Unreachable, Err: err.Error()}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if cctx.Err() == context.DeadlineExceeded {
			return Result{URL: url, Status: Timeout, Err: err.Error()}
		}
		return Result{URL: url, Status: Unreachable, Err: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()
	code := resp.StatusCode
	if code >= 500 {
		return Result{URL: url, Status: AppError, Code: code}
	}
	return Result{URL: url, Status: Healthy, Code: code}
}

// Services probes every application service that publishes a host port
// (infra services like postgres/redis have no entry in portMap and are
// skipped). It hits "/" on each published port with a short timeout.
func Services(ctx context.Context, services []detector.Service, portMap map[string]int) []Result {
	var out []Result
	for _, s := range services {
		hp := portMap[s.ID]
		if hp == 0 {
			continue // unexposed / infra
		}
		url := fmt.Sprintf("http://localhost:%d/", hp)
		r := Check(ctx, url, 5*time.Second)
		r.Service = s.ID
		out = append(out, r)
	}
	return out
}

// AllHealthy reports whether every probe got a healthy response. An empty
// slice (no HTTP services) is considered healthy — there was nothing to
// refute the container-health check.
func AllHealthy(results []Result) bool {
	for _, r := range results {
		if !r.Healthy() {
			return false
		}
	}
	return true
}
