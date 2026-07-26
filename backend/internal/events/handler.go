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
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/authz"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/httpx"
)

// Handler exposes the events REST surface. Every route below is mounted under
// organizer authentication by the router.
type Handler struct {
	service *Service
	logger  *slog.Logger
}

// NewHandler wires a Handler to its service.
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

// Routes returns the events sub-router, mounted by the caller at /events.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.create)
	r.Post("/search", h.search)
	r.Get("/{eventId}", h.get)
	r.Patch("/{eventId}", h.update)
	r.Post("/{eventId}/publish", h.publish)

	return r
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateEventRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteBadRequest(w, err)
		return
	}

	// The creator is the authenticated organizer, never a client-supplied value.
	identity, _ := authz.IdentityFrom(r.Context())
	createdBy := identity.Email
	if createdBy == "" {
		createdBy = identity.UserID
	}

	input, err := req.toCreateInput(createdBy)
	if err != nil {
		httpx.WriteDomainError(w, r, h.logger, err)
		return
	}

	event, err := h.service.Create(r.Context(), input)
	if err != nil {
		httpx.WriteDomainError(w, r, h.logger, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, toDTO(event))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	event, err := h.service.Get(r.Context(), chi.URLParam(r, "eventId"))
	if err != nil {
		httpx.WriteDomainError(w, r, h.logger, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, toDTO(event))
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var req UpdateEventRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteBadRequest(w, err)
		return
	}

	input, err := req.toUpdateInput()
	if err != nil {
		httpx.WriteDomainError(w, r, h.logger, err)
		return
	}

	event, err := h.service.Update(r.Context(), chi.URLParam(r, "eventId"), input)
	if err != nil {
		httpx.WriteDomainError(w, r, h.logger, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, toDTO(event))
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	var req SearchEventsRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteBadRequest(w, err)
		return
	}

	page, filter := req.toPageAndFilter()
	found, total, err := h.service.Search(r.Context(), page, filter)
	if err != nil {
		httpx.WriteDomainError(w, r, h.logger, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, httpx.NewSearchResult(toDTOs(found), total))
}

func (h *Handler) publish(w http.ResponseWriter, r *http.Request) {
	event, err := h.service.Publish(r.Context(), chi.URLParam(r, "eventId"))
	if err != nil {
		httpx.WriteDomainError(w, r, h.logger, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, toDTO(event))
}
