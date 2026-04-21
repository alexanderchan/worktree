package internal

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	sfuzzy "github.com/sahilm/fuzzy"
)

// Item represents a selectable worktree or recent branch.
type Item struct {
	Branch       string
	Path         string // full path (empty for branch-only)
	DisplayPath  string // shortened path for display
	IsWorktree   bool
	IsCurrent    bool
	IsMain       bool
	Head         string     // short commit hash
	Score        float64    // frecency score
	UseCount     int        // from DB
	LastUsed     *time.Time // last time selected via wt-go (frecency) — nil if no history
	ActivityTime *time.Time // last commit date or dir mtime — used for displayed age
	ReflogPos    int        // -1 if not in reflog
}

// ScoreItems computes frecency scores and sorts items highest first.
func ScoreItems(items []Item, usage map[string]UsageRecord, reflogLen int) []Item {
	for i := range items {
		it := &items[i]
		if r, ok := usage[it.Branch]; ok {
			it.Score = calcFrecency(r.Count, r.LastUsed)
			it.UseCount = r.Count
			t := r.LastUsed
			it.ActivityTime = &t
		} else if it.ReflogPos >= 0 {
			// Synthetic score: most recent reflog entry → ~0.5, oldest → ~0.05
			frac := float64(reflogLen-1-it.ReflogPos) / float64(max(reflogLen-1, 1))
			it.Score = 0.05 + frac*0.45
		} else if it.IsWorktree {
			it.Score = 0.02 // baseline for worktrees not yet visited via wt-go
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		// Demote stale items below fresh ones so V1's grouped layout (ACTIVE
		// above STALE) matches the cursor's linear walk through filtered —
		// otherwise pressing ↓ can jump the cursor from ACTIVE into STALE
		// when a stale item outscores a fresh one on frecency.
		si, sj := isStale(items[i]), isStale(items[j])
		if si != sj {
			return !si
		}
		return items[i].Score > items[j].Score
	})
	return items
}

func calcFrecency(count int, lastUsed time.Time) float64 {
	d := time.Since(lastUsed)
	var ts float64
	switch {
	case d < 24*time.Hour:
		ts = 1.0
	case d < 7*24*time.Hour:
		ts = 0.5
	case d < 30*24*time.Hour:
		ts = 0.2
	default:
		ts = 0.1
	}
	fs := math.Min(float64(count)/10.0, 1.0)
	return fs*0.4 + ts*0.6
}

// FilterItems filters and re-ranks items by fuzzy match against query.
func FilterItems(items []Item, query string) []Item {
	query = strings.TrimSpace(query)
	if query == "" {
		return items
	}
	type ranked struct {
		item Item
		rank int
	}
	var results []ranked
	for _, item := range items {
		if r := fuzzyRank(item.Branch, query); r > 0 {
			results = append(results, ranked{item, r})
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		// Same stale-demotion as ScoreItems so V1's ACTIVE/STALE grouping
		// stays aligned with cursor order after a fuzzy filter.
		si, sj := isStale(results[i].item), isStale(results[j].item)
		if si != sj {
			return !si
		}
		if results[i].rank != results[j].rank {
			return results[i].rank > results[j].rank
		}
		return results[i].item.Score > results[j].item.Score
	})
	out := make([]Item, len(results))
	for i, r := range results {
		out[i] = r.item
	}
	return out
}

func fuzzyRank(branch, query string) int {
	lower := strings.ToLower(branch)
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return 0
	}

	// Phase 1: exact/prefix/contains/sahilm — every word must match.
	// Dashes/slashes in branch names act as word separators, so "doc intel"
	// matches "implement-doc-intelligence" because both words are found.
	totalScore := 0
	for _, word := range words {
		var wordScore int
		if lower == word {
			wordScore = 1000
		} else if strings.HasPrefix(lower, word) {
			wordScore = 500
		} else if strings.Contains(lower, word) {
			wordScore = 300
		} else {
			// sahilm/fuzzy: word-boundary and consecutive-character bonuses.
			// A non-empty result means the chars were found in order (valid
			// subsequence). Use the score for ranking but floor at 1 so a
			// marginal subsequence match is never discarded.
			if m := sfuzzy.Find(word, []string{lower}); len(m) > 0 {
				wordScore = max(1, m[0].Score+40)
			}
		}
		if wordScore == 0 {
			totalScore = 0
			break
		}
		totalScore += wordScore
	}
	if totalScore > 0 {
		return totalScore
	}

	// Phase 2: typo-tolerant fallback via Levenshtein on word segments.
	// Scores are intentionally lower than phase 1 so typo matches rank below
	// clean matches.
	return typoFallbackRank(branch, query)
}

// typoFallbackRank is used when fuzzyRank returns 0. It splits the branch on
// separators and checks each query word against each segment using Levenshtein
// edit distance. This catches transpositions and missing characters that break
// subsequence matching (e.g. "lcl" → "local").
//
// Scores are intentionally much lower than fuzzyRank so typo matches always
// rank below clean matches.
func typoFallbackRank(branch, query string) int {
	lower := strings.ToLower(branch)
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return 0
	}

	segments := strings.FieldsFunc(lower, func(r rune) bool {
		return r == '-' || r == '/' || r == '_' || r == '.' || r == ' '
	})
	if len(segments) == 0 {
		segments = []string{lower}
	}

	totalScore := 0
	for _, word := range words {
		// Allow 1 error per 2 chars above a minimum length.
		// Short words (≤3 chars) get no typo tolerance — too many false positives.
		// 4 chars→1, 6 chars→2, 8 chars→3, etc.
		threshold := max(0, (len([]rune(word))-2)/2)
		best := 0
		for _, seg := range segments {
			d := levenshtein(word, seg)
			if d <= threshold {
				score := max(1, 30-d*10) // 30 → 20 → 10 → 1 as errors increase
				if score > best {
					best = score
				}
			}
		}
		if best == 0 {
			return 0 // word didn't match any segment even with typos
		}
		totalScore += best
	}
	return totalScore
}

// levenshtein computes the edit distance between two strings.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	m, n := len(ra), len(rb)
	dp := make([]int, n+1)
	for j := range dp {
		dp[j] = j
	}
	for i := 1; i <= m; i++ {
		prev := dp[0]
		dp[0] = i
		for j := 1; j <= n; j++ {
			tmp := dp[j]
			if ra[i-1] == rb[j-1] {
				dp[j] = prev
			} else {
				dp[j] = 1 + min(prev, min(dp[j], dp[j-1]))
			}
			prev = tmp
		}
	}
	return dp[n]
}

// --- Styles ---

var (
	branchStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#71BEF2"))
	currentStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A8CC8C"))
	recentStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#DBAB79"))
	dimStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	veryDimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	cursorSty       = lipgloss.NewStyle().Foreground(lipgloss.Color("#D290E4")).Bold(true)
	promptSty       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#71BEF2"))
	selectedBg      = lipgloss.NewStyle().Background(lipgloss.Color("#2A2A2A"))
	pathSty         = lipgloss.NewStyle().Foreground(lipgloss.Color("#B9BFCA"))
	selectedPathSty = lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E5E5"))
	hashSty         = lipgloss.NewStyle().Foreground(lipgloss.Color("#C678DD"))
	headerSty       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8A8FA3"))
	staleBranchSty  = lipgloss.NewStyle().Foreground(lipgloss.Color("#8A8FA3"))
	barSty          = lipgloss.NewStyle().Foreground(lipgloss.Color("#B9BFCA")).Background(lipgloss.Color("#1E1E2E"))

	// Age gradient: fresh → stale
	ageFreshSty = lipgloss.NewStyle().Foreground(lipgloss.Color("#A8CC8C")) // < 1d  green
	ageWarmSty  = lipgloss.NewStyle().Foreground(lipgloss.Color("#E5C07B")) // 1–7d  yellow
	ageCoolSty  = lipgloss.NewStyle().Foreground(lipgloss.Color("#DBAB79")) // 7–14d orange
	ageStaleSty = lipgloss.NewStyle().Foreground(lipgloss.Color("#6C7086")) // ≥ 14d gray
)

const staleThreshold = 14 * 24 * time.Hour

// isStale reports whether an item's activity time puts it in the stale bucket.
func isStale(it Item) bool {
	return it.ActivityTime != nil && time.Since(*it.ActivityTime) >= staleThreshold
}

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
	case d < staleThreshold:
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

// --- Variant 1: grouped active/stale with age-bracket prefix ---

func renderV1(items []Item, cursor, width int) string {
	var activeIdx, staleIdx []int
	for i, it := range items {
		if isStale(it) {
			staleIdx = append(staleIdx, i)
		} else {
			activeIdx = append(activeIdx, i)
		}
	}
	var b strings.Builder
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

func renderRowV1(it Item, width int, selected bool) string {
	cursor := "  "
	if selected {
		cursor = cursorSty.Render("❯ ")
	}
	ageStr := fmt.Sprintf("[%-3s]", ageShort(it.ActivityTime))
	age := ageStyle(it.ActivityTime).Render(ageStr)
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
		branch = currentStyle.Render(branchPlain)
	case !it.IsWorktree:
		branch = recentStyle.Render(branchPlain)
	case isStale(it):
		branch = staleBranchSty.Render(branchPlain)
	default:
		branch = branchStyle.Render(branchPlain)
	}
	line := cursor + age + "  " + branch
	if selected {
		line = selectedBg.Width(width).Render(line)
	}
	return line
}

// --- Variant 2: aligned columns + always-on path status bar ---

func renderV2(items []Item, cursor, width int) string {
	ageW, hashW, countW := 4, 7, 4
	cursorW := 2
	branchW := width - cursorW - ageW - hashW - countW - 6
	branchW = max(branchW, 10)
	var b strings.Builder
	for i, it := range items {
		b.WriteString(renderRowV2(it, branchW, ageW, hashW, countW, i == cursor) + "\n")
	}
	return b.String()
}

func renderRowV2(it Item, branchW, ageW, hashW, countW int, selected bool) string {
	cursor := "  "
	if selected {
		cursor = cursorSty.Render("❯ ")
	}
	var icon string
	switch {
	case it.IsCurrent:
		icon = currentStyle.Render("▶")
	case !it.IsWorktree:
		icon = recentStyle.Render("⎇")
	default:
		icon = dimStyle.Render("●")
	}
	branchText := truncate(it.Branch, branchW-2)
	var branch string
	switch {
	case it.IsCurrent:
		branch = currentStyle.Render(branchText)
	case !it.IsWorktree:
		branch = recentStyle.Render(branchText)
	default:
		branch = branchStyle.Render(branchText)
	}
	branchCol := padRight(icon+" "+branch, branchW)
	ageCol := padRight(ageStyle(it.ActivityTime).Render(ageShort(it.ActivityTime)), ageW)
	hashCol := padRight(hashSty.Render(it.Head), hashW)
	countStr := ""
	if it.UseCount > 0 {
		countStr = fmt.Sprintf("%d×", it.UseCount)
	}
	countCol := padRight(dimStyle.Render(countStr), countW)

	line := cursor + branchCol + "  " + ageCol + "  " + hashCol + "  " + countCol
	total := lipgloss.Width(cursor) + branchW + 2 + ageW + 2 + hashW + 2 + countW
	if selected {
		line = selectedBg.Width(total).Render(line)
	}
	return line
}

// --- Variant 3: two-line rows ---

func renderV3(items []Item, cursor, width int) string {
	var b strings.Builder
	for i, it := range items {
		b.WriteString(renderRowV3(it, width, i == cursor))
	}
	return b.String()
}

func renderRowV3(it Item, width int, selected bool) string {
	cursor := "  "
	if selected {
		cursor = cursorSty.Render("❯ ")
	}
	var branch string
	switch {
	case it.IsCurrent:
		branch = currentStyle.Render("▶ " + it.Branch)
	case !it.IsWorktree:
		branch = recentStyle.Render("⎇ " + it.Branch)
	default:
		branch = branchStyle.Render("● " + it.Branch)
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
	ps := veryDimStyle
	if selected {
		ps = selectedPathSty
	}
	line2 := "    " + ps.Render(middleTruncate(p, width-20)) + "  " + age + dimStyle.Render(count)
	if selected {
		line1 = selectedBg.Width(width).Render(line1)
		line2 = selectedBg.Width(width).Render(line2)
	}
	return line1 + "\n" + line2 + "\n"
}

// --- Branch details panel ---

// BranchDetails holds async-fetched info about the currently selected item.
type BranchDetails struct {
	FullPath    string
	LastCommit  string // relative date string from git log
	IsDirty     bool
	DirtyCount  int
	Err         string // non-empty if fetch failed
}

// detailsMsg is a Bubble Tea message carrying fetched details.
type detailsMsg struct {
	details BranchDetails
}

// fetchDetails returns a Bubble Tea Cmd that fetches branch details asynchronously.
func fetchDetails(item Item) tea.Cmd {
	return func() tea.Msg {
		d := BranchDetails{FullPath: item.Path}

		if item.Path == "" {
			d.Err = "no local path (recent branch)"
			return detailsMsg{d}
		}

		// Last commit date (relative).
		out, err := exec.Command("git", "-C", item.Path, "log", "-1", "--format=%cd", "--date=relative").Output()
		if err == nil {
			d.LastCommit = strings.TrimSpace(string(out))
		}

		// Dirty status — count lines from `git status --porcelain`.
		out, err = exec.Command("git", "-C", item.Path, "status", "--porcelain").Output()
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			count := 0
			for _, l := range lines {
				if strings.TrimSpace(l) != "" {
					count++
				}
			}
			d.DirtyCount = count
			d.IsDirty = count > 0
		}

		return detailsMsg{d}
	}
}

// --- Bubble Tea model ---

const (
	headerLines  = 4 // title + blank + filter + blank
	helpLines    = 1
	detailLines  = 4 // separator + path + commit/dirty + blank
	minVPHeight  = 3
)

type filterableSelector struct {
	filter         textinput.Model
	viewport       viewport.Model
	all            []Item
	filtered       []Item
	cursor         int
	result         *Item
	quitting       bool
	width          int
	height         int
	variant        int // 1=grouped, 2=aligned-columns, 3=two-line
	showDetails    bool
	details        *BranchDetails
	detailsLoading bool
}

func (m *filterableSelector) Init() tea.Cmd {
	return textinput.Blink
}

func (m *filterableSelector) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case detailsMsg:
		d := msg.details
		m.details = &d
		m.detailsLoading = false
		return m, nil

	case tea.KeyMsg:
		// Intercept '?' before passing to textinput (rune keys can vary by terminal).
		if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == '?' {
			m.showDetails = !m.showDetails
			m.viewport.Height = m.vpHeight()
			if m.showDetails && len(m.filtered) > 0 {
				m.details = nil
				m.detailsLoading = true
				return m, fetchDetails(m.filtered[m.cursor])
			}
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit

		case "tab":
			m.variant++
			if m.variant > 3 {
				m.variant = 1
			}
			m.viewport.Height = m.vpHeight()
			return m, nil

		case "shift+tab":
			m.variant--
			if m.variant < 1 {
				m.variant = 3
			}
			m.viewport.Height = m.vpHeight()
			return m, nil

		case "enter":
			if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
				item := m.filtered[m.cursor]
				m.result = &item
				m.quitting = true
				return m, tea.Quit
			}

		case "esc", "ctrl+u":
			if m.showDetails {
				m.showDetails = false
				m.viewport.Height = m.vpHeight()
				return m, nil
			}
			m.filter.SetValue("")
			m.filtered = FilterItems(m.all, "")
			m.cursor = 0
			m.viewport.GotoTop()

		case "up":
			if m.cursor > 0 {
				m.cursor--
			} else {
				m.cursor = max(0, len(m.filtered)-1)
			}
			m.scrollToCursor()
			if m.showDetails && len(m.filtered) > 0 {
				m.details = nil
				m.detailsLoading = true
				return m, fetchDetails(m.filtered[m.cursor])
			}

		case "down":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			} else {
				m.cursor = 0
			}
			m.scrollToCursor()
			if m.showDetails && len(m.filtered) > 0 {
				m.details = nil
				m.detailsLoading = true
				return m, fetchDetails(m.filtered[m.cursor])
			}

		default:
			m.filter, cmd = m.filter.Update(msg)
			m.filtered = FilterItems(m.all, m.filter.Value())
			if m.cursor >= len(m.filtered) {
				m.cursor = 0
			}
			m.scrollToCursor()
			if m.showDetails && len(m.filtered) > 0 {
				m.details = nil
				m.detailsLoading = true
				return m, tea.Batch(cmd, fetchDetails(m.filtered[m.cursor]))
			}
			return m, cmd
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.filter.Width = msg.Width - 3
		m.viewport.Width = msg.Width
		m.viewport.Height = m.vpHeight()
	}
	return m, cmd
}

// vpHeight computes the viewport height based on terminal size and panel visibility.
func (m *filterableSelector) vpHeight() int {
	extra := 0
	if m.showDetails {
		extra = detailLines
	}
	if m.variant == 2 {
		extra += 1 // path status bar
	}
	return max(m.height-headerLines-helpLines-extra, minVPHeight)
}

// cursorLineOffset returns the line index (in rendered content) where the
// cursor's row starts, given the current variant.
func (m *filterableSelector) cursorLineOffset() int {
	switch m.variant {
	case 3:
		return m.cursor * 2
	case 1:
		// Count group headers + blank line separators above the cursor.
		activeCount := 0
		for _, it := range m.filtered {
			if !isStale(it) {
				activeCount++
			}
		}
		staleCount := len(m.filtered) - activeCount
		if m.cursor < activeCount {
			offset := 0
			if activeCount > 0 {
				offset += 1 // ACTIVE header
			}
			return offset + m.cursor
		}
		// In stale bucket.
		offset := 0
		if activeCount > 0 {
			offset += 1 + activeCount + 1 // ACTIVE header + items + blank
		}
		if staleCount > 0 {
			offset += 1 // STALE header
		}
		return offset + (m.cursor - activeCount)
	default:
		return m.cursor
	}
}

// rowHeight returns how many rendered lines the current cursor row spans.
func (m *filterableSelector) rowHeight() int {
	if m.variant == 3 {
		return 2
	}
	return 1
}

func (m *filterableSelector) scrollToCursor() {
	offset := m.cursorLineOffset()
	rh := m.rowHeight()
	if offset < m.viewport.YOffset {
		m.viewport.YOffset = offset
	} else if offset+rh > m.viewport.YOffset+m.viewport.Height {
		m.viewport.YOffset = offset + rh - m.viewport.Height
	}
}

func (m *filterableSelector) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder
	b.WriteString(promptSty.Render("Switch worktree") + "  " + dimStyle.Render(fmt.Sprintf("[v%d]", m.variant)) + "\n\n")
	b.WriteString(m.filter.View() + "\n\n")

	var content string
	if len(m.filtered) == 0 {
		content = dimStyle.Render("  No matches") + "\n"
	} else {
		switch m.variant {
		case 2:
			content = renderV2(m.filtered, m.cursor, m.width)
		case 3:
			content = renderV3(m.filtered, m.cursor, m.width)
		default:
			content = renderV1(m.filtered, m.cursor, m.width)
		}
	}

	m.viewport.SetContent(content)
	b.WriteString(m.viewport.View())

	// Variant 2: always show path status bar at the bottom.
	if m.variant == 2 && len(m.filtered) > 0 && m.cursor < len(m.filtered) {
		it := m.filtered[m.cursor]
		p := it.Path
		if p == "" {
			p = "(no local path — recent branch)"
		}
		b.WriteString("\n" + barSty.Width(m.width).Render(" "+truncate(p, m.width-1)))
	}

	if m.showDetails {
		b.WriteString("\n" + dimStyle.Render(strings.Repeat("─", m.width)) + "\n")
		if m.detailsLoading {
			b.WriteString(dimStyle.Render("  Loading…") + "\n")
		} else if m.details != nil {
			if m.details.Err != "" {
				b.WriteString(dimStyle.Render("  "+m.details.Err) + "\n")
			} else {
				path := m.details.FullPath
				if path == "" {
					path = "(no local path)"
				}
				b.WriteString(pathSty.Render("  "+path) + "\n")
				var meta []string
				if m.details.LastCommit != "" {
					meta = append(meta, "last commit: "+m.details.LastCommit)
				}
				if m.details.IsDirty {
					meta = append(meta, fmt.Sprintf("dirty: %d file(s)", m.details.DirtyCount))
				} else {
					meta = append(meta, "clean")
				}
				b.WriteString(dimStyle.Render("  "+strings.Join(meta, "  •  ")) + "\n")
			}
		}
	}

	helpText := "↑/↓: navigate • tab: layout • enter: select • esc: clear • ?: details • q: quit"
	b.WriteString(dimStyle.Render("\n" + helpText))
	return b.String()
}

// ShowSelection runs the interactive TUI and returns the selected item (or nil if cancelled).
// Uses /dev/tty for both input and output so the TUI works even when stdout is piped,
// leaving stdout clean for the shell wrapper to capture the selected path.
func ShowSelection(items []Item) (*Item, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no items available")
	}

	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("cannot open /dev/tty: %w", err)
	}
	defer tty.Close()

	// /dev/tty bypasses stdout, so lipgloss's default renderer (which probes
	// stdout) sees no color support. Build a renderer against the tty and
	// force TrueColor so all our styled output actually gets colored.
	r := lipgloss.NewRenderer(tty)
	r.SetColorProfile(termenv.TrueColor)
	lipgloss.SetDefaultRenderer(r)

	ti := textinput.New()
	ti.Placeholder = "Type to filter..."
	ti.Focus()
	ti.CharLimit = 100
	ti.Width = 80
	ti.Prompt = "/ "

	vp := viewport.New(80, 10)

	m := &filterableSelector{
		filter:   ti,
		viewport: vp,
		all:      items,
		filtered: items,
		height:   24, // sensible default; overwritten by WindowSizeMsg
		variant:  1,
	}

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithInput(tty), tea.WithOutput(tty))
	final, err := p.Run()
	if err != nil {
		return nil, err
	}
	if fm, ok := final.(*filterableSelector); ok {
		return fm.result, nil
	}
	return nil, nil
}
