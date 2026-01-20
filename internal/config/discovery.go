package config

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/monochromegane/go-gitignore"
)

// Discover populates the Config.Dirs map by scanning ONLY top-level directories.
// It aggregates extensions from subdirectories to ensure the top-level rule covers children.
func Discover(root string, existingCfg *Config) (*Config, error) {
	cfg := DefaultConfig()
	if existingCfg != nil {
		cfg = *existingCfg
		if cfg.Dirs == nil {
			cfg.Dirs = make(map[string]DirRule)
		}
	}

	ignoreMatcher := getIgnoreMatcher(root)

	// 1. Update Root (.) Rule
	// We scan the *entire* project to find common extensions for the root fallback
	rootExtensions := deepScanExtensions(root, root, ignoreMatcher)
	
	// Preserve existing root settings if they exist, otherwise update extensions
	if val, ok := cfg.Dirs["."]; ok {
		// Optional: Merge extensions if you want, or just keep user's. 
		// For now, let's trust the user if they edited it, or update if empty.
		if len(val.Extensions) == 0 {
			val.Extensions = rootExtensions
			cfg.Dirs["."] = val
		}
	} else {
		cfg.Dirs["."] = DirRule{
			Enabled:    true,
			Extensions: rootExtensions,
		}
	}

	// 2. Update Immediate Subdirectories (Depth 1)
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == ".git" {
			continue
		}

		// Check if ignored by git
		fullPath := filepath.Join(root, entry.Name())
		if ignoreMatcher.Match(fullPath, true) {
			// If gitignored, DO NOT add to YAML. 
			// The runtime scanner will skip it automatically.
			continue
		}

		relPath := entry.Name() // Since we are at root, name is relPath

		// If rule exists, respect it
		if _, exists := cfg.Dirs[relPath]; exists {
			continue
		}

		// Deep scan this specific folder to find all extensions used inside it
		dirExtensions := deepScanExtensions(fullPath, root, ignoreMatcher)

		// Create the rule
		cfg.Dirs[relPath] = DirRule{
			Enabled:    true,
			Extensions: dirExtensions,
		}
	}

	return &cfg, nil}

// deepScanExtensions recursively walks a directory to find all unique file extensions
// visible (not ignored by git).
func deepScanExtensions(startPath, rootPath string, matcher gitignore.IgnoreMatcher) []string {
	extMap := make(map[string]bool)

	filepath.WalkDir(startPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // ignore errors
		}

		// Skip .git
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}

		// Check Gitignore
		if matcher.Match(path, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if !d.IsDir() {
			ext := filepath.Ext(d.Name())
			if len(ext) > 1 {
				cleanExt := strings.TrimPrefix(ext, ".")
				extMap[cleanExt] = true
			}
		}
		return nil
	})

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
