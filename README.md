<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/assets/logo-dark.svg">
    <img src="docs/assets/logo-light.svg" alt="lo0" width="200">
  </picture>
</p>

<p align="center">
  <b>Find the local model that fits your machine and your work.</b><br>
  lo0 reads your hardware and your Claude Code and Codex history, shows which open models can do your work, and switches your coding agent over when you're ready.
</p>

<p align="center">
  <a href="https://lo0.ai">lo0.ai</a> ·
  <a href="#install">Install</a> ·
  <a href="#how-it-works">How it works</a> ·
  <a href="#reading-the-report">Reading the report</a> ·
  <a href="#privacy">Privacy</a>
</p>

<p align="center">
  <img src="docs/assets/home.png" alt="lo0's home screen: 65% of the work can run on this machine today, with Qwen3.6-35B-A3B already downloaded, a typical reply estimated at 45 seconds against 8 with Opus 5.5, and the next step to try it in Claude Code for one session" width="760">
</p>

Benchmarks tell you how a model does on someone else's tasks. lo0 looks at yours: how much context your sessions actually reach, which tools and MCP servers they depend on, and how much model your hardware can hold. From that it works out which local models fit, what you would give up and how fast they would run, then sets one up in Ollama, LM Studio or llama.cpp and starts Claude Code or OpenCode on it.

The assessment only reads. Anything that changes your machine, such as downloading a model or writing a launcher, is shown as a plan first and runs only when you confirm it, and `lo0 migrate --undo` reverses what was kept.

## Install

```bash
curl -fsSL https://lo0.ai/install.sh | sh
```

The script downloads the binary for your macOS or Linux machine from the latest GitHub release, checks it against the release's `checksums.txt` and puts it in `~/.local/bin`. Set `LO0_INSTALL_DIR` to put it elsewhere or `LO0_VERSION=v0.1.0` to pin a release.

On Windows, in PowerShell:

```powershell
irm https://lo0.ai/install.ps1 | iex
```

It does the same checks, installs `lo0.exe` to `%LOCALAPPDATA%\Programs\lo0` and adds that folder to your user `PATH`. Both scripts are served from [`site/public`](site/public).

With Go 1.27 or newer you can also run `go install lo0.ai/lo0/cmd/lo0@latest`, or build a checkout with `make build`, which writes `bin/lo0`. [lo0.ai/install](https://lo0.ai/install) covers every way, and uninstalling.

Releases are cut by pushing a `v*` tag: `.github/workflows/release.yml` runs the full CI at that commit, publishes the binaries it tested with `checksums.txt`, then installs the release from lo0.ai on every platform.

## Usage

```bash
lo0
```

Run in a terminal, `lo0` opens an interactive workbench with five screens:

- **Home** opens on the answer: how much of your work runs on this machine today, how long a typical reply takes against the model you use now, where each of your commands sends its requests, and the best next step, one key away.
- **Models** shows every model with what its lab says it is good and bad at, how it fits your memory and your sessions, and how fast it runs, and compares any two to four side by side. `t` tries one for a session and `b` measures its real speed.
- **Harnesses** shows what of your setup carries over to each harness, and tries or keeps one.
- **Installed** lists the models on disk, what lo0 changed and how to undo it.
- **Your Claude** draws your sessions against local context windows, month by month and project by project, with what they are worth at API prices.

Press `e` to save the report to share, with your project paths replaced. Use `tab` or `1`–`5` to move between screens, `ctrl+k` to jump to anything and `?` for every key.

```
lo0 ui                       the interactive view, explicitly
lo0 web                      the same assessment in your browser, served to this machine only
lo0 match                    verdict, models that fit and what carries over to each harness
lo0 match --json             full assessment as JSON (schema lo0.match/v1)
lo0 try                      start the recommended harness on the best model for one session
lo0 try --harness opencode --model ID   try another harness or model; settings stay as they are
lo0 try -- --resume          pass everything after -- to the harness
lo0 benchmark                measure a downloaded model's real reading and writing speed on this machine
lo0 migrate                  show the plan to set up the recommended model and harness, then ask
lo0 migrate --to opencode    move to OpenCode instead of Claude Code on a local model
lo0 migrate --runtime lmstudio --model ID   pick the runtime or the model yourself
lo0 migrate --dry-run --json the plan as JSON (schema lo0.migrate/v1), changing nothing
lo0 migrate --undo           show how to reverse everything lo0 migrate changed, then ask
lo0 scan                     the inventory alone: hardware, runtimes, setup, history
lo0 scan --json              full scan as JSON (schema lo0.scan/v1)
lo0 <command> --claude-dir DIR   read Claude Code files from DIR instead of ~/.claude
lo0 <command> --redact       replace your project paths and home directory, to share the output
lo0 <command> --offline      skip lookups that only add information, such as the newest Claude Code release
lo0 version
```

Progress goes to stderr and the report to stdout, so `lo0 match > report.txt` does what you expect. Colour is dropped when stdout is not a terminal or `NO_COLOR` is set.

## How it works

| Step | Command | What it does |
|---|---|---|
| **Scan** | `lo0 scan` | Profiles hardware, local runtimes, Claude Code setup and Claude Code and Codex history |
| **Match** | `lo0 match` | Scores open-weight models against your memory, context needs and tool use, and says which sessions would not fit and why |
| **Try** | `lo0 try` | Prepares a model in a local runtime and starts Claude Code or OpenCode on it for one session, without touching their settings |
| **Keep** | `lo0 migrate` | Keeps the setup: a `claude-local` command beside `claude`, or OpenCode pointed at the model |
| **Undo** | `lo0 migrate --undo` | Reverses everything migrate changed, from the journal it kept as it went |

Only Try, Keep and Undo change anything, and only after you have seen exactly what they will change. Try changes only the runtime, such as downloading the model; the harness gets everything else in its environment for that session alone. Migrate backs up every file it replaces and records every change, so `lo0 migrate --undo` puts things back; files you have edited since are left alone.

[Run Claude Code on a local model](https://lo0.ai/guides/claude-code-local-model) walks through the environment Claude Code is given and what keeps working.

### What the scan reads

| Source | Where | What it records |
|---|---|---|
| Hardware | OS, `system_profiler`, `nvidia-smi`, `rocm-smi`, the Windows registry | CPU, memory and whether it is unified, GPUs and VRAM, free disk |
| Local runtimes | Ollama and LM Studio on localhost, model folders, the llama.cpp cache | Installed runtimes and models, with size, quantisation, context length and tool-calling support |
| Claude Code setup | `~/.claude`, `~/.claude.json`, each project's `.claude/` and `.mcp.json`, plugins, managed settings | Instruction files, MCP servers, hooks, commands, subagents, skills, plugins, permission rules |
| Session history | `~/.claude/projects/**/*.jsonl`, `~/.codex/sessions/**/*.jsonl` | Peak context per session, compactions, tool and MCP calls, languages, repositories, subagent use, tool calls and wait per request, reasoning effort, images, lines changed and pull requests, usage limits and retries, skill loads and hook runs, Claude Code versions |
| Claude Code | `claude --version`, the Claude app's Claude Code builds, the npm registry | The versions you run and the newest release; the registry lookup sends nothing about you |

A missing GPU tool or a malformed transcript line becomes a warning at the end of the report. It does not stop the scan.

## Reading the report

Most of the report is inventory. Three lines carry the verdict.

**Usable by models.** How much memory a model can occupy while staying on the GPU. On Apple Silicon that is the share of unified memory macOS lets the GPU wire: about two thirds, or three quarters above 36 GiB. On discrete GPUs it is total VRAM across cards. Weights and KV cache both have to fit inside it.

**Peak context.** The largest prompt the model saw in each session, read from token usage in the transcripts, as percentiles across all sessions.

**Fits within.** The share of your sessions that would have fitted in a 32k, 64k, 128k, 256k and 1M token window. Most local models top out at 128k or 256k, and long contexts cost memory on top of the weights. If only a third of your sessions fit in 128k, a 128k model will cover a third of your work without changes to how you work.

## Privacy

lo0 reads sensitive files, so it is strict about what it keeps.

- **Nothing about you leaves your machine.** lo0 goes online to look up releases and to download what a confirmed plan names, from the source the plan names. No request carries your usage profile, paths, prompts or setup, and none goes to a lo0 server. `--offline` skips every lookup that only adds information.
- **Read-only by construction.** Every scan stage receives an `fs.FS`, which has no write methods.
- **Counts and names, not content.** The profile never holds prompt text, model output, file contents, command arguments or secret values. MCP server `env` and `headers` are reduced to their key names.
- **Paths stay local.** Repository paths appear in your own report. `--redact` and the saved report replace them, and your home directory, before you share it.
- **`lo0 web` stays on this machine.** It listens on the loopback address only, admits a request only with the token it prints and loads nothing from another host.

Tests feed secrets, prompts and command arguments through the scanners and fail if any of them reach the output.

## Model catalogue

Match draws on a catalogue of 36 open-weight models built for agentic coding or with native tool calling, including Qwen3-Coder, gpt-oss, Devstral, Ministral, GLM, DeepSeek, Kimi, MiniMax, Gemma, Nemotron and Granite. Each entry records parameter counts, attention layout and quantised file sizes, which is enough to work out memory use at a given context length. The catalogue is generated from [`internal/catalog/sources.json`](internal/catalog/sources.json) with `go generate`.

Models are ranked A to D by how well they hold up in long tool-calling sessions, judged from each model's published agentic coding scores.

## Platforms

macOS, Linux and Windows on amd64 and arm64. GPU detection covers Apple Silicon, NVIDIA and AMD. CI tests every change natively on all six: the unit tests, end-to-end tests that drive the release binary against a fake home directory and a fake Ollama, and both install scripts. Every night it also installs a real model in a real Ollama, starts Claude Code and OpenCode on it, undoes the migration, and runs the quick start above against lo0.ai.

## Development

```bash
make build          # bin/lo0
make check          # vet, staticcheck, golangci-lint and race-enabled tests
make e2e            # the end-to-end tests alone, against a fresh build
make dist           # binaries for every platform and checksums.txt in dist/
make test-install   # run install.sh against dist/ instead of GitHub
```

Linters are pinned in `tools/go.mod` and run through `go tool`, so there is nothing to install globally.

Read [AGENTS.md](AGENTS.md) before opening a pull request. It sets the architecture, design rules and testing standards, and applies to people and agents alike.

## The name

`lo0` is the loopback interface on macOS and the BSDs, the one network device that only talks to its own machine. Anything sent to it goes nowhere and comes straight back. That is the plan for your prompts too.

The dot in the logo is a packet on its way home. Say it "lo-zero". "Loo" is accepted, though not encouraged.

## Licence

lo0 is free software under the [GNU Affero General Public License v3.0](LICENSE). Copyright 2026 Dave Earley.
