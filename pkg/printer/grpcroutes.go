/*
Copyright The Kubernetes Authors.

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

package printer //nolint:revive

import (
	"io"

	"sigs.k8s.io/gwctl/pkg/topology"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func (p *TablePrinter) printGRPCRoute(grpcRouteNode *topology.Node, w io.Writer) error {
	grpcRoute := topology.MustAccessObject(grpcRouteNode, &gatewayv1.GRPCRoute{})
	return p.printRoute(grpcRouteNode, w, grpcRoute.Spec.Hostnames, grpcRoute.Spec.CommonRouteSpec, grpcRoute.Status.RouteStatus)
}

func (p *DescriptionPrinter) printGRPCRoute(grpcRouteNode *topology.Node, w io.Writer) error {
	grpcRoute := topology.MustAccessObject(grpcRouteNode, &gatewayv1.GRPCRoute{})
	return p.printRoute(grpcRouteNode, w, grpcRoute.ObjectMeta, grpcRoute.Spec, grpcRoute.Status)
}
