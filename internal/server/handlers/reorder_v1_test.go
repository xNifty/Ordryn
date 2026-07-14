package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"GoTodo/internal/server/utils"
)

func TestApplyPageWindowOrder(t *testing.T) {
	all := []int{1, 2, 3, 4, 5, 6}
	got := applyPageWindowOrder(all, []int{3, 1, 2}, 1, 3)
	want := []int{3, 1, 2, 4, 5, 6}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("page 1 = %v, want %v", got, want)
	}

	got = applyPageWindowOrder(all, []int{6, 4, 5}, 2, 3)
	want = []int{1, 2, 3, 6, 4, 5}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("page 2 = %v, want %v", got, want)
	}

	got = applyPageWindowOrder(all, nil, 1, 3)
	if !reflect.DeepEqual(got, all) {
		t.Fatalf("empty order mutated list: %v", got)
	}
}

func TestAPIV1ReorderUnauthorized(t *testing.T) {
	body := bytes.NewBufferString(`{"task_ids":[1,2],"favorite":false}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/reorder", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	APIV1TasksRouter(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload["error"] != "unauthorized" {
		t.Fatalf("error = %q, want unauthorized", payload["error"])
	}
}

func TestAPIV1ReorderInvalidBody(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "missing task_ids", body: `{"favorite":false}`},
		{name: "empty task_ids", body: `{"task_ids":[],"favorite":false}`},
		{name: "missing favorite", body: `{"task_ids":[1,2]}`},
		{name: "invalid json", body: `{`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/reorder", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req = utils.SetAPIUserID(req, 1)
			rec := httptest.NewRecorder()

			APIV1TasksRouter(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

func TestAPIV1ReorderMethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/reorder", nil)
	req = utils.SetAPIUserID(req, 1)
	rec := httptest.NewRecorder()

	APIV1TasksRouter(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}
