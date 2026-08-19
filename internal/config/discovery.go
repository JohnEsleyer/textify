package config

import (
	"os"
	"path/filepath"
	"strings"
)

// Discover populates the Config.Dirs map by scanning top-level directories and subdirectories.
// It checks for subfolder .gitignore files and adds ignored subfolders to Config.Dirs with enabled: false.
func Discover(root string, existingCfg *Config) (*Config, error) {
	cfg := DefaultConfig()
	if existingCfg != nil {
		cfg = *existingCfg
		if cfg.Dirs == nil {
			cfg.Dirs = make(map[string]DirRule)
		}
	}

	chain := NewGitIgnoreChain(root)

	// 1. Update Root (.) Rule
	rootExtensions := deepScanExtensions(root, root, chain)

	if val, ok := cfg.Dirs["."]; ok {
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

	// 2. Discover subdirectories recursively
	var discoverDirs func(currentPath string, depth int, currentChain GitIgnoreChain) error
	discoverDirs = func(currentPath string, depth int, currentChain GitIgnoreChain) error {
		entries, err := os.ReadDir(currentPath)
		if err != nil {
			return err
		}

		for _, entry := range entries {
			if !entry.IsDir() || entry.Name() == ".git" {
				continue
			}

			fullPath := filepath.Join(currentPath, entry.Name())
			relPath, err := filepath.Rel(root, fullPath)
			if err != nil {
				continue
			}
			relPath = filepath.ToSlash(relPath)

			isIgnored := currentChain.Match(fullPath, true)

			if isIgnored {
				// Ignored directory (by root or subfolder .gitignore)
				if _, exists := cfg.Dirs[relPath]; !exists {
					cfg.Dirs[relPath] = DirRule{
						Enabled: false,
					}
				}
				// Do not descend into ignored directories
				continue
			}

			// Non-ignored top-level subdirectory
			if depth == 1 {
				if _, exists := cfg.Dirs[relPath]; !exists {
					dirChain := currentChain.AddIfExists(fullPath)
					dirExtensions := deepScanExtensions(fullPath, root, dirChain)
					cfg.Dirs[relPath] = DirRule{
						Enabled:    true,
						Extensions: dirExtensions,
					}
				}
			}

			// Recurse into non-ignored subdirectory
			nextChain := currentChain.AddIfExists(fullPath)
			if err := discoverDirs(fullPath, depth+1, nextChain); err != nil {
				return err
			}
		}
		return nil
	}

	if err := discoverDirs(root, 1, chain); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// deepScanExtensions recursively walks a directory to find all unique file extensions
// visible (not ignored by git).
func deepScanExtensions(startPath, rootPath string, chain GitIgnoreChain) []string {
	extMap := make(map[string]bool)

	var walkDir func(currentPath string, currentChain GitIgnoreChain)
	walkDir = func(currentPath string, currentChain GitIgnoreChain) {
		entries, err := os.ReadDir(currentPath)
		if err != nil {
			return
		}

		for _, entry := range entries {
			if entry.Name() == ".git" {
				continue
			}

			entryPath := filepath.Join(currentPath, entry.Name())
			isDir := entry.IsDir()

			if currentChain.Match(entryPath, isDir) {
				continue
			}

			if isDir {
				nextChain := currentChain.AddIfExists(entryPath)
				walkDir(entryPath, nextChain)
			} else {
				ext := filepath.Ext(entry.Name())
				if len(ext) > 1 {
					cleanExt := strings.TrimPrefix(ext, ".")
					extMap[cleanExt] = true
				}
			}
		}
	}

	walkDir(startPath, chain)

	var extensions []string
	for ext := range extMap {
		extensions = append(extensions, ext)
	}
	return extensions
}
