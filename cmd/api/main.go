package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"code-base-golang/internal/adapters/audio"
	"code-base-golang/internal/adapters/database"
	"code-base-golang/internal/adapters/embedding"
	"code-base-golang/internal/adapters/llm"
	"code-base-golang/internal/adapters/rabbitmq"
	"code-base-golang/internal/adapters/redis"
	"code-base-golang/internal/adapters/s3"
	"code-base-golang/internal/adapters/smtp"
	"code-base-golang/internal/adapters/stt"
	"code-base-golang/internal/config"
	"code-base-golang/internal/controllers"
	"code-base-golang/internal/pkg/migration"
	"code-base-golang/internal/repositories"
	"code-base-golang/internal/routes"
	"code-base-golang/internal/services"
	"code-base-golang/internal/workers"
)

func main() {
	// Dispatch CLI migration commands if invoked as: go run cmd/api/main.go migrate <cmd>
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		cfg := config.Load()
		if err := migration.Run(cfg, os.Args[2:]); err != nil {
			log.Fatalf("migration failed: %v", err)
		}
		return
	}

	cfg := config.Load()

	// Initialize database adapter
	db, err := database.NewPostgres(cfg)
	if err != nil {
		if cfg.AppEnv == "production" {
			log.Fatalf("failed to connect to postgres: %v", err)
		}
		log.Printf("[WARN] postgres connection failed: %v", err)
	} else {
		log.Println("postgres adapter connected successfully")
		defer func() {
			if err := db.Close(); err != nil {
				log.Printf("error closing postgres connection: %v", err)
			}
		}()

		// Auto-run pending database migrations on startup if enabled via AUTO_MIGRATE=true
		if cfg.AutoMigrate {
			log.Println("running database auto-migration on startup...")
			if err := migration.Up(cfg, -1); err != nil {
				if cfg.AppEnv == "production" {
					log.Fatalf("database auto-migration failed: %v", err)
				}
				log.Printf("[WARN] database auto-migration failed: %v", err)
			} else {
				log.Println("database auto-migration finished successfully")
			}
		}
	}

	// Initialize redis adapter
	rdb, err := redis.New(cfg)
	if err != nil {
		if cfg.AppEnv == "production" {
			log.Fatalf("failed to connect to redis: %v", err)
		}
		log.Printf("[WARN] redis connection failed: %v", err)
	} else {
		log.Println("redis adapter connected successfully")
		defer func() {
			if err := rdb.Close(); err != nil {
				log.Printf("error closing redis connection: %v", err)
			}
		}()
	}

	// Initialize s3 storage adapter
	var storage services.FileStorage
	s3Adapter, err := s3.New(cfg)
	if err != nil {
		if cfg.AppEnv == "production" {
			log.Fatalf("failed to connect to s3: %v", err)
		}
		log.Printf("[WARN] s3 connection failed: %v", err)
	} else {
		storage = s3Adapter
		log.Println("s3 adapter connected successfully")
	}

	// Initialize rabbitmq adapter
	var broker *rabbitmq.RabbitMQ
	rmq, err := rabbitmq.New(cfg)
	if err != nil {
		if cfg.AppEnv == "production" {
			log.Fatalf("failed to connect to rabbitmq: %v", err)
		}
		log.Printf("[WARN] rabbitmq connection failed: %v", err)
	} else {
		broker = rmq
		log.Println("rabbitmq adapter connected successfully")
		defer func() {
			if err := rmq.Close(); err != nil {
				log.Printf("error closing rabbitmq connection: %v", err)
			}
		}()
	}

	// Initialize SMTP mailer adapter
	smtpAdapter := smtp.New(cfg)

	// Initialize AI & Media provider adapters
	sttAdapter := stt.NewOmniRoute(cfg)
	llmAdapter := llm.NewOmniRoute(cfg)
	embeddingAdapter := embedding.NewOmniRoute(cfg)

	var audioExtractor services.AudioExtractor = audio.NewFFmpeg()
	if !audio.NewFFmpeg().IsAvailable() {
		log.Println("[INFO] ffmpeg binary not found in PATH, using fallback audio extractor")
		audioExtractor = audio.NewMock()
	}

	repo := repositories.New(db.DB(), rdb)
	var publisher services.EventPublisher
	if broker != nil {
		publisher = broker
	}
	svc := services.New(cfg, repo, storage, publisher).
		WithMailer(smtpAdapter).
		WithSTT(sttAdapter).
		WithLLM(llmAdapter).
		WithEmbedding(embeddingAdapter).
		WithAudioExtractor(audioExtractor)
	ctrls := controllers.New(cfg, svc)
	router := routes.NewRouter(cfg, ctrls, svc)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Register worker subscription to Message Queue and background cleanup scheduler
	if broker != nil {
		workerServer := workers.New(cfg, svc, broker)
		workerServer.RegisterWorker()
		workerServer.StartCleanupTicker(ctx, 30*time.Minute)
	}

	httpServer := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("starting %s on port %s", cfg.AppName, cfg.HTTPPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server listen error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server forced to shutdown: %v", err)
	}

	log.Println("server exited")
}
