package detector

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// detectRust classifies a Rust project. A Cargo.toml file is the signal. The
// service runs as a compiled binary with `cargo build --release`.
func detectRust(dir string, svc *Service) bool {
	svc.Language = "rust"
	svc.PackageManager = "cargo"
	svc.addEvidence("language=rust", "Cargo.toml exists", "strong")

	cargoPath := filepath.Join(dir, "Cargo.toml")
	if _, err := os.Stat(cargoPath); err != nil {
		return false
	}

	// Determine the binary name from [package] name in Cargo.toml.
	binaryName := "app"
	data, _ := os.ReadFile(cargoPath)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name = ") {
			name := strings.TrimSpace(strings.TrimPrefix(line, "name = "))
			name = strings.Trim(name, "\"")
			if name != "" {
				binaryName = name
				break
			}
		}
	}

	svc.Framework = "rust"
	svc.Type = "backend"
	svc.Confidence = "medium"
	svc.Port = 8080
	svc.HasLockfile = exists(filepath.Join(dir, "Cargo.lock"))
	svc.InstallCmd = []string{} // cargo fetch is implicit in build
	svc.BuildCmd = []string{"cargo", "build", "--release"}
	svc.StartCmd = []string{"./target/release/" + binaryName}
	svc.addEvidence("framework=rust", "Cargo.toml present", "strong")
	svc.addEvidence("port=8080", "rust default", "weak")
	_ = strconv.Itoa // keep import for future port-scan
	return true
}
