package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// repoPath maps a git remote to a relative path of the form host/owner/repo:
//
//	https://github.com/user/proj        → github.com/user/proj
//	git@github.com:user/proj.git        → github.com/user/proj
//	ssh://git@host:2222/group/sub/proj  → host/group/sub/proj
func repoPath(url string) (string, error) {
	var host, path string
	if i := strings.Index(url, "://"); i >= 0 {
		// scheme://[user@]host[:port]/path
		rest := url[i+3:]
		slash := strings.IndexByte(rest, '/')
		if slash < 0 {
			return "", fmt.Errorf("no repository path in %q", url)
		}
		host, path = rest[:slash], rest[slash+1:]
		if at := strings.LastIndexByte(host, '@'); at >= 0 {
			host = host[at+1:]
		}
		if c := strings.LastIndexByte(host, ':'); c >= 0 && !strings.HasSuffix(host, "]") {
			host = host[:c] // drop port
		}
	} else if c := strings.IndexByte(url, ':'); c > 0 && !strings.ContainsRune(url[:c], '/') {
		// scp-like: [user@]host:path
		host, path = url[:c], url[c+1:]
		if at := strings.LastIndexByte(host, '@'); at >= 0 {
			host = host[at+1:]
		}
	} else {
		return "", fmt.Errorf("cannot derive host/owner/repo from %q; pass a path explicitly", url)
	}

	path = strings.Trim(path, "/")
	path = strings.TrimRight(strings.TrimSuffix(path, ".git"), "/")
	if host == "" || path == "" {
		return "", fmt.Errorf("cannot derive host/owner/repo from %q; pass a path explicitly", url)
	}
	return cleanRel(host + "/" + path)
}

// cleanRel validates a slash-separated relative path so it can't escape the
// base directory.
func cleanRel(p string) (string, error) {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.HasPrefix(part, "~") {
			return "", fmt.Errorf("invalid repository path %q", p)
		}
	}
	return filepath.Join(parts...), nil
}

// maxScanDepth bounds how deep listRepos looks below the base directory
// (host/owner/repo is 3; GitLab subgroups can go deeper).
const maxScanDepth = 5

// listRepos returns the paths (relative to root) of git repositories below
// root, e.g. "github.com/user/proj". It doesn't descend into a repository or
// hidden directories. Top-level directories that aren't git repositories and
// contain none are still listed, so plain folders in root stay reachable.
func listRepos(root string) ([]string, error) {
	var names []string
	var walk func(dir, rel string, depth int) (bool, error)
	walk = func(dir, rel string, depth int) (bool, error) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return false, err
		}
		found := false
		for _, e := range entries {
			name := e.Name()
			child := filepath.Join(dir, name)
			if strings.HasPrefix(name, ".") || !isDir(child, e) {
				continue
			}
			childRel := filepath.Join(rel, name)
			if isRepo(child) {
				names = append(names, childRel)
				found = true
				continue
			}
			sub := false
			if depth < maxScanDepth {
				sub, _ = walk(child, childRel, depth+1)
			}
			if sub {
				found = true
			} else if depth == 1 {
				names = append(names, childRel)
			}
		}
		return found, nil
	}
	if _, err := walk(root, "", 1); err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

func isRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

func isDir(path string, e os.DirEntry) bool {
	if e.IsDir() {
		return true
	}
	if e.Type()&os.ModeSymlink != 0 {
		fi, err := os.Stat(path)
		return err == nil && fi.IsDir()
	}
	return false
}

// removeEmptyParents removes dir and its ancestors while they are empty,
// stopping at (and never removing) root. Used to tidy up host/owner folders
// after a failed clone or a delete.
func removeEmptyParents(dir, root string) {
	for {
		rel, err := filepath.Rel(root, dir)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return
		}
		if os.Remove(dir) != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}
