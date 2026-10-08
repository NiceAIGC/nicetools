package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port                                                                       string
	DBPath, StaticDir, CatalogPath, AdminUsername, AdminPassword, PublicOrigin string
	CookieSecure                                                               bool
}

func Load() (Config, error) {
	c := Config{
		Port: value("PORT", "8080"), DBPath: value("DB_PATH", "./data/nicetools.db"),
		StaticDir: value("STATIC_DIR", "../dist"), CatalogPath: value("TOOL_CATALOG", "../src/tools/catalog.json"),
		AdminUsername: value("ADMIN_USERNAME", "admin"), AdminPassword: os.Getenv("ADMIN_PASSWORD"),
		PublicOrigin: os.Getenv("PUBLIC_ORIGIN"),
	}
	port, err := strconv.Atoi(c.Port)
	if err != nil || port < 1 || port > 65535 {
		return c, fmt.Errorf("PORT must be 1..65535")
	}
	if raw := os.Getenv("COOKIE_SECURE"); raw != "" {
		c.CookieSecure, err = strconv.ParseBool(raw)
		if err != nil {
			return c, fmt.Errorf("COOKIE_SECURE must be true or false")
		}
	}
	if c.PublicOrigin != "" {
		u, err := url.Parse(c.PublicOrigin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return c, fmt.Errorf("PUBLIC_ORIGIN must be an http(s) origin")
		}
		c.PublicOrigin = strings.TrimSuffix(c.PublicOrigin, "/")
		if u.Scheme == "https" && !c.CookieSecure {
			return c, fmt.Errorf("HTTPS PUBLIC_ORIGIN requires COOKIE_SECURE=true")
		}
	}
	return c, nil
}

func value(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
