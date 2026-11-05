---
layout: ../../layouts/Prose.astro
title: Run Claude Code on a local model
seoTitle: Run Claude Code on a local model with Ollama · lo0
eyebrow: Guide
description: How to point Claude Code at a model running on your own machine with Ollama, which model to pick for your hardware, what keeps working and what to expect from speed.
---

Claude Code does not have to talk to Anthropic. It sends its requests to whatever address `ANTHROPIC_BASE_URL` names, and [Ollama](https://ollama.com) serves an Anthropic-compatible API on your own machine. Point one at the other and Claude Code runs on an open-weight model, with your instructions, MCP servers, hooks and skills as they are.

The wiring is the easy part. The hard part is choosing a model your hardware can hold, with enough context for the way you work, and knowing what you give up. lo0 works that out.

## 1. See what your machine can run

```bash
curl -fsSL https://lo0.ai/install.sh | sh
lo0
```

lo0 reads your hardware and your Claude Code history and opens on the answer: how much of your work runs on this machine today, the model that fits best, and how long a typical reply would take compared with the model you use now.

Three numbers decide it:

- **Memory the GPU can use.** On Apple Silicon, macOS lets the GPU wire about two thirds of unified memory, or three quarters above 36 GiB. On a PC it is your VRAM. The model's weights and its KV cache both have to fit.
- **The context your sessions reach.** lo0 reads each session's largest prompt from your transcripts. A session that grew past a model's window would have had to compact.
- **Speed.** Writing speed is set mostly by memory bandwidth. lo0 estimates it from llama.cpp's published results for your chip, and `lo0 benchmark` measures a downloaded model on your machine.

## 2. Try a model for one session

```bash
lo0 try
```

This shows a plan before it does anything: the model and quantisation, every download with its size and source, and the environment Claude Code will get. Once you confirm, lo0 pulls the model into Ollama, creates a copy with the context window your sessions need, checks that it answers, and starts Claude Code on it.

The settings live in that session's environment, so your Claude Code configuration does not change. For a model called `lo0/qwen3.6-35b-a3b:256k` the environment looks like this:

```bash
ANTHROPIC_BASE_URL=http://127.0.0.1:11434
ANTHROPIC_AUTH_TOKEN=ollama
ANTHROPIC_MODEL=lo0/qwen3.6-35b-a3b:256k
ANTHROPIC_DEFAULT_OPUS_MODEL=lo0/qwen3.6-35b-a3b:256k
ANTHROPIC_DEFAULT_SONNET_MODEL=lo0/qwen3.6-35b-a3b:256k
ANTHROPIC_DEFAULT_HAIKU_MODEL=lo0/qwen3.5-2b:256k
CLAUDE_CODE_MAX_CONTEXT_TOKENS=262144
CLAUDE_CODE_ATTRIBUTION_HEADER=0
CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1
```

Every model alias points at a local model, so nothing goes to Anthropic. The haiku alias, which Claude Code uses for titles, summaries and quick subagents such as Explore, goes to a small background model that answers faster. `CLAUDE_CODE_MAX_CONTEXT_TOKENS` tells Claude Code the real window instead of the 200k it assumes for names it does not know, and turning off the attribution header lets the runtime reuse its cache of your prompt between requests. `ANTHROPIC_API_KEY` is unset for the session so it is not sent with the placeholder token.

Pass anything after `--` to Claude Code, such as `lo0 try -- --resume`. To try another model, use `lo0 try --model ID`, or press `t` on it in the Models screen.

## 3. Keep it

```bash
lo0 migrate
```

After the same kind of review, lo0 writes one file: a `claude-local` command in `~/.local/bin` that sets that environment and starts Claude Code. Run `claude-local` in any project to work on the local model; `claude` still uses Claude, so you can switch per task.

`lo0 migrate --undo` shows what it will reverse, then removes it.

## What keeps working

| Part of your setup | On a local model |
| --- | --- |
| `CLAUDE.md` instructions | As they are |
| MCP servers | As they are |
| Hooks, commands, subagents, skills, plugins | As they are |
| Permission rules and settings | As they are |
| Tools only the Claude apps provide, such as their browser | Not available |

lo0 lists each of yours and counts how often your history relied on the ones that stop working. A browser MCP server can stand in for the apps' browser tools.

## What to expect

Local models are slower. On an Apple M5 Pro with 64 GiB, Qwen3.6-35B-A3B writes about 33 tokens a second, so a typical reply that takes 8 seconds with Opus 5.5 takes about 45. Long tool-calling chains are where local models most often lose track, and sessions longer than the window have to compact.

What you get in return: no usage limits, no per-token cost and code that never leaves your machine. You can keep Claude for the long, hard sessions and use `claude-local` for the rest. lo0 shows which of your projects and sessions would have fitted, so you can decide where the line goes.

## Prefer OpenCode?

```bash
lo0 try --harness opencode
```

lo0 maps your Claude Code setup to OpenCode and says which parts work as they are, which it converts and which have no equivalent, then runs OpenCode on the same local model.
