package segmentedreply

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/service"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools"
)

//go:embed config.json
var defaultConfigJSON []byte

const (
	maxContentSize     = 5 * 1024 * 1024
	minSegmentLength   = 50
	maxSegmentLength   = 5000
	maxIntervalSeconds = 10.0
)

type Config struct {
	ForceSegmentedReply    bool              `json:"force_segmented_reply"`
	IntervalSeconds        float64           `json:"interval_seconds"`
	SegmentLengthThreshold int               `json:"segment_length_threshold"`
	MaxSegments            int               `json:"max_segments"`
	OutputDir              string            `json:"output_dir"`
	SplitWords             []string          `json:"split_words"`
	ContentFilter          ContentFilter     `json:"content_filter"`
}

type ContentFilter struct {
	BlockedWords []string          `json:"blocked_words"`
	ReplaceRules map[string]string `json:"replace_rules"`
}

func init() {
	tools.Register(register)
}

func register(s *server.MCPServer, deps tools.Deps) {
	tool := mcp.NewTool("segmented_reply",
		mcp.WithDescription("Create segmented reply files under the data directory. Use this when the bot needs to send a long reply in multiple messages."),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("The full reply content to split into segments."),
		),
		mcp.WithString("session_id",
			mcp.Description("Session identifier used in file names (default 'default')."),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		content, err := req.RequireString("content")
		if err != nil {
			return tools.ResultError("invalid content: " + err.Error()), nil
		}
		sessionID := req.GetString("session_id", "default")

		slog.Info("segmented_reply requested", "session", sessionID, "bytes", len(content))
		result := run(deps.Cfg.DataDir, content, sessionID)

		if result["success"] == true {
			if files, ok := result["files"].([]string); ok {
				urls := make([]string, 0, len(files))
				for _, f := range files {
					if u := tools.DownloadURL(deps.Cfg, f); u != "" {
						urls = append(urls, u)
					}
				}
				if len(urls) > 0 {
					result["download_urls"] = urls
				}
			}
		}

		status := "failed"
		if result["success"] == true {
			status = "success"
		}
		message, _ := result["message"].(string)
		tools.RecordOperation(ctx, deps.Store, tools.SessionID(ctx), "segmented_reply", "", status, message, map[string]any{"segment_count": result["segment_count"]})

		return tools.Result(result), nil
	})
}

func run(dataDir, content, sessionID string) map[string]any {
	if strings.TrimSpace(content) == "" {
		return fail("Error: content cannot be empty.")
	}
	if len(content) > maxContentSize {
		return fail("Security Error: Content exceeds 5MB limit.")
	}

	var cfg Config
	_ = json.Unmarshal(defaultConfigJSON, &cfg)

	allowed, filteredContent, blockedWords := applyContentFilter(content, cfg.ContentFilter)
	if !allowed {
		return map[string]any{
			"success":       false,
			"message":       "Content blocked by segmented reply content_filter.blocked_words.",
			"blocked_words": blockedWords,
		}
	}

	threshold := cfg.SegmentLengthThreshold
	if threshold < minSegmentLength || threshold > maxSegmentLength {
		return fail(fmt.Sprintf("Error: segment_length_threshold must be between %d and %d.", minSegmentLength, maxSegmentLength))
	}

	intervalSeconds := cfg.IntervalSeconds
	if intervalSeconds < 0 {
		intervalSeconds = 0
	}
	if intervalSeconds > maxIntervalSeconds {
		intervalSeconds = maxIntervalSeconds
	}

	var segments []string
	if cfg.ForceSegmentedReply || len([]rune(filteredContent)) > threshold {
		segments = buildSegments(filteredContent, cfg.SplitWords, threshold)
	} else {
		segments = []string{strings.TrimSpace(filteredContent)}
	}

	if len(segments) == 0 {
		return fail("Error: no valid reply segments were generated.")
	}
	if len(segments) > cfg.MaxSegments {
		return fail(fmt.Sprintf("Error: too many segments (%d). Maximum is %d.", len(segments), cfg.MaxSegments))
	}

	outputDir := cfg.OutputDir
	if outputDir == "" {
		outputDir = "segmented_replies"
	}

	safeSession := safeSessionID(sessionID)
	timestamp := time.Now().Format("20060102_150405.000000")

	files := make([]string, 0, len(segments))
	for i, segment := range segments {
		fileName := fmt.Sprintf("%s_%s_%02d.txt", timestamp, safeSession, i+1)
		relPath := path.Join(outputDir, fileName)
		if err := service.SafeWrite(dataDir, relPath, segment); err != nil {
			return fail("Segmented reply failed: " + err.Error())
		}
		files = append(files, relPath)
		if i < len(segments)-1 && intervalSeconds > 0 {
			time.Sleep(time.Duration(intervalSeconds * float64(time.Second)))
		}
	}

	return map[string]any{
		"success":               true,
		"message":               fmt.Sprintf("Successfully created %d segmented reply file(s).", len(files)),
		"segment_count":         len(files),
		"interval_seconds":      intervalSeconds,
		"force_segmented_reply": cfg.ForceSegmentedReply,
		"files":                 files,
		"segments":              segments,
	}
}

func fail(message string) map[string]any {
	return map[string]any{"success": false, "message": message}
}

func applyContentFilter(content string, filter ContentFilter) (bool, string, []string) {
	var blocked []string
	for _, word := range filter.BlockedWords {
		if word != "" && strings.Contains(content, word) {
			blocked = append(blocked, word)
		}
	}
	if len(blocked) > 0 {
		return false, content, blocked
	}

	filtered := content
	for source, target := range filter.ReplaceRules {
		filtered = strings.ReplaceAll(filtered, source, target)
	}
	return true, filtered, nil
}

func buildSegments(content string, splitWords []string, threshold int) []string {
	markerSegments := splitByMarkers(content, splitWords)
	var segments []string
	for _, seg := range markerSegments {
		if len([]rune(seg)) <= threshold {
			segments = append(segments, seg)
		} else {
			segments = append(segments, splitByLength(seg, threshold)...)
		}
	}
	return segments
}

func splitByMarkers(content string, markers []string) []string {
	var filtered []string
	for _, m := range markers {
		if m != "" {
			filtered = append(filtered, m)
		}
	}
	if len(filtered) == 0 {
		return []string{strings.TrimSpace(content)}
	}

	escaped := make([]string, len(filtered))
	for i, m := range filtered {
		escaped[i] = regexp.QuoteMeta(m)
	}
	pattern := regexp.MustCompile(strings.Join(escaped, "|"))
	parts := pattern.Split(content, -1)

	var result []string
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			result = append(result, s)
		}
	}
	return result
}

func splitByLength(text string, maxLen int) []string {
	var segments []string
	runes := []rune(strings.TrimSpace(text))
	for len(runes) > 0 {
		if len(runes) <= maxLen {
			segments = append(segments, string(runes))
			break
		}
		idx := lastSeparatorIndex(runes, maxLen)
		if idx <= 0 {
			idx = maxLen
		} else {
			idx++
		}
		segments = append(segments, strings.TrimSpace(string(runes[:idx])))
		runes = runes[idx:]
	}

	var result []string
	for _, s := range segments {
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}

func lastSeparatorIndex(runes []rune, maxLen int) int {
	limit := maxLen
	if limit > len(runes) {
		limit = len(runes)
	}
	best := -1
	for i := 0; i < limit; i++ {
		if isSeparator(runes[i]) {
			best = i
		}
	}
	return best
}

func isSeparator(r rune) bool {
	switch r {
	case '\n', '。', '！', '？', '；', '，', '.', '!', '?', ' ':
		return true
	}
	return false
}

func safeSessionID(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		if (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	id := b.String()
	if len(id) > 64 {
		id = id[:64]
	}
	if id == "" {
		return "default"
	}
	return id
}
