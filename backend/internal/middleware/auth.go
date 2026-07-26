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

package middleware

import (
	"net/http"
	"strings"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/authz"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/config"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/httpx"
)

const bearerPrefix = "Bearer "

// OrganizerValidator resolves an Asgardeo id token. It is an interface so the
// router can inject either the JWKS-backed validator or the decode-only one
// without this package knowing which.
type OrganizerValidator interface {
	Validate(rawToken string) (authz.Identity, error)
}

// Auth authenticates every request carrying a bearer token and stores the
// resulting identity on the context.
//
// Team tokens are tried first because they are cheap to verify locally and
// carry our own issuer; anything else is handed to the organizer validator.
// A token that satisfies neither is a 401 — the response never says which
// check failed.
func Auth(cfg config.Config, organizer OrganizerValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := bearerToken(r)
			if !ok {
				httpx.WriteUnauthorized(w)
				return
			}

			identity, err := authz.VerifyTeamToken(cfg.TeamTokenSecret, raw)
			if err != nil {
				identity, err = organizer.Validate(raw)
			}
			if err != nil {
				httpx.WriteUnauthorized(w)
				return
			}

			next.ServeHTTP(w, r.WithContext(authz.WithIdentity(r.Context(), identity)))
		})
	}
}

// RequireOrganizer rejects anyone who is not staff. Mount it on every
// organizer route group, under Auth.
func RequireOrganizer(next http.Handler) http.Handler {
	return requireKind(authz.KindOrganizer, next)
}

// RequireTeam rejects anyone who is not an in-car phone.
func RequireTeam(next http.Handler) http.Handler {
	return requireKind(authz.KindTeam, next)
}

// RequireAdmin gates the organizer actions that change an event's shape, such
// as publishing it or importing vehicles.
func RequireAdmin(cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity, ok := authz.IdentityFrom(r.Context())
			if !ok {
				httpx.WriteUnauthorized(w)
				return
			}
			if !identity.IsOrganizer() || !authz.CheckRoles([]string{cfg.AdminRole}, identity.Groups) {
				httpx.WriteError(w, http.StatusForbidden, httpx.MsgForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func requireKind(kind authz.Kind, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, ok := authz.IdentityFrom(r.Context())
		if !ok {
			httpx.WriteUnauthorized(w)
			return
		}
		if identity.Kind != kind {
			httpx.WriteError(w, http.StatusForbidden, httpx.MsgForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// bearerToken extracts the credential from an Authorization header. The scheme
// match is case-insensitive per RFC 7235; the token itself is not trimmed
// beyond surrounding spaces.
func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if len(header) < len(bearerPrefix) || !strings.EqualFold(header[:len(bearerPrefix)], bearerPrefix) {
		return "", false
	}

	token := strings.TrimSpace(header[len(bearerPrefix):])

	return token, token != ""
}
