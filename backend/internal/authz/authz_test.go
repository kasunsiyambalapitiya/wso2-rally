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
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

const testSecret = "test-secret"

var testTeamClaims = TeamClaims{
	SessionID:    "sess1",
	VehicleID:    "veh1",
	DeviceID:     "dev1",
	CrewMemberID: "crew1",
}

func TestMintTeamToken_RoundTrip(t *testing.T) {
	tok, err := MintTeamToken(testSecret, testTeamClaims, time.Hour)
	require.NoError(t, err)

	id, err := VerifyTeamToken(testSecret, tok)

	require.NoError(t, err)
	require.Equal(t, KindTeam, id.Kind)
	require.Equal(t, "sess1", id.SessionID)
	require.Equal(t, "veh1", id.VehicleID)
}

// Every phone in a car shares one session, so the session id alone no longer
// says who is calling. Handlers need the device and the member behind it to
// record who answered a task and to report who is sharing location.
func TestMintTeamToken_CarriesDeviceAndCrew(t *testing.T) {
	tok, err := MintTeamToken(testSecret, testTeamClaims, time.Hour)
	require.NoError(t, err)

	id, err := VerifyTeamToken(testSecret, tok)

	require.NoError(t, err)
	require.Equal(t, "dev1", id.DeviceID)
	require.Equal(t, "crew1", id.CrewMemberID)
}

// A token minted before phones became distinguishable identifies a session but
// no device. It must fail closed so the phone re-joins, rather than reaching a
// handler that would then have to invent a device for it.
func TestVerifyTeamToken_RejectsTokenWithoutDevice(t *testing.T) {
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss": teamTokenIssuer,
		"sub": "sess1",
		"veh": "veh1",
		"exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte(testSecret))
	require.NoError(t, err)

	_, err = VerifyTeamToken(testSecret, tok)

	require.ErrorIs(t, err, ErrInvalidToken)
}

func TestMintTeamToken_RejectsEmptySecret(t *testing.T) {
	_, err := MintTeamToken("", testTeamClaims, time.Hour)

	require.ErrorIs(t, err, ErrNoSigningSecret)
}

func TestVerifyTeamToken_WrongSecret(t *testing.T) {
	tok, err := MintTeamToken(testSecret, testTeamClaims, time.Hour)
	require.NoError(t, err)

	_, err = VerifyTeamToken("other-secret", tok)

	require.ErrorIs(t, err, ErrInvalidToken)
}

func TestVerifyTeamToken_Expired(t *testing.T) {
	tok, err := MintTeamToken(testSecret, testTeamClaims, -time.Minute)
	require.NoError(t, err)

	_, err = VerifyTeamToken(testSecret, tok)

	require.ErrorIs(t, err, ErrInvalidToken)
}

func TestVerifyTeamToken_RejectsGarbage(t *testing.T) {
	_, err := VerifyTeamToken(testSecret, "not.a.token")

	require.ErrorIs(t, err, ErrInvalidToken)
}

// A token signed with "alg":"none" must never be accepted, and neither must an
// organizer token presented on the team path.
func TestVerifyTeamToken_RejectsUnsignedAndForeignIssuer(t *testing.T) {
	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"iss": teamTokenIssuer,
		"sub": "sess1",
		"exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	foreignIssuer, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss": "https://api.asgardeo.io/t/wso2",
		"sub": "sess1",
		"exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte(testSecret))
	require.NoError(t, err)

	for name, tok := range map[string]string{"alg none": unsigned, "foreign issuer": foreignIssuer} {
		t.Run(name, func(t *testing.T) {
			_, err := VerifyTeamToken(testSecret, tok)
			require.ErrorIs(t, err, ErrInvalidToken)
		})
	}
}

func TestVerifyTeamToken_RequiresSessionID(t *testing.T) {
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss": teamTokenIssuer,
		"veh": "veh1",
		"exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte(testSecret))
	require.NoError(t, err)

	_, err = VerifyTeamToken(testSecret, tok)

	require.ErrorIs(t, err, ErrInvalidToken)
}

func TestDecodeOrganizer(t *testing.T) {
	tok := organizerToken(t, jwt.MapClaims{
		"sub":    "user-1",
		"email":  "organizer@wso2.com",
		"groups": []string{"rally-admin", "everyone"},
	})

	id, err := DecodeOrganizer(tok)

	require.NoError(t, err)
	require.Equal(t, KindOrganizer, id.Kind)
	require.Equal(t, "user-1", id.UserID)
	require.Equal(t, "organizer@wso2.com", id.Email)
	require.Equal(t, []string{"rally-admin", "everyone"}, id.Groups)
}

func TestDecodeOrganizer_AcceptsSingleStringGroup(t *testing.T) {
	tok := organizerToken(t, jwt.MapClaims{"sub": "u", "groups": "rally-admin"})

	id, err := DecodeOrganizer(tok)

	require.NoError(t, err)
	require.Equal(t, []string{"rally-admin"}, id.Groups)
}

func TestDecodeOrganizer_MissingGroupsIsEmpty(t *testing.T) {
	tok := organizerToken(t, jwt.MapClaims{"sub": "u"})

	id, err := DecodeOrganizer(tok)

	require.NoError(t, err)
	require.Empty(t, id.Groups)
}

func TestDecodeOrganizer_RejectsMalformed(t *testing.T) {
	_, err := DecodeOrganizer("garbage")

	require.ErrorIs(t, err, ErrInvalidToken)
}

func TestDecodeOrganizer_RejectsExpired(t *testing.T) {
	tok := organizerToken(t, jwt.MapClaims{"sub": "u", "exp": time.Now().Add(-time.Hour).Unix()})

	_, err := DecodeOrganizer(tok)

	require.ErrorIs(t, err, ErrInvalidToken)
}

func TestDecodeOrganizer_RejectsTeamIssuer(t *testing.T) {
	teamTok, err := MintTeamToken(testSecret, testTeamClaims, time.Hour)
	require.NoError(t, err)

	_, err = DecodeOrganizer(teamTok)

	require.ErrorIs(t, err, ErrInvalidToken)
}

func TestCheckRoles(t *testing.T) {
	tests := []struct {
		name     string
		required []string
		have     []string
		want     bool
	}{
		{"single match", []string{"rally-admin"}, []string{"x", "rally-admin"}, true},
		{"missing", []string{"rally-admin"}, []string{"x"}, false},
		{"all required present", []string{"a", "b"}, []string{"b", "c", "a"}, true},
		{"one of several missing", []string{"a", "b"}, []string{"a"}, false},
		{"no requirement passes", nil, []string{"a"}, true},
		{"requirement with no groups fails", []string{"a"}, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, CheckRoles(tt.required, tt.have))
		})
	}
}

func TestIdentity_ContextRoundTrip(t *testing.T) {
	want := Identity{Kind: KindOrganizer, UserID: "u1", Email: "u@wso2.com"}

	got, ok := IdentityFrom(WithIdentity(context.Background(), want))

	require.True(t, ok)
	require.Equal(t, want, got)
}

func TestIdentityFrom_EmptyContext(t *testing.T) {
	_, ok := IdentityFrom(context.Background())

	require.False(t, ok)
}

func TestIdentity_HasRole(t *testing.T) {
	id := Identity{Groups: []string{"rally-admin"}}

	require.True(t, id.HasRole("rally-admin"))
	require.False(t, id.HasRole("other"))
}

// organizerToken builds a signed token that stands in for an Asgardeo id
// token. DecodeOrganizer never checks the signature, so any key works.
func organizerToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()

	if _, ok := claims["exp"]; !ok {
		claims["exp"] = time.Now().Add(time.Hour).Unix()
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("asgardeo-key"))
	require.NoError(t, err)

	return tok
}
