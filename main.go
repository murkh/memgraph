package main

import "fmt"

type Entity struct {
	ID   string
	Name string
	Type string
}

type Edge struct {
	From     string
	To       string
	Relation string
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

	g.Edges = append(g.Edges, edge)
	g.Outgoing[edge.From] = append(g.Outgoing[edge.From], edge)

	return nil
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
	g := Graph{
		Entities: map[string]Entity{
			"user":   {ID: "user", Name: "user"},
			"google": {ID: "google", Name: "google"},
			"pune":   {ID: "pune", Name: "pune"},
		},
		Edges: []Edge{
			{From: "user", To: "google", Relation: "works_at"},
			{From: "user", To: "pune", Relation: "lives_in"},
		},
		Outgoing: make(map[string][]Edge),
	}

	username, relation := "user", "works_at"
	entities, err := g.getRelatedEntities("user")

	if err != nil {
		fmt.Println("No relation exists")
	}

	fmt.Printf("%v is %v at %v", username, relation, entities)

}
