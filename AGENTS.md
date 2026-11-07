# AGENTS.md

Rules for anyone, human or agent, changing this repository. They are not suggestions. If a rule gets in the way of doing the right thing, change the rule in the same pull request and say why.

## What lo0 is

A tool for setting up and using local AI for development. It reads a developer's machine and coding-agent history, recommends the models and harnesses that fit, installs them, lets the user try and compare them, and moves their setup onto the one they choose. The assessment (Scan → Match → Report) only reads. Anything that changes the machine, such as installing a harness, downloading a model or rewriting a config, is shown as a plan first and runs only after the user confirms.

## Toolchain

- Go 1.27 or newer. The module's `go` directive is the floor; do not lower it.
- `make check` must pass before every commit. It runs formatting, `go vet`, staticcheck, golangci-lint and the race-enabled test suite.
- Linters run through `go tool` from `tools/go.mod`, so nobody installs anything globally. The one exception is actionlint, which needs a different version of a YAML library than golangci-lint; CI runs it pinned with `go run`.
- Supported platforms: macOS, Linux and Windows on amd64 and arm64. CI tests all six natively, on their own runners. Never use a build tag where a runtime check on an injected value would do, because it hides code from tests on other platforms.

## Architecture

```
cmd/lo0                 entry point only: builds the app and calls cli.Run
cmd/lo0wasm             entry point for the site's hardware page, built to
                        WebAssembly with make wasm
e2e                     end-to-end tests: runs the built binary as a user would
internal/cli            command-line front end: flags, output mode, progress
internal/ui             terminal theme, report rendering, progress view,
                        block-character charts (internal/ui/draw) and the
                        interactive TUI (internal/ui/tui)
internal/web            browser front end: pages served to this machine
                        only, embedded templates, styles and fonts
internal/browser        front end for the website: assesses hardware the
                        visitor names, with no machine or history to read
internal/wiring         composition root shared by every front end
internal/app            use cases (Scan, Assess, PlanMigration, Migrate): the
                        only API front ends call
internal/match          model fit, harness recommendation and the verdict
internal/harness        what a setup becomes in each target harness; one
                        subpackage per harness (claude, opencode, codex) holds its
                        feature mapping, its migration plan and how
                        it starts for one session
internal/migrate        plans and applies migrations: steps, effects, Apply
internal/backend        prepares a model in each local runtime for Migrate
internal/catalog        embedded model knowledge base and its generator
internal/scan           orchestrates the read-only stages and reports progress
internal/machine        hardware profile and GPU probes
internal/runtimes       local model runtimes (Ollama, LM Studio, llama.cpp)
internal/claudecode     Claude Code layout, setup inventory, transcripts,
                        their summariser (history) and installed builds
internal/codex          Codex layout, setup inventory (setup), session
                        logs (rollout) and their summariser (history)
internal/usage          turns session summaries from any harness into a
                        usage profile; knows no harness's file formats
internal/format         number, size and date formatting shared by all text
internal/version        compares and parses release numbers
internal/jsonl          reads JSONL logs of any line length
internal/redact         replaces project paths and home in shared output
internal/system         process execution behind an interface
site                    the lo0.ai website (Astro), deployed separately
```

Dependency direction is strict: front ends (`cli`, `ui`, `web`, `browser`, a future desktop app) → `wiring` → `app` → `match` → `scan` → domain packages → `system`. The history seam is `usage.Session`: each harness package (`claudecode`, `codex`) reads its own logs and summarises every session as a `usage.Session`, and nothing above `usage` sees a harness's records. `usage` must not import a harness package; a test enforces it. A field only one harness can fill stays zero for the others, and the report leaves zero rows out. `usage.Pool` profiles every harness's sessions together for fit, and keeps each harness's own profile in `Profile.Harnesses`; anything tied to one harness's setup, prices or versions reads it through `Profile.Of`, never the pool. Nothing below `app` imports `ui`, `cli` or Bubble Tea. Domain types carry data, not presentation. Any decision a user sees, such as a verdict or a recommendation, is made in `match` or below, never in a front end, so every front end gives the same answer.

The model catalogue is data, not code. Every "good at" and "watch out" line in a model's profile must be backed by the model card it links to; lo0's own machine-specific observations live in `match.Insights` and are always shown apart from the lab's claims.

API prices live in `internal/cost/prices.go` (Anthropic) and `internal/cost/openai.go` (OpenAI), copied from each provider's pricing page with the date they were taken (`PricesAsOf`, `OpenAIPricesAsOf`). ChatGPT plan prices in `internal/cost/plan.go` carry their date in `ChatGPTPlanFor`'s comment. Update prices and dates together, and never quote a price from memory.

The model catalogue is generated, not written. Change `internal/catalog/sources.json` and run `go generate ./internal/catalog`; never hand-edit `catalog.json`. Tiers are judgement calls backed by each model's published scores.

## Design rules

- **Single responsibility.** One package, one reason to change. A file holds one concept. If a file passes ~300 lines, it is probably two concepts.
- **Interfaces belong to the consumer.** Define the smallest interface where it is used, not where it is implemented. Accept interfaces, return concrete types.
- **Inject every side effect.** Filesystem access goes through `fs.FS`, processes through `system.Runner`, HTTP through an `*http.Client` and base URL, time through a `func() time.Time`. No code below `internal/cli` reads `os.Getenv`, `os.UserHomeDir` or the wall clock directly.
- **No package-level mutable state** and no `init()` with side effects. The only exception is the linker-set version variable.
- **Read-only by construction.** Every scan stage receives `fs.FS`, which has no write methods. Writes happen only inside `migrate.Apply`, through the `migrate.Effects` it is given; planning only reads, and a front end always shows the plan before applying it. Every file Migrate replaces is backed up first, nothing is written into the user's own repositories, and every change is recorded in the journal as it happens so `lo0 migrate --undo` can reverse it. A step that changes a runtime records the change with `Effects.Record`, and its backend's `Revert` undoes it.
- **Harnesses and runtimes are plug-ins.** A target harness is one package under `internal/harness` that implements `match.Harness` (its feature mapping), `migrate.Harness` (its plan) and `migrate.Launcher` (one session, set up through its environment or flags), listed in `wiring.Harnesses`. A harness that can take over a Codex setup also implements `match.CodexReader`; for any other, match records the Codex setup as left behind. A runtime is one `migrate.Backend` in `internal/backend`, listed in `wiring.Migrator`. Adding either touches no other package, and no code outside them switches on a harness or runtime ID.
- **Degrade, don't fail.** A missing GPU tool or a malformed transcript line is a warning on the result, not a fatal error. Only context cancellation and programmer errors stop a scan.
- **Errors carry context.** Wrap with `fmt.Errorf("reading %s: %w", path, err)`. Compare with `errors.Is` and `errors.AsType`. Never compare error strings. Never panic outside `main`.
- **Context first.** Every function that blocks or does I/O takes `ctx context.Context` as its first parameter and honours cancellation.
- **Bounded concurrency.** Use `sync.WaitGroup.Go` or `errgroup` with a limit. Every goroutine has an owner that waits for it. The test suite runs with `-race`.

## Privacy

The usage profile never contains prompt text, model output, file contents or secret values. MCP server `env` and `headers` are reduced to their key names. Diffs are reduced to the first character of each line and created files to a line count, by decoders that never build the string. Paths to the user's own repositories may appear in local output but must be removable: `--redact` and the TUI's saved report pass everything through `internal/redact`. Tests assert this.

lo0 uses the network when the job needs it: looking up releases, fetching catalogue updates, installing harnesses and downloading models. What the user's machine and history hold stays on it. No request carries the usage profile, paths, prompt text, setup details or anything else about the user, and none goes to a lo0 server. Each request goes to a source the code names, such as a package registry, a harness's own release page or the Hugging Face repository the catalogue names. A download or install is part of a confirmed plan and says what it fetches and how large it is. Lookups that only add information are skipped with `--offline` or `LO0_OFFLINE` and fail silently without a connection; a command that cannot work offline says so. Migrate never shows MCP `env` or `headers` values in a plan: they move from Claude Code's file to the target's on the same machine.

## Claude Code versions

Claude Code changes its files between releases. `claudecode.VerifiedVersion` is the newest release whose layout and transcript format lo0 was checked against.

- When a newer release is installed or appears in the history, lo0 says so as a warning instead of guessing. It also warns when many responses lack token counts or when transcripts contain record types outside `transcript.KnownType`.
- To support a new release: run lo0 against its files, check the setup inventory and the JSON for missing or zero fields, add any new record types, then raise `VerifiedVersion` in the same commit and say what changed.
- Decoders read only the fields lo0 needs and tolerate missing ones, so an unknown shape degrades to a zero, never to an error.

## Modern Go

Use the language and library as they are now: range-over-func iterators (`iter.Seq`), `for range n`, the `min`/`max` builtins, `slices` and `maps`, `strings.Lines` and `strings.SplitSeq`, the `omitzero` JSON tag, `errors.AsType`, `sync.WaitGroup.Go`, `t.Context()`, `t.Chdir()` and `b.Loop()`. Do not hand-roll what the standard library already provides. Do not enable `GOEXPERIMENT` features in shipped code.

## Comments

- Every exported identifier has a doc comment that starts with its name and says what it does or guarantees, in one line where one will do and two at most.
- No explanatory comments. Do not walk through reasoning, restate the code, narrate how a figure was derived or describe what callers do with a value. A comment earns its place only when the code cannot be understood or changed safely without it; longer rationale belongs in `docs/design`. An unexported identifier whose name says what it is gets no comment.
- Inside function bodies, comment only to explain *why*: a non-obvious constraint, a format quirk, a workaround with a link. Never restate what the code does.
- No commented-out code, no `TODO` without an issue link, no banner or divider comments.

## Testing

- Every package has tests. New behaviour ships with tests in the same change.
- Prefer table-driven tests with `t.Run` and descriptive case names.
- Use fakes, not mocking frameworks. `fstest.MapFS` for files, `httptest.Server` for HTTP, a fake `system.Runner` for processes.
- Fixtures live in `testdata/`. Rendered output is checked with golden files; regenerate with `go test ./... -update` and review the diff.
- `internal/match/archetypes_test.go` matches the embedded catalogue against about twenty common machines, from an 8 GB M1 to a 128 GB M5 Max, NVIDIA cards from 8 to 32 GB, two cards, a 7900 XTX and CPU-only boxes, each with short and long sessions. It checks constraints any good answer meets, such as no tier D model, everything fitting the budget and no CPU machine called ready, and records the top three picks in `testdata/archetypes.golden`. A change to fit, speed, ranking or the catalogue that moves a pick shows up there: regenerate with `go test ./internal/match -update` and explain the diff in the pull request. The nightly catalogue refresh regenerates it too. When a constraint is known to fail, mark the archetype (as `slowTopPick` does) rather than loosening the check, so the fix has to remove the mark.
- Tests never touch the network, the real home directory or the real clock.
- Benchmarks use `b.Loop()`.
- `e2e` runs the real binary in its own process, with a home directory copied from `e2e/testdata/home`, a fake Ollama on loopback, fake harnesses on `PATH` and a proxy that refuses and records every other connection. A promise the README makes to users, such as read-only assessments, `--redact`, `--offline` or `migrate --undo` putting files back, gets a test there as well as in its package. When lo0 starts calling a new Ollama endpoint, the fake fails the test until it is taught it.
- Fixture strings lo0 must never print start with `SECRET_`. Add new private fields to `e2e/testdata/home` the same way.

## CI

- `ci.yml` runs on every push and pull request: lint, vulnerability and workflow checks, then the full test suite natively on all six platforms, then `make dist` and, on each platform again, the end-to-end tests and both installers (`site/test/install.sh`, `site/test/install.ps1`) against those exact release binaries.
- `release.yml` runs `ci.yml` at the tag, publishes the binaries it tested, then runs `quickstart.yml`: the README's install commands and `go install` against the live site and the new release.
- `nightly.yml` catches what changes without a commit. It regenerates the catalogue and opens a pull request when the models changed, opens an issue when Claude Code or Codex ships a release newer than its `VerifiedVersion`, installs the newest of each, runs govulncheck, migrates to and undoes a real model in a real Ollama with each harness on Linux and Windows, runs `ci.yml` and the live quick start, and opens or updates one issue when any of it fails.
- A workflow change passes actionlint. A new platform, runtime or harness adds its job to these workflows in the same change.

## UI

The output must read as made by a careful person, not generated.

- No emoji, ASCII-art logos, gradient banners or boxes around every section. Structure comes from spacing, alignment and one accent colour.
- Numbers are right-aligned in columns, use thousands separators and consistent units (`GiB`, `k` and `M` for tokens).
- Fit in 80 columns. Truncate, never wrap, inside tables.
- Spinners only for waits the user can feel. Anything instant just appears.
- Progress goes to stderr and the report to stdout, so `lo0 scan > report.txt` works.
- When stdout is not a terminal, print plain text. Honour `NO_COLOR`. `--json` prints the machine-readable result and nothing else.
- Copy is plain and specific. Say what was found and what it means. No exclamation marks, no "Oops", no filler.
- Never name internal roadmap stages or unbuilt features in user-facing text ("not yet verified by Prove"), and don't hedge with boilerplate. State what lo0 knows and where it came from.
- The TUI reuses the report's section and detail renderers, so a fact reads the same in `lo0 match` and in `lo0 ui`. Never fork a renderer for one front end.
- The TUI must work at 80×24, never block on terminal queries, and leave the terminal as it found it. Screens are checked with golden files at 120×36 and 80×24.
- Each TUI screen implements the `screen` interface and is a value: `update` returns the next state. Never keep maps or reused slices in screen state, because copies would share them.
- No background-coloured panels: terminals have their own backgrounds, often translucent. Separate panels with space, section labels and `─` rules; mark selection with the accent bar `▌`.

## Web front end

`lo0 web` serves the assessment to a browser. HTML cannot reuse the terminal renderers, so `internal/web` draws its own pages; the rules that keep front ends agreeing still hold.

- Pages only read. Every change is a command the page shows (`lo0 try`, `lo0 migrate`, `lo0 ui`), so plans are still reviewed where they are applied.
- Copy that names a fact, such as a stage label, a pace or a verdict level, comes from `internal/ui/report` or below, never a second wording in `web`.
- The server listens on the loopback address only, admits a request only with the per-run token and only when it names lo0's own address, and accepts forms only from lo0's own origin. Never relax any of these.
- Nothing is fetched from another host: fonts, styles and scripts are embedded, and the Content-Security-Policy says so.
- The UI rules above apply. Plus: pages work without JavaScript, apart from live scan progress and copy buttons; layouts hold from 375 px wide; light and dark follow the system; motion is one short entrance and honours `prefers-reduced-motion`.
- Pages are checked with golden files; regenerate with `go test ./internal/web -update` and review the diff.

## Website

`site/` is the lo0.ai website, an Astro project with its own toolchain. The Go rules above do not apply to it; these do.

- `npm run build` in `site/` must pass. It type-checks before it builds.
- Facts about lo0 live in `site/src/site.ts` and the example report in `site/src/data/example.ts`. Model data comes from `internal/catalog/catalog.json` at build time; never copy it into the site.
- The copy follows the UI rules above: plain, specific, no emoji or filler.
- Graphics work without JavaScript and honour `prefers-reduced-motion`. Add a script only when HTML and CSS cannot do the job.
- `/fit` runs `internal/browser` as WebAssembly, so its answers come from `match` like every other front end. `make wasm` builds it into `site/public/wasm/`, which is not committed; the site workflow builds it before the site.
- Merging to `main` deploys it. See `site/README.md`.

## Commits

Small, focused commits with an imperative subject under 72 characters. Never commit generated binaries.
