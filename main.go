package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

type Entity struct {
	ID   string
	Name string
	Type string
}

type Edge struct {
	ID       string
	From     string
	To       string
	Relation string

	ValidFrom time.Time
	ValidTo   *time.Time

	RecordedFrom time.Time
	RecordedTo   *time.Time

	SourceEpisodeID string
	Evidence        string
}

type Graph struct {
	Entities map[string]Entity
	Edges    []Edge
	Outgoing map[string][]Edge

	nextEdgeID uint64
}

func (g *Graph) AddEntity(entity Entity) {
	g.Entities[entity.ID] = entity
}

func (g *Graph) AddEdge(edge Edge) error {
	if err := g.validateEdge(edge); err != nil {
		return err
	}

	g.nextEdgeID++
	edge.ID = fmt.Sprintf("edge-%d", g.nextEdgeID)

	g.Edges = append(g.Edges, edge)
	g.Outgoing[edge.From] = append(g.Outgoing[edge.From], edge)

	return nil
}

func (g *Graph) validateEdge(edge Edge) error {
	if _, exists := g.Entities[edge.From]; !exists {
		return fmt.Errorf("source entity %q not found", edge.From)
	}

	if _, exists := g.Entities[edge.To]; !exists {
		return fmt.Errorf("destination entity %q not found", edge.To)
	}

	if edge.ValidFrom.IsZero() {
		return fmt.Errorf("edge valid-from time is required")
	}

	if edge.ValidTo != nil && !edge.ValidTo.After(edge.ValidFrom) {
		return fmt.Errorf("edge valid-to must be after valid-from")
	}

	if edge.RecordedFrom.IsZero() {
		return fmt.Errorf("edge recorded-from time is required")
	}

	if edge.RecordedTo != nil &&
		!edge.RecordedTo.After(edge.RecordedFrom) {
		return fmt.Errorf(
			"recorded-to must be after recorded-from",
		)
	}

	return nil
}

func (g *Graph) findEdgeByID(edgeID string) (*Edge, error) {
	for i := range g.Edges {
		if g.Edges[i].ID == edgeID {
			return &g.Edges[i], nil
		}
	}

	return nil, fmt.Errorf("edge %q not found", edgeID)
}

func (g *Graph) setRecordedTo(edgeID string, recordedTo time.Time) error {
	// 1. Find the edge in g.Edges.
	// 2. Verify its RecordedTo is nil.
	// 3. Find the same ID in g.Outgoing[edge.From].
	// 4. Verify the outgoing copy exists exactly once.
	// 5. Update RecordedTo in BOTH copies.

	edge, err := g.findEdgeByID(edgeID)
	if err != nil {
		return err
	}

	if edge.RecordedTo != nil {
		return fmt.Errorf("edge %q already has a recorded-to time", edgeID)
	}

	outgoingEdges := g.Outgoing[edge.From]
	var outgoingEdge *Edge

	for i := range outgoingEdges {
		if outgoingEdges[i].ID == edgeID {
			if outgoingEdge != nil {
				return fmt.Errorf(
					"multiple outgoing edges with ID %q found",
					edgeID,
				)
			}

			outgoingEdge = &outgoingEdges[i]
		}
	}

	if outgoingEdge == nil {
		return fmt.Errorf(
			"outgoing edge with ID %q not found in g.Outgoing[%q]",
			edgeID, edge.From,
		)
	}

	outgoingEdge.RecordedTo = &recordedTo
	edge.RecordedTo = &recordedTo

	return nil
}

func (g *Graph) CorrectEdge(oldEdgeID string, replacement Edge, recordedAt time.Time) error {
	// 1. Validate that recordedAt is not zero. Convert it to UTC.
	// 2. Find the old edge using findEdgeByID. Reject the operation if it doesn't exist or its RecordedTo is already set.
	// 3. Check that recordedAt is strictly later than the old edge's RecordedFrom.
	// 4. Verify that replacement.From, replacement.To and replacement.Relation match the original edge. Require a source episode ID and supporting evidence.
	// 5. Give the replacement a new recorded-time interval: RecordedFrom = recordedAt and RecordedTo = nil. Clear its ID so AddEdge assigns a new one.
	// 6. Validate the replacement completely before modifying the old edge. Reuse the same edge-validation rules as AddEdge.
	// 7. Close the old recorded-time version using setRecordedTo.Insert the corrected version using AddEdge.

	recordedAt = recordedAt.UTC()

	if recordedAt.IsZero() {
		return fmt.Errorf("recordedAt time is required")
	}

	oldEdge, err := g.findEdgeByID(oldEdgeID)
	if err != nil {
		return err
	}

	if oldEdge.RecordedTo != nil {
		return fmt.Errorf("edge %q already has a recorded-to time", oldEdgeID)
	}

	if !recordedAt.After(oldEdge.RecordedFrom) {
		return fmt.Errorf(
			"recordedAt time must be after the old edge's recorded-from time",
		)
	}

	if replacement.From != oldEdge.From ||
		replacement.To != oldEdge.To ||
		replacement.Relation != oldEdge.Relation {
		return fmt.Errorf(
			"replacement edge must have the same From, To, and Relation as the original edge",
		)
	}

	if strings.TrimSpace(replacement.SourceEpisodeID) == "" {
		return fmt.Errorf("replacement edge must have a source episode ID")
	}

	if strings.TrimSpace(replacement.Evidence) == "" {
		return fmt.Errorf("replacement edge must have supporting evidence")
	}

	replacement.RecordedFrom = recordedAt
	replacement.RecordedTo = nil
	replacement.ID = ""

	if err := g.validateEdge(replacement); err != nil {
		return fmt.Errorf("invalid replacement edge: %v", err)
	}

	// setRecordedTo checks everything before mutating, and AddEdge cannot
	// fail on an already-validated edge, so no partial update is possible.
	if err := g.setRecordedTo(oldEdgeID, recordedAt); err != nil {
		return fmt.Errorf("failed to close old edge: %v", err)
	}

	if err := g.AddEdge(replacement); err != nil {
		return fmt.Errorf("failed to add replacement edge: %v", err)
	}

	return nil
}

func (e Edge) IsKnownAt(at time.Time) bool {
	if at.Before(e.RecordedFrom) {
		return false
	}

	if e.RecordedTo != nil && !at.Before(*e.RecordedTo) {
		return false
	}

	return true
}

func (e Edge) IsValidAndKnownAt(validAt time.Time, knownAt time.Time) bool {
	return e.IsValidAt(validAt) && e.IsKnownAt(knownAt)
}

func (e Edge) IsValidAt(at time.Time) bool {
	if at.Before(e.ValidFrom) {
		return false
	}

	if e.ValidTo != nil && !at.Before(*e.ValidTo) {
		return false
	}

	return true
}

func NewGraph() *Graph {
	return &Graph{
		Entities: make(map[string]Entity),
		Edges:    make([]Edge, 0),
		Outgoing: make(map[string][]Edge),
	}
}

func (g *Graph) getReachableEntitiesAt(
	startID string,
	validAt time.Time,
	knownAt time.Time,
) ([]Entity, error) {
	if _, exists := g.Entities[startID]; !exists {
		return nil, fmt.Errorf("entity %q not found", startID)
	}

	queue := []string{startID}
	visited := map[string]bool{startID: true}
	related := make([]Entity, 0)

	for head := 0; head < len(queue); head++ {
		for _, edge := range g.Outgoing[queue[head]] {
			if !edge.IsValidAndKnownAt(validAt, knownAt) || visited[edge.To] {
				continue
			}

			neighbor, exists := g.Entities[edge.To]
			if !exists {
				continue
			}

			visited[edge.To] = true
			queue = append(queue, edge.To)
			related = append(related, neighbor)
		}
	}

	return related, nil
}

func (g *Graph) getReachableEntities(startId string, validAt time.Time, knownAt time.Time) ([]Entity, error) {
	if _, exists := g.Entities[startId]; !exists {
		return nil, fmt.Errorf("entity %q not found", startId)
	}

	queue := []string{startId}
	visited := map[string]bool{startId: true}
	related := make([]Entity, 0)

	for head := 0; head < len(queue); head++ {
		currentId := queue[head]

		for _, edge := range g.Outgoing[currentId] {
			neighborId := edge.To

			if visited[neighborId] {
				continue
			}

			neighbor, exists := g.Entities[neighborId]
			if !exists {
				continue
			}

			visited[neighborId] = true
			queue = append(queue, neighborId)
			related = append(related, neighbor)
		}
	}

	return related, nil
}

func (g *Graph) getRelatedEntities(entityId string) ([]Entity, error) {
	if _, exists := g.Entities[entityId]; !exists {
		return nil, fmt.Errorf("entity %q not found", entityId)
	}

	var related []Entity
	queue := []string{entityId}
	visited := map[string]bool{entityId: true}

	for len(queue) > 0 {
		first := queue[0]
		queue = queue[1:]

		element := g.Entities[first]

		if first != entityId {
			related = append(related, element)
		}

		for _, edge := range g.Edges {
			if edge.From != element.ID {
				continue
			}

			neighbor := edge.To
			if neighbor == "" || visited[neighbor] {
				continue
			}

			if _, exists := g.Entities[neighbor]; !exists {
				continue
			}

			visited[neighbor] = true
			queue = append(queue, neighbor)
		}
	}

	return related, nil
}

func ValidateExtraction(extraction Extraction) ValidationReport {
	report := ValidationReport{
		Ready:    make([]ParsedFact, 0),
		Pending:  make([]FactIssue, 0),
		Rejected: make([]FactIssue, 0),
	}

	// Build a lookup of names supplied by the extractor.
	entityNames := make(map[string]bool)

	for _, entity := range extraction.Entities {
		name := normalizeName(entity.Name)

		if name != "" && strings.TrimSpace(entity.Type) != "" {
			entityNames[name] = true
		}
	}

	for _, fact := range extraction.Facts {
		reject := func(reason string) {
			report.Rejected = append(report.Rejected, FactIssue{
				Fact:   fact,
				Reason: reason,
			})
		}

		pending := func(reason string) {
			report.Pending = append(report.Pending, FactIssue{
				Fact:   fact,
				Reason: reason,
			})
		}

		// 1. Required text fields.
		if strings.TrimSpace(fact.Subject) == "" ||
			strings.TrimSpace(fact.Relation) == "" ||
			strings.TrimSpace(fact.Object) == "" {
			reject("subject, relation, and object are required")
			continue
		}

		if strings.TrimSpace(fact.Evidence) == "" {
			reject("supporting evidence is required")
			continue
		}

		// 2. Both endpoints must be among the extracted entities.
		if !entityNames[normalizeName(fact.Subject)] {
			reject("subject does not match an extracted entity")
			continue
		}

		if !entityNames[normalizeName(fact.Object)] {
			reject("object does not match an extracted entity")
			continue
		}

		// 3. Validate temporal status.
		switch fact.TemporalStatus {
		case "current", "historical", "unknown":
		default:
			reject("invalid temporal_status")
			continue
		}

		// 4. Parse the exact date strings.
		validFrom, err := parseExactDate(fact.ValidFrom)
		if err != nil {
			reject(err.Error())
			continue
		}

		validTo, err := parseExactDate(fact.ValidTo)
		if err != nil {
			reject(err.Error())
			continue
		}

		hasStart := fact.ValidFrom != ""
		hasEnd := fact.ValidTo != ""

		// 5. An interval must have a positive duration.
		if hasStart && hasEnd && !validTo.After(validFrom) {
			reject("valid_to must be after valid_from")
			continue
		}

		// 6. A current relationship cannot have a known end.
		if fact.TemporalStatus == "current" && hasEnd {
			reject("current fact cannot have a valid_to date")
			continue
		}

		// 7. Do not invent missing boundaries.
		if !hasStart {
			pending("exact valid_from date is unknown")
			continue
		}

		if fact.TemporalStatus == "historical" && !hasEnd {
			pending("historical fact has no exact end boundary")
			continue
		}

		if fact.TemporalStatus == "unknown" {
			pending(
				"temporal status is unknown; do not create an open-ended edge",
			)
			continue
		}

		// 8. Convert the optional end date into a pointer.
		var validToPtr *time.Time

		if hasEnd {
			validToCopy := validTo
			validToPtr = &validToCopy
		}

		report.Ready = append(report.Ready, ParsedFact{
			Fact:      fact,
			ValidFrom: validFrom,
			ValidTo:   validToPtr,
		})
	}

	return report
}

func normalizeName(value string) string {
	return strings.ToLower(
		strings.Join(strings.Fields(value), " "),
	)
}

func main() {
	graph := NewGraph()

	graph.AddEntity(Entity{ID: "alice", Name: "Alice", Type: "Person"})
	graph.AddEntity(Entity{ID: "acme", Name: "Acme", Type: "Company"})
	graph.AddEntity(Entity{ID: "bob", Name: "Bob", Type: "Person"})
	graph.AddEntity(Entity{ID: "apollo", Name: "Apollo", Type: "Project"})
	graph.AddEntity(Entity{ID: "globex", Name: "Globex", Type: "Company"})

	jan2024 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	sep2025 := time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC)
	oct2026 := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)

	edges := []Edge{
		{From: "alice", To: "acme", Relation: "works_at", ValidFrom: jan2024, RecordedFrom: jan2024, RecordedTo: &oct2026},
		{From: "alice", To: "acme", Relation: "works_at", ValidFrom: jan2024, ValidTo: &sep2025, RecordedFrom: oct2026},
		{From: "alice", To: "globex", Relation: "works_at", ValidFrom: sep2025, RecordedFrom: oct2026},
		{From: "alice", To: "bob", Relation: "knows", ValidFrom: jan2024, RecordedFrom: jan2024},
		{From: "acme", To: "apollo", Relation: "owns", ValidFrom: jan2024, RecordedFrom: jan2024},
		{From: "bob", To: "alice", Relation: "knows", ValidFrom: jan2024, RecordedFrom: jan2024}, // Cycle
	}

	for _, edge := range edges {
		if err := graph.AddEdge(edge); err != nil {
			fmt.Println("Could not add edge: ", err)
			return
		}
	}

	conversation := `
		Alice worked at Acme from January 1, 2024 through August 31, 2025.
		She joined Globex on September 1, 2025, and she still works there.
		`

	ctx := context.Background()

	extraction, err := ExtractMemories(
		ctx, conversation, os.Getenv("OPENAI_MODEL"),
	)

	if err != nil {
		fmt.Println("Memory Extraction failed:", err)
	}

	output, err := json.MarshalIndent(extraction, "", " ")
	if err != nil {
		fmt.Println("Could not marshal extraction:", err)
		return
	}

	fmt.Println("\n LLM EXTRACTED")
	fmt.Println(string(output))

	report := ValidateExtraction(extraction)

	fmt.Printf("\n Ready: %d\n", len(report.Ready))
	fmt.Printf("\n Pending: %d\n", len(report.Pending))
	fmt.Printf("\n Rejected: %d\n", len(report.Rejected))

	for _, fact := range report.Ready {
		fmt.Printf(
			"READY: %s --%s--> %s (from %s)\n",
			fact.Fact.Subject,
			fact.Fact.Relation,
			fact.Fact.Object,
			fact.ValidFrom.Format("2006-01-02"),
		)
	}

	for _, issue := range report.Pending {
		fmt.Printf("PENDING: %s\n", issue.Reason)
	}

	for _, issue := range report.Rejected {
		fmt.Printf("REJECTED: %s\n", issue.Reason)
	}

	if len(report.Ready) == 0 {
		fmt.Println("No facts are ready to be stored.")
		return
	}

	sourceID := "conversation-001"

	if err := ApplyExtraction(
		graph,
		extraction,
		report,
		sourceID,
		time.Now().UTC(),
	); err != nil {
		fmt.Println("Could not apply extraction:", err)
		return
	}

	fmt.Printf(
		"Memory updated: %d facts accepted\n",
		len(report.Ready),
	)

}
