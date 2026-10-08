package indexer

import (
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"TOkenMonitor/internal/config"
)

const (
	usdtContract = "0xdAC17F958D2ee523a2206206994597C13D831ec7"
	usdcContract = "0xA0b86991c6218b36c1d19D4a2e9Eb0ce3606eb48"
)

// USDC paired corroboration topic0s (01-RESEARCH.md line 88): evidence only,
// never additional supply rows.
var (
	usdcMintTopic = common.HexToHash("0xab8530f87dc9b59234c4623bf917212bb2536d647574c8e7e5da92c2ede0c9f8")
	usdcBurnTopic = common.HexToHash("0xcc16f5dbb4873280815c1ee09dbd06736cffcc184412cf7a71a0fdb75d397ca5")
)

// word32 renders v as the exact 32-byte big-endian ABI word it occupies in
// non-indexed event data.
func word32(v *big.Int) []byte {
	b := make([]byte, 32)
	v.FillBytes(b)
	return b
}

// topicAddress renders an address as the 32-byte indexed-topic word.
func topicAddress(addr string) common.Hash {
	return common.BytesToHash(common.HexToAddress(addr).Bytes())
}

// addrWord renders an address as the left-padded 32-byte non-indexed word.
func addrWord(addr string) []byte {
	h := common.BytesToHash(common.HexToAddress(addr).Bytes())
	return h[:]
}

// concat joins byte slices into a fresh slice.
func concat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func usdtToken() config.Token {
	return config.Token{Symbol: "USDT", Issuer: "Tether", Contract: usdtContract, Decimals: 6}
}

func usdcToken() config.Token {
	return config.Token{Symbol: "USDC", Issuer: "Circle", Contract: usdcContract, Decimals: 6}
}

// TestDecodePinnedMainnetFixtures classifies the five deployed-contract
// receipts pinned in 01-RESEARCH.md lines 92-98 (EVENT-01, EVENT-02).
func TestDecodePinnedMainnetFixtures(t *testing.T) {
	const destroyedAddr = "0x532961b23a28a9e5a8b1769bdb3e8906fc7c7a01"
	const mintRecipient = "0x7860596511559633d317b6d7c0a58bc654c8279c"
	const burnSender = "0xd55125758c353586b488d9514cfb868f633c02d7"
	fixtures := []struct {
		name     string
		token    config.Token
		log      types.Log
		wantType string
		wantAmt  uint64
		wantFrom string
		wantTo   string
	}{
		{
			name:  "USDT Issue -> mint (tx 0x3a296e7b..., log 0x249)",
			token: usdtToken(),
			log: types.Log{
				Address:     common.HexToAddress(usdtContract),
				Topics:      []common.Hash{IssueTopic},
				Data:        word32(new(big.Int).SetUint64(1000000000000000)),
				BlockNumber: 0x167ab1a,
				TxHash:      common.HexToHash("0x3a296e7b8289666b5ea54e48d3da404fc0b337cc4d419276b5675395244b115c"),
				Index:       0x249,
			},
			wantType: EventMint,
			wantAmt:  1000000000000000,
		},
		{
			name:  "USDT Redeem -> redeem (tx 0x8642010e..., log 0x2ca)",
			token: usdtToken(),
			log: types.Log{
				Address:     common.HexToAddress(usdtContract),
				Topics:      []common.Hash{RedeemTopic},
				Data:        word32(new(big.Int).SetUint64(3000000000000000)),
				BlockNumber: 0x1726ef7,
				TxHash:      common.HexToHash("0x8642010ea16fda150d6db81b7e5a377453d825021452cc076a2fb6fbda524e1c"),
				Index:       0x2ca,
			},
			wantType: EventRedeem,
			wantAmt:  3000000000000000,
		},
		{
			name:  "USDT DestroyedBlackFunds -> destroyed_black_funds with destroyed address (tx 0x06abbfaa..., log 0x277)",
			token: usdtToken(),
			log: types.Log{
				Address:     common.HexToAddress(usdtContract),
				Topics:      []common.Hash{DestroyedBlackFundsTopic},
				Data:        concat(addrWord(destroyedAddr), word32(new(big.Int).SetUint64(200000000000))),
				BlockNumber: 0x15aac45,
				TxHash:      common.HexToHash("0x06abbfaa405699132123d74e1cdf712e42bf88fff00ed83b8a9a0a9f112c6055"),
				Index:       0x277,
			},
			wantType: EventDestroyedBlackFunds,
			wantAmt:  200000000000,
			wantFrom: destroyedAddr,
		},
		{
			name:  "USDC zero-from Transfer -> mint (tx 0xc7066b50..., log 0x2ad)",
			token: usdcToken(),
			log: types.Log{
				Address:     common.HexToAddress(usdcContract),
				Topics:      []common.Hash{TransferTopic, topicAddress("0x0000000000000000000000000000000000000000"), topicAddress(mintRecipient)},
				Data:        word32(new(big.Int).SetUint64(2000000000000)),
				BlockNumber: 0x1632fcb,
				TxHash:      common.HexToHash("0xc7066b502fa8321a090fbff8842f69d136e2d4c27849a04cbc1ed02cac98963e"),
				Index:       0x2ad,
			},
			wantType: EventMint,
			wantAmt:  2000000000000,
			wantTo:   mintRecipient,
		},
		{
			name:  "USDC zero-to Transfer -> burn (tx 0x4f52e25d..., log 0x162)",
			token: usdcToken(),
			log: types.Log{
				Address:     common.HexToAddress(usdcContract),
				Topics:      []common.Hash{TransferTopic, topicAddress(burnSender), topicAddress("0x0000000000000000000000000000000000000000")},
				Data:        word32(new(big.Int).SetUint64(1383937060000)),
				BlockNumber: 0x1638e19,
				TxHash:      common.HexToHash("0x4f52e25d39e006606f26de5a496eb7f9198c8b8467686f0a994d07d7a2a4ca9f"),
				Index:       0x162,
			},
			wantType: EventBurn,
			wantAmt:  1383937060000,
			wantFrom: burnSender,
		},
	}
	for _, tc := range fixtures {
		t.Run(tc.name, func(t *testing.T) {
			ev, err := DecodeSupplyEvent(tc.token, tc.log)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if ev == nil {
				t.Fatal("expected a supply event, got nil")
			}
			if ev.Token != tc.token.Symbol {
				t.Errorf("Token = %q, want %q", ev.Token, tc.token.Symbol)
			}
			if ev.EventType != tc.wantType {
				t.Errorf("EventType = %q, want %q", ev.EventType, tc.wantType)
			}
			if ev.Amount == nil || ev.Amount.Cmp(new(big.Int).SetUint64(tc.wantAmt)) != 0 {
				t.Errorf("Amount = %v, want %d", ev.Amount, tc.wantAmt)
			}
			if ev.FromAddress != tc.wantFrom {
				t.Errorf("FromAddress = %q, want %q", ev.FromAddress, tc.wantFrom)
			}
			if ev.ToAddress != tc.wantTo {
				t.Errorf("ToAddress = %q, want %q", ev.ToAddress, tc.wantTo)
			}
		})
	}
}

// TestDecodeNegativesYieldNoSupplyRow covers logs that must never produce a
// supply row: USDT zero-address Transfer, USDC ordinary Transfer, and USDC's
// paired Mint/Burn corroboration events.
func TestDecodeNegativesYieldNoSupplyRow(t *testing.T) {
	const someone = "0x7860596511559633d317b648d9514c0a58bc654c8279c"
	const other = "0xd55125758c353586b488d9514cfb868f633c02d7"
	cases := []struct {
		name  string
		token config.Token
		log   types.Log
	}{
		{
			name:  "USDT zero-address Transfer is never a supply row",
			token: usdtToken(),
			log: types.Log{
				Address: common.HexToAddress(usdtContract),
				Topics:  []common.Hash{TransferTopic, topicAddress("0x0000000000000000000000000000000000000000"), topicAddress(someone)},
				Data:    word32(new(big.Int).SetUint64(500000000000)),
			},
		},
		{
			name:  "USDC ordinary Transfer (both parties nonzero) is not a supply change",
			token: usdcToken(),
			log: types.Log{
				Address: common.HexToAddress(usdcContract),
				Topics:  []common.Hash{TransferTopic, topicAddress(someone), topicAddress(other)},
				Data:    word32(new(big.Int).SetUint64(1234567890)),
			},
		},
		{
			name:  "USDC paired Mint topic adds no row",
			token: usdcToken(),
			log: types.Log{
				Address: common.HexToAddress(usdcContract),
				Topics:  []common.Hash{usdcMintTopic, topicAddress(someone), topicAddress(other)},
				Data:    word32(new(big.Int).SetUint64(2000000000000)),
			},
		},
		{
			name:  "USDC paired Burn topic adds no row",
			token: usdcToken(),
			log: types.Log{
				Address: common.HexToAddress(usdcContract),
				Topics:  []common.Hash{usdcBurnTopic, topicAddress(someone)},
				Data:    word32(new(big.Int).SetUint64(1383937060000)),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev, err := DecodeSupplyEvent(tc.token, tc.log)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if ev != nil {
				t.Fatalf("must never produce a supply row, got %+v", ev)
			}
		})
	}
}

// TestDecodeMalformedAndMismatchAreErrors covers rejected shapes: both-zero
// USDC Transfer, wrong data lengths on recognized events (never a zero
// amount), and a log emitted by a different contract.
func TestDecodeMalformedAndMismatchAreErrors(t *testing.T) {
	cases := []struct {
		name        string
		token       config.Token
		log         types.Log
		wantMessage []string
	}{
		{
			name:  "USDC Transfer with from and to both zero is rejected",
			token: usdcToken(),
			log: types.Log{
				Address: common.HexToAddress(usdcContract),
				Topics:  []common.Hash{TransferTopic, topicAddress("0x0000000000000000000000000000000000000000"), topicAddress("0x0000000000000000000000000000000000000000")},
				Data:    word32(new(big.Int).SetUint64(1000000)),
			},
			wantMessage: []string{"Transfer", "zero address"},
		},
		{
			name:  "USDT Issue with 31-byte data is rejected, not a zero amount",
			token: usdtToken(),
			log: types.Log{
				Address: common.HexToAddress(usdtContract),
				Topics:  []common.Hash{IssueTopic},
				Data:    word32(new(big.Int).SetUint64(1000000))[:31],
			},
			wantMessage: []string{"Issue", "32 data bytes", "31"},
		},
		{
			name:  "USDT DestroyedBlackFunds with 32-byte data is rejected",
			token: usdtToken(),
			log: types.Log{
				Address: common.HexToAddress(usdtContract),
				Topics:  []common.Hash{DestroyedBlackFundsTopic},
				Data:    word32(new(big.Int).SetUint64(200000000000)),
			},
			wantMessage: []string{"DestroyedBlackFunds", "64 data bytes", "32"},
		},
		{
			name:  "log from a different contract than configured is rejected",
			token: usdtToken(),
			log: types.Log{
				Address: common.HexToAddress("0x1111111111111111111111111111111111111111"),
				Topics:  []common.Hash{IssueTopic},
				Data:    word32(new(big.Int).SetUint64(1000000)),
			},
			wantMessage: []string{"does not match configured contract"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev, err := DecodeSupplyEvent(tc.token, tc.log)
			if err == nil {
				t.Fatalf("expected an error, got event %+v", ev)
			}
			for _, want := range tc.wantMessage {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err.Error(), want)
				}
			}
		})
	}
}

// TestDecodeUSDTIssueFixture pins the deployed Mainnet USDT Issue receipt from
// 01-RESEARCH.md lines 92-98: tx 0x3a296e7b..., log index 0x249, raw amount
// 1000000000000000 — classified as a mint. (Tracer case, kept from RED.)
func TestDecodeUSDTIssueFixture(t *testing.T) {
	log := types.Log{
		Address: common.HexToAddress(usdtContract),
		Topics:  []common.Hash{IssueTopic},
		Data:    word32(new(big.Int).SetUint64(1000000000000000)),
		TxHash:  common.HexToHash("0x3a296e7b8289666b5ea54e48d3da404fc0b337cc4d419276b5675395244b115c"),
		Index:   0x249,
	}
	ev, err := DecodeSupplyEvent(usdtToken(), log)
	if err != nil {
		t.Fatalf("decode USDT Issue fixture: %v", err)
	}
	if ev == nil {
		t.Fatal("expected a supply event for the USDT Issue fixture, got nil")
	}
	if ev.Token != "USDT" {
		t.Errorf("Token = %q, want USDT", ev.Token)
	}
	if ev.EventType != "mint" {
		t.Errorf("EventType = %q, want mint", ev.EventType)
	}
	if ev.Amount == nil || ev.Amount.Cmp(new(big.Int).SetUint64(1000000000000000)) != 0 {
		t.Errorf("Amount = %v, want 1000000000000000", ev.Amount)
	}
}

// TestDecodeUSDTZeroAddressTransferYieldsNoSupplyRow guards the phase's core
// prohibition: a USDT Transfer whose from is the zero address looks exactly
// like an ERC20 mint but is never a USDT supply change. (Tracer case, kept
// from RED.)
func TestDecodeUSDTZeroAddressTransferYieldsNoSupplyRow(t *testing.T) {
	log := types.Log{
		Address: common.HexToAddress(usdtContract),
		Topics: []common.Hash{
			TransferTopic,
			topicAddress("0x0000000000000000000000000000000000000000"),
			topicAddress("0x8dAC17F958D2ee523a2206206994597C13D831ec6"),
		},
		Data:   word32(new(big.Int).SetUint64(500000000000)),
		TxHash: common.HexToHash("0x1111111111111111111111111111111111111111111111111111111111111111"),
		Index:  0x0,
	}
	ev, err := DecodeSupplyEvent(usdtToken(), log)
	if err != nil {
		t.Fatalf("decode USDT zero-address Transfer: %v", err)
	}
	if ev != nil {
		t.Fatalf("USDT zero-address Transfer must never produce a supply row, got %+v", ev)
	}
}
