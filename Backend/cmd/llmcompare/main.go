// Command llmcompare is a development-only tool that runs the same journal
// entries through every configured provider so their reflections can be
// compared side by side. It never runs in production and never writes output
// anywhere but stdout.
//
// Usage (from the Backend module root):
//
//	go run ./cmd/llmcompare
//	go run ./cmd/llmcompare -file path/to/entries.txt -tags "Calm,Overwhelmed"
//
// A provider is skipped (reporting that it is not configured) when its key is
// unset, so only ANTHROPIC_API_KEY and/or GEMINI_API_KEY are required.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"Backend/internal/llm"
)

func main() {
	file := flag.String("file", "cmd/llmcompare/testdata/messages.txt", "file with one journal entry per line")
	tags := flag.String("tags", "", "comma-separated mood tags applied to every entry")
	flag.Parse()

	entries, err := readEntries(*file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "llmcompare: reading entries:", err)
		os.Exit(1)
	}
	if len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "llmcompare: no entries found in", *file)
		os.Exit(1)
	}

	moodTags := splitTags(*tags)

	providers := []struct {
		name      string
		reflector llm.Reflector
	}{
		{"claude", llm.NewClaude(os.Getenv("ANTHROPIC_API_KEY"), envModel("CLAUDE_MODEL", llm.DefaultClaudeModel))},
		{"gemini", llm.NewGemini(os.Getenv("GEMINI_API_KEY"), envModel("GEMINI_MODEL", llm.DefaultGeminiModel))},
	}

	fmt.Printf("Comparing %d entries across %d providers\n", len(entries), len(providers))
	for i, entry := range entries {
		fmt.Printf("\n=== [%d] %s\n", i+1, entry)
		for _, p := range providers {
			compare(p.name, p.reflector, entry, moodTags)
		}
	}
}

// compare runs one provider and prints its reflection or a coarse error. The
// journal content is printed only as the labelled input above, never as part of
// an error.
func compare(name string, reflector llm.Reflector, entry string, tags []string) {
	ctx, cancel := context.WithTimeout(context.Background(), llm.RequestTimeout)
	defer cancel()

	out, err := reflector.Reflect(ctx, entry, tags)
	if err != nil {
		fmt.Printf("  %-7s error: %v\n", name, err)
		return
	}
	fmt.Printf("  %-7s %s\n", name, out)
}

func readEntries(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		entries = append(entries, line)
	}
	return entries, scanner.Err()
}

func splitTags(raw string) []string {
	var tags []string
	for _, t := range strings.Split(raw, ",") {
		if t = strings.TrimSpace(t); t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

func envModel(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}
