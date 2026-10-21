package config

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// knownKeys are cleared before every case so a developer's shell cannot leak into a test.
var knownKeys = []string{
	"APP_ENV",
	"HTTP_HOST", "HTTP_PORT", "HTTP_READ_HEADER_TIMEOUT", "HTTP_READ_TIMEOUT",
	"HTTP_WRITE_TIMEOUT", "HTTP_IDLE_TIMEOUT", "HTTP_SHUTDOWN_TIMEOUT",
	"LOG_LEVEL", "LOG_FORMAT",
	"DATABASE_URL", "PG_HOST", "PG_PORT", "PG_USER", "PG_PASSWORD", "PG_DATABASE",
	"PG_SSLMODE", "PG_POOL_MAX_CONNS", "PG_POOL_MIN_CONNS",
	"PG_CONN_MAX_LIFETIME", "PG_CONN_MAX_IDLE_TIME", "PG_CONNECT_TIMEOUT", "PG_STATEMENT_TIMEOUT",
	"JWT_SECRET", "JWT_ACCESS_TTL", "JWT_REFRESH_TTL", "JWT_ISSUER", "JWT_AUDIENCE", "BCRYPT_COST",
}

func isolateEnv(t *testing.T) {
	t.Helper()
	for _, key := range knownKeys {
		t.Setenv(key, "")
	}
}

func setEnv(t *testing.T, env map[string]string) {
	t.Helper()
	isolateEnv(t)
	for key, value := range env {
		t.Setenv(key, value)
	}
}

func requiredEnv(extra map[string]string) map[string]string {
	env := map[string]string{
		"PG_USER":     "dentacore",
		"PG_PASSWORD": "secret",
		"PG_DATABASE": "dentacore",
		"JWT_SECRET":  testJWTSecret,
	}
	for key, value := range extra {
		env[key] = value
	}
	return env
}

// testJWTSecret is exactly the 32-byte minimum HS256 accepts.
const testJWTSecret = "0123456789abcdef0123456789abcdef"

func defaultConfig() Config {
	return Config{
		Env: EnvDevelopment,
		HTTP: HTTP{
			Host:              "0.0.0.0",
			Port:              8080,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      15 * time.Second,
			IdleTimeout:       60 * time.Second,
			ShutdownTimeout:   10 * time.Second,
		},
		Postgres: Postgres{
			Host:             "localhost",
			Port:             5432,
			User:             "dentacore",
			Password:         "secret",
			Database:         "dentacore",
			SSLMode:          "disable",
			MaxConns:         10,
			MinConns:         0,
			MaxConnLifetime:  time.Hour,
			MaxConnIdleTime:  15 * time.Minute,
			ConnectTimeout:   5 * time.Second,
			StatementTimeout: 5 * time.Second,
		},
		Log: Log{Level: slog.LevelInfo, Format: LogFormatJSON},
		Auth: Auth{
			JWTSecret:       testJWTSecret,
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 720 * time.Hour,
			Issuer:          "dentacore",
			Audience:        "dentacore-api",
			BcryptCost:      12,
		},
	}
}

func TestLoadDefaults(t *testing.T) {
	setEnv(t, requiredEnv(nil))

	got, err := Load()

	require.NoError(t, err)
	assert.Equal(t, defaultConfig(), got)
}

func TestLoadOverrides(t *testing.T) {
	tests := []struct {
		name   string
		env    map[string]string
		expect func(*Config)
	}{
		{
			name: "http bindings and timeouts",
			env: requiredEnv(map[string]string{
				"HTTP_HOST":                "127.0.0.1",
				"HTTP_PORT":                "9090",
				"HTTP_READ_HEADER_TIMEOUT": "1s",
				"HTTP_READ_TIMEOUT":        "2s",
				"HTTP_WRITE_TIMEOUT":       "3s",
				"HTTP_IDLE_TIMEOUT":        "4s",
				"HTTP_SHUTDOWN_TIMEOUT":    "6s",
			}),
			expect: func(c *Config) {
				c.HTTP = HTTP{
					Host: "127.0.0.1", Port: 9090,
					ReadHeaderTimeout: time.Second, ReadTimeout: 2 * time.Second,
					WriteTimeout: 3 * time.Second, IdleTimeout: 4 * time.Second,
					ShutdownTimeout: 6 * time.Second,
				}
			},
		},
		{
			name: "text logs at debug level",
			env:  requiredEnv(map[string]string{"LOG_LEVEL": "debug", "LOG_FORMAT": "text"}),
			expect: func(c *Config) {
				c.Log = Log{Level: slog.LevelDebug, Format: LogFormatText}
			},
		},
		{
			name: "warn level accepts upper case",
			env:  requiredEnv(map[string]string{"LOG_LEVEL": "WARN"}),
			expect: func(c *Config) {
				c.Log.Level = slog.LevelWarn
			},
		},
		{
			name: "staging environment",
			env:  requiredEnv(map[string]string{"APP_ENV": "staging"}),
			expect: func(c *Config) {
				c.Env = EnvStaging
			},
		},
		{
			name: "DATABASE_URL makes discrete PG_* fields optional",
			env: map[string]string{
				"DATABASE_URL": "postgres://u:p@db.internal:6543/dentacore?sslmode=require",
				"JWT_SECRET":   testJWTSecret,
			},
			expect: func(c *Config) {
				c.Postgres.URL = "postgres://u:p@db.internal:6543/dentacore?sslmode=require"
				// Credentials now live inside the URL, so the discrete fields stay empty.
				c.Postgres.User = ""
				c.Postgres.Password = ""
				c.Postgres.Database = ""
			},
		},
		{
			name: "pool tuning",
			env: requiredEnv(map[string]string{
				"PG_POOL_MAX_CONNS":     "25",
				"PG_POOL_MIN_CONNS":     "5",
				"PG_CONN_MAX_LIFETIME":  "30m",
				"PG_CONN_MAX_IDLE_TIME": "5m",
				"PG_CONNECT_TIMEOUT":    "2s",
				"PG_STATEMENT_TIMEOUT":  "750ms",
			}),
			expect: func(c *Config) {
				c.Postgres.MaxConns = 25
				c.Postgres.MinConns = 5
				c.Postgres.MaxConnLifetime = 30 * time.Minute
				c.Postgres.MaxConnIdleTime = 5 * time.Minute
				c.Postgres.ConnectTimeout = 2 * time.Second
				c.Postgres.StatementTimeout = 750 * time.Millisecond
			},
		},
		{
			name: "token policy",
			env: requiredEnv(map[string]string{
				"JWT_SECRET":      "a-much-longer-signing-secret-than-the-minimum",
				"JWT_ACCESS_TTL":  "5m",
				"JWT_REFRESH_TTL": "168h",
				"JWT_ISSUER":      "clinic.example",
				"JWT_AUDIENCE":    "patients-app",
				"BCRYPT_COST":     "14",
			}),
			expect: func(c *Config) {
				c.Auth.JWTSecret = "a-much-longer-signing-secret-than-the-minimum"
				c.Auth.AccessTokenTTL = 5 * time.Minute
				c.Auth.RefreshTokenTTL = 168 * time.Hour
				c.Auth.Issuer = "clinic.example"
				c.Auth.Audience = "patients-app"
				c.Auth.BcryptCost = 14
			},
		},
		{
			name: "a blank issuer or audience falls back to the default",
			env: requiredEnv(map[string]string{
				"JWT_ISSUER":   "   ",
				"JWT_AUDIENCE": "",
			}),
			expect: func(c *Config) {
				c.Auth.Issuer = "dentacore"
				c.Auth.Audience = "dentacore-api"
			},
		},
		{
			name: "production with TLS",
			env: requiredEnv(map[string]string{
				"APP_ENV":    "production",
				"PG_SSLMODE": "verify-full",
				"PG_HOST":    "db.internal",
				"PG_PORT":    "6543",
			}),
			expect: func(c *Config) {
				c.Env = EnvProduction
				c.Postgres.Host = "db.internal"
				c.Postgres.Port = 6543
				c.Postgres.SSLMode = "verify-full"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setEnv(t, tt.env)

			got, err := Load()
			require.NoError(t, err)

			want := defaultConfig()
			tt.expect(&want)
			assert.Equal(t, want, got)
		})
	}
}

func TestLoadValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr []string
	}{
		{
			name:    "unsupported APP_ENV",
			env:     requiredEnv(map[string]string{"APP_ENV": "qa"}),
			wantErr: []string{`APP_ENV: unsupported value "qa"`},
		},
		{
			name:    "non-numeric HTTP_PORT",
			env:     requiredEnv(map[string]string{"HTTP_PORT": "eighty"}),
			wantErr: []string{"HTTP_PORT: must be an integer"},
		},
		{
			name:    "HTTP_PORT out of range",
			env:     requiredEnv(map[string]string{"HTTP_PORT": "70000"}),
			wantErr: []string{"HTTP_PORT: 70000 is out of range 1..65535"},
		},
		{
			name:    "whitespace-only HTTP_PORT falls back to the default",
			env:     requiredEnv(map[string]string{"HTTP_PORT": "   "}),
			wantErr: nil,
		},
		{
			name: "missing PG_USER",
			env: map[string]string{
				"PG_PASSWORD": "secret",
				"PG_DATABASE": "dentacore",
			},
			wantErr: []string{"PG_USER: required when DATABASE_URL is not set"},
		},
		{
			name: "missing PG_DATABASE",
			env: map[string]string{
				"PG_USER":     "dentacore",
				"PG_PASSWORD": "secret",
			},
			wantErr: []string{"PG_DATABASE: required when DATABASE_URL is not set"},
		},
		{
			name:    "unsupported PG_SSLMODE",
			env:     requiredEnv(map[string]string{"PG_SSLMODE": "yes-please"}),
			wantErr: []string{`PG_SSLMODE: unsupported value "yes-please"`},
		},
		{
			name: "production forbids plaintext connections",
			env: requiredEnv(map[string]string{
				"APP_ENV":    "production",
				"PG_SSLMODE": "disable",
			}),
			wantErr: []string{`PG_SSLMODE: must not be "disable" in production`},
		},
		{
			name:    "non-numeric PG_PORT",
			env:     requiredEnv(map[string]string{"PG_PORT": "5432x"}),
			wantErr: []string{"PG_PORT: must be an integer"},
		},
		{
			name:    "malformed duration",
			env:     requiredEnv(map[string]string{"HTTP_READ_TIMEOUT": "banana"}),
			wantErr: []string{`HTTP_READ_TIMEOUT: must be a duration like 5s or 1m30s, got "banana"`},
		},
		{
			name:    "zero shutdown timeout",
			env:     requiredEnv(map[string]string{"HTTP_SHUTDOWN_TIMEOUT": "0s"}),
			wantErr: []string{"HTTP_SHUTDOWN_TIMEOUT: must be greater than zero"},
		},
		{
			name:    "negative read header timeout",
			env:     requiredEnv(map[string]string{"HTTP_READ_HEADER_TIMEOUT": "-1s"}),
			wantErr: []string{"HTTP_READ_HEADER_TIMEOUT: must be greater than zero"},
		},
		{
			name:    "negative idle timeout",
			env:     requiredEnv(map[string]string{"HTTP_IDLE_TIMEOUT": "-5s"}),
			wantErr: []string{"HTTP_IDLE_TIMEOUT: must not be negative"},
		},
		{
			name:    "zero max connections",
			env:     requiredEnv(map[string]string{"PG_POOL_MAX_CONNS": "0"}),
			wantErr: []string{"PG_POOL_MAX_CONNS: must be greater than zero"},
		},
		{
			name:    "negative min connections",
			env:     requiredEnv(map[string]string{"PG_POOL_MIN_CONNS": "-1"}),
			wantErr: []string{"PG_POOL_MIN_CONNS: must not be negative"},
		},
		{
			name: "min connections above max",
			env: requiredEnv(map[string]string{
				"PG_POOL_MAX_CONNS": "2",
				"PG_POOL_MIN_CONNS": "5",
			}),
			wantErr: []string{"PG_POOL_MIN_CONNS (5) must not exceed PG_POOL_MAX_CONNS (2)"},
		},
		{
			name:    "zero connect timeout",
			env:     requiredEnv(map[string]string{"PG_CONNECT_TIMEOUT": "0s"}),
			wantErr: []string{"PG_CONNECT_TIMEOUT: must be greater than zero"},
		},
		{
			name:    "malformed statement timeout",
			env:     requiredEnv(map[string]string{"PG_STATEMENT_TIMEOUT": "soon"}),
			wantErr: []string{"PG_STATEMENT_TIMEOUT: must be a duration"},
		},
		{
			name:    "negative statement timeout",
			env:     requiredEnv(map[string]string{"PG_STATEMENT_TIMEOUT": "-1s"}),
			wantErr: []string{"PG_STATEMENT_TIMEOUT: must not be negative"},
		},
		{
			name:    "malformed LOG_LEVEL",
			env:     requiredEnv(map[string]string{"LOG_LEVEL": "loud"}),
			wantErr: []string{"LOG_LEVEL:"},
		},
		{
			name:    "malformed LOG_FORMAT",
			env:     requiredEnv(map[string]string{"LOG_FORMAT": "yaml"}),
			wantErr: []string{`LOG_FORMAT: unsupported value "yaml"`},
		},
		{
			name:    "missing JWT_SECRET",
			env:     map[string]string{"PG_USER": "dentacore", "PG_PASSWORD": "secret", "PG_DATABASE": "dentacore"},
			wantErr: []string{"JWT_SECRET: required"},
		},
		{
			name:    "JWT_SECRET shorter than the HS256 minimum",
			env:     requiredEnv(map[string]string{"JWT_SECRET": "too-short"}),
			wantErr: []string{"JWT_SECRET: must be at least 32 bytes for HS256, got 9"},
		},
		{
			name: "JWT_SECRET whitespace is not trimmed away",
			env: requiredEnv(map[string]string{
				"JWT_SECRET": "                              ",
			}),
			wantErr: []string{"JWT_SECRET: must be at least 32 bytes"},
		},
		{
			name:    "malformed JWT_ACCESS_TTL",
			env:     requiredEnv(map[string]string{"JWT_ACCESS_TTL": "soon"}),
			wantErr: []string{"JWT_ACCESS_TTL: must be a duration"},
		},
		{
			name:    "zero access TTL",
			env:     requiredEnv(map[string]string{"JWT_ACCESS_TTL": "0s"}),
			wantErr: []string{"JWT_ACCESS_TTL: must be greater than zero"},
		},
		{
			name: "refresh TTL does not outlast the access TTL",
			env: requiredEnv(map[string]string{
				"JWT_ACCESS_TTL":  "1h",
				"JWT_REFRESH_TTL": "30m",
			}),
			wantErr: []string{"JWT_REFRESH_TTL (30m0s) must exceed JWT_ACCESS_TTL (1h0m0s)"},
		},
		{
			name:    "bcrypt cost below the floor",
			env:     requiredEnv(map[string]string{"BCRYPT_COST": "8"}),
			wantErr: []string{"BCRYPT_COST: must be between 10 and 15, got 8"},
		},
		{
			name:    "bcrypt cost above the ceiling",
			env:     requiredEnv(map[string]string{"BCRYPT_COST": "18"}),
			wantErr: []string{"BCRYPT_COST: must be between 10 and 15, got 18"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateEnv(t)
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			got, err := Load()

			if len(tt.wantErr) == 0 {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidConfig)
			assert.Zero(t, got, "a rejected configuration must not be partially returned")
			for _, want := range tt.wantErr {
				assert.ErrorContains(t, err, want)
			}
		})
	}
}

func TestLoadReportsEveryProblemAtOnce(t *testing.T) {
	setEnv(t, map[string]string{
		"APP_ENV":   "qa",
		"HTTP_PORT": "nope",
		"LOG_LEVEL": "loud",
	})

	_, err := Load()

	require.Error(t, err)
	assert.ErrorContains(t, err, "APP_ENV")
	assert.ErrorContains(t, err, "HTTP_PORT")
	assert.ErrorContains(t, err, "LOG_LEVEL")
	assert.ErrorContains(t, err, "PG_USER")
	assert.ErrorContains(t, err, "PG_DATABASE")
	assert.ErrorContains(t, err, "JWT_SECRET")
}

// TestLoadNeverEchoesSecrets: a rejected config is printed to stderr and often pasted into
// an issue, so no validation message may quote a credential back.
func TestLoadNeverEchoesSecrets(t *testing.T) {
	const secret = "partial-signing-secret"
	setEnv(t, requiredEnv(map[string]string{"JWT_SECRET": secret, "PG_PASSWORD": "sup3r-s3cret-pg"}))

	_, err := Load()
	require.Error(t, err)
	assert.ErrorContains(t, err, "JWT_SECRET: must be at least 32 bytes")
	assert.NotContains(t, err.Error(), secret, "a rejected secret must never be quoted back")
	assert.NotContains(t, err.Error(), "sup3r-s3cret-pg")
}

func TestLoadPreservesWhitespaceInPassword(t *testing.T) {
	setEnv(t, requiredEnv(map[string]string{"PG_PASSWORD": "  p@ss word  "}))

	got, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "  p@ss word  ", got.Postgres.Password)
	assert.Equal(t,
		"postgres://dentacore:%20%20p%40ss%20word%20%20@localhost:5432/dentacore?sslmode=disable",
		got.Postgres.DSN(),
	)
}

func TestPostgresDSN(t *testing.T) {
	tests := []struct {
		name string
		cfg  Postgres
		want string
	}{
		{
			name: "builds from discrete fields",
			cfg: Postgres{
				Host: "localhost", Port: 5432,
				User: "dentacore", Password: "secret",
				Database: "dentacore", SSLMode: "disable",
			},
			want: "postgres://dentacore:secret@localhost:5432/dentacore?sslmode=disable",
		},
		{
			name: "percent-encodes credential special characters",
			cfg: Postgres{
				Host: "db.internal", Port: 6543,
				User: "denta@core", Password: "p@ss w/ord",
				Database: "dentacore", SSLMode: "require",
			},
			want: "postgres://denta%40core:p%40ss%20w%2Ford@db.internal:6543/dentacore?sslmode=require",
		},
		{
			name: "empty password is allowed for local trust auth",
			cfg: Postgres{
				Host: "localhost", Port: 5432,
				User: "dentacore", Database: "dentacore", SSLMode: "disable",
			},
			want: "postgres://dentacore:@localhost:5432/dentacore?sslmode=disable",
		},
		{
			name: "explicit URL wins over discrete fields",
			cfg: Postgres{
				URL:  "postgres://u:p@db.internal:6543/dentacore?sslmode=verify-full",
				Host: "ignored", Port: 1, User: "ignored", Password: "ignored",
				Database: "ignored", SSLMode: "disable",
			},
			want: "postgres://u:p@db.internal:6543/dentacore?sslmode=verify-full",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.cfg.DSN())
		})
	}
}

func TestHTTPAddr(t *testing.T) {
	tests := []struct {
		name string
		http HTTP
		want string
	}{
		{name: "ipv4", http: HTTP{Host: "0.0.0.0", Port: 8080}, want: "0.0.0.0:8080"},
		{name: "localhost", http: HTTP{Host: "localhost", Port: 443}, want: "localhost:443"},
		{name: "ipv6 is bracketed", http: HTTP{Host: "::1", Port: 8080}, want: "[::1]:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.http.Addr())
		})
	}
}

func TestEnvironmentIsProduction(t *testing.T) {
	tests := []struct {
		env  Environment
		want bool
	}{
		{env: EnvProduction, want: true},
		{env: EnvStaging, want: false},
		{env: EnvDevelopment, want: false},
		{env: EnvTest, want: false},
		{env: Environment("unknown"), want: false},
	}

	for _, tt := range tests {
		t.Run(string(tt.env), func(t *testing.T) {
			assert.Equal(t, tt.want, tt.env.IsProduction())
		})
	}
}

func TestParseEnvironment(t *testing.T) {
	tests := []struct {
		raw     string
		want    Environment
		wantErr bool
	}{
		{raw: "development", want: EnvDevelopment},
		{raw: "staging", want: EnvStaging},
		{raw: "production", want: EnvProduction},
		{raw: "test", want: EnvTest},
		{raw: "PRODUCTION", want: EnvProduction},
		{raw: "", wantErr: true},
		{raw: "prod", wantErr: true},
	}

	for _, tt := range tests {
		t.Run("raw="+tt.raw, func(t *testing.T) {
			got, err := parseEnvironment(tt.raw)

			if tt.wantErr {
				require.Error(t, err)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetIntRejectsPartialNumbers(t *testing.T) {
	t.Setenv("TEST_INT", "12ab")

	_, err := getInt("TEST_INT", 7)

	require.Error(t, err)
	assert.ErrorContains(t, err, `must be an integer, got "12ab"`)
}
