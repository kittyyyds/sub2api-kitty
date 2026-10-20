package mediawalletapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/mediasession"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const SessionPath = "/internal/media-wallet/v1/session"

type SessionAuthority interface {
	Exchange(context.Context, string, string, string, string, string) (*mediasession.Grant, error)
	Check(context.Context, string, string) (*mediasession.Identity, error)
	Balance(context.Context, string) (*mediasession.Identity, *service.MediaWalletBalance, error)
	Revoke(context.Context, string) error
}

type SessionCommand struct {
	KeyID       int64                         `json:"key_id,omitempty"`
	Version     string                        `json:"version"`
	Operation   string                        `json:"operation"`
	Code        string                        `json:"code,omitempty"`
	Verifier    string                        `json:"verifier,omitempty"`
	State       string                        `json:"state,omitempty"`
	Audience    string                        `json:"audience,omitempty"`
	CallbackURL string                        `json:"callback_url,omitempty"`
	Handle      string                        `json:"handle,omitempty"`
	Subject     string                        `json:"subject,omitempty"`
	ClientIP    string                        `json:"client_ip,omitempty"`
	UserAgent   string                        `json:"user_agent,omitempty"`
	GroupID     int64                         `json:"group_id,omitempty"`
	Model       string                        `json:"model,omitempty"`
	Models      []service.MediaModelCandidate `json:"models,omitempty"`
}

type SessionResponse struct {
	TextKeys  []service.MediaTextKey            `json:"text_keys,omitempty"`
	Version   string                            `json:"version"`
	Identity  *mediasession.Identity            `json:"identity,omitempty"`
	Handle    string                            `json:"handle,omitempty"`
	Balance   *service.MediaWalletBalance       `json:"balance,omitempty"`
	Error     string                            `json:"error,omitempty"`
	Assistant *service.MediaAssistantCredential `json:"assistant,omitempty"`
	Groups    []service.MediaPermissionGroup    `json:"groups,omitempty"`
}

// ClientIP and UserAgent come only from the media service's trusted request
// boundary, never a browser JSON field or untrusted forwarding header.
func sessionBinding(ip, ua string) (*service.SessionBinding, bool) {
	address, err := netip.ParseAddr(ip)
	if err != nil || address.Zone() != "" || ua == "" || len(ua) > 512 || strings.ContainsAny(ua, "\r\n\x00") {
		return nil, false
	}
	return &service.SessionBinding{IP: ip, UserAgent: ua}, true
}

func serveSession(w http.ResponseWriter, r *http.Request, authority SessionAuthority) {
	var cmd SessionCommand
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	err := d.Decode(&cmd)
	var trailing any
	if err != nil || d.Decode(&trailing) != io.EOF || !validSessionCommand(cmd) {
		writeSession(w, http.StatusBadRequest, SessionResponse{Error: "invalid_request"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if cmd.Operation != "revoke" {
		binding, ok := sessionBinding(cmd.ClientIP, cmd.UserAgent)
		if !ok {
			writeSession(w, http.StatusBadRequest, SessionResponse{Error: "invalid_request"})
			return
		}
		ctx = service.WithSessionBinding(ctx, binding)
	}
	result := SessionResponse{}
	switch cmd.Operation {
	case "source_logout":
		source, ok := authority.(interface {
			SourceLogout(context.Context, string, string) (*mediasession.Identity, error)
		})
		if !ok {
			err = mediasession.ErrUnavailable
		} else {
			result.Identity, err = source.SourceLogout(ctx, cmd.Handle, cmd.Subject)
		}
	case "text_keys":
		reader, ok := authority.(interface {
			TextKeys(context.Context, string, string, int64) (*mediasession.Identity, []service.MediaTextKey, error)
		})
		if !ok {
			err = mediasession.ErrUnavailable
		} else {
			result.Identity, result.TextKeys, err = reader.TextKeys(ctx, cmd.Handle, cmd.Subject, cmd.KeyID)
		}
	case "exchange":
		var grant *mediasession.Grant
		grant, err = authority.Exchange(ctx, cmd.Code, cmd.Verifier, cmd.State, cmd.Audience, cmd.CallbackURL)
		if err == nil {
			result.Identity, result.Handle = &grant.Identity, grant.Handle
		}
	case "check":
		result.Identity, err = authority.Check(ctx, cmd.Handle, cmd.Subject)
	case "balance":
		result.Identity, result.Balance, err = authority.Balance(ctx, cmd.Handle)
	case "revoke":
		err = authority.Revoke(ctx, cmd.Handle)
	case "assistant_key":
		reader, ok := authority.(interface {
			AssistantCredential(context.Context, string, string, int64, string) (*mediasession.Identity, *service.MediaAssistantCredential, error)
		})
		if !ok {
			err = mediasession.ErrUnavailable
		} else {
			result.Identity, result.Assistant, err = reader.AssistantCredential(ctx, cmd.Handle, cmd.Subject, cmd.GroupID, cmd.Model)
		}
	case "permissions":
		reader, ok := authority.(interface {
			Permissions(context.Context, string, string, []service.MediaModelCandidate) (*mediasession.Identity, []service.MediaPermissionGroup, error)
		})
		if !ok {
			err = mediasession.ErrUnavailable
		} else {
			result.Identity, result.Groups, err = reader.Permissions(ctx, cmd.Handle, cmd.Subject, cmd.Models)
		}
	}
	status := http.StatusOK
	if err != nil {
		status, result = http.StatusUnauthorized, SessionResponse{Error: "session_required"}
		if errors.Is(err, mediasession.ErrUnavailable) {
			status, result = http.StatusServiceUnavailable, SessionResponse{Error: "unavailable"}
		}
	}
	writeSession(w, status, result)
}

func validSessionCommand(c SessionCommand) bool {
	if c.KeyID < 0 || c.Operation != "text_keys" && c.KeyID != 0 {
		return false
	}
	if c.Version != Version || len(c.ClientIP) > 64 || len(c.UserAgent) > 512 {
		return false
	}
	if c.Operation != "assistant_key" && (c.GroupID != 0 || c.Model != "") {
		return false
	}
	if c.Operation != "permissions" && len(c.Models) != 0 {
		return false
	}
	hasExchange := c.Code != "" || c.Verifier != "" || c.State != "" || c.Audience != "" || c.CallbackURL != ""
	switch c.Operation {
	case "exchange":
		return c.Handle == "" && c.Subject == "" && len(c.Code) == 43 && len(c.Verifier) >= 43 && len(c.Verifier) <= 128 &&
			len(c.State) == 43 && len(c.Audience) > 0 && len(c.Audience) <= 128 && len(c.CallbackURL) <= 512
	case "assistant_key":
		return !hasExchange && len(c.Handle) == 43 && len(c.Subject) == 43 && c.GroupID > 0 && c.Model != "" && len(c.Model) <= 512 && c.Model == strings.TrimSpace(c.Model) && !strings.ContainsAny(c.Model, "\r\n\x00")
	case "source_logout", "text_keys", "check":
		return !hasExchange && len(c.Handle) == 43 && len(c.Subject) == 43
	case "permissions":
		if hasExchange || len(c.Handle) != 43 || len(c.Subject) != 43 || len(c.Models) < 1 || len(c.Models) > 500 {
			return false
		}
		seen := map[string]bool{}
		for _, m := range c.Models {
			if m.Model == "" || len(m.Model) > 512 || m.Model != strings.TrimSpace(m.Model) || strings.ContainsAny(m.Model, "\r\n\x00") || (m.Kind != "image" && m.Kind != "video" && m.Kind != "text") || seen[m.Model] {
				return false
			}
			seen[m.Model] = true
		}
		return true
	case "balance":
		return !hasExchange && len(c.Handle) == 43 && c.Subject == ""
	case "revoke":
		return !hasExchange && len(c.Handle) == 43 && c.Subject == "" && c.ClientIP == "" && c.UserAgent == ""
	}
	return false
}

func writeSession(w http.ResponseWriter, status int, result SessionResponse) {
	result.Version = Version
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(result)
}
