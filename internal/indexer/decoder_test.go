package indexer

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"TOkenMonitor/internal/config"
)

const (
	usdtContract = "0xdAC17F958D2ee523a2206206994597C13D831ec7"
	usdcContract = "0xA0b86991c6218b36c1d19D4a2e9Eb0ce3606eb48"
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

func usdtToken() config.Token {
	return config.Token{Symbol: "USDT", Issuer: "Tether", Contract: usdtContract, Decimals: 6}
}

// TestDecodeUSDTIssueFixture pins the deployed Mainnet USDT Issue receipt from
// 01-RESEARCH.md lines 92-98: tx 0x3a296e7b..., log index 0x249, raw amount
// 1000000000000000 — classified as a mint.
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
// like an ERC20 mint but is never a USDT supply change.
func TestDecodeUSDTZeroAddressTransferYieldsNoSupplyRow(t *testing.T) {
	log := types.Log{
		Address: common.HexToAddress(usdtContract),
		Topics: []common.Hash{
			TransferTopic,
			common.BytesToHash(common.HexToAddress("0x0000000000000000000000000000000000000000").Bytes()),
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
