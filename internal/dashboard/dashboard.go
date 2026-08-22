// Package dashboard provides the Bubbletea TUI for `yoink dash`. It shows a
// live view of a running compose stack: per-service status with bound URLs,
// scrollable log tail, editable env overrides, per-service controls
// (start/stop/restart/rebuild), and a docker-stats footer that ticks every
// couple of seconds.
package dashboard

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"yoink/internal/docker"
	"yoink/internal/state"
	"yoink/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Run starts the dashboard against the given saved stack. It blocks until
// the user quits.
func Run(ctx context.Context, mgr *state.Manager, lock *state.Lock) error {
	composePath := filepath.Join(lock.RepoPath, lock.OutputSubdir, "docker-compose.yml")
	cm := docker.New(composePath, lock.RepoPath, "yoink-"+lock.Repo)

	m := &model{
		ctx:    ctx,
		mgr:    mgr,
		lock:   lock,
		cm:     cm,
		logs:   map[string][]string{},
		tab:    tabLogs,
		ticker: 2 * time.Second,
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

const (
	tabLogs     = 0
	tabEnv      = 1
	tabControls = 2

	maxLogLines = 500
)

type model struct {
	ctx  context.Context
	mgr  *state.Manager
	lock *state.Lock
	cm   *docker.Compose

	containers []docker.Container
	stats      map[string]docker.Stat // keyed by Service
	logs       map[string][]string

	width, height int
	selected      int
	tab           int
	logScroll     int

	pending string // action in flight, e.g. "restart service-1"
	flash   string
	flashAt time.Time
	ticker  time.Duration
	err     error
}

// --- Bubbletea wiring ---

func (m *model) Init() tea.Cmd {
	return tea.Batch(
		m.pollPs(),
		m.pollStats(),
		m.tick(),
	)
}

type tickMsg time.Time
type psMsg struct {
	containers []docker.Container
	err        error
}
type statsMsg struct {
	stats []docker.Stat
	err   error
}
type logsMsg struct {
	service string
	lines   []string
	err     error
}
type actionDoneMsg struct {
	label string
	err   error
}
type envSavedMsg struct {
	service string
	err     error
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tickMsg:
		// Refresh ps + stats every tick. Logs for the selected service follow.
		return m, tea.Batch(m.pollPs(), m.pollStats(), m.tick())

	case psMsg:
		if msg.err == nil {
			m.containers = msg.containers
			m.normaliseSelection()
		} else if !isMissingStack(msg.err) {
			m.err = msg.err
		}
		// Pull fresh logs for the selected service.
		if svc := m.currentService(); svc != "" {
			return m, m.pollLogs(svc)
		}
		return m, nil

	case statsMsg:
		if msg.err == nil {
			m.stats = indexStats(msg.stats)
		}
		return m, nil

	case logsMsg:
		if msg.err == nil {
			m.logs[msg.service] = msg.lines
		}
		return m, nil

	case actionDoneMsg:
		m.pending = ""
		if msg.err != nil {
			m.setFlash("✗ " + msg.label + ": " + msg.err.Error())
		} else {
			m.setFlash("✓ " + msg.label)
		}
		return m, m.pollPs()

	case envSavedMsg:
		if msg.err != nil {
			m.setFlash("✗ env save: " + msg.err.Error())
		} else {
			m.setFlash("✓ env saved for " + msg.service + " (run `yoink up` to apply)")
		}
		return m, nil
	}
	return m, nil
}

func (m *model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c", "esc":
		return m, tea.Quit
	case "up", "k":
		if m.selected > 0 {
			m.selected--
			m.logScroll = 0
		}
	case "down", "j":
		if m.selected < len(m.containers)-1 {
			m.selected++
			m.logScroll = 0
		}
	case "tab", "right", "l":
		m.tab = (m.tab + 1) % 3
	case "shift+tab", "left", "h":
		m.tab = (m.tab + 2) % 3
	case "pgup", "K":
		m.logScroll += 10
	case "pgdown", "J":
		m.logScroll -= 10
		if m.logScroll < 0 {
			m.logScroll = 0
		}
	case "g":
		m.logScroll = 9999
	case "G":
		m.logScroll = 0
	case "r":
		return m, m.runAction("restart", "restart", func(ctx context.Context, svc string) error {
			_, err := m.cm.Restart(ctx, svc)
			return err
		})
	case "s":
		return m, m.runAction("start", "up -d", func(ctx context.Context, svc string) error {
			// First try `start` (works on already-created containers); fall
			// back to a no-deps `up` if it isn't created yet.
			if _, err := m.cm.Start(ctx, svc); err == nil {
				return nil
			}
			_, err := m.cm.Up(ctx, "--no-deps", svc)
			return err
		})
	case "x":
		return m, m.runAction("stop", "stop", func(ctx context.Context, svc string) error {
			_, err := m.cm.Stop(ctx, svc)
			return err
		})
	case "b":
		return m, m.runAction("rebuild", "up --build", func(ctx context.Context, svc string) error {
			_, err := m.cm.Recreate(ctx, svc)
			return err
		})
	case "e":
		return m, m.editEnv()
	}
	return m, nil
}

// --- Polling commands ---

func (m *model) tick() tea.Cmd {
	return tea.Tick(m.ticker, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *model) pollPs() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
		defer cancel()
		cs, err := m.cm.Ps(ctx)
		return psMsg{containers: cs, err: err}
	}
}

func (m *model) pollStats() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
		defer cancel()
		s, err := m.cm.Stats(ctx)
		return statsMsg{stats: s, err: err}
	}
}

func (m *model) pollLogs(service string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
		defer cancel()
		out, err := m.cm.Logs(ctx, service, maxLogLines)
		if err != nil {
			return logsMsg{service: service, err: err}
		}
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		// Compose logs prefix each line with "service | ". Strip it for clarity.
		clean := make([]string, 0, len(lines))
		for _, l := range lines {
			if idx := strings.Index(l, " | "); idx > 0 && idx < 60 {
				clean = append(clean, l[idx+3:])
			} else {
				clean = append(clean, l)
			}
		}
		return logsMsg{service: service, lines: clean}
	}
}

func (m *model) runAction(label, _ string, fn func(ctx context.Context, service string) error) tea.Cmd {
	svc := m.currentService()
	if svc == "" {
		return func() tea.Msg { return actionDoneMsg{label: label, err: fmt.Errorf("no service selected")} }
	}
	m.pending = label + " " + svc
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
		defer cancel()
		err := fn(ctx, svc)
		return actionDoneMsg{label: label + " " + svc, err: err}
	}
}

// editEnv launches $EDITOR (or vi) on a temp .env file for the selected
// service. When the editor exits we re-read the file, diff against the
// generated .env.example to extract per-key overrides, and persist them to
// env-overrides.json. The dashboard surrenders the terminal during editing.
func (m *model) editEnv() tea.Cmd {
	svc := m.currentService()
	if svc == "" {
		return func() tea.Msg { return envSavedMsg{err: fmt.Errorf("no service selected")} }
	}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	envDir := filepath.Join(m.lock.RepoPath, m.lock.OutputSubdir, "env-vars", svc)
	dotenv := filepath.Join(envDir, ".env")
	example := filepath.Join(envDir, ".env.example")

	// Make sure .env exists (seeded from .env.example when missing).
	if _, err := os.Stat(dotenv); os.IsNotExist(err) {
		if data, readErr := os.ReadFile(example); readErr == nil {
			_ = os.WriteFile(dotenv, data, 0600)
		}
	}

	cmd := exec.Command(editor, dotenv)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return envSavedMsg{service: svc, err: err}
		}
		// Diff .env against .env.example to extract overrides.
		exampleData, _ := os.ReadFile(example)
		envData, _ := os.ReadFile(dotenv)
		overrides := diffEnv(string(exampleData), string(envData))
		if len(overrides) == 0 {
			return envSavedMsg{service: svc}
		}
		current, _ := m.mgr.LoadOverrides()
		if current[svc] == nil {
			current[svc] = map[string]string{}
		}
		for k, v := range overrides {
			current[svc][k] = v
		}
		if err := m.mgr.SaveOverrides(current); err != nil {
			return envSavedMsg{service: svc, err: err}
		}
		return envSavedMsg{service: svc}
	})
}

// --- View ---

func (m *model) View() string {
	if m.width == 0 || m.height == 0 {
		return "  " + ui.MutedStyle.Render(ui.SymWork+" Connecting to Docker…")
	}

	narrow := m.width < 100

	header := m.renderHeader(narrow)
	footer := m.renderFooter()
	footerH := lipgloss.Height(footer)
	headerH := lipgloss.Height(header)
	statsBlock := m.renderStats(narrow)
	statsH := lipgloss.Height(statsBlock)
	serviceBlock := m.renderServices(narrow)
	servicesH := lipgloss.Height(serviceBlock)

	bodyH := m.height - headerH - footerH - statsH - servicesH - 3
	if bodyH < 5 {
		bodyH = 5
	}
	detail := m.renderDetail(bodyH, narrow)
	return strings.Join([]string{
		header,
		serviceBlock,
		detail,
		statsBlock,
		footer,
	}, "\n")
}

// renderHeader: a single calm line — "YOINK" muted, project bold, overall
// badge right-aligned. No coloured background bar (§19).
func (m *model) renderHeader(narrow bool) string {
	overall := m.overallState()
	title := ui.MutedStyle.Render("YOINK") + "  " + ui.BoldStyle.Render(projectName(m.lock))
	badge := ui.OverallStatus(overall)
	gap := m.width - lipgloss.Width(title) - lipgloss.Width(badge)
	if gap < 1 {
		gap = 1
	}
	line := title + strings.Repeat(" ", gap) + badge
	flash := m.flashLine()
	if flash != "" {
		line += "  " + flash
	}
	return line
}

// overallState derives a project-level status from the current containers.
func (m *model) overallState() string {
	if len(m.containers) == 0 {
		return "stopped"
	}
	worst := 0
	for _, c := range m.containers {
		if c.State != "running" {
			return "failed"
		}
		switch c.Health {
		case "healthy":
			r := 0
			if r > worst {
				worst = r
			}
		case "", "starting":
			if 1 > worst {
				worst = 1
			}
		case "unhealthy":
			return "unhealthy"
		}
	}
	switch worst {
	case 0:
		return "running"
	case 1:
		return "starting"
	}
	return "running"
}

func (m *model) flashLine() string {
	if m.flash == "" || time.Since(m.flashAt) > 4*time.Second {
		return ""
	}
	if strings.HasPrefix(m.flash, "✗") {
		return ui.ErrorStyle.Render(m.flash)
	}
	return ui.SuccessStyle.Render(m.flash)
}

// renderServices: aligned list — dot, name, health, :port — with thin rules
// instead of a heavy border (§19, §20).
func (m *model) renderServices(narrow bool) string {
	var b strings.Builder
	b.WriteString(ui.Section("Services") + "\n")
	if len(m.containers) == 0 {
		b.WriteString("  " + ui.MutedStyle.Render("No containers running."))
		b.WriteString("\n  " + ui.MutedStyle.Render("Run: yoink up "+projectName(m.lock)))
		return b.String()
	}
	showURL := !narrow
	for i, c := range m.containers {
		dot, label := ui.ServiceStatus(c.State, c.Health)
		cur := " "
		name := c.Service
		if i == m.selected {
			cur = ui.PrimaryStyle.Render(ui.SymSel)
			name = ui.BoldStyle.Render(c.Service)
		}
		row := fmt.Sprintf("  %s %s %s  %s", cur, dot, name, label)
		if showURL {
			if port, ok := m.lock.PortMap[c.Service]; ok {
				row += "  " + ui.HighlightStyle.Render(fmt.Sprintf(":%d", port))
			}
		}
		b.WriteString(row + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderDetail: tabs (Logs/Env/Controls) + the active pane, with a thin rule.
func (m *model) renderDetail(maxLines int, narrow bool) string {
	var b strings.Builder
	tabs := []string{"Logs", "Env", "Controls"}
	for i, name := range tabs {
		style := ui.MutedStyle
		if i == m.tab {
			style = ui.PrimaryStyle.Bold(true)
		}
		if i > 0 {
			b.WriteString(ui.DimStyle.Render("  " + ui.SymSep + "  "))
		}
		b.WriteString(style.Render(name))
	}
	b.WriteString("\n" + ui.DimStyle.Render(strings.Repeat(ui.SymBar, m.width-4)) + "\n")

	svc := m.currentService()
	body := maxLines - 3
	if body < 2 {
		body = 2
	}
	switch m.tab {
	case tabLogs:
		b.WriteString(m.renderLogPane(svc, body))
	case tabEnv:
		b.WriteString(m.renderEnvPane(svc, body))
	case tabControls:
		b.WriteString(m.renderControlsPane(svc, body))
	}
	return b.String()
}

// renderLogPane with a polished empty state (§22).
func (m *model) renderLogPane(svc string, maxLines int) string {
	if svc == "" {
		return "  " + ui.MutedStyle.Render("Select a service to view logs.")
	}
	lines := m.logs[svc]
	if len(lines) == 0 {
		return "  " + ui.MutedStyle.Render("No logs yet.") + "\n  " + ui.MutedStyle.Render("The service is running but hasn't emitted output.")
	}
	end := len(lines) - m.logScroll
	if end < 1 {
		end = 1
	}
	start := end - maxLines
	if start < 0 {
		start = 0
	}
	if end > len(lines) {
		end = len(lines)
	}
	var b strings.Builder
	for _, l := range lines[start:end] {
		b.WriteString("  " + l + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderEnvPane with an "incomplete" empty state (§22).
func (m *model) renderEnvPane(svc string, maxLines int) string {
	if svc == "" {
		return "  " + ui.MutedStyle.Render("Select a service to view environment.")
	}
	envPath := filepath.Join(m.lock.RepoPath, m.lock.OutputSubdir, "env-vars", svc, ".env")
	data, err := os.ReadFile(envPath)
	if err != nil {
		data, _ = os.ReadFile(strings.TrimSuffix(envPath, ".env") + ".env.example")
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	missing := 0
	var b strings.Builder
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "#") {
			b.WriteString("  " + ui.DimStyle.Render(l) + "\n")
			continue
		}
		if _, v, ok := strings.Cut(t, "="); ok && strings.TrimSpace(v) == "" {
			missing++
		}
		b.WriteString("  " + l + "\n")
	}
	if missing > 0 {
		b.WriteString("\n  " + ui.WarningStyle.Render(fmt.Sprintf("Environment incomplete. %d variable(s) missing.", missing)))
		b.WriteString("\n  " + ui.MutedStyle.Render("Press e to configure."))
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderControlsPane: a clean detail card for the selected service.
func (m *model) renderControlsPane(svc string, _ int) string {
	if svc == "" {
		return "  " + ui.MutedStyle.Render("Select a service.")
	}
	var b strings.Builder
	b.WriteString("  " + ui.BoldStyle.Render(svc) + "\n\n")
	binds := []ui.KeyBind{
		{Key: "s", Label: "start"}, {Key: "x", Label: "stop"}, {Key: "r", Label: "restart"}, {Key: "b", Label: "rebuild"}, {Key: "e", Label: "edit env"},
	}
	b.WriteString("  " + ui.Footer(binds))
	return b.String()
}

// renderStats: a single-line resource strip (§19). Collapses columns when
// the terminal is narrow.
func (m *model) renderStats(narrow bool) string {
	if len(m.stats) == 0 || len(m.containers) == 0 {
		return ""
	}
	var totCPU float64
	var totMem, totNet string
	for _, c := range m.containers {
		s, ok := m.stats[c.Name]
		if !ok {
			continue
		}
		totCPU += s.CPUPct
		totMem = s.MemUsage
		totNet = s.NetIO
	}
	_ = totMem
	parts := []string{
		ui.MutedStyle.Render("CPU") + " " + fmt.Sprintf("%.1f%%", totCPU),
	}
	if !narrow {
		parts = append(parts,
			ui.MutedStyle.Render("MEM")+" "+truncateMem(totMem),
			ui.MutedStyle.Render("NET")+" "+totNet,
		)
	}
	return ui.DimStyle.Render(strings.Repeat(ui.SymBar, m.width-4)) + "\n" + strings.Join(parts, "    ")
}

// renderFooter: a subtle, contextual single line (§24).
func (m *model) renderFooter() string {
	var binds []ui.KeyBind
	if len(m.containers) == 0 {
		binds = []ui.KeyBind{{Key: "↑↓", Label: "select"}, {Key: "q", Label: "quit"}}
	} else {
		binds = []ui.KeyBind{
			{Key: "↑↓", Label: "select"}, {Key: "tab", Label: "pane"}, {Key: "r", Label: "restart"}, {Key: "x", Label: "stop"}, {Key: "b", Label: "rebuild"}, {Key: "e", Label: "env"}, {Key: "q", Label: "quit"},
		}
	}
	return ui.Footer(binds)
}

// --- helpers ---

func projectName(lock *state.Lock) string {
	if lock.Project != "" {
		return lock.Project
	}
	return lock.Repo
}

func truncateMem(s string) string {
	// "84 MiB / 1 GiB" → keep the used portion only.
	if i := strings.Index(s, " / "); i > 0 {
		return s[:i]
	}
	return s
}

func (m *model) currentService() string {
	if m.selected < 0 || m.selected >= len(m.containers) {
		return ""
	}
	return m.containers[m.selected].Service
}

func (m *model) normaliseSelection() {
	if m.selected >= len(m.containers) {
		m.selected = len(m.containers) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
}

func (m *model) setFlash(msg string) {
	m.flash, m.flashAt = msg, time.Now()
}

func indexStats(stats []docker.Stat) map[string]docker.Stat {
	out := map[string]docker.Stat{}
	for _, s := range stats {
		out[s.Name] = s
	}
	return out
}

// diffEnv compares the template against the user-edited content and returns
// every key whose value differs (or whose key wasn't in the template).
func diffEnv(template, edited string) map[string]string {
	base := parseEnv(template)
	now := parseEnv(edited)
	out := map[string]string{}
	for k, v := range now {
		if base[k] != v {
			out[k] = v
		}
	}
	return out
}

func parseEnv(s string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out
}

func isMissingStack(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no configuration file") ||
		strings.Contains(msg, "no such file") ||
		strings.Contains(msg, "no services found")
}

// SortedServices returns the saved app + infra service names in the order
// they should appear in the dashboard's service list. Exported for tests.
func SortedServices(lock *state.Lock) []string {
	out := make([]string, 0, len(lock.Services)+len(lock.Infra))
	for _, s := range lock.Services {
		out = append(out, s.ID)
	}
	out = append(out, lock.Infra...)
	sort.Strings(out)
	return out
}
