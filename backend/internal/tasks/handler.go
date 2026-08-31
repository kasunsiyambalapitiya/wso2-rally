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
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/authz"
	"github.com/wso2-open-operations/wso2-motor-rally/backend/internal/httpx"
)

// Handler exposes the task-library REST surface.
type Handler struct {
	service *Service
	logger  *slog.Logger
}

// NewHandler wires a Handler to its service.
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

// Register adds the task endpoints to r.
//
// GET /tasks/{taskId} is also read by the micro app, which needs the
// definition to render a task body, so it is mounted for both identities.
func (h *Handler) Register(r chi.Router) {
	r.Post("/events/{eventId}/tasks", h.create)
	r.Post("/events/{eventId}/tasks/search", h.search)
	r.Patch("/tasks/{taskId}", h.update)
}

// RegisterShared adds the endpoints both organizers and crews may call. It is
// mounted once, above the role gates, because chi cannot carry the same path
// in two sibling groups.
func (h *Handler) RegisterShared(r chi.Router) {
	r.Get("/tasks/{taskId}", h.get)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateTaskRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteBadRequest(w, err)
		return
	}

	task, err := h.service.Create(r.Context(), req.toCreateInput(chi.URLParam(r, "eventId")))
	if err != nil {
		httpx.WriteDomainError(w, r, h.logger, err)
		return
	}

	httpx.WriteCreated(w, "/tasks/"+task.ID, toDTO(task))
}

// get returns a task definition. Crews read the same endpoint to render a task
// body, so their copy has the answers stripped out — otherwise the cipher, the
// grid solution, and the barcode payload would all be one request away.
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	task, err := h.service.Get(r.Context(), chi.URLParam(r, "taskId"))
	if err != nil {
		httpx.WriteDomainError(w, r, h.logger, err)
		return
	}

	dto := toDTO(task)
	if identity, ok := authz.IdentityFrom(r.Context()); ok && identity.IsTeam() {
		dto.Config = RedactForCrew(dto.Config)
	}

	httpx.WriteJSON(w, http.StatusOK, dto)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var req UpdateTaskRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteBadRequest(w, err)
		return
	}

	task, err := h.service.Update(r.Context(), chi.URLParam(r, "taskId"), req.toUpdateInput())
	if err != nil {
		httpx.WriteDomainError(w, r, h.logger, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, toDTO(task))
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	var req SearchTasksRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteBadRequest(w, err)
		return
	}

	found, total, err := h.service.Search(r.Context(),
		chi.URLParam(r, "eventId"), httpx.NormalizePage(req.Offset, req.Limit))
	if err != nil {
		httpx.WriteDomainError(w, r, h.logger, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, httpx.NewSearchResult(toDTOs(found), total))
}
