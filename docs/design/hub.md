# lo0 as a local AI hub: TUI redesign

Status: phases 1 to 5 are built (5 November 2026). The TUI has five screens, Home, Your sessions, Models, Agents and This machine (named Home, Models, Harnesses, Installed and Your Claude until 7 November 2026; see "Navigation, 7 November 2026" below); Readiness, Carry-over and Machine are retired. Speed is shown against what you use, and every change (try, keep, download, remove, install, undo) is a reviewed plan. In the TUI, keep is `m`, since `k` moves up. Benchmarking and Codex as a harness are still proposals. It replaces the framing of [tui.md](tui.md), which was a report about moving off Claude Code, and keeps that doc's navigation mechanics, visual language, Models detail and Your Claude charts.

The mockups use real numbers from this machine: a 64 GiB M5 Pro, Ollama 0.34.4 with four models, Claude Code 2.1.221, 477 Claude Code sessions and 46 Codex logs. The one exception is the screen shown after a session ends, whose numbers are marked as illustrative.

## What is wrong now

- Every screen answers one question: can your Claude Code work run locally? That was the right first question and is now the wrong headline. The answer is in, and what people want next is to act on it.
- Nothing can be done from the TUI. `lo0 try` and `lo0 migrate` exist only on the command line. The TUI recommends Qwen3.6-35B-A3B and offers no way to try it.
- What is on the machine is a scan finding, not something you manage. Nothing shows that `qwen3-coder:30b` takes 17.3 GiB and is beaten on every measure by a model you already have, or offers to remove it.
- Speed is one number with no reference. "33 tok/s" means nothing until it sits next to what you are used to, which in your sessions is Opus 5.5 at 96 tok/s.

## Principles

1. **Your setup first.** Home opens on what you work in, what is on the machine and the one best next step. The verdict moves to Your Claude.
2. **Every recommendation has a key.** If lo0 says a model fits, `t` tries it right there and `k` keeps it.
3. **One plan view.** Every change, from the TUI or the command line, is reviewed in the same view, rendered by `report.RenderPlan`. What you approve in the TUI reads exactly as `lo0 try --dry-run` prints it.
4. **Leave and come back.** A try hands the terminal to the harness and returns to lo0 when you quit, with what happened in that session.
5. **Cost before commitment.** Every change states disk, memory and download size before it asks.
6. **Speed is always relative.** Every local speed is shown next to the speed you already get from Claude or Codex, measured from your own history.

## Screen map

| Now | Becomes | Why |
|---|---|---|
| Home: can your work run locally? | **Home**: your local AI setup | The verdict is a finding; the setup and the next step are the point |
| Models | **Models**, with speed against your reference and `t k p x` | Already the strongest screen; it gains actions |
| Carry-over | **Harnesses** detail | What carries over is a property of a harness, next to trying and keeping it |
| Machine | **Installed** header, Models memory panel | Hardware matters only as limits on models; disk use matters more |
| — | **Installed**: what is on this machine | Models on disk, harnesses, what lo0 changed, with remove and undo |
| Readiness | top of **Your Claude** | "How much of your work can run locally" belongs with how you work |
| Your Claude | **Your Claude** | Unchanged apart from gaining the verdict |

Five screens, reached with `tab` or `1`–`5`, in this order: Home, Models, Harnesses, Installed, Your Claude. The plan view and the session report are overlays, not screens.

### Navigation, 7 November 2026

A first-run audit found the screens hard to find and their names unclear, so the order and names changed:

| Was | Now | Why |
|---|---|---|
| 5 Your work | **2 Your sessions** | It opens on the verdict that Home's figure summarises, so it comes second. "Sessions" is the word Home and Models already use for the history lo0 reads. |
| 2 Models | 3 Models | |
| 3 Harnesses | **4 Agents** | "Harness" is lo0's own word; Claude Code, Codex and OpenCode are known as coding agents. The command line keeps `--harness`. |
| 4 Installed | **5 This machine** | Home's column for it was already called This machine. |

The names live in `internal/ui/report`, so `lo0 web` uses the same ones; it has no Agents page.

- Below 100 columns the sidebar becomes a line of tabs under the top bar, not a breadcrumb, so every screen stays visible at 80×24.
- The sidebar, the tabs and Home's steps take mouse clicks. Mouse mode was already on for the scroll wheel, which stops the terminal selecting text, so clicks have to do something.
- `esc` closes what is open on a screen and, with nothing open, goes back to Home.
- Home's title says what lo0 is for, "Local models for your Claude Code and Codex work", and the verdict level sits beside the share.
- Try comes first among Home's steps even when the model is not downloaded, because a try plans the download itself; the step's note gives the size. The separate download step is gone.

## Speed against what you use

Speed gets its own design because it is where local models disappoint first, and an estimate on its own hides that.

**The reference is measured, not quoted.** For each harness lo0 reads, it measures the output speed of every reply in your history: output tokens divided by the time from the record before the reply to the reply's last record, so time spent running tools is excluded. The reference for a harness is the model with the most replies in the last 30 days. On this machine:

| Reference | Replies | Median | Middle half | Typical reply |
|---|---:|---:|---:|---|
| Opus 5.5 in Claude Code | 22,974 | 96 tok/s | 85–108 | 565 tokens in 6.1 s |
| GPT-5.5 in Codex | 1,023 | 45 tok/s | 40–48 | 405 tokens in 9.6 s |

Rules for the measurement: replies under 200 tokens and gaps over 10 minutes are left out, and a model needs at least 200 replies to be a reference. Some older Codex releases write their log in batches, which gives impossible figures such as 668 tok/s for gpt-5.2-codex from 45 replies; the 200-reply floor already excludes them, and lo0 shows no reference rather than a wrong one.

**What the user sees** is the local estimate as a multiple of the reference, and the wait it adds to a typical reply:

```
Qwen3.6-35B-A3B   33 tok/s   2.9× slower than Opus 5.5: a typical reply takes 17 s instead of 6 s
Devstral-Small-2  7 tok/s    very slow, 13× slower than Opus 5.5: a typical reply takes 76 s instead of 6 s
```

Seconds for a typical reply is the number people feel. "Typical reply" is the median reply length in that harness, 565 tokens here.

**Pace bands** are decided in `match`, never in a front end, so the CLI report and the TUI agree:

| Band | Rule | Shown as |
|---|---|---|
| Comparable | at least half the reference | the multiple, plain |
| Slower | a fifth to a half of the reference | the multiple, in amber |
| Very slow | under a fifth of the reference, or under 10 tok/s | "very slow" and the multiple, in red |

The 10 tok/s floor is the existing `minSpeed` in `match/verdict.go`. With no reference, because the user has too little history, the bands fall back to the existing absolute thresholds: under 10 tok/s very slow, under 25 slower.

On this machine every model that fits is in the amber band, and Devstral-Small-2-24B is red. That is the honest picture: a local model on this Mac writes at a third of Opus 5.5's speed at best. The Home step and the verdict say so in words, not only in colour.

**Reading the context comes first.** lo0's estimate is decode speed at half your median context, about 95k tokens. A local runtime must first read the context, which takes long on a Mac when nothing is cached. The speed panel says so in one line, and `b` (benchmark) measures both on this machine; measured figures then replace the estimates everywhere.

### Words, not emoji

Very slow models are marked with the words "very slow" in red, not a snail emoji. Decided on 5 November 2026:

- An emoji is two columns wide in most terminals and one in some. That breaks the right-aligned columns AGENTS.md requires, and the breakage differs per terminal, so golden files cannot catch it.
- The word carries the meaning on its own and survives `NO_COLOR`; the colour only adds emphasis.
- The AGENTS.md rule against emoji stands.

## Home: "Your local AI setup"

```
 lo0                                                  Apple M5 Pro · 48 GiB for models · 197 GiB free · Ollama 0.34.4

 ▌ Home         Your local AI setup
   Models
   Harnesses    WORKING IN                       THIS MACHINE                     YOUR CLAUDE
   Installed    Claude Code 2.1.221 on Claude    3 models · 41.9 GiB              477 sessions in 5 months
   Your Claude  no local setup kept              Ollama running                   $14,480 at API prices
                last try: Qwen3.6, today         197 GiB free for downloads       6 in 10 could run locally

                BEST NEXT STEP
                Qwen3.6-35B-A3B is already downloaded and fits 65% of your sessions. At 33 tok/s it is 2.9×
                slower than Opus 5.5 in your sessions: a typical reply takes 17 s instead of 6 s.

              › Try it in Claude Code for one session              nothing in Claude Code changes            t
                Keep it: run claude-local with it from now on      one new file, undo any time               k
                Compare it with Qwen3-Coder-Next                   39 tok/s, needs 43.8 GiB                  c
                Free 17.3 GiB: remove qwen3-coder:30b              Qwen3.6 fits the same sessions faster     x

 ↑↓ choose   enter run   ctrl+k commands   ? keys
```

- The three columns answer where you work now, what you have and what your work is worth.
- "Best next step", now labelled "What to do next", is chosen in `app`, from the assessment and what is installed, so it reads the same in a future desktop app. Its order: try the best fit, downloading it first if needed; keep it once tried; then housekeeping such as a model that another installed model beats on fit and speed.
- The speed sentence is part of the recommendation, not small print.

At 80 columns the sidebar becomes a breadcrumb and the columns become rows:

```
 lo0 › Home                               M5 Pro · 48 GiB for models · Ollama

 Your local AI setup

 WORKING IN     Claude Code 2.1.221 on Claude · last try today
 THIS MACHINE   3 models · 41.9 GiB · 197 GiB free
 YOUR CLAUDE    477 sessions · $14,480 at API prices · 6 in 10 could run here

 BEST NEXT STEP
 Qwen3.6-35B-A3B is downloaded and fits 65% of your sessions. At 33 tok/s it
 is 2.9× slower than Opus 5.5: a typical reply takes 17 s instead of 6 s.

 › Try it in Claude Code for one session                                   t
   Keep it: run claude-local with it from now on                           k
   Compare it with Qwen3-Coder-Next                                        c
   Free 17.3 GiB: remove qwen3-coder:30b                                   x

 ↑↓ choose  enter run  ctrl+k commands  ? keys
```

## Models: "What can you run, and how fast?"

```
 lo0  › Models                                       48 GiB for models · 307 GB/s · against Opus 5.5 at 96 tok/s

   ● Fits 20    ○ Installed 2    ○ All 28                                     sort  best for you ▾      / filter

   MODEL                     YOUR SESSIONS         SPEED                       TIER  MEMORY
 › Qwen3.6-35B-A3B       ✓   █████████▊░░░░░  65%  33 tok/s  2.9× slower       A    ███████████▍░░░░░░░  28.9 GiB
   Qwen3-Coder-30B-A3B   ✓   █████████▊░░░░░  65%  24 tok/s  4.0× slower       A    ████████████▌░░░░░░  31.5 GiB
   Qwen3-Coder-Next          █████████▊░░░░░  65%  39 tok/s  2.5× slower       A    █████████████████▎░  43.8 GiB
   GLM-4.7-Flash             ████████░░░░░░░  54%  26 tok/s  3.7× slower       A    ███████████░░░░░░░░  27.6 GiB
   Devstral-Small-2-24B      ████████████░░░  81%   7 tok/s  very slow, 13×    A    ██████████████████▌  46.9 GiB
   Gemma-4-26B-A4B           █████████▊░░░░░  65%  29 tok/s  3.3× slower       B    ██████████▍░░░░░░░░  26.4 GiB

  ─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
   Qwen3.6-35B-A3B                                    in Ollama as qwen3.6:latest and lo0/qwen3.6-35b-a3b:256k

   SPEED AGAINST WHAT YOU USE
   Opus 5.5 in Claude Code   ████████████████████████████████████████  96 tok/s   typical reply   6 s
   GPT-5.5 in Codex          ██████████████████▊                       45 tok/s   typical reply  10 s
   Qwen3.6-35B-A3B here      █████████████▋                            33 tok/s   typical reply  17 s   estimate
   From 22,974 Opus 5.5 and 1,023 GPT-5.5 replies in your history. Reading a long context first adds to the wait
   on this machine; b measures it.

 ↑↓ model   t try   k keep   p pull   x remove   c compare   b benchmark   enter details
```

- The speed column always carries the multiple; amber and red mark the bands. The header names the reference once.
- The detail panel draws the local model against both references on one scale, so the gap is visible before it is read.
- The existing "good at", "watch out for" and memory panels stay below, reached by scrolling or `enter`.
- `x` appears only on installed models, `p` only on models that are not.

At 80 columns:

```
 lo0 › Models                            48 GiB · against Opus 5.5 at 96 tok/s

 ● Fits 20  ○ Installed 2  ○ All 28                                 / filter

   MODEL                    SESSIONS   SPEED                    MEMORY
 › Qwen3.6-35B-A3B      ✓       65%   33 tok/s  2.9× slower    28.9 GiB
   Qwen3-Coder-30B-A3B  ✓       65%   24 tok/s  4.0× slower    31.5 GiB
   Qwen3-Coder-Next             65%   39 tok/s  2.5× slower    43.8 GiB
   GLM-4.7-Flash                54%   26 tok/s  3.7× slower    27.6 GiB
   Devstral-Small-2-24B         81%    7 tok/s  very slow      46.9 GiB
   Gemma-4-26B-A4B              65%   29 tok/s  3.3× slower    26.4 GiB
 ────────────────────────────────────────────────────────────────────────────
 Qwen3.6-35B-A3B · in Ollama
 Opus 5.5 in Claude Code   ████████████████████████████   96 tok/s    6 s
 Qwen3.6-35B-A3B here      █████████▋                     33 tok/s   17 s
 Typical reply: 565 tokens, the median in your Claude Code sessions.

 ↑↓ model  t try  k keep  p pull  c compare  ? keys
```

## Harnesses: "Where do you work with it?"

```
 lo0  › Harnesses                                                                        where you work with a model

   HARNESS        INSTALLED                      USES NOW      YOUR SETUP CARRIES OVER
 › Claude Code    2.1.221 · 2.1.318 is out       Claude        █████████████████▏░░░  86%
   OpenCode       not installed                  –             ██████████████▍░░░░░░  72%

  ─────────────────────────────────────────────────────────────────────────────────────────────────────────────────
   Claude Code on a local model                                    claude stays on Claude; claude-local runs locally

   ✓ 112 work as is    ↻ 1 converted    ◐ 2 partly    ✕ 11 lost

   ✓ Skills 33   Instructions 34   Hooks 28   MCP servers 6   Permissions 6   Plugins 3
   ◐ Browser tools    2   Claude_Browser, claude-in-chrome · 7,704 calls · a browser MCP server can take over
   ✕ App tools       11   Claude app only · 467 calls
   ↻ Settings         1   "model" is set by claude-local

 ↑↓ harness   t try   k keep   i install   enter details
```

- This absorbs Carry-over. What is lost is listed with how much you used it, so 467 calls lost reads differently from 7,704.
- `i` installs a harness that is missing. The plan view shows the harness's own documented install command verbatim, and lo0 runs it only after you confirm.
- A harness lo0 finds but cannot set up does not get a row. Codex is installed here and lo0 already reads its logs, so it is the natural next harness package; it gets a row when that package exists.

## Installed: "What is on this machine?"

```
 lo0  › Installed                                          Ollama 0.34.4 running · 41.9 GiB of models · 197 GiB free

   OLLAMA                                     ON DISK      IS                          USED BY
 › qwen3.6:latest                            22.3 GiB      Qwen3.6-35B-A3B · Q4_K_M    –
     lo0/qwen3.6-35b-a3b:256k               shares it      a 256k window, made by lo0  lo0 try, today
   qwen3-coder:30b                           17.3 GiB      Qwen3-Coder-30B-A3B         –
   hf.co/PantheonUnbound/Satyr-V0.1-4B        2.3 GiB      not in the catalogue        –

   LM STUDIO   not installed          LLAMA.CPP   not installed

   CHANGES LO0 MADE
   Created lo0/qwen3.6-35b-a3b:256k in Ollama, today                                              u  undo all

 ↑↓ model   x remove   p pull another   enter details
```

- Disk use counts shared weights once. lo0's variants are a new context window on a model you have, not a second copy, and the list says so; Ollama's own list shows 22.3 GiB twice.
- "Used by" comes from lo0's change journal, the claude-local launcher and OpenCode's configuration, so removing a model a setup depends on warns first.
- `x` on a model lo0 created only removes the variant. On a download it frees the disk, and the plan states how much.

## The try loop

1. `t` on a model, a harness or the Home step builds the plan with `app.PlanTry`. With more than one harness, a one-line picker asks which, defaulting to the recommended one.
2. The plan overlay is `report.RenderPlan`, the same text `lo0 try` prints, scrollable, with `enter` to run and `esc` to go back. A plan that changes nothing, because the model is ready, skips straight to step 4.
3. Steps run with progress in the overlay, through `app.Migrate` and its observer, as the CLI does.
4. lo0 prints "Starting claude with Qwen3.6-35B-A3B through Ollama. Quit it to return to lo0." and hands over the terminal with Bubble Tea's `ExecProcess`, using `Session.Environ` for the environment. The TUI is restored as it was when the harness exits.
5. The session report opens. Numbers below are illustrative:

```
 lo0  › Try                                                              Claude Code · Qwen3.6-35B-A3B · Ollama

   SESSION ENDED   14 min · 23 requests · exited normally

   Peak context    61k of 256k        ███▊░░░░░░░░░░░░░░░░
   Speed           29 tok/s measured  estimate was 33 · 3.3× slower than Opus 5.5
   Tool calls      118 · 6 failed     5%, against 2% in your Opus 5.5 sessions
   Compactions     0

 › Keep this setup: claude-local with Qwen3.6-35B-A3B                                                         k
   Try another model                                                                                          t
   Back to Models                                                                                           esc
```

- lo0 finds the session in Claude Code's transcripts by its model name, which is lo0's own, and its start time. Measured speed then replaces the estimate for that model on this machine.
- For a harness whose logs lo0 does not read, such as OpenCode, the report shows duration and exit only, and says nothing it does not know.
- `lo0 try` on the command line prints the same report after the session, from the same renderer.

## Effects on the architecture

- **`usage`** gains per-reply speed: `Profile.Speeds`, one entry per harness and model with reply count, median, quartiles and the median reply length and time. Each harness's history package measures its own replies; `usage` knows no log format, as now.
- **`match`** gains `Candidate.Pace`: the reference, the multiple, the band and the typical-reply seconds for both. The bands live here and nowhere else.
- **`app`** gains `NextSteps(Assessment)` for Home and an `Installed` view combining runtimes, the journal and harness configuration. Removing a model is a `migrate.Plan` with one runtime step, so it is reviewed and journalled like everything else.
- **`ui/report`** gains the speed panel and the session report, used by both `lo0 match`/`lo0 try` and the TUI, so no renderer is forked.
- **TUI** screens stay values. The plan overlay holds a `migrate.Plan`, which holds slices; the overlay never mutates it and builds a new state per update, as the rule on shared slices requires.
- **AGENTS.md**: the architecture table gains the Installed and Harnesses screens.

## Phases

1. **Speed against what you use.** Per-reply speed in `claudecode/history` and `codex/history`, pace in `match`, the speed column and panel in Models and the CLI match report. Standalone and useful at once.
2. **Try from the TUI.** `t` on Models, the plan overlay, `ExecProcess`, the session report, also printed by `lo0 try`.
3. **Installed.** The screen, shared-weight disk use, `x` remove and `u` undo, `p` pull.
4. **Harnesses.** The screen with carry-over folded in, `i` install, `k` keep.
5. **New Home.** Next steps from `app`; Readiness moves to Your Claude; Readiness, Carry-over and Machine retire.
6. **Speed from evidence.** See [speed.md](speed.md): every speed comes from the best evidence lo0 has (measured here, calibrated on this machine, published results for this chip, or bandwidth alone) and says which; reading time joins writing time; `b` benchmarks a downloaded model.
7. **Codex as a harness.**

Each phase ships working, with golden screens at 120×36 and 80×24.

## Open questions

- Whether the reference should be the model you use most or the fastest one you use. Most used is proposed: it is what you are used to waiting for.
- Whether `k` from a try should ask again. Proposed: yes, it shows the migrate plan, because it writes a file where a try writes none.
