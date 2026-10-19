package main

import (
	"testing"
	"time"
)

func TestApplyExtractionIsIdempotent(t *testing.T) {
	graph := NewGraph()

	extraction := Extraction{
		Entities: []ExtractedEntity{
			{Name: "Alice", Type: "Person"},
			{Name: "Globex", Type: "Company"},
		},
		Facts: []ExtractedFact{
			{
				Subject:        "Alice",
				Relation:       "employed_by",
				Object:         "Globex",
				TemporalStatus: "current",
				ValidFrom:      "2025-09-01",
				Evidence:       "She still works at Globex.",
			},
		},
	}

	validFrom := time.Date(
		2025, time.September, 1,
		0, 0, 0, 0, time.UTC,
	)

	report := ValidationReport{
		Ready: []ParsedFact{
			{
				Fact:      extraction.Facts[0],
				ValidFrom: validFrom,
			},
		},
	}

	recordedAt := time.Date(
		2026, time.October, 19,
		12, 0, 0, 0, time.UTC,
	)

	err := ApplyExtraction(
		graph, extraction, report,
		"conversation-001", recordedAt,
	)
	if err != nil {
		t.Fatalf("first application failed: %v", err)
	}

	if len(graph.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(graph.Edges))
	}

	// Retry the same source. Use a different processing timestamp.
	err = ApplyExtraction(
		graph, extraction, report,
		"conversation-001", recordedAt.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("retry failed: %v", err)
	}

	if len(graph.Edges) != 1 {
		t.Fatalf(
			"expected retry to preserve 1 edge, got %d",
			len(graph.Edges),
		)
	}

	if len(graph.Outgoing["person:alice"]) != 1 {
		t.Fatal("expected one outgoing edge from Alice")
	}

	if graph.Edges[0].SourceEpisodeID != "conversation-001" {
		t.Fatal("source episode ID was not preserved")
	}
}
