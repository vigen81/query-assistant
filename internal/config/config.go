package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"gitlab.smartbet.am/golang/query-assistant/internal/plugin/ams"
	"go.uber.org/fx"
)

const mockConfig = `{
	"server": {
		"port": ":8080",
		"read_timeout": "30s",
		"write_timeout": "30s",
		"idle_timeout": "120s"
	},
	"clickhouse": {
		"host": "localhost",
		"port": "9000",
		"database": "default",
		"username": "default",
		"password": "",
		"max_open_conns": 10,
		"max_idle_conns": 5,
		"conn_max_lifetime": 300,
		"query_timeout": "60s"
	},
	"openai": {
		"api_key": "",
		"model": "gpt-4o",
		"max_tokens": 2000,
		"temperature": 0.1,
		"timeout": "30s"
	},
	"query": {
		"max_execution_time": "30s",
		"max_result_rows": 10000,
		"enable_query_validation": true,
		"allowed_operations": ["SELECT"],
		"forbidden_keywords": ["DROP", "DELETE", "TRUNCATE", "ALTER", "CREATE", "INSERT", "UPDATE"]
	},
	"banner": {
		"variant_count": 4,
		"default_provider": "openai"
	},
	"swagger": {
		"enabled": true,
		"host": "localhost:8080",
		"title": "Query Assistant API",
		"version": "1.0"
	},
	"logging": {
		"graylog_addr": "gelf-udp-service:12222",
		"service_name": "query-assistant"
	},
	"auth": {
		"jwt_secret": "your-secret-key",
		"skip_auth": true
	}
}`

type Config struct {
	Server     ServerConfig     `json:"server"`
	ClickHouse ClickHouseConfig `json:"clickhouse"`
	OpenAI     OpenAIConfig     `json:"openai"`
	Query      QueryConfig      `json:"query"`
	Banner     BannerConfig     `json:"banner"`
	Swagger    SwaggerConfig    `json:"swagger"`
	Logging    LoggingConfig    `json:"logging"`
	Auth       AuthConfig       `json:"auth"`
}

type ServerConfig struct {
	Port         string `json:"port"`
	ReadTimeout  string `json:"read_timeout"`
	WriteTimeout string `json:"write_timeout"`
	IdleTimeout  string `json:"idle_timeout"`
}

type ClickHouseConfig struct {
	Host            string `json:"host"`
	Port            string `json:"port"`
	Database        string `json:"database"`
	Username        string `json:"username"`
	Password        string `json:"password"`
	MaxOpenConns    int    `json:"max_open_conns"`
	MaxIdleConns    int    `json:"max_idle_conns"`
	ConnMaxLifetime int    `json:"conn_max_lifetime"`
	QueryTimeout    string `json:"query_timeout"`
}

type OpenAIConfig struct {
	APIKey      string  `json:"api_key"`
	Model       string  `json:"model"`
	MaxTokens   int     `json:"max_tokens"`
	Temperature float32 `json:"temperature"`
	Timeout     string  `json:"timeout"`
}

type QueryConfig struct {
	MaxExecutionTime      string   `json:"max_execution_time"`
	MaxResultRows         int      `json:"max_result_rows"`
	EnableQueryValidation bool     `json:"enable_query_validation"`
	AllowedOperations     []string `json:"allowed_operations"`
	ForbiddenKeywords     []string `json:"forbidden_keywords"`
}

// BannerConfig controls the image-generation feature.
type BannerConfig struct {
	// VariantCount is the number of image variants generated per request. Default: 4.
	VariantCount int `json:"variant_count"`
	// DefaultProvider selects the image generation backend. Default: "openai".
	DefaultProvider string `json:"default_provider"`
}

type SwaggerConfig struct {
	Enabled bool   `json:"enabled"`
	Host    string `json:"host"`
	Title   string `json:"title"`
	Version string `json:"version"`
}

type LoggingConfig struct {
	GraylogAddr string `json:"graylog_addr"`
	ServiceName string `json:"service_name"`
}

// AuthConfig holds JWT / auth settings.
type AuthConfig struct {
	JWTSecret string `json:"jwt_secret"`
	SkipAuth  bool   `json:"skip_auth"`
}

// ---------------------------------------------------------------------------
// Duration helpers
// ---------------------------------------------------------------------------

func parseDuration(s string, fallback time.Duration) time.Duration {
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	return fallback
}

func (c *Config) GetServerReadTimeout() time.Duration {
	return parseDuration(c.Server.ReadTimeout, 30*time.Second)
}

func (c *Config) GetServerWriteTimeout() time.Duration {
	return parseDuration(c.Server.WriteTimeout, 30*time.Second)
}

func (c *Config) GetServerIdleTimeout() time.Duration {
	return parseDuration(c.Server.IdleTimeout, 120*time.Second)
}

func (c *Config) GetQueryTimeout() time.Duration {
	return parseDuration(c.ClickHouse.QueryTimeout, 60*time.Second)
}

func (c *Config) GetMaxExecutionTime() time.Duration {
	return parseDuration(c.Query.MaxExecutionTime, 30*time.Second)
}

func (c *Config) GetOpenAITimeout() time.Duration {
	return parseDuration(c.OpenAI.Timeout, 30*time.Second)
}

func (c *Config) GetClickHouseDSN() string {
	return fmt.Sprintf("clickhouse://%s:%s@%s:%s/%s",
		c.ClickHouse.Username,
		c.ClickHouse.Password,
		c.ClickHouse.Host,
		c.ClickHouse.Port,
		c.ClickHouse.Database,
	)
}

// ---------------------------------------------------------------------------
// Loader
// ---------------------------------------------------------------------------

func (cnf *Config) run(serviceName string) error {
	var data []byte
	var err error

	if os.Getenv("POD_ENV") == "local" {
		data = []byte(mockConfig)
	} else {
		secretName := fmt.Sprintf("/%s/%s", os.Getenv("POD_ENV"), serviceName)
		r := ams.NewSource(ams.WithSecretName(secretName))
		data, err = r.Read()
		if err != nil {
			return fmt.Errorf("failed to read config from AWS Parameter Store: %w", err)
		}
	}

	if err := json.Unmarshal(data, cnf); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	cnf.overrideWithEnv()
	cnf.applyDefaults()
	return nil
}

func (c *Config) overrideWithEnv() {
	if port := os.Getenv("SERVER_PORT"); port != "" {
		c.Server.Port = port
	}
	if chHost := os.Getenv("CLICKHOUSE_HOST"); chHost != "" {
		c.ClickHouse.Host = chHost
	}
	if chPass := os.Getenv("CLICKHOUSE_PASSWORD"); chPass != "" {
		c.ClickHouse.Password = chPass
	}
	if openaiKey := os.Getenv("OPENAI_API_KEY"); openaiKey != "" {
		c.OpenAI.APIKey = openaiKey
	}
	if graylogAddr := os.Getenv("GRAYLOG_ADDR"); graylogAddr != "" {
		c.Logging.GraylogAddr = graylogAddr
	}
	if openaiModel := os.Getenv("OPENAI_MODEL"); openaiModel != "" {
		c.OpenAI.Model = openaiModel
	}
}

func (c *Config) applyDefaults() {
	if c.Banner.VariantCount <= 0 {
		c.Banner.VariantCount = 4
	}
	if c.Banner.DefaultProvider == "" {
		c.Banner.DefaultProvider = "openai"
	}
}

// Provider creates a new config instance using Uber FX lifecycle.
func Provider(lifecycle fx.Lifecycle, serviceName string) (*Config, error) {
	c := &Config{}
	err := c.run(serviceName)

	lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return nil
		},
	})

	return c, err
}
