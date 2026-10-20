package app

import "errors"

// validateDAG is shared by workflow definitions and project collaboration plans.
// Dependencies name predecessors; completion is checked separately at dispatch.
func validateDAG(dependencies map[string][]string) error {
	degree := map[string]int{}
	children := map[string][]string{}
	queue := []string{}
	for node, parents := range dependencies {
		seen := map[string]bool{}
		for _, parent := range parents {
			if _, ok := dependencies[parent]; !ok || parent == node || seen[parent] {
				return errors.New("依赖包含未知节点、自引用或重复连线")
			}
			seen[parent] = true
			children[parent] = append(children[parent], node)
		}
		degree[node] = len(parents)
		if len(parents) == 0 {
			queue = append(queue, node)
		}
	}
	visited := 0
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		visited++
		for _, child := range children[node] {
			degree[child]--
			if degree[child] == 0 {
				queue = append(queue, child)
			}
		}
	}
	if visited != len(dependencies) {
		return errors.New("依赖存在环路，仅支持无环计划")
	}
	return nil
}
