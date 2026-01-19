
# Textify

**Textify** is a developer tool designed to prepare your codebase for Large Language Models (LLMs) like Claude, ChatGPT, and Gemini.

It flattens your project structure into a single, well-formatted text file. Instead of manually copy-pasting dozens of files, Textify scans your project, respects your `.gitignore`, filters out binaries, and applies custom inclusion rules defined in a simple configuration file.

---

## 🚀 Key Features

*   **Workflow Driven:** Simple `init` and `start` commands to manage your context.
*   **Directory-Scoped Configuration:** Define different rules for different folders (e.g., only include `.ts` files in `/frontend` but `.go` files in `/backend`).
*   **Smart Filtering:** Automatically respects `.gitignore` and detects binary files (images, PDFs, executables) to keep output clean.
*   **Force Includes:** Easily include files normally ignored (like `.env.example` or GitHub workflow configs) using the allowlist.
*   **Token Efficient:** strict extension filtering ensures you don't feed your LLM unnecessary build artifacts or lock files.

---

## 📦 Installation

### Option 1: Go Install (Recommended)
If you have Go installed, this is the fastest way to get started:

```bash
go install github.com/yourusername/textify/cmd/textify@latest
```

### Option 2: Build from Source
```bash
git clone https://github.com/yourusername/textify.git
cd textify
go build -o textify ./cmd/textify
```
*(Optional) Move the binary to your path:*
```bash
sudo mv textify /usr/local/bin/
```

---

## 📖 How to Use

Textify uses a two-step workflow: **Initialize** and **Start**.

### 1. Initialize
Navigate to your project root and run:
```bash
textify init
```
This scans your current directory structure and generates a `textify.toml` configuration file. It automatically detects subdirectories and sets up a default template for you.

### 2. Configure (Optional)
Open `textify.toml`. You will see sections for your root directory `.` and detected subdirectories. You can customize what gets included.

**Example `textify.toml`:**
```toml
output_file = "codebase.txt"

[dirs]
  # Root Directory Rules
  [dirs."."]
  extensions = ["md", "json", "yaml"] # Only these extensions in root
  include = [".env.example"]          # Force include this specific file

  # Backend Directory Rules
  [dirs."cmd"]
  extensions = ["go"]

  # Frontend Directory Rules
  [dirs."frontend"]
  extensions = ["ts", "tsx", "css"]   # Allow TypeScript and Styles
  include = ["package.json"]          # Explicitly include package.json
```

### 3. Generate
Once satisfied with your config, run:
```bash
textify start
```
This reads your configuration and generates `codebase.txt` (or whatever you named your output file).

---

## ⚙️ Configuration Guide

The `textify.toml` file gives you granular control over what gets sent to the LLM.

### `output_file`
The name of the generated text file.
```toml
output_file = "context_for_ai.txt"
```

### `[dirs]`
This section maps directory paths to rules.
*   **Keys:** The directory path relative to the project root (e.g., `.`, `src`, `src/components`).
*   **Inheritance:** If a subdirectory is not explicitly listed in `[dirs]`, it inherits the rules from its parent directory.

#### `extensions`
A list of file extensions to include.
*   If provided (e.g., `["go", "js"]`), **only** files with these extensions will be included.
*   If empty (`[]`), **all** text files not ignored by `.gitignore` will be included.

#### `include`
A list of specific files or folders to **Force Include**, regardless of extension rules or `.gitignore`.
*   Useful for including `.env` files, specific config files in build folders, or dotfiles.
*   Supports standard glob patterns (e.g., `scripts/*.sh`).

---

## 🛡️ Default Exclusions

Textify includes hardcoded logic to prevent scanning itself or common noise:
*   **Always Ignored:** `.git` folder, `textify.toml`, and the defined `output_file`.
*   **Binaries:** Automatically detects and skips non-text files (images, compiled binaries).
*   **Gitignore:** Respects your project's `.gitignore` rules unless overridden by the `include` list in TOML.

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