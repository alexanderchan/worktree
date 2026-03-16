package internal

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lithammer/fuzzysearch/fuzzy"
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
		r := fuzzyRank(item.Branch, query)
		if r > 0 {
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
	q := strings.ToLower(query)

	words := strings.Fields(q)

	// Single-word: exact → prefix → contains → fuzzy
	if len(words) <= 1 {
		if lower == q {
			return 1000
		}
		if strings.HasPrefix(lower, q) {
			return 500
		}
		if strings.Contains(lower, q) {
			return 300
		}
		if fuzzy.Match(q, lower) {
			return 100
		}
		return 0
	}

	// Multi-word: every word must independently match the branch name.
	// Dashes/slashes in branch names act as word separators, so "doc intel"
	// should match "implement-doc-intelligence".
	minScore := 250
	for _, word := range words {
		if strings.Contains(lower, word) {
			// exact substring — keep current score
		} else if fuzzy.Match(word, lower) {
			minScore = min(minScore, 100) // weaker match lowers overall score
		} else {
			return 0 // any word missing → no match
		}
	}
	return minScore
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
		// Truncate path if needed
		if maxWidth > 0 && len(p) > 30 {
			p = "…" + p[len(p)-29:]
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

// --- Bubble Tea model ---

const (
	headerLines = 4 // title + blank + filter + blank
	helpLines   = 1
	minVPHeight = 3
)

type filterableSelector struct {
	filter   textinput.Model
	viewport viewport.Model
	all      []Item
	filtered []Item
	cursor   int
	result   *Item
	quitting bool
	width    int
	height   int
}

func (m *filterableSelector) Init() tea.Cmd {
	return textinput.Blink
}

func (m *filterableSelector) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
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
			m.filter.SetValue("")
			m.filtered = m.all
			m.cursor = 0
			m.viewport.GotoTop()

		case "up":
			if m.cursor > 0 {
				m.cursor--
			} else {
				m.cursor = max(0, len(m.filtered)-1)
			}
			m.scrollToCursor()

		case "down":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			} else {
				m.cursor = 0
			}
			m.scrollToCursor()

		default:
			m.filter, cmd = m.filter.Update(msg)
			m.filtered = FilterItems(m.all, m.filter.Value())
			if m.cursor >= len(m.filtered) {
				m.cursor = 0
			}
			m.scrollToCursor()
			return m, cmd
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.filter.Width = msg.Width - 3
		vpH := max(msg.Height-headerLines-helpLines, minVPHeight)
		m.viewport.Width = msg.Width
		m.viewport.Height = vpH
	}
	return m, cmd
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
	m.scrollToCursor()
	b.WriteString(m.viewport.View())
	b.WriteString(dimStyle.Render("\n↑/↓: navigate • enter: select • esc: clear • q: quit"))
	return b.String()
}

// ShowSelection runs the interactive TUI and returns the selected item (or nil if cancelled).
func ShowSelection(items []Item) (*Item, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no items available")
	}

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
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return nil, err
	}
	if fm, ok := final.(*filterableSelector); ok {
		return fm.result, nil
	}
	return nil, nil
}
