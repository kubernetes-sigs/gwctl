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
	"maps"
	"slices"

	"github.com/emicklei/dot"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/gwctl/pkg/common"
	"sigs.k8s.io/gwctl/pkg/topology"
)

// TODO:
//   - Show policy nodes. Attempt to group policy nodes along with their target
//     nodes in a single subgraph so they get rendered closer together.
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

	nsList := slices.Collect(maps.Keys(namespaces))
	slices.Sort(nsList)

	for _, ns := range nsList {

		cluster := dotGraph.Subgraph("cluster_"+ns, dot.ClusterOption{})
		cluster.Attr("label", "Namespace: "+ns)
		cluster.Attr("style", "dashed")
		cluster.Attr("color", "black")
		clusterMap[ns] = cluster
	}

	// Create nodes.
	dotNodeMap := map[common.GKNN]dot.Node{}

	groupKinds := make([]schema.GroupKind, 0, len(gwctlGraph.Nodes))
	for gk := range gwctlGraph.Nodes {
		groupKinds = append(groupKinds, gk)
	}

	slices.SortFunc(groupKinds, func(a, b schema.GroupKind) int {
		if c := cmp.Compare(a.Group, b.Group); c != 0 {
			return c
		}
		return cmp.Compare(a.Kind, b.Kind)
	})

	for _, gk := range groupKinds {

		nodeMap := gwctlGraph.Nodes[gk]

		namespacedNames := make([]types.NamespacedName, 0, len(nodeMap))

		for nn := range nodeMap {
			namespacedNames = append(namespacedNames, nn)
		}

		slices.SortFunc(namespacedNames, func(a, b types.NamespacedName) int {
			if c := cmp.Compare(a.Namespace, b.Namespace); c != 0 {
				return c
			}
			return cmp.Compare(a.Name, b.Name)
		})

		for _, nn := range namespacedNames {
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
				Attr("style", "filled").
				Attr("color", mapNodeColor(node))

			dotNodeMap[node.GKNN()] = dotNode

			// Set the Node label
			gk := node.GKNN().GroupKind()
			if gk.Group == common.GatewayGK.Group {
				gk.Group = ""
			}
			dotNode.Label(gk.String() + "\n" + node.GKNN().Name)
		}
	}

	gknnList := make([]common.GKNN, 0, len(dotNodeMap))

	for gknn := range dotNodeMap {
		gknnList = append(gknnList, gknn)
	}
	slices.SortFunc(gknnList, func(a, b common.GKNN) int {
		if c := cmp.Compare(a.Group, b.Group); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Kind, b.Kind); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Namespace, b.Namespace); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})

	// Create edges.
	for _, fromNodeGKNN := range gknnList {

		dotFromNode := dotNodeMap[fromNodeGKNN]
		fromNode := gwctlGraph.Nodes[fromNodeGKNN.GroupKind()][fromNodeGKNN.NamespacedName()]

		relations := make([]*topology.Relation, 0, len(fromNode.OutNeighbors))

		for relation := range fromNode.OutNeighbors {
			relations = append(relations, relation)
		}

		slices.SortFunc(relations, func(a, b *topology.Relation) int {
			return cmp.Compare(a.Name, b.Name)
		})

		for _, relation := range relations {
			outNodeMap := fromNode.OutNeighbors[relation]

			toGKNNList := make([]common.GKNN, 0, len(outNodeMap))

			for toNodeGKNN := range outNodeMap {
				toGKNNList = append(toGKNNList, toNodeGKNN)
			}

			slices.SortFunc(toGKNNList, func(a, b common.GKNN) int {
				if c := cmp.Compare(a.Group, b.Group); c != 0 {
					return c
				}
				if c := cmp.Compare(a.Kind, b.Kind); c != 0 {
					return c
				}
				if c := cmp.Compare(a.Namespace, b.Namespace); c != 0 {
					return c
				}
				return cmp.Compare(a.Name, b.Name)
			})

			for _, toNodeGKNN := range toGKNNList {

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
	}

	return dotGraph.String(), nil
}

func mapNodeColor(node *topology.Node) string {
	switch node.GKNN().GroupKind() {
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
