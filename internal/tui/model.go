package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"arbor/internal/gitgraph"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type model struct {
	repoPath string
	provider *gitgraph.CommitProvider
	headName string

	width     int
	height    int
	didLayout bool

	cursor int
	offset int

	showSidebar bool
	showFiles   bool

	searchActive  bool
	searchQuery   string
	filter        string
	filtered      []int
	filterScanned int

	filesCache map[string][]string
	err        error
}

func NewModel(path string, provider *gitgraph.CommitProvider, headName string) tea.Model {
	m := &model{
		repoPath:    path,
		provider:    provider,
		headName:    headName,
		showSidebar: true,
		filesCache:  make(map[string][]string),
	}
	_ = m.provider.Ensure(0)
	return m
}

func (m *model) Init() tea.Cmd {
	return nil
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if !m.didLayout {
			m.cursor = 0
			m.offset = 0
			m.didLayout = true
		}
		m.ensureVisible()
		m.normalizePosition()
		return m, nil
	case tea.KeyMsg:
		if m.searchActive {
			next, cmd := m.handleSearchKey(msg)
			if mm, ok := next.(*model); ok {
				mm.ensureVisible()
				mm.normalizePosition()
			}
			return next, cmd
		}
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			m.moveCursor(-1)
		case "down", "j":
			m.moveCursor(1)
		case "enter":
			m.showFiles = !m.showFiles
		case "/":
			m.searchActive = true
			m.searchQuery = m.filter
			m.normalizePosition()
		case "tab":
			m.showSidebar = !m.showSidebar
		}
		m.ensureVisible()
		m.normalizePosition()
		return m, nil
	}
	return m, nil
}

func (m *model) View() string {
	header := m.headerView(m.width)

	mainWidth := m.width
	sidebarWidth := 0
	if m.showSidebar && m.width >= 60 {
		sidebarWidth = max(30, m.width/3)
		mainWidth = m.width - sidebarWidth - 1
	}

	listView := m.renderList(mainWidth)
	var row string
	if sidebarWidth == 0 {
		row = listView
	} else {
		sidebar := m.renderSidebar(sidebarWidth)
		row = lipgloss.JoinHorizontal(lipgloss.Top, listView, sidebar)
	}

	footer := m.footerView(m.width)
	if m.searchActive {
		return lipgloss.JoinVertical(lipgloss.Left, header, row, footer, m.searchView(m.width))
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, row, footer)
}

func (m *model) renderList(width int) string {
	if width <= 0 {
		return ""
	}
	viewport := m.viewportHeight()
	lines := make([]string, 0, viewport)
	listLen := m.listLength()
	start := min(m.offset, max(0, listLen-1))
	end := min(start+viewport, listLen)

	for i := start; i < end; i++ {
		rowIndex := i
		if m.filter != "" {
			if i >= len(m.filtered) {
				break
			}
			rowIndex = m.filtered[i]
		}
		if rowIndex >= len(m.provider.Commits) {
			break
		}
		commit := m.provider.Commits[rowIndex]
		line := m.renderRow(commit, i == m.cursor, width, i%2 == 1)
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		lines = append(lines, m.emptyRow(width))
	}
	for i := len(lines); i < viewport; i++ {
		rowIndex := start + i
		lines = append(lines, m.blankRow(width, rowIndex%2 == 1))
	}
	return strings.Join(lines, "\n")
}

func (m *model) renderRow(commit *gitgraph.CommitInfo, selected bool, width int, alt bool) string {
	bg := palette.bg
	subjectColor := palette.text
	authorColor := palette.textMuted
	if alt {
		bg = palette.bgAlt
	}
	if selected {
		bg = palette.highlightBg
		subjectColor = palette.highlightText
		authorColor = palette.highlightText
	}

	graph := renderGraph(commit.Graph, bg)
	space := rowSpacerStyle.Background(bg).Render(" ")
	sep := rowSeparatorStyle.Foreground(palette.textDim).Background(bg).Render(" - ")
	hash := hashStyle.Foreground(palette.accent).Background(bg).Render(commit.ShortHash)
	subject := subjectStyle.Foreground(subjectColor).Background(bg).Render(commit.Subject)
	author := authorStyle.Foreground(authorColor).Background(bg).Render(commit.Author)
	meta := hash + space + subject + sep + author
	row := graph + space + meta
	return fitLine(row, width, bg)
}

func (m *model) renderSidebar(width int) string {
	commit := m.selectedCommit()
	if commit == nil {
		return sidebarStyle.Width(width).MaxHeight(m.viewportHeight()).Render("No commit selected")
	}
	lines := []string{
		sidebarTitleStyle.Render(commit.ShortHash),
		commit.Author,
		commit.When.Format(time.RFC1123),
		"",
	}
	message := strings.TrimSpace(commit.Commit.Message)
	lines = append(lines, wrapText(message, width-2)...)

	if m.showFiles {
		lines = append(lines, "", sidebarSubtitleStyle.Render("Changed files"))
		files := m.changedFiles(commit)
		for _, f := range files {
			lines = append(lines, fmt.Sprintf("- %s", f))
		}
	}

	return sidebarStyle.Width(width).MaxHeight(m.viewportHeight()).Render(strings.Join(lines, "\n"))
}

func (m *model) searchView(width int) string {
	if width <= 0 {
		width = m.width
	}
	input := searchStyle.Width(width).Render(fmt.Sprintf("/%s", m.searchQuery))
	return input
}

func (m *model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.searchActive = false
		m.searchQuery = ""
		return m, nil
	case tea.KeyEnter:
		m.searchActive = false
		m.applyFilter(m.searchQuery)
		return m, nil
	case tea.KeyBackspace, tea.KeyDelete:
		if len(m.searchQuery) > 0 {
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
		}
		return m, nil
	}
	if msg.String() == "q" {
		return m, tea.Quit
	}
	if msg.Runes != nil {
		m.searchQuery += string(msg.Runes)
	}
	return m, nil
}

func (m *model) applyFilter(query string) {
	m.filter = strings.TrimSpace(query)
	m.filtered = nil
	m.filterScanned = 0
	m.cursor = 0
	m.offset = 0
	if m.filter == "" {
		return
	}
	m.refreshFilter()
}

func (m *model) refreshFilter() {
	if m.filter == "" {
		return
	}
	filterLower := strings.ToLower(m.filter)
	for m.filterScanned < len(m.provider.Commits) {
		commit := m.provider.Commits[m.filterScanned]
		if strings.Contains(strings.ToLower(commit.Subject), filterLower) ||
			strings.Contains(strings.ToLower(commit.Author), filterLower) {
			m.filtered = append(m.filtered, m.filterScanned)
		}
		m.filterScanned++
	}
}

func (m *model) ensureVisible() {
	buffer := 5
	viewport := m.viewportHeight()
	if viewport <= 0 {
		return
	}
	if m.filter == "" {
		target := m.offset + viewport + buffer
		_ = m.provider.Ensure(target)
		return
	}

	for len(m.filtered) <= m.offset+viewport+buffer && m.provider.HasMore() {
		_ = m.provider.Ensure(len(m.provider.Commits))
		m.refreshFilter()
	}
}

func (m *model) moveCursor(delta int) {
	if m.listLength() == 0 {
		return
	}
	m.cursor = clamp(m.cursor+delta, 0, m.listLength()-1)
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+m.viewportHeight() {
		m.offset = m.cursor - m.viewportHeight() + 1
	}
	if delta > 0 {
		m.ensureVisible()
		if m.cursor >= m.listLength()-1 && m.provider.HasMore() {
			m.ensureVisible()
		}
	}
}

func (m *model) listLength() int {
	if m.filter != "" {
		return len(m.filtered)
	}
	return len(m.provider.Commits)
}

func (m *model) viewportHeight() int {
	headerHeight, footerHeight, searchHeight := m.layoutHeights()
	height := m.height - headerHeight - footerHeight - searchHeight
	if height < 1 {
		return 1
	}
	return height
}

func (m *model) selectedCommit() *gitgraph.CommitInfo {
	if m.listLength() == 0 {
		return nil
	}
	index := m.cursor
	if m.filter != "" {
		if m.cursor >= len(m.filtered) {
			return nil
		}
		index = m.filtered[m.cursor]
	}
	if index >= len(m.provider.Commits) {
		return nil
	}
	return m.provider.Commits[index]
}

func (m *model) changedFiles(commit *gitgraph.CommitInfo) []string {
	key := commit.Hash.String()
	if cached, ok := m.filesCache[key]; ok {
		return cached
	}
	files, err := filesForCommit(commit.Commit)
	if err != nil {
		m.filesCache[key] = []string{"(unable to load files)"}
		return m.filesCache[key]
	}
	m.filesCache[key] = files
	return files
}

func (m *model) normalizePosition() {
	listLen := m.listLength()
	if listLen == 0 {
		m.cursor = 0
		m.offset = 0
		return
	}
	m.cursor = clamp(m.cursor, 0, listLen-1)
	viewport := m.viewportHeight()
	maxOffset := max(0, listLen-viewport)
	m.offset = clamp(m.offset, 0, maxOffset)
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+viewport {
		m.offset = m.cursor - viewport + 1
	}
}

func renderGraph(cells []gitgraph.GraphCell, bg lipgloss.TerminalColor) string {
	parts := make([]string, 0, len(cells))
	for _, cell := range cells {
		style := branchStyles[cell.Color%len(branchStyles)]
		parts = append(parts, style.Background(bg).Render(cell.Ch))
	}
	return strings.Join(parts, "")
}

func filesForCommit(commit *object.Commit) ([]string, error) {
	var parent *object.Commit
	if commit.NumParents() > 0 {
		p, err := commit.Parent(0)
		if err == nil {
			parent = p
		}
	}
	patch, err := commit.Patch(parent)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(patch.FilePatches()))
	for _, filePatch := range patch.FilePatches() {
		from, to := filePatch.Files()
		if to != nil {
			paths = append(paths, to.Path())
			continue
		}
		if from != nil {
			paths = append(paths, from.Path())
		}
	}
	if len(paths) == 0 {
		return []string{"(no file changes)"}, nil
	}
	sort.Strings(paths)
	return paths, nil
}

func (m *model) headerView(width int) string {
	if width <= 0 {
		return ""
	}

	contentWidth := max(0, width-2)
	leftParts := []string{
		headerTitleStyle.Render("arbor"),
		headerSepStyle.Render("|"),
		headerRepoStyle.Render(m.repoPath),
	}
	if m.filter != "" {
		leftParts = append(leftParts, headerFilterStyle.Render(fmt.Sprintf("/%s", m.filter)))
	}
	if m.headName != "" {
		leftParts = append(leftParts, headerBadgeStyle.Render(fmt.Sprintf("branch %s", m.headName)))
	}
	left := strings.Join(leftParts, " ")

	visible := m.listLength()
	loaded := len(m.provider.Commits)
	right := headerMetaStyle.Render(fmt.Sprintf("%d visible | %d loaded", visible, loaded))

	maxRight := contentWidth - lipgloss.Width(left) - 1
	if maxRight < 0 {
		maxRight = 0
	}
	if lipgloss.Width(right) > maxRight {
		right = ansi.Truncate(right, maxRight, "")
	}

	space := contentWidth - lipgloss.Width(left) - lipgloss.Width(right)
	if space < 1 {
		maxLeft := contentWidth - lipgloss.Width(right) - 1
		if maxLeft < 0 {
			maxLeft = 0
		}
		left = ansi.Truncate(left, maxLeft, "")
		space = contentWidth - lipgloss.Width(left) - lipgloss.Width(right)
		if space < 1 {
			space = 1
		}
	}

	line := left + strings.Repeat(" ", space) + right
	return headerStyle.Width(width).Render(line)
}

func (m *model) footerView(width int) string {
	if width <= 0 {
		return ""
	}
	contentWidth := max(0, width-2)
	hints := footerHintStyle.Render("up/down k/j move | enter files | / search | tab sidebar | q quit")

	total := m.listLength()
	position := 0
	if total > 0 {
		position = m.cursor + 1
	}
	loaded := len(m.provider.Commits)
	more := ""
	if m.provider.HasMore() {
		more = "+"
	}

	statusParts := []string{fmt.Sprintf("%d/%d", position, total), fmt.Sprintf("loaded %d%s", loaded, more)}
	if m.filter != "" {
		statusParts = append([]string{fmt.Sprintf("filter %q", m.filter)}, statusParts...)
	}
	status := footerStatusStyle.Render(strings.Join(statusParts, " | "))

	space := contentWidth - lipgloss.Width(hints) - lipgloss.Width(status)
	if space < 1 {
		maxHints := contentWidth - lipgloss.Width(status) - 1
		if maxHints < 0 {
			maxHints = 0
		}
		hints = footerHintStyle.Render(truncateText("up/down k/j move | enter files | / search | tab sidebar | q quit", maxHints))
		space = contentWidth - lipgloss.Width(hints) - lipgloss.Width(status)
		if space < 1 {
			space = 1
		}
	}
	line := hints + strings.Repeat(" ", space) + status
	return footerStyle.Width(width).Render(line)
}

func (m *model) layoutHeights() (int, int, int) {
	width := m.width
	if width <= 0 {
		return 1, 1, 0
	}
	header := m.headerView(width)
	footer := m.footerView(width)
	headerHeight := max(1, lipgloss.Height(header))
	footerHeight := max(1, lipgloss.Height(footer))
	searchHeight := 0
	if m.searchActive {
		searchHeight = max(1, lipgloss.Height(m.searchView(width)))
	}
	return headerHeight, footerHeight, searchHeight
}

func (m *model) emptyRow(width int) string {
	bg := palette.bg
	msg := emptyStyle.Foreground(palette.textDim).Background(bg).Render("No commits")
	return fitLine(msg, width, bg)
}

func (m *model) blankRow(width int, alt bool) string {
	bg := palette.bg
	if alt {
		bg = palette.bgAlt
	}
	return fitLine("", width, bg)
}

func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	lines := []string{}
	line := ""
	for _, word := range words {
		if len(line)+len(word)+1 > width {
			lines = append(lines, line)
			line = word
			continue
		}
		if line == "" {
			line = word
			continue
		}
		line = line + " " + word
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

func truncateText(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if len(text) <= maxWidth {
		return text
	}
	if maxWidth <= 3 {
		return text[:maxWidth]
	}
	return text[:maxWidth-3] + "..."
}

func fitLine(text string, width int, bg lipgloss.TerminalColor) string {
	if width <= 0 {
		return text
	}
	truncated := ansi.Truncate(text, width, "")
	pad := width - lipgloss.Width(truncated)
	if pad < 0 {
		pad = 0
	}
	if pad == 0 {
		return truncated
	}
	return truncated + rowSpacerStyle.Background(bg).Render(strings.Repeat(" ", pad))
}

func clamp(val, minVal, maxVal int) int {
	if val < minVal {
		return minVal
	}
	if val > maxVal {
		return maxVal
	}
	return val
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var (
	palette = struct {
		bg            lipgloss.AdaptiveColor
		bgAlt         lipgloss.AdaptiveColor
		panelBg       lipgloss.AdaptiveColor
		panelBorder   lipgloss.AdaptiveColor
		text          lipgloss.AdaptiveColor
		textMuted     lipgloss.AdaptiveColor
		textDim       lipgloss.AdaptiveColor
		accent        lipgloss.AdaptiveColor
		accentAlt     lipgloss.AdaptiveColor
		highlightBg   lipgloss.AdaptiveColor
		highlightText lipgloss.AdaptiveColor
		headerBg      lipgloss.AdaptiveColor
		searchBg      lipgloss.AdaptiveColor
		footerBg      lipgloss.AdaptiveColor
	}{
		bg:            lipgloss.AdaptiveColor{Light: "#f7f4ee", Dark: "#0f1411"},
		bgAlt:         lipgloss.AdaptiveColor{Light: "#efe9df", Dark: "#141b16"},
		panelBg:       lipgloss.AdaptiveColor{Light: "#f2eee6", Dark: "#141c18"},
		panelBorder:   lipgloss.AdaptiveColor{Light: "#c9bda8", Dark: "#2c3a32"},
		text:          lipgloss.AdaptiveColor{Light: "#2a271f", Dark: "#e6f0e6"},
		textMuted:     lipgloss.AdaptiveColor{Light: "#5e5648", Dark: "#a6b4a6"},
		textDim:       lipgloss.AdaptiveColor{Light: "#8a8171", Dark: "#7b887f"},
		accent:        lipgloss.AdaptiveColor{Light: "#2f6d4b", Dark: "#6fd08a"},
		accentAlt:     lipgloss.AdaptiveColor{Light: "#7a5a2a", Dark: "#d2a76a"},
		highlightBg:   lipgloss.AdaptiveColor{Light: "#d8efe2", Dark: "#264c37"},
		highlightText: lipgloss.AdaptiveColor{Light: "#1f3b2a", Dark: "#eaf6ee"},
		headerBg:      lipgloss.AdaptiveColor{Light: "#e9efe6", Dark: "#18221d"},
		searchBg:      lipgloss.AdaptiveColor{Light: "#e9efe6", Dark: "#18221d"},
		footerBg:      lipgloss.AdaptiveColor{Light: "#e9efe6", Dark: "#18221d"},
	}

	branchColors = []lipgloss.TerminalColor{
		lipgloss.AdaptiveColor{Light: "#2f6d4b", Dark: "#6fd08a"},
		lipgloss.AdaptiveColor{Light: "#4f8a5b", Dark: "#7ee1a0"},
		lipgloss.AdaptiveColor{Light: "#7a5a2a", Dark: "#d2a76a"},
		lipgloss.AdaptiveColor{Light: "#4d7f75", Dark: "#7fd3c5"},
		lipgloss.AdaptiveColor{Light: "#4f6f8a", Dark: "#8fb9e0"},
		lipgloss.AdaptiveColor{Light: "#9a6b2f", Dark: "#f0c07a"},
		lipgloss.AdaptiveColor{Light: "#6e8b3d", Dark: "#a8e063"},
		lipgloss.AdaptiveColor{Light: "#3f5a4a", Dark: "#6cb08a"},
		lipgloss.AdaptiveColor{Light: "#6c7a74", Dark: "#a9b6b0"},
	}

	branchStyles = func() []lipgloss.Style {
		styles := make([]lipgloss.Style, 0, len(branchColors))
		for _, color := range branchColors {
			styles = append(styles, lipgloss.NewStyle().Foreground(color))
		}
		return styles
	}()

	headerStyle       = lipgloss.NewStyle().Foreground(palette.text).Background(palette.headerBg).Padding(0, 1)
	headerTitleStyle  = lipgloss.NewStyle().Bold(true).Foreground(palette.accent).Background(palette.headerBg)
	headerRepoStyle   = lipgloss.NewStyle().Foreground(palette.text).Background(palette.headerBg)
	headerFilterStyle = lipgloss.NewStyle().Foreground(palette.accentAlt).Background(palette.headerBg)
	headerSepStyle    = lipgloss.NewStyle().Foreground(palette.textDim).Background(palette.headerBg)
	headerMetaStyle   = lipgloss.NewStyle().Foreground(palette.textDim).Background(palette.headerBg)
	headerBadgeStyle  = lipgloss.NewStyle().Foreground(palette.highlightText).Background(palette.accent).Padding(0, 1)

	rowSeparatorStyle = lipgloss.NewStyle()
	rowSpacerStyle    = lipgloss.NewStyle()
	hashStyle         = lipgloss.NewStyle().Foreground(palette.accent).Bold(true)
	subjectStyle      = lipgloss.NewStyle().Foreground(palette.text).Bold(true)
	authorStyle       = lipgloss.NewStyle().Foreground(palette.textMuted)

	sidebarStyle         = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(palette.panelBorder).Padding(0, 1).Background(palette.panelBg).Foreground(palette.text)
	sidebarTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(palette.accentAlt).Background(palette.panelBg)
	sidebarSubtitleStyle = lipgloss.NewStyle().Bold(true).Foreground(palette.accent).Background(palette.panelBg)
	searchStyle          = lipgloss.NewStyle().Foreground(palette.text).Background(palette.searchBg).Padding(0, 1)
	emptyStyle           = lipgloss.NewStyle().Foreground(palette.textDim)

	footerStyle       = lipgloss.NewStyle().Foreground(palette.text).Background(palette.footerBg).Padding(0, 1)
	footerHintStyle   = lipgloss.NewStyle().Foreground(palette.textMuted).Background(palette.footerBg)
	footerStatusStyle = lipgloss.NewStyle().Foreground(palette.accent).Background(palette.footerBg)
)
