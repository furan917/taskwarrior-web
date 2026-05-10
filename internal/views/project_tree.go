package views

import (
	"sort"
	"strings"
)

// ProjectTreeNode is one node in the project name hierarchy. Taskwarrior uses
// dot-notation (e.g. "work.client.acme") to express parent/child relationships
// between projects. BuildProjectTree splits those names and assembles them into
// a tree so the Browse page can render collapsible branches instead of a flat list.
type ProjectTreeNode struct {
	Segment     string // the last dot-segment of this node's name
	FullName    string // full dot-joined name, used as the /project/<name> href
	SelfCount   int    // open tasks attributed to exactly this project name
	TotalCount  int    // SelfCount + sum of all descendant TotalCounts
	HasOwnTasks bool   // SelfCount > 0; controls whether the node links to /project/<FullName>
	Children    []ProjectTreeNode
}

// BuildProjectTree converts the flat []Counted list (project name + open-task
// count) into a tree of ProjectTreeNodes. Children within each node are sorted
// by TotalCount descending then FullName ascending, matching the existing
// sortedCounted order for the flat Browse list.
func BuildProjectTree(items []Counted) []ProjectTreeNode {
	if len(items) == 0 {
		return nil
	}

	// nodeMap holds pointers during construction so we can wire children and
	// accumulate TotalCount without being tripped up by Go map iteration order.
	nodeMap := map[string]*ProjectTreeNode{}

	for _, item := range items {
		segments := strings.Split(item.Name, ".")
		for i, seg := range segments {
			full := strings.Join(segments[:i+1], ".")
			if _, ok := nodeMap[full]; !ok {
				nodeMap[full] = &ProjectTreeNode{
					Segment:  seg,
					FullName: full,
				}
			}
			if i == len(segments)-1 {
				nodeMap[full].SelfCount = item.Count
				nodeMap[full].HasOwnTasks = item.Count > 0
			}
		}
	}

	// Record parent→children relationships using pointers from nodeMap.
	// We must NOT append value copies here — the children map is built from
	// nodeMap pointers so that each entry always refers to the canonical node.
	ptrChildren := make(map[string][]*ProjectTreeNode)
	var rootPtrs []*ProjectTreeNode
	for full := range nodeMap {
		dot := strings.LastIndex(full, ".")
		if dot == -1 {
			rootPtrs = append(rootPtrs, nodeMap[full])
		} else {
			parentFull := full[:dot]
			ptrChildren[parentFull] = append(ptrChildren[parentFull], nodeMap[full])
		}
	}

	// Recursively build a value tree bottom-up, accumulating TotalCount.
	var buildNode func(full string) ProjectTreeNode
	buildNode = func(full string) ProjectTreeNode {
		node := *nodeMap[full]
		node.Children = nil
		for _, childPtr := range ptrChildren[full] {
			node.Children = append(node.Children, buildNode(childPtr.FullName))
		}
		node.TotalCount = node.SelfCount
		for _, c := range node.Children {
			node.TotalCount += c.TotalCount
		}
		sortNodes(node.Children)
		return node
	}

	result := make([]ProjectTreeNode, 0, len(rootPtrs))
	for _, r := range rootPtrs {
		result = append(result, buildNode(r.FullName))
	}
	sortNodes(result)
	return result
}

func sortNodes(nodes []ProjectTreeNode) {
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].TotalCount != nodes[j].TotalCount {
			return nodes[i].TotalCount > nodes[j].TotalCount
		}
		return nodes[i].FullName < nodes[j].FullName
	})
}
