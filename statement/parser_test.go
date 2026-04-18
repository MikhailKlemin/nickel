//go:build go1.25

package statement

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParse_FromTextFixture(t *testing.T) {
	inputPath := "testdata/sample_statement.txt"
	goldenPath := "testdata/sample_statement.golden.json"

	raw, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatalf("read input fixture: %v", err)
	}

	gotStmt, err := Parse(string(raw), nil)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}

	gotJSON, err := json.MarshalIndent(gotStmt, "", "  ")
	if err != nil {
		t.Fatalf("marshal parsed statement: %v", err)
	}

	wantJSON, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden file: %v", err)
	}

	got := strings.TrimSpace(string(gotJSON))
	want := strings.TrimSpace(string(wantJSON))
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("golden mismatch (-want +got):\n%s", diff)
	}
}
