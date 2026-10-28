
# Textify ⚡

[![Go Version](https://img.shields.io/badge/go-1.18%2B-blue.svg)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

**Textify** is a high-performance CLI tool that converts your codebase into structured, AI-ready context for Large Language Models (LLMs) such as Claude, ChatGPT, DeepSeek, and Gemini.

Instead of pasting random files into chat windows, Textify structures your project with directory trees, 1-based source line numbers, automated secret redaction, and intelligent filtering.

---

## 💡 Why Textify?

- 📉 **Save 80–90% of Tokens:** Use the **2-stage interactive workflow**—export function/type signatures first, prompt the LLM, then slice out only the exact files or line ranges it asks to see.
- 🎯 **Ground AI with Line Numbers:** Every line is indexed (`  120 | func Run()`), enabling LLMs to provide exact line references, diffs, and surgical refactorings.
- 🛡️ **Built-in Security:** Automatic secret redaction for API keys, AWS credentials, GitHub tokens, and private keys before code leaves your machine.
- 🌲 **Context-Aware Visual Tree:** Automatically prepends an ASCII directory tree of the included files.
- 🧠 **Nested `.gitignore` & Binary Detection:** Respects multi-tier `.gitignore` files, automatically detects binary files, and ignores minified assets (>2,000 chars/line).
- 📋 **Zero-Friction Clipboard Integration:** Native clipboard copy (`--clip` / `-c`) supporting macOS (`pbcopy`), Linux (`wl-copy`, `xclip`, `xsel`), Windows (`clip`), and terminal OSC 52 fallback.

---

## 📦 Installation

### Option 1: Go Install (Recommended)
```bash
go install github.com/JohnEsleyer/textify/cmd/textify@latest
```

### Option 2: Build from Source
```bash
git clone https://github.com/JohnEsleyer/textify.git
cd textify
go build -o textify ./cmd/textify
sudo mv textify /usr/local/bin/   # Optional: move to PATH
```

---

## ⚡ Quick Start

```bash
# 1. Initialize configuration by scanning your repository
textify init

# 2. Bundle your codebase and copy directly to your clipboard
textify start -c
```

The output file (default: `codebase.txt`) is generated with line-numbered XML-style tags, an ASCII tree, and token usage statistics:

```xml
PROJECT STRUCTURE:
├── cmd/
│   └── textify/
│       └── main.go
└── internal/
    └── scanner/
        └── scanner.go

============================================================

<file path="cmd/textify/main.go" lines="1-50">
    1 | package main
    2 | 
    3 | import (
    4 |     "fmt"
...
   50 | }
</file>
```

---

## 🎯 2-Stage Interactive Mode (Save Thousands of Tokens)

For large codebases, dumping all source files wastes context windows and inflates costs. Use Textify's two-step interaction:

```
┌─────────────────┐       1. textify signatures -c      ┌─────────────────┐
│                 ├────────────────────────────────────►│                 │
│                 │       2. textify prompt             │                 │
│                 │                                     │       LLM       │
│  Local Codebase │                                     │ (Claude/ChatGPT)│
│                 │◄────────────────────────────────────┤                 │
│                 │       3. LLM returns <request> tags │                 │
└────────┬────────┘                                     └─────────────────┘
         │
         │ 4. textify load request.txt -c
         ▼
   Clipboard filled with ONLY
   requested lines & functions
```

### Step 1: Export Signatures & Prompt the LLM
Export structs, types, interfaces, and function signatures without implementations (docs like `.md` stay fully intact):
```bash
textify signatures -c
```
Paste this into your LLM. Then run:
```bash
textify prompt
```
Copy the output instructions and append them to your query:
> *"If you need full implementations, respond using `<request><file path="..." lines="X-Y" /></request>`."*

### Step 2: Load the LLM's Request
Save the LLM's response into `request.txt` (Textify ignores surrounding conversational text).

**Supported formats within the response:**
* XML tags (Recommended):
  ```xml
  <request>
    <file path="cmd/textify/main.go" />
    <file path="internal/scanner/scanner.go" lines="50-120" />
    <file path="internal/config/config.go" symbols="Config,DirRule" />
  </request>
  ```
* Markdown lists:
  ```markdown
  - cmd/textify/main.go
  - internal/scanner/scanner.go:50-120
  ```

Then load the exact slices and copy them directly to your clipboard:
```bash
textify load request.txt -c
```

---

## 🛠️ Commands

| Command | Description |
| :--- | :--- |
| `textify init` | Scans directory tree, checks `.gitignore` rules, and generates `textify.yaml`. |
| `textify scan` | Detects new directories and safely appends them to `textify.yaml` without overwriting existing settings. |
| `textify start` | Bundles project files based on rules in `textify.yaml`. |
| `textify signatures` | Extracts types, structs, interfaces, and function headers while preserving line numbers. |
| `textify diff` | Bundles **only** modified, staged, or untracked Git files (`git status --porcelain`). |
| `textify load <file>` | Extracts specific files, line ranges, or symbols requested by the LLM. |
| `textify prompt` | Prints copy-pasteable instructions for the LLM during Stage 1. |
| `textify version` | Prints current version. |
| `textify help` | Shows CLI usage and available options. |

---

## 🚩 Command Flags

Available for `start`, `signatures`, `diff`, and `load`:

| Flag | Shorthand | Default | Description |
| :--- | :---: | :---: | :--- |
| `--clip` | `-c` | `false` | Copies generated output directly to your system clipboard. |
| `--format` | | `raw` / `xml` | Output styling: `xml` (or `tags`), `markdown`, or `raw`. |
| `--stdout` | | `false` | Writes directly to stdout (useful for piping: `textify start --stdout \| pbcopy`). |
| `--no-tree` | | `false` | Disables prepending the ASCII directory structure map. |
| `--no-redact` | | `false` | Disables automatic scrubbing of sensitive tokens/API keys. |
| `-o <file>` | | *(config)* | Override output destination filename (`load` command only). |

---

## ⚙️ Configuration (`textify.yaml`)

`textify init` creates a commented `textify.yaml` at your project root.

```yaml
# Global Output Configuration
output_file: codebase.txt
format: raw              # Options: raw, markdown, xml
tree: true               # Prepend ASCII directory map (default: true)
max_file_size: 512000    # File size limit in bytes (default: 500 KB)

dirs:
  # Root directory settings
  .:
    enabled: true
    extensions:
      - go
      - md
      - yaml
    include:
      - .env.example     # Force-include ignored files or specific dotfiles

  # Backend directory
  cmd:
    enabled: true
    extensions:
      - go

  # Frontend directory
  frontend:
    enabled: true
    compact: true        # Strips extra blank lines & leading/trailing whitespace
    extensions:
      - ts
      - tsx
      - css
    exclude:
      - "**/*.test.ts"   # Skip test files
    exclude_extensions:
      - map

  # Excluded directories
  node_modules:
    enabled: false

  dist:
    enabled: false
```

### Directory Rule Reference

| Property | Type | Description |
| :--- | :---: | :--- |
| `enabled` | `bool` | Set to `false` to skip this folder and all its subdirectories. |
| `compact` | `bool` | Strips empty lines and trims whitespace to optimize token usage. |
| `extensions` | `list` | Allow-list of extensions (e.g., `[go, ts]`). If empty, all text files are allowed. |
| `exclude_extensions` | `list` | Block-list of extensions (e.g., `[tmp, log, map]`). |
| `include` | `list` | Glob patterns/files to **force-include** (bypasses `.gitignore` and extension filters). |
| `exclude` | `list` | Glob patterns/files to **force-exclude** (highest priority). |

---

## 🛡️ Built-in Exclusions & Security

Textify safeguards context windows and prevents accidental credential leaks:

1. **Always Excluded:**
   - `.git/` folder
   - `textify.yaml` configuration
   - The configured `output_file`
2. **Binary & Asset Detection:**
   - Evaluates the first 512 bytes for null bytes and invalid UTF-8 sequences.
   - Automatically drops images, PDFs, compiled executables, etc.
3. **Minified Asset Guard:**
   - Skips files with line lengths exceeding 2,000 characters (e.g., minified JS bundles, single-line JSON dumps).
4. **Automated Secret Redaction:**
   - Redacts private keys (`BEGIN PRIVATE KEY`), AWS Access Keys (`AKIA...`), GitHub PATs (`ghp_...`), and key-value API tokens (`api_key = "..."`).
   - Summary statistics display how many tokens were sanitized.

---

## 📄 License

Distributed under the MIT License. See [LICENSE](LICENSE) for more information.
