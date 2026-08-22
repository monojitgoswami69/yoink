package detector

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

// detectGo classifies a Go project. A go.mod file is the signal. The service
// runs as a compiled binary with `go build` + the resulting executable.
func detectGo(dir string, svc *Service) bool {
	svc.Language = "go"
	svc.PackageManager = "go"
	svc.addEvidence("language=go", "go.mod exists", "strong")

	modPath := filepath.Join(dir, "go.mod")
	if _, err := os.Stat(modPath); err != nil {
		return false
	}

	binaryName := filepath.Base(dir)
	if binaryName == "" || binaryName == "." {
		binaryName = "app"
	}

	svc.Framework = "go"
	svc.Type = "backend"
	svc.Confidence = "medium"
	svc.Port = 8080
	svc.HasLockfile = exists(filepath.Join(dir, "go.sum"))
	svc.InstallCmd = []string{"go", "mod", "download"}
	svc.BuildCmd = []string{"go", "build", "-o", binaryName, "."}
	svc.StartCmd = []string{"./" + binaryName}
	svc.addEvidence("framework=go", "go.mod present", "strong")
	svc.addEvidence("port=8080", "go default", "weak")

	if port, ok := portFromGoSource(dir); ok {
		svc.Port = port
		svc.addEvidence("port="+strconv.Itoa(port), "source .Listen() or PORT env", "strong")
	}
	return true
}

var (
	goListenRe = regexp.MustCompile(`\.Listen\(\s*['"]?(\d{2,5})['"]?`)
	goPortRe   = regexp.MustCompile(`os\.Getenv\(["']PORT["']\)|.*PORT.*\|\|.*?(\d{2,5})`)
)

func portFromGoSource(dir string) (int, bool) {
	found := false
	var port int
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == "" || name[0] == '.' || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		text := string(data)
		if m := goListenRe.FindStringSubmatch(text); len(m) == 2 {
			if n, e := strconv.Atoi(m[1]); e == nil && n >= 80 && n <= 65535 {
				port = n
				found = true
				return filepath.SkipDir
			}
		}
		if m := goPortRe.FindStringSubmatch(text); len(m) == 2 {
			if n, e := strconv.Atoi(m[1]); e == nil && n >= 80 && n <= 65535 {
				port = n
				found = true
				return filepath.SkipDir
			}
		}
		return nil
	})
	return port, found
}
