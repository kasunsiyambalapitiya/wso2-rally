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

// Package authz resolves the two identities this backend serves.
//
// Organizers arrive with Asgardeo id tokens; crews arrive with team tokens the
// backend mints itself at POST /sessions/bind. Both reduce to an Identity that
// middleware places on the request context, and every authorization decision
// reads from there.
package authz

import (
	"context"
	"errors"
	"slices"
)

// Kind distinguishes the two callers sharing this backend.
type Kind string

const (
	// KindOrganizer is an Asgardeo-authenticated staff user.
	KindOrganizer Kind = "organizer"
	// KindTeam is an in-car phone holding a backend-minted team token.
	KindTeam Kind = "team"
)

// Errors callers match on. Middleware maps both to 401 with a generic message
// so a probing client learns nothing about why a token was rejected.
var (
	// ErrInvalidToken covers every rejection reason: bad signature, expired,
	// malformed, wrong issuer, missing required claim.
	ErrInvalidToken = errors.New("invalid token")
	// ErrNoSigningSecret means the server is misconfigured and cannot mint.
	ErrNoSigningSecret = errors.New("team token secret is not configured")
)

// Identity is the authenticated caller behind a request. Organizer fields
// (Email, UserID, Groups) and team fields (SessionID, VehicleID) are populated
// according to Kind.
type Identity struct {
	Kind Kind

	// Organizer.
	Email  string
	UserID string
	Groups []string

	// Team.
	SessionID string
	VehicleID string
}

// IsOrganizer reports whether the caller is staff.
func (i Identity) IsOrganizer() bool { return i.Kind == KindOrganizer }

// IsTeam reports whether the caller is an in-car phone.
func (i Identity) IsTeam() bool { return i.Kind == KindTeam }

// HasRole reports whether the identity carries the given group claim.
func (i Identity) HasRole(role string) bool { return slices.Contains(i.Groups, role) }

type ctxKey struct{}

// WithIdentity returns a context carrying the authenticated identity.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// IdentityFrom returns the identity middleware stored on ctx. The boolean is
// false on unauthenticated paths such as GET /health.
func IdentityFrom(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(ctxKey{}).(Identity)
	return id, ok
}

// CheckRoles reports whether every required role is present in have. An empty
// requirement passes: the caller only needed to be authenticated.
func CheckRoles(required, have []string) bool {
	for _, role := range required {
		if !slices.Contains(have, role) {
			return false
		}
	}

	return true
}
