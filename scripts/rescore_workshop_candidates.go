//go:build ignore

// Rescores a workshop candidates.json with ScoreCandidate (Platt/conformal).
// Usage: go run scripts/rescore_workshop_candidates.go path/to/candidates.json
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/tyler-beach/pf_2/internal/obligations"
	"github.com/tyler-beach/pf_2/internal/obligations/calibration"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run scripts/rescore_workshop_candidates.go <candidates.json>")
		os.Exit(2)
	}
	path := os.Args[1]
	raw, err := os.ReadFile(path)
	if err != nil {
		fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		fatal(err)
	}
	candsRaw, ok := payload["candidates"].([]any)
	if !ok {
		fatal(fmt.Errorf("missing candidates array"))
	}

	type thin struct {
		ID                 string   `json:"id"`
		AdrID              string   `json:"adr_id"`
		Confidence         string   `json:"confidence"`
		Modality           string   `json:"modality"`
		Kind               string   `json:"kind"`
		PatternHint        string   `json:"pattern_hint"`
		Statement          string   `json:"statement"`
		SourcePath         string   `json:"source_path"`
		SourceLineStart    int      `json:"source_line_start"`
		SourceLineEnd      int      `json:"source_line_end"`
		EscalationReasons  []string `json:"escalation_reasons"`
		Ambiguity          []string `json:"ambiguity"`
		Exceptions         []string `json:"exceptions"`
		Conditions         []string `json:"conditions"`
		ReviewHints        []string `json:"review_hints"`
		SubmissionKind     string   `json:"submission_kind"`
		ConfidenceP        float64  `json:"confidence_p"`
		ConfidenceMethod   string   `json:"confidence_method"`
		ConfidenceCoverage float64  `json:"confidence_coverage"`
		PredictionSet      []string `json:"prediction_set"`
	}

	out := make([]thin, 0, len(candsRaw))
	high, medium, low := 0, 0, 0
	method := ""
	coverage := 0.0
	for _, item := range candsRaw {
		b, err := json.Marshal(item)
		if err != nil {
			fatal(err)
		}
		var t thin
		if err := json.Unmarshal(b, &t); err != nil {
			fatal(err)
		}
		if t.Kind == "" {
			t.Kind = "obligation"
		}
		if t.Modality == "" {
			// Infer from existing band / statement cues for minimal fixtures.
			switch t.Confidence {
			case "high":
				t.Modality = "must"
			case "medium":
				t.Modality = "should"
			default:
				t.Modality = "should"
			}
		}
		scored := obligations.ScoreCandidate(obligations.Candidate{
			Modality:    t.Modality,
			Kind:        t.Kind,
			PatternHint: t.PatternHint,
			Ambiguity:   t.Ambiguity,
			Exceptions:  t.Exceptions,
			Conditions:  t.Conditions,
			ReviewHints: t.ReviewHints,
		})
		t.Confidence = scored.ConfidenceBand
		t.ConfidenceP = scored.ConfidenceP
		t.ConfidenceMethod = scored.ConfidenceMethod
		t.ConfidenceCoverage = scored.ConfidenceCoverage
		t.PredictionSet = scored.PredictionSet
		if len(scored.EscalationReasons) > 0 {
			t.EscalationReasons = scored.EscalationReasons
		}
		switch scored.ConfidenceBand {
		case "high":
			high++
		case "medium":
			medium++
		default:
			low++
		}
		if method == "" {
			method = scored.ConfidenceMethod
		}
		if coverage <= 0 {
			coverage = scored.ConfidenceCoverage
		}
		out = append(out, t)
	}

	payload["candidates"] = out
	payload["candidate_count"] = len(out)
	payload["confidence_summary"] = map[string]int{"high": high, "medium": medium, "low": low}
	if method == "" {
		method = calibration.MethodPlattConformal
	}
	if coverage <= 0 {
		coverage = 0.9
	}
	payload["calibration"] = map[string]any{"method": method, "coverage": coverage}

	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("rescored %s: n=%d high=%d medium=%d low=%d method=%s\n", path, len(out), high, medium, low, method)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
