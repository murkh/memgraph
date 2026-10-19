package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func resolveEntityID(
	g *Graph,
	candidate ExtractedEntity,
) (string, bool, error) {
	name := normalizeName(candidate.Name)
	entityType := strings.ToLower(strings.TrimSpace(candidate.Type))

	if name == "" || entityType == "" {
		return "", false, fmt.Errorf("entity name and type are required")
	}

	matchedID := ""

	// Prefer an existing entity with the same normalized name and type.
	for id, existing := range g.Entities {
		if normalizeName(existing.Name) != name {
			continue
		}

		existingType := strings.ToLower(strings.TrimSpace(existing.Type))
		if existingType != entityType {
			continue
		}

		if matchedID != "" && matchedID != id {
			return "", false, fmt.Errorf(
				"ambiguous entity %q: multiple matching IDs",
				candidate.Name,
			)
		}

		matchedID = id
	}

	if matchedID != "" {
		return matchedID, true, nil
	}

	// For a new entity, derive a deterministic ID.
	newID := entityType + ":" + name

	if existing, exists := g.Entities[newID]; exists {
		return "", false, fmt.Errorf(
			"cannot create entity %q: ID %q already belongs to %q",
			candidate.Name, newID, existing.Name,
		)
	}

	return newID, false, nil
}

func ApplyExtraction(
	g *Graph,
	extraction Extraction,
	report ValidationReport,
	sourceID string,
	recordedAt time.Time,
) error {
	if g == nil {
		return fmt.Errorf("graph cannot be nil")
	}

	if g.Entities == nil {
		return fmt.Errorf("graph is not initialized; use NewGraph()")
	}

	if strings.TrimSpace(sourceID) == "" {
		return fmt.Errorf("source episode ID is required")
	}

	if recordedAt.IsZero() {
		return fmt.Errorf("recorded-at time is required")
	}

	if g.Outgoing == nil {
		g.Outgoing = make(map[string][]Edge)
	}

	recordedAt = recordedAt.UTC()

	// Build a lookup of entities returned by the extractor.
	candidates := make(map[string]ExtractedEntity)

	for _, candidate := range extraction.Entities {
		name := normalizeName(candidate.Name)
		candidate.Name = strings.TrimSpace(candidate.Name)
		candidate.Type = strings.TrimSpace(candidate.Type)

		if name == "" || candidate.Type == "" {
			return fmt.Errorf("extraction contains an entity with an empty name or type")
		}

		if previous, exists := candidates[name]; exists {
			if !strings.EqualFold(previous.Type, candidate.Type) {
				return fmt.Errorf(
					"entity name %q has conflicting types %q and %q",
					candidate.Name, previous.Type, candidate.Type,
				)
			}

			continue
		}

		candidates[name] = candidate
	}

	// Identify entities needed by validated facts.
	neededNames := make(map[string]bool)

	for _, parsed := range report.Ready {
		for _, name := range []string{
			parsed.Fact.Subject,
			parsed.Fact.Object,
		} {
			key := normalizeName(name)

			if _, exists := candidates[key]; !exists {
				return fmt.Errorf(
					"fact references entity %q missing from extraction",
					name,
				)
			}

			neededNames[key] = true
		}
	}

	// Resolve IDs and plan new nodes without mutating the graph yet.
	keys := make([]string, 0, len(neededNames))
	for key := range neededNames {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	resolvedIDs := make(map[string]string)
	plannedEntities := make(map[string]Entity)

	for _, key := range keys {
		candidate := candidates[key]

		id, exists, err := resolveEntityID(g, candidate)
		if err != nil {
			return err
		}

		resolvedIDs[key] = id

		if exists {
			continue
		}

		entity := Entity{
			ID:   id,
			Name: candidate.Name,
			Type: candidate.Type,
		}

		if previous, collision := plannedEntities[id]; collision {
			if normalizeName(previous.Name) != normalizeName(entity.Name) ||
				!strings.EqualFold(previous.Type, entity.Type) {
				return fmt.Errorf("planned entity ID collision: %q", id)
			}
		}

		plannedEntities[id] = entity
	}

	// Construct edges, but do not insert them yet.
	plannedEdges := make([]Edge, 0, len(report.Ready))

	for _, parsed := range report.Ready {
		fact := parsed.Fact

		if parsed.ValidFrom.IsZero() {
			return fmt.Errorf(
				"fact %q -> %q has no valid start time",
				fact.Subject, fact.Object,
			)
		}

		// Unknown status must not silently become an open-ended
		// relationship in our current graph model.
		if fact.TemporalStatus == "unknown" {
			return fmt.Errorf(
				"fact %q -> %q has unknown temporal status",
				fact.Subject, fact.Object,
			)
		}

		if fact.TemporalStatus == "current" && parsed.ValidTo != nil {
			return fmt.Errorf("current fact cannot have a valid end time")
		}

		if fact.TemporalStatus == "historical" {
			if parsed.ValidTo == nil {
				return fmt.Errorf("historical fact requires an end time")
			}

			if !parsed.ValidTo.After(parsed.ValidFrom) {
				return fmt.Errorf("historical fact has an invalid time interval")
			}
		}

		fromID := resolvedIDs[normalizeName(fact.Subject)]
		toID := resolvedIDs[normalizeName(fact.Object)]

		if fromID == "" || toID == "" {
			return fmt.Errorf("could not resolve fact endpoints")
		}

		if strings.TrimSpace(fact.Relation) == "" {
			return fmt.Errorf("fact relation cannot be empty")
		}

		if strings.TrimSpace(fact.Evidence) == "" {
			return fmt.Errorf("fact evidence cannot be empty")
		}

		edge := Edge{
			From:            fromID,
			To:              toID,
			Relation:        strings.TrimSpace(fact.Relation),
			ValidFrom:       parsed.ValidFrom.UTC(),
			ValidTo:         cloneTime(parsed.ValidTo),
			RecordedFrom:    recordedAt,
			RecordedTo:      nil,
			SourceEpisodeID: sourceID,
			Evidence:        fact.Evidence,
		}

		// Avoid duplicate edges if this same source is processed again.
		if containsSameSourceFact(g.Edges, edge) ||
			containsSameSourceFact(plannedEdges, edge) {
			continue
		}

		plannedEdges = append(plannedEdges, edge)
	}

	// Commit the plan to the in-memory graph.
	entityIDs := make([]string, 0, len(plannedEntities))
	for id := range plannedEntities {
		entityIDs = append(entityIDs, id)
	}
	sort.Strings(entityIDs)

	for _, id := range entityIDs {
		g.AddEntity(plannedEntities[id])
	}

	for _, edge := range plannedEdges {
		if err := g.AddEdge(edge); err != nil {
			return fmt.Errorf("add extracted fact to graph: %w", err)
		}
	}

	return nil
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}

	copy := value.UTC()
	return &copy
}

func containsSameSourceFact(edges []Edge, candidate Edge) bool {
	for _, edge := range edges {
		if edge.SourceEpisodeID != candidate.SourceEpisodeID {
			continue
		}

		if edge.From != candidate.From ||
			edge.To != candidate.To ||
			edge.Relation != candidate.Relation ||
			!edge.ValidFrom.Equal(candidate.ValidFrom) {
			continue
		}

		if sameOptionalTime(edge.ValidTo, candidate.ValidTo) {
			return true
		}
	}

	return false
}

func sameOptionalTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}

	return a.Equal(*b)
}
