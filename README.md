# Textify

**Textify** is a developer tool designed to prepare your codebase for Large Language Models (LLMs) like Claude, ChatGPT, and Gemini.

It flattens your project structure into a single, well-formatted text file. Instead of manually copy-pasting dozens of files, Textify scans your project, respects your `.gitignore`, filters out binaries, and applies custom inclusion rules defined in a simple configuration file.

---

## 🚀 Key Features

*   **Smart Discovery:** Automatically detects directory structures and file extensions to pre-populate your configuration.
*   **Workflow Driven:** Simple `init`, `scan`, and `start` commands to manage your context.
*   **Directory-Scoped Configuration:** Define different rules for different folders (e.g., only include `.ts` files in `/frontend` but `.go` files in `/backend`).
*   **Granular Control:** Use the `enabled` flag to quickly include or exclude entire directory branches.
*   **Smart Filtering:** Automatically respects `.gitignore` and detects binary files (images, PDFs, executables) to keep output clean.
*   **Force Includes:** Easily include files normally ignored (like `.env.example` or GitHub workflow configs) using the allowlist.

---

## 📦 Installation

### Option 1: Go Install (Recommended)
If you have Go installed, this is the fastest way to get started:

```bash
go install github.com/JohnEsleyer/textify/cmd/textify@latest
```

### Option 2: Build from Source
```bash
git clone https://github.com/JohnEsleyer/textify.git
cd textify
go build -o textify ./cmd/textify
```
*(Optional) Move the binary to your path:*
```bash
sudo mv textify /usr/local/bin/
```

---

## 📖 How to Use

Textify uses a three-step workflow: **Initialize**, **Refine**, and **Start**.

### 1. Initialize
Navigate to your project root and run:
```bash
textify init
```
This scans your current directory structure, detects extensions used in each folder, and generates a `textify.yaml` configuration file. It automatically marks ignored folders (like `node_modules` or `dist`) as `enabled: false`.

### 2. Update (Optional)
If you add new directories to your project, you don't need to rebuild your config manually. Just run:
```bash
textify scan
```
This will detect new folders and append them to your `textify.yaml` while preserving your existing rules.

### 3. Configure (Optional)
Open `textify.yaml`. You can customize what gets included by toggling the `enabled` flag or modifying extensions.

**Example `textify.yaml`:**
```yaml
output_file: codebase.txt

dirs:
  # Root Directory Rules
  .:
    enabled: true
    extensions: 
      - md
      - json
      - yaml
    include:
      - .env.example

  # Backend Directory Rules
  cmd:
    enabled: true
    extensions:
      - go

  # Frontend Directory Rules
  frontend:
    enabled: true
    extensions:
      - ts
      - tsx
      - css
    include:
      - package.json

  # Excluded Directory
  node_modules:
    enabled: false
```

### 4. Generate
Once satisfied with your config, run:
```bash
textify start
```
This reads your configuration and generates `codebase.txt` (or whatever you named your output file).

---

## ⚙️ Configuration Guide

The `textify.yaml` file gives you granular control over what gets sent to the LLM.

### `output_file`
The name of the generated text file.
```yaml
output_file: context_for_ai.txt
```

### `dirs`
This section maps directory paths to rules.
*   **Keys:** The directory path relative to the project root (e.g., `.`, `src`, `src/components`).
*   **Inheritance:** If a subdirectory is not explicitly listed in `dirs`, it inherits the rules from its parent directory.

#### `enabled`
A boolean (`true`/`false`) that determines if the directory and all its children should be scanned.
*   If `false`, Textify will skip this entire branch.

#### `extensions`
A list of file extensions to include.
*   If provided (e.g., `[go, js]`), **only** files with these extensions will be included.
*   If empty, **all** text files not ignored by `.gitignore` will be included.

#### `include`
A list of specific files or folders to **Force Include**, regardless of extension rules or `.gitignore`.
*   Useful for including `.env` files, specific config files in build folders, or dotfiles.
*   Supports standard glob patterns (e.g., `scripts/*.sh`).

---

## 🛡️ Default Exclusions

Textify includes hardcoded logic to prevent scanning itself or common noise:
*   **Always Ignored:** `.git` folder, `textify.yaml`, and the defined `output_file`.
*   **Binaries:** Automatically detects and skips non-text files (images, compiled binaries).
*   **Gitignore:** Respects your project's `.gitignore` rules during `init` and `scan` to set default `enabled` states.

---

## 🤝 Contributing

1.  Fork the repository.
2.  Create your feature branch (`git checkout -b feature/amazing-feature`).
3.  Commit your changes (`git commit -m 'Add some amazing feature'`).
4.  Push to the branch (`git push origin feature/amazing-feature`).
5.  Open a Pull Request.

---

## 📄 License

Distributed under the MIT License. See `LICENSE` for more information.