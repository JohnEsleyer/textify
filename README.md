# Textify

**Textify** is a professional-grade CLI tool designed to "flatten" complex directory structures into a single, well-formatted text document. 

It is specifically built for developers who need to feed their codebase into Large Language Models (LLMs) like ChatGPT, Claude, or Gemini. Instead of manually copy-pasting dozens of files, Textify gathers your entire project—respecting your `.gitignore` and skipping binary noise—into one clean file with clear headers.

---

## 🚀 Key Features

-   **AI-Ready Output:** Automatically labels every code block with its relative file path, allowing LLMs to understand your project architecture instantly.
-   **Smart Filtering:** Native support for `.gitignore`. It automatically excludes dependencies (like `node_modules`) and build artifacts.
-   **Force-Include Override:** Use the `-i` flag to include specific files (like `.env.example` or hidden configs) that are normally ignored.
-   **Binary Protection:** Automatically detects and skips images, PDFs, and compiled binaries (like `.exe` or `.pyc`) to keep your output clean.
-   **Self-Aware Exclusions:** Hardcoded logic to ensure it never "scans itself." It automatically ignores common output names like `codebase.txt` and `textify.txt`.

---

## 🛠 How to Build

To build the project locally, ensure you have [Go](https://go.dev/doc/install) installed (1.18+).

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/yourusername/textify.git
    cd textify
    ```

2.  **Compile the binary:**
    ```bash
    go build -o textify ./cmd/textify
    ```
    This creates a `textify` executable in your current folder.

---

## 🌍 Global Installation (Easy Access)

To use `textify` from any directory on your system, you can move it to a global path and update your shell configuration.

### Option 1: The Go Way (Recommended)
This is the fastest method. Go will compile and place the binary in your `$GOPATH/bin`.

```bash
go install ./cmd/textify
```

If you haven't added Go's bin folder to your path yet, add this to your `~/.bashrc`:
```bash
export PATH=$PATH:$(go env GOPATH)/bin
```

### Option 2: Manual Move (Global Path)
If you want to move it to a standard system directory:

1.  **Move the binary:**
    ```bash
    sudo mv textify /usr/local/bin/
    ```

2.  **Ensure `/usr/local/bin` is in your `~/.bashrc`:**
    Open your `~/.bashrc` file:
    ```bash
    nano ~/.bashrc
    ```
    Add the following line at the bottom:
    ```bash
    export PATH="/usr/local/bin:$PATH"
    ```

3.  **Apply the changes:**
    ```bash
    source ~/.bashrc
    ```

Now you can simply type `textify` from anywhere!

---

## 📖 Usage

### Basic Scan
Generates a `codebase.txt` in the current directory containing all text files in the project.
```bash
textify -d .
```

### Custom Output and Force Includes
Include a `.env` file that is normally ignored and save the output to a specific path.
```bash
textify -d ./my-project -o my_codebase.txt -i ".env"
```

### Flags
- `-d`: The root directory to scan (default: `.`)
- `-o`: The name of the output text file (default: `codebase.txt`)
- `-i`: Comma-separated list of patterns to force include (e.g., `"*.env,secrets.yml"`)

---

## 🧪 Testing

Textify comes with a robust test suite to ensure filtering and binary detection work as expected.

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...
```

---

## 📁 Project Structure

- `cmd/textify`: CLI entry point and flag parsing.
- `internal/scanner`: Core logic for directory walking and filtering.
- `internal/fileutil`: Utilities for binary detection and file safety.