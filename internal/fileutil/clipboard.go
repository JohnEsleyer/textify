package fileutil

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// CopyToClipboard copies string content to the system clipboard.
// Tries platform-native commands first, then falls back to terminal OSC 52.
func CopyToClipboard(content string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.Command("wl-copy")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		}
	case "windows":
		cmd = exec.Command("clip")
	}

	if cmd != nil {
		cmd.Stdin = bytes.NewReader([]byte(content))
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	// Fallback to ANSI OSC 52 terminal copy
	b64 := base64.StdEncoding.EncodeToString([]byte(content))
	osc52 := fmt.Sprintf("\x1b]52;c;%s\x07", b64)
	if _, err := os.Stderr.WriteString(osc52); err == nil {
		return nil
	}

	return fmt.Errorf("clipboard command not available")
}

// ReadFromClipboard reads string content from the system clipboard.
func ReadFromClipboard() (string, error) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbpaste")
	case "linux":
		if _, err := exec.LookPath("wl-paste"); err == nil {
			cmd = exec.Command("wl-paste", "--no-newline")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard", "-o")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--output")
		}
	case "windows":
		cmd = exec.Command("powershell", "-NoProfile", "-Command", "Get-Clipboard")
	}

	if cmd != nil {
		out, err := cmd.Output()
		if err == nil {
			return string(out), nil
		}
	}

	return "", fmt.Errorf("clipboard read command not available or failed")
}
