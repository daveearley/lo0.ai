/**
 * One consistent example machine and history, used by every graphic on the
 * landing page. The figures follow the shape of a real `lo0 match` run on an
 * Apple M5 Pro with 64 GiB; change them here and every section follows.
 */

import catalog from '../../../internal/catalog/catalog.json';

export const machine = {
  chip: 'Apple M5 Pro',
  memory: 64,
  usable: 48,
  bandwidth: 307,
  sessions: 427,
  turns: 51_041,
  span: '8 Mar – 4 Nov 2026',
};

/** Peak context per session, as cumulative share, at log2(tokens). */
const cdfAnchors: [number, number][] = [
  [14, 0],
  [14.6, 0.001],
  [15, 0.004],
  [16, 0.012],
  [17, 0.33],
  [Math.log2(190 * 1024), 0.5],
  [18, 0.61],
  [Math.log2(560 * 1024), 0.9],
  [Math.log2(943 * 1024), 1],
  [20, 1],
];

// Monotone cubic (Fritsch–Carlson) slopes, so the curve passes through every
// anchor without overshooting and the histogram has no hard edges.
const slopes = (() => {
  const n = cdfAnchors.length;
  const d = cdfAnchors.slice(1).map(([x1, y1], i) => (y1 - cdfAnchors[i][1]) / (x1 - cdfAnchors[i][0]));
  const m = cdfAnchors.map((_, i) => (i === 0 ? d[0] : i === n - 1 ? d[n - 2] : (d[i - 1] + d[i]) / 2));
  for (let i = 0; i < n - 1; i++) {
    if (d[i] === 0) { m[i] = 0; m[i + 1] = 0; continue; }
    const a = m[i] / d[i];
    const b = m[i + 1] / d[i];
    const h = a * a + b * b;
    if (h > 9) {
      const t = 3 / Math.sqrt(h);
      m[i] = t * a * d[i];
      m[i + 1] = t * b * d[i];
    }
  }
  return m;
})();

/** Share of sessions whose peak context fits within `tokens`. */
export function fits(tokens: number): number {
  const x = Math.log2(tokens);
  if (x <= cdfAnchors[0][0]) return 0;
  for (let i = 1; i < cdfAnchors.length; i++) {
    const [x1, y1] = cdfAnchors[i];
    if (x <= x1) {
      const [x0, y0] = cdfAnchors[i - 1];
      const h = x1 - x0;
      const t = (x - x0) / h;
      const t2 = t * t;
      const t3 = t2 * t;
      return (
        (2 * t3 - 3 * t2 + 1) * y0 +
        (t3 - 2 * t2 + t) * h * slopes[i - 1] +
        (-2 * t3 + 3 * t2) * y1 +
        (t3 - t2) * h * slopes[i]
      );
    }
  }
  return 1;
}

export const histogram = (() => {
  const step = 0.2;
  const from = 14;
  const bins = Array.from({ length: 30 }, (_, i) => {
    const lo = from + i * step;
    const hi = Math.round((lo + step) * 100) / 100;
    const count = Math.round(machine.sessions * (fits(2 ** hi) - fits(2 ** lo)));
    return { lo: Math.round(lo * 100) / 100, hi, count };
  });
  const max = Math.max(...bins.map((b) => b.count));
  return { from, to: 20, bins, max };
})();

/** Context windows a reader can try against the history. */
export const windows = [
  { id: '32k', tokens: 32 * 1024, label: '32k' },
  { id: '64k', tokens: 64 * 1024, label: '64k' },
  { id: '128k', tokens: 128 * 1024, label: '128k' },
  { id: '256k', tokens: 256 * 1024, label: '256k' },
  { id: '1m', tokens: 1024 * 1024, label: '1M' },
].map((w) => ({ ...w, log: Math.log2(w.tokens), share: fits(w.tokens) }));

export const percentiles = [
  { label: 'p50', tokens: 190 * 1024, text: '190k' },
  { label: 'p90', tokens: 560 * 1024, text: '560k' },
];

export type Carry = 'as is' | 'converted' | 'partly' | 'lost';

/** What carries over from Claude Code to OpenCode, from the example report. */
export const carryOver: { what: string; counts: [Carry, number][]; note?: string }[] = [
  { what: 'Instructions', counts: [['as is', 10], ['converted', 24]] },
  { what: 'Skills', counts: [['as is', 9], ['converted', 24]] },
  { what: 'MCP servers', counts: [['converted', 6]] },
  { what: 'Permissions', counts: [['converted', 6]] },
  { what: 'Hooks', counts: [['converted', 8], ['partly', 2], ['lost', 18]] },
  { what: 'Plugins', counts: [['partly', 3]] },
  { what: 'Settings', counts: [['converted', 2], ['lost', 1]] },
  { what: 'App tools', counts: [['lost', 20]], note: '10,071 calls in your history' },
];

const GiB = 2 ** 30;

/**
 * Memory budgets for Qwen3-Coder-30B-A3B on the example machine, worked out
 * from the catalogue the CLI ships: quantised file sizes for the weights and
 * the attention layout for the KV cache.
 */
export const budget = (() => {
  const runtime = 1.5;
  const maxWindow = 256 * 1024;
  const model = catalog.models.find((m) => m.id === 'qwen3-coder-30b-a3b');
  if (!model) throw new Error('qwen3-coder-30b-a3b is missing from the catalogue');
  const fileSize = (name: string) => {
    const q = model.quants.find((x) => x.name === name);
    if (!q) throw new Error(`qwen3-coder-30b-a3b has no ${name} quant`);
    return Math.round((q.bytes / GiB) * 10) / 10;
  };
  // Elements a token adds to the cache across every full-attention layer.
  const kvElements = model.attention.fullLayers * model.attention.fullKV;
  const quants = [
    { id: 'q8', label: 'Q8_0', weights: fileSize('Q8_0'), quality: 100 },
    { id: 'q6', label: 'Q6_K', weights: fileSize('Q6_K'), quality: 99 },
    { id: 'q4', label: 'Q4_K_M', weights: fileSize('Q4_K_M'), quality: 97 },
  ];
  const caches = [
    { id: 'f16', label: 'f16', perToken: kvElements * 2, quality: 0 },
    // q8_0 stores 32 values in 34 bytes.
    { id: 'q8kv', label: '8-bit', perToken: (kvElements * 34) / 32, quality: -1 },
  ];
  const combos = quants.flatMap((q) =>
    caches.map((c) => {
      const free = machine.usable - q.weights - runtime;
      const window = Math.min(maxWindow, Math.floor((free * GiB) / c.perToken));
      const kv = (window * c.perToken) / GiB;
      return {
        id: `${q.id}-${c.id}`,
        quant: q.id,
        cache: c.id,
        weights: q.weights,
        windowTokens: window,
        kv: Math.round(kv * 10) / 10,
        runtime,
        total: Math.round((q.weights + kv + runtime) * 10) / 10,
        windowK: Math.round(window / 1024),
        fits: Math.round(fits(window) * 100),
        quality: q.quality + c.quality,
      };
    }),
  );
  return { model: model.name, usable: machine.usable, runtime, quants, caches, combos };
})();
