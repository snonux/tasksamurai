package ui

import (
	"reflect"
	"testing"
)

func TestWordWrapPreservesASCIIWrapping(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		width int
		want  []string
	}{
		{
			name:  "wraps at word boundary",
			text:  "alpha beta gamma delta",
			width: 12,
			want:  []string{"alpha beta", "gamma delta"},
		},
		{
			name:  "normalizes ascii whitespace",
			text:  "alpha   beta\ngamma",
			width: 10,
			want:  []string{"alpha beta", "gamma"},
		},
		{
			name:  "keeps over-width word intact",
			text:  "alphabetagamma delta",
			width: 8,
			want:  []string{"alphabetagamma", "delta"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wordWrap(tt.text, tt.width)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("wordWrap(%q, %d) = %#v, want %#v", tt.text, tt.width, got, tt.want)
			}
		})
	}
}

func TestWordWrapCountsUTF8Runes(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		width int
		want  []string
	}{
		{
			name:  "accented text fits by rune count",
			text:  "café latte",
			width: 10,
			want:  []string{"café latte"},
		},
		{
			name:  "cjk text fits by rune count",
			text:  "漢字 test",
			width: 7,
			want:  []string{"漢字 test"},
		},
		{
			name:  "emoji text fits by rune count",
			text:  "fix 😀 bug",
			width: 9,
			want:  []string{"fix 😀 bug"},
		},
		{
			name:  "wraps multibyte text when rune count exceeds width",
			text:  "café latte crème",
			width: 10,
			want:  []string{"café latte", "crème"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wordWrap(tt.text, tt.width)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("wordWrap(%q, %d) = %#v, want %#v", tt.text, tt.width, got, tt.want)
			}
		})
	}
}
