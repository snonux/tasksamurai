# Debugging a stuck or unresponsive session

Task Samurai occasionally ends up in a state where hotkeys stop doing
anything — for example, pressing `d` has no visible effect. This is rarely a
true hang (the process is still running, just not reacting the way you
expect). This document explains why that happens, how to capture a full
diagnostic dump when it does, and how to read the result.

## Why hotkeys can stop working

Key handling in `internal/ui/table.go`'s `Update()` is gated by a chain of
mode flags checked in order, roughly:

1. Is a row/field currently blinking (`blinkID`)?
2. Is the shell output panel open (`shellOutputVisible`)?
3. Is the task detail overlay open (`showTaskDetail`)?
4. Is the help screen open (`showHelp`)?
5. Is ultra mode active (`showUltra`)?
6. Is any inline editing mode active — annotate, description, tags, due
   date, recurrence, project, priority, filter, add-task, search, shell
   prompt, help search (`handleEditingModes`)?
7. Otherwise: normal mode (`handleNormalMode`), where `d` and friends live.

If any flag earlier in that chain is left `true` by an error path that
forgot to reset it, every keypress gets routed to that stuck mode instead of
normal mode — `d` doesn't do nothing, it's being silently consumed
somewhere else. This is functionally different from a goroutine deadlock,
and a goroutine dump alone won't show it: the UI goroutine is still running
fine, it's just dispatching input to the wrong place.

Note that the very first gate, `blinkID`, is the one with no visual tell:
the table looks completely normal, arrow keys/`j`/`k` still move the cursor
(`handleBlinkingState` forwards navigation), and every other key is dropped.
Two ways to strand it were fixed in `startBlink`/`handleBlinkMsg` — a blink
started for a task with no row in the current table, and a detail-view blink
swallowing an in-flight row blink's tick — so if you see it again, it is a
third path that leaves `blinkID` set with no `blinkCmd()` tick scheduled.

The other possibility is a genuine hang — the Bubble Tea event loop itself
blocked on something (an external `task` command, an editor subprocess, a
channel op) — which *does* show up as a blocked goroutine.

The diagnostics below cover both: a goroutine/profile dump for genuine
hangs, and a UI state dump for the stuck-mode-flag case.

## Building with diagnostics enabled

Runtime diagnostics are compiled out of normal builds (`task build` /
`go build ./cmd/tasksamurai`) to keep production binaries lean and to avoid
carrying pprof/signal-handling machinery into every build. Opt in with the
`debugsignals` build tag:

```bash
go build -tags debugsignals -o tasksamurai ./cmd/tasksamurai
```

Keep a `debugsignals`-tagged binary around for whenever this kind of issue
comes up — it behaves identically to a normal build until you signal it.

Not available on Windows: signal handling relies on `SIGUSR1`/`SIGUSR2`,
which don't exist there. On Windows, reach for `GODEBUG` environment
variables or attach a debugger instead.

## Capturing a dump

While the session you want to inspect is still running (**don't kill it
first**):

```bash
pgrep -f tasksamurai              # find the PID
kill -SIGUSR1 <pid>               # goroutine stacks + UI state snapshot
kill -SIGUSR2 <pid>               # + heap/cpu/block profiles (adds ~5s pause)
```

By default, files are written to the current working directory. Point them
somewhere stable with `--debug-dir`:

```bash
tasksamurai --debug-dir=/tmp/tasksamurai-debug
```

### SIGUSR1 — goroutine stacks + UI state (the first thing to try)

Writes two files:

- `tasksamurai-goroutines-<timestamp>.txt` — every goroutine's stack trace.
  Look here for a genuine hang: a goroutine blocked in a syscall (external
  `task` command or editor not responding) or blocked reading stdin.
- `tasksamurai-state-<timestamp>.txt` — a snapshot of the running model's
  own fields: every mode flag listed above (in the order `Update()` checks
  them), task/table counts and cursor position, and window/config state.
  **This is the one to check first for the "`d` does nothing" symptom** —
  whichever flag is unexpectedly `true` is almost always the answer.

This snapshot is captured safely: the signal handler doesn't reach into the
model's fields directly (that would race with the UI goroutine mutating
them). Instead it sends a message into the same Bubble Tea event loop that
handles every keypress, so the dump runs on the model's own goroutine, in
between ordinary updates, using the same code path as everything else.

### SIGUSR2 — full profile dump (for performance/memory issues)

Adds:

- `tasksamurai-<timestamp>-goroutines.txt` — goroutine stacks (again, text)
- `tasksamurai-<timestamp>-heap.pprof` — memory allocations
- `tasksamurai-<timestamp>-cpu.pprof` — a 5-second CPU profile (the process
  pauses to sample this — expect a brief freeze)
- `tasksamurai-<timestamp>-block.pprof` — lock/channel contention events

Analyze with `go tool pprof`:

```bash
go tool pprof tasksamurai-TIMESTAMP-heap.pprof
go tool pprof -top tasksamurai-TIMESTAMP-cpu.pprof
go tool pprof -web tasksamurai-TIMESTAMP-cpu.pprof   # requires graphviz
```

## Example workflow

1. Notice hotkeys aren't responding. **Don't kill the process.**
2. Build a `debugsignals` binary if you don't already have one, or make sure
   you're running one.
3. `pgrep -f tasksamurai` to find the PID.
4. `kill -SIGUSR1 <pid>`.
5. Open `tasksamurai-state-<timestamp>.txt` first:
   - A mode flag that's `true` when it shouldn't be (e.g.
     `tagsEditing=true` after you finished editing tags, or
     `blinkID` stuck non-zero) is your answer — that's the code path to fix.
   - Everything `false`/idle but keys still don't work? Move on to the
     goroutine dump.
6. Open `tasksamurai-goroutines-<timestamp>.txt` and look for a goroutine
   blocked in a syscall, exec/wait, or channel receive that never resolves.
7. Still unclear, or looks CPU/memory related? `kill -SIGUSR2 <pid>` and
   work through the pprof files.

## Extending the state dump

If a future bug turns out to hinge on a `Model` field that isn't currently
in the snapshot, add it to the relevant `write*` helper in
`internal/ui/debug_dump.go` rather than introducing a new dump mechanism —
`dumpState()` already runs on the correct goroutine and already has a file
per session, so this is normally a one-line addition.
