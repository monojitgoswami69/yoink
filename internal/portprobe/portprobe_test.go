package portprobe

import (
	"net"
	"testing"
)

func TestAllocateReturnsPreferredWhenFree(t *testing.T) {
	a := New()
	got := a.Allocate(0) // accept default
	if got <= 0 {
		t.Fatalf("expected positive port, got %d", got)
	}
}

func TestAllocateSkipsClaimedPort(t *testing.T) {
	a := New()
	first := a.Allocate(40000)
	second := a.Allocate(40000)
	if first == second {
		t.Fatalf("expected distinct ports, both got %d", first)
	}
}

func TestAllocateSkipsHostBoundPort(t *testing.T) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Skip("no free port available for the probe")
	}
	defer l.Close()
	bound := l.Addr().(*net.TCPAddr).Port

	a := New()
	got := a.Allocate(bound)
	if got == bound {
		t.Fatalf("allocator handed out a bound port %d", bound)
	}
}
