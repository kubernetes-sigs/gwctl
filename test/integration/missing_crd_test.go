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

package integration

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	cmdget "sigs.k8s.io/gwctl/cmd/get"
	"sigs.k8s.io/gwctl/pkg/common"
)

const testdataWithoutGRPCRoutes = `
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: foo-com-external-gateway-class
spec:
  controllerName: foo.com/external-gateway-class
---
apiVersion: v1
kind: Namespace
metadata:
  name: default
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: gateway-1
  namespace: default
spec:
  gatewayClassName: foo-com-external-gateway-class
  listeners:
  - name: http
    protocol: HTTP
    port: 80
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: httproute-1
  namespace: default
spec:
  parentRefs:
  - kind: Gateway
    name: gateway-1
  rules:
  - backendRefs:
    - name: svc-1
      port: 80
---
apiVersion: v1
kind: Service
metadata:
  name: svc-1
  namespace: default
spec:
  type: ClusterIP
`

// Regression test for graph expansion on clusters where the GRPCRoute CRD is
// not installed: fetches done for relation expansion must skip the unknown
// resource type instead of failing the whole command.
func TestGetSucceedsWithoutGRPCRouteCRD(t *testing.T) {
	factory := NewTestFactoryWithoutResources(t, []string{"grpcroutes"}, testdataWithoutGRPCRoutes)
	factory.namespace = "default"

	iostreams, _, out, errOut := genericiooptions.NewTestIOStreams()
	// `-o wide` forces relationship expansion, which includes the GRPCRoute
	// relations.
	cmd := cmdget.NewCmd(factory, iostreams, false)
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"gateways", "-o", "wide"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("get gateways -o wide should succeed without the GRPCRoute CRD, got: %v\nout=%v\nerrOut=%v",
			err, out.String(), errOut.String())
	}

	wantOut := `
NAME       CLASS                           ADDRESSES  PORTS  PROGRAMMED  AGE        POLICIES  ROUTES
gateway-1  foo-com-external-gateway-class             80     Unknown     <unknown>  0         1
`

	got := common.MultiLine(out.String())
	want := common.MultiLine(strings.TrimPrefix(wantOut, "\n"))
	if diff := cmp.Diff(want, got, common.MultiLineTransformer); diff != "" {
		t.Fatalf("Unexpected diff:\n\ngot =\n\n%v\n\nwant =\n\n%v\n\ndiff (-want, +got) =\n\n%v",
			got, want, common.MultiLine(diff))
	}
}

// The fetcher used for graph expansion must return no resources (not an
// error) for a resource type the server does not recognize, while still
// returning resources for types it does.
func TestFetchToleratesMissingResourceType(t *testing.T) {
	factory := NewTestFactoryWithoutResources(t, []string{"grpcroutes"}, testdataWithoutGRPCRoutes)
	fetcher := common.NewDefaultGroupKindFetcher(factory)

	grpcRoutes, err := fetcher.Fetch(common.GRPCRouteGK)
	if err != nil {
		t.Fatalf("fetching an unknown resource type should be tolerated, got: %v", err)
	}
	if len(grpcRoutes) != 0 {
		t.Fatalf("expected no GRPCRoutes, got %d", len(grpcRoutes))
	}

	httpRoutes, err := fetcher.Fetch(common.HTTPRouteGK)
	if err != nil {
		t.Fatalf("fetching a known resource type should succeed, got: %v", err)
	}
	if len(httpRoutes) != 1 {
		t.Fatalf("expected 1 HTTPRoute, got %d", len(httpRoutes))
	}
}
