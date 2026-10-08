// Package config carries operator-supplied configuration across the indexer:
// token contract metadata, confirmation policy, and environment lookups.
// Values come from config/tokens.json and the environment (D-01); nothing is
// embedded in business logic.
package config

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
