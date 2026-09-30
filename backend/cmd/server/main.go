package main

//go:generate go run -mod=mod github.com/google/wire/cmd/wire

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/managedinstance"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/mihomo"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/server"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/setup"
	"github.com/Wei-Shaw/sub2api/internal/web"
	_ "github.com/lib/pq"

	"github.com/gin-gonic/gin"
)

//go:embed VERSION
var embeddedVersion string

// Build-time variables (can be set by ldflags)
var (
	Version   = ""
	Commit    = "unknown"
	Date      = "unknown"
	BuildType = "source" // "source" for manual builds, "release" for CI builds (set by ldflags)
)

func init() {
	// 如果 Version 已通过 ldflags 注入（例如 -X main.Version=...），则不要覆盖。
	if strings.TrimSpace(Version) != "" {
		return
	}

	// 默认从 embedded VERSION 文件读取版本号（编译期打包进二进制）。
	Version = strings.TrimSpace(embeddedVersion)
	if Version == "" {
		Version = "0.0.0-dev"
	}
}

// initLogger configures the default slog handler based on gin.Mode().
// In non-release mode, Debug level logs are enabled.
func main() {
	logger.InitBootstrap()
	defer logger.Sync()

	// Parse command line flags
	setupMode := flag.Bool("setup", false, "Run setup wizard in CLI mode")
	migrateOnly := flag.Bool("migrate-only", false, "Apply database migrations and exit")
	showVersion := flag.Bool("version", false, "Show version information")
	migrateMihomo := flag.String("migrate-mihomo", "", "Stage legacy Mihomo into the specified Sub2API data directory (root deployment only)")
	checkMihomo := flag.String("check-managed-mihomo", "", "Check managed Mihomo in the specified data directory")
	flag.Parse()
	if *checkMihomo != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := mihomo.CheckManaged(ctx, *checkMihomo); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *migrateMihomo != "" {
		if os.Geteuid() != 0 {
			log.Fatal("Mihomo migration requires root")
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := mihomo.PrepareLegacy(ctx, *migrateMihomo, "/etc/mihomo-codex/config.yaml", "/var/lib/mihomo-codex/providers/airport.yaml", "/usr/local/bin/mihomo"); err != nil {
			log.Fatalf("Mihomo migration failed: %v", err)
		}
		log.Print("Mihomo migration staged; original configuration retained")
		return
	}

	if *showVersion {
		log.Printf("Sub2API %s (commit: %s, built: %s)\n", Version, Commit, Date)
		return
	}
	if *setupMode && *migrateOnly {
		log.Fatal("--setup and --migrate-only cannot be used together")
	}
	if os.Getenv("KITTY_MANAGED_INSTANCE") == "true" && (*setupMode || *migrateOnly) {
		log.Fatal("Managed instances require an existing prepared database; setup and migrations run separately")
	}
	if *migrateOnly {
		if err := runMigrationsOnly(); err != nil {
			log.Fatalf("Database migration failed: %v", err)
		}
		log.Println("Database migrations completed")
		return
	}

	// CLI setup mode
	if *setupMode {
		if err := setup.RunCLI(); err != nil {
			log.Fatalf("Setup failed: %v", err)
		}
		return
	}

	// Check if setup is needed
	if setup.NeedsSetup() {
		if os.Getenv("KITTY_MANAGED_INSTANCE") == "true" {
			log.Fatal("Managed instance requires the existing installation configuration; automatic setup is disabled")
		}
		// Check if auto-setup is enabled (for Docker deployment)
		if setup.AutoSetupEnabled() {
			log.Println("Auto setup mode enabled...")
			if err := setup.AutoSetupFromEnv(); err != nil {
				log.Fatalf("Auto setup failed: %v", err)
			}
			// Continue to main server after auto-setup
		} else {
			log.Println("First run detected, starting setup wizard...")
			runSetupServer()
			return
		}
	}

	// Normal server mode
	runMainServer()
}

func runMigrationsOnly() error {
	return runMigrationsOnlyWith(config.LoadForBootstrap, sql.Open, repository.ApplyMigrations)
}

func runMigrationsOnlyWith(
	loadConfig func() (*config.Config, error),
	openDatabase func(driverName, dataSourceName string) (*sql.DB, error),
	applyMigrations func(context.Context, *sql.DB) error,
) error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("load bootstrap config: %w", err)
	}
	if cfg == nil {
		return errors.New("load bootstrap config: empty config")
	}
	db, err := openDatabase("postgres", cfg.Database.DSNWithTimezone(cfg.Timezone))
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := applyMigrations(ctx, db); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

func runSetupServer() {
	r := gin.New()
	r.Use(middleware.Recovery())
	r.Use(middleware.CORS(config.CORSConfig{}))
	r.Use(middleware.SecurityHeaders(config.CSPConfig{Enabled: true, Policy: config.DefaultCSPPolicy}, nil))

	// Register setup routes
	setup.RegisterRoutes(r)

	// Serve embedded frontend if available
	if web.HasEmbeddedFrontend() {
		r.Use(web.ServeEmbeddedFrontend())
	}

	// Get server address from config.yaml or environment variables (SERVER_HOST, SERVER_PORT)
	// This allows users to run setup on a different address if needed
	addr := config.GetServerAddress()
	log.Printf("Setup wizard available at http://%s", addr)
	log.Println("Complete the setup wizard to configure Sub2API")

	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	server := &http.Server{
		Addr:              addr,
		Handler:           server.SetupHandler(r),
		ReadHeaderTimeout: 30 * time.Second,
		IdleTimeout:       120 * time.Second,
		Protocols:         protocols,
	}

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Failed to start setup server: %v", err)
	}
}

func runMainServer() {
	cfg, err := config.LoadForBootstrap()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	if err := logger.Init(logger.OptionsFromConfig(cfg.Log)); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	if cfg.RunMode == config.RunModeSimple {
		log.Println("⚠️  WARNING: Running in SIMPLE mode - billing and quota checks are DISABLED")
	}

	buildInfo := handler.BuildInfo{
		Commit:    Commit,
		Version:   Version,
		BuildType: BuildType,
	}

	if err := managedinstance.ConfigureFromEnv(); err != nil {
		log.Fatalf("Managed instance configuration failed: %v", err)
	}
	if managedinstance.Enabled() && !cfg.RunsBackgroundJobs() {
		log.Fatal("managed handoff requires runtime.role=full; gateway replicas use the standard lifecycle")
	}
	app, err := initializeApplication(buildInfo)
	if err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}
	defer app.Cleanup()
	if app.PluginManager != nil {
		if managedinstance.Enabled() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := app.PluginManager.ValidateManagedStartup(ctx)
			cancel()
			if err != nil {
				log.Fatalf("Managed plugin compatibility check failed: %v", err)
			}
		}
		if err := app.PluginManager.Start(context.Background()); err != nil {
			log.Printf("Plugin manager started in degraded state: %v", err)
		}
	}
	if app.PromptAudit != nil {
		if err := app.PromptAudit.Start(context.Background()); err != nil {
			// Startup continues so unrelated APIs stay up. Fail-closed (unavailable)
			// applies only when a persisted blocking policy was observed; without
			// blocking intent, Prompt Audit stays ModeOff so the gateway remains
			// usable and administrators can still disable the feature (#4560).
			log.Printf("Prompt Audit started in degraded state: %v", err)
		}
	}

	// Bind before publishing readiness; a failed bind never appears ready.
	listener, err := net.Listen("tcp", app.Server.Addr)
	if err != nil {
		log.Fatalf("Listen failed: %v", err)
	}
	if instance := managedinstance.Current(); instance != nil {
		instance.MarkReady()
		go instance.Run(context.Background())
	}
	// 启动服务器
	go func() {
		if err := app.Server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	log.Printf("Server started on %s", app.Server.Addr)

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	app.Lifecycle.BeginDrain()
	if delay := time.Duration(cfg.Server.ShutdownDrainDelay) * time.Second; delay > 0 {
		time.Sleep(delay)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout())
	defer cancel()
	if instance := managedinstance.Current(); instance != nil {
		instance.Drain()
		ctx = context.Background() // A managed host never turns a deadline into SIGKILL.
	}

	if err := app.Server.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	if instance := managedinstance.Current(); instance != nil {
		_ = instance.WaitDrained(context.Background()) // includes hijacked WebSockets
	}
	if err := app.Lifecycle.Wait(ctx); err != nil {
		log.Printf("Active request drain deadline reached: %v", err)
	}
	if err := app.Server.Close(); err != nil {
		log.Printf("Closing server connections: %v", err)
	}
	log.Println("Server exited")
	mihomo.CloseAll()
}
