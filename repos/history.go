package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// History is a plain list of absolute repository paths, most recent first,
// stored at $XDG_STATE_HOME/repos/history (default ~/.local/state/repos/history).
// Errors are deliberately ignored: history is a convenience.

const historyLimit = 500

func historyFile() string {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "repos", "history")
}

func loadHistory() []string {
	f, err := os.Open(historyFile())
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// recordHistory moves path to the front of the history.
func recordHistory(path string) {
	writeHistory(append([]string{path}, without(loadHistory(), path)...))
}

func forgetHistory(path string) {
	writeHistory(without(loadHistory(), path))
}

func without(list []string, path string) []string {
	out := list[:0]
	for _, p := range list {
		if p != path {
			out = append(out, p)
		}
	}
	return out
}

func writeHistory(paths []string) {
	file := historyFile()
	if file == "" {
		return
	}
	if len(paths) > historyLimit {
		paths = paths[:historyLimit]
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return
	}
	tmp := file + ".tmp"
	data := strings.Join(paths, "\n")
	if data != "" {
		data += "\n"
	}
	if os.WriteFile(tmp, []byte(data), 0o644) == nil {
		_ = os.Rename(tmp, file)
	}
}
