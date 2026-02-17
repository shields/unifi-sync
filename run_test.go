package main

import (
	"bytes"
	"testing"
)

func TestRunNoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)
	if code != 0 {
		t.Errorf("run(nil) = %d, want 0", code)
	}
}
