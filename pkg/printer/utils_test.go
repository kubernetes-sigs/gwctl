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

package printer //nolint:revive

import (
	"bytes"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/gwctl/pkg/common"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestDescribe(t *testing.T) {
	pairs := []*DescriberKV{
		{Key: "Key1", Value: "string"},
		{Key: "Key2", Value: []any{"list-string1", 1234, "list-string-3"}},
		{
			Key: "Key3-with-nested-structures",
			Value: map[string]any{
				"a": "b",
				"d": map[string]any{
					"e": []string{"v1", "v2", "v3"},
				},
				"c": 123,
			},
		},
		{
			Key: "Key4-table",
			Value: &Table{
				ColumnNames: []string{"col1", "col2", "col3"},
				Rows: [][]string{
					{"row1-a", "row1-b", "row1-c"},
					{"row2-a", "row2-b", "row2-c"},
					{"row3-a", "row3-b", "row3-c"},
				},
				UseSeparator: true,
			},
		},
	}

	writable := &bytes.Buffer{}
	Describe(writable, pairs)

	got := writable.String()
	want := `Key1: string
Key2:
- list-string1
- 1234
- list-string-3
Key3-with-nested-structures:
  a: b
  c: 123
  d:
    e:
    - v1
    - v2
    - v3
Key4-table:
  col1    col2    col3
  ----    ----    ----
  row1-a  row1-b  row1-c
  row2-a  row2-b  row2-c
  row3-a  row3-b  row3-c
`
	if diff := cmp.Diff(common.MultiLine(want), common.MultiLine(got), common.MultiLineTransformer); diff != "" {
		t.Fatalf("Unexpected diff:\n\ngot =\n\n%v\n\nwant =\n\n%v\n\ndiff (-want, +got) =\n\n%v", common.MultiLine(got), common.MultiLine(want), common.MultiLine(diff))
	}
}

func TestTable_writeTable(t *testing.T) {
	testcases := []struct {
		name   string
		table  *Table
		indent int
		want   string
	}{
		{
			name: "without separator",
			table: &Table{
				ColumnNames: []string{"Kind", "Name"},
				Rows: [][]string{
					{"HTTPRoute", "default/my-httproute"},
					{"TCPRoute", "ns2/my-tcproute"},
				},
			},
			indent: 0,
			want: `
Kind       Name
HTTPRoute  default/my-httproute
TCPRoute   ns2/my-tcproute
`,
		},
		{
			name: "with separator",
			table: &Table{
				ColumnNames: []string{"Kind", "Name"},
				Rows: [][]string{
					{"HTTPRoute", "default/my-httproute"},
					{"TCPRoute", "ns2/my-tcproute"},
				},
				UseSeparator: true,
			},
			indent: 0,
			want: `
Kind       Name
----       ----
HTTPRoute  default/my-httproute
TCPRoute   ns2/my-tcproute
`,
		},
		{
			name: "with indent and separator",
			table: &Table{
				ColumnNames: []string{"Kind", "Name"},
				Rows: [][]string{
					{"HTTPRoute", "default/my-httproute"},
					{"TCPRoute", "ns2/my-tcproute"},
				},
				UseSeparator: true,
			},
			indent: 3, // We want 3 spaces at the start of each row.
			want: `
   Kind       Name
   ----       ----
   HTTPRoute  default/my-httproute
   TCPRoute   ns2/my-tcproute
`,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			writable := &bytes.Buffer{}
			tc.table.Write(writable, tc.indent)

			got := common.MultiLine(writable.String())
			want := common.MultiLine(strings.TrimPrefix(tc.want, "\n"))
			if diff := cmp.Diff(want, got, common.MultiLineTransformer); diff != "" {
				t.Fatalf("Unexpected diff:\n\ngot =\n\n%v\n\nwant =\n\n%v\n\ndiff (-want, +got) =\n\n%v", got, want, common.MultiLine(diff))
			}
		})
	}
}

func TestRouteAcceptedAndResolvedStatus(t *testing.T) {
	parent := func(accepted, resolved metav1.ConditionStatus) gatewayv1.RouteParentStatus {
		return gatewayv1.RouteParentStatus{Conditions: []metav1.Condition{
			{Type: "Accepted", Status: accepted},
			{Type: "ResolvedRefs", Status: resolved},
		}}
	}

	testCases := []struct {
		name         string
		parents      []gatewayv1.RouteParentStatus
		wantAccepted string
		wantResolved string
	}{
		{
			name:         "no parents",
			wantAccepted: "Unknown",
			wantResolved: "Unknown",
		},
		{
			name:         "accepted but unresolved",
			parents:      []gatewayv1.RouteParentStatus{parent(metav1.ConditionTrue, metav1.ConditionFalse)},
			wantAccepted: "True",
			wantResolved: "False",
		},
		{
			name: "partially accepted, resolved everywhere",
			parents: []gatewayv1.RouteParentStatus{
				parent(metav1.ConditionTrue, metav1.ConditionTrue),
				parent(metav1.ConditionFalse, metav1.ConditionTrue),
			},
			wantAccepted: "Partial",
			wantResolved: "True",
		},
		{
			name:         "conditions missing",
			parents:      []gatewayv1.RouteParentStatus{{}},
			wantAccepted: "False",
			wantResolved: "False",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotAccepted, gotResolved := routeAcceptedAndResolvedStatus(tc.parents)
			if gotAccepted != tc.wantAccepted || gotResolved != tc.wantResolved {
				t.Errorf("got (%q, %q), want (%q, %q)", gotAccepted, gotResolved, tc.wantAccepted, tc.wantResolved)
			}
		})
	}
}
