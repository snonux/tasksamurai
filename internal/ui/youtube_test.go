package ui

import "testing"

// TestIsYouTubeURL verifies that only real YouTube video hosts are recognised,
// covering the canonical domains and short link, subdomains, and cases where
// "youtube.com" merely appears in a path or query string of another host.
func TestIsYouTubeURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want bool
	}{
		{"youtube.com watch", "https://youtube.com/watch?v=abc123", true},
		{"www.youtube.com", "https://www.youtube.com/watch?v=abc123", true},
		{"m.youtube.com", "https://m.youtube.com/watch?v=abc123", true},
		{"youtu.be short link", "https://youtu.be/abc123", true},
		{"uppercase host", "https://WWW.YOUTUBE.COM/watch?v=abc", true},
		{"plain http", "http://youtube.com/", true},
		{"not youtube", "https://example.com/watch?v=abc123", false},
		{"youtube in path only", "https://example.com/youtube.com/x", false},
		{"youtube in query only", "https://example.com/?u=youtube.com", false},
		{"lookalike host", "https://notyoutube.com/watch", false},
		{"empty", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isYouTubeURL(tc.url); got != tc.want {
				t.Fatalf("isYouTubeURL(%q) = %v, want %v", tc.url, got, tc.want)
			}
		})
	}
}

// TestBrowserForURL verifies the browser-selection logic: YouTube links use the
// configured alternative browser only when one is set, and every other URL (as
// well as YouTube links with no override) falls back to the default browser.
func TestBrowserForURL(t *testing.T) {
	cases := []struct {
		name         string
		browser      string
		youtube      string
		url          string
		wantSelected string
	}{
		{"youtube with override", "firefox", "chromium", "https://youtu.be/x", "chromium"},
		{"youtube without override", "firefox", "", "https://youtu.be/x", "firefox"},
		{"non-youtube with override set", "firefox", "chromium", "https://example.com", "firefox"},
		{"non-youtube no override", "firefox", "", "https://example.com", "firefox"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &Model{browserCmd: tc.browser, youtubeBrowserCmd: tc.youtube}
			if got := m.browserForURL(tc.url); got != tc.wantSelected {
				t.Fatalf("browserForURL(%q) = %q, want %q", tc.url, got, tc.wantSelected)
			}
		})
	}
}

// TestSetYouTubeBrowserCmdTrimsWhitespace verifies that a blank flag value is
// treated as unset so YouTube links keep using the default browser.
func TestSetYouTubeBrowserCmdTrimsWhitespace(t *testing.T) {
	m := &Model{browserCmd: "firefox"}
	m.SetYouTubeBrowserCmd("   ")
	if m.youtubeBrowserCmd != "" {
		t.Fatalf("blank value should be empty, got %q", m.youtubeBrowserCmd)
	}
	if got := m.browserForURL("https://youtu.be/x"); got != "firefox" {
		t.Fatalf("browserForURL = %q, want firefox", got)
	}

	m.SetYouTubeBrowserCmd("  chromium  ")
	if m.youtubeBrowserCmd != "chromium" {
		t.Fatalf("value should be trimmed to chromium, got %q", m.youtubeBrowserCmd)
	}
}
