# Textify ⚡

[![Version](https://img.shields.io/badge/version-0.6.0-blue.svg)](https://github.com/JohnEsleyer/textify/releases)
[![Go Version](https://img.shields.io/badge/go-1.18%2B-blue.svg)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

**Textify** is a high-performance CLI tool that converts your codebase into structured, AI-ready context for Large Language Models (LLMs) such as Claude, ChatGPT, DeepSeek, and Gemini.

Instead of dumping bloated folders into chat windows, Textify structures your project with directory trees, 1-based source line numbers, automated secret redaction, and a **2-stage interactive workflow** that saves up to 90% of your tokens.

---

## 🌟 Why Textify?

- 📉 **Save 80–90% of Tokens:** Export lightweight type and function signatures first. Then, load only the exact files, sections, or search terms requested by the AI.
- 🎯 **Stop Guessing Line Numbers:** AI models can request code using keywords (`find="..."`), block delimiters (`from="..." to="..."`), function/struct names (`symbol="..."`), or project-wide keyword search (`<grep pattern="..." />`).
- ⚡ **Instant Search CLI (`textify grep`):** Locate any identifier across the entire project with surrounding context lines and copy directly to your clipboard.
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
   > *"If you need full implementations, respond using `<request><file path="..." find="..." /></request>` or `<grep pattern="..." />`."*

### Stage 2: Load the AI's Request
Once the AI responds with requested directives:
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

## 📋 Comprehensive Request Directives (No Guessing!)

Textify supports intuitive, pattern-based requests so neither you nor the LLM has to guess line ranges in large files:

### 1. Find by Keyword with Context (`find` / `around` / `context`)
Ideal when you know what code construct you are looking for inside a file:
```xml
<request>
  <!-- Finds 'case *PromoteExpr:' and returns 20 lines of context around it -->
  <file path="pkg/compiler/codegen_expr.go" find="case *PromoteExpr:" context="20" />
</request>
```

### 2. Block Extraction (`from` ... `to`)
Extracts a complete block or handler from a starting pattern to a stopping delimiter:
```xml
<request>
  <!-- Grabs the entire AST handler from 'case *PromoteExpr:' down to the next case -->
  <file path="pkg/compiler/typechecker_expr.go" from="case *PromoteExpr:" to="case *" />
</request>
```

### 3. Project-Wide Grep (`<grep pattern="..." />`)
Finds every occurrence across the entire codebase with surrounding context lines:
```xml
<request>
  <!-- Finds all occurrences of PromoteExpr across all project files -->
  <grep pattern="PromoteExpr" context="10" />

  <!-- Restrict search to specific file extensions -->
  <grep pattern="ube_arena_enter" glob="*.h" context="15" />
</request>
```

### 4. Symbol Extraction (`symbol` / `symbols`)
Extracts an entire function, method, struct, or enum by name:
```xml
<request>
  <file path="pkg/compiler/typechecker_expr.go" symbol="inferExpr" />
</request>
```

### 5. Explicit Line Ranges (`lines`)
When line numbers are already known from Stage 1 signatures:
```xml
<request>
  <!-- Single line range -->
  <file path="pkg/compiler/ast.go" lines="50-120" />

  <!-- Multiple line ranges -->
  <file path="pkg/compiler/typechecker.go" lines="10-35, 120-160" />

  <!-- Open-ended range to EOF -->
  <file path="pkg/compiler/codegen.go" lines="100:" />
</request>
```

### 6. AST-Exact Definitions & Execution Flow (`<def>`)
For Go and TypeScript/JavaScript projects, Textify can parse declarations directly
and return the exact line range instead of guessing from text patterns:

```xml
<request>
  <!-- Exact function, method, struct, class, interface, or type definition -->
  <def name="handleRequest" />
  <def name="UserConfig" />

  <!-- Include an execution flow: funcA() -> funcB() -> funcC() -->
  <def name="handleRequest" trace="true" />

  <!-- Also works as a file attribute; file is used as an optional filter -->
  <file path="src/service.ts" def="UserService" trace="true" />
</request>
```

Structs/classes trace their attached methods and type dependency hierarchy, while
functions/methods trace their call chain. Markdown bullets `- def: Foo` and
`- trace: Foo` are supported too.

---

## 🧭 Instant CLI Definition & Trace

```bash
# Print the exact definition with line numbers
textify def handleRequest

# Include the execution flow (funcA() -> funcB() -> funcC())
textify def handleRequest --trace

# Trace a function or struct/class dependency hierarchy
textify trace UserService --lang ts

# Limit indexing to Go files and copy the result to the clipboard
textify def Server --lang go --trace -c
```

---

## 🔍 Instant CLI Grep (`textify grep`)

You can also search your codebase directly from the CLI:

```bash
# Search for keyword and copy matches with 5 context lines directly to clipboard
textify grep "PromoteExpr" -C 5 -c

# Restrict grep to Go files and print to terminal
textify grep "ube_arena_enter" --glob "*.go" -C 3
```

---

## 🛠️ CLI Commands & Flags

| Command | Description |
| :--- | :--- |
| `textify init` | Scans directory tree and generates `textify.yaml`. |
| `textify scan` | Detects new directories and appends them to `textify.yaml`. |
| `textify start [flags]` | Bundles project files based on rules in `textify.yaml`. |
| `textify signatures [flags]` | Extracts types, structs, interfaces, and function headers. |
| `textify diff [flags]` | Bundles **only** modified, staged, or untracked Git files. |
| `textify grep <pattern>` | Searches codebase for pattern and extracts matching context. |
| `textify def <symbol>` | Extracts an AST-exact function/struct/class definition (Go/TypeScript). |
| `textify trace <symbol>` | Traces a function call flow or a struct/class dependency hierarchy. |
| `textify load [file] [flags]` | Extracts requested files, slices, searches, or symbols. |
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

### `textify def` / `textify trace` Flags

| Flag | Shorthand | Default | Description |
| :--- | :---: | :---: | :--- |
| `--lang` | | `all` | Languages to index: `go`, `ts`, `js`, or a comma-separated list. |
| `--depth` | | `5` | Maximum trace depth. |
| `--trace` | | `false` | Include execution flow with `def`. |
| `--clip` | `-c` | `false` | Copy output directly to system clipboard. |

---

## 📄 License

Distributed under the MIT License. See [LICENSE](LICENSE) for more information.