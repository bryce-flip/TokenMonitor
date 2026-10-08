package config

import (
	"strings"
	"testing"
)

// validConfig is the minimal config that must pass Validate.
func validConfig() *Config {
	return &Config{
		ChainID:            1,
		ConfirmationBlocks: 20,
		Tokens: []Token{{
			Symbol:   "USDT",
			Issuer:   "Tether",
			Contract: "0xdac17f958d2ee523a2206206994597c13d831ec7",
			Decimals: 6,
		}},
	}
}

// TestValidateSymbolFailFast requires Validate to reject empty and
// unrecognized symbols at the config trust boundary (WR-03): a typo'd JSON
// key yields an empty symbol and an unknown symbol has no fetch/decode path,
// so both must fail the run at startup, not skip the token mid-run.
func TestValidateSymbolFailFast(t *testing.T) {
	cases := []struct {
		name    string
		symbol  string
		wantErr string
	}{
		{"empty symbol (typo'd JSON key)", "", "empty symbol"},
		{"whitespace-only symbol", "   ", "empty symbol"},
		{"padded known symbol", " USDT", "not supported"},
		{"unknown symbol", "DAI", "not supported"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.Tokens[0].Symbol = tc.symbol
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("Validate with symbol %q: got nil error, want failure", tc.symbol)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Validate with symbol %q error = %q, want it to mention %q", tc.symbol, err.Error(), tc.wantErr)
			}
		})
	}
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("valid config must still pass: %v", err)
	}
}
