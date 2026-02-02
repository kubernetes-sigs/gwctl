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

//go:embed testdata/graphviz/graph-single-namespace.gv
var testdataGraphSingleNamespaceDot string

//go:embed testdata/graphviz/graph-multi-namespace.yaml
var testdataGraphMultiNamespace string

//go:embed testdata/graphviz/graph-multi-namespace.gv
var testdataGraphMultiNamespaceDot string

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
			name:      "get gateways -o graph -n default",
			inputArgs: []string{"gateways", "-o", "graph"},
			namespace: "default",
			describe:  false,
			yaml:      testdataGraphSingleNamespace,
			wantOut:   testdataGraphSingleNamespaceDot,
		},
		{
			name:      "get gateways -o graph -A",
			inputArgs: []string{"gateways", "-o", "graph"},
			namespace: "", // All namespaces
			describe:  false,
			yaml:      testdataGraphMultiNamespace,
			wantOut:   testdataGraphMultiNamespaceDot,
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
