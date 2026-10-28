# Textify ⚡

[![Go Version](https://img.shields.io/badge/go-1.18%2B-blue.svg)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

**Textify** is a high-performance CLI tool that converts your codebase into structured, AI-ready context for Large Language Models (LLMs) like Claude, ChatGPT, DeepSeek, and Gemini.

Instead of dumping bloated folders into chat windows, Textify structures your project with directory trees, 1-based source line numbers, automated secret redaction, and a **2-stage interactive workflow** that saves up to 90% of your tokens.

---

## 🌟 Why Textify?

- 📉 **Save 80–90% of Tokens:** Export lightweight type and function signatures first. Then, load only the exact files, line ranges, or search terms requested by the AI.
- 🎯 **Ground AI with Line Numbers:** Every source line is indexed (`  120 | func Run()`), enabling LLMs to write surgical edits and accurate patch directives.
- 🔍 **Flexible Request Directives:** Slices, multiple ranges, open-ended ranges, keyword searches (`search`/`grep`), and symbol extraction are all supported out-of-the-box.
- 📋 **1-Click Clipboard Workflow:** Pipe requests straight from clipboard to clipboard with `textify load -ci -c`. Zero file juggling required.
- 🛡️ **Automated Secret Redaction:** Scrub API keys, private keys, AWS tokens, and GitHub PATs before code ever leaves your machine.

---

## 📦 Installation

### Option 1: Go Install (Recommended)
```bash
go install github.com/JohnEsleyer/textify/cmd/textify@latest
```

### Option 2: Build and Install from Source
```bash
git clone https://github.com/JohnEsleyer/textify.git
cd textify
go install .
```
*(Or compile locally with `go build .`)*

---

## 🎯 The 2-Stage Interactive Workflow

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
         │ 4. textify load -ci -c
         ▼
   Clipboard filled with ONLY
   requested lines & functions
```

### Stage 1: Export Signatures & Prompt the LLM
1. Extract signatures and copy them to your clipboard:
   ```bash
   textify signatures -c
   ```
2. Paste into your LLM chat.
3. Append instructions by copying from `textify prompt`:
   > *"If you need full implementations, respond using `<request><file path="..." lines="X-Y" /></request>`."*

### Stage 2: Load the AI's Request
Once the AI responds with requested files:
1. **Instant Clipboard Workflow:** Click "Copy" on the AI's response, then run:
   ```bash
   textify load -ci -c
   ```
   *Your clipboard is immediately replaced with the requested line ranges, formatted and ready to paste!*
2. **File Workflow:** Or save the response into `request.txt` and run:
   ```bash
   textify load -c
   ```

---

## 📋 Comprehensive Request Directives Reference

Textify's `load` command supports multiple intuitive ways for LLMs to request code:

### 1. XML Directives (`<request> ... </request>`)

#### Specific Line Ranges
```xml
<request>
  <!-- Single line range -->
  <file path="pkg/compiler/ast.go" lines="50-120" />

  <!-- Multiple line ranges in a single file -->
  <file path="pkg/compiler/typechecker.go" lines="10-35, 120-160" />

  <!-- From line 100 through end of file -->
  <file path="pkg/compiler/codegen.go" lines="100:" />

  <!-- From start of file through line 50 -->
  <file path="pkg/compiler/parser.go" lines=":50" />
</request>
```

#### Keyword & Pattern Search (`search` / `grep` / `context`)
When the AI knows what code to look for but doesn't know exact line numbers:
```xml
<request>
  <!-- Search for a keyword with 5 lines of context before and after -->
  <file path="pkg/compiler/codegen_expr.go" search="PromoteExpr" context="5" />

  <!-- Regex search -->
  <file path="runtime/ube_runtime.h" grep="ube_arena_.*enter" context="10" />
</request>
```

#### Symbol Extraction (`symbols`)
Extract specific functions, structs, or methods:
```xml
<request>
  <file path="pkg/compiler/typechecker_expr.go" symbols="inferExpr,checkCallArgs" />
</request>
```

#### Glob Patterns & Whole Files
```xml
<request>
  <!-- Entire file -->
  <file path="cmd/ube/main.go" />

  <!-- All files matching a glob pattern -->
  <file glob="tests/test_*.ube" />
</request>
```

*Note: `<slice ... />`, `<read ... />`, and `<get ... />` tags are also accepted as aliases for `<file ... />`.*

---

### 2. Markdown Bullet Lists

Textify also parses standard Markdown lists:

```markdown
- cmd/ube/main.go:50-120
- `pkg/compiler/parser.go`:10-30, 80-110
- pkg/compiler/codegen.go (lines 100:)
- pkg/compiler/lexer.go (search: "TokenPromote")
- pkg/compiler/ast.go (symbols: Program, Node)
- examples/01_hello/main.ube
```

---

## 🛠️ CLI Commands & Flags

| Command | Description |
| :--- | :--- |
| `textify init` | Scans directory tree and generates `textify.yaml`. |
| `textify scan` | Detects new directories and appends them to `textify.yaml`. |
| `textify start` | Bundles project files based on rules in `textify.yaml`. |
| `textify signatures` | Extracts types, structs, interfaces, and function headers. |
| `textify diff` | Bundles **only** modified, staged, or untracked Git files. |
| `textify load [file]` | Extracts specific files, slices, searches, or symbols. |
| `textify prompt` | Prints copy-pasteable instructions for the LLM. |
| `textify version` | Prints current version. |
| `textify help` | Shows CLI usage and available options. |

### `textify load` Flags

| Flag | Shorthand | Default | Description |
| :--- | :---: | :---: | :--- |
| `--clip-in` | `-ci` | `false` | Reads request directly from system clipboard. |
| `--clip` | `-c` | `false` | Copies output slices directly to system clipboard. |
| `--format` | | `raw` | Output format: `raw`, `markdown`, or `xml`. |
| `--stdout` | | `false` | Streams output directly to standard output (for piping). |
| `--no-redact` | | `false` | Disables automated scrubbing of sensitive tokens/keys. |
| `-o <file>` | | *(config)* | Override output destination filename. |

### Examples

```bash
# 1-Click Clipboard workflow: Read request from clipboard, put slices back on clipboard
textify load -ci -c

# Load from request.txt and copy to clipboard
textify load request.txt -c

# Pipe from stdin and copy to clipboard
cat request.txt | textify load - -c
```

---

## 📄 License

Distributed under the MIT License. See [LICENSE](LICENSE) for more information.