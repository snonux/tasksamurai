# Jira support — implemented design and verification

Status: phases 1 and 2 implemented on 2026-10-30. This is the design record for
`internal/config`, `internal/jira` and their UI integration. The original Jira
task series is implemented; creation of Jira issues from the TUI remains outside
scope. The client create helper serves the gated test harness.

## 1. Scope and decisions

Jira Cloud is an optional second source merged into the Taskwarrior table and
ultra view. Taskwarrior remains required. The implementation supports issue and
comment reads, adding comments, opening issues, workflow transitions, due dates,
priorities, labels and summaries. The existing ID column shows Jira issue keys;
there is no separate Jira table or new source column.

One TOML file configures one Jira site. `github.com/BurntSushi/toml` is the only
new dependency. Tokens live in a separate referenced file. `6` cycles cached
source views, and `F` selects local or saved cloud filters with fuzzy search.

## 2. Config file (`internal/config`)

Default location: `~/.config/tasksamurai/config.toml`. Config-path precedence is
explicit `--config` > non-empty `TASKSAMURAI_CONFIG` > default path. File values
override built-in defaults; there are no per-field environment overrides.
A missing default file or absent `[jira]` disables Jira. An explicitly selected
missing file is an error.

```toml
[jira]
enabled = true
site = "https://example.atlassian.net"
email = "your-atlassian-account@example.com"
token_file = "~/.jira_api_token"
saved_filters = true
default_filter = "tests"
default_view = "both"
max_results = 100
timeout = "10s"

[[jira.filters]]
name = "tests"
jql = "project in (TST, TST2) ORDER BY updated DESC"

[[jira.filters]]
name = "mine"
jql = "project in (TST, TST2) AND assignee = currentUser() ORDER BY updated DESC"
```

Defaults are enabled when `[jira]` exists, saved filters enabled, view `both`,
100 issues maximum and a 10s request timeout. Without `default_filter`, selection
starts at `(none)`. A site that forbids unbounded search then shows the F-picker
hint until a restricted filter succeeds.

Enabled configurations validate an HTTPS origin without credentials/path/query/
fragment, email, positive bounds, view enum, unique ordered filter names,
non-empty JQL, a valid default-filter name and a readable non-empty token file.
Unknown settings, TOML syntax and field-type errors fail startup. Disabled Jira
(`enabled=false` or `--no-jira`) skips semantic validation and credential reads;
structural TOML errors still fail.

`~/` expands via the home directory; `~user` is rejected. Relative token-file
paths resolve against the config directory. Only surrounding token whitespace
is trimmed; interior characters and any `=…` suffix remain intact. Recommend
mode 600 for token and config files. Config values never retain token contents.
Explicit space refresh rereads the token; other config changes need a restart.

Optional workflow overrides preserve native JSON field shapes:

```toml
[jira.transitions]
done = "Close Issue"
# start = "Begin Work"
# stop = "Return to Backlog"

[jira.transition_fields]
resolution = { name = "Done" }
```

## 3. Jira client (`internal/jira`)

The client uses `net/http` and `encoding/json`, Basic auth with the Atlassian
account email and complete token, per-operation deadlines and bounded bodies.
Redirects are disabled. Client formatting, error fragments and debug paths
redact credentials. Credentials are owned behind the client, with synchronized
rereading through `ReloadToken`.

| Operation | API and contract |
| --- | --- |
| Issue search | Read-only POST `/rest/api/3/search/jql`; `nextPageToken` pagination, total result cap, key deduplication and repeated-token detection |
| Issue detail | GET `/rest/api/3/issue/{key}` with only mapped fields |
| Comments | First GET page at `/issue/{key}/comment`, max 50, plus remaining count; POST uses ADF and returns the created annotation |
| Transitions | GET `/issue/{key}/transitions?expand=transitions.fields`; POST uses a string transition ID and native `fields` |
| Field edits | One PUT `/issue/{key}` for summary, labels, priority or due date |
| Saved filters | GET `/filter/search?expand=jql` with `startAt` pagination; missing/inaccessible JQL is omitted |
| Test issue creation | POST `/issue` with project, discovered issue-type ID and a **plain string** summary |

Search requests include summary, status, priority, duedate, project, labels,
updated, created, assignee and resolutiondate. Legacy `/search` is not used.
The test harness discovers issue types through create metadata. The UI never
uses admin workflow-search APIs.

Comments convert multiline text into ADF paragraphs. Reading ADF handles nested
paragraphs/lists, hard breaks, mentions, cards and unknown wrappers. Blank
comments and summaries are refused. UI priority writes use names High/Medium/Low.
Due dates are a date string or JSON null for clearing; labels are arrays.

Read operations, including POST search, retry once on 429/5xx with cancelable,
bounded Retry-After/backoff. Writes never retry automatically. A transition or
field write generally returns 204; one issue GET refreshes its cache row. An
applied write followed by a failed GET is reported as that distinct outcome.
Malformed/HTML/oversized responses and missing required result arrays fail
without replacing last-good rows.

401 reports the email/full-token hint, 403 reports operation permission, 404
reports missing issue, and invalid JQL preserves a bounded redacted API fragment.
Unbounded JQL 400 prompts for a restricted filter. Issue endpoints may mask failed
authentication as 404, so the live bad-token test uses `/myself` as its 401 oracle.

The UI owns `IssueSource`; `FilterSource`, `TransitionSource` and `FieldSource`
are optional capabilities. Phase-1 fakes need not implement later operations.
`WithJira` supplies source/settings/token-reload callback through variadic model
options. Existing `New`/`NewWithTaskwarrior` calls remain valid.

## 4. Mapping issues → `task.Task`

| Task field | Jira mapping |
| --- | --- |
| `IDText` | Issue key such as `TST-42` or `TST2-42` |
| `UUID` | `jira:KEY`, the source discriminator and stable mutation identity |
| `Description` / `Project` | Summary string / project key |
| `Status` / `Start` | Category `new` → pending; `indeterminate` → pending plus updated-time approximation for Start; `done` → completed with Start cleared |
| `Entry` / `End` | Created / resolutiondate, falling back to updated for completed issues; never fetch time |
| `Due` | Jira date-only value mapped to `YYYYMMDDT000000Z` |
| `Priority` | Highest/High/H → H, Lowest/Low/L → L, otherwise M |
| `Tags` | Virtual `jira` tag plus source labels; normal search indexes these |
| `JiraLabels` | Separate raw label metadata, excluded from Taskwarrior JSON, preserving a real label named `jira` during unrelated edits |
| `Assignee` | Display-only assignee name in details |
| `Annotations` | Lazy first comment page, plus confirmed newly posted comments |
| `Urgency` | Display score from priority and due date; existing sorting remains overdue/start/priority/due/tags/ID |

Unknown status categories, including `undefined`, fall back to pending and a
debug warning. Dates accept Jira offsets and convert to UTC. Urgency base is
H=6/M=3/L=1 plus a clamped 0..12 due contribution decreasing linearly from seven
days overdue to fourteen days ahead.

The UI assigns nonzero negative display IDs stable by UUID for the session,
sharing an allocator with completed Taskwarrior rows. These IDs never reach the
Taskwarrior CLI. Issue keys render in normal/expanded/detail/ultra views and
search, with ANSI/CJK-aware width calculation.

Completed Jira rows retain their key and completed strikethrough styling;
completed Taskwarrior rows show `✓`. `V` filters Jira completion times locally.
The selected JQL must itself include completed issues for them to be available.

## 5. UI integration

### Caching and freshness

Taskwarrior and Jira have independent async flights and contexts. Constructors
load Taskwarrior; Init schedules Jira without delaying first paint. The model
holds separate Taskwarrior/Jira caches and filters them by source/completed
window before search, display and statistics. Source cycling never fetches.

Issue-list fetch triggers are startup, space, periodic Z refresh and Jira filter
changes. A Taskwarrior mutation reload does not fetch Jira. Space also rereads
the token; a failed credential read aborts that refresh and retains last-good
rows. Same-filter failures retain cache; a new filter clears the old rows.

Opening Jira details fetches fresh fields and the first comment page, then
patches the cache. Assignee, URL and a `+n more` comment hint appear in detail.
Jira list rows omit annotation counts. Comment success appends the server-created
annotation/date; field/transition success refreshes just the affected issue.
Cached comments survive a fields-only GET. There is no disk cache.

Network completion messages change caches only in Update. Commands capture
snapshots. Read generations reject superseded work; writes invalidate stale
same-lifetime reads. Filter epochs distinguish A→B→A, so an earlier write cannot
cancel the fetch for a newly selected filter. Jira contexts survive unrelated
Taskwarrior context recreation. Quit cancels outstanding requests. Table and
ultra selection follow active-view UUIDs through sorting/filtering.

The persistent token shows source mode, filter, last successful issue-list fetch
time and loading/result-cap indicators. Cloud filters add `(cloud)`. Saved-filter
refresh time is separate and cannot make stale issue rows appear fresh. Status
text is bounded to available width.

### Source cycle and filter picker

`6` cycles tw→both→jira→tw and is configurable with `--jira-hotkey`; disabled
Jira makes it a no-op. Default view comes from config (`both` fallback).

`F` works in table/ultra with `(none)`, local filters in file order and saved
cloud entries marked by origin. Cloud identity uses its ID, not name, allowing
duplicate names. Typing matches rune subsequences across names and JQL with
stable ranking; clearing restores source order. `(none)` stays first/reachable.
Printable j/k/r enter the query. Arrows/Ctrl+n/Ctrl+p navigate; Enter applies,
Esc cancels and Ctrl+r refreshes cloud filters.

Saved filters load lazily at first open, or earlier through space/auto-refresh.
Reopening reuses cache. Refresh preserves query/selected identity and never
changes active JQL. Failure keeps the last-good cloud list, or locals alone
before first success, with a status note. `saved_filters=false` makes no cloud
requests.

### Comments, browser and field editors

`a` uses a visible `comment:` input and only appends after confirmation. `o`
constructs the escaped issue URL locally and uses the normal browser chain,
ahead of description URL/@file scanning. Details refresh on every open in table
and ultra; completed issues moving outside V remain readable in their open
detail view through the authoritative cache.

Jira summary edits use the inline editor. Field editors stay visible in a
scrolled detail viewport; typing stays in the input and cannot trigger another
mutation hotkey. In the table Enter opens details and `i` edits the selected
cell; inside details Enter/`i` edit the highlighted field. The client supports
multiline ADF comments; the TUI uses its existing text input.

## 6. Control behavior matrix on Jira rows

`jiraguard.go` is the shared global/Taskwarrior/Jira/blocked/field policy.
Aliases and reasons are centralized; activation, commit, external-editor,
delayed-done and undo boundaries enforce routing. Generic TW-only completion
rejects Jira addresses even though `d` is a supported Jira operation.

| Control | Implemented Jira behavior |
| --- | --- |
| `a`, `o` | Add comment / open issue |
| `d` | Available done-category transition; completed issues no-op politely |
| `s` | Available indeterminate/new transition for start/stop |
| `w`, `W`, `r` | Set, clear or randomize due date |
| `p`, `t`, `e`/`E` | Priority, incremental labels, inline summary |
| Enter / `i` | Open details or delegate to an editable field in the relevant view |
| `A` | Block wholesale comment replacement; use a |
| `D`, `U` | Block Jira deletion and undo |
| `R`/Ctrl+r | Block recurrence outside the picker |
| `J`, table `T` | Block project moves/tag-to-project conversion |
| `;` | Block selected Jira task commands |
| `+`, global `:` | Taskwarrior creation/command prompt, including Jira-only view |
| `f`, `3`/`4`/`5` | Taskwarrior filters only |
| `6`, `F`, space, `Z`, `V` | Source/filter/refresh/completed-window controls in table/ultra |

Transition detection is lazy at keypress. Category-based candidates are filtered
by an optional exact-name override, then selected deterministically by name/ID.
The chosen transition is reported. Required screen fields use correctly shaped
config or metadata defaults; missing fields are named without posting.

Jira writes serialize against other Jira writes, independently of Taskwarrior.
The generic mutation pipeline distinguishes failed writes from applied writes
whose follow-up read failed. It never repeats an ambiguous write or searches
the entire list afterward.

## 7. Testing strategy and gates

### 7.1 Unit/fixtures

Config parsing/defaults/validation/path precedence/token handling; mapping and
source labels; literal control outcomes and actual TW UUID calls; shared-field
editors; CJK/long keys; table/ultra/detail rendering; cancellation, stale reads,
filter epochs, token reload failure and active-view selection. Distinct TST/TST2 issue-key cells are asserted in hermetic UI rendering tests.
Jira list rows omit redundant project values and synthetic source tags; real
labels remain visible, while source metadata is retained for filtering.

### 7.2 httptest

Real HTTP request/response assertions cover search/filter pagination, auth,
field payloads, ADF, metadata/required fields, retries, cancellation, 401/403/404/
400/429/5xx, HTML/malformed/missing arrays, redirects and body limits. UI→HTTP
field tests assert one PUT plus one issue GET and no TW mutation or bulk search.

### 7.3 Live integration (gated)

The live files use `//go:build integration`; runtime opt-in remains mandatory:

```bash
export JIRA_TEST_SITE=https://snonux.atlassian.net
export JIRA_TEST_EMAIL=your-atlassian-account@example.com
export JIRA_TEST_TOKEN_FILE=~/.jira_api_token
export JIRA_TEST_PROJECT=TST
export JIRA_TEST_SECOND_PROJECT=TST2
mage jiracheck
JIRA_INTEGRATION=1 JIRA_TEST_UNBOUNDED=1 mage jiratest
```

Both projects must be distinct and explicit for the mutation suite. Optional
`JIRA_TEST_ISSUE_TYPE_ID` selects a creatable non-subtask type. Otherwise create
metadata is used, preferring Task when available. `jiracheck` is read-only and
can check one project, or both when the second is set.

Each run creates four fixtures across both projects with an exact
`ts-integration-<unix-nano>` summary prefix. Queries restrict project and run
marker. Every mutation/delete re-fetches and validates project, key, UUID and
exact marker. `t.Cleanup` deletes only its registered fixtures; 403 triggers a
fenced done fallback and warning. No TestMain/cross-run sweep is implemented.
Token/header contents never appear in diagnostics.

Coverage includes auth/project/type preflight, bounded eventual-index polling,
real one-issue pages through the production client, distinct project/issue keys,
summary/priority/labels/due-set-clear, multiline comments, start→stop→start→done,
resolved comments, 404, invalid bounded JQL 400 and bad-token 401 via `/myself`.
`JIRA_TEST_UNBOUNDED=1` additionally enables this site's unbounded-query 400 probe;
omit it for sites that permit unbounded queries.

### 7.4 Normal gates

`mage build`, `mage test` and `mage verify` never enable live Jira tests.
`JIRA_INTEGRATION=0 go test -tags integration ./internal/jira` compiles the live
harness while skipping network operations. Hermetic ownership tests reject
foreign projects, keys, UUIDs, other markers and marker-prefix collisions.

## 8. Implementation record

| Tasks | Delivered |
| --- | --- |
| ux3 | Plan/API/code review and clarified task contracts |
| vx3, wx3 | TOML config, safe REST client/mapping and initial live scaffolding |
| xx3, yx3 | Issue-key rendering, independent cached merge, stable IDs and initial guards |
| zx3, 0y3, 8y3, 9y3 | Source cycle, local picker, fuzzy search and cloud filters |
| 1y3, 2y3, 3y3 | Confirmed comments/fresh details, browser opening and complete routing policy |
| 5y3, 6y3 | Dynamic transitions and field edits through the shared write pipeline |
| 4y3 | Fenced two-project live lifecycle suite |
| fy3, ly3 | Existing ignored-error gate repairs and comment/filter lifetime race fix |
| 7y3 | README, agent guidance and this implementation record |

Implementation refinements relative to the initial plan: real category keys
are new/indeterminate/done; summaries are strings; writes returning 204 require a
per-issue GET; consumer capabilities are optional; source IDs are synthetic;
contexts and filter epochs protect requests across navigation/writes; raw labels
preserve a real `jira` label; printable picker keys take precedence over old
j/k/r navigation; cleanup is per-run rather than a cross-run sweep.

## 9. Review and verification record (2026-10-30)

The initial read-only review observed zero projects. The user then authorized
API creation and requested two test projects. `TST` (id 10000) and `TST2`
(id 10001) were created from the classic Kanban template and passed read-only
checks. Status-category reads confirmed new/indeterminate/done plus undefined.

`mage build` and full `mage verify` passed after integration, as did focused
race/static checks and the opted-out tagged harness. The full live
`mage jiratest` passed in 39.4s with both projects and the optional unbounded
probe enabled. Both project lifecycles passed; distinct mapping and real
pagination were exercised. Rendering of the corresponding different keys is
covered separately by UI fixtures, not a claim of an interactive live TUI run.

A first live run exposed an incorrect 401 oracle: issue lookup with a bad token
returned 404. The authenticated `/myself` probe passed, followed by the full
rerun. Live DELETE returned 403. All eight marked fixtures from the two runs
were independently read back as done in their original projects through the
permitted cleanup fallback. No unrelated issue was mutated or deleted.

Required fresh-context task reviews found and resolved selection, editor
visibility and filter-lifetime races. Test-only refinements include an exact
ANSI strikethrough assertion and live pagination without high fixture volume.

Primary API references checked during implementation:

- [Issues: create, fields and transitions](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issues/)
- [Saved filters and JQL expansion](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-filters/)
- [Status categories](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-workflow-status-categories/)
- [Project creation](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-projects/)
