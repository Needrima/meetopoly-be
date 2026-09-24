package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	httpadapter "meetopoly-be/internal/adapters/http"
	wsadapter "meetopoly-be/internal/adapters/websocket"
	"meetopoly-be/internal/platform/config"
	mongoplatform "meetopoly-be/internal/platform/mongo"
	redisplatform "meetopoly-be/internal/platform/redis"
	gamerepo "meetopoly-be/internal/repository/game"
	locationrepo "meetopoly-be/internal/repository/location"
	sessionrepo "meetopoly-be/internal/repository/session"
	tablerepo "meetopoly-be/internal/repository/table"
	userrepo "meetopoly-be/internal/repository/user"
	verificationrepo "meetopoly-be/internal/repository/verification"
	"meetopoly-be/internal/services/auth"
	gamesvc "meetopoly-be/internal/services/game"
	"meetopoly-be/internal/services/health"
	locationsvc "meetopoly-be/internal/services/location"
	"meetopoly-be/internal/services/mail"
	tablesvc "meetopoly-be/internal/services/table"
	usersvc "meetopoly-be/internal/services/user"
)

func main() {
	cfg := config.Load()

	closeLog, err := cfg.SetupLogger()
	if err != nil {
		slog.Error("logging setup failed", "err", err)
		os.Exit(1)
	}
	defer func() { _ = closeLog() }()

	ctx := context.Background()

	mongoClient, err := mongoplatform.Connect(ctx, cfg.MongoURI, cfg.PingTimeout)
	if err != nil {
		slog.Error("mongo connect failed", "err", err)
		os.Exit(1)
	}
	defer func() {
		disconnectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = mongoClient.Disconnect(disconnectCtx)
	}()

	redisClient, err := redisplatform.Connect(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.PingTimeout)
	if err != nil {
		slog.Error("redis connect failed", "err", err)
		os.Exit(1)
	}
	defer func() { _ = redisClient.Close() }()

	db := mongoClient.Database(cfg.MongoDatabase)
	users := userrepo.NewMongoRepository(db)
	codes := verificationrepo.NewMongoRepository(db)
	locations := locationrepo.NewMongoRepository(db)
	tables := tablerepo.NewMongoRepository(db)
	repoGames := gamerepo.NewMongoRepository(db)
	sessions := sessionrepo.NewRedisRepository(redisClient)

	indexCtx, indexCancel := context.WithTimeout(ctx, 10*time.Second)
	defer indexCancel()
	if err := users.EnsureIndexes(indexCtx); err != nil {
		slog.Error("user indexes failed", "err", err)
		os.Exit(1)
	}
	if err := codes.EnsureIndexes(indexCtx); err != nil {
		slog.Error("verification indexes failed", "err", err)
		os.Exit(1)
	}
	if err := locations.EnsureIndexes(indexCtx); err != nil {
		slog.Error("location indexes failed", "err", err)
		os.Exit(1)
	}
	if err := tables.EnsureIndexes(indexCtx); err != nil {
		slog.Error("table indexes failed", "err", err)
		os.Exit(1)
	}
	if err := repoGames.EnsureIndexes(indexCtx); err != nil {
		slog.Error("game indexes failed", "err", err)
		os.Exit(1)
	}

	mailer := mail.New(mail.Config{
		Host: cfg.SMTPHost,
		Port: cfg.SMTPPort,
		User: cfg.SMTPUser,
		Pass: cfg.SMTPPass,
		From: cfg.SMTPFrom,
	})

	authSvc := auth.New(users, codes, sessions, mailer, auth.Config{
		SignupTokenTTL:      cfg.SignupTokenTTL,
		VerificationCodeTTL: cfg.VerificationCodeTTL,
	})
	userSvc := usersvc.New(users)
	locationSvc := locationsvc.New(locations)
	gameSvc := gamesvc.New(repoGames, gamesvc.NewLocationSpaceCatalog(locations), gamesvc.Config{
		DisconnectHold: cfg.GameDisconnectHold,
	})
	tableSvc := tablesvc.New(tables, tablesvc.Config{
		DisconnectHold: 45 * time.Second,
	})
	tableSvc.SetGameStarter(gamesvc.TableBridge{Games: gameSvc})
	resolveUser := httpadapter.ResolveWSUser(authSvc)
	tableWS := wsadapter.NewHub(tableSvc, resolveUser)
	gameWS := wsadapter.NewGameHub(gameSvc, resolveUser)
	presenceWS := wsadapter.NewPresenceHub(gameSvc, resolveUser)
	healthSvc := health.New(
		health.NewMongoPinger(mongoClient),
		health.NewRedisPinger(redisClient),
		cfg.Version,
	)

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpadapter.NewRouter(httpadapter.Deps{
			Health:     healthSvc,
			Auth:       authSvc,
			Users:      userSvc,
			Locations:  locationSvc,
			Tables:     tableSvc,
			Games:      gameSvc,
			TableWS:    tableWS,
			GameWS:     gameWS,
			PresenceWS: presenceWS,
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("meetopoly-be listening",
			"addr", cfg.HTTPAddr,
			"mongo", cfg.MongoURI,
			"redis", cfg.RedisAddr,
			"log_file", cfg.LogFile,
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server failed", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "err", err)
	}
}
