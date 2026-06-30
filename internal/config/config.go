package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	HTTPAddr      string
	MySQLUser     string
	MySQLPassword string
	MySQLHost     string
	MySQLPort     string
	MySQLDatabase string
	MySQLSocket   string
	AdminUsername string
	AdminPassword string
	AdminDisplay  string
	SessionTTL    time.Duration
}

func Load() Config {
	return Config{
		HTTPAddr:      env("SHOREOS_HTTP_ADDR", ":8090"),
		MySQLUser:     env("SHOREOS_MYSQL_USER", "root"),
		MySQLPassword: env("SHOREOS_MYSQL_PASSWORD", ""),
		MySQLHost:     env("SHOREOS_MYSQL_HOST", "127.0.0.1"),
		MySQLPort:     env("SHOREOS_MYSQL_PORT", "3306"),
		MySQLDatabase: env("SHOREOS_MYSQL_DATABASE", "shoreos"),
		MySQLSocket:   env("SHOREOS_MYSQL_SOCKET", ""),
		AdminUsername: env("SHOREOS_ADMIN_USERNAME", "shore"),
		AdminPassword: env("SHOREOS_ADMIN_PASSWORD", "shoreos"),
		AdminDisplay:  env("SHOREOS_ADMIN_DISPLAY_NAME", "Shore"),
		SessionTTL:    30 * 24 * time.Hour,
	}
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
