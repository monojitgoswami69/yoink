// Package healthcheck produces compose-style healthcheck definitions per
// framework. The strategy is intentionally minimal: a connect-and-respond
// probe via curl that ignores HTTP status, so any response (including 4xx)
// counts as "alive". This avoids false negatives on apps that don't expose a
// dedicated /health endpoint.
package healthcheck

import "fmt"

// Spec mirrors the compose-style healthcheck structure used elsewhere
// (matches infra.Healthcheck in shape).
type Spec struct {
	Test     []string // CMD or CMD-SHELL form
	Interval string
	Timeout  string
	Retries  int
	// StartPeriod gives slow-starting services (Django, Next builds) a grace
	// window before the first failure counts.
	StartPeriod string
}

// ForApp returns a probe appropriate for an application service rendered by
// Yoink's Dockerfile templates. Static-served SPAs always hit nginx on port
// 80 inside the container; other frameworks accept the supplied port.
func ForApp(framework string, port int) Spec {
	switch framework {
	case "react", "vite", "cra", "astro":
		return httpProbe(80, "20s")
	case "next", "nest":
		return httpProbe(port, "30s")
	case "django":
		return httpProbe(port, "30s")
	case "fastapi", "flask":
		return httpProbe(port, "15s")
	case "express", "fastify", "node":
		return httpProbe(port, "15s")
	case "go", "rust":
		return httpProbe(port, "20s")
	}
	return httpProbe(port, "20s")
}

// httpProbe builds a curl-based readiness probe. `curl -sS` (no -f) treats
// any HTTP response — including 4xx and 5xx — as success, so we only fail on
// connection refusal or timeout. That is the closest "is the process
// listening?" check that survives apps without a /health route.
func httpProbe(port int, startPeriod string) Spec {
	return Spec{
		Test: []string{
			"CMD-SHELL",
			fmt.Sprintf("curl -sS -o /dev/null --max-time 3 http://localhost:%d/ || exit 1", port),
		},
		Interval:    "10s",
		Timeout:     "5s",
		Retries:     5,
		StartPeriod: startPeriod,
	}
}

// RuntimeNeedsCurl reports whether the runtime image for the given framework
// requires curl to be installed in the Dockerfile in order for the
// healthcheck to work. All current probes go through curl, so this is true
// for every framework Yoink generates Dockerfiles for.
func RuntimeNeedsCurl(framework string) bool { return true }

// InstallCurl returns the package-manager command line that installs curl in
// the runtime image used for the given framework. Returns an empty string
// when no installation step is needed.
func InstallCurl(framework string) string {
	switch framework {
	case "fastapi", "flask", "django", "python":
		// python:3.12-slim is debian-based.
		return "RUN apt-get update && apt-get install -y --no-install-recommends curl && rm -rf /var/lib/apt/lists/*"
	case "go":
		// alpine:latest uses apk.
		return "RUN apk add --no-cache curl"
	case "rust":
		// debian:bookworm-slim is debian-based.
		return "RUN apt-get update && apt-get install -y --no-install-recommends curl && rm -rf /var/lib/apt/lists/*"
	}
	// node:20-alpine and nginx:alpine both use apk.
	return "RUN apk add --no-cache curl"
}
