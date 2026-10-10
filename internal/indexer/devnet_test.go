// Kurtosis devnet integration suite (02-03, D-02): restart/resume/replay
// continuity against a REAL consensus-driven finalized head, and a REAL
// deployed TetherToken (verbatim Mainnet creation bytecode fixture) whose
// Issue/Redeem events must classify and survive replay unchanged. Both tests
// are env-gated opt-in, identical to CLICKHOUSE_URL/ETH_RPC_URL: they skip —
// never fail — without DEVNET_RPC_URL (and CLICKHOUSE_URL for the sync
// runs). The localnet is for behavior only; Mainnet pinned fixtures remain
// the classification anchor (D-02).
//
// This file is package indexer_test because it drives internal/rpc.Client
// (which imports internal/indexer — the external test package breaks the
// cycle). Pitfall 5 rule: devnet configs are constructed directly with the
// enclave chain id 3151908 and NEVER passed through config.Validate (which
// enforces Mainnet chain id 1).
package indexer_test

import (
	"context"
	ecdsa "crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/ethereum/go-ethereum/accounts/abi"
	bindv2 "github.com/ethereum/go-ethereum/accounts/abi/bind/v2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"TOkenMonitor/internal/config"
	"TOkenMonitor/internal/indexer"
	"TOkenMonitor/internal/rpc"
	"TOkenMonitor/internal/storage"
)

// devnetChainID is the eth-devnet enclave's chain id (kurtosis enclave
// inspect eth-devnet; probe-verified at research and execution time).
const devnetChainID = 3151908

// The public ethereum-package test key (genesis_constants.star) prefunded on
// every ethereum-package enclave. This is fixture material published with the
// package, never a secret. (The 02-RESEARCH copy was one hex nibble short;
// this value is the single-nibble repair that derives exactly the funded
// address below — verified cryptographically.)
const (
	devnetFundedAddr = "0x8943545177806ED17B9F23F0a21ee5948eCaa776"
	devnetFundedKey  = "0xbcdf20249abf0ed6d944c0288fad489e33f66b3960d9e6229c1cd214ed3bbe31"
)

// devnetRPC gates the suite on DEVNET_RPC_URL and returns the endpoint.
func devnetRPC(t *testing.T) string {
	t.Helper()
	u := strings.TrimSpace(os.Getenv("DEVNET_RPC_URL"))
	if u == "" {
		t.Skip("DEVNET_RPC_URL not set; start/inspect the localnet first:\n" +
			"  kurtosis run github.com/ethpandaops/ethereum-package --enclave eth-devnet\n" +
			"  kurtosis enclave inspect eth-devnet   (el-1-geth-lighthouse rpc port)")
	}
	return u
}

// devnetConfig builds the sync config directly (Pitfall 5: never Validate).
func devnetConfig() *config.Config {
	return &config.Config{
		ChainID:            devnetChainID,
		ConfirmationBlocks: 20,
		HeadPolicy:         "finalized",
		WindowBlocks:       50,
		WindowFloor:        10,
		PollSeconds:        2,
		Tokens: []config.Token{
			{Symbol: "USDT", Issuer: "Tether", Contract: "0xdAC17F958D2ee523a2206206994597C13D831ec7", Decimals: 6},
		},
	}
}

// devnetDB is a fresh ClickHouse database per test — full isolation from the
// operator's real tables (RunSync always keys checkpoints under the "ethereum"
// chain constant, so the namespace is the database). ctrl stays connected to
// the default database for DDL and qualified-name queries.
type devnetDB struct {
	store *storage.Store
	ctrl  driver.Conn
	name  string
}

func openDevnetDB(t *testing.T) *devnetDB {
	t.Helper()
	base := strings.TrimSpace(os.Getenv("CLICKHOUSE_URL"))
	if base == "" {
		t.Skip("CLICKHOUSE_URL not set; the devnet sync runs need the real server:\n" +
			"  docker run -d --name tm-clickhouse -p 127.0.0.1:9000:9000 -e CLICKHOUSE_SKIP_USER_SETUP=1 clickhouse/clickhouse-server:26.8")
	}
	ctx := context.Background()
	opts, err := clickhouse.ParseDSN(base)
	if err != nil {
		t.Fatalf("parse CLICKHOUSE_URL: %v", err)
	}
	ctrl, err := clickhouse.Open(opts)
	if err != nil {
		t.Fatalf("open control connection: %v", err)
	}
	db := &devnetDB{ctrl: ctrl, name: fmt.Sprintf("tm_devnet_%d", time.Now().UnixNano())}
	if err := ctrl.Exec(ctx, fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", db.name)); err != nil {
		t.Fatalf("create fresh database: %v", err)
	}
	t.Cleanup(func() {
		_ = ctrl.Exec(ctx, fmt.Sprintf("DROP DATABASE IF NOT EXISTS %s", db.name))
		_ = ctrl.Close()
	})

	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse CLICKHOUSE_URL: %v", err)
	}
	u.Path = "/" + db.name
	store, err := storage.Open(ctx, u.String())
	if err != nil {
		t.Fatalf("open fresh store: %v", err)
	}
	if err := store.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure schema in fresh database: %v", err)
	}
	db.store = store
	return db
}

// checkpointHeights on the physical table cannot be replayed as a sequence:
// ReplacingMergeTree merges the rows away within seconds. The restart test
// instead observes the exact window sequence through recordingStore below.

// recordingStore wraps the real store, recording every WriteCheckpoint
// height in memory — the exact, ordered checkpoint sequence across restarts.
type recordingStore struct {
	*storage.Store
	mu     sync.Mutex
	writes []uint64
}

func (r *recordingStore) WriteCheckpoint(ctx context.Context, chain string, height uint64, blockHash string) error {
	r.mu.Lock()
	r.writes = append(r.writes, height)
	r.mu.Unlock()
	return r.Store.WriteCheckpoint(ctx, chain, height, blockHash)
}

func (r *recordingStore) checkpointWrites() []uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]uint64(nil), r.writes...)
}

// waitForSlow is waitFor with a human-timescale tick (devnet finality waits
// run many minutes; a 2 ms poll would hammer the enclave).
func waitForSlow(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(1 * time.Second)
	}
	t.Fatalf("%s not reached within %v", what, timeout)
}

// deadlineBudget grows a floor budget up to the go-test deadline minus a
// reserve: the enclave's finality arrives in batched epoch jumps (measured
// 13-19 minutes from tip), so the finality wait uses every second the
// -timeout affords instead of a fixed window.
func deadlineBudget(t *testing.T, floor, reserve time.Duration) time.Duration {
	if d, ok := t.Deadline(); ok {
		if b := time.Until(d) - reserve; b > floor {
			return b
		}
	}
	return floor
}

// TestDevnetFollowsFinalizedHeadAcrossRestarts proves restart/resume/replay
// continuity against the real consensus-driven finalized head (D-02): a run
// cancelled mid-catch-up leaves a durable checkpoint; a restart resumes from
// it with a gap-free window sequence (every checkpoint advance is at most one
// window beyond the previous, across the restart boundary) and catches up to
// a previously-finalized height; replaying the covered range changes no FINAL
// totals.
func TestDevnetFollowsFinalizedHeadAcrossRestarts(t *testing.T) {
	rpcURL := devnetRPC(t)
	ctx0 := context.Background()
	client, err := rpc.New(ctx0, rpcURL)
	if err != nil {
		t.Fatalf("dial devnet: %v", err)
	}
	db := openDevnetDB(t)
	rec := &recordingStore{Store: db.store}
	cfg := devnetConfig()
	cfg.PollSeconds = 1
	cfg.StartBlock = 1 // catch-up from the beginning: windows to cancel into

	// Run 1 — cancelled mid-catch-up (simulated crash).
	errCh1, cancel1 := runSyncAsync(t, client, rec, cfg)
	waitFor(t, 2*time.Minute, "mid-catch-up progress past block 151", func() bool {
		h, _, found, err := db.store.ReadCheckpoint(ctx0, "ethereum")
		return err == nil && found && h >= 151
	})
	cancel1()
	awaitCanceled(t, errCh1)
	run1Writes := rec.checkpointWrites()

	h1, _, found, err := db.store.ReadCheckpoint(ctx0, "ethereum")
	if err != nil || !found {
		t.Fatalf("checkpoint after cancelled run: found=%v err=%v", found, err)
	}
	if h1 < 151 {
		t.Fatalf("checkpoint after cancelled run = %d, want >= 151 (mid-catch-up)", h1)
	}
	n1, s1, err := db.store.RangeTotals(ctx0, "ethereum", 1, h1)
	if err != nil {
		t.Fatalf("totals after cancelled run: %v", err)
	}

	// A height that is already finalized stays canonical: the restart must
	// reach it without waiting for new finality.
	finalized0, _, err := client.EligibleHead(ctx0, cfg)
	if err != nil {
		t.Fatalf("finalized head: %v", err)
	}
	if finalized0 <= h1 {
		t.Fatalf("finalized head %d not above the cancelled checkpoint %d; rerun with a lower start_block", finalized0, h1)
	}

	// Run 2 — restart: resumes from the checkpoint and catches up.
	errCh2, cancel2 := runSyncAsync(t, client, rec, cfg)
	waitForSlow(t, 10*time.Minute, fmt.Sprintf("restart catch-up to finalized head %d", finalized0), func() bool {
		h, _, _, err := db.store.ReadCheckpoint(ctx0, "ethereum")
		return err == nil && h >= finalized0
	})
	cancel2()
	awaitCanceled(t, errCh2)

	// Continuity and no block gaps: the recorded checkpoint-write sequence
	// (both runs) starts at the seed, is strictly increasing, advances at
	// most one window at a time — including across the restart boundary —
	// and reaches finalized0.
	writes := rec.checkpointWrites()
	if len(writes) < len(run1Writes)+1 || len(run1Writes) < 2 || writes[0] != 1 {
		t.Fatalf("checkpoint write sequence = %v (run 1: %v), want seed 1 then strictly advancing writes in both runs", writes, run1Writes)
	}
	maxDelta := uint64(0)
	for i := 1; i < len(writes); i++ {
		if writes[i] <= writes[i-1] {
			t.Fatalf("checkpoint writes not strictly increasing at %d: %v", i, writes)
		}
		if d := writes[i] - writes[i-1]; d > maxDelta {
			maxDelta = d
		}
	}
	if maxDelta > cfg.WindowBlocks {
		t.Fatalf("largest checkpoint advance = %d blocks, want <= window_blocks %d (a block gap means a skipped range)", maxDelta, cfg.WindowBlocks)
	}
	if writes[len(writes)-1] < finalized0 {
		t.Fatalf("final checkpoint %d never reached the already-finalized head %d", writes[len(writes)-1], finalized0)
	}

	// Replay idempotence across the restart: the covered range's FINAL
	// totals are unchanged (the restart never re-ingests below its
	// checkpoint, and any replay collapses under FINAL).
	n2, s2, err := db.store.RangeTotals(ctx0, "ethereum", 1, h1)
	if err != nil {
		t.Fatalf("totals after restart: %v", err)
	}
	if n1 != n2 || s1.Cmp(s2) != 0 {
		t.Fatalf("FINAL totals over [1,%d] changed across the restart: (%d,%s) -> (%d,%s)",
			h1, n1, s1.String(), n2, s2.String())
	}
}

// tetherMinimalABI carries exactly the two supply-moving functions the
// lifecycle test drives on the deployed fixture contract.
const tetherMinimalABI = `[
 {"type":"function","name":"issue","inputs":[{"name":"amount","type":"uint256"}],"outputs":[],"stateMutability":"nonpayable"},
 {"type":"function","name":"redeem","inputs":[{"name":"amount","type":"uint256"}],"outputs":[],"stateMutability":"nonpayable"}
]`

// devnetSend issues one ABI-packed call as a plain legacy transaction and
// waits for its receipt (status must be 1); returns the mined block number.
func devnetSend(t *testing.T, ctx context.Context, ec *ethclient.Client, key *ecdsa.PrivateKey, chainID *big.Int, nonce uint64, to common.Address, data []byte) uint64 {
	t.Helper()
	gasPrice, err := ec.SuggestGasPrice(ctx)
	if err != nil {
		t.Fatalf("suggest gas price: %v", err)
	}
	toAddr := to
	tx := types.NewTx(&types.LegacyTx{
		Nonce:    nonce,
		To:       &toAddr,
		Value:    new(big.Int),
		Gas:      200_000,
		GasPrice: gasPrice,
		Data:     data,
	})
	signed, err := types.SignTx(tx, types.LatestSignerForChainID(chainID), key)
	if err != nil {
		t.Fatalf("sign tx: %v", err)
	}
	if err := ec.SendTransaction(ctx, signed); err != nil {
		t.Fatalf("send tx: %v", err)
	}
	mineCtx, cancelMine := context.WithTimeout(ctx, 3*time.Minute)
	defer cancelMine()
	receipt, err := bindv2.WaitMined(mineCtx, ec, signed.Hash())
	if err != nil {
		t.Fatalf("wait mined: %v", err)
	}
	if receipt.Status != 1 {
		t.Fatalf("tx %s failed (status %d)", signed.Hash().Hex(), receipt.Status)
	}
	return receipt.BlockNumber.Uint64()
}

// TestDevnetUSDTLifecycle deploys the verbatim Mainnet TetherToken creation
// bytecode (testdata/usdt_creation.hex; provenance in README) on the devnet,
// drives real issue/redeem supply events, runs the continuous indexer over
// the range, and asserts the stored rows classify with the exact amounts —
// then replays the whole range through the sync pipeline and asserts FINAL
// totals and the row set are unchanged (D-02).
func TestDevnetUSDTLifecycle(t *testing.T) {
	rpcURL := devnetRPC(t)
	ctx0 := context.Background()
	client, err := rpc.New(ctx0, rpcURL)
	if err != nil {
		t.Fatalf("dial devnet: %v", err)
	}
	ec, err := ethclient.DialContext(ctx0, rpcURL)
	if err != nil {
		t.Fatalf("dial devnet ethclient: %v", err)
	}

	// Deployer: the public ethereum-package prefunded test key (documented
	// fixture, never a secret). Explicit GasLimit per research A6.
	key, err := crypto.HexToECDSA(strings.TrimPrefix(devnetFundedKey, "0x"))
	if err != nil {
		t.Fatalf("parse fixture key: %v", err)
	}
	chainID := big.NewInt(devnetChainID)
	auth := bindv2.NewKeyedTransactor(key, chainID)
	auth.GasLimit = 3_000_000
	auth.Context = ctx0

	hexBytes, err := os.ReadFile("../../testdata/usdt_creation.hex")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	input, err := hex.DecodeString(strings.TrimSpace(string(hexBytes)))
	if err != nil {
		t.Fatalf("decode fixture hex: %v", err)
	}
	tokenAddr, deployTx, err := bindv2.DeployContract(auth, input, ec, nil)
	if err != nil {
		t.Fatalf("deploy TetherToken fixture: %v", err)
	}
	deployCtx, cancelDeploy := context.WithTimeout(ctx0, 3*time.Minute)
	defer cancelDeploy()
	receipt, err := bindv2.WaitMined(deployCtx, ec, deployTx.Hash())
	if err != nil {
		t.Fatalf("wait deploy mined: %v", err)
	}
	if receipt.Status != 1 || receipt.ContractAddress != tokenAddr {
		t.Fatalf("deploy receipt = (status %d, contract %s), want (1, %s)", receipt.Status, receipt.ContractAddress.Hex(), tokenAddr.Hex())
	}
	deployBlk := receipt.BlockNumber.Uint64()

	// Drive supply events: issue then redeem (owner is the deployer; the
	// issue precedes the redeem so the balance covers it regardless of the
	// constructor's initial supply).
	ab, err := abi.JSON(strings.NewReader(tetherMinimalABI))
	if err != nil {
		t.Fatalf("parse minimal ABI: %v", err)
	}
	issueAmt := uint64(1_234_567)
	redeemAmt := uint64(234_567)
	issueData, err := ab.Pack("issue", new(big.Int).SetUint64(issueAmt))
	if err != nil {
		t.Fatalf("pack issue: %v", err)
	}
	redeemData, err := ab.Pack("redeem", new(big.Int).SetUint64(redeemAmt))
	if err != nil {
		t.Fatalf("pack redeem: %v", err)
	}
	nonce, err := ec.PendingNonceAt(ctx0, auth.From)
	if err != nil {
		t.Fatalf("pending nonce: %v", err)
	}
	issueBlk := devnetSend(t, ctx0, ec, key, chainID, nonce, tokenAddr, issueData)
	redeemBlk := devnetSend(t, ctx0, ec, key, chainID, nonce+1, tokenAddr, redeemData)
	t.Logf("fixture deployed at block %d (%s); issue at %d; redeem at %d", deployBlk, tokenAddr.Hex(), issueBlk, redeemBlk)

	// Sync over the range: seed just below the deployment and let the loop
	// idle-poll until the eligible head covers the redeem block. The head
	// policy here is "confirmed" with a 64-block (2-epoch) depth — a
	// deliberate, documented deviation from the finalized default: this
	// enclave's beacon finality needs ~20 minutes to cover a tip block
	// (measured 2026-10-09: 19.2 min; 2026-10-10: ~20 min — batched epoch
	// jumps), which cannot fit inside the suite's pinned 20-minute go-test
	// budget together with the deploy and the replay. Real finalized-head
	// following is proven separately by TestDevnetFollowsFinalizedHeadAcross
	// Restarts (finalized policy); this test's subject is the token
	// semantics, classification, and replay (D-02), for which 2 epochs of
	// confirmation depth is the D-01 policy-space equivalent margin.
	db := openDevnetDB(t)
	cfg := devnetConfig()
	cfg.HeadPolicy = "confirmed"
	cfg.ConfirmationBlocks = 64
	cfg.Tokens = []config.Token{{Symbol: "USDT", Issuer: "Tether", Contract: tokenAddr.Hex(), Decimals: 6}}
	seed := deployBlk - 1
	if seed == 0 {
		seed = 1
	}
	cfg.StartBlock = seed

	topBlk := issueBlk
	if redeemBlk > topBlk {
		topBlk = redeemBlk
	}
	assertRows := func(stage string) {
		t.Helper()
		n, s, err := db.store.RangeTotals(ctx0, "ethereum", seed, topBlk+16)
		if err != nil {
			t.Fatalf("%s: totals: %v", stage, err)
		}
		mints, redeems := 0, 0
		rows, err := db.store.Inspect(ctx0, seed, topBlk+16, 64)
		if err != nil {
			t.Fatalf("%s: inspect: %v", stage, err)
		}
		for _, r := range rows {
			switch r.EventType {
			case indexer.EventMint:
				mints++
				if r.RawAmount.Uint64() != issueAmt {
					t.Errorf("%s: mint amount = %s, want exact %d", stage, r.RawAmount.String(), issueAmt)
				}
			case indexer.EventRedeem:
				redeems++
				if r.RawAmount.Uint64() != redeemAmt {
					t.Errorf("%s: redeem amount = %s, want exact %d", stage, r.RawAmount.String(), redeemAmt)
				}
			default:
				t.Errorf("%s: unexpected classified row %+v", stage, r)
			}
		}
		if mints != 1 || redeems != 1 || n != 2 {
			t.Fatalf("%s: rows = (mints %d, redeems %d, FINAL count %d, sum %s), want exactly one of each (count 2)", stage, mints, redeems, n, s.String())
		}
		if want := new(big.Int).Add(new(big.Int).SetUint64(issueAmt), new(big.Int).SetUint64(redeemAmt)); s.Cmp(want) != 0 {
			t.Fatalf("%s: FINAL sum = %s, want %s", stage, s.String(), want.String())
		}
	}

	// D-04: seeding above the eligible head is an immediate sync error, and
	// the deployment sits at the tip — so wait for the eligible head to
	// cover the seed BEFORE starting the loop, then ingest and wait for the
	// events.
	headBudget := deadlineBudget(t, 14*time.Minute, 3*time.Minute)
	waitForSlow(t, headBudget, fmt.Sprintf("eligible (confirmed-%d) head to cover the seed block %d", cfg.ConfirmationBlocks, seed), func() bool {
		h, _, err := client.EligibleHead(ctx0, cfg)
		return err == nil && h >= seed
	})
	errCh, cancel := runSyncAsync(t, client, db.store, cfg)
	waitForSlow(t, deadlineBudget(t, 5*time.Minute, 1*time.Minute), fmt.Sprintf("both events at blocks %d-%d stored and checkpoint past %d", issueBlk, redeemBlk, topBlk), func() bool {
		h, _, _, err := db.store.ReadCheckpoint(ctx0, "ethereum")
		if err != nil || h < topBlk {
			return false
		}
		n, _, err := db.store.RangeTotals(ctx0, "ethereum", seed, topBlk+16)
		return err == nil && n == 2
	})
	assertRows("after first sync")
	preH, _, _, err := db.store.ReadCheckpoint(ctx0, "ethereum")
	if err != nil {
		t.Fatalf("read checkpoint: %v", err)
	}

	// Surface any early RunSync exit with its real error instead of polling
	// an exited loop for the full budget.
	cancel()
	awaitCanceled(t, errCh)

	// Replay the whole range through the real sync pipeline: clear the
	// checkpoint table (fresh scratch database — safe) and re-run from the
	// same seed. FINAL totals and the classified row set must be unchanged.
	if err := db.ctrl.Exec(ctx0, fmt.Sprintf("TRUNCATE TABLE %s.indexer_checkpoint", db.name)); err != nil {
		t.Fatalf("truncate checkpoint for replay: %v", err)
	}

	errCh2, cancel2 := runSyncAsync(t, client, db.store, cfg)
	waitForSlow(t, deadlineBudget(t, 5*time.Minute, 1*time.Minute), fmt.Sprintf("replay catch-up back past %d", preH), func() bool {
		h, _, _, err := db.store.ReadCheckpoint(ctx0, "ethereum")
		return err == nil && h >= preH
	})
	cancel2()
	awaitCanceled(t, errCh2)
	assertRows("after replay")
}
