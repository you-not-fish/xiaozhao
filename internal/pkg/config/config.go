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
	return nil
}
