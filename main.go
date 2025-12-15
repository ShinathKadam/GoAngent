package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

//
// ===================== CONFIG =====================
//

type Config struct {
	Server *ServerConfig        `yaml:"server,omitempty"`
	AI     *AIConfig            `yaml:"ai,omitempty"`
	Apps   map[string]AppConfig `yaml:"apps"`
}

type ServerConfig struct {
	Addr         string `yaml:"addr,omitempty"`
	DefaultLines int    `yaml:"default_lines,omitempty"`
	MaxLines     int    `yaml:"max_lines,omitempty"`
}

type AIConfig struct {
	BaseURL        string `yaml:"base_url"`
	APIKey         string `yaml:"api_key,omitempty"`
	TimeoutSeconds int    `yaml:"timeout_seconds,omitempty"`
}

type AppConfig struct {
	Logs map[string]LogTarget `yaml:"logs"`
}

type LogTarget struct {
	Type string `yaml:"type"`
	Path string `yaml:"path,omitempty"`
	URL  string `yaml:"url,omitempty"`
}

var (
	globalConfig *Config
)

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Server == nil {
		cfg.Server = &ServerConfig{}
	}
	if cfg.Server.DefaultLines <= 0 {
		cfg.Server.DefaultLines = 100
	}
	if cfg.Server.MaxLines <= 0 {
		cfg.Server.MaxLines = 1000
	}

	return &cfg, nil
}

//
// ===================== LOG SOURCES =====================
//

type LogSource interface {
	ReadLogs(ctx context.Context, lines int) (string, error)
}

type FileLogSource struct {
	Path string
}

func (f *FileLogSource) ReadLogs(ctx context.Context, lines int) (string, error) {
	file, err := os.Open(f.Path)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	var allLines []string
	scanner := bufio.NewScanner(file)

	// Read all lines
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		allLines = append(allLines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan file: %w", err)
	}

	// If file is empty:
	if len(allLines) == 0 {
		return "", nil
	}

	// Determine how many lines to return
	if lines <= 0 || lines > len(allLines) {
		lines = len(allLines)
	}

	// START from bottom
	start := len(allLines) - lines
	selected := allLines[start:]

	// Join
	result := strings.Join(selected, "\n") + "\n"
	return result, nil
}

type APILogSource struct {
	URL    string
	Client *http.Client
}

func (a *APILogSource) ReadLogs(ctx context.Context, lines int) (string, error) {
	if a.Client == nil {
		a.Client = &http.Client{
			Timeout: 10 * time.Second,
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	resp, err := a.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("remote API error: %s", resp.Status)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	return string(bodyBytes), nil
}

//
// ===================== HELPERS =====================
//

func parseLines(r *http.Request) int {
	linesStr := r.URL.Query().Get("lines")
	if linesStr == "" {
		if globalConfig != nil && globalConfig.Server != nil {
			return globalConfig.Server.DefaultLines
		}
		return 100
	}
	n, err := strconv.Atoi(linesStr)
	if err != nil || n <= 0 {
		if globalConfig != nil && globalConfig.Server != nil {
			return globalConfig.Server.DefaultLines
		}
		return 100
	}
	if globalConfig != nil && globalConfig.Server != nil && n > globalConfig.Server.MaxLines {
		n = globalConfig.Server.MaxLines
	}
	return n
}

func selectSourceFromQuery(r *http.Request) (LogSource, error) {
	source := r.URL.Query().Get("source")
	switch source {
	case "file":
		path := r.URL.Query().Get("path")
		if path == "" {
			return nil, fmt.Errorf("missing 'path' for file source")
		}
		return &FileLogSource{Path: path}, nil
	case "api":
		url := r.URL.Query().Get("url")
		if url == "" {
			return nil, fmt.Errorf("missing 'url' for api source")
		}
		return &APILogSource{
			URL: url,
			Client: &http.Client{
				Timeout: 10 * time.Second,
			},
		}, nil
	default:
		return nil, fmt.Errorf("invalid or missing 'source' (expected 'file' or 'api')")
	}
}

func sourceFromConfig(appName, logKey string) (LogSource, error) {
	if globalConfig == nil {
		return nil, fmt.Errorf("config not loaded; start server with -config flag")
	}

	appCfg, ok := globalConfig.Apps[appName]
	if !ok {
		return nil, fmt.Errorf("unknown app %q", appName)
	}

	target, ok := appCfg.Logs[logKey]
	if !ok {
		return nil, fmt.Errorf("unknown log key %q for app %q", logKey, appName)
	}

	switch target.Type {
	case "file":
		if target.Path == "" {
			return nil, fmt.Errorf("log %q for app %q: missing path", logKey, appName)
		}
		return &FileLogSource{Path: target.Path}, nil
	case "api":
		if target.URL == "" {
			return nil, fmt.Errorf("log %q for app %q: missing url", logKey, appName)
		}
		return &APILogSource{
			URL: target.URL,
			Client: &http.Client{
				Timeout: 10 * time.Second,
			},
		}, nil
	default:
		return nil, fmt.Errorf("log %q for app %q: invalid type %q (expected file or api)", logKey, appName, target.Type)
	}
}

// ===================== HUMAN-READABLE LOG PARSING =====================

func sanitizeBinary(data []byte) string {
	cleaned := make([]rune, 0, len(data))
	for _, b := range data {
		if b >= 32 && b <= 126 {
			cleaned = append(cleaned, rune(b))
		} else if b == '\n' || b == '\r' || b == '\t' {
			cleaned = append(cleaned, rune(b))
		}
	}
	return string(cleaned)
}

var (
	timeRegex     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2}`)
	javaStackLine = regexp.MustCompile(`^\s*at\s+[\w.$_]+\(.*:\d+\)$`)
)

func formatLogLine(line string) map[string]interface{} {
	result := map[string]interface{}{
		"raw": line,
	}

	if timeRegex.MatchString(line) {
		result["type"] = "timestamped"
	}

	if javaStackLine.MatchString(line) {
		result["type"] = "stacktrace_line"
	}

	switch {
	case strings.Contains(line, "ERROR"):
		result["severity"] = "ERROR"
	case strings.Contains(line, "WARN"):
		result["severity"] = "WARN"
	case strings.Contains(line, "INFO"):
		result["severity"] = "INFO"
	case strings.Contains(line, "DEBUG"):
		result["severity"] = "DEBUG"
	}

	return result
}

// ===================== HTTP HANDLERS =====================

func logsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	appName := q.Get("app")
	logKey := q.Get("log")

	var (
		sourceImpl LogSource
		err        error
	)

	switch {
	case appName != "" && logKey != "":
		sourceImpl, err = sourceFromConfig(appName, logKey)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	case q.Get("source") != "":
		sourceImpl, err = selectSourceFromQuery(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	default:
		http.Error(w, "must provide either app+log or source", http.StatusBadRequest)
		return
	}

	lines := parseLines(r)
	rawLogs, err := sourceImpl.ReadLogs(ctx, lines)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read logs: %v", err), http.StatusInternalServerError)
		return
	}

	clean := sanitizeBinary([]byte(rawLogs))

	var parsed interface{}
	if json.Unmarshal([]byte(clean), &parsed) == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(parsed)
		return
	}

	scanner := bufio.NewScanner(strings.NewReader(clean))
	var output []map[string]interface{}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		formatted := formatLogLine(line)
		output = append(output, formatted)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(output)
}

// ===================== /logs/analyze =====================
type AnalyzeRequest struct {
	OpenAIAPIKey string                   `json:"openai_api_key"`
	Logs         []map[string]interface{} `json:"logs"`
}

func logsAnalyzeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AnalyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Example: ignore OpenAI key, just return sample recommendations
	sampleResponse := map[string]interface{}{
		"recommendations": []map[string]string{
			{
				"title":       "Check DEBUG logs",
				"description": fmt.Sprintf("You sent %d log entries. Review DEBUG logs for unnecessary output.", len(req.Logs)),
				"severity":    "LOW",
			},
			{
				"title":       "Review HikariPool stats",
				"description": "Pool cleanup messages detected frequently; ensure proper connection management.",
				"severity":    "MEDIUM",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sampleResponse)
}

// ===================== /logs/apply-patch =====================
type ApplyPatchRequest struct {
	Recommendations []map[string]string `json:"recommendations"`
}

func applyPatchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ApplyPatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Simulate checking if files are remote
	remote := true // change to false to simulate local files

	var resp map[string]string
	if remote {
		resp = map[string]string{
			"status":  "success",
			"message": "Applied recommendations to remote files",
		}
	} else {
		resp = map[string]string{
			"status":  "skipped",
			"message": "Files are local; no changes applied",
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

// ===================== MAIN =====================

func main() {
	addrFlag := flag.String("addr", "127.0.0.1:8080", "HTTP listen address")
	configPath := flag.String("config", "", "path to YAML config file")
	flag.Parse()

	if *configPath != "" {
		cfg, err := loadConfig(*configPath)
		if err != nil {
			fmt.Printf("failed to load config: %v\n", err)
			os.Exit(1)
		}
		globalConfig = cfg
		fmt.Println("config loaded from", *configPath)
	}

	addr := *addrFlag
	if globalConfig != nil && globalConfig.Server != nil && globalConfig.Server.Addr != "" && *addrFlag == ":8080" {
		addr = globalConfig.Server.Addr
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/logs", logsHandler)
	mux.HandleFunc("/logs/analyze", logsAnalyzeHandler)
	mux.HandleFunc("/logs/apply-patch", applyPatchHandler)
	mux.HandleFunc("/health", healthHandler)

	fmt.Printf("Starting log agent on %s\n", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Printf("server error: %v\n", err)
	}
}
