package handlers

import (
	"GoTodo/internal/sessionstore"
	"GoTodo/internal/storage"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAPIV1SavedViewsCRUD(t *testing.T) {
	startSavedViewsTestDatabase(t)

	unauthorized := performSavedViewAPIRequest(t, http.MethodGet, "/api/v1/saved-views", "", 0, "")
	assertSavedViewStatus(t, unauthorized, http.StatusUnauthorized)

	create := performSavedViewAPIRequest(t, http.MethodPost, "/api/v1/saved-views", `{
		"name": "  Overdue work  ",
		"filter": {
			"project": "2",
			"status": "incomplete",
			"due": "overdue",
			"completed": "",
			"priority": "2",
			"tag": "3",
			"sort": "priority",
			"search": "  release  "
		}
	}`, 1, "owner@example.com")
	assertSavedViewStatus(t, create, http.StatusCreated)

	var created savedViewAPIResponse
	decodeSavedViewResponse(t, create, &created)
	if created.ID <= 0 || created.Name != "Overdue work" {
		t.Fatalf("unexpected created view: %+v", created)
	}
	if created.Filter.Search != "release" || created.Filter.Due != "overdue" {
		t.Fatalf("filter was not normalized: %+v", created.Filter)
	}
	if location := create.Header().Get("Location"); location != fmt.Sprintf("/api/v1/saved-views/%d", created.ID) {
		t.Fatalf("unexpected Location header %q", location)
	}

	duplicate := performSavedViewAPIRequest(t, http.MethodPost, "/api/v1/saved-views",
		`{"name":"Overdue work","filter":{}}`, 1, "owner@example.com")
	assertSavedViewStatus(t, duplicate, http.StatusConflict)
	assertSavedViewErrorCode(t, duplicate, "name_conflict")

	list := performSavedViewAPIRequest(t, http.MethodGet, "/api/v1/saved-views", "", 1, "owner@example.com")
	assertSavedViewStatus(t, list, http.StatusOK)
	var views []savedViewAPIResponse
	decodeSavedViewResponse(t, list, &views)
	if len(views) != 1 || views[0].ID != created.ID {
		t.Fatalf("unexpected saved view list: %+v", views)
	}

	get := performSavedViewAPIRequest(t, http.MethodGet,
		fmt.Sprintf("/api/v1/saved-views/%d", created.ID), "", 1, "owner@example.com")
	assertSavedViewStatus(t, get, http.StatusOK)

	otherUserGet := performSavedViewAPIRequest(t, http.MethodGet,
		fmt.Sprintf("/api/v1/saved-views/%d", created.ID), "", 2, "other@example.com")
	assertSavedViewStatus(t, otherUserGet, http.StatusNotFound)

	update := performSavedViewAPIRequest(t, http.MethodPut,
		fmt.Sprintf("/api/v1/saved-views/%d", created.ID), `{
			"name": "Today",
			"filter": {
				"project": "none",
				"status": "completed",
				"due": "TODAY",
				"completed": "week",
				"priority": "3",
				"tag": "4",
				"sort": "PRIORITY",
				"search": ""
			},
			"sort_order": 4
		}`, 1, "owner@example.com")
	assertSavedViewStatus(t, update, http.StatusOK)

	var updated savedViewAPIResponse
	decodeSavedViewResponse(t, update, &updated)
	if updated.Name != "Today" || updated.Filter.Status != "complete" ||
		updated.Filter.Due != "today" || updated.Filter.Completed != "week" ||
		updated.Filter.Sort != "priority" || updated.SortOrder != 4 {
		t.Fatalf("unexpected updated view: %+v", updated)
	}
	if updated.UpdatedAt.Before(updated.CreatedAt) {
		t.Fatalf("updated_at predates created_at: %+v", updated)
	}

	invalid := performSavedViewAPIRequest(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/saved-views/%d", created.ID),
		`{"filter":{"priority":"9"}}`, 1, "owner@example.com")
	assertSavedViewStatus(t, invalid, http.StatusBadRequest)
	assertSavedViewErrorCode(t, invalid, "invalid_request")

	deleteResponse := performSavedViewAPIRequest(t, http.MethodDelete,
		fmt.Sprintf("/api/v1/saved-views/%d", created.ID), "", 1, "owner@example.com")
	assertSavedViewStatus(t, deleteResponse, http.StatusNoContent)
	if deleteResponse.Body.Len() != 0 {
		t.Fatalf("expected empty delete response, got %q", deleteResponse.Body.String())
	}

	deletedGet := performSavedViewAPIRequest(t, http.MethodGet,
		fmt.Sprintf("/api/v1/saved-views/%d", created.ID), "", 1, "owner@example.com")
	assertSavedViewStatus(t, deletedGet, http.StatusNotFound)
}

func TestValidateSavedViewFilterRejectsInvalidValues(t *testing.T) {
	cases := []struct {
		name   string
		filter storage.SavedViewFilter
	}{
		{name: "project", filter: storage.SavedViewFilter{Project: "-1"}},
		{name: "status", filter: storage.SavedViewFilter{Status: "open"}},
		{name: "due", filter: storage.SavedViewFilter{Due: "tomorrow"}},
		{name: "completed", filter: storage.SavedViewFilter{Completed: "month"}},
		{name: "priority", filter: storage.SavedViewFilter{Priority: "4"}},
		{name: "tag", filter: storage.SavedViewFilter{Tag: "zero"}},
		{name: "sort", filter: storage.SavedViewFilter{Sort: "name"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := validateSavedViewFilter(&testCase.filter); err == nil {
				t.Fatalf("expected %s filter to be rejected", testCase.name)
			}
		})
	}
}

func startSavedViewsTestDatabase(t *testing.T) {
	t.Helper()

	const port = uint32(55439)
	database := embeddedpostgres.NewDatabase(
		embeddedpostgres.DefaultConfig().Port(port).Database("saved_views_test"),
	)
	if err := database.Start(); err != nil {
		t.Fatalf("start embedded postgres: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Stop(); err != nil {
			t.Errorf("stop embedded postgres: %v", err)
		}
	})

	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", fmt.Sprintf("%d", port))
	t.Setenv("DB_USER", "postgres")
	t.Setenv("DB_PASSWORD", "postgres")
	t.Setenv("DB_NAME", "saved_views_test")

	pool, err := pgxpool.New(context.Background(),
		fmt.Sprintf("postgres://postgres:postgres@localhost:%d/saved_views_test?sslmode=disable", port))
	if err != nil {
		t.Fatalf("connect to embedded postgres: %v", err)
	}
	defer pool.Close()

	_, err = pool.Exec(context.Background(), `
		CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			email TEXT NOT NULL,
			is_banned BOOLEAN NOT NULL DEFAULT FALSE
		);
		INSERT INTO users (id, email) VALUES
			(1, 'owner@example.com'),
			(2, 'other@example.com');
	`)
	if err != nil {
		t.Fatalf("create users schema: %v", err)
	}
	if err := storage.CreateSavedViewsTable(); err != nil {
		t.Fatalf("create saved views schema: %v", err)
	}
}

func performSavedViewAPIRequest(
	t *testing.T,
	method string,
	path string,
	body string,
	userID int,
	email string,
) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if userID > 0 {
		session, err := sessionstore.Store.Get(request, "session")
		if err != nil {
			t.Fatalf("create session: %v", err)
		}
		session.Values["email"] = email
		session.Values["user_id"] = userID
		session.Values["role_id"] = 1
		session.Values["permissions"] = []string{}

		cookieRecorder := httptest.NewRecorder()
		if err := session.Save(request, cookieRecorder); err != nil {
			t.Fatalf("save session: %v", err)
		}
		for _, cookie := range cookieRecorder.Result().Cookies() {
			request.AddCookie(cookie)
		}
	}

	response := httptest.NewRecorder()
	APIV1SavedViewsRouter(response, request)
	return response
}

func assertSavedViewStatus(t *testing.T, response *httptest.ResponseRecorder, expected int) {
	t.Helper()
	if response.Code != expected {
		t.Fatalf("expected HTTP %d, got %d: %s", expected, response.Code, response.Body.String())
	}
}

func assertSavedViewErrorCode(t *testing.T, response *httptest.ResponseRecorder, expected string) {
	t.Helper()
	var payload map[string]string
	decodeSavedViewResponse(t, response, &payload)
	if payload["error"] != expected {
		t.Fatalf("expected error code %q, got %#v", expected, payload)
	}
}

func decodeSavedViewResponse(t *testing.T, response *httptest.ResponseRecorder, destination any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(response.Body.Bytes()))
	if err := decoder.Decode(destination); err != nil {
		t.Fatalf("decode response %q: %v", response.Body.String(), err)
	}
}

func TestMain(m *testing.M) {
	// sessionstore initializes before tests, so callers should still provide
	// SESSION_KEY. This fallback documents the required value for direct runs.
	if os.Getenv("SESSION_KEY") == "" {
		fmt.Fprintln(os.Stderr, "SESSION_KEY must be set before running handler tests")
		os.Exit(1)
	}
	os.Exit(m.Run())
}
