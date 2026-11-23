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

package get

import (
	"errors"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/restmapper"

	"sigs.k8s.io/gwctl/pkg/printer"
)

var errWatchBuilderCallback = errors.New("watch builder callback should not be called")

type watchTestFactory struct{}

func (watchTestFactory) NewBuilder() *resource.Builder {
	return resource.NewFakeBuilder(
		func(schema.GroupVersion) (resource.RESTClient, error) {
			return nil, errWatchBuilderCallback
		},
		func() (meta.RESTMapper, error) {
			return nil, errWatchBuilderCallback
		},
		func() (restmapper.CategoryExpander, error) {
			return resource.FakeCategoryExpander, nil
		},
	)
}

func (watchTestFactory) KubeConfigNamespace() (string, bool, error) {
	return "default", false, nil
}

func TestValidateWatch(t *testing.T) {
	tests := []struct {
		name    string
		options getOptions
		wantErr string
	}{
		{
			name: "table output",
			options: getOptions{
				watch:         true,
				resourceTypes: []string{"gateways"},
			},
		},
		{
			name: "wide output",
			options: getOptions{
				watch:         true,
				output:        printer.OutputFormatWide,
				resourceTypes: []string{"gateways"},
			},
		},
		{
			name: "json output",
			options: getOptions{
				watch:         true,
				output:        printer.OutputFormatJSON,
				resourceTypes: []string{"gateways"},
			},
			wantErr: `--watch is not supported with output format "json"`,
		},
		{
			name: "multiple resource types",
			options: getOptions{
				watch:         true,
				resourceTypes: []string{"gateways", "httproutes"},
			},
			wantErr: "you may only specify a single resource type when watching",
		},
		{
			name: "policy type",
			options: getOptions{
				watch:     true,
				hasPolicy: true,
			},
			wantErr: "watch is not supported for policy/policycrd types",
		},
		{
			name: "policy and another type",
			options: getOptions{
				watch:         true,
				resourceTypes: []string{"gateways"},
				hasPolicy:     true,
			},
			wantErr: "you may only specify a single resource type when watching",
		},
		{
			name: "describe",
			options: getOptions{
				watch:         true,
				isDescribe:    true,
				resourceTypes: []string{"gateways"},
			},
			wantErr: "--watch is not supported with describe; use get instead",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.options.validateWatch()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateWatch() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateWatch() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestWatchResourcesRejectsExpandedCategory(t *testing.T) {
	o := getOptions{
		factory:       watchTestFactory{},
		namespace:     "default",
		output:        printer.OutputFormatTable,
		watch:         true,
		resourceTypes: []string{"all"},
	}
	if err := o.validateWatch(); err != nil {
		t.Fatalf("validateWatch() error = %v, want nil", err)
	}

	err := o.watchResources([]string{"all"})
	if !errors.Is(err, resource.ErrMultipleResourceTypes) {
		t.Fatalf("watchResources() error = %v, want %v", err, resource.ErrMultipleResourceTypes)
	}
}

func TestNewCmdRegistersWatchOnlyForGet(t *testing.T) {
	getCmd := NewCmd(nil, genericiooptions.IOStreams{}, false)
	watchFlag := getCmd.Flags().Lookup("watch")
	if watchFlag == nil {
		t.Fatal("get command does not register --watch")
	}
	if watchFlag.Shorthand != "w" {
		t.Fatalf("--watch shorthand = %q, want %q", watchFlag.Shorthand, "w")
	}

	describeCmd := NewCmd(nil, genericiooptions.IOStreams{}, true)
	if describeCmd.Flags().Lookup("watch") != nil {
		t.Fatal("describe command unexpectedly registers --watch")
	}
}
