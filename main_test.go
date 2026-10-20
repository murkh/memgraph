package main

import (
	"testing"
	"time"
)

func TestCorrectEdgeClosesOldVersion(t *testing.T) {
	graph := NewGraph()

	graph.AddEntity(Entity{ID: "person:alice", Name: "Alice", Type: "Person"})
	graph.AddEntity(Entity{ID: "company:acme", Name: "Acme", Type: "Company"})

	original := Edge{
		From:            "person:alice",
		To:              "company:acme",
		Relation:        "employed_by",
		ValidFrom:       time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC),
		RecordedFrom:    time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC),
		SourceEpisodeID: "conversation-001",
		Evidence:        "Alice works at Acme.",
	}

	if err := graph.AddEdge(original); err != nil {
		t.Fatalf("add original edge: %v", err)
	}

	originalID := graph.Edges[0].ID

	end := time.Date(
		2025, time.September, 1,
		0, 0, 0, 0, time.UTC,
	)

	corrected := Edge{
		From:            "person:alice",
		To:              "company:acme",
		Relation:        "employed_by",
		ValidFrom:       original.ValidFrom,
		ValidTo:         &end,
		SourceEpisodeID: "conversation-002",
		Evidence:        "I left Acme on September 1, 2025.",
	}

	recordedAt := time.Date(
		2026, time.October, 19,
		12, 0, 0, 0, time.UTC,
	)

	if err := graph.CorrectEdge(originalID, corrected, recordedAt); err != nil {
		t.Fatalf("correct edge: %v", err)
	}

	if len(graph.Edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(graph.Edges))
	}

	outgoing := graph.Outgoing["person:alice"]
	if len(outgoing) != 2 {
		t.Fatalf("expected 2 outgoing edges from Alice, got %d", len(outgoing))
	}

	// Both copies of the old version must be closed at recordedAt.
	for _, old := range []Edge{graph.Edges[0], outgoing[0]} {
		if old.ID != originalID {
			t.Fatalf("expected old edge %q, got %q", originalID, old.ID)
		}

		if old.RecordedTo == nil || !old.RecordedTo.Equal(recordedAt) {
			t.Fatalf("old edge recorded-to = %v, want %v", old.RecordedTo, recordedAt)
		}
	}

	// Both copies of the new version must be open from recordedAt.
	for _, edge := range []Edge{graph.Edges[1], outgoing[1]} {
		if edge.ID == "" || edge.ID == originalID {
			t.Fatalf("expected a fresh ID for the corrected edge, got %q", edge.ID)
		}

		if !edge.RecordedFrom.Equal(recordedAt) || edge.RecordedTo != nil {
			t.Fatalf(
				"corrected edge recorded interval = [%v, %v), want [%v, open)",
				edge.RecordedFrom, edge.RecordedTo, recordedAt,
			)
		}

		if edge.ValidTo == nil || !edge.ValidTo.Equal(end) {
			t.Fatalf("corrected edge valid-to = %v, want %v", edge.ValidTo, end)
		}

		if edge.SourceEpisodeID != "conversation-002" {
			t.Fatalf("corrected edge source = %q", edge.SourceEpisodeID)
		}
	}

	// October 2025: before the correction we believed Alice still worked at
	// Acme; after it, we know she had already left.
	oct2025 := time.Date(2025, time.October, 1, 0, 0, 0, 0, time.UTC)

	before, err := graph.getReachableEntitiesAt("person:alice", oct2025, recordedAt.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	if len(before) != 1 || before[0].ID != "company:acme" {
		t.Fatalf("before correction: expected [company:acme], got %v", before)
	}

	after, err := graph.getReachableEntitiesAt("person:alice", oct2025, recordedAt)
	if err != nil {
		t.Fatal(err)
	}

	if len(after) != 0 {
		t.Fatalf("after correction: expected no reachable entities, got %v", after)
	}
}

func TestCorrectEdgeFailureLeavesGraphUnchanged(t *testing.T) {
	graph := NewGraph()

	graph.AddEntity(Entity{ID: "person:alice", Name: "Alice", Type: "Person"})
	graph.AddEntity(Entity{ID: "company:acme", Name: "Acme", Type: "Company"})

	jan2024 := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, time.September, 1, 0, 0, 0, 0, time.UTC)

	original := Edge{
		From:            "person:alice",
		To:              "company:acme",
		Relation:        "employed_by",
		ValidFrom:       jan2024,
		RecordedFrom:    jan2024,
		SourceEpisodeID: "conversation-001",
		Evidence:        "Alice works at Acme.",
	}

	if err := graph.AddEdge(original); err != nil {
		t.Fatalf("add original edge: %v", err)
	}

	// Drop the outgoing copy so closing the old version must fail.
	delete(graph.Outgoing, "person:alice")

	corrected := original
	corrected.ValidTo = &end
	corrected.SourceEpisodeID = "conversation-002"
	corrected.Evidence = "I left Acme on September 1, 2025."

	recordedAt := time.Date(2026, time.October, 19, 12, 0, 0, 0, time.UTC)

	if err := graph.CorrectEdge(graph.Edges[0].ID, corrected, recordedAt); err == nil {
		t.Fatal("expected correction to fail")
	}

	if len(graph.Edges) != 1 {
		t.Fatalf("expected failed correction to keep 1 edge, got %d", len(graph.Edges))
	}

	if len(graph.Outgoing["person:alice"]) != 0 {
		t.Fatal("failed correction inserted an outgoing edge")
	}

	if graph.Edges[0].RecordedTo != nil {
		t.Fatal("failed correction closed the old edge")
	}
}

func TestAddEdgeFailureDoesNotConsumeID(t *testing.T) {
	graph := NewGraph()

	graph.AddEntity(Entity{ID: "person:alice", Name: "Alice", Type: "Person"})
	graph.AddEntity(Entity{ID: "company:acme", Name: "Acme", Type: "Company"})

	jan2024 := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)

	edge := Edge{
		From:         "person:alice",
		To:           "company:missing",
		Relation:     "employed_by",
		ValidFrom:    jan2024,
		RecordedFrom: jan2024,
	}

	if err := graph.AddEdge(edge); err == nil {
		t.Fatal("expected edge to a missing entity to be rejected")
	}

	edge.To = "company:acme"

	if err := graph.AddEdge(edge); err != nil {
		t.Fatalf("add valid edge: %v", err)
	}

	if graph.Edges[0].ID != "edge-1" {
		t.Fatalf("expected first stored edge to be edge-1, got %q", graph.Edges[0].ID)
	}
}
