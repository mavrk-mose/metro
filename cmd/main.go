package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
)

func main() {
	ctx := context.Background()

	// Load config from env
	cfg := LoadConfig()

	// Initialize PostgreSQL
	pgPool, err := initPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to init Postgres: %v", err)
	}
	defer pgPool.Close()

	// Initialize Redis
	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.RedisAddr,
	})
	defer rdb.Close()

	// Test Redis connection
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	// Initialize NATS
	nc, err := initNATS(cfg.NATSUrl)
	if err != nil {
		log.Fatalf("Failed to init NATS: %v", err)
	}
	defer nc.Close()

	// Load subway graph from Postgres
	graph, err := LoadSubwayGraph(ctx, pgPool)
	if err != nil {
		log.Fatalf("Failed to load subway graph: %v", err)
	}
	log.Printf("Loaded subway graph: %d stations", len(graph.stations))

	// Start ETA worker pool
	workerPool := NewETAWorkerPool(nc, rdb, graph, cfg.WorkerCount)
	if err := workerPool.Start(ctx); err != nil {
		log.Fatalf("Failed to start worker pool: %v", err)
	}

	// Create server
	server := NewServer(pgPool, rdb, nc, graph, cfg)

	// Start Gin HTTP server
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()

	// Middleware
	router.Use(CORSMiddleware())
	router.Use(LoggingMiddleware())
	router.Use(ErrorHandlingMiddleware())

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Routes
	server.RegisterRoutes(router)

	// Start server
	httpServer := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		log.Println("Shutting down server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("Shutdown error: %v", err)
		}
		nc.Close()
	}()

	log.Printf("Starting server on %s", cfg.ListenAddr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}

func initPostgres(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		return nil, err
	}

	return pool, nil
}

func initNATS(url string) (*nats.Conn, error) {
	return nats.Connect(url,
		nats.Name("seoul-metro-api"),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			log.Printf("NATS disconnected: %v", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			log.Printf("NATS reconnected to %s", nc.ConnectedUrl())
		}),
	)
}
