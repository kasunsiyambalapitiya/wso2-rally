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

package authz

import (
	"context"
	"fmt"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

// Asgardeo id-token claims the organizer identity is built from.
const (
	emailClaim  = "email"
	groupsClaim = "groups"
)

// OrganizerValidator turns an Asgardeo id token into an Identity.
//
// It is built once at startup: the JWKS-backed variant keeps a cached,
// background-refreshing key set, so validating a request never blocks on a
// call to Asgardeo.
type OrganizerValidator struct {
	// keyFunc is nil in decode-only mode.
	keyFunc jwt.Keyfunc
}

// NewJWKSValidator builds a validator that verifies signatures against the
// keys published at jwksEndpoint. Use this in every deployed environment.
func NewJWKSValidator(ctx context.Context, jwksEndpoint string) (*OrganizerValidator, error) {
	if jwksEndpoint == "" {
		return nil, fmt.Errorf("jwks endpoint is required for token validation")
	}

	k, err := keyfunc.NewDefaultCtx(ctx, []string{jwksEndpoint})
	if err != nil {
		return nil, fmt.Errorf("initialise jwks key set from %s: %w", jwksEndpoint, err)
	}

	return &OrganizerValidator{keyFunc: k.Keyfunc}, nil
}

// NewDecodeOnlyValidator builds a validator that reads claims WITHOUT checking
// the signature.
//
// This is for local development only, where there is no Asgardeo tenant to
// call. It trusts whatever the client sends, so it must never be enabled in a
// deployed environment — config.TokenValidatorEnabled gates it.
func NewDecodeOnlyValidator() *OrganizerValidator {
	return &OrganizerValidator{}
}

// Validate resolves an organizer identity from a raw bearer token.
func (v *OrganizerValidator) Validate(raw string) (Identity, error) {
	if v.keyFunc == nil {
		return DecodeOrganizer(raw)
	}

	parsed, err := jwt.Parse(raw, v.keyFunc,
		jwt.WithValidMethods([]string{"RS256", "RS384", "RS512", "ES256", "ES384", "ES512"}),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !parsed.Valid {
		return Identity{}, ErrInvalidToken
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return Identity{}, ErrInvalidToken
	}

	return organizerFromClaims(claims)
}

// DecodeOrganizer reads an organizer identity from a token WITHOUT verifying
// its signature. Expiry and issuer are still enforced, so an obviously stale
// or team-issued token is rejected.
//
// Only the decode-only validator calls this. See NewDecodeOnlyValidator for
// why that is development-only.
func DecodeOrganizer(raw string) (Identity, error) {
	var claims jwt.MapClaims
	if _, _, err := jwt.NewParser().ParseUnverified(raw, &claims); err != nil {
		return Identity{}, ErrInvalidToken
	}

	// A team token on the organizer path would otherwise be silently accepted
	// with an empty group list.
	if issuer, _ := claims.GetIssuer(); issuer == teamTokenIssuer {
		return Identity{}, ErrInvalidToken
	}

	exp, err := claims.GetExpirationTime()
	if err != nil {
		return Identity{}, ErrInvalidToken
	}
	if exp != nil && exp.Before(time.Now()) {
		return Identity{}, ErrInvalidToken
	}

	return organizerFromClaims(claims)
}

func organizerFromClaims(claims jwt.MapClaims) (Identity, error) {
	userID := stringClaim(claims, "sub")
	if userID == "" {
		return Identity{}, ErrInvalidToken
	}

	return Identity{
		Kind:   KindOrganizer,
		UserID: userID,
		Email:  stringClaim(claims, emailClaim),
		Groups: groupsFromClaim(claims[groupsClaim]),
	}, nil
}

// groupsFromClaim normalises the "groups" claim, which Asgardeo emits as a
// JSON array but which collapses to a bare string for a single group.
func groupsFromClaim(raw any) []string {
	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []string:
		return v
	case []any:
		groups := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				groups = append(groups, s)
			}
		}
		if len(groups) == 0 {
			return nil
		}
		return groups
	default:
		return nil
	}
}
