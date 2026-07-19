package ui

import (
	"os"
	"path/filepath"
	"testing"

	"codeberg.org/snonux/tasksamurai/internal/task"
)

// TestExtractFileRef verifies that @path/to/file.txt style references are
// parsed out of free text (and that non-references such as email addresses are
// ignored).
func TestExtractFileRef(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"plain reference", "see @notes/todo.txt for details", "notes/todo.txt"},
		{"reference at start", "@path/to/file.txt is here", "path/to/file.txt"},
		{"absolute path", "check @/etc/hosts now", "/etc/hosts"},
		{"trailing punctuation", "open @docs/readme.md.", "docs/readme.md"},
		{"email is not a reference", "mail paul@example.com please", ""},
		{"no reference", "nothing to open here", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractFileRef(tc.text); got != tc.want {
				t.Fatalf("extractFileRef(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}

// TestResolveFileRefPathTilde verifies that a leading "~" expands to the user's
// home directory while relative paths are left untouched.
func TestResolveFileRefPathTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory available: %v", err)
	}

	if got := extractFileRef("edit @~/notes.txt today"); got != filepath.Join(home, "notes.txt") {
		t.Fatalf("tilde path = %q, want %q", got, filepath.Join(home, "notes.txt"))
	}
	if got := extractFileRef("edit @relative/notes.txt"); got != "relative/notes.txt" {
		t.Fatalf("relative path = %q, want %q", got, "relative/notes.txt")
	}
}

// TestFindTaskFileRefFallsBackToAnnotations verifies that the description is
// scanned first and annotations are used only when the description has no
// reference.
func TestFindTaskFileRefFallsBackToAnnotations(t *testing.T) {
	desc := task.Task{Description: "open @main.go"}
	if got := findTaskFileRef(&desc); got != "main.go" {
		t.Fatalf("description ref = %q, want %q", got, "main.go")
	}

	ann := task.Task{
		Description: "no file here",
		Annotations: []task.Annotation{{Description: "see @docs/spec.md"}},
	}
	if got := findTaskFileRef(&ann); got != "docs/spec.md" {
		t.Fatalf("annotation ref = %q, want %q", got, "docs/spec.md")
	}

	none := task.Task{Description: "plain text"}
	if got := findTaskFileRef(&none); got != "" {
		t.Fatalf("no ref = %q, want empty", got)
	}
}

// TestFindTaskURLPrecedence documents that URL detection is independent from
// file references so handleOpenURL can prefer URLs when both are present.
func TestFindTaskURLPrecedence(t *testing.T) {
	both := task.Task{Description: "see https://example.com and @notes.txt"}
	if got := findTaskURL(&both); got != "https://example.com" {
		t.Fatalf("url = %q, want https://example.com", got)
	}
	if got := findTaskFileRef(&both); got != "notes.txt" {
		t.Fatalf("file ref = %q, want notes.txt", got)
	}
}
