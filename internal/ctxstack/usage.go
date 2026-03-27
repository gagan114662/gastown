package ctxstack

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func InferUsage(cwd string, maxTokens int) (*UsageSample, error) {
	if maxTokens <= 0 {
		maxTokens = DefaultMaxContextTokens
	}
	if override := strings.TrimSpace(os.Getenv("GT_CONTEXT_BUDGET_TOKENS")); override != "" {
		used, err := strconv.Atoi(override)
		if err != nil {
			return nil, fmt.Errorf("parsing GT_CONTEXT_BUDGET_TOKENS: %w", err)
		}
		return buildUsageSample(used, maxTokens, "env"), nil
	}

	transcriptPath := strings.TrimSpace(os.Getenv("GT_TRANSCRIPT_PATH"))
	if transcriptPath == "" {
		transcriptPath = findLatestClaudeTranscript(cwd)
	}
	if transcriptPath == "" {
		return nil, nil
	}

	used, err := readTranscriptUsage(transcriptPath)
	if err != nil {
		return nil, err
	}
	if used <= 0 {
		return nil, nil
	}
	return buildUsageSample(used, maxTokens, transcriptPath), nil
}

func buildUsageSample(used, maxTokens int, source string) *UsageSample {
	if maxTokens <= 0 {
		maxTokens = DefaultMaxContextTokens
	}
	return &UsageSample{
		UsedTokens: used,
		MaxTokens:  maxTokens,
		Ratio:      float64(used) / float64(maxTokens),
		Source:     source,
	}
}

func findLatestClaudeTranscript(cwd string) string {
	if cwd == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	projectDir := filepath.Join(home, ".claude", "projects", strings.ReplaceAll(cwd, "/", "-"))
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return ""
	}
	var latestPath string
	var latestMod int64
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Unix() > latestMod {
			latestMod = info.ModTime().Unix()
			latestPath = filepath.Join(projectDir, entry.Name())
		}
	}
	return latestPath
}

func readTranscriptUsage(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	type usageEnvelope struct {
		Type    string `json:"type"`
		Message struct {
			Usage struct {
				InputTokens              int `json:"input_tokens"`
				CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
				CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			} `json:"usage"`
		} `json:"message"`
	}

	used := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var env usageEnvelope
		if err := json.Unmarshal([]byte(line), &env); err != nil {
			continue
		}
		if env.Type != "assistant" {
			continue
		}
		total := env.Message.Usage.InputTokens + env.Message.Usage.CacheCreationInputTokens + env.Message.Usage.CacheReadInputTokens
		if total > 0 {
			used = total
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return used, nil
}
