package help

import (
	"fmt"
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
)

// Item is a single key binding and description in a help section.
type Item struct {
	Key  string
	Desc string
}

// Section groups related help items under a title.
type Section struct {
	Title string
	Items []Item
}

// Palette contains the colors needed to render help content.
type Palette struct {
	HeaderFG string
	HeaderBG string
	KeyFG    string
	DescFG   string
	SearchFG string
	SearchBG string
}

// Render converts help sections into styled terminal content.
func Render(sections []Section, palette Palette, search *regexp.Regexp) string {
	headerStyle, keyStyle, descStyle := styles(palette)
	lines := make([]string, 0, len(sections)*4)
	for i, section := range sections {
		lines = append(lines, headerStyle.Render(section.Title))
		for _, item := range section.Items {
			lines = append(lines, formatLine(item.Key, item.Desc, keyStyle, descStyle))
		}
		if i < len(sections)-1 {
			lines = append(lines, "")
		}
	}

	if search != nil {
		for i, line := range lines {
			if search.MatchString(line) {
				lines[i] = highlightLine(line, palette, search)
			}
		}
	}

	return strings.Join(lines, "\n")
}

// Lines returns plain text lines for searchable help content.
func Lines(sections []Section) []string {
	lines := make([]string, 0, len(sections)*4)
	for i, section := range sections {
		lines = append(lines, section.Title)
		for _, item := range section.Items {
			lines = append(lines, fmt.Sprintf("%s: %s", item.Key, item.Desc))
		}
		if i < len(sections)-1 {
			lines = append(lines, "")
		}
	}
	return lines
}

func styles(palette Palette) (lipgloss.Style, lipgloss.Style, lipgloss.Style) {
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(palette.HeaderFG)).
		Background(lipgloss.Color(palette.HeaderBG)).
		Padding(0, 1)

	keyStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(palette.KeyFG))

	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(palette.DescFG))

	return headerStyle, keyStyle, descStyle
}

func formatLine(key, desc string, keyStyle, descStyle lipgloss.Style) string {
	paddedKey := fmt.Sprintf("%-12s", key)
	return keyStyle.Render(paddedKey) + " " + descStyle.Render(desc)
}

func highlightLine(line string, palette Palette, search *regexp.Regexp) string {
	matches := search.FindAllStringIndex(line, -1)
	if len(matches) == 0 {
		return line
	}

	highlightStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(palette.SearchBG)).
		Foreground(lipgloss.Color(palette.SearchFG))

	highlighted := line
	offset := 0
	for _, match := range matches {
		start := match[0] + offset
		end := match[1] + offset
		rendered := highlightStyle.Render(highlighted[start:end])
		highlighted = highlighted[:start] + rendered + highlighted[end:]
		offset += len(rendered) - (end - start)
	}

	return highlighted
}
