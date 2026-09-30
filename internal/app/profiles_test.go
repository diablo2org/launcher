package app

import (
	"os"
	"strings"
	"testing"
)

func TestCheck(t *testing.T) {
	example, err := os.ReadFile("../../examples/slashdiablo/profile.json")
	if err != nil {
		t.Fatal(err)
	}

	s := NewProfileService(example)

	if r := s.Check(s.Example()); !r.OK || r.Name != "SlashDiablo" || len(r.Problems) != 0 {
		t.Errorf("example: %+v", r)
	}

	if r := s.Check("{"); r.OK || len(r.Problems) == 0 {
		t.Errorf("broken JSON: %+v", r)
	}

	// Schema failures come back one per problem, each saying where, with no
	// summary line counted as a problem.
	r := s.Check(`{"schema":1,"id":"Bad Id","name":"X","version":1,"hosts":["x.net"],"game":{"version":"1.12","baseArchives":"link"},"gateways":[],"channels":[]}`)
	if r.OK || len(r.Problems) != 4 {
		t.Fatalf("got %d problems, want 4: %q", len(r.Problems), r.Problems)
	}
	for _, p := range r.Problems {
		if !strings.HasPrefix(p, "at /") {
			t.Errorf("problem %q does not say where", p)
		}
	}
}
