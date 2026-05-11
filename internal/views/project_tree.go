package views

import (
	"sort"
	"strings"
)

// ProjectInput is the per-project row fed to BuildProjectTree. Pending is the
// open (pending+waiting) task count; Completed is the finished-task count;
// TotalAgeSecs is the sum of (now − entry) in seconds across all Pending tasks,
// used to compute an average age for the subtree.
type ProjectInput struct {
	Name         string
	Pending      int
	Completed    int
	TotalAgeSecs int64
}

// ProjectTreeNode is one node in the project name hierarchy. Taskwarrior uses
// dot-notation (e.g. "work.client.acme") to express parent/child relationships
// between projects. BuildProjectTree splits those names and assembles them into
// a tree so the Browse page can render collapsible branches instead of a flat list.
type ProjectTreeNode struct {
	Segment        string // the last dot-segment of this node's name
	FullName       string // full dot-joined name, used as the /project/<name> href
	SelfCount      int    // open tasks attributed to exactly this project name
	TotalCount     int    // SelfCount + sum of all descendant TotalCounts (pending)
	SelfCompleted  int    // completed tasks at exactly this project level
	TotalCompleted int    // SelfCompleted + sum of descendant TotalCompleted
	TotalAgeSecs   int64  // sum of pending-task ages in seconds across this subtree
	HasOwnTasks    bool   // SelfCount > 0; controls whether the node links to /project/<FullName>
	Children       []ProjectTreeNode
}

// PercentComplete returns the integer percentage of tasks in this subtree that
// are complete: TotalCompleted / (TotalCount + TotalCompleted) * 100.
// Returns 0 when no tasks of either kind exist.
func (n ProjectTreeNode) PercentComplete() int {
	total := n.TotalCount + n.TotalCompleted
	if total == 0 {
		return 0
	}
	return n.TotalCompleted * 100 / total
}

// AvgAgeDays returns the mean age in days of the pending tasks in this subtree.
func (n ProjectTreeNode) AvgAgeDays() float64 {
	if n.TotalCount == 0 {
		return 0
	}
	return float64(n.TotalAgeSecs) / float64(n.TotalCount) / 86400
}

// BuildProjectTree converts the flat []ProjectInput list into a tree of
// ProjectTreeNodes. Children within each node are sorted by TotalCount
// descending then FullName ascending.
func BuildProjectTree(items []ProjectInput) []ProjectTreeNode {
	if len(items) == 0 {
		return nil
	}

	// nodeMap holds pointers during construction so we can wire children and
	// accumulate counts without being tripped up by Go map iteration order.
	nodeMap := map[string]*ProjectTreeNode{}

	for _, item := range items {
		// Defence-in-depth: a stored project name with a leading/trailing
		// dot (e.g. "work.") would otherwise produce a child node with an
		// empty Segment, rendering as a nameless leaf. Taskwarrior doesn't
		// emit these in normal use; a corrupted/hostile sqlite could. Trim
		// before splitting; if the trimmed name is empty, skip the row.
		name := strings.Trim(item.Name, ".")
		if name == "" {
			continue
		}
		segments := strings.Split(name, ".")
		for i, seg := range segments {
			full := strings.Join(segments[:i+1], ".")
			if _, ok := nodeMap[full]; !ok {
				nodeMap[full] = &ProjectTreeNode{
					Segment:  seg,
					FullName: full,
				}
			}
			if i == len(segments)-1 {
				nodeMap[full].SelfCount = item.Pending
				nodeMap[full].SelfCompleted = item.Completed
				nodeMap[full].TotalAgeSecs = item.TotalAgeSecs
				nodeMap[full].HasOwnTasks = item.Pending > 0
			}
		}
	}

	// Record parent→children relationships using pointers from nodeMap.
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

	// Recursively build a value tree bottom-up, accumulating all subtree totals.
	var buildNode func(full string) ProjectTreeNode
	buildNode = func(full string) ProjectTreeNode {
		node := *nodeMap[full]
		node.Children = nil
		for _, childPtr := range ptrChildren[full] {
			node.Children = append(node.Children, buildNode(childPtr.FullName))
		}
		node.TotalCount = node.SelfCount
		node.TotalCompleted = node.SelfCompleted
		for _, c := range node.Children {
			node.TotalCount += c.TotalCount
			node.TotalCompleted += c.TotalCompleted
			node.TotalAgeSecs += c.TotalAgeSecs
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
