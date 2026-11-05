/**
 * Typed access to the model catalogue the CLI ships, and the memory
 * arithmetic lo0 does with it. The formulas follow internal/catalog and
 * internal/match so the site and the CLI give the same figures.
 */

import catalog from '../../../internal/catalog/catalog.json';

export type Model = (typeof catalog.models)[number];
export type Quant = Model['quants'][number];

export const GiB = 2 ** 30;
export const KiB = 2 ** 10;

/** Bytes one cached value takes: f16, or q8_0's 32 values in 34 bytes. */
export const kvBytesPerValue = { f16: 2, q8: 34 / 32 } as const;
export type KVType = keyof typeof kvBytesPerValue;

/** What lo0 allows for compute buffers, the Metal or CUDA context and the runtime. */
export const runtimeOverhead = 1.5 * GiB;

/** The catalogue entry for id. A missing model fails the build. */
export function model(id: string): Model {
  const m = catalog.models.find((x) => x.id === id);
  if (!m) throw new Error(`${id} is missing from the catalogue`);
  return m;
}

/** The named quantisation of m. A missing quant fails the build. */
export function quant(m: Model, name: string): Quant {
  const q = m.quants.find((x) => x.name === name);
  if (!q) throw new Error(`${m.id} has no ${name} quant`);
  return q;
}

type Attention = {
  fullLayers: number;
  fullKV: number;
  slidingLayers?: number;
  slidingKV?: number;
  window?: number;
  stateBytes?: number;
};

/** KV cache and recurrent state for a context of tokens, as catalog.Attention.KVBytes. */
export function kvBytes(m: Model, tokens: number, kv: KVType = 'f16'): number {
  const a = m.attention as Attention;
  const values = a.fullLayers * a.fullKV * tokens + (a.slidingLayers ?? 0) * (a.slidingKV ?? 0) * Math.min(tokens, a.window ?? 0);
  return values * kvBytesPerValue[kv] + (a.stateBytes ?? 0);
}

/** The part of the cache a new token attends to, which grows with the context. */
export function growingKVBytes(m: Model, tokens: number, kv: KVType = 'f16'): number {
  return kvBytes(m, tokens, kv) - kvBytes(m, 0, kv);
}

/** Cache bytes each extra token of context adds once past any sliding window. */
export function kvPerToken(m: Model, kv: KVType = 'f16'): number {
  const a = m.attention as Attention;
  return a.fullLayers * a.fullKV * kvBytesPerValue[kv];
}

/** The lab's own active parameter count where the card states one. */
export function statedActive(m: Model): number {
  return 'statedActiveParams' in m && m.statedActiveParams ? (m.statedActiveParams as number) : m.activeParams;
}

/** Weights read from memory to write one token at quantisation q. */
export function activeWeightBytes(m: Model, q: Quant): number {
  return (m.activeParams * q.bitsPerWeight) / 8;
}

export const isMoE = (m: Model) => m.activeParams < m.params;

/** 30.5B, 480B: billions to one decimal under 100. */
export function billions(n: number): string {
  const b = n / 1e9;
  return b >= 100 ? `${Math.round(b)}B` : `${b.toFixed(1).replace(/\.0$/, '')}B`;
}

/** 32k, 256k, 1M. */
export function tokens(n: number): string {
  return n >= 1024 * 1024 ? `${Math.round(n / (1024 * 1024))}M` : `${Math.round(n / 1024)}k`;
}

export const catalogueDate = new Date(catalog.generated).toLocaleDateString('en-GB', {
  day: 'numeric',
  month: 'short',
  year: 'numeric',
});
