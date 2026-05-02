package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"

	"github.com/lingcdn/control/internal/buildinfo"
	"github.com/lingcdn/control/internal/cert"
	"github.com/lingcdn/control/internal/compiler"
	"github.com/lingcdn/control/internal/config"
	"github.com/lingcdn/control/internal/metrics"
	"github.com/lingcdn/control/internal/nodehub"
	"github.com/lingcdn/control/internal/publisher"
	"github.com/lingcdn/control/internal/purge"
	"github.com/lingcdn/control/internal/server"
	"github.com/lingcdn/control/internal/store"
)

// NOTE: the authoritative version lives in internal/buildinfo.appVersion
// and is set via -ldflags at build time. We only mirror it onto
// cobra.Command.Version so that `lingcdn-control --version` prints it.
// Do NOT read os.Getenv("APP_VERSION") here — see buildinfo.Version() for
// why that used to break upgrades.

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	root := &cobra.Command{Use: "lingcdn-control"}
	root.Version = buildinfo.Version()

	root.AddCommand(serveCmd(), migrateCmd(), seedCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func serveCmd() *cobra.Command {
	var configFile string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start control plane (gRPC + admin HTTP)",
		RunE: func(cmd *cobra.Command, args []string) error {
			loadResult, err := config.LoadDetailed(config.LoadOptions{File: configFile, AutoCreate: true})
			if err != nil {
				return err
			}
			cfg := loadResult.Config
			configureLogger(cfg)
			if loadResult.Generated {
				log.Info().
					Str("path", loadResult.File).
					Msg("generated config file")
			}

			log.Info().
				Str("grpc", cfg.GRPCAddr).
				Str("http", cfg.HTTPAddr).
				Str("metrics", cfg.MetricsAddr).
				Msg("starting control plane")

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			storeBackend := strings.ToLower(cfg.StoreBackend)
			var db store.Store
			var storeErr error

			switch storeBackend {
			case "postgres", "pg":
				db, storeErr = store.NewPostgres(ctx, cfg.DatabaseURL)
				if storeErr != nil {
					return storeErr
				}
				if err := db.Ping(ctx); err != nil {
					return err
				}
			case "memory", "mem":
				db = store.NewMemory(cfg.ServiceToken, cfg.AdminBootstrapToken)
				log.Info().Msg("using in-memory store (dev mode)")
			default:
				return fmt.Errorf("unknown store backend: %s", cfg.StoreBackend)
			}
			defer db.Close()

			if err := db.Migrate(ctx); err != nil {
				return err
			}

			rdb := store.NewNoopRedis()
			if strings.TrimSpace(cfg.RedisURL) != "" {
				if rr, err := store.NewRedis(ctx, cfg.RedisURL); err != nil {
					log.Warn().Err(err).Msg("failed to connect redis; cache/coordination disabled")
				} else {
					rdb = rr
					log.Info().Msg("redis connected")
				}
			}

			if err := ensureDefaultAdmin(cmd.Context(), db, cfg); err != nil {
				return err
			}

			if err := ensureDefaultProductGroup(cmd.Context(), db); err != nil {
				return err
			}

			hub := nodehub.New()
			comp := compiler.New(db)
			pub := publisher.New(hub, comp, db)
			purger := purge.New(hub, rdb)
			certMgr := cert.New()
			m := metrics.New()

			srv := server.New(cfg, hub, comp, pub, purger, certMgr, db, m)

			if err := srv.Serve(ctx); err != nil {
				return err
			}
			log.Info().Msg("control plane stopped")
			return nil
		},
	}
	addConfigFlag(cmd, &configFile)
	return cmd
}

func migrateCmd() *cobra.Command {
	var configFile string

	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run database migrations (stub)",
		RunE: func(cmd *cobra.Command, args []string) error {
			loadResult, err := config.LoadDetailed(config.LoadOptions{File: configFile, AutoCreate: true})
			if err != nil {
				return err
			}
			cfg := loadResult.Config
			configureLogger(cfg)
			if loadResult.Generated {
				log.Info().
					Str("path", loadResult.File).
					Msg("generated config file")
			}

			ctx := context.Background()
			switch strings.ToLower(cfg.StoreBackend) {
			case "postgres", "pg":
				db, err := store.NewPostgres(ctx, cfg.DatabaseURL)
				if err != nil {
					return err
				}
				defer db.Close()
				return db.Migrate(ctx)
			case "memory", "mem":
				log.Info().Msg("memory store selected; migrate skipped")
				return nil
			default:
				return fmt.Errorf("unknown store backend: %s", cfg.StoreBackend)
			}

		},
	}
	addConfigFlag(cmd, &configFile)
	return cmd
}

func seedCmd() *cobra.Command {
	var configFile string

	cmd := &cobra.Command{
		Use:   "seed",
		Short: "Seed admin/service tokens (stub)",
		RunE: func(cmd *cobra.Command, args []string) error {
			loadResult, err := config.LoadDetailed(config.LoadOptions{File: configFile, AutoCreate: true})
			if err != nil {
				return err
			}
			cfg := loadResult.Config
			configureLogger(cfg)
			if loadResult.Generated {
				log.Info().
					Str("path", loadResult.File).
					Msg("generated config file")
			}

			ctx := context.Background()
			switch strings.ToLower(cfg.StoreBackend) {
			case "postgres", "pg":
				db, err := store.NewPostgres(ctx, cfg.DatabaseURL)
				if err != nil {
					return err
				}
				defer db.Close()
				return db.Seed(ctx)
			case "memory", "mem":
				log.Info().Msg("memory store selected; seed skipped")
				return nil
			default:
				return fmt.Errorf("unknown store backend: %s", cfg.StoreBackend)
			}
		},
	}
	addConfigFlag(cmd, &configFile)
	return cmd
}

func addConfigFlag(cmd *cobra.Command, target *string) {
	cmd.Flags().StringVar(target, "config", "", "Path to a config file (yaml/yml/json). Auto-detects config.yaml when omitted.")
}

func configureLogger(cfg config.Config) {
	level, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	var output *os.File
	output = os.Stdout
	if cfg.LogFile != "" {
		if f, err := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640); err == nil {
			output = f
		} else {
			log.Error().Err(err).Msg("failed to open log file, using stdout")
		}
	}

	if cfg.LogFormat == "console" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: output, TimeFormat: time.RFC3339})
	} else {
		log.Logger = log.Output(output)
	}
}

func ensureDefaultAdmin(ctx context.Context, db store.Store, cfg config.Config) error {
	const defaultRole = "admin"
	if cfg.AdminEmail == "" || cfg.AdminPassword == "" {
		log.Warn().Msg("admin email/password not provided; skipping default admin creation")
		return nil
	}

	email := strings.ToLower(cfg.AdminEmail)
	username := strings.ToLower(cfg.AdminUsername)
	existing, err := db.GetUserByLogin(ctx, username)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user := &store.User{
		ID:           uuid.NewString(),
		Username:     username,
		Email:        email,
		PasswordHash: string(hash),
		Role:         defaultRole,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := db.CreateUser(ctx, user); err != nil {
		return err
	}

	log.Info().
		Str("email", email).
		Msg("default admin created")
	return nil
}

func ensureDefaultProductGroup(ctx context.Context, db store.Store) error {
	groups, err := db.ListProductGroups(ctx)
	if err != nil {
		return err
	}
	if len(groups) > 0 {
		return nil
	}

	g := &store.ProductGroup{
		ID:          uuid.NewString(),
		Name:        "默认分组",
		Sort:        1,
		Description: "默认套餐分组",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := db.CreateProductGroup(ctx, g); err != nil {
		return err
	}
	log.Info().Msg("default product group created")
	return nil
}
