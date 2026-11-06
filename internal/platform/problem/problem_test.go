package problem

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWrite(t *testing.T) {
	rec := httptest.NewRecorder()
	Write(rec, http.StatusConflict, "seats taken", map[string]any{"unavailable_seat_ids": []string{"a"}, "status": 999})
	if rec.Code != http.StatusConflict || rec.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("code=%d type=%q", rec.Code, rec.Header().Get("Content-Type"))
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["title"] != "Conflict" || body["status"] != float64(409) || body["detail"] != "seats taken" || body["type"] != "about:blank" {
		t.Fatalf("body = %v", body)
	}
	if ids, ok := body["unavailable_seat_ids"].([]any); !ok || len(ids) != 1 {
		t.Fatalf("extension missing: %v", body)
	}
}
