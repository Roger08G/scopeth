package analysis

import "sort"

func stronglyConnectedComponents(nodes []string, adjacency map[string][]string) [][]string {
	index := 0
	indices := make(map[string]int, len(nodes))
	lowlink := make(map[string]int, len(nodes))
	onStack := make(map[string]bool, len(nodes))
	stack := make([]string, 0, len(nodes))
	var components [][]string
	var connect func(string)
	connect = func(node string) {
		indices[node] = index
		lowlink[node] = index
		index++
		stack = append(stack, node)
		onStack[node] = true
		for _, target := range adjacency[node] {
			if _, seen := indices[target]; !seen {
				connect(target)
				lowlink[node] = min(lowlink[node], lowlink[target])
			} else if onStack[target] {
				lowlink[node] = min(lowlink[node], indices[target])
			}
		}
		if lowlink[node] != indices[node] {
			return
		}
		var component []string
		for {
			last := len(stack) - 1
			member := stack[last]
			stack = stack[:last]
			onStack[member] = false
			component = append(component, member)
			if member == node {
				break
			}
		}
		sort.Strings(component)
		components = append(components, component)
	}
	for _, node := range nodes {
		if _, seen := indices[node]; !seen {
			connect(node)
		}
	}
	return components
}

func calleeFirstComponentOrder(components [][]string, adjacency map[string][]string) []int {
	componentOf := make(map[string]int)
	for index, component := range components {
		for _, functionID := range component {
			componentOf[functionID] = index
		}
	}
	edges := make(map[int]map[int]struct{}, len(components))
	for from, targets := range adjacency {
		fromComponent := componentOf[from]
		if edges[fromComponent] == nil {
			edges[fromComponent] = make(map[int]struct{})
		}
		for _, target := range targets {
			toComponent := componentOf[target]
			if fromComponent != toComponent {
				edges[fromComponent][toComponent] = struct{}{}
			}
		}
	}
	indices := make([]int, len(components))
	for index := range components {
		indices[index] = index
	}
	sort.Slice(indices, func(i, j int) bool { return components[indices[i]][0] < components[indices[j]][0] })
	visited := make(map[int]bool, len(components))
	var order []int
	var visit func(int)
	visit = func(component int) {
		if visited[component] {
			return
		}
		visited[component] = true
		dependencies := make([]int, 0, len(edges[component]))
		for dependency := range edges[component] {
			dependencies = append(dependencies, dependency)
		}
		sort.Slice(dependencies, func(i, j int) bool { return components[dependencies[i]][0] < components[dependencies[j]][0] })
		for _, dependency := range dependencies {
			visit(dependency)
		}
		order = append(order, component)
	}
	for _, component := range indices {
		visit(component)
	}
	return order
}
