package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tyler-beach/pf_2/internal/adringest"
	"github.com/tyler-beach/pf_2/internal/obligations"
)

type ingestionIndex struct {
	SchemaVersion string               `json:"schema_version"`
	SourceRoot    string               `json:"source_root"`
	Documents     []adringest.Document `json:"documents"`
}

func readObligationIndex(path string) (obligations.Index, error) {
	var index obligations.Index
	data, err := os.ReadFile(path)
	if err != nil {
		return index, fmt.Errorf("read candidate index: %w", err)
	}
	if err := json.Unmarshal(data, &index); err != nil {
		return index, fmt.Errorf("decode candidate index: %w", err)
	}
	return index, nil
}

func readReviewProposal(path string) (obligations.ReviewProposal, error) {
	var proposal obligations.ReviewProposal
	data, err := os.ReadFile(path)
	if err != nil {
		return proposal, fmt.Errorf("read review proposal: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&proposal); err != nil {
		return proposal, fmt.Errorf("decode review proposal: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return proposal, fmt.Errorf("decode review proposal: trailing JSON value")
		}
		return proposal, fmt.Errorf("decode review proposal: trailing content: %w", err)
	}
	return proposal, nil
}
