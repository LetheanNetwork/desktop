// SPDX-Licence-Identifier: EUPL-1.2

// Package apikey owns the per-Mac bearer token lthn issues to local
// SDK clients (Claude Code, Raycast, OpenCode, Codex, …). The token
// fronts every request that enters lthn's HTTP surface and is the
// single string a client needs to authenticate against the local
// API — same shape OpenAI clients already understand
// (`Authorization: Bearer sk-lthn-…`).
//
// Lifecycle:
//
//   - First launch: GenerateOrLoad() finds no key in config, generates
//     a fresh sk-lthn-<32 hex> via crypto/rand, persists via
//     dappco.re/go/config, returns it.
//   - Subsequent launches: GenerateOrLoad() finds the key in config,
//     returns it unchanged.
//   - Rotate(): generates a new key, persists, returns. The old key
//     stops working at the next server restart (Bearer middleware is
//     constructed at Engine build time with the value the package
//     held when New(...) was called).
//
// The Wails-bindable variant lives in pkg/apikey/wails.go.
//
// Usage example:
//
//	c := core.New()
//	if rr := config.RegisterFromFile(c, ""); !rr.OK { return rr }
//	key, r := apikey.GenerateOrLoad(c)
//	if !r.OK { return r }
//	srv := server.NewService(server.Options{LocalKey: key})

package apikey

import (
	"crypto/rand"
	"encoding/hex"

	core "dappco.re/go"
	"dappco.re/go/config"
)

// ConfigKey is the dot-path the API key persists under via
// dappco.re/go/config. Stable across launches.
const ConfigKey = "api.key"

// keyPrefix is the human-facing prefix every lthn-issued bearer
// token carries. Mirrors OpenAI's "sk-…" convention so client
// configs need no special-case handling.
const keyPrefix = "sk-lthn-"

// keyEntropyBytes is the random-byte length backing the suffix.
// 16 bytes → 32 hex chars → 128 bits of entropy, comfortably above
// the typical 80-bit-secret minimum.
const keyEntropyBytes = 16

// GenerateOrLoad resolves the local API key. Returns the existing
// value when config carries one, or generates + persists + returns
// a fresh key otherwise. Idempotent across launches.
//
// Usage example:
//
//	key, r := apikey.GenerateOrLoad(c)
//	if !r.OK { return r }
func GenerateOrLoad(c *core.Core) (string, core.Result) {
	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	if !ok || cfg == nil {
		return "", core.Fail(core.E("apikey.GenerateOrLoad", "config service not registered", nil))
	}
	var existing string
	if r := cfg.Get(ConfigKey, &existing); r.OK && existing != "" {
		return existing, core.Ok(existing)
	}
	return generateAndStore(cfg)
}

// Rotate generates a fresh key, persists it, and returns the new
// value. The previous key remains valid until the server engine is
// rebuilt — typically at next launch.
//
// Usage example:
//
//	newKey, r := apikey.Rotate(c)
//	if !r.OK { return r }
//	// surface newKey to the user; next restart applies it
func Rotate(c *core.Core) (string, core.Result) {
	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	if !ok || cfg == nil {
		return "", core.Fail(core.E("apikey.Rotate", "config service not registered", nil))
	}
	return generateAndStore(cfg)
}

// generateAndStore mints a fresh key, writes it to config, commits.
// Shared by GenerateOrLoad's first-launch path and Rotate's
// regeneration path so the persistence shape stays single-source.
func generateAndStore(cfg *config.Service) (string, core.Result) {
	key, err := generate()
	if err != nil {
		return "", core.Fail(core.E("apikey.generateAndStore", "entropy read failed", err))
	}
	if r := cfg.Set(ConfigKey, key); !r.OK {
		return "", r
	}
	if r := cfg.Commit(); !r.OK {
		return "", r
	}
	return key, core.Ok(key)
}

// generate builds a fresh sk-lthn-<32 hex> string from crypto/rand.
// 16 random bytes hex-encoded = 32 chars, prepended with the
// keyPrefix. Returns an error only if the underlying entropy read
// fails — extremely rare on a healthy system.
func generate() (string, error) {
	buf := make([]byte, keyEntropyBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return keyPrefix + hex.EncodeToString(buf), nil
}

// Mask returns a UI-safe form of the key — full prefix + first 4
// suffix chars + 4 bullets + last 4 suffix chars. Used by panels
// that want to show "the key exists" without leaking it on screen.
// Returns "" for "" so empty config doesn't render a meaningless
// mask.
//
// Usage example:
//
//	apikey.Mask("sk-lthn-0011223344556677889aabbccddeeff")
//	// → "sk-lthn-0011••••••••••••••••eeff"
func Mask(key string) string {
	if key == "" {
		return ""
	}
	if len(key) < 12 {
		return key
	}
	const headRunes = 4
	const tailRunes = 4
	mid := len(key) - len(keyPrefix) - headRunes - tailRunes
	if mid < 4 {
		return key
	}
	bullets := ""
	for i := 0; i < mid; i++ {
		bullets += "•"
	}
	return key[:len(keyPrefix)+headRunes] + bullets + key[len(key)-tailRunes:]
}
