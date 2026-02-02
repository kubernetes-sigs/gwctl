package integration

import (
	_ "embed"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	cmdget "sigs.k8s.io/gwctl/cmd/get"
	"sigs.k8s.io/gwctl/pkg/common"
)

//go:embed testdata/graphviz/graph-single-namespace.yaml
var testdataGraphSingleNamespace string

//go:embed testdata/graphviz/graph-multi-namespace.yaml
var testdataGraphMultiNamespace string

func TestGraphviz(t *testing.T) {

	testCases := []struct {
		name      string
		inputArgs []string
		namespace string // Controls the '-n' flag. Empty value means all-namespaces (-A)
		describe  bool
		yaml      string
		wantOut   string
	}{
		{
			name:      "graph single namespace",
			inputArgs: []string{"gateways", "-o", "graph"},
			namespace: "default",
			describe:  false,
			yaml:      testdataGraphSingleNamespace,
			wantOut: `
digraph  {
	subgraph cluster_s1 {
		color="black";label="Namespace: default";style="dashed";
		n3[color="#ebcb8b",label="Gateway\ndemo-gateway",style="filled"];
		n5[color="#a3be8c",label="HTTPRoute\ndemo-httproute",style="filled"];
		n2[color="#88c0d0",label="Service\ndemo-svc",style="filled"];
		
	}
	compound="true";rankdir="BT";
	n4[color="#e5e9f0",label="GatewayClass\ndemo-gateway-class",style="filled"];
	n3->n4[label="GatewayClass"];
	n5->n3[label="ParentRef"];
	n2->n5[dir="back",label="BackendRef"];
	
}

`,
		},
	}

	for _, tc := range testCases {

		t.Run(tc.name, func(t *testing.T) {

			factory := NewTestFactory(t, tc.yaml)

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
