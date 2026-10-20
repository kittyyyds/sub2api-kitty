package mediasession

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
)

// IssueHandler is deliberately unmounted. Mount only behind SUB's authenticated
// route guard, backend-mode guard, fail-closed rate limit and binding middleware.
// The same-origin UI supplies the existing post-MFA bearer token in the header.
func (a *Authority) IssueHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		// SameSite is not sufficient when both sites share a parent domain.
		if r.Header.Get("Origin") != a.cfg.Issuer || r.URL.RawQuery != "" || r.URL.ForceQuery ||
			r.Header.Get("Sec-Fetch-Site") != "" && r.Header.Get("Sec-Fetch-Site") != "same-origin" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" || r.Header.Get("Content-Encoding") != "" {
			w.WriteHeader(http.StatusUnsupportedMediaType)
			return
		}
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		var request struct {
			Challenge string `json:"challenge"`
			State     string `json:"state"`
		}
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		var trailing any
		if d.Decode(&request) != nil || d.Decode(&trailing) != io.EOF {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		code, err := a.Issue(r.Context(), parts[1], request.Challenge, request.State)
		if err != nil {
			status := http.StatusUnauthorized
			if errors.Is(err, ErrUnavailable) {
				status = http.StatusServiceUnavailable
			}
			w.WriteHeader(status)
			return
		}
		_ = json.NewEncoder(w).Encode(struct {
			Code int `json:"code"`
			Data struct {
				Code        string `json:"code"`
				CallbackURL string `json:"callback_url"`
			} `json:"data"`
		}{Data: struct {
			Code        string `json:"code"`
			CallbackURL string `json:"callback_url"`
		}{Code: code, CallbackURL: a.cfg.CallbackURL}})
	})
}
