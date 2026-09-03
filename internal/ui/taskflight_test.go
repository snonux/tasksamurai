package ui

import (
	"context"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/snonux/tasksamurai/internal/task"
)

func TestBeginTaskFlightSupersedesCompleting(t *testing.T) {
	m := Model{}
	m.initTaskContext()
	oldCtx := m.taskContext
	if !m.beginTaskFlight(taskFlightCompleting, "") {
		t.Fatal("expected completing flight to start")
	}
	m.shellCompletionLoad = true
	completingGen := m.taskOpGen

	if !m.beginTaskFlight(taskFlightShell, "Running task…") {
		t.Fatal("shell should supersede completing")
	}
	if m.taskFlight != taskFlightShell {
		t.Fatalf("taskFlight = %s, want shell", m.taskFlight)
	}
	if m.shellCompletionLoad {
		t.Fatal("superseding completing should clear shellCompletionLoad")
	}
	if m.taskOpGen == completingGen {
		t.Fatal("expected taskOpGen to bump when superseding")
	}
	if oldCtx.Err() == nil {
		t.Fatal("supersede should cancel the previous task context")
	}
	if m.taskContext == oldCtx || m.taskContext.Err() != nil {
		t.Fatal("supersede should install a fresh live task context")
	}
	if m.matchesTaskOpGen(completingGen) {
		t.Fatal("stale completing gen should not match")
	}
}

func TestBeginTaskFlightRejectsSecondBlockingFlight(t *testing.T) {
	m := Model{}
	if !m.beginTaskFlight(taskFlightShell, "Running task…") {
		t.Fatal("expected shell flight to start")
	}
	if m.beginTaskFlight(taskFlightReloading, "Reloading…") {
		t.Fatal("second blocking flight should be rejected")
	}
	if !m.rejectIfBusy() {
		t.Fatal("rejectIfBusy should be true during shell flight")
	}
	if !strings.Contains(m.statusMsg, "Busy:") {
		t.Fatalf("statusMsg = %q, want Busy prefix", m.statusMsg)
	}
}

func TestShellCompletionLoadCmdDoesNotCloseOverModel(t *testing.T) {
	tw := &completionSourcesTaskwarrior{}
	cmd := shellCompletionLoadCmd(context.Background(), tw, 7)
	msg := cmd()
	got, ok := msg.(shellCompletionMsg)
	if !ok {
		t.Fatalf("got %T, want shellCompletionMsg", msg)
	}
	if got.gen != 7 {
		t.Fatalf("gen = %d, want 7", got.gen)
	}
	if !tw.called {
		t.Fatal("expected LoadCompletionSources to be called on the snapshotted client")
	}
}

type completionSourcesTaskwarrior struct {
	fakeTaskwarrior
	called bool
}

func (f *completionSourcesTaskwarrior) LoadCompletionSources(context.Context) task.CompletionSources {
	f.called = true
	return task.CompletionSources{Commands: []string{"next"}}
}

func TestShellEnterRejectedWhenBusyKeepsPrompt(t *testing.T) {
	fake := &completionSourcesTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}

	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: ':', Text: ":"})
	m = *mv.(*Model)
	if cmd == nil {
		t.Fatal("expected completion load cmd when idle")
	}
	mv, _ = (&m).Update(cmd())
	m = *mv.(*Model)

	for _, r := range "ls" {
		mv, _ = (&m).Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = *mv.(*Model)
	}
	if !m.beginTaskFlight(taskFlightReloading, "Reloading…") {
		t.Fatal("could not arm reloading flight")
	}

	mv, cmd = (&m).Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = *mv.(*Model)
	if cmd != nil {
		t.Fatal("Enter should not start shell while busy")
	}
	if !m.shellActive {
		t.Fatal("Busy Enter should keep the shell prompt open")
	}
	if !strings.Contains(m.statusMsg, "Busy:") {
		t.Fatalf("statusMsg = %q, want Busy", m.statusMsg)
	}
}

func TestStaleShellDoneMsgIgnored(t *testing.T) {
	fake := &fakeTaskwarrior{
		tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	if !m.beginTaskFlight(taskFlightShell, "Running task…") {
		t.Fatal("arm shell flight")
	}
	liveGen := m.taskOpGen
	m.invalidateTaskFlights()
	// Re-arm a newer shell flight so flight kind is shell but gen differs.
	if !m.beginTaskFlight(taskFlightShell, "Running task…") {
		t.Fatal("re-arm shell")
	}

	mv, cmd := (&m).Update(shellDoneMsg{
		result: task.RunResult{Args: []string{"ls"}},
		gen:    liveGen,
	})
	m = *mv.(*Model)
	if cmd != nil {
		t.Fatalf("stale done unexpectedly returned cmd")
	}
	if m.taskFlight != taskFlightShell {
		t.Fatalf("stale done cleared flight: %s", m.taskFlight)
	}
}

func TestShellDoneRequiresMatchingFlightKind(t *testing.T) {
	fake := &fakeTaskwarrior{
		tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	if !m.beginTaskFlight(taskFlightCompleting, "") {
		t.Fatal("arm completing")
	}
	gen := m.taskOpGen
	baselineTasks := len(m.tasks)

	mv, _ := (&m).Update(shellDoneMsg{
		result: task.RunResult{Args: []string{"ls"}},
		gen:    gen,
	})
	m = *mv.(*Model)
	if m.taskFlight != taskFlightCompleting {
		t.Fatalf("wrong-kind done cleared flight: %s", m.taskFlight)
	}
	if len(m.tasks) != baselineTasks {
		t.Fatal("wrong-kind shell done should not reload")
	}
}

func TestZeroGenShellDoneIgnored(t *testing.T) {
	m := Model{}
	if !m.beginTaskFlight(taskFlightShell, "Running task…") {
		t.Fatal("arm shell")
	}
	mv, _ := (&m).Update(shellDoneMsg{gen: 0})
	m = *mv.(*Model)
	if m.taskFlight != taskFlightShell {
		t.Fatal("gen 0 done should be ignored")
	}
}

func TestQuitInvalidatesTaskFlights(t *testing.T) {
	fake := &fakeTaskwarrior{
		tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	if !m.beginTaskFlight(taskFlightShell, "Running task…") {
		t.Fatal("arm shell")
	}
	liveGen := m.taskOpGen

	_, quitCmd := m.handleQuitKey()
	if quitCmd == nil {
		t.Fatal("want tea.Quit")
	}
	if m.taskFlightActive() {
		t.Fatalf("quit left flight active: %s", m.taskFlight)
	}
	if m.matchesTaskOpGen(liveGen) {
		t.Fatal("quit should invalidate the live gen")
	}

	mv, _ := (&m).Update(shellDoneMsg{
		result: task.RunResult{Args: []string{"ls"}},
		gen:    liveGen,
	})
	m = *mv.(*Model)
	if m.shellOutputVisible {
		t.Fatal("late shell done after quit should not apply UI")
	}
}

func TestStaleCompletionAfterSupersedeIgnored(t *testing.T) {
	fake := &completionSourcesTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	if !m.beginTaskFlight(taskFlightCompleting, "") {
		t.Fatal("arm completing")
	}
	oldGen := m.taskOpGen
	if !m.beginTaskFlight(taskFlightShell, "Running task…") {
		t.Fatal("supersede with shell")
	}

	mv, _ := (&m).Update(shellCompletionMsg{
		sources: task.CompletionSources{Commands: []string{"bogus"}},
		gen:     oldGen,
	})
	m = *mv.(*Model)
	if m.taskFlight != taskFlightShell {
		t.Fatalf("stale completion cleared shell flight: %s", m.taskFlight)
	}
	if len(m.shellCompletion.Commands) != 0 {
		t.Fatalf("stale completion applied sources: %#v", m.shellCompletion.Commands)
	}
}

func TestAutoRefreshSkipsDuringFlightAndBlink(t *testing.T) {
	fake := &countingExportTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	baseline := fake.exports
	m.autoRefresh = true
	m.autoRefreshGen = 1

	m.blinkID = 9
	mv, _ := (&m).Update(autoRefreshMsg{gen: 1})
	m = *mv.(*Model)
	if fake.exports != baseline {
		t.Fatalf("auto-refresh ran during blink: exports %d -> %d", baseline, fake.exports)
	}

	m.blinkID = 0
	if !m.beginTaskFlight(taskFlightShell, "Running task…") {
		t.Fatal("arm shell")
	}
	mv, _ = (&m).Update(autoRefreshMsg{gen: 1})
	m = *mv.(*Model)
	if fake.exports != baseline {
		t.Fatalf("auto-refresh ran during flight: exports %d -> %d", baseline, fake.exports)
	}

	m.endTaskFlight()
	mv, _ = (&m).Update(autoRefreshMsg{gen: 1})
	m = *mv.(*Model)
	if fake.exports != baseline+1 {
		t.Fatalf("auto-refresh exports = %d, want %d", fake.exports, baseline+1)
	}
}

type countingExportTaskwarrior struct {
	fakeTaskwarrior
	exports int
}

func (f *countingExportTaskwarrior) Export(ctx context.Context, filters ...string) ([]task.Task, error) {
	f.exports++
	return f.fakeTaskwarrior.Export(ctx, filters...)
}

func TestCancelCompletingFlightOnEsc(t *testing.T) {
	m := Model{}
	m.initTaskContext()
	oldCtx := m.taskContext
	if !m.beginTaskFlight(taskFlightCompleting, "") {
		t.Fatal("arm completing")
	}
	m.shellCompletionLoad = true
	m.cancelCompletingFlight()
	if m.taskFlightActive() {
		t.Fatalf("completing flight still active: %s", m.taskFlight)
	}
	if oldCtx.Err() == nil {
		t.Fatal("cancelCompletingFlight should cancel prior context")
	}
	if m.shellCompletionLoad {
		t.Fatal("shellCompletionLoad should be cleared")
	}
}

func TestBusyFlightKeyRejectsMutationsAllowsNav(t *testing.T) {
	fake := &fakeTaskwarrior{
		tasks: []task.Task{
			{ID: 1, UUID: "u1", Description: "a", Status: "pending"},
			{ID: 2, UUID: "u2", Description: "b", Status: "pending"},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	mv, _ := (&m).Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = *mv.(*Model)
	if !m.beginTaskFlight(taskFlightShell, "Running task…") {
		t.Fatal("arm shell")
	}

	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = *mv.(*Model)
	if cmd != nil {
		t.Fatal("s should not start work while busy")
	}
	if !strings.Contains(m.statusMsg, "Busy:") {
		t.Fatalf("statusMsg = %q, want Busy", m.statusMsg)
	}

	before := m.tbl.Cursor()
	mv, _ = (&m).Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = *mv.(*Model)
	if m.tbl.Cursor() == before && len(m.tasks) > 1 {
		t.Fatal("j should still navigate while busy")
	}
}

func TestBlankShellEnterCancelsCompleting(t *testing.T) {
	fake := &completionSourcesTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	mv, cmd := (&m).Update(tea.KeyPressMsg{Code: ':', Text: ":"})
	m = *mv.(*Model)
	if cmd == nil {
		t.Fatal("expected completion cmd")
	}
	// Leave completing in flight (do not deliver completion msg).
	if m.taskFlight != taskFlightCompleting {
		t.Fatalf("flight = %s, want completing", m.taskFlight)
	}
	oldCtx := m.taskContext

	mv, _ = (&m).Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = *mv.(*Model)
	if m.shellActive {
		t.Fatal("blank enter should close prompt")
	}
	if m.taskFlightActive() {
		t.Fatalf("blank enter should cancel completing, flight=%s", m.taskFlight)
	}
	if oldCtx.Err() == nil {
		t.Fatal("blank enter should cancel prior completion context")
	}
}

func TestUltraStartupQuitInvalidatesFlights(t *testing.T) {
	m := Model{ultraModeState: ultraModeState{ultraStartup: true}, ultraState: ultraState{showUltra: true}}
	m.initTaskContext()
	if !m.beginTaskFlight(taskFlightShell, "Running task…") {
		t.Fatal("arm shell")
	}
	liveGen := m.taskOpGen
	_, cmd := m.handleUltraExitKey(true)
	if cmd == nil {
		t.Fatal("want tea.Quit")
	}
	if m.matchesTaskOpGen(liveGen) {
		t.Fatal("ultra startup quit should invalidate gen")
	}
	if m.taskFlightActive() {
		t.Fatal("ultra startup quit should clear flight")
	}
}

func TestTaskFlightStateEndsCleanly(t *testing.T) {
	m := Model{}
	if !m.beginTaskFlight(taskFlightShell, "Running task…") {
		t.Fatal("arm shell")
	}
	m.endTaskFlight()
	if m.taskFlightActive() {
		t.Fatalf("flight still active: %s", m.taskFlight)
	}
	if m.rejectIfBusy() {
		t.Fatal("rejectIfBusy after end")
	}
}

func TestQuitWithSearchDuringFlightSkipsReload(t *testing.T) {
	fake := &countingExportTaskwarrior{
		fakeTaskwarrior: fakeTaskwarrior{
			tasks: []task.Task{{ID: 1, UUID: "u1", Description: "a", Status: "pending"}},
		},
	}
	m, err := NewWithTaskwarrior(nil, "firefox", fake)
	if err != nil {
		t.Fatalf("NewWithTaskwarrior: %v", err)
	}
	baseline := fake.exports
	m.searchRegex = regexp.MustCompile("a")
	if !m.beginTaskFlight(taskFlightShell, "Running task…") {
		t.Fatal("arm shell")
	}
	_, cmd := m.handleQuitKey()
	if cmd == nil {
		t.Fatal("want tea.Quit while busy with search applied")
	}
	if fake.exports != baseline {
		t.Fatalf("quit reloaded during flight: exports %d -> %d", baseline, fake.exports)
	}
	if m.taskFlightActive() {
		t.Fatal("quit should clear flight")
	}
}
