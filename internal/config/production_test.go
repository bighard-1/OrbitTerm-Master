package config

import (
	"strings"
	"testing"
)

func TestProductionRejectsDemoCredentials(t *testing.T) {
	base := Config{AppEnv: "production", JWTSecret: "a-unique-production-key-that-is-long-enough", DatabaseURL: "postgres://orbit:unusually-long-database-credential@localhost/orbit"}
	for name, change := range map[string]func(*Config){
		"default JWT":               func(c *Config) { c.JWTSecret = "replace-this-with-a-strong-secret" },
		"short JWT":                 func(c *Config) { c.JWTSecret = "short" },
		"repeated JWT":              func(c *Config) { c.JWTSecret = strings.Repeat("a", 64) },
		"default database":          func(c *Config) { c.DatabaseURL = "host=localhost user=postgres password=postgres dbname=orbit" },
		"missing database password": func(c *Config) { c.DatabaseURL = "postgres://orbit@localhost/orbit" },
		"demo bootstrap":            func(c *Config) { c.AdminBootstrapToken = "replace-with-strong-admin-bootstrap-token" },
		"unknown mode":              func(c *Config) { c.AppEnv = "prodution" },
	} {
		t.Run(name, func(t *testing.T) {
			c := base
			change(&c)
			if err := c.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid production config: %v", err)
	}
	base.AppEnv = "development"
	base.JWTSecret = "replace-this-with-a-strong-secret"
	if err := base.Validate(); err != nil {
		t.Fatalf("development config: %v", err)
	}
}
