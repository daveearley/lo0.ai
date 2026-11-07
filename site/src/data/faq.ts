/**
 * Questions people ask before they self-host a coding model. The home page
 * shows them and publishes them as FAQPage structured data, so the wording
 * lives here once. Answers are plain text; `code` spans are marked with
 * backticks.
 */
import catalog from '../../../internal/catalog/catalog.json';

export const faq: { q: string; a: string }[] = [
  {
    q: 'Can Claude Code run on a local model?',
    a: 'Yes. Claude Code sends its requests to whatever Anthropic-compatible address `ANTHROPIC_BASE_URL` names, and Ollama serves one. lo0 creates the model in Ollama with the context window your sessions need. `lo0 try` starts Claude Code on it for one session, and `lo0 migrate` adds a `claude-local` command that always does, while `claude` keeps using Claude. Your instructions, MCP servers, hooks and skills keep working. Tools only the Claude apps provide, such as their built-in browser, do not.',
  },
  {
    q: 'Which local model is best for coding on my machine?',
    a: `The best one is the strongest model that fits your memory with room left for the context your sessions reach. lo0 knows ${catalog.models.length} open-weight models built for agentic coding or tool calling, including Qwen, GLM, Devstral, Gemma, gpt-oss, Nemotron, DeepSeek, Kimi and MiniMax. For each it works out the memory it needs at your context length, the share of your past sessions its window would have held and how fast it writes on your chip, then ranks them.`,
  },
  {
    q: 'How much memory do I need to self-host a coding model?',
    a: 'Weights and the KV cache both have to fit in the memory the GPU can use. On Apple Silicon, macOS lets the GPU wire about two thirds of unified memory, or three quarters above 36 GiB; on a PC it is your VRAM, plus system RAM for the experts of a mixture-of-experts model, which llama.cpp can keep there. Without a GPU, the model runs on the CPU from system memory. Qwen3.6-35B-A3B at Q4_K_M with a 256k window takes 28.9 GiB: 22.3 GiB of weights, 5.1 GiB of KV cache and 1.5 GiB for the runtime. lo0 shows the same sum for every model on your machine, and how much more the ones that do not fit would need.',
  },
  {
    q: 'Is a local model as fast as Claude or GPT?',
    a: 'Usually not, and lo0 says by how much before you switch. On an Apple M5 Pro, Qwen3.6-35B-A3B writes about 33 tokens a second, so a typical reply that takes 8 seconds with Opus 5.5 takes about 45. lo0 estimates speed from llama.cpp results for your chip, and `lo0 benchmark` measures a downloaded model on your own machine.',
  },
  {
    q: 'Does lo0 send my code or prompts anywhere?',
    a: 'No. lo0 reads your history on your machine and keeps counts and names, never prompt text, model output, file contents or secret values. It goes online to check for updates and to download what you confirmed in a plan, from the sources the plan shows. No request carries anything about you, and none goes to a lo0 server. `--offline` skips every lookup that only adds information.',
  },
  {
    q: 'What does lo0 change on my machine?',
    a: 'The assessment only reads. Trying or keeping a model is shown as a plan first, with every download and its size, every file and every command, and runs only when you confirm it. `lo0 try` changes nothing in your harness. `lo0 migrate` backs up every file it replaces and records each change, so `lo0 migrate --undo` puts them back.',
  },
  {
    q: 'Which runtimes and coding agents does lo0 support?',
    a: 'Runtimes: Ollama, set up end to end, plus LM Studio and llama.cpp with guided steps. Harnesses: Claude Code, OpenCode and Codex, each on a local model. History: Claude Code and Codex session logs. It runs on macOS, Linux and Windows, and detects Apple Silicon, NVIDIA and AMD GPUs. Machines without a GPU are assessed for models that run on the CPU.',
  },
  {
    q: 'What would a new GPU or Mac run?',
    a: 'Ask lo0 before you buy. `lo0 match --gpu "RTX 4090"` assesses your own sessions and setup on that card, with `--vram`, `--gpus` and `--ram` for other configurations, and `lo0 match --mac "M4 Max" --memory 128` does the same for a Mac. `lo0 ui` and `lo0 web` take the same flags. Speeds are estimated from published llama.cpp results for that hardware, and lo0 sets nothing up for a machine that is not there.',
  },
  {
    q: 'Is lo0 free?',
    a: 'Yes. lo0 is free software under the GNU AGPL v3.0. There is no account, no subscription and no telemetry.',
  },
];

/** The answer with backticked spans as plain text, for structured data. */
export const plain = (s: string) => s.replaceAll('`', '');
