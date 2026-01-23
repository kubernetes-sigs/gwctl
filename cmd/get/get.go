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

package get

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/utils/clock"

	"sigs.k8s.io/gwctl/pkg/common"
	"sigs.k8s.io/gwctl/pkg/extension"
	"sigs.k8s.io/gwctl/pkg/extension/directlyattachedpolicy"
	"sigs.k8s.io/gwctl/pkg/extension/gatewayeffectivepolicy"
	"sigs.k8s.io/gwctl/pkg/extension/notfoundrefvalidator"
	"sigs.k8s.io/gwctl/pkg/extension/refgrantvalidator"
	gwctlflags "sigs.k8s.io/gwctl/pkg/flags"
	"sigs.k8s.io/gwctl/pkg/policymanager"
	"sigs.k8s.io/gwctl/pkg/printer"
	"sigs.k8s.io/gwctl/pkg/topology"
	topologygw "sigs.k8s.io/gwctl/pkg/topology/gateway"
)

func NewCmd(factory common.Factory, iostreams genericiooptions.IOStreams, isDescribe bool) *cobra.Command {
	flags := newGetFlags()

	cmdName := "get"
	if isDescribe {
		cmdName = "describe"
	}

	cmd := &cobra.Command{
		Use:   fmt.Sprintf("%v TYPE [RESOURCE_NAME]", cmdName),
		Short: "Display one or many resources",
		Args:  cobra.RangeArgs(1, 2),
		Run: func(_ *cobra.Command, args []string) {
			o, err := flags.ToOptions(args, factory, iostreams, isDescribe)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v", err)
				os.Exit(1)
			}

			err = o.Run(args)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v", err)
				os.Exit(1)
			}
		},
	}

	flags.resourceBuilderFlags.AddFlags(cmd.Flags())

	if !isDescribe {
		printableAllowedFormats := strings.Join(printer.AllowedOutputFormatsForHelp(), ",")
		cmd.Flags().StringVarP(&flags.outputFormat, "output", "o", "", fmt.Sprintf("Output format. Must be one of: %v", printableAllowedFormats))

		flags.forFlag.AddFlag(cmd.Flags())
	}

	return cmd
}

// getFlags contains the flags used with get command.
type getFlags struct {
	resourceBuilderFlags *genericclioptions.ResourceBuilderFlags
	outputFormat         string
	forFlag              gwctlflags.ForFlag
}

func newGetFlags() *getFlags {
	resourceBuilderFlags := genericclioptions.NewResourceBuilderFlags().
		WithAllNamespaces(false).
		WithLabelSelector("")
	resourceBuilderFlags.FileNameFlags = nil

	return &getFlags{
		resourceBuilderFlags: resourceBuilderFlags,
	}
}

func (f *getFlags) ToOptions(args []string, factory common.Factory, iostreams genericiooptions.IOStreams, isDescribe bool) (*getOptions, error) {
	o := &getOptions{
		isDescribe:    isDescribe,
		factory:       factory,
		IOStreams:     iostreams,
		allNamespaces: *f.resourceBuilderFlags.AllNamespaces,
		labelSelector: *f.resourceBuilderFlags.LabelSelector,
	}

	var err error
	o.resourceTypes, o.resourceNames, err = parseResourceTypeOrNameArgs(args)
	if err != nil {
		return nil, err
	}

	o.namespace, _, err = factory.KubeConfigNamespace()
	if err != nil {
		return nil, err
	}

	// Parse outputFormat
	o.output, err = printer.ValidateAndReturnOutputFormat(f.outputFormat)
	if err != nil {
		return nil, err
	}

	return o, nil
}

type getOptions struct {
	isDescribe bool

	factory common.Factory

	allNamespaces bool
	namespace     string
	labelSelector string
	output        printer.OutputFormat

	// resourceTypes holds the requested resource types (e.g., ["gateway", "httproute"])
	resourceTypes []string
	// resourceNames holds the specific resource names when querying single type with name
	resourceNames []string

	genericclioptions.IOStreams
}

func (o *getOptions) Run(args []string) error {
	// Collect all nodes from all requested resource types
	allNodes := []*topology.Node{}

	// Check if we're querying policies
	isPolicy := false
	isPolicyCRD := false

	for _, resourceType := range o.resourceTypes {
		if resourceType == "policy" || resourceType == "policies" {
			isPolicy = true
		} else if resourceType == "policycrd" || resourceType == "policycrds" {
			isPolicyCRD = true
		}
	}

	// Handle policy/policycrd resources separately
	if isPolicy || isPolicyCRD {
		nodes, err := o.handlePolicy(args)
		if err != nil {
			return err
		}
		allNodes = append(allNodes, nodes...)
	}

	// Handle other resources (gateway, httproute, etc)
	nonPolicyTypes := []string{}
	for _, rt := range o.resourceTypes {
		if rt != "policy" && rt != "policies" && rt != "policycrd" && rt != "policycrds" {
			nonPolicyTypes = append(nonPolicyTypes, rt)
		}
	}

	if len(nonPolicyTypes) > 0 {
		// Build a resource builder for non-policy types
		b := o.factory.NewBuilder().
			Unstructured().
			Flatten().
			NamespaceParam(o.namespace).DefaultNamespace().AllNamespaces(o.allNamespaces).
			LabelSelectorParam(o.labelSelector).
			ContinueOnError()

		// Add resource types to the builder
		if len(o.resourceNames) > 0 {
			// If resource names are provided, pass them with the type
			for _, rt := range nonPolicyTypes {
				b = b.ResourceTypeOrNameArgs(true, rt, o.resourceNames[0])
			}
		} else {
			// Otherwise just pass the resource types
			for _, rt := range nonPolicyTypes {
				b = b.ResourceTypeOrNameArgs(true, rt)
			}
		}

		infos, err := b.Do().Infos()
		if err != nil {
			return err
		}

		sources := []*unstructured.Unstructured{}
		for _, info := range infos {
			o, err := runtime.DefaultUnstructuredConverter.ToUnstructured(info.Object) //nolint:govet
			if err != nil {
				return err
			}
			u := &unstructured.Unstructured{Object: o}
			sources = append(sources, u)
		}

		var graph *topology.Graph
		if o.isDescribe || o.output == printer.OutputFormatWide || o.output == printer.OutputFormatGraph {
			graph, err = topology.NewBuilder(common.NewDefaultGroupKindFetcher(o.factory)).
				StartFrom(sources).
				UseRelationships(topologygw.AllRelations).
				Build()
			if err != nil {
				return err
			}

			policyManager := policymanager.New(common.NewDefaultGroupKindFetcher(o.factory))
			if err := policyManager.Init(); err != nil { //nolint:govet
				return err
			}

			err := extension.ExecuteAll(graph, //nolint:govet
				directlyattachedpolicy.NewExtension(policyManager),
				gatewayeffectivepolicy.NewExtension(),
				refgrantvalidator.NewExtension(refgrantvalidator.NewDefaultReferenceGrantFetcher(o.factory)),
				notfoundrefvalidator.NewExtension(),
			)
			if err != nil {
				return err
			}
		} else {
			graph, err = topology.NewBuilder(common.NewDefaultGroupKindFetcher(o.factory)).
				StartFrom(sources).
				Build()
			if err != nil {
				return err
			}
		}

		if o.output == printer.OutputFormatGraph {
			toDotGraph, err := topologygw.ToDot(graph)
			if err != nil {
				return err
			}
			fmt.Fprintf(o.IOStreams.Out, "%v\n", toDotGraph)
			return nil
		}

		allNodes = append(allNodes, graph.Sources...)
	}

	if o.output == printer.OutputFormatGraph && len(allNodes) > 0 {
		// Graph output was already handled above for non-policy types
		return nil
	}

	// Print all collected nodes, including policies and non-policy resources
	return o.printNodes(allNodes)
}

func (o *getOptions) handlePolicy(args []string) ([]*topology.Node, error) {
	policyManager := policymanager.New(common.NewDefaultGroupKindFetcher(o.factory))
	if err := policyManager.Init(); err != nil {
		return nil, err
	}

	nodes := []*topology.Node{}

	// Process resource types in the order they appear in the command line
	for _, resourceType := range o.resourceTypes {
		if resourceType == "policy" || resourceType == "policies" {
			for _, policy := range policyManager.GetPolicies() {
				shouldSkip := (!o.allNamespaces && o.namespace != policy.GKNN().Namespace) ||
					(len(o.resourceNames) > 0 && o.resourceNames[0] != policy.GKNN().Name)
				if shouldSkip {
					continue
				}
				nodes = append(nodes, encodePolicyAsNode(policy))
			}
		} else if resourceType == "policycrd" || resourceType == "policycrds" {
			for _, policyCRD := range policyManager.GetCRDs() {
				shouldSkip := len(o.resourceNames) > 0 && (o.resourceNames[0] != policyCRD.CRD.GetName())
				if shouldSkip {
					continue
				}
				node, err := encodePolicyCRDAsNode(policyCRD)
				if err != nil {
					return nil, err
				}
				nodes = append(nodes, node)
			}
		}
	}

	return nodes, nil
}

func (o *getOptions) printNodes(nodes []*topology.Node) error {
	printerOptions := printer.PrinterOptions{
		OutputFormat: o.output,
		Clock:        clock.RealClock{},
		Description:  o.isDescribe,
		EventFetcher: printer.NewDefaultEventFetcher(o.factory),
	}
	p := printer.NewPrinter(printerOptions)
	defer p.Flush(o.IOStreams.Out)

	// Group nodes by resource type, preserving the order of resource types as specified
	nodesByType := make(map[string][]*topology.Node)
	typeOrder := []string{}

	for _, node := range nodes {
		gknn := node.GKNN()
		typeStr := gknn.GroupKind().String()
		if _, exists := nodesByType[typeStr]; !exists {
			typeOrder = append(typeOrder, typeStr)
		}
		nodesByType[typeStr] = append(nodesByType[typeStr], node)
	}

	// Print nodes grouped by type, sorted within each type
	for _, typeStr := range typeOrder {
		groupNodes := nodesByType[typeStr]
		sortedGroupNodes := topology.SortedNodes(groupNodes)
		for _, node := range sortedGroupNodes {
			err := p.PrintNode(node, o.IOStreams.Out)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func parseResourceTypeOrNameArgs(args []string) (resourceTypes []string, resourceNames []string, err error) {
	if len(args) == 0 {
		return nil, nil, fmt.Errorf("at least one resource type must be specified")
	}

	// Parse resource types and names
	typesStr := args[0]
	types := strings.Split(typesStr, ",")

	for _, t := range types {
		t = strings.TrimSpace(t)
		if t != "" {
			resourceTypes = append(resourceTypes, t)
		}
	}

	// If a resource name is provided (second argument), store it
	if len(args) > 1 {
		resourceNames = append(resourceNames, args[1])
	}

	return resourceTypes, resourceNames, nil
}

func encodePolicyAsNode(policy *policymanager.Policy) *topology.Node {
	return &topology.Node{
		Object: policy.Unstructured,
		Metadata: map[string]any{
			common.PolicyGK.String(): policy,
		},
	}
}

func encodePolicyCRDAsNode(policyCRD *policymanager.PolicyCRD) (*topology.Node, error) {
	o, err := runtime.DefaultUnstructuredConverter.ToUnstructured(policyCRD.CRD)
	if err != nil {
		return nil, err
	}
	u := &unstructured.Unstructured{Object: o}

	return &topology.Node{
		Object: u,
		Metadata: map[string]any{
			common.PolicyCRDGK.String(): policyCRD,
		},
	}, nil
}
