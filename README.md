# Textify ⚡

[![Version](https://img.shields.io/badge/version-0.7.0-blue.svg)](https://github.com/JohnEsleyer/textify/releases)
[![Go Version](https://img.shields.io/badge/go-1.18%2B-blue.svg)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

**Textify** is a high-performance CLI tool that converts your codebase into structured, AI-ready context for Large Language Models (LLMs) such as Claude, ChatGPT, DeepSeek, and Gemini — and **safely applies multi-file code modifications** directly back to your project.

Instead of dumping bloated folders into chat windows or manually editing files, Textify provides a complete **3-stage interactive round-trip workflow** that saves up to 90% of your tokens and applies AI-suggested modifications with zero manual copy-pasting errors.

---

## 🌟 Why Textify?

- 📉 **Save 80–90% of Tokens:** Export lightweight type and function signatures first. Then, load only the exact files, sections, or search terms requested by the AI.
- 🎯 **Stop Guessing Line Numbers:** AI models can request code using keywords (`find="..."`), block delimiters (`from="..." to="..."`), function/struct names (`symbol="..."`), or project-wide keyword search (`<grep pattern="..." />`).
- 🩹 **Atomic Two-Pass Patch Application (`textify patch`):** Safely apply multi-file modifications from LLMs. All changes are validated in an in-memory Virtual File System (VFS) first — if any search target fails, **zero files on disk are touched**.
- 🧩 **5-Tier Fuzzy Search Matching & Ellipsis (`...`):** Match code regardless of CRLF vs LF, trailing whitespace, or indentation shifts, and use `...` to preserve boilerplate without reproducing it.
- ⚡ **Instant Search CLI (`textify grep`):** Locate any identifier across the entire project with surrounding context lines and copy directly to your clipboard.
- 📋 **1-Click Clipboard Workflow:** Pipe requests straight from clipboard to clipboard with `textify load -ci -c` and apply with `textify patch -ci`. Zero file juggling required.
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
go install ./cmd/textify
```
*(Or compile locally with `go build -o textify ./cmd/textify`)*

---

## 🎯 The 3-Stage Interactive Workflow

```
┌─────────────────┐       1. textify signatures -c      ┌─────────────────┐
│                 ├────────────────────────────────────►│                 │
│                 │       2. textify prompt             │                 │
│                 │                                     │       LLM       │
│  Local Codebase │                                     │ (Claude/ChatGPT)│
│                 │◄────────────────────────────────────┤                 │
│                 │       3. LLM returns <request> tags │                 │
└────────┬────────┘                                     └────────┬────────┘
         │                                                       │
         │ 4. textify load -ci -c                                │ 5. LLM returns patch
         ▼                                                       │    (<file>, <replace>, <delete>)
   Clipboard filled with ONLY                                    │
   requested lines & functions                                   ▼
         │                                              ┌─────────────────┐
         └─────────────────────────────────────────────►│ 6. textify patch│
                                                        │    -ci          │
                                                        └─────────────────┘
                                                                 │
                                                        Atomic disk update
```

### Stage 1: Export Signatures & Prompt the LLM
1. Extract signatures and copy them to your clipboard:
   ```bash
   textify signatures -c
   ```
2. Paste into your LLM chat.
3. Append instructions by copying from `textify prompt`:
   > *"If you need full implementations, respond using `<request><file path="..." find="..." /></request>`. When writing solutions, provide patch directives using `<file>`, `<replace>`, or `<delete>`."*

### Stage 2: Load the AI's Request
Once the AI responds with requested directives:
1. **Instant Clipboard Workflow:** Click "Copy" on the AI's response, then run:
   ```bash
   textify load -ci -c
   ```
   *Your clipboard is immediately replaced with the requested line ranges, formatted and ready to paste back into the chat!*

### Stage 3: Apply the AI's Solution
When the AI provides its solution:
1. Click "Copy" on the AI's response.
2. Run:
   ```bash
   textify patch -ci
   ```
   *(Or preview first with `textify patch -ci --dry-run`)*

---

## 🩹 Patch Directives & Patcher Format

Textify's patch engine supports surgical, robust directives. Conversational text and markdown wrappers outside the tags are automatically ignored.

### 1. Create or Overwrite a File: `<file>`
```xml
<file path="cmd/app/main.go">
package main

import "fmt"

func main() {
    fmt.Println("Hello, World!")
}
</file>
```

### 2. AST-Exact Symbol Replacement (`symbol="..."`)
```xml
<replace path="pkg/compiler/codegen.go" symbol="inferExpr">
<with>
func inferExpr(ctx *Context, expr Expr) Type {
    return ctx.Resolve(expr)
}
</with>
</replace>

<!-- Disambiguate receiver methods -->
<replace path="pkg/server/server.go" symbol="Server.Start">
<with>
func (s *Server) Start() error {
    return s.listener.Accept()
}
</with>
</replace>
```

### 3. Search Block with Wildcards / Ellipsis (`...`)
```xml
<replace path="pkg/config/config.go">
<search>
type Config struct {
    ...
    Timeout int
    ...
}
</search>
<with>
type Config struct {
    ...
    Timeout int
    Retries int
    ...
}
</with>
</replace>
```

### 4. Anchor Insertions (`after="..."` or `before="..."`)
```xml
<replace path="pkg/api/server.go" after="router := gin.Default()">
<with>
    router.Use(cors.Default())
</with>
</replace>
```

### 5. Block Replacement (`from="..." to="..."`)
```xml
<replace path="pkg/compiler/typechecker.go" from="case *PromoteExpr:" to="case *">
<with>
case *PromoteExpr:
    val := e.Value
    genPromote(val)
case *
</with>
</replace>
```

### 6. Delete a File: `<delete />`
```xml
<delete path="path/to/obsolete_file.go" />
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
| `textify patch [file] [flags]` | Safely applies multi-file LLM patches (atomic two-pass VFS). |
| `textify prompt` | Prints copy-pasteable instructions for the LLM. |
| `textify version` | Prints current version. |
| `textify help` | Shows CLI usage and available options. |

### `textify patch` Flags

| Flag | Shorthand | Default | Description |
| :--- | :---: | :---: | :--- |
| `--dry-run` | `-n` | `false` | Validate and preview changes without modifying files on disk. |
| `--clip-in` | `-ci` | `false` | Read patch directly from system clipboard. |

---

## 📄 License

Distributed under the MIT License. See [LICENSE](LICENSE) for more information.
