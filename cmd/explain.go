package cmd

import (
	"fmt"
	"sort"
	"strings"

	"yoink/internal/detector"
	"yoink/internal/graph"
	"yoink/internal/infra"
	"yoink/internal/project"
	"yoink/internal/state"
	"yoink/internal/ui"

	"github.com/spf13/cobra"
)

var explainCmd = &cobra.Command{
	Use:   "explain [project]",
	Short: "Summarise what Yoink detected, generated, and repaired",
	Long: `Summarise the evidence already recorded for a project: detected
frameworks, inferred infrastructure (local vs external), the service
graph, agent repairs, and any configuration still required.

No LLM is invoked and no information is invented — this command only
reports what Yoink already determined during init.`,
	Args:          cobra.MaximumNArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	Run: func(cmd *cobra.Command, args []string) {
		name := ""
		if len(args) > 0 {
			name = args[0]
		}
		if err := runExplain(name); err != nil {
			fmt.Println()
			fmt.Println(ui.ErrorBox.Render(err.Error()))
		}
	},
}

func runExplain(name string) error {
	p, err := project.Resolve(name)
	if err != nil {
		return err
	}
	lock := p.Lock

	// Rebuild the service graph from the persisted compact projection so
	// explain can render dependencies without re-running detection.
	infras := make([]infra.Service, 0, len(lock.InfraDetails))
	for _, d := range lock.InfraDetails {
		infras = append(infras, infra.Service{
			Name: d.Name, Kind: infra.Kind(d.Kind), Mode: d.Mode,
			Provider: d.Provider, Port: d.Port, Reason: d.Reason,
		})
	}
	links := make(map[string][]infra.AppLink, len(lock.Links))
	for appID, refs := range lock.Links {
		ls := make([]infra.AppLink, 0, len(refs))
		for _, r := range refs {
			ls = append(ls, infra.AppLink{ServiceName: r.To})
		}
		links[appID] = ls
	}
	g := graph.Build(lock.Services, infras, links, lock.PortMap)

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", ui.BoldStyle.Render("YOINK / "+p.Name))

	// Detected.
	b.WriteString(ui.Section("Detected") + "\n")
	fws, pms := uniqueFrameworks(lock.Services), uniquePMs(lock.Services)
	if len(fws) > 0 {
		fmt.Fprintf(&b, "  Frameworks    %s\n", ui.HighlightStyle.Render(strings.Join(fws, ", ")))
	}
	if len(pms) > 0 {
		fmt.Fprintf(&b, "  Package mgr   %s\n", ui.HighlightStyle.Render(strings.Join(pms, ", ")))
	}
	if ports := appPorts(lock.Services); ports != "" {
		fmt.Fprintf(&b, "  Port          %s\n", ui.HighlightStyle.Render(ports))
	}
	b.WriteString("\n")

	// Infrastructure.
	if len(infras) > 0 {
		b.WriteString(ui.Section("Infrastructure") + "\n")
		for _, n := range infraNodes(g) {
			fmt.Fprintf(&b, "  %-12s %s\n", n.Label, ui.MutedStyle.Render(n.Detail))
		}
		b.WriteString("\n")
	}

	// Services.
	if len(lock.Services) > 0 {
		b.WriteString(ui.Section("Services") + "\n")
		for _, s := range lock.Services {
			fw := s.Framework
			if fw == "" {
				fw = s.Language
			}
			fmt.Fprintf(&b, "  %-12s %s\n", s.ID, ui.HighlightStyle.Render(fw))
		}
		b.WriteString("\n")
	}

	// Dependencies (service graph edges).
	if deps := dependencyRows(g); len(deps) > 0 {
		b.WriteString(ui.Section("Dependencies") + "\n")
		for _, r := range deps {
			fmt.Fprintf(&b, "  %s\n", r)
		}
		b.WriteString("\n")
	}

	// Agent repairs.
	if reps := repairRows(p.Manager); len(reps) > 0 {
		b.WriteString(ui.Section("Agent repairs") + "\n")
		for _, r := range reps {
			fmt.Fprintf(&b, "  %s %s\n", ui.SuccessStyle.Render(ui.SymDone), r)
		}
		b.WriteString("\n")
	}

	// Configuration required.
	if reqs := requiredEnvRows(lock.Services); len(reqs) > 0 {
		b.WriteString(ui.Section("Configuration") + "\n")
		for _, r := range reqs {
			fmt.Fprintf(&b, "  %s  %-30s %s\n", ui.WarningStyle.Render(ui.SymFail), r.Name, ui.MutedStyle.Render(r.Class))
		}
		b.WriteString("\n")
		fmt.Fprintf(&b, "Run:\n  %s\n", ui.HighlightStyle.Render("yoink env "+state.Canonicalize(p.Name)))
		fmt.Fprintf(&b, "Then:\n  %s\n", ui.HighlightStyle.Render("yoink up "+state.Canonicalize(p.Name)))
	}

	fmt.Print(b.String())
	return nil
}

// infraNodeRow is a rendered infrastructure node row.
type infraNodeRow struct {
	Label  string
	Detail string // "external · neon" / "local"
}

func infraNodes(g *graph.ServiceGraph) []infraNodeRow {
	var out []infraNodeRow
	for _, n := range g.Nodes {
		if n.Kind == graph.NodeApp {
			continue
		}
		label := n.InfraKind
		if n.Label != "" && n.Label != n.InfraKind {
			label = n.Label
		}
		var detail string
		switch n.Kind {
		case graph.NodeExternal:
			detail = "external"
			if n.Provider != "" {
				detail = "external · " + n.Provider
			}
		default:
			detail = "local"
		}
		out = append(out, infraNodeRow{Label: label, Detail: detail})
	}
	return out
}

func dependencyRows(g *graph.ServiceGraph) []string {
	var out []string
	for _, e := range g.Edges {
		arrow := "→"
		line := fmt.Sprintf("%s %s %s", e.From, ui.MutedStyle.Render(arrow), e.To)
		if e.EnvVar != "" {
			line += "  " + ui.DimStyle.Render("("+e.EnvVar+")")
		}
		out = append(out, line)
	}
	return out
}

func repairRows(mgr *state.Manager) []string {
	if mgr == nil {
		return nil
	}
	rh, err := mgr.LoadRepairHistory()
	if err != nil || rh == nil || len(rh.Repairs) == 0 {
		return nil
	}
	out := make([]string, 0, len(rh.Repairs))
	for _, r := range rh.Repairs {
		s := r.File
		if r.Summary != "" {
			s = r.Summary
		}
		if r.Operation != "" {
			s += "  " + ui.DimStyle.Render("("+r.Operation+")")
		}
		out = append(out, s)
	}
	return out
}

type envReqRow struct {
	Name  string
	Class string // secret | configuration
}

func requiredEnvRows(services []detector.Service) []envReqRow {
	// state.Lock has Services []detector.Service; BuildEnv holds the injected
	// placeholders. A var is "required" when it's a secret, empty, or a
	// placeholder the user must replace.
	seen := map[string]bool{}
	var out []envReqRow
	for _, s := range services {
		for name, val := range s.BuildEnv {
			if seen[name] {
				continue
			}
			req := false
			class := "configuration"
			if isSecretEnvName(name) {
				req = true
				class = "secret"
			} else if val == "" || val == "yoink-build-placeholder" {
				req = true
			}
			if !req {
				continue
			}
			seen[name] = true
			out = append(out, envReqRow{Name: name, Class: class})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// isSecretEnvName mirrors envvar.isSecretName without exporting it.
func isSecretEnvName(name string) bool {
	up := strings.ToUpper(name)
	for _, frag := range []string{"SECRET", "PASSWORD", "TOKEN", "CREDENTIAL", "PRIVATE_KEY", "API_KEY"} {
		if strings.Contains(up, frag) {
			return true
		}
	}
	return false
}

func uniqueFrameworks(svcs []detector.Service) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range svcs {
		f := s.Framework
		if f == "" {
			f = s.Language
		}
		if f != "" && !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	return out
}

func uniquePMs(svcs []detector.Service) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range svcs {
		if s.PackageManager != "" && !seen[s.PackageManager] {
			seen[s.PackageManager] = true
			out = append(out, s.PackageManager)
		}
	}
	return out
}

func appPorts(svcs []detector.Service) string {
	var ports []string
	seen := map[int]bool{}
	for _, s := range svcs {
		p := s.Port
		if p == 0 || seen[p] {
			continue
		}
		seen[p] = true
		ports = append(ports, fmt.Sprintf("%d", p))
	}
	return strings.Join(ports, ", ")
}
