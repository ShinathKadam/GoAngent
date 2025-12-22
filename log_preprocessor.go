package main

import (
	"errors"
	"sort"
	"strings"
	"time"
)

//
// ===================== INTERNAL TYPES =====================
//

type RawLogGo struct {
	Timestamp  string
	Level      string
	Service    string
	Pod        *string
	Message    string
	ErrorClass *string
}

type LogPatternGo struct {
	Pattern         string
	Count           int
	FirstOccurrence string
	LastOccurrence  string
	ErrorClass      *string
}

type SequenceItemGo struct {
	Timestamp     string
	Type          string
	Message       string
	SequenceIndex int
}

type MetricsGo struct {
	ErrorRateZ float64
	LatencyZ   float64
}

type CorrelationBundleGo struct {
	WindowStart         string
	WindowEnd           string
	RootService         *string
	AffectedServices    []string
	LogPatterns         []LogPatternGo
	Events              []string
	Metrics             MetricsGo
	DependencyGraph     []string
	Sequence            []SequenceItemGo
	DerivedRootCauseHint string
}

//
// ===================== LOG PARSER =====================
//

type LogParserGo struct{}

func (p *LogParserGo) ParseLogs(rawData []map[string]interface{}) ([]RawLogGo, error) {
	if rawData == nil {
		return nil, errors.New("rawData is nil")
	}

	var parsedLogs []RawLogGo
	for _, entry := range rawData {
		timestamp := ""
		if v, ok := entry["timestamp"].(string); ok && v != "" {
			timestamp = v
		} else {
			timestamp = time.Now().UTC().Format(time.RFC3339)
		}

		level := "INFO"
		if v, ok := entry["level"].(string); ok && v != "" {
			level = v
		}

		service := "unknown"
		if v, ok := entry["service"].(string); ok && v != "" {
			service = v
		}

		var pod *string
		if v, ok := entry["pod"].(string); ok {
			pod = &v
		}

		message := ""
		if v, ok := entry["message"].(string); ok {
			message = v
		}

		var errorClass *string
		if v, ok := entry["errorClass"].(string); ok {
			errorClass = &v
		}

		parsedLogs = append(parsedLogs, RawLogGo{
			Timestamp:  timestamp,
			Level:      level,
			Service:    service,
			Pod:        pod,
			Message:    message,
			ErrorClass: errorClass,
		})
	}

	// Sort by timestamp
	sort.Slice(parsedLogs, func(i, j int) bool {
		return parsedLogs[i].Timestamp < parsedLogs[j].Timestamp
	})

	return parsedLogs, nil
}

//
// ===================== LOG PATTERN MINER =====================
//

type LogPatternMinerGo struct{}

func (m *LogPatternMinerGo) MinePatterns(logs []RawLogGo) []LogPatternGo {
	patternMap := make(map[string]LogPatternGo)

	for _, log := range logs {
		key := log.Message
		p, exists := patternMap[key]
		if !exists {
			p = LogPatternGo{
				Pattern:         key,
				Count:           0,
				FirstOccurrence: log.Timestamp,
				LastOccurrence:  log.Timestamp,
				ErrorClass:      log.ErrorClass,
			}
		}

		p.Count++
		if log.Timestamp < p.FirstOccurrence {
			p.FirstOccurrence = log.Timestamp
		}
		if log.Timestamp > p.LastOccurrence {
			p.LastOccurrence = log.Timestamp
		}
		patternMap[key] = p
	}

	var patterns []LogPatternGo
	for _, p := range patternMap {
		patterns = append(patterns, p)
	}
	return patterns
}

//
// ===================== BUNDLE FACTORY =====================
//

type BundleFactoryGo struct{}

func (f *BundleFactoryGo) CreateBundle(logs []RawLogGo, patterns []LogPatternGo) (*CorrelationBundleGo, error) {
	if len(logs) == 0 {
		return nil, errors.New("cannot create bundle from empty logs")
	}

	windowStart := logs[0].Timestamp
	windowEnd := logs[len(logs)-1].Timestamp

	affectedServicesMap := make(map[string]struct{})
	for _, log := range logs {
		affectedServicesMap[log.Service] = struct{}{}
	}
	var affectedServices []string
	for k := range affectedServicesMap {
		affectedServices = append(affectedServices, k)
	}

	var sequence []SequenceItemGo
	for i, log := range logs {
		sequence = append(sequence, SequenceItemGo{
			Timestamp:     log.Timestamp,
			Type:          "log",
			Message:       "[" + log.Service + "] " + log.Message,
			SequenceIndex: i,
		})
	}

	var rootService *string
	for _, log := range logs {
		if log.Level == "ERROR" || log.Level == "FATAL" || log.Level == "CRITICAL" {
			rootService = &log.Service
			break
		}
	}

	errorRateZ := 0.0
	latencyZ := 0.0
	fullText := strings.Join(func() []string {
		var msgs []string
		for _, l := range logs {
			msgs = append(msgs, l.Message)
		}
		return msgs
	}(), " ")
	fullText = strings.ToLower(fullText)

	if strings.Contains(fullText, "timeout") || strings.Contains(fullText, "messages") {
		latencyZ = 3.0
	}
	for _, l := range logs {
		if l.Level == "ERROR" {
			errorRateZ = 5.0
			break
		}
	}

	return &CorrelationBundleGo{
		WindowStart:      windowStart,
		WindowEnd:        windowEnd,
		RootService:      rootService,
		AffectedServices: affectedServices,
		LogPatterns:      patterns,
		Events:           []string{},
		Metrics: MetricsGo{
			ErrorRateZ: errorRateZ,
			LatencyZ:   latencyZ,
		},
		DependencyGraph:     affectedServices,
		Sequence:            sequence,
		DerivedRootCauseHint: func() string {
			if rootService != nil {
				return "Issue detected in " + *rootService
			}
			return "Unknown issue"
		}(),
	}, nil
}

//
// ===================== MAIN ORCHESTRATOR =====================
//

type LogPreprocessorFullGo struct {
	Parser  *LogParserGo
	Miner   *LogPatternMinerGo
	Factory *BundleFactoryGo
}

func NewLogPreprocessorFullGo() *LogPreprocessorFullGo {
	return &LogPreprocessorFullGo{
		Parser:  &LogParserGo{},
		Miner:   &LogPatternMinerGo{},
		Factory: &BundleFactoryGo{},
	}
}

func (p *LogPreprocessorFullGo) Process(rawData []map[string]interface{}) (*CorrelationBundleGo, error) {
	logs, err := p.Parser.ParseLogs(rawData)
	if err != nil {
		return nil, err
	}
	patterns := p.Miner.MinePatterns(logs)
	bundle, err := p.Factory.CreateBundle(logs, patterns)
	if err != nil {
		return nil, err
	}
	return bundle, nil
}
