package main

import (
	"os"
	"os/exec"
)

// In Go standard layout, the primary binary lives at cmd/textify/main.go.
// If invoking `go run .` from root, delegate directly to the CLI entrypoint:
func main() {
	cmd := exec.Command("go", append([]string{"run", "./cmd/textify"}, os.Args[1:]...)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}
