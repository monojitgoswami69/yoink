package docker

import (
	"strings"
	"testing"
)

func TestParsePsOutputNDJSON(t *testing.T) {
	in := `{"Name":"yoink-web","Service":"web","State":"running","Health":"healthy"}
{"Name":"yoink-api","Service":"api","State":"exited","Health":"unhealthy"}`
	got, err := parsePsOutput(in)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(got))
	}
	if got[0].Service != "web" || got[1].State != "exited" {
		t.Errorf("unexpected rows: %+v", got)
	}
}

func TestParsePsOutputArray(t *testing.T) {
	in := `[{"Name":"yoink-web","Service":"web","State":"running"}]`
	got, err := parsePsOutput(in)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(got) != 1 || got[0].Service != "web" {
		t.Errorf("unexpected rows: %+v", got)
	}
}

func TestParsePsOutputEmpty(t *testing.T) {
	got, err := parsePsOutput("   \n\n")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty result, got %+v", got)
	}
}

// Regression: compose v2 emits Publishers as a JSON array. Earlier versions of
// the Container struct declared `Ports string \"Publishers\"`, which caused
// json.Unmarshal to fail on every row — parsePsOutput then silently dropped
// each container, leaving Ps() returning zero rows and breaking the healthcheck
// wait in `yoink up`.
func TestParsePsOutputComposeV2WithPublishers(t *testing.T) {
	in := `{"ID":"e316","Name":"yoink-web","Service":"web","State":"running","Status":"Up 3 minutes (healthy)","Health":"healthy","Ports":"0.0.0.0:5000->5000/tcp","Publishers":[{"URL":"0.0.0.0","TargetPort":5000,"PublishedPort":5000,"Protocol":"tcp"}]}`
	got, err := parsePsOutput(in)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 container, got %d", len(got))
	}
	if got[0].Health != "healthy" || got[0].Service != "web" {
		t.Errorf("unexpected row: %+v", got[0])
	}
}

func TestExtractFailedServiceFromCommonFormats(t *testing.T) {
	cases := []struct {
		log  string
		want string
	}{
		{"=> ERROR [api 3/5] RUN pip install\n\nfailed to solve: process did not complete successfully\nERROR: failed to build api", "api"},
		{"Service 'web' failed to build : Build failed", "web"},
		{"backend | ERROR: thing exploded", "backend"},
	}
	for _, tc := range cases {
		if got := ExtractFailedService(tc.log); got != tc.want {
			t.Errorf("ExtractFailedService(%q) = %q, want %q", tc.log, got, tc.want)
		}
	}
}

func TestTailLinesCapsLongInput(t *testing.T) {
	in := strings.Repeat("line\n", 50)
	out := TailLines(in, 5)
	if strings.Count(out, "\n") != 5 {
		t.Errorf("expected 5 newlines, got %d (%q)", strings.Count(out, "\n"), out)
	}
}

func TestParsePctHandlesSuffix(t *testing.T) {
	if got := parsePct("12.5%"); got != 12.5 {
		t.Errorf("expected 12.5, got %v", got)
	}
	if got := parsePct(""); got != 0 {
		t.Errorf("expected 0 for empty, got %v", got)
	}
}
