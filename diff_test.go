package main

import (
	"strings"
	"testing"
)

func TestComputeDiffIdentical(t *testing.T) {
	a := []string{"one", "two", "three"}
	ops := computeDiff(a, a)
	if len(ops) != len(a) {
		t.Fatalf("len(ops) = %d, want %d", len(ops), len(a))
	}
	for _, op := range ops {
		if op.kind != diffEqual {
			t.Errorf("expected all equal, got %v", op)
		}
	}
}

func TestComputeDiffAdditions(t *testing.T) {
	a := []string{"one", "three"}
	b := []string{"one", "two", "three"}
	ops := computeDiff(a, b)
	hasAdd := false
	for _, op := range ops {
		if op.kind == diffAdd {
			hasAdd = true
			if op.line != "two" {
				t.Errorf("added line = %q, want %q", op.line, "two")
			}
		}
	}
	if !hasAdd {
		t.Error("expected an addition")
	}
}

func TestComputeDiffDeletions(t *testing.T) {
	a := []string{"one", "two", "three"}
	b := []string{"one", "three"}
	ops := computeDiff(a, b)
	hasDel := false
	for _, op := range ops {
		if op.kind == diffDel {
			hasDel = true
			if op.line != "two" {
				t.Errorf("deleted line = %q, want %q", op.line, "two")
			}
		}
	}
	if !hasDel {
		t.Error("expected a deletion")
	}
}

func TestComputeDiffModification(t *testing.T) {
	a := []string{"one", "two", "three"}
	b := []string{"one", "TWO", "three"}
	ops := computeDiff(a, b)
	hasDel, hasAdd := false, false
	for _, op := range ops {
		if op.kind == diffDel && op.line == "two" {
			hasDel = true
		}
		if op.kind == diffAdd && op.line == "TWO" {
			hasAdd = true
		}
	}
	if !hasDel || !hasAdd {
		t.Errorf("expected del+add for modification, got del=%v add=%v", hasDel, hasAdd)
	}
}

func TestComputeDiffEmpty(t *testing.T) {
	ops := computeDiff(nil, nil)
	if len(ops) != 0 {
		t.Errorf("expected empty diff, got %d ops", len(ops))
	}
}

func TestComputeDiffAllNew(t *testing.T) {
	ops := computeDiff(nil, []string{"a", "b"})
	adds := 0
	for _, op := range ops {
		if op.kind == diffAdd {
			adds++
		}
	}
	if adds != 2 {
		t.Errorf("expected 2 additions, got %d", adds)
	}
}

func TestComputeDiffAllDeleted(t *testing.T) {
	ops := computeDiff([]string{"a", "b"}, nil)
	dels := 0
	for _, op := range ops {
		if op.kind == diffDel {
			dels++
		}
	}
	if dels != 2 {
		t.Errorf("expected 2 deletions, got %d", dels)
	}
}

func TestFormatUnifiedDiffNoChanges(t *testing.T) {
	a := []string{"one", "two"}
	ops := computeDiff(a, a)
	out := formatDiff(ops, "a.json", "b.json", false)
	if out != "" {
		t.Errorf("expected empty output for identical, got %q", out)
	}
}

func TestFormatUnifiedDiffWithChanges(t *testing.T) {
	a := []string{`{`, `  "name": "old"`, `}`}
	b := []string{`{`, `  "name": "new"`, `}`}
	ops := computeDiff(a, b)
	out := formatDiff(ops, "local", "remote", false)
	if !strings.Contains(out, "--- local") {
		t.Error("missing --- header")
	}
	if !strings.Contains(out, "+++ remote") {
		t.Error("missing +++ header")
	}
	if !strings.Contains(out, "-  \"name\": \"old\"") {
		t.Errorf("missing deletion line in:\n%s", out)
	}
	if !strings.Contains(out, "+  \"name\": \"new\"") {
		t.Errorf("missing addition line in:\n%s", out)
	}
}

func TestFormatUnifiedDiffColor(t *testing.T) {
	a := []string{"old"}
	b := []string{"new"}
	ops := computeDiff(a, b)
	out := formatDiff(ops, "a", "b", true)
	if !strings.Contains(out, "\033[") {
		t.Error("expected ANSI color codes")
	}
}

func TestFormatUnifiedDiffNoColor(t *testing.T) {
	a := []string{"old"}
	b := []string{"new"}
	ops := computeDiff(a, b)
	out := formatDiff(ops, "a", "b", false)
	if strings.Contains(out, "\033[") {
		t.Error("expected no ANSI color codes")
	}
}
