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

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoad_MissingRequired(t *testing.T) {
	t.Setenv("DB_DSN", "")
	t.Setenv("TEAM_TOKEN_SECRET", "")

	_, err := Load()

	require.Error(t, err)
	require.Contains(t, err.Error(), "DB_DSN")
	require.Contains(t, err.Error(), "TEAM_TOKEN_SECRET")
}

func TestLoad_DefaultsAndValues(t *testing.T) {
	t.Setenv("DB_DSN", "user:pass@tcp(localhost:3306)/rally")
	t.Setenv("TEAM_TOKEN_SECRET", "s3cret")

	c, err := Load()

	require.NoError(t, err)
	require.Equal(t, "8080", c.Port)
	require.Equal(t, "INFO", c.LogLevel)
	require.Equal(t, "user:pass@tcp(localhost:3306)/rally", c.DBDsn)
	require.Equal(t, "rally-admin", c.AdminRole)
	require.Equal(t, 12*time.Hour, c.TeamTokenTTL)
	require.False(t, c.TokenValidatorEnabled)
}

func TestLoad_TokenValidatorRequiresJWKS(t *testing.T) {
	t.Setenv("DB_DSN", "dsn")
	t.Setenv("TEAM_TOKEN_SECRET", "s3cret")
	t.Setenv("TOKEN_VALIDATOR_ENABLED", "true")
	t.Setenv("JWKS_ENDPOINT", "")

	_, err := Load()

	require.Error(t, err)
	require.Contains(t, err.Error(), "JWKS_ENDPOINT")
}

func TestLoad_InvalidTeamTokenTTL(t *testing.T) {
	t.Setenv("DB_DSN", "dsn")
	t.Setenv("TEAM_TOKEN_SECRET", "s3cret")
	t.Setenv("TEAM_TOKEN_TTL", "not-a-duration")

	_, err := Load()

	require.Error(t, err)
	require.Contains(t, err.Error(), "TEAM_TOKEN_TTL")
}

func TestLogLevel_ParsesKnownLevels(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"DEBUG", "DEBUG"},
		{"debug", "DEBUG"},
		{"WARN", "WARN"},
		{"ERROR", "ERROR"},
		{"nonsense", "INFO"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			require.Equal(t, tt.want, Config{LogLevel: tt.in}.SlogLevel().String())
		})
	}
}
