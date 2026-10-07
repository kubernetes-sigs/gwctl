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
	"fmt"
	"maps"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"sigs.k8s.io/gwctl/pkg/common"
	"sigs.k8s.io/gwctl/pkg/topology"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var (
	AllRelations = []*topology.Relation{
		GatewayParentGatewayClassRelation,
		HTTPRouteParentGatewaysRelation,
		HTTPRouteChildBackendRefsRelation,
		GRPCRouteParentGatewaysRelation,
		GRPCRouteChildBackendRefsRelation,
		GatewayNamespace,
		HTTPRouteNamespace,
		GRPCRouteNamespace,
		BackendNamespace,
	}

	// GatewayParentGatewayClassRelation returns GatewayClass for the Gateway.
	GatewayParentGatewayClassRelation = &topology.Relation{
		From: common.GatewayGK,
		To:   common.GatewayClassGK,
		Name: "GatewayClass",
		NeighborFunc: func(u *unstructured.Unstructured) []common.GKNN {
			gateway := &gatewayv1.Gateway{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.UnstructuredContent(), gateway); err != nil {
				panic(fmt.Sprintf("failed to convert unstructured Gateway to structured: %v", err))
			}
			return []common.GKNN{{
				Group: common.GatewayClassGK.Group,
				Kind:  common.GatewayClassGK.Kind,
				Name:  string(gateway.Spec.GatewayClassName),
			}}
		},
	}

	// HTTPRouteParentGatewayRelation returns Gateways which the HTTPRoute is
	// attached to.
	HTTPRouteParentGatewaysRelation = &topology.Relation{
		From: common.HTTPRouteGK,
		To:   common.GatewayGK,
		Name: "ParentRef",
		NeighborFunc: func(u *unstructured.Unstructured) []common.GKNN {
			httpRoute := &gatewayv1.HTTPRoute{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.UnstructuredContent(), httpRoute); err != nil {
				panic(fmt.Sprintf("failed to convert unstructured HTTPRoute to structured: %v", err))
			}
			result := []common.GKNN{}
			for _, gatewayRef := range httpRoute.Spec.ParentRefs {
				namespace := httpRoute.GetNamespace()
				if namespace == "" {
					namespace = metav1.NamespaceDefault
				}
				if gatewayRef.Namespace != nil {
					namespace = string(*gatewayRef.Namespace)
				}

				result = append(result, common.GKNN{
					Group:     common.GatewayGK.Group,
					Kind:      common.GatewayGK.Kind,
					Namespace: namespace,
					Name:      string(gatewayRef.Name),
				})
			}
			return result
		},
	}

	// HTTPRouteChildBackendRefsRelation returns Backends which the HTTPRoute
	// references.
	HTTPRouteChildBackendRefsRelation = &topology.Relation{
		From: common.HTTPRouteGK,
		To:   common.ServiceGK,
		Name: "BackendRef",
		NeighborFunc: func(u *unstructured.Unstructured) []common.GKNN {
			httpRoute := &gatewayv1.HTTPRoute{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.UnstructuredContent(), httpRoute); err != nil {
				panic(fmt.Sprintf("failed to convert unstructured HTTPRoute to structured: %v", err))
			}
			// Aggregate all BackendRefs
			var backendRefs []gatewayv1.BackendObjectReference
			for _, rule := range httpRoute.Spec.Rules {
				for _, backendRef := range rule.BackendRefs {
					backendRefs = append(backendRefs, backendRef.BackendObjectReference)
				}
				for _, filter := range rule.Filters {
					if filter.Type != gatewayv1.HTTPRouteFilterRequestMirror {
						continue
					}
					if filter.RequestMirror == nil {
						continue
					}
					backendRefs = append(backendRefs, filter.RequestMirror.BackendRef)
				}
			}

			return backendRefsToUniqueGKNNs(backendRefs, httpRoute.GetNamespace())
		},
	}

	// GRPCRouteParentGatewaysRelation returns Gateways which the GRPCRoute is
	// attached to.
	GRPCRouteParentGatewaysRelation = &topology.Relation{
		From: common.GRPCRouteGK,
		To:   common.GatewayGK,
		Name: "ParentRef",
		NeighborFunc: func(u *unstructured.Unstructured) []common.GKNN {
			grpcRoute := &gatewayv1.GRPCRoute{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.UnstructuredContent(), grpcRoute); err != nil {
				panic(fmt.Sprintf("failed to convert unstructured GRPCRoute to structured: %v", err))
			}
			result := []common.GKNN{}
			for _, gatewayRef := range grpcRoute.Spec.ParentRefs {
				// ParentRefs may target other kinds (for example a Service in
				// mesh deployments); only emit edges for refs that actually
				// point at a Gateway. Both Group and Kind default to the
				// Gateway values when unset.
				if gatewayRef.Group != nil && string(*gatewayRef.Group) != common.GatewayGK.Group {
					continue
				}
				if gatewayRef.Kind != nil && string(*gatewayRef.Kind) != common.GatewayGK.Kind {
					continue
				}

				namespace := grpcRoute.GetNamespace()
				if namespace == "" {
					namespace = metav1.NamespaceDefault
				}
				if gatewayRef.Namespace != nil {
					namespace = string(*gatewayRef.Namespace)
				}

				result = append(result, common.GKNN{
					Group:     common.GatewayGK.Group,
					Kind:      common.GatewayGK.Kind,
					Namespace: namespace,
					Name:      string(gatewayRef.Name),
				})
			}
			return result
		},
	}

	// GRPCRouteChildBackendRefsRelation returns Backends which the GRPCRoute
	// references.
	GRPCRouteChildBackendRefsRelation = &topology.Relation{
		From: common.GRPCRouteGK,
		To:   common.ServiceGK,
		Name: "BackendRef",
		NeighborFunc: func(u *unstructured.Unstructured) []common.GKNN {
			grpcRoute := &gatewayv1.GRPCRoute{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.UnstructuredContent(), grpcRoute); err != nil {
				panic(fmt.Sprintf("failed to convert unstructured GRPCRoute to structured: %v", err))
			}
			// Aggregate all BackendRefs
			var backendRefs []gatewayv1.BackendObjectReference
			for _, rule := range grpcRoute.Spec.Rules {
				for _, backendRef := range rule.BackendRefs {
					backendRefs = append(backendRefs, backendRef.BackendObjectReference)
				}
				for _, filter := range rule.Filters {
					if filter.Type != gatewayv1.GRPCRouteFilterRequestMirror {
						continue
					}
					if filter.RequestMirror == nil {
						continue
					}
					backendRefs = append(backendRefs, filter.RequestMirror.BackendRef)
				}
			}

			return backendRefsToUniqueGKNNs(backendRefs, grpcRoute.GetNamespace())
		},
	}

	// GatewayNamespace returns the Namespace for the Gateway.
	GatewayNamespace = &topology.Relation{
		From: common.GatewayGK,
		To:   common.NamespaceGK,
		Name: "Namespace",
		NeighborFunc: func(u *unstructured.Unstructured) []common.GKNN {
			return []common.GKNN{{
				Group: common.NamespaceGK.Group,
				Kind:  common.NamespaceGK.Kind,
				Name:  u.GetNamespace(),
			}}
		},
	}

	// HTTPRouteNamespace returns the Namespace for the HTTPRoute.
	HTTPRouteNamespace = &topology.Relation{
		From: common.HTTPRouteGK,
		To:   common.NamespaceGK,
		Name: "Namespace",
		NeighborFunc: func(u *unstructured.Unstructured) []common.GKNN {
			return []common.GKNN{{
				Group: common.NamespaceGK.Group,
				Kind:  common.NamespaceGK.Kind,
				Name:  u.GetNamespace(),
			}}
		},
	}

	// GRPCRouteNamespace returns the Namespace for the GRPCRoute.
	GRPCRouteNamespace = &topology.Relation{
		From: common.GRPCRouteGK,
		To:   common.NamespaceGK,
		Name: "Namespace",
		NeighborFunc: func(u *unstructured.Unstructured) []common.GKNN {
			return []common.GKNN{{
				Group: common.NamespaceGK.Group,
				Kind:  common.NamespaceGK.Kind,
				Name:  u.GetNamespace(),
			}}
		},
	}

	// BackendNamespace returns the Namespace for the Gateway.
	BackendNamespace = &topology.Relation{
		From: common.ServiceGK,
		To:   common.NamespaceGK,
		Name: "Namespace",
		NeighborFunc: func(u *unstructured.Unstructured) []common.GKNN {
			return []common.GKNN{{
				Group: common.NamespaceGK.Group,
				Kind:  common.NamespaceGK.Kind,
				Name:  u.GetNamespace(),
			}}
		},
	}
)

// backendRefsToUniqueGKNNs converts BackendRefs to GKNNs and deduplicates
// them. GKNN does not use pointers and thus is easily comparable.
func backendRefsToUniqueGKNNs(backendRefs []gatewayv1.BackendObjectReference, routeNamespace string) []common.GKNN {
	resultSet := make(map[common.GKNN]bool)
	for _, backendRef := range backendRefs {
		objRef := common.GKNN{
			Name: string(backendRef.Name),
			// Assume namespace is unspecified in the backendRef and
			// check later to override the default value.
			Namespace: routeNamespace,
		}
		if backendRef.Group != nil {
			objRef.Group = string(*backendRef.Group)
		}
		if backendRef.Kind != nil {
			objRef.Kind = string(*backendRef.Kind)
		} else {
			// Although for resources existing on the server, this value
			// should have received a default before getting persisted.
			// We still explicitly set this for the local analysis when
			// the defaults do not get set automatically.
			objRef.Kind = common.ServiceGK.Kind
		}
		if backendRef.Namespace != nil {
			objRef.Namespace = string(*backendRef.Namespace)
		}
		resultSet[objRef] = true
	}

	// Return unique objRefs
	var result []common.GKNN
	for objRef := range resultSet {
		result = append(result, objRef)
	}
	return result
}

// routeRelations maps each Route kind to the relations used to traverse from
// that Route to its neighbors.
var routeRelations = map[schema.GroupKind]struct {
	parentGateways   *topology.Relation
	childBackendRefs *topology.Relation
	namespace        *topology.Relation
}{
	common.HTTPRouteGK: {
		parentGateways:   HTTPRouteParentGatewaysRelation,
		childBackendRefs: HTTPRouteChildBackendRefsRelation,
		namespace:        HTTPRouteNamespace,
	},
	common.GRPCRouteGK: {
		parentGateways:   GRPCRouteParentGatewaysRelation,
		childBackendRefs: GRPCRouteChildBackendRefsRelation,
		namespace:        GRPCRouteNamespace,
	},
}

type gatewayClassNode interface {
	Gateways() map[common.GKNN]*topology.Node
}

type gatewayNodeClassImpl struct {
	node *topology.Node
}

func GatewayClassNode(node *topology.Node) gatewayClassNode { //nolint:revive
	return &gatewayNodeClassImpl{node: node}
}

func (n *gatewayNodeClassImpl) Gateways() map[common.GKNN]*topology.Node {
	return n.node.InNeighbors[GatewayParentGatewayClassRelation]
}

type gatewayNode interface {
	Namespace() *topology.Node
	GatewayClass() *topology.Node
	Routes() map[common.GKNN]*topology.Node
}

type gatewayNodeImpl struct {
	node *topology.Node
}

func GatewayNode(node *topology.Node) gatewayNode { //nolint:revive
	return &gatewayNodeImpl{node: node}
}

func (n *gatewayNodeImpl) Namespace() *topology.Node {
	for _, namespaceNode := range n.node.OutNeighbors[GatewayNamespace] {
		return namespaceNode
	}
	return nil
}

func (n *gatewayNodeImpl) GatewayClass() *topology.Node {
	for _, gatewayClassNode := range n.node.OutNeighbors[GatewayParentGatewayClassRelation] {
		return gatewayClassNode
	}
	return nil
}

// Routes returns all Routes (of any kind) attached to the Gateway.
func (n *gatewayNodeImpl) Routes() map[common.GKNN]*topology.Node {
	result := make(map[common.GKNN]*topology.Node)
	for _, relations := range routeRelations {
		maps.Copy(result, n.node.InNeighbors[relations.parentGateways])
	}
	return result
}

type routeNode interface {
	Namespace() *topology.Node
	Gateways() map[common.GKNN]*topology.Node
	Backends() map[common.GKNN]*topology.Node
}

type routeNodeImpl struct {
	node *topology.Node
}

// RouteNode wraps a topology Node of any Route kind (HTTPRoute, GRPCRoute)
// with accessors for its neighbors.
func RouteNode(node *topology.Node) routeNode {
	return &routeNodeImpl{node: node}
}

func (n *routeNodeImpl) Namespace() *topology.Node {
	for _, namespaceNode := range n.node.OutNeighbors[routeRelations[n.node.GKNN().GroupKind()].namespace] {
		return namespaceNode
	}
	return nil
}

func (n *routeNodeImpl) Gateways() map[common.GKNN]*topology.Node {
	return n.node.OutNeighbors[routeRelations[n.node.GKNN().GroupKind()].parentGateways]
}

func (n *routeNodeImpl) Backends() map[common.GKNN]*topology.Node {
	return n.node.OutNeighbors[routeRelations[n.node.GKNN().GroupKind()].childBackendRefs]
}

type backendNode interface {
	Namespace() *topology.Node
	Routes() map[common.GKNN]*topology.Node
}

type backendNodeImpl struct {
	node *topology.Node
}

func BackendNode(node *topology.Node) backendNode {
	return &backendNodeImpl{node: node}
}

func (n *backendNodeImpl) Namespace() *topology.Node {
	for _, namespaceNode := range n.node.OutNeighbors[BackendNamespace] {
		return namespaceNode
	}
	return nil
}

// Routes returns all Routes (of any kind) that reference the Backend.
func (n *backendNodeImpl) Routes() map[common.GKNN]*topology.Node {
	result := make(map[common.GKNN]*topology.Node)
	for _, relations := range routeRelations {
		maps.Copy(result, n.node.InNeighbors[relations.childBackendRefs])
	}
	return result
}
