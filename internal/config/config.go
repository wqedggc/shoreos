package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr             string
	MySQLUser            string
	MySQLPassword        string
	MySQLHost            string
	MySQLPort            string
	MySQLDatabase        string
	MySQLSocket          string
	MySQLDefaultsFile    string
	AdminUsername        string
	AdminPassword        string
	AdminDisplay         string
	SessionTTL           time.Duration
	LoginMaxFailures     int
	LoginFailureWindow   time.Duration
	EnableLocalBootstrap bool
	EnableRegistration   bool
	WechatAppID          string
	WechatSecret         string
	IdentityBindTTL      time.Duration
	LedgerStorageDir     string
	LedgerPythonBin      string
	LedgerWorkerPath     string
}

func Load() Config {
	cfg := Config{
		HTTPAddr:             env("SHOREOS_HTTP_ADDR", "127.0.0.1:8090"),
		MySQLUser:            os.Getenv("SHOREOS_MYSQL_USER"),
		MySQLPassword:        os.Getenv("SHOREOS_MYSQL_PASSWORD"),
		MySQLHost:            os.Getenv("SHOREOS_MYSQL_HOST"),
		MySQLPort:            os.Getenv("SHOREOS_MYSQL_PORT"),
		MySQLDatabase:        os.Getenv("SHOREOS_MYSQL_DATABASE"),
		MySQLSocket:          os.Getenv("SHOREOS_MYSQL_SOCKET"),
		MySQLDefaultsFile:    os.Getenv("SHOREOS_MYSQL_DEFAULTS_FILE"),
		AdminUsername:        env("SHOREOS_ADMIN_USERNAME", "shore"),
		AdminPassword:        env("SHOREOS_ADMIN_PASSWORD", ""),
		AdminDisplay:         env("SHOREOS_ADMIN_DISPLAY_NAME", "Shore"),
		SessionTTL:           durationEnv("SHOREOS_SESSION_TTL", 7*24*time.Hour),
		LoginMaxFailures:     positiveIntEnv("SHOREOS_LOGIN_MAX_FAILURES", 8),
		LoginFailureWindow:   durationEnv("SHOREOS_LOGIN_FAILURE_WINDOW", 15*time.Minute),
		EnableLocalBootstrap: envBool("SHOREOS_ENABLE_LOCAL_BOOTSTRAP", false),
		EnableRegistration:   envBool("SHOREOS_ENABLE_REGISTRATION", true),
		WechatAppID:          env("SHOREOS_WECHAT_APPID", ""),
		WechatSecret:         env("SHOREOS_WECHAT_SECRET", ""),
		IdentityBindTTL:      durationEnv("SHOREOS_IDENTITY_BIND_TTL", 10*time.Minute),
		LedgerStorageDir:     env("SHOREOS_LEDGER_STORAGE_DIR", "storage/ledger"),
		LedgerPythonBin:      env("SHOREOS_LEDGER_PYTHON_BIN", defaultLedgerPythonBin()),
		LedgerWorkerPath:     env("SHOREOS_LEDGER_WORKER_PATH", "tools/ledger_importer.py"),
	}
	if cfg.MySQLDefaultsFile != "" {
		defaults, err := loadMySQLDefaultsFile(cfg.MySQLDefaultsFile)
		if err == nil {
			cfg.MySQLUser = first(cfg.MySQLUser, defaults["user"])
			cfg.MySQLPassword = first(cfg.MySQLPassword, defaults["password"])
			cfg.MySQLHost = first(cfg.MySQLHost, defaults["host"])
			cfg.MySQLPort = first(cfg.MySQLPort, defaults["port"])
			cfg.MySQLDatabase = first(cfg.MySQLDatabase, defaults["database"])
			cfg.MySQLSocket = first(cfg.MySQLSocket, defaults["socket"])
		}
	}
	cfg.MySQLUser = first(cfg.MySQLUser, "root")
	cfg.MySQLHost = first(cfg.MySQLHost, "127.0.0.1")
	cfg.MySQLPort = first(cfg.MySQLPort, "3306")
	cfg.MySQLDatabase = first(cfg.MySQLDatabase, "shoreos")
	return cfg
}

func defaultLedgerPythonBin() string {
	const localLedgerPython = ".venv-ledger/bin/python"
	if info, err := os.Stat(localLedgerPython); err == nil && !info.IsDir() {
		return localLedgerPython
	}
	return "python3"
}

func loadMySQLDefaultsFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	values := make(map[string]string)
	inClientSection := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inClientSection = strings.EqualFold(strings.TrimSpace(line[1:len(line)-1]), "client")
			continue
		}
		if !inClientSection {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		switch key {
		case "user", "password", "host", "port", "database", "socket":
			values[key] = strings.TrimSpace(value)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func first(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func (c Config) DSN() string {
	network := "tcp"
	address := fmt.Sprintf("%s:%s", c.MySQLHost, c.MySQLPort)
	if c.MySQLSocket != "" {
		network = "unix"
		address = c.MySQLSocket
	}
	return fmt.Sprintf(
		"%s:%s@%s(%s)/%s?charset=utf8mb4&parseTime=true&loc=Local&multiStatements=true",
		c.MySQLUser,
		c.MySQLPassword,
		network,
		address,
		c.MySQLDatabase,
	)
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func positiveIntEnv(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
}
