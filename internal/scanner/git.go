package scanner

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// GetGitChangedFiles returns a set of relative file paths changed compared to HEAD or untracked.
func GetGitChangedFiles(root string) (map[string]bool, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	changed := make(map[string]bool)
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if len(line) < 4 {
			continue
		}
		// status is 2 chars + space + path (handling potential renames R  orig -> new)
		filePath := strings.TrimSpace(line[3:])
		if idx := strings.Index(filePath, "->"); idx != -1 {
			filePath = strings.TrimSpace(filePath[idx+2:])
		}
		// normalize slash
		filePath = filepath.ToSlash(filePath)
		changed[filePath] = true
	}

	return changed, nil
}
