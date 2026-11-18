package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"
)

// pick runs the interactive picker and prints the chosen path on stdout.
func pick(root string, depth int, icons bool) error {
	rels, err := listRepos(root)
	if err != nil {
		return err
	}
	if len(rels) == 0 {
		return fmt.Errorf("no repositories found in %s (try `repo clone <url>`)", root)
	}

	// stdout is captured by the shell function, so draw on the terminal.
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("repo needs a terminal: %w", err)
	}
	defer tty.Close()

	r := lipgloss.NewRenderer(tty)
	lipgloss.SetDefaultRenderer(r)
	r.HasDarkBackground() // query the terminal now, before it enters raw mode

	m := newModel(root, depth, orderByHistory(root, rels), r, icons, tty)
	res, err := tea.NewProgram(m, tea.WithInput(tty), tea.WithOutput(tty), tea.WithAltScreen()).Run()
	if err != nil {
		return err
	}
	chosen := res.(*model).chosen
	if chosen == "" {
		os.Exit(exitCancelled)
	}
	recordHistory(chosen)
	fmt.Println(chosen)
	return nil
}

type item struct {
	rel  string // relative to root, used for display and matching
	path string // absolute
	git  bool
}

// orderByHistory puts recently used repos first, the rest alphabetically.
func orderByHistory(root string, rels []string) []item {
	byPath := make(map[string]item, len(rels))
	for _, rel := range rels {
		p := filepath.Join(root, rel)
		byPath[p] = item{rel: rel, path: p, git: isRepo(p)}
	}
	items := make([]item, 0, len(rels))
	for _, p := range loadHistory() {
		if it, ok := byPath[p]; ok {
			items = append(items, it)
			delete(byPath, p)
		}
	}
	for _, rel := range rels {
		if it, ok := byPath[filepath.Join(root, rel)]; ok {
			items = append(items, it)
		}
	}
	return items
}

type match struct {
	item int   // index into model.items
	hl   []int // byte offsets of matched characters in item.rel
}

type tab int

const (
	tabInfo tab = iota
	tabTree
)

type model struct {
	root  string
	depth int
	tty   *os.File
	r     *lipgloss.Renderer
	st    styles
	icons bool

	items   []item
	matches []match
	status  map[string]gitStatus
	info    map[string]repoInfo
	loading map[string]bool
	readme  map[string][]string // rendered README lines by "path@width"

	input          textinput.Model
	host           string // ctrl-t host filter; "" shows every host
	cursor, offset int
	width, height  int
	tab            tab

	confirm *item // pending delete
	flash   string
	flashID int

	chosen string
}

type (
	statusMsg struct {
		path string
		st   gitStatus
	}
	infoMsg struct {
		path string
		info repoInfo
	}
	flashMsg      struct{ text string }
	clearFlashMsg struct{ id int }
	editorMsg     struct {
		path string
		err  error
	}
	deletedMsg struct {
		path string
		err  error
	}
)

func newModel(root string, depth int, items []item, r *lipgloss.Renderer, icons bool, tty *os.File) *model {
	st := newStyles(r)
	in := textinput.New()
	in.Prompt = "❯ "
	in.PromptStyle = st.accent
	in.Placeholder = "search repositories"
	in.PlaceholderStyle = st.faint
	in.Focus()

	m := &model{
		root: root, depth: depth, tty: tty, r: r, st: st, icons: icons,
		items:   items,
		status:  map[string]gitStatus{},
		info:    map[string]repoInfo{},
		loading: map[string]bool{},
		readme:  map[string][]string{},
		input:   in,
	}
	m.filter()
	return m
}

func (m *model) Init() tea.Cmd {
	// Load git status for every repo in the background, a few at a time.
	sem := make(chan struct{}, 8)
	cmds := []tea.Cmd{textinput.Blink, m.loadInfo()}
	for _, it := range m.items {
		if !it.git {
			continue
		}
		path := it.path
		cmds = append(cmds, func() tea.Msg {
			sem <- struct{}{}
			defer func() { <-sem }()
			return statusMsg{path, loadStatus(path)}
		})
	}
	return tea.Batch(cmds...)
}

// hostOf returns the host segment of a repo path ("github.com" for
// "github.com/user/proj"), or "" for folders not laid out by host.
func hostOf(rel string) string {
	host, _, found := strings.Cut(rel, string(filepath.Separator))
	if !found || !strings.Contains(host, ".") {
		return ""
	}
	return host
}

// hosts lists the hosts present, the ones with most repos first.
func (m *model) hosts() []string {
	count := map[string]int{}
	for _, it := range m.items {
		if h := hostOf(it.rel); h != "" {
			count[h]++
		}
	}
	hosts := make([]string, 0, len(count))
	for h := range count {
		hosts = append(hosts, h)
	}
	sort.Slice(hosts, func(i, j int) bool {
		if count[hosts[i]] != count[hosts[j]] {
			return count[hosts[i]] > count[hosts[j]]
		}
		return hosts[i] < hosts[j]
	})
	return hosts
}

// cycleHost moves the ctrl-t filter to the next host, wrapping back to all.
func (m *model) cycleHost() {
	hosts := m.hosts()
	next := ""
	for i, h := range hosts {
		if h == m.host {
			if i+1 < len(hosts) {
				next = hosts[i+1]
			}
			break
		}
	}
	if m.host == "" && len(hosts) > 0 {
		next = hosts[0]
	}
	m.host = next
}

// splitQuery separates "@host" terms from the path query.
func splitQuery(q string) (hostTerms []string, path string) {
	var rest []string
	for _, f := range strings.Fields(q) {
		if strings.HasPrefix(f, "@") {
			if len(f) > 1 {
				hostTerms = append(hostTerms, f[1:])
			}
		} else {
			rest = append(rest, f)
		}
	}
	return hostTerms, strings.Join(rest, " ")
}

func (m *model) filter() {
	hostTerms, q := splitQuery(m.input.Value())

	// Decide once per distinct host whether it passes the host filters.
	hostOK := map[string]bool{}
	allowed := func(rel string) bool {
		h := hostOf(rel)
		if ok, seen := hostOK[h]; seen {
			return ok
		}
		ok := m.host == "" || h == m.host
		for _, t := range hostTerms {
			ok = ok && h != "" && len(fuzzy.Find(t, []string{h})) > 0
		}
		hostOK[h] = ok
		return ok
	}

	var idx []int // candidate items, in history order
	var rels []string
	for i, it := range m.items {
		if allowed(it.rel) {
			idx = append(idx, i)
			rels = append(rels, it.rel)
		}
	}

	m.matches = m.matches[:0]
	if q == "" {
		for _, i := range idx {
			m.matches = append(m.matches, match{item: i})
		}
	} else {
		// Ties keep the input order, so recently used repos win.
		for _, fm := range fuzzy.Find(q, rels) {
			m.matches = append(m.matches, match{item: idx[fm.Index], hl: fm.MatchedIndexes})
		}
	}
	m.cursor, m.offset = 0, 0
}

func (m *model) current() (item, bool) {
	if m.cursor < 0 || m.cursor >= len(m.matches) {
		return item{}, false
	}
	return m.items[m.matches[m.cursor].item], true
}

func (m *model) loadInfo() tea.Cmd {
	it, ok := m.current()
	if !ok || m.loading[it.path] {
		return nil
	}
	if _, ok := m.info[it.path]; ok {
		return nil
	}
	m.loading[it.path] = true
	depth, label := m.depth, m.treeLabel
	return func() tea.Msg {
		return infoMsg{it.path, loadInfo(it.path, depth, label)}
	}
}

func (m *model) move(delta int) tea.Cmd {
	m.cursor = max(0, min(len(m.matches)-1, m.cursor+delta))
	return m.loadInfo()
}

func (m *model) setFlash(text string) tea.Cmd {
	m.flash = text
	m.flashID++
	id := m.flashID
	return tea.Tick(2500*time.Millisecond, func(time.Time) tea.Msg { return clearFlashMsg{id} })
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case statusMsg:
		m.status[msg.path] = msg.st
		return m, nil

	case infoMsg:
		m.info[msg.path] = msg.info
		delete(m.loading, msg.path)
		return m, nil

	case flashMsg:
		return m, m.setFlash(msg.text)

	case clearFlashMsg:
		if msg.id == m.flashID {
			m.flash = ""
		}
		return m, nil

	case editorMsg:
		if msg.err != nil {
			return m, m.setFlash(m.st.red.Render("editor: " + msg.err.Error()))
		}
		m.chosen = msg.path
		return m, tea.Quit

	case deletedMsg:
		if msg.err != nil {
			return m, m.setFlash(m.st.red.Render("delete failed: " + msg.err.Error()))
		}
		m.removeItem(msg.path)
		rel, _ := filepath.Rel(m.root, msg.path)
		return m, tea.Batch(m.setFlash("deleted "+rel), m.loadInfo())

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.confirm != nil {
		it := *m.confirm
		m.confirm = nil
		if msg.String() == "y" || msg.String() == "Y" {
			root := m.root
			return m, func() tea.Msg {
				err := os.RemoveAll(it.path)
				if err == nil {
					removeEmptyParents(filepath.Dir(it.path), root)
					forgetHistory(it.path)
				}
				return deletedMsg{it.path, err}
			}
		}
		return m, nil
	}

	switch msg.String() {
	case "ctrl+c", "esc":
		return m, tea.Quit
	case "enter":
		if it, ok := m.current(); ok {
			m.chosen = it.path
			return m, tea.Quit
		}
		return m, nil
	case "up", "ctrl+p", "ctrl+k":
		return m, m.move(-1)
	case "down", "ctrl+n", "ctrl+j":
		return m, m.move(1)
	case "pgup":
		return m, m.move(-m.listHeight())
	case "pgdown":
		return m, m.move(m.listHeight())
	case "home":
		return m, m.move(-len(m.matches))
	case "end":
		return m, m.move(len(m.matches))
	case "tab", "shift+tab":
		m.tab = 1 - m.tab
		return m, nil
	case "ctrl+t":
		m.cycleHost()
		m.filter()
		return m, m.loadInfo()
	case "ctrl+o":
		it, ok := m.current()
		if !ok {
			return m, nil
		}
		recordHistory(it.path)
		return m, tea.ExecProcess(editorCommand(it.path, m.tty), func(err error) tea.Msg {
			return editorMsg{it.path, err}
		})
	case "ctrl+b":
		return m, m.browse()
	case "ctrl+y":
		it, ok := m.current()
		if !ok {
			return m, nil
		}
		tty := m.tty
		return m, func() tea.Msg {
			if err := copyToClipboard(it.path, tty); err != nil {
				return flashMsg{m.st.red.Render("copy failed: " + err.Error())}
			}
			return flashMsg{"copied " + tildify(it.path)}
		}
	case "ctrl+d":
		if it, ok := m.current(); ok {
			m.confirm = &it
		}
		return m, nil
	}

	prev := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != prev {
		m.filter()
		return m, tea.Batch(cmd, m.loadInfo())
	}
	return m, cmd
}

func (m *model) browse() tea.Cmd {
	it, ok := m.current()
	if !ok {
		return nil
	}
	remote := m.info[it.path].Remote
	return func() tea.Msg {
		if remote == "" && it.git {
			remote, _ = git(it.path, "remote", "get-url", "origin")
		}
		url := webURL(remote)
		if url == "" {
			return flashMsg{m.st.yellow.Render("no remote to open")}
		}
		if err := openBrowser(url); err != nil {
			return flashMsg{m.st.red.Render("open failed: " + err.Error())}
		}
		return flashMsg{"opened " + url}
	}
}

func (m *model) removeItem(path string) {
	for i, it := range m.items {
		if it.path == path {
			m.items = append(m.items[:i], m.items[i+1:]...)
			break
		}
	}
	cursor := m.cursor
	m.filter()
	m.cursor = max(0, min(len(m.matches)-1, cursor))
}

// --- view -------------------------------------------------------------------

func (m *model) listHeight() int { return max(1, m.height-3) }

func (m *model) View() string {
	if m.width == 0 {
		return ""
	}
	header := m.viewHeader()
	footer := m.viewFooter()
	bodyH := m.listHeight()

	listW := m.width
	showPreview := m.width >= 80
	if showPreview {
		listW = m.width * 45 / 100
	}
	list := m.viewList(listW, bodyH)
	body := list
	if showPreview {
		body = lipgloss.JoinHorizontal(lipgloss.Top, list, " ", m.viewPreview(m.width-listW-1, bodyH))
	}
	return header + "\n" + body + "\n\n" + footer
}

func (m *model) viewHeader() string {
	count := m.st.faint.Render(fmt.Sprintf("%d/%d  %s", len(m.matches), len(m.items), tildify(m.root)))
	if m.host != "" {
		pill := m.st.tabActive.Render("@" + m.host)
		if m.icons {
			pill = m.iconText(hostIcon(m.host+string(filepath.Separator), true)) + " " + pill
		}
		count = pill + "  " + count
	}
	m.input.Width = max(10, m.width-lipgloss.Width(count)-4)
	in := m.input.View()
	gap := max(1, m.width-lipgloss.Width(in)-lipgloss.Width(count))
	return ansi.Truncate(in+strings.Repeat(" ", gap)+count, m.width, "")
}

func (m *model) viewFooter() string {
	if m.confirm != nil {
		msg := m.st.red.Render("Delete ") + m.st.bold.Render(tildify(m.confirm.path)) + m.st.red.Render("?")
		if st, ok := m.status[m.confirm.path]; ok && (st.Dirty() || st.Ahead > 0) {
			msg += m.st.yellow.Render(" It has uncommitted or unpushed work.")
		}
		return ansi.Truncate(msg+m.st.faint.Render("  [y/N]"), m.width, "…")
	}
	if m.flash != "" {
		return ansi.Truncate(m.st.green.Render("● ")+m.flash, m.width, "…")
	}
	keys := [][2]string{
		{"↵", "cd"}, {"^o", "edit"}, {"^b", "browse"}, {"^y", "copy"},
		{"^d", "delete"}, {"^t", "host"}, {"tab", "info/tree"}, {"esc", "quit"},
	}
	var parts []string
	for _, k := range keys {
		parts = append(parts, m.st.accent.Render(k[0])+" "+m.st.faint.Render(k[1]))
	}
	return ansi.Truncate(strings.Join(parts, m.st.faint.Render("  ·  ")), m.width, "…")
}

func (m *model) viewList(width, height int) string {
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+height {
		m.offset = m.cursor - height + 1
	}
	lines := make([]string, 0, height)
	if len(m.matches) == 0 {
		lines = append(lines, m.st.faint.Render("  no matches"))
	}
	for i := m.offset; i < len(m.matches) && len(lines) < height; i++ {
		lines = append(lines, m.viewRow(m.matches[i], i == m.cursor, width))
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(strings.Join(lines, "\n"))
}

func (m *model) viewRow(mt match, selected bool, width int) string {
	it := m.items[mt.item]
	marker := "  "
	if selected {
		marker = m.st.accent.Render("▌ ")
	}
	if m.icons {
		marker += m.iconText(hostIcon(it.rel, it.git)) + " "
	}

	right := m.statusBadge(it)
	name := m.highlight(it.rel, mt.hl, selected)
	avail := width - lipgloss.Width(marker) - lipgloss.Width(right) - 1
	if avail < 8 {
		right, avail = "", width-lipgloss.Width(marker)
	}
	name = ansi.Truncate(name, avail, "…")
	gap := max(1, width-lipgloss.Width(marker)-lipgloss.Width(name)-lipgloss.Width(right))
	return marker + name + strings.Repeat(" ", gap) + right
}

// highlight dims the host/owner prefix, emphasises the repo name and marks
// fuzzy-matched characters.
func (m *model) highlight(rel string, hl []int, selected bool) string {
	hit := make(map[int]bool, len(hl))
	for _, i := range hl {
		hit[i] = true
	}
	split := strings.LastIndexByte(rel, '/') + 1
	base := m.st.bold
	if selected {
		base = m.st.bold.Foreground(colAccent)
	}
	// Style classes: 0 matched, 1 host/owner prefix, 2 repo name.
	classStyles := [3]lipgloss.Style{m.st.matched, m.st.faint, base}
	classOf := func(i int) int {
		switch {
		case hit[i]:
			return 0
		case i < split:
			return 1
		default:
			return 2
		}
	}

	var b strings.Builder
	runStart, runClass := 0, -1
	for i := range rel { // byte offsets, matching fuzzy's MatchedIndexes
		if c := classOf(i); c != runClass {
			if runClass >= 0 {
				b.WriteString(classStyles[runClass].Render(rel[runStart:i]))
			}
			runStart, runClass = i, c
		}
	}
	if runClass >= 0 {
		b.WriteString(classStyles[runClass].Render(rel[runStart:]))
	}
	return b.String()
}

func (m *model) statusBadge(it item) string {
	if !it.git {
		return ""
	}
	st, ok := m.status[it.path]
	if !ok {
		return m.st.faint.Render("…")
	}
	if st.Err != nil {
		return m.st.red.Render("!")
	}
	var parts []string
	if st.Ahead > 0 {
		parts = append(parts, m.st.green.Render(fmt.Sprintf("↑%d", st.Ahead)))
	}
	if st.Behind > 0 {
		parts = append(parts, m.st.red.Render(fmt.Sprintf("↓%d", st.Behind)))
	}
	if st.Dirty() {
		parts = append(parts, m.st.yellow.Render("●"))
	}
	branch := st.Branch
	if len(branch) > 18 {
		branch = branch[:17] + "…"
	}
	parts = append(parts, m.st.faint.Render(branch))
	return strings.Join(parts, " ")
}

func (m *model) viewPreview(width, height int) string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colBorder).
		Padding(0, 1).
		Width(width - 2).
		Height(height - 2)
	innerW, innerH := width-4, height-2

	it, ok := m.current()
	if !ok {
		return box.Render("")
	}

	tabs := []string{"Info", "Tree"}
	var tabLine []string
	for i, t := range tabs {
		if tab(i) == m.tab {
			tabLine = append(tabLine, m.st.tabActive.Render(t))
		} else {
			tabLine = append(tabLine, m.st.tabIdle.Render(t))
		}
	}
	lines := []string{strings.Join(tabLine, m.st.faint.Render(" │ ")), ""}

	info, loaded := m.info[it.path]
	switch {
	case !loaded:
		lines = append(lines, m.st.faint.Render("loading…"))
	case m.tab == tabTree:
		lines = append(lines, strings.Split(strings.TrimRight(info.Tree, "\n"), "\n")...)
	default:
		lines = append(lines, m.infoLines(it, info, innerW)...)
	}

	if len(lines) > innerH {
		lines = lines[:innerH]
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, innerW, "…")
	}
	return box.Render(strings.Join(lines, "\n"))
}

func (m *model) infoLines(it item, info repoInfo, width int) []string {
	s := m.st
	lines := []string{s.bold.Render(it.rel)}

	if !it.git {
		return append(lines, s.faint.Render("not a git repository — press tab for the file tree"))
	}

	if st, ok := m.status[it.path]; !ok {
		lines = append(lines, s.faint.Render("reading status…"))
	} else if st.Err != nil {
		lines = append(lines, s.red.Render("git status failed: "+errText(st.Err)))
	} else {
		parts := []string{s.accent.Render(" " + st.Branch)}
		switch {
		case !st.Upstream:
			parts = append(parts, s.faint.Render("no upstream"))
		case st.Ahead == 0 && st.Behind == 0:
			parts = append(parts, s.faint.Render("up to date"))
		default:
			if st.Ahead > 0 {
				parts = append(parts, s.green.Render(fmt.Sprintf("↑%d ahead", st.Ahead)))
			}
			if st.Behind > 0 {
				parts = append(parts, s.red.Render(fmt.Sprintf("↓%d behind", st.Behind)))
			}
		}
		if st.Dirty() {
			parts = append(parts, s.yellow.Render(fmt.Sprintf("● %d changed", len(st.Changes))))
		} else {
			parts = append(parts, s.green.Render("✓ clean"))
		}
		lines = append(lines, strings.Join(parts, s.faint.Render("  ·  ")))
	}
	if info.Remote != "" {
		lines = append(lines, s.faint.Render(info.Remote))
	}

	if st, ok := m.status[it.path]; ok && st.Dirty() {
		lines = append(lines, "", s.bold.Render("Changes"))
		for i, c := range st.Changes {
			if i == 8 {
				lines = append(lines, s.faint.Render(fmt.Sprintf("  … %d more", len(st.Changes)-i)))
				break
			}
			lines = append(lines, "  "+m.changeLine(c))
		}
	}

	if len(info.Commits) > 0 {
		lines = append(lines, "", s.bold.Render("Recent commits"))
		for _, c := range info.Commits {
			lines = append(lines, "  "+s.yellow.Render(c.Hash)+" "+s.faint.Render(fmt.Sprintf("%-4s", shortWhen(c.When)))+" "+c.Subject)
		}
	}

	if len(info.Readme) > 0 {
		lines = append(lines, "", s.bold.Render("README"))
		lines = append(lines, m.readmeLines(it.path, info, width)...)
	}
	return lines
}

// readmeLines renders the README for the given width, caching the result
// since View runs on every keystroke.
func (m *model) readmeLines(path string, info repoInfo, width int) []string {
	key := fmt.Sprintf("%s@%d", path, width)
	if lines, ok := m.readme[key]; ok {
		return lines
	}
	var lines []string
	if info.ReadmeMD {
		src := strings.Join(info.Readme, "\n")
		if out, err := renderMarkdown(src, width, m.r.HasDarkBackground(), m.r.ColorProfile()); err == nil {
			lines = strings.Split(out, "\n")
		}
	}
	if lines == nil { // plain text, or glamour failed
		for _, l := range info.Readme {
			if strings.HasPrefix(l, "#") {
				lines = append(lines, m.st.accent.Render(strings.TrimLeft(l, "# ")))
			} else {
				lines = append(lines, m.st.faint.Render(l))
			}
		}
	}
	m.readme[key] = lines
	return lines
}

// treeLabel decorates a file tree entry with colour and, optionally, an icon.
func (m *model) treeLabel(name string, isDir bool) string {
	label := name
	if isDir {
		label = m.st.accent.Render(name + "/")
	}
	if !m.icons {
		return label
	}
	return m.iconText(fileIcon(name, isDir)) + " " + label
}

func (m *model) iconText(ic icon) string {
	if ic.color == "" {
		return ic.glyph
	}
	return m.r.NewStyle().Foreground(lipgloss.Color(ic.color)).Render(ic.glyph)
}

func (m *model) changeLine(c string) string {
	code, path := c[:2], c[3:]
	var st lipgloss.Style
	switch {
	case code == "??":
		st = m.st.faint
	case strings.ContainsAny(code, "U"):
		st = m.st.red
	case strings.ContainsAny(code, "D"):
		st = m.st.red
	case strings.ContainsAny(code, "A"):
		st = m.st.green
	default:
		st = m.st.yellow
	}
	return st.Render(code) + " " + path
}

func errText(err error) string {
	var ee interface{ ExitCode() int }
	if errors.As(err, &ee) {
		return fmt.Sprintf("exit %d", ee.ExitCode())
	}
	return err.Error()
}
