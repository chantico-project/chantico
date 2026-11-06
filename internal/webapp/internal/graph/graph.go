package graph

type Node struct {
	Name    string
	Parents []*Node
}

type Element struct {
	Data map[string]string `json:"data"`
}

type Elements struct {
	Nodes []Element `json:"nodes"`
	Edges []Element `json:"edges"`
}

func BuildElements(nodes []*Node) Elements {
	elements := Elements{
		Nodes: []Element{},
		Edges: []Element{},
	}

	for _, node := range nodes {
		elements.Nodes = append(elements.Nodes, Element{
			Data: map[string]string{"id": node.Name},
		})
		for _, parent := range node.Parents {
			elements.Edges = append(elements.Edges, Element{
				Data: map[string]string{
					"id":     node.Name + "->" + parent.Name,
					"source": node.Name,
					"target": parent.Name,
				},
			})
		}
	}

	return elements
}
