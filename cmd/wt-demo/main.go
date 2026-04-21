// wt-demo renders the wt-go TUI with baked-in sample data so we can compare
// layout variants without touching git. Pick a variant with --variant N.
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	wt "github.com/alexanderchan/wt/internal"
)

// --- sample data ---

func ago(d time.Duration) *time.Time {
	t := time.Now().Add(-d)
	return &t
}

func sampleItems(showBranches bool) []wt.Item {
	day := 24 * time.Hour
	items := []wt.Item{
		{Branch: "raf-permissions-views", Path: "/Users/alex/dev/project/wt/raf-permissions-views", DisplayPath: "./wt/raf-permissions-views", IsWorktree: true, IsCurrent: true, Head: "8088e40", UseCount: 3, ActivityTime: ago(4 * time.Hour)},
		{Branch: "fix-smoke-503-retry", Path: "/Users/alex/dev/project/wt/fix-smoke-503-retry", DisplayPath: "./wt/fix-smoke-503-retry", IsWorktree: true, Head: "3c4d5e6", UseCount: 4, ActivityTime: ago(2 * day)},
		{Branch: "graph-connector-settings", Path: "/Users/alex/dev/project/wt/graph-connector-settings", DisplayPath: "./wt/graph-connector-settings", IsWorktree: true, Head: "7a8b9c0", UseCount: 1, ActivityTime: ago(3 * day)},
		{Branch: "allow-more-access-to-extraction", Path: "/Users/alex/dev/project/wt/allow-more-access-to-extraction", DisplayPath: "./wt/allow-more-access-to-extraction", IsWorktree: true, Head: "cbb5acf", UseCount: 1, ActivityTime: ago(4 * day)},
		{Branch: "feature-AB#69128-implement-doc-intelligence", Path: "/Users/alex/dev/project/wt/feature-AB#69128-implement-doc-intelligence", DisplayPath: "./wt/feature-AB#69128-implement-doc-intelligence", IsWorktree: true, Head: "ca4123a", UseCount: 1, ActivityTime: ago(4 * day)},
		{Branch: "field-ticket-raf-next-phase", Path: "/Users/alex/dev/project/wt/field-ticket-raf-next-phase", DisplayPath: "./wt/field-ticket-raf-next-phase", IsWorktree: true, Head: "a1b2c3d", UseCount: 2, ActivityTime: ago(4 * day)},
		{Branch: "fix-ci-timeouts", Path: "/Users/alex/dev/project/wt/fix-ci-timeouts", DisplayPath: "./wt/fix-ci-timeouts", IsWorktree: true, Head: "b7c8d9e", UseCount: 5, ActivityTime: ago(4 * day)},
		{Branch: "fix-lock-acquire-polling-query", Path: "/Users/alex/dev/project/wt/fix-lock-acquire-polling-query", DisplayPath: "./wt/fix-lock-acquire-polling-query", IsWorktree: true, Head: "f1a2b3c", UseCount: 2, ActivityTime: ago(5 * day)},
		{Branch: "agent-a2d5f2bb", Path: "/Users/alex/dev/project/wt/agent-a2d5f2bb", DisplayPath: "./wt/agent-a2d5f2bb", IsWorktree: true, Head: "9e8d7c6", UseCount: 1, ActivityTime: ago(5 * day)},
		{Branch: "fix-back-history", Path: "/Users/alex/dev/project/wt/fix-back-history", DisplayPath: "./wt/fix-back-history", IsWorktree: true, Head: "d4e5f6a", UseCount: 1, ActivityTime: ago(6 * day)},
		{Branch: "wt-add-raf-refresh", Path: "/Users/alex/dev/project/wt/wt-add-raf-refresh", DisplayPath: "./wt/wt-add-raf-refresh", IsWorktree: true, Head: "a0a0a0a", UseCount: 1, ActivityTime: ago(24 * day)},
		{Branch: "add-claude-code-review-action", Path: "/Users/alex/dev/project/wt/add-claude-code-review-action", DisplayPath: "./wt/add-claude-code-review-action", IsWorktree: true, Head: "1a2b3c4", UseCount: 1, ActivityTime: ago(24 * day)},
		{Branch: "ap-case-comments", Path: "/Users/alex/dev/project/wt/ap-case-comments", DisplayPath: "./wt/ap-case-comments", IsWorktree: true, Head: "5d6e7f8", UseCount: 1, ActivityTime: ago(29 * day)},
		{Branch: "playwright-e2e", Path: "/Users/alex/dev/project/wt/playwright-e2e", DisplayPath: "./wt/playwright-e2e", IsWorktree: true, Head: "2b3c4d5", UseCount: 1, ActivityTime: ago(34 * day)},
		{Branch: "security-review-pr41", Path: "/Users/alex/dev/project/wt/security-review-pr41", DisplayPath: "./wt/security-review-pr41", IsWorktree: true, Head: "c1c2c3c", UseCount: 1, ActivityTime: ago(34 * day)},
		{Branch: "move-ft-to-reads", Path: "/Users/alex/dev/project/wt/move-ft-to-reads", DisplayPath: "./wt/move-ft-to-reads", IsWorktree: true, Head: "d1d2d3d", UseCount: 1, ActivityTime: ago(34 * day)},
		{Branch: "reads-template-diff-view", Path: "/Users/alex/dev/project/wt/reads-template-diff-view", DisplayPath: "./wt/reads-template-diff-view", IsWorktree: true, Head: "e1e2e3e", UseCount: 1, ActivityTime: ago(47 * day)},
		{Branch: "ms-graph-email-integration", Path: "/Users/alex/dev/project/wt/ms-graph-email-integration", DisplayPath: "./wt/ms-graph-email-integration", IsWorktree: true, Head: "6f7a8b9", UseCount: 1, ActivityTime: ago(49 * day)},
	}

	if showBranches {
		items = append(items,
			wt.Item{Branch: "feat/template-matcher-idf", ReflogPos: 0, ActivityTime: ago(4 * day), UseCount: 1},
			wt.Item{Branch: "fix-principal-prop", ReflogPos: 1},
			wt.Item{Branch: "devops/aws-human-access-rio-developers-group", ReflogPos: 2},
		)
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].IsCurrent != items[j].IsCurrent {
			return items[i].IsCurrent
		}
		if items[i].IsWorktree != items[j].IsWorktree {
			return items[i].IsWorktree
		}
		var ti, tj time.Time
		if items[i].ActivityTime != nil {
			ti = *items[i].ActivityTime
		}
		if items[j].ActivityTime != nil {
			tj = *items[j].ActivityTime
		}
		return ti.After(tj)
	})
	return items
}

// --- styles ---

var (
	// Age-gradient: fresh → stale
	ageFreshSty = lipgloss.NewStyle().Foreground(lipgloss.Color("#A8CC8C")) // green  (< 1d)
	ageWarmSty  = lipgloss.NewStyle().Foreground(lipgloss.Color("#E5C07B")) // yellow (1-7d)
	ageCoolSty  = lipgloss.NewStyle().Foreground(lipgloss.Color("#DBAB79")) // orange (7-14d)
	ageStaleSty = lipgloss.NewStyle().Foreground(lipgloss.Color("#6C7086")) // gray   (≥ 14d)

	branchSty   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#71BEF2"))
	currentSty  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A8CC8C"))
	staleBranch = lipgloss.NewStyle().Foreground(lipgloss.Color("#8A8FA3"))
	recentSty   = lipgloss.NewStyle().Foreground(lipgloss.Color("#DBAB79"))
	dimSty      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	veryDimSty  = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	pathSty         = lipgloss.NewStyle().Foreground(lipgloss.Color("#B9BFCA"))
	selectedPathSty = lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E5E5"))
	hashSty     = lipgloss.NewStyle().Foreground(lipgloss.Color("#C678DD"))
	cursorSty   = lipgloss.NewStyle().Foreground(lipgloss.Color("#D290E4")).Bold(true)
	selectedBg  = lipgloss.NewStyle().Background(lipgloss.Color("#2A2A2A"))
	promptSty   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#71BEF2"))
	headerSty   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8A8FA3"))
	barSty      = lipgloss.NewStyle().Foreground(lipgloss.Color("#B9BFCA")).Background(lipgloss.Color("#1E1E2E"))
)

// --- helpers ---

func ageShort(t *time.Time) string {
	if t == nil {
		return "—"
	}
	d := time.Since(*t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	default:
		return fmt.Sprintf("%dw", int(d.Hours()/24/7))
	}
}

func ageStyle(t *time.Time) lipgloss.Style {
	if t == nil {
		return ageStaleSty
	}
	d := time.Since(*t)
	switch {
	case d < 24*time.Hour:
		return ageFreshSty
	case d < 7*24*time.Hour:
		return ageWarmSty
	case d < 14*24*time.Hour:
		return ageCoolSty
	default:
		return ageStaleSty
	}
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

func middleTruncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 3 {
		return "…"
	}
	left := (n - 1) / 2
	right := n - 1 - left
	return string(r[:left]) + "…" + string(r[len(r)-right:])
}

func padRight(s string, n int) string {
	w := lipgloss.Width(s)
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}

// --- variant 1: grouped active/stale, age-bracket prefix, path hidden ---
//
// Uses the "Keeping / Stale" reference aesthetic: age in square brackets,
// branch name next, path revealed via ? toggle. Stale items are dimmed.

func renderV1(items []wt.Item, cursor, width int) string {
	var b strings.Builder
	activeIdx, staleIdx := []int{}, []int{}
	for i, it := range items {
		if it.ActivityTime != nil && time.Since(*it.ActivityTime) >= 14*24*time.Hour {
			staleIdx = append(staleIdx, i)
		} else {
			activeIdx = append(activeIdx, i)
		}
	}
	writeGroup := func(label string, idxs []int) {
		if len(idxs) == 0 {
			return
		}
		b.WriteString(headerSty.Render(fmt.Sprintf("%s (%d)", label, len(idxs))) + "\n")
		for _, i := range idxs {
			b.WriteString(renderRowV1(items[i], width, i == cursor) + "\n")
		}
		b.WriteString("\n")
	}
	writeGroup("ACTIVE", activeIdx)
	writeGroup("STALE", staleIdx)
	return b.String()
}

func renderRowV1(it wt.Item, width int, selected bool) string {
	cursor := "  "
	if selected {
		cursor = cursorSty.Render("❯ ")
	}
	ageStr := fmt.Sprintf("[%-3s]", ageShort(it.ActivityTime))
	age := ageStyle(it.ActivityTime).Render(ageStr)
	// Measure visible width of prefix (cursor + age + 2 spaces) then truncate
	// plain branch text before styling, so we never slice ANSI escapes.
	prefixW := lipgloss.Width(cursor) + lipgloss.Width(age) + 2
	branchPlain := it.Branch
	if !it.IsWorktree {
		branchPlain = "⎇ " + it.Branch
	} else if it.IsCurrent {
		branchPlain = "▶ " + it.Branch
	}
	branchPlain = truncate(branchPlain, width-prefixW)
	var branch string
	switch {
	case it.IsCurrent:
		branch = currentSty.Render(branchPlain)
	case !it.IsWorktree:
		branch = recentSty.Render(branchPlain)
	case it.ActivityTime != nil && time.Since(*it.ActivityTime) >= 14*24*time.Hour:
		branch = staleBranch.Render(branchPlain)
	default:
		branch = branchSty.Render(branchPlain)
	}
	line := cursor + age + "  " + branch
	if selected {
		line = selectedBg.Width(width).Render(line)
	}
	return line
}

// --- variant 2: aligned columns + status bar ---
//
// Path hidden inline; full path of highlighted row is shown in a footer bar.
// Fixed columns make scanning easy.

func renderV2(items []wt.Item, cursor, width int) string {
	var b strings.Builder
	// column widths
	ageW, hashW, countW := 4, 7, 4
	cursorW := 2
	branchW := width - cursorW - ageW - hashW - countW - 6 // 6 for spacing
	branchW = max(branchW, 10)
	for i, it := range items {
		b.WriteString(renderRowV2(it, branchW, ageW, hashW, countW, i == cursor) + "\n")
	}
	return b.String()
}

func renderRowV2(it wt.Item, branchW, ageW, hashW, countW int, selected bool) string {
	cursor := "  "
	if selected {
		cursor = cursorSty.Render("❯ ")
	}
	var icon string
	switch {
	case it.IsCurrent:
		icon = currentSty.Render("▶")
	case !it.IsWorktree:
		icon = recentSty.Render("⎇")
	default:
		icon = dimSty.Render("●")
	}
	branchText := truncate(it.Branch, branchW-2)
	var branch string
	if it.IsCurrent {
		branch = currentSty.Render(branchText)
	} else if !it.IsWorktree {
		branch = recentSty.Render(branchText)
	} else {
		branch = branchSty.Render(branchText)
	}
	branchCol := padRight(icon+" "+branch, branchW)

	ageCol := padRight(ageStyle(it.ActivityTime).Render(ageShort(it.ActivityTime)), ageW)
	hashCol := padRight(hashSty.Render(it.Head), hashW)
	countStr := ""
	if it.UseCount > 0 {
		countStr = fmt.Sprintf("%d×", it.UseCount)
	}
	countCol := padRight(dimSty.Render(countStr), countW)

	line := cursor + branchCol + "  " + ageCol + "  " + hashCol + "  " + countCol
	total := lipgloss.Width(cursor) + branchW + 2 + ageW + 2 + hashW + 2 + countW
	if selected {
		line = selectedBg.Width(total).Render(line)
	}
	return line
}

// --- variant 3: two-line rows ---
//
// Line 1: branch + hash. Line 2: dim indented path + age + count.

func renderV3(items []wt.Item, cursor, width int) string {
	var b strings.Builder
	for i, it := range items {
		b.WriteString(renderRowV3(it, width, i == cursor))
	}
	return b.String()
}

func renderRowV3(it wt.Item, width int, selected bool) string {
	cursor := "  "
	if selected {
		cursor = cursorSty.Render("❯ ")
	}
	var branch string
	switch {
	case it.IsCurrent:
		branch = currentSty.Render("▶ " + it.Branch)
	case !it.IsWorktree:
		branch = recentSty.Render("⎇ " + it.Branch)
	default:
		branch = branchSty.Render("● " + it.Branch)
	}
	head := ""
	if it.Head != "" {
		head = "  " + hashSty.Render(it.Head)
	}
	line1 := cursor + branch + head

	p := it.DisplayPath
	if p == "" {
		p = "(recent branch)"
	}
	age := ageStyle(it.ActivityTime).Render(ageShort(it.ActivityTime))
	count := ""
	if it.UseCount > 0 {
		count = fmt.Sprintf(" · %d×", it.UseCount)
	}
	pathSty := veryDimSty
	if selected {
		pathSty = selectedPathSty
	}
	meta := "    " + pathSty.Render(middleTruncate(p, width-20)) + "  " + age + dimSty.Render(count)
	line2 := meta
	if selected {
		line1 = selectedBg.Width(width).Render(line1)
		line2 = selectedBg.Width(width).Render(line2)
	}
	return line1 + "\n" + line2 + "\n"
}

// --- variant 4: compact single-line, smart truncation ---
//
// Age right-aligned in fixed column. Path middle-truncated. Branch gets
// whatever space remains.

func renderV4(items []wt.Item, cursor, width int) string {
	var b strings.Builder
	ageW := 4
	cursorW := 2
	// Split remaining between branch and path 50/50 but branch gets priority.
	remain := width - cursorW - ageW - 4 // 4 for spacing
	remain = max(remain, 20)
	branchW := remain * 3 / 5
	pathW := remain - branchW
	for i, it := range items {
		b.WriteString(renderRowV4(it, branchW, pathW, ageW, i == cursor) + "\n")
	}
	return b.String()
}

func renderRowV4(it wt.Item, branchW, pathW, ageW int, selected bool) string {
	cursor := "  "
	if selected {
		cursor = cursorSty.Render("❯ ")
	}
	var icon, branch string
	branchText := truncate(it.Branch, branchW-2)
	switch {
	case it.IsCurrent:
		icon = currentSty.Render("▶")
		branch = currentSty.Render(branchText)
	case !it.IsWorktree:
		icon = recentSty.Render("⎇")
		branch = recentSty.Render(branchText)
	default:
		icon = dimSty.Render("●")
		branch = branchSty.Render(branchText)
	}
	branchCol := padRight(icon+" "+branch, branchW)

	p := it.DisplayPath
	if p == "" {
		p = "(recent branch)"
	}
	pathCol := padRight(pathSty.Render(middleTruncate(p, pathW)), pathW)

	ageText := ageShort(it.ActivityTime)
	ageCol := ageStyle(it.ActivityTime).Render(fmt.Sprintf("%*s", ageW, ageText))

	line := cursor + branchCol + "  " + pathCol + "  " + ageCol
	total := lipgloss.Width(cursor) + branchW + 2 + pathW + 2 + ageW
	if selected {
		line = selectedBg.Width(total).Render(line)
	}
	return line
}

// --- model ---

type model struct {
	variant  int
	filter   textinput.Model
	viewport viewport.Model
	all      []wt.Item
	filtered []wt.Item
	cursor   int
	width    int
	height   int
	quitting bool
	showPath bool // variant 1: toggled with ?
}

func (m *model) Init() tea.Cmd {
	return textinput.Blink
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == '?' {
			m.showPath = !m.showPath
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			m.quitting = true
			return m, tea.Quit
		case "tab":
			m.variant++
			if m.variant > 4 {
				m.variant = 1
			}
			return m, nil
		case "shift+tab":
			m.variant--
			if m.variant < 1 {
				m.variant = 4
			}
			return m, nil
		case "up":
			if m.cursor > 0 {
				m.cursor--
			} else {
				m.cursor = len(m.filtered) - 1
			}
		case "down":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			} else {
				m.cursor = 0
			}
		default:
			m.filter, cmd = m.filter.Update(msg)
			m.filtered = wt.FilterItems(m.all, m.filter.Value())
			if m.cursor >= len(m.filtered) {
				m.cursor = 0
			}
			return m, cmd
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.filter.Width = msg.Width - 3
		m.viewport.Width = msg.Width
		m.viewport.Height = max(msg.Height-7, 5)
	}
	return m, cmd
}

func (m *model) View() string {
	if m.quitting {
		return ""
	}
	var b strings.Builder
	title := fmt.Sprintf("Switch worktree  [variant %d]", m.variant)
	b.WriteString(promptSty.Render(title) + "\n\n")
	b.WriteString(m.filter.View() + "\n\n")

	var content string
	switch m.variant {
	case 1:
		content = renderV1(m.filtered, m.cursor, m.width)
	case 2:
		content = renderV2(m.filtered, m.cursor, m.width)
	case 3:
		content = renderV3(m.filtered, m.cursor, m.width)
	case 4:
		content = renderV4(m.filtered, m.cursor, m.width)
	}
	m.viewport.SetContent(content)
	b.WriteString(m.viewport.View())

	// variant 2 always shows path bar; variant 1 shows when toggled
	if m.variant == 2 || (m.variant == 1 && m.showPath) {
		if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
			it := m.filtered[m.cursor]
			p := it.Path
			if p == "" {
				p = "(no local path — recent branch)"
			}
			b.WriteString("\n" + barSty.Width(m.width).Render(" "+truncate(p, m.width-1)))
		}
	}

	help := "↑/↓ move • tab: next variant • ?: toggle path (v1) • enter/esc: quit"
	b.WriteString("\n" + dimSty.Render(help))
	return b.String()
}

// --- main ---

func main() {
	var variant int
	var showBranches bool
	flag.IntVar(&variant, "variant", 1, "render variant (1-4)")
	flag.BoolVar(&showBranches, "branches", false, "show recent branches (hidden by default)")
	flag.Parse()
	if variant < 1 || variant > 4 {
		fmt.Fprintln(os.Stderr, "--variant must be 1-4")
		os.Exit(2)
	}

	items := sampleItems(showBranches)

	ti := textinput.New()
	ti.Placeholder = "Type to filter…"
	ti.Focus()
	ti.CharLimit = 100
	ti.Width = 80
	ti.Prompt = "/ "

	vp := viewport.New(80, 20)

	m := &model{
		variant:  variant,
		filter:   ti,
		viewport: vp,
		all:      items,
		filtered: items,
		height:   24,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
