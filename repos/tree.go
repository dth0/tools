package main

import (
	"os"
	"path/filepath"
	"strings"
)

// renderTree draws a `tree`-style listing of dir, limited to maxDepth levels
// and maxLines lines so large repos stay fast to preview. label decorates each
// entry name (colour, icon); it receives names without a trailing slash.
func renderTree(dir string, maxDepth, maxLines int, label func(name string, isDir bool) string) string {
	var b strings.Builder
	b.WriteString(label(filepath.Base(dir), true) + "\n")
	lines := 1

	var walk func(path, prefix string, depth int)
	walk = func(path, prefix string, depth int) {
		if depth > maxDepth || lines >= maxLines {
			return
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return
		}
		entries = visible(entries)
		for i, e := range entries {
			if lines >= maxLines {
				b.WriteString(prefix + "…\n")
				return
			}
			branch, next := "├── ", "│   "
			if i == len(entries)-1 {
				branch, next = "└── ", "    "
			}
			child := filepath.Join(path, e.Name())
			isDir := isDir(child, e)
			b.WriteString(prefix + branch + label(e.Name(), isDir) + "\n")
			lines++
			if isDir {
				walk(child, prefix+next, depth+1)
			}
		}
	}
	walk(dir, "", 1)
	return b.String()
}

func visible(entries []os.DirEntry) []os.DirEntry {
	out := entries[:0]
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e)
		}
	}
	return out
}
