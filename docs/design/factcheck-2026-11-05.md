# Fact check, 5 November 2026

## Status after fixes (5 Nov 2026)

Fixed: catalogue items 1 to 6 (card figures shown for active parameters, context and release overrides, pinned benchmarks, profile lines reworded or removed, the missing Ollama tags added); prices and plan names (7, 8); the bandwidth and published-speed tables and device matching (9 to 11); the llama.cpp hub cache and the Ollama comment (12, 13; 14 needed no change, since the code already says lo0 sets the port); the fable alias, subagent comment, hook types and OpenCode version flag (15 to 18); the README count (24); OpenCode 2 support (19, see below). Site items 20 to 23 were already gone in the site rewrite that was in progress.

Open: moving DeepSeek-V4-Flash to the `-0731` release; raising the Codex verified version; the tier judgement calls.

Scope: every fact in this repository about models, prices, hardware, runtimes and harnesses. That covers the 36 catalogue entries (`internal/catalog/sources.json` and the generated `catalog.json`), `internal/cost/prices.go`, the bandwidth and llama.cpp tables in `internal/match`, the runtime and harness packages, and the site and README copy. Each claim was checked against its primary source on 5 Nov 2026: the Hugging Face model card and config, ollama.com/library, platform.claude.com, claude.com/pricing, the llama.cpp discussions and source, and the Claude Code and OpenCode docs. Per-field tables for every model follow the summary.

## Headline

- **Every benchmark score in `sources.json` matches its source**: 36 models, about 130 numbers, each with the right variant and setting.
- **Every API price row matches** the pricing page. Every listed Ollama tag and every GGUF repo exists and points at the right model.
- The errors are elsewhere: generated parameter counts, a few dates, missing tags and hardware rows, two plan names, and several claims about lo0 itself that are no longer true.

## Wrong

### Catalogue (generated fields)

1. **`activeParams` is higher than the lab's figure for almost every MoE model.** `Architecture.ActiveParams` deliberately counts embedding tables, and on some models MTP layers, as active (`derive.go:34`). That is a reasonable upper bound for speed estimates, but the site and TUI present the number as "active", where it contradicts the card:

   | Model | Catalogue | Card |
   |---|---|---|
   | qwen3-coder-next | 3.87B | 3B |
   | qwen3.6-35b-a3b | 4.75B | 3B |
   | gpt-oss-20b / 120b | 4.19B / 5.75B | 3.61B / 5.13B |
   | glm-4.7-flash | 5.17B | ~3B |
   | glm-4.5-air | 17.0B | 12B |
   | deepseek-v3.2 | 51.9B | 37B |
   | gemma-4-26b-a4b | 4.40B | 3.8B |
   | nemotron-3-super-120b-a12b | 15.7B | 12B |
   | nemotron-3.5-lightning | 3.58B | 3B |

   Fix: either add `activeParams` overrides, as Qwen3.8-Flash-Next and the Gemma E models already have, or keep the estimate for speed and show the card's figure.
2. **Context lengths that disagree with the card.**
   - qwen3-32b: 40,960 in the catalogue; the card says 32,768 native, and the entry's own watchOut says 32K.
   - devstral-small-2-24b: 393,216 from the config; the card says 256k. The site example shows "384k".
   - deepseek-v3.2: 163,840; the tech report says 128K.
3. **`released` is the Hugging Face repo creation date, not the release date.**
   - devstral-small-2-24b: catalogue 2025-11-28, released 2025-12-09.
   - Ministral 3 (all three): catalogue 2025-10-31, released 2025-12-02.
   - Nemotron 3 Nano: catalogue 2025-12-04, card 15 Dec 2025.
   - Nemotron 3.5 Lightning: catalogue 2026-08-01, card 11 Aug 2026.
   - Nemotron 3 Super: catalogue 2026-03-10, card 11 Mar 2026.
   - Granite 4.2 (all three): catalogue 2026-08-07, card 25 Aug 2026.
4. **Totals off slightly:** glm-4.5-air 110.5B vs 106B; deepseek-v4-flash 290.9B vs 284B.
5. **deepseek-v4-flash SWE-bench Multilingual 73.3** was pulled from the Hub's eval results without a setting. It is the Max-mode score, which matches the other entries, but it should be labelled.
6. **nemotron-3-super swe-bench-multilingual 45.8** came from the Hub. The card says 45.78 (OpenHands). Pin it in `sources.json`.

### Prices and plans (`internal/cost/prices.go`)

7. **Claude Mythos 5 (`claude-mythos-5`) is missing.** The page lists $10 / $12.50 / $20 / $1 / $50, and `claude-mythos-5-1` is not a prefix of it, so `RatesFor` returns nothing. Opus 4.1, Opus 4, Sonnet 4 and Haiku 3.5 are also still on the page and unpriced, so older history goes unpriced.
8. **Plan names changed on 30 Oct 2026.** Max 5x is now "Max" ($100) and Max 20x is now "Ultra" ($200, 10x Pro weekly, no 5-hour limit for verified accounts). The prices are unchanged. `PlanFor` still prints "Max 5x" and "Max 20x". The tier detection in `setup/scanner.go` was not checked against an Ultra account.

### Hardware tables (`internal/match`)

9. **M5 Ultra is missing from `appleBandwidth`, and falls through to the base M5 row.** `containsWord("APPLE M5 ULTRA", "M5")` matches, so an M5 Ultra Mac Studio (shipping since Sep 2026, Apple quotes 1.2 TB/s) is estimated at **153 GB/s**, about 8x too slow. `published.go` already has M5 Ultra rows.
10. **Rows missing from `applePublished`:** M3 10-core, M5 10-core, M6 8-core, M1 7-core and M2 8-core. The 29 rows present are exact, as are the build-date comments.
11. **The CUDA substring match picks the wrong rows.**
    - "RTX 5090 Laptop" gets the desktop 5090 numbers, 14073/290 instead of 6667/156. Any "Laptop GPU" has the same problem.
    - "RTX A1000" contains "A100".
    - 4090 D, 4080 SUPER and 3090 Ti take the plain-name row.
    - In `discreteBandwidth`, "3060 Ti" and "3070 Ti" match the non-Ti rows.

### Runtimes

12. **The llama.cpp cache dir is wrong for `-hf` downloads** (`runtimes/llamacpp.go:13-27`). Since build b8498 (Mar 2026), `-hf` downloads go to the Hugging Face hub cache (`LLAMA_CACHE`, then `HF_HUB_CACHE`, `HUGGINGFACE_HUB_CACHE`, `$HF_HOME/hub`, then `~/.cache/huggingface/hub`). lo0's own llama.cpp step uses `-hf`, so it never sees the models it downloaded.
13. **The Ollama default-context comment** (`backend/ollama.go:24-26`) is out of date. The default now scales with VRAM: 4k, 32k, or 256k at 48 GiB and above. Setting `num_ctx` is still right. `minOllamaAnthropic = 0.14.0` is correct.
14. **The llama-server default port is now 9931.** lo0 passes `--port 8080`, so nothing breaks, but `DefaultLlamaServerURL` should not be described as the server's default.

### Harnesses

15. **`claude/plan.go:59`: "every model alias resolves to a local model"** is not true. `ANTHROPIC_DEFAULT_FABLE_MODEL` is not set, so the `fable` and `best` aliases request `claude-fable-5-1` from the local runtime.
16. **`claude/plan.go:75`: the subagent comment is backwards.** A model passed at spawn time or set in an agent's definition beats `CLAUDE_CODE_SUBAGENT_MODEL`. Forcing one model takes `CLAUDE_CODE_SUBAGENT_MODEL_FORCE` (2.1.257+).
17. **Hook types.**
    - `claude/features.go:51` says the local model judges every non-command hook. `opencode/features.go:138` says those hooks are "evaluated by Claude".
    - Only `prompt` and `agent` hooks use a model; `http` and `mcp_tool` hooks run as they are.
18. **`opencode/status.go:19`: "OpenCode documents no version flag"** is wrong; `--version`/`-v` is documented.
19. **OpenCode 2 is not handled.**
    - OpenCode 2.0 shipped on 11 Sep 2026 and Homebrew now installs it.
    - V2 only reads AGENTS.md, not CLAUDE.md, and V1 plugins do not run in it.
    - lo0's install script and feature mapping are V1 only.
    - Fixed: lo0 reads the installed version with `opencode --version` during the assessment and maps, plans and starts OpenCode 1 or 2 to match; an OpenCode that is not installed is treated as 2, which is what lo0 installs. For 2, the user's `~/.claude/CLAUDE.md` is added to `"instructions"`, the config uses `providers` (package `@opencode/ai/providers/openai-compatible`, `settings.baseURL`) and `mcp.servers` without `enabled`, subagents get a `permissions` rule list with `shell` for bash, and hooks name the V2 plugin hooks. `OPENCODE_CONFIG_CONTENT` is not in the V2 docs but V2 2.2.5 still loads it above global and project config (`packages/core/src/config.ts`), so `lo0 try` keeps using it. The installer is `https://opencode.ai/v2/install` and the npm package `@opencode/cli`.

### Site and README

20. **"Runs offline · changes nothing"** contradicts what lo0 does now, which is npm and release lookups, model downloads, Try and Migrate. The phrase appears in the site hero (`index.astro:49`) and the meta description (`site.ts`).
21. **`privacy.md`** says the only network connections lo0 makes are to localhost runtimes, and that there is "no update check". The npm registry lookup (`claudecode/install`) and the Hugging Face downloads contradict it.
22. **"Prove" is named in user-facing copy** (`Terminal.astro:87`, `index.astro:198`), against the AGENTS.md rule.
23. **The site example lists Nemotron-3-Nano-30B-A3B as tier B** (`example.ts`). The catalogue has it as C.
24. **The README says "a catalogue of 28 open-weight models".** There are 36, including Granite and Ministral, which the list leaves out.

## Unsupported by the card (the profile rule)

These lines may be true, but the linked card does not say them. Move them to `match.Insights` or reword them.

- qwen3-coder-next: "Hybrid DeltaNet attention for efficient long context". The card shows the layout but makes no efficiency claim.
- devstral-small-2-24b: "Dense 24B: slower than small-active MoE models".
- granite-4.2-30b: "Dense 30B: slower than MoE models of similar size".
- granite-4.2-30b: "Best results need a custom reasoning parser". This overstates the card: the built-in `nemotron_v3` parser works, and the custom one only formats reasoning better.
- ministral-3-14b and -8b: "General edge model, not tuned for coding". The cards list agentic use cases.
- glm-5.3 ("about 753B") and minimax-m2.7 ("about 229B"): these counts come from Hugging Face metadata, not the card.
- deepseek-v3.2: "about 685B parameters". DeepSeek's own V4 card says 671B; 685B includes the MTP layer.
- qwen2.5-coder-32b: "Aider 73.7" and "40+ languages" are in the Qwen blog the card links to, not the card itself.

## Missing official Ollama tags

| Model | Tag |
|---|---|
| qwen3-coder-next | `qwen3-coder-next` |
| qwen3.8-27b | `qwen3.8:27b` |
| qwen3.8-flash-next | `qwen3.8-flash-next:125b-a6b-q4_K_M` (no `latest`) |
| devstral-small-2-24b | `devstral-small-2:24b` |
| ministral-3-14b | `ministral-3:14b` |
| ministral-3-8b | add `ministral-3` (the 8B is `latest`) |
| glm-4.7-flash | `glm-4.7-flash` |
| gemma-4-26b-a4b / 31b | `gemma4:26b`, `gemma4:31b` |
| nemotron-3-nano-30b-a3b | `nemotron-3-nano:30b` (not `:4b`) |
| nemotron-3.5-lightning | `nemotron-3.5-lightning:30b` |
| nemotron-3-super-120b-a12b | `nemotron-3-super:120b` |
| granite-4.2-30b | `granite4.2:30b` |

GLM-5.3 and MiniMax-M2.7 have only `:cloud` tags, which do not run locally. Kimi K2 Thinking, DeepSeek V3.2 and V4 Flash have empty Ollama pages, so leaving those models without tags is correct.

## Worth knowing

- **Qwen's SWE-bench Pro scores** for 3.6 and 3.8 are on Qwen's own corrected task set, not stock `ScaleAI/SWE-bench_Pro`.
- **DeepSeek-V4-Flash** has been superseded by `DeepSeek-V4-Flash-0731`, its official release. Ollama's page describes the new one.
- **Tier judgement calls** were flagged but are not clear errors:
  - Qwen3.5-9B/4B and Ministral 3 at C may fit B.
  - gpt-oss-20b (B) and 120b (A) are 1.7 points apart on SWE-bench Verified.
- **Verified versions:**
  - Claude Code is verified at 2.1.317; latest is 2.1.318.
  - Codex is verified at 0.155.0; latest is 0.166.0, 11 minor releases behind.

## Unverifiable

- GLM-5.3 and MiniMax-M2.7 active counts: neither card states one.
- The LM Studio models path `~/.lmstudio/models`.
- OpenCode following XDG on every platform.
- Whether claude.ai connectors load when `ANTHROPIC_AUTH_TOKEN` is set.

---

# Per-model and per-file detail

## Fact-check: Qwen models in internal/catalog (2026-11-05)

Sources fetched for every model: `https://huggingface.co/<repo>/raw/main/README.md`, `https://huggingface.co/api/models/<repo>`, `https://huggingface.co/<repo>/raw/main/config.json`, `https://huggingface.co/api/models/<gguf>` (all 12 GGUF repos returned 200), `https://ollama.com/library/<name>/tags`. Extra: Qwen2.5-Coder blog (card defers to it), Qwen3.8-Flash-Next LICENSE file.

Verdicts: OK / WRONG / UNSUPPORTED / UNVERIFIABLE. "OK (inference)" = not stated in the card, but follows directly from facts the card does state.

Caveat that applies to every "swe-bench-pro" score from Qwen3.6 or Qwen3.8 cards: Qwen says "We correct some problematic tasks in the public set of SWE-bench Pro" (3.6) and "Problematic tasks were corrected" (3.8). These scores are on Qwen's refined set, not the stock `ScaleAI/SWE-bench_Pro` that the `swe-bench-pro` key names.

---

## qwen3-coder-30b-a3b (Qwen/Qwen3-Coder-30B-A3B-Instruct)

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| summary | Compact MoE for agentic coding in Qwen Code, Cline | "supporting for most platform such as Qwen Code, CLINE" | OK | |
| goodAt | 30.5B MoE, 3.3B active | "30.5B in total and 3.3B activated" | OK | |
| goodAt | 256K native, 1M with YaRN | "native support for 256K ... up to 1M tokens using Yarn" | OK | |
| goodAt | Tuned for agentic coding with tool calling | "Qwen3-Coder excels in tool calling capabilities" | OK | |
| goodAt | Works with Qwen Code and Cline | Qwen Code, CLINE named | OK | |
| watchOut | Non-thinking only | "supports only non-thinking mode" | OK | |
| watchOut | Own tool-call format; runtime must support it | "featuring a specially designed function call format" | OK | |
| benchmarks | none | Scores only in an image and the blog | OK | |
| ollama | qwen3-coder:30b, qwen3-coder | both exist, same digest 06c1097efce0 (19GB) | OK | |
| gguf | unsloth/...-GGUF | HTTP 200 | OK | |
| catalog | apache-2.0, 2025-07-31, 30.53B / 3.35B, 262144 | apache-2.0, createdAt 2025-07-31, 30.5B/3.3B, 262,144 | OK | |
| tier | A | Built for agentic coding; scores published in the card image and blog, none recorded in sources | OK | You could add benchmark entries from the blog |

## qwen3-coder-next (Qwen/Qwen3-Coder-Next)

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| summary | Coding-agent model for local dev, long-horizon agentic coding | "designed specifically for coding agents and local development" | OK | |
| goodAt | 80B MoE, 3B active | "80B in total and 3B activated" | OK | |
| goodAt | Trained for tool use and failure recovery | "complex tool usage, and recovery from execution failures" | OK | |
| goodAt | 256K; Claude Code, Cline etc. | "256k context ... (e.g., Claude Code, Qwen Code, ... Cline" | OK | |
| goodAt | Hybrid DeltaNet attention for efficient long context | The card gives the layout "Gated DeltaNet -> MoE" but makes no efficiency or long-context claim | UNSUPPORTED (minor) | "Hybrid Gated DeltaNet + attention layout" |
| watchOut | Non-thinking only | "supports only non-thinking mode" | OK | |
| watchOut | 80B weights need lots of memory | 80B total | OK (inference) | |
| watchOut | Needs recent runtime with its tool-call parser | "sglang>=v0.5.8", "vllm>=0.15.0", "--tool-call-parser qwen3_coder" | OK | |
| ollama | none listed | The official `qwen3-coder-next` exists: latest/q4_K_M (52GB), q8_0 (85GB) | WRONG (omission) | Add `qwen3-coder-next`, `qwen3-coder-next:q4_K_M` |
| gguf | unsloth/Qwen3-Coder-Next-GGUF | 200 | OK | |
| catalog license/released/params/ctx | apache-2.0, 2026-01-30, 79.67B, 262144 | apache-2.0, 2026-01-30, 80B, 262,144 | OK | |
| catalog activeParams | 3,874,929,408 (3.9B) | "3B activated" | WRONG (vs card) | Override `activeParams: 3000000000` in sources.json, as is done for Flash-Next |
| tier | A | Agentic coding model; SWE-bench scores in card images | OK | |

## qwen3-coder-480b-a35b (Qwen/Qwen3-Coder-480B-A35B-Instruct)

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| summary | Flagship, comparable to Claude Sonnet | "achieving results comparable to Claude Sonnet" | OK | |
| goodAt | Lab claims Sonnet-level agentic coding | same | OK | |
| goodAt | 256K native, 1M YaRN | "256K ... up to 1M tokens using Yarn" | OK | |
| goodAt | Tool calling for Qwen Code, Cline | Qwen Code, CLINE; "excels in tool calling" | OK | |
| watchOut | 480B total, 35B active | "480B in total and 35B activated" | OK | |
| watchOut | Non-thinking only | "supports only non-thinking mode" | OK | |
| ollama | qwen3-coder:480b | exists, 290GB, digest 72b144baf727 (= 480b-a35b-q4_K_M) | OK | |
| gguf | unsloth/...-GGUF | 200 | OK | |
| catalog | apache-2.0, 2025-07-22, 480.15B / 35.47B, 262144 | matches | OK | |
| tier | A | | OK | |

## qwen3.6-35b-a3b (Qwen/Qwen3.6-35B-A3B)

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| summary | Multimodal MoE; agentic coding, repo-level reasoning | "Causal Language Model with Vision Encoder"; "repository-level reasoning" | OK | |
| goodAt | SWE-bench Verified 73.4, 3B active | 73.4 (Qwen3.6-35BA3B column); "3B activated" | OK | |
| goodAt | Terminal-Bench 2.0 51.5 | 51.5 | OK | |
| goodAt | Preserve-thinking option | `preserve_thinking` option documented | OK | |
| goodAt | 262K native, ~1M YaRN | "262,144 natively and extensible up to 1,010,000" | OK | |
| watchOut | Thinks by default; disable via chat_template_kwargs | "think by default"; `"chat_template_kwargs": {"enable_thinking": False}` | OK | |
| watchOut | Lab advises ≥128k | "at least 128K tokens" | OK | |
| watchOut | No /think, /nothink | "does not officially support the soft switch" | OK | |
| bench swe-bench-verified | 73.4 | 73.4 | OK | |
| bench terminal-bench-2.0 | 51.5 | 51.5 (Terminus-2 harness) | OK | |
| bench swe-bench-pro | 49.5 | 49.5, on Qwen's corrected set | OK (variant caveat) | |
| bench MCPMark | 37.0 | 37.0 | OK | |
| ollama | qwen3.6, qwen3.6:35b | latest = 35b = 35b-a3b, digest 86b2d171ce90 | OK | |
| gguf | unsloth/Qwen3.6-35B-A3B-GGUF | 200 | OK | |
| catalog license/released/params/ctx | apache-2.0, 2026-04-15, 35.95B, 262144 | matches (35B) | OK | |
| catalog activeParams | 4,746,200,944 (4.7B) | "35B in total and 3B activated" | WRONG (vs card) | Override `activeParams: 3000000000` |
| tier | A | | OK | |

## qwen3.6-27b (Qwen/Qwen3.6-27B)

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| summary | Dense multimodal, agentic coding, repo-level | 27B dense with vision encoder | OK | |
| goodAt | SWE-bench Verified 77.2 | 77.2 (last column; Claude 4.5 Opus is 80.9) | OK | |
| goodAt | Terminal-Bench 2.0 59.3 | 59.3 (Claude 4.5 Opus also 59.3) | OK | |
| goodAt | SkillsBench 48.2 via OpenCode | "SkillsBench Avg5" 48.2; "Evaluated via OpenCode" | OK | |
| goodAt | Preserve-thinking option | documented | OK | |
| watchOut | Dense 27B slower than 3B-active MoE | dense 27B | OK (inference) | |
| watchOut | Thinks by default; long outputs | "thinking mode by default"; 32,768 to 81,920 output advised | OK | |
| watchOut | ≥128k context | "at least 128K tokens" | OK | |
| bench swe-bench-verified | 77.2 | 77.2 | OK | |
| bench terminal-bench-2.0 | 59.3 | 59.3 | OK | |
| bench swe-bench-pro | 53.5 | 53.5, on Qwen's corrected set | OK (variant caveat) | |
| bench SkillsBench Avg5 | 48.2 | 48.2 | OK | |
| ollama | qwen3.6:27b | exists, 18-19GB | OK | |
| gguf | 200 | | OK | |
| catalog | apache-2.0, 2026-04-21, 27.78B dense, 262144 | matches | OK | |
| tier | A | | OK | |

## qwen3.8-27b (Qwen/Qwen3.8-27B)

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| summary | Dense VL model, coding and long-horizon agentic | "native vision-language model"; "long-horizon agentic tasks" | OK | |
| goodAt | Terminal Bench 2.1 73.0 (Terminus) | "Terminal Bench 2.1 (Terminus)" 73.0 | OK | |
| goodAt | SWE-bench Pro 61.7 in Claude Code harness | 61.7; "evaluated with the Claude Code harness" | OK | |
| goodAt | Tunable reasoning_effort; preserved thinking default | "`preserve_thinking` is enabled by default" | OK | |
| goodAt | 262K native, 1M YaRN | "262,144 natively and extensible up to 1,000,000" | OK | |
| watchOut | Dense 27B slower than MoE | dense | OK (inference) | |
| watchOut | reasoning_effort defaults to xhigh | "`xhigh` (default)" | OK | |
| watchOut | Lab does not document tool calling setup | The card has no tool-call parser or tool section and defers to the SGLang/vLLM recipes | OK | |
| bench terminal-bench-2.1 Terminus | 73.0 | 73.0 | OK | |
| bench swe-bench-pro | 61.7 | 61.7, Claude Code harness, corrected set | OK (variant caveat) | |
| bench NL2Repo-Bench | 42.3 | 42.3 | OK | |
| bench deep-swe-1.1 | 42.2 | "DeepSWE 1.1" 42.2 | OK | |
| ollama | none listed | The official `qwen3.8` exists: latest = 27b (18GB, e118e4d12a70), 27b-q4_K_M, 27b-q8_0, ... | WRONG (omission) | Add `qwen3.8:27b`, `qwen3.8` |
| gguf | 200 | | OK | |
| catalog | apache-2.0, 2026-08-05, 27.78B, 262144 | matches | OK | |
| tier | A | | OK | |

## qwen3.8-flash-next (Qwen/Qwen3.8-Flash-Next)

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| summary | Experimental preview of Qwen4 architecture, sparse MoE | "experimental preview of the architecture that will underpin Qwen4" | OK | |
| goodAt | 125B MoE, 6B active | "125B with 6B activated" | OK | |
| goodAt | SWE-bench Pro 62.5, Multilingual 81.0 | 62.5; 81.0 | OK | |
| goodAt | Toolathlon Verified 73.5 | "Toolathlon Verified (Pass@1)" 73.5 | OK | |
| goodAt | Sparse attention cuts long-context latency | "This cuts long-context latency significantly" | OK | |
| watchOut | Experimental preview | same | OK | |
| watchOut | Custom licence; MaaS firms need separate deal | license `qwen-community-1.0`; LICENSE §2: "Model as a Service ... shall obtain a separate license" | OK (LICENSE file, linked from the card) | |
| watchOut | ~180B weights incl. 51B n-gram | "plus 51B n-gram embedding and 4B MTP" (125+51+4 = 180); safetensors 179.99B | OK | |
| bench swe-bench-pro | 62.5 | 62.5, Claude Code harness, corrected set | OK (variant caveat) | |
| bench swe-bench-multilingual | 81.0 | 81.0 (mini-SWE-agent) | OK | |
| bench deep-swe-1.1 | 58.7 | 58.7; "highest score across the two harnesses" | OK (best of Claude Code / mini-SWE-agent) | Consider a setting note |
| bench Toolathlon | 73.5 | 73.5 | OK | |
| ollama | none listed | `qwen3.8-flash-next` exists: 125b-a6b-q4_K_M (120GB), q8_0, bf16, nvfp4, mlx. No `latest` tag | WRONG (omission) | Add `qwen3.8-flash-next:125b-a6b-q4_K_M` (do not use the bare name, which has no `latest`) |
| gguf | 200 | | OK | |
| catalog license | other | other / qwen-community-1.0 | OK | |
| catalog released | 2026-08-24 | createdAt 2026-08-24 | OK | |
| catalog params / active | 180.0B / 6B (override) | 125B + 51B n-gram + 4B MTP; 6B active | OK | |
| catalog ctx | 262144 | 262,144 natively | OK | |
| tier | A | | OK | |

## qwen3.5-9b (Qwen/Qwen3.5-9B)

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| summary | Small dense multimodal, thinking, tool use | 9B, vision encoder, thinking default, tool parser | OK | |
| goodAt | BFCL-V4 66.1, TAU2 79.1 | 66.1; 79.1 (Qwen3.5-9B column) | OK | |
| goodAt | 262K native, ~1M YaRN | "262,144 natively and extensible up to 1,010,000" | OK | |
| goodAt | Tool calling, qwen3_coder parser | `--tool-call-parser qwen3_coder` | OK | |
| watchOut | No agentic coding scores | no SWE-bench or Terminal-Bench on the card | OK | |
| watchOut | LCB 65.6, below gpt-oss-20b | 65.6 vs GPT-OSS-20B 74.6 | OK | |
| watchOut | Thinks by default; 128k+ | both stated | OK | |
| bench bfcl-v4 / tau2 / lcb-v6 / OJBench | 66.1 / 79.1 / 65.6 / 29.2 | 66.1 / 79.1 / 65.6 / 29.2 | OK | |
| ollama | qwen3.5:9b | exists (= latest, 6.6-7.6GB) | OK | |
| gguf | 200 | | OK | |
| catalog | apache-2.0, 2026-02-27, 9.65B, 262144 | matches (9B LM + vision) | OK | |
| tier | C | Documented tool parser and TAU2 79.1. The card gives no evidence of drift | Debatable | B arguably fits tierNotes better; also see qwen3-32b, rated B with less evidence |

## qwen3.5-4b (Qwen/Qwen3.5-4B)

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| summary | Small multimodal, thinking, documented tool calling | yes | OK | |
| goodAt | TAU2 79.9 > 9B's 79.1 | 79.9 vs 79.1 | OK | |
| goodAt | 262K, ~1M YaRN | 1,010,000 | OK | |
| goodAt | qwen3_coder parser | yes | OK | |
| watchOut | No agentic coding scores | none on card | OK | |
| watchOut | BFCL-V4 50.3 vs 66.1 | 50.3 / 66.1 | OK | |
| watchOut | Thinks by default; 128K+ | both stated | OK | |
| bench bfcl / tau2 / lcb / OJBench | 50.3 / 79.9 / 55.8 / 24.1 | 50.3 / 79.9 / 55.8 / 24.1 | OK | |
| ollama | qwen3.5:4b | exists, 3.3-4.0GB | OK | |
| gguf | 200 | | OK | |
| catalog | apache-2.0, 2026-02-27, 4.66B, 262144 | matches | OK | |
| tier | C | Same argument as for 9B; BFCL is weaker | OK / debatable | |

## qwen3.5-2b (Qwen/Qwen3.5-2B)

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| summary | Aimed at prototyping and fine-tuning | "intended use cases are prototyping, task-specific fine-tuning" | OK | |
| goodAt | qwen3_coder parser | `--tool-call-parser qwen3_coder` | OK | |
| goodAt | BFCL-V4 43.6 > Qwen3-4B-2507 39.9 | 43.6 vs 39.9 (thinking) | OK | |
| goodAt | 262K native | "262,144 natively" (no YaRN claim; correctly omitted) | OK | |
| goodAt | Non-thinking default, thinking on request | "Qwen3.5-2B operates in non-thinking mode by default" | OK | |
| watchOut | prototyping/fine-tuning | yes | OK | |
| watchOut | No coding/agentic coding scores | none in the language table | OK | |
| watchOut | TAU2 48.8 with thinking | 48.8 under "General Agent (Thinking)" | OK | |
| bench bfcl-v4 thinking / tau2 thinking | 43.6 / 48.8 | 43.6 / 48.8 | OK | |
| ollama | qwen3.5:2b | exists, 2.7-3.1GB | OK | |
| gguf | 200 | | OK | |
| catalog | apache-2.0, 2026-02-28, 2.27B, 262144 | createdAt 2026-02-28T23:56Z; 2B | OK | |
| tier | C | | OK | |

## qwen3-32b (Qwen/Qwen3-32B)

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| summary | General dense, switchable thinking, agent tools | "seamless switching between thinking mode ... and non-thinking mode" | OK | |
| goodAt | /think, /no_think | soft switch documented | OK | |
| goodAt | Tools in both modes | "external tools in both thinking and unthinking modes" | OK | |
| goodAt | 131K validated with YaRN | "validated ... up to 131,072 tokens using the YaRN" | OK | |
| watchOut | Native 32K | "32,768 natively" | OK | |
| watchOut | Not code-specialised | general Qwen3 model | OK (inference) | |
| watchOut | Dense 32B slower | dense | OK (inference) | |
| ollama | qwen3:32b | exists, 20GB, 40K ctx | OK | |
| gguf | 200 | | OK | |
| catalog license/released/params | apache-2.0, 2025-04-27, 32.76B | matches | OK | |
| catalog contextLength | 40960 | "32,768 natively"; 40,960 is config max_position_embeddings ("reserving 32,768 tokens for outputs and 8,192 tokens for typical prompts") | WRONG (vs card) | Override to 32768, or accept 40960 knowingly; it contradicts the "Native context only 32K" watchOut |
| tier | B | Tool calling documented, no agentic coding scores | OK | |

## qwen2.5-coder-32b (Qwen/Qwen2.5-Coder-32B-Instruct)

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| summary | Code generation, reasoning, repair | "code generation, code reasoning and code fixing" | OK | |
| goodAt | Aider code repair 73.7 (blog) | Blog: "Aider is a popular benchmark for code repair, and ... scored 73.7" | OK (blog, which the card links) | |
| goodAt | 40+ languages | Blog: "more than 40 programming languages" | OK (blog) | |
| goodAt | 128K with YaRN | "Long-context Support up to 128K"; YaRN beyond 32,768 | OK | |
| watchOut | Tool calling not documented | no tool or function content on the card | OK | |
| watchOut | 32k default; YaRN can hurt short prompts | "config.json is set for context length up to 32,768"; "potentially impacting performance on shorter texts" | OK | |
| watchOut | Dense 32B; Nov 2024 | createdAt 2024-11-06 | OK | |
| bench Aider | 73.7 (name "Aider", not the aider-polyglot key) | 73.7 on 2024 Aider code editing; correctly kept off the `aider-polyglot` key | OK | |
| ollama | qwen2.5-coder:32b | exists, 20GB | OK | |
| gguf | 200 | | OK | |
| catalog | apache-2.0, 2024-11-06, 32.76B, 32768 | apache-2.0; card says "32.5B"; "Full 131,072 tokens" but config is 32,768 | OK (ctx uses config value; card's headline is 131,072 with YaRN) | |
| tier | C | No documented tool calling. tierNotes C covers "undocumented" | OK | |

## Catalogue fact-check: misc models (2026-11-05)

Sources fetched: `https://huggingface.co/<repo>/raw/main/README.md`, `https://huggingface.co/api/models/<repo>`, `<repo>/raw/main/config.json`, `https://ollama.com/library/<name>/tags`, and where the card defers: gpt-oss model card (arXiv 2508.10925), GLM-4.5 tech report (arXiv 2508.06471), DeepSeek-V3.2 report (arXiv 2512.02556, same as repo `assets/paper.pdf`), Mistral blogs for release dates. Every GGUF repo listed returned HTTP 200 from the HF API.

Common pattern: catalog.json `activeParams`/`params` are computed from safetensors and config, so they count embeddings and MTP layers. They come out above the figures the labs publish (gpt-oss, GLM-4.5-Air, GLM-4.7-Flash, DeepSeek-V3.2, DeepSeek-V4-Flash). `released` is the HF repo `createdAt`, which for gated Mistral repos is weeks before the public release.

## gpt-oss-20b

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| summary | low-latency, local and agentic use | "for lower latency, and local or specialized use cases" | OK | |
| goodAt | 21B MoE, 3.6B active; runs within 16GB | "21B parameters with 3.6B active"; "run within 16GB of memory" | OK | |
| goodAt | Native function calling and structured outputs | "native capabilities for function calling ... and Structured Outputs" | OK | |
| goodAt | SWE-bench Verified 60.7 at high | arXiv table: 20b high 60.7 | OK | |
| goodAt | Adjustable reasoning effort low/medium/high | "Configurable reasoning effort ... (low, medium, high)" | OK | |
| watchOut | Needs harmony format | "should only be used with the harmony format" | OK | |
| watchOut | No reasoning-off mode documented | only low/medium/high documented | OK | |
| watchOut | Aider Polyglot only 34.2 even at high | arXiv: 20b high 34.2 | OK | |
| bench swe-bench-verified (high) | 60.7 | 60.7 | OK | |
| bench tau-bench-retail (high) | 54.8 | 54.8 | OK | |
| bench aider-polyglot (high) | 34.2 | 34.2 | OK | |
| bench codeforces-elo (with tools, high) | 2516 | 2516 | OK | |
| ollama | gpt-oss:20b, gpt-oss | both exist, same digest f38aa0c53da5 (14GB) | OK | |
| gguf | ggml-org/gpt-oss-20b-GGUF | 200 | OK | |
| catalog license/released | apache-2.0 / 2025-08-04 | card apache-2.0; createdAt 2025-08-04 | OK | |
| catalog params | 20.91B | arXiv Table 1: 20.91B | OK | |
| catalog activeParams | 4.19B | arXiv Table 1: 3.61B active | WRONG | add `activeParams: 3610000000` override |
| catalog contextLength | 131072 | config 131072 | OK | |
| catalog quants MXFP4 | 13.78 GB | 12.8 GiB checkpoint (13.74 GB) | OK | |
| tier | B | SWE-V 60.7, general reasoning model | OK (see note) | 120b is A on 62.4. A 1.7-point gap with different tiers is inconsistent. Both are general models, so B/B is easier to defend. |

## gpt-oss-120b

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| summary | production, high-reasoning and agentic use | "for production, general purpose, high reasoning use cases" | OK | |
| goodAt (3 lines) | function calling; SWE-V 62.4 high; low/medium/high | card + arXiv 120b high 62.4 | OK | |
| watchOut (2 lines) | harmony; no reasoning-off | as 20b | OK | |
| bench swe-bench-verified (high) | 62.4 | 62.4 | OK | |
| bench tau-bench-retail (high) | 67.8 | 67.8 | OK | |
| bench aider-polyglot (high) | 44.4 | 44.4 | OK | |
| bench codeforces-elo (with tools, high) | 2622 | 2622 | OK | |
| ollama | gpt-oss:120b | exists, 65GB, digest 714d86958b6e | OK | |
| gguf | ggml-org/gpt-oss-120b-GGUF | 200 | OK | |
| catalog params | 116.83B | 116.83B | OK | |
| catalog activeParams | 5.75B | arXiv Table 1: 5.13B active | WRONG | override `activeParams: 5130000000` |
| catalog quants MXFP4 | 65.29 GB | 60.8 GiB (65.28 GB) | OK | |
| tier | A | general model, SWE-V 62.4, Aider 44.4 | borderline | see 20b note |

## devstral-small-2-24b

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| summary | explores codebases, multi-file edits | "excels at using tools to explore codebases, editing multiple files" | OK | |
| goodAt | SWE-bench Verified 68.0% at 24B | table: Devstral Small 2, 24, 68.0% | OK | |
| goodAt | 256K context window | "A 256k context window" | OK | |
| goodAt | Apache 2.0; tool calling via mistral parser | Apache 2.0; `--tool-call-parser mistral` | OK | |
| watchOut | Terminal Bench 2 only 22.5% | 22.5% | OK | |
| watchOut | Lab warns local serving can underperform | "If you notice subpar performance with local serving..." | OK (slightly stronger than the source) | could say "Lab suggests its API if local serving underperforms" |
| watchOut | Dense 24B: slower than small-active MoE models | card gives size only; nothing about dense vs MoE speed | UNSUPPORTED | move to match.Insights or drop |
| bench swe-bench-verified | 68.0 | 68.0% | OK | |
| bench swe-bench-multilingual | 55.7 | 55.7% | OK | |
| bench terminal-bench-2.0 | 22.5 | "Terminal Bench 2" 22.5% | OK | |
| ollama | none | official `devstral-small-2:24b` (= latest, 15GB, digest 24277f07f62d); the card links ollama.com/library/devstral-small-2 | MISSING | add `devstral-small-2:24b`, `devstral-small-2` |
| gguf | unsloth/...-GGUF | 200 | OK | |
| catalog license | apache-2.0 | apache-2.0 | OK | |
| catalog released | 2025-11-28 | Mistral blog dated 2025-12-09 (createdAt of gated repo is 2025-11-28) | WRONG | 2025-12-09 |
| catalog params | 24.01B | "24 billion parameters" | OK | |
| catalog contextLength | 393216 | card "256k"; vLLM example `--max-model-len 262144` (config max_position_embeddings 393216) | WRONG vs card | 262144 |
| tier | A | agentic coding model, SWE-V 68.0 | OK | |

## devstral-small-2507

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| summary | Devstral Small 1.1 built with All Hands AI | "collaboration between Mistral AI and All Hands AI" | OK | |
| goodAt | SWE-bench Verified 53.6% | "53.6% on SWE-Bench Verified" (OpenHands scaffold) | OK | |
| goodAt | Mistral function calling; OpenHands recommended | "Supports Mistral's function calling format"; "We recommend ... OpenHands" | OK | |
| goodAt | Apache 2.0 | Apache 2.0 | OK | |
| watchOut | Text-only (vision encoder removed) | "text-only ... the vision encoder was removed" | OK | |
| watchOut | 128K context, less than newer models | "A 128k context window" | OK (the comparison is lo0's own) | |
| watchOut | Needs Mistral tokenizer and tool parser | `--tokenizer_mode mistral ... --tool-call-parser mistral` | OK | |
| bench swe-bench-verified | 53.6 | 53.6 | OK | |
| ollama | none | Ollama `devstral` tags are all small-2505; no 2507 tag | OK | |
| gguf | unsloth/Devstral-Small-2507-GGUF | 200 | OK | |
| catalog | apache-2.0, 2025-07-04, 23.57B, 131072 | card/config agree | OK | |
| tier | A | | OK | |

## ministral-3-14b

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| summary | edge-oriented, vision, native function calling | "designed for edge deployment"; vision; "native function calling" | OK | |
| goodAt | function calling and JSON output | "native function calling and JSON outputting" | OK | |
| goodAt | 256K context | "Supports a 256k context window" | OK | |
| goodAt | Apache 2.0 | Apache 2.0 | OK | |
| watchOut | General edge model, not tuned for coding | card never says this; it lists "Advanced local agentic use cases" | UNSUPPORTED | e.g. "No coding focus in its stated use cases" |
| watchOut | No coding-agent benchmarks for instruct model | instruct table: Arena Hard, WildBench, MATH, MM MTBench only | OK | |
| ollama | none | official `ministral-3:14b` (9.1GB, digest 4760c35aeb9d) | MISSING | add `ministral-3:14b` |
| gguf | unsloth/...-GGUF | 200 | OK | |
| catalog released | 2025-10-31 | Mistral 3 blog dated 2025-12-02 | WRONG | 2025-12-02 |
| catalog params / context | 13.95B / 262144 | 13.5B LM + 0.4B vision; 256k | OK | |
| tier | C | tool calling documented, no long-session evidence | soft mismatch | C says "drift ... or undocumented"; neither applies. B ("unproven on long agentic coding sessions") fits better. Same for 8B/3B. |

## ministral-3-8b

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| goodAt | function calling/JSON; 256K; Apache 2.0 | as 14B | OK | |
| goodAt | Fits in 12GB VRAM in FP8 | "capable of fitting in 12GB of VRAM in FP8" | OK | |
| watchOut | General edge model, not tuned for coding | not stated; use cases are chat, assistant, translation, "Specialized agentic use cases" | UNSUPPORTED | rephrase as above |
| watchOut | No coding-agent benchmarks | correct | OK | |
| watchOut | Lab advises keeping the tool set minimal | "limit their number to the minimum required" | OK | |
| ollama | ministral-3:8b | exists, 6.0GB, digest 1922accd5827 (= `ministral-3:latest`) | OK | could add alias `ministral-3` |
| gguf | 200 | | OK | |
| catalog released | 2025-10-31 | 2025-12-02 | WRONG | 2025-12-02 |
| catalog params / context | 8.92B / 262144 | 8.4B LM + 0.4B vision; 256k | OK | |
| tier | C | | soft mismatch | see 14B |

## ministral-3-3b

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| summary | smallest Ministral 3 | "The smallest model in the Ministral 3 family" | OK | |
| goodAt | Fits in 8GB VRAM in FP8 | "fitting in 8GB of VRAM in FP8" | OK | |
| goodAt (others) | function calling/JSON; 256K; Apache 2.0 | | OK | |
| watchOut | Aimed at captioning, classification, extraction | "Image captioning, Text classification ... Data extraction" | OK | |
| watchOut | No coding-agent benchmarks; keep tools minimal | | OK | |
| ollama | ministral-3:3b | exists, 3.0GB, digest f04aa1c738f6 | OK | |
| gguf | 200 | | OK | |
| catalog released | 2025-10-31 | 2025-12-02 | WRONG | 2025-12-02 |
| catalog params / context | 3.85B / 262144 | 3.4B LM + 0.4B vision; 256k | OK | |
| tier | C | | soft mismatch | see 14B |

## glm-4.7-flash

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| summary | 30B-A3B MoE, performance/efficiency, local deployment | "30B-A3B MoE ... lightweight deployment that balances performance and efficiency" | OK | |
| goodAt | SWE-bench Verified 59.2 with ~3B active | 59.2; "30B-A3B" | OK | |
| goodAt | τ²-Bench 79.5 | 79.5 | OK | |
| goodAt | BrowseComp 42.8 | 42.8 | OK | |
| goodAt | MIT | license: mit | OK | |
| watchOut | Needs GLM tool and reasoning parser | `--tool-call-parser glm47 --reasoning-parser glm45` | OK | |
| watchOut | Enable Preserved Thinking for agentic tasks | "For multi-turn agentic tasks (τ²-Bench and Terminal Bench 2) ... Preserved Thinking" | OK | |
| bench swe-bench-verified | 59.2 | 59.2 | OK | |
| bench tau2-bench | 79.5 | 79.5 | OK | |
| bench livecodebench-v6 | 64.0 | "LCB v6" 64.0 | OK | |
| bench BrowseComp | 42.8 | 42.8 | OK | |
| ollama | none | official `glm-4.7-flash` (latest = q4_K_M 19GB; q8_0, bf16) | MISSING | add `glm-4.7-flash`, `glm-4.7-flash:q4_K_M` |
| gguf | unsloth/GLM-4.7-Flash-GGUF | 200 | OK | |
| catalog | mit, 2026-01-19, 31.2B, ctx 202752 | createdAt 2026-01-19; config 202752 | OK | |
| catalog activeParams | 5.17B | card "30B-A3B" | WRONG vs card | override ~3B (the lab gives no exact figure) |
| tier | A | | OK | |

## glm-4.5-air

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| summary | compact GLM-4.5 for agents, reasoning/coding/tools | "more compact design"; "unify reasoning, coding, and intelligent agent capabilities" | OK | |
| goodAt | 106B MoE with 12B active | "106 billion total ... 12 billion active" | OK | |
| goodAt | Hybrid thinking/non-thinking | "thinking mode for complex reasoning and tool usage, and non-thinking mode for immediate responses" | OK | |
| goodAt | SWE-bench Verified 57.6 (tech report) | report Table 5: Air 57.6 | OK | |
| goodAt | MIT, commercial use allowed | "MIT open-source license ... can be used commercially" | OK | |
| watchOut | 106B total weights need lots of memory | 106B | OK | |
| bench swe-bench-verified | 57.6 | 57.6 | OK | |
| bench Terminal-Bench | 30.0 | Table 5: Air 30.0 (Terminus, original Terminal-Bench) | OK | |
| bench bfcl-v3 | 76.4 | Table 3: Air 76.4 | OK | |
| bench tau-bench-retail | 77.9 | Table 3 TAU-Retail: Air 77.9 | OK | |
| ollama | none | `glm-4.5-air` and `glm-4.5` return 404 | OK | |
| gguf | 200 | | OK | |
| catalog | mit, 2025-07-20, ctx 131072 | | OK | |
| catalog params | 110.5B | card 106B (HF count includes the MTP layer) | minor mismatch | optional |
| catalog activeParams | 17.04B | card "12 billion active" | WRONG vs card | override `activeParams: 12000000000` |
| tier | A | | OK | |

## glm-5.3

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| summary | frontier coding, post-trained for complex coding/long-horizon | "every gain comes from post-training ... complex coding and long-horizon tasks" | OK | |
| goodAt | Terminal Bench 2.1 88.2 in Claude Code | 88.2; footnote "evaluate in Claude Code 2.1.207" | OK | |
| goodAt | DeepSWE 66.9, SWE-Marathon 42.5 | DeepSWE (v1.1) 66.9; SWE-Marathon (v1.1) 42.5 | OK | |
| goodAt | Toolathlon Verified 73.0 | 73.0 | OK | |
| goodAt | reasoning_effort low/high/max | "accepts three levels: low, high, and max" | OK | |
| watchOut | Huge: about 753B parameters | the card gives no parameter count; 753.3B is HF safetensors metadata | UNSUPPORTED by card text | acceptable as metadata; note the source |
| watchOut | Custom GLM-5.3 licence, not MIT/Apache | license_name glm-5.3; MIT-style plus a security review for MaaS firms over $10B revenue | OK | |
| watchOut | reasoning_effort defaults to max: token-heavy | "It defaults to max if not passed" | OK ("token-heavy" is inference) | |
| bench terminal-bench-2.1 | 88.2 | 88.2 | OK | |
| bench terminal-bench-3.0 | 28.3 | 28.3 | OK | |
| bench deep-swe-1.1 | 66.9 | 66.9 | OK | |
| bench SWE-Marathon (v1.1) | 42.5 | 42.5 | OK | |
| ollama | none | only `glm-5.3:cloud` (not local) | OK | |
| gguf | unsloth/GLM-5.3-GGUF | 200 | OK | |
| catalog | other, 2026-08-25, ctx 1048576 | createdAt 2026-08-25; config 1048576; evals use 1M context | OK | |
| catalog activeParams | 51.2B | not stated in card | UNVERIFIABLE | |
| tier | A | | OK | |

## minimax-m2.7

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| summary | SWE, agent harnesses, multi-agent productivity | "building complex agent harnesses and completing highly elaborate productivity tasks" | OK | |
| goodAt | Terminal Bench 2 57.0% | "Terminal Bench 2 (57.0%)" | OK | |
| goodAt | SWE-Pro 56.22%, SWE Multilingual 76.5 | "SWE-Pro ... 56.22%"; "SWE Multilingual (76.5)" | OK | |
| goodAt | Native Agent Teams and dynamic tool search | "native Agent Teams"; "dynamic tool search" | OK | |
| goodAt | Tool calling guide provided | "Tool Calling Guide" section | OK | |
| watchOut | Non-commercial licence; commercial needs approval | LICENSE: "commercial use requires prior written authorization" | OK | |
| watchOut | Large: about 229B parameters | the card gives no count; HF metadata 228.7B | UNSUPPORTED by card text | acceptable as metadata |
| bench swe-bench-pro | 56.22 | card says "SWE-Pro" (taken as SWE-bench Pro) | OK | |
| bench terminal-bench-2.0 | 57.0 | "Terminal Bench 2" | OK | |
| bench swe-bench-multilingual | 76.5 | "SWE Multilingual" | OK | |
| bench Multi SWE Bench | 52.7 | 52.7 | OK | |
| ollama | none | only `minimax-m2.7:cloud` | OK | |
| gguf | 200 | | OK | |
| catalog | other, 2026-04-09, 228.7B, ctx 204800 | createdAt 2026-04-09; config 204800 | OK | |
| catalog activeParams | 11.03B | not stated in card | UNVERIFIABLE | |
| tier | A | | OK | |

## kimi-k2-thinking

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| summary | interleaves reasoning with tool calls over long horizons | "interleave chain-of-thought reasoning with function calls" | OK | |
| goodAt | Stable over 200-300 sequential tool calls | "stable tool-use across 200–300 sequential calls" | OK | |
| goodAt | SWE-bench Verified 71.3 | 71.3 | OK | |
| goodAt | Native INT4 (QAT) ~2x faster decoding | "QAT ... roughly 2x generation speed improvement" | OK | |
| goodAt | 256K context | "Context Length 256K" | OK | |
| watchOut | 1T total, 32B active | 1T / 32B | OK | |
| watchOut | no non-thinking mode documented | none documented | OK | |
| watchOut | Needs runtime support for its tool-call format | "requires the inference engine to support Kimi-K2's native tool-parsing logic" | OK | |
| bench swe-bench-verified | 71.3 | 71.3 | OK | |
| bench swe-bench-multilingual | 61.1 | 61.1 | OK | |
| bench Terminal-Bench | 47.1 | 47.1, "w/ simulated tools (JSON)", Terminus-2 | OK | consider setting "simulated tools (JSON)" |
| bench livecodebench-v6 | 83.1 | "LiveCodeBenchV6" 83.1 | OK | |
| ollama | none | library page exists but lists "No models" | OK | |
| gguf | 200 | | OK | |
| catalog | other (Modified MIT), 2025-11-04, 1.026T, 32.86B, ctx 262144 | matches card | OK | |
| tier | A | | OK | |

## deepseek-v3.2

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| summary | sparse attention, thinking-with-tools | DSA; "thinking with tools" | OK | |
| goodAt | SWE Verified 73.1 (tech report) | report: DeepSeek-V3.2 Thinking 73.1 | OK | |
| goodAt | Thinking integrated into tool calls | "integrate reasoning into tool-use" | OK | |
| goodAt | Sparse attention, cheaper long context | DSA "optimized for long-context scenarios" | OK | |
| goodAt | MIT | MIT | OK | |
| watchOut | Huge: about 685B parameters | DeepSeek's V4 card lists V3.2-Base as 671B total; 685B is the HF count with MTP | minor mismatch | "about 671B" |
| watchOut | No standard chat template | "does not include a Jinja-format chat template" | OK | |
| watchOut | Report: use non-thinking in Roo Code/Terminus | "Roo Code or Terminus ... we recommend utilizing non-thinking models" | OK | |
| bench swe-bench-verified | 73.1 | 73.1 | OK | |
| bench terminal-bench-2.0 | 46.4 | 46.4 (run in Claude Code; Terminus non-thinking 39.3) | OK | consider setting "Claude Code" |
| bench swe-bench-multilingual | 70.2 | 70.2 | OK | |
| bench tau2-bench | 80.3 | 80.3 | OK | |
| ollama | none | library page lists "No models" | OK | |
| gguf | 200 | | OK | |
| catalog | mit, 2025-12-01 | | OK | |
| catalog activeParams | 51.88B | V4-Flash card table: V3.2-Base "37B" activated | WRONG | override `activeParams: 37000000000` |
| catalog contextLength | 163840 | report: "supports a maximum context length of only 128K" | WRONG | 131072 |
| tier | A | | OK | |

## deepseek-v4-flash

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| summary | Preview MoE, million-token reasoning and agentic | "a preview version"; "one million tokens" | OK | |
| goodAt | 284B MoE with 13B active | "284B parameters (13B activated)" | OK | |
| goodAt | 1M context, efficient hybrid attention | "Hybrid Attention Architecture" CSA+HCA; 1M | OK | |
| goodAt | SWE Verified 79.0 in Think Max | V4-Flash Max 79.0 | OK | |
| goodAt | Three modes: non-think, high, max | "Non-think / Think High / Think Max" | OK | |
| watchOut | Preview release | "preview version" | OK, but superseded | DeepSeek-V4-Flash-0731 (created 2026-07-31) is "the official release ... superseding the preview version"; Ollama `deepseek-v4-flash` describes the official release |
| watchOut | No standard chat template | "does not include a Jinja-format chat template" | OK | |
| watchOut | Think Max needs >=384K context | "context window to at least 384K tokens" | OK | |
| bench swe-bench-verified (Max) | 79.0 | 79.0 | OK | |
| bench terminal-bench-2.0 (Max) | 56.9 | 56.9 | OK | |
| bench swe-bench-pro (Max) | 52.6 | "SWE Pro" Max 52.6 | OK | |
| bench Toolathlon (Max) | 47.8 | 47.8 | OK | |
| catalog bench swe-bench-multilingual (added from HF evals) | 73.3, no setting | 73.3 is the Max column (High 70.2, Non-think 69.7) | incomplete | add to sources with setting "Max" |
| ollama | none | library page lists "No models" | OK | |
| gguf | 200 | | OK | |
| catalog | mit, 2026-04-22, ctx 1048576, active 13B | | OK | |
| catalog params | 290.9B | card "284B" | WRONG vs card | override params 284000000000 if supported |
| tier | A | | OK | |

## Fact-check: Gemma 4, Nemotron 3 / 3.5, Granite 4.2

Checked 2026-11-05 against primary sources, fetched live:

- Model cards: `https://huggingface.co/<repo>/raw/main/README.md` (none of the Gemma repos are gated, so I used Google's own HF card and did not need a mirror).
- `https://huggingface.co/api/models/<repo>` for whether the repo exists, its license tag, createdAt and safetensors totals, plus `?expand[]=evalResults`. The generator uses these eval results to add the extra benchmarks that appear in catalog.json.
- `https://ollama.com/library/<name>/tags` and `/library/<name>`.

Release dates in catalog.json are the HF `createdAt` values (sources.json has no `released` override). Where the card states its own release date, I compared against that.

---

## gemma-4-26b-a4b (card: https://huggingface.co/google/gemma-4-26B-A4B-it)

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| Repo exists / license | google/gemma-4-26B-A4B-it, apache-2.0 | API 200, license:apache-2.0, not gated | OK | |
| summary | MoE for reasoning, coding, agentic workflows on workstations | "well-suited for reasoning, agentic workflows, coding"; 26B A4B targets "consumer GPUs and workstations" | OK | |
| goodAt 1 | 25.2B MoE, 3.8B active: near-4B speed | Total 25.2B, Active 3.8B; "runs almost as fast as a 4B-parameter model" | OK | |
| goodAt 2 | Native function calling | "Native support for structured tool use" | OK | |
| goodAt 3 | 256K context window | Context Length 256K tokens | OK | |
| goodAt 4 | LiveCodeBench v6 77.1%; Apache 2.0 | LiveCodeBench v6: 77.1%; License Apache 2.0 | OK | |
| watchOut 1 | No published agentic coding scores | Card has no SWE-bench or Terminal-Bench rows | OK | (Note: NVIDIA's Lightning card reports third-party Gemma 4 26B scores of SWE-V 57.40 and TB2.1 37.22. They are not on this card.) |
| watchOut 2 | MRCR 128k only 44.1% | MRCR v2 8 needle 128k (average): 44.1% | OK | |
| watchOut 3 | Thinking via <\|think\|> token in system prompt | "including the `<\|think\|>` token at the start of the system prompt" | OK | |
| bench livecodebench-v6 | 77.1 | 77.1% (26B A4B column) | OK | |
| bench tau2-bench (average of 3) | 68.2 | "Tau2 (average over 3)" 68.2% | OK | |
| bench codeforces-elo | 1718 | Codeforces ELO 1718 | OK | |
| Ollama | none listed | `gemma4:26b` (and `gemma4:26b-a4b`) exists: 16–19GB, 256K | UNSUPPORTED (missing tag) | Add `gemma4:26b` |
| GGUF | unsloth/gemma-4-26B-A4B-it-GGUF | API 200, 29 .gguf files, base_model google/gemma-4-26B-A4B-it | OK | |
| catalog license | apache-2.0 | apache-2.0 | OK | |
| catalog released | 2026-03-11 | createdAt 2026-03-11; card states no date | OK | |
| catalog params | 25,805,936,206 | card 25.2B (safetensors total 25.8B includes ~550M vision encoder) | OK (encoder included) | |
| catalog activeParams | 4,395,325,006 (4.4B) | card "Active Parameters 3.8B" | WRONG | Add `"activeParams": 3800000000` override, as E4B/E2B already do |
| catalog contextLength | 262144 | 256K | OK | |
| tier | B | Native tool calling, Tau2 68.2, no agentic coding scores on the card | OK | |

## gemma-4-31b (card: https://huggingface.co/google/gemma-4-31B-it)

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| Repo / license | apache-2.0 | API 200, apache-2.0 | OK | |
| summary | Dense multimodal, reasoning/coding/agentic | Dense 31B, Text+Image; "reasoning, agentic workflows, coding" | OK | |
| goodAt 1 | LCB v6 80.0%, Codeforces 2150 | 80.0%, 2150 | OK | |
| goodAt 2 | Native function calling | "Native support for structured tool use" | OK | |
| goodAt 3 | Tau2 76.9% for tool-using agents | Tau2 (average over 3) 76.9% | OK | |
| goodAt 4 | 256K context; Apache 2.0 | 256K tokens; Apache 2.0 | OK | |
| watchOut 1 | Dense 30.7B: slower than MoE of similar size | Total 30.7B; MoE "excellent choice for fast inference compared to the dense 31B" | OK | |
| watchOut 2 | No published agentic coding scores | none on card | OK | |
| watchOut 3 | Strip past thoughts from history (except tools) | "Thoughts from previous model turns must *not be added*… exception of tool call turns" | OK | |
| bench lcb-v6 / tau2 / codeforces | 80.0 / 76.9 / 2150 | 80.0% / 76.9% / 2150 | OK | |
| Ollama | none listed | `gemma4:31b` exists: 19–20GB, 256K | UNSUPPORTED (missing tag) | Add `gemma4:31b` |
| GGUF | unsloth/gemma-4-31B-it-GGUF | API 200, 30 .gguf | OK | |
| catalog released | 2026-03-11 | createdAt 2026-03-11 | OK | |
| catalog params/active | 31,273,088,876 | card 30.7B (+~550M vision encoder) | OK (encoder included) | |
| catalog contextLength | 262144 | 256K | OK | |
| tier | B | as above | OK | |

## gemma-4-e4b (card: https://huggingface.co/google/gemma-4-E4B-it)

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| summary | On-device multimodal, function calling, audio input | audio on E2B/E4B/12B; mobile/edge target | OK | |
| goodAt 1 | Native function calling | family-wide | OK | |
| goodAt 2 | 4.5B effective, 8B with embeddings | "4.5B effective (8B with embeddings)" | OK | |
| goodAt 3 | Built for local use on laptops and phones | "designed for efficient local execution on laptops and mobile devices" | OK | |
| goodAt 4 | 128K context; Apache 2.0 | 128K; Apache 2.0 | OK | |
| watchOut 1 | No published agentic coding scores | none | OK | |
| watchOut 2 | Tau2 42.2% vs 68.2% for 26B A4B | 42.2% / 68.2% | OK | |
| watchOut 3 | MRCR 128k only 25.4% | 25.4% | OK | |
| bench lcb-v6 / tau2 / cf | 52.0 / 42.2 / 940 | 52.0% / 42.2% / 940 | OK | |
| Ollama `gemma4:e4b` | listed | exists (also `latest`): 6.6–9.5GB, 128K | OK | |
| GGUF | unsloth/gemma-4-E4B-it-GGUF | API 200 | OK | |
| catalog released | 2026-03-02 | createdAt 2026-03-02 | OK | |
| catalog params / active | 7,996,156,490 / 4.5B | 8B with embeddings / 4.5B effective | OK | |
| catalog contextLength | 131072 | 128K | OK | |
| tier | C | Tau2 42.2, no agentic scores | OK | |

## gemma-4-e2b (card: https://huggingface.co/google/gemma-4-E2B-it)

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| summary | Smallest Gemma 4, phones/edge, function calling | E2B is the smallest of five sizes; "mobile and edge devices (E2B, E4B)" | OK | |
| goodAt 1–3 | Native FC; 2.3B effective / 5.1B; 128K; Apache 2.0 | "2.3B effective (5.1B with embeddings)", 128K, Apache 2.0 | OK | |
| watchOut 1–3 | No agentic scores; Tau2 24.5%; MRCR 19.1% | 24.5%; 19.1% | OK | |
| bench lcb-v6 / tau2 / cf | 44.0 / 24.5 / 633 | 44.0% / 24.5% / 633 | OK | |
| Ollama `gemma4:e2b` | listed | exists: 4.6–7.5GB, 128K | OK | |
| GGUF | unsloth/gemma-4-E2B-it-GGUF | API 200 | OK | |
| catalog | 2026-03-02, 5,123,178,051 / 2.3B, 131072 | createdAt 2026-03-02; 5.1B / 2.3B; 128K | OK | |
| tier | C | | OK | |

---

## nemotron-3-nano-30b-a3b (card: https://huggingface.co/nvidia/NVIDIA-Nemotron-3-Nano-30B-A3B-BF16)

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| Repo / license | license other | license_name nvidia-nemotron-open-model-license | OK | |
| summary | Unified reasoning/chat hybrid Mamba-MoE for AI agents | "unified model for both reasoning and non-reasoning tasks"; Mamba2-Transformer Hybrid MoE | OK | |
| goodAt 1 | 30B hybrid Mamba-MoE, ~3.5B active | "3.5B active parameters and 30B parameters in total" | OK | |
| goodAt 2 | Up to 1M; RULER 1M 86.3 | "supports up to a 1M context size"; RULER-100@1M 86.3 | OK | |
| goodAt 3 | BFCL v4 53.8 | BFCL v4 53.8 | OK | |
| goodAt 4 | Reasoning on/off and token budget control | reasoning flag in chat template; "Using Budget Control" / `reasoning_budget` | OK | |
| watchOut 1 | Weak agentic coding: SWE-Bench 38.8 (OpenHands) | "SWE-Bench (OpenHands) 38.8" | OK | |
| watchOut 2 | Needs a custom reasoning parser | vLLM: "Download the custom parser" nano_v3_reasoning_parser.py; SGLang uses built-in nano_v3 | OK (vLLM) | Optional: "vLLM needs a custom reasoning parser" |
| watchOut 3 | NVIDIA Open Model License, not Apache | "NVIDIA Nemotron Open Model License" | OK | |
| bench "SWE-Bench (OpenHands)" | 38.8 (named, unkeyed) | 38.8; card does not say Verified | OK | Right to leave it unkeyed |
| bench "Terminal Bench (hard subset)" | 8.5 | 8.5 | OK | |
| bench bfcl-v4 | 53.8 | 53.8 | OK | |
| bench livecodebench-v6 | 68.3 | "LiveCodeBench (v6 2025-08–2025-05)" 68.3 | OK | Card labels it v6; its date window looks like a typo |
| Ollama | none listed | `nemotron-3-nano:30b` (= `30b-a3b`, `latest`) exists: 18–24GB, 1M | UNSUPPORTED (missing tag) | Add `nemotron-3-nano:30b`. Not `:4b`, which is a different model. |
| GGUF | unsloth/Nemotron-3-Nano-30B-A3B-GGUF | API 200, 24 .gguf, base_model this repo | OK | |
| catalog license | other | NVIDIA Nemotron Open Model License | OK | |
| catalog released | 2025-12-04 | card "Release Date: December 15, 2025"; createdAt 2025-12-04 | WRONG (vs card) | Card's date is 2025-12-15. Needs a `released` override, or document that `released` means HF repo creation |
| catalog params / active | 31,577,937,344 / 3,580,073,408 | 30B / 3.5B (rounded) | OK | |
| catalog contextLength | 262144 | "default context size in the Hugging Face configuration is 256k"; up to 1M | OK | |
| tier | C | SWE-Bench 38.8, TB hard 8.5, BFCL v4 53.8 | OK | |

## nemotron-3.5-lightning-30b-a3b (card: https://huggingface.co/nvidia/NVIDIA-Nemotron-3.5-Lightning-30B-A3B-BF16)

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| Repo / license | license other | license_name openmdw-1.1 ("OpenMDW License Agreement, version 1.1") | OK | Not the Nemotron Open Model License: do not reuse Nano's licence line here |
| summary | Full-precision reference release of 30B-A3B reasoning model, for customization | "full-precision (BF16) release… intended primarily as the starting point for customization" | OK | |
| goodAt 1 | Up to 1M context on bigger setups | single H100 256K; 8×H100 / GB200 / B200 1M | OK | |
| goodAt 2 | PinchBench 85.37 | 85.37 | OK | |
| goodAt 3 | Speculative decoding: MTP, DFlash, DSpark | "two external draft models for speculative decoding as well as MTP" (DFlash, DSpark) | OK | |
| watchOut 1 | This build is for fine-tuning; serve the NVFP4 build | "For latency- and throughput-optimized inference, use the NVFP4 release" | OK | |
| watchOut 2 | Trails Qwen3.6-35B-A3B on every coding row | Coding & Agentic rows: SWE-V 51.56 vs 70.12, Multi 39.33 vs 63.40, TB2.1 24.58 vs 44.38, PinchBench 85.37 vs 88.07, all lower | OK | |
| watchOut 3 | Coding agents need a non-empty-content setting | "For coding agents, add … force_nonempty_content: True" | OK | |
| bench swe-bench-verified | 51.56 | SWE-bench Verified 51.56 (also in evalResults) | OK | |
| bench swe-bench-multilingual | 39.33 | 39.33 | OK | |
| bench terminal-bench-2.1 | 24.58 | Terminal-Bench 2.1 24.58 | OK | |
| bench PinchBench | 85.37 | 85.37 | OK | |
| Ollama | none listed | `nemotron-3.5-lightning:30b` (= `30b-a3b`, `latest`) exists: 19–25GB, 1M | UNSUPPORTED (missing tag) | Add `nemotron-3.5-lightning:30b` |
| GGUF | unsloth/NVIDIA-Nemotron-3.5-Lightning-30B-A3B-GGUF | API 200, 19 .gguf, base_model this repo. (The card itself links ggml-org/…-GGUF.) | OK | |
| catalog released | 2026-08-01 | card "Release Date: August 11, 2026"; createdAt 2026-08-01 | WRONG (vs card) | 2026-08-11 |
| catalog params | 31,577,937,344 | 30B total | OK | |
| catalog activeParams | 3,580,073,408 (3.58B) | "3B active parameters" | WRONG (minor) | Card says 3B. Override `activeParams` or accept the computed figure; Nano's card says 3.5B for the same shape |
| catalog contextLength | 262144 | up to 1M; 256K default on H100 | OK | |
| tier | B | SWE-V 51.56, TB2.1 24.58 | OK | |

## nemotron-3-super-120b-a12b (card: https://huggingface.co/nvidia/NVIDIA-Nemotron-3-Super-120B-A12B-BF16)

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| Repo / license | other | NVIDIA Nemotron Open Model License | OK | |
| summary | Hybrid Mamba LatentMoE for collaborative agents, high-volume agentic work | "optimized for collaborative agents and high-volume workloads"; LatentMoE Mamba-2 hybrid | OK | |
| goodAt 1 | 120B LatentMoE, 12B active, built-in MTP | "120B (12B active)"; "Includes a built-in MTP head" | OK | |
| goodAt 2 | Up to 1M; RULER 1M 91.75 | "Up to 1M tokens"; RULER @ 1M 91.75 | OK | |
| goodAt 3 | SWE-Bench 59.20 in OpenCode harness | "SWE-Bench (OpenCode) 59.20" | OK | |
| goodAt 4 | Documented OpenCode setup | "### OpenCode" section with opencode.json | OK | |
| watchOut 1 | Needs a custom reasoning parser | "For each inference backend, you'll need the custom `super_v3` reasoning parser" | OK | |
| watchOut 2 | Coding agents need non-empty-content setting | "For coding agents add … force_nonempty_content: True" | OK | |
| bench swe-bench-verified (OpenHands) | 60.47 | Table "SWE-Bench (OpenHands) 60.47"; footnote names "SWE Bench Verified (OpenHands)"; evalResults → SWE-bench_Verified, note "OpenHands harness" | OK | |
| bench swe-bench-verified (OpenCode) | 59.20 | Table "SWE-Bench (OpenCode) 59.20". The card never says "Verified" for this row; only evalResults (filed by an HF user citing the card) map it to SWE-bench_Verified | OK, with caveat | See note below |
| bench terminal-bench-2.0 | 31.00 | "Terminal Bench Core 2.0 31.00" (Harbor); evalResults terminal-bench-2.0 = 31 | OK | Card name differs ("Terminal Bench Core 2.0") but it is the same benchmark |
| bench "LiveCodeBench (v5)" | 81.19 | "LiveCodeBench (v5 2024-07↔2024-12) 81.19" | OK | Right to leave it unkeyed (not v6) |
| catalog bench swe-bench-multilingual (added from evalResults) | 45.8 | card "SWE-Bench Multilingual (OpenHands) 45.78"; evalResults has 45.8 | WRONG (rounded) | Add `{"key":"swe-bench-multilingual","setting":"OpenHands","score":"45.78"}` to sources.json so the card's value is used |
| Ollama | none listed | `nemotron-3-super:120b` (= `120b-a12b`, `latest`) exists: 87GB, 256K | UNSUPPORTED (missing tag) | Add `nemotron-3-super:120b` |
| GGUF | unsloth/NVIDIA-Nemotron-3-Super-120B-A12B-GGUF | API 200, 77 .gguf, base_model this repo | OK | |
| catalog released | 2026-03-10 | card "Release Date: March 11, 2026"; createdAt 2026-03-10T18:32Z | WRONG (minor, vs card) | 2026-03-11 |
| catalog params | 123,611,012,096 | 120B total (includes MTP) | OK | |
| catalog activeParams | 15,712,541,696 (15.7B) | "12B active" | WRONG | Override `"activeParams": 12000000000` |
| catalog contextLength | 262144 | up to 1M; vLLM/SGLang default 256k | OK | |
| tier | A | SWE-V 60.47 OpenHands, TB 2.0 31.00, documented OpenCode | OK | |

**Duplicate swe-bench-verified key.** 60.47 is SWE-bench Verified run in the OpenHands harness. The card's footnote names it as Verified. 59.20 is the same row family run in the OpenCode harness, and a third run, Codex 53.73, is omitted. Keeping both entries under one key with different `setting` values represents the source accurately. The table labels both rows "SWE-Bench", and only the footnote and evalResults say Verified. `catalog.Model.Benchmark(key)` returns the first match, so 60.47 (OpenHands) is the number used in cross-model comparisons, and goodAt quotes 59.20. That is consistent, as long as the order in sources.json stays OpenHands first. FillFromEvals correctly skips this key because the eval results hold three values for it.

---

## granite-4.2-30b (card: https://huggingface.co/ibm-granite/granite-4.2-30b)

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| Repo / license | apache-2.0 | apache-2.0 | OK | |
| summary | Dense reasoning model for code, tool calling, agentic workflows, enterprise-focused | "Decoder-only Dense Transformer (Reasoning)"; Best For "Reasoning, Code Generation, Tool Calling, Agentic Workflows"; "designed for enterprise and research use" | OK | |
| goodAt 1 | SWE-bench Verified 57 | SWE Bench Verified 57 | OK | |
| goodAt 2 | BFCL v4 61.39; reasoning-augmented tool calls | BFCL (v4) 61.39; "Reasoning-Augmented Tool Calling" | OK | |
| goodAt 3 | Documented OpenCode, Pi, OpenHands setups | sections "### OpenCode", "### Pi", "### OpenHands" | OK | |
| goodAt 4 | Apache 2.0; 128K native, 512K extended | "Natively Supports 128K (Long-context extension to 512K)" | OK | |
| watchOut 1 | Dense 30B: slower than MoE models of similar size | Card says only "Dense"; it makes no speed comparison | UNSUPPORTED | Reword to a card fact (e.g. "Dense 30B: all weights active per token") or move it to match.Insights |
| watchOut 2 | Best results need a custom reasoning parser | "also works with the built-in `nemotron_v3` parser, but `granite_thinking_parser` provides better formatting" | WRONG (overstated) | Use the 8B's wording: "Custom vLLM parser advised for reasoning output" |
| watchOut 3 | Terminal-Bench 2.1 only 29.24 | 29.24 | OK | |
| bench swe-bench-verified | 57 | 57 | OK | |
| bench swe-bench-pro | 33.29 | 33.29 | OK | |
| bench terminal-bench-2.1 | 29.24 | 29.24 | OK | |
| bench bfcl-v4 | 61.39 | 61.39 | OK | |
| catalog bench swe-bench-multilingual (added) | 41.89 | 41.89 | OK | |
| Ollama | none listed | `granite4.2:30b` exists: 18GB, 128K | UNSUPPORTED (missing tag) | Add `granite4.2:30b` |
| GGUF | ibm-granite/granite-4.2-30b-GGUF | API 200, 16 .gguf, base_model this repo | OK | |
| catalog released | 2026-08-07 | card "Release Date: August 25, 2026"; createdAt 2026-08-07 | WRONG (vs card) | 2026-08-25 |
| catalog params | 29,276,770,304 | 30B | OK | |
| catalog contextLength | 131072 | 128K native (config 131072) | OK | |
| tier | B | SWE-V 57, Pro 33.29, TB2.1 29.24, three harness guides, extra agentic SFT | OK (borderline A) | Judgement call: close to Super (A, 60.47), but not a clear mismatch |

## granite-4.2-8b (card: https://huggingface.co/ibm-granite/granite-4.2-8b)

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| summary | Dense 8B reasoning model, tool calling, documented coding-agent setups | 8B dense; OpenCode/Pi/OpenHands sections | OK | |
| goodAt 1 | SWE-bench Verified 47.67 | 47.67 | OK | |
| goodAt 2 | Reasoning-augmented tool calls; BFCL v4 52.39 | 52.39 | OK | |
| goodAt 3 | Documented OpenCode, Pi, OpenHands | present | OK | |
| goodAt 4 | Apache 2.0; 128K/512K | same | OK | |
| watchOut 1 | TB 2.1 20.56, SWE-bench Pro 19.11 | 20.56; 19.11 | OK | |
| watchOut 2 | Extra agentic SFT phase was 30B only | "For the 30B model, we conducted a second SFT phase…" | OK | |
| watchOut 3 | Custom vLLM parser advised for reasoning output | "`granite_thinking_parser` provides better formatting of reasoning output" | OK | |
| bench verified / pro / tb2.1 / bfcl | 47.67 / 19.11 / 20.56 / 52.39 | same | OK | |
| catalog bench swe-bench-multilingual (added) | 30.78 | 30.78 | OK | |
| Ollama `granite4.2:8b` | listed | exists (= `latest`): 5.3GB, 128K | OK | |
| GGUF | ibm-granite/granite-4.2-8b-GGUF | API 200, 15 .gguf | OK | |
| catalog released | 2026-08-07 | card "August 25, 2026" | WRONG (vs card) | 2026-08-25 |
| catalog params / ctx | 8,791,592,960 / 131072 | 8B / 128K | OK | |
| tier | B | SWE-V 47.67 | OK | |

## granite-4.2-3b (card: https://huggingface.co/ibm-granite/granite-4.2-3b)

| Field | Repo says | Source says | Verdict | Fix |
|---|---|---|---|---|
| summary | Compact 3B reasoning model, tool calling, documented coding-agent setups | 3B dense; OpenCode/Pi/OpenHands sections present | OK | |
| goodAt 1 | BFCL v4 52.41, level with 8B's 52.39 | 52.41 / 52.39 | OK | |
| goodAt 2 | LiveCodeBench v6 69.71 | 69.71 | OK | |
| goodAt 3 | Documented OpenCode, Pi, OpenHands | present | OK | |
| goodAt 4 | Apache 2.0; 128K/512K | same | OK | |
| watchOut 1 | No agentic coding scores: card lists NA | 3B column NA for all four Agentic (Coding) rows | OK | |
| watchOut 2 | τ³-bench average 45.78 vs 58.06 for 8B | τ³-bench (AVG) 45.78 / 58.06 | OK | |
| watchOut 3 | RULER 128K only 55.30 | RULER 128K 55.30 | OK | |
| bench bfcl-v4 / τ³ AVG / lcb-v6 / SciCode | 52.41 / 45.78 / 69.71 / 24.11 | same | OK | |
| Ollama `granite4.2:3b` | listed | exists: 2.2GB, 128K | OK | |
| GGUF | ibm-granite/granite-4.2-3b-GGUF | API 200, 15 .gguf | OK | |
| catalog released | 2026-08-07 | card "August 25, 2026" | WRONG (vs card) | 2026-08-25 |
| catalog params / ctx | 3,659,737,600 / 131072 | 3B / 128K | OK | |
| tier | C | no agentic coding scores, BFCL v4 52.41 | OK | |

---

Nothing in scope was unverifiable. All 10 model repos and all 10 GGUF repos returned 200 from the HF API, and none of the Gemma repos are gated.

## lo0 non-catalogue fact check (2026-11-05)

Format: file:line | claim | source says | verdict | fix

## 1. API prices (internal/cost/prices.go) vs https://platform.claude.com/docs/en/about-claude/pricing

| file:line | claim | source says | verdict | fix |
|---|---|---|---|---|
| prices.go:45 | claude-fable-5-1 10 / 12.50 / 20 / 0.25 / 50 | Fable 5.1 $10 / $12.50 / $20 / $0.25 / $50 | OK | |
| prices.go:46 | claude-mythos-5-1 10 / 12.50 / 20 / 0.25 / 50 | Mythos 5.1 same | OK | |
| prices.go:47 | claude-fable-5 10 / 12.50 / 20 / 1 / 50 | Fable 5 same | OK | |
| prices.go:48 | claude-opus-5-5 4 / 5 / 8 / 0.20 / 20 | Opus 5.5 same | OK | |
| prices.go:49-53 | opus-5, 4-8, 4-7, 4-6, 4-5: 5 / 6.25 / 10 / 0.50 / 25 | same for all five | OK | |
| prices.go:54 | claude-sonnet-5-5 2 / 2.50 / 4 / 0.10 / 10 | same | OK | |
| prices.go:55 | claude-sonnet-5 2 / 2.50 / 4 / 0.20 / 10 | same; footnote 3: $2/$10 is now the standard price (planned rise to $3/$15 on 2026-09-01 cancelled) | OK | |
| prices.go:56-57 | sonnet-4-6, sonnet-4-5: 3 / 3.75 / 6 / 0.30 / 15 | same | OK | |
| prices.go:58 | claude-haiku-5-5 0.10 / 0.125 / 0.20 / 0.01 / 0.50 | Haiku 5.5 (<=100k prompt) same | OK | |
| prices.go:59 | claude-haiku-4-5 1 / 1.25 / 2 / 0.10 / 5 | same | OK | |
| prices.go:64 | longPrompt haiku-5-5 0.50 / 0.625 / 1 / 0.05 / 2.50, >100k | Haiku 5.5 (>100k prompt) same; prompt length counts input + cache reads + cache writes | OK | |
| prices.go:44-60 | (map completeness) | Page also lists Claude Mythos 5 ($10/$12.50/$20/$1/$50, ID `claude-mythos-5`), and retired-but-still-on-partner-clouds Opus 4.1, Opus 4 ($15/$18.75/$30/$1.50/$75), Sonnet 4 ($3/$3.75/$6/$0.30/$15), Haiku 3.5 ($0.80/$1/$1.60/$0.08/$4) | WRONG (omission) | Add `claude-mythos-5`. `RatesFor("claude-mythos-5")` finds nothing because `claude-mythos-5-1` is not a prefix of it. Optionally add opus-4-1, opus-4, sonnet-4, 3-5-haiku for older history |
| prices.go:44-60 | model ID strings | IDs match the official ones: claude-fable-5-1, claude-mythos-5-1, claude-fable-5, claude-opus-5-5, claude-opus-5, claude-opus-4-8/4-7/4-6, claude-opus-4-5 (alias of claude-opus-4-5-20251101), claude-sonnet-5-5, claude-sonnet-5, claude-sonnet-4-6, claude-sonnet-4-5 (alias of -20250929), claude-haiku-5-5, claude-haiku-4-5 (alias of -20251001). Sources: models/overview, models/model-ids-and-versions, per-model overview pages | OK | |
| prices.go:34-36 | fast mode 2x in every category | Fast mode: Opus 5.5 $8/$40 (2x of $4/$20); Opus 5 and Opus 4.8 $10/$50 (2x of $5/$25). Prompt caching multipliers apply on top of fast pricing, so cache rates are also 2x | OK | Offered only on Opus 5.5, Opus 5, Opus 4.8 (Opus 5 and 4.8 in research preview). The code applies 2x wherever the transcript marks fast, which is correct |
| prices.go:37-38 | US-only 1.1x from Claude 4.6 on | "For Claude 4.6 and later models, ... inference_geo ... 1.1x multiplier on all token pricing categories"; also applies to Haiku 5.5 >100k rates | OK | |
| prices.go:41-43 | Claude 4.6+ bill full 1M at standard rates; Haiku 5.5 is the exception | "Claude 4.6 and later models (except Claude Haiku 5.5) and Claude Mythos Preview include the full 1M token context window at standard pricing" | OK | |

## 2. Subscription plans (cost.PlanFor, prices.go:99-118) vs https://claude.com/pricing

| file:line | claim | source says | verdict | fix |
|---|---|---|---|---|
| prices.go:103 | Pro $20/month | Pro: $20 if billed monthly; $17/month with annual billing ($200 up front) | OK (monthly list price) | |
| prices.go:108 | "Max 5x" $100 | Plan is now called "Max", $100/month, "5x Pro usage" | OK price, name outdated | Name it "Max" |
| prices.go:106 | "Max 20x" $200 | Plan is now "Ultra", $200/month, "10x Pro usage" weekly, no 5-hour limit for verified accounts. The pricing page no longer lists "Max 20x". Press reports date the rename to 2026-10-30/31 and say existing Max 20x subscribers moved to Ultra automatically | OK price, name WRONG | Name it "Ultra". Whether an Ultra account's `organizationRateLimitTier` still contains "20x" (scanner.go:103-126) is UNVERIFIABLE without an Ultra account's ~/.claude.json |

## 3. Published llama.cpp results (internal/match/published.go)

Apple: https://github.com/ggml-org/llama.cpp/discussions/4167 (body last updated 2026-10-27, fetched with gh api graphql)

| file:line | claim | source says | verdict |
|---|---|---|---|
| published.go:42 | M5 Ultra 80c 4944.66 / 179.10 | 80c Q4_0 4944.66 / 179.10, commit c1d0e7a | OK |
| published.go:43 | M5 Max 40c 3219.99 / 119.92 | same, c1d0e7a | OK |
| published.go:44 | M5 Pro 16c 1340.44 / 67.59 | same, commit 60081bb | OK |
| published.go:45 | M5 Pro 20c 1620.64 / 66.33 | same, c1d0e7a | OK |
| published.go:46-70 | All 25 M1-M4 rows | Every row matches the Q4_0 PP/TG columns exactly (I checked all of them) | OK |
| published.go:36-38 | M1-M4 from a 2023 build, M5 from 2026 builds | M1-M4 at 8e672ef (2023-11-21). M5 at c1d0e7a (2026-08-25) and 60081bb (2026-09-18) | OK |
| published.go:19-25 | Llama 2 7B Q4_0, 3.56 GiB, 6.74B params, pp512/tg128 | llama-bench output in the discussion: "llama 7B mostly Q4_0, 3.56 GiB, 6.74 B, pp 512 / tg 128" | OK |
| published.go:41-71 | (completeness) | Table also has M1 7c (107.81/14.19), M2 8c (145.91/21.7), M3 10c (186.75/21.34, partial), M5 10c (722.79/31.88, partial, c1d0e7a), M6 8c @170 GB/s (988.93/35.74, c1d0e7a) | UNSUPPORTED (omission): with no base M3, M5 or M6 row, plain M3/M5/M6 Macs get no published result. M1 7c and M2 8c fall back to the 8c/10c rows, which overstates them slightly. Fix: add those rows |

CUDA: https://github.com/ggml-org/llama.cpp/discussions/15013, "CUDA Scoreboard for Llama 2 7B, Q4_0 (no FA)" (body updated 2026-10-16)

| file:line | claim | source says | verdict |
|---|---|---|---|
| published.go:76 | RTX 4060 Ti 3394.63 / 63.86 | same | OK |
| published.go:77 | RTX A6000 4913.93 / 138.73 | same | OK |
| published.go:78 | RTX 5090 14073.41 / 290.02 | same | OK |
| published.go:79 | RTX 5080 8297.36 / 181.99 | same | OK |
| published.go:80 | RTX 4090 11992.70 / 186.21 | same | OK |
| published.go:81 | RTX 4080 8031.64 / 142.49 | same | OK |
| published.go:82 | RTX 3090 5174.69 / 158.16 | same | OK |
| published.go:83 | RTX 3080 5013.86 / 139.65 | same | OK |
| published.go:84 | RTX 3060 2137.50 / 75.57 | same | OK |
| published.go:85 | A100 4849.53 / 190.88 | "A100 80 GB" same | OK |
| published.go:86 | H100 9918.34 / 267.81 | "H100 80 GB" same | OK |
| published.go:111-115 | substring match | The scoreboard has separate rows that the substring match folds into these: "RTX 5090 Laptop" 6667.06/156.49 (gets desktop 14073/290), "RTX 4090 D", "RTX 4080 SUPER", "RTX 3090 Ti". Any "...Laptop GPU" name gets desktop numbers. "NVIDIA RTX A1000" contains "a100", so it gets A100 numbers | WRONG (matching logic). Fix: add the variant rows ahead of the base rows, exclude names with "laptop", and match "A100" on a word boundary |

## 4. Ollama (internal/backend/ollama.go)

| file:line | claim | source says | verdict | fix |
|---|---|---|---|---|
| ollama.go:32-34 | minOllamaAnthropic = 0.14.0 | v0.14.0 release notes (2026-01-10): "Anthropic API compatibility: support for the `/v1/messages` API". No earlier release (I searched every release body) mentions Anthropic or /v1/messages. https://github.com/ollama/ollama/releases/tag/v0.14.0 | OK | Current latest is v0.40.14 (2026-11-03) |
| ollama.go:24-26 | Ollama's default context is far smaller than an agent needs | https://docs.ollama.com/context-length: default depends on VRAM: <24 GiB gives 4k, 24-48 GiB gives 32k, >=48 GiB gives 256k. Docs recommend >=64k for agents and coding tools | Partly WRONG: true below 48 GiB, false at 48 GiB or more (256k) | Reword, e.g. "Ollama's default depends on VRAM and is 4k-32k below 48 GiB". Setting num_ctx explicitly is still right |

## 5. Install commands and URLs

| file:line | claim | source says | verdict |
|---|---|---|---|
| claude/status.go:16 | curl -fsSL https://claude.ai/install.sh \| bash | Same in https://code.claude.com/docs/en/setup (URL resolves to downloads.claude.ai bootstrap.sh) | OK |
| claude/status.go:17 | irm https://claude.ai/install.ps1 \| iex | Same in setup docs | OK |
| claude/session.go:46 | npm install -g @anthropic-ai/claude-code | Documented alternative | OK |
| claude/status.go:27 | `claude --version` prints "2.1.211 (Claude Code)" | Local run prints "2.1.221 (Claude Code)", same format | OK |
| opencode/status.go:16 | curl -fsSL https://opencode.ai/install \| bash | Documented at https://opencode.ai/docs, but that page is now the OpenCode 1 docs. The script redirects to the anomalyco/opencode v1 branch and says it "only installs OpenCode 1". OpenCode 2 (npm @opencode/cli 2.2.5, 2.0.0 released 2026-09-11) installs with `curl -fsSL https://opencode.ai/v2/install \| bash`, and `brew install opencode` and Arch `pacman -S opencode` now install V2 | OK for V1; see the V2 issues in section 8 |
| opencode/status.go:70, plan.go:54 | npm install -g opencode-ai; scoop install opencode | Both listed in the V1 docs (opencode-ai 1.19.5 on npm, 2026-10-28) | OK (V1) |
| opencode/status.go:19-20 | "OpenCode documents no version flag" | https://opencode.ai/docs/cli Global Flags: `--version`, `-v`: "Print version number" | WRONG: read the version with `opencode --version` |
| opencode/plan.go:130, session.go:25 | $schema https://opencode.ai/config.json | Resolves to a JSON schema with $id https://opencode.ai/config.json; used in both the V1 and V2 docs | OK |
| backend/ollama.go:53,59 | https://ollama.com/download | 200 | OK |
| backend/lmstudio.go:41 | https://lmstudio.ai/download | 200 | OK |
| backend/llamacpp.go:58-59 | brew install llama.cpp; GitHub releases | docs/install.md lists Homebrew (Mac/Linux); formula llama.cpp exists; releases page is live. Windows also has `winget install llama.cpp` | OK |

## 6. Ports and the llama.cpp cache

| file:line | claim | source says | verdict |
|---|---|---|---|
| runtimes/ollama.go:18 | 127.0.0.1:11434 | docs.ollama.com/faq: "Ollama binds 127.0.0.1 port 11434 by default" | OK |
| runtimes/lmstudio.go:18 | 127.0.0.1:1234 | LM Studio OpenAI-compat and Anthropic-compat docs use localhost:1234 ("assume the server port is 1234") | OK |
| runtimes/lmstudio.go:22 | ~/.lmstudio/models | Not found in the LM Studio docs pages I fetched | UNVERIFIABLE |
| backend/llamacpp.go:30-33 | lo0 asks llama-server to listen on 127.0.0.1:8080 and sets --port | llama-server's own default port is now 9931 (common/common.h `int32_t port = 9931`; server README "default: 9931"); default host 127.0.0.1. lo0 passes `--port` explicitly, so this works | OK (the comment is accurate). Don't describe 8080 as llama-server's default anywhere |
| runtimes/llamacpp.go:13-27 | llama.cpp model cache: LLAMA_CACHE, then ~/Library/Caches/llama.cpp (macOS), %LOCALAPPDATA%\llama.cpp (Windows), $XDG_CACHE_HOME or ~/.cache/llama.cpp (Linux) | That is `fs_get_cache_directory()` (common/common.cpp:1000), now used only for Docker-registry pulls. Since commit 8c7957ca3 (2026-03-24, "common : add standard Hugging Face cache support", #20775), `-hf` downloads go to the Hugging Face cache in common/hf-cache.cpp. Order: LLAMA_CACHE, then HF_HUB_CACHE, HUGGINGFACE_HUB_CACHE, $HF_HOME/hub, $XDG_CACHE_HOME/huggingface/hub, then ~/.cache/huggingface/hub (%USERPROFILE%\.cache\huggingface\hub on Windows) on every OS. Older manifests are migrated there at startup | WRONG: lo0's own migrate step uses `llama-server -hf` (backend/llamacpp.go:63-66), so the models it downloads land where the detector does not look. Fix: scan the HF hub cache (models--org--repo/snapshots/*/...gguf) with that env order, and keep the legacy dir as a fallback |

## 7. Version report (not flagged)

- Claude Code: VerifiedVersion 2.1.317. npm @anthropic-ai/claude-code: latest = next = 2.1.318, stable = 2.1.311 (registry modified 2026-11-04). One release behind latest; ahead of stable.
- Codex CLI: VerifiedVersion 0.155.0. npm @openai/codex latest = 0.166.0; GitHub release rust-v0.166.0 (2026-10-29). Alpha 0.168.0-alpha.3. 11 minor releases behind.

## 8. Harness feature claims

Claude Code (https://code.claude.com/docs/en/env-vars, /model-config, /hooks, /mcp, /setup)

| file:line | claim | source says | verdict | fix |
|---|---|---|---|---|
| claude/plan.go:68-84 | ANTHROPIC_BASE_URL, ANTHROPIC_AUTH_TOKEN, ANTHROPIC_MODEL, ANTHROPIC_DEFAULT_OPUS/SONNET/HAIKU_MODEL, CLAUDE_CODE_SUBAGENT_MODEL, CLAUDE_CODE_MAX_CONTEXT_TOKENS, CLAUDE_CODE_MAX_OUTPUT_TOKENS, CLAUDE_CODE_ATTRIBUTION_HEADER=0, CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1 | All documented in env-vars with these meanings | OK | |
| claude/plan.go:88-90 | unset ANTHROPIC_API_KEY | ANTHROPIC_API_KEY is sent as X-Api-Key and takes precedence over a subscription | OK | |
| claude/plan.go:63-64 | Claude Code assumes 200k for unknown model names; CLAUDE_CODE_MAX_CONTEXT_TOKENS sets the real window | model-config "Correct the window...": unrecognised IDs are assumed to be 200K; the variable applies directly to IDs that don't start with `claude-` and have no `[1m]` | OK | |
| claude/plan.go:59-62, features.go:60 | "Every model alias resolves to a local model" / "every model alias resolves to the local model" | Aliases include `fable` (and `best`, which uses fable where available). ANTHROPIC_DEFAULT_FABLE_MODEL is documented and is not set by lo0. Unless it is set, `fable` resolves to Fable 5.1 (`claude-fable-5-1`), and "a gateway that isn't configured to serve claude-fable-5-1 rejects requests for that model" | WRONG | Also set ANTHROPIC_DEFAULT_FABLE_MODEL to the main model |
| claude/plan.go:75-76 | "The subagent setting overrides every subagent's own model" | env-vars: CLAUDE_CODE_SUBAGENT_MODEL is the default for subagents "that aren't assigned a model another way. Two sources take precedence over it: a model Claude passes when it spawns the agent, and a model field in the agent's definition, including inherit. To change that, set CLAUDE_CODE_SUBAGENT_MODEL_FORCE" (v2.1.257+) | WRONG (the comment's reasoning is backwards; the variable does not override a haiku subagent) | Fix the comment. The decision may still be fine, but a subagent whose definition names a full Claude model ID (not an alias) still requests that ID from the local runtime |
| claude/features.go:49-52 | Non-command hooks: "the local model judges <type> hooks instead of Claude" | hooks: type is "command", "http", "mcp_tool", "prompt" or "agent". Only prompt and agent hooks use a model (default: the background model). http and mcp_tool hooks run without a model | WRONG for http and mcp_tool | Treat http and mcp_tool like command ("runs as is") |
| claude/features.go:89-91 | claude.ai connectors need the claude.ai sign-in | mcp: connectors are available "if you've logged into Claude Code with a claude.ai account". The docs don't say whether they load when ANTHROPIC_AUTH_TOKEN/BASE_URL is set | UNVERIFIABLE (plausible) | |
| claude/features.go:36-74, 103-106 | instructions, MCP, skills, commands, agents, plugins, permissions, statusLine, outputStyle, env carry over as is | Same binary and config, so these carry over; nothing in the docs contradicts it | OK | |

OpenCode V1 (https://opencode.ai/docs, now labelled OpenCode 1)

| file:line | claim | source says | verdict |
|---|---|---|---|
| opencode/plan.go:62-70 | global config ~/.config/opencode/opencode.json; "follows the XDG base directory convention on every platform" | Docs give ~/.config/opencode/opencode.json. XDG_CONFIG_HOME and Windows behaviour are not documented | Path OK; XDG/every-platform UNSUPPORTED by the docs |
| opencode/plan.go:126 | lo0 can't merge an opencode.json with comments | OpenCode accepts JSON and JSONC (opencode.jsonc). lo0 also ignores an existing opencode.jsonc | OK as stated; gap: an existing opencode.jsonc is not detected or merged |
| opencode/plan.go:132-133, 163-172 | provider entry: npm @ai-sdk/openai-compatible, name, options.baseURL ".../v1", models.{id}.name, limit.context/output; model "provider/model" | providers docs (llama.cpp example) use this exact shape with "llama-server (local)" and baseURL http://127.0.0.1:8080/v1 | OK |
| opencode/plan.go:224-241, features.go:94-98 | mcp: type local, command array, environment, enabled; type remote, url, headers | mcp-servers docs match | OK |
| features.go:96 | OpenCode does not document SSE | No mention of SSE in mcp-servers docs | OK |
| convert.go:17-23 | {env:VAR} substitution | config docs: "Use {env:VARIABLE_NAME}" | OK |
| plan.go:142-152 | "instructions" array | rules/config docs | OK |
| features.go:69-74 | AGENTS.md read; ~/.claude/CLAUDE.md used when no ~/.config/opencode/AGENTS.md; project CLAUDE.md used when no AGENTS.md | rules docs, "Claude Code Compatibility" | OK for V1 |
| features.go:152, convert.go:72-79 | commands in .opencode/commands and ~/.config/opencode/commands; $ARGUMENTS | commands docs | OK |
| features.go:155, convert.go:84-115 | agents in agents/ with mode: subagent; permission edit/bash/webfetch: deny | agents docs show `mode: subagent` and `permission: edit: deny, bash: deny`; permissions docs list edit (covers write, patch), bash, webfetch | OK |
| features.go:161-163 | plugin skills copied to ~/.config/opencode/skills; .claude/skills read as is | skills docs: .claude/skills and ~/.claude/skills are searched, and so is ~/.config/opencode/skills | OK |
| features.go:34-39, 204 | plugin hooks tool.execute.before/after, events session.created, session.idle, permission.asked, experimental.session.compacting, shell.env | plugins docs list all of these | OK for V1 |
| features.go:195 | permission: last matching rule wins | permissions docs: "the last matching rule winning" | OK |
| session.go:9-13 | OPENCODE_CONFIG_CONTENT inline config merged over global/project | config docs: inline config (OPENCODE_CONFIG_CONTENT) is loaded after project config; configs merge key by key | OK for V1 |
| features.go:202-203 | no custom status line, no output styles | Not documented in V1 docs | UNVERIFIABLE (absence) |

OpenCode 2 (https://opencode.ai/v2/docs/, migrate guide https://opencode.ai/v2/docs/migrate-v1/)

| file:line | claim | source says | verdict | fix |
|---|---|---|---|---|
| opencode/* (whole harness) | Targets one "OpenCode" | OpenCode 2 is out (npm @opencode/cli, 2.0.0 on 2026-09-11, now 2.2.5) and uses the same `opencode` command. `brew install opencode` and Arch `pacman -S opencode` install V2. V2 reads V1 config from the same locations | UNSUPPORTED | Detect the major version with `opencode --version` and branch the V2-sensitive claims below |
| features.go:72-74 | CLAUDE.md fallbacks read natively | V2: "V2 currently only discovers AGENTS.md"; instructions page: "It does not use CLAUDE.md" | WRONG on V2 | On V2, translate CLAUDE.md to instructions |
| features.go:34-39, 204 | hooks become plugin hooks | V2: "V1 plugin implementations do not run in V2" (new plugin API) | WRONG on V2 | |
| session.go:13 | OPENCODE_CONFIG_CONTENT | The V2 docs I fetched don't mention it; only OPENCODE_CLI_CONFIG_CONTENT, for cli.json | UNVERIFIABLE on V2 | |
| features.go:161-163 | skills | V2 still searches ~/.claude/skills and .claude/skills | OK on V2 | |
