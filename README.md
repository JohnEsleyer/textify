
# Textify

Textify is a CLI tool written in Go that converts a directory of source code (or any text files) into a single `.txt` file. This is particularly useful for creating context files to feed into Large Language Models (LLMs) or for documentation purposes.

It respects `.gitignore` rules, skips binary files, and provides flexible configuration for including or excluding specific file extensions.

## Features

- 🚀 **Single File Output**: Recursively walks a directory and concatenates files into one text file.
- 🙈 **GitIgnore Support**: Automatically respects `.gitignore` rules in the root directory.
- 🚫 **Binary Detection**: Automatically skips binary files (images, executables, etc.) to keep the output clean.
- ⚙️ **Configurable**: Whitelist or blacklist specific file extensions via `config.json`.
- 📊 **Word Counting**: Calculates the total word count of the generated codebase or any specified text file.

## Installation

### Prerequisites
- [Go 1.18+](https://go.dev/dl/)

### Build
Clone the repository and build the binary:

```bash
go build -o textify .
```

## Usage

### 1. Generate Codebase Text File
By default, running the tool scans the current directory and outputs to `codebase.txt`.

```bash
# Run with default settings
./textify

# Specify a root directory and output file
./textify -d /path/to/project -o my-project-code.txt

# Specify a custom configuration file
./textify -c my-config.json
```

**Flags:**
| Flag | Default | Description |
|------|---------|-------------|
| `-d` | `.` | The root directory to scan. |
| `-o` | `codebase.txt` | The output text file path. |
| `-c` | `config.json` | Path to the configuration JSON file. |

### 2. Word Count Command
You can use the `count` subcommand to count words in a text file without generating a new one.

```bash
# Count words in the default codebase.txt
./textify count

# Count words in a specific file
./textify count ./path/to/document.txt
```

*Note: When running the standard generation command, a word count summary is automatically displayed at the end.*

## Configuration

You can control which files are processed by creating a `config.json` file in the directory where you run the tool.

### Example `config.json`
```json
{
  "include_extensions": [],
  "exclude_extensions": [".exe", ".dll", ".so", ".jpg", ".png", ".sum", ".test"]
}
```

### Configuration Rules

1.  **exclude_extensions**:
    *   Files with these extensions will **always** be skipped.
    *   Case-insensitive (e.g., `.JPG` matches `.jpg`).

2.  **include_extensions**:
    *   If this list is **empty** `[]`: The tool includes all files (except those in `exclude_extensions` or `.gitignore`).
    *   If this list is **populated** (e.g., `[".go", ".md"]`): The tool will **only** include files matching these extensions. All other files are ignored.

## Output Format

The generated text file separates files with a header for easy reading/parsing:

```text
--------------------------------------------------
FILE: main.go
--------------------------------------------------

package main
... content ...


--------------------------------------------------
FILE: config/config.json
--------------------------------------------------

{
  ... content ...
}
```

## Development

To run the tool directly without building:

```bash
# Generate text file
go run . -d ./my-project

# Count words
go run . count codebase.txt
```
