// Package portprobe finds free host ports. It is used by the compose
// generator to remap container ports onto host ports that are not currently
// bound by some other process.
package portprobe

import (
	"net"
	"strconv"
)

// Allocator hands out host ports, remembering what it has already promised.
// Use New to construct one and Allocate to claim a port.
type Allocator struct {
	used map[int]bool
}

// New returns an Allocator with no ports yet claimed.
func New() *Allocator { return &Allocator{used: map[int]bool{}} }

// Allocate returns a free host port to map onto containerPort. It starts at
// the preferred host port and walks forward until it finds one that is both
// unclaimed by this Allocator and unbound on the host. The chosen port is
// marked claimed.
//
// The host probe binds to all interfaces (0.0.0.0); a port that is bound on
// any interface is considered busy. If every candidate is busy (extremely
// unlikely), the loop falls back to the next unclaimed integer above
// preferred without verifying it on the host.
func (a *Allocator) Allocate(preferred int) int {
	if preferred <= 0 {
		preferred = 3000
	}
	const maxScan = 100
	p := preferred
	for i := 0; i < maxScan; i++ {
		if !a.used[p] && IsFree(p) {
			a.used[p] = true
			return p
		}
		p++
	}
	// Last resort: take the next unclaimed integer, even if the host probe
	// says it's busy. Better than spinning forever.
	for a.used[p] {
		p++
	}
	a.used[p] = true
	return p
}

// IsFree probes whether tcp/p is currently bindable on all interfaces.
func IsFree(p int) bool {
	addr := ":" + strconv.Itoa(p)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	_ = l.Close()
	return true
}
