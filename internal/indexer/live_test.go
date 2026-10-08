// Live fixture proof suite: the five pinned Mainnet receipts from
// .planning/phases/01-live-supply-event-path/01-RESEARCH.md (fixture table)
// classified through the operator's real provider. Every test is gated on
// ETH_RPC_URL (D-01: the credential comes from the environment only; this
// file never contains a provider URL) and skips — never fails — when it is
// unset.
//
// This file is package indexer_test, not indexer, because it drives
// internal/rpc.Client, which itself imports internal/indexer; the external
// test package breaks that cycle.
package indexer_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"

	"TOkenMonitor/internal/config"
	"TOkenMonitor/internal/indexer"
	"TOkenMonitor/internal/rpc"
)

// USDC paired corroboration topic0s (01-RESEARCH.md): Mint/Burn logs arrive
// in address-only fetches and must never produce supply rows. The ERC20
// Transfer topic0 identifies transfer-family logs in the negative proofs.
var (
	liveUSDCMintTopic = common.HexToHash("0xab8530f87dc9b59234c4623bf917212bb2536d647574c8e7e5da92c2ede0c9f8")
	liveUSDCBurnTopic = common.HexToHash("0xcc16f5dbb4873280815c1ee09dbd06736cffcc184412cf7a71a0fdb75d397ca5")
	liveTransferTopic = common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")
)

// liveFixture pins one researched Mainnet receipt: block, transaction hash,
// in-block log index, expected classification, exact raw amount as a decimal
// string (big.Int String comparisons only — never a float), and the decoded
// address sides (lowercase; empty when the event carries none).
type liveFixture struct {
	symbol      string
	block       uint64
	txHash      string
	logIndex    uint
	eventType   string
	amount      string
	fromAddress string
	toAddress   string
}

// The five pinned fixtures (01-RESEARCH.md fixture table; USDC counterparty
// addresses and the DestroyedBlackFunds blacklisted address verified from the
// live receipts).
var liveFixtures = []liveFixture{
	{symbol: "USDT", block: 0x167ab1a, txHash: "0x3a296e7b8289666b5ea54e48d3da404fc0b337cc4d419276b5675395244b115c", logIndex: 0x249, eventType: indexer.EventMint, amount: "1000000000000000"},
	{symbol: "USDT", block: 0x1726ef7, txHash: "0x8642010ea16fda150d6db81b7e5a377453d825021452cc076a2fb6fbda524e1c", logIndex: 0x2ca, eventType: indexer.EventRedeem, amount: "3000000000000000"},
	{symbol: "USDT", block: 0x15aac45, txHash: "0x06abbfaa405699132123d74e1cdf712e42bf88fff00ed83b8a9a0a9f112c6055", logIndex: 0x277, eventType: indexer.EventDestroyedBlackFunds, amount: "200000000000", fromAddress: "0x532961b23a28a9e5a8b1769bdb3e8906fc7c7a01"},
	{symbol: "USDC", block: 0x1632fcb, txHash: "0xc7066b502fa8321a090fbff8842f69d136e2d4c27849a04cbc1ed02cac98963e", logIndex: 0x2ad, eventType: indexer.EventMint, amount: "2000000000000", toAddress: "0x2b52e60c844d7946b6d910d3296940dc889cc785"},
	{symbol: "USDC", block: 0x1638e19, txHash: "0x4f52e25d39e006606f26de5a496eb7f9198c8b8467686f0a994d07d7a2a4ca9f", logIndex: 0x162, eventType: indexer.EventBurn, amount: "1383937060000", fromAddress: "0x55fe002aeff02f77364de339a1292923a15844b8"},
}

// Free-tier providers (D-01) rate-limit bursty eth_getLogs (observed live:
// HTTP 429 / JSON-RPC -32005 "Too Many Requests" and transient 503s). The
// tests pace every call and retry rate-limited reads with backoff — safe
// because the fixtures are immutable historical blocks. Test-only: the
// production ingest path owns real retry policy in the continuous-sync phase.
var (
	livePaceMu      sync.Mutex
	liveLastCall    time.Time
	liveMinInterval = 500 * time.Millisecond
	liveBackoffs    = []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}
)

func isRateLimitErr(err error) bool {
	s := err.Error()
	return strings.Contains(s, "429") ||
		strings.Contains(s, "Too Many Requests") ||
		strings.Contains(s, "-32005") ||
		strings.Contains(s, "temporarily unavailable")
}

// fetchLogsPaced runs one paced, rate-limit-retrying single-block FetchLogs.
func fetchLogsPaced(t *testing.T, ctx context.Context, client *rpc.Client, token config.Token, topics []common.Hash, block uint64) []types.Log {
	t.Helper()
	for attempt := 0; ; attempt++ {
		livePaceMu.Lock()
		if wait := liveMinInterval - time.Since(liveLastCall); wait > 0 {
			time.Sleep(wait)
		}
		liveLastCall = time.Now()
		livePaceMu.Unlock()
		logs, err := client.FetchLogs(ctx, token, topics, block, block)
		if err == nil {
			return logs
		}
		if attempt >= len(liveBackoffs) || !isRateLimitErr(err) {
			t.Fatalf("FetchLogs block 0x%x: %s", block, scrub(err))
		}
		time.Sleep(liveBackoffs[attempt])
	}
}

// isPinnedLog reports whether lg is this fixture's exact log.
func (fx liveFixture) isPinnedLog(lg types.Log) bool {
	return lg.TxHash == common.HexToHash(fx.txHash) && lg.Index == fx.logIndex
}

// loadLiveConfig walks up from the test working directory to the module root
// so `go test` resolves config/tokens.json from any package directory.
func loadLiveConfig(t *testing.T) *config.Config {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	for {
		p := filepath.Join(dir, "config", "tokens.json")
		if _, err := os.Stat(p); err == nil {
			cfg, err := config.Load(p)
			if err != nil {
				t.Fatalf("config.Load(%s): %v", p, err)
			}
			if err := cfg.Validate(); err != nil {
				t.Fatalf("config.Validate: %v", err)
			}
			return cfg
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("config/tokens.json not found in or above the working directory")
		}
		dir = parent
	}
}

func tokenBySymbol(t *testing.T, cfg *config.Config, symbol string) config.Token {
	t.Helper()
	for _, tok := range cfg.Tokens {
		if tok.Symbol == symbol {
			return tok
		}
	}
	t.Fatalf("token %s not configured", symbol)
	return config.Token{}
}

// liveURLEndpoint is the resolved ETH_RPC_URL; scrub replaces it with
// scheme://host in failure output because net/http *url.Error messages embed
// the complete credential-bearing request URL (D-01 hygiene).
var liveURLEndpoint string

func scrub(err error) string {
	if liveURLEndpoint == "" {
		return err.Error()
	}
	return strings.ReplaceAll(err.Error(), liveURLEndpoint, config.HostOnly(liveURLEndpoint))
}

// liveClient gates the suite on ETH_RPC_URL (skip, not fail, when unset —
// the researched opt-in design), asserts Mainnet via eth_chainId, and builds
// one rpc.Client per test. The D-02 range probe is deliberately NOT invoked
// here: these are single-block historical queries, not ingest ranges.
func liveClient(t *testing.T) (*rpc.Client, *config.Config, context.Context) {
	t.Helper()
	rpcURL, err := config.RPCURL()
	if err != nil {
		t.Skipf("set ETH_RPC_URL to a Mainnet provider with historical eth_getLogs")
	}
	liveURLEndpoint = rpcURL
	cfg := loadLiveConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	ec, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		t.Fatalf("dial provider: %s", scrub(err))
	}
	chainID, err := ec.ChainID(ctx)
	ec.Close()
	if err != nil {
		t.Fatalf("eth_chainId: %s", scrub(err))
	}
	if chainID.Int64() != 1 {
		t.Fatalf("provider chain id %d, want 1 (Ethereum Mainnet)", chainID.Int64())
	}
	client, err := rpc.New(ctx, rpcURL)
	if err != nil {
		t.Fatalf("rpc.New: %s", scrub(err))
	}
	return client, cfg, ctx
}

// TestLiveFixtureClassification drives internal/rpc.Client.FetchLogs +
// indexer.DecodeSupplyEvent against the operator's real provider: each of
// the five pinned Mainnet receipts must classify with the exact researched
// event type, raw amount, and address sides.
func TestLiveFixtureClassification(t *testing.T) {
	client, cfg, ctx := liveClient(t)
	for _, fx := range liveFixtures {
		fx := fx
		t.Run(fx.symbol+"/"+fx.eventType, func(t *testing.T) {
			token := tokenBySymbol(t, cfg, fx.symbol)
			logs := fetchLogsPaced(t, ctx, client, token, indexer.SupplyTopics(token), fx.block)
			var got *indexer.SupplyEvent
			seen := 0
			for _, lg := range logs {
				if !fx.isPinnedLog(lg) {
					continue
				}
				seen++
				ev, err := indexer.DecodeSupplyEvent(token, lg)
				if err != nil {
					t.Fatalf("DecodeSupplyEvent: %v", err)
				}
				got = ev
			}
			if seen != 1 {
				t.Fatalf("pinned log matched %d times in block 0x%x, want exactly 1", seen, fx.block)
			}
			if got == nil {
				t.Fatalf("pinned %s log at block 0x%x decoded to no supply row", fx.symbol, fx.block)
			}
			if got.Token != fx.symbol {
				t.Errorf("token = %q, want %q", got.Token, fx.symbol)
			}
			if got.EventType != fx.eventType {
				t.Errorf("event type = %q, want %q", got.EventType, fx.eventType)
			}
			if got.Amount.String() != fx.amount {
				t.Errorf("raw amount = %s, want exact %s", got.Amount.String(), fx.amount)
			}
			if got.FromAddress != fx.fromAddress {
				t.Errorf("from address = %q, want %q", got.FromAddress, fx.fromAddress)
			}
			if got.ToAddress != fx.toAddress {
				t.Errorf("to address = %q, want %q", got.ToAddress, fx.toAddress)
			}
		})
	}
}

// TestLiveUSDCPairedEventsNotCounted proves EVENT-02's no-double-count rule
// end to end on real chain data: an address-only fetch (no topic filter)
// delivers the USDC paired Mint/Burn corroboration logs alongside the
// zero-address Transfers, and decoding yields a supply row for exactly the
// zero-address Transfers — never for the paired events. Each fixture receipt
// itself must contribute exactly one row with its pinned classification.
//
// The row count is asserted per receipt, not per block: a live probe
// (2026-10-08) showed block 0x1638e19 also carries two unrelated USDC mints
// from tx 0xf48467ac..., which legitimately produce their own rows. The
// block-wide invariant is the 1:1 correspondence between zero-address
// Transfers and supply rows.
func TestLiveUSDCPairedEventsNotCounted(t *testing.T) {
	client, cfg, ctx := liveClient(t)
	usdc := tokenBySymbol(t, cfg, indexer.SymbolUSDC)
	var zeroHash common.Hash
	for _, fx := range liveFixtures {
		if fx.symbol != indexer.SymbolUSDC {
			continue
		}
		fx := fx
		t.Run(fmt.Sprintf("block_0x%x", fx.block), func(t *testing.T) {
			logs := fetchLogsPaced(t, ctx, client, usdc, nil, fx.block) // address-only
			pairedSeen := 0
			zeroAddrTransfers := 0
			rows := map[string]*indexer.SupplyEvent{} // key txHash:logIndex
			for _, lg := range logs {
				if len(lg.Topics) == 0 {
					continue
				}
				switch lg.Topics[0] {
				case liveUSDCMintTopic, liveUSDCBurnTopic:
					pairedSeen++
				case liveTransferTopic:
					if len(lg.Topics) == 3 && (lg.Topics[1] == zeroHash || lg.Topics[2] == zeroHash) {
						zeroAddrTransfers++
					}
				}
				ev, err := indexer.DecodeSupplyEvent(usdc, lg)
				if err != nil {
					t.Fatalf("DecodeSupplyEvent tx %s log %d: %v", lg.TxHash.Hex(), lg.Index, err)
				}
				if ev != nil {
					rows[fmt.Sprintf("%s:%d", lg.TxHash.Hex(), lg.Index)] = ev
				}
			}
			if pairedSeen == 0 {
				t.Fatal("no paired Mint/Burn logs reached the decode step; the no-double-count proof is vacuous")
			}
			if len(rows) != zeroAddrTransfers {
				t.Fatalf("supply rows = %d, want %d (exactly the zero-address Transfers; paired events must add none)", len(rows), zeroAddrTransfers)
			}
			row, ok := rows[fmt.Sprintf("%s:%d", common.HexToHash(fx.txHash).Hex(), fx.logIndex)]
			if !ok {
				t.Fatalf("fixture receipt %s log %d produced no supply row", fx.txHash, fx.logIndex)
			}
			if row.EventType != fx.eventType {
				t.Errorf("fixture row event type = %q, want %q", row.EventType, fx.eventType)
			}
			if row.Amount.String() != fx.amount {
				t.Errorf("fixture row raw amount = %s, want exact %s", row.Amount.String(), fx.amount)
			}
		})
	}
}

// TestLiveUSDTTransfersYieldNothing proves the prohibition end to end: in
// the three USDT fixture blocks fetched address-only, every Transfer-topic
// log decodes to no supply row (USDT supply moves only through
// Issue/Redeem/DestroyedBlackFunds), while each pinned supply event still
// decodes with its exact classification.
func TestLiveUSDTTransfersYieldNothing(t *testing.T) {
	client, cfg, ctx := liveClient(t)
	usdt := tokenBySymbol(t, cfg, indexer.SymbolUSDT)
	for _, fx := range liveFixtures {
		if fx.symbol != indexer.SymbolUSDT {
			continue
		}
		fx := fx
		t.Run(fmt.Sprintf("block_0x%x", fx.block), func(t *testing.T) {
			logs := fetchLogsPaced(t, ctx, client, usdt, nil, fx.block) // address-only
			transfers := 0
			var pinned *indexer.SupplyEvent
			pinnedSeen := 0
			for _, lg := range logs {
				isTransfer := len(lg.Topics) > 0 && lg.Topics[0] == liveTransferTopic
				if isTransfer {
					transfers++
				}
				ev, err := indexer.DecodeSupplyEvent(usdt, lg)
				if err != nil {
					t.Fatalf("DecodeSupplyEvent tx %s log %d: %v", lg.TxHash.Hex(), lg.Index, err)
				}
				if isTransfer && ev != nil {
					t.Fatalf("USDT Transfer tx %s log %d produced a supply row (%s) — USDT Transfers are never supply events",
						lg.TxHash.Hex(), lg.Index, ev.EventType)
				}
				if fx.isPinnedLog(lg) {
					pinnedSeen++
					pinned = ev
				}
			}
			if transfers == 0 {
				t.Fatal("no USDT Transfer logs in the fetched block; the negative proof is vacuous")
			}
			if pinnedSeen != 1 || pinned == nil {
				t.Fatalf("pinned supply event matched %d times and decoded to %v, want exactly one supply row", pinnedSeen, pinned)
			}
			if pinned.EventType != fx.eventType {
				t.Errorf("pinned event type = %q, want %q", pinned.EventType, fx.eventType)
			}
			if pinned.Amount.String() != fx.amount {
				t.Errorf("pinned raw amount = %s, want exact %s", pinned.Amount.String(), fx.amount)
			}
		})
	}
}
