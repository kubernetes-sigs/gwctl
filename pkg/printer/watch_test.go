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

package printer //nolint:revive

import (
	"bytes"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"sigs.k8s.io/gwctl/pkg/common"
	"sigs.k8s.io/gwctl/pkg/topology"
)

func TestTablePrinterFlushWatch(t *testing.T) {
	node := testData(t)[common.GatewayGK][0]
	secondNode := &topology.Node{Object: node.Object.DeepCopy()}
	secondNode.Object.SetName("g")

	p := &TablePrinter{}
	out := &bytes.Buffer{}
	if err := p.PrintNode(node, out); err != nil {
		t.Fatalf("PrintNode() error = %v", err)
	}
	if err := p.FlushWatch(out); err != nil {
		t.Fatalf("FlushWatch() error = %v", err)
	}
	if err := p.PrintNode(secondNode, out); err != nil {
		t.Fatalf("PrintNode() error = %v", err)
	}
	if err := p.FlushWatch(out); err != nil {
		t.Fatalf("FlushWatch() error = %v", err)
	}

	// The header is written once, with the initial list. A later, shorter name
	// must still use the initial list's column width.
	wantOut := `
NAME       CLASS            ADDRESSES                   PORTS  PROGRAMMED  AGE
gateway-1  gateway-class-1  10.0.0.1,10.0.0.2 + 1 more  80     True        <unknown>
g          gateway-class-1  10.0.0.1,10.0.0.2 + 1 more  80     True        <unknown>
`

	got := common.MultiLine(out.String())
	want := common.MultiLine(strings.TrimPrefix(wantOut, "\n"))

	if diff := cmp.Diff(want, got, common.MultiLineTransformer); diff != "" {
		t.Fatalf("Unexpected diff:\n\ngot =\n\n%v\n\nwant =\n\n%v\n\ndiff (-want, +got) =\n\n%v",
			got, want, common.MultiLine(diff))
	}
}
