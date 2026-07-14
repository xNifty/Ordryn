package handlers

import (
	"GoTodo/internal/server/utils"
	"GoTodo/internal/storage"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxSavedViewNameLength   = 80
	maxSavedViewSearchLength = 500
	maxSavedViewRequestBytes = 1 << 20
)

type savedViewAPIResponse struct {
	ID        int                     `json:"id"`
	Name      string                  `json:"name"`
	Filter    storage.SavedViewFilter `json:"filter"`
	SortOrder int                     `json:"sort_order"`
	CreatedAt time.Time               `json:"created_at"`
	UpdatedAt time.Time               `json:"updated_at"`
}

type savedViewAPIRequest struct {
	Name      *string                  `json:"name"`
	Filter    *storage.SavedViewFilter `json:"filter"`
	SortOrder *int                     `json:"sort_order"`
}

// APIV1SavedViewsRouter handles the saved-view collection and item endpoints.
func APIV1SavedViewsRouter(w http.ResponseWriter, r *http.Request) {
	userID, ok := savedViewAPIUser(w, r)
	if !ok {
		return
	}

	id, hasID, validPath := savedViewPathID(r)
	if !validPath {
		writeSavedViewAPIError(w, http.StatusBadRequest, "invalid_request", "Invalid saved view id.")
		return
	}

	if !hasID {
		switch r.Method {
		case http.MethodGet:
			apiV1ListSavedViews(w, userID)
		case http.MethodPost:
			apiV1CreateSavedView(w, r, userID)
		default:
			w.Header().Set("Allow", "GET, POST")
			writeSavedViewAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.")
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		apiV1GetSavedView(w, userID, id)
	case http.MethodPut, http.MethodPatch:
		apiV1UpdateSavedView(w, r, userID, id)
	case http.MethodDelete:
		apiV1DeleteSavedView(w, userID, id)
	default:
		w.Header().Set("Allow", "GET, PUT, PATCH, DELETE")
		writeSavedViewAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.")
	}
}

func apiV1ListSavedViews(w http.ResponseWriter, userID int) {
	views, err := storage.ListSavedViewsForUser(userID)
	if err != nil {
		writeSavedViewAPIError(w, http.StatusInternalServerError, "internal_error", "Failed to list saved views.")
		return
	}

	response := make([]savedViewAPIResponse, 0, len(views))
	for _, view := range views {
		response = append(response, savedViewToAPIResponse(view))
	}
	writeSavedViewAPIJSON(w, http.StatusOK, response)
}

func apiV1GetSavedView(w http.ResponseWriter, userID, id int) {
	view, err := storage.GetSavedViewByID(id, userID)
	if err != nil {
		writeSavedViewStorageError(w, err)
		return
	}
	writeSavedViewAPIJSON(w, http.StatusOK, savedViewToAPIResponse(*view))
}

func apiV1CreateSavedView(w http.ResponseWriter, r *http.Request, userID int) {
	var request savedViewAPIRequest
	if err := decodeSavedViewJSON(w, r, &request); err != nil {
		writeSavedViewAPIError(w, http.StatusBadRequest, "invalid_request", "Request body must be a single valid JSON object.")
		return
	}

	name, err := validateSavedViewName(request.Name)
	if err != nil {
		writeSavedViewAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	filter, err := validateSavedViewFilter(request.Filter)
	if err != nil {
		writeSavedViewAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if request.SortOrder != nil && *request.SortOrder < 0 {
		writeSavedViewAPIError(w, http.StatusBadRequest, "invalid_request", "sort_order must be zero or greater.")
		return
	}

	view, err := storage.CreateSavedView(userID, name, filter, request.SortOrder)
	if err != nil {
		writeSavedViewStorageError(w, err)
		return
	}

	basePath := strings.TrimSuffix(utils.GetBasePath(), "/")
	w.Header().Set("Location", basePath+"/api/v1/saved-views/"+strconv.Itoa(view.ID))
	writeSavedViewAPIJSON(w, http.StatusCreated, savedViewToAPIResponse(*view))
}

func apiV1UpdateSavedView(w http.ResponseWriter, r *http.Request, userID, id int) {
	var request savedViewAPIRequest
	if err := decodeSavedViewJSON(w, r, &request); err != nil {
		writeSavedViewAPIError(w, http.StatusBadRequest, "invalid_request", "Request body must be a single valid JSON object.")
		return
	}
	if request.Name == nil && request.Filter == nil && request.SortOrder == nil {
		writeSavedViewAPIError(w, http.StatusBadRequest, "invalid_request", "At least one field must be provided.")
		return
	}

	var name *string
	if request.Name != nil {
		validated, err := validateSavedViewName(request.Name)
		if err != nil {
			writeSavedViewAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		name = &validated
	}

	var filter *storage.SavedViewFilter
	if request.Filter != nil {
		validated, err := validateSavedViewFilter(request.Filter)
		if err != nil {
			writeSavedViewAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		filter = &validated
	}
	if request.SortOrder != nil && *request.SortOrder < 0 {
		writeSavedViewAPIError(w, http.StatusBadRequest, "invalid_request", "sort_order must be zero or greater.")
		return
	}

	view, err := storage.UpdateSavedView(id, userID, name, filter, request.SortOrder)
	if err != nil {
		writeSavedViewStorageError(w, err)
		return
	}
	writeSavedViewAPIJSON(w, http.StatusOK, savedViewToAPIResponse(*view))
}

func apiV1DeleteSavedView(w http.ResponseWriter, userID, id int) {
	if err := storage.DeleteSavedView(id, userID); err != nil {
		writeSavedViewStorageError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func savedViewAPIUser(w http.ResponseWriter, r *http.Request) (int, bool) {
	email, _, _, loggedIn := utils.GetSessionUser(r)
	userID := utils.GetSessionUserID(r)
	if !loggedIn || userID == nil {
		writeSavedViewAPIError(w, http.StatusUnauthorized, "unauthorized", "Authentication is required.")
		return 0, false
	}
	if email != "" {
		if banned, err := storage.IsUserBanned(email); err == nil && banned {
			writeSavedViewAPIError(w, http.StatusUnauthorized, "unauthorized", "Authentication is required.")
			return 0, false
		}
	}
	return *userID, true
}

func savedViewPathID(r *http.Request) (int, bool, bool) {
	path := r.URL.Path
	basePath := strings.TrimSuffix(utils.GetBasePath(), "/")
	if basePath != "" && basePath != "/" {
		if !strings.HasPrefix(path, basePath+"/") {
			return 0, false, false
		}
		path = strings.TrimPrefix(path, basePath)
	}

	const collectionPath = "/api/v1/saved-views"
	if path == collectionPath || path == collectionPath+"/" {
		return 0, false, true
	}
	if !strings.HasPrefix(path, collectionPath+"/") {
		return 0, false, false
	}
	subpath := strings.TrimPrefix(path, collectionPath+"/")
	if subpath == "" || strings.Contains(subpath, "/") {
		return 0, false, false
	}
	id, err := strconv.Atoi(subpath)
	if err != nil || id <= 0 {
		return 0, false, false
	}
	return id, true, true
}

func validateSavedViewName(name *string) (string, error) {
	if name == nil {
		return "", errors.New("name is required.")
	}
	trimmed := strings.TrimSpace(*name)
	if trimmed == "" {
		return "", errors.New("name is required.")
	}
	if utf8.RuneCountInString(trimmed) > maxSavedViewNameLength {
		return "", errors.New("name must be 80 characters or less.")
	}
	return trimmed, nil
}

func validateSavedViewFilter(filter *storage.SavedViewFilter) (storage.SavedViewFilter, error) {
	if filter == nil {
		return storage.SavedViewFilter{}, nil
	}

	normalized := storage.SavedViewFilter{
		Search: strings.TrimSpace(filter.Search),
	}
	if utf8.RuneCountInString(normalized.Search) > maxSavedViewSearchLength {
		return storage.SavedViewFilter{}, errors.New("filter.search must be 500 characters or less.")
	}

	project := strings.ToLower(strings.TrimSpace(filter.Project))
	switch project {
	case "":
	case "none", "0":
		normalized.Project = project
	default:
		id, err := strconv.Atoi(project)
		if err != nil || id <= 0 {
			return storage.SavedViewFilter{}, errors.New("filter.project must be a positive id, 0, or \"none\".")
		}
		normalized.Project = strconv.Itoa(id)
	}

	rawStatus := strings.TrimSpace(filter.Status)
	normalized.Status = normalizeStatusFilter(rawStatus)
	if rawStatus != "" && normalized.Status == "" {
		return storage.SavedViewFilter{}, errors.New("filter.status must be \"complete\" or \"incomplete\".")
	}

	rawDue := strings.TrimSpace(filter.Due)
	normalized.Due = normalizeDueFilter(rawDue)
	if rawDue != "" && normalized.Due == "" {
		return storage.SavedViewFilter{}, errors.New("filter.due must be \"overdue\", \"today\", \"week\", or \"none\".")
	}

	rawCompleted := strings.ToLower(strings.TrimSpace(filter.Completed))
	if rawCompleted != "" && rawCompleted != "week" {
		return storage.SavedViewFilter{}, errors.New("filter.completed must be \"week\".")
	}
	normalized.Completed = rawCompleted

	rawPriority := strings.TrimSpace(filter.Priority)
	normalized.Priority = normalizePriorityFilter(rawPriority)
	if rawPriority != "" && normalized.Priority == "" {
		return storage.SavedViewFilter{}, errors.New("filter.priority must be between 0 and 3.")
	}

	rawTag := strings.TrimSpace(filter.Tag)
	normalized.Tag = normalizeTagFilter(rawTag)
	if rawTag != "" && normalized.Tag == "" {
		return storage.SavedViewFilter{}, errors.New("filter.tag must be a positive id.")
	}

	rawSort := strings.TrimSpace(filter.Sort)
	normalized.Sort = normalizeSortFilter(rawSort)
	if rawSort != "" && normalized.Sort == "" {
		return storage.SavedViewFilter{}, errors.New("filter.sort must be \"priority\".")
	}
	return normalized, nil
}

func decodeSavedViewJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxSavedViewRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}

func savedViewToAPIResponse(view storage.SavedView) savedViewAPIResponse {
	return savedViewAPIResponse{
		ID:        view.ID,
		Name:      view.Name,
		Filter:    view.Filter,
		SortOrder: view.SortOrder,
		CreatedAt: view.CreatedAt,
		UpdatedAt: view.UpdatedAt,
	}
}

func writeSavedViewStorageError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storage.ErrSavedViewNotFound):
		writeSavedViewAPIError(w, http.StatusNotFound, "not_found", "Saved view not found.")
	case errors.Is(err, storage.ErrSavedViewLimit):
		writeSavedViewAPIError(w, http.StatusConflict, "limit_reached", "A maximum of 20 saved views is allowed.")
	case errors.Is(err, storage.ErrSavedViewNameConflict):
		writeSavedViewAPIError(w, http.StatusConflict, "name_conflict", "A saved view with this name already exists.")
	default:
		writeSavedViewAPIError(w, http.StatusInternalServerError, "internal_error", "Saved view operation failed.")
	}
}

func writeSavedViewAPIError(w http.ResponseWriter, status int, code, message string) {
	writeSavedViewAPIJSON(w, status, map[string]string{
		"error":   code,
		"message": message,
	})
}

func writeSavedViewAPIJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
