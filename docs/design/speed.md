# Speed from evidence

Status: built, 5 November 2026. Every speed comes from the best evidence lo0 has and says which: measured (`lo0 benchmark`, or `b` on Models), this machine's calibration, llama.cpp's published results for the chip, or bandwidth alone. Reading time is part of every typical reply, and with Ollama only the 16-bit cache it really uses is planned. `TestPredictionsMatchMeasurements` holds the model to the measurements below. It replaces phase 6 ("Benchmark") of [hub.md](hub.md).

## What is wrong now

lo0 estimates writing speed with one formula on every machine:

```
tok/s = 0.55 × memory bandwidth ÷ (active weight bytes + KV cache bytes at the context)
```

The 0.55 is a single guess at how much of its peak bandwidth a runtime reaches, and reading speed is not estimated at all. Measured on this machine, a 64 GiB M5 Pro, with Ollama 0.34.4's own timings and a fresh prompt each time so nothing was cached:

| Qwen3.6-35B-A3B, Q4_K_M | 7k context | 38k | 113k |
|---|---:|---:|---:|
| Writing, measured | 73 tok/s | 55 tok/s | 31 tok/s |
| Writing, lo0's estimate | 50 tok/s | 42 tok/s | 30 tok/s |
| Reading, measured | 1,423 tok/s | 774 tok/s | 294 tok/s |
| Time to read that context cold | 5 s | 49 s | 6 min 24 s |

| Qwen3-Coder-30B-A3B, Q4_K_M | 7k context | 38k | 113k |
|---|---:|---:|---:|
| Writing, measured | 56 tok/s | 22 tok/s | did not finish |
| Writing, lo0's estimate | 70 tok/s | 42 tok/s | 21 tok/s |
| Reading, measured | 725 tok/s | 199 tok/s | |
| Time to read that context cold | 9 s | 3 min 9 s | over 20 min |

Loaded with its 256k window it took 41.9 GiB, where lo0 estimated 31.5 GiB.

The two models differ most where lo0's formula cannot see: Qwen3.6 keeps a full cache in only some of its layers and slows gently with context; Qwen3-Coder keeps one in every layer and becomes unusable for reading past about 40k on this machine.

Four things follow.

1. **The efficiency is too low for this chip.** At 7k the measured speed implies about 80% of the M5 Pro's 307 GB/s, not 55%. llama.cpp's published Apple Silicon results agree: Llama 2 7B at Q4_0 writes 66–67 tok/s on an M5 Pro, about 84% of its bandwidth, and 50 tok/s on an M4 Pro, about 70%. One constant cannot be right for both.
2. **The slowdown with context is steeper than modelled, and depends on the model.** Qwen3.6's measured writing falls 57% from 7k to 113k against an estimated 39%; Qwen3-Coder's falls 61% by 38k against an estimated 40%. For Qwen3.6 the two errors cancel near 100k, which is why its long-context figure looked right.
3. **Reading is the larger cost and lo0 shows none of it.** It falls from 1,423 to 294 tok/s as the context grows. A local runtime keeps what it has read, so within a session only new tokens are read, but at 100k a 5k-token tool result takes about 17 s to read before the reply starts. The "typical reply takes 17s" lo0 shows today counts writing only.

4. **lo0 estimates a setup the runtime does not run.** For Qwen3-Coder-30B-A3B lo0 recommends an 8-bit KV cache, and its memory and speed figures assume one. Ollama sets the cache type for the whole server (`OLLAMA_KV_CACHE_TYPE`, which also needs flash attention), lo0 sets neither, and nothing on this machine does, so Ollama runs the model with a 16-bit cache. Measured, it writes 56 tok/s at 7k against lo0's 70. An estimate must describe what the runtime will do: lo0 either sets the cache type when it prepares the model, which changes the user's Ollama configuration and so is a step in the plan, or plans only the cache the runtime is set to use.

## The rule

Every speed has a source, shown beside it, and lo0 uses the best source it has:

| | Source | Shown as |
|---|---|---|
| 1 | Measured on this machine for this model and build: a benchmark, or the user's own sessions with it, which lo0 already times reply by reply | measured |
| 2 | Estimated with this machine's own calibration, fitted from any model measured on it | estimated from this machine |
| 3 | Estimated with published results for this chip or card | estimated from llama.cpp's M5 Pro results |
| 4 | Estimated from the chip's published bandwidth alone | rough estimate |

A figure from a lower rung is never shown as if it came from a higher one. "Measured" means measured on this machine, with this model, this quantization and this runtime.

## The model behind the estimates

Writing is bound by memory reads; reading by compute. Both slow with context. lo0 models each as a cost per token that grows with the context:

```
seconds per written token = weights ÷ (efficiency × bandwidth) + context × write slope
seconds per read token    = active parameters ÷ read rate      + context × read slope
```

The coefficients are what the evidence sets:

- **Efficiency and read rate** per chip, from published results. llama.cpp's Apple Silicon table ([discussion 4167](https://github.com/ggml-org/llama.cpp/discussions/4167)) and CUDA table ([discussion 15013](https://github.com/ggml-org/llama.cpp/discussions/15013)) give reading (pp512) and writing (tg128) speed for one reference model on dozens of chips. lo0 keeps the figures it uses in a data file with the URL and the date they were taken, like API prices in `internal/cost/prices.go`, and links rather than copies the pages. Neither table measures context length, so they set the base rates, not the slopes.
- **The slopes** come from the model's architecture (KV bytes per token, how many layers keep a full cache), scaled by what this machine measured. A model with a hybrid or sliding-window attention design slows less with context than a model whose every layer keeps a full cache, and the catalogue already records which is which.
- **This machine's calibration** replaces the published figures as soon as anything is measured here. Three measurements at different context sizes fit both the base and the slope; one fits the base.

## Experts in system memory

On a discrete GPU, a mixture-of-experts model that does not fit can keep the experts of some layers in system memory while attention, the shared weights and the KV cache stay on the card (llama.cpp's `--n-cpu-moe`). lo0 plans this only where it gains something, only with llama.cpp, and only within the share of system memory a machine without a GPU gives models. For each build it tries every number of layers and keeps the one that does the user's work best.

The curve for the whole model on the GPU is adjusted in two places:

```
writing: the routed experts a token reads from system memory move from the GPU's cost
         to bytes ÷ (0.48 × 50 GB/s)
reading: + offloaded expert bytes ÷ (16 GB/s × 512 tokens), the copy to the GPU per batch
```

lo0 does not read the memory's speed or the link's width, so both assumptions sit below current desktops: dual-channel DDR4-3200 and the peak of PCIe 4.0 x8. Checked against a published run, Qwen3.6-35B-A3B at Q4_K_M on an RTX 5080 with 15 layers' experts in DDR5-5600, which wrote 79 tok/s at 4k: lo0 estimates 69. Measurements from `lo0 benchmark` are taken in Ollama with the whole model on the GPU, so they calibrate the GPU's part and never stand in for an offloaded build.

Because reading slows far more than writing, ranking counts a typical reply's reading as well as its writing: 1,292 new tokens read and 568 written, the medians in the history lo0 was developed against.

## Where measurements come from

- **Benchmark (`b`).** For a model that is downloaded: one load, then a fresh prompt at 8k, 32k and half the user's median context, each reading cold and writing 300 tokens, from the runtime's own timings. About 1 minute at 8k and 32k together, and 1 to 6 minutes at the median depending on the model; the plan says so before it starts. Results are kept per model, quantization, runtime and machine, with the date.
- **Safeguards.** A measurement is refused when the runtime read less than 90% of the prompt (Ollama drops part of one that overflows its window rather than failing), when the system moved more than 256 MiB to swap while it ran, or when the three points do not follow one curve within 25%. The second rule came from a real run: with swap nearly full, Qwen3.6 wrote 12.9 tok/s at 98k where an undisturbed run wrote 31 at 113k, and a curve fitted through it claimed 158 tok/s at 8k.
- **The user's sessions.** lo0 already times every reply in Claude Code's logs. Sessions with a model lo0 set up give measured writing speed at the context those sessions had, and the session report already shows it. These count as rung 1 for that model.
- **Published tables.** Shipped with lo0 and updated with it, never fetched at run time.

## What the user sees

The speed column on Models carries its source. In the detail panel:

```
SPEED ON THIS MACHINE                           measured 5 Nov · Ollama 0.34.4
Writing     73 tok/s at 7k   55 at 38k   31 at 113k
Reading      1,423 tok/s at 7k   774 at 38k   294 at 113k
A reply     at 113k into a session: reading a 5k tool result 17 s, writing 565 tokens 18 s
            Opus 5.5 in your sessions: 6 s
```

The typical reply is now both halves: the new tokens a reply reads in a session at the user's median context, and the reply it writes. That is the wait a user feels, and it is the figure the pace bands use.

## What changes in the code

- `catalog`: a `speeds.json` of published per-chip results, generated like the catalogue, with source URLs and the date taken.
- `match`: the speed model above, returning a figure and its source; `Pace` uses the full reply time.
- `migrate` or a new `bench` package: the benchmark as a plan with one runtime step per context size.
- `app`: measurements kept in the state directory, read into the assessment so `match` can use them.
- `report` and the TUI: the source beside every speed, and reading time wherever writing time is shown.

## Open questions

- How many published chips to ship. Proposed: every Apple M-series row in discussion 4167 and the twenty most common NVIDIA cards in 15013.
- Whether to run the benchmark on its own after a download. Proposed: offer it in the download plan's last step, default on, since it takes a few minutes and makes every later figure for that model measured.
