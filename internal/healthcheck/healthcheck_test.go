package healthcheck

import (
	"strings"
	"testing"
)

func TestForAppUsesCurlAgainstSpecifiedPort(t *testing.T) {
	s := ForApp("fastapi", 8000)
	if len(s.Test) != 2 || s.Test[0] != "CMD-SHELL" {
		t.Fatalf("expected CMD-SHELL probe, got %+v", s.Test)
	}
	if !strings.Contains(s.Test[1], "http://localhost:8000/") {
		t.Errorf("probe should target supplied port: %q", s.Test[1])
	}
	if s.Retries < 3 {
		t.Errorf("retries should be at least 3, got %d", s.Retries)
	}
}

func TestForAppForcesPort80OnStaticSPA(t *testing.T) {
	for _, fw := range []string{"react", "vite", "cra"} {
		s := ForApp(fw, 3000)
		if !strings.Contains(s.Test[1], "http://localhost:80/") {
			t.Errorf("%s should probe nginx on port 80, got %q", fw, s.Test[1])
		}
	}
}

func TestInstallCurlPicksRightPackageManager(t *testing.T) {
	if got := InstallCurl("fastapi"); !strings.Contains(got, "apt-get") {
		t.Errorf("python runtime should use apt-get, got %q", got)
	}
	if got := InstallCurl("next"); !strings.Contains(got, "apk add") {
		t.Errorf("node runtime should use apk, got %q", got)
	}
}
