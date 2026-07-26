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

package alerts

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/authz"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/httpx"
)

// Handler exposes the organizer alert surface. The crew's own reporting
// endpoint lives in the sessions domain, which resolves the vehicle from the
// team token.
type Handler struct {
	service *Service
	logger  *slog.Logger
}

// NewHandler wires a Handler to its service.
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

// Register adds the organizer alert endpoints to r.
func (h *Handler) Register(r chi.Router) {
	r.Post("/vehicles/{vehicleId}/alerts", h.raise)
	r.Patch("/alerts/{alertId}", h.resolve)
	r.Post("/events/{eventId}/alerts/search", h.search)
}

func (h *Handler) raise(w http.ResponseWriter, r *http.Request) {
	var req RaiseAlertRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteBadRequest(w, err)
		return
	}

	identity, _ := authz.IdentityFrom(r.Context())
	raisedBy := identity.Email
	if raisedBy == "" {
		raisedBy = identity.UserID
	}

	alert, err := h.service.Raise(r.Context(), RaiseAlertInput{
		VehicleID: chi.URLParam(r, "vehicleId"),
		Type:      Type(req.Type),
		Note:      req.Note,
		Source:    SourceOrganizer,
		RaisedBy:  raisedBy,
		Lat:       req.Lat,
		Lng:       req.Lng,
	})
	if err != nil {
		httpx.WriteDomainError(w, r, h.logger, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, ToDTO(alert))
}

// resolve closes an alert. PATCH carries no body: resolving is the only
// transition an alert supports.
func (h *Handler) resolve(w http.ResponseWriter, r *http.Request) {
	alert, err := h.service.Resolve(r.Context(), chi.URLParam(r, "alertId"))
	if err != nil {
		httpx.WriteDomainError(w, r, h.logger, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, ToDTO(alert))
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	var req SearchAlertsRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteBadRequest(w, err)
		return
	}

	found, total, err := h.service.Search(r.Context(),
		chi.URLParam(r, "eventId"),
		SearchFilter{OpenOnly: req.Filters.OpenOnly},
		httpx.NormalizePage(req.Offset, req.Limit),
	)
	if err != nil {
		httpx.WriteDomainError(w, r, h.logger, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, httpx.NewSearchResult(toDTOs(found), total))
}
