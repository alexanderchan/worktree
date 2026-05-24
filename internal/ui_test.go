package internal

import "testing"

// --- Unit tests for internal components ---
// These test specific algorithms directly. If we swap an algorithm, we expect
// these to change or be replaced along with the implementation.

func TestLevenshtein(t *testing.T) {
	cases := []struct{ a, b string; want int }{
		{"", "", 0},
		{"abc", "abc", 0},
		{"lcl", "local", 2},
		{"autentication", "authentication", 1},
		{"implment", "implement", 1},
		{"kitten", "sitting", 3},
		{"baz", "bar", 1},
	}
	for _, tc := range cases {
		if got := levenshtein(tc.a, tc.b); got != tc.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestFuzzyRankInternal(t *testing.T) {
	cases := []struct {
		branch, query string
		wantZero      bool
		wantMin       int
	}{
		// Exact match scores highest
		{branch: "main", query: "main", wantMin: 1000},
		// Prefix
		{branch: "feature/foo", query: "feature", wantMin: 500},
		// Substring
		{branch: "feature/foo-bar", query: "foo", wantMin: 300},
		{branch: "implement-auth", query: "auth", wantMin: 300},
		// No match
		{branch: "xyzfoo", query: "nope", wantZero: true},
		// Multi-word via substring
		{branch: "worktree-feature-AB#69128-implement-doc-intelligence", query: "doc intel", wantMin: 100},
		// No partial multi-word match
		{branch: "feature/foo-bar", query: "foo baz", wantZero: true},
		{branch: "main", query: "doc intel", wantZero: true},
	}
	for _, tc := range cases {
		t.Run(tc.query+"→"+tc.branch, func(t *testing.T) {
			score := fuzzyRank(tc.branch, tc.query)
			if tc.wantZero && score != 0 {
				t.Errorf("expected no match, got score %d", score)
			} else if !tc.wantZero && score < tc.wantMin {
				t.Errorf("expected score >= %d, got %d", tc.wantMin, score)
			}
		})
	}
}

func TestRankItemBranchBeatsPath(t *testing.T) {
	// Two items where the same query ("auth") substring-matches one item's
	// branch and another item's path basename. The branch match should rank
	// higher so users searching by branch name still see branch-named hits
	// first.
	branchHit := Item{Branch: "feature/user-auth", Path: "/repo/.claude/worktrees/unrelated-dir"}
	pathHit := Item{Branch: "worktree-something", Path: "/repo/.claude/worktrees/fix-auth-flow"}

	got := FilterItems([]Item{pathHit, branchHit}, "auth")
	if len(got) < 2 {
		t.Fatalf("expected both items to match, got %d", len(got))
	}
	if got[0].Branch != "feature/user-auth" {
		t.Errorf("branch match should rank first, got %q", got[0].Branch)
	}
}

// --- Integration tests: algorithm-agnostic ---
// These describe WHAT the search should do, not HOW. They should survive
// algorithm swaps. A regression here means the user experience got worse.

func TestSearch(t *testing.T) {
	branches := []Item{
		{Branch: "main", Score: 0.9},
		{Branch: "worktree-feature-AB#69128-implement-doc-intelligence", Score: 0.5},
		{Branch: "feature/user-auth", Score: 0.3},
		{Branch: "fix/unrelated-thing", Score: 0.2},
		{Branch: "local-user-ft-access-documentation", Score: 0.1},
		{Branch: "feature/local-config", Score: 0.1},
		{Branch: "feature/implement-authentication", Score: 0.1},
		// Worktree whose folder name diverges from its branch name — the
		// real-world case where users create a worktree dir like
		// "improve-logging" but git names the branch something different.
		{Branch: "worktree-observability-logs-viewer", Path: "/repo/.claude/worktrees/improve-logging", Score: 0.1},
	}

	cases := []struct {
		query       string
		mustMatch   []string // at least one of these must appear in results
		mustExclude []string // none of these may appear in results
		topResult   string   // if set, this branch must be first
	}{
		{
			query:       "doc intel",
			mustMatch:   []string{"worktree-feature-AB#69128-implement-doc-intelligence"},
			mustExclude: []string{"main", "fix/unrelated-thing"},
		},
		{
			query:       "user auth",
			mustMatch:   []string{"feature/user-auth"},
			mustExclude: []string{"main", "fix/unrelated-thing"},
		},
		{
			// typo: missing 'o' and 'a' from "local" — subsequence "lcl" still in order
			query:     "lcl",
			mustMatch: []string{"local-user-ft-access-documentation", "feature/local-config"},
			mustExclude: []string{"main", "fix/unrelated-thing"},
		},
		{
			// typo: missing 'h' from "authentication"
			query:     "autentication",
			mustMatch: []string{"feature/implement-authentication"},
		},
		{
			// typo: missing 'e' from "implement"
			query:     "implment",
			mustMatch: []string{"feature/implement-authentication"},
		},
		{
			// empty query returns everything unsorted
			query:     "",
			mustMatch: []string{"main"},
		},
		{
			// exact match should rank first regardless of frecency score
			query:     "main",
			topResult: "main",
		},
		{
			// Path-basename match: "improve" only appears in the worktree
			// directory name, not in the branch — search must still find it.
			query:     "improve",
			mustMatch: []string{"worktree-observability-logs-viewer"},
		},
		{
			// Substring of the path basename ("logging") should match too.
			query:     "logging",
			mustMatch: []string{"worktree-observability-logs-viewer"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			got := FilterItems(branches, tc.query)

			// Check topResult
			if tc.topResult != "" {
				if len(got) == 0 || got[0].Branch != tc.topResult {
					top := ""
					if len(got) > 0 {
						top = got[0].Branch
					}
					t.Errorf("top result: want %q, got %q", tc.topResult, top)
				}
			}

			gotSet := make(map[string]bool, len(got))
			for _, item := range got {
				gotSet[item.Branch] = true
			}

			for _, want := range tc.mustMatch {
				if !gotSet[want] {
					t.Errorf("expected %q in results for query %q, not found", want, tc.query)
				}
			}
			for _, excl := range tc.mustExclude {
				if gotSet[excl] {
					t.Errorf("expected %q excluded from results for query %q, but it appeared", excl, tc.query)
				}
			}
		})
	}
}
