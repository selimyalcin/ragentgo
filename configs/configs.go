package configs

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// ProviderConfig describes an embedding or LLM backend.
type ProviderConfig struct {
	Provider       string `mapstructure:"provider"`
	Model          string `mapstructure:"model"`
	BaseURL        string `mapstructure:"base_url"`
	APIKey         string `mapstructure:"api_key"`
	Dimension      int    `mapstructure:"dimension"`
	MaxInputTokens int    `mapstructure:"max_input_tokens"`
	MaxTokens      int    `mapstructure:"max_tokens"`
	MaxConcurrency int    `mapstructure:"max_concurrency"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
}

// DatabaseConfig holds PostgreSQL settings. DSN on the root config is still
// the primary connection string, matching the TelegramDownloader layout.
type DatabaseConfig struct {
	URL       string `mapstructure:"url"`
	Dimension int    `mapstructure:"dimension"`
}

// RetrievalConfig is query-time search.
type RetrievalConfig struct {
	TopK     int     `mapstructure:"top_k"`
	MinScore float32 `mapstructure:"min_score"`
	Hybrid   bool    `mapstructure:"hybrid"`
}

// CacheConfig controls answer reuse. Omitted or 0 defaults to one hour.
// Set to -1 to disable the answer cache. Corpus fingerprints still bust
// stale entries when documents change, even before the TTL expires.
type CacheConfig struct {
	AnswerTTLSeconds int `mapstructure:"answer_ttl_seconds"`
}

// ChunkingConfig is document splitting.
type ChunkingConfig struct {
	Strategy string `mapstructure:"strategy"`
	Size     int    `mapstructure:"size"`
	Overlap  int    `mapstructure:"overlap"`
}

// VectorConfig is pgvector index settings.
type VectorConfig struct {
	Index    string `mapstructure:"index"`
	Distance string `mapstructure:"distance"`
}

// CorsConfig is HTTP CORS.
type CorsConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
}

// RateLimiterConfig is a simple per-IP cap.
type RateLimiterConfig struct {
	MaxRequestsPerIP int `mapstructure:"max_requests"`
	IntervalSeconds  int `mapstructure:"interval_seconds"`
}

// CrawlerConfig is the Chrome site crawler.
type CrawlerConfig struct {
	MaxPages             int    `mapstructure:"max_pages"`
	MaxDepth             int    `mapstructure:"max_depth"`
	DelayMS              int    `mapstructure:"delay_ms"`
	PageTimeoutSeconds   int    `mapstructure:"page_timeout_seconds"`
	ChallengeWaitSeconds int    `mapstructure:"challenge_wait_seconds"`
	Headless             bool   `mapstructure:"headless"`
	ChromePath           string `mapstructure:"chrome_path"`
}

// SecurityConfig covers ingestion limits and URL loader policy.
type SecurityConfig struct {
	MaxDocumentBytes int64    `mapstructure:"max_document_bytes"`
	MaxQueryChars    int      `mapstructure:"max_query_chars"`
	AllowPrivateURLs bool     `mapstructure:"allow_private_urls"`
	AllowedHosts     []string `mapstructure:"allowed_hosts"`
}

// Config is the process configuration. ENVIRONMENT selects config.<env>.yaml
// under ./configs, the same pattern used by TelegramDownloader.
type Config struct {
	AppName     string `mapstructure:"app_name"`
	ServiceSlug string `mapstructure:"slug"`
	Environment string `mapstructure:"environment"`
	Host        string `mapstructure:"host"`
	Port        string `mapstructure:"port"`
	Dsn         string `mapstructure:"dsn"`
	DbSchema    string `mapstructure:"db_schema"`
	AIApiURL    string `mapstructure:"ai_api_url"`
	AIApiKey    string `mapstructure:"ai_api_key"`
	Store       string `mapstructure:"store"`

	Database    DatabaseConfig    `mapstructure:"database"`
	Embedding   ProviderConfig    `mapstructure:"embedding"`
	LLM         ProviderConfig    `mapstructure:"llm"`
	Retrieval   RetrievalConfig   `mapstructure:"retrieval"`
	Cache       CacheConfig       `mapstructure:"cache"`
	Chunking    ChunkingConfig    `mapstructure:"chunking"`
	Vector      VectorConfig      `mapstructure:"vector"`
	Cors        CorsConfig        `mapstructure:"cors"`
	RateLimiter RateLimiterConfig `mapstructure:"rate_limiter"`
	Security    SecurityConfig    `mapstructure:"security"`
	Crawler     CrawlerConfig     `mapstructure:"crawler"`
}

// LoadConfig reads YAML for the current environment, then applies env overrides.
func LoadConfig() (*Config, error) {
	viper.SetDefault("environment", "development")
	viper.SetDefault("host", "0.0.0.0")
	viper.SetDefault("port", "8080")
	viper.SetDefault("store", "pgvector")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	_ = viper.BindEnv("environment", "ENVIRONMENT", "RAGENTGO_ENVIRONMENT")
	_ = viper.BindEnv("dsn", "RAGENTGO_DATABASE_URL", "DATABASE_URL", "DSN")
	_ = viper.BindEnv("ai_api_url", "RAGENTGO_AI_API_URL")
	_ = viper.BindEnv("ai_api_key", "RAGENTGO_OPENAI_API_KEY", "OPENAI_API_KEY", "RAGENTGO_AI_API_KEY")
	_ = viper.BindEnv("embedding.model", "RAGENTGO_EMBEDDING_MODEL")
	_ = viper.BindEnv("embedding.provider", "RAGENTGO_EMBEDDING_PROVIDER")
	_ = viper.BindEnv("embedding.dimension", "RAGENTGO_EMBEDDING_DIMENSION")
	_ = viper.BindEnv("embedding.max_input_tokens", "RAGENTGO_EMBEDDING_MAX_INPUT_TOKENS")
	_ = viper.BindEnv("llm.model", "RAGENTGO_LLM_MODEL")
	_ = viper.BindEnv("llm.max_tokens", "RAGENTGO_LLM_MAX_TOKENS")
	_ = viper.BindEnv("retrieval.hybrid", "RAGENTGO_RETRIEVAL_HYBRID")
	_ = viper.BindEnv("retrieval.top_k", "RAGENTGO_RETRIEVAL_TOP_K")
	_ = viper.BindEnv("cache.answer_ttl_seconds", "RAGENTGO_CACHE_ANSWER_TTL_SECONDS")
	_ = viper.BindEnv("embedding.base_url", "RAGENTGO_EMBEDDING_BASE_URL")
	_ = viper.BindEnv("llm.base_url", "RAGENTGO_LLM_BASE_URL")
	_ = viper.BindEnv("embedding.api_key", "RAGENTGO_EMBEDDING_API_KEY")
	_ = viper.BindEnv("llm.api_key", "RAGENTGO_LLM_API_KEY")
	_ = viper.BindEnv("port", "PORT", "RAGENTGO_PORT")
	_ = viper.BindEnv("store", "RAGENTGO_STORE")

	env := viper.GetString("environment")
	if env == "" {
		env = "development"
	}

	viper.SetConfigName(fmt.Sprintf("config.%s", env))
	viper.AddConfigPath(".")
	viper.AddConfigPath("./configs")
	viper.SetConfigType("yaml")

	var cfg Config
	if err := viper.ReadInConfig(); err != nil {
		log.Printf("config file not found for %s environment: %v", env, err)
	}
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("config unmarshalling failed: %v", err)
	}
	cfg.Environment = env
	cfg.applyDefaults()
	cfg.applyAliases()
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.AppName == "" {
		c.AppName = "ragentgo"
	}
	if c.Host == "" {
		c.Host = "0.0.0.0"
	}
	if c.Port == "" {
		c.Port = "8080"
	}
	if c.AIApiURL == "" {
		c.AIApiURL = "http://localhost:8000/v1"
	}
	if c.Store == "" {
		c.Store = "pgvector"
	}
	if c.Embedding.Provider == "" {
		c.Embedding.Provider = "openai-compat"
	}
	if c.Embedding.BaseURL == "" {
		c.Embedding.BaseURL = c.AIApiURL
	}
	if c.Embedding.APIKey == "" {
		c.Embedding.APIKey = c.AIApiKey
	}
	if c.Embedding.Model == "" {
		c.Embedding.Model = "text-embedding-3-small"
	}
	if c.Embedding.Dimension == 0 {
		c.Embedding.Dimension = 1536
	}
	if c.Embedding.MaxConcurrency == 0 {
		c.Embedding.MaxConcurrency = 4
	}
	if c.LLM.Provider == "" {
		c.LLM.Provider = "openai-compat"
	}
	if c.LLM.BaseURL == "" {
		c.LLM.BaseURL = c.AIApiURL
	}
	if c.LLM.APIKey == "" {
		c.LLM.APIKey = c.AIApiKey
	}
	if c.LLM.Model == "" {
		c.LLM.Model = "gpt-4.1-mini"
	}
	if c.Retrieval.TopK == 0 {
		c.Retrieval.TopK = 5
	}
	if c.Cache.AnswerTTLSeconds == 0 {
		c.Cache.AnswerTTLSeconds = 3600
	}
	if c.Chunking.Size == 0 {
		c.Chunking.Size = 800
	}
	if c.Chunking.Overlap == 0 {
		c.Chunking.Overlap = 120
	}
	if c.Chunking.Strategy == "" {
		c.Chunking.Strategy = "recursive"
	}
	if c.Vector.Index == "" {
		c.Vector.Index = "hnsw"
	}
	if c.Vector.Distance == "" {
		c.Vector.Distance = "cosine"
	}
	if c.Database.Dimension == 0 {
		c.Database.Dimension = c.Embedding.Dimension
	}
	if len(c.Cors.AllowedOrigins) == 0 {
		c.Cors.AllowedOrigins = []string{"*"}
	}
	if c.RateLimiter.MaxRequestsPerIP == 0 {
		c.RateLimiter.MaxRequestsPerIP = 120
	}
	if c.RateLimiter.IntervalSeconds == 0 {
		c.RateLimiter.IntervalSeconds = 60
	}
	if c.Security.MaxDocumentBytes == 0 {
		c.Security.MaxDocumentBytes = 8 << 20
	}
	if c.Security.MaxQueryChars == 0 {
		c.Security.MaxQueryChars = 8000
	}
	if c.Crawler.MaxPages == 0 {
		c.Crawler.MaxPages = 40
	}
	if c.Crawler.MaxDepth == 0 {
		c.Crawler.MaxDepth = 3
	}
	if c.Crawler.DelayMS == 0 {
		c.Crawler.DelayMS = 400
	}
	if c.Crawler.PageTimeoutSeconds == 0 {
		c.Crawler.PageTimeoutSeconds = 45
	}
	if c.Crawler.ChallengeWaitSeconds == 0 {
		c.Crawler.ChallengeWaitSeconds = 20
	}
	if !c.Crawler.Headless && c.Crawler.ChromePath == "" {
		c.Crawler.Headless = true
	}
}

func (c *Config) applyAliases() {
	if c.Database.URL != "" && c.Dsn == "" {
		c.Dsn = c.Database.URL
	}
	if c.Dsn != "" && c.Database.URL == "" {
		c.Database.URL = c.Dsn
	}
	// Shared AI URL/key fill only empty nested fields so embedding and LLM
	// can point at different providers (local MiniLM + hosted DeepSeek).
	if c.Embedding.BaseURL == "" && c.AIApiURL != "" {
		c.Embedding.BaseURL = c.AIApiURL
	}
	if c.LLM.BaseURL == "" && c.AIApiURL != "" {
		c.LLM.BaseURL = c.AIApiURL
	}
	if c.Embedding.APIKey == "" && c.AIApiKey != "" {
		c.Embedding.APIKey = c.AIApiKey
	}
	if c.LLM.APIKey == "" && c.AIApiKey != "" {
		c.LLM.APIKey = c.AIApiKey
	}
	if c.Embedding.Dimension > 0 {
		c.Database.Dimension = c.Embedding.Dimension
	}
}

// Addr is host:port for Fiber.
func (c *Config) Addr() string {
	host := c.Host
	if host == "" {
		host = "0.0.0.0"
	}
	return host + ":" + c.Port
}

// ProviderTimeout converts seconds to a duration with a sane floor.
func ProviderTimeout(seconds int, fallback time.Duration) time.Duration {
	if seconds <= 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}
