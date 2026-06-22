package help

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestLinesFlattensSectionsForSearch(t *testing.T) {
	sections := []Section{
		{
			Title: "Navigation",
			Items: []Item{
				{Key: "j", Desc: "move down"},
				{Key: "k", Desc: "move up"},
			},
		},
		{
			Title: "General",
			Items: []Item{
				{Key: "q", Desc: "quit"},
			},
		},
	}

	got := Lines(sections)
	want := []string{
		"Navigation",
		"j: move down",
		"k: move up",
		"",
		"General",
		"q: quit",
	}

	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("Lines() mismatch\nwant:\n%s\ngot:\n%s", strings.Join(want, "\n"), strings.Join(got, "\n"))
	}
}

func TestRenderHighlightsSearchMatches(t *testing.T) {
	sections := []Section{
		{
			Title: "Search",
			Items: []Item{
				{Key: "/", Desc: "search tasks"},
				{Key: "n", Desc: "next match"},
			},
		},
	}
	palette := Palette{
		HeaderFG: "15",
		HeaderBG: "8",
		KeyFG:    "14",
		DescFG:   "250",
		SearchFG: "0",
		SearchBG: "11",
	}

	base := Render(sections, palette, nil)
	got := Render(sections, palette, regexp.MustCompile("search"))
	if !strings.Contains(ansi.Strip(got), "search tasks") {
		t.Fatalf("rendered help omitted matching item: %q", got)
	}
	if got == base {
		t.Fatalf("search highlight did not change rendered help")
	}

	noMatch := Render(sections, palette, regexp.MustCompile("not-present"))
	if noMatch != base {
		t.Fatalf("non-matching search changed rendered help\nbase: %q\nnoMatch: %q", base, noMatch)
	}
}
