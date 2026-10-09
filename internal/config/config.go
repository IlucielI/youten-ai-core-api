package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppName  string
	AppEnv   string
	HTTPPort string
	Version  string
	GitHash  string

	// Database Configuration
	DBHost                string
	DBPort                string
	DBUser                string
	DBPass                string
	DBName                string
	DBSSLMode             string
	DBPoolMaxOpenConn     int
	DBPoolMaxIdleConn     int
	DBPoolMaxConnLifetime time.Duration
	DBPoolMaxConnIdleTime time.Duration
	AutoMigrate           bool

	// Redis Configuration
	RedisHost         string
	RedisPort         string
	RedisPassword     string
	RedisDB           int
	RedisPoolSize     int
	RedisDialTimeout  time.Duration
	RedisReadTimeout  time.Duration
	RedisWriteTimeout time.Duration

	// S3 Configuration
	S3Endpoint       string
	S3AccessKey      string
	S3SecretKey      string
	S3BucketName     string
	S3Region         string
	S3UseSSL         bool
	S3ForcePathStyle bool

	// RabbitMQ Configuration
	RabbitMQHost        string
	RabbitMQPort        string
	RabbitMQUser        string
	RabbitMQPassword    string
	RabbitMQVHost       string
	RabbitMQDialTimeout time.Duration

	// JWT Configuration
	JWTSecret            string
	JWTAccessExpiration  time.Duration
	JWTRefreshExpiration time.Duration

	// Basic Auth Configuration
	BasicAuthUsername string
	BasicAuthPassword string

	// Idempotency Configuration
	IdempotencyTTL time.Duration

	// CORS Configuration
	CORSAllowedOrigins   []string
	CORSAllowedMethods   []string
	CORSAllowedHeaders   []string
	CORSAllowCredentials bool
	CORSMaxAge           time.Duration

	// Rate Limiter Configuration
	RateLimiterEnabled bool
	RateLimiterLimit   float64
	RateLimiterBurst   int

	// Quota Configuration (< 0 means unlimited)
	GuestDailyQuota int
	UserDailyQuota  int

	// Request Body Limit Configuration
	MaxRequestBodySize int64

	// SMTP Configuration
	SMTPHost      string
	SMTPPort      string
	SMTPUsername  string
	SMTPPassword  string
	SMTPFromEmail string
	SMTPFromName  string
	SMTPUseTLS    bool
	SMTPUseSSL    bool

	// AI Services & OmniRoute Configuration
	LLMAPIKey             string
	LLMProvider           string
	LLMBaseURL            string
	LLMModel              string
	STTModel              string
	STTDiarizeModel       string
	EmbeddingModel        string
	EmbeddingDimension    int
	RAGChunkTokens        int
	RAGChunkOverlapTokens int
	RAGTopK               int

	// Meeting Voice Bot Configuration
	MeetingBotEnabled          bool
	DiscordBotEnabled          bool
	DiscordBotToken            string
	GoogleMeetBotEnabled       bool
	GoogleMeetBotWebhookSecret string
}

func Load() Config {
	return Config{
		AppName:  getEnv("APP_NAME", "code-base-golang"),
		AppEnv:   getEnv("APP_ENV", "development"),
		HTTPPort: getEnv("HTTP_PORT", "8080"),
		Version:  getEnv("APP_VERSION", "0.1.0"),
		GitHash:  getEnv("GIT_HASH", "dev"),

		// Database settings (POSTGRES_* matching official Docker image, with DB_* fallback)
		DBHost:                getEnvWithFallback("POSTGRES_HOST", "DB_HOST", "localhost"),
		DBPort:                getEnvWithFallback("POSTGRES_PORT", "DB_PORT", "5432"),
		DBUser:                getEnvWithFallback("POSTGRES_USER", "DB_USER", "postgres"),
		DBPass:                getEnvWithFallback("POSTGRES_PASSWORD", "DB_PASS", "postgres"),
		DBName:                getEnvWithFallback("POSTGRES_DB", "DB_NAME", "code_base_golang"),
		DBSSLMode:             getEnvWithFallback("POSTGRES_SSLMODE", "DB_SSLMODE", "disable"),
		DBPoolMaxOpenConn:     getEnvInt("DB_POOL_MAX_OPEN_CONN", 25),
		DBPoolMaxIdleConn:     getEnvInt("DB_POOL_MAX_IDLE_CONN", 10),
		DBPoolMaxConnLifetime: getEnvDuration("DB_POOL_MAX_CONN_LIFETIME", 30*time.Minute),
		DBPoolMaxConnIdleTime: getEnvDuration("DB_POOL_MAX_CONN_IDLE_TIME", 10*time.Minute),
		AutoMigrate:           getEnvWithFallbackBool("AUTO_MIGRATE", "DB_AUTO_MIGRATE", false),

		// Redis settings
		RedisHost:         getEnv("REDIS_HOST", "localhost"),
		RedisPort:         getEnv("REDIS_PORT", "6379"),
		RedisPassword:     getEnv("REDIS_PASSWORD", ""),
		RedisDB:           getEnvInt("REDIS_DB", 0),
		RedisPoolSize:     getEnvInt("REDIS_POOL_SIZE", 10),
		RedisDialTimeout:  getEnvDuration("REDIS_DIAL_TIMEOUT", 5*time.Second),
		RedisReadTimeout:  getEnvDuration("REDIS_READ_TIMEOUT", 3*time.Second),
		RedisWriteTimeout: getEnvDuration("REDIS_WRITE_TIMEOUT", 3*time.Second),

		// S3 settings (S3_* with MINIO_ROOT_* fallback for direct env_file compatibility)
		S3Endpoint:       getEnv("S3_ENDPOINT", "localhost:9000"),
		S3AccessKey:      getEnvWithFallback("S3_ACCESS_KEY", "MINIO_ROOT_USER", "minioadmin"),
		S3SecretKey:      getEnvWithFallback("S3_SECRET_KEY", "MINIO_ROOT_PASSWORD", "minioadmin"),
		S3BucketName:     getEnv("S3_BUCKET_NAME", "code-base-golang"),
		S3Region:         getEnv("S3_REGION", "us-east-1"),
		S3UseSSL:         getEnvBool("S3_USE_SSL", false),
		S3ForcePathStyle: getEnvBool("S3_FORCE_PATH_STYLE", true),

		// RabbitMQ settings
		RabbitMQHost:        getEnv("RABBITMQ_HOST", "localhost"),
		RabbitMQPort:        getEnv("RABBITMQ_PORT", "5672"),
		RabbitMQUser:        getEnv("RABBITMQ_USER", "guest"),
		RabbitMQPassword:    getEnv("RABBITMQ_PASSWORD", "guest"),
		RabbitMQVHost:       getEnv("RABBITMQ_VHOST", "/"),
		RabbitMQDialTimeout: getEnvDuration("RABBITMQ_DIAL_TIMEOUT", 10*time.Second),

		// JWT settings
		JWTSecret:            getEnv("JWT_SECRET", "code-base-golang-jwt-secret-key"),
		JWTAccessExpiration:  getEnvDuration("JWT_ACCESS_EXPIRATION", 24*time.Hour),
		JWTRefreshExpiration: getEnvDuration("JWT_REFRESH_EXPIRATION", 7*24*time.Hour),

		// Basic Auth settings
		BasicAuthUsername: getEnv("BASIC_AUTH_USERNAME", "client-app"),
		BasicAuthPassword: getEnv("BASIC_AUTH_PASSWORD", "supersecretclientkey"),

		// Idempotency settings (default 24 hours, supports 1s, 5s, 1m, 5m, 1h, 24h)
		IdempotencyTTL: getEnvDuration("IDEMPOTENCY_TTL", 24*time.Hour),

		// CORS settings
		CORSAllowedOrigins:   getEnvSlice("CORS_ALLOWED_ORIGINS", []string{"*"}),
		CORSAllowedMethods:   getEnvSlice("CORS_ALLOWED_METHODS", []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}),
		CORSAllowedHeaders:   getEnvSlice("CORS_ALLOWED_HEADERS", []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Request-ID", "Idempotency-Key"}),
		CORSAllowCredentials: getEnvBool("CORS_ALLOW_CREDENTIALS", true),
		CORSMaxAge:           getEnvDuration("CORS_MAX_AGE", 12*time.Hour),

		// Rate Limiter settings
		RateLimiterEnabled: getEnvBool("RATE_LIMITER_ENABLED", true),
		RateLimiterLimit:   getEnvFloat64("RATE_LIMITER_LIMIT", 20.0),
		RateLimiterBurst:   getEnvInt("RATE_LIMITER_BURST", 30),

		// Quota settings (default: constants fallback; negative value disables quota check)
		GuestDailyQuota: getEnvInt("GUEST_DAILY_QUOTA", 1),
		UserDailyQuota:  getEnvInt("USER_DAILY_QUOTA", 5),

		// Request Body Limit settings (default 2MB)
		MaxRequestBodySize: int64(getEnvInt("MAX_REQUEST_BODY_SIZE", 2*1024*1024)),

		// SMTP settings
		SMTPHost:      getEnv("SMTP_HOST", "localhost"),
		SMTPPort:      getEnv("SMTP_PORT", "1025"),
		SMTPUsername:  getEnv("SMTP_USERNAME", ""),
		SMTPPassword:  getEnv("SMTP_PASSWORD", ""),
		SMTPFromEmail: getEnv("SMTP_FROM_EMAIL", "noreply@example.com"),
		SMTPFromName:  getEnv("SMTP_FROM_NAME", "Application Notification"),
		SMTPUseTLS:    getEnvBool("SMTP_USE_TLS", false),
		SMTPUseSSL:    getEnvBool("SMTP_USE_SSL", false),

		// AI Services & OmniRoute settings
		LLMAPIKey:             getEnv("LLM_API_KEY", ""),
		LLMProvider:           getEnv("LLM_PROVIDER", "openai_compatible"),
		LLMBaseURL:            getEnv("LLM_BASE_URL", "http://localhost:20128/v1"),
		LLMModel:              getEnv("LLM_MODEL", "gpt-4o-mini"),
		STTModel:              getEnv("STT_MODEL", "whisper-1"),
		STTDiarizeModel:       getEnv("STT_DIARIZE_MODEL", ""),
		EmbeddingModel:        getEnv("EMBEDDING_MODEL", "cf/@cf/baai/bge-m3"),
		EmbeddingDimension:    getEnvInt("EMBEDDING_DIMENSION", 1024),
		RAGChunkTokens:        getEnvInt("RAG_CHUNK_TOKENS", 300),
		RAGChunkOverlapTokens: getEnvInt("RAG_CHUNK_OVERLAP_TOKENS", 50),
		RAGTopK:               getEnvInt("RAG_TOP_K", 5),

		// Meeting Voice Bot settings
		MeetingBotEnabled:          getEnvBool("MEETING_BOT_ENABLED", false),
		DiscordBotEnabled:          getEnvBool("DISCORD_BOT_ENABLED", false),
		DiscordBotToken:            getEnv("DISCORD_BOT_TOKEN", ""),
		GoogleMeetBotEnabled:       getEnvBool("GOOGLE_MEET_BOT_ENABLED", false),
		GoogleMeetBotWebhookSecret: getEnv("GOOGLE_MEET_BOT_WEBHOOK_SECRET", ""),
	}
}

// DSN constructs the PostgreSQL Data Source Name connection string.
func (c Config) DSN() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.DBHost, c.DBPort, c.DBUser, c.DBPass, c.DBName, c.DBSSLMode,
	)
}

// MigrationURI returns the postgres connection URL formatted for golang-migrate CLI / runner.
func (c Config) MigrationURI() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.DBUser, c.DBPass, c.DBHost, c.DBPort, c.DBName, c.DBSSLMode,
	)
}

// RedisAddr returns the formatted host:port address for Redis.
func (c Config) RedisAddr() string {
	return fmt.Sprintf("%s:%s", c.RedisHost, c.RedisPort)
}

// RabbitMQURL constructs the AMQP connection URL string (e.g. amqp://guest:guest@localhost:5672/).
func (c Config) RabbitMQURL() string {
	vhost := c.RabbitMQVHost
	if !strings.HasPrefix(vhost, "/") {
		vhost = "/" + vhost
	}
	return fmt.Sprintf("amqp://%s:%s@%s:%s%s",
		url.QueryEscape(c.RabbitMQUser),
		url.QueryEscape(c.RabbitMQPassword),
		c.RabbitMQHost,
		c.RabbitMQPort,
		vhost,
	)
}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getEnvWithFallback(primaryKey string, secondaryKey string, fallback string) string {
	if val := os.Getenv(primaryKey); val != "" {
		return val
	}
	if val := os.Getenv(secondaryKey); val != "" {
		return val
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	valStr := os.Getenv(key)
	if valStr == "" {
		return fallback
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return fallback
	}
	return val
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	valStr := os.Getenv(key)
	if valStr == "" {
		return fallback
	}
	val, err := time.ParseDuration(valStr)
	if err != nil || val <= 0 {
		return fallback
	}
	return val
}

func getEnvBool(key string, fallback bool) bool {
	valStr := os.Getenv(key)
	if valStr == "" {
		return fallback
	}
	val, err := strconv.ParseBool(valStr)
	if err != nil {
		return fallback
	}
	return val
}

func getEnvWithFallbackBool(primaryKey, secondaryKey string, fallback bool) bool {
	if val := os.Getenv(primaryKey); val != "" {
		parsed, err := strconv.ParseBool(val)
		if err == nil {
			return parsed
		}
	}
	if val := os.Getenv(secondaryKey); val != "" {
		parsed, err := strconv.ParseBool(val)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func getEnvFloat64(key string, fallback float64) float64 {
	valStr := os.Getenv(key)
	if valStr == "" {
		return fallback
	}
	val, err := strconv.ParseFloat(valStr, 64)
	if err != nil || val <= 0 {
		return fallback
	}
	return val
}

func getEnvSlice(key string, fallback []string) []string {
	valStr := os.Getenv(key)
	if valStr == "" {
		return fallback
	}
	parts := strings.Split(valStr, ",")
	res := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			res = append(res, trimmed)
		}
	}
	if len(res) == 0 {
		return fallback
	}
	return res
}
