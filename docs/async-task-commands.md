# Plan: Run all external Taskwarrior commands asynchronously

**Status:** plan only — do not implement from this document without a follow-up task.  
**Motivation:** `task` can be slow on large databases. Today most calls run
inside Bubble Tea's `Update` path and block the entire TUI event loop until
`task` exits (also noted in `docs/debugging.md` under genuine hangs).

## Current state

### Partial async today (do not treat as finished)

| Path | What is non-blocking | What still blocks `Update` |
|------|----------------------|----------------------------|
| `:prompt` / shell (`shellRunCmd`) | `task` line runs in a `tea.Cmd` | `handleShellDone` → sync `reloadAndReport()` / `Export` |
| Shell completion load | load runs in a `tea.Cmd` | seven `task _*` readers inside that Cmd (OK off-Update, but can overlap writers) |
| Open URL (`o`) | browser launch via `tea.Cmd` | N/A — not a Taskwarrior command |
| Open file ref in `$EDITOR` | `tea.ExecProcess` | **Suspends the TUI** (foreground). Not a pattern for async `task`. |
| `task edit` | `tea.ExecProcess` | Suspends TUI; `handleEditDone` then sync-reloads |
| Description editor | `tea.ExecProcess` | Suspends TUI; `handleDescEditDone` may sync `SetDescriptionContext` + reload |

`tea.ExecProcess` is **out of scope** as a reuse pattern for keeping the UI
responsive: it hands the terminal to a child and freezes the Bubble Tea loop
by design. Reuse only the `shellRunCmd` style: `tea.Cmd` + result message,
with **no** sync `task`/`export` left in the `*DoneMsg` handler.

`internal/task.RunArgs` already accepts `context.Context` and uses
`exec.CommandContext`, so process cancellation exists; the UI often does not
drive it asynchronously end-to-end.

### Still synchronous on the `Update` goroutine

These call `taskwarriorClient().…` and/or `reload()` / `reloadAndReport()`
directly from key/input/`*DoneMsg`/blink handlers:

| Area | Call sites (representative) |
|------|-----------------------------|
| Start / stop | `handleToggleStart` → mutate + reload, then blink |
| Done | `finishBlinkImmediately`, `advanceRowBlink` (Done when `markDone`) |
| **Every blink end** | `advanceRowBlink` / `finishBlinkImmediately` **always** `reloadAndReport()` even when `markDone == false` |
| Delete + series | `deleteTaskWithUndo` → `RecurringSeries` + N× `SetStatusUUIDContext` |
| Undo | `handleUndo` (+ optional per-UUID `Export`) |
| Due set/clear/random | `handleRemoveDueDate`, `handleRandomDueDate`, `handleDueEditMode` |
| Priority / project / tags / desc / annotate / recur | `handlers.go` enter handlers via `handleTextInput` |
| Recurring series recur | UI Enter → `SetRecurringSeriesRecurrenceContext` (multi-`task` loop inside `internal/task`) |
| Tag → project | `handleTagToProject` |
| Add task | add-mode Enter (`AddLineContext`) |
| Filter apply / agent toggle / search apply / search clear | sync reload (`handlers.go`, `keyhandlers.go`) |
| Manual refresh / auto-refresh | `reloadAndReport` → `Export` |
| Shell / edit completion | `handleShellDone`, `handleEditDone`, `handleDescEditDone` |
| Startup | `NewWithTaskwarrior` → sync `reload()` before `program.Start` (blocks process start, not `Update`) |

Every mutation is typically **mutate + full `task export` reload**. On a huge
DB, the export dominates latency even when the mutation itself is cheap. Many
flows then blink and **export again** at blink end — a hidden second freeze.

### Important nuance: two meanings of “async”

1. **Bubble Tea async** — do not wait on `task`/`export` inside `Update`
   (including inside `*DoneMsg` handlers); return a `tea.Cmd` instead.
2. **Overlapping / concurrent `task` processes** — Taskwarrior’s on-disk DB
   is not designed for concurrent writers. Parallel mutations race and can
   corrupt or confuse undo.

This plan targets (1). Writers (and overlapping export vs write) must stay
**serialized** via a single-flight gate — including paths that are already
partially async today (`shellRunCmd`).

## What end-to-end async would mean

### User-visible behavior

- Hotkeys and typing keep working while `task` / `export` runs (no frozen frame).
- Status line shows in-flight work (e.g. `Starting…`, `Reloading…`).
- Errors still surface the same way (`showError` / timed status).
- Quit cancels in-flight work via context and **ignores late result msgs**.
- Startup either shows a loading shell quickly, or keeps a sync first load
  (documented as Phase E).

### Architectural meaning

Single UI-side pattern (same family as `shellRunCmd`, not `ExecProcess`):

```text
key/input / blink-end / *DoneMsg handler
  → set flight / status (optional optimistic UI — not in v1)
  → return tea.Cmd with value snapshots only (never close over *Model)
  → Cmd runs task op and/or export with ctx; returns data+err msg
  → Update handles msg (check gen token)
       → clear flight, processTasks/renderTasks, blink, etc.
```

Hard rule: **Cmds must not touch `*Model`.** Pass value snapshots (`filters`,
ultra filter IDs, op args, `Taskwarrior` client). Return `reloadData` / error.
Only `Update` calls `processTasks` / `renderTasks`. Negative test: helpers that
accept `*Model` are forbidden.

Do **not** sprinkle ad-hoc goroutines outside `tea.Cmd`.

### Serialization meaning (Phase A prerequisite)

Introduce an explicit **single-flight gate** on `Model` before converting more
paths to Cmds:

- At most one in-flight pipeline that runs `task` (mutate and/or export),
  including: mutating Cmds, reload Cmds, `shellRunCmd`, and (recommended)
  completion loads — or document completion as accepted read-only overlap with
  Taskwarrior risk.
- Prefer a small enum (`idle` / `mutating` / `reloading` / `shell`) plus label
  over a lone bool that forgets shell/blink.
- Second action while in flight (v1): **reject** with status `Busy: …`
  (no multi-op queue).
- Auto-refresh must skip when `inFlight || blinkID != 0`. Do **not** rely on
  `shellActive`: it is cleared **before** `shellRunCmd` is returned, so it is
  false while the shell `task` is still running. The flight gate is what
  covers in-flight shell. Today’s `anyInputActive()` also does **not** cover
  blink.

Without this gate, turning more work into `tea.Cmd` *increases* DB race risk
relative to today’s mostly-serial `Update` blocking.

## Recommended design

### 1. Shared command helpers in `internal/ui`

```go
// Pseudocode — illustrative only; no *Model parameters
func taskMutateCmd(parent context.Context, timeout time.Duration, op func(ctx context.Context) error) tea.Cmd
func taskReloadCmd(parent context.Context, tw task.Taskwarrior, snap reloadSnapshot) tea.Cmd
func taskMutateThenReloadCmd(...) tea.Cmd  // common path today
```

`reloadSnapshot` holds filters + ultra IDs (whatever `fetchTasks` needs) copied
by value at schedule time.

Result messages carry `gen int`, payload, and `err`. Prefer bundling
mutate+reload in one Cmd when that matches today’s ordering.

### 2. Stale results / cancel semantics

- Add `taskOpGen` (like `autoRefreshGen`): every schedule bumps gen; handlers
  ignore msgs with mismatched gen.
- Quit: `cancelTaskOperations()` + ignore further task msgs. Note: Bubble Tea
  does not abort Cmd goroutines by itself; cancellation only works through
  `CommandContext` / derived ctx.
- Mid-session cancel (if ever used): **recreate** parent `taskContext` —
  today’s `initTaskContext` no-ops when non-nil, so a cancelled ctx would poison
  later ops.
- Tests: superseded reload, quit-during-op, busy reject.

### 3. Convert call sites in phases

| Phase | Scope | Notes |
|-------|--------|-------|
| **0** | Single-flight gate + status + gen token; put **existing** `shellRunCmd` + completions behind it; snapshot client in completion Cmds; extend `debug_dump` with flight/gen | Prerequisite — do before adding more Cmds. Dump belongs here (stuck Busy looks like blink wedge). |
| **A** | Async reload for `r`, auto-refresh, agent toggle, search apply/clear; async reload in `handleShellDone` / `handleEditDone` | Highest latency; paths that already return `tea.Cmd` or do not use `handleTextInput`. **Filter apply waits for Phase B** (it uses `onEnter` and cannot return a Cmd yet). |
| **B** | `handleTextInput` redesign + filter apply (with snapshot restore) + single-task mutations: start/stop, due, priority, tags, desc, annotate, project, recur, add, tag→project, desc-edit apply | Start blink **only from success `*DoneMsg`** — today’s Enter handlers `return …, startBlink` and would drop a mutate Cmd if left as-is. |
| **C** | Blink completion: optional Done + **always-async reload**; blink-disabled immediate path | Today **every** blink ends with sync reload — not Done-only. Arm flight **before** clearing `blinkID`, then return Cmd (see below). |
| **D** | Delete series + undo as one gated Cmd each | Long critical sections; rollback + stack consistency |
| **E** | Optional async startup + README / `debugging.md` polish | Empty/loading UI + initial reload Cmd |

Phases C/D are highest regression risk (blink wedge history + undo).

### 3b. Blink-end flight arming order (Phase C)

While `blinkID != 0`, keys go to `handleBlinkingState` (nav only). When blink
ends asynchronously:

1. Set `inFlight` (and label/gen) **first**
2. Clear `blinkID` / blink fields
3. Return the Done and/or reload `tea.Cmd`

If `blinkID` is cleared before flight is armed, there is a window where
neither blink routing nor the gate blocks `d`/`s`/`U`. Auto-refresh skip
alone does not close that window.

### 4. `handleTextInput` redesign (required for Phase B)

Today `onEnter func(string) error` mutates+reloads **inside** `Update` before
`onExit`. You cannot return a `tea.Cmd` from that callback.

Decision for implementers:

- Change the pattern so Enter schedules a Cmd (e.g. `onEnter` returns
  `(tea.Cmd, error)` or a dedicated “commit” helper returns `tea.Cmd`).
- Define when to `Blur` / clear editing flags: either leave the field in a
  read-only/busy state until DoneMsg, or exit edit mode immediately and rely
  on the flight gate (prefer **exit edit mode + busy status**, matching
  today’s “mode cleared on Enter” feel).
- **Do not** `return model, m.startBlink(...)` on Enter after scheduling a
  mutate Cmd — that drops the Cmd. Start blink only from the success
  `*DoneMsg` (after reload data is applied), same as other post-op UX.
- Multi-step Enter ops (`AddTags`+`RemoveTags`, tag→project,
  `ReplaceAnnotations`) must be **one** gated Cmd.

### 5. Keep `internal/task` mostly sync

Async is a **UI concern**. Package-level multi-step helpers stay sync
functions invoked from a single `tea.Cmd` so the gate covers the whole series.

### 6. Done / undo timing (Phase C/D)

Current code pushes undo **before** `DoneContext` and can leave an undo entry
even if Done fails. Async must not preserve that blindly.

Specify:

- Push undo only on **successful** Done, in the DoneMsg handler (or inside the
  Cmd result after success — applied in Update).
- Reject `U` while a flight is active.
- Phase D: define partial multi-UUID failure, rollback, and stack consistency;
  `rollbackUndoRestores` today uses `context.Background()` — switch to a
  timeout tied to quit policy so rollback is not immortal.

**Phase D consistency (implemented):**

- Delete and undo each run as **one** `taskFlightMutating` Cmd (mutate + export).
- **Delete partial failure:** roll back already-deleted UUIDs via
  `rollbackUndoRestores(quitCtx, …)` with `WithTimeout(quitCtx,
  taskOperationTimeout)` — cancellable on quit, not `context.Background()`.
  Undo stack is unchanged on mutate failure; pushed only in `*DoneMsg` after
  successful deletes (also when reload fails after a successful delete, so `U`
  can still restore).
- **Undo partial failure:** roll already-restored UUIDs back to pre-undo status
  (`deleted` for delete undos, `completed` for done undos). Stack is popped
  only on successful `undoActionDoneMsg` (snapshot taken at schedule time;
  entry stays on the stack while in flight).
- Cmds take value snapshots only; Update checks gen + `taskFlightMutating`.

### 7. Filter apply (Phase B detail)

Snapshot previous filters → schedule reload Cmd with new filters → on success
apply+render; on error **restore the snapshot** (prefer previous filters, not
unconditional `nil` as today’s lossy path).

### 8. Timeouts

- Keep `taskOperationTimeout` (30s) for normal ops.
- Keep longer shell timeout (2m) for `:prompt`.
- Every Cmd uses derived ctx so quit can kill `task`.

### 9. Testing strategy

- Controllable fake delays / block channels: status/`inFlight` set before
  completion; second mutation rejected; busy cleared on error; gen mismatch
  ignored.
- Negative: cancelled context, `task` failure mid series, reload failure after
  successful mutate, Cmd helper must not take `*Model`.
- Blink: clearing `blinkID` and tick scheduling remain correct when reload/Done
  move into Cmds (no blink wedge).
- Guard test or grep-oriented check: no sync `Export` / mutate from UI Update
  paths once phases claim complete (except documented Phase E constructor).

### 10. Docs / debug dump

- Extend `debug_dump.go` in **Phase 0** with flight enum/label, op gen,
  in-flight kind next to mode flags (stuck Busy looks like blink wedge).
- Update `docs/debugging.md` and README in Phase E (or earlier if useful):
  mode-flag wedging vs old sync-`task` freeze vs stuck `inFlight`.

## Explicit non-goals

- Not replacing Taskwarrior with a direct DB library.
- Not parallelizing writers.
- Not changing Taskwarrior CLI / `rc.*` defaults beyond what exists.
- Not optimistic local table edits without reload (later optimization).
- Not using `tea.ExecProcess` to “async” CLI `task` calls.

## Risks and open decisions

| Topic | Options | Recommendation |
|-------|---------|----------------|
| Second keypress while busy | Ignore / queue 1 / queue N | Ignore with status (v1) |
| Auto-refresh during blink/flight | Skip | Skip when `inFlight \|\| blinkID != 0` (not `shellActive`) |
| Completion vs gate | Gate / accept overlap | Gate in v1; snapshot `Taskwarrior` client into Cmd (today’s completion Cmd closes over `*Model`) |
| Failed mutate + optimistic UI | N/A | No optimistic edits in v1 |
| Non-Done blink trailing reload | Keep / drop if mutate already reloaded | Decide in Phase C; today often double-exports |
| Done undo push | Before / after success | After success only |
| Startup | Sync first load vs async | Keep sync until Phase E |

## Success criteria (future implementation)

1. No `task` / `export` wait remains inside `Update` — including `*DoneMsg`
   handlers and blink completion — except a documented Phase E constructor
   load if deferred.
2. Single-flight gate covers mutations, reloads, shell, and completion loads
   (or an explicit risk waiver for completions only).
3. Full suite green; tests for busy gating, gen staleness, text-input commit
   Cmds, blink+async reload, delete/undo failure paths.
4. Manual check on a large DB: mash `s` / `r` / `d` / `:…` — UI stays
   responsive; final table matches `task export`.

## Suggested follow-up task breakdown

1. `+async` Phase 0: single-flight gate + gen + wire shell/completions + debug_dump  
2. `+async` Phase A: async reload for r/auto-refresh/agent/search + shell/edit Done (not filter)  
3. `+async` Phase B: text-input redesign + filter + single-task mutate-then-reload (blink from DoneMsg)  
4. `+async` Phase C: blink completion (arm flight before clear blinkID; Done + always-async reload)  
5. `+async` Phase D: delete series + undo  
6. `+async` Phase E (optional): async startup + README/debugging docs  

Do not start implementation until those tasks (or an explicit “implement the
plan”) are created and prioritized.
