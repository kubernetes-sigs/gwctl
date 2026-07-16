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

package gatewayeffectivepolicy

import (
	"fmt"
	"maps"

	"k8s.io/klog/v2"

	"sigs.k8s.io/gwctl/pkg/common"
	"sigs.k8s.io/gwctl/pkg/extension/directlyattachedpolicy"
	"sigs.k8s.io/gwctl/pkg/policymanager"
	"sigs.k8s.io/gwctl/pkg/topology"
	topologygw "sigs.k8s.io/gwctl/pkg/topology/gateway"
)

const (
	extensionName = "InheritedPolicy"
)

type Extension struct{}

func NewExtension() *Extension {
	return &Extension{}
}

// Extension calculates the effective policies for all Gateways, Routes, and
// Backends in the Graph.
func (a *Extension) Execute(graph *topology.Graph) error {
	graph.RemoveMetadata(extensionName)
	if err := a.calculateInheritedPolicies(graph); err != nil {
		return err
	}
	return a.calculateEffectivePolicies(graph)
}

// calculateInheritedPolicies calculates the inherited polices for all Gateways,
// Routes, and Backends in the Graph.
func (a *Extension) calculateInheritedPolicies(graph *topology.Graph) error {
	if err := a.calculateInheritedPoliciesForGateways(graph); err != nil {
		return err
	}
	if err := a.calculateInheritedPoliciesForRoutes(graph); err != nil {
		return err
	}
	if err := a.calculateInheritedPoliciesForBackends(graph); err != nil {
		return err
	}
	return nil
}

// calculateInheritedPoliciesForGateways calculates the inherited policies for
// all Gateways present in the Graph.
func (a *Extension) calculateInheritedPoliciesForGateways(graph *topology.Graph) error {
	for _, gatewayNode := range graph.Nodes[common.GatewayGK] {
		result := make(map[common.GKNN]*policymanager.Policy)

		// Policies inherited from Gateway's namespace.
		namespaceNode := topologygw.GatewayNode(gatewayNode).Namespace()
		if namespaceNode != nil {
			namespacePoliciesMap, err := directlyattachedpolicy.Access(namespaceNode)
			if err != nil {
				return err
			}
			maps.Copy(result, filterInheritablePolicies(namespacePoliciesMap))
		}

		// Policies inherited from GatewayClass.
		gatewayClassNode := topologygw.GatewayNode(gatewayNode).GatewayClass()
		if gatewayClassNode != nil {
			gatewayClassPoliciesMap, err := directlyattachedpolicy.Access(gatewayClassNode)
			if err != nil {
				return err
			}
			maps.Copy(result, filterInheritablePolicies(gatewayClassPoliciesMap))
		}

		gatewayNode.Metadata[extensionName] = &NodeMetadata{GatewayInheritedPolicies: result}
	}
	return nil
}

// calculateInheritedPoliciesForRoutes calculates the inherited policies for
// all Routes present in the Graph.
func (a *Extension) calculateInheritedPoliciesForRoutes(graph *topology.Graph) error {
	for _, routeGK := range common.RouteGKs {
		for _, routeNode := range graph.Nodes[routeGK] {
			result := make(map[common.GKNN]*policymanager.Policy)

			// Policies inherited from Route's namespace.
			namespaceNode := topologygw.RouteNode(routeNode).Namespace()
			if namespaceNode != nil {
				namespacePoliciesMap, err := directlyattachedpolicy.Access(namespaceNode)
				if err != nil {
					return err
				}
				maps.Copy(result, filterInheritablePolicies(namespacePoliciesMap))
			}

			// Policies inherited from Gateways.
			for _, gatewayNode := range topologygw.RouteNode(routeNode).Gateways() {
				// Add policies inherited by GatewayNode.
				effPolicyMetadata, err := Access(gatewayNode)
				if err != nil {
					return err
				}
				if effPolicyMetadata != nil {
					maps.Copy(result, effPolicyMetadata.GatewayInheritedPolicies)
				}

				// Add inheritable policies directly applied to GatewayNode.
				gatewayPoliciesMap, err := directlyattachedpolicy.Access(gatewayNode)
				if err != nil {
					return err
				}
				maps.Copy(result, filterInheritablePolicies(gatewayPoliciesMap))
			}

			routeNode.Metadata[extensionName] = &NodeMetadata{RouteInheritedPolicies: result}
		}
	}
	return nil
}

// calculateInheritedPoliciesForBackends calculates the inherited policies for
// all Backends present in ResourceModel.
func (a *Extension) calculateInheritedPoliciesForBackends(graph *topology.Graph) error {
	for _, backendNode := range graph.Nodes[common.ServiceGK] {
		result := make(map[common.GKNN]*policymanager.Policy)

		// Policies inherited from Backend's namespace.
		namespaceNode := topologygw.BackendNode(backendNode).Namespace()
		if namespaceNode != nil {
			namespacePoliciesMap, err := directlyattachedpolicy.Access(namespaceNode)
			if err != nil {
				return err
			}
			maps.Copy(result, filterInheritablePolicies(namespacePoliciesMap))
		}

		// Policies inherited from Routes.
		for _, routeNode := range topologygw.BackendNode(backendNode).Routes() {
			// Add policies inherited by RouteNode.
			effPolicyMetadata, err := Access(routeNode)
			if err != nil {
				return err
			}
			if effPolicyMetadata != nil {
				maps.Copy(result, effPolicyMetadata.RouteInheritedPolicies)
			}

			// Add inheritable policies directly applied to RouteNode.
			routePoliciesMap, err := directlyattachedpolicy.Access(routeNode)
			if err != nil {
				return err
			}
			maps.Copy(result, filterInheritablePolicies(routePoliciesMap))
		}

		backendNode.Metadata[extensionName] = &NodeMetadata{BackendInheritedPolicies: result}
	}
	return nil
}

// filterInheritablePolicies filters and returns policies which can be inherited.
func filterInheritablePolicies(policies map[common.GKNN]*policymanager.Policy) map[common.GKNN]*policymanager.Policy {
	inheritablePolicies := make(map[common.GKNN]*policymanager.Policy)

	for gknn, policy := range policies {
		if policy.IsInheritable() {
			inheritablePolicies[gknn] = policy
		}
	}

	return inheritablePolicies
}

func (a *Extension) calculateEffectivePolicies(graph *topology.Graph) error {
	if err := a.calculateEffectivePoliciesForGateways(graph); err != nil {
		return err
	}
	if err := a.calculateEffectivePoliciesForRoutes(graph); err != nil {
		return err
	}
	if err := a.calculateEffectivePoliciesForBackends(graph); err != nil {
		return err
	}
	return nil
}

// calculateEffectivePoliciesForGateways calculates the effective policies for
// each Gateway by merging policies from different hierarchies (GatewayClass,
// Namespace, and Gateway).
func (a *Extension) calculateEffectivePoliciesForGateways(graph *topology.Graph) error {
	for _, gatewayNode := range graph.Nodes[common.GatewayGK] {
		if gatewayNode.Depth > graph.MaxDepth {
			continue
		}

		gatewayClassNode := topologygw.GatewayNode(gatewayNode).GatewayClass()
		if gatewayClassNode == nil {
			klog.V(3).InfoS("No GatewayClass node found for Gateway, skipping effective policy calculation", "gateway", gatewayNode.GKNN())
			continue
		}
		namespaceNode := topologygw.GatewayNode(gatewayNode).Namespace()
		if namespaceNode == nil {
			klog.V(3).InfoS("No Namespace node found for Gateway, skipping effective policy calculation", "gateway", gatewayNode.GKNN())
			continue
		}

		gatewayClassPoliciesMap, err := directlyattachedpolicy.Access(gatewayClassNode)
		if err != nil {
			return err
		}
		namespacePoliciesMap, err := directlyattachedpolicy.Access(namespaceNode)
		if err != nil {
			return err
		}
		gatewayPoliciesMap, err := directlyattachedpolicy.Access(gatewayNode)
		if err != nil {
			return err
		}

		// Do not calculate effective policy for the Gateway if the referenced
		// GatewayClass does not exist. For now, we only calculate effective policy
		// once the references are corrected.
		if gatewayClassNode == nil {
			continue
		}

		// Fetch all policies.
		gatewayClassPolicies := policymanager.ConvertPoliciesMapToSlice(filterInheritablePolicies(gatewayClassPoliciesMap))
		gatewayNamespacePolicies := policymanager.ConvertPoliciesMapToSlice(filterInheritablePolicies(namespacePoliciesMap))
		gatewayPolicies := policymanager.ConvertPoliciesMapToSlice(filterInheritablePolicies(gatewayPoliciesMap))

		// Merge policies by their kind.
		gatewayClassPoliciesByKind, err := policymanager.MergePoliciesOfSimilarKind(gatewayClassPolicies)
		if err != nil {
			return err
		}
		gatewayNamespacePoliciesByKind, err := policymanager.MergePoliciesOfSimilarKind(gatewayNamespacePolicies)
		if err != nil {
			return err
		}
		gatewayPoliciesByKind, err := policymanager.MergePoliciesOfSimilarKind(gatewayPolicies)
		if err != nil {
			return err
		}

		// Merge all hierarchial policies.
		result, err := policymanager.MergePoliciesOfDifferentHierarchy(gatewayClassPoliciesByKind, gatewayNamespacePoliciesByKind)
		if err != nil {
			return err
		}

		result, err = policymanager.MergePoliciesOfDifferentHierarchy(result, gatewayPoliciesByKind)
		if err != nil {
			return err
		}

		gatewayNodeMetadata, err := Access(gatewayNode)
		if err != nil {
			return err
		}
		if gatewayNodeMetadata == nil {
			gatewayNodeMetadata = &NodeMetadata{}
			gatewayNode.Metadata[extensionName] = gatewayNodeMetadata
		}
		gatewayNodeMetadata.GatewayEffectivePolicies = result
	}
	return nil
}

// calculateEffectivePoliciesForRoutes calculates the effective policies for
// each Route, taking into account policies from different hierarchies
// (GatewayClass, Namespace, Gateway, and Route).
func (a *Extension) calculateEffectivePoliciesForRoutes(graph *topology.Graph) error {
	for _, routeGK := range common.RouteGKs {
		for _, routeNode := range graph.Nodes[routeGK] {
			result := make(map[common.GKNN]map[policymanager.PolicyCrdID]*policymanager.Policy)

			namespaceNode := topologygw.RouteNode(routeNode).Namespace()
			if namespaceNode == nil {
				klog.V(3).InfoS("No Namespace node found for Route, skipping effective policy calculation", "route", routeNode.GKNN())
				continue
			}

			routePoliciesMap, err := directlyattachedpolicy.Access(routeNode)
			if err != nil {
				return err
			}
			namespacePoliciesMap, err := directlyattachedpolicy.Access(namespaceNode)
			if err != nil {
				return err
			}

			// Step 1: Aggregate all policies of the Route and the
			// Route-namespace.
			routePolicies := policymanager.ConvertPoliciesMapToSlice(filterInheritablePolicies(routePoliciesMap))
			routeNamespacePolicies := policymanager.ConvertPoliciesMapToSlice(filterInheritablePolicies(namespacePoliciesMap))

			// Step 2: Merge Route and Route-namespace policies by their kind.
			routePoliciesByKind, err := policymanager.MergePoliciesOfSimilarKind(routePolicies)
			if err != nil {
				return err
			}
			routeNamespacePoliciesByKind, err := policymanager.MergePoliciesOfSimilarKind(routeNamespacePolicies)
			if err != nil {
				return err
			}

			// Step 3: Loop through all Gateways and merge policies for each Gateway.
			// End result is we get policies partitioned by each Gateway.
			for gatewayGKNN, gatewayNode := range topologygw.RouteNode(routeNode).Gateways() {
				gatewayNodeMetadata, err := Access(gatewayNode) //nolint:govet
				if err != nil {
					return err
				}
				gatewayPoliciesByKind := gatewayNodeMetadata.GatewayEffectivePolicies

				// Merge all hierarchial policies.
				mergedPolicies, err := policymanager.MergePoliciesOfDifferentHierarchy(gatewayPoliciesByKind, routeNamespacePoliciesByKind)
				if err != nil {
					return err
				}

				mergedPolicies, err = policymanager.MergePoliciesOfDifferentHierarchy(mergedPolicies, routePoliciesByKind)
				if err != nil {
					return err
				}

				result[gatewayGKNN] = mergedPolicies
			}

			routeNodeMetadata, err := Access(routeNode)
			if err != nil {
				return err
			}
			if routeNodeMetadata == nil {
				routeNodeMetadata = &NodeMetadata{}
				routeNode.Metadata[extensionName] = routeNodeMetadata
			}
			routeNodeMetadata.RouteEffectivePolicies = result
		}
	}
	return nil
}

// calculateEffectivePoliciesForBackends calculates the effective policies for
// each Backend, considering policies from different hierarchies (GatewayClass,
// Namespace, Gateway, Route, and Backend).
func (a *Extension) calculateEffectivePoliciesForBackends(graph *topology.Graph) error {
	for _, backendNode := range graph.Nodes[common.ServiceGK] {
		result := make(map[common.GKNN]map[policymanager.PolicyCrdID]*policymanager.Policy)

		namespaceNode := topologygw.BackendNode(backendNode).Namespace()
		if namespaceNode == nil {
			klog.V(3).InfoS("No Namespace node found for Backend, skipping effective policy calculation", "backend", backendNode.GKNN())
			continue
		}

		backendPoliciesMap, err := directlyattachedpolicy.Access(backendNode)
		if err != nil {
			return err
		}
		namespacePoliciesMap, err := directlyattachedpolicy.Access(namespaceNode)
		if err != nil {
			return err
		}

		// Step 1: Aggregate all policies of the Backend and the Backend-namespace.
		backendPolicies := policymanager.ConvertPoliciesMapToSlice(filterInheritablePolicies(backendPoliciesMap))
		backendNamespacePolicies := policymanager.ConvertPoliciesMapToSlice(filterInheritablePolicies(namespacePoliciesMap))

		// Step 2: Merge Backend and Backend-namespace policies by their kind.
		backendPoliciesByKind, err := policymanager.MergePoliciesOfSimilarKind(backendPolicies)
		if err != nil {
			return err
		}
		backendNamespacePoliciesByKind, err := policymanager.MergePoliciesOfSimilarKind(backendNamespacePolicies)
		if err != nil {
			return err
		}

		// Step 3: Loop through all Routes and get their effective policies. Merge
		// effective policies such that we get policies partitioned by Gateway.
		for _, routeNode := range topologygw.BackendNode(backendNode).Routes() {
			routeNodeMetadata, err := Access(routeNode) //nolint:govet
			if err != nil {
				return err
			}
			if routeNodeMetadata == nil {
				klog.V(3).InfoS("No effective policy metadata found for Route, skipping", "route", routeNode.GKNN())
				continue
			}
			routePoliciesByGateway := routeNodeMetadata.RouteEffectivePolicies

			for gatewayID, policies := range routePoliciesByGateway {
				result[gatewayID], err = policymanager.MergePoliciesOfSameHierarchy(result[gatewayID], policies)
				if err != nil {
					return err
				}
			}
		}

		// Step 4: Loop through all Gateways and merge the Backend and
		// Backend-namespace specific policies. Note that this needs to be done
		// separately from Step 4 i.e. we can't have this loop within Step 4 itself.
		// This is because we first want to merge all policies of the same-hierarchy
		// together and then move to the next hierarchy of Backend and
		// Backend-namespace.
		for gatewayID := range result {
			// Merge all hierarchial policies.
			result[gatewayID], err = policymanager.MergePoliciesOfDifferentHierarchy(result[gatewayID], backendNamespacePoliciesByKind)
			if err != nil {
				return err
			}

			result[gatewayID], err = policymanager.MergePoliciesOfDifferentHierarchy(result[gatewayID], backendPoliciesByKind)
			if err != nil {
				return err
			}
		}

		backendNodeMetadata, err := Access(backendNode)
		if err != nil {
			return err
		}
		if backendNodeMetadata == nil {
			backendNodeMetadata = &NodeMetadata{}
			backendNode.Metadata[extensionName] = backendNodeMetadata
		}
		backendNodeMetadata.BackendEffectivePolicies = result
	}
	return nil
}

type NodeMetadata struct {
	GatewayInheritedPolicies map[common.GKNN]*policymanager.Policy
	RouteInheritedPolicies   map[common.GKNN]*policymanager.Policy
	BackendInheritedPolicies map[common.GKNN]*policymanager.Policy

	GatewayEffectivePolicies map[policymanager.PolicyCrdID]*policymanager.Policy
	RouteEffectivePolicies   map[common.GKNN]map[policymanager.PolicyCrdID]*policymanager.Policy
	BackendEffectivePolicies map[common.GKNN]map[policymanager.PolicyCrdID]*policymanager.Policy
}

func Access(node *topology.Node) (*NodeMetadata, error) {
	rawData, ok := node.Metadata[extensionName]
	if !ok || rawData == nil {
		klog.V(3).InfoS(fmt.Sprintf("no data found in node for %v", extensionName), "node", node.GKNN())
		return nil, nil
	}
	data, ok := rawData.(*NodeMetadata)
	if !ok {
		return nil, fmt.Errorf("unable to perform type assertion for %v in node %v", extensionName, node.GKNN())
	}
	return data, nil
}
