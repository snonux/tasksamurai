package ui

import (
	"math/rand"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type fwModel struct {
	width          int
	height         int
	start          time.Time
	fws            []firework
	ignoreFirstKey bool
}

type firework struct {
	x, y  int
	frame int
}

type tickMsg struct{}

func tick() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m fwModel) Init() tea.Cmd {
	m.start = time.Now()
	// Ignore the first key in case the exit key from the previous program
	// is still buffered when the fireworks start.
	m.ignoreFirstKey = true
	return tick()
}

var frames = [][]string{
	{"*"},
	{" * ", "* *", " * "},
	{"  *  ", " * * ", "*   *", " * * ", "  *  "},
}

func (m fwModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tickMsg:
		if time.Since(m.start) > 5*time.Second {
			return m, tea.Quit
		}
		// advance frames
		for i := 0; i < len(m.fws); {
			m.fws[i].frame++
			if m.fws[i].frame >= len(frames) {
				m.fws = append(m.fws[:i], m.fws[i+1:]...)
			} else {
				i++
			}
		}
		// maybe create new firework
		if m.width > 0 && m.height > 0 {
			if rand.Float64() < 0.4 {
				x := rand.Intn(m.width)
				y := rand.Intn(m.height)
				m.fws = append(m.fws, firework{x: x, y: y})
			}
		}
		return m, tick()
	case tea.KeyMsg:
		if !m.ignoreFirstKey {
			m.ignoreFirstKey = true
			return m, nil
		}
		return m, tea.Quit
	}
	return m, nil
}

func (m fwModel) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	grid := make([][]rune, m.height)
	for i := range grid {
		row := make([]rune, m.width)
		for j := range row {
			row[j] = ' '
		}
		grid[i] = row
	}
	for _, fw := range m.fws {
		fr := frames[fw.frame]
		oy := fw.y - len(fr)/2
		for dy, line := range fr {
			y := oy + dy
			if y < 0 || y >= m.height {
				continue
			}
			ox := fw.x - len([]rune(line))/2
			for dx, r := range line {
				x := ox + dx
				if x < 0 || x >= m.width {
					continue
				}
				if r != ' ' {
					grid[y][x] = r
				}
			}
		}
	}
	var b strings.Builder
	for _, row := range grid {
		b.WriteString(string(row))
		b.WriteByte('\n')
	}
	return b.String()
}

// Fireworks runs a short fireworks animation. It stops after five seconds or
// when a key is pressed.
func Fireworks() {
	rand.Seed(time.Now().UnixNano())
	p := tea.NewProgram(fwModel{}, tea.WithAltScreen())
	p.Run()
}
