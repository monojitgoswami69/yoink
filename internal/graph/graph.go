// Package graph builds an explicit, evidence-driven ServiceGraph over the
// detected application services, the inferred local infrastructure, and the
// external infrastructure (Neon, Upstash, …). It is a projection on top of
// the existing detector.Service / infra.Service / infra.AppLink models — it
// does NOT replace them.
//
// The graph models three node kinds (app, local infra, external infra) and
// two edge kinds:
//
//   - dependency  (app -> infra, from infra.AppLink — the env/deps evidence)
//   - env-binding (app -> app, from a build-time env var whose value is a
//     URL referencing another app service's port — only when unambiguous)
//
// Relationships are never guessed: edges are only added when there is
// concrete evidence (an AppLink, or a port referenced by name in an env var).
package graph

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"yoink/internal/detector"
	"yoink/internal/infra"
)

// NodeKind classifies a graph node.
type NodeKind int

const (
	NodeApp      NodeKind = iota // an application service (detector.Service)
	NodeInfra                    // local infrastructure Yoink provisions in compose
	NodeExternal                 // external/cloud infrastructure (Neon, Upstash, …) — not provisioned
)

func (k NodeKind) String() string {
	switch k {
	case NodeApp:
		return "app"
	case NodeInfra:
		return "infra"
	case NodeExternal:
		return "external"
	}
	return "unknown"
}

// Node is the flat, stably-ordered projection of a service or infra resource.
type Node struct {
	ID        string // canonical id (service ID or infra name)
	Kind      NodeKind
	Label     string // display name
	Framework string // app only
	Language  string // app only
	Type      string // app: frontend | backend
	Port      int    // internal (container) port
	HostPort  int    // published host port (app only)
	InfraKind string // postgres | redis | … (infra only)
	Provider  string // cloud provider (external only)
	Evidence  string // why this node exists / was linked
}

// EdgeKind classifies a graph edge.
type EdgeKind int

const (
	EdgeDepends    EdgeKind = iota // app depends on an infra service (depends_on)
	EdgeEnvBinding                 // app binds to another app via an env var URL
)

func (k EdgeKind) String() string {
	if k == EdgeEnvBinding {
		return "env-binding"
	}
	return "depends"
}

// Edge is a runtime dependency from one node to another.
type Edge struct {
	From        string // source node id
	To          string // target node id
	Kind        EdgeKind
	EnvVar      string // binding var name (EdgeEnvBinding only)
	InternalURL string // compose-DNS url the server side should use, e.g. http://backend:8000
	ExternalURL string // host url a browser should use, e.g. http://localhost:8001 (app targets only)
	Evidence    string // why this edge exists
}

// ServiceGraph is the deterministic, stably-ordered service graph.
type ServiceGraph struct {
	Nodes []Node
	Edges []Edge
}

// Find returns the node with the given id, or nil.
func (g *ServiceGraph) Find(id string) *Node {
	for i := range g.Nodes {
		if g.Nodes[i].ID == id {
			return &g.Nodes[i]
		}
	}
	return nil
}

// IsExternal reports whether id is an external (cloud) infra node.
func (g *ServiceGraph) IsExternal(id string) bool {
	if n := g.Find(id); n != nil {
		return n.Kind == NodeExternal
	}
	return false
}

// EdgesFrom returns edges whose source is id.
func (g *ServiceGraph) EdgesFrom(id string) []Edge {
	var out []Edge
	for _, e := range g.Edges {
		if e.From == id {
			out = append(out, e)
		}
	}
	return out
}

// StartOrder returns node IDs in the order services should be started:
// local infrastructure first, then application services (dependency-aware,
// stable on ties). External infra is omitted — it is never started locally.
func (g *ServiceGraph) StartOrder() []string {
	var infra, apps []string
	for _, n := range g.Nodes {
		switch n.Kind {
		case NodeInfra:
			infra = append(infra, n.ID)
		case NodeApp:
			apps = append(apps, n.ID)
		}
	}
	sort.Strings(infra)
	apps = topoApps(g, apps)
	return append(infra, apps...)
}

// PublicAppNodes returns app nodes that publish a host port (i.e. the ones
// HTTP verification and `yoink open` care about), in stable order.
func (g *ServiceGraph) PublicAppNodes() []Node {
	var out []Node
	for _, n := range g.Nodes {
		if n.Kind == NodeApp && n.HostPort != 0 {
			out = append(out, n)
		}
	}
	return out
}

// Build constructs an evidence-driven service graph from the detected
// services, inferred infra, app->infra links, and the published port map.
// No relationships are guessed: edges come only from infra.AppLink evidence
// or from an unambiguous env-var URL binding between two app services.
func Build(services []detector.Service, infras []infra.Service, links map[string][]infra.AppLink, portMap map[string]int) *ServiceGraph {
	g := &ServiceGraph{}

	// App nodes.
	for _, s := range services {
		n := Node{
			ID:        s.ID,
			Kind:      NodeApp,
			Label:     s.Framework,
			Framework: s.Framework,
			Language:  s.Language,
			Type:      s.Type,
			Port:      s.Port,
			HostPort:  portMap[s.ID],
			Evidence:  fmt.Sprintf("detected %s/%s", s.Language, s.Framework),
		}
		if n.Port == 0 {
			n.Port = 3000
		}
		g.Nodes = append(g.Nodes, n)
	}

	// Infra nodes (local + external).
	for _, in := range infras {
		n := Node{
			ID:        in.Name,
			Label:     in.Name,
			InfraKind: string(in.Kind),
			Provider:  in.Provider,
			Port:      in.Port,
		}
		switch in.Mode {
		case "external":
			n.Kind = NodeExternal
			n.Evidence = in.Reason
			if in.Provider != "" {
				n.Evidence = fmt.Sprintf("external %s (%s)", in.Provider, in.Reason)
			}
		default: // "local" or "unknown" -> provisioned locally
			n.Kind = NodeInfra
			n.Evidence = in.Reason
		}
		g.Nodes = append(g.Nodes, n)
	}

	// Edges: app -> infra (from AppLink evidence).
	for appID, ls := range links {
		for _, link := range ls {
			target := g.Find(link.ServiceName)
			if target == nil {
				continue // link to an infra node that wasn't materialised
			}
			e := Edge{
				From:     appID,
				To:       link.ServiceName,
				Kind:     EdgeDepends,
				Evidence: linkEnvEvidence(link),
			}
			if target.Port != 0 {
				e.InternalURL = fmt.Sprintf("http://%s:%d", link.ServiceName, target.Port)
			}
			g.Edges = append(g.Edges, e)
		}
	}

	// Edges: app -> app (env-var URL binding, unambiguous only).
	addAppAppEdges(g, services, portMap)

	// Stable ordering: nodes by (kind, id), edges by (from, to, kind).
	sort.Slice(g.Nodes, func(i, j int) bool {
		if g.Nodes[i].Kind != g.Nodes[j].Kind {
			return g.Nodes[i].Kind < g.Nodes[j].Kind
		}
		return g.Nodes[i].ID < g.Nodes[j].ID
	})
	sort.Slice(g.Edges, func(i, j int) bool {
		if g.Edges[i].From != g.Edges[j].From {
			return g.Edges[i].From < g.Edges[j].From
		}
		return g.Edges[i].To < g.Edges[j].To
	})

	return g
}

// linkEnvEvidence summarises the env vars an AppLink injects, as the reason
// the dependency edge exists.
func linkEnvEvidence(link infra.AppLink) string {
	if len(link.EnvVars) == 0 {
		return "inferred dependency"
	}
	keys := make([]string, 0, len(link.EnvVars))
	for k := range link.EnvVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return "env: " + strings.Join(keys, ", ")
}

// addAppAppEdges adds app->app edges where one app's build-time env var value
// is a URL referencing another app's internal port. Only when exactly one
// other app exposes that port (unambiguous) — ambiguous bindings are left
// unresolved rather than guessed.
func addAppAppEdges(g *ServiceGraph, services []detector.Service, portMap map[string]int) {
	// port -> the single app that exposes it (nil if shared/absent).
	byPort := map[int]*detector.Service{}
	for i := range services {
		s := &services[i]
		port := s.Port
		if port == 0 {
			port = 3000
		}
		if prev := byPort[port]; prev != nil {
			byPort[port] = nil // collision -> ambiguous
		} else {
			byPort[port] = s
		}
	}
	for i := range services {
		a := &services[i]
		for name, val := range a.BuildEnv {
			port, ok := urlPort(val)
			if !ok {
				continue
			}
			b := byPort[port]
			if b == nil || b.ID == a.ID {
				continue // ambiguous or self
			}
			g.Edges = append(g.Edges, Edge{
				From:        a.ID,
				To:          b.ID,
				Kind:        EdgeEnvBinding,
				EnvVar:      name,
				InternalURL: fmt.Sprintf("http://%s:%d", b.ID, port),
				ExternalURL: fmt.Sprintf("http://localhost:%d", portMap[b.ID]),
				Evidence:    fmt.Sprintf("env %s references port %d", name, port),
			})
		}
	}
}

// urlPort extracts the port from an http(s)://host:port[...] URL value. It
// tolerates trailing paths. Returns ok=false for placeholders or non-URLs.
func urlPort(v string) (int, bool) {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "http://") && !strings.HasPrefix(v, "https://") {
		return 0, false
	}
	if strings.Contains(v, "yoink-build-placeholder") {
		return 0, false
	}
	// strip scheme
	rest := v[strings.Index(v, "://")+3:]
	// drop path
	if i := strings.IndexAny(rest, "/?"); i >= 0 {
		rest = rest[:i]
	}
	// host[:port] -> take the part after the last colon
	i := strings.LastIndex(rest, ":")
	if i < 0 {
		return 0, false
	}
	port, err := strconv.Atoi(rest[i+1:])
	if err != nil || port <= 0 || port > 65535 {
		return 0, false
	}
	return port, true
}

// topoApps returns app IDs in dependency order: an app that env-binds to
// another app is ordered after its target. Ties (including cycles) fall back
// to stable id order so the result is always total and deterministic.
func topoApps(g *ServiceGraph, apps []string) []string {
	order := map[string]int{}
	for i, id := range apps {
		order[id] = i
	}
	sort.SliceStable(apps, func(i, j int) bool {
		// i before j if j depends-on i (an edge i -> j exists).
		for _, e := range g.Edges {
			if e.From == apps[j] && e.To == apps[i] {
				return true
			}
		}
		if order[apps[i]] != order[apps[j]] {
			return order[apps[i]] < order[apps[j]]
		}
		return apps[i] < apps[j]
	})
	return apps
}
