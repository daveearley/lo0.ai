---
layout: ../layouts/Prose.astro
title: Privacy
eyebrow: How lo0 handles your data
description: lo0 reads sensitive files to set up local AI for you, so it is strict about what it keeps and where it goes.
---

lo0 reads your Claude Code configuration and your Claude Code and Codex session logs. Those files can hold prompts, source code, file paths and secrets, so the rules below are built into how the code is structured and checked by its tests.

## Nothing about you leaves your machine

lo0 uses the network when the job needs it: looking up the newest releases, fetching catalogue updates, installing a harness and downloading models. Each request goes to a source the code names, such as a package registry, a harness's own release page or the Hugging Face repository the catalogue names.

No request carries your usage profile, paths, prompts, setup or anything else about you, and none goes to a lo0 server. There is no telemetry and no account. Lookups that only add information fail silently without a connection, and `--offline`, or `LO0_OFFLINE` in your environment, skips them.

## Read-only until you confirm

Every scan stage receives a Go `fs.FS`, an interface with no write methods, so the assessment cannot change a file even by mistake. Writing happens only when you confirm a plan from `lo0 try` or `lo0 migrate`, and the plan lists every download with its size, every file and every command first.

`lo0 migrate` backs up every file it replaces, never writes into your own repositories and records each change as it happens, so `lo0 migrate --undo` can reverse it. Files you have edited since are left alone.

## Counts and names, not content

The usage profile lo0 builds never holds:

- prompt text or model output
- file contents
- command arguments
- secret values

MCP server `env` and `headers` are reduced to their key names, so lo0 can tell you a server needs `GITHUB_TOKEN` without holding the token. Diffs are reduced to a count of added and removed lines by decoders that never build the text. When a migration moves an MCP server to another harness, its values go from one file to the other on your machine and are never shown in the plan.

## Paths stay local

Paths to your own repositories appear in your local report, because they help you read it. `--redact`, and the report the workbench saves, replace them and your home directory so you can share the result.

## `lo0 web` stays on this machine

The browser view listens on the loopback address only, admits a request only with the token it prints at start-up, and loads no fonts, styles or scripts from anywhere else.

## How this is tested

The test suite feeds secrets, prompts and command arguments through every scanner and fails if any of them reach the output. Those tests run on macOS, Linux and Windows for every change.
