package state

// InfraDetail is a compact projection of an inferred infra service, persisted
// in the lock so `yoink explain` and the ServiceGraph can render
// infrastructure mode/provider without re-running detection.
type InfraDetail struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Mode     string `json:"mode,omitempty"`     // local | external
	Provider string `json:"provider,omitempty"` // neon | upstash | ...
	Port     int    `json:"port,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// LinkRef is one app->infra dependency edge persisted for the ServiceGraph
// rebuild and `yoink explain`.
type LinkRef struct {
	To string `json:"to"` // infra service name (depends_on target)
}
