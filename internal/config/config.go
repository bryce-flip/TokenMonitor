// Package config carries operator-supplied configuration across the indexer:
// token contract metadata, confirmation policy, and environment lookups.
// Values come from config/tokens.json and the environment (D-01); nothing is
// embedded in business logic.
package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

// Token describes one configured stablecoin contract (requirements.md §3).
type Token struct {
	Symbol   string `json:"symbol"`
	Issuer   string `json:"issuer"`
	Contract string `json:"contract"`
	Decimals uint8  `json:"decimals"`
}

// Config is the root of config/tokens.json.
type Config struct {
	ChainID            int64   `json:"chain_id"`
	ConfirmationBlocks uint64  `json:"confirmation_blocks"`
	Tokens             []Token `json:"tokens"`
}

// Load reads and parses the JSON config at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config load %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config parse %s: %w", path, err)
	}
	return &cfg, nil
}

// supportedSymbols mirrors the token set indexer.SupplyTopics classifies
// (USDT and USDC, requirements.md §2-3). It lives here because this package
// cannot import internal/indexer (which imports config). A symbol outside
// this set has no fetch/decode path, so it must fail at startup instead of
// being silently skipped mid-run.
var supportedSymbols = map[string]bool{"USDT": true, "USDC": true}

// Validate enforces the Phase 1 invariants: Mainnet only, a positive
// confirmation depth, at least one well-formed, supported-symbol, uniquely
// named token.
func (c *Config) Validate() error {
	if c.ChainID != 1 {
		return fmt.Errorf("config: chain_id must be 1 (Ethereum Mainnet), got %d", c.ChainID)
	}
	if c.ConfirmationBlocks == 0 {
		return fmt.Errorf("config: confirmation_blocks must be > 0")
	}
	if len(c.Tokens) == 0 {
		return fmt.Errorf("config: at least one token is required")
	}
	seen := make(map[string]bool, len(c.Tokens))
	for _, t := range c.Tokens {
		if strings.TrimSpace(t.Symbol) == "" {
			return fmt.Errorf("config: token with contract %s has an empty symbol", t.Contract)
		}
		if !supportedSymbols[t.Symbol] {
			return fmt.Errorf("config: token symbol %q (contract %s) is not supported; supported symbols: USDT, USDC", t.Symbol, t.Contract)
		}
		if seen[t.Symbol] {
			return fmt.Errorf("config: duplicate token symbol %q", t.Symbol)
		}
		seen[t.Symbol] = true
		if !common.IsHexAddress(t.Contract) {
			return fmt.Errorf("config: token %s contract %q is not a valid hex address", t.Symbol, t.Contract)
		}
		if t.Decimals > 18 {
			return fmt.Errorf("config: token %s decimals %d out of range 0..18", t.Symbol, t.Decimals)
		}
	}
	return nil
}

// RPCURL returns the operator-supplied Ethereum Mainnet HTTP endpoint from
// ETH_RPC_URL (D-01: external, never embedded in source).
func RPCURL() (string, error) {
	raw := strings.TrimSpace(os.Getenv("ETH_RPC_URL"))
	if raw == "" {
		return "", fmt.Errorf("ETH_RPC_URL is not set: export ETH_RPC_URL with an Ethereum Mainnet HTTP endpoint (provider dashboard -> API keys)")
	}
	return raw, nil
}

// ClickHouseURL returns the ClickHouse DSN from CLICKHOUSE_URL, defaulting to
// the local container started for Phase 1 (127.0.0.1-bound only, T-01-03).
func ClickHouseURL() string {
	if u := strings.TrimSpace(os.Getenv("CLICKHOUSE_URL")); u != "" {
		return u
	}
	return "clickhouse://default@127.0.0.1:9000/default"
}

// HostOnly reduces a URL to scheme://host so error and log output never
// carries credentials embedded in the URL (D-01 hygiene).
func HostOnly(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "invalid-url"
	}
	return u.Scheme + "://" + u.Host
}
