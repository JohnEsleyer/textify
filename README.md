Here is the updated `README.md` reflecting the new features.

```markdown
# Textify

Textify is a lightweight CLI tool written in Go that converts a directory of source code (or text files) into a single `.txt` file. 

It is primarily designed to help developers strictly format entire codebases to provide context for **Large Language Models (LLMs)** like ChatGPT, Claude, or Llama, allowing for better code analysis and refactoring suggestions.

## ✨ Features

- **📦 Single File Output**: Recursively walks a directory tree and concatenates all files into one readable text file.
- **🙈 GitIgnore Support**: Automatically respects `.gitignore` rules to exclude build artifacts and hidden files.
- **📚 Context Injection (`docs/`)**: Automatically detects a root `docs` folder and includes its content **even if it is git-ignored**. This allows you to feed external documentation to the LLM without cluttering your git history.
- **⚙️ Smart Configuration**: Auto-generates a `textify.json` file to manage included or excluded file extensions.
- **🚫 Binary Protection**: Automatically detects and skips binary files (images, executables) to keep the output clean.
- **📊 Word Counter**: Includes a built-in command to count words in your codebase, helping you estimate token usage.

## 🛠️ Installation

### Prerequisites
- [Go 1.18+](https://go.dev/dl/) installed on your machine.

### Build from Source

1. Clone the repository:
   ```bash
   git clone https://github.com/yourusername/textify.git
   cd textify
   ```

2. Build the binary:
   ```bash
   go build -o textify .
   ```

3. (Optional) Move to your PATH:
   ```bash
   mv textify /usr/local/bin/
   ```

## 🚀 Usage

### 1. Generate Codebase File
Run the tool in the root of your project. If `textify.json` does not exist, it will be created automatically with default settings.

```bash
# Run in current directory, outputs to codebase.txt
./textify

# Scan a specific directory
./textify -d /path/to/my/project

# Output to a specific filename
./textify -o full_context.txt
```

### 2. Count Words
You can use the `count` subcommand to check the word count of a file without regenerating it. This is useful for checking if your context fits within an LLM's context window.

```bash
# Count words in the default output file (codebase.txt)
./textify count

# Count words in a specific file
./textify count my_context.txt
```

*Note: A word count summary is also displayed automatically after every generation run.*

## 📚 Injecting External Context (`docs/`)

Textify has a special feature for handling external documentation.

If you create a folder named `docs` in the root of your project, Textify will **always** include its contents in the output file, **even if `docs/` is listed in your `.gitignore`**.

### Why is this useful?
You often need to provide an LLM with context about libraries or frameworks you are using (e.g., "Here is the documentation for the specific physics engine I am using").

**The Workflow:**
1. Create a `docs/` folder in your project.
2. Add `docs/` to your `.gitignore` (so you don't commit huge text files to your repo).
3. Paste text files, markdown, or API specs into that folder.
4. Run `textify`.

Textify will see that you have a `docs` folder, bypass the gitignore rule specifically for that folder, and include that context at the top of your prompt file.

## ⚙️ Configuration (`textify.json`)

On the first run, Textify creates a `textify.json` file in the working directory. You can edit this to control exactly what gets included.

```json
{
  "include_extensions": [],
  "exclude_extensions": [
    ".exe", ".dll", ".so", ".test", 
    ".jpg", ".png", ".gif", ".sum"
  ]
}
```

### How it works:

1.  **`include_extensions` (Whitelist)**
    *   If this list is **populated** (e.g., `[".go", ".js", ".md"]`), Textify will **ONLY** process files with these extensions. All other files are ignored.
    *   If this list is **empty** `[]` (default), Textify will process **ALL** files.

2.  **`exclude_extensions` (Blacklist)**
    *   Files matching these extensions are always skipped.
    *   Useful for skipping lock files (`.sum`, `.lock`) or media assets.

## 📝 CLI Options

| Flag | Default | Description |
|------|---------|-------------|
| `-d` | `.` | The root directory to scan. |
| `-o` | `codebase.txt` | The output filename. |
| `-c` | `textify.json` | Path to the configuration file. |

## 📄 Output Format

The generated file is formatted with clear separators to help LLMs distinguish between different files:

```text
--------------------------------------------------
FILE: docs/library_reference.txt
--------------------------------------------------

API Documentation v2.0...


--------------------------------------------------
FILE: src/main.go
--------------------------------------------------

package main

func main() {
    println("Hello World")
}
```
```