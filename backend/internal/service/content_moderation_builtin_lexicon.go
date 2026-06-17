package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	_ "embed"

	"github.com/Karrecy/sensitive-go/dict"
)

//go:embed content_moderation_builtin_words.jsonl
var contentModerationBuiltinLexiconData []byte

type contentModerationBuiltinLexiconEntry struct {
	Text     string `json:"text"`
	Category string `json:"category"`
	Level    string `json:"level"`
}

func loadContentModerationBuiltinLexicon() ([]dict.Word, error) {
	if len(contentModerationBuiltinLexiconData) == 0 {
		return nil, fmt.Errorf("embedded builtin lexicon is empty")
	}
	scanner := bufio.NewScanner(bytes.NewReader(contentModerationBuiltinLexiconData))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	words := make([]dict.Word, 0, 130000)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var entry contentModerationBuiltinLexiconEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return nil, fmt.Errorf("parse builtin lexicon line %d: %w", lineNo, err)
		}
		text := strings.TrimSpace(entry.Text)
		if text == "" {
			return nil, fmt.Errorf("builtin lexicon line %d has empty text", lineNo)
		}
		category, err := contentModerationDictCategoryFromString(entry.Category)
		if err != nil {
			return nil, fmt.Errorf("builtin lexicon line %d: %w", lineNo, err)
		}
		level, err := contentModerationDictLevelFromString(entry.Level)
		if err != nil {
			return nil, fmt.Errorf("builtin lexicon line %d: %w", lineNo, err)
		}
		words = append(words, dict.Word{
			Text:     text,
			Category: category,
			Level:    level,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan builtin lexicon: %w", err)
	}
	return words, nil
}

func contentModerationDictCategoryFromString(category string) (dict.Category, error) {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "political":
		return dict.CategoryPolitical, nil
	case "pornographic":
		return dict.CategoryPornographic, nil
	case "violence":
		return dict.CategoryViolence, nil
	case "abuse":
		return dict.CategoryAbuse, nil
	case "ad":
		return dict.CategoryAd, nil
	case "illegal":
		return dict.CategoryIllegal, nil
	case "other":
		return dict.CategoryOther, nil
	default:
		return dict.CategoryOther, fmt.Errorf("unknown category %q", category)
	}
}

func contentModerationDictLevelFromString(level string) (dict.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "low":
		return dict.LevelLow, nil
	case "medium":
		return dict.LevelMedium, nil
	case "high":
		return dict.LevelHigh, nil
	case "critical":
		return dict.LevelCritical, nil
	default:
		return dict.LevelLow, fmt.Errorf("unknown level %q", level)
	}
}
