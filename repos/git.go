package main

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const gitTimeout = 5 * time.Second

// git runs a read-only git command in dir and returns its trimmed stdout.
func git(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	// Don't take index.lock for status refreshes, so we never block (or are
	// blocked by) a git command the user is running in that repo.
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	out, err := cmd.Output()
	return strings.TrimRight(string(out), "\n"), err
}

type gitStatus struct {
	Branch        string
	Upstream      bool
	Ahead, Behind int
	Changes       []string // "XY path", like `git status --short`
	Err           error
}

func (s gitStatus) Dirty() bool { return len(s.Changes) > 0 }

func loadStatus(dir string) gitStatus {
	out, err := git(dir, "status", "--porcelain=v2", "--branch")
	if err != nil {
		return gitStatus{Err: err}
	}
	return parseStatus(out)
}

// parseStatus parses `git status --porcelain=v2 --branch` output.
func parseStatus(out string) gitStatus {
	var st gitStatus
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "# branch.head "):
			st.Branch = strings.TrimPrefix(line, "# branch.head ")
		case strings.HasPrefix(line, "# branch.upstream "):
			st.Upstream = true
		case strings.HasPrefix(line, "# branch.ab "):
			for _, f := range strings.Fields(strings.TrimPrefix(line, "# branch.ab ")) {
				n, _ := strconv.Atoi(f[1:])
				if f[0] == '+' {
					st.Ahead = n
				} else {
					st.Behind = n
				}
			}
		case strings.HasPrefix(line, "1 "):
			if f := strings.SplitN(line, " ", 9); len(f) == 9 {
				st.Changes = append(st.Changes, xy(f[1])+" "+f[8])
			}
		case strings.HasPrefix(line, "2 "):
			if f := strings.SplitN(line, " ", 10); len(f) == 10 {
				path, orig, _ := strings.Cut(f[9], "\t")
				st.Changes = append(st.Changes, xy(f[1])+" "+orig+" → "+path)
			}
		case strings.HasPrefix(line, "u "):
			if f := strings.SplitN(line, " ", 11); len(f) == 11 {
				st.Changes = append(st.Changes, xy(f[1])+" "+f[10])
			}
		case strings.HasPrefix(line, "? "):
			st.Changes = append(st.Changes, "?? "+line[2:])
		}
	}
	return st
}

func xy(s string) string { return strings.ReplaceAll(s, ".", " ") }

type commit struct{ Hash, Subject, When string }

type repoInfo struct {
	Remote   string
	Commits  []commit
	Readme   []string
	ReadmeMD bool // render Readme as Markdown
	Tree     string
}

const readmeLines = 80

func loadInfo(dir string, depth int, label func(string, bool) string) repoInfo {
	info := repoInfo{Tree: renderTree(dir, depth, 300, label)}
	if !isRepo(dir) {
		return info
	}
	info.Remote, _ = git(dir, "remote", "get-url", "origin")
	if out, err := git(dir, "log", "-5", "--format=%h%x1f%s%x1f%cr"); err == nil && out != "" {
		for _, l := range strings.Split(out, "\n") {
			if f := strings.Split(l, "\x1f"); len(f) == 3 {
				info.Commits = append(info.Commits, commit{f[0], f[1], f[2]})
			}
		}
	}
	info.Readme, info.ReadmeMD = readReadme(dir)
	return info
}

func readReadme(dir string) ([]string, bool) {
	for _, name := range []string{"README.md", "README", "README.rst", "README.txt", "readme.md"} {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		defer f.Close()
		var lines []string
		sc := bufio.NewScanner(f)
		for sc.Scan() && len(lines) < readmeLines {
			l := strings.TrimRight(sc.Text(), " \t")
			// Skip HTML and badge images, and collapse blank runs.
			if t := strings.TrimSpace(l); strings.HasPrefix(t, "<") || strings.HasPrefix(t, "[![") || strings.HasPrefix(t, "![") {
				continue
			}
			if l == "" && (len(lines) == 0 || lines[len(lines)-1] == "") {
				continue
			}
			lines = append(lines, strings.ReplaceAll(l, "\t", "    "))
		}
		return lines, strings.HasSuffix(strings.ToLower(name), ".md")
	}
	return nil, false
}

// webURL turns a git remote into a browsable https URL.
func webURL(remote string) string {
	if strings.HasPrefix(remote, "https://") || strings.HasPrefix(remote, "http://") {
		u := strings.TrimSuffix(strings.TrimRight(remote, "/"), ".git")
		// Strip credentials such as https://user:token@host/...
		scheme, rest, _ := strings.Cut(u, "://")
		if at := strings.IndexByte(rest, '@'); at >= 0 && at < strings.IndexByte(rest+"/", '/') {
			rest = rest[at+1:]
		}
		return scheme + "://" + rest
	}
	if p, err := repoPath(remote); err == nil {
		return "https://" + filepath.ToSlash(p)
	}
	return ""
}

// shortWhen compacts git's relative dates: "5 months ago" → "5mo",
// "1 year, 2 months ago" → "1y".
func shortWhen(rel string) string {
	f := strings.Fields(strings.TrimSuffix(rel, " ago"))
	if len(f) < 2 {
		return rel
	}
	unit := strings.TrimSuffix(strings.TrimSuffix(f[1], ","), "s")
	abbr := map[string]string{"second": "s", "minute": "m", "hour": "h", "day": "d", "week": "w", "month": "mo", "year": "y"}[unit]
	if abbr == "" {
		return rel
	}
	return f[0] + abbr
}
