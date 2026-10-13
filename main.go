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
}

func (g *Graph) getRelatedEntities(entityId string, relation string) ([]Entity, error) {

	if _, exists := g.Entities[entityId]; !exists {
		return nil, fmt.Errorf("entity %q not found", entityId)
	}

	var related []Entity

	for _, edge := range g.Edges {
		if edge.From == entityId && edge.Relation == relation {
			entity, exists := g.Entities[edge.To]
			if exists {
				related = append(related, entity)
			}
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
	}

	username, relation := "user", "works_at"
	entities, err := g.getRelatedEntities("user", "works_at")

	if err != nil {
		fmt.Println("No relation exists")
	}

	fmt.Printf("%v is %v at %v", username, relation, entities)

}
