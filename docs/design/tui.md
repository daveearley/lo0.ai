# lo0 TUI redesign

The framing here, a report about moving off Claude Code, is replaced by [hub.md](hub.md), which turns the TUI into a place to try, keep and manage local setups. Navigation, visual language, Models and Your Claude below still apply.

Status: phases 1 to 3 are built (shell, Home, Models with detail and compare, Your Claude with its charts), and export from phase 6: `e` saves the match report as plain text with project paths replaced, after asking, and never over an existing file. Readiness, Carry-over and Machine use interim screens built from the report sections until phases 4 and 5. Filtering Your Claude by repository is not built.

Changes from the proposal while building:

- Screens are reached with `tab`, `shift+tab` or `1`–`6`, not first letters, so letters stay free for each screen's own actions. Scanning again is `ctrl+r`.
- No tinted panels. Terminal backgrounds vary and are often translucent, so a fixed panel colour looks like a grey slab. Panels are separated by space, labels and rules.
- The lab's claims ("good at", "watch out for", scores) are shown apart from lo0's own findings for this machine, so it is always clear who is saying what.

Original proposal follows.

Status at proposal: not built. Mockups use real numbers from a 64 GB M5 Pro with 430 Claude Code sessions wherever lo0 already computes them. Two things are illustrative: model "good at" and "watch out" lines, until the model-card research lands, and the "over time" and histogram shapes on Your Claude, which need per-session data lo0 does not collect yet.

## What is wrong now

- It opens on a wall of text. Nothing says what lo0 is, what it found or what to do next.
- Tabs look like a report split into pages. Five equal tabs say "all of this matters equally", which is false: the verdict and the models matter most.
- Models are a spreadsheet. Nothing says what a model is good at, where it falls down, or why its numbers are what they are.
- Everything is text. A TUI can draw: memory as a bar, sessions as a histogram, the context window as a line across it.
- It only reports. A multi-tool lets you act: compare, benchmark, preview a setup, pull a model.

## Principles

1. **Answer first.** The first screen answers "can I switch, and to what?" in five seconds. Detail is one keypress away, never in the way.
2. **Show, don't tabulate.** Every number that has a shape gets drawn: bars for memory and coverage, histograms for context, sparklines for trends.
3. **Honest by design.** Every model says what it is bad at as prominently as what it is good at. Estimates are labelled; measured numbers replace them when available.
4. **One screen, one job.** Each screen has a title that is a question it answers.
5. **Always know where you are and what you can do.** A persistent sidebar, contextual key hints, and a command palette for everything.
6. **Calm.** One accent colour, generous spacing, no borders around everything. Panels are set apart by a faint background, not boxes.

## Navigation

A sidebar replaces tabs. It is always visible on wide terminals and collapses to a one-line breadcrumb below 100 columns.

```
  lo0                     Home
                          Readiness       can I switch?
  ▌ Home                  Models          what can I run?
    Readiness             Your Claude     how do I work?
    Models                Carry-over      what moves with me?
    Your Claude           Machine         what does this Mac have?
    Carry-over
    Machine
```

- `↑↓` or `j k` moves within a screen; `⇥` / `⇧⇥` or the first letter of a screen (`h r m y c a`) moves between screens.
- `ctrl+k` opens a command palette: jump to any screen, model or feature, or run an action ("benchmark qwen3-coder:30b", "compare", "export report").
- `?` shows keys for the current screen only, as an overlay.
- The footer always shows the three or four keys that matter on this screen, never a full list.

## Home: "Can your Claude Code work run locally?"

```
 lo0                                                         Apple M5 Pro · 48 GB for models · Ollama running

 ▌ Home        Can your Claude Code work run locally?
   Readiness
   Models      ███████████████████████████████████████████▎                       66
   Your Claude Ready with trade-offs
   Carry-over
   Machine     A local model can take about 6 in 10 of your sessions today. The rest need
               more context than this Mac can hold, so they would have to compact.

               BEST FIT FOR YOU             YOUR SESSIONS                  WHAT CHANGES
               Qwen3-Coder-30B-A3B   ✓      peak context                   ↻ 7,682 browser calls
               61% of sessions fit          ▁▂▄▇█▆▄▃▂▁▁ ┊                    move to chrome-devtools
               ~23 tok/s · tier A           median 190k · 256k ┊           ✕ 18 of 28 hooks
               already installed            1 in 10 pass 560k              ✓ 19 files and skills as is

               NEXT
             › Compare the three best models                                                      m
               See which sessions would not fit, and why                                          y
               Measure qwen3-coder:30b's real speed on this Mac                                   b
               Preview your OpenCode setup                                                        c

 ↑↓ choose   enter open   ctrl+k commands   ? keys
```

- The readiness bar fills on first load (one short animation, then still).
- The three cards are the three questions people ask: what should I use, how do I work, what do I lose.
- "Next" turns findings into actions, each with its key.

## Models: "What can you run, and how well?"

```
 lo0  › Models                                                                    48 GB for models · 307 GB/s

   ● Fits 6    ○ Installed 1    ○ All 28                              sort  best for you ▾      / filter

   MODEL                      YOUR SESSIONS         SPEED        TIER  MEMORY
 › Qwen3-Coder-30B-A3B   ✓    ██████████▎░░░░  61%   23 tok/s     A    ████████████░░░░░░  32 GB
   Qwen3-Coder-Next           ██████████▎░░░░  61%   38 tok/s     A    ██████████████████  45 GB
   GLM-4.7-Flash              ████████▊░░░░░░  52%   26 tok/s     A    ███████████░░░░░░░  30 GB
   Devstral-Small-2-24B       █████████████▍░  79%    7 tok/s     A    █████████████░░░░░  35 GB
   Gemma-4-26B-A4B            ██████████▎░░░░  61%   29 tok/s     B    ███████████░░░░░░░  29 GB

  ─────────────────────────────────────────────────────────────────────────────────────────────────────────────
   Qwen3-Coder-30B-A3B                                   Qwen · Apache 2.0 · July 2025 · installed in Ollama
   Coding model tuned for agentic use in coding harnesses.

   GOOD AT                                       WATCH OUT FOR
   + Tool calling in multi-step coding loops     – Every layer caches the full context: slows to
   + Only 3.4B of 30B weights active per token     ~12 tok/s past 200k
   + 256k context                                – Nothing above 256k: 39% of your sessions exceed it

   ON THIS MAC
   Memory     ██████████████████████████████████████████████░░░░░░░░░░  31.5 of 48 GB
              weights 17.2   KV cache 12.8   runtime 1.5
   Sessions   ▁▂▄▇█▆▄▃▂▁▁▁▁   fits up to 256k ━━━━━━━━━━━━━━━━━┫ 61%
   Speed      32k  41 tok/s    128k  28 tok/s    256k  15 tok/s           estimated · b to measure

 ↑↓ model   enter full details   c compare   b benchmark   p pull
```

- Filter chips at the top; the default is "fits", because a model that cannot run is noise.
- Every row draws coverage and memory as bars, so the table can be scanned in a second.
- The lower panel follows the cursor. "Good at" and "Watch out for" come from the model's card and lo0's own analysis, side by side and equally prominent.
- "Speed vs context" makes the long-context slowdown visible instead of hiding it in one number.
- `c` marks models for comparison; with two or three marked, a side-by-side view opens.

### Compare

```
                       Qwen3-Coder-30B-A3B      Qwen3-Coder-Next        Devstral-Small-2-24B
   Sessions that fit   61% ██████████▎          61% ██████████▎          79% █████████████▍
   Speed at 95k        23 tok/s                 38 tok/s  fastest        7 tok/s   slowest
   Memory              32 GB                    45 GB     tight          35 GB
   Quality kept        96%  Q4_K_M              88%  Q3_K_M  lowest      97%  Q4_K_M
   Tier                A                        A                        A
   Installed           ✓                        –                        –
   Best for            everyday sessions        long sessions, speed     the longest sessions
```

## Your Claude: "How do you actually work?"

The Claude analysis is lo0's unique data, so it gets its own screen and gets richer.

```
 lo0  › Your Claude                                                     430 sessions · 5 Jun – 4 Nov 2026

   CONTEXT PER SESSION                                          local model windows
   sessions                                                     ┊128k     ┊256k            ┊1M
   ▕██▍                                                          ┊         ┊
   ▕████████▊                                                    ┊         ┊
   ▕███████████████▏                                             ┊         ┊
   ▕██████████████████████▋        median 190k                   ┊         ┊
   ▕████████████████▌                                            ┊         ┊
     0        128k      256k      384k      512k     640k     768k     896k

   OVER TIME     Jun ▂▃  Jul ▃▅  Aug ▅▆  Sep ▆▇  Oct ▇█  Nov █   (illustrative: median peak by month)

   WHERE         hi.events        ████████████████████  336   p90 560k
                 hi.events-web    ████▏                  61   p90 539k
                 hi.events-app    █▍                     20   p90 583k

   TOOLS         Bash ████████████████████ 35k   Edit ██▏ 3.8k   Read █▊ 3.0k   Write █▏ 2.0k
   MCP           Claude_Browser ███ 4.7k   claude-in-chrome ██ 2.9k   ccd_turn █ 1.6k
   LANGUAGES     TypeScript 43%   PHP 26%   Markdown 16%   CSS 8%
   HABITS        31% of sessions use subagents · 3% hit compaction · 52% on Opus 5.5
```

- A histogram with the local context windows drawn across it says more than any table.
- "Over time" needs per-session dates and peaks; the usage profile gains a slim per-session record (date, peak context, repository; never content).
- Selecting a repository filters every chart to it.

## Carry-over: "What moves with you?"

```
 lo0  › Carry-over                                                         to OpenCode · preview only

   ✓ 19 work as is   ↻ 70 converted   ◐ 7 partly   ✕ 37 lost

   ↻ Skills         33   9 read as is, 24 copied out of plugins
   ↻ Instructions   34   10 read as is, 24 added to "instructions"
   ↻ MCP servers     6   become "mcp" entries
 › ◐ Browser tools   2   chrome-devtools can take over · 7,682 calls
   ✕ Hooks          18   no OpenCode event · 10 more become plugin hooks
   ✕ App tools      18   Claude app only · 2,418 calls

   ─────────────────────────────────────────────────────────────────────────────────
   opencode.json (preview)
     "mcp": {
       "chrome-devtools": { "type": "local", "command": ["npx", "chrome-devtools-mcp@latest"] }
     }
```

- Grouped by outcome, sorted by impact, so what is lost is never buried.
- The preview pane shows the OpenCode config each item would become. This is the first visible piece of Migrate, and it is read-only.
- The screen opens on the recommended harness, the one most of the setup carries over to; `t` steps through the others, and the title names the one on show.

## Machine: "What does this Mac have?"

Hardware, memory as a gauge (total, usable by models, free now), each runtime with its models and what is loaded right now, and disk space for pulls.

## Actions

| Key | Action | Writes? |
|---|---|---|
| `b` | Benchmark an installed model on this machine: real prompt and decode speed at 8k, 32k and your median context. Measured numbers replace estimates everywhere. | No |
| `p` | Pull a model through Ollama, showing size and free disk first. | Yes, after confirming |
| `c` | Compare two or three models | No |
| `e` | Export the report as Markdown or JSON | Writes one file you name |

Benchmarking is the most valuable: it turns lo0's speed estimates into facts for this machine, and is the first step of Prove.

## Visual language

- **Panels, not boxes.** A faint background tint marks a panel. Rules (`─`) separate the list from the detail. This updates the AGENTS.md rule against boxes: no borders around sections, still.
- **Data glyphs.** Bars use eighth blocks (`▏▎▍▌▋▊▉█`) for smooth lengths, sparklines `▁▂▃▄▅▆▇█`, a dotted `┊` for thresholds.
- **Status glyphs.** `✓` as is, `↻` converted, `◐` partly, `✕` lost, `✓` installed. No emoji.
- **Colour.** Accent for selection and the one thing that matters; green, amber and red only for good, caution and bad.
- **Motion.** One short fill animation per screen on first view, spinners only for real waits.
- **Width.** Designed at 120 columns, fully usable at 80×24: the sidebar becomes a breadcrumb and panels stack.

## Phases

1. **Shell and design system.** Sidebar, breadcrumb, command palette, contextual help, panel and chart components (bar, histogram, sparkline, gauge, chips), Home screen.
2. **Models.** Catalogue gains "summary", "good at", "watch out" and benchmarks from model cards; Models screen with filters, sort, detail and compare.
3. **Your Claude.** Per-session records; histogram, timeline, repositories, tools.
4. **Carry-over** with OpenCode config preview.
5. **Machine** with live runtime state.
6. **Actions**: benchmark first, then pull and export.

Each phase ships working, with golden screens at 120×36 and 80×24.
