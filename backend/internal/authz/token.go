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
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// teamTokenIssuer marks a token as minted by this backend. It is what lets
// auth middleware tell a crew token apart from an Asgardeo id token.
const teamTokenIssuer = "rally-team"

// vehicleClaim carries the vehicle a session is bound to.
const vehicleClaim = "veh"

// MintTeamToken issues the JWT an in-car phone holds for the rest of the
// rally. There is no participant login: binding a vehicle is what authenticates
// the device, and this token is the proof.
func MintTeamToken(secret, sessionID, vehicleID string, ttl time.Duration) (string, error) {
	if secret == "" {
		return "", ErrNoSigningSecret
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"iss":        teamTokenIssuer,
		"sub":        sessionID,
		vehicleClaim: vehicleID,
		"iat":        now.Unix(),
		"exp":        now.Add(ttl).Unix(),
	}

	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		return "", err
	}

	return tok, nil
}

// VerifyTeamToken checks the signature, issuer, and expiry of a team token and
// returns the identity it asserts.
//
// The signing method is pinned to HMAC so a token claiming "alg":"none" — or an
// RS256 token whose "key" is our public secret — cannot be substituted.
func VerifyTeamToken(secret, raw string) (Identity, error) {
	parsed, err := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return []byte(secret), nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(teamTokenIssuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !parsed.Valid {
		return Identity{}, ErrInvalidToken
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return Identity{}, ErrInvalidToken
	}

	sessionID := stringClaim(claims, "sub")
	if sessionID == "" {
		return Identity{}, ErrInvalidToken
	}

	return Identity{
		Kind:      KindTeam,
		SessionID: sessionID,
		VehicleID: stringClaim(claims, vehicleClaim),
	}, nil
}

func stringClaim(claims jwt.MapClaims, key string) string {
	s, _ := claims[key].(string)
	return s
}
