package ui

import (
	"encoding/json"
	"fmt"
	"io"

	"routerlens/internal/scorer"
)

// JSONOutputWrapper encapsulates the overall JSON output format
type JSONOutputWrapper struct {
	Config  scorer.ScoringConfig            `json:"config"`
	Results []*scorer.ModelEvaluationResult `json:"results"`
}

// RenderJSON outputs evaluations as formatted JSON
func RenderJSON(w io.Writer, results []*scorer.ModelEvaluationResult, cfg scorer.ScoringConfig) error {
	wrapper := JSONOutputWrapper{
		Config:  cfg,
		Results: results,
	}

	data, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	_, err = fmt.Fprintln(w, string(data))
	return err
}
