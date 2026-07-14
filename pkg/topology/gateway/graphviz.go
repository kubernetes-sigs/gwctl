/*
Copyright 2024 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package gateway

import (
	"cmp"
	"fmt"
	"maps"
	"slices"

	"github.com/emicklei/dot"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/gwctl/pkg/common"
	"sigs.k8s.io/gwctl/pkg/extension/directlyattachedpolicy"
	"sigs.k8s.io/gwctl/pkg/topology"
)

// The DOT graph generated here needs to be deterministic. This makes sure that integration tests
// can validate the output produced. Since the graph is created using maps,
// we can get determinism by converting map keys to slices and sorting them.
func ToDot(gwctlGraph *topology.Graph) (string, error) {
	dotGraph := dot.NewGraph(dot.Directed)
	dotGraph.Attr("rankdir", "BT")
	dotGraph.Attr("compound", "true")

	// Collect all unique namespaces from nodes
	namespaces := map[string]struct{}{}
	for _, nodeMap := range gwctlGraph.Nodes {
		for _, node := range nodeMap {
			if node.GKNN().GroupKind() == common.NamespaceGK {
				continue
			}
			if ns := node.GKNN().Namespace; ns != "" {
				namespaces[ns] = struct{}{}
			}
		}
	}

	// Create subgraphs for each namespace
	clusterMap := map[string]*dot.Graph{}

	for _, ns := range slices.Sorted(maps.Keys(namespaces)) {
		cluster := dotGraph.Subgraph("cluster_"+ns, dot.ClusterOption{})
		cluster.Attr("label", "Namespace: "+ns)
		cluster.Attr("style", "dashed")
		cluster.Attr("color", "black")
		clusterMap[ns] = cluster
	}

	// Create nodes.
	dotNodeMap := map[common.GKNN]dot.Node{}

	for _, gk := range slices.SortedFunc(maps.Keys(gwctlGraph.Nodes), compareByString[schema.GroupKind]) {
		nodeMap := gwctlGraph.Nodes[gk]

		for _, nn := range slices.SortedFunc(maps.Keys(nodeMap), compareByString[types.NamespacedName]) {
			node := nodeMap[nn]

			// Skip Namespace nodes - they will be represented as clusters
			if node.GKNN().GroupKind() == common.NamespaceGK {
				continue
			}

			var targetGraph *dot.Graph
			if ns := node.GKNN().Namespace; ns != "" {
				targetGraph = clusterMap[ns]
			} else {
				targetGraph = dotGraph
			}

			dotNode := targetGraph.Node(node.GKNN().String()).
				Attr("shape", "box").
				Attr("style", "filled,rounded").
				Attr("color", mapNodeColor(node))

			dotNodeMap[node.GKNN()] = dotNode

			// Set the Node label
			gk := node.GKNN().GroupKind()
			if gk.Group == common.GatewayGK.Group {
				gk.Group = ""
			}
			dotNode.Label(gk.String() + "\n" + node.GKNN().Name)

			policies, err := directlyattachedpolicy.Access(node)
			if err != nil {
				return "", fmt.Errorf("failed to access direct attached policies: %w", err)
			}
			for _, gknn := range slices.SortedFunc(maps.Keys(policies), compareByString[common.GKNN]) {
				dotNodeMap[gknn] = targetGraph.Node(gknn.String()).
					Label(gknn.Kind+"\n"+gknn.Name).
					Attr("shape", "box").
					Attr("style", "filled,rounded").
					Attr("color", "#ffd2d2")
			}
		}
	}

	// Create edges.
	for _, fromNodeGKNN := range slices.SortedFunc(maps.Keys(dotNodeMap), compareByString[common.GKNN]) {
		dotFromNode := dotNodeMap[fromNodeGKNN]

		nodes, ok := gwctlGraph.Nodes[fromNodeGKNN.GroupKind()]
		if !ok {
			continue
		}

		fromNode, ok := nodes[fromNodeGKNN.NamespacedName()]
		if !ok {
			continue
		}

		for _, relation := range slices.SortedFunc(maps.Keys(fromNode.OutNeighbors), func(a, b *topology.Relation) int {
			return cmp.Compare(a.Name, b.Name)
		}) {
			outNodeMap := fromNode.OutNeighbors[relation]

			for _, toNodeGKNN := range slices.SortedFunc(maps.Keys(outNodeMap), compareByString[common.GKNN]) {
				// Skip edges to Namespace nodes - namespace relationship are represented by cluster membership
				if toNodeGKNN.GroupKind() == common.NamespaceGK {
					continue
				}

				dotToNode := dotNodeMap[toNodeGKNN]

				// If this is an edge from an HTTPRoute to a Service, then
				// reverse the direction of the edge (to affect the rank), and
				// then reverse the display again to show the correct direction.
				// The end result being that Services now get assigned the
				// correct rank.
				reverse := fromNode.GKNN().GroupKind() == common.HTTPRouteGK && toNodeGKNN.GroupKind() == common.ServiceGK
				u, v := dotFromNode, dotToNode
				if reverse {
					u, v = v, u
				}

				e := dotGraph.Edge(u, v, relation.Name)

				if reverse {
					e.Attr("dir", "back")
				}
			}
		}

		policies, err := directlyattachedpolicy.Access(fromNode)
		if err != nil {
			return "", fmt.Errorf("failed to access direct attached policies: %w", err)
		}
		for _, gknn := range slices.SortedFunc(maps.Keys(policies), compareByString[common.GKNN]) {
			dotGraph.Edge(dotFromNode, dotNodeMap[gknn], "TargetRef").
				Attr("dir", "back").
				Attr("constraint", "false")
		}
	}

	return dotGraph.String(), nil
}

func compareByString[T fmt.Stringer](a, b T) int {
	return cmp.Compare(a.String(), b.String())
}

func mapNodeColor(node *topology.Node) string {
	switch node.GKNN().GroupKind() {
	case common.GatewayClassGK:
		return "#e6d8ff"
	case common.GatewayGK:
		return "#cfe0ff"
	case common.HTTPRouteGK:
		return "#f7efc6"
	case common.ServiceGK:
		return "#d6f5df"
	}
	return "#d8dee9"
}
