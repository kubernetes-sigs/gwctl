/*
Copyright The Kubernetes Authors.

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

package analyze

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyErrors(t *testing.T) {
	tests := []struct {
		name                string
		errorsBeforeChanges map[string]bool
		errorsAfterChanges  map[string]bool
		wantNew             []string
		wantFixed           []string
		wantUnchanged       []string
	}{
		{
			name:                "no errors",
			errorsBeforeChanges: map[string]bool{},
			errorsAfterChanges:  map[string]bool{},
			wantNew:             nil,
			wantFixed:           nil,
			wantUnchanged:       nil,
		},
		{
			name:                "all new",
			errorsBeforeChanges: map[string]bool{},
			errorsAfterChanges:  map[string]bool{"err-a": true, "err-b": true},
			wantNew:             []string{"err-a", "err-b"},
			wantFixed:           nil,
			wantUnchanged:       nil,
		},
		{
			name:                "all fixed",
			errorsBeforeChanges: map[string]bool{"err-a": true, "err-b": true},
			errorsAfterChanges:  map[string]bool{},
			wantNew:             nil,
			wantFixed:           []string{"err-a", "err-b"},
			wantUnchanged:       nil,
		},
		{
			name:                "unchanged errors are not double-counted",
			errorsBeforeChanges: map[string]bool{"err-a": true, "err-b": true},
			errorsAfterChanges:  map[string]bool{"err-a": true, "err-b": true},
			wantNew:             nil,
			wantFixed:           nil,
			wantUnchanged:       []string{"err-a", "err-b"},
		},
		{
			name:                "mixed",
			errorsBeforeChanges: map[string]bool{"err-old": true, "err-kept": true},
			errorsAfterChanges:  map[string]bool{"err-new": true, "err-kept": true},
			wantNew:             []string{"err-new"},
			wantFixed:           []string{"err-old"},
			wantUnchanged:       []string{"err-kept"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotNew, gotFixed, gotUnchanged := classifyErrors(tc.errorsBeforeChanges, tc.errorsAfterChanges)
			assert.ElementsMatch(t, tc.wantNew, gotNew, "newIssues")
			assert.ElementsMatch(t, tc.wantFixed, gotFixed, "fixedIssues")
			assert.ElementsMatch(t, tc.wantUnchanged, gotUnchanged, "unchangedIssues")
		})
	}
}
