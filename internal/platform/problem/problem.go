// Package problem writes RFC 9457 problem details (application/problem+json),
// the error format of the public API (PDF p.24). The PDF does not define
// problem type URIs (AMB-12), so "type" is left as "about:blank".
package problem

import (
	"encoding/json"
	"net/http"
)

// Write sends a problem response. ext adds extension members; it may be nil.
func Write(w http.ResponseWriter, status int, detail string, ext map[string]any) {
	body := map[string]any{
		"type":   "about:blank",
		"title":  http.StatusText(status),
		"status": status,
	}
	if detail != "" {
		body["detail"] = detail
	}
	for k, v := range ext {
		if _, reserved := body[k]; !reserved {
			body[k] = v
		}
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
