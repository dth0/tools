package main

import (
	"path/filepath"
	"strings"
)

// Nerd Font icons, shown only with --icons / REPO_ICONS=1 because they render
// as boxes without a patched font. Codepoints follow the Nerd Fonts v3 set
// used by eza and lsd.

type icon struct {
	glyph string
	color string // hex; empty means the default foreground
}

var (
	iconDir     = icon{"", "#7AA2F7"}
	iconFile    = icon{"", ""}
	iconGitRepo = icon{"", "#F1502F"}
)

var iconByName = map[string]icon{
	".gitignore":         {"", "#F1502F"},
	".gitattributes":     {"", "#F1502F"},
	".gitmodules":        {"", "#F1502F"},
	"dockerfile":         {"", "#2496ED"},
	"docker-compose.yml": {"", "#2496ED"},
	"compose.yaml":       {"", "#2496ED"},
	"makefile":           {"", "#6D8086"},
	"justfile":           {"", "#6D8086"},
	"license":            {"", "#CBCB41"},
	"license.md":         {"", "#CBCB41"},
	"go.mod":             {"", "#00ADD8"},
	"go.sum":             {"", "#00ADD8"},
	"cargo.toml":         {"", "#DEA584"},
	"cargo.lock":         {"", "#DEA584"},
	"package.json":       {"", "#E8274B"},
	"flake.nix":          {"", "#7EBAE4"},
	"flake.lock":         {"", "#7EBAE4"},
	".github":            {"", ""},
	"node_modules":       {"", "#E8274B"},
}

var iconByExt = map[string]icon{
	".go":    {"", "#00ADD8"},
	".rs":    {"", "#DEA584"},
	".py":    {"", "#FFBC03"},
	".js":    {"", "#F1E05A"},
	".mjs":   {"", "#F1E05A"},
	".ts":    {"", "#3178C6"},
	".tsx":   {"", "#3178C6"},
	".jsx":   {"", "#61DAFB"},
	".java":  {"", "#B07219"},
	".c":     {"", "#599EFF"},
	".h":     {"", "#A074C4"},
	".cpp":   {"", "#F34B7D"},
	".rb":    {"", "#CC342D"},
	".lua":   {"", "#51A0CF"},
	".nix":   {"", "#7EBAE4"},
	".sh":    {"", "#89E051"},
	".bash":  {"", "#89E051"},
	".zsh":   {"", "#89E051"},
	".md":    {"", "#519ABA"},
	".json":  {"", "#CBCB41"},
	".yaml":  {"", "#6D8086"},
	".yml":   {"", "#6D8086"},
	".toml":  {"", "#6D8086"},
	".ini":   {"", "#6D8086"},
	".html":  {"", "#E34C26"},
	".css":   {"", "#563D7C"},
	".scss":  {"", "#C6538C"},
	".sql":   {"", "#DAD8D8"},
	".txt":   {"", ""},
	".lock":  {"", "#BBBBBB"},
	".png":   {"", "#A074C4"},
	".jpg":   {"", "#A074C4"},
	".jpeg":  {"", "#A074C4"},
	".gif":   {"", "#A074C4"},
	".svg":   {"", "#FFB13B"},
	".pdf":   {"", "#B30B00"},
	".zip":   {"", "#ECA517"},
	".gz":    {"", "#ECA517"},
	".tf":    {"", "#5F43E9"},
	".proto": {"", ""},
}

func fileIcon(name string, dir bool) icon {
	lower := strings.ToLower(name)
	if ic, ok := iconByName[lower]; ok {
		return ic
	}
	if dir {
		return iconDir
	}
	if ic, ok := iconByExt[filepath.Ext(lower)]; ok {
		return ic
	}
	return iconFile
}

// hostIcon picks an icon for a list entry from its first path segment.
func hostIcon(rel string, git bool) icon {
	host, _, _ := strings.Cut(rel, string(filepath.Separator))
	switch {
	case strings.Contains(host, "github"):
		return icon{"", ""}
	case strings.Contains(host, "gitlab"):
		return icon{"", "#FC6D26"}
	case strings.Contains(host, "bitbucket"):
		return icon{"", "#2684FF"}
	case git:
		return iconGitRepo
	default:
		return iconDir
	}
}
