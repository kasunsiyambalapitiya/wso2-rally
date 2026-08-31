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

package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteError(t *testing.T) {
	rr := httptest.NewRecorder()

	WriteError(rr, http.StatusNotFound, MsgNotFound)

	require.Equal(t, http.StatusNotFound, rr.Code)
	require.Equal(t, "application/json", rr.Header().Get("Content-Type"))
	var body map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	require.Equal(t, "Resource not found.", body["message"])
	require.Len(t, body, 1, "error bodies carry only a message")
}

func TestWriteJSON(t *testing.T) {
	rr := httptest.NewRecorder()

	WriteJSON(rr, http.StatusCreated, map[string]int{"n": 1})

	require.Equal(t, http.StatusCreated, rr.Code)
	require.JSONEq(t, `{"n":1}`, rr.Body.String())
}

func TestWriteNoContent(t *testing.T) {
	rr := httptest.NewRecorder()

	WriteNoContent(rr)

	require.Equal(t, http.StatusNoContent, rr.Code)
	require.Empty(t, rr.Body.String())
}

func TestDecodeJSON_Valid(t *testing.T) {
	var dst struct {
		Name string `json:"name"`
	}
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"name":"rally"}`))

	require.NoError(t, DecodeJSON(req, &dst))
	require.Equal(t, "rally", dst.Name)
}

func TestDecodeJSON_RejectsUnknownFields(t *testing.T) {
	var dst struct {
		Name string `json:"name"`
	}
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"name":"r","nope":1}`))

	err := DecodeJSON(req, &dst)

	require.Error(t, err)
	require.Contains(t, err.Error(), "nope")
}

func TestDecodeJSON_RejectsEmptyBody(t *testing.T) {
	var dst struct{}
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(""))

	require.ErrorIs(t, DecodeJSON(req, &dst), ErrEmptyBody)
}

func TestDecodeJSON_RejectsTrailingContent(t *testing.T) {
	var dst struct {
		Name string `json:"name"`
	}
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"name":"a"}{"name":"b"}`))

	require.ErrorIs(t, DecodeJSON(req, &dst), ErrTrailingContent)
}

func TestDecodeJSON_RejectsOversizedBody(t *testing.T) {
	var dst struct {
		Name string `json:"name"`
	}
	huge := `{"name":"` + strings.Repeat("a", int(MaxBodyBytes)) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(huge))

	require.ErrorIs(t, DecodeJSON(req, &dst), ErrBodyTooLarge)
}

func TestDecodeJSON_WrapsSyntaxErrors(t *testing.T) {
	var dst struct {
		Name string `json:"name"`
	}
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"name":`))

	err := DecodeJSON(req, &dst)

	require.Error(t, err)
	require.NotErrorIs(t, err, ErrEmptyBody)
}

func TestRequestID_RoundTrip(t *testing.T) {
	ctx := WithRequestID(context.Background(), "abc123")

	require.Equal(t, "abc123", RequestIDFrom(ctx))
	require.Empty(t, RequestIDFrom(context.Background()))
}

func TestErrEmptyBody_IsDistinct(t *testing.T) {
	require.False(t, errors.Is(ErrEmptyBody, ErrBodyTooLarge))
}

// RFC 7235 requires a challenge on a 401; without it a generic client cannot
// tell the response apart from a 403.
func TestWriteUnauthorized_CarriesTheBearerChallenge(t *testing.T) {
	rr := httptest.NewRecorder()

	WriteUnauthorized(rr)

	require.Equal(t, http.StatusUnauthorized, rr.Code)
	require.Equal(t, `Bearer realm="rally"`, rr.Header().Get("WWW-Authenticate"))
	require.Equal(t, MsgUnauthorized, messageOf(t, rr))
}

func TestWriteCreated_PointsAtTheNewResource(t *testing.T) {
	rr := httptest.NewRecorder()

	WriteCreated(rr, "/events/abc", map[string]string{"id": "abc"})

	require.Equal(t, http.StatusCreated, rr.Code)
	require.Equal(t, "/events/abc", rr.Header().Get("Location"))
	require.JSONEq(t, `{"id":"abc"}`, rr.Body.String())
}

func TestWriteCreated_OmitsAnEmptyLocation(t *testing.T) {
	rr := httptest.NewRecorder()

	WriteCreated(rr, "", map[string]string{"id": "abc"})

	require.Equal(t, http.StatusCreated, rr.Code)
	require.Empty(t, rr.Header().Get("Location"))
}

func messageOf(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()

	var body struct {
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))

	return body.Message
}
