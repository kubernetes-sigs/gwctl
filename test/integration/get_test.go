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

package integration

import (
	_ "embed"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	cmdget "sigs.k8s.io/gwctl/cmd/get"
	"sigs.k8s.io/gwctl/pkg/common"
)

//go:embed testdata/sample1.yaml
var testdataSample1 string

func TestGetWatch(t *testing.T) {
	factory := NewTestFactory(t, testdataSample1)
	factory.namespace = "default"
	factory.setWatchEvents(t,
		watch.Event{Type: watch.Added, Object: watchGateway("gateway-added")},
		watch.Event{Type: watch.Modified, Object: watchGateway("gateway-modified")},
		watch.Event{Type: watch.Deleted, Object: watchGateway("gateway-deleted")},
	)

	iostreams, _, out, errOut := genericiooptions.NewTestIOStreams()
	cmd := cmdget.NewCmd(factory, iostreams, false)
	cmd.SetArgs([]string{"gateways", "--watch"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	if count := strings.Count(got, "NAME"); count != 1 {
		t.Fatalf("header count = %d, want 1\noutput:\n%s", count, got)
	}
	for _, name := range []string{"gateway-3", "gateway-added", "gateway-modified", "gateway-deleted"} {
		if !strings.Contains(got, name) {
			t.Fatalf("output does not contain %q:\n%s", name, got)
		}
	}
	if !strings.Contains(errOut.String(), "server closed the watch stream") {
		t.Fatalf("stderr does not report the server closing the watch:\n%s", errOut.String())
	}
}

func TestGetWatchNamedResource(t *testing.T) {
	factory := NewTestFactory(t, testdataSample1)
	factory.namespace = "default"
	factory.setWatchEvents(t,
		watch.Event{Type: watch.Added, Object: watchGateway("gateway-3")},
		watch.Event{Type: watch.Modified, Object: watchGateway("gateway-modified")},
	)

	iostreams, _, out, _ := genericiooptions.NewTestIOStreams()
	cmd := cmdget.NewCmd(factory, iostreams, false)
	cmd.SetArgs([]string{"gateways", "gateway-3", "--watch"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	if count := strings.Count(got, "gateway-3"); count != 1 {
		t.Fatalf("gateway-3 row count = %d, want 1\noutput:\n%s", count, got)
	}
	if !strings.Contains(got, "gateway-modified") {
		t.Fatalf("output does not contain the modified resource:\n%s", got)
	}
}

// TestGetWatchNamedResourceFirstEventNotAdded ensures the synthetic-ADDED
// suppression on a named-resource watch is disarmed by the first event of any
// type, not only by an ADDED event. Otherwise a genuine ADDED arriving after
// a first MODIFIED/DELETED would be silently dropped.
func TestGetWatchNamedResourceFirstEventNotAdded(t *testing.T) {
	factory := NewTestFactory(t, testdataSample1)
	factory.namespace = "default"
	factory.setWatchEvents(t,
		watch.Event{Type: watch.Modified, Object: watchGateway("gateway-modified")},
		watch.Event{Type: watch.Added, Object: watchGateway("gateway-added")},
	)

	iostreams, _, out, _ := genericiooptions.NewTestIOStreams()
	cmd := cmdget.NewCmd(factory, iostreams, false)
	cmd.SetArgs([]string{"gateways", "gateway-3", "--watch"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	for _, name := range []string{"gateway-modified", "gateway-added"} {
		if !strings.Contains(got, name) {
			t.Fatalf("output does not contain %q:\n%s", name, got)
		}
	}
}

func TestGetWatchNamedResourceInitialEventsEndBookmark(t *testing.T) {
	factory := NewTestFactory(t, testdataSample1)
	factory.namespace = "default"
	bookmark := watchGateway("gateway-3")
	bookmark.SetAnnotations(map[string]string{metav1.InitialEventsAnnotationKey: "true"})
	factory.setWatchEvents(t,
		watch.Event{Type: watch.Bookmark, Object: bookmark},
		watch.Event{Type: watch.Added, Object: watchGateway("gateway-added")},
	)

	iostreams, _, out, _ := genericiooptions.NewTestIOStreams()
	cmd := cmdget.NewCmd(factory, iostreams, false)
	cmd.SetArgs([]string{"gateways", "gateway-3", "--watch"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got := out.String(); !strings.Contains(got, "gateway-added") {
		t.Fatalf("output does not contain the added resource:\n%s", got)
	}
}

func TestGetWatchWide(t *testing.T) {
	factory := NewTestFactory(t, testdataSample1)
	factory.namespace = "default"
	factory.setWatchEvents(t, watch.Event{Type: watch.Added, Object: watchGateway("gateway-added")})

	iostreams, _, out, _ := genericiooptions.NewTestIOStreams()
	cmd := cmdget.NewCmd(factory, iostreams, false)
	cmd.SetArgs([]string{"gateways", "--watch", "-o", "wide"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "POLICIES") || !strings.Contains(got, "HTTPROUTES") {
		t.Fatalf("wide output does not contain its additional columns:\n%s", got)
	}
	if !strings.Contains(got, "gateway-added") {
		t.Fatalf("output does not contain the added resource:\n%s", got)
	}
}

func watchGateway(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "Gateway",
		"metadata": map[string]any{
			"name":      name,
			"namespace": "default",
		},
		"spec": map[string]any{
			"gatewayClassName": "foo-com-external-gateway-class",
			"listeners": []any{
				map[string]any{
					"name":     "http",
					"port":     80,
					"protocol": "HTTP",
				},
			},
		},
	}}
}

func TestGet(t *testing.T) {
	factory := NewTestFactory(t, testdataSample1)

	testCases := []struct {
		name       string
		inputArgs  []string
		namespace  string // Controls the '-n' flag. Empty value means all-namespaces (-A)
		describe   bool
		wantOut    string
		wantErrOut string
	}{
		{
			name:      "get gateways -n test",
			inputArgs: []string{"gateways"},
			namespace: "test",
			wantOut: `
NAME       CLASS                           ADDRESSES  PORTS  PROGRAMMED  AGE
gateway-1  foo-com-external-gateway-class             80     Unknown     <unknown>
gateway-2  bar-com-internal-gateway-class             443    Unknown     <unknown>
`,
		},
		{
			name:      "get gateways",
			inputArgs: []string{"gateways"},
			namespace: "default",
			wantOut: `
NAME       CLASS                           ADDRESSES  PORTS  PROGRAMMED  AGE
gateway-3  foo-com-external-gateway-class             80     Unknown     <unknown>
`,
		},
		{
			name:      "get gateways -A",
			inputArgs: []string{"gateways", "-A"},
			namespace: "", // All namespaces
			wantOut: `
NAMESPACE  NAME       CLASS                           ADDRESSES  PORTS  PROGRAMMED  AGE
default    gateway-3  foo-com-external-gateway-class             80     Unknown     <unknown>
test       gateway-1  foo-com-external-gateway-class             80     Unknown     <unknown>
test       gateway-2  bar-com-internal-gateway-class             443    Unknown     <unknown>
`,
		},
		{
			name:      "get gateways --all-namespaces",
			inputArgs: []string{"gateways", "--all-namespaces"},
			namespace: "", // All namespaces
			wantOut: `
NAMESPACE  NAME       CLASS                           ADDRESSES  PORTS  PROGRAMMED  AGE
default    gateway-3  foo-com-external-gateway-class             80     Unknown     <unknown>
test       gateway-1  foo-com-external-gateway-class             80     Unknown     <unknown>
test       gateway-2  bar-com-internal-gateway-class             443    Unknown     <unknown>
`,
		},
		{
			name:      "get gatewayclasses",
			inputArgs: []string{"gatewayclasses"},
			wantOut: `
NAME                            CONTROLLER                      ACCEPTED  AGE
bar-com-internal-gateway-class  bar.baz/internal-gateway-class  Unknown   <unknown>
foo-com-external-gateway-class  foo.com/external-gateway-class  Unknown   <unknown>
`,
		},
		{
			name:      "get httproutes",
			inputArgs: []string{"httproutes"},
			namespace: "default",
			wantOut: `
NAME         HOSTNAMES     PARENT REFS  ACCEPTED  RESOLVED  AGE
httproute-3  example4.com  1            Unknown   Unknown   <unknown>
`,
		},
		{
			name:      "get httproutes -A",
			inputArgs: []string{"httproutes", "-A"},
			namespace: "", // All namespaces
			wantOut: `
NAMESPACE  NAME         HOSTNAMES                          PARENT REFS  ACCEPTED  RESOLVED  AGE
default    httproute-3  example4.com                       1            Unknown   Unknown   <unknown>
test       httproute-1  demo.com                           1            Unknown   Unknown   <unknown>
test       httproute-2  example.com,example2.com + 1 more  2            Unknown   Unknown   <unknown>
`,
		},
		{
			name:      "get httproutes --all-namespaces",
			inputArgs: []string{"httproutes", "--all-namespaces"},
			namespace: "", // All namespaces
			wantOut: `
NAMESPACE  NAME         HOSTNAMES                          PARENT REFS  ACCEPTED  RESOLVED  AGE
default    httproute-3  example4.com                       1            Unknown   Unknown   <unknown>
test       httproute-1  demo.com                           1            Unknown   Unknown   <unknown>
test       httproute-2  example.com,example2.com + 1 more  2            Unknown   Unknown   <unknown>
`,
		},
		{
			name:      "get services",
			inputArgs: []string{"services"},
			namespace: "default",
			wantOut: `
NAME   TYPE     AGE
svc-3  Service  <unknown>
`,
		},
		{
			name:      "get policies",
			inputArgs: []string{"policies"},
			namespace: "test",
			wantOut: `
NAME      KIND                                        TARGET(S)                               POLICY TYPE  ACCEPTED  AGE
policy-1  BackendTLSPolicy.gateway.networking.k8s.io  Service/test/svc-1, Service/test/svc-2  Direct       True      <unknown>
`,
		},
		{
			name:      "get policies -A",
			inputArgs: []string{"policies", "-A"},
			namespace: "", // All namespaces
			wantOut: `
NAMESPACE  NAME      KIND                                        TARGET(S)                               POLICY TYPE  ACCEPTED  AGE
default    policy-2  BackendTLSPolicy.gateway.networking.k8s.io  Service/default/svc-3                   Direct       Partial   <unknown>
test       policy-1  BackendTLSPolicy.gateway.networking.k8s.io  Service/test/svc-1, Service/test/svc-2  Direct       True      <unknown>
`,
		},
		{
			name:      "get policies --all-namespaces",
			inputArgs: []string{"policies", "--all-namespaces"},
			namespace: "", // All namespaces
			wantOut: `
NAMESPACE  NAME      KIND                                        TARGET(S)                               POLICY TYPE  ACCEPTED  AGE
default    policy-2  BackendTLSPolicy.gateway.networking.k8s.io  Service/default/svc-3                   Direct       Partial   <unknown>
test       policy-1  BackendTLSPolicy.gateway.networking.k8s.io  Service/test/svc-1, Service/test/svc-2  Direct       True      <unknown>
`,
		},
		{
			name:      "describe gateways -n test",
			inputArgs: []string{"gateways"},
			namespace: "test",
			describe:  true,
			wantOut: `
Name: gateway-1
Namespace: test
Labels: null
Annotations: null
APIVersion: gateway.networking.k8s.io/v1
Kind: Gateway
Metadata:
  uid: uid-for-test-gateway-1
Spec:
  gatewayClassName: foo-com-external-gateway-class
  listeners:
  - name: http
    port: 80
    protocol: HTTP
Status: {}
AttachedRoutes:
  Kind       Name
  ----       ----
  HTTPRoute  test/httproute-1
  HTTPRoute  test/httproute-2
Backends:
  Kind     Name
  ----     ----
  Service  test/svc-1
  Service  test/svc-2
DirectlyAttachedPolicies: <none>
InheritedPolicies: <none>
Events:
  Type     Reason  Age      From                   Message
  ----     ------  ---      ----                   -------
  Warning  SYNC    Unknown  my-gateway-controller  test message


Name: gateway-2
Namespace: test
Labels: null
Annotations: null
APIVersion: gateway.networking.k8s.io/v1
Kind: Gateway
Metadata: {}
Spec:
  gatewayClassName: bar-com-internal-gateway-class
  listeners:
  - name: https
    port: 443
    protocol: HTTPS
Status: {}
AttachedRoutes:
  Kind       Name
  ----       ----
  HTTPRoute  test/httproute-2
Backends:
  Kind     Name
  ----     ----
  Service  test/svc-2
DirectlyAttachedPolicies: <none>
InheritedPolicies: <none>
Events: <none>
`,
		},
		{
			name:      "describe gateways gateway-1 -n test",
			inputArgs: []string{"gateways", "gateway-1"},
			namespace: "test",
			describe:  true,
			wantOut: `
Name: gateway-1
Namespace: test
Labels: null
Annotations: null
APIVersion: gateway.networking.k8s.io/v1
Kind: Gateway
Metadata:
  uid: uid-for-test-gateway-1
Spec:
  gatewayClassName: foo-com-external-gateway-class
  listeners:
  - name: http
    port: 80
    protocol: HTTP
Status: {}
AttachedRoutes:
  Kind       Name
  ----       ----
  HTTPRoute  test/httproute-1
  HTTPRoute  test/httproute-2
Backends:
  Kind     Name
  ----     ----
  Service  test/svc-1
  Service  test/svc-2
DirectlyAttachedPolicies: <none>
InheritedPolicies: <none>
Events:
  Type     Reason  Age      From                   Message
  ----     ------  ---      ----                   -------
  Warning  SYNC    Unknown  my-gateway-controller  test message
`,
		},
		{
			name:      "get gateways -o json -n default",
			inputArgs: []string{"gateways", "-o", "json"},
			namespace: "default",
			wantOut: `
{
    "apiVersion": "gateway.networking.k8s.io/v1",
    "kind": "Gateway",
    "metadata": {
        "name": "gateway-3",
        "namespace": "default"
    },
    "spec": {
        "gatewayClassName": "foo-com-external-gateway-class",
        "listeners": [
            {
                "name": "http",
                "port": 80,
                "protocol": "HTTP"
            }
        ]
    }
}
`,
		},
		{
			name:      "get gateways -o yaml -n default",
			inputArgs: []string{"gateways", "-o", "yaml"},
			namespace: "default",
			wantOut: `
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: gateway-3
  namespace: default
spec:
  gatewayClassName: foo-com-external-gateway-class
  listeners:
  - name: http
    port: 80
    protocol: HTTP
`,
		},
		{
			name:      "get gateways -o wide -n default",
			inputArgs: []string{"gateways", "-o", "wide"},
			namespace: "default",
			wantOut: `
NAME       CLASS                           ADDRESSES  PORTS  PROGRAMMED  AGE        POLICIES  HTTPROUTES
gateway-3  foo-com-external-gateway-class             80     Unknown     <unknown>  0         1
`,
		},
		{
			name:      "get gateways,httproutes -n test",
			inputArgs: []string{"gateways,httproutes"},
			namespace: "test",
			wantOut: `
NAME       CLASS                           ADDRESSES  PORTS  PROGRAMMED  AGE
gateway-1  foo-com-external-gateway-class             80     Unknown     <unknown>
gateway-2  bar-com-internal-gateway-class             443    Unknown     <unknown>

NAME         HOSTNAMES                          PARENT REFS  ACCEPTED  RESOLVED  AGE
httproute-1  demo.com                           1            Unknown   Unknown   <unknown>
httproute-2  example.com,example2.com + 1 more  2            Unknown   Unknown   <unknown>
`,
		},
		{
			name:      "get gateways,services",
			inputArgs: []string{"gateways,services"},
			namespace: "default",
			wantOut: `
NAME       CLASS                           ADDRESSES  PORTS  PROGRAMMED  AGE
gateway-3  foo-com-external-gateway-class             80     Unknown     <unknown>

NAME   TYPE     AGE
svc-3  Service  <unknown>
`,
		},
		{
			name:      "get httproutes,policies -n test",
			inputArgs: []string{"httproutes,policies"},
			namespace: "test",
			wantOut: `
NAME      KIND                                        TARGET(S)                               POLICY TYPE  ACCEPTED  AGE
policy-1  BackendTLSPolicy.gateway.networking.k8s.io  Service/test/svc-1, Service/test/svc-2  Direct       True      <unknown>

NAME         HOSTNAMES                          PARENT REFS  ACCEPTED  RESOLVED  AGE
httproute-1  demo.com                           1            Unknown   Unknown   <unknown>
httproute-2  example.com,example2.com + 1 more  2            Unknown   Unknown   <unknown>
`,
		},
		{
			name:      "get policies,policycrds -A",
			inputArgs: []string{"policies,policycrds", "-A"},
			namespace: "",
			wantOut: `
NAMESPACE  NAME      KIND                                        TARGET(S)                               POLICY TYPE  ACCEPTED  AGE
default    policy-2  BackendTLSPolicy.gateway.networking.k8s.io  Service/default/svc-3                   Direct       Partial   <unknown>
test       policy-1  BackendTLSPolicy.gateway.networking.k8s.io  Service/test/svc-1, Service/test/svc-2  Direct       True      <unknown>

NAME                                          POLICY TYPE  SCOPE       AGE
backendtlspolicies.gateway.networking.k8s.io  Direct       Namespaced  <unknown>
`,
		},
		{
			name:      "get gateways,gatewayclasses -A",
			inputArgs: []string{"gateways,gatewayclasses", "-A"},
			namespace: "",
			wantOut: `
NAMESPACE  NAME       CLASS                           ADDRESSES  PORTS  PROGRAMMED  AGE
default    gateway-3  foo-com-external-gateway-class             80     Unknown     <unknown>
test       gateway-1  foo-com-external-gateway-class             80     Unknown     <unknown>
test       gateway-2  bar-com-internal-gateway-class             443    Unknown     <unknown>

NAME                            CONTROLLER                      ACCEPTED  AGE
bar-com-internal-gateway-class  bar.baz/internal-gateway-class  Unknown   <unknown>
foo-com-external-gateway-class  foo.com/external-gateway-class  Unknown   <unknown>
`,
		},
		{
			name:      "get policies,gateways -n test",
			inputArgs: []string{"policies,gateways"},
			namespace: "test",
			wantOut: `
NAME      KIND                                        TARGET(S)                               POLICY TYPE  ACCEPTED  AGE
policy-1  BackendTLSPolicy.gateway.networking.k8s.io  Service/test/svc-1, Service/test/svc-2  Direct       True      <unknown>

NAME       CLASS                           ADDRESSES  PORTS  PROGRAMMED  AGE
gateway-1  foo-com-external-gateway-class             80     Unknown     <unknown>
gateway-2  bar-com-internal-gateway-class             443    Unknown     <unknown>
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			factory.namespace = tc.namespace

			iostreams, _, out, errOut := genericiooptions.NewTestIOStreams()
			cmd := cmdget.NewCmd(factory, iostreams, tc.describe)
			cmd.SetOut(out)
			cmd.SetErr(out)
			cmd.SetArgs(tc.inputArgs)

			err := cmd.Execute()
			if err != nil {
				t.Logf("Failed to execute command: %v", err)
				t.Logf("Debug: out=\n%v\n", out.String())
				t.Logf("Debug: errOut=\n%v\n", errOut.String())
				t.FailNow()
			}

			got := common.MultiLine(out.String())
			want := common.MultiLine(strings.TrimPrefix(tc.wantOut, "\n"))

			if diff := cmp.Diff(want, got, common.MultiLineTransformer); diff != "" {
				t.Fatalf("Unexpected diff:\n\ngot =\n\n%v\n\nwant =\n\n%v\n\ndiff (-want, +got) =\n\n%v", got, want, common.MultiLine(diff))
			}
		})
	}
}
