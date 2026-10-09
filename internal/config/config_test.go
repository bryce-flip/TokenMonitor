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
		HeadPolicy:         "finalized",
		WindowBlocks:       5000,
		WindowFloor:        100,
		PollSeconds:        60,
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

// TestValidateHeadPolicyEnum pins the D-01 head policy surface: only
// "finalized" and "confirmed" are legal, and the error names head_policy so a
// typo'd config fails loudly at startup. Under "finalized" a zero
// confirmation_blocks is legal (the depth only applies to "confirmed").
func TestValidateHeadPolicyEnum(t *testing.T) {
	cfg := validConfig()
	cfg.HeadPolicy = "latest"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate with head_policy \"latest\": got nil error, want failure")
	}
	if !strings.Contains(err.Error(), "head_policy") {
		t.Errorf("error %q does not mention head_policy", err.Error())
	}

	// Zero confirmation_blocks is legal under finalized (D-01 relaxes the
	// Phase 1 unconditional requirement) but illegal under confirmed.
	relaxed := validConfig()
	relaxed.ConfirmationBlocks = 0
	if err := relaxed.Validate(); err != nil {
		t.Fatalf("finalized policy with confirmation_blocks 0 must pass: %v", err)
	}
	confirmed := validConfig()
	confirmed.HeadPolicy = "confirmed"
	confirmed.ConfirmationBlocks = 0
	if err := confirmed.Validate(); err == nil {
		t.Fatal("confirmed policy with confirmation_blocks 0 must fail")
	}
}

// TestValidateDuplicateContractRejected pins the IN-08 double-counting guard:
// two tokens sharing a contract address (case-insensitively) are rejected,
// because both fetch paths would store every event twice.
func TestValidateDuplicateContractRejected(t *testing.T) {
	cfg := validConfig()
	cfg.Tokens = append(cfg.Tokens, Token{
		Symbol:   "USDC",
		Issuer:   "Circle",
		Contract: "0xDAC17F958D2EE523A2206206994597C13D831EC7", // same contract, different case
		Decimals: 6,
	})
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate with two tokens on one contract: got nil error, want failure")
	}
	if !strings.Contains(err.Error(), "duplicate token contract address") {
		t.Errorf("error %q does not name the duplicate-contract guard", err.Error())
	}
}
