# Alibi

Alibi is a CLI tool that tracks contributions to a Git repository, records what changed at the function level, and gives you a local dashboard (plus exportable PDF reports) to review that history — optionally with AI-generated summaries of each commit.

It installs itself into your repo's Git hooks, so tracking happens automatically on every commit and push. No server, database, or account is required — everything is stored as JSON files on your machine.

## Features

- **Automatic tracking** — `post-commit` and `pre-push` Git hooks call `alibi track` for you; no manual step once initialized.
- **Function-level diffing** — changed files are parsed with [tree-sitter](https://github.com/smacker/go-tree-sitter) (Go, Python, JavaScript, TypeScript, TSX, C#, Java, PHP, with a regex-based fallback for other languages) to detect which functions were added, modified, removed, or left untouched by a commit.
- **Local dashboard** — `alibi serve` starts a local web server with a built-in UI (embedded into the binary, no separate install) for browsing tracked projects, commits, and pushes.
- **AI-generated summaries** (optional) — plug in an API key for OpenAI, DeepSeek, OpenRouter, or Hugging Face and Alibi can turn a commit's diff into a human-readable summary, with per-provider model listing and pricing shown in the dashboard.
- **PDF/HTML reports** — generate a shareable report of tracked activity for a date range, rendered via a local Chromium/Chrome/Edge/Brave install (falls back to an HTML file if none is found).
- **Config-driven include/exclude filtering** — control which files/paths are tracked via glob patterns in `alibi.yml`.
- **`alibi doctor`** — sanity-checks that your local config matches your Git remote and that the project's data directory exists.

## How it works

1. `alibi init` runs once inside a Git repository: it verifies a `.git` directory and remote exist, asks for a project name, lets you pick which remote/repo to track, writes an `alibi.yml` config file, creates a per-project data directory next to the Alibi binary, and installs the `post-commit`/`pre-push` hooks.
2. From then on, every commit and push triggers `alibi track`, which inspects the relevant commits via `git show`/`git log`, applies your `include`/`exclude` filters, extracts function-level changes, and appends the result as JSON under that project's data directory (one file per day).
3. `alibi serve` reads that data back, serves it through a small REST API, and renders it in the embedded dashboard UI in your browser at `http://localhost:<port>` (default `2025`).
4. From the dashboard you can request an AI summary of a commit (if a provider key is configured) or generate a PDF/HTML report for a date range.
5. `alibi doctor` can be run at any time to verify the config still matches your Git setup.

## Installation

### Windows installer

A prebuilt Windows installer is produced from [`installer/installer.iss`](installer/installer.iss) ([Inno Setup](https://jrsoftware.org/isinfo.php)) and installs `alibi.exe` under `Program Files\Alibi\bin`, adding a Start Menu shortcut.

### Building from source

The project cross-compiles a Windows binary from a Linux build container (Alibi uses CGO for tree-sitter, so the Docker image bundles a MinGW-w64 cross-compiler):

```sh
make docker-setup   # builds the alibi-builder image, then runs the build once
# or, on subsequent builds:
make build
```

This produces `bin/alibi.exe`. See [`dockerfile`](dockerfile) and [`makefile`](makefile) for the exact toolchain.

## Getting started

```sh
cd your-git-project
alibi init      # one-time setup: writes alibi.yml, installs Git hooks
# ...work as usual: commit and push normally...
alibi serve     # opens the dashboard at http://localhost:2025
```

Run `alibi doctor` any time to confirm your config is still valid, and `alibi --version` / `alibi -v` to check the installed version.

## Configuration (`alibi.yml`)

`alibi init` generates this file for you from a template, but it can be edited by hand afterwards:

```yaml
project:
  name: "my_project"                 # project name (used as the data-folder name)
  owner: "you@example.com"           # your Git author email
  source: "/path/to/your/codebase"   # absolute path to the project's working directory
  data: "/path/to/alibi/data/for/the/project" # where tracked activity JSON is stored

tracking:
  repo: "https://git-repo-host/username/my-project.git"
  remote: "origin"
  branch: "main"
  include:                 # glob patterns for files to track
    - "*"
  exclude:                 # glob patterns to skip
    - ".git/**"
    - "node_modules/**"
    - "vendor/**"
    - "dist/**"
    - "build/**"

AIProviders:
  OpenAI: ""
  DeepSeek: ""
  OpenRouter: ""
  HuggingFace: ""
```

AI provider keys can also be added later via `alibi init`'s prompt, or by editing `alibi.yml` directly — they're only used to request commit summaries and model/pricing lists, never sent anywhere else.

## Commands

| Command | Description |
|---|---|
| `alibi init` | Initialize Alibi in the current Git repository (config, data directory, Git hooks). |
| `alibi track --event=commit\|push [--from] [--to]` | Record the current commit/push. Normally invoked automatically by the Git hooks, not by hand. |
| `alibi serve [--port]` | Serve the local dashboard (default port `2025`, falls back to a free port if taken). |
| `alibi doctor` | Verify `alibi.yml` matches your local Git remote and that the project data directory exists. |
| `alibi --version` / `-v` | Print the CLI version. |

## Project layout

```
cli/                      Go module source for the CLI
  cmd/cli/                main package / entrypoint
  internal/cli/           Cobra commands (init, track, serve, doctor) + supporting packages
    constants/            Shared filesystem path conventions
    helpers/              Git, AI-provider, and tree-sitter-based parsing helpers
    pdf/                  Chromium-driven HTML → PDF report generation
    sample/               Embedded alibi.yml template used by `alibi init`
    types/                Shared config/data types
web/                      Static dashboard UI, embedded into the binary via go:embed
installer/                Windows (Inno Setup) installer definition
dockerfile, makefile      Cross-compilation build pipeline
```

## Requirements

- A Google Chrome, Microsoft Edge, Chromium, or Brave install if you want PDF report generation (otherwise reports are saved as HTML).
- An API key from OpenAI, DeepSeek, OpenRouter, or Hugging Face if you want AI-generated commit summaries — entirely optional.
