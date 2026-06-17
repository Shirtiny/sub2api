package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type lexiconEntry struct {
	Text       string `json:"text"`
	Category   string `json:"category"`
	Level      string `json:"level"`
	ReviewedBy string `json:"reviewed_by,omitempty"`
	ReviewNote string `json:"review_note,omitempty"`
}

func main() {
	cmd := "generate"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "generate":
		generate()
	case "stats":
		if err := stats(os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "validate":
		if err := validate(os.Stdin); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "apply-review":
		if len(os.Args) != 4 {
			fmt.Fprintf(os.Stderr, "usage: %s apply-review <lexicon.jsonl> <review.jsonl>\n", os.Args[0])
			os.Exit(2)
		}
		if err := applyReviewInPlace(os.Args[2], os.Args[3]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "usage: %s [generate|stats|validate|apply-review]\n", os.Args[0])
		os.Exit(2)
	}
}

func generate() {
	writer := bufio.NewWriterSize(os.Stdout, 1024*1024)
	defer writer.Flush()

	for _, word := range service.ContentModerationTaggedBuiltinWordsForExport() {
		entry := lexiconEntry{
			Text:     word.Text,
			Category: word.Category.String(),
			Level:    word.Level.String(),
		}
		raw, err := json.Marshal(entry)
		if err != nil {
			fmt.Fprintf(os.Stderr, "marshal %q: %v\n", word.Text, err)
			os.Exit(1)
		}
		if _, err := writer.Write(raw); err != nil {
			fmt.Fprintf(os.Stderr, "write %q: %v\n", word.Text, err)
			os.Exit(1)
		}
		if err := writer.WriteByte('\n'); err != nil {
			fmt.Fprintf(os.Stderr, "write newline: %v\n", err)
			os.Exit(1)
		}
	}
}

func stats(r io.Reader, w io.Writer) error {
	levelCounts := map[string]int{}
	categoryCounts := map[string]int{}
	total := 0
	if err := scanEntries(r, func(_ int, entry lexiconEntry) error {
		total++
		levelCounts[strings.TrimSpace(entry.Level)]++
		categoryCounts[strings.TrimSpace(entry.Category)]++
		return nil
	}); err != nil {
		return err
	}
	fmt.Fprintf(w, "total %d\n", total)
	writeCounts(w, "level", levelCounts)
	writeCounts(w, "category", categoryCounts)
	return nil
}

func validate(r io.Reader) error {
	seen := 0
	return scanEntries(r, func(lineNo int, entry lexiconEntry) error {
		seen++
		if strings.TrimSpace(entry.Text) == "" {
			return fmt.Errorf("line %d: text is required", lineNo)
		}
		if !knownCategory(entry.Category) {
			return fmt.Errorf("line %d: unknown category %q", lineNo, entry.Category)
		}
		if !knownLevel(entry.Level) {
			return fmt.Errorf("line %d: unknown level %q", lineNo, entry.Level)
		}
		return nil
	})
}

func applyReview(lexiconPath string, reviewPath string, w io.Writer) error {
	reviews := map[string]lexiconEntry{}
	reviewFile, err := os.Open(filepath.Clean(reviewPath))
	if err != nil {
		return fmt.Errorf("open review: %w", err)
	}
	defer reviewFile.Close()
	if err := scanEntries(reviewFile, func(lineNo int, entry lexiconEntry) error {
		if strings.TrimSpace(entry.Text) == "" {
			return fmt.Errorf("review line %d: text is required", lineNo)
		}
		if !knownCategory(entry.Category) {
			return fmt.Errorf("review line %d: unknown category %q", lineNo, entry.Category)
		}
		if !knownLevel(entry.Level) {
			return fmt.Errorf("review line %d: unknown level %q", lineNo, entry.Level)
		}
		reviews[entry.Text] = entry
		return nil
	}); err != nil {
		return err
	}

	lexiconFile, err := os.Open(filepath.Clean(lexiconPath))
	if err != nil {
		return fmt.Errorf("open lexicon: %w", err)
	}
	defer lexiconFile.Close()

	writer := bufio.NewWriterSize(w, 1024*1024)
	defer writer.Flush()

	applied := make(map[string]int)
	if err := scanEntries(lexiconFile, func(_ int, entry lexiconEntry) error {
		if review, ok := reviews[entry.Text]; ok {
			entry.Category = review.Category
			entry.Level = review.Level
			applied[entry.Text]++
		}
		raw, err := json.Marshal(lexiconEntry{
			Text:     entry.Text,
			Category: entry.Category,
			Level:    entry.Level,
		})
		if err != nil {
			return fmt.Errorf("marshal %q: %w", entry.Text, err)
		}
		if _, err := writer.Write(raw); err != nil {
			return fmt.Errorf("write %q: %w", entry.Text, err)
		}
		return writer.WriteByte('\n')
	}); err != nil {
		return err
	}
	if len(applied) != len(reviews) {
		missing := make([]string, 0, len(reviews)-len(applied))
		for text := range reviews {
			if applied[text] == 0 {
				missing = append(missing, text)
			}
		}
		return fmt.Errorf("applied %d of %d review entries; missing: %s", len(applied), len(reviews), strings.Join(missing, ", "))
	}
	return nil
}

func applyReviewInPlace(lexiconPath string, reviewPath string) error {
	cleanLexiconPath := filepath.Clean(lexiconPath)
	tmpFile, err := os.CreateTemp(filepath.Dir(cleanLexiconPath), ".content-moderation-lexicon-*.jsonl")
	if err != nil {
		return fmt.Errorf("create temp lexicon: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if err := applyReview(cleanLexiconPath, reviewPath, tmpFile); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp lexicon: %w", err)
	}
	if err := os.Rename(tmpPath, cleanLexiconPath); err != nil {
		return fmt.Errorf("replace lexicon: %w", err)
	}
	return nil
}

func scanEntries(r io.Reader, fn func(lineNo int, entry lexiconEntry) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var entry lexiconEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return fmt.Errorf("line %d: parse: %w", lineNo, err)
		}
		if err := fn(lineNo, entry); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan: %w", err)
	}
	return nil
}

func writeCounts(w io.Writer, name string, counts map[string]int) {
	fmt.Fprintf(w, "%s\n", name)
	for _, key := range []string{"critical", "high", "medium", "low", "illegal", "pornographic", "ad", "abuse", "violence", "political", "other"} {
		if count, ok := counts[key]; ok {
			fmt.Fprintf(w, "%s %d\n", key, count)
			delete(counts, key)
		}
	}
	for key, count := range counts {
		fmt.Fprintf(w, "%s %d\n", key, count)
	}
}

func knownCategory(category string) bool {
	switch strings.TrimSpace(category) {
	case "political", "pornographic", "violence", "abuse", "ad", "illegal", "other":
		return true
	default:
		return false
	}
}

func knownLevel(level string) bool {
	switch strings.TrimSpace(level) {
	case "low", "medium", "high", "critical":
		return true
	default:
		return false
	}
}
