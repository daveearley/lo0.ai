/** Site-wide facts. Change them here, not in components. */
export const site = {
  name: 'lo0',
  url: 'https://lo0.ai',
  title: 'lo0 · Find the local model that fits your machine and your work',
  description:
    'lo0 reads your hardware and your Claude Code and Codex history, shows which open models can do your work, and moves Claude Code or OpenCode onto the one you choose. Free and open source.',
  repo: 'https://github.com/daveearley/lo0',
  module: 'lo0.ai/lo0',
  install: {
    unix: 'curl -fsSL https://lo0.ai/install.sh | sh',
    windows: 'irm https://lo0.ai/install.ps1 | iex',
    go: 'go install lo0.ai/lo0/cmd/lo0@latest',
  },
  goVersion: '1.27',
  licence: 'AGPL-3.0',
  year: 2026,
  author: 'Dave Earley',
} as const;
