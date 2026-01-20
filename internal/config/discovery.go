package config

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/monochromegane/go-gitignore"
)

// Discover populates the Config.Dirs map by scanning the file system.
// It respects .gitignore to set the 'Enabled' boolean.
// It preserves existing rules in existingCfg if provided.
func Discover(root string, existingCfg *Config) (*Config, error) {
	cfg := DefaultConfig()
	if existingCfg != nil {
		cfg = *existingCfg
		if cfg.Dirs == nil {
			cfg.Dirs = make(map[string]DirRule)
		}
	}

	// Load gitignore logic
	ignoreMatcher := getIgnoreMatcher(root)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Calculate relative path for map key
		relPath, _ := filepath.Rel(root, path)
		if relPath == "." {
			relPath = "."
		} else {
			relPath = filepath.ToSlash(relPath)
		}

		// Skip .git folder entirely
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}

		// If entry already exists in config, we don't overwrite it
		// except maybe to update extensions if we wanted to be very aggressive,
		// but usually preserving user config is safer.
		if _, exists := cfg.Dirs[relPath]; exists {
			// If the user manually disabled it, we don't recurse if it's a dir
			if !cfg.Dirs[relPath].Enabled && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Check GitIgnore
		isIgnored := ignoreMatcher.Match(path, d.IsDir())

		if d.IsDir() {
			// If it's a directory
			rule := DirRule{
				Enabled: !isIgnored,
			}

			// If it's ignored (like node_modules), set Enabled=false and SKIP recursing
			// This prevents adding 10,000 lines for node_modules subfolders
			if isIgnored {
				cfg.Dirs[relPath] = rule
				return filepath.SkipDir
			}

			// If valid directory, detect extensions inside it (shallow check)
			rule.Extensions = detectExtensions(path)
			
			// If no extensions found but it's a dir, we still add it but maybe it's empty
			if len(rule.Extensions) > 0 {
				cfg.Dirs[relPath] = rule
			}
		}

		return nil
	})

	return &cfg, err
}

// detectExtensions scans the immediate files in a directory to gather extensions.
func detectExtensions(dirPath string) []string {
	extMap := make(map[string]bool)
	entries, _ := os.ReadDir(dirPath)

	for _, e := range entries {
		if !e.IsDir() {
			ext := filepath.Ext(e.Name())
			if len(ext) > 1 {
				// Remove dot
				cleanExt := strings.TrimPrefix(ext, ".")
				extMap[cleanExt] = true
			}
		}
	}

	var extensions []string
	for ext := range extMap {
		extensions = append(extensions, ext)
	}
	return extensions
}

// getIgnoreMatcher attempts to load .gitignore from the root path.
func getIgnoreMatcher(root string) gitignore.IgnoreMatcher {
	gitignorePath := filepath.Join(root, ".gitignore")
	matcher, err := gitignore.NewGitIgnore(gitignorePath)
	if err != nil {
		return gitignore.NewGitIgnoreFromReader(root, strings.NewReader(""))
	}
	return matcher
}
