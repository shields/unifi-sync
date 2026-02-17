package main

import "testing"

func TestMain_osExit(t *testing.T) {
	exitCode := -1
	origExit := osExit
	osExit = func(code int) { exitCode = code }
	defer func() { osExit = origExit }()

	main()

	if exitCode != 0 {
		t.Errorf("main() exit code = %d, want 0", exitCode)
	}
}
