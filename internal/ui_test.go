package internal

import (
	"testing"
)

func TestFuzzyRank(t *testing.T) {
	cases := []struct {
		branch   string
		query    string
		wantZero bool // true = expect no match
		wantMin  int  // if not zero, score must be >= this
	}{
		// Single-word cases
		{branch: "main", query: "main", wantMin: 1000},
		{branch: "feature/foo", query: "feature", wantMin: 500},
		{branch: "feature/foo-bar", query: "foo", wantMin: 300},
		{branch: "implement-auth", query: "auth", wantMin: 300},
		{branch: "xyzfoo", query: "nope", wantZero: true},

		// Multi-word: space-separated terms match dash-separated branch names
		{branch: "worktree-feature-AB#69128-implement-doc-intelligence", query: "doc intel", wantMin: 100},
		{branch: "worktree-feature-AB#69128-implement-doc-intelligence", query: "implement doc", wantMin: 100},
		{branch: "feature/add-user-auth", query: "user auth", wantMin: 100},
		{branch: "fix/payment-gateway-timeout", query: "payment timeout", wantMin: 100},

		// Multi-word: partial miss should not match
		{branch: "feature/foo-bar", query: "foo baz", wantZero: true},
		{branch: "main", query: "doc intel", wantZero: true},
	}

	for _, tc := range cases {
		t.Run(tc.query+"→"+tc.branch, func(t *testing.T) {
			score := fuzzyRank(tc.branch, tc.query)
			if tc.wantZero {
				if score != 0 {
					t.Errorf("expected no match, got score %d", score)
				}
				return
			}
			if score < tc.wantMin {
				t.Errorf("expected score >= %d, got %d", tc.wantMin, score)
			}
		})
	}
}

func TestFilterItems(t *testing.T) {
	items := []Item{
		{Branch: "main", Score: 0.9},
		{Branch: "worktree-feature-AB#69128-implement-doc-intelligence", Score: 0.5},
		{Branch: "feature/user-auth", Score: 0.3},
		{Branch: "fix/unrelated-thing", Score: 0.2},
	}

	got := FilterItems(items, "doc intel")
	if len(got) == 0 {
		t.Fatal("expected at least one match for 'doc intel', got none")
	}
	if got[0].Branch != "worktree-feature-AB#69128-implement-doc-intelligence" {
		t.Errorf("expected doc-intelligence branch first, got %q", got[0].Branch)
	}
	// unrelated branch should not appear
	for _, item := range got {
		if item.Branch == "fix/unrelated-thing" {
			t.Errorf("unrelated branch should not match 'doc intel'")
		}
	}
}
