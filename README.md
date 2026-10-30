
# Textify ⚡

[![Version](https://img.shields.io/badge/version-0.8.0-blue.svg)](https://github.com/JohnEsleyer/textify/releases)
[![Go Version](https://img.shields.io/badge/go-1.18%2B-blue.svg)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

**Textify** is a high-performance CLI tool that converts codebases into compact, AI-ready context for Large Language Models (Claude, ChatGPT, DeepSeek, Gemini) and **safely applies multi-file atomic patches** directly back to disk.

Instead of dumping bloated directories into chat windows or enduring 5–8 iterative question-and-answer round trips, Textify equips LLMs with **deep AST graph traversal, surgical sub-branch queries, and atomic VFS patching** — enabling complete multi-file implementations in **1 to 2 requests max**.

---

## 🌟 Why Textify?

- 🎯 **Target 1–2 Request Turnarounds:** Use deep AST tracing (`<trace>`), composite bundles (`<feature-context>`), and switch-case slicing (`<select>`) to give the model everything it needs on the very first turn.
- 📉 **Save 80–90% of Tokens:** Export lightweight symbol signatures first (`textify signatures`), then extract only the relevant call graphs and declarations.
- 🩺 **Deep Execution Flow Tracing (`<trace>`):** Automatically traverses functions, extracting not just a call graph, but the actual source code of callees and referenced data types.
- 🧩 **Surgical Switch-Case Isolation (`<select>` & `fn="..." case="..."`):** Inspect and modify individual `switch/case` clauses without dumping 800+ lines of enclosing functions.
- 🩹 **Atomic Two-Pass Patch Engine (`textify patch`):** Evaluates multi-file changes inside an in-memory Virtual File System (VFS). If any target anchor or search block fails, **disk files are untouched**.
- 🛠️ **Patch-Ready Anchors (`for-patch="true"`):** Generates clean `<search>` blocks with zero line-number prefixes, eliminating whitespace/indentation mismatch errors.
- 📋 **1-Click Clipboard Pipeline:** Pipe requests straight from clipboard to clipboard with `textify load -ci -c` and apply solutions with `textify patch -ci`.
- 🛡️ **Automated Secret Redaction:** Scrubs API keys, private keys, AWS tokens, and GitHub PATs before context ever leaves your machine.

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
go install ./cmd/textify
```

---

## ⚡ The 2-Turn Maximum Workflow

The ideal session requires only **two LLM steps**:
1. **Turn 1 (Inspect):** LLM receives signatures, immediately requests deep context via `<trace>` or `<feature-context>`.
2. **Turn 2 (Solve):** LLM receives the complete implementation flow and returns the atomic patch.

```
Local Codebase                          LLM (Claude / ChatGPT)
┌──────────────┐                          ┌──────────────────┐
│              │   1. textify sigs -c     │                  │
│              ├─────────────────────────►│ Turn 1: Requests │
│              │   (Codebase Signatures)  │ <trace> / <def>  │
│              │                          │                  │
│              │◄─────────────────────────┤                  │
│              │   2. AI Request Directives                  │
│              │      (Copied to clipboard)                  │
│              │                                             │
│ textify load │                                             │
│    -ci -c    │                                             │
│      │       │                                             │
│      ▼       │   3. Full Context Loaded │                  │
│ [Clipboard]  ├─────────────────────────►│ Turn 2: Returns  │
│              │                          │ complete atomic  │
│              │◄─────────────────────────┤ patch blocks     │
│              │   4. AI Patch Response   │                  │
└──────┬───────┘                          └──────────────────┘
       │
       │ textify patch -ci
       ▼
[Atomic Disk Update]
```

---

## 🔍 Deep AST Inspection Directives (For LLM Requests)

When requesting context, instruct the LLM to **avoid consecutive round trips** by using deep structural queries.

### 1. The `<trace>` Tag: Instant Deep Context (No Blind Spots)
Instead of returning a flat text chain, Textify's `<trace>` parses the AST, extracts the root function, and recursively pulls the **actual implementation code** of internal callees and referenced structs up to the specified `depth`.

```xml
<request>
  <!-- Recursively loads the target function, its callees' bodies, and referenced types -->
  <trace symbol="Typechecker.checkStmt" depth="2" />
</request>
```

**What `<trace>` delivers in a single payload:**
1. The full body of `checkStmt`.
2. The implementation bodies of helper functions called directly inside it.
3. The definitions of all referenced structs, interfaces, and types.
*(Eliminates 3–5 back-and-forth queries asking for internal callees).*

### 2. Surgical Switch-Case Branch Extraction: `<select>`
Avoid dumping massive 1,000-line parser or compiler files just to read one branch:

```xml
<request>
  <!-- Extract only the specific case handler within a large switch -->
  <select path="pkg/compiler/typechecker_expr.go" 
          fn="inferExpr" 
          case="*MemberExpr" />

  <!-- Extract specific methods on a type -->
  <select struct="Server" methods="Start,Shutdown" />
</request>
```

### 3. Patch-Ready Extraction (`for-patch="true"`)
Tells Textify to omit line-number prefixes (`12 | ...`) and render directly into a valid `<replace>` or `<search>` block:

```xml
<request>
  <file path="pkg/api/router.go" search="SetupRoutes" context="5" for-patch="true" />
</request>
```

### 4. Cross-References: `<xref>`
Finds definition sites and all read/write/call locations across the codebase:

```xml
<request>
  <xref symbol="Config" action="write" />
</request>
```

### 5. Intent-Based Composite Context: `<feature-context>`
Bundle complete end-to-end features (definitions + traces + touchpoints) in one request:

```xml
<request>
  <feature-context lang="go">
    <target-type name="compiler.StructDecl" />
    <entrypoint file="pkg/compiler/parser.go" symbol="Parser.parseDecl" depth="2" />
    <touchpoints pattern="TokenStruct" />
  </feature-context>
</request>
```

---

## 🤖 System Prompt for LLMs (Enforcing 1–2 Request Workflows)

Paste this prompt alongside your codebase signatures or output from `textify prompt`:

```markdown
You are assisting with code modifications managed by Textify.

CRITICAL INSTRUCTION FOR CONTEXT RETRIEVAL:
Minimize consecutive round trips. Aim to finish all inspections in AT MOST 1 request before providing the patch.
NEVER ask for symbols or files one by one if they are part of a call chain.

When inspecting code, use structural XML directives:
- Deep Execution Trace: <trace symbol="FuncName" depth="2" />
  (Pulls the target function, callees' implementation bodies, and referenced structs in 1 shot)
- Switch-case branches: <select path="file.go" fn="funcName" case="caseTarget" />
- Specific struct methods: <select struct="StructName" methods="Method1,Method2" />
- Cross-references: <xref symbol="SymbolName" action="read|write|call|all" />
- Composite bundles: <feature-context lang="go"><target-type name="T"/><entrypoint symbol="S" depth="2"/></feature-context>
- Text/Regex pattern: <grep pattern="keyword" context="10" glob="*.go" />
- Patch-ready clean block: Add for-patch="true" to any directive to strip line numbers.

When submitting changes, output ONLY atomic patch directives:
- Overwrite/Create: <file path="...">...</file>
- Surgical replacement:
  <replace path="..." [within="functionName"]>
  <search>
  exact code to match
  </search>
  <with>
  replacement code
  </with>
  </replace>
- AST Case clause replacement:
  <replace path="..." fn="..." case="...">
  <with>
      case target:
          ...
  </with>
  </replace>
- Delete: <delete path="..." />
```

---

## 🩹 Patch Directives & Patcher Format

Conversational commentary and markdown outside the tags are safely ignored.

### 1. AST Case Clause Replacement (`fn="..." case="..."`)
Deterministically locates and updates `case` clauses in Go files via AST without guessing tabs or indentation:
```xml
<replace path="pkg/compiler/typechecker_expr.go" fn="inferExpr" case="*MemberExpr">
<with>
    case *MemberExpr:
        return handleMember(ctx, e)
</with>
</replace>
```

### 2. Surgical Function Scope Bounding (`within="..."`)
Restricts the `<search>` block strictly to the specified function, preventing accidental replacements of identical code elsewhere:
```xml
<replace path="pkg/compiler/typechecker.go" within="Typechecker.inferExpr">
<search>
    if variants, isEnum := tc.Global.Enums[id.Name]; isEnum {
        ...
    }
</search>
<with>
    if variants, isEnum := tc.Global.Enums[id.Name]; isEnum {
        handleEnumV2(variants)
    }
</with>
</replace>
```

### 3. Search with Wildcards (`...`)
Match large blocks while ignoring intermediate lines:
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

### 4. Anchor Insertions (`after="..."` / `before="..."`)
```xml
<replace path="pkg/api/server.go" after="router := gin.Default()">
<with>
    router.Use(cors.Default())
</with>
</replace>
```

### 5. Create, Overwrite, or Delete Files
```xml
<!-- Create or Overwrite -->
<file path="cmd/app/main.go">
package main

func main() {}
</file>

<!-- Delete -->
<delete path="obsolete.go" />
```

---

## 🛠️ CLI Reference

| Command | Description |
| :--- | :--- |
| `textify init` | Scans workspace and creates `textify.yaml`. |
| `textify scan` | Discovers new folders and appends them to `textify.yaml`. |
| `textify start [flags]` | Bundles full project codebase into context file. |
| `textify signatures, sig [flags]` | Extracts types, structs, interfaces, and function headers. |
| `textify diff [flags]` | Bundles only modified, staged, or untracked Git files. |
| `textify trace <symbol> [flags]` | Traces function call flow and callee implementations (`--deep`, `--depth`). |
| `textify def <symbol> [flags]` | Extracts AST definition of function/struct/class (Go/TypeScript). |
| `textify grep <pattern> [flags]` | Searches pattern across workspace with context lines. |
| `textify load [file] [flags]` | Reads request XML and outputs requested slices, traces, and files. |
| `textify patch [file] [flags]` | Applies LLM patch bundle atomically using two-pass VFS. |
| `textify prompt` | Prints copy-pasteable instructions for the LLM. |
| `textify version` | Prints current version (`0.8.0`). |

### CLI Flags Cheat Sheet

* `-c`, `--clip`: Copy output to clipboard.
* `-ci`, `--clip-in`: Read input directly from clipboard (for `load` and `patch`).
* `-s`, `--stdout`: Print output to terminal.
* `-r`, `--redact`: Redact secrets and sensitive tokens.
* `-n`, `--dry-run`: Preview patch changes without modifying files on disk.
* `-f`, `--fix`: Enable fuzzy tolerance for slight indentation or variable drift during patching.

---

## 📄 License

Distributed under the MIT License. See [LICENSE](LICENSE) for details.
