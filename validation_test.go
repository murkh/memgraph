package main

import "testing"

func TestValidateExtractionRejectsInvalidInterval(t *testing.T) {
	extraction := Extraction{
		Entities: []ExtractedEntity{
			{Name: "Alice", Type: "Person"},
			{Name: "Acme", Type: "Company"},
		},
		Facts: []ExtractedFact{
			{
				Subject:        "Alice",
				Relation:       "employed_by",
				Object:         "Acme",
				TemporalStatus: "historical",
				ValidFrom:      "2025-01-01",
				ValidTo:        "2024-12-01",
				Evidence:       "Alice worked at Acme.",
			},
		},
	}

	report := ValidateExtraction(extraction)

	if len(report.Rejected) != 1 {
		t.Fatalf(
			"expected 1 rejected fact, got %d",
			len(report.Rejected),
		)
	}

	if len(report.Ready) != 0 {
		t.Fatalf(
			"expected 0 ready facts, got %d",
			len(report.Ready),
		)
	}
}
