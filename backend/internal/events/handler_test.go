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

package events

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/authz"
)

func newTestHandler(t *testing.T) (http.Handler, *fakeRepo) {
	t.Helper()

	repo := newFakeRepo()
	handler := NewHandler(NewService(repo), slog.New(slog.NewTextHandler(io.Discard, nil)))

	// Stand in for the auth middleware the router mounts in production.
	withIdentity := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := authz.Identity{Kind: authz.KindOrganizer, UserID: "u1", Email: "organizer@wso2.com"}
			next.ServeHTTP(w, r.WithContext(authz.WithIdentity(r.Context(), id)))
		})
	}

	return withIdentity(handler.Routes()), repo
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(method, path, reader))

	return rr
}

const createBody = `{
	"name": "WSO2 Motor Rally 2027",
	"eventDate": "2027-02-13",
	"startTime": "09:00",
	"start": {"label":"Start","lat":6.8901,"lng":79.92,"radiusM":40},
	"end": {"label":"Pearl Bay","lat":6.848,"lng":79.928,"radiusM":30},
	"cipher": "API Integration"
}`

func TestHandler_Create(t *testing.T) {
	h, _ := newTestHandler(t)

	rr := do(t, h, http.MethodPost, "/", createBody)

	require.Equal(t, http.StatusCreated, rr.Code)
	var got EventDTO
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	require.Len(t, got.ID, 32)
	require.Equal(t, "setup", got.Status)
	require.Equal(t, "2027-02-13", got.EventDate)
	require.Equal(t, "organizer@wso2.com", got.CreatedBy, "the creator comes from the token, not the body")
}

func TestHandler_Create_RejectsMalformedJSON(t *testing.T) {
	h, _ := newTestHandler(t)

	rr := do(t, h, http.MethodPost, "/", `{"name":`)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.NotEmpty(t, messageOf(t, rr))
}

func TestHandler_Create_RejectsUnknownField(t *testing.T) {
	h, _ := newTestHandler(t)

	rr := do(t, h, http.MethodPost, "/", `{"name":"R","createdBy":"attacker@example.com"}`)

	require.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandler_Create_ValidationIsExplained(t *testing.T) {
	h, _ := newTestHandler(t)

	rr := do(t, h, http.MethodPost, "/", `{"name":"","eventDate":"2027-02-13","startTime":"09:00"}`)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.Contains(t, messageOf(t, rr), "Name is required")
}

func TestHandler_Create_RejectsBadDate(t *testing.T) {
	h, _ := newTestHandler(t)

	rr := do(t, h, http.MethodPost, "/", `{"name":"R","eventDate":"13-02-2027","startTime":"09:00"}`)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.Contains(t, messageOf(t, rr), "YYYY-MM-DD")
}

func TestHandler_Get_Unknown404(t *testing.T) {
	h, _ := newTestHandler(t)

	rr := do(t, h, http.MethodGet, "/does-not-exist", "")

	require.Equal(t, http.StatusNotFound, rr.Code)
	require.NotEmpty(t, messageOf(t, rr))
}

func TestHandler_GetAfterCreate(t *testing.T) {
	h, _ := newTestHandler(t)
	created := createEvent(t, h)

	rr := do(t, h, http.MethodGet, "/"+created.ID, "")

	require.Equal(t, http.StatusOK, rr.Code)
	var got EventDTO
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	require.Equal(t, created.ID, got.ID)
}

func TestHandler_Patch(t *testing.T) {
	h, _ := newTestHandler(t)
	created := createEvent(t, h)

	rr := do(t, h, http.MethodPatch, "/"+created.ID, `{"name":"Renamed"}`)

	require.Equal(t, http.StatusOK, rr.Code)
	var got EventDTO
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	require.Equal(t, "Renamed", got.Name)
	require.Equal(t, created.StartTime, got.StartTime)
}

func TestHandler_Publish(t *testing.T) {
	h, _ := newTestHandler(t)
	created := createEvent(t, h)

	rr := do(t, h, http.MethodPost, "/"+created.ID+"/publish", "")

	require.Equal(t, http.StatusOK, rr.Code)
	var got EventDTO
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	require.Equal(t, "active", got.Status)
}

func TestHandler_Search(t *testing.T) {
	h, _ := newTestHandler(t)
	createEvent(t, h)

	rr := do(t, h, http.MethodPost, "/search", `{"offset":0,"limit":10,"filters":{"status":"setup"}}`)

	require.Equal(t, http.StatusOK, rr.Code)
	var got struct {
		Items      []EventDTO `json:"items"`
		TotalCount int        `json:"totalCount"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	require.Equal(t, 1, got.TotalCount)
	require.Len(t, got.Items, 1)
}

func TestHandler_Search_UnknownStatusIs400(t *testing.T) {
	h, _ := newTestHandler(t)

	rr := do(t, h, http.MethodPost, "/search", `{"filters":{"status":"archived"}}`)

	require.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandler_Search_EmptyResultIsAnArray(t *testing.T) {
	h, _ := newTestHandler(t)

	rr := do(t, h, http.MethodPost, "/search", `{}`)

	require.Equal(t, http.StatusOK, rr.Code)
	require.JSONEq(t, `{"items":[],"totalCount":0}`, rr.Body.String())
}

func createEvent(t *testing.T, h http.Handler) EventDTO {
	t.Helper()

	rr := do(t, h, http.MethodPost, "/", createBody)
	require.Equal(t, http.StatusCreated, rr.Code)
	var created EventDTO
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &created))

	return created
}

func messageOf(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()

	var body struct {
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))

	return body.Message
}
