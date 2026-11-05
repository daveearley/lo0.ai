# lo0.ai

The website for lo0, built with [Astro](https://astro.build) and served as static files from Cloudflare.

## Run it

Needs Node 22.23 or newer (`.nvmrc` pins 24).

```bash
npm install
npm run dev       # http://localhost:4321
npm run build     # type-checks, then writes dist/
npm run preview   # serves dist/
```

## Add a page

Drop a Markdown file in `src/pages/`. Its path is its URL, so `src/pages/docs/flags.md` becomes `/docs/flags`.

```markdown
---
layout: ../layouts/Prose.astro
title: Flags
eyebrow: Reference
description: Every flag lo0 accepts.
---

Write the page here.
```

For a page that needs components, make an `.astro` file and wrap it in `Base` from `src/layouts/Base.astro`. The sitemap picks up new pages on its own.

## Where things live

| Path | What |
| --- | --- |
| `src/site.ts` | Name, URLs, install commands. Change facts here, not in components. |
| `src/data/example.ts` | The example machine and history the interactive graphics draw from. |
| `src/data/screens/`, `src/assets/screens/` | Captured `lo0 ui` screens and `lo0 web` screenshots. Regenerate with `scripts/screens`. |
| `src/data/faq.ts` | Questions on the home page, also published as FAQPage structured data. |
| `public/install.sh`, `public/install.ps1` | The install scripts served at lo0.ai. They read the GitHub release `checksums.txt`. |
| `test/install.sh`, `test/install.ps1` | Tests for the install scripts. CI runs them on every platform against a local release; `make test-install` runs the shell one. |
| `src/styles/global.css` | Colour, type and spacing tokens, light and dark. |
| `src/components/` | One file per section graphic. |
| `../internal/catalog/catalog.json` | The model catalogue the CLI ships. The site reads it at build time, so the model list and memory figures stay in step. |

The interactive graphics are plain HTML and CSS: radio buttons, `:has()` and typed `@property` values. The only scripts on the page are the copy button and closing the mobile menu.

Every page carries a `go-import` meta tag, so `go install lo0.ai/lo0/cmd/lo0@latest` resolves to the GitHub repository. Go reads it from any path, the 404 page included.

## Deploy

Every push to `main` runs `.github/workflows/site.yml` once `ci.yml` has passed on that commit, because `ci.yml` is what tests the installers the site serves. It builds the site and runs `wrangler deploy`. Pull requests that touch `site/` or the catalogue build without deploying. The workflow needs two repository secrets:

- `CLOUDFLARE_API_TOKEN`, with the Workers Scripts: Edit permission
- `CLOUDFLARE_ACCOUNT_ID`

To serve it on `lo0.ai`, add the domain to the `lo0-site` Worker under Settings → Domains & Routes in the Cloudflare dashboard, or add a `routes` entry to `wrangler.jsonc`.
