package scanner

import (
	"sort"
	"strings"
)

type treeNode struct {
	name     string
	isDir    bool
	children map[string]*treeNode
}

func newTreeNode(name string, isDir bool) *treeNode {
	return &treeNode{
		name:     name,
		isDir:    isDir,
		children: make(map[string]*treeNode),
	}
}

// GenerateTree renders an ASCII directory tree of the relative paths.
func GenerateTree(paths []string) string {
	if len(paths) == 0 {
		return ""
	}

	root := newTreeNode(".", true)

	for _, p := range paths {
		parts := strings.Split(p, "/")
		curr := root
		for i, part := range parts {
			if part == "" || part == "." {
				continue
			}
			isDir := i < len(parts)-1
			if _, exists := curr.children[part]; !exists {
				curr.children[part] = newTreeNode(part, isDir)
			}
			curr = curr.children[part]
		}
	}

	var sb strings.Builder
	sb.WriteString("PROJECT STRUCTURE:\n")
	renderNode(root, "", true, &sb, true)
	sb.WriteString("\n")
	return sb.String()
}

func renderNode(node *treeNode, prefix string, isLast bool, sb *strings.Builder, isRoot bool) {
	if !isRoot {
		connector := "├── "
		if isLast {
			connector = "└── "
		}
		sb.WriteString(prefix + connector + node.name)
		if node.isDir {
			sb.WriteString("/")
		}
		sb.WriteString("\n")

		if isLast {
			prefix += "    "
		} else {
			prefix += "│   "
		}
	}

	var names []string
	for name := range node.children {
		names = append(names, name)
	}

	sort.Slice(names, func(i, j int) bool {
		childA := node.children[names[i]]
		childB := node.children[names[j]]
		// Directories first, then alphabetical
		if childA.isDir != childB.isDir {
			return childA.isDir
		}
		return names[i] < names[j]
	})

	for i, name := range names {
		isChildLast := i == len(names)-1
		renderNode(node.children[name], prefix, isChildLast, sb, false)
	}
}
