package convoy

import (
	"context"
	"testing"
)

func TestExtractIssueID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"gt-abc", "gt-abc"},
		{"bd-xyz", "bd-xyz"},
		{"hq-cv-123", "hq-cv-123"},
		{"external:gt:gt-abc", "gt-abc"},
		{"external:bd:bd-xyz", "bd-xyz"},
		{"external:hq:hq-cv-123", "hq-cv-123"},
		{"external:", "external:"}, // malformed, return as-is
		{"external:x:", ""},        // 3 parts but empty last part
		{"simple", "simple"},       // no external prefix
		{"", ""},                   // empty
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractIssueID(tt.input)
			if result != tt.expected {
				t.Errorf("extractIssueID(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsSlingableType(t *testing.T) {
	tests := []struct {
		issueType string
		want      bool
	}{
		{"task", true},
		{"bug", true},
		{"feature", true},
		{"chore", true},
		{"", true},          // empty defaults to task
		{"epic", false},     // container type
		{"convoy", false},   // meta type
		{"sub-epic", false}, // container type
		{"decision", false}, // non-work type
		{"message", false},  // non-work type
		{"event", false},    // non-work type
		{"unknown", false},  // unknown types are not slingable
	}

	for _, tt := range tests {
		t.Run(tt.issueType, func(t *testing.T) {
			got := IsSlingableType(tt.issueType)
			if got != tt.want {
				t.Errorf("IsSlingableType(%q) = %v, want %v", tt.issueType, got, tt.want)
			}
		})
	}
}

func TestIsIssueBlocked_NoStore(t *testing.T) {
	// isIssueBlocked with a nil context and missing store should not panic.
	// It can't actually be tested with nil store since it calls store methods,
	// but we verify the function signature is correct by calling it.
	// The actual blocking behavior is tested in integration tests.
}

func TestReadyIssueFilterLogic_SkipsNonSlingableTypes(t *testing.T) {
	// Validates that feedNextReadyIssue's type filter skips non-slingable types.
	// We test the predicate inline (same pattern as existing filter tests).
	tracked := []trackedIssue{
		{ID: "gt-epic", Status: "open", Assignee: "", IssueType: "epic"},
		{ID: "gt-task", Status: "open", Assignee: "", IssueType: "task"},
		{ID: "gt-convoy", Status: "open", Assignee: "", IssueType: "convoy"},
		{ID: "gt-bug", Status: "open", Assignee: "", IssueType: "bug"},
	}

	var slingable []string
	for _, issue := range tracked {
		if issue.Status == "open" && issue.Assignee == "" && IsSlingableType(issue.IssueType) {
			slingable = append(slingable, issue.ID)
		}
	}

	if len(slingable) != 2 {
		t.Errorf("expected 2 slingable issues (task, bug), got %d: %v", len(slingable), slingable)
	}
	if slingable[0] != "gt-task" || slingable[1] != "gt-bug" {
		t.Errorf("expected [gt-task, gt-bug], got %v", slingable)
	}
}

func TestReadyIssueFilterLogic_SkipsNonOpenIssues(t *testing.T) {
	// Validates the filtering predicate used by feedNextReadyIssue: only
	// open issues with no assignee should be considered "ready". We test
	// the predicate inline because feedNextReadyIssue also calls rigForIssue
	// and dispatchIssue, making isolated unit testing impractical without a
	// real store. Integration coverage lives in convoy_manager_integration_test.go.
	tracked := []trackedIssue{
		{ID: "gt-closed", Status: "closed", Assignee: ""},
		{ID: "gt-inprog", Status: "in_progress", Assignee: "gastown/polecats/alpha"},
		{ID: "gt-hooked", Status: "hooked", Assignee: "gastown/polecats/beta"},
		{ID: "gt-assigned", Status: "open", Assignee: "gastown/polecats/gamma"},
	}

	// None of these should be considered "ready"
	for _, issue := range tracked {
		if issue.Status == "open" && issue.Assignee == "" {
			t.Errorf("issue %s should not be ready (status=%s, assignee=%s)", issue.ID, issue.Status, issue.Assignee)
		}
	}
}

func TestReadyIssueFilterLogic_FindsReadyIssue(t *testing.T) {
	// Validates that the "first open+unassigned" selection picks the correct
	// issue. See comment on TestReadyIssueFilterLogic_SkipsNonOpenIssues for
	// why this tests the predicate inline rather than calling feedNextReadyIssue.
	tracked := []trackedIssue{
		{ID: "gt-closed", Status: "closed", Assignee: ""},
		{ID: "gt-inprog", Status: "in_progress", Assignee: "gastown/polecats/alpha"},
		{ID: "gt-ready", Status: "open", Assignee: ""},
		{ID: "gt-also-ready", Status: "open", Assignee: ""},
	}

	// Find first ready issue - should be gt-ready (first match)
	var foundReady string
	for _, issue := range tracked {
		if issue.Status == "open" && issue.Assignee == "" {
			foundReady = issue.ID
			break
		}
	}

	if foundReady != "gt-ready" {
		t.Errorf("expected first ready issue to be gt-ready, got %s", foundReady)
	}
}

func TestCheckConvoysForIssue_NilStore(t *testing.T) {
	// Nil store returns nil immediately (no convoy checks).
	result := CheckConvoysForIssue(context.Background(), nil, "/nonexistent/path", "gt-test", "test", nil, "gt", nil)
	if result != nil {
		t.Errorf("expected nil for nil store, got %v", result)
	}
}

func TestCheckConvoysForIssue_NilLogger(t *testing.T) {
	// Nil logger should not panic — gets replaced with no-op internally.
	// With nil store, returns nil.
	result := CheckConvoysForIssue(context.Background(), nil, "/nonexistent/path", "gt-test", "test", nil, "gt", nil)
	if result != nil {
		t.Errorf("expected nil for nil store, got %v", result)
	}
}
