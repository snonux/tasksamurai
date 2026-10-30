# Jira support for TaskSamurai — design plan

Status: planned (not implemented). This document is the single source of truth
for the Jira integration. Task descriptions in `ask` reference sections of this
file; keep both in sync when the plan changes.

## 1. Scope and decisions (already made)

- Read-only phase 1 plus two write operations: **comment** and **open in
  browser**. Phase 2 adds transitions and field edits.
- Config file: **TOML** (`BurntSushi/toml` — the only new dependency).
- Token via **`token_file` reference** (`~/.jira_api_token`), not inline.
- Jira issue keys render in the **existing Taskwarrior ID column**.
- Issues are **merged into the main table** (no separate view).
- View-source cycle on a new configurable hotkey, default **6** (`0`–`5` are
  taken: First/Random/RandomNoDue/Track/Agent/Auto).
- Jira filter selection on hotkey **F** (lowercase `f` is the Taskwarrior
  filter; `J` is taken by "edit project").

## 2. Config file (`internal/config`, new package)

Location `~/.config/tasksamurai/config.toml`; override via `--config` flag or
`TASKSAMURAI_CONFIG` env. Precedence: flag > env > file > default. Missing
file or missing `[jira]` section → Jira disabled, current behavior unchanged.

```toml
[[jira.filters]]
name = "open"
jql = "project = X AND resolution = Unresolved ORDER BY updated DESC"

[[jira.filters]]
name = "mine"
jql = "project = X AND assignee = currentUser() ORDER BY due ASC"

[jira]
enabled       = true             # optional; default true when the section exists
site          = "https://snonux.atlassian.net"
email         = "paul.buetow@gmail.com"
token_file    = "~/.jira_api_token"
saved_filters = true             # also list the account's Jira saved filters in the picker
default_filter = "open"         # optional; name from the list, absent = "none"
default_view  = "both"           # "tw" | "both" | "jira"
max_results   = 100
timeout      = "10s"
```

- `enabled = false` (or the `--no-jira` runtime flag) switches Jira off
  without deleting the config. **One site per config in v1** — a multi-site
  `[[jira.instances]]` shape is a possible future extension, explicitly out
  of scope now.
- Recommend `chmod 600` on the config file; the token file already is 600.

- `[[jira.filters]]` (array of tables) preserves order → deterministic menu
  order. Validation: unique names, non-empty JQL, `default_filter` must
  reference an existing name, timeout parses, token file exists and non-empty.
  Malformed TOML or unreadable file → startup error naming path/line, exit.
- Token-file reading: trim only **surrounding** whitespace/newlines, never
  interior characters — the token's trailing `=13A2ACBF` is part of it
  (verified: stripping the `=…` suffix causes 401).
- Phase-2 optional overrides:

```toml
[jira.transitions]        # optional disambiguation only
done = "Close Issue"

[jira.transition_fields]   # optional defaults for screened transitions
resolution = "Done"
```

## 3. Jira client (`internal/jira`, new package)

`net/http` + `encoding/json` only. Basic auth `email:token`. All facts below
verified against the live `snonux.atlassian.net` account on 2026-10-30
(token `~/.jira_api_token`, login `paul.buetow@gmail.com`; note:
`paul@buetow.org` gives 401 — the Atlassian account email is the gmail one).

### API facts

- Search: **`POST /rest/api/3/search/jql`** (JSON body). The old
  `GET /rest/api/3/search` is gone (410 with migration notice). The new API
  paginates with `nextPageToken`, not `startAt`. Send a `fields` array limited
  to the mapped fields (`summary`, `status`, `priority`, `duedate`, `project`,
  `labels`, `updated`, `created`, `assignee`) — smaller, faster payloads.
- Comment fetch pagination: `GET /rest/api/3/issue/{key}/comment` paginates;
  fetch the first page only and render a "+n more" truncation hint in the
  detail view rather than pretending the list is complete.
- The site rejects **unbounded JQL** with 400 "Unbounded JQL queries are not
  allowed here" → configured/selected filters must include a restriction
  (e.g. `project = X AND …`).
- Comments: `POST /rest/api/3/issue/{issueKey}/comment` with **ADF** body
  (v3 requires ADF, not plain strings):

  ```json
  {"body":{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"the comment"}]}]}}
  ```

  Multi-line input → one `<paragraph>` per line. Endpoint and auth verified
  reachable (404 only for a fake issue key, which short-circuits before body
  validation).
- Fetch comments of one issue: `GET /rest/api/3/issue/{key}/comment` (used
  for the lazy-load in the detail view).
- Transitions: `GET /rest/api/3/issue/{key}/transitions` returns exactly the
  transitions available from the issue's current status (what the web UI
  uses; works with a plain user token). `POST` executes via
  `{"transition":{"id":N}}` plus `fields` when the transition has a required
  screen.
- `GET /rest/api/3/statuses`… not needed: every status carries a
  **`statusCategory.key`** ∈ {`todo`,`in-progress`,`done`} — a fixed triad
  independent of custom workflows. This is the workflow-independent
  discriminator for both mapping and transition selection.
- `GET /rest/api/3/workflow/search` happens to be readable with this token,
  but it is an admin API: **never design against it**.
- `GET /rest/api/3/filter/search` lists **saved filters** (paginated,
  `values[].name` + `values[].jql`; all filters visible to the user);
  `GET /rest/api/3/filter/my` (own filters) also works. The plain
  `GET /rest/api/3/filter` is deprecated — its error response itself points
  to the Search filters operation. Both search/my verified 200 with the
  token (0 filters on the site so far, matching 0 projects).
- The site currently has **zero projects** — a manual test project (suggested
  key `TST`) must be created once before integration tests can run.

### Error mapping (status bar, never exit, never degrade TW rows)

| Condition | Message shape |
|---|---|
| 401 | "Jira: wrong email or truncated token (the `=…` suffix is part of the token)" |
| 400 unbounded JQL | "this Jira site requires a restricted filter — press F to pick one" |
| 400 bad JQL | show the API's JQL error fragment verbatim |
| 404 | "Jira issue not found" |
| 403 | "no permission for this Jira operation" (e.g. can view but not comment) |
| 429 / 5xx | one retry with backoff, then degrade (keep last-good rows, message) |
| non-JSON body (Cloudflare HTML pages) | "unexpected response from Jira (HTTP nnn)" — never panic on `json.Unmarshal` |

- Client and config code must never log, dump or echo the token
  (incl. `Authorization` headers and the model debug dump in
  `internal/ui/debug_dump.go` — if config state is dumped, the token field
  is redacted).

### Interface seams

- `IssueSource` (for UI test fakes):

  ```go
  type IssueSource interface {
      FetchIssues(ctx context.Context, jql string) ([]task.Task, error)
      AddComment(ctx context.Context, issueKey, text string) error
      // phase 2:
      Transitions(ctx context.Context, issueKey string) ([]Transition, error)
      DoTransition(ctx context.Context, issueKey string, id string, fields map[string]string) error
      SetField(ctx context.Context, issueKey, field string, value any) error
      FetchFilters(ctx context.Context) ([]SavedFilter, error) // GET /rest/api/3/filter/search
  }
  ```

- `CreateIssue` helper is implemented early: needed by the integration test
  setup (`POST /rest/api/3/issue` with summary in ADF) and later by a
  possible `+`-creates-issue feature.

## 4. Mapping issues → `task.Task`

Reuses the existing struct so table/search/detail/ultra all work unchanged.

| Task field | Jira source | Note |
|---|---|---|
| ID column text | issue key `TS-42` | new optional string field on `task.Task` (`IDText`); when set, the ID column renders it instead of `strconv.Itoa(t.ID)`; the `✓` completed glyph stays for TW rows |
| `Description` | `fields.summary` | no prefix (key is in the ID column) |
| `UUID` | `jira:TS-42` | prefixed → collision-safe; the guard's discriminator |
| `Project` | `fields.project.key` | |
| `Status` | **`fields.status.statusCategory.key`** | `todo`→`pending`, `in-progress`→`pending` + `Start` set, `done`→`completed`; unknown category → `pending` + debug-log warning. Never map by status *name* (workflows are custom) |
| `Due` | `fields.duedate` | date-only; convert to `YYYYMMDDT000000Z` (start of day) — pinned choice |
| `Priority` | `fields.priority` | Jira name → H/M/L map, default M |
| `Tags` | `jira` + labels | the `jira` tag gives free tag filtering; the tag is virtual: exclude it from label edits |
| `Urgency` | **computed** | TW-like urgency from the mapped priority (H/M/L) + due date, so Jira rows interleave sanibly with TW rows in the default sort instead of sinking to the bottom with urgency 0 |
| `Annotations` | comments (lazy) | fetched when the detail view opens, rendered in the annotations area; first page + "+n more" hint |
| `Assignee` | `fields.assignee.displayName` | new optional string field, display-only: shown in the Jira detail view, never a column or tag |

**Resolved (done-category) Jira rows:** keep the issue **key** in the ID
  column (never the `✓` glyph — the key is their identity), apply the same
  strikethrough styling as completed TW rows, and follow the same
  completed-task window (`V`) as TW completed rows.

## 5. UI integration

### Caching and freshness

Faster navigation and fewer API calls: an **in-memory cache** on the Model
holds the last-fetched issues, the selected filter, the saved-filter list and
a last-fetched timestamp. Jira refetch happens **only** on:

1. startup (first fetch),
2. explicit `space` refresh (refetches TW + Jira issues + saved filters),
3. **periodic auto-refresh** (`Z` interval) — also refetches Jira; the
   interval is user-controlled so Jira API budget follows it, and the
   429-retry/backoff handles throttling,
4. filter selection change (new JQL requires a fetch),
5. Jira write operations — which update the cache **from the API response**
   (optimistic append, transition response), never via blanket refetches.

Explicitly *not* refetching on: post-mutation reloads (of TW rows), view
cycling (hotkey 6 filters at render — already the design), picker open,
navigation. The persistent status bar shows the age:
`[jira: mine · 14:32]`.

**Opening a Jira task's detail view is always fresh**: single
`GET /rest/api/3/issue/{key}` plus the comments fetch on open; the response
**updates the cached row**, so returning to the table shows current data for
that issue — cheap per-issue refresh semantics.

Not in v1: a disk cache for instant/offline startup (token-protected data at
rest) — deferred.

**Concurrency and startup rules:**

- The **first Jira fetch is async**: `ui.New` must never block first paint
  on the network — TW rows render immediately, Jira rows appear when the
  first fetch completes (a slow/unreachable site can only delay Jira rows,
  never the TUI).
- **Fetch coalescing**: a filter change or explicit refresh cancels/supersedes
  an in-flight Jira fetch (reuse the async reload machinery's cancellation);
  results apply with a sequence guard so a stale response can never overwrite
  newer data.
- **Token rotation without restart**: the token file is re-read on every
  explicit `space` refresh, so a rotated token takes effect without killing
  the TUI.
- **Status bar layout**: mode, filter and age compress into one token —
  `[jira: mine · 14:32]` — so it shares the status line with existing
  TW info and error messages without overflow.

### Merged table and reload

- Extend the reload pipeline (`asyncreload.go` / `exportReloadData`) to fetch
  TW tasks and Jira issues in one cycle. Jira errors degrade gracefully:
  status-bar message, keep last-good rows, TW rows unaffected.
- `ui.NewWithTaskwarrior` gains an optional `IssueSource` (nil = disabled);
  existing call sites/tests unchanged.

### View cycle (hotkey 6, configurable `-jira-hotkey`)

- States: `tw` → `both` → `jira` → `tw`… Default from config `default_view`
  (fallback `both`). Disabled Jira → hotkey no-ops.
- Both sources stay in memory; filtering happens at render → cycling is
  instant, no refetch. Status bar shows the mode briefly after toggling.
- Register in the `Hotkeys` struct + `SetHotkeys` conflict validation (pattern
  of Agent/Auto/Track), help screen entry, README table.

### Named filters and the `F` picker

- Model state: selected filter (start = `default_filter`, else "none").
  Per-session; no config write-back.
- `F` opens a lightweight list overlay (patterned on `filterInput` overlays;
  must work in table and ultra mode): first entry `(none) all issues`, then
  the named filters (`name` + dim truncated JQL hint). `j`/`k`/arrows,
  `Enter` applies, `Esc` cancels.
- On apply: async **Jira-only** refetch with the new JQL (TW rows untouched),
  status bar `jira filter: mine`. Active filter name shown persistently
  while Jira rows are visible, e.g. `[jira: mine]`.
- "(none)" = fetch with no JQL restriction; on the 400 unbounded-JQL error
  show the pick-a-filter hint and no Jira rows until a filter is chosen.
- **Fuzzy search over the picker entries**: the picker grows an embedded
  text input; typing live-filters the list with an in-repo subsequence-style
  matcher (fzf-like, **no new dependency**) matching over `name` and the
  JQL text. `j`/`k`/arrows navigate the filtered list, `Enter` applies,
  `Esc` cancels, the `(none)` entry always stays reachable (it sorts first
  and never gets filtered out entirely). Clearing the input restores the
  full list.
- **Saved Jira filters** (`saved_filters = true`, default true):
  `FetchFilters` lazily on the picker's first open, cached for the session
  (the picker gets an `r` key to refresh). The picker shows three groups:
  `(none)`, local `[[jira.filters]]`, then the account's saved filters with
  an origin marker (e.g. dim `cloud`); duplicate names across groups are
  fine because the origin distinguishes them. On fetch failure: local list
  only + one status-bar note. Saved-filter JQL can be unbounded → the same
  400 pick-a-filter handling as any other filter.
- Orthogonal to the view cycle: view controls *whether* Jira rows show,
  filter controls *which* issues are fetched.

### Commenting (`a` on a Jira row)

- The existing annotate input opens with prompt `"comment: "`; on submit the
  text becomes a Jira comment instead of `task annotate`. This is the one
  mutation Jira rows allow in v1.
- Optimistic UI: on success append to the row's local `Annotations`
  (entry = now) so the count/detail update immediately; the next reload
  reconciles. Blink machinery reused.
- Detail view lazy-loads comments (`GET …/comment`), mapping
  `created` (ISO-8601) → `DateFormat` and `body` (ADF → plain text) →
  annotation entries. In list mode Jira rows show no comment count in v1.
- Empty input refused locally (no API call).

### `o` opens the issue

- In `handleOpenURL` (`keyactions.go`), check the `jira:` address prefix
  **first**: open `{site}/browse/{key}` through the existing
  `openURLCmd`/`browserForURL` chain (atlassian.net ≠ YouTube →
  `browserCmd`). Issue URL wins over description-URL/file-ref scanning. No
  pre-validation, no extra API call. Help text:
  `"o: open URL, @file, or Jira issue in browser"`.

## 6. Control behavior matrix on Jira rows

One routing table at a single dispatch point keyed off the `jira:` prefix:
control → `{TW method | Jira method | blocked-reason}`. Blocked-reasons are
per-control data so the status bar says *why*. Table-driven unit test
enumerates every control × jira row.

### Works unchanged (global)

`H`, `q`, `esc`, `/` `?` search, `f`, `c` `C` `x` `B` `v` `Z` `V`,
`space` (now also refetches Jira), `0`–`3`, `g`/`G`, arrows, Enter → detail
view (read-only, comments lazy-loaded), ultra mode.

### Translates to Jira

Phase 1: `a` → comment; `o` → open `{site}/browse/{key}`.

Phase 2:

| Control | Jira operation | API |
|---|---|---|
| `d` done | transition to a done-category status | `GET`+`POST /transitions` |
| `s` start/stop | transition to in-progress / back to todo | same |
| `w`/`W` | set/clear `fields.duedate` | `PUT /issue/{key}` |
| `p` | set `fields.priority` (H/M/L ↔ Jira name map) | `PUT /issue/{key}` |
| `t` | edit `fields.labels` (virtual `jira` tag excluded) | `PUT /issue/{key}` |
| `e` | edit `fields.summary` | `PUT /issue/{key}` |

### Transition selection (phase 2, dynamic detection)

Detection is **lazy, at keypress time** (one extra GET per action, never
prefetched on reload). Selection: `to.statusCategory.key == "done"` (resp.
`"in-progress"`); config name override → single candidate → if still
ambiguous, pick deterministically + status-bar note *"transitioned via
'Close Issue'"*; zero candidates → polite message (e.g. `d` on a done issue).
`GET /transitions` includes each transition's field metadata: required
screen fields with a config default are sent along; otherwise error names
the fields (*"transition 'Close Issue' requires: Resolution"*).

### Blocked

| Control | Reason / message |
|---|---|
| `A` replace annotations | "comments can't be replaced wholesale — use `a` to add" |
| `D` delete | "Jira issues can't be deleted from here" |
| `U` undo | "no undo for Jira issues" |
| `R`, `ctrl+r` recurrence | "Jira issues have no recurrence" |
| `J` edit project (move issue) | "moving Jira issues not supported (yet)" |
| `T` tag→project | same family |
| `;` task prompt on jira row | "task commands don't work on Jira issues" |
| `+` add | unchanged (adds TW task); `+` creating Jira issues in jira-only view is a possible phase 3 |

For phase-2-translatable controls still blocked in v1 the message says so:
*"not yet — Jira write support for due dates is planned"*.

## 7. Testing strategy (three layers)

### 7.1 Unit/fixtures (fast, hermetic)

- `internal/config`: TOML parse (array order), `~` expansion, token-file
  trimming (surrounding whitespace only — fixture shaped like the real file
  with the `=…` suffix), every validation error, precedence flag > env > file.
- Mapping: statusCategory mapping (incl. unknown category fallback), duedate
  conversion pinned to `T000000Z`, priority/labels/tags, `jira:` UUID prefix,
  `IDText` rendering (and that the `✓` glyph still applies to TW rows).
- Comment: ADF building incl. multi-line paragraph split; empty input refused
  locally; `created` ISO → `DateFormat`.
- Routing table: every control × `jira:` row → expected outcome.

### 7.2 httptest (fake API, full negative matrix)

Auth header; pagination via `nextPageToken`; 401; 400 unbounded JQL; 400 bad
JQL fragment; 404; 429 + backoff retry; 5xx; **HTML body instead of JSON**
(never panic); missing optional fields; transition scenarios (single /
ambiguous with and without override / zero candidates / required fields
defaulted vs. error); comment round-trip; `s` on an in-progress issue;
permission-denied (can't be produced on a single-user site → fake only).

### 7.3 Integration against the live site (gated)

File `internal/jira/integration_test.go` with `//go:build integration` —
not compiled in normal `mage test`/`mage verify`/CI. A `requireIntegration(t)`
skip guard checks env even under the tag:

```bash
JIRA_INTEGRATION=1                          # explicit opt-in
JIRA_TEST_SITE=https://snonux.atlassian.net
JIRA_TEST_EMAIL=paul.buetow@gmail.com
JIRA_TEST_TOKEN_FILE=~/.jira_api_token      # default
JIRA_TEST_PROJECT=TST                       # mandatory, no default — refuse without
```

**Safety protocol:** every issue created in a run gets summary marker
`ts-integration-<unix-nano>`; tests only query
`project = TST AND summary ~ "ts-integration-"` and only mutate/delete
marked issues; `TestMain` sweeps leftovers from crashed runs (same bounded
JQL, same project fence); the guard refuses to run without
`JIRA_TEST_PROJECT`; `t.Cleanup` deletes created issues best-effort
(fallback: transition to done + warning — deletion is hygiene, not
assertion). Token never echoed; failures print response bodies, never
`Authorization` headers. Sequential, small volumes (rate limits).

**Coverage:** auth sanity (`/myself`); bounded JQL finds created issues;
**unbounded JQL → 400** (pins the site's real restriction); create → search →
map (`IDText`, statusCategory) → comment → transition todo→in-progress→done
via category detection → mapping shows completed → cleanup (end-to-end);
comment on a *resolved* issue still works; 401 with a deliberately bad
token; 404 fake key; pagination (N issues, small page size).

**Prereq:** the site currently has no projects — create project `TST`
("TaskSamurai Integration Tests") manually once.

### 7.4 New mage targets (optional, everything else unchanged)

```text
mage jiratest    # prereq-check (token file, JIRA_TEST_PROJECT set), then
                 # go test -tags integration -count=1 -v ./internal/jira/...
mage jiracheck   # dry-run: site reachable, project exists, prints the plan
```

`Test`/`Verify`/`Build` never gain tags, env lookups, or network. CI (if any)
runs `mage verify` only.

## 8. Implementation order and phases

1. `internal/config` + TOML dependency wiring (+ tests)
2. `internal/jira` client: search, mapping, error mapping, `CreateIssue`
   (+ httptest); integration harness scaffolding (build tag, mage targets)
3. `IDText` rendering change in the ID column (+ mapping tests)
4. Reload pipeline: parallel fetch + graceful degradation + merged rows
5. View cycle hotkey 6 (+ `Hotkeys` plumbing, help, README later)
6. Filter picker `F` (+ status bar), then picker fuzzy search, then saved
   Jira filters in the picker
7. Commenting via `a` (+ lazy-load comments in detail view)
8. `o` opens issue in browser
9. Routing table + blocked messages (v1 matrix complete)
10. Integration suite grows per feature (mapping → comment → end-to-end)
11. Phase 2: transitions (`d`, `s`) with dynamic detection
12. Phase 2: field edits (`w`/`W`, `p`, `t`, `e`)
13. Docs: README (hotkey table, config section, `F`/`6`), AGENTS.md update,
    this file kept as the design record

Dependency chain for agent tasks mirrors this order; see the `ask` task
list (`+jira`).