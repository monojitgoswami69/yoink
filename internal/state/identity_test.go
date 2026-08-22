package state

import "testing"

func TestCanonicalize(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Sevatra", "sevatra"},
		{"My Project", "my-project"},
		{"my__project", "my-project"},
		{"  Mixed   Case  ", "mixed-case"},
		{"already-canonical", "already-canonical"},
		{"UPPER", "upper"},
		{"trailing---hyphens---", "trailing-hyphens"},
		{"snake_case_name", "snake-case-name"},
		{"", ""},
		{"   ", ""},
		{"---", ""},
	}
	for _, c := range cases {
		if got := Canonicalize(c.in); got != c.want {
			t.Errorf("Canonicalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
