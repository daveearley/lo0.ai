// @ts-check
import { defineConfig } from 'astro/config';

export default defineConfig({
  site: 'https://lo0.ai',
  trailingSlash: 'never',
  build: {
    format: 'file',
    inlineStylesheets: 'always',
  },
  prefetch: {
    prefetchAll: true,
    defaultStrategy: 'viewport',
  },
  devToolbar: { enabled: false },
});
