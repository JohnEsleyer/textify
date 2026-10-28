package scanner

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/JohnEsleyer/textify/internal/config"
	"github.com/JohnEsleyer/textify/internal/fileutil"
)

// FileRequest describes a target file and optional sub-slice requested by the LLM.
type FileRequest struct {
	Path    string
	Start   int // 1-based start line (0 if full file)
	End     int // 1-based end line (0 if full file)
	Symbols []string
}

var (
	xmlFileTagRegex = regexp.MustCompile("(?i)<file\\s+([^>]+)(?:/>|>(.*?)\x3c/file\x3e)")
	xmlAttrPath     = regexp.MustCompile(`(?i)\bpath=["']([^"']+)["']`)
	xmlAttrLines    = regexp.MustCompile(`(?i)\blines=["'](\d+)(?:-(\d+))?["']`)
	xmlAttrSymbols  = regexp.MustCompile(`(?i)\bsymbols?=["']([^"']+)["']`)

	// Markdown bullet patterns like: - path/to/file.go:50-120
	mdLinePattern = regexp.MustCompile(`(?m)^\s*[-*]\s+([^\s:]+)(?::(\d+)(?:-(\d+))?)?`)
)

// ParseRequest parses an LLM response containing file requests.
// Supports XML tags (<request><file path="..." lines="X-Y"/></request>)
// as well as markdown bullet lists (- path/to/file.ext:X-Y).
func ParseRequest(input string) []FileRequest {
	var requests []FileRequest
	seen := make(map[string]bool)

	// 1. Try XML Tag Matching first
	matches := xmlFileTagRegex.FindAllStringSubmatch(input, -1)
	for _, m := range matches {
		attrs := m[1]
		pathMatch := xmlAttrPath.FindStringSubmatch(attrs)
		if len(pathMatch) < 2 {
			continue
		}

		req := FileRequest{
			Path: filepath.ToSlash(strings.TrimSpace(pathMatch[1])),
		}

		if linesMatch := xmlAttrLines.FindStringSubmatch(attrs); len(linesMatch) >= 2 {
			req.Start, _ = strconv.Atoi(linesMatch[1])
			if len(linesMatch) >= 3 && linesMatch[2] != "" {
				req.End, _ = strconv.Atoi(linesMatch[2])
			} else {
				req.End = req.Start
			}
		}

		if symMatch := xmlAttrSymbols.FindStringSubmatch(attrs); len(symMatch) >= 2 {
			rawSyms := strings.Split(symMatch[1], ",")
			for _, s := range rawSyms {
				trimmed := strings.TrimSpace(s)
				if trimmed != "" {
					req.Symbols = append(req.Symbols, trimmed)
				}
			}
		}

		key := fmt.Sprintf("%s:%d-%d:%s", req.Path, req.Start, req.End, strings.Join(req.Symbols, ","))
		if !seen[key] {
			seen[key] = true
			requests = append(requests, req)
		}
	}

	if len(requests) > 0 {
		return requests
	}

	// 2. Fallback to Markdown bullet lists
	mdMatches := mdLinePattern.FindAllStringSubmatch(input, -1)
	for _, m := range mdMatches {
		p := filepath.ToSlash(strings.TrimSpace(m[1]))
		if strings.HasPrefix(p, "http") || strings.HasPrefix(p, "#") || p == "" {
			continue
		}

		req := FileRequest{Path: p}
		if len(m) >= 3 && m[2] != "" {
			req.Start, _ = strconv.Atoi(m[2])
			if len(m) >= 4 && m[3] != "" {
				req.End, _ = strconv.Atoi(m[3])
			} else {
				req.End = req.Start
			}
		}

		key := fmt.Sprintf("%s:%d-%d", req.Path, req.Start, req.End)
		if !seen[key] {
			seen[key] = true
			requests = append(requests, req)
		}
	}

	return requests
}

// LoadRequests reads the requested files, extracts line slices or symbols,
// applies formatting and secret redaction, and writes the output.
func LoadRequests(rootPath string, cfg *config.Config, requests []FileRequest, writer io.Writer, opts ScanOptions) (*ScanResult, error) {
	bufWriter := bufio.NewWriter(writer)
	defer bufWriter.Flush()

	format := "raw"
	if cfg.Format != "" {
		format = cfg.Format
	}
	if opts.Format != "" {
		format = opts.Format
	}

	result := &ScanResult{}

	for _, req := range requests {
		absPath := filepath.Join(rootPath, filepath.FromSlash(req.Path))

		isBin, err := fileutil.IsBinary(absPath)
		if err != nil || isBin {
			continue
		}

		rawBytes, err := os.ReadFile(absPath)
		if err != nil {
			if !opts.Stdout {
				fmt.Printf("⚠️ Warning: Could not read requested file '%s': %v\n", req.Path, err)
			}
			continue
		}

		content, startLine, endLine := extractSlice(string(rawBytes), req.Start, req.End, req.Symbols)

		redactedCount := 0
		if opts.RedactSecrets {
			content, redactedCount = fileutil.RedactSecrets(content)
		}

		formatted := renderFormattedEntry(req.Path, content, format, startLine, endLine)

		n, writeErr := bufWriter.WriteString(formatted)
		if writeErr != nil {
			return nil, writeErr
		}

		result.FilesCount++
		result.TotalBytes += int64(n)
		result.RedactedSecrets += redactedCount

		if !opts.Stdout {
			rangeDesc := "full"
			if startLine > 0 && endLine > 0 {
				rangeDesc = fmt.Sprintf("lines %d-%d", startLine, endLine)
			}
			fmt.Printf("Added: %s (%s)\n", req.Path, rangeDesc)
		}
	}

	result.ApproxTokens = int(result.TotalBytes / 4)
	return result, nil
}

func extractSlice(content string, start, end int, symbols []string) (string, int, int) {
	lines := strings.Split(content, "\n")
	totalLines := len(lines)

	if len(symbols) > 0 {
		return extractSymbols(lines, symbols)
	}

	if start <= 0 && end <= 0 {
		var sb strings.Builder
		for i, line := range lines {
			sb.WriteString(fmt.Sprintf("%5d | %s\n", i+1, line))
		}
		return sb.String(), 1, totalLines
	}

	if start < 1 {
		start = 1
	}
	if end > totalLines || end < start {
		end = totalLines
	}

	var sb strings.Builder
	for i := start - 1; i < end && i < totalLines; i++ {
		sb.WriteString(fmt.Sprintf("%5d | %s\n", i+1, lines[i]))
	}

	return sb.String(), start, end
}

func extractSymbols(lines []string, symbols []string) (string, int, int) {
	var sb strings.Builder
	inTarget := false
	braceCount := 0
	minLine := len(lines)
	maxLine := 1

	for idx, line := range lines {
		lineNum := idx + 1
		trimmed := strings.TrimSpace(line)

		if !inTarget {
			for _, sym := range symbols {
				if strings.Contains(trimmed, sym) && (strings.HasPrefix(trimmed, "func ") ||
					strings.HasPrefix(trimmed, "type ") ||
					strings.HasPrefix(trimmed, "class ") ||
					strings.HasPrefix(trimmed, "def ") ||
					strings.HasPrefix(trimmed, "struct ") ||
					strings.HasPrefix(trimmed, "interface ")) {
					inTarget = true
					braceCount = 0
					if lineNum < minLine {
						minLine = lineNum
					}
					break
				}
			}
		}

		if inTarget {
			sb.WriteString(fmt.Sprintf("%5d | %s\n", lineNum, line))
			if lineNum > maxLine {
				maxLine = lineNum
			}

			for _, ch := range trimmed {
				if ch == '{' {
					braceCount++
				} else if ch == '}' {
					braceCount--
				}
			}

			if braceCount <= 0 && strings.Contains(trimmed, "}") {
				inTarget = false
				sb.WriteString("\n")
			}
		}
	}

	if sb.Len() == 0 {
		return extractSlice(strings.Join(lines, "\n"), 0, 0, nil)
	}

	return sb.String(), minLine, maxLine
}
