/**
 * The local AI glossary at /glossary. Each term's body is plain text with
 * two marks: `code`, and [[id]] or [[id|words]] for a link to another term.
 * Figures with numbers in them come from the catalogue, never from here.
 */

import { model, billions, statedActive, kvPerToken, KiB, GiB } from './models';

export type Figure = 'experts' | 'quants' | 'context' | 'speed';

export interface Term {
  id: string;
  term: string;
  /** Other names for the same thing; search matches them too. */
  aka?: string[];
  /** One line, the answer someone searching the term wants. */
  short: string;
  body: string;
  /** Where the term turns up: flags, file names, settings. */
  seen?: string[];
  figure?: Figure;
}

export interface Section {
  id: string;
  title: string;
  intro: string;
  terms: Term[];
}

const coder = model('qwen3-coder-30b-a3b');
const devstral = model('devstral-small-2-24b');
const gemma = model('gemma-4-26b-a4b');
const gptoss = model('gpt-oss-20b');
type Sliding = { fullLayers: number; slidingLayers: number; window: number };
const gemmaAttn = gemma.attention as Sliding;
const gptossAttn = gptoss.attention as Sliding;
const kib = (b: number) => `${Math.round(b / KiB)} KiB`;
const gib = (b: number) => `${(b / GiB).toFixed(1)} GiB`;
const spell = (n: number) => ['none', 'one', 'two', 'three', 'four', 'five', 'six', 'seven', 'eight', 'nine'][n] ?? String(n);

export const sections: Section[] = [
  {
    id: 'models',
    title: 'The model',
    intro: 'What the numbers and words in a model’s name say about it.',
    terms: [
      {
        id: 'parameters',
        term: 'Parameters',
        aka: ['weights', 'B', 'params'],
        short: 'The numbers a model is made of. “30B” means 30 billion of them.',
        body:
          'Training sets the parameters; running the model multiplies your input through them. The count is the first guide to both quality and cost: every parameter has to sit in memory, so it decides whether a model fits on your machine before anything else does. “Weights” is the same thing, usually meaning the file that holds them.',
        seen: ['7B', '30B', '480B'],
      },
      {
        id: 'open-weight',
        term: 'Open-weight',
        aka: ['open weights', 'open model'],
        short: 'The lab publishes the trained weights, so you can download and run the model yourself.',
        body:
          'Open-weight is not the same as open source. You get the finished model, usually under a licence that says what you may do with it, but rarely the training data or code. For running locally, the weights are what matters. See [[licence]].',
      },
      {
        id: 'dense',
        term: 'Dense model',
        short: 'A model that uses every parameter for every token.',
        body:
          'Dense models are the simple case: a 24B dense model reads all 24 billion weights to write each token. That makes them slow for their size, because writing speed is limited by how fast those weights come out of memory. See [[memory-bandwidth]].',
      },
      {
        id: 'moe',
        term: 'Mixture of experts',
        aka: ['MoE', 'experts', 'sparse model', 'router'],
        short: 'A model split into many small “experts”, of which only a few run for each token.',
        body: `A small router picks which experts handle each token, layer by layer. The model needs memory for all of them but only reads the chosen few, so it writes at the speed of a much smaller model. ${coder.name} stores ${billions(coder.params)} parameters and uses about ${billions(statedActive(coder))} per token. Experts do not specialise in subjects such as maths or Python; the router learns its own split during training.`,
        figure: 'experts',
      },
      {
        id: 'active-parameters',
        term: 'Active parameters',
        aka: ['A3B', 'A35B', 'activated parameters'],
        short: 'How many parameters a mixture-of-experts model uses per token. The “A3B” in a name.',
        body:
          '“30B-A3B” reads as 30 billion in total, 3 billion active. The two numbers answer different questions: the total sets how much [[unified-memory|memory]] the model needs, the active count sets how fast it writes. A model with few active parameters is fast, but it has less to think with on each token than a dense model of the same total size.',
        seen: ['30B-A3B', '480B-A35B', '120B-A12B'],
      },
      {
        id: 'instruct',
        term: 'Instruct',
        aka: ['base model', 'chat model', 'it'],
        short: 'The version tuned to follow instructions and hold a conversation.',
        body:
          'A lab first trains a base model, which only continues text. It then tunes that into an instruct model that answers questions, follows a [[chat-template]] and calls [[tool-calling|tools]]. For a coding agent you always want the instruct version. Gemma names it `-it`.',
        seen: ['-Instruct', '-it', '-Base'],
      },
      {
        id: 'fine-tune',
        term: 'Fine-tune',
        aka: ['fine-tuning', 'post-training', 'LoRA'],
        short: 'Further training of an existing model for one kind of work.',
        body:
          'A coder model is usually a general model fine-tuned on code and agent runs. LoRA is a cheap way to fine-tune that trains a small add-on instead of every weight. Community fine-tunes on Hugging Face vary a lot in quality; the lab’s own releases are the safe default.',
        seen: ['-Coder', 'LoRA', 'adapter'],
      },
      {
        id: 'distillation',
        term: 'Distillation',
        aka: ['distilled'],
        short: 'Training a small model to imitate a larger one.',
        body:
          'The small model learns from the large model’s outputs instead of only from raw text, so it does better than its size suggests. A name like “R1-Distill-Qwen-32B” means a Qwen model taught by R1. It does not mean a shrunken copy of R1.',
      },
      {
        id: 'multimodal',
        term: 'Multimodal',
        aka: ['vision-language', 'VLM', 'vision'],
        short: 'The model also takes images, and sometimes audio or video.',
        body:
          'For coding it means you can paste a screenshot of a broken layout. The vision part adds a little to the file size. Some runtimes need a separate projector file, often named `mmproj`, to use it.',
        seen: ['mmproj', '-VL'],
      },
      {
        id: 'model-card',
        term: 'Model card',
        aka: ['card', 'lab'],
        short: 'The lab’s page for a model: what it is for, its scores, its settings and its licence.',
        body:
          'On Hugging Face it is the README of the model’s repository. Read it for the recommended [[sampling|sampling settings]], the context length and how [[tool-calling]] is set up. Those details decide whether the model works in your setup.',
      },
      {
        id: 'frontier',
        term: 'Frontier model',
        short: 'The strongest models available at the moment, usually only through an API.',
        body:
          'Claude, GPT and Gemini at their top tiers. Open-weight models trail them, but the gap on coding has narrowed. A model whose card claims frontier-level scores is worth checking on your own tasks before you believe it.',
      },
    ],
  },
  {
    id: 'context',
    title: 'Context',
    intro: 'How much a model can see at once, and what that costs.',
    terms: [
      {
        id: 'token',
        term: 'Token',
        aka: ['tokens', 'tokenizer', 'tokeniser'],
        short: 'The unit models read and write. About three quarters of an English word.',
        body:
          'A [[tokenizer]] splits text into common chunks: “parse_args” might be three tokens. Code and non-English text take more tokens per word than prose. Context limits, speeds and API prices are all counted in tokens.',
      },
      {
        id: 'context-window',
        term: 'Context window',
        aka: ['context length', 'ctx', 'num_ctx', 'n_ctx'],
        short: 'The most tokens a model can see at once: instructions, files, tool output and the conversation.',
        body:
          'A coding agent fills it fast. Its system prompt and tool definitions take 10–20k tokens before you type anything, and every file it reads stays in context. Some runtimes, Ollama among them, load a far smaller window by default and silently drop whatever does not fit, so check the setting. The window you can afford is limited by the [[kv-cache]], not only by the model.',
        seen: ['num_ctx', '-c 65536', '--ctx-size', '256k'],
      },
      {
        id: 'yarn',
        term: 'YaRN',
        aka: ['RoPE scaling', 'native context', 'extended context', 'rope'],
        short: 'A way to stretch a model’s context window past what it was trained on.',
        body:
          'Most models encode token positions with RoPE. YaRN rescales it so that, for example, a model trained on 256k can run at 1M. Cards list the native length and the extended one separately for a reason: quality past the native length is lower, and some labs warn that turning YaRN on hurts short prompts. Turn it on only when you need the room.',
        seen: ['rope_scaling', '--rope-scaling yarn'],
      },
      {
        id: 'attention',
        term: 'Attention',
        aka: ['GQA', 'grouped-query attention', 'full attention', 'heads'],
        short: 'The part of a model that lets each token look back at every earlier token.',
        body:
          'Attention is why a model can use what it read 50k tokens ago, and why long context is expensive: every new token looks at everything before it. Grouped-query attention (GQA) shares one set of keys and values between several heads, which typically shrinks the [[kv-cache]] four to eight times. Most current models use it.',
      },
      {
        id: 'kv-cache',
        term: 'KV cache',
        aka: ['key-value cache', 'context memory', 'cache-type-k'],
        short: 'Memory a model uses to hold the context it has already read. It grows with every token.',
        body: `Without it the model would reread the whole conversation for every token it writes. The cost per token depends on the model’s [[attention]] design: ${devstral.name} keeps ${kib(kvPerToken(devstral))} per token, ${coder.name} ${kib(kvPerToken(coder))}. At 256k tokens that is ${gib(kvPerToken(devstral) * 262144)} and ${gib(kvPerToken(coder) * 262144)}, on top of the weights.`,
        seen: ['--cache-type-k q8_0', 'OLLAMA_KV_CACHE_TYPE'],
        figure: 'context',
      },
      {
        id: 'kv-quantisation',
        term: 'KV cache quantisation',
        aka: ['q8_0 cache', '8-bit cache', 'cache-type-v'],
        short: 'Storing the KV cache at lower precision to fit more context in the same memory.',
        body:
          'An 8-bit cache (`q8_0`) halves the cache with little loss. A 4-bit cache saves more and loses more, so test it on your own work before relying on it. In llama.cpp a quantised V cache needs [[flash-attention]].',
        seen: ['--cache-type-k q8_0', '--cache-type-v q8_0', 'OLLAMA_KV_CACHE_TYPE=q8_0'],
      },
      {
        id: 'sliding-window',
        term: 'Sliding-window attention',
        aka: ['SWA', 'local attention', 'interleaved attention'],
        short: 'Layers that only look at a fixed window of recent tokens, so their cache stops growing.',
        body: `${gemma.name.split('-').slice(0, 2).join(' ')} looks back ${gemmaAttn.window.toLocaleString('en-GB')} tokens in its sliding layers and keeps one full-attention layer for every ${spell(gemmaAttn.slidingLayers / gemmaAttn.fullLayers)} sliding ones; gpt-oss uses a ${gptossAttn.window}-token window and alternates the two. The sliding layers keep a fixed-size cache however long the session runs, and the full layers keep the long-range view. The more sliding layers, the smaller the [[kv-cache]] at long context.`,
      },
      {
        id: 'mla',
        term: 'Multi-head latent attention',
        aka: ['MLA'],
        short: 'DeepSeek’s way of compressing keys and values before caching them.',
        body:
          'MLA stores one small compressed vector per token and layer instead of full keys and values. DeepSeek, Kimi and some GLM models (GLM-4.7-Flash and GLM-5) use it. Its cache per token is far smaller than standard attention’s, and smaller than most [[attention|GQA]] designs.',
      },
      {
        id: 'hybrid-attention',
        term: 'Hybrid attention',
        aka: ['Mamba', 'DeltaNet', 'linear attention', 'state space model', 'SSM', 'recurrent state'],
        short: 'Models that replace most attention layers with layers whose memory does not grow with context.',
        body:
          'Mamba (a state space model) and Gated DeltaNet (a form of linear attention) keep a fixed-size state instead of a cache of every token. Nemotron 3 uses Mamba layers, and Qwen3-Next and Qwen3.5 onwards use Gated DeltaNet, each with a few full-attention layers for exact recall. The [[kv-cache]] shrinks to a fraction, and speed holds up better as the context fills.',
      },
      {
        id: 'sparse-attention',
        term: 'Sparse attention',
        aka: ['DSA'],
        short: 'Attention that only reads the earlier tokens most likely to matter.',
        body:
          'A cheap scorer picks a subset of the context for each new token. The cache stays about the same size, but reading it costs less, so long-context replies come faster. DeepSeek-V3.2 is the best-known example.',
      },
      {
        id: 'compaction',
        term: 'Compaction',
        aka: ['auto-compact', 'compact', 'summarisation'],
        short: 'An agent summarising the conversation so far to free up context.',
        body:
          'When a session nears the [[context-window]] limit, Claude Code and other [[harness|harnesses]] replace the history with a summary and carry on. It works, but detail is lost. A model with a smaller window compacts more often, so the context your sessions actually reach is a good measure of the window you need.',
        seen: ['/compact'],
      },
    ],
  },
  {
    id: 'memory',
    title: 'Memory and files',
    intro: 'Whether a model fits, and the files it comes in.',
    terms: [
      {
        id: 'vram',
        term: 'VRAM',
        aka: ['GPU memory', 'video memory'],
        short: 'Memory on a graphics card. A model runs fast only if it fits there.',
        body:
          'The weights, the [[kv-cache]] and some working space all have to fit. Memory on two cards adds up, though splitting a model across them costs some speed. Whatever does not fit goes to system RAM through [[offloading]], and speed drops sharply.',
      },
      {
        id: 'unified-memory',
        term: 'Unified memory',
        aka: ['Apple Silicon memory', 'wired memory', 'iogpu.wired_limit_mb'],
        short: 'Apple Silicon’s single pool of memory shared by the CPU and GPU.',
        body:
          'It is why a Mac with 64 GB can run models no 24 GB graphics card can hold. By default macOS lets the GPU use only part of it, roughly two thirds to three quarters. The `iogpu.wired_limit_mb` setting raises the limit, leaving less memory for everything else.',
        seen: ['sudo sysctl iogpu.wired_limit_mb=…'],
      },
      {
        id: 'quantisation',
        term: 'Quantisation',
        aka: ['quantization', 'quant', 'quantized', 'quantised'],
        short: 'Storing weights with fewer bits each, so the model is smaller and faster.',
        body:
          'Most models are released at 16 bits per weight. Quantising to around 4 bits makes the file a little under a third of the size, with a small loss in quality. Agent work shows the loss sooner, because one wrong character breaks a [[tool-calling|tool call]]. Very large models cope with low bit counts better than small ones. Quantisation shrinks the weights; it does nothing for the [[kv-cache]].',
        figure: 'quants',
      },
      {
        id: 'k-quants',
        term: 'K-quants',
        aka: ['Q4_K_M', 'Q5_K_M', 'Q6_K', 'Q8_0', 'Q3_K_M', 'Q2_K', 'Q4_0', 'quant names'],
        short: 'llama.cpp’s quantisation formats, named by bits and mix: Q4_K_M averages just under 5 bits a weight.',
        body:
          'The number is the rough bit count. `K` marks the k-quant method: weights sit in blocks of 256, split into smaller groups whose scales are themselves stored at low precision. `S`, `M` and `L` are small, medium and large mixes, and the larger mixes keep more of the sensitive tensors at higher precision. `Q8_0` is an older, simpler format at 8.5 bits and is close to lossless. `Q4_K_M` is the usual default.',
        seen: ['Q4_K_M', 'Q5_K_S', 'Q6_K', 'Q8_0'],
      },
      {
        id: 'i-quants',
        term: 'I-quants and imatrix',
        aka: ['IQ4_XS', 'IQ3_XXS', 'imatrix', 'importance matrix', 'UD', 'dynamic quants'],
        short: 'An importance matrix that steers quantisation, and the low-bit formats that rely on it.',
        body:
          'An importance matrix (imatrix) is measured on sample text and tells the quantiser which weights to keep more precisely. Any format can use one: `i1-Q4_K_M` is an ordinary K-quant made with an imatrix. I-quants (`IQ`) are a separate family that packs weights more tightly, and the low-bit ones need an imatrix. The gain is largest at 2 and 3 bits.',
        seen: ['IQ4_XS', 'i1-Q4_K_M', 'UD-Q4_K_XL'],
      },
      {
        id: 'bits-per-weight',
        term: 'Bits per weight',
        aka: ['bpw'],
        short: 'The average storage per weight, and the fair way to compare quantisations.',
        body:
          'File size ≈ parameters × bits per weight ÷ 8, so a 30.5B model at 4.85 bpw is about 18.5 GB. Names round: the same format averages a slightly different number of bits in different models, and two “4-bit” files can differ by a gigabyte or more.',
      },
      {
        id: 'precision',
        term: 'BF16 and F16',
        aka: ['bf16', 'fp16', 'f16', 'full precision', 'FP8'],
        short: 'The 16-bit formats models are trained and released in. “Full precision” in local use.',
        body:
          'BF16 has the range of 32-bit floats with less precision, and most labs release in it. F16 is the older 16-bit format. FP8 halves the size again and is common on data-centre GPUs. Locally, 16-bit files are mostly a source for making quantisations.',
      },
      {
        id: 'fp4',
        term: 'MXFP4 and NVFP4',
        aka: ['FP4', 'INT4', '4-bit float'],
        short: '4-bit number formats, which some models are released in by their lab.',
        body:
          'gpt-oss was post-trained in MXFP4, so it loses less than a 4-bit quantisation of a 16-bit model would. NVFP4 is NVIDIA’s format for its newest GPUs; many NVFP4 uploads are ordinary quantisations, so check the card. See also [[qat]].',
      },
      {
        id: 'qat',
        term: 'Quantisation-aware training',
        aka: ['QAT'],
        short: 'Training a model so it expects to be quantised, which cuts the loss when it is.',
        body:
          'The model sees the rounding during training and learns around it. Kimi K2 Thinking and some Gemma releases are QAT models. Use the lab’s QAT file rather than quantising the full-precision one yourself.',
      },
      {
        id: 'gguf',
        term: 'GGUF',
        aka: ['.gguf'],
        short: 'llama.cpp’s model file: weights, tokenizer and chat template in one file.',
        body:
          'GGUF is what Ollama, LM Studio and llama.cpp load. Hugging Face repositories ending in `-GGUF` hold one file per [[k-quants|quantisation]]; pick the one that fits. Large models are split into numbered parts; llama.cpp loads them from the first one.',
        seen: ['model-Q4_K_M.gguf', '-00001-of-00003.gguf'],
      },
      {
        id: 'mlx',
        term: 'MLX',
        aka: ['mlx-lm', 'mlx-community'],
        short: 'Apple’s machine-learning framework, and the model format that goes with it.',
        body:
          'MLX is built for Apple Silicon and mostly used there, though it also has CUDA and Linux CPU builds. LM Studio can run MLX models instead of GGUF, often a little faster on a Mac. MLX models are quantised separately, usually to 4, 6 or 8 bits, and many live under `mlx-community` on Hugging Face.',
        seen: ['mlx-community/…-4bit'],
      },
      {
        id: 'safetensors',
        term: 'Safetensors',
        aka: ['.safetensors', 'Hugging Face format'],
        short: 'The standard format labs publish full-precision weights in.',
        body:
          'It is what vLLM and training tools load, and what GGUF and MLX files are converted from. Unlike PyTorch’s older pickle files, loading one cannot run code.',
      },
      {
        id: 'offloading',
        term: 'Offloading',
        aka: ['n-gpu-layers', 'ngl', 'partial offload', 'n-cpu-moe', 'CPU offload'],
        short: 'Running part of a model from system RAM when it does not fit on the GPU.',
        body:
          'The layers left in RAM run at RAM speed, which is several times slower. For [[moe|MoE]] models there is a better split: keep attention on the GPU and move the expert weights to RAM, since each token only reads a few of them. llama.cpp does this with `--n-cpu-moe`.',
        seen: ['-ngl 99', '--n-gpu-layers', '--n-cpu-moe 20'],
      },
    ],
  },
  {
    id: 'speed',
    title: 'Speed',
    intro: 'Why one model writes fifty tokens a second and another five.',
    terms: [
      {
        id: 'inference',
        term: 'Inference',
        short: 'Running a trained model to get output, as opposed to training it.',
        body: 'Everything you do with a local model is inference. An “inference engine” is a [[runtime]].',
      },
      {
        id: 'prefill',
        term: 'Prefill',
        aka: ['prompt processing', 'pp', 'pp512', 'prompt eval'],
        short: 'Reading the prompt before the first word of the answer.',
        body:
          'Prefill handles many tokens at once, so it is limited by compute rather than memory. It is the pause after a coding agent reads a large file. Apple Silicon is slower at prefill than an NVIDIA card of similar memory speed; M5 chips narrow the gap but do not close it. That is why a Mac can write quickly and still take a while to start answering. llama-bench reports it as `pp512`.',
        seen: ['pp512', 'prompt eval rate'],
      },
      {
        id: 'decode',
        term: 'Decode',
        aka: ['generation', 'token generation', 'tg', 'tg128', 'eval rate'],
        short: 'Writing the answer, one token at a time.',
        body:
          'For each token the model reads its active weights and the whole [[kv-cache]] from memory, so decode is limited by [[memory-bandwidth]]. That is why decode slows as the context fills. llama-bench reports it as `tg128`.',
        seen: ['tg128', 'eval rate'],
      },
      {
        id: 'tokens-per-second',
        term: 'Tokens per second',
        aka: ['tok/s', 't/s', 'tps'],
        short: 'The usual speed figure. Without saying which, it means decode speed at short context.',
        body:
          'Around 20 tok/s reads comfortably; an agent writing long diffs wants more. Treat a figure without a context length with suspicion: the same model can lose half its speed or more by 100k tokens.',
      },
      {
        id: 'memory-bandwidth',
        term: 'Memory bandwidth',
        aka: ['GB/s', 'bandwidth'],
        short: 'How fast memory can be read. The ceiling on how fast a model can write.',
        body:
          'Without [[speculative-decoding]], decode speed cannot beat bandwidth ÷ bytes read per token. An M5 Pro reads 307 GB/s and an RTX 4090 1,008 GB/s, so the same model writes about three times faster on the card, if it fits. The bytes read per token are the active weights plus the cache, which is why [[moe|MoE]] models and small caches matter so much.',
        figure: 'speed',
      },
      {
        id: 'ttft',
        term: 'Time to first token',
        aka: ['TTFT', 'latency'],
        short: 'How long you wait before the answer starts.',
        body:
          'Mostly [[prefill]]. With an agent, every tool result has to be read before the model can reply, so slow prefill shows up again and again. Runtimes that keep the cache between turns only have to read the new part.',
      },
      {
        id: 'speculative-decoding',
        term: 'Speculative decoding',
        aka: ['draft model', 'MTP', 'multi-token prediction', 'EAGLE'],
        short: 'Guessing several tokens ahead with something cheap and checking them all at once.',
        body:
          'A small draft model, or extra prediction heads built into the model (MTP), proposes the next few tokens. The main model checks them in one pass and keeps the ones it would have written itself. The output is what the main model would have produced on its own; it arrives faster when the guesses are good, which in code they often are.',
        seen: ['--model-draft', '-md'],
      },
      {
        id: 'flash-attention',
        term: 'Flash attention',
        aka: ['FA', '-fa'],
        short: 'A faster way to compute attention that uses less working memory.',
        body:
          'It gives the same result with fewer trips to memory. Most runtimes turn it on by default where the hardware supports it. In llama.cpp a quantised V cache requires it.',
        seen: ['-fa on', 'OLLAMA_FLASH_ATTENTION=1'],
      },
    ],
  },
  {
    id: 'running',
    title: 'Running it',
    intro: 'The software between a model file and your editor.',
    terms: [
      {
        id: 'runtime',
        term: 'Runtime',
        aka: ['inference engine', 'Ollama', 'LM Studio', 'llama.cpp', 'vLLM', 'server'],
        short: 'The program that loads a model and serves it: Ollama, LM Studio, llama.cpp, vLLM.',
        body:
          'llama.cpp is the engine underneath most local setups. Ollama started as a wrapper around it and now also has its own engine on the same ggml library, plus model downloads and a background service. LM Studio adds a desktop app and can also run [[mlx|MLX]]. vLLM is built for serving many users from data-centre GPUs. All of them expose an [[openai-api|HTTP API]] that coding tools connect to.',
        seen: ['localhost:11434', 'localhost:1234', 'llama-server'],
      },
      {
        id: 'backend',
        term: 'GPU backend',
        aka: ['Metal', 'CUDA', 'ROCm', 'Vulkan'],
        short: 'The layer a runtime uses to talk to your GPU: Metal on Mac, CUDA on NVIDIA, ROCm on AMD.',
        body:
          'Vulkan works across vendors and covers GPUs the others do not. If a model runs far slower than expected, check that the runtime is using the GPU at all; a CPU-only build or a missing driver is the usual cause.',
      },
      {
        id: 'openai-api',
        term: 'OpenAI-compatible API',
        aka: ['/v1/chat/completions', 'Anthropic-compatible', 'base URL', 'endpoint'],
        short: 'The HTTP interface most runtimes speak, so any tool that talks to OpenAI can talk to them.',
        body:
          'Point a tool’s base URL at `http://localhost:11434/v1` and it uses your local model. Ollama also speaks Anthropic’s API, so Claude Code can run against it by setting `ANTHROPIC_BASE_URL`.',
        seen: ['OPENAI_BASE_URL', 'ANTHROPIC_BASE_URL'],
      },
      {
        id: 'chat-template',
        term: 'Chat template',
        aka: ['prompt template', 'Jinja', 'harmony', 'chat format', 'chat_template_kwargs'],
        short: 'The exact text format a model was trained to see conversations in.',
        body:
          'Each model marks system, user, assistant and tool turns with its own special tokens. GGUF files carry the template as Jinja code; llama.cpp and LM Studio run it, while Ollama uses templates of its own. A wrong or outdated template is a common reason a good model behaves badly locally. gpt-oss uses its own format, called harmony. Template settings such as turning [[thinking]] off are passed as `chat_template_kwargs`.',
        seen: ['--jinja', 'chat_template_kwargs', '<|im_start|>'],
      },
      {
        id: 'tool-call-parser',
        term: 'Tool-call parser',
        aka: ['reasoning parser', 'tool parser'],
        short: 'Runtime code that turns a model’s raw output into structured tool calls.',
        body:
          'Models write tool calls in their own formats: XML-style tags, JSON, special tokens. The runtime has to recognise that format and hand the agent a clean call. A reasoning parser does the same for [[thinking]] text. When a card says a model needs a recent runtime, this is usually why.',
        seen: ['--tool-call-parser qwen3_coder', '--reasoning-parser'],
      },
      {
        id: 'tokenizer',
        term: 'Tokenizer',
        aka: ['tokeniser', 'vocabulary', 'vocab'],
        short: 'The part that splits text into tokens and back. Each model family has its own.',
        body:
          'Token counts are not comparable across families: the same file can come to 10% more tokens with one tokenizer than another. GGUF files include the tokenizer, so you rarely handle it yourself.',
      },
      {
        id: 'system-prompt',
        term: 'System prompt',
        aka: ['system message', 'instructions'],
        short: 'Instructions placed before the conversation that set how the model behaves.',
        body:
          'A coding agent’s system prompt is long: rules and tool descriptions, with your project’s [[instruction-files]] added alongside. All of it counts against the [[context-window]] on every request, which is one reason agents need more context than chat.',
      },
      {
        id: 'sampling',
        term: 'Sampling settings',
        aka: ['temperature', 'top_p', 'top_k', 'min_p', 'repetition penalty', 'presence penalty'],
        short: 'How the next token is chosen from the model’s candidates.',
        body:
          'Temperature sets how adventurous the choice is; low is more predictable. Top-p, top-k and min-p cut unlikely candidates first. Labs publish the settings their model was tuned for, and agents work best with them. Many “this model is bad” reports come down to a runtime default overriding them.',
        seen: ['temperature 0.7', 'top_p 0.8', '--min-p 0.01'],
      },
      {
        id: 'hugging-face',
        term: 'Hugging Face',
        aka: ['HF', 'hub', 'repo'],
        short: 'Where labs publish models and the community publishes conversions.',
        body:
          'A repository like `Qwen/Qwen3-Coder-30B-A3B-Instruct` holds the lab’s release; one like `unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF` holds someone else’s [[gguf|GGUF]] conversions of it. Check who made a conversion before downloading tens of gigabytes of it.',
      },
    ],
  },
  {
    id: 'agents',
    title: 'Agents',
    intro: 'What turns a model into something that edits your code.',
    terms: [
      {
        id: 'harness',
        term: 'Harness',
        aka: ['coding agent', 'scaffold', 'agent', 'Claude Code', 'Codex', 'OpenCode', 'Cline', 'Aider'],
        short: 'The program around a model that gives it tools and runs the loop: Claude Code, Codex, OpenCode, Cline.',
        body:
          'The model only writes text. The harness turns that text into file edits and shell commands, feeds the results back, and decides when to stop. The same model can score very differently in different harnesses, which is why benchmark results name the one they used.',
      },
      {
        id: 'agentic',
        term: 'Agentic',
        aka: ['agentic coding', 'agent loop', 'long-horizon'],
        short: 'Working in a loop of read, act and check, rather than answering once.',
        body:
          'An agentic coding session can run hundreds of [[tool-calling|tool calls]]. “Long-horizon” means the model stays on track that whole time. Plenty of models answer one coding question well and then lose the thread on step forty; that is the gap agent benchmarks try to measure.',
      },
      {
        id: 'tool-calling',
        term: 'Tool calling',
        aka: ['function calling', 'tool use', 'tools'],
        short: 'The model asking the harness to run something: read a file, run tests, search.',
        body:
          'The harness sends a description of each tool with every request. The model replies with a structured call instead of prose, and the harness runs it and returns the result. A model that cannot do this reliably cannot drive a coding agent, however well it writes code.',
      },
      {
        id: 'structured-output',
        term: 'Structured output',
        aka: ['JSON mode', 'grammar', 'constrained decoding'],
        short: 'Forcing a model’s output to match a format such as a JSON schema.',
        body:
          'The runtime blocks any token that would break the format, so the output always parses. llama.cpp calls these grammars. It helps with tool calls from weaker models, at a small cost in speed.',
      },
      {
        id: 'mcp',
        term: 'MCP',
        aka: ['Model Context Protocol', 'MCP server'],
        short: 'A standard way to plug extra tools into an agent: a database, GitHub, a browser.',
        body:
          'An MCP server is a small program the harness starts or connects to. It lists its tools, and the model can call them like built-in ones. MCP works across harnesses, so servers usually carry over when you switch. Their settings often hold API keys, so treat the config file as a secret.',
        seen: ['.mcp.json', 'mcpServers'],
      },
      {
        id: 'subagents',
        term: 'Subagents',
        aka: ['subagent', 'Task tool', 'agent teams'],
        short: 'Helper agents a main agent starts for a side task, each with a fresh context.',
        body:
          'A subagent explores the codebase or runs a check, then returns a short summary. The main context stays small. Harnesses often send subagents to a smaller, faster model.',
      },
      {
        id: 'skills',
        term: 'Skills',
        aka: ['SKILL.md'],
        short: 'Packaged instructions an agent loads only when a task needs them.',
        body:
          'A skill is a folder with a `SKILL.md` and any scripts it uses. The agent sees a one-line description of each skill and reads the rest on demand, which keeps the [[system-prompt]] short.',
        seen: ['SKILL.md', '.claude/skills/'],
      },
      {
        id: 'hooks',
        term: 'Hooks',
        aka: ['hook', 'PreToolUse'],
        short: 'Scripts a harness runs on events, such as before a tool call or when a session ends.',
        body:
          'Hooks enforce rules the model might forget: format after every edit, block writes to a folder. Their event names and inputs differ between harnesses, so they carry over less well than instructions or MCP servers.',
      },
      {
        id: 'instruction-files',
        term: 'Instruction files',
        aka: ['AGENTS.md', 'CLAUDE.md', 'rules', 'memory file'],
        short: 'Markdown files in your repository that tell an agent how the project works.',
        body:
          '`AGENTS.md` is read by most harnesses, including Claude Code when a project has no `CLAUDE.md`; a `CLAUDE.md` can also import it. Their contents go into the context on every request, so keep them short.',
        seen: ['AGENTS.md', 'CLAUDE.md'],
      },
    ],
  },
  {
    id: 'reasoning',
    title: 'Reasoning',
    intro: 'Models that think before they answer, and the switches that control it.',
    terms: [
      {
        id: 'thinking',
        term: 'Thinking',
        aka: ['reasoning', 'reasoning model', 'chain of thought', 'CoT', 'non-thinking'],
        short: 'The model writes out its working before the answer.',
        body:
          'Thinking improves results on hard problems and costs tokens and time: a short reply can come with thousands of thinking tokens, all written at [[decode]] speed. “Non-thinking” models answer straight away and suit fast agent loops.',
      },
      {
        id: 'reasoning-effort',
        term: 'Reasoning effort',
        aka: ['reasoning_effort', 'thinking budget', 'low/medium/high'],
        short: 'A setting for how much a model thinks: low, medium, high, sometimes max.',
        body:
          'Higher effort usually scores better and runs slower. On a local machine the cost is wall-clock time, not money. Benchmark scores are usually taken at the highest setting; check which one before comparing.',
        seen: ['reasoning_effort: high', '--reasoning-budget'],
      },
      {
        id: 'hybrid-thinking',
        term: 'Hybrid thinking',
        aka: ['/think', '/no_think', 'enable_thinking', 'thinking toggle'],
        short: 'One model that can think or not, switched per request.',
        body:
          'Some models take a flag in the [[chat-template]] (`enable_thinking`); some older Qwen models also take `/think` and `/no_think` in the message. Labs differ, so check the card.',
        seen: ['enable_thinking: false', '/no_think'],
      },
      {
        id: 'preserved-thinking',
        term: 'Preserved thinking',
        aka: ['interleaved thinking', 'thinking with tools'],
        short: 'Keeping the model’s earlier thinking in context across tool calls.',
        body:
          'Many chat templates drop past thinking from the history to save context. In a long agent loop that means the model loses its own plan between steps. Some models built for agents can keep it: GLM-4.7-Flash’s card recommends turning this on for agent tasks, and Qwen3.6 has a similar option.',
      },
    ],
  },
  {
    id: 'benchmarks',
    title: 'Benchmarks',
    intro: 'The scores on model cards, and what each one measures.',
    terms: [
      {
        id: 'swe-bench',
        term: 'SWE-bench',
        aka: ['SWE-bench Verified', 'SWE-bench Pro', 'SWE-bench Multilingual'],
        short: 'Fixing real GitHub issues in real repositories. The main agentic coding score.',
        body:
          'The model gets an issue and a checkout and has to produce a patch that passes hidden tests. Verified is a human-checked set of 500 Python tasks. Pro is newer and harder, covers more languages and is designed to resist [[contamination]]. Scores depend heavily on the [[harness]] used.',
      },
      {
        id: 'terminal-bench',
        term: 'Terminal-Bench',
        short: 'Completing tasks in a real shell: building, configuring, debugging.',
        body:
          'Many tasks are system administration, security or data work rather than application code, and most models score lower here than on SWE-bench. Results name the harness, such as Terminus or Claude Code.',
      },
      {
        id: 'aider-polyglot',
        term: 'Aider Polyglot',
        short: 'Editing code to solve 225 exercises in six languages.',
        body:
          'It tests whether a model can make correct edits in a fixed edit format across C++, Go, Java, JavaScript, Python and Rust. It is a good check on languages other than Python.',
      },
      {
        id: 'livecodebench',
        term: 'LiveCodeBench',
        aka: ['Codeforces', 'competitive programming'],
        short: 'Competition programming problems, dated so a model can be scored only on ones published after its training cutoff.',
        body:
          'Scoring on fresh problems limits [[contamination]]. It measures algorithmic skill, not working in a codebase. A high score here with no agent scores tells you little about how a model will do in Claude Code.',
      },
      {
        id: 'tool-benchmarks',
        term: 'BFCL and τ²-bench',
        aka: ['BFCL', 'Berkeley Function Calling Leaderboard', 'tau-bench', 'τ²-Bench', 'Toolathlon'],
        short: 'Scores for calling tools correctly, in single calls and over long conversations.',
        body:
          'BFCL started with single calls, checking for the right function with the right arguments, and now also tests multi-turn and agentic use. τ²-bench runs conversations with a simulated user and tools. For agent work they are worth reading alongside the coding scores.',
      },
      {
        id: 'long-context-benchmarks',
        term: 'RULER and MRCR',
        aka: ['RULER', 'MRCR', 'needle in a haystack', 'long-context recall'],
        short: 'Tests of whether a model still finds and uses information deep in a long context.',
        body:
          'A 1M [[context-window]] is only useful if the model can use what is in it. These scores, given at a length such as 128k or 1M, are a better guide than the advertised window.',
      },
      {
        id: 'contamination',
        term: 'Contamination',
        aka: ['benchmaxxing', 'data leakage'],
        short: 'When benchmark questions were in the training data, so the score overstates ability.',
        body:
          'Public benchmarks leak into web data, and models can be tuned to their quirks. Newer sets refresh their tasks or keep them private for this reason. Try a model on your own tasks before trusting its scores.',
      },
    ],
  },
  {
    id: 'licences',
    title: 'Licences',
    intro: 'What you may do with the weights.',
    terms: [
      {
        id: 'licence',
        term: 'Licence',
        aka: ['license', 'Apache 2.0', 'MIT', 'open model licence', 'non-commercial'],
        short: 'What the lab lets you do with the weights. Apache 2.0 and MIT allow almost anything.',
        body:
          'Most coding models use Apache 2.0 or MIT, which allow commercial use. Others use the lab’s own licence: some restrict companies that serve the model to others, and some forbid commercial use without approval. For personal use on your own machine most of them are fine, though a non-commercial licence may rule out using the model for paid work. Read the licence before building a product on one.',
      },
    ],
  },
];

/** Every term in page order. */
export const terms = sections.flatMap((s) => s.terms);
