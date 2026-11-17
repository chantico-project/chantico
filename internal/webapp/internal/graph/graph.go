package graph

type Node struct {
	Name    string
	Label   string
	Type    string
	Status  string
	Parents []*Node
}

type Element struct {
	Data map[string]string `json:"data"`
}

type Elements struct {
	Nodes []Element `json:"nodes"`
	Edges []Element `json:"edges"`
}

// Neighbourhood returns the node named name together with its direct parents
// and direct children, keeping the order of nodes. It returns nil when name is
// not present in nodes.
func Neighbourhood(nodes []*Node, name string) []*Node {
	var target *Node
	for _, node := range nodes {
		if node.Name == name {
			target = node
			break
		}
	}
	if target == nil {
		return nil
	}

	included := map[string]bool{target.Name: true}
	for _, parent := range target.Parents {
		included[parent.Name] = true
	}
	for _, node := range nodes {
		for _, parent := range node.Parents {
			if parent.Name == target.Name {
				included[node.Name] = true
			}
		}
	}

	result := []*Node{}
	for _, node := range nodes {
		if included[node.Name] {
			result = append(result, node)
		}
	}
	return result
}

func BuildElements(nodes []*Node) Elements {
	elements := Elements{
		Nodes: []Element{},
		Edges: []Element{},
	}

	for _, node := range nodes {
		elements.Nodes = append(elements.Nodes, Element{
			Data: map[string]string{"id": node.Name, "label": node.Label, "type": node.Type, "status": node.Status},
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
