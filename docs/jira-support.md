# Jira support for TaskSamurai — design plan

Status: planned (not implemented). This document is the single source of truth
for the Jira integration. Task descriptions in `ask` reference sections of this
file; keep both in sync when the plan changes.

## 1. Scope and decisions (already made)

- Phase 1 reads issues and comments, adds **comments**, and **opens issues in
  a browser** (opening is a local action, not a Jira write). Phase 2 adds transitions and field edits.
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
`TASKSAMURAI_CONFIG` env. Config-path precedence: explicit flag > non-empty env > default path. Values
come from the selected file over built-in defaults; there are no per-field env
overrides. A missing default path disables Jira; an explicitly selected missing
path is an error. Missing
default file or missing `[jira]` section → Jira disabled, current behavior unchanged.

```toml
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

[[jira.filters]]
name = "open"
jql = "project = X AND resolution = Unresolved ORDER BY updated DESC"

[[jira.filters]]
name = "mine"
jql = "project = X AND assignee = currentUser() ORDER BY due ASC"
```

- `enabled = false` (or the `--no-jira` runtime flag) switches Jira off
  without deleting the config. **One site per config in v1** — a multi-site
  `[[jira.instances]]` shape is a possible future extension, explicitly out
  of scope now.
- Recommend `chmod 600` on the config file; the token file already is 600.

- `[[jira.filters]]` (array of tables) preserves order → deterministic menu
  order. Validation: unique names, non-empty JQL, `default_filter` must
  reference an existing name, timeout parses, token file exists and non-empty.
  Malformed TOML → startup error naming path/line; unreadable file → path
  and cause, exit. Validate enabled configs only: HTTPS origin site (no
  userinfo, query, fragment or path), email, positive max_results and timeout,
  default_view enum, filters, token file. Tests may inject an HTTP httptest
  origin at the client boundary. Unknown TOML keys are errors. --no-jira or
  enabled=false bypass Jira-specific validation and token reads (TOML syntax
  errors still fail). Defaults: both, 100, 10s, saved_filters=true. Resolve
  relative token_file paths against the config directory; expand ~/ via the
  home directory, reject unsupported ~user syntax.
- Token-file reading: trim only **surrounding** whitespace/newlines, never
  interior characters — the token's trailing `=13A2ACBF` is part of it
  (verified: stripping the `=…` suffix causes 401).
- Phase-2 optional overrides:

```toml
[jira.transitions]        # optional disambiguation only
done = "Close Issue"

[jira.transition_fields]   # optional defaults for screened transitions
resolution = { name = "Done" } # native JSON object, not a scalar string
```

## 3. Jira client (`internal/jira`, new package)

`net/http` + `encoding/json` only. Basic auth `email:token`. Historical observations below were recorded against the live `snonux.atlassian.net` account on 2026-10-30
(token `~/.jira_api_token`, login `paul.buetow@gmail.com`; note:
`paul@buetow.org` gives 401 — the Atlassian account email is the gmail one).

### API facts

- Search: **`POST /rest/api/3/search/jql`** (JSON body). The old
  `GET /rest/api/3/search` is gone (410 with migration notice). The new API
  paginates with `nextPageToken`, not `startAt`. Send a `fields` array limited
  to the mapped fields (`summary`, `status`, `priority`, `duedate`, `project`,
  `labels`, `updated`, `created`, `assignee`, `resolutiondate`) — smaller, faster payloads.
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
- Transitions: `GET /rest/api/3/issue/{key}/transitions` with `?expand=transitions.fields` returns the
  transitions available from the issue's current status (what the web UI
  uses; works with a plain user token). `POST` executes via
  `{"transition":{"id":"N"}}` plus `fields` when the transition has a required
  screen.
- `GET /rest/api/3/statuses`… not needed: statuses carry a
  **`statusCategory.key`**; recognized categories are `new`, `indeterminate`,
  and `done` (`undefined` follows the unknown-category fallback)
  independent of custom workflows. This is the workflow-independent
  discriminator for both mapping and transition selection.
- `GET /rest/api/3/workflow/search` happens to be readable with this token,
  but it is an admin API: **never design against it**.
- `GET /rest/api/3/filter/search?expand=jql` lists **saved filters** (paginated,
  `values[].name` + `values[].jql`; all filters visible to the user);
  `GET /rest/api/3/filter/my` (own filters) also works. The plain
  `GET /rest/api/3/filter` is deprecated — its error response itself points
  to the Search filters operation. Both search/my verified 200 with the
  token (0 filters on the site so far, matching 0 projects).
- The original inspection found **zero projects** — a manual test project (suggested
  key `TST`) must be created once before integration tests can run.

### Error mapping (status bar, never exit, never degrade TW rows)

| Condition | Message shape |
|---|---|
| 401 | "Jira: wrong email or truncated token (the `=…` suffix is part of the token)" |
| 400 unbounded JQL | "this Jira site requires a restricted filter — press F to pick one" |
| 400 bad JQL | show the API's JQL error fragment verbatim |
| 404 | "Jira issue not found" |
| 403 | "no permission for this Jira operation" (e.g. can view but not comment) |
| 429 / 5xx | reads only: one cancelable bounded backoff retry (honor Retry-After within timeout), then keep last-good rows and report error |
| non-JSON body (Cloudflare HTML pages) | "unexpected response from Jira (HTTP nnn)" — never panic on `json.Unmarshal` |

- Client and config code must never log, dump or echo the token
  (incl. `Authorization` headers and the model debug dump in
  `internal/ui/debug_dump.go` — if config state is dumped, the token field
  is redacted).

### Interface seams

- Define the consumer interface in `internal/ui`; the concrete client stays in
  `internal/jira`. This avoids making phase-1 fakes implement unused phase-2
  methods. Shared value types (CommentPage, SavedFilter, Transition) live in jira.

  ```go
  type IssueSource interface {
      FetchIssues(ctx context.Context, jql string) ([]task.Task, error)
      FetchIssue(ctx context.Context, issueKey string) (task.Task, error)
      FetchComments(ctx context.Context, issueKey string) (jira.CommentPage, error)
      AddComment(ctx context.Context, issueKey, text string) (task.Annotation, error)
  }
  // Add optional FilterSource, TransitionSource and FieldSource capabilities
  // as their tasks land; do not require unsupported methods on phase-1 fakes.
  // FetchFilters(ctx) ([]jira.SavedFilter, error)
  // Transitions(ctx, key) ([]jira.Transition, error)
  // DoTransition(ctx, key, id string, fields map[string]any) error
  // SetField(ctx, key, field string, value any) error
  ```

- `CommentPage` contains annotations and a remaining count. `SavedFilter`
  contains ID, name, JQL and origin; identity is origin + ID (local: name),
  never just display name. Missing JQL is not an unrestricted filter: report
  or omit the inaccessible entry.
- Token rotation is injected as a credential-reload callback at the UI boundary;
  client requests snapshot credentials under a lock. A failed reload reports
  the error and retains last-good rows; it must not silently use the stale token
  for the requested refresh. The model never contains raw credentials.
- Every request has a context deadline, bounded response size, closed body and
  escaped issue-key path segment. Do not forward credentials across origins
  on redirects. Redact token and encoded Basic credentials from API error
  fragments before display. Never automatically retry comment/create/transition
  or field writes: ambiguous failures may already have committed remotely.
- `CreateIssue(ctx, projectKey, issueTypeID, summary)` is for gated tests (not
  the UI): POST /issue with a **plain string summary**, project and issue type.
  Only descriptions and comment bodies use ADF. Discover a usable issue type
  for the test project through create metadata (or an explicit test setting).
- Comments return their created annotation; transitions and edits usually
  return 204 without an updated issue. On their success perform one per-issue
  GET and patch the cache, never a blanket search. If the write succeeds but
  GET fails, report that distinction; do not repeat the write. ADF-to-text
  handles paragraphs, lists, hardBreak, mentions and unknown nested nodes.

## 4. Mapping issues → `task.Task`

Reuses the existing struct with the identity and rendering adjustments below
so table/search/detail/ultra support both sources.

| Task field | Jira source | Note |
|---|---|---|
| ID column text | issue key `TS-42` | new optional string field on `task.Task` (`IDText`); when set, the ID column renders it instead of `strconv.Itoa(t.ID)`; the `✓` completed glyph stays for TW rows |
| `Description` | `fields.summary` | no prefix (key is in the ID column) |
| `UUID` | `jira:TS-42` | prefixed → collision-safe; the guard's discriminator |
| `Project` | `fields.project.key` | |
| `Status` | **`fields.status.statusCategory.key`** | `new`→`pending`, `indeterminate`→`pending` + `Start` set, `done`→`completed`; unknown category → `pending` + debug-log warning. Never map by status *name* (workflows are custom) |
| `Due` | `fields.duedate` | date-only; convert to `YYYYMMDDT000000Z` (start of day) — pinned choice |
| `Priority` | `fields.priority` | Jira name → H/M/L map, default M |
| `Tags` | `jira` + labels | the `jira` tag gives free tag filtering; the tag is virtual: exclude it from label edits |
| `Urgency` | **computed** | Display score: H=6, M=3, L=1 plus a due contribution clamped to 0..12, decreasing linearly from overdue by 7 days (12) to due in 14 days (0); no due=0. Existing default sort uses overdue/start/priority/due/tags/ID, not urgency; retain it |
| `Entry`, `Start`, `End` | created, updated, resolutiondate | Parse RFC3339 including Jira offsets. Start uses updated as an approximation for indeterminate; done End uses resolutiondate, falling back to updated. Clear Start on done. End drives V window; never substitute fetch time |
| `Annotations` | comments (lazy) | fetched when the detail view opens, rendered in the annotations area; first page + "+n more" hint |
| `Assignee` | `fields.assignee.displayName` | new optional string field, display-only: shown in the Jira detail view, never a column or tag |

**UI row identity:** Every Jira row receives a unique nonzero synthetic numeric
ID in the UI before rendering, stable by `jira:KEY` for the session. Reserve a
negative-ID allocator shared with completed TW rows to prevent collisions;
never use ID=0, a Jira issue number, or a hash with unchecked collisions.
UUID remains the mutation/detail identity. Selection, ultra, blinking, editor
callbacks and reload restoration must preserve UUID across re-sort; synthetic
IDs must never be sent to Taskwarrior. IDText applies in table, width
calculation, cell expansion/search and detail labels, including completed rows.

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
5. Jira write operations — which update the cache **after confirmed success**
   (created-comment response, or one per-issue GET for 204 writes), never via blanket refetches.

Explicitly *not* refetching on: post-mutation reloads (of TW rows), view
cycling (hotkey 6 filters at render — already the design), picker open,
navigation. The persistent status bar shows source mode, filter and last successful issue
fetch time, for example `[both · jira: mine · 14:32]`; initial state is loading
or never. Saved-filter timestamps are separate; a filter fetch must not make
stale issue rows appear fresh.

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
  an in-flight Jira fetch (use separate Jira cancellation/generation state);
  results apply with a sequence guard so a stale response can never overwrite
  newer data.
- **Token rotation without restart**: the token file is re-read on every
  explicit `space` refresh, so a rotated token takes effect without killing
  the TUI.
- **Status bar layout**: mode, filter and age compress into one token —
  `[both · jira: mine · 14:32]` — so it shares the status line with existing
  TW info and error messages without overflow.

### Merged table and reload

- Keep independent async Jira and TW flights. TW constructor/reloads use
  `exportReloadData` unchanged; Jira completion merges cached rows on the
  Bubble Tea goroutine without waiting for TW or vice versa. Jira errors degrade gracefully:
  status-bar message, keep last-good rows, TW rows unaffected.
- `ui.NewWithTaskwarrior` gains variadic options (`WithJira` carrying source and settings; nil = disabled);
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
  the named filters (`name` + dim truncated JQL hint). `j`/`k`/arrows before search is added,
  `Enter` applies, `Esc` cancels.
- On apply: async **Jira-only** refetch with the new JQL (TW rows untouched),
  status bar `jira filter: mine`. Active filter name shown persistently
  while Jira rows are visible, e.g. `[jira: mine]`.
- "(none)" = fetch with no JQL restriction; on the 400 unbounded-JQL error
  show the pick-a-filter hint and no Jira rows until a filter succeeds. On any filter identity change, clear prior-filter rows
  immediately: a failed new filter must never display old-filter issues under
  its name. A same-filter refresh failure retains last-good rows.
- **Fuzzy search over the picker entries**: the picker grows an embedded
  text input; typing live-filters the list with an in-repo subsequence-style
  matcher (fzf-like, **no new dependency**) matching over `name` and the
  JQL text. arrows and Ctrl+n/Ctrl+p navigate the filtered list; printable `j`/`k` enter text, `Enter` applies,
  `Esc` cancels, the `(none)` entry always stays reachable (it sorts first
  and never gets filtered out entirely). Clearing the input restores the
  full list.
- **Saved Jira filters** (`saved_filters = true`, default true):
  `FetchFilters` lazily on the picker's first open, cached for the session
  (Ctrl+r refreshes the picker; printable `r` enters search text). The picker shows three groups:
  `(none)`, local `[[jira.filters]]`, then the account's saved filters with
  an origin marker (e.g. dim `cloud`); duplicate names across groups are
  fine because the origin distinguishes them. On fetch failure: local list
  only on the first failure, or retain last-good cloud entries on refresh failure,
  plus one status-bar note. Saved-filter JQL can be unbounded → the same
  400 pick-a-filter handling as any other filter.
- Orthogonal to the view cycle: view controls *whether* Jira rows show,
  filter controls *which* issues are fetched.

### Commenting (`a` on a Jira row)

- The existing annotate input opens with prompt `"comment: "`; on submit the
  text becomes a Jira comment instead of `task annotate`. This is the one
  mutation Jira rows allow in v1.
- Confirmed-success UI: on success append the returned annotation to the cache row's `Annotations`
  (entry from returned comment timestamp) so the count/detail update immediately; the next explicit Jira refresh
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

A shared routing policy used by all dispatch points keyed off the `jira:` prefix:
control → `{TW method | Jira method | blocked-reason}`. Blocked-reasons are
per-control data so the status bar says *why*. Table-driven unit test
enumerates every control × jira row. Install the baseline guard in the merge
task before any Jira row is selectable; the later routing task consolidates
and verifies it. Test table, ultra, detail, aliases and field editors; assert
that no blocked path invokes a Taskwarrior mutation with a synthetic ID or
`jira:` UUID. Global filters f/3/4/5 remain TW-only; search applies to both.
Global U on a Jira row is blocked; off that row it still undoes the last TW
action. Adding with + always creates a TW task.

### Works unchanged (global)

`H`, `q`, `esc`, `/` `?` search, `f`, `c` `C` `x` `B` `v` `Z` `V`,
`space` (now also refetches Jira), `:` (global TW prompt), `0`–`3`, `g`/`G`, arrows, Enter → detail
view (read-only, comments lazy-loaded), ultra mode.

### Translates to Jira

Phase 1: `a` → comment; `o` → open `{site}/browse/{key}`.

Phase 2:

| Control | Jira operation | API |
|---|---|---|
| `d` done | transition to a done-category status | `GET`+`POST /transitions` |
| `s` start/stop | transition to indeterminate / back to new | same |
| `w`/`W` | set/clear `fields.duedate` | `PUT /issue/{key}` |
| `p` | set `fields.priority` (H/M/L ↔ Jira name map) | `PUT /issue/{key}` |
| `t` | edit `fields.labels` (virtual `jira` tag excluded) | `PUT /issue/{key}` |
| `e`/`E` | edit plain string `fields.summary` | `PUT /issue/{key}` |

### Transition selection (phase 2, dynamic detection)

Detection is **lazy, at keypress time** (one extra GET per action, never
prefetched on reload). Selection: `to.statusCategory.key == "done"` (resp.
`"indeterminate"`); config name override → single candidate → if still
ambiguous, pick deterministically + status-bar note *"transitioned via
'Close Issue'"*; zero candidates → polite message (e.g. `d` on a done issue).
`GET /transitions?expand=transitions.fields` includes each transition's field metadata: required
screen fields with a config default of the correct JSON shape are sent along; otherwise error names
the fields (*"transition 'Close Issue' requires: Resolution"*).

### Blocked

| Control | Reason / message |
|---|---|
| `A` replace annotations | "comments can't be replaced wholesale — use `a` to add" |
| `D` delete | "Jira issues can't be deleted from here" |
| `U` undo | "no undo for Jira issues" |
| `R`, `ctrl+r` recurrence | "Jira issues have no recurrence" |
| `J` edit project (move issue) | "moving Jira issues not supported (yet)" |
| `T` tag→project (table) | same family |
| `E`, `r`, `i` / column Enter / detail Enter | aliases must route through the same policy: E summary edit, r random due (blocked in v1, due edit in phase 2); i/Enter dispatch by field; never bypass via external task editor |
| `;` task prompt on jira row | "task commands don't work on Jira issues" |
| `+` add | unchanged (adds TW task); `+` creating Jira issues in jira-only view is a possible phase 3 |

For phase-2-translatable controls still blocked in v1 the message says so:
*"not yet — Jira write support for due dates is planned"*.

## 7. Testing strategy (three layers)

### 7.1 Unit/fixtures (fast, hermetic)

- `internal/config`: TOML parse (array order), `~` expansion, token-file
  trimming (surrounding whitespace only — fixture shaped like the real file
  with the `=…` suffix), every validation error, config-path precedence flag > env > default.
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
`ts-integration-<unix-nano>`; tests query the configured, safely quoted project plus the exact run marker and re-fetch and verify project + exact marker immediately before every mutation/deletion; cleanup sweeps only this run's exact marker; cross-run cleanup
requires a separate explicit opt-in and a stale-age fence; the guard refuses to run without
`JIRA_TEST_PROJECT`; `t.Cleanup` deletes created issues best-effort
(fallback: transition to done + warning — deletion is hygiene, not
assertion). Token never echoed; failures print bounded, secret-redacted response fragments, never
`Authorization` headers. Sequential, small volumes (rate limits).

**Coverage:** auth sanity (`/myself`); bounded JQL finds created issues;
**unbounded JQL → 400** only when a site-specific opt-in enables this probe
(the cleanup/search helpers always stay bounded); create → search →
map (`IDText`, statusCategory) → comment → transition new→indeterminate→done
via category detection → mapping shows completed → cleanup (end-to-end);
comment on a *resolved* issue still works; 401 with a deliberately bad
token; 404 fake key; pagination (N issues, small page size).

**Prereq:** historical inspection found no projects. Before opting in, verify
an explicitly designated test project (suggested `TST`, "TaskSamurai Integration
Tests") exists and permits the intended operations; no automatic project creation.

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
4. Reload pipeline: independent async fetch + graceful degradation + merged rows,
   CLI client/options wiring, stable synthetic IDs and baseline mutation guards
5. View cycle hotkey 6 (+ `Hotkeys` plumbing, help, README later)
6. Filter picker `F` (+ status bar), then picker fuzzy search, then saved
   Jira filters in the picker
7. Commenting via `a` (+ lazy-load comments in detail view)
8. `o` opens issue in browser
9. Routing policy consolidation + blocked messages (v1 matrix complete)
10. Integration scaffolding remains hermetic; full live-suite task follows step 12
11. Phase 2: transitions (`d`, `s`) with dynamic detection
12. Phase 2: field edits (`w`/`W`, `p`, `t`, `e`), then full gated integration suite
13. Docs: README (hotkey table, config section, `F`/`6`), AGENTS.md update,
    this file kept as the design record

Dependency chain for agent tasks mirrors this order; see the `ask` task
list (`+jira`).

## 9. Review record (ux3, 2026-10-30)

Reviewed all 16 Jira tasks against the current constructors, ID-based selection,
completed-ID allocator, keybindings (including E/r/i), reload single-flight,
detail editors and Mage targets. This plan governs conflicting older task
annotations. Every affected task receives a review annotation.

A read-only live GET /statuscategory returned new / indeterminate / done plus
undefined; the original todo / in-progress assumption was incorrect. Historical
zero-project and authentication observations above remain context, not a current
prerequisite check; jiracheck must verify the configured project without creating
it. No live issue mutation was performed for this review.

API references checked:

- [Issues: create, edit, transitions and their response shapes](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issues/)
- [Filters: search and expansion](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-filters/)
- [Status categories](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-workflow-status-categories/)

Tests must cover cancellations, stale detail results (issue identity and fetch
sequence), filter changes during in-flight work, writes racing search/detail
responses, duplicate cloud filter names/IDs and credential reload failure.
A write invalidates older read generations so late reads cannot erase a newly
posted comment or field change. Only the Update loop mutates caches; commands
use immutable snapshots. Completed-window filtering applies locally to cached
Jira rows before source/search filtering and statistics. Bound max_results over
all search pages, deduplicate by key, stop repeated page tokens with an error,
and preserve last-good rows on partial-page failure. A truncated result cap is
visible. Filter list refresh failure keeps last-good cloud entries (or locals
only if none exist) and never changes the active JQL.

Implementation acceptance includes mage build, mage verify, targeted hermetic
coverage and gated-test compilation with opt-in absent. Live-suite execution is
reported separately and remains pending if credentials/project/permissions are
unavailable; passing skipped tests does not prove live behavior. The integration
task depends on both phase-2 tasks so it can test the complete sequence rather
than leaving TODO stubs. Docs land after it; no phase-2 placeholder is completion.
