// Package config loads and validates application configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/NumerR/ElitStom/internal/domain"
)

// ErrInvalidConfig wraps every validation failure reported by Load.
var ErrInvalidConfig = errors.New("invalid configuration")

type Environment string

const (
	EnvDevelopment Environment = "development"
	EnvStaging     Environment = "staging"
	EnvProduction  Environment = "production"
	EnvTest        Environment = "test"
)

func (e Environment) IsProduction() bool { return e == EnvProduction }

const (
	LogFormatJSON = "json"
	LogFormatText = "text"
)

var sslModes = map[string]struct{}{
	"disable":     {},
	"allow":       {},
	"prefer":      {},
	"require":     {},
	"verify-ca":   {},
	"verify-full": {},
}

type Config struct {
	Env      Environment
	HTTP     HTTP
	Postgres Postgres
	Log      Log
	Auth     Auth
	Clinic   Clinic
}

// Clinic carries the booking calendar as configured. Load has already proven that
// Schedule succeeds, so callers may treat its error as unreachable only after Load.
type Clinic struct {
	Timezone    string
	OpensAt     string
	ClosesAt    string
	Workdays    []int
	Slot        time.Duration
	HorizonDays int
}

func (c Clinic) Schedule() (domain.Schedule, error) {
	return domain.NewSchedule(c.Timezone, c.OpensAt, c.ClosesAt, c.Workdays, c.Slot, c.HorizonDays)
}

// Auth groups the credential and token policy. JWTSecret is deliberately kept out of
// any String()/Log() helper: it signs sessions and must never be rendered.
type Auth struct {
	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	Issuer          string
	Audience        string
	BcryptCost      int
}

type HTTP struct {
	Host              string
	Port              int
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

func (h HTTP) Addr() string { return net.JoinHostPort(h.Host, strconv.Itoa(h.Port)) }

type Postgres struct {
	URL              string
	Host             string
	Port             int
	User             string
	Password         string
	Database         string
	SSLMode          string
	MaxConns         int32
	MinConns         int32
	MaxConnLifetime  time.Duration
	MaxConnIdleTime  time.Duration
	ConnectTimeout   time.Duration
	StatementTimeout time.Duration
}

// DSN builds the connection string. An explicit DATABASE_URL wins over the discrete PG_* fields.
// It never fails: malformed values are caught by pgxpool.ParseConfig at pool creation time.
func (p Postgres) DSN() string {
	if p.URL != "" {
		return p.URL
	}
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(p.User, p.Password),
		Host:     net.JoinHostPort(p.Host, strconv.Itoa(p.Port)),
		Path:     p.Database,
		RawQuery: url.Values{"sslmode": {p.SSLMode}}.Encode(),
	}
	return u.String()
}

type Log struct {
	Level  slog.Level
	Format string
}

// Load reads the process environment and returns a fully populated Config.
// All problems are collected and reported together instead of failing on the first one.
func Load() (Config, error) {
	env, envErr := parseEnvironment(getString("APP_ENV", string(EnvDevelopment)))
	logCfg, logErr := loadLog()
	httpCfg, httpErr := loadHTTP()
	pgCfg, pgErr := loadPostgres(env)
	authCfg, authErr := loadAuth()
	clinicCfg, clinicErr := loadClinic()

	if err := errors.Join(envErr, logErr, httpErr, pgErr, authErr, clinicErr); err != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrInvalidConfig, err)
	}
	return Config{Env: env, HTTP: httpCfg, Postgres: pgCfg, Log: logCfg, Auth: authCfg, Clinic: clinicCfg}, nil
}

func parseEnvironment(raw string) (Environment, error) {
	switch env := Environment(strings.ToLower(raw)); env {
	case EnvDevelopment, EnvStaging, EnvProduction, EnvTest:
		return env, nil
	default:
		return "", fmt.Errorf("APP_ENV: unsupported value %q, want one of %s, %s, %s, %s",
			raw, EnvDevelopment, EnvStaging, EnvProduction, EnvTest)
	}
}

func loadLog() (Log, error) {
	var errs []error

	var level slog.Level
	if err := level.UnmarshalText([]byte(getString("LOG_LEVEL", "info"))); err != nil {
		errs = append(errs, fmt.Errorf("LOG_LEVEL: %w", err))
	}

	format := strings.ToLower(getString("LOG_FORMAT", LogFormatJSON))
	switch format {
	case LogFormatJSON, LogFormatText:
	default:
		errs = append(errs, fmt.Errorf("LOG_FORMAT: unsupported value %q, want %q or %q", format, LogFormatJSON, LogFormatText))
	}

	return Log{Level: level, Format: format}, errors.Join(errs...)
}

func loadHTTP() (HTTP, error) {
	var errs []error

	cfg := HTTP{Host: getString("HTTP_HOST", "0.0.0.0")}

	port, err := getInt("HTTP_PORT", 8080)
	if err != nil {
		errs = append(errs, fmt.Errorf("HTTP_PORT: %w", err))
	} else if port < 1 || port > 65535 {
		errs = append(errs, fmt.Errorf("HTTP_PORT: %d is out of range 1..65535", port))
	}
	cfg.Port = port

	cfg.ReadHeaderTimeout, err = getDuration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second)
	if err != nil {
		errs = append(errs, fmt.Errorf("HTTP_READ_HEADER_TIMEOUT: %w", err))
	}
	cfg.ReadTimeout, err = getDuration("HTTP_READ_TIMEOUT", 10*time.Second)
	if err != nil {
		errs = append(errs, fmt.Errorf("HTTP_READ_TIMEOUT: %w", err))
	}
	cfg.WriteTimeout, err = getDuration("HTTP_WRITE_TIMEOUT", 15*time.Second)
	if err != nil {
		errs = append(errs, fmt.Errorf("HTTP_WRITE_TIMEOUT: %w", err))
	}
	cfg.IdleTimeout, err = getDuration("HTTP_IDLE_TIMEOUT", 60*time.Second)
	if err != nil {
		errs = append(errs, fmt.Errorf("HTTP_IDLE_TIMEOUT: %w", err))
	}
	cfg.ShutdownTimeout, err = getDuration("HTTP_SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		errs = append(errs, fmt.Errorf("HTTP_SHUTDOWN_TIMEOUT: %w", err))
	}

	for _, d := range []struct {
		name string
		val  time.Duration
	}{
		{"HTTP_READ_HEADER_TIMEOUT", cfg.ReadHeaderTimeout},
		{"HTTP_READ_TIMEOUT", cfg.ReadTimeout},
		{"HTTP_WRITE_TIMEOUT", cfg.WriteTimeout},
		{"HTTP_SHUTDOWN_TIMEOUT", cfg.ShutdownTimeout},
	} {
		if d.val <= 0 {
			errs = append(errs, fmt.Errorf("%s: must be greater than zero, got %s", d.name, d.val))
		}
	}
	if cfg.IdleTimeout < 0 {
		errs = append(errs, fmt.Errorf("HTTP_IDLE_TIMEOUT: must not be negative, got %s", cfg.IdleTimeout))
	}

	return cfg, errors.Join(errs...)
}

func loadPostgres(env Environment) (Postgres, error) {
	var errs []error

	cfg := Postgres{
		URL:      getString("DATABASE_URL", ""),
		Host:     getString("PG_HOST", "localhost"),
		User:     getString("PG_USER", ""),
		Password: os.Getenv("PG_PASSWORD"), // not trimmed: whitespace is significant in a password
		Database: getString("PG_DATABASE", ""),
		SSLMode:  getString("PG_SSLMODE", "disable"),
	}

	port, err := getInt("PG_PORT", 5432)
	if err != nil {
		errs = append(errs, fmt.Errorf("PG_PORT: %w", err))
	} else if port < 1 || port > 65535 {
		errs = append(errs, fmt.Errorf("PG_PORT: %d is out of range 1..65535", port))
	}
	cfg.Port = port

	if cfg.URL == "" {
		if cfg.User == "" {
			errs = append(errs, errors.New("PG_USER: required when DATABASE_URL is not set"))
		}
		if cfg.Database == "" {
			errs = append(errs, errors.New("PG_DATABASE: required when DATABASE_URL is not set"))
		}
		if _, ok := sslModes[cfg.SSLMode]; !ok {
			errs = append(errs, fmt.Errorf("PG_SSLMODE: unsupported value %q", cfg.SSLMode))
		}
		// Plaintext credentials over the wire are only tolerable outside production.
		if env.IsProduction() && cfg.SSLMode == "disable" {
			errs = append(errs, errors.New("PG_SSLMODE: must not be \"disable\" in production"))
		}
	}

	cfg.MaxConns, err = getInt32("PG_POOL_MAX_CONNS", 10)
	if err != nil {
		errs = append(errs, fmt.Errorf("PG_POOL_MAX_CONNS: %w", err))
	}
	cfg.MinConns, err = getInt32("PG_POOL_MIN_CONNS", 0)
	if err != nil {
		errs = append(errs, fmt.Errorf("PG_POOL_MIN_CONNS: %w", err))
	}
	if cfg.MaxConns <= 0 {
		errs = append(errs, fmt.Errorf("PG_POOL_MAX_CONNS: must be greater than zero, got %d", cfg.MaxConns))
	}
	if cfg.MinConns < 0 {
		errs = append(errs, fmt.Errorf("PG_POOL_MIN_CONNS: must not be negative, got %d", cfg.MinConns))
	}
	if cfg.MinConns > cfg.MaxConns {
		errs = append(errs, fmt.Errorf("PG_POOL_MIN_CONNS (%d) must not exceed PG_POOL_MAX_CONNS (%d)", cfg.MinConns, cfg.MaxConns))
	}

	cfg.MaxConnLifetime, err = getDuration("PG_CONN_MAX_LIFETIME", time.Hour)
	if err != nil {
		errs = append(errs, fmt.Errorf("PG_CONN_MAX_LIFETIME: %w", err))
	}
	cfg.MaxConnIdleTime, err = getDuration("PG_CONN_MAX_IDLE_TIME", 15*time.Minute)
	if err != nil {
		errs = append(errs, fmt.Errorf("PG_CONN_MAX_IDLE_TIME: %w", err))
	}
	cfg.ConnectTimeout, err = getDuration("PG_CONNECT_TIMEOUT", 5*time.Second)
	if err != nil {
		errs = append(errs, fmt.Errorf("PG_CONNECT_TIMEOUT: %w", err))
	}
	if cfg.ConnectTimeout <= 0 {
		errs = append(errs, fmt.Errorf("PG_CONNECT_TIMEOUT: must be greater than zero, got %s", cfg.ConnectTimeout))
	}

	// Server-side, so it still bounds a query when the Go context carries no deadline.
	cfg.StatementTimeout, err = getDuration("PG_STATEMENT_TIMEOUT", 5*time.Second)
	if err != nil {
		errs = append(errs, fmt.Errorf("PG_STATEMENT_TIMEOUT: %w", err))
	}
	if cfg.StatementTimeout < 0 {
		errs = append(errs, fmt.Errorf("PG_STATEMENT_TIMEOUT: must not be negative, got %s", cfg.StatementTimeout))
	}

	return cfg, errors.Join(errs...)
}

// HS256 signs with a symmetric key, so anything shorter than 256 bits is brute-forceable
// offline once one token is captured. bcrypt above 15 cost makes login unusably slow.
const (
	minJWTSecretBytes = 32
	minBcryptCost     = 10
	maxBcryptCost     = 15
)

func loadAuth() (Auth, error) {
	var errs []error

	cfg := Auth{
		// Byte-exact on purpose: a secret must never be silently reformatted.
		JWTSecret: os.Getenv("JWT_SECRET"),
		Issuer:    getString("JWT_ISSUER", "dentacore"),
		Audience:  getString("JWT_AUDIENCE", "dentacore-api"),
	}

	if cfg.JWTSecret == "" {
		errs = append(errs, errors.New("JWT_SECRET: required"))
	} else if len(cfg.JWTSecret) < minJWTSecretBytes {
		errs = append(errs, fmt.Errorf("JWT_SECRET: must be at least %d bytes for HS256, got %d", minJWTSecretBytes, len(cfg.JWTSecret)))
	}

	var err error
	cfg.AccessTokenTTL, err = getDuration("JWT_ACCESS_TTL", 15*time.Minute)
	if err != nil {
		errs = append(errs, fmt.Errorf("JWT_ACCESS_TTL: %w", err))
	}
	cfg.RefreshTokenTTL, err = getDuration("JWT_REFRESH_TTL", 720*time.Hour)
	if err != nil {
		errs = append(errs, fmt.Errorf("JWT_REFRESH_TTL: %w", err))
	}
	cfg.BcryptCost, err = getInt("BCRYPT_COST", 12)
	if err != nil {
		errs = append(errs, fmt.Errorf("BCRYPT_COST: %w", err))
	}

	if cfg.AccessTokenTTL <= 0 {
		errs = append(errs, fmt.Errorf("JWT_ACCESS_TTL: must be greater than zero, got %s", cfg.AccessTokenTTL))
	}
	if cfg.RefreshTokenTTL <= cfg.AccessTokenTTL {
		errs = append(errs, fmt.Errorf("JWT_REFRESH_TTL (%s) must exceed JWT_ACCESS_TTL (%s)", cfg.RefreshTokenTTL, cfg.AccessTokenTTL))
	}
	if cfg.BcryptCost < minBcryptCost || cfg.BcryptCost > maxBcryptCost {
		errs = append(errs, fmt.Errorf("BCRYPT_COST: must be between %d and %d, got %d", minBcryptCost, maxBcryptCost, cfg.BcryptCost))
	}

	return cfg, errors.Join(errs...)
}

var clinicEnvKeys = map[string]string{
	"timezone":             "CLINIC_TIMEZONE",
	"opens_at":             "CLINIC_OPENS_AT",
	"closes_at":            "CLINIC_CLOSES_AT",
	"workdays":             "CLINIC_WORKDAYS",
	"slot":                 "APPOINTMENT_SLOT",
	"booking_horizon_days": "BOOKING_HORIZON_DAYS",
}

func loadClinic() (Clinic, error) {
	var errs []error

	cfg := Clinic{
		Timezone: getString("CLINIC_TIMEZONE", "Europe/Moscow"),
		OpensAt:  getString("CLINIC_OPENS_AT", "09:00"),
		ClosesAt: getString("CLINIC_CLOSES_AT", "20:00"),
	}

	workdays, err := getIntList("CLINIC_WORKDAYS", []int{1, 2, 3, 4, 5, 6})
	if err != nil {
		errs = append(errs, fmt.Errorf("CLINIC_WORKDAYS: %w", err))
	}
	cfg.Workdays = workdays

	cfg.Slot, err = getDuration("APPOINTMENT_SLOT", 30*time.Minute)
	if err != nil {
		errs = append(errs, fmt.Errorf("APPOINTMENT_SLOT: %w", err))
	}
	cfg.HorizonDays, err = getInt("BOOKING_HORIZON_DAYS", 60)
	if err != nil {
		errs = append(errs, fmt.Errorf("BOOKING_HORIZON_DAYS: %w", err))
	}

	// Parse failures above already explain themselves; re-validating defaults would only add noise.
	if len(errs) == 0 {
		if _, err := cfg.Schedule(); err != nil {
			var ve *domain.ValidationError
			if !errors.As(err, &ve) {
				return cfg, fmt.Errorf("clinic schedule: %w", err)
			}
			for _, f := range ve.Fields {
				errs = append(errs, fmt.Errorf("%s: %s", clinicEnvKeys[f.Field], f.Message))
			}
		}
	}

	return cfg, errors.Join(errs...)
}

func getString(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func getInt(key string, def int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def, fmt.Errorf("must be an integer, got %q", raw)
	}
	return v, nil
}

func getInt32(key string, def int32) (int32, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def, nil
	}
	v, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return def, fmt.Errorf("must be an integer, got %q", raw)
	}
	return int32(v), nil
}

func getIntList(key string, def []int) ([]int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		v, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return def, fmt.Errorf("must be a comma-separated list of integers, got %q", raw)
		}
		out = append(out, v)
	}
	return out, nil
}

func getDuration(key string, def time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def, nil
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		return def, fmt.Errorf("must be a duration like 5s or 1m30s, got %q", raw)
	}
	return v, nil
}
