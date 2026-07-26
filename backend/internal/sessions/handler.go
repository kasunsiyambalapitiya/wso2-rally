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

package sessions

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/alerts"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/authz"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/httpx"
)

// Handler exposes the in-car REST surface.
type Handler struct {
	service *Service
	logger  *slog.Logger
}

// NewHandler wires a Handler to its service.
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

// RegisterPublic adds the one endpoint that runs before a crew has any
// credential: binding is what issues the team token.
func (h *Handler) RegisterPublic(r chi.Router) {
	r.Post("/sessions/bind", h.bind)
}

// RegisterTeam adds the endpoints a bound phone calls. Every one of them takes
// its session from the team token, never from the request, so a crew can only
// ever act as itself.
func (h *Handler) RegisterTeam(r chi.Router) {
	r.Get("/sessions/me", h.state)
	r.Post("/sessions/me/location", h.ping)
	r.Get("/sessions/me/tasks", h.listTasks)
	r.Post("/sessions/me/tasks/{taskId}/submit", h.submitTask)
	r.Post("/sessions/me/alerts", h.raiseAlert)
	r.Post("/sessions/me/finish", h.finish)
	r.Get("/sessions/me/vouchers", h.vouchers)
}

func (h *Handler) bind(w http.ResponseWriter, r *http.Request) {
	var req BindRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteBadRequest(w, err)
		return
	}

	session, token, err := h.service.Bind(r.Context(), BindInput{
		VehicleID:     req.VehicleID,
		CrewMemberIDs: req.CrewMemberIDs,
	})
	if err != nil {
		httpx.WriteDomainError(w, r, h.logger, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, BindResponse{
		TeamToken: token,
		Session:   toSessionDTO(session),
	})
}

func (h *Handler) state(w http.ResponseWriter, r *http.Request) {
	sessionID, ok := sessionIDFrom(w, r)
	if !ok {
		return
	}

	state, err := h.service.State(r.Context(), sessionID)
	if err != nil {
		httpx.WriteDomainError(w, r, h.logger, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, toStateDTO(state))
}

func (h *Handler) ping(w http.ResponseWriter, r *http.Request) {
	sessionID, ok := sessionIDFrom(w, r)
	if !ok {
		return
	}

	var req LocationRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteBadRequest(w, err)
		return
	}

	// Accuracy is accepted so the client need not special-case it, but nothing
	// is decided from it yet: a geofence call is made from the reported point.
	result, err := h.service.Ping(r.Context(), sessionID, LatLng{Lat: req.Lat, Lng: req.Lng})
	if err != nil {
		httpx.WriteDomainError(w, r, h.logger, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, toPingResponse(result))
}

func (h *Handler) listTasks(w http.ResponseWriter, r *http.Request) {
	sessionID, ok := sessionIDFrom(w, r)
	if !ok {
		return
	}

	states, err := h.service.ListTasks(r.Context(), sessionID)
	if err != nil {
		httpx.WriteDomainError(w, r, h.logger, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, toTaskStateDTOs(states))
}

// submitTask scores one attempt. The payload shape is per task type, so it is
// passed through to the engine untouched.
func (h *Handler) submitTask(w http.ResponseWriter, r *http.Request) {
	sessionID, ok := sessionIDFrom(w, r)
	if !ok {
		return
	}

	var payload json.RawMessage
	if err := httpx.DecodeJSON(r, &payload); err != nil {
		httpx.WriteBadRequest(w, err)
		return
	}

	result, err := h.service.SubmitTask(r.Context(), sessionID, chi.URLParam(r, "taskId"), payload)
	if err != nil {
		httpx.WriteDomainError(w, r, h.logger, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, SubmitResultDTO{
		Correct:       result.Correct,
		AwardedPoints: result.AwardedPoints,
		Detail:        result.Detail,
	})
}

func (h *Handler) raiseAlert(w http.ResponseWriter, r *http.Request) {
	sessionID, ok := sessionIDFrom(w, r)
	if !ok {
		return
	}

	var req CrewAlertRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteBadRequest(w, err)
		return
	}

	raised, err := h.service.RaiseCrewAlert(r.Context(), sessionID, CrewAlertInput{
		Type: req.Type,
		Note: req.Note,
		Lat:  req.Lat,
		Lng:  req.Lng,
	})
	if err != nil {
		httpx.WriteDomainError(w, r, h.logger, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, alerts.ToDTO(raised))
}

func (h *Handler) finish(w http.ResponseWriter, r *http.Request) {
	sessionID, ok := sessionIDFrom(w, r)
	if !ok {
		return
	}

	session, err := h.service.Finish(r.Context(), sessionID)
	if err != nil {
		httpx.WriteDomainError(w, r, h.logger, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, toSessionDTO(session))
}

func (h *Handler) vouchers(w http.ResponseWriter, r *http.Request) {
	sessionID, ok := sessionIDFrom(w, r)
	if !ok {
		return
	}

	voucher, err := h.service.Vouchers(r.Context(), sessionID)
	if err != nil {
		httpx.WriteDomainError(w, r, h.logger, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, toVoucherDTO(voucher))
}

// sessionIDFrom reads the caller's session from its team token. It writes the
// 401 itself and reports false, so handlers can return immediately.
func sessionIDFrom(w http.ResponseWriter, r *http.Request) (string, bool) {
	identity, ok := authz.IdentityFrom(r.Context())
	if !ok || !identity.IsTeam() || identity.SessionID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.MsgUnauthorized)
		return "", false
	}

	return identity.SessionID, true
}
