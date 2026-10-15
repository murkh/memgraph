package main

import (
	"fmt"
	"time"
)

type Entity struct {
	ID   string
	Name string
	Type string
}

type Edge struct {
	From     string
	To       string
	Relation string

	ValidFrom time.Time
	ValidTo   *time.Time

	RecordedFrom time.Time
	RecordedTo   *time.Time
}

type Graph struct {
	Entities map[string]Entity
	Edges    []Edge
	Outgoing map[string][]Edge
}

func (g *Graph) AddEntity(entity Entity) {
	g.Entities[entity.ID] = entity
}

func (g *Graph) AddEdge(edge Edge) error {
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
		return fmt.Errorf("edge valid-from time is required")
	}

	if edge.RecordedTo != nil &&
		!edge.RecordedTo.After(edge.RecordedFrom) {
		return fmt.Errorf(
			"recorded-to must be after recorded-from",
		)
	}

	g.Edges = append(g.Edges, edge)
	g.Outgoing[edge.From] = append(g.Outgoing[edge.From], edge)

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
	at time.Time,
) ([]Entity, error) {
	if _, exists := g.Entities[startID]; !exists {
		return nil, fmt.Errorf("entity %q not found", startID)
	}

	queue := []string{startID}
	visited := map[string]bool{startID: true}
	related := make([]Entity, 0)

	for head := 0; head < len(queue); head++ {
		for _, edge := range g.Outgoing[queue[head]] {
			if !edge.IsValidAt(at) || visited[edge.To] {
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

func (g *Graph) getReachableEntities(startId string) ([]Entity, error) {
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

func main() {
	graph := NewGraph()

	graph.AddEntity(Entity{ID: "alice", Name: "Alice", Type: "Person"})
	graph.AddEntity(Entity{ID: "acme", Name: "Acme", Type: "Company"})
	graph.AddEntity(Entity{ID: "bob", Name: "Bob", Type: "Person"})
	graph.AddEntity(Entity{ID: "apollo", Name: "Apollo", Type: "Project"})

	edges := []Edge{
		{From: "alice", To: "acme", Relation: "works_at"},
		{From: "alice", To: "bob", Relation: "knows"},
		{From: "acme", To: "apollo", Relation: "owns"},
		{From: "bob", To: "alice", Relation: "knows"}, // Cycle
	}

	for _, edge := range edges {
		if err := graph.AddEdge(edge); err != nil {
			fmt.Println("Could not add edge: ", err)
			return
		}
	}

	related, err := graph.getReachableEntities("alice")
	if err != nil {
		fmt.Println("Traversal failed: ", err)
		return
	}

	fmt.Println("Entities reachable from Alice:")
	for _, entity := range related {
		fmt.Printf("- %s (%s)\n", entity.Name, entity.Type)
	}

}
