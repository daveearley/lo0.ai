---
layout: ../layouts/Prose.astro
title: Install lo0
seoTitle: Install lo0 on macOS, Linux and Windows
eyebrow: macOS, Linux and Windows
description: One command installs lo0, a single binary that finds the local AI models your machine can run for coding and sets them up. No account, no runtime to install first.
---

lo0 is one binary with nothing to install beside it. It runs on macOS, Linux and Windows, on amd64 and arm64, and detects Apple Silicon, NVIDIA and AMD GPUs.

## macOS and Linux

```bash
curl -fsSL https://lo0.ai/install.sh | sh
```

The script picks the build for your system, downloads it from the [GitHub releases](https://github.com/daveearley/lo0/releases), checks it against the release's `checksums.txt` and installs it to `~/.local/bin`. It changes nothing else. It is a short POSIX shell script, so you can [read it](/install.sh) before you run it.

Two variables change what it does:

| Variable | Default | What it does |
| --- | --- | --- |
| `LO0_INSTALL_DIR` | `~/.local/bin` | Where the binary goes |
| `LO0_VERSION` | the newest release | A release tag such as `v0.3.0` |

```bash
curl -fsSL https://lo0.ai/install.sh | LO0_INSTALL_DIR="$HOME/bin" sh
```

## Windows

In PowerShell:

```powershell
irm https://lo0.ai/install.ps1 | iex
```

It installs `lo0.exe` to `%LOCALAPPDATA%\Programs\lo0`, checks it against the release checksum and adds the folder to your user `PATH`. Open a new terminal afterwards. The same `LO0_INSTALL_DIR` and `LO0_VERSION` variables apply. [Read the script](/install.ps1).

## With Go

```bash
go install lo0.ai/lo0/cmd/lo0@latest
```

Needs Go 1.27 or newer. The binary lands in `$(go env GOPATH)/bin`.

## From source

```bash
git clone https://github.com/daveearley/lo0
cd lo0
make build    # writes bin/lo0
```

## Run it

```bash
lo0
```

In a terminal, `lo0` opens the interactive workbench: how much of your work can run on this machine, every model that fits and the next step. `lo0 web` shows the same in your browser, and `lo0 match` prints it as a report. Nothing changes on your machine until you confirm a plan.

You do not need a model runtime first. When you try a model, the plan says what is missing: lo0 sets models up in [Ollama](https://ollama.com/download) on its own, and gives guided steps for LM Studio and llama.cpp.

## Uninstall

Delete the binary. If you kept a setup with `lo0 migrate`, undo it first so lo0 can put your files back:

```bash
lo0 migrate --undo
rm ~/.local/bin/lo0
```

Models lo0 downloaded stay in your runtime until you remove them, from the Installed screen or with the runtime's own command, such as `ollama rm`.
