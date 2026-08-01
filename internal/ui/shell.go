package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"codeberg.org/snonux/tasksamurai/internal/task"
)

const shellCommandTimeout = 2 * time.Minute

func shellRunCmd(parent context.Context, tw task.Taskwarrior, line string, selectedID int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, shellCommandTimeout)
		defer cancel()

		result, err := tw.RunShellLine(ctx, line)
		return shellDoneMsg{result: result, err: err, selectedID: selectedID}
	}
}

func (m *Model) shellCommandContext() context.Context {
	m.initTaskContext()
	return m.taskContext
}

// shellEditLaunchMsg is emitted once the temp file holding the :prompt text
// has been written; Update responds by suspending the TUI and launching
// $EDITOR on it.
type shellEditLaunchMsg struct {
	tempFile string
	err      error
}

// shellEditDoneMsg is emitted when the editor launched from the : prompt
// finishes. tempFile is removed by the handler after reading the content
// back into the prompt.
type shellEditDoneMsg struct {
	err      error
	tempFile string
}

// prepareShellEditCmd writes the current prompt text to a temp file and
// returns a shellEditLaunchMsg so Update can launch $EDITOR on it. Splitting
// the launch into a prepare step (returns a msg) and an exec step mirrors the
// description-editor flow and keeps tea.ExecProcess at the top level.
func prepareShellEditCmd(content string) tea.Cmd {
	return func() tea.Msg {
		f, err := os.CreateTemp("", "tasksamurai-shell-*.txt")
		if err != nil {
			return shellEditLaunchMsg{err: err}
		}
		if _, err := f.WriteString(content); err != nil {
			f.Close()
			os.Remove(f.Name())
			return shellEditLaunchMsg{err: err}
		}
		name := f.Name()
		if err := f.Close(); err != nil {
			os.Remove(name)
			return shellEditLaunchMsg{err: err}
		}
		return shellEditLaunchMsg{tempFile: name}
	}
}

// launchShellEditorCmd suspends the TUI and runs $EDITOR on the temp file.
// When the editor exits it emits a shellEditDoneMsg carrying the temp path.
func launchShellEditorCmd(tempFile string) tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	c := exec.Command(editor, tempFile)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return shellEditDoneMsg{err: err, tempFile: tempFile}
	})
}

// handleShellEditDone reads the edited temp file back into the : prompt,
// keeping the prompt open so the user can review and press Enter to run.
// Taskwarrior commands are single-line, so any newlines the editor
// introduced are collapsed to spaces.
func (m *Model) handleShellEditDone(msg shellEditDoneMsg) (tea.Model, tea.Cmd) {
	if msg.tempFile != "" {
		defer os.Remove(msg.tempFile)
	}
	if msg.err != nil {
		m.showError(fmt.Errorf("opening editor: %w", msg.err))
		return m, nil
	}
	data, err := os.ReadFile(msg.tempFile)
	if err != nil {
		m.showError(fmt.Errorf("reading edited prompt: %w", err))
		return m, nil
	}
	line := strings.ReplaceAll(string(data), "\r\n", " ")
	line = strings.ReplaceAll(line, "\r", " ")
	line = strings.ReplaceAll(line, "\n", " ")
	line = strings.TrimSpace(line)
	m.shellInput.SetValue(line)
	m.shellInput.CursorEnd()
	m.refreshShellSuggestions()
	if line == "" {
		m.statusMsg = "Editor saved an empty prompt"
	} else {
		m.statusMsg = "Edited in $EDITOR \u2014 press Enter to run"
	}
	return m, nil
}

func (m *Model) handleShellPrompt() (tea.Model, tea.Cmd) {
	return m.openShellPrompt("")
}

func (m *Model) handleShellPromptForSelectedTask() (tea.Model, tea.Cmd) {
	uuid := m.shellSelectedTaskUUID()
	if uuid == "" {
		return m.handleShellPrompt()
	}
	return m.openShellPrompt(uuid + " ")
}

func (m *Model) openShellPrompt(value string) (tea.Model, tea.Cmd) {
	m.clearEditingModes()
	m.shellActive = true
	m.shellInput.SetValue(value)
	m.shellInput.CursorEnd()
	m.shellInput.Focus()
	m.refreshShellSuggestions()
	m.updateTableHeight()
	return m, m.loadShellCompletionsCmd()
}

func (m *Model) handleShellMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		line := strings.TrimSpace(m.shellInput.Value())
		if line == "" {
			m.shellActive = false
			m.shellInput.Blur()
			m.updateTableHeight()
			return m, nil
		}

		selectedID := m.shellSelectedTaskID()
		m.shellHistory = append(m.shellHistory, line)
		m.shellActive = false
		m.shellInput.Blur()
		m.updateTableHeight()
		return m, shellRunCmd(m.shellCommandContext(), m.taskwarriorClient(), line, selectedID)
	case "esc":
		m.shellActive = false
		m.shellInput.Blur()
		m.updateTableHeight()
		return m, nil
	case "ctrl+o":
		// Pop the current prompt text out into $EDITOR. The TUI suspends while
		// the editor runs; on return the edited text is loaded back into the
		// prompt (still open) so the user can review and press Enter to run.
		return m, prepareShellEditCmd(m.shellInput.Value())
	case "tab":
		m.refreshShellSuggestions()
		if len(m.shellCompletion.Commands) == 0 {
			return m, m.loadShellCompletionsCmd()
		}
	}

	var cmd tea.Cmd
	m.shellInput, cmd = m.shellInput.Update(msg)
	m.refreshShellSuggestions()
	return m, cmd
}

func (m *Model) handleShellDone(msg shellDoneMsg) (tea.Model, tea.Cmd) {
	if !m.reloadAndReport() {
		return m, nil
	}
	if msg.selectedID > 0 {
		_ = m.selectTaskByID(msg.selectedID)
	}

	output := shellOutput(msg.result, msg.err)
	if strings.TrimSpace(output) == "" {
		if msg.err != nil {
			m.showError(msg.err)
		} else {
			m.statusMsg = fmt.Sprintf("task %s completed", strings.Join(msg.result.Args, " "))
		}
		return m, nil
	}

	m.showShellOutput(shellTitle(msg.result, msg.err), output)
	return m, nil
}

func (m *Model) handleShellCompletion(msg shellCompletionMsg) (tea.Model, tea.Cmd) {
	m.shellCompletion = msg.sources
	m.shellCompletionLoad = false
	if m.shellActive {
		m.refreshShellSuggestions()
	}
	return m, nil
}

func (m *Model) handleShellOutputMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "enter":
		m.shellOutputVisible = false
		return m, nil
	case "up", "k":
		m.shellOutputViewport.ScrollUp(1)
	case "down", "j":
		m.shellOutputViewport.ScrollDown(1)
	case "pgup", "b":
		m.shellOutputViewport.PageUp()
	case "pgdown", "space":
		m.shellOutputViewport.PageDown()
	case "g", "home":
		m.shellOutputViewport.GotoTop()
	case "G", "end":
		m.shellOutputViewport.GotoBottom()
	}
	return m, nil
}

func (m *Model) renderShellOutputScreen() string {
	width := m.tbl.Width()
	if width <= 0 {
		width = 80
	}
	height := m.windowHeight - 2
	if height < 1 {
		height = 1
	}

	m.shellOutputViewport.SetWidth(width)
	m.shellOutputViewport.SetHeight(height)

	title := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.theme.StatusFG)).
		Background(lipgloss.Color(m.theme.StatusBG)).
		Width(width).
		Render(m.shellOutputTitle)
	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.theme.StatusFG)).
		Background(lipgloss.Color(m.theme.StatusBG)).
		Width(width).
		Render("Esc/q/Enter close | j/k scroll | PgUp/PgDn page")
	return lipgloss.JoinVertical(lipgloss.Left, title, m.shellOutputViewport.View(), footer)
}

func (m *Model) showShellOutput(title, output string) {
	width := m.tbl.Width()
	if width <= 0 {
		width = 80
	}
	height := m.windowHeight - 2
	if height < 1 {
		height = 1
	}

	m.shellOutputVisible = true
	m.shellOutputTitle = title
	m.shellOutputViewport = viewport.New(viewport.WithWidth(width), viewport.WithHeight(height))
	m.shellOutputViewport.SetContent(strings.TrimRight(output, "\n"))
}

func (m *Model) shellSelectedTaskID() int {
	if m.showUltra {
		id, err := m.getUltraSelectedTaskID()
		if err == nil {
			return id
		}
		return 0
	}
	id, err := m.getSelectedTaskID()
	if err == nil {
		return id
	}
	return 0
}

func (m *Model) shellSelectedTaskUUID() string {
	if m.showUltra {
		tasks := m.ultraTaskList()
		if m.ultraCursor < 0 || m.ultraCursor >= len(tasks) {
			return ""
		}
		return strings.TrimSpace(tasks[m.ultraCursor].UUID)
	}

	tsk := m.getTaskAtCursor()
	if tsk == nil {
		return ""
	}
	return strings.TrimSpace(tsk.UUID)
}

func (m *Model) refreshShellSuggestions() {
	m.shellInput.ShowSuggestions = true
	m.shellInput.SetSuggestions(m.shellLineSuggestions())
}

func (m *Model) loadShellCompletionsCmd() tea.Cmd {
	if m.shellCompletionLoad || len(m.shellCompletion.Commands) > 0 {
		return nil
	}
	m.shellCompletionLoad = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return shellCompletionMsg{sources: m.taskwarriorClient().LoadCompletionSources(ctx)}
	}
}

func (m *Model) shellLineSuggestions() []string {
	value := m.shellInput.Value()
	start, end, token := shellTokenAt(value, m.shellInput.Position())
	replacementTokens := m.shellReplacementTokens(token, shellTokenIndex(value, start))
	if len(replacementTokens) == 0 {
		return nil
	}

	prefix := string([]rune(value)[:start])
	suffix := string([]rune(value)[end:])
	suggestions := make([]string, 0, len(replacementTokens))
	seen := make(map[string]struct{})
	for _, replacement := range replacementTokens {
		candidate := prefix + replacement + suffix
		if candidate == value {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(candidate), strings.ToLower(value)) {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		suggestions = append(suggestions, candidate)
	}
	return suggestions
}

func (m *Model) shellReplacementTokens(token string, tokenIndex int) []string {
	var out []string
	commandPosition := tokenIndex == 0 || (tokenIndex == 1 && shellFirstTokenIsTask(m.shellInput.Value()))
	if commandPosition {
		if token != "" && strings.HasPrefix(strings.ToLower("task"), strings.ToLower(token)) {
			out = append(out, "task")
		}
		if !strings.Contains(token, ":") && !strings.HasPrefix(token, "+") && !strings.HasPrefix(token, "-") {
			out = append(out, matchingShellValues(token, m.shellCompletion.Commands)...)
		}
	}
	out = append(out, m.attributeCompletions(token)...)
	out = append(out, m.tagCompletions(token)...)
	out = append(out, matchingShellValues(token, m.shellCompletion.IDs)...)
	out = append(out, matchingShellValues(token, m.shellCompletion.UUIDs)...)
	return out
}

func (m *Model) attributeCompletions(token string) []string {
	if strings.Contains(token, ":") {
		key, value, _ := strings.Cut(token, ":")
		switch strings.ToLower(key) {
		case "project", "proj":
			return prefixedValues(key+":", value, m.shellCompletion.Projects)
		case "status":
			return prefixedValues(key+":", value, []string{"pending", "completed", "deleted", "waiting", "recurring"})
		case "priority", "pri":
			return prefixedValues(key+":", value, []string{"H", "M", "L"})
		}
		return nil
	}

	keys := append([]string(nil), m.shellCompletion.Columns...)
	keys = append(keys, m.shellCompletion.UDAs...)
	for i, key := range keys {
		keys[i] = key + ":"
	}
	return matchingShellValues(token, keys)
}

func (m *Model) tagCompletions(token string) []string {
	if !strings.HasPrefix(token, "+") && !strings.HasPrefix(token, "-") {
		return nil
	}
	sign := token[:1]
	prefix := strings.TrimPrefix(token[1:], "#")
	var tags []string
	for _, tag := range m.shellCompletion.Tags {
		tag = strings.TrimPrefix(tag, "#")
		tags = append(tags, sign+tag)
	}
	return matchingShellValues(sign+prefix, tags)
}

func shellTokenAt(value string, pos int) (int, int, string) {
	runes := []rune(value)
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}

	start := pos
	for start > 0 && !unicode.IsSpace(runes[start-1]) {
		start--
	}
	end := pos
	for end < len(runes) && !unicode.IsSpace(runes[end]) {
		end++
	}
	return start, end, string(runes[start:end])
}

func shellTokenIndex(value string, tokenStart int) int {
	prefix := string([]rune(value)[:tokenStart])
	return len(strings.Fields(prefix))
}

func shellFirstTokenIsTask(value string) bool {
	fields := strings.Fields(value)
	return len(fields) > 0 && fields[0] == "task"
}

func matchingShellValues(prefix string, values []string) []string {
	var matches []string
	for _, value := range values {
		if strings.HasPrefix(strings.ToLower(value), strings.ToLower(prefix)) {
			matches = append(matches, value)
		}
	}
	return matches
}

func prefixedValues(prefix, valuePrefix string, values []string) []string {
	var matches []string
	for _, value := range values {
		if strings.HasPrefix(strings.ToLower(value), strings.ToLower(valuePrefix)) {
			matches = append(matches, prefix+value)
		}
	}
	return matches
}

func shellOutput(result task.RunResult, err error) string {
	var parts []string
	if err != nil {
		parts = append(parts, "Error: "+err.Error())
	}
	if strings.TrimSpace(result.Stdout) != "" {
		parts = append(parts, strings.TrimRight(result.Stdout, "\n"))
	}
	if strings.TrimSpace(result.Stderr) != "" {
		stderr := strings.TrimRight(result.Stderr, "\n")
		if err == nil || !strings.Contains(err.Error(), strings.TrimSpace(result.Stderr)) {
			parts = append(parts, stderr)
		}
	}
	return strings.Join(parts, "\n\n")
}

func shellTitle(result task.RunResult, err error) string {
	status := "output"
	if err != nil {
		status = "error"
	}
	command := strings.Join(result.Args, " ")
	if command == "" {
		command = "(empty)"
	}
	return fmt.Sprintf("task %s | %s", command, status)
}
