package main

import "testing"

func TestMain_osExit(t *testing.T) {
	exitCode := -1
	origExit := osExit
	osExit = func(code int) { exitCode = code }
	defer func() { osExit = origExit }()

	main()

	if exitCode == -1 {
		t.Error("main() did not call osExit")
	}
}
