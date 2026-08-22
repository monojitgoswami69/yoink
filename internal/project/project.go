// Package project is the shared, project-centric service layer that every
// Yoink command operates through. It owns three concerns that previously
// leaked into individual commands:
//
//  1. Resolution — turn a project name (or "use the most recent one") into a
//     fully-wired *Project (state manager + lock + compose handle).
//  2. Health — one authoritative mapping from `docker compose ps` onto a
//     stopped/starting/healthy/unhealthy/failed model.
//  3. URLs — derive the user-facing http://localhost:<port> list from the
//     generated port map, so up/status/open agree.
//
// Commands must NOT independently re-implement repo lookup, compose wiring,
// or Docker ps parsing. They call this package.
package project

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"yoink/internal/docker"
	"yoink/internal/state"
)

// Project bundles everything a command needs for an initialised stack.
type Project struct {
	Name      string          // user-facing project name
	Manager   *state.Manager  // ~/.yoink/state/<name>/ handle
	Lock      *state.Lock     // authoritative record
	Compose   *docker.Compose // bound to <repo>/<output>/docker-compose.yml
	OutputDir string          // <repo>/<outputSubdir>
}

// Resolve returns the named project, or the most recently initialised one
// when name is "". It never creates state directories as a side effect, and
// returns an actionable error (with the list of known projects) when the
// name doesn't resolve.
func Resolve(name string) (*Project, error) {
	mgrs, err := state.AllManagers()
	if err != nil {
		return nil, err
	}
	if len(mgrs) == 0 {
		return nil, ErrNoProjects
	}
	var mgr *state.Manager
	if name == "" {
		mgr = mgrs[0] // AllManagers returns most-recent-init first
	} else {
		for _, m := range mgrs {
			if m.Repo == name {
				mgr = m
				break
			}
		}
		if mgr == nil {
			return nil, &UnknownProjectError{Name: name, Available: managerNames(mgrs)}
		}
	}
	lock, err := mgr.LoadLock()
	if err != nil {
		return nil, err
	}
	if lock == nil {
		return nil, &MissingLockError{Project: mgr.Repo}
	}
	return fromManager(mgr, lock), nil
}

// All returns every initialised project (most-recent first), skipping state
// directories that have no yoink.lock. The list command uses this.
func All() ([]*Project, error) {
	mgrs, err := state.AllManagers()
	if err != nil {
		return nil, err
	}
	out := make([]*Project, 0, len(mgrs))
	for _, m := range mgrs {
		lock, err := m.LoadLock()
		if err != nil || lock == nil {
			continue
		}
		out = append(out, fromManager(m, lock))
	}
	return out, nil
}

// Names returns the known project names (most-recent first).
func Names() ([]string, error) {
	mgrs, err := state.AllManagers()
	if err != nil {
		return nil, err
	}
	return managerNames(mgrs), nil
}

func fromManager(mgr *state.Manager, lock *state.Lock) *Project {
	name := lock.Project
	if name == "" {
		name = lock.Repo // backward compat with pre-Project-field locks
	}
	outputDir := filepath.Join(lock.RepoPath, lock.OutputSubdir)
	composePath := filepath.Join(outputDir, "docker-compose.yml")
	return &Project{
		Name:      name,
		Manager:   mgr,
		Lock:      lock,
		Compose:   docker.New(composePath, lock.RepoPath, "yoink-"+name),
		OutputDir: outputDir,
	}
}

func managerNames(mgrs []*state.Manager) []string {
	out := make([]string, 0, len(mgrs))
	for _, m := range mgrs {
		out = append(out, m.Repo)
	}
	return out
}

// ServiceURL is one public-facing service URL. Infra services (postgres,
// redis, …) have no entry because they aren't published for browser access.
type ServiceURL struct {
	Service string
	URL     string
}

// URLs returns the public service URLs derived from the generated port map
// and the currently-running containers. up, status, and open share this so
// the URL shown is always the one compose actually bound.
func (p *Project) URLs(ctx context.Context) ([]ServiceURL, error) {
	ps, err := p.Compose.Ps(ctx)
	if err != nil {
		// When the daemon is down we can still report the configured URLs.
		return configuredURLs(p), nil
	}
	var out []ServiceURL
	seen := map[string]bool{}
	for _, c := range ps {
		port, ok := p.Lock.PortMap[c.Service]
		if !ok {
			continue
		}
		if seen[c.Service] {
			continue
		}
		seen[c.Service] = true
		out = append(out, ServiceURL{Service: c.Service, URL: fmt.Sprintf("http://localhost:%d", port)})
	}
	if len(out) == 0 {
		out = configuredURLs(p)
	}
	return out, nil
}

// configuredURLs falls back to the port map recorded at init time, so `open`
// and `status` work even before `up` binds the ports.
func configuredURLs(p *Project) []ServiceURL {
	var out []ServiceURL
	// Preserve lock order (services slice) for stable output.
	for _, svc := range p.Lock.Services {
		if port, ok := p.Lock.PortMap[svc.ID]; ok {
			out = append(out, ServiceURL{Service: svc.ID, URL: fmt.Sprintf("http://localhost:%d", port)})
		}
	}
	return out
}

// ConfiguredURLs is the exported fallback used by `open` (and any command
// that wants the URL without a running daemon). It returns the public URLs
// recorded in the lock, in service order.
func ConfiguredURLs(p *Project) []ServiceURL {
	return configuredURLs(p)
}

// ServiceHealth is one row of the health view.
type ServiceHealth struct {
	Service string
	State   string // running | exited | restarting | …
	Health  string // healthy | unhealthy | starting | "" (no healthcheck)
	Status  string // docker's "Up 12 seconds (healthy)" verbatim
	URL     string // public URL, or "" for infra
	Public  bool
}

// Health is the aggregate view a status command renders.
type Health struct {
	Overall  string
	Services []ServiceHealth
	// Started is time since the project's last `up`; zero when unknown.
	Started time.Duration
}

const (
	overallStopped   = "stopped"
	overallStarting  = "starting"
	overallHealthy   = "running"
	overallUnhealthy = "unhealthy"
	overallFailed    = "failed"
)

// Health returns per-service and overall health. A project with no running
// containers is "stopped"; otherwise the overall status is the worst
// per-service status (healthy < starting < unhealthy < failed).
func (p *Project) Health(ctx context.Context) (*Health, error) {
	ps, err := p.Compose.Ps(ctx)
	if err != nil {
		return nil, err
	}
	h := &Health{}
	if len(ps) == 0 {
		h.Overall = overallStopped
		return h, nil
	}
	worst := 0 // healthy
	for _, c := range ps {
		sh := ServiceHealth{
			Service: c.Service,
			State:   c.State,
			Health:  c.Health,
			Status:  c.Status,
		}
		if port, ok := p.Lock.PortMap[c.Service]; ok {
			sh.URL = fmt.Sprintf("http://localhost:%d", port)
			sh.Public = true
		}
		h.Services = append(h.Services, sh)
		if r := rank(sh); r > worst {
			worst = r
		}
	}
	switch worst {
	case 0:
		h.Overall = overallHealthy
	case 1:
		h.Overall = overallStarting
	case 2:
		h.Overall = overallUnhealthy
	default:
		h.Overall = overallFailed
	}
	if !p.Lock.LastUp.IsZero() {
		h.Started = time.Since(p.Lock.LastUp)
	}
	return h, nil
}

// IsRunning reports whether the project has at least one running container.
func (p *Project) IsRunning(ctx context.Context) (bool, error) {
	ps, err := p.Compose.Ps(ctx)
	if err != nil {
		return false, err
	}
	return len(ps) > 0, nil
}

func rank(s ServiceHealth) int {
	if s.State != "running" {
		return 3 // failed
	}
	switch s.Health {
	case "healthy":
		return 0
	case "", "starting":
		return 1
	case "unhealthy":
		return 2
	}
	return 1
}
