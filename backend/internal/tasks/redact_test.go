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

package tasks

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedactForCrew(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			"cipher answer is removed, prompt survives",
			`{"prompt":"Translate the sign","options":["A","B"],"answer":"A"}`,
			`{"prompt":"Translate the sign","options":["A","B"]}`,
		},
		{
			"multi-select answers",
			`{"options":["a","b","c"],"answers":["a","c"]}`,
			`{"options":["a","b","c"]}`,
		},
		{
			"grid solution",
			`{"rows":3,"cols":3,"solution":["A","B","C"]}`,
			`{"rows":3,"cols":3}`,
		},
		{
			"barcode payload",
			`{"hint":"On the signpost","payload":"WSO2-CHECKPOINT-4"}`,
			`{"hint":"On the signpost"}`,
		},
		{
			"blind timer target and tolerance",
			`{"label":"Count 45 seconds","targetSec":45,"tolerance":3}`,
			`{"label":"Count 45 seconds"}`,
		},
		{
			"branch scoring",
			`{"prompt":"Solve or skip?","solvePoints":40,"skipPoints":-40}`,
			`{"prompt":"Solve or skip?"}`,
		},
		{"nothing to redact", `{"prompt":"Hello"}`, `{"prompt":"Hello"}`},
		{"empty object", `{}`, `{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.JSONEq(t, tt.want, string(RedactForCrew(json.RawMessage(tt.in))))
		})
	}
}

// An unreadable config cannot be checked for secrets, so nothing is sent.
func TestRedactForCrew_UnreadableConfigBecomesEmpty(t *testing.T) {
	for _, in := range []string{`[1,2,3]`, `"a string"`, `not json`, ``} {
		require.JSONEq(t, `{}`, string(RedactForCrew(json.RawMessage(in))))
	}
}

// Every secret key must actually be stripped, so adding one to the list is
// enough to protect it.
func TestRedactForCrew_StripsEverySecretKey(t *testing.T) {
	fields := map[string]any{"prompt": "keep me"}
	for _, key := range secretConfigKeys {
		fields[key] = "secret"
	}
	config, err := json.Marshal(fields)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(RedactForCrew(config), &got))

	require.Equal(t, map[string]any{"prompt": "keep me"}, got)
}
