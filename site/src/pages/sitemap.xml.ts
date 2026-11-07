import type { APIRoute } from 'astro';
import { site } from '../site';

// Every page file except the 404 and endpoints like this one.
const pages = import.meta.glob('./**/*.{astro,md,mdx}');

export const GET: APIRoute = () => {
  const urls = Object.keys(pages)
    .map((p) => p.replace(/^\.\//, '/').replace(/\.(astro|md|mdx)$/, '').replace(/\/index$/, '/'))
    .filter((p) => !p.includes('[') && p !== '/404')
    .sort()
    .map((p) => `  <url><loc>${new URL(p, site.url).href.replace(/\/$/, '') || site.url}</loc></url>`);
  const body = `<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n${urls.join('\n')}\n</urlset>\n`;
  return new Response(body, { headers: { 'Content-Type': 'application/xml' } });
};
