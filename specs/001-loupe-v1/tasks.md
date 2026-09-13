---

description: "Task list for loupe v1"
---

# Tasks: loupe v1

**Input**: Design documents from `specs/001-loupe-v1/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/cli.md](contracts/cli.md), [quickstart.md](quickstart.md), `docs/comment-format.md`, `docs/github-facts.md`, `.specify/memory/constitution.md`

**Tests**: REQUIRED. Constitution "Development Workflow" mandates failing test → implementation → passing check for every task, and quickstart.md lists the scenarios the suite MUST contain. Test tasks in each story MUST be written first and MUST fail before the implementation tasks that follow them.

**Organization**: Tasks are grouped by user story so each story can be implemented and tested as an increment.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task in the same phase)
- **[Story]**: The user story the task belongs to (US1 to US8)
- Every task names its exact file paths

## Conventions every task MUST follow

- Module path `github.com/eriksaulnier/loupe`; `go.mod` declares `go 1.25.0` (plan.md Complexity Tracking). Build with `GOTOOLCHAIN=local`; a dependency that raises the directive is a stop-and-ask. `golang.org/x/term` stays at v0.45.0.
- Add a dependency with `go get` in the task that first imports it. Runtime dependencies are limited to the list in plan.md; anything else requires a plan.md and research.md row first (constitution VI).
- Domain errors are `*refusal.Error` values (T006) carrying `Code`, `Message`, `Fix`, `Details`; the codes are exactly the table in contracts/cli.md. `internal/cli` only maps them to envelopes and exit codes.
- `draft`, `diff`, `render`, `publish`, `run`, `markdown`, `gitx`, `github` MUST NOT import `tui` or `cli`. `run` MUST NOT import `draft`.
- Tests MUST NOT use a real pseudo-terminal, a real reviewer, a real GitHub host or any non-loopback address (constitution VII). GitHub is `internal/testutil/fakegh`; Git is `internal/testutil/gitrepo`; terminal input is injected.
- After each task: `mise run check` (`go vet ./... && golangci-lint run && go test ./...`) MUST pass, then one Conventional Commit (`type(scope): subject`, lowercase imperative subject at most 72 characters, no trailing period).
- Comments explain why, never what. Run the `cleanup-comments` skill over the diff before each commit.

## Path Conventions

Single Go module at repository root: `cmd/loupe/`, `internal/<package>/` with tests beside their code, `internal/integration/` for end-to-end tests, `testdata/` for fixtures and golden files, `plugin/` for the Claude Code plugin. Layout per plan.md "Project Structure".

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Module, toolchain, lint and hooks.

- [X] T001 Create `go.mod` with `go mod init github.com/eriksaulnier/loupe` and set the directive to `go 1.23`; create `.gitignore` containing `dist/`, `*.test`, `coverage.out`
- [X] T002 [P] Create `mise.toml` pinning `go` (1.25.x), `golangci-lint` (2.x) and `goreleaser` (2.x), with tasks `lint` (`golangci-lint run`), `test` (`go test ./...`), `check` (`go vet ./... && golangci-lint run && go test ./...`) and `build` (`CGO_ENABLED=0 go build -o dist/loupe ./cmd/loupe`)
- [X] T003 [P] Create `.golangci.yml` in golangci-lint v2 format (`version: "2"`) enabling `errcheck`, `govet`, `staticcheck`, `unused`, `ineffassign`, `misspell` with `locale: US`, and the `gofmt` and `goimports` formatters
- [X] T004 [P] Create `lefthook.yml` with a `pre-commit` command `mise run check` and a `commit-msg` command that rejects a subject not matching `^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\([a-z0-9-]+\))?!?: [a-z]`, longer than 72 characters, or ending in a period
- [X] T005 Run `mise trust && mise install && mise exec -- lefthook install` and confirm `go version`, `golangci-lint version` and `goreleaser --version` print the pinned versions (depends on T002, T004)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Error type, run storage, locking, draft store and derivation, diff parsing, GitHub client and fake, Git test repository, CLI skeleton. Every story depends on these.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T006 Create `internal/refusal/refusal.go` and `internal/refusal/refusal_test.go`: type `Error struct { Code, Message, Fix string; Details map[string]any }` implementing `error`; a `Code` constant for every row of the contracts/cli.md error table (`usage`, `no-run`, `record`, `origin`, `pr`, `same-head`, `auth`, `input`, `location`, `markdown`, `version`, `count`, `not-found`, `lock`, `tty`, `head-moved`, `own-pr`, `blocking`, `not-ready`, `empty`, `attempt`, `github`, `internal`); `New(code, message, fix string) *Error`; `As(err error) (*Error, bool)` using `errors.As`. This is a leaf package because `internal/cli` depends on every domain package, so the refusal type cannot live in `internal/cli/errors.go` without an import cycle
- [X] T007 [P] Create `internal/run/root.go` and `internal/run/root_test.go`: `DataRoot(getenv func(string) string) (string, error)` returning `$LOUPE_HOME`, else `$XDG_DATA_HOME/loupe`, else `$HOME/.local/share/loupe`; `RunsDir(root)` is `<root>/runs`; `RunDir(root, owner, repo string, number, round int)` is `<root>/runs/<owner>/<repo>/<number>/<round>`
- [X] T008 [P] Create `internal/run/ref.go` and `internal/run/ref_test.go`: `type Ref struct { Owner, Repo string; Number, Round int }` where `Round == 0` means newest; `ParseRef(s)` accepts `owner/repo#123` and `owner/repo#123@2` and refuses anything else with code `usage` and fix `use owner/repo#123 or owner/repo#123@2`; `Ref.String()` round-trips, printing `@N` only when `Round > 0`
- [X] T009 [P] Create `internal/run/atomic.go` and `internal/run/atomic_test.go`: `WriteFileAtomic(path string, data []byte)` writes a temp file in the same directory, fsyncs, renames over `path`, fsyncs the directory; `WriteJSONAtomic(path, v)` marshals with two-space indent and a trailing newline; `ReadJSON(path, v)` decodes with `DisallowUnknownFields` and returns code `record` naming the path, with fix `inspect it with: cat <path>`, when the file is missing, unreadable, malformed or has unknown fields. loupe MUST NOT repair or delete a damaged file
- [X] T010 [P] Create `internal/run/lock.go` and `internal/run/lock_test.go`: `Lock(dir, command string, getenv) (*Held, error)` opens `<dir>/.lock`, calls `syscall.Flock(fd, LOCK_EX|LOCK_NB)` in a loop every 25 ms until `LOUPE_LOCK_TIMEOUT_MS` elapses ("default 3000, max 60000"; a non-integer or out-of-range value refuses with `usage`), then truncates the file and writes `<pid> <command>`; `Held.Unlock()` releases the flock and closes the file and MUST NOT delete it. On timeout refuse with code `lock`, message naming the path and the holder's pid and command read from the file, fix `wait for pid <pid> (<command>) to finish, or stop it, then retry`. The test re-executes the test binary as a helper process (guarded by an env var) that holds the lock, asserts the timeout refusal text with `LOUPE_LOCK_TIMEOUT_MS=100`, and asserts a waiter acquires the lock after the helper exits
- [X] T011 [P] Create `internal/draft/model.go` and `internal/draft/model_test.go` with the types in data-model.md, JSON field names in camelCase: `Draft{Schema, Version, Summary, Findings, Decisions map[string]Decision, Notes, Replies}`; `Finding{ID, Rev, Title, Body, Location *Location, General, Label, Blocking, Confidence, Severity, SuggestedFix, By, Included, CreatedAt, UpdatedAt, History}`; `Location{Path, Side, Line, StartLine}`; `Decision{FindingID, Decision, FindingRev, At}`; `Note{ID, FindingID, Body, At, Status, ClosedAt}`; `Reply{ID, NoteID, Body, By, At}`; `HistoryEntry{At, By, Changed map[string]any}`. Constraints to encode as constants and document on the fields: `schema` is 1; `version` "starts at 0 on capture; every mutation increments by one"; finding `id` "`f-001`, sequential within the run, never reused"; `rev` "starts at 1; increments on any change to a publishable field or to `included`"; note id `n-001`; reply id `r-001`; `side` "`RIGHT` or `LEFT`", default `RIGHT`; `decision` "`accepted` or `excluded`"; note `status` "`open`, `resolved`, `dismissed`"; `by` "`agent` or `human`". Provide `NewEmpty()` (schema 1, version 0, empty non-nil collections) and `NextFindingID`, `NextNoteID`, `NextReplyID` (`%03d`, widening past 999, derived from collection length because collections are append-only)
- [X] T012 [P] Create `internal/diff/parse.go` and `internal/diff/parse_test.go` wrapping `bluekeyes/go-gitdiff`: `Parse([]byte) (*Diff, error)` into `File{OldName, NewName, IsNew, IsDelete, IsRename, IsBinary, Hunks}`, `Hunk{OldStart, OldLines, NewStart, NewLines, Lines}`, `Line{Kind (Context|Add|Delete), OldNum, NewNum, Text, NoNewlineAtEOF}` with old and new numbers computed per line; `Diff.File(path)` matches `NewName`, or `OldName` for a deleted file; per-file line indexes are built lazily on first lookup and the diff is never re-parsed. Commit fixtures in `testdata/diffs/` generated by running `git diff --no-ext-diff --no-color --no-textconv --src-prefix=a/ --dst-prefix=b/ --find-renames` in a scratch repository: `rename.diff`, `binary.diff`, `no-newline.diff`, `mode-change.diff`, `multi-hunk.diff`, `new-file.diff`, `deleted-file.diff`
- [X] T013 [P] Create `internal/github/client.go` and `internal/github/client_test.go`: `type Client interface { PullRequest(ctx, owner, repo string, number int) (PullRequest, error); PullRequestsForBranch(ctx, owner, repo, branch string) ([]PullRequest, error); Viewer(ctx) (string, error); ListReviews(ctx, owner, repo string, number int) ([]Review, error); CreateReview(ctx, owner, repo string, number int, req ReviewRequest) (Review, error) }`; `PullRequest{Number, URL, Title, State, Author, BaseRef, BaseSHA, HeadSHA}`; `Review{ID, User, CommitID, State, Body, HTMLURL}`; `ReviewRequest{CommitID, Body, Event string; Comments []ReviewComment}` with `ReviewComment{Path, Line, Side, StartLine, StartSide, Body}` encoded as `commit_id`, `body`, `event`, `comments[].{path,line,side,start_line,start_side,body}` with `start_*` omitted when zero; `HTTPError{Status int; Message string}` with `Definite()` true for 4xx; transport errors, timeouts and 5xx are ambiguous. `NewREST(transport http.RoundTripper)` builds a `cli/go-gh/v2` REST client with `Host: "github.com"` and the token from go-gh `auth.TokenForHost("github.com")`; an empty token refuses with `auth`, fix `gh auth login --hostname github.com`. Endpoints: `GET /repos/{o}/{r}/pulls/{n}` (404 refuses with `pr`), `GET /repos/{o}/{r}/pulls?state=open&head={o}:{branch}`, `GET /user`, `GET /repos/{o}/{r}/pulls/{n}/reviews?per_page=100` following every page, `POST /repos/{o}/{r}/pulls/{n}/reviews`. The unit test uses an `httptest` server and a rewriting transport to assert request encoding
- [X] T014 [P] Create `internal/testutil/gitrepo/gitrepo.go` and `internal/testutil/gitrepo/gitrepo_test.go`: `New(t, owner, repo string, number int) *Repo` builds under `t.TempDir()` a bare remote with `uploadpack.allowAnySHA1InWant=true`, a clone whose `remote.origin.url` is `https://github.com/<owner>/<repo>.git` with `url.<bare path>.insteadOf` set in the clone's config so fetches reach the bare remote, a base commit on `main`, and a pull request head commit pushed to `refs/pull/<number>/head` that edits, adds, deletes and renames files across more than one hunk. Isolate Git with `GIT_CONFIG_GLOBAL=/dev/null`, `GIT_CONFIG_NOSYSTEM=1` and fixed author and committer env via `t.Setenv`. Helpers: `BaseSHA()`, `HeadSHA()`, `PushHead(files map[string]string) string` (moves the pull request head), `Snapshot() Snapshot` capturing `git status --porcelain`, `git rev-parse HEAD`, `git symbolic-ref HEAD`, SHA-256 of `.git/index` and `git for-each-ref`, and `Snapshot.DiffIgnoringLoupeRefs(other)` returning differences outside `refs/loupe/`
- [X] T015 Create `internal/run/target.go` and `internal/run/target_test.go` (depends on T007, T009): `Target` struct with every field in data-model.md "Target": `schema` (1), `owner`, `repo`, `number`, `url`, `title`, `author`, `viewer`, `baseSha`, `headSha`, `round` ("1-based, one higher than any existing round for the pull request"), `previousRound` (optional, "`round - 1` when any earlier round exists, published or not; lineage only"), `capturedAt`, `clonePath`, `baseRef`, `headRef` (`refs/loupe/<owner>/<repo>/<number>/<round>/{base,head}`), `diffSha256`; `LoadTarget(dir)` via `ReadJSON`; `CreateRun(dir string, target Target, diff []byte, draftJSON []byte)` writes `target.json`, `pr.diff` and `draft.json` into a sibling temp directory and renames it to `dir`, refusing with `internal` if `dir` already exists. `run` MUST NOT import `draft`, which is why the draft arrives as bytes
- [X] T016 Create `internal/run/resolve.go` and `internal/run/resolve_test.go` (depends on T008, T015): `Rounds(root, owner, repo, number) ([]int, error)` lists numeric round directories sorted ascending, ignoring non-numeric entries such as temp directories; `Newest(...)`; `ResolveRef(root, ref Ref) (dir string, resolved Ref, err error)` resolving `Round == 0` to the newest round and refusing with `no-run`, fix `loupe capture <url>` or `--run <ref>`, when the directory does not exist; `HasReceipt(dir)` and `HasAttempt(dir)` stat `receipt.json` and `attempt.json`
- [X] T017 Create `internal/draft/store.go` and `internal/draft/store_test.go` (depends on T009, T010, T011): `Load(dir) (*Draft, error)` via `ReadJSON`; `Mutate(dir, command string, expectVersion *int, getenv, fn func(*Draft) error) (*Draft, error)` takes the run lock, reads the draft, refuses with `version` (message `draft is at version N, expected M`, fix `re-read with loupe show --json and retry`) when `expectVersion` differs, calls `fn`, writes nothing if `fn` returns an error, otherwise increments `Version` by one and writes atomically before unlocking. Tests: an error from `fn` leaves `draft.json` byte-identical; version increments by exactly one; stale `expectVersion` refuses; a truncated `draft.json` refuses with `record`; two helper processes each run 20 mutations concurrently and every mutation lands with strictly increasing versions (quickstart "Two processes mutate one draft")
- [X] T018 [P] Create `internal/draft/derive.go` and `internal/draft/derive_test.go` (depends on T011) implementing the data-model.md "Derived values" table exactly: a decision is current iff `decision.findingRev == finding.rev`; `accepted` is "`included` and a current `accepted` decision"; `excluded` is "a current `excluded` decision"; `withdrawn` is "not `included` and no current decision"; `pending` is "`included` and no current decision"; readiness is "no finding is `pending` and no note is `open`"; `Readiness{Ready bool; Accepted, Pending, Excluded, Withdrawn, OpenNotes []string}` with counts derived from list lengths; `Dispositions(d) map[string]string`; `IncludedCount(d)` counts findings with `included: true`. Table tests cover a stale decision ignored, an excluded finding still `included: true`, a withdrawn finding, an open note blocking readiness, and a finding with `by: human` still pending (clarification 4)
- [X] T019 [P] Create `internal/testutil/fakegh/fakegh.go` and `internal/testutil/fakegh/fakegh_test.go` (depends on T013): an `httptest.Server` implementing the five endpoints in T013 from in-memory state: `SetPR(owner, repo string, pr github.PullRequest)`, `SetHead(owner, repo string, number int, sha string)`, `SetViewer(login)`, `AddReview(...)` (including `PENDING` reviews); a queue of outcomes for `POST .../reviews`: `OK` (store and return 200 with `id` and `html_url`), `Reject422(message)`, `ServerErrorAfterRecord` (store, then 500), `ServerErrorDrop` (500 without storing); `Requests()` returns every received request with method, path and decoded body; `CreateCount()`; `Client(t) github.Client` returns the production `github.NewREST` client with a transport that rewrites scheme and host to the fake and a fixed token, so request encoding is exercised end to end
- [X] T020 [P] Create `internal/cli/errors.go`, `internal/cli/envelope.go` and `internal/cli/envelope_test.go` (depends on T006): success envelope `{"loupe": 1, "ok": true, "command": ..., "run": ..., "version": ..., <payload keys at top level>}` with `run` and `version` omitted when unset; refusal envelope `{"loupe": 1, "ok": false, "command", "run"?, "error": {"code", "message", "fix", "details"?}}`; exit code 0 on success, 2 for `usage`, 1 for every other refusal; a non-refusal error becomes code `internal` with fix `file an issue` and its stack on stderr; without `--json` a refusal prints `error: <message>` and `fix: <fix>` on stderr. `--json` MUST print exactly one object on stdout and nothing else there
- [X] T021 [P] Create `internal/cli/input.go` and `internal/cli/input_test.go` (depends on T006): `DecodeInput(from string, stdin io.Reader, v any) error` reads the file at `from` or stdin for `-`; before typed decoding, parse into `json.RawMessage`, and for the top-level object or each element of a top-level array refuse with `input` when a key is `included`, `decision`, `decisions`, `status` or `findingRev`, naming the key and `details.entry` (zero-based) for arrays; then decode with `DisallowUnknownFields`, refusing unknown fields and malformed JSON with `input` naming the field or byte offset and fix `see loupe <command> --help for the input shape`; trailing data after the value refuses. `ConflictsWithFrom(cmd, flags...)` refuses with `usage` when `--from` is combined with a content flag
- [X] T022 Create `internal/cli/root.go`, `internal/cli/run.go` and `cmd/loupe/main.go` (depends on T013, T016, T020): `type Deps struct { Stdin io.Reader; Stdout, Stderr io.Writer; Getenv func(string) string; Now func() time.Time; WorkDir string; GitHub func() (github.Client, error); IsTerminal func() bool }`; `NewRoot(deps) *cobra.Command` with persistent `--json`, `SilenceUsage` and `SilenceErrors`, flag and argument errors mapped to `usage`, and cobra usage text routed to stderr when `--json` is set; `Execute(deps, args) int`. `run.go` holds `resolveRun(cmd, deps, positional string)` implementing contracts/cli.md run selection order `--run` or positional ref, then `LOUPE_RUN`, then (until US5) refusal `no-run`. `main.go` builds production `Deps` (`os` streams, `os.Getenv`, `time.Now`, `os.Getwd`, `github.NewREST(nil)`, `term.IsTerminal` on stdin and stdout) and calls `os.Exit(cli.Execute(deps, os.Args[1:]))`

**Checkpoint**: `mise run check` passes; `go run ./cmd/loupe --help` prints cobra's default root help.

---

## Phase 3: User Story 1 - Agent files findings against a captured pull request (Priority: P1) 🎯 MVP

**Goal**: `capture`, `add`, `summary` and `show` work end to end against a local Git remote and the fake GitHub, with location, Markdown, batch and count validation.

**Independent Test**: With only `loupe --help`, capture a pull request, file one located and one general finding, set a summary with the intended count, and read the draft back with stable ids; a finding off the diff is refused with nearest-line hints and the draft is unchanged.

### Tests for User Story 1 ⚠️

> Write these first and confirm they fail.

- [X] T023 [P] [US1] Write `internal/markdown/allowlist_test.go` covering every row of the `docs/comment-format.md` "Allowlist" table, each asserting code `markdown`, `details.rule` and the one-based `details.line`: 64 KiB accepted and 64 KiB + 1 byte refused as `limit` at line 1; an unclosed backtick fence and a tilde fence closed by a shorter fence refused as `fence`; `<br>`, `<img>` and an inline `text <details>` refused as `html`; `<!-- x -->` refused as `html`; `<details>` followed by prose instead of `<summary>` refused as `html`; `</details>` without an opener refused as `depth`; nesting 15 accepted and 16 refused in a body; 16 accepted and 17 refused in a summary; `<details>` inside a code span, inside a fence, after a backslash escape, or written as `&lt;details&gt;` accepted
- [X] T024 [P] [US1] Write `internal/diff/locate_test.go` against `testdata/diffs/multi-hunk.diff` and friends: a RIGHT line that is added or context accepted; a LEFT line that is deleted or context accepted; a line between hunks refused with code `location`, `details.nearest` holding up to three valid lines below and three above on that side, sorted ascending, and a `fix` naming valid ranges as `use one of: <path>:<a>-<b>, <c>-<d>`; a path not in the diff refused with `details.paths` listing the diff's paths; `startLine > line` refused; a range spanning two hunks refused; any line in a binary file refused; `HunkFor` returns the hunk containing the anchor
- [X] T025 [P] [US1] Write `internal/gitx/gitx_test.go` using `gitrepo`: `OriginMatches` accepts the raw configured URL, accepts an `insteadOf`-expanded shorthand such as `gh:o/r` that expands to `https://github.com/o/r`, accepts `git@github.com:o/r.git` and `ssh://git@github.com/o/r`, compares owner and repo case-insensitively, and refuses `other/repo` with code `origin` and fix `loupe capture <url> --repo <path>`; `FetchPR` writes exactly the two `refs/loupe/...` refs, leaves `Snapshot` otherwise identical, and creates no `FETCH_HEAD`; `Diff` output parses with `diff.Parse`
- [X] T026 [P] [US1] Write `internal/draft/mutate_add_test.go`: `Add` with one object and with an array; ids `f-001`, `f-002` in order with `rev` 1, `included` true, `by` defaulting to `agent`; a batch whose second entry has an off-diff location stores nothing and refuses with `details.entry` 1; missing title, missing body, both `location` and `general: true`, neither of them, `confidence: "certain"`, and a `side` other than `RIGHT` or `LEFT` each refused with `input`; a body failing the allowlist refused with `markdown` and fix `loupe edit <id> --from -`; `SetSummary` with `expectFindings` 3 and two included findings refuses with `count`, lists `details.included` as `[{id, title}]` and leaves the summary unchanged; `SetSummary` with a matching count stores the summary
- [X] T027 [P] [US1] Write `internal/cli/help_test.go`: with `LOUPE_HOME` pointing at a nonexistent directory and no Git repository in `WorkDir`, walk every subcommand returned by `NewRoot(deps).Commands()` (so commands added in later stories are covered automatically), run `<cmd> --help`, and assert exit 0, non-empty output and no file created under `LOUPE_HOME`; assert `loupe --help` names the workflow `capture → add → summary → review → publish`, the run reference syntax, and states `review` and `publish` are human-only and MUST NOT be run by an agent; assert the help for `add`, `edit`, `summary` and `reply` contains its JSON input example and every command that supports `--json` shows its result payload keys
- [X] T028 [US1] Write `internal/integration/harness_test.go` and `internal/integration/capture_test.go`: the harness builds a `gitrepo`, a `fakegh` with the pull request at the repo's shas, a temp `LOUPE_HOME`, and `cli.Deps` with injected `Stdin`, `IsTerminal`, `GitHub` returning the fake client, and `WorkDir` at the clone, exposing `Run(args ...string) (stdout, stderr string, exit int)` and `RunJSON` decoding the envelope. Scenarios: capture leaves the clone untouched per `Snapshot.DiffIgnoringLoupeRefs` (quickstart row 1, FR-002); `target.json` has every field, round 1, no `previousRound`, `diffSha256` equal to SHA-256 of `pr.diff`; the envelope carries `target`, `refs`, `cleanup` (two `git update-ref -d` commands) and `next`; a clone whose origin is another repository refuses with `origin`; `add` of a located and a general finding returns `f-001` and `f-002` and advances `version`; an off-diff finding refuses with nearest lines and `draft.json` is byte-identical afterward; a two-entry batch with one bad entry stores nothing; `summary --expect-findings 3` with two findings refuses with `count`; `show --json` returns both findings with their ids

### Implementation for User Story 1

- [X] T029 [P] [US1] Implement `internal/markdown/allowlist.go`: `type Kind int` (`Body`, `Summary`); `Check(text string, kind Kind, fix string) error` as a line scanner, not a parser, enforcing the `docs/comment-format.md` Allowlist table: "At most 64 KiB of UTF-8" (`limit`, line 1); "Every fence opened with three or more backticks or tildes is closed by a fence of the same character and at least the same length; fence content is literal" (`fence`); "Outside fences and code spans, the only raw HTML is `<details>`, `<details open>`, `<summary>`, `</summary>` and `</details>`, each alone on its line (surrounding whitespace allowed)" (`html`); "No HTML comments, declarations, CDATA or processing instructions outside fences" (`html`); "Every `<details>` is followed, after optional blank lines, by `<summary>` on the next content line, and `</summary>` closes it on the same line or a later one before any other content" (`html`); "`<details>` nesting is balanced and at most 15 levels deep in a body, 16 in a summary" (`depth`). Refusal code `markdown`, message naming the rule and line, `details` `{rule, line}`, and the caller's `fix` (makes T023 pass)
- [X] T030 [P] [US1] Implement `internal/diff/locate.go`: `(*Diff).Validate(path, side string, line, startLine int) error` per data-model.md "Location": "`path` must be a file in `pr.diff`", "`line` must appear on that side in some hunk of that path", "`startLine` when present, `startLine <= line` and both in the same hunk"; RIGHT lines are context and added lines by new number, LEFT lines are context and deleted lines by old number; refusal code `location` with message `<path>:<line> is not in the diff on side <side>`, `details.nearest` (up to three below and three above) and a fix listing valid ranges; `(*Diff).HunkFor(path, side, line) (*File, *Hunk, error)` (makes T024 pass)
- [X] T031 [P] [US1] Implement `internal/gitx/gitx.go`: every call runs `git -C <clone>` with `GIT_TERMINAL_PROMPT=0` and returns stderr in errors; `ParseGitHubURL(s) (owner, repo string, ok bool)` for https, `git@github.com:` and `ssh://` forms, stripping `.git`; `OriginMatches(clone, owner, repo) error` reads `git config --get remote.origin.url` (raw) and `git ls-remote --get-url origin` (after `insteadOf` expansion) and accepts when either parses to github.com and matches owner and repo case-insensitively — the raw URL is accepted too because a clone that rewrites a github.com URL to a mirror is still that repository, and the tests' bare remote relies on it — otherwise refuses `origin`; `FetchPR` MUST fetch from the `origin` remote by name and MUST NOT fetch from a URL built from the pull request; `FetchPR(clone string, number int, baseSHA, baseRef, headRef string) error` runs `git -c core.hooksPath=/dev/null -c maintenance.auto=false -c gc.auto=0 -c fetch.recurseSubmodules=false -c submodule.recurse=false fetch --no-tags --no-recurse-submodules --no-prune --no-write-fetch-head --no-auto-maintenance origin +refs/pull/<n>/head:<headRef> +<baseSHA>:<baseRef>`; `RevParse(clone, ref)` via `rev-parse --verify <ref>^{commit}`; `Diff(clone, baseRef, headRef) ([]byte, error)` via `diff --no-ext-diff --no-color --no-textconv --src-prefix=a/ --dst-prefix=b/ --find-renames <base>..<head>` so user diff config cannot change the stored bytes (makes T025 pass)
- [X] T032 [US1] Implement `Add` in `internal/draft/mutate.go` (depends on T017, T029, T030): `Add(d *Draft, inputs []FindingInput, dif *diff.Diff, by string, now time.Time) ([]Finding, error)` validates every entry before appending any, prefixing refusals with `details.entry`; rules quoted from data-model.md "Finding": `title` "required, non-empty"; `body` "required, Markdown, allowlist-checked, at most 64 KiB" (fix `loupe edit <id> --from -`); "exactly one of `location` or `general: true`"; `label` "`issue`, `suggestion`, `question` or any other word kept verbatim; empty means none"; `blocking` "default false"; `confidence` "`high`, `medium` or `low` when present"; `severity` "arbitrary text"; `suggestedFix` "prose or code"; `side` defaults to `RIGHT`; new findings get sequential ids, `rev` 1, `included` true, `createdAt` and `updatedAt` = `now`, empty `history`. `FindingInput` has exactly the contract's `add` input fields and no `included`
- [X] T033 [US1] Implement `SetSummary` in `internal/draft/mutate.go` (depends on T032): `SetSummary(d *Draft, summary string, expectFindings *int, by string) error` checks the summary with `markdown.Check(..., Summary, "loupe summary --from -")`, and when `expectFindings` is set compares it with `IncludedCount(d)` (findings with `included: true`), refusing with `count`, message `N findings are included, expected M`, `details.included` `[{id, title}]`, fix naming those ids, and leaving the summary unchanged (makes T026 pass)
- [X] T034 [US1] Implement `internal/cli/capture.go` (depends on T015, T016, T022, T031): `loupe capture <pr-url> [--repo <path>] [--json]`. Parse `https://github.com/<owner>/<repo>/pull/<n>` (trailing path segments such as `/files` allowed); any other host or shape refuses `pr` with fix `loupe capture https://github.com/<owner>/<repo>/pull/<number>`. Order: GitHub `PullRequest` (closed refuses `pr`); `Viewer`; compute round as one higher than `Newest` (1 when none) and `previousRound` as `round - 1` when any round exists; `OriginMatches` on `--repo` or `WorkDir`; `FetchPR` into `refs/loupe/<owner>/<repo>/<number>/<round>/{base,head}`; `RevParse` both refs and refuse `head-moved` with fix `rerun loupe capture <url>` when either differs from the API's shas; `Diff`; `diff.Parse` must succeed; `CreateRun` with `diffSha256`, `clonePath` absolute and `draft.NewEmpty()`. Result payload: `target`, `refs` `{base, head}`, `cleanup` (`git -C <clone> update-ref -d <ref>` for both), `next` (`loupe add --run <ref> --from <file> --json`, `loupe summary --run <ref> --expect-findings <n> --json`, then the human runs `loupe review <ref>`). Human output prints the run reference, both refs, the cleanup commands and the next steps (FR-006). The help text shows the result shape
- [X] T035 [US1] Implement `internal/cli/add.go` (depends on T021, T032, T034): `loupe add [--from <path>|-] [--title] [--body] [--path] [--line] [--start-line] [--side] [--general] [--label] [--blocking] [--confidence] [--severity] [--suggested-fix] [--by agent|human] [--expect-version n] [--run <ref>] [--json]`; `--from` input is one finding object or an array; flags build a single finding and conflict with `--from`; loads `pr.diff` once, runs `draft.Mutate` with `Add`, result payload `findings` `[{id, rev}]` and `version`. The help shows the contract's JSON input example and result shape
- [X] T036 [US1] Implement `internal/cli/summary.go` (depends on T033, T035): `loupe summary [--from <path>|-] [--body <text>] --expect-findings <n> [--by agent|human] [--expect-version n] [--run <ref>] [--json]`; input `{"summary": "Markdown"}`; `--expect-findings` is required when `--by agent` (missing refuses `usage`) and optional for `--by human`; result payload `version`, `includedCount`
- [X] T037 [US1] Implement `internal/cli/show.go` without `--previous` (depends on T018, T035): `loupe show [--run <ref>] [--json]` returns the whole draft (including `history`, decisions, notes and replies) plus `target`, `dispositions` (`{id: disposition}`) and `readiness`; human output lists the summary and one line per finding with id, disposition, blocking marker, label, title and `path:line` or `general`
- [X] T038 [US1] Write the top-level help in `internal/cli/root.go` (depends on T034 to T037): the workflow `capture → add → summary → review → publish` with one example line per step, the send-back loop (`feedback`, `edit`, `reply`), run reference syntax `owner/repo#123` and `owner/repo#123@2`, run selection order, `--json`, `--from <file>|-`, `--expect-version`, the environment variables in contracts/cli.md, and the statement that `review` and `publish` are human-only and MUST NOT be run by an agent, pipe confirmation, or allocate a pseudo-terminal (makes T027 and T028 pass)

**Checkpoint**: `go test ./internal/markdown/... ./internal/diff/... ./internal/gitx/... ./internal/draft/... ./internal/cli/... ./internal/integration/...` passes; US1 is demonstrable with `loupe capture`, `add`, `summary`, `show`.

---

## Phase 4: User Story 2 - Human decides each finding with the diff in view (Priority: P1)

**Goal**: `loupe review` opens a Bubble Tea interface (or plain mode) where the human accepts, excludes, sends back, restores and resolves or dismisses notes, each persisted immediately with a stale-write check.

**Independent Test**: With a three-finding draft and injected keys, the interface shows the hunk for a located finding, records accept, exclude and send-back with a note, keeps them across quit and reopen, and refuses a decision whose finding changed after it was displayed.

### Tests for User Story 2 ⚠️

- [X] T039 [P] [US2] Write `internal/diff/view_test.go`: `HunkView(path, side, line, startLine, context int)` returns the hunk's lines with the anchored range flagged and at most `context` lines outside it; `FileView(path, markers map[int][]string)` returns every hunk of the file with separators and flags each RIGHT line (or LEFT for deleted lines) carrying finding ids; `NextMarker` and `PrevMarker` wrap correctly
- [X] T040 [P] [US2] Write `internal/draft/mutate_decide_test.go`: `Accept` on a pending included finding stores `{decision: accepted, findingRev: rev, at}`; `Accept` on a withdrawn finding refuses `input` ("accept is for included findings only"); `Exclude` stores `excluded`; `SendBack(id, note)` appends `n-001` with status `open`, deletes the finding's decision and leaves the finding pending; `Restore` on an excluded finding deletes its decision so it is pending; `ResolveNote` and `DismissNote` set status and `closedAt` and refuse on a note that is not open; an unknown finding or note id refuses `not-found` with fix `loupe show`; every one of these through `Mutate` with a stale expected version refuses `version` and writes nothing
- [X] T041 [P] [US2] Write `internal/render/escape_test.go`: `ForDisplay` replaces C0 controls except `\n` and `\t`, DEL, C1 controls, and U+202A to U+202E and U+2066 to U+2069 with visible escapes such as `‮`, and leaves other text including emoji unchanged
- [X] T042 [P] [US2] Write `internal/tui/term_test.go`: `ChooseMode(opts)` returns plain for `--plain`, for `TERM=dumb`, when the injected raw-mode probe fails, and for sizes 59x40 and 80x11; full-screen otherwise; `Glyphs(getenv)` returns ASCII glyphs when none of `LC_ALL`, `LC_CTYPE`, `LANG` (checked in that order) contains `UTF-8` or `utf8`, case-insensitive
- [X] T043 [US2] Write `internal/tui/app_test.go` with `charmbracelet/x/exp/teatest` over a temp run directory holding a three-finding draft (two located, one general) and a real `pr.diff`: the list shows the summary, counts `accepted`, `pending`, `excluded`, `withdrawn` and open notes, and one row per finding with disposition glyph, id, blocking marker, label, title and `path:line`; `enter` on a located finding shows its body and the hunk with anchored lines highlighted; `f` shows the file diff with markers and `]` moves between findings; `a`, `x` and `s` then typed note text then `enter` persist to `draft.json` immediately; `q` quits; a second program over the same directory shows the same dispositions (US2 AS1 to AS7). A second test opens a finding, then mutates `draft.json` through `draft.Mutate` (bumping that finding's `rev` and the version, standing in for an agent edit, which arrives in US4), presses `a`, and asserts no decision was stored, a notice is shown, and the updated title is displayed (US2 AS6, FR-023)
- [X] T044 [US2] Write `internal/tui/plain_test.go`: plain mode over the same fixture with an injected reader answering `a`, `x`, `s` then a note line, then `q`, writes the same decisions as the full-screen test; each prompt is preceded by the finding's title, body, and hunk for a located finding; an agent mutation between display and answer refuses the decision and reprints the finding (US2 AS8)

### Implementation for User Story 2

- [X] T045 [P] [US2] Implement `internal/diff/view.go`: `HunkView`, `FileView`, `NextMarker`, `PrevMarker` as specified in T039, reusing `Diff` indexes from T012 (makes T039 pass)
- [X] T046 [P] [US2] Implement `Accept`, `Exclude`, `SendBack`, `Restore`, `ResolveNote`, `DismissNote` in `internal/draft/mutate.go` per data-model.md "Decision" ("Only the review interface writes decisions"; "A send-back deletes the finding's decision") and "Note" (`closedAt` set on resolve or dismiss); note bodies are non-empty and checked with `markdown.Check(..., Body, ...)` (makes T040 pass)
- [X] T047 [P] [US2] Implement `internal/render/escape.go` with `ForDisplay(s string) string` as specified in T041. This is display only; payloads are sent verbatim (makes T041 pass)
- [X] T048 [P] [US2] Implement `internal/tui/term.go`: `ChooseMode(Options{Plain bool; Getenv; Width, Height int; RawProbe func() error}) Mode` with the thresholds 60 columns by 12 rows, and `Glyphs(getenv) GlyphSet` (UTF-8: `✓` accepted, `·` pending, `✗` excluded, `↩` withdrawn, `●` blocking; ASCII: `+`, `.`, `x`, `-`, `!`) (makes T042 pass)
- [X] T049 [US2] Implement `internal/tui/app.go` (depends on T045 to T048): root `tea.Model` holding run dir, loaded draft, parsed diff, displayed version, glyphs, color on or off (`NO_COLOR`), current view (list, detail, file diff, help, and a slot for confirm added in US3), window size, and a notice line; one width-aware layout helper that every view renders through; `?` opens a help overlay listing the keys for the current view; every string drawn from draft content passes through `render.ForDisplay`; `Decide(fn)` runs `draft.Mutate` with `expectVersion` = displayed version and no lock held otherwise, and on a `version` refusal reloads the draft, keeps the same finding open, and sets the notice `finding changed since it was displayed; nothing was recorded`; the draft is re-read after every refused write and on returning to the list
- [X] T050 [US2] Implement `internal/tui/list.go` (depends on T049): header with `owner/repo#N`, `round N` and counts from `draft.Readiness`; the summary above the rows, collapsible with `tab`; rows as in T043; keys `j`/`k`/arrows, `enter`, `?`, `q`
- [X] T051 [US2] Implement `internal/tui/detail.go` (depends on T049): top region with title, chips for label, blocking and confidence, the body rendered with `charmbracelet/glamour` (word-wrapped to width, `notty` style under `NO_COLOR`), suggested fix in a fence, and each note on the finding with status; bottom region with `HunkView` (three lines of context) with anchored lines highlighted, or `general finding`; keys `a` accept (included only), `x` exclude, `s` send back with a one-line `textinput` from `charmbracelet/bubbles`, `u` restore an excluded finding, `r`/`d` resolve or dismiss the finding's open note, `f` file diff, `n`/`N` next and previous finding, `esc` back
- [X] T052 [US2] Implement `internal/tui/filediff.go` (depends on T049): scrollable `FileView` of the open finding's file using a `bubbles/viewport`, added and removed lines colored (no syntax highlighting), gutter marker on lines with findings, `]`/`[` jump between findings, `enter` opens the finding under the cursor, `esc` back (makes T043 pass)
- [X] T053 [US2] Implement `internal/tui/plain.go` (depends on T046, T047): `RunPlain(dir string, in io.Reader, out io.Writer, getenv) error` prints the summary and counts, then one finding at a time with title, chips, body as plain text, notes, and the hunk with anchored lines prefixed `>`; reads one answer per line: `a` accept, `x` exclude, `s` send back (next line is the note), `u` restore, `r` resolve note, `d` dismiss note, `n` next, `b` previous, `q` quit; decisions use the same `Mutate` and stale-version handling as T049, reprinting the finding on refusal; end of input quits (makes T044 pass)
- [X] T054 [US2] Implement `internal/cli/review.go` (depends on T050 to T053): `loupe review [<ref>] [--plain]`; refuse with `tty`, fix `run loupe review in an interactive terminal`, before resolving or reading anything when `deps.IsTerminal()` is false (FR-022); resolve the run; choose mode with `tui.ChooseMode`; run `tea.NewProgram` with `deps.Stdin`/`deps.Stdout` or `RunPlain`. The help text states the command is human-only

**Checkpoint**: US1 and US2 tests pass; a human can decide every finding of a captured run.

---

## Phase 5: User Story 3 - Human publishes one confirmed review (Priority: P1)

**Goal**: `loupe publish` and the TUI `p` flow compose the review per `docs/comment-format.md`, show it and the exact payload, and send exactly one review on `y`, writing a receipt; a receipt makes later publishes a no-op.

**Independent Test**: With a ready draft and the fake GitHub, confirming sends exactly one request with the composed body and inline comments and writes a receipt; a second publish sends nothing and prints the URL; each refusal (no TTY, head moved, own pull request with approve, approve with a blocking finding, unready, empty, draft changed after display) names its fix.

### Tests for User Story 3 ⚠️

- [X] T055 [P] [US3] Write `internal/render/codespan_test.go`: `CodeSpan` uses a backtick run one longer than the longest run in the content and pads with a space when the content starts or ends with a backtick, and never HTML-escapes; `Fence` length is `max(3, longest backtick run + 1)`; `OneLine` collapses every whitespace run including newlines to one space and trims; `EscapeHTML` escapes `&`, `<`, `>`, `"`, `'`
- [X] T056 [P] [US3] Write `internal/render/body_test.go` and golden files in `testdata/golden/` (update with `-update`): `example.md` reproduces the `docs/comment-format.md` "Body composition" example byte for byte for the same input (round 2, `request-changes`, one blocking `issue` at `internal/publish/publish.go:88` with confidence `high` and a suggested fix, one nonblocking `perf-nit` at `internal/draft/store.go:10–14` with severity `minor`), with the digest and publication id injected; further goldens: `approve.md` (NOTE, **Approved**), `comment-blocking.md` (IMPORTANT, **Comment** with blocking count), `comment-plain.md` (NOTE), `summary-only.md` (no findings), `unlabeled-blocking.md` (`⚪ <b>(blocking):</b>`), `left-side.md` (location suffix ` (LEFT)` and anchor `L`), `hostile.md` (title with a newline and `</summary>`, severity with backticks and a newline, suggested fix containing a ```` ``` ```` run); assert section order Blocking, Issues, Suggestions, Questions, Other; Blocking sorted by label group then id; chips omit zeros and count blocking only in the blocking chip; `loupe-meta` counts every included finding by label including blocking ones; a blank line precedes every `---`; excluded and withdrawn findings appear nowhere
- [X] T057 [P] [US3] Write `internal/render/inline_test.go` with goldens in `testdata/golden/inline-*.json`: mode `none` yields no comments; `blocking` yields every accepted, located, blocking finding; `all` yields every accepted, located finding; general findings are never inline; a range yields `start_line` and `start_side`; a comment body is the summary line as a bold first line with dot and label word, the meta block with a plain code-span location (no link), the body and the suggested fix, without `<details>`
- [X] T058 [P] [US3] Write `internal/draft/digest_test.go`: the digest is stable across reordering of `findings` in the file, across changes to notes, replies, `history`, `by` and timestamps, and across accepting a pending finding (accept decisions do not change it); it changes when the summary or any publishable field of a pending or accepted finding changes, when a finding is human-excluded or restored, and when a finding is withdrawn or included; a draft whose findings are all pending and the same draft after every finding is accepted have equal digests; `internal/cli/show_test.go` asserts `show --json` carries `digest` equal to `draft.Digest` of the stored draft
- [ ] T059 [P] [US3] Write `internal/publish/publish_test.go` with `fakegh` and a temp run: gates run in the order no-TTY (`tty`), head moved (`head-moved`, fix `loupe capture <url>`), viewer is author with `approve` or `request-changes` (`own-pr`, fix `--action comment`), `approve` with an accepted blocking finding (`blocking`, fix naming `--action comment`, `--action request-changes`, and excluding or unblocking the finding in `loupe review`), empty draft (`empty`, fix `loupe add` / `loupe summary`), not ready (`not-ready`, fix `loupe review`); a failing allowlist recheck on an accepted finding body or the summary refuses `markdown`; a composed body over 256 KiB refuses with fix `exclude a finding with loupe edit <id> --exclude or shorten bodies`; a declined confirmation sends nothing and writes nothing; a draft mutated between display and `y` refuses `version` and sends nothing; a `receipt.json` written, or an `attempt.json` written, from inside the `Confirm` callback (standing in for a second publisher that passed the first check while the lock was released) makes the confirmed publish refuse and send nothing, and the fake receives zero POSTs from it; `y` sends exactly one `CreateReview` with `commit_id` = captured head, `event` from the action, the composed body and comments, then writes `receipt.json` and removes `attempt.json`; a second call prints the receipt URL without contacting GitHub for anything; a definite 4xx refuses `github` with the message, removes `attempt.json`, and leaves `draft.json` byte-identical; `CreateReview` is the only write the fake receives in every test
- [ ] T060 [P] [US3] Write `internal/tui/confirm_test.go` with `teatest`: the confirmation view shows the rendered body with every `<details>` shown open and each inline comment, with control and bidi characters escaped; `v` and `tab` toggle to the exact envelope JSON and back; `y` returns confirmed; `n`, `esc`, `ctrl+c` and any other key return declined; the action picker offers only `comment` when the viewer is the author and shows `approve` disabled with the `blocking` reason when an accepted finding blocks; the `p` key on the list is refused with the `not-ready` fix when the draft is not ready
- [ ] T061 [US3] Write `internal/integration/publish_test.go`: capture → add three findings → summary → `review --plain` with injected answers accepting two and excluding one → `publish --action comment --inline all --plain` with stdin `y` → exactly one POST, `receipt.json` present, stdout ends with the review URL; the POST body matches the rendered body and has two comments; a second `publish` sends nothing and prints the same URL (US3 AS1 to AS3); and separate scenarios for `IsTerminal` false refusing `tty` before anything is printed on stdout, `gitrepo.PushHead` plus `fakegh.SetHead` refusing `head-moved`, viewer equal to author with `--action approve` refusing `own-pr`, a blocking accepted finding with `--action approve` refusing `blocking`, a pending finding refusing `not-ready`, and an empty draft refusing `empty`

### Implementation for User Story 3

- [X] T062 [P] [US3] Implement `internal/render/codespan.go` with `CodeSpan`, `Fence`, `OneLine` and `EscapeHTML` per `docs/comment-format.md` "The meta block" and "Suggested fix" (makes T055 pass)
- [X] T063 [P] [US3] Implement `internal/draft/digest.go`: `PublishableSet(d *Draft) []Finding` returns findings that are "`included` and not human-excluded: disposition `accepted` or `pending`" (data-model.md "Publishable set"); `Digest(d *Draft) string` is hex SHA-256 over a canonical JSON encoding of `{"summary": ..., "findings": [...]}` where findings are the publishable set, sorted by id, each reduced to `id` and the publishable fields `title`, `body`, `location`, `general`, `label`, `blocking`, `confidence`, `severity`, `suggestedFix` in that fixed key order; add `digest` (`draft.Digest`) to the `loupe show` payload in `internal/cli/show.go` per contracts/cli.md, and to its help text (makes T058 pass)
- [X] T064 [US3] Implement `internal/render/body.go` (depends on T062): `Input{Owner, Repo string; Number, Round int; HeadSHA, Action, Inline, Summary, Digest, PublicationID string; Findings []Finding}` where `Finding` carries id and publishable fields; `Body(in) string` composes, in order, the verdict callout from the "Verdict" table with ` — N blocking finding(s).` when non-zero, the chips row per "Chips", the summary, a `---` divider, sections in fixed order with `### <Section>` and one `<details>` per finding whose `<summary>` follows "The summary line" table (dot and label word in Blocking, neither in label sections, label word only in Other; `(blocking)` suffix; `<b>` the only tag; title collapsed then HTML-escaped), the meta block per "The meta block" (location as a link `https://github.com/<o>/<r>/pull/<n>/files#diff-<sha256 hex of path>R<line>` or `R<start>-R<line>`, `L` for LEFT, en dash `–` in ranges, ` (LEFT)` suffix; `**Confidence:** <level>`; `` severity `<text>` ``; lines joined by `\` hard break), the body, `**Suggested fix**` in a `Fence`, dividers preceded by blank lines, the footer `` loupe · round N · reviewed `<7-char sha>` `` as a `CodeSpan`, then `<!-- loupe digest=<digest> publication=<uuid> -->` and `<!-- loupe-meta v=1 round=N inline=<mode> blocking=B issues=I suggestions=S questions=Q other=O -->` (makes T056 pass)
- [X] T065 [US3] Implement `internal/render/inline.go` (depends on T064): `Comments(in Input) []github.ReviewComment` selecting findings per the "Inline modes" table and rendering each per its last paragraph, with `start_line`/`start_side` for ranges (makes T057 pass)
- [X] T066 [US3] Implement `internal/publish/records.go`: `Envelope` with every field in data-model.md "Envelope" (`target {owner, repo, number, headSha, round}`, `viewer`, `action`, `event` "`COMMENT`, `APPROVE`, `REQUEST_CHANGES`", `commitId` "equals `target.headSha`", `draftVersion`, `digest`, `publicationId` "UUID v4, generated per attempt", `inline` "`none`, `blocking`, `all`", `body` "at most 256 KiB", `comments`, `findings [{id, title, body, location, label, blocking}]`); `Attempt` (`schema` 1, `state` "`in-flight` or `unknown`", `startedAt`, `updatedAt`, `envelope`, `confirmed {version, digest, dispositions}`, `lastError`); `Receipt` (`schema` 1, `reviewId`, `reviewUrl`, `action`, `postedAt`, `envelope`); load and atomic save for `attempt.json` and `receipt.json`, delete for `attempt.json` only; `newPublicationID()` builds a v4 UUID from `crypto/rand`
- [ ] T067 [US3] Implement `internal/publish/envelope.go` (depends on T063 to T066): `Build(target run.Target, d *draft.Draft, viewer, action, inline string) (Envelope, error)` rechecks `markdown.Check` for the summary and every accepted finding body (fixes `loupe summary --from -` and `loupe edit <id> --from -`), composes body and comments, and refuses a body over 256 KiB with code `markdown`, `details.rule` `limit`, fix `exclude a finding with loupe edit <id> --exclude or shorten bodies`
- [ ] T068 [US3] Implement `internal/publish/gates.go` (depends on T018): `Gates(ctx, in GateInput) error` in the order of research.md "Publication" step 3 with `empty` checked before `not-ready`: `tty`, `head-moved` (refetch `PullRequest`, compare `HeadSHA` with `target.headSha`), `own-pr` (`viewer == author` and action is `approve` or `request-changes`), `blocking` (action `approve` and any accepted finding has `blocking`), `empty` (empty summary and no accepted findings), `not-ready`; each refusal carries the fix in the contracts/cli.md table
- [ ] T069 [US3] Implement `internal/publish/publish.go` (depends on T067, T068): `Run(ctx, Options{Dir, Target, GitHub, IsTerminal, Action, Inline, RetryUnknown, Confirm func(Preview) (bool, error), Now, Getenv}) (Receipt, error)`. Under the run lock: if `receipt.json` exists return it without any GitHub call; if `attempt.json` exists refuse `attempt` with the pull request URL and fix `inspect <url>, then loupe publish --retry-unknown` (reconciliation arrives in US7); release the lock. Run `Gates`; `Build`; call `Confirm` with `Preview{Body, Comments, EnvelopeJSON, Version, Digest, Dispositions}` holding no lock, so agent `reply` calls are not blocked while the human reads; declined returns a `declined` result that writes nothing. On confirm: recheck head and viewer; retake the lock; refuse with `attempt` (fix `loupe publish` to see its state) if `receipt.json` or `attempt.json` now exists, because the lock was released during confirmation and a second publisher may have passed the first check, confirmed and sent; refuse if version, digest or readiness differ from the preview (`version`, fix `loupe review`); write `attempt.json` `in-flight` with the envelope and confirmed dispositions; call `CreateReview` exactly once; 2xx writes `receipt.json` then deletes the attempt; a definite 4xx deletes the attempt and refuses `github` with GitHub's message; anything else sets the attempt `unknown` with `lastError` and refuses `attempt`; unlock. The draft is never written. This is the only call site of `CreateReview` (makes T059 pass)
- [ ] T070 [US3] Implement `internal/tui/confirm.go` and the `p` flow in `internal/tui/list.go` (depends on T049, T069): `p` checks readiness and shows the `not-ready` refusal and fix in the notice when not ready; an action picker (`comment`, `approve`, `request-changes`) disabling `approve` and `request-changes` with the `own-pr` reason when the viewer is the author and disabling `approve` with the `blocking` reason; an inline picker (`none`, `blocking` default, `all`); then the confirmation view as specified in T060 wired as `publish.Options.Confirm`; any publish refusal shows reason and fix and returns to the list; success shows the review URL (makes T060 pass)
- [ ] T071 [US3] Implement plain confirmation in `internal/tui/plain.go` (depends on T069): `ConfirmPlain(in, out) func(publish.Preview) (bool, error)` prints the rendered body with details shown open, each inline comment, and the envelope JSON, all through `render.ForDisplay`, then `Publish this review? [y/N]`; only a line equal to `y` confirms; end of input declines
- [ ] T072 [US3] Implement `internal/cli/publish.go` (depends on T070, T071): `loupe publish [<ref>] --action comment|approve|request-changes [--inline none|blocking|all] [--retry-unknown] [--plain]`; `--inline` defaults to `blocking`; a missing or invalid `--action` refuses `usage`; the receipt replay path runs before the TTY check so a replay prints the URL anywhere; otherwise refuse `tty` before printing anything; choose the confirmation surface with `tui.ChooseMode`; print the review URL on success and on replay. The help text states the command is human-only (makes T061 pass)

**Checkpoint**: US1 to US3 pass; the MVP loop capture → add → summary → review → publish → receipt runs in `go test`.

---

## Phase 6: User Story 4 - Send-back loop between human and agent (Priority: P2)

**Goal**: The agent reads open notes with `feedback`, revises or withdraws findings with `edit`, answers with `reply`; any change removes acceptance; only the human closes notes.

**Independent Test**: A note created through injected review input is listed by `feedback`; the agent edits the finding and replies; the human sees the reply and resolves the note; readiness becomes true. A reply carrying a decision or status is refused.

### Tests for User Story 4 ⚠️

- [ ] T073 [P] [US4] Write `internal/draft/mutate_edit_test.go`: `Edit` changing each publishable field increments `rev`, removes the decision, records `history` `{at, by, changed: {field: previousValue}}` and updates `updatedAt`; an edit that sets a field to its current value changes nothing and does not bump `rev`; `null` clears `location`, `label`, `confidence`, `severity` and `suggestedFix` and is refused with `input` for `title`, `body` and `blocking`; setting `location` clears `general` and setting `general: true` clears `location`; a new location is validated against the diff; `Withdraw` sets `included` false, bumps `rev`, removes the decision; `Include` on a withdrawn finding sets `included` true and leaves it pending; `Reply(noteId, body, by)` appends `r-001` and changes no note status or decision; an unknown id refuses `not-found`
- [ ] T074 [P] [US4] Write `internal/cli/edit_test.go` and `internal/cli/reply_test.go`: edit input `{"location": null}` clears the location while an absent key leaves it; edit input carrying `included` refuses `input`; reply input `{"body": "x", "status": "resolved"}` and `{"body": "x", "decision": "accepted"}` refuse `input` (US4 AS5); `--include` with `--exclude` refuses `usage`; `--clear-label` with `--label` refuses `usage`
- [ ] T075 [US4] Write `internal/integration/feedback_test.go`: after capture, add and summary, plain review accepts `f-001` and sends back `f-002` with a note; `feedback --json` lists `n-001` open on `f-002`, dispositions and `ready: false` (US4 AS1); `reply n-001` attaches `r-001` and the note stays open (AS2); `edit f-001 --title ...` returns `clearedDecision: true` and `f-001` is pending (AS3); plain review shows the reply under the note, accepts both findings and resolves `n-001`; `feedback` reports `ready: true`; a withdrawn finding with an open note makes `publish` refuse `not-ready` (AS4)

### Implementation for User Story 4

- [ ] T076 [US4] Implement `Edit`, `Withdraw`, `Include` and `Reply` in `internal/draft/mutate.go` (depends on T046): `EditInput` uses `*json.RawMessage` per field so absent, `null` and a value are distinguishable (plan.md Design Notes); allowed `null` fields are exactly `location`, `label`, `confidence`, `severity`, `suggestedFix`; validation of new values reuses the `Add` rules; `rev` increments "on any change to a publishable field or to `included`" and the finding's decision is deleted; each change appends one history entry with previous values; replies are non-empty, allowlist-checked with fix `loupe reply <note-id> --from -`, and record `by` (makes T073 pass)
- [ ] T077 [US4] Implement `internal/cli/edit.go` (depends on T076): `loupe edit <finding-id> [--from <path>|-] [add flags] [--clear-location] [--clear-label] [--clear-confidence] [--clear-severity] [--clear-suggested-fix] [--not-blocking] [--include|--exclude] [--by agent|human] [--expect-version n] [--run <ref>] [--json]`; `--exclude` withdraws and `--include` restores, neither recording a human decision; result payload `finding` `{id, rev, included}`, `version`, `clearedDecision`; help shows the input shape including `null` clearing
- [ ] T078 [US4] Implement `internal/cli/reply.go` (depends on T076): `loupe reply <note-id> [--from <path>|-] [--body <text>] [--by agent|human] [--expect-version n] [--run <ref>] [--json]`; input `{"body": "Markdown"}`; result payload `reply` `{id, noteId}`, `version` (makes T074 pass)
- [ ] T079 [US4] Implement `internal/cli/feedback.go` (depends on T018): `loupe feedback [--run <ref>] [--json]`; result payload `readiness` `{ready, accepted, pending, excluded, withdrawn, openNotes}` with id lists, `notes` each `{id, findingId, status, body, at, replies[]}`, `findings` `[{id, title, disposition, rev}]`; human output groups open notes first with their replies
- [ ] T080 [US4] Show replies in `internal/tui/detail.go` and `internal/tui/plain.go` (depends on T051, T053): under each note, list replies in order with `by` and body rendered through `render.ForDisplay` (makes T075 pass)

**Checkpoint**: US1 to US4 pass independently.

---

## Phase 7: User Story 5 - Resume without identifiers (Priority: P2)

**Goal**: With no run named, commands resolve the current branch's pull request at its newest round; `list` shows every run.

**Independent Test**: With runs for two pull requests, `review` with no argument from a branch tied to one selects it; `list` shows both newest first with correct state and counts.

### Tests for User Story 5 ⚠️

- [ ] T081 [P] [US5] Write `internal/run/branch_test.go` with `fakegh` and `gitrepo`: `ResolveBranch` returns the single open pull request for the current branch using the origin's owner and repo; zero results refuse `no-run` with fix `loupe capture <url>` or `--run <ref>`; two results refuse `no-run` with `details.pullRequests` `[{number, base}]` and fix `--run <ref>`; a detached HEAD refuses `no-run`; `Walk(root)` returns every run directory with its loaded target
- [ ] T082 [US5] Write `internal/integration/resume_test.go`: capture pull requests 7 and 8 from branches `feature-a` and `feature-b` in the same clone; with `feature-b` checked out and no `LOUPE_RUN`, `show --json` resolves `owner/repo#8@1` and `review --plain` opens it (US5 AS1); on a branch with no open pull request, `review` refuses `no-run` (AS2); `list --json` returns both runs newest capture first with `ref`, `url`, `title`, `round`, `state`, `counts`, `capturedAt`, and states `captured`, `ready` and `published` each appear across variants (AS3)

### Implementation for User Story 5

- [ ] T083 [US5] Add `CurrentBranch(clone) (string, error)` to `internal/gitx/gitx.go` via `git symbolic-ref --short -q HEAD`, returning a `no-run` refusal on detached HEAD
- [ ] T084 [US5] Implement `internal/run/branch.go` (depends on T083): `ResolveBranch(ctx, root, clone string, gh github.Client) (Ref, error)` reads the origin owner and repo with `gitx.ParseGitHubURL`, calls `PullRequestsForBranch(owner, repo, branch)` (plan.md Design Notes: `head={owner}:{branch}` with the origin owner), and returns the pull request's newest round; and `Walk(root) ([]Entry, error)` over `runs/<owner>/<repo>/<number>/<round>` returning `{Ref, Dir, Target}` (makes T081 pass)
- [ ] T085 [US5] Extend `resolveRun` in `internal/cli/run.go` with the branch fallback after `LOUPE_RUN` (depends on T084), used by every run-scoped command including `review` and `publish`
- [ ] T086 [US5] Implement `internal/cli/list.go` (depends on T084): `loupe list [--json]`; state is derived per data-model.md "`published` if `receipt.json` exists, else `ready` if readiness holds, else `captured`"; counts from `draft.Readiness`; sorted by `capturedAt` descending; a damaged run refuses `record` naming the file rather than being skipped; human output is one aligned row per run (makes T082 pass)

**Checkpoint**: US1 to US5 pass independently.

---

## Phase 8: User Story 6 - Follow-up rounds on the same pull request (Priority: P2)

**Goal**: Capture after new pushes creates a linked round; the same unpublished head is refused; the agent reads the newest earlier published round's findings.

**Independent Test**: After a published round 1, the head moves; capture creates round 2 linked to round 1; `show --previous` returns round 1's published findings; round 2's published footer names round 2.

### Tests for User Story 6 ⚠️

- [ ] T087 [P] [US6] Write `internal/run/previous_test.go`: `PreviousPublished(root, ref)` for round 3 returns round 1 when round 2 has no `receipt.json` and round 1 does (clarification 2); returns round 2 when both are published; refuses `not-found` with message `no earlier round of owner/repo#N was published` when none is
- [ ] T088 [US6] Write `internal/integration/rounds_test.go`: publish round 1; `PushHead` and `SetHead`; capture creates round 2 with `previousRound` 1 (US6 AS1); `show --previous --json` returns `{round: 1, reviewUrl, findings: [{id, title, body, location, label, blocking}]}` matching round 1's receipt (AS2); publishing round 2 produces a footer `loupe · round 2 · reviewed` (AS3); with round 2 unpublished, a new push and capture creates round 3 and round 2 stays readable with `show --run owner/repo#N@2` (AS4); capturing again at round 3's head while it is unpublished refuses `same-head` naming `owner/repo#N@3` and creates no directory or ref (AS5); after publishing round 3, capturing at the same head creates round 4 (AS6); `show --previous` on round 1 refuses `not-found`

### Implementation for User Story 6

- [ ] T089 [US6] Implement `internal/run/previous.go`: `PreviousPublished(root string, ref Ref) (round int, dir string, err error)` walking rounds downward from `round - 1` to the first with `receipt.json` (makes T087 pass)
- [ ] T090 [US6] Add the same-head check to `internal/cli/capture.go` before `OriginMatches` and before any fetch (depends on T089): when the newest round's `headSha` equals the API's head sha and that round has no `receipt.json`, refuse `same-head` with message naming the run reference and fix `--run <ref>`; a published round at the same head proceeds (FR-005)
- [ ] T091 [US6] Implement `--previous` in `internal/cli/show.go` (depends on T089, T066): load the found round's `receipt.json` and return `{round, reviewUrl, findings}` from `envelope.findings` (makes T088 pass)

**Checkpoint**: US1 to US6 pass independently.

---

## Phase 9: User Story 7 - Recover an ambiguous publish outcome (Priority: P3)

**Goal**: An unknown attempt is reconciled against GitHub by the hidden marker before anything else; without a match only `--retry-unknown` sends again, after every gate and a fresh confirmation.

**Independent Test**: The fake records the review and returns 500; the attempt is unknown; the next publish finds the marker and writes a receipt without sending. With the fake dropping the request, the next publish refuses until `--retry-unknown`, which sends once.

### Tests for User Story 7 ⚠️

- [ ] T092 [P] [US7] Write `internal/publish/reconcile_test.go`: `Match(reviews, attempt)` matches a review whose `user.login` equals `envelope.viewer`, `commit_id` equals `envelope.commitId`, `state` is not `PENDING`, and body contains the exact line `<!-- loupe digest=<digest> publication=<publicationId> -->`; a `PENDING` review with the marker does not match; a review by another user with the marker does not match; a 422 whose message contains `pending review` refuses `github` with fix `submit or discard your pending review on <pr url> first` (US7 AS5)
- [ ] T093 [US7] Write `internal/integration/unknown_test.go`: with `ServerErrorAfterRecord`, publish refuses `attempt`, `attempt.json` is `unknown` with the envelope and `draft.json` is byte-identical (AS1); the next publish lists reviews first, writes `receipt.json` from the saved envelope, deletes the attempt, sends no POST and prints the URL (AS2). With `ServerErrorDrop`, the next publish without `--retry-unknown` refuses `attempt` naming the pull request URL and sends nothing (AS3); `--retry-unknown` with stdin `y` passes the gates, shows a new confirmation, and sends exactly one more POST with a new `publicationId`, then writes a receipt (AS4); with `--retry-unknown` and a head that moved, it refuses `head-moved` and keeps the unknown attempt

### Implementation for User Story 7

- [ ] T094 [US7] Implement `internal/publish/reconcile.go`: `Match` as in T092 and `Reconcile(ctx, gh, attempt) (*Receipt, error)` listing all reviews and building a receipt from the saved envelope and the matched review's id and URL (makes T092 pass)
- [ ] T095 [US7] Extend `internal/publish/publish.go` (depends on T094): under the first lock, when `attempt.json` exists, reconcile before any gate; a match writes the receipt, deletes the attempt and returns; no match without `RetryUnknown` refuses `attempt` with the pull request URL; with `RetryUnknown` remember the unknown attempt's `publicationId`, continue through every gate and a fresh confirmation, and after `y` with the lock retaken refuse if `receipt.json` exists or `attempt.json` is no longer that same unknown attempt; only then overwrite the attempt with a new `in-flight` one carrying a new `publicationId`. After a send ends ambiguous, mark the attempt `unknown`, reconcile once, and refuse `attempt` if still unmatched. A definite 422 whose message mentions a pending review uses the submit-or-discard fix line (FR-030, FR-031) (makes T093 pass)

**Checkpoint**: US1 to US7 pass independently.

---

## Phase 10: User Story 8 - Claude Code plugin (Priority: P3)

**Goal**: A plugin whose `/loupe <pr-url>` command runs the capture, investigate and report workflow with the ordinary CLI and hands `loupe review` to the user.

**Independent Test**: The plugin files parse as the host's plugin format, the skill contains the workflow and the prohibitions, and the command passes the URL through.

### Tests for User Story 8 ⚠️

- [ ] T096 [US8] Write `internal/cli/plugin_test.go` reading files relative to the repository root: `plugin/.claude-plugin/plugin.json` and `.claude-plugin/marketplace.json` parse as JSON with the required keys of the current Claude Code plugin and marketplace formats; `plugin/skills/loupe/SKILL.md` has YAML frontmatter with `name` and `description`, and its body contains `loupe capture`, `loupe show --previous`, `git show`, `loupe add --from`, `loupe summary --expect-findings`, `loupe feedback`, `loupe reply`, `loupe review`, and prohibitions on running `loupe review`, running `loupe publish`, allocating a pseudo-terminal, piping confirmation, and creating GitHub reviews by any other route; `plugin/commands/loupe.md` contains `$ARGUMENTS` and names the skill

### Implementation for User Story 8

- [ ] T097 [P] [US8] Create `plugin/.claude-plugin/plugin.json` and `.claude-plugin/marketplace.json` (marketplace entry with `source: "./plugin"`), confirming the current key set against the Claude Code plugin documentation before writing
- [ ] T098 [P] [US8] Write `plugin/skills/loupe/SKILL.md` per research.md "Claude Code plugin" using RFC 2119 keywords and one line per paragraph: capture the URL with `--json` and keep the run reference; when `target.previousRound` is set, run `loupe show --previous --json` and check each published finding; investigate only with `git show <headSha>:<path>` and `git diff <baseRef> <headRef>`, never checking out; write findings to a file and run `loupe add --run <ref> --from <file> --json`; run `loupe summary --run <ref> --expect-findings <n> --json`; when asked to handle feedback, run `loupe feedback`, then `loupe edit` and `loupe reply`; finish by telling the user to run `loupe review <ref>` in their own terminal. It MUST state the agent MUST NOT run `loupe review` or `loupe publish`, allocate a pseudo-terminal, pipe or script confirmation, or write GitHub reviews by any other route
- [ ] T099 [US8] Write `plugin/commands/loupe.md` that invokes the `loupe` skill with `$ARGUMENTS` as the pull request URL (depends on T098; makes T096 pass)

**Checkpoint**: All user stories pass.

---

## Phase 11: Polish & Cross-Cutting Concerns

**Purpose**: Constitution checks, performance, distribution, final validation.

- [ ] T100 [P] Add `scripts/check-tests.sh` and call it from the `check` task in `mise.toml`: fail when any `_test.go` or `internal/testutil` file imports a pseudo-terminal package (`creack/pty`, `github.com/*/pty`, `os/exec` of `script`), when any test file contains `api.github.com` or a non-loopback `http://` or `https://` host other than inside `github.com/<owner>/<repo>` URL literals, or when `CreateReview(` is called outside `internal/publish/publish.go`, `internal/github/` and `internal/testutil/fakegh/` (SC-007, constitution II and VII)
- [ ] T101 [P] Add `BenchmarkParseAndLocate` in `internal/diff/perf_test.go` generating a 500-file synthetic diff in the test, and `TestOpenUnder100ms` in `internal/tui/perf_test.go` that builds a 500-file draft and diff in a temp run and asserts the model's first render of the first finding's detail and hunk completes under 100 ms, skipped under `-short` (SC-004)
- [ ] T102 [P] Create `.goreleaser.yaml` building `./cmd/loupe` with `CGO_ENABLED=0` for linux and darwin on amd64 and arm64, archives with checksums, and a GitHub Releases publisher; no `brews` entry, since the Homebrew tap is deferred (research.md "Distribution"); the file MUST pass `goreleaser check`. No release is run (constitution, Development Workflow)
- [ ] T103 Run the `cleanup-comments` skill over the whole tree and fix what it reports in the affected `internal/**` files
- [ ] T104 Walk every row of the "Automated validation" table in `specs/001-loupe-v1/quickstart.md`, name the passing test for each, and run `mise run check` showing its output; any row without a passing test is reported as unverified, together with the owner's manual checks listed in `HANDOFF.md`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: none.
- **Foundational (Phase 2)**: depends on Setup; blocks every story.
- **US1 (Phase 3)**: depends on Foundational.
- **US2 (Phase 4)**: depends on Foundational; its tests build runs directly on disk, so it does not need US1's commands, but `review` reads runs that US1's `capture` creates for manual use.
- **US3 (Phase 5)**: depends on US2 (decisions, `render.ForDisplay`, TUI shell, plain mode) and uses US1's commands in its integration test.
- **US4 (Phase 6)**: depends on US2 (notes, decisions, detail view); integration uses US1 and US3.
- **US5 (Phase 7)**: depends on Foundational and US1; `list` state uses US3's receipt.
- **US6 (Phase 8)**: depends on US1 (capture) and US3 (receipts, footer).
- **US7 (Phase 9)**: depends on US3.
- **US8 (Phase 10)**: depends on the command surface of US1 and US4 being final; no code dependency.
- **Polish (Phase 11)**: depends on every story included in the release.

### Within Phase 2

T006 first. Then T007 to T014 in parallel. T015 after T007 and T009. T016 after T008 and T015. T017 after T009 to T011. T018 after T011. T019 after T013. T020 and T021 after T006. T022 after T013, T016 and T020.

### Within Each User Story

- Tests before implementation, and each test run MUST fail first.
- Leaf packages (`markdown`, `diff`, `gitx`, `render`) before `draft` mutations, `draft` before `publish`, domain before `tui`, `tui` and domain before `cli`.
- Tasks editing the same file (`internal/draft/mutate.go`, `internal/tui/plain.go`, `internal/cli/capture.go`, `internal/cli/show.go`, `internal/publish/publish.go`) are sequential even across stories.

### Parallel Opportunities

- Phase 1: T002 to T004.
- Phase 2: T007 to T014, then T018 to T021.
- US1: tests T023 to T027; implementation T029 to T031.
- US2: tests T039 to T042; implementation T045 to T048.
- US3: tests T055 to T060; implementation T062 and T063.
- US4: tests T073 and T074.
- US5 and US6 can proceed in parallel once US3 is complete; their CLI edits land in different files (T085 in `internal/cli/run.go`, T090 in `internal/cli/capture.go`), but T083 and T091 both follow earlier edits to `gitx.go` and `show.go`.
- US8 can be written any time after US4's command surface is final.
- Polish: T100 to T102.

---

## Parallel Example: User Story 1

```bash
# Tests, all failing first:
Task: "Write internal/markdown/allowlist_test.go"
Task: "Write internal/diff/locate_test.go"
Task: "Write internal/gitx/gitx_test.go"
Task: "Write internal/draft/mutate_add_test.go"
Task: "Write internal/cli/help_test.go"

# Leaf implementations:
Task: "Implement internal/markdown/allowlist.go"
Task: "Implement internal/diff/locate.go"
Task: "Implement internal/gitx/gitx.go"
```

## Parallel Example: User Story 3

```bash
Task: "Write internal/render/codespan_test.go"
Task: "Write internal/render/body_test.go and testdata/golden/"
Task: "Write internal/render/inline_test.go"
Task: "Write internal/draft/digest_test.go"
Task: "Write internal/publish/publish_test.go"
Task: "Write internal/tui/confirm_test.go"
```

---

## Implementation Strategy

### MVP First

The spec marks US1, US2 and US3 all P1, and none delivers the product alone: filing without deciding is a relay, and deciding without publishing has no deliverable. The MVP is Phases 1 to 5.

1. Phase 1 and Phase 2.
2. Phase 3 (US1): stop and validate that an agent can capture and file from `--help` alone.
3. Phase 4 (US2): stop and validate decisions persist and stale writes are refused.
4. Phase 5 (US3): stop and validate the full capture-to-receipt integration test.

### Incremental Delivery

1. MVP (US1 to US3): one round, accept and exclude, one published review.
2. US4: send-back loop in the terminal.
3. US5: no more copying run references.
4. US6: second and third rounds.
5. US7: ambiguous-outcome recovery.
6. US8: Claude Code plugin.
7. Polish, then the owner's manual checks from `HANDOFF.md`.

---

## Notes

- `[P]` tasks touch different files and have no dependency on incomplete tasks.
- Each task ends with `mise run check` passing and one Conventional Commit.
- A task is not done until the quickstart.md row it implements is green (quickstart.md "Automated validation").
- Stop and ask the owner when a task's instruction conflicts with the spec, research, contract or `docs/comment-format.md`; the constitution outranks all of them.
