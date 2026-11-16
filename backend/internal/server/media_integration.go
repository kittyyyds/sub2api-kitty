package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/mediasession"
	"github.com/Wei-Shaw/sub2api/internal/mediawalletapi"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type MediaIntegration struct {
	Authority             *mediasession.Authority
	server                *http.Server
	cache                 *redis.Client
	callback, mediaOrigin string
}

func ProvideMediaIntegration(cfg *config.Config, db *sql.DB, cache *redis.Client, auth *service.AuthService, billing *service.BillingCacheService, keys *service.APIKeyService) (*MediaIntegration, error) {
	if os.Getenv("KITTY_SUB_MEDIA_ENABLED") != "true" {
		return &MediaIntegration{}, nil
	}
	if cfg == nil || cfg.Runtime.Role == config.RuntimeRoleGateway || !cfg.Server.TrustedProxiesConfigured || db == nil || cache == nil || auth == nil || billing == nil {
		return nil, errors.New("media integration requires primary dependencies and an explicit trusted proxy policy")
	}
	addr := os.Getenv("KITTY_MEDIA_WALLET_LISTEN")
	host, port, err := net.SplitHostPort(addr)
	ip, ipErr := netip.ParseAddr(host)
	portNumber, portErr := strconv.Atoi(port)
	if err != nil || ipErr != nil || portErr != nil || portNumber < 1024 || portNumber > 65535 || ip.Zone() != "" || !ip.IsLoopback() && !ip.IsPrivate() {
		return nil, errors.New("media wallet listener must bind an explicit private or loopback address")
	}
	cert, err := tls.LoadX509KeyPair(os.Getenv("KITTY_MEDIA_WALLET_CERT_FILE"), os.Getenv("KITTY_MEDIA_WALLET_KEY_FILE"))
	if err != nil {
		return nil, errors.New("media wallet certificate unavailable")
	}
	ca, err := os.ReadFile(os.Getenv("KITTY_MEDIA_WALLET_CA_FILE"))
	pool := x509.NewCertPool()
	if err != nil || !pool.AppendCertsFromPEM(ca) {
		return nil, errors.New("media wallet client CA unavailable")
	}
	var pins [][32]byte
	for _, raw := range strings.Split(os.Getenv("KITTY_MEDIA_WALLET_CLIENT_SPKI"), ",") {
		pin, err := hex.DecodeString(strings.TrimSpace(raw))
		if err != nil || len(pin) != 32 {
			return nil, errors.New("invalid media wallet client SPKI")
		}
		pins = append(pins, [32]byte(pin))
	}
	callback := os.Getenv("KITTY_MEDIA_CALLBACK_URL")
	callbackURL, err := url.Parse(callback)
	if err != nil || callbackURL.Path != "/api/media-login/callback" {
		return nil, errors.New("invalid fixed media callback")
	}
	identities := repository.NewMediaWalletIdentityRepository(db)
	authority, err := mediasession.New(mediasession.Config{Issuer: os.Getenv("KITTY_SUB_ISSUER"), Audience: os.Getenv("KITTY_SUB_MEDIA_AUDIENCE"), CallbackURL: callback, Permissions: keys}, cache, auth, identities, repository.NewMediaWalletBalanceReader(db))
	if err != nil {
		return nil, err
	}
	server, err := mediawalletapi.NewServer(mediawalletapi.Config{Addr: addr, ServerCertificate: cert, ClientCAs: pool, AllowedClientSPKI: pins, ResolveSubject: identities.ResolveSubject, Sessions: authority}, repository.NewMediaWalletRepository(db))
	if err != nil {
		return nil, err
	}
	if err = billing.RequireMediaWalletBalanceAuthority(repository.NewMediaWalletBalanceReader(db)); err != nil {
		return nil, err
	}
	return &MediaIntegration{Authority: authority, server: server, cache: cache, callback: callback, mediaOrigin: callbackURL.Scheme + "://" + callbackURL.Host}, nil
}

func (m *MediaIntegration) mount(r *gin.Engine, lifecycle *Lifecycle, jwt middleware2.JWTAuthMiddleware, settings *service.SettingService) error {
	if m.Authority == nil {
		return nil
	}
	listener, err := net.Listen("tcp", m.server.Addr)
	if err != nil {
		return errors.New("media wallet private listener unavailable")
	}
	var healthy atomic.Bool
	healthy.Store(true)
	lifecycle.checks = append(lifecycle.checks, func(context.Context) error {
		if !healthy.Load() {
			return errors.New("media wallet listener unavailable")
		}
		return nil
	})
	lifecycle.onDrain = append(lifecycle.onDrain, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := m.server.Shutdown(ctx); err != nil {
			_ = m.server.Close()
		}
	})
	go func() {
		if err := m.server.ServeTLS(listener, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			healthy.Store(false)
			slog.Error("media wallet private listener stopped")
		}
	}()
	group := r.Group("/api/v1/media-login", gin.HandlerFunc(jwt), middleware2.BackendModeUserGuard(settings))
	group.GET("/config", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"enabled": true, "media_origin": m.mediaOrigin, "callback_url": m.callback}})
	})
	group.POST("/issue", func(c *gin.Context) {
		subject, ok := middleware2.GetAuthSubjectFromContext(c)
		if !ok || subject.UserID <= 0 {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		count, err := m.cache.Eval(ctx, `local n=redis.call('INCR',KEYS[1]); if n==1 then redis.call('EXPIRE',KEYS[1],60) end; return n`, []string{"kitty:media-issue:user:" + strconv.FormatInt(subject.UserID, 10)}).Int64()
		if err != nil {
			c.AbortWithStatus(http.StatusServiceUnavailable)
			return
		}
		if count > 10 {
			c.Header("Retry-After", "60")
			c.AbortWithStatus(http.StatusTooManyRequests)
			return
		}
		m.Authority.IssueHandler().ServeHTTP(c.Writer, c.Request.WithContext(ctx))
	})
	return nil
}
