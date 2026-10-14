/*
Copyright 2026 The Kubernetes Authors.

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
	"testing"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	"sigs.k8s.io/gwctl/pkg/common"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// GRPCRoute ParentRefs may target kinds other than Gateway (for example a
// Service in mesh deployments); only refs that actually point at a Gateway
// may produce Gateway edges.
func TestGRPCRouteParentGatewaysRelationSkipsNonGatewayParents(t *testing.T) {
	grpcRoute := &gatewayv1.GRPCRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gatewayv1.GroupVersion.String(),
			Kind:       "GRPCRoute",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "grpc-route",
			Namespace: "ns-1",
		},
		Spec: gatewayv1.GRPCRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{
					// Implicit Gateway (both Group and Kind unset).
					{Name: "implicit-gateway"},
					// Explicit Gateway.
					{
						Group: ptr.To(gatewayv1.Group(common.GatewayGK.Group)),
						Kind:  ptr.To(gatewayv1.Kind("Gateway")),
						Name:  "explicit-gateway",
					},
					// Service parent (mesh case): must not become a Gateway.
					{
						Group: ptr.To(gatewayv1.Group("")),
						Kind:  ptr.To(gatewayv1.Kind("Service")),
						Name:  "parent-svc",
					},
					// Non-Gateway kind in the Gateway API group.
					{
						Kind: ptr.To(gatewayv1.Kind("XListenerSet")),
						Name: "listener-set",
					},
				},
			},
		},
	}

	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(grpcRoute)
	if err != nil {
		t.Fatal(err)
	}

	got := GRPCRouteParentGatewaysRelation.NeighborFunc(&unstructured.Unstructured{Object: obj})

	want := []common.GKNN{
		{
			Group:     common.GatewayGK.Group,
			Kind:      common.GatewayGK.Kind,
			Namespace: "ns-1",
			Name:      "implicit-gateway",
		},
		{
			Group:     common.GatewayGK.Group,
			Kind:      common.GatewayGK.Kind,
			Namespace: "ns-1",
			Name:      "explicit-gateway",
		},
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("unexpected neighbors (-want, +got):\n%s", diff)
	}
}
