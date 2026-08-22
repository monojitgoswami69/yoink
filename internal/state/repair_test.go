package state

import (
	"testing"
	"yoink/internal/detector"
)

func TestRepairProvenanceRecordAndLoad(t *testing.T) {
	defer withTempHome(t)()
	m, err := For("test-project")
	if err != nil {
		t.Fatal(err)
	}
	rec := RepairRecord{
		ID: "repair-1", Service: "service-1", File: "Dockerfile.service-1",
		OriginalHash: "abc", ResultingHash: "def", Operation: "llm-patch",
		Diagnosis: "Missing OS package", Summary: "Added libc6-compat",
	}
	if err := m.RecordRepair(rec); err != nil {
		t.Fatal(err)
	}
	rh, err := m.LoadRepairHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(rh.Repairs) != 1 {
		t.Fatalf("expected 1 repair, got %d", len(rh.Repairs))
	}
	if rh.Repairs[0].File != "Dockerfile.service-1" {
		t.Errorf("file: %s", rh.Repairs[0].File)
	}
}

func TestIsHealedFile(t *testing.T) {
	defer withTempHome(t)()
	m, _ := For("test-project2")
	m.RecordRepair(RepairRecord{File: "Dockerfile.service-1", Operation: "llm-patch"})
	if !m.IsHealedFile("Dockerfile.service-1") {
		t.Error("should be healed")
	}
	if m.IsHealedFile("Dockerfile.service-2") {
		t.Error("should not be healed")
	}
}

func TestHealedFiles(t *testing.T) {
	defer withTempHome(t)()
	m, _ := For("test-project3")
	m.RecordRepair(RepairRecord{File: "Dockerfile.service-1"})
	m.RecordRepair(RepairRecord{File: "docker-compose.yml"})
	files := m.HealedFiles()
	if len(files) != 2 {
		t.Fatalf("expected 2 healed files, got %d", len(files))
	}
}

func TestStateHashIncludesBuildCmdStartCmd(t *testing.T) {
	svc1 := []detector.Service{
		{ID: "s1", Framework: "node", Port: 3000, BuildCmd: []string{"npm", "run", "build"}, StartCmd: []string{"npm", "start"}},
	}
	hash1 := HashDetection(svc1)
	// Same services but different BuildCmd → different hash
	svc2 := []detector.Service{
		{ID: "s1", Framework: "node", Port: 3000, BuildCmd: []string{"npm", "run", "build2"}, StartCmd: []string{"npm", "start"}},
	}
	hash2 := HashDetection(svc2)
	if hash1 == hash2 {
		t.Error("hash should change when BuildCmd changes")
	}
	// Same services with same BuildCmd → same hash
	svc3 := []detector.Service{
		{ID: "s1", Framework: "node", Port: 3000, BuildCmd: []string{"npm", "run", "build"}, StartCmd: []string{"npm", "start"}},
	}
	hash3 := HashDetection(svc3)
	if hash1 != hash3 {
		t.Error("hash should be stable when BuildCmd is unchanged")
	}
}
