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

package common

import (
	"errors"
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestIsResourceTypeNotFoundError(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "typed NoResourceMatchError",
			err: &meta.NoResourceMatchError{
				PartialResource: schema.GroupVersionResource{Resource: "grpcroutes"},
			},
			want: true,
		},
		{
			name: "typed NoKindMatchError",
			err: &meta.NoKindMatchError{
				GroupKind: GRPCRouteGK,
			},
			want: true,
		},
		{
			name: "wrapped typed error",
			err: fmt.Errorf("fetching: %w", &meta.NoResourceMatchError{
				PartialResource: schema.GroupVersionResource{Resource: "grpcroutes"},
			}),
			want: true,
		},
		{
			name: "untyped resource builder message",
			err:  errors.New(`the server doesn't have a resource type "GRPCRoute"`),
			want: true,
		},
		{
			name: "unrelated error",
			err:  errors.New("connection refused"),
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isResourceTypeNotFoundError(tc.err); got != tc.want {
				t.Errorf("isResourceTypeNotFoundError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
