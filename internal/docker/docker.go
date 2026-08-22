// Package docker wraps the host `docker` and `docker compose` binaries with
// a typed API. The wrapper is deliberately minimal — it forwards arguments,
// streams output, and decodes the JSON responses that compose / stats / ps
// emit when asked. It is the single place in Yoink that runs an external
// container command.
package docker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// Compose binds to a single docker-compose.yml on disk.
type Compose struct {
	// File is the absolute or working-dir-relative path to docker-compose.yml.
	File string
	// Dir is the working directory the docker command will be run from. The
	// compose file's relative paths (context: ..) are resolved from here. For
	// Yoink-generated stacks this is typically the cloned repo root.
	Dir string
	// Project is the docker-compose project name. Defaults to the directory
	// name when empty.
	Project string
	// bin is the discovered docker binary (cached).
	bin string
	// v1 is true when the runtime is docker-compose v1 (separate binary).
	v1 bool
}

// New returns a Compose handle bound to the given compose file. It does not
// touch the filesystem until a method is called.
func New(file, dir, project string) *Compose {
	return &Compose{File: file, Dir: dir, Project: project}
}

// Available reports whether docker (or docker-compose v1) is reachable in
// PATH. Use this before offering compose-dependent UX.
func Available() bool {
	if _, err := exec.LookPath("docker"); err == nil {
		return true
	}
	if _, err := exec.LookPath("docker-compose"); err == nil {
		return true
	}
	return false
}

// DaemonRunning reports whether the docker daemon is actually reachable,
// not merely installed. `Available()` returns true with Docker Desktop
// installed but not started; this distinguishes the two cases so commands
// can give an actionable "start Docker" hint instead of a bare exit code.
func DaemonRunning(ctx context.Context) bool {
	if p, err := exec.LookPath("docker"); err == nil {
		cmd := exec.CommandContext(ctx, p, "info", "--format", "{{.ServerVersion}}")
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if err := cmd.Run(); err == nil {
			return true
		}
	}
	return false
}

func (c *Compose) detect() error {
	if c.bin != "" {
		return nil
	}
	if p, err := exec.LookPath("docker"); err == nil {
		c.bin, c.v1 = p, false
		return nil
	}
	if p, err := exec.LookPath("docker-compose"); err == nil {
		c.bin, c.v1 = p, true
		return nil
	}
	return errors.New("docker is not installed: install Docker Desktop or the docker engine + compose v2")
}

// composeArgs returns the leading argv that selects the compose subcommand
// plus the -f / -p flags shared by every invocation.
func (c *Compose) composeArgs() []string {
	if c.v1 {
		args := []string{"-f", c.File}
		if c.Project != "" {
			args = append(args, "-p", c.Project)
		}
		return args
	}
	args := []string{"compose", "-f", c.File}
	if c.Project != "" {
		args = append(args, "-p", c.Project)
	}
	return args
}

// runCapture runs `<docker> compose -f ... <args>` and returns the combined
// stdout/stderr. The exit code is propagated as part of the returned error.
func (c *Compose) runCapture(ctx context.Context, args ...string) (string, error) {
	if err := c.detect(); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, c.bin, append(c.composeArgs(), args...)...)
	cmd.Dir = c.Dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// Build runs `docker compose build`. Returns the combined output regardless
// of success — heal logic consumes this when the build fails.
func (c *Compose) Build(ctx context.Context) (string, error) {
	return c.runCapture(ctx, "build")
}

// BuildService runs `docker compose build <name>`. Used to rebuild only the
// service that the heal loop just modified.
func (c *Compose) BuildService(ctx context.Context, name string) (string, error) {
	return c.runCapture(ctx, "build", name)
}

// Up starts the stack detached. Pass extra flags via extra (e.g.
// "--build", "--remove-orphans").
func (c *Compose) Up(ctx context.Context, extra ...string) (string, error) {
	args := append([]string{"up", "-d"}, extra...)
	return c.runCapture(ctx, args...)
}

// Down brings the stack down. Pass volumes=true to also remove named volumes.
func (c *Compose) Down(ctx context.Context, volumes bool) (string, error) {
	args := []string{"down"}
	if volumes {
		args = append(args, "-v")
	}
	return c.runCapture(ctx, args...)
}

// Restart restarts a single service.
func (c *Compose) Restart(ctx context.Context, name string) (string, error) {
	return c.runCapture(ctx, "restart", name)
}

// Stop stops a single service (without removing it).
func (c *Compose) Stop(ctx context.Context, name string) (string, error) {
	return c.runCapture(ctx, "stop", name)
}

// Start starts a single previously-created service.
func (c *Compose) Start(ctx context.Context, name string) (string, error) {
	return c.runCapture(ctx, "start", name)
}

// Recreate stops, removes, and re-starts a single service while keeping the
// rest of the stack running. Use this after editing env files.
func (c *Compose) Recreate(ctx context.Context, name string) (string, error) {
	return c.runCapture(ctx, "up", "-d", "--force-recreate", "--no-deps", name)
}

// Container is one row of `docker compose ps --format json`.
type Container struct {
	ID       string `json:"ID"`
	Name     string `json:"Name"`
	Service  string `json:"Service"`
	State    string `json:"State"`  // running | exited | restarting | ...
	Status   string `json:"Status"` // "Up 12 seconds (healthy)" etc.
	Health   string `json:"Health"` // healthy | unhealthy | starting | "" (no healthcheck)
	Image    string `json:"Image"`
	Project  string `json:"Project"`
	ExitCode int    `json:"ExitCode"`
}

// Ps returns one Container per compose service that is part of the project.
// Services not yet started are omitted.
func (c *Compose) Ps(ctx context.Context) ([]Container, error) {
	out, err := c.runCapture(ctx, "ps", "--format", "json", "--all")
	if err != nil {
		return nil, fmt.Errorf("docker compose ps: %w (%s)", err, truncate(out, 400))
	}
	return parsePsOutput(out)
}

// parsePsOutput tolerates both formats compose has emitted:
//   - newer: one JSON object per line (NDJSON)
//   - older: a single JSON array
func parsePsOutput(s string) ([]Container, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if s[0] == '[' {
		var arr []Container
		if err := json.Unmarshal([]byte(s), &arr); err != nil {
			return nil, fmt.Errorf("parse compose ps array: %w", err)
		}
		return arr, nil
	}
	var out []Container
	sc := bufio.NewScanner(strings.NewReader(s))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] != '{' {
			continue
		}
		var c Container
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			continue
		}
		out = append(out, c)
	}
	return out, sc.Err()
}

// Logs returns the last `tail` lines of logs for one service (or the whole
// stack when service is ""). Use Follow for a live tail.
func (c *Compose) Logs(ctx context.Context, service string, tail int) (string, error) {
	args := []string{"logs", "--no-color", "--tail", strconv.Itoa(tail)}
	if service != "" {
		args = append(args, service)
	}
	out, err := c.runCapture(ctx, args...)
	return out, err
}

// Follow streams `docker compose logs -f` to stdout/stderr until ctx is
// cancelled. When service is "" every service is followed. This is the
// backend for `yoink logs <project> --follow`.
func (c *Compose) Follow(ctx context.Context, service string, tail int) error {
	if err := c.detect(); err != nil {
		return err
	}
	args := append(c.composeArgs(), "logs", "-f", "--no-color", "--tail", strconv.Itoa(tail))
	if service != "" {
		args = append(args, service)
	}
	cmd := exec.CommandContext(ctx, c.bin, args...)
	cmd.Dir = c.Dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Block until the context cancels (ctrl-C / ctx done) or the process
	// exits on its own.
	return cmd.Run()
}

// StreamLogs starts `docker compose logs -f <service>` and returns the
// stdout reader along with a stop function. The caller MUST call stop when
// done to terminate the underlying process.
func (c *Compose) StreamLogs(ctx context.Context, service string) (io.ReadCloser, func(), error) {
	if err := c.detect(); err != nil {
		return nil, nil, err
	}
	args := append(c.composeArgs(), "logs", "-f", "--no-color", "--tail", "200", service)
	cmd := exec.CommandContext(ctx, c.bin, args...)
	cmd.Dir = c.Dir
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	cmd.Stderr = cmd.Stdout // route stderr into the same stream
	if err := cmd.Start(); err != nil {
		_ = pipe.Close()
		return nil, nil, err
	}
	stop := func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = pipe.Close()
		_, _ = cmd.Process.Wait()
	}
	return pipe, stop, nil
}

// Stat is one row of `docker stats --no-stream --format json`. CPU and
// memory percentages are normalised to numeric values.
type Stat struct {
	Container string  `json:"Container"`
	Name      string  `json:"Name"`
	CPUPct    float64 `json:"-"`
	MemPct    float64 `json:"-"`
	MemUsage  string  `json:"MemUsage"`
	NetIO     string  `json:"NetIO"`
	BlockIO   string  `json:"BlockIO"`
	PIDs      string  `json:"PIDs"`
	// Raw fields preserved for callers that want them.
	CPUPercRaw string `json:"CPUPerc"`
	MemPercRaw string `json:"MemPerc"`
}

// Stats runs `docker stats --no-stream` and returns one Stat per running
// container. The wrapper does not filter by project — callers can do that
// themselves against the names returned by Ps.
func (c *Compose) Stats(ctx context.Context) ([]Stat, error) {
	if err := c.detect(); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, c.bin, "stats", "--no-stream", "--format", "{{json .}}")
	cmd.Dir = c.Dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker stats: %w", err)
	}
	out := []Stat{}
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var s Stat
		if err := json.Unmarshal([]byte(line), &s); err != nil {
			continue
		}
		s.CPUPct = parsePct(s.CPUPercRaw)
		s.MemPct = parsePct(s.MemPercRaw)
		out = append(out, s)
	}
	return out, nil
}

func parsePct(s string) float64 {
	s = strings.TrimSpace(strings.TrimSuffix(s, "%"))
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...[truncated]"
}

// ExtractFailedService scans a `docker compose build` output blob and
// returns the first service whose build failed. Compose v2 + BuildKit
// labels steps like "[service-2 builder 5/6]" and references
// "Dockerfile.service-2" in the build definition load step. Returns "" when
// no service can be identified.
func ExtractFailedService(output string) string {
	lines := strings.Split(output, "\n")
	// Phase 1: walk from the bottom up looking for explicit service names.
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		low := strings.ToLower(line)
		// "Service 'web' failed to build : ..." — extract the quoted name.
		if strings.Contains(low, "service '") && strings.Contains(low, "failed") {
			start := strings.Index(line, "'") + 1
			end := strings.Index(line[start:], "'")
			if end > 0 {
				return line[start : start+end]
			}
		}
		// "ERROR: failed to build api" — last token after the verb.
		if idx := strings.Index(low, "failed to build "); idx >= 0 {
			tail := strings.TrimSpace(line[idx+len("failed to build "):])
			if tail != "" {
				if sp := strings.IndexAny(tail, " :,."); sp > 0 {
					tail = tail[:sp]
				}
				return strings.Trim(tail, ":,. ")
			}
		}
	}
	// Phase 2: parse BuildKit step labels "[service-id stage N/M]" from
	// the bottom up. The failing step's label is near the ERROR line.
	stepLabelRe := regexp.MustCompile(`\[([a-z0-9][a-z0-9-]*)\s+\w+\s+\d+/\d+\]`)
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(strings.ToLower(lines[i]), "error") || strings.Contains(strings.ToLower(lines[i]), "failed") {
			// Walk upward from the error line to find the step label.
			for j := i; j >= 0 && j > i-20; j-- {
				if m := stepLabelRe.FindStringSubmatch(lines[j]); len(m) == 2 {
					return m[1]
				}
			}
		}
	}
	// Phase 3: parse "Dockerfile.<service-id>" from the build definition.
	dfRe := regexp.MustCompile(`Dockerfile\.([a-z0-9][a-z0-9-]*)`)
	for i := len(lines) - 1; i >= 0; i-- {
		if m := dfRe.FindStringSubmatch(lines[i]); len(m) == 2 {
			return m[1]
		}
	}
	// Phase 4: fall back to a "svc | ERROR" line prefix.
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if idx := strings.Index(line, " | "); idx > 0 && idx < 80 {
			low := strings.ToLower(line[idx:])
			if strings.Contains(low, "error") || strings.Contains(low, "fail") {
				return strings.TrimSpace(line[:idx])
			}
		}
	}
	return ""
}

// TailLines returns the last n lines of s as a single string with a trailing
// newline. Lines past the limit are dropped. Useful for capping the
// build-log payload sent to the LLM.
func TailLines(s string, n int) string {
	if n <= 0 {
		return ""
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		if strings.HasSuffix(s, "\n") {
			return s
		}
		return s + "\n"
	}
	return strings.Join(lines[len(lines)-n:], "\n") + "\n"
}
