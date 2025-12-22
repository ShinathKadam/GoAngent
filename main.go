package main

import (
	"bufio"
	"context"
	"encoding/binary"
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
	"unicode/utf16"

	"gopkg.in/yaml.v3"
)

//
// ================= CONFIG =================
//

type Config struct {
	Server *ServerConfig        `yaml:"server,omitempty"`
	Apps   map[string]AppConfig `yaml:"apps"`
}

type ServerConfig struct {
	Addr         string `yaml:"addr,omitempty"`
	DefaultLines int    `yaml:"default_lines,omitempty"`
	MaxLines     int    `yaml:"max_lines,omitempty"`
}

type AppConfig struct {
	Logs map[string]LogTarget `yaml:"logs"`
}

type LogTarget struct {
	Type    string `yaml:"type"`
	Path    string `yaml:"path,omitempty"`
	URL     string `yaml:"url,omitempty"`
	Service string `yaml:"service"`
}

var globalConfig *Config

//
// ================= STREAM MANAGER =================
//

var streamMgr = NewStreamManager(DefaultStreamConfig())

//
// ================= STREAM STATUS =================
//

type StreamStatus struct {
	Active    bool      `json:"active"`
	AppName   string    `json:"app_name,omitempty"`
	LogType   string    `json:"log_type,omitempty"`
	Path      string    `json:"path,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
}

var currentStream = &StreamStatus{}

//
// ================= OUTPUT SCHEMA =================
//

type LogOutput struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Service   string `json:"service"`
	Message   string `json:"message"`
}

//
// ================= UTF-16 FILE READER =================
//

func readFileAutoUTF(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	if len(data) > 2 && data[0] == 0xFF && data[1] == 0xFE {
		u16 := make([]uint16, (len(data)-2)/2)
		for i := range u16 {
			u16[i] = binary.LittleEndian.Uint16(data[2+i*2:])
		}
		return string(utf16.Decode(u16)), nil
	}

	if len(data) > 2 && data[0] == 0xFE && data[1] == 0xFF {
		u16 := make([]uint16, (len(data)-2)/2)
		for i := range u16 {
			u16[i] = binary.BigEndian.Uint16(data[2+i*2:])
		}
		return string(utf16.Decode(u16)), nil
	}

	return string(data), nil
}

//
// ================= LOG SOURCES =================
//

type LogSource interface {
	ReadLogs(ctx context.Context, lines int) (string, error)
}

type FileLogSource struct {
	Path string
}

func (f *FileLogSource) ReadLogs(ctx context.Context, lines int) (string, error) {
	content, err := readFileAutoUTF(f.Path)
	if err != nil {
		return "", err
	}

	var all []string
	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		all = append(all, sc.Text())
	}

	if len(all) == 0 {
		return "", nil
	}

	if lines <= 0 || lines > len(all) {
		lines = len(all)
	}

	start := len(all) - lines
	return strings.Join(all[start:], "\n"), nil
}

type APILogSource struct {
	URL string
}

func (a *APILogSource) ReadLogs(ctx context.Context, lines int) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	return string(b), nil
}

//
// ================= HELPERS =================
//

func parseLines(r *http.Request) int {
	n, err := strconv.Atoi(r.URL.Query().Get("lines"))
	if err != nil || n <= 0 {
		return globalConfig.Server.DefaultLines
	}
	if n > globalConfig.Server.MaxLines {
		n = globalConfig.Server.MaxLines
	}
	return n
}

func sourceFromConfig(app, key string) (LogSource, LogTarget, error) {
	a, ok := globalConfig.Apps[app]
	if !ok {
		return nil, LogTarget{}, fmt.Errorf("unknown app")
	}
	t, ok := a.Logs[key]
	if !ok {
		return nil, LogTarget{}, fmt.Errorf("unknown log key")
	}

	if t.Type == "file" {
		return &FileLogSource{Path: t.Path}, t, nil
	}
	return &APILogSource{URL: t.URL}, t, nil
}

//
// ================= LOG PARSER =================
//

var timeRegex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`)

func parseLine(line, service string) LogOutput {
	level := "INFO"
	for _, l := range []string{"ERROR", "WARN", "DEBUG"} {
		if strings.Contains(line, l) {
			level = l
			break
		}
	}

	msg := strings.TrimSpace(timeRegex.ReplaceAllString(line, ""))

	return LogOutput{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     level,
		Service:   service,
		Message:   msg,
	}
}

//
// ================= EXISTING HANDLERS =================
//

func logsHandler(w http.ResponseWriter, r *http.Request) {
	app := r.URL.Query().Get("app")
	key := r.URL.Query().Get("log")

	src, target, err := sourceFromConfig(app, key)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	raw, _ := src.ReadLogs(r.Context(), parseLines(r))
	sc := bufio.NewScanner(strings.NewReader(raw))

	var out []LogOutput
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" {
			out = append(out, parseLine(line, target.Service))
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func analyzeHandler(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]interface{}{
		"recommendations": []map[string]string{
			{"title": "Reduce DEBUG logs", "severity": "LOW"},
		},
	})
}

func applyPatchHandler(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

//
// ================= ROTATION-SAFE TAIL =================
//

func tailFile(ctx context.Context, path, service string, onLog func(map[string]interface{})) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	var lastSize int64
	file.Seek(0, io.SeekEnd)
	reader := bufio.NewReader(file)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			info, err := file.Stat()
			if err == nil && info.Size() < lastSize {
				file.Close()
				file, _ = os.Open(path)
				reader = bufio.NewReader(file)
				lastSize = 0
			}

			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					time.Sleep(time.Second)
					continue
				}
				return err
			}

			lastSize += int64(len(line))
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			log := parseLine(line, service)
			onLog(map[string]interface{}{
				"timestamp": log.Timestamp,
				"level":     log.Level,
				"service":   log.Service,
				"message":   log.Message,
			})
		}
	}
}

//
// ================= STREAM HANDLERS =================
//

func streamIngestHandler(w http.ResponseWriter, r *http.Request) {
	app := r.URL.Query().Get("app_name")
	logType := r.URL.Query().Get("log_type")

	if app == "" || logType == "" {
		http.Error(w, "app_name and log_type required", 400)
		return
	}

	src, target, err := sourceFromConfig(app, logType)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	fileSrc, ok := src.(*FileLogSource)
	if !ok {
		http.Error(w, "only file logs supported", 400)
		return
	}

	currentStream.Active = true
	currentStream.AppName = app
	currentStream.LogType = logType
	currentStream.Path = fileSrc.Path
	currentStream.StartedAt = time.Now()

	lines := parseLines(r)
	raw, _ := fileSrc.ReadLogs(r.Context(), lines)
	sc := bufio.NewScanner(strings.NewReader(raw))

	accepted := 0

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		log := parseLine(line, target.Service)
		if streamMgr.Ingest(map[string]interface{}{
			"timestamp": log.Timestamp,
			"level":     log.Level,
			"service":   log.Service,
			"message":   log.Message,
		}) {
			accepted++
		}
	}

	go tailFile(r.Context(), fileSrc.Path, target.Service, func(m map[string]interface{}) {
		if streamMgr.Ingest(m) {
			accepted++
		}
	})

	var bundle *CorrelationBundle
	flushed := false
	if streamMgr.ShouldFlush() {
		bundle = streamMgr.Flush()
		flushed = bundle != nil
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"accepted": accepted,
		"flushed":  flushed,
		"bundle":   bundle,
	})
}

func streamStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(currentStream)
}

func streamLiveHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", 500)
		return
	}

	ch := streamMgr.Subscribe()
	defer streamMgr.Unsubscribe(ch)

	for {
		select {
		case <-r.Context().Done():
			return
		case bundle := <-ch:
			b, _ := json.Marshal(bundle)
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
	}
}

func preprocessHandler(w http.ResponseWriter, r *http.Request) {
	// Read JSON body
	var rawData []map[string]interface{}
	err := json.NewDecoder(r.Body).Decode(&rawData)
	if err != nil {
		http.Error(w, "invalid JSON body", 400)
		return
	}

	processor := NewLogPreprocessorFullGo()
	bundle, err := processor.Process(rawData)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(bundle)
}

//
// ================= MAIN =================
//

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "")
	cfg := flag.String("config", "", "")
	flag.Parse()

	if *cfg != "" {
		b, _ := os.ReadFile(*cfg)
		yaml.Unmarshal(b, &globalConfig)
	}

	if globalConfig.Server == nil {
		globalConfig.Server = &ServerConfig{DefaultLines: 100, MaxLines: 1000}
	}

	http.HandleFunc("/logs", logsHandler)
	http.HandleFunc("/logs/analyze", analyzeHandler)
	http.HandleFunc("/logs/apply-patch", applyPatchHandler)

	http.HandleFunc("/stream/ingest", streamIngestHandler)
	http.HandleFunc("/stream/status", streamStatusHandler)
	http.HandleFunc("/stream/live", streamLiveHandler)
	http.HandleFunc("/logs/preprocess", preprocessHandler)

	fmt.Println("Agent running on", *addr)
	http.ListenAndServe(*addr, nil)
}
