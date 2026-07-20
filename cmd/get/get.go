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
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
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
		cmd.Flags().BoolVarP(&flags.watch, "watch", "w", false, "After listing/getting the requested object, watch for changes.")

		flags.forFlag.AddFlag(cmd.Flags())
	}

	return cmd
}

// getFlags contains the flags used with get command.
type getFlags struct {
	resourceBuilderFlags *genericclioptions.ResourceBuilderFlags
	outputFormat         string
	watch                bool
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
		watch:         f.watch,
	}

	var err error
	o.resourceTypes, o.hasPolicy, o.hasPolicyCRD, err = parseResourceTypeOrNameArgs(args)
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
	if err := o.validateWatch(); err != nil {
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
	watch         bool

	resourceTypes []string
	hasPolicy     bool
	hasPolicyCRD  bool

	genericclioptions.IOStreams
}

func (o *getOptions) Run(args []string) error {
	if o.watch {
		return o.watchResources(args)
	}

	needsExtensions := o.isDescribe || o.output == printer.OutputFormatWide || o.output == printer.OutputFormatGraph

	// Initialize PolicyManager if needed (by either non-policy path extensions or policy path)
	var pm *policymanager.PolicyManager
	if o.hasPolicy || o.hasPolicyCRD || needsExtensions {
		pm = policymanager.New(common.NewDefaultGroupKindFetcher(o.factory))
		if err := pm.Init(); err != nil {
			return err
		}
	}

	var allNodes []*topology.Node

	// Process non-policy resource types through k8s resource builder
	if len(o.resourceTypes) > 0 {
		nonPolicyArgs := make([]string, len(args))
		nonPolicyArgs[0] = strings.Join(o.resourceTypes, ",")
		copy(nonPolicyArgs[1:], args[1:])

		infos, err := o.factory.NewBuilder().
			Unstructured().
			Flatten().
			NamespaceParam(o.namespace).DefaultNamespace().AllNamespaces(o.allNamespaces).
			ResourceTypeOrNameArgs(true, nonPolicyArgs...).
			LabelSelectorParam(o.labelSelector).
			ContinueOnError().
			Do().
			Infos()
		if err != nil {
			return err
		}

		sources := []*unstructured.Unstructured{}
		for _, info := range infos {
			obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(info.Object) //nolint:govet
			if err != nil {
				return err
			}
			sources = append(sources, &unstructured.Unstructured{Object: obj})
		}

		var graph *topology.Graph
		if needsExtensions {
			graph, err = o.buildExtendedGraph(sources, pm)
		} else {
			graph, err = topology.NewBuilder(common.NewDefaultGroupKindFetcher(o.factory)).StartFrom(sources).Build()
		}
		if err != nil {
			return err
		}

		if o.output == printer.OutputFormatGraph {
			if o.hasPolicy || o.hasPolicyCRD {
				fmt.Fprintf(o.ErrOut, "Warning: policy types are not shown in graph output\n")
			}
			toDotGraph, err := topologygw.ToDot(graph)
			if err != nil {
				return err
			}
			fmt.Fprintf(o.Out, "%v\n", toDotGraph)
			return nil
		}

		allNodes = append(allNodes, graph.Sources...)
	}

	// Process policy types through PolicyManager
	if o.hasPolicy || o.hasPolicyCRD {
		nodes, err := o.collectPolicyNodes(pm, args)
		if err != nil {
			return err
		}
		allNodes = append(allNodes, nodes...)
	}

	return o.printNodes(allNodes)
}

func (o *getOptions) validateWatch() error {
	if !o.watch {
		return nil
	}
	if o.isDescribe {
		return fmt.Errorf("--watch is not supported with describe; use get instead")
	}

	typeCount := len(o.resourceTypes)
	if o.hasPolicy {
		typeCount++
	}
	if o.hasPolicyCRD {
		typeCount++
	}
	if typeCount > 1 {
		return fmt.Errorf("you may only specify a single resource type when watching")
	}
	if o.hasPolicy || o.hasPolicyCRD {
		return fmt.Errorf("watch is not supported for policy/policycrd types")
	}
	if o.output != printer.OutputFormatTable && o.output != printer.OutputFormatWide {
		return fmt.Errorf("--watch is not supported with output format %q", o.output)
	}
	return nil
}

func (o *getOptions) watchResources(args []string) error {
	var pm *policymanager.PolicyManager
	if o.output == printer.OutputFormatWide {
		pm = policymanager.New(common.NewDefaultGroupKindFetcher(o.factory))
		if err := pm.Init(); err != nil {
			return err
		}
	}

	nonPolicyArgs := make([]string, len(args))
	nonPolicyArgs[0] = strings.Join(o.resourceTypes, ",")
	copy(nonPolicyArgs[1:], args[1:])

	result := o.factory.NewBuilder().
		Unstructured().
		NamespaceParam(o.namespace).DefaultNamespace().AllNamespaces(o.allNamespaces).
		ResourceTypeOrNameArgs(true, nonPolicyArgs...).
		SingleResourceType().
		LabelSelectorParam(o.labelSelector).
		Latest().
		ContinueOnError().
		Do()
	if err := result.Err(); err != nil {
		return err
	}
	if _, err := result.Infos(); err != nil {
		return err
	}

	obj, err := result.Object()
	if err != nil {
		return err
	}
	objects, resourceVersion, err := extractInitialObjects(obj)
	if err != nil {
		return err
	}
	skipInitialAdded := !meta.IsListType(obj)

	p := &printer.TablePrinter{PrinterOptions: printer.PrinterOptions{
		OutputFormat:  o.output,
		Clock:         clock.RealClock{},
		AllNamespaces: o.allNamespaces,
	}}
	nodes, err := o.buildWatchNodes(objects, pm)
	if err != nil {
		return err
	}
	for _, node := range topology.SortedNodes(nodes) {
		if err := p.PrintNode(node, o.Out); err != nil {
			return err
		}
	}
	if err := p.FlushWatch(o.Out); err != nil {
		return err
	}

	watcher, err := result.Watch(resourceVersion)
	if err != nil {
		return err
	}
	defer watcher.Stop()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-watcher.ResultChan():
			if !ok {
				fmt.Fprintln(o.ErrOut, "server closed the watch stream; exiting")
				return nil
			}

			switch event.Type {
			case watch.Bookmark:
				if skipInitialAdded && isInitialEventsEndBookmark(event.Object) {
					// A named resource can disappear between the initial get and
					// the watch-list snapshot, which ends without an ADDED event.
					skipInitialAdded = false
				}
			case watch.Added, watch.Modified, watch.Deleted:
				if skipInitialAdded {
					// The first event on a named-resource watch is the
					// synthetic ADDED for the object already printed above.
					skipInitialAdded = false
					if event.Type == watch.Added {
						continue
					}
				}
				nodes, err := o.buildWatchNodes([]runtime.Object{event.Object}, pm)
				if err != nil {
					return err
				}
				for _, node := range nodes {
					if err := p.PrintNode(node, o.Out); err != nil {
						return err
					}
				}
				if err := p.FlushWatch(o.Out); err != nil {
					return err
				}
			case watch.Error:
				return apierrors.FromObject(event.Object)
			}
		}
	}
}

// buildExtendedGraph builds a topology graph from sources using all gateway
// relationships and runs the full set of extensions over it.
func (o *getOptions) buildExtendedGraph(sources []*unstructured.Unstructured, pm *policymanager.PolicyManager) (*topology.Graph, error) {
	graph, err := topology.NewBuilder(common.NewDefaultGroupKindFetcher(o.factory)).
		StartFrom(sources).
		UseRelationships(topologygw.AllRelations).
		Build()
	if err != nil {
		return nil, err
	}
	if err := extension.ExecuteAll(graph, //nolint:govet
		directlyattachedpolicy.NewExtension(pm),
		gatewayeffectivepolicy.NewExtension(),
		refgrantvalidator.NewExtension(refgrantvalidator.NewDefaultReferenceGrantFetcher(o.factory)),
		notfoundrefvalidator.NewExtension(),
	); err != nil {
		return nil, err
	}
	return graph, nil
}

func extractInitialObjects(obj runtime.Object) ([]runtime.Object, string, error) {
	if !meta.IsListType(obj) {
		return []runtime.Object{obj}, "0", nil
	}

	resourceVersion, err := meta.NewAccessor().ResourceVersion(obj)
	if err != nil {
		return nil, "", err
	}
	objects, err := meta.ExtractList(obj)
	if err != nil {
		return nil, "", err
	}
	return objects, resourceVersion, nil
}

func isInitialEventsEndBookmark(obj runtime.Object) bool {
	if obj == nil {
		return false
	}
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return false
	}
	return accessor.GetAnnotations()[metav1.InitialEventsAnnotationKey] == "true"
}

func (o *getOptions) buildWatchNodes(objects []runtime.Object, pm *policymanager.PolicyManager) ([]*topology.Node, error) {
	sources := make([]*unstructured.Unstructured, 0, len(objects))
	for _, object := range objects {
		obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(object)
		if err != nil {
			return nil, err
		}
		sources = append(sources, &unstructured.Unstructured{Object: obj})
	}

	if o.output != printer.OutputFormatWide {
		nodes := make([]*topology.Node, 0, len(sources))
		for _, source := range sources {
			nodes = append(nodes, &topology.Node{Object: source})
		}
		return nodes, nil
	}

	graph, err := o.buildExtendedGraph(sources, pm)
	if err != nil {
		return nil, err
	}
	return graph.Sources, nil
}

func (o *getOptions) collectPolicyNodes(pm *policymanager.PolicyManager, args []string) ([]*topology.Node, error) {
	nodes := []*topology.Node{}
	if o.hasPolicy {
		for _, policy := range pm.GetPolicies() {
			shouldSkip := (!o.allNamespaces && o.namespace != policy.GKNN().Namespace) ||
				(len(args) == 2 && args[1] != policy.GKNN().Name)
			if shouldSkip {
				continue
			}
			nodes = append(nodes, encodePolicyAsNode(policy))
		}
	}
	if o.hasPolicyCRD {
		for _, policyCRD := range pm.GetCRDs() {
			shouldSkip := len(args) == 2 && (args[1] != policyCRD.CRD.GetName())
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
	return nodes, nil
}

func (o *getOptions) printNodes(nodes []*topology.Node) error {
	printerOptions := printer.PrinterOptions{
		OutputFormat:  o.output,
		Clock:         clock.RealClock{},
		Description:   o.isDescribe,
		EventFetcher:  printer.NewDefaultEventFetcher(o.factory),
		AllNamespaces: o.allNamespaces,
	}
	p := printer.NewPrinter(printerOptions)
	defer p.Flush(o.Out)
	for _, node := range topology.SortedNodes(nodes) {
		err := p.PrintNode(node, o.Out)
		if err != nil {
			return err
		}
	}
	return nil
}

func parseResourceTypeOrNameArgs(args []string) (resourceTypes []string, hasPolicy, hasPolicyCRD bool, err error) {
	tokens := strings.Split(args[0], ",")
	totalTokens := 0
	hasSlash := false
	for _, t := range tokens {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		totalTokens++
		switch t {
		case "policy", "policies":
			hasPolicy = true
		case "policycrd", "policycrds":
			hasPolicyCRD = true
		default:
			if strings.Contains(t, "/") {
				hasSlash = true
			}
			resourceTypes = append(resourceTypes, t)
		}
	}
	if hasSlash && totalTokens > 1 {
		return nil, false, false, fmt.Errorf("cannot combine TYPE/NAME syntax (e.g. gateway/my-gw) with multiple comma-separated types")
	}
	return resourceTypes, hasPolicy, hasPolicyCRD, nil
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
