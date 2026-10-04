package config_test

import (
	"testing"
	"time"

	"code-base-golang/internal/config"
)

func TestConfig_LoadDefaults(t *testing.T) {
	cfg := config.Load()

	if cfg.AppName != "code-base-golang" {
		t.Errorf("expected AppName 'code-base-golang', got %q", cfg.AppName)
	}
	if cfg.AppEnv != "development" {
		t.Errorf("expected AppEnv 'development', got %q", cfg.AppEnv)
	}
	if cfg.HTTPPort != "8080" {
		t.Errorf("expected HTTPPort '8080', got %q", cfg.HTTPPort)
	}
	if cfg.Version != "0.1.0" {
		t.Errorf("expected Version '0.1.0', got %q", cfg.Version)
	}
	if cfg.GitHash != "dev" {
		t.Errorf("expected GitHash 'dev', got %q", cfg.GitHash)
	}
	if cfg.DBHost == "" {
		t.Error("expected non-empty DBHost")
	}
	if cfg.DBPoolMaxOpenConn <= 0 {
		t.Errorf("expected positive DBPoolMaxOpenConn, got %d", cfg.DBPoolMaxOpenConn)
	}
	if cfg.DBPoolMaxConnLifetime <= 0 {
		t.Errorf("expected positive DBPoolMaxConnLifetime, got %v", cfg.DBPoolMaxConnLifetime)
	}
	if cfg.DBPoolMaxConnIdleTime != 10*time.Minute && cfg.DBPoolMaxConnIdleTime <= 0 {
		t.Errorf("expected valid DBPoolMaxConnIdleTime, got %v", cfg.DBPoolMaxConnIdleTime)
	}
	if cfg.RedisHost != "localhost" {
		t.Errorf("expected RedisHost 'localhost', got %q", cfg.RedisHost)
	}
	if cfg.RedisPort != "6379" {
		t.Errorf("expected RedisPort '6379', got %q", cfg.RedisPort)
	}
	if cfg.RedisDB != 0 {
		t.Errorf("expected RedisDB 0, got %d", cfg.RedisDB)
	}
	if cfg.RedisPoolSize != 10 {
		t.Errorf("expected RedisPoolSize 10, got %d", cfg.RedisPoolSize)
	}
	if cfg.RedisDialTimeout != 5*time.Second {
		t.Errorf("expected RedisDialTimeout 5s, got %v", cfg.RedisDialTimeout)
	}
	if cfg.S3Endpoint != "localhost:9000" {
		t.Errorf("expected S3Endpoint 'localhost:9000', got %q", cfg.S3Endpoint)
	}
	if cfg.S3AccessKey != "minioadmin" {
		t.Errorf("expected S3AccessKey 'minioadmin', got %q", cfg.S3AccessKey)
	}
	if cfg.S3SecretKey != "minioadmin" {
		t.Errorf("expected S3SecretKey 'minioadmin', got %q", cfg.S3SecretKey)
	}
	if cfg.S3BucketName != "code-base-golang" {
		t.Errorf("expected S3BucketName 'code-base-golang', got %q", cfg.S3BucketName)
	}
	if cfg.S3UseSSL != false {
		t.Errorf("expected S3UseSSL false, got %v", cfg.S3UseSSL)
	}
	if cfg.S3ForcePathStyle != true {
		t.Errorf("expected S3ForcePathStyle true, got %v", cfg.S3ForcePathStyle)
	}
	if cfg.IdempotencyTTL != 24*time.Hour {
		t.Errorf("expected IdempotencyTTL 24h, got %v", cfg.IdempotencyTTL)
	}
	if cfg.LLMAPIKey != "" {
		t.Errorf("expected empty LLMAPIKey, got %q", cfg.LLMAPIKey)
	}
	if cfg.LLMProvider != "openai_compatible" {
		t.Errorf("expected LLMProvider 'openai_compatible', got %q", cfg.LLMProvider)
	}
	if cfg.LLMBaseURL != "http://localhost:20128/v1" {
		t.Errorf("expected LLMBaseURL 'http://localhost:20128/v1', got %q", cfg.LLMBaseURL)
	}
	if cfg.LLMModel != "gpt-4o-mini" {
		t.Errorf("expected LLMModel 'gpt-4o-mini', got %q", cfg.LLMModel)
	}
	if cfg.STTModel != "whisper-1" {
		t.Errorf("expected STTModel 'whisper-1', got %q", cfg.STTModel)
	}
	if cfg.EmbeddingModel != "cf/@cf/baai/bge-m3" {
		t.Errorf("expected EmbeddingModel 'cf/@cf/baai/bge-m3', got %q", cfg.EmbeddingModel)
	}
	if cfg.EmbeddingDimension != 1024 {
		t.Errorf("expected EmbeddingDimension 1024, got %d", cfg.EmbeddingDimension)
	}
	if cfg.RAGChunkTokens != 300 {
		t.Errorf("expected RAGChunkTokens 300, got %d", cfg.RAGChunkTokens)
	}
	if cfg.RAGChunkOverlapTokens != 50 {
		t.Errorf("expected RAGChunkOverlapTokens 50, got %d", cfg.RAGChunkOverlapTokens)
	}
	if cfg.RAGTopK != 5 {
		t.Errorf("expected RAGTopK 5, got %d", cfg.RAGTopK)
	}
}

func TestConfig_CustomEnv(t *testing.T) {
	t.Setenv("APP_NAME", "custom-api")
	t.Setenv("APP_ENV", "production")
	t.Setenv("HTTP_PORT", "9000")
	t.Setenv("APP_VERSION", "1.0.0")
	t.Setenv("GIT_HASH", "commit123")
	t.Setenv("LLM_API_KEY", "sk-secret123")
	t.Setenv("LLM_PROVIDER", "custom_gateway")
	t.Setenv("LLM_BASE_URL", "https://api.omniroute.ai/v1")
	t.Setenv("LLM_MODEL", "gpt-4o")
	t.Setenv("STT_MODEL", "whisper-large-v3")
	t.Setenv("EMBEDDING_MODEL", "text-embedding-3-small")
	t.Setenv("EMBEDDING_DIMENSION", "1536")
	t.Setenv("RAG_CHUNK_TOKENS", "500")
	t.Setenv("RAG_CHUNK_OVERLAP_TOKENS", "100")
	t.Setenv("RAG_TOP_K", "10")

	cfg := config.Load()

	if cfg.AppName != "custom-api" {
		t.Errorf("expected AppName 'custom-api', got %q", cfg.AppName)
	}
	if cfg.AppEnv != "production" {
		t.Errorf("expected AppEnv 'production', got %q", cfg.AppEnv)
	}
	if cfg.HTTPPort != "9000" {
		t.Errorf("expected HTTPPort '9000', got %q", cfg.HTTPPort)
	}
	if cfg.Version != "1.0.0" {
		t.Errorf("expected Version '1.0.0', got %q", cfg.Version)
	}
	if cfg.GitHash != "commit123" {
		t.Errorf("expected GitHash 'commit123', got %q", cfg.GitHash)
	}
	if cfg.LLMAPIKey != "sk-secret123" {
		t.Errorf("expected LLMAPIKey 'sk-secret123', got %q", cfg.LLMAPIKey)
	}
	if cfg.LLMProvider != "custom_gateway" {
		t.Errorf("expected LLMProvider 'custom_gateway', got %q", cfg.LLMProvider)
	}
	if cfg.LLMBaseURL != "https://api.omniroute.ai/v1" {
		t.Errorf("expected LLMBaseURL 'https://api.omniroute.ai/v1', got %q", cfg.LLMBaseURL)
	}
	if cfg.LLMModel != "gpt-4o" {
		t.Errorf("expected LLMModel 'gpt-4o', got %q", cfg.LLMModel)
	}
	if cfg.STTModel != "whisper-large-v3" {
		t.Errorf("expected STTModel 'whisper-large-v3', got %q", cfg.STTModel)
	}
	if cfg.EmbeddingModel != "text-embedding-3-small" {
		t.Errorf("expected EmbeddingModel 'text-embedding-3-small', got %q", cfg.EmbeddingModel)
	}
	if cfg.EmbeddingDimension != 1536 {
		t.Errorf("expected EmbeddingDimension 1536, got %d", cfg.EmbeddingDimension)
	}
	if cfg.RAGChunkTokens != 500 {
		t.Errorf("expected RAGChunkTokens 500, got %d", cfg.RAGChunkTokens)
	}
	if cfg.RAGChunkOverlapTokens != 100 {
		t.Errorf("expected RAGChunkOverlapTokens 100, got %d", cfg.RAGChunkOverlapTokens)
	}
	if cfg.RAGTopK != 10 {
		t.Errorf("expected RAGTopK 10, got %d", cfg.RAGTopK)
	}
}

func TestConfig_DSN(t *testing.T) {
	cfg := config.Config{
		DBHost:    "127.0.0.1",
		DBPort:    "5432",
		DBUser:    "myuser",
		DBPass:    "mypassword",
		DBName:    "taskdb",
		DBSSLMode: "disable",
	}

	expected := "host=127.0.0.1 port=5432 user=myuser password=mypassword dbname=taskdb sslmode=disable"
	got := cfg.DSN()

	if got != expected {
		t.Errorf("expected DSN %q, got %q", expected, got)
	}
}

func TestConfig_LoadPostgresEnv(t *testing.T) {
	t.Setenv("POSTGRES_HOST", "custom-pg-host")
	t.Setenv("POSTGRES_PORT", "5433")
	t.Setenv("POSTGRES_USER", "custom-user")
	t.Setenv("POSTGRES_PASSWORD", "custom-secret")
	t.Setenv("POSTGRES_DB", "custom-dbname")
	t.Setenv("POSTGRES_SSLMODE", "require")

	cfg := config.Load()

	if cfg.DBHost != "custom-pg-host" {
		t.Errorf("expected DBHost 'custom-pg-host', got %q", cfg.DBHost)
	}
	if cfg.DBPort != "5433" {
		t.Errorf("expected DBPort '5433', got %q", cfg.DBPort)
	}
	if cfg.DBUser != "custom-user" {
		t.Errorf("expected DBUser 'custom-user', got %q", cfg.DBUser)
	}
	if cfg.DBPass != "custom-secret" {
		t.Errorf("expected DBPass 'custom-secret', got %q", cfg.DBPass)
	}
	if cfg.DBName != "custom-dbname" {
		t.Errorf("expected DBName 'custom-dbname', got %q", cfg.DBName)
	}
	if cfg.DBSSLMode != "require" {
		t.Errorf("expected DBSSLMode 'require', got %q", cfg.DBSSLMode)
	}
}

func TestConfig_LoadRedisEnv(t *testing.T) {
	t.Setenv("REDIS_HOST", "custom-redis-host")
	t.Setenv("REDIS_PORT", "6380")
	t.Setenv("REDIS_PASSWORD", "secret-pass")
	t.Setenv("REDIS_DB", "2")
	t.Setenv("REDIS_POOL_SIZE", "50")
	t.Setenv("REDIS_DIAL_TIMEOUT", "10s")
	t.Setenv("REDIS_READ_TIMEOUT", "2s")
	t.Setenv("REDIS_WRITE_TIMEOUT", "4s")

	cfg := config.Load()

	if cfg.RedisHost != "custom-redis-host" {
		t.Errorf("expected RedisHost 'custom-redis-host', got %q", cfg.RedisHost)
	}
	if cfg.RedisPort != "6380" {
		t.Errorf("expected RedisPort '6380', got %q", cfg.RedisPort)
	}
	if cfg.RedisPassword != "secret-pass" {
		t.Errorf("expected RedisPassword 'secret-pass', got %q", cfg.RedisPassword)
	}
	if cfg.RedisDB != 2 {
		t.Errorf("expected RedisDB 2, got %d", cfg.RedisDB)
	}
	if cfg.RedisPoolSize != 50 {
		t.Errorf("expected RedisPoolSize 50, got %d", cfg.RedisPoolSize)
	}
	if cfg.RedisDialTimeout != 10*time.Second {
		t.Errorf("expected RedisDialTimeout 10s, got %v", cfg.RedisDialTimeout)
	}
	if cfg.RedisReadTimeout != 2*time.Second {
		t.Errorf("expected RedisReadTimeout 2s, got %v", cfg.RedisReadTimeout)
	}
	if cfg.RedisWriteTimeout != 4*time.Second {
		t.Errorf("expected RedisWriteTimeout 4s, got %v", cfg.RedisWriteTimeout)
	}
}

func TestConfig_RedisAddr(t *testing.T) {
	cfg := config.Config{
		RedisHost: "10.0.0.1",
		RedisPort: "6379",
	}

	expected := "10.0.0.1:6379"
	got := cfg.RedisAddr()

	if got != expected {
		t.Errorf("expected RedisAddr %q, got %q", expected, got)
	}
}

func TestConfig_LoadS3Env(t *testing.T) {
	t.Setenv("S3_ENDPOINT", "s3.amazonaws.com")
	t.Setenv("S3_ACCESS_KEY", "custom-key")
	t.Setenv("S3_SECRET_KEY", "custom-secret")
	t.Setenv("S3_BUCKET_NAME", "custom-bucket")
	t.Setenv("S3_REGION", "ap-southeast-1")
	t.Setenv("S3_USE_SSL", "true")
	t.Setenv("S3_FORCE_PATH_STYLE", "false")

	cfg := config.Load()

	if cfg.S3Endpoint != "s3.amazonaws.com" {
		t.Errorf("expected S3Endpoint 's3.amazonaws.com', got %q", cfg.S3Endpoint)
	}
	if cfg.S3AccessKey != "custom-key" {
		t.Errorf("expected S3AccessKey 'custom-key', got %q", cfg.S3AccessKey)
	}
	if cfg.S3SecretKey != "custom-secret" {
		t.Errorf("expected S3SecretKey 'custom-secret', got %q", cfg.S3SecretKey)
	}
	if cfg.S3BucketName != "custom-bucket" {
		t.Errorf("expected S3BucketName 'custom-bucket', got %q", cfg.S3BucketName)
	}
	if cfg.S3Region != "ap-southeast-1" {
		t.Errorf("expected S3Region 'ap-southeast-1', got %q", cfg.S3Region)
	}
	if cfg.S3UseSSL != true {
		t.Errorf("expected S3UseSSL true, got %v", cfg.S3UseSSL)
	}
	if cfg.S3ForcePathStyle != false {
		t.Errorf("expected S3ForcePathStyle false, got %v", cfg.S3ForcePathStyle)
	}
}

func TestConfig_LoadIdempotencyTTLEnv(t *testing.T) {
	testCases := []struct {
		envVal   string
		expected time.Duration
	}{
		{"1s", 1 * time.Second},
		{"5s", 5 * time.Second},
		{"1m", 1 * time.Minute},
		{"5m", 5 * time.Minute},
		{"1h", 1 * time.Hour},
		{"24h", 24 * time.Hour},
		{"0s", 24 * time.Hour},
		{"-1s", 24 * time.Hour},
		{"invalid", 24 * time.Hour},
	}

	for _, tc := range testCases {
		t.Setenv("IDEMPOTENCY_TTL", tc.envVal)
		cfg := config.Load()
		if cfg.IdempotencyTTL != tc.expected {
			t.Errorf("for IDEMPOTENCY_TTL=%q, expected %v, got %v", tc.envVal, tc.expected, cfg.IdempotencyTTL)
		}
	}
}
