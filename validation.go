package main

import (
	"fmt"
	"time"
)

type ParsedFact struct {
	Fact      ExtractedFact
	ValidFrom time.Time
	ValidTo   *time.Time
}

type FactIssue struct {
	Fact   ExtractedFact
	Reason string
}

type ValidationReport struct {
	Ready    []ParsedFact
	Pending  []FactIssue
	Rejected []FactIssue
}

func parseExactDate(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}

	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, fmt.Errorf(
			"invalid date %q: expected YYYY-MM-DD", value,
		)
	}

	return parsed.UTC(), nil
}
