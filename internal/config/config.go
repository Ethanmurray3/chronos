// Package config resolves Chronos's runtime settings from flags and
// environment variables (flags win, then env, then defaults).
package config

import (
	"flag"
	"os"
)

// Config is the minimal runtime configuration for the single binary.
type Config struct {
	Addr   string // listen address, e.g. ":8787"
	DBPath string // path to the SQLite file
}

// Load parses flags/env once and returns the resolved config.
func Load() Config {
	addr := flag.String("addr", envOr("CHRONOS_ADDR", defaultAddr()), "listen address (host:port)")
	db := flag.String("db", envOr("CHRONOS_DB", "./chronos.db"), "path to the SQLite database file")
	flag.Parse()
	return Config{Addr: *addr, DBPath: *db}
}

// defaultAddr honours a bare PORT env (handy for PaaS) before falling back.
func defaultAddr() string {
	if p := os.Getenv("PORT"); p != "" {
		return ":" + p
	}
	return ":8787"
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
