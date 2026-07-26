// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package apperr

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidationf_IsMatchableAndExplained(t *testing.T) {
	err := Validationf("name is required")

	require.ErrorIs(t, err, ErrValidation)
	require.NotErrorIs(t, err, ErrConflict)
	require.Equal(t, "name is required", Message(err))
}

func TestConflictf(t *testing.T) {
	err := Conflictf("vehicle already has a live session")

	require.ErrorIs(t, err, ErrConflict)
	require.Equal(t, "vehicle already has a live session", Message(err))
}

func TestMessage(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"uncategorised", errors.New("db down"), ""},
		{"category with no explanation", ErrNotFound, ""},
		{"wrapped by a caller", fmt.Errorf("create event: %w", Validationf("name is required")), "name is required"},
		{"domain sentinel", fmt.Errorf("%w: event", ErrNotFound), "event"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, Message(tt.err))
		})
	}
}
