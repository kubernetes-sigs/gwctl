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
	"github.com/emicklei/dot"
	"sigs.k8s.io/gwctl/pkg/common"
	"sigs.k8s.io/gwctl/pkg/topology"
)

// TODO:
//   - Show policy nodes. Attempt to group policy nodes along with their target
//     nodes in a single subgraph so they get rendered closer together.
func ToDot(gwctlGraph *topology.Graph) (string, error) {
	dotGraph := dot.NewGraph(dot.Directed)

	// Create nodes.
	dotNodeMap := map[common.GKNN]dot.Node{}
	for _, nodeMap := range gwctlGraph.Nodes {
		for _, node := range nodeMap {
			dotNode := dotGraph.Node(node.GKNN().String()).
				Attr("style", "filled").
				Attr("color", mapNodeColor(node))

			dotNodeMap[node.GKNN()] = dotNode

			// Set the Node label
			gk := node.GKNN().GroupKind()
			if gk.Group == common.GatewayGK.Group {
				gk.Group = ""
			}
			name := node.GKNN().NamespacedName().String()
			if node.GKNN().Namespace == "" {
				name = node.GKNN().Name
			}
			dotNode.Label(gk.String() + "\n" + name)
		}
	}

	// Create edges.
	for fromNodeGKNN, dotFromNode := range dotNodeMap {
		fromNode := gwctlGraph.Nodes[fromNodeGKNN.GroupKind()][fromNodeGKNN.NamespacedName()]

		for relation, outNodeMap := range fromNode.OutNeighbors {
			for toNodeGKNN := range outNodeMap {
				dotToNode := dotNodeMap[toNodeGKNN]

				// If this is an edge from an HTTPRoute to a Service, then
				// reverse the direction of the edge (to affect the rank), and
				// then reverse the display again to show the correct direction.
				// The end result being that Services now get assigned the
				// correct rank.
				reverse := (fromNode.GKNN().GroupKind() == common.HTTPRouteGK && toNodeGKNN.GroupKind() == common.ServiceGK) ||
					(fromNode.GKNN().GroupKind() == common.GatewayGK && toNodeGKNN.GroupKind() == common.NamespaceGK)
				u, v := dotFromNode, dotToNode
				if reverse {
					u, v = v, u
				}

				e := dotGraph.Edge(u, v, relation.Name)

				if reverse {
					e.Attr("dir", "back")
				}
				// Create a dotted line for the relation to the namespace.
				if toNodeGKNN.Kind == common.NamespaceGK.Kind {
					e.Dotted()
				}
			}
		}
	}

	return dotGraph.String(), nil
}

func mapNodeColor(node *topology.Node) string {
	switch node.GKNN().GroupKind() {
	case common.NamespaceGK:
		return "#d08770"
	case common.GatewayClassGK:
		return "#e5e9f0"
	case common.GatewayGK:
		return "#ebcb8b"
	case common.HTTPRouteGK:
		return "#a3be8c"
	case common.ServiceGK:
		return "#88c0d0"
	}
	return "#d8dee9"
}
