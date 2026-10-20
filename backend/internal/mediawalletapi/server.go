// Package mediawalletapi provides a private mTLS-only wallet boundary. It is not
// registered on the public router; identity mapping and media adapters are gates
// to enabling it, not optional fallbacks.
package mediawalletapi

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"regexp"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/mediasession"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const Version = "kitty-media-wallet-api-v1"
const Path = "/internal/media-wallet/v1/orders"
const maxBody = 160 << 10

var ErrSubjectNotFound = service.ErrMediaWalletSubjectNotFound
var identifier = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var subjectID = regexp.MustCompile(`^[A-Za-z0-9_-]{22,128}$`)
var money = regexp.MustCompile(`^(0|[1-9][0-9]{0,5})(\.[0-9]{1,8})?$`)

type Reserve struct {
	AmountUSD     string          `json:"amount_usd"`
	RequestHash   string          `json:"request_hash"`
	Quote         json.RawMessage `json:"quote"`
	PriceContract json.RawMessage `json:"price_contract"`
}

type Submission struct {
	State        string `json:"state"`
	UpstreamID   string `json:"upstream_id"`
	EvidenceHash string `json:"evidence_hash"`
}

type Settlement struct {
	ActualUSD    string `json:"actual_usd,omitempty"`
	EvidenceHash string `json:"evidence_hash"`
}

// Subject is a persistent opaque mapping owned by SUB. Never pass through a
// browser-supplied email, raw user ID, or New API user ID as this identity.
type Command struct {
	Version    string      `json:"version"`
	Operation  string      `json:"operation"`
	Subject    string      `json:"subject"`
	Session    string      `json:"session,omitempty"`
	ClientIP   string      `json:"client_ip,omitempty"`
	UserAgent  string      `json:"user_agent,omitempty"`
	OrderID    string      `json:"order_id"`
	Reserve    *Reserve    `json:"reserve,omitempty"`
	Submission *Submission `json:"submission,omitempty"`
	Settlement *Settlement `json:"settlement,omitempty"`
}

type Receipt struct {
	Subject         string          `json:"subject"`
	OrderID         string          `json:"order_id"`
	Fingerprint     string          `json:"fingerprint"`
	Currency        string          `json:"currency"`
	HoldUSD         string          `json:"hold_usd"`
	ActualUSD       string          `json:"actual_usd"`
	ReleasedUSD     string          `json:"released_usd"`
	ExpiredUSD      string          `json:"expired_usd"`
	BillingState    string          `json:"billing_state"`
	SubmissionState string          `json:"submission_state"`
	UpstreamID      string          `json:"upstream_id"`
	EvidenceHash    string          `json:"evidence_hash"`
	Quote           json.RawMessage `json:"quote"`
	PriceContract   json.RawMessage `json:"price_contract"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type Response struct {
	Version           string   `json:"version"`
	Order             *Receipt `json:"order,omitempty"`
	Created           bool     `json:"created"`
	SubmissionGranted bool     `json:"submission_granted"`
	Error             string   `json:"error,omitempty"`
}

type Config struct {
	Addr              string
	ServerCertificate tls.Certificate
	ClientCAs         *x509.CertPool
	AllowedClientSPKI [][32]byte
	MaxConcurrent     int
	// ResolveSubject must read an immutable server-owned mapping. There is
	// intentionally no default parser, email lookup, or auto-linking behavior.
	// Existing orders must remain resolvable after a user is disabled so that
	// refunds can complete; the repository denies new use by inactive users.
	ResolveSubject func(context.Context, string) (int64, error)
	// New holds/submissions require a live SUB-backed session. Nil is recovery-
	// only, never implicit permission to spend. Old orders remain service-only.
	Sessions SessionAuthority
}

func NewServer(cfg Config, repo service.MediaWalletRepository) (*http.Server, error) {
	if cfg.Addr == "" || repo == nil || cfg.ResolveSubject == nil || cfg.ClientCAs == nil ||
		len(cfg.ServerCertificate.Certificate) == 0 || cfg.ServerCertificate.PrivateKey == nil || len(cfg.AllowedClientSPKI) == 0 {
		return nil, errors.New("media wallet requires explicit listener, repository, identity resolver and mutual TLS credentials")
	}
	if cfg.MaxConcurrent == 0 {
		cfg.MaxConcurrent = 8
	}
	if cfg.MaxConcurrent < 1 || cfg.MaxConcurrent > 64 {
		return nil, errors.New("invalid media wallet concurrency limit")
	}
	pins := make(map[[32]byte]bool, len(cfg.AllowedClientSPKI))
	for _, pin := range cfg.AllowedClientSPKI {
		if pin == [32]byte{} {
			return nil, errors.New("invalid media wallet client public key pin")
		}
		pins[pin] = true
	}
	verify := func(cs tls.ConnectionState) error {
		if cs.Version < tls.VersionTLS13 || len(cs.VerifiedChains) == 0 || len(cs.PeerCertificates) == 0 ||
			!pins[sha256.Sum256(cs.PeerCertificates[0].RawSubjectPublicKeyInfo)] {
			return errors.New("media wallet client authentication failed")
		}
		now := time.Now()
		if now.Before(cs.PeerCertificates[0].NotBefore) || !now.Before(cs.PeerCertificates[0].NotAfter) {
			return errors.New("media wallet client authentication failed")
		}
		return nil
	}
	limit := make(chan struct{}, cfg.MaxConcurrent)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.TLS == nil || verify(*r.TLS) != nil {
			slog.Warn("media wallet request denied", "reason", "client_authentication")
			respond(w, http.StatusUnauthorized, Response{Error: "unauthorized"})
			return
		}
		// Do not turn this privileged listener into a cookie-authenticated browser
		// API or trust client-certificate assertions from a reverse proxy header.
		if r.Header.Get("Origin") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			respond(w, http.StatusForbidden, Response{Error: "service_only"})
			return
		}
		if r.URL.Path != Path && (r.URL.Path != SessionPath || cfg.Sessions == nil) || r.URL.RawQuery != "" || r.URL.ForceQuery {
			respond(w, http.StatusNotFound, Response{Error: "not_found"})
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			respond(w, http.StatusMethodNotAllowed, Response{Error: "method_not_allowed"})
			return
		}
		contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || contentType != "application/json" || r.Header.Get("Content-Encoding") != "" {
			respond(w, http.StatusUnsupportedMediaType, Response{Error: "unsupported_media_type"})
			return
		}
		select {
		case limit <- struct{}{}:
			defer func() { <-limit }()
		default:
			respond(w, http.StatusTooManyRequests, Response{Error: "busy"})
			return
		}
		if r.URL.Path == SessionPath {
			serveSession(w, r, cfg.Sessions)
			return
		}
		var cmd Command
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		err = decoder.Decode(&cmd)
		if err == nil {
			var trailing any
			if trailingErr := decoder.Decode(&trailing); trailingErr != io.EOF {
				err = trailingErr
				if err == nil {
					err = errors.New("trailing request data")
				}
			}
		}
		if err != nil || !validCommand(cmd) {
			var oversized *http.MaxBytesError
			if errors.As(err, &oversized) {
				respond(w, http.StatusRequestEntityTooLarge, Response{Error: "request_too_large"})
			} else {
				respond(w, http.StatusBadRequest, Response{Error: "invalid_request"})
			}
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		if cmd.Operation == "reserve" || cmd.Operation == "begin_submission" {
			if cfg.Sessions == nil {
				respond(w, http.StatusUnauthorized, Response{Error: "session_required"})
				return
			}
			binding, ok := sessionBinding(cmd.ClientIP, cmd.UserAgent)
			if !ok {
				respond(w, http.StatusUnauthorized, Response{Error: "session_required"})
				return
			}
			ctx = service.WithSessionBinding(ctx, binding)
			if _, err := cfg.Sessions.Check(ctx, cmd.Session, cmd.Subject); err != nil {
				status := http.StatusUnauthorized
				code := "session_required"
				if errors.Is(err, mediasession.ErrUnavailable) {
					status, code = http.StatusServiceUnavailable, "wallet_unavailable"
				}
				respond(w, status, Response{Error: code})
				return
			}
		}
		userID, err := cfg.ResolveSubject(ctx, cmd.Subject)
		if err != nil || userID <= 0 {
			status := http.StatusServiceUnavailable
			code := "wallet_unavailable"
			if errors.Is(err, ErrSubjectNotFound) || err == nil {
				status, code = http.StatusNotFound, "not_found"
			}
			slog.Warn("media wallet identity resolution denied", "status", status)
			respond(w, status, Response{Error: code})
			return
		}
		key := service.MediaWalletKey{UserID: userID, OrderID: cmd.OrderID}
		var order *service.MediaWalletOrder
		response := Response{}
		switch cmd.Operation {
		case "reserve":
			v := cmd.Reserve
			order, response.Created, err = repo.Reserve(ctx, service.MediaWalletReserve{Key: key, AmountUSD: v.AmountUSD, RequestHash: v.RequestHash, Quote: v.Quote, PriceContract: v.PriceContract})
		case "get":
			order, err = repo.Get(ctx, key)
		case "begin_submission":
			order, response.SubmissionGranted, err = repo.BeginSubmission(ctx, key)
		case "record_submission":
			v := cmd.Submission
			order, err = repo.RecordSubmission(ctx, key, v.State, v.UpstreamID, v.EvidenceHash)
		case "capture":
			order, err = repo.Capture(ctx, key, cmd.Settlement.ActualUSD, cmd.Settlement.EvidenceHash)
		case "release":
			order, err = repo.Release(ctx, key, cmd.Settlement.EvidenceHash)
		}
		status := http.StatusOK
		if err != nil {
			status, response = errorResponse(err)
		} else if order == nil || order.Key != key {
			status, response = http.StatusServiceUnavailable, Response{Error: "wallet_unavailable"}
		} else {
			response.Order = &Receipt{
				Subject: cmd.Subject, OrderID: key.OrderID, Fingerprint: order.Fingerprint, Currency: order.Currency,
				HoldUSD: order.HoldUSD, ActualUSD: order.ActualUSD, ReleasedUSD: order.ReleasedUSD, ExpiredUSD: order.ExpiredUSD,
				BillingState: order.BillingState, SubmissionState: order.SubmissionState, UpstreamID: order.UpstreamID,
				EvidenceHash: order.EvidenceHash, Quote: order.Quote, PriceContract: order.PriceContract, UpdatedAt: order.UpdatedAt,
			}
		}
		slog.Info("media wallet operation", "operation", cmd.Operation, "order_id", key.OrderID, "user_id", userID, "status", status)
		respond(w, status, response)
	})
	return &http.Server{
		Addr: cfg.Addr, Handler: handler,
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cfg.ServerCertificate},
			ClientCAs: cfg.ClientCAs.Clone(), ClientAuth: tls.RequireAndVerifyClientCert, VerifyConnection: verify},
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second,
		IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8 << 10,
	}, nil
}

func validHash(raw string) bool {
	decoded, err := hex.DecodeString(raw)
	return err == nil && len(raw) == 64 && len(decoded) == sha256.Size
}

func validCommand(cmd Command) bool {
	if cmd.Version != Version || !subjectID.MatchString(cmd.Subject) || !identifier.MatchString(cmd.OrderID) ||
		len(cmd.Session) > 128 || len(cmd.ClientIP) > 64 || len(cmd.UserAgent) > 512 {
		return false
	}
	switch cmd.Operation {
	case "reserve":
		v := cmd.Reserve
		return v != nil && cmd.Submission == nil && cmd.Settlement == nil && money.MatchString(v.AmountUSD) && validHash(v.RequestHash) &&
			len(v.Quote) > 0 && len(v.Quote) <= 64<<10 && len(v.PriceContract) > 0 && len(v.PriceContract) <= 64<<10
	case "get", "begin_submission":
		return cmd.Reserve == nil && cmd.Submission == nil && cmd.Settlement == nil
	case "record_submission":
		v := cmd.Submission
		return v != nil && cmd.Reserve == nil && cmd.Settlement == nil && validHash(v.EvidenceHash) && len(v.UpstreamID) <= 512 &&
			(v.State == "accepted" || v.State == "rejected" || v.State == "unknown") && (v.State != "unknown" || v.UpstreamID == "")
	case "capture", "release":
		v := cmd.Settlement
		return v != nil && cmd.Reserve == nil && cmd.Submission == nil && validHash(v.EvidenceHash) &&
			(cmd.Operation == "capture" && money.MatchString(v.ActualUSD) || cmd.Operation == "release" && v.ActualUSD == "")
	default:
		return false
	}
}

func errorResponse(err error) (int, Response) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return http.StatusNotFound, Response{Error: "not_found"}
	case errors.Is(err, service.ErrMediaWalletConflict):
		return http.StatusConflict, Response{Error: "order_conflict"}
	case errors.Is(err, service.ErrMediaWalletState):
		return http.StatusConflict, Response{Error: "invalid_order_state"}
	case errors.Is(err, service.ErrBatchImageInsufficientBalance):
		return http.StatusPaymentRequired, Response{Error: "insufficient_balance"}
	case errors.Is(err, service.ErrBatchImageSettlementCostExceedsHold):
		return http.StatusUnprocessableEntity, Response{Error: "actual_exceeds_hold"}
	default:
		return http.StatusServiceUnavailable, Response{Error: "wallet_unavailable"}
	}
}

func respond(w http.ResponseWriter, status int, response Response) {
	response.Version = Version
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}
