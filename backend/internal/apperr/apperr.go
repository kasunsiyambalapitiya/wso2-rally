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

// Package apperr defines the small set of error categories every domain shares
// so one place — httpx.WriteDomainError — can map them onto status codes.
//
// A domain declares its own sentinels by wrapping these, which keeps
// domain-specific wording while staying matchable by category:
//
//	var ErrNotFound = fmt.Errorf("%w: event", apperr.ErrNotFound)
package apperr

import (
	"errors"
	"fmt"
	"strings"
)

// The error categories. Anything a service returns that matches none of them
// is treated as an internal fault: logged in full, reported as a generic 500.
var (
	// ErrNotFound: the addressed resource does not exist. → 404
	ErrNotFound = errors.New("not found")
	// ErrValidation: the caller sent something unusable. → 400
	//
	// Messages wrapped in this category are shown to the client verbatim, so
	// they must never contain internal detail.
	ErrValidation = errors.New("validation failed")
	// ErrConflict: the resource's current state forbids the action. → 409
	ErrConflict = errors.New("state conflict")
	// ErrForbidden: authenticated, but not allowed to touch this resource. → 403
	ErrForbidden = errors.New("forbidden")
)

// Validationf builds a client-safe validation error.
func Validationf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrValidation, fmt.Sprintf(format, args...))
}

// Conflictf builds a client-safe conflict error.
func Conflictf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrConflict, fmt.Sprintf(format, args...))
}

// Message extracts the client-safe explanation from a categorised error —
// everything after the category prefix. It returns "" when err carries no
// explanation of its own, in which case the caller falls back to a generic
// message.
func Message(err error) string {
	if err == nil {
		return ""
	}

	for _, category := range []error{ErrValidation, ErrConflict, ErrNotFound, ErrForbidden} {
		if !errors.Is(err, category) {
			continue
		}
		if _, after, found := strings.Cut(err.Error(), category.Error()+": "); found {
			return after
		}
	}

	return ""
}
