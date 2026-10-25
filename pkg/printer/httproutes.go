/*
Copyright 2023 The Kubernetes Authors.

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
	"fmt"
	"io"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/duration"

	"sigs.k8s.io/gwctl/pkg/extension/directlyattachedpolicy"
	"sigs.k8s.io/gwctl/pkg/extension/gatewayeffectivepolicy"
	extensionutils "sigs.k8s.io/gwctl/pkg/extension/utils"
	"sigs.k8s.io/gwctl/pkg/policymanager"
	"sigs.k8s.io/gwctl/pkg/topology"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func (p *TablePrinter) printHTTPRoute(httpRouteNode *topology.Node, w io.Writer) error {
	httpRoute := topology.MustAccessObject(httpRouteNode, &gatewayv1.HTTPRoute{})
	return p.printRoute(httpRouteNode, w, httpRoute.Spec.Hostnames, httpRoute.Spec.CommonRouteSpec, httpRoute.Status.RouteStatus)
}

// printRoute prints the table row shared by all Route kinds. The row only
// needs the fields common to every Route, so the kind-specific printers pass
// those in rather than the typed object.
func (p *TablePrinter) printRoute(routeNode *topology.Node, w io.Writer, hostnames []gatewayv1.Hostname, spec gatewayv1.CommonRouteSpec, status gatewayv1.RouteStatus) error {
	if err := p.checkTypeChange(routeNode.GKNN().Kind, w); err != nil {
		return err
	}

	if p.table == nil {
		columnNames := namespacedBaseColumnNames(p.AllNamespaces)
		columnNames = append(columnNames, "HOSTNAMES", "PARENT REFS", "ACCEPTED", "RESOLVED", "AGE")
		if p.OutputFormat == OutputFormatWide {
			columnNames = append(columnNames, "POLICIES")
		}
		p.table = &Table{
			ColumnNames:  columnNames,
			UseSeparator: false,
		}
	}

	var hostNames []string
	for _, hostName := range hostnames {
		hostNames = append(hostNames, string(hostName))
	}
	hostNamesOutput := "None"
	if hostNamesCount := len(hostNames); hostNamesCount > 0 {
		if hostNamesCount > 2 {
			hostNamesOutput = fmt.Sprintf("%v + %v more", strings.Join(hostNames[:2], ","), hostNamesCount-2)
		} else {
			hostNamesOutput = strings.Join(hostNames, ",")
		}
	}

	parentRefsCount := fmt.Sprintf("%d", len(spec.ParentRefs))

	acceptedStatus, resolvedStatus := routeAcceptedAndResolvedStatus(status.Parents)

	age := "<unknown>"
	creationTimestamp := routeNode.Object.GetCreationTimestamp()
	if !creationTimestamp.IsZero() {
		age = duration.HumanDuration(p.Clock.Since(creationTimestamp.Time))
	}

	row := append(rowPrefixNamespaced(routeNode.Object, p.AllNamespaces), hostNamesOutput, parentRefsCount, acceptedStatus, resolvedStatus, age)
	if p.OutputFormat == OutputFormatWide {
		policiesMap, err := directlyattachedpolicy.Access(routeNode)
		if err != nil {
			return err
		}
		policiesCount := fmt.Sprintf("%d", len(policiesMap))
		row = append(row, policiesCount)
	}
	p.table.Rows = append(p.table.Rows, row)
	return nil
}

func (p *DescriptionPrinter) printHTTPRoute(httpRouteNode *topology.Node, w io.Writer) error {
	httpRoute := topology.MustAccessObject(httpRouteNode, &gatewayv1.HTTPRoute{})
	return p.printRoute(httpRouteNode, w, httpRoute.ObjectMeta, httpRoute.Spec, httpRoute.Status)
}

// printRoute describes a Route of any kind. The spec and status are printed
// as-is, so they are passed untyped.
func (p *DescriptionPrinter) printRoute(routeNode *topology.Node, w io.Writer, objectMeta metav1.ObjectMeta, spec, status any) error {
	if p.printSeparator {
		fmt.Fprintf(w, "\n\n")
	}
	p.printSeparator = true

	metadata := objectMeta.DeepCopy()
	metadata.Labels = nil
	metadata.Annotations = nil
	metadata.Name = ""
	metadata.Namespace = ""
	metadata.ManagedFields = nil

	pairs := []*DescriberKV{
		{"Name", objectMeta.Name},
		{"Namespace", objectMeta.Namespace},
		{"Label", objectMeta.Labels},
		{"Annotations", objectMeta.Annotations},
		{"APIVersion", routeNode.Object.GetAPIVersion()},
		{"Kind", routeNode.Object.GetKind()},
		{"Metadata", metadata},
		{"Spec", spec},
		{"Status", status},
	}

	// DirectlyAttachedPolicies
	policiesMap, err := directlyattachedpolicy.Access(routeNode)
	if err != nil {
		return err
	}
	policies := policymanager.ConvertPoliciesMapToSlice(policiesMap)
	pairs = append(pairs, &DescriberKV{Key: "DirectlyAttachedPolicies", Value: convertPoliciesToRefsTable(policies, false)})

	// InheritedPolicies
	effectivePolicies, err := gatewayeffectivepolicy.Access(routeNode)
	if err != nil {
		return err
	}
	policies = policymanager.ConvertPoliciesMapToSlice(effectivePolicies.RouteInheritedPolicies)
	pairs = append(pairs, &DescriberKV{Key: "InheritedPolicies", Value: convertPoliciesToRefsTable(policies, true)})

	// EffectivePolicies
	if len(effectivePolicies.RouteEffectivePolicies) != 0 {
		pairs = append(pairs, &DescriberKV{Key: "EffectivePolicies", Value: effectivePolicies.RouteEffectivePolicies})
	}

	// Analysis
	analysisErrors, err := extensionutils.AggregateAnalysisErrors(routeNode)
	if err != nil {
		return err
	}
	if len(analysisErrors) != 0 {
		pairs = append(pairs, &DescriberKV{Key: "Analysis", Value: convertErrorsToString(analysisErrors)})
	}

	// Events
	events, err := p.EventFetcher.FetchEventsFor(routeNode.Object)
	if err != nil {
		return err
	}
	pairs = append(pairs, &DescriberKV{Key: "Events", Value: convertEventsSliceToTable(events, p.Clock)})

	Describe(w, pairs)
	return nil
}
