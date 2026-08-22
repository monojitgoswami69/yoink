package httpcheck

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"yoink/internal/detector"
)

func TestCheckHealthyNon5xx(t *testing.T) {
	for code := 200; code <= 404; code += 1 {
		if code >= 300 && code < 400 {
			continue // keep the matrix small; 301 covered separately
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
		}))
		got := Check(context.Background(), srv.URL, 2*time.Second)
		srv.Close()
		if got.Status != Healthy {
			t.Errorf("code %d: want healthy, got %s", code, got.Status)
		}
		if got.Code != code {
			t.Errorf("code %d: got code %d", code, got.Code)
		}
	}
}

func TestCheck301RedirectHealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/ok", http.StatusMovedPermanently)
			return
		}
		w.WriteHeader(200)
	}))
	got := Check(context.Background(), srv.URL, 2*time.Second)
	srv.Close()
	// Default client follows the 301 to /ok (200) -> the app answered.
	if got.Status != Healthy {
		t.Errorf("301->200: want healthy, got %s", got.Status)
	}
}

func TestCheck5xxAppError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	got := Check(context.Background(), srv.URL, 2*time.Second)
	srv.Close()
	if got.Status != AppError {
		t.Errorf("503: want application_error, got %s", got.Status)
	}
	if got.Code != 503 {
		t.Errorf("503: got code %d", got.Code)
	}
}

func TestCheckUnreachable(t *testing.T) {
	// A port with no listener.
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	_ = l.Close()
	got := Check(context.Background(), "http://"+addr+"/", 1*time.Second)
	if got.Status != Unreachable {
		t.Errorf("refused: want unreachable, got %s (err=%s)", got.Status, got.Err)
	}
}

func TestCheckTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	got := Check(context.Background(), srv.URL, 50*time.Millisecond)
	srv.Close()
	if got.Status != Timeout {
		t.Errorf("want timeout, got %s (err=%s)", got.Status, got.Err)
	}
}

func TestServicesSkipsInfraAndUnexposed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()
	port := portOf(t, srv.URL)

	services := []detector.Service{
		{ID: "service-1", Framework: "next", Port: 3000}, // app, published
		{ID: "postgres", Framework: "", Port: 5432},      // infra (no port map entry)
		{ID: "service-2", Framework: "vite", Port: 5173}, // app, not published
	}
	portMap := map[string]int{"service-1": port}

	res := Services(context.Background(), services, portMap)
	if len(res) != 1 {
		t.Fatalf("expected 1 probe (only service-1 publishes), got %d: %+v", len(res), res)
	}
	if res[0].Service != "service-1" || res[0].Status != Healthy {
		t.Errorf("service-1 should be healthy, got %+v", res[0])
	}
}

func TestAllHealthy(t *testing.T) {
	if !AllHealthy(nil) {
		t.Error("nil/empty should be vacuously healthy")
	}
	if !AllHealthy([]Result{{Status: Healthy}, {Status: Healthy}}) {
		t.Error("two healthy should be all-healthy")
	}
	if AllHealthy([]Result{{Status: Healthy}, {Status: AppError, Code: 500}}) {
		t.Error("one app-error should not be all-healthy")
	}
}

// portOf extracts the port from an httptest URL like http://127.0.0.1:PORT.
func portOf(t *testing.T, url string) int {
	t.Helper()
	_, port, err := net.SplitHostPort(url[len("http://"):])
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	var p int
	for _, c := range port {
		p = p*10 + int(c-'0')
	}
	return p
}
