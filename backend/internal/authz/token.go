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

// Claims a team token carries beyond the registered ones.
const (
	// vehicleClaim carries the vehicle whose session this is.
	vehicleClaim = "veh"
	// deviceClaim identifies which of the car's phones holds this token.
	deviceClaim = "dev"
	// crewClaim carries the member that phone belongs to.
	crewClaim = "crw"
)

// TeamClaims is what a team token asserts about the phone holding it.
//
// The session alone is no longer enough to identify a caller: every phone in
// the car shares one, so the device and the member behind it travel with it.
// A struct rather than four positional strings, because two of them are
// 32-char hex ids that would otherwise be trivial to transpose at a call site.
type TeamClaims struct {
	SessionID    string
	VehicleID    string
	DeviceID     string
	CrewMemberID string
}

// MintTeamToken issues the JWT an in-car phone holds for the rest of the
// rally. There is no participant login: joining a vehicle is what authenticates
// the device, and this token is the proof.
func MintTeamToken(secret string, in TeamClaims, ttl time.Duration) (string, error) {
	if secret == "" {
		return "", ErrNoSigningSecret
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"iss":        teamTokenIssuer,
		"sub":        in.SessionID,
		vehicleClaim: in.VehicleID,
		deviceClaim:  in.DeviceID,
		crewClaim:    in.CrewMemberID,
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

	// A token without a device predates phones being distinguishable. Failing
	// closed sends that phone back through join, which is cheap; accepting it
	// would hand a handler a caller it cannot attribute anything to.
	deviceID := stringClaim(claims, deviceClaim)
	if deviceID == "" {
		return Identity{}, ErrInvalidToken
	}

	return Identity{
		Kind:         KindTeam,
		SessionID:    sessionID,
		VehicleID:    stringClaim(claims, vehicleClaim),
		DeviceID:     deviceID,
		CrewMemberID: stringClaim(claims, crewClaim),
	}, nil
}

func stringClaim(claims jwt.MapClaims, key string) string {
	s, _ := claims[key].(string)
	return s
}
