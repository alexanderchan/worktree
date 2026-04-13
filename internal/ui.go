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
	sfuzzy "github.com/sahilm/fuzzy"
)

// Item represents a selectable worktree or recent branch.
type Item struct {
	Branch      string
	Path        string // full path (empty for branch-only)
	DisplayPath string // shortened path for display
	IsWorktree  bool
	IsCurrent   bool
	IsMain      bool
	Head        string     // short commit hash
	Score       float64    // frecency score
	UseCount    int        // from DB
	LastUsed    *time.Time // nil if no history
	ReflogPos   int        // -1 if not in reflog
}

// ScoreItems computes frecency scores and sorts items highest first.
func ScoreItems(items []Item, usage map[string]UsageRecord, reflogLen int) []Item {
	for i := range items {
		it := &items[i]
		if r, ok := usage[it.Branch]; ok {
			it.Score = calcFrecency(r.Count, r.LastUsed)
			it.UseCount = r.Count
			t := r.LastUsed
			it.LastUsed = &t
		} else if it.ReflogPos >= 0 {
			// Synthetic score: most recent reflog entry → ~0.5, oldest → ~0.05
			frac := float64(reflogLen-1-it.ReflogPos) / float64(max(reflogLen-1, 1))
			it.Score = 0.05 + frac*0.45
		} else if it.IsWorktree {
			it.Score = 0.02 // baseline for worktrees not yet visited via wt-go
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
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
	branchStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#71BEF2"))
	currentStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A8CC8C"))
	recentStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#DBAB79"))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	cursorSty    = lipgloss.NewStyle().Foreground(lipgloss.Color("#D290E4"))
	promptSty    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#71BEF2"))
	selectedBg   = lipgloss.NewStyle().Background(lipgloss.Color("#2A2A2A"))
	pathSty      = lipgloss.NewStyle().Foreground(lipgloss.Color("#B9BFCA"))
)

func formatItem(item Item, maxWidth int) string {
	var icon string
	if item.IsCurrent {
		icon = currentStyle.Render("▶ ")
	} else if item.IsWorktree {
		icon = dimStyle.Render("● ")
	} else {
		icon = recentStyle.Render("⎇ ")
	}

	branch := branchStyle.Render(item.Branch)

	var right string
	if item.IsWorktree {
		p := item.DisplayPath
		if p == "" {
			p = item.Path
		}
		// Truncate path if needed (rune-safe).
		if runes := []rune(p); maxWidth > 0 && len(runes) > 30 {
			p = "…" + string(runes[len(runes)-29:])
		}
		right = pathSty.Render(p)
		if item.Head != "" {
			right += dimStyle.Render("  " + item.Head)
		}
	} else {
		right = recentStyle.Render("recent branch")
	}

	var stats string
	if item.LastUsed != nil {
		stats = dimStyle.Render(fmt.Sprintf("  %d× %s", item.UseCount, timeAgo(*item.LastUsed)))
	}

	return fmt.Sprintf("%s%s  %s%s", icon, branch, right, stats)
}

func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return fmt.Sprintf("%dw ago", int(d.Hours()/24/7))
	}
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
	return max(m.height-headerLines-helpLines-extra, minVPHeight)
}

func (m *filterableSelector) scrollToCursor() {
	if m.cursor < m.viewport.YOffset {
		m.viewport.YOffset = m.cursor
	} else if m.cursor >= m.viewport.YOffset+m.viewport.Height {
		m.viewport.YOffset = m.cursor - m.viewport.Height + 1
	}
}

func (m *filterableSelector) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder
	b.WriteString(promptSty.Render("Switch worktree") + "\n\n")
	b.WriteString(m.filter.View() + "\n\n")

	cursorStr := cursorSty.Render("❯ ")
	blank := strings.Repeat(" ", lipgloss.Width(cursorStr))
	cursorW := lipgloss.Width(cursorStr)

	var content strings.Builder
	if len(m.filtered) == 0 {
		content.WriteString(dimStyle.Render("  No matches") + "\n")
	} else {
		for i, item := range m.filtered {
			line := formatItem(item, m.width-cursorW)
			prefix := blank
			if i == m.cursor {
				prefix = cursorStr
				line = selectedBg.Width(m.width - cursorW).Render(line)
			}
			content.WriteString(prefix + line + "\n")
		}
	}

	m.viewport.SetContent(content.String())
	b.WriteString(m.viewport.View())

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

	helpText := "↑/↓: navigate • enter: select • esc: clear • ?: details • q: quit"
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
