package ui

import (
	"strconv"
	"testing"
)

func TestContrastColorUsesHighContrastForeground(t *testing.T) {
	for bg := 0; bg < 256; bg++ {
		fg := contrastColor(strconv.Itoa(bg))
		if fg != "0" && fg != "15" {
			t.Fatalf("contrastColor(%d) = %q, want black or white foreground", bg, fg)
		}

		r, g, b := xtermRGB(bg)
		var ratio float64
		if fg == "0" {
			ratio = contrastRatio(r, g, b, 0, 0, 0)
		} else {
			ratio = contrastRatio(r, g, b, 255, 255, 255)
		}
		if ratio < 4.5 {
			t.Fatalf("contrastColor(%d) = %q with contrast ratio %.2f, want >= 4.5", bg, fg, ratio)
		}
	}
}

func TestContrastColorFallsBackForInvalidInput(t *testing.T) {
	tests := []string{"not-a-color", "-1", "256"}
	for _, input := range tests {
		if got := contrastColor(input); got != "15" {
			t.Fatalf("contrastColor(%q) = %q, want 15", input, got)
		}
	}
}
