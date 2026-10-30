# AGENTS.md

Guidance for coding agents working in this repository.

## Project overview

TaskSamurai is a Go terminal UI for Taskwarrior, built with Bubble Tea. Optional
Jira Cloud issues merge into the same table and ultra view. Taskwarrior remains
required; an absent default config or absent `[jira]` section disables Jira.

## Development commands

```bash
mage build          # default mage target also builds
mage run
mage test
mage verify         # gofmt, errcheck, ordinary tests and Mage tests
mage install
mage clean

./tasksamurai --debug-log debug.log
./tasksamurai --browser-cmd chromium
./tasksamurai --disco
./tasksamurai --config /path/to/config.toml
./tasksamurai --no-jira
```

Optional live Jira checks need real credentials and dedicated projects:

```bash
export JIRA_TEST_SITE=https://snonux.atlassian.net
export JIRA_TEST_EMAIL=your-atlassian-account@example.com
export JIRA_TEST_TOKEN_FILE=~/.jira_api_token
export JIRA_TEST_PROJECT=TST
export JIRA_TEST_SECOND_PROJECT=TST2
mage jiracheck
JIRA_INTEGRATION=1 JIRA_TEST_UNBOUNDED=1 mage jiratest
```

`jiracheck` only reads authentication, project and create-metadata endpoints.
`jiratest` needs two distinct projects and explicit opt-in. The optional
`JIRA_TEST_UNBOUNDED` probe asserts this site's restriction; leave it unset on
other sites. `mage build`, `mage test` and `mage verify` never enable live Jira
tests. Do not claim skipped integration tests validate live behavior.

## Go conventions

Use value semantics where practical, consistent receiver styles, context as the
first parameter for blocking operations, wrapped errors, and focused consumer
interfaces. Keep code under `internal/`, entry points under `cmd/`, format with
gofmt/goimports, and run errcheck. The local full reference, when available, is
`~/.agents/skills/go-best-practices/SKILL.md`.

## Architecture

- `cmd/tasksamurai/main.go`: flags, config loading, optional Jira client wiring
  and Bubble Tea startup.
- `internal/config/`: strict TOML loading, path/default selection, validation and
  credential-file rereading. Configuration values never retain the token.
- `internal/task/`: Taskwarrior CLI operations, JSON parsing, sorting and stats.
  `task.Task` also carries optional Jira display/source metadata.
- `internal/jira/`: bounded, contextual REST requests; mapping, comments/ADF,
  transitions, field edits, worklogs and saved filters. Reads may retry once; writes
  never automatically retry.
- `internal/ui/table.go`: main `Model`, message dispatch and table rendering.
  `keyhandlers.go` and `keyactions.go` define bindings/actions; `handlers.go`
  handles inputs, and `detail_handlers.go` handles detail fields.
- `internal/ui/asyncreload.go`, `asyncmutate.go`, `asyncdelete.go` and
  `taskflight.go`: the Taskwarrior single-flight pipelines.
- `internal/ui/jirareload.go`, `jiradetail.go`, `jiramutation.go`, `jirafields.go`
  and `jirafilters.go`: independent Jira caches, requests and completion messages.
  `jirasource.go` owns consumer interfaces; `jiraguard.go` owns routing policy.
- `internal/atable/`: table sizing, scrolling and cell rendering.
- `internal/ui/theme.go`: themes and disco mode.

Network results change issue/filter caches only in Bubble Tea's Update loop.
Commands capture immutable snapshots. Jira and Taskwarrior request contexts remain independent;
quit cancels outstanding work. Fetch generations reject stale reads, and filter
epochs distinguish A→B→A selection changes during writes.

## Important invariants

- Jira identity is the `jira:KEY` UUID prefix. Numeric display IDs are synthetic,
  unique and stable for a Jira session; never send them or Jira UUIDs to `task`.
  `IDText` supplies issue keys in all views, including completed issues.
- Keep the routing policy and every activation/commit/editor/delayed-action
  boundary consistent. A route-policy change alone does not enable a write.
- Jira search caches refetch on startup, space, auto-refresh and filter changes.
  View cycling and ordinary navigation reuse them. Detail opening refreshes
  only that issue and its first comment page; writes patch from confirmed
  results, with one per-issue GET after a 204 field/transition response.
- Same-filter refresh failures retain last-good rows. A changed filter clears
  old rows, and late writes must not cancel the new filter's fetch.
- `f` and tag toggles filter Taskwarrior. `F` selects Jira JQL; `6` cycles sources.
  The global `:` prompt and `+` remain Taskwarrior operations.
- Keep credentials, Basic auth headers and raw token contents out of model/debug
  dumps and diagnostics. Token-file reads trim surrounding whitespace only.
- Preserve real Jira labels separately from the virtual `jira` display tag.
  Priority writes use High/Medium/Low; summaries are strings, comments are ADF.
- Ctrl+w logs Jira work through the shared serialized write pipeline; never
  retry worklog creation automatically or adjust remaining estimates implicitly.
  Keep total logged time in the shared annotation/Logged cell, ultra and details.
- Map statuses by category `new`/`indeterminate`/`done`, never custom status names.
  Use `resolutiondate`, falling back to `updated`, for completed-window timing.

## Testing

Tests live alongside implementation files. There are three Jira layers:

1. Unit/fixture tests cover config, mapping, routing, cache races and rendering.
2. `httptest` servers cover real HTTP payloads, pagination, auth and failures.
3. `//go:build integration` tests opt into the live site and both test projects.

Tests that invoke Taskwarrior use isolated temporary data directories and skip
when `task` is unavailable. Run packages rather than individual source files:

```bash
go test ./internal/task
go test ./internal/ui
go test -race ./internal/jira ./internal/ui -run 'Test(Jira|Routing|SetField)'
JIRA_INTEGRATION=0 go test -tags integration ./internal/jira
```

Live fixtures use an exact per-run summary marker. Re-fetch and check project,
key, UUID and marker before each mutation or deletion. Cleanup touches only the
current run: delete best-effort, or resolve with a warning when deletion is
forbidden. Never sweep other runs or real user issues. The configured account
returned DELETE 403 during verification; resolved marked fixtures remain in
`TST` and `TST2`.

See `README.md` for user controls and `docs/jira-support.md` for the implemented
design, deviations and verification record.
