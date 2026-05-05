// Package config loads the service configuration from a YAML file and
// environment variables. Environment variables use the prefix XIAOZHAO_ and
// replace dots in key paths with underscores (e.g. server.port -> XIAOZHAO_SERVER_PORT).
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Log       LogConfig       `mapstructure:"log"`
	Postgres  PostgresConfig  `mapstructure:"postgres"`
	Redis     RedisConfig     `mapstructure:"redis"`
	Auth      AuthConfig      `mapstructure:"auth"`
	RateLimit RateLimitConfig `mapstructure:"ratelimit"`
	Model     ModelConfig     `mapstructure:"model"`
	Embedding EmbeddingConfig `mapstructure:"embedding"`
	Agent     AgentConfig     `mapstructure:"agent"`
	Storage   StorageConfig   `mapstructure:"storage"`
	WebSearch WebSearchConfig `mapstructure:"web_search"`
	WebFetch  WebFetchConfig  `mapstructure:"web_fetch"`
}

type ServerConfig struct {
	Host                   string `mapstructure:"host"`
	Port                   int    `mapstructure:"port"`
	ReadTimeoutSeconds     int    `mapstructure:"read_timeout_seconds"`
	WriteTimeoutSeconds    int    `mapstructure:"write_timeout_seconds"`
	ShutdownTimeoutSeconds int    `mapstructure:"shutdown_timeout_seconds"`
}

func (s ServerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

func (s ServerConfig) ReadTimeout() time.Duration {
	return time.Duration(s.ReadTimeoutSeconds) * time.Second
}

func (s ServerConfig) WriteTimeout() time.Duration {
	return time.Duration(s.WriteTimeoutSeconds) * time.Second
}

func (s ServerConfig) ShutdownTimeout() time.Duration {
	return time.Duration(s.ShutdownTimeoutSeconds) * time.Second
}

type LogConfig struct {
	Level    string `mapstructure:"level"`
	Encoding string `mapstructure:"encoding"`
	Caller   bool   `mapstructure:"caller"`
}

type PostgresConfig struct {
	DSN                    string `mapstructure:"dsn"`
	MaxOpenConns           int    `mapstructure:"max_open_conns"`
	MaxIdleConns           int    `mapstructure:"max_idle_conns"`
	ConnMaxLifetimeSeconds int    `mapstructure:"conn_max_lifetime_seconds"`
	AutoMigrate            bool   `mapstructure:"auto_migrate"`
}

func (p PostgresConfig) ConnMaxLifetime() time.Duration {
	return time.Duration(p.ConnMaxLifetimeSeconds) * time.Second
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	PoolSize int    `mapstructure:"pool_size"`
}

type AuthConfig struct {
	JWTSecret              string `mapstructure:"jwt_secret"`
	JWTIssuer              string `mapstructure:"jwt_issuer"`
	AccessTokenTTLSeconds  int    `mapstructure:"access_token_ttl_seconds"`
	RefreshTokenTTLSeconds int    `mapstructure:"refresh_token_ttl_seconds"`
	PasswordMinLength      int    `mapstructure:"password_min_length"`
}

func (a AuthConfig) AccessTokenTTL() time.Duration {
	return time.Duration(a.AccessTokenTTLSeconds) * time.Second
}

func (a AuthConfig) RefreshTokenTTL() time.Duration {
	return time.Duration(a.RefreshTokenTTLSeconds) * time.Second
}

type RateLimitConfig struct {
	Enabled          bool `mapstructure:"enabled"`
	PerUserPerMinute int  `mapstructure:"per_user_per_minute"`
	PerIPPerMinute   int  `mapstructure:"per_ip_per_minute"`
}

// ModelConfig 配置模型网关。MVP 支持两种 Provider：
//   - mock：本地脚本化 Provider，开发/演示默认值，无需任何外部依赖；
//   - openai：OpenAI-compatible，适用于 OpenAI 以及任何对齐 Chat Completions 协议的国内外供应商。
type ModelConfig struct {
	// Provider 选择哪种 Provider：mock / openai。留空默认 mock。
	Provider string `mapstructure:"provider"`
	// DefaultModel 是 OpenAI-compatible 调用时传给上游的 model 字段；
	// mock 模式下只作为审计字段使用。
	DefaultModel string `mapstructure:"default_model"`
	// OpenAI 具体 Provider 配置；Provider=openai 时必填。
	OpenAI OpenAIProviderConfig `mapstructure:"openai"`
}

// OpenAIProviderConfig 覆盖 OpenAI-compatible Provider 所需参数。
// APIKey 建议通过环境变量 XIAOZHAO_MODEL_OPENAI_APIKEY 注入，避免写入配置文件。
type OpenAIProviderConfig struct {
	Name           string `mapstructure:"name"`            // 审计用标识，缺省 "openai"
	BaseURL        string `mapstructure:"base_url"`        // 例如 https://api.openai.com/v1
	APIKey         string `mapstructure:"api_key"`         // Bearer token
	TimeoutSeconds int    `mapstructure:"timeout_seconds"` // 单次调用超时
}

type EmbeddingConfig struct {
	Provider string                `mapstructure:"provider"`
	Model    string                `mapstructure:"model"`
	Dim      int                   `mapstructure:"dim"`
	OpenAI   OpenAIEmbeddingConfig `mapstructure:"openai"`
}

type OpenAIEmbeddingConfig struct {
	Name           string `mapstructure:"name"`
	BaseURL        string `mapstructure:"base_url"`
	APIKey         string `mapstructure:"api_key"`
	Model          string `mapstructure:"model"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
}

// AgentConfig 控制 Agent Orchestrator 的运行参数。
type AgentConfig struct {
	MaxToolIterations int `mapstructure:"max_tool_iterations"` // 工具循环上限，默认 5
	ModelTimeoutSec   int `mapstructure:"model_timeout_seconds"`
	ToolTimeoutSec    int `mapstructure:"tool_timeout_seconds"`
	MaxToolResultKB   int `mapstructure:"max_tool_result_kb"` // 工具结果截断阈值
}

type StorageConfig struct {
	Provider         string `mapstructure:"provider"`
	Endpoint         string `mapstructure:"endpoint"`
	AccessKey        string `mapstructure:"access_key"`
	SecretKey        string `mapstructure:"secret_key"`
	Bucket           string `mapstructure:"bucket"`
	Region           string `mapstructure:"region"`
	UseSSL           bool   `mapstructure:"use_ssl"`
	AutoCreateBucket bool   `mapstructure:"auto_create_bucket"`
	MaxUploadMB      int64  `mapstructure:"max_upload_mb"`
}

func (s StorageConfig) MaxUploadBytes() int64 {
	return s.MaxUploadMB * 1024 * 1024
}

type WebSearchConfig struct {
	Provider         string            `mapstructure:"provider"`
	MaxResults       int               `mapstructure:"max_results"`
	DefaultFreshness string            `mapstructure:"default_freshness"`
	TimeoutSeconds   int               `mapstructure:"timeout_seconds"` // 工具调用整体超时（model/Orchestrator 层）
	Bocha            BochaSearchConfig `mapstructure:"bocha"`
}

type BochaSearchConfig struct {
	BaseURL        string `mapstructure:"base_url"`
	APIKey         string `mapstructure:"api_key"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
}

type WebFetchConfig struct {
	TimeoutSeconds int   `mapstructure:"timeout_seconds"`
	MaxBodyKB      int64 `mapstructure:"max_body_kb"`
	MaxRedirects   int   `mapstructure:"max_redirects"`
}

// Load reads the configuration from the given path. Environment variables
// with prefix XIAOZHAO_ override values from the file.
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	v.SetEnvPrefix("XIAOZHAO")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if c.Server.Port <= 0 {
		return fmt.Errorf("server.port must be > 0")
	}
	if c.Postgres.DSN == "" {
		return fmt.Errorf("postgres.dsn must not be empty")
	}
	if len(c.Auth.JWTSecret) < 16 {
		return fmt.Errorf("auth.jwt_secret must be at least 16 chars")
	}
	if c.Auth.AccessTokenTTLSeconds <= 0 {
		return fmt.Errorf("auth.access_token_ttl_seconds must be > 0")
	}
	if c.Auth.PasswordMinLength < 6 {
		c.Auth.PasswordMinLength = 6
	}
	// Model 默认走 mock，使服务在未配置真实供应商时也能启动。
	if c.Model.Provider == "" {
		c.Model.Provider = "mock"
	}
	switch c.Model.Provider {
	case "mock":
		// 允许 default_model 为空，mock 会直接忽略它。
	case "openai":
		if c.Model.OpenAI.BaseURL == "" {
			return fmt.Errorf("model.openai.base_url must not be empty when provider=openai")
		}
		if c.Model.OpenAI.APIKey == "" {
			return fmt.Errorf("model.openai.api_key must not be empty when provider=openai")
		}
		if c.Model.OpenAI.Name == "" {
			c.Model.OpenAI.Name = "openai"
		}
		if c.Model.OpenAI.TimeoutSeconds <= 0 {
			c.Model.OpenAI.TimeoutSeconds = 60
		}
	default:
		return fmt.Errorf("model.provider must be one of: mock, openai (got %q)", c.Model.Provider)
	}
	if c.Embedding.Provider == "" {
		c.Embedding.Provider = "mock"
	}
	if c.Embedding.Model == "" {
		c.Embedding.Model = "text-embedding-3-small"
	}
	if c.Embedding.Dim <= 0 {
		c.Embedding.Dim = 1536
	}
	if c.Embedding.Dim != 1536 {
		return fmt.Errorf("embedding.dim must be 1536 for MVP pgvector schema")
	}
	if c.Embedding.Provider == "openai" {
		if c.Embedding.OpenAI.BaseURL == "" {
			return fmt.Errorf("embedding.openai.base_url must not be empty when provider=openai")
		}
		if c.Embedding.OpenAI.APIKey == "" {
			return fmt.Errorf("embedding.openai.api_key must not be empty when provider=openai")
		}
		if c.Embedding.OpenAI.Name == "" {
			c.Embedding.OpenAI.Name = "openai"
		}
		if c.Embedding.OpenAI.Model == "" {
			c.Embedding.OpenAI.Model = c.Embedding.Model
		}
		if c.Embedding.OpenAI.TimeoutSeconds <= 0 {
			c.Embedding.OpenAI.TimeoutSeconds = 60
		}
	} else if c.Embedding.Provider != "mock" {
		return fmt.Errorf("embedding.provider must be one of: mock, openai (got %q)", c.Embedding.Provider)
	}
	if c.Agent.MaxToolIterations <= 0 {
		c.Agent.MaxToolIterations = 5
	}
	if c.Agent.ModelTimeoutSec <= 0 {
		c.Agent.ModelTimeoutSec = 60
	}
	if c.Agent.ToolTimeoutSec <= 0 {
		c.Agent.ToolTimeoutSec = 15
	}
	if c.Agent.MaxToolResultKB <= 0 {
		c.Agent.MaxToolResultKB = 64
	}
	if c.Storage.Provider == "" {
		c.Storage.Provider = "minio"
	}
	if c.Storage.Provider != "minio" {
		return fmt.Errorf("storage.provider must be minio (got %q)", c.Storage.Provider)
	}
	if c.Storage.Endpoint == "" {
		return fmt.Errorf("storage.endpoint must not be empty")
	}
	if c.Storage.Bucket == "" {
		return fmt.Errorf("storage.bucket must not be empty")
	}
	if c.Storage.MaxUploadMB <= 0 {
		c.Storage.MaxUploadMB = 50
	}
	if c.WebSearch.Provider == "" {
		c.WebSearch.Provider = "mock"
	}
	if c.WebSearch.MaxResults <= 0 {
		c.WebSearch.MaxResults = 5
	}
	if c.WebSearch.MaxResults > 10 {
		c.WebSearch.MaxResults = 10
	}
	if c.WebSearch.DefaultFreshness == "" {
		c.WebSearch.DefaultFreshness = "noLimit"
	}
	switch c.WebSearch.DefaultFreshness {
	case "noLimit", "oneDay", "oneWeek", "oneMonth", "oneYear":
	default:
		return fmt.Errorf("web_search.default_freshness must be one of: noLimit, oneDay, oneWeek, oneMonth, oneYear (got %q)", c.WebSearch.DefaultFreshness)
	}
	if c.WebSearch.TimeoutSeconds <= 0 {
		c.WebSearch.TimeoutSeconds = 15
	}
	switch c.WebSearch.Provider {
	case "mock":
	case "bocha":
		if c.WebSearch.Bocha.BaseURL == "" {
			c.WebSearch.Bocha.BaseURL = "https://api.bochaai.com/v1/web-search"
		}
		if c.WebSearch.Bocha.APIKey == "" {
			return fmt.Errorf("web_search.bocha.api_key must not be empty when provider=bocha")
		}
		if c.WebSearch.Bocha.TimeoutSeconds <= 0 {
			c.WebSearch.Bocha.TimeoutSeconds = 15
		}
	default:
		return fmt.Errorf("web_search.provider must be one of: mock, bocha (got %q)", c.WebSearch.Provider)
	}
	if c.WebFetch.TimeoutSeconds <= 0 {
		c.WebFetch.TimeoutSeconds = 15
	}
	if c.WebFetch.MaxBodyKB <= 0 {
		c.WebFetch.MaxBodyKB = 1024
	}
	if c.WebFetch.MaxRedirects <= 0 {
		c.WebFetch.MaxRedirects = 3
	}
	return nil
}
