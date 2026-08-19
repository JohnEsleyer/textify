package config

import (
	"path/filepath"

	"github.com/monochromegane/go-gitignore"
)

// GitIgnoreChain holds a hierarchy of gitignore matchers from root down to a subfolder.
type GitIgnoreChain []gitignore.IgnoreMatcher

// NewGitIgnoreChain initializes a GitIgnoreChain with the root .gitignore if present.
func NewGitIgnoreChain(root string) GitIgnoreChain {
	var chain GitIgnoreChain
	rootGitignore := filepath.Join(root, ".gitignore")
	if matcher, err := gitignore.NewGitIgnore(rootGitignore); err == nil {
		chain = append(chain, matcher)
	}
	return chain
}

// AddIfExists checks if the directory contains a .gitignore file, and if so,
// returns a new GitIgnoreChain with the new matcher appended.
func (c GitIgnoreChain) AddIfExists(dir string) GitIgnoreChain {
	gitignorePath := filepath.Join(dir, ".gitignore")
	if matcher, err := gitignore.NewGitIgnore(gitignorePath); err == nil {
		newChain := make(GitIgnoreChain, len(c), len(c)+1)
		copy(newChain, c)
		return append(newChain, matcher)
	}
	return c
}

// Match checks if any matcher in the chain matches the given path.
func (c GitIgnoreChain) Match(path string, isDir bool) bool {
	for _, matcher := range c {
		if matcher.Match(path, isDir) {
			return true
		}
	}
	return false
}
