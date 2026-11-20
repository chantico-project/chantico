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
// Neighbourhood returns the node named name together with all of its ancestors
// (parents, grandparents, ...) and all of its descendants (children, children
// of children, ...), keeping the order of nodes. Parent references are limited
// to the returned nodes so that no edge points outside the result. It returns
// nil when name is not present in nodes.
func Neighbourhood(nodes []*Node, name string) []*Node {
	byName := make(map[string]*Node, len(nodes))
	children := make(map[string][]string)
	for _, node := range nodes {
		if _, exists := byName[node.Name]; !exists {
			byName[node.Name] = node
		}
		for _, parent := range node.Parents {
			children[parent.Name] = append(children[parent.Name], node.Name)
		}
	}
	if _, ok := byName[name]; !ok {
		return nil
	}

	parentsOf := func(current string) []string {
		var names []string
		if node, ok := byName[current]; ok {
			for _, parent := range node.Parents {
				names = append(names, parent.Name)
			}
		}
		return names
	}
	childrenOf := func(current string) []string {
		return children[current]
	}

	included := map[string]bool{}
	for member := range walk(name, parentsOf) {
		included[member] = true
	}
	for member := range walk(name, childrenOf) {
		included[member] = true
	}

	result := []*Node{}
	for _, node := range nodes {
		if !included[node.Name] {
			continue
		}
		kept := &Node{
			Name:   node.Name,
			Label:  node.Label,
			Type:   node.Type,
			Status: node.Status,
		}
		for _, parent := range node.Parents {
			if included[parent.Name] {
				kept.Parents = append(kept.Parents, parent)
			}
		}
		result = append(result, kept)
	}
	return result
}

// walk returns the set of names reachable from start by repeatedly applying
// next, including start itself. The visited set makes cyclic references finite.
func walk(start string, next func(string) []string) map[string]bool {
	seen := map[string]bool{start: true}
	queue := []string{start}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, neighbour := range next(current) {
			if !seen[neighbour] {
				seen[neighbour] = true
				queue = append(queue, neighbour)
			}
		}
	}
	return seen
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
