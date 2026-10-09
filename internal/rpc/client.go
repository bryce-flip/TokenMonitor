// Package rpc is a thin Ethereum JSON-RPC reader over go-ethereum's
// ethclient. Every error message renders the endpoint through
// config.HostOnly so provider credentials embedded in the URL never reach
// logs (D-01, T-01-02).
package rpc

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	ethrpc "github.com/ethereum/go-ethereum/rpc"

	"TOkenMonitor/internal/config"
	"TOkenMonitor/internal/indexer"
)

// rateLimitBackoffs paces retries of throttled RPC reads. Free-tier
// providers reject bursty requests (observed live: HTTP 429 / JSON-RPC
// -32005); requirements.md §24 requires the indexer to tolerate them.
var rateLimitBackoffs = []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}

// isResultCapErr reports a provider result-cap / query-duration rejection:
// Infura documents BOTH shapes under code -32005 (docs.infura.io, retrieved
// 2026-10-09), so only the message text discriminates them from throttles
// (D-05, 02-RESEARCH Pitfall 1):
//
//	{"code":-32005,"message":"query returned more than 10000 results"}
//	{"code":-32005,"message":"query timeout exceeded"}
func isResultCapErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "more than 10000 results") ||
		strings.Contains(s, "query timeout exceeded")
}

// isRateLimitErr reports a provider throttle response. The bare -32005
// disjunct is gone (02-02): that code is shared with the result cap above,
// and a cap misrouted here would burn the bounded backoff on a query that
// can only be fixed by shrinking the range. Live-observed throttle shapes:
// 429 / -32005 "Too Many Requests" (Phase 1), -32005 "Rate limit exceeded"
// (eth.merkle.io, 02-RESEARCH).
func isRateLimitErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "429") ||
		strings.Contains(s, "Too Many Requests") ||
		strings.Contains(s, "Rate limit exceeded") ||
		strings.Contains(s, "temporarily unavailable")
}

// withRateLimitRetry retries a throttled RPC read with bounded backoff.
// Every read here is an idempotent historical query, so retrying is safe.
func withRateLimitRetry(ctx context.Context, call func() error) error {
	err := call()
	for attempt := 0; err != nil && isRateLimitErr(err) && attempt < len(rateLimitBackoffs); attempt++ {
		select {
		case <-time.After(rateLimitBackoffs[attempt]):
		case <-ctx.Done():
			return err
		}
		err = call()
	}
	return err
}

// Client wraps ethclient for the bounded reads the indexer needs.
type Client struct {
	ec     *ethclient.Client
	rawURL string // never logged directly; errors render config.HostOnly(rawURL)
}

// New dials the Ethereum JSON-RPC endpoint at rawURL.
func New(ctx context.Context, rawURL string) (*Client, error) {
	ec, err := ethclient.DialContext(ctx, rawURL)
	if err != nil {
		return nil, fmt.Errorf("rpc dial %s: %w", config.HostOnly(rawURL), err)
	}
	return &Client{ec: ec, rawURL: rawURL}, nil
}

// Probe fail-fasts before any ingest (D-02): the chain ID must match the
// configured chain, the requested range must be confirmed under the
// configured confirmation depth, and the provider must serve a sample
// eth_getLogs at the exact from-block for the first configured token. An
// empty sample result passes — the probe proves historical query capability,
// not content.
func (c *Client) Probe(ctx context.Context, cfg *config.Config, from, to uint64) error {
	// IN-03: Validate rejects an empty token list, but Probe must not index
	// cfg.Tokens[0] on a config that skipped validation.
	if len(cfg.Tokens) == 0 {
		return fmt.Errorf("probe %s: no tokens configured; at least one token is required for the sample eth_getLogs", config.HostOnly(c.rawURL))
	}
	chainID, err := c.ec.ChainID(ctx)
	if err != nil {
		return fmt.Errorf("probe %s: eth_chainId: %w", config.HostOnly(c.rawURL), err)
	}
	if chainID.Int64() != cfg.ChainID {
		return fmt.Errorf("probe %s: chain id %d, want %d", config.HostOnly(c.rawURL), chainID.Int64(), cfg.ChainID)
	}
	head, err := c.ec.BlockNumber(ctx)
	if err != nil {
		return fmt.Errorf("probe %s: eth_blockNumber: %w", config.HostOnly(c.rawURL), err)
	}
	eligible := uint64(0) // head below the confirmation depth: nothing is eligible
	if head > cfg.ConfirmationBlocks {
		eligible = head - cfg.ConfirmationBlocks
	}
	if to > eligible {
		return fmt.Errorf("probe %s: to-block %d exceeds confirmed head %d minus confirmation_blocks %d (eligible through %d)",
			config.HostOnly(c.rawURL), to, head, cfg.ConfirmationBlocks, eligible)
	}
	token := cfg.Tokens[0]
	sampleQuery := ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(from),
		ToBlock:   new(big.Int).SetUint64(from),
		Addresses: []common.Address{common.HexToAddress(token.Contract)},
		Topics:    [][]common.Hash{indexer.SupplyTopics(token)},
	}
	if err := withRateLimitRetry(ctx, func() error {
		_, err := c.ec.FilterLogs(ctx, sampleQuery)
		return err
	}); err != nil {
		return fmt.Errorf("probe %s: sample eth_getLogs at block %d for %s: %w (provider may not serve historical logs)",
			config.HostOnly(c.rawURL), from, token.Symbol, err)
	}
	return nil
}

// FetchLogs returns the logs of token's supply topics over [from, to],
// filtered by the token's configured contract address.
func (c *Client) FetchLogs(ctx context.Context, token config.Token, topics []common.Hash, from, to uint64) ([]types.Log, error) {
	query := ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(from),
		ToBlock:   new(big.Int).SetUint64(to),
		Addresses: []common.Address{common.HexToAddress(token.Contract)},
		Topics:    [][]common.Hash{topics},
	}
	var logs []types.Log
	if err := withRateLimitRetry(ctx, func() error {
		var err error
		logs, err = c.ec.FilterLogs(ctx, query)
		return err
	}); err != nil {
		return nil, fmt.Errorf("eth_getLogs %s %s blocks %d-%d: %w", token.Symbol, config.HostOnly(c.rawURL), from, to, err)
	}
	return logs, nil
}

// FetchLogsResilient fetches token's supply-topic logs over [from, to] under
// the D-05 window policy: walk the range in sub-ranges starting at
// min(window, remaining); a result-cap rejection (isResultCapErr — checked
// BEFORE any throttle classification, T-02-07) halves the sub-range and
// retries the SAME starting block immediately (the query was rejected, not
// throttled — no backoff); a throttle is retried on the same sub-range by
// the existing bounded backoff inside FetchLogs. A cap that persists down to
// the floor fails closed with a named error — a range that cannot be fetched
// is never silently skipped (SYNC-02). The sub-range size resets to the
// passed window on every call: halving state never leaks across outer
// windows (D-05 stateless policy).
func (c *Client) FetchLogsResilient(ctx context.Context, token config.Token, topics []common.Hash, from, to, window, floor uint64) ([]types.Log, error) {
	if floor < 1 {
		floor = 1 // a sub-range is never smaller than one block
	}
	var out []types.Log
	size := window
	start := from
	for start <= to {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("eth_getLogs %s %s blocks %d-%d: %w", token.Symbol, config.HostOnly(c.rawURL), from, to, err)
		}
		if remaining := to - start + 1; size > remaining {
			size = remaining
		}
		end := start + size - 1
		logs, err := c.FetchLogs(ctx, token, topics, start, end)
		if err != nil {
			// Classify cap before throttle: a misroute either dead-ends dense
			// catch-up (cap into backoff exhaustion) or amplifies throttling
			// (throttle into immediate retries) — Pitfall 1.
			if isResultCapErr(err) {
				if size <= floor {
					return nil, fmt.Errorf("eth_getLogs %s %s blocks %d-%d: result cap persists at window_floor %d — halving cannot shrink the sub-range further; failing closed, the range is never skipped (SYNC-02)",
						token.Symbol, config.HostOnly(c.rawURL), start, end, floor)
				}
				size /= 2
				if size < 1 {
					size = 1
				}
				continue // same starting block, no backoff
			}
			// Throttle: withRateLimitRetry already applied bounded backoff on
			// this exact sub-range; exhaustion (or a permanent error) fails.
			return nil, err
		}
		out = append(out, logs...)
		start = end + 1
	}
	return out, nil
}

// Header returns the canonical block header (hash + timestamp) at number.
func (c *Client) Header(ctx context.Context, number uint64) (*types.Header, error) {
	num := new(big.Int).SetUint64(number)
	h, err := c.headerByNumber(ctx, num, fmt.Sprintf("block %d", number))
	if err != nil {
		return nil, err
	}
	return h, nil
}

// headerByNumber fetches a header by big.Int number (negative values are the
// go-ethereum block tags) with rate-limit retry and HostOnly error wrapping.
func (c *Client) headerByNumber(ctx context.Context, num *big.Int, what string) (*types.Header, error) {
	var h *types.Header
	if err := withRateLimitRetry(ctx, func() error {
		var err error
		h, err = c.ec.HeaderByNumber(ctx, num)
		return err
	}); err != nil {
		return nil, fmt.Errorf("eth_getBlockByNumber %s %s: %w", config.HostOnly(c.rawURL), what, err)
	}
	return h, nil
}

// EligibleHead resolves the highest block the configured head policy allows
// the sync loop to ingest (D-01): "finalized" reads the consensus finalized
// tag; "confirmed" reads latest and subtracts confirmation_blocks (clamped
// at 0). Both paths carry the provider's canonical hash for that height.
func (c *Client) EligibleHead(ctx context.Context, cfg *config.Config) (uint64, common.Hash, error) {
	switch cfg.HeadPolicy {
	case "finalized":
		// go-ethereum serializes the tag constant to "finalized" itself
		// (02-RESEARCH "Don't Hand-Roll", verified in v1.17.7).
		h, err := c.headerByNumber(ctx, big.NewInt(int64(ethrpc.FinalizedBlockNumber)), "tag finalized")
		if err != nil {
			return 0, common.Hash{}, err
		}
		if h == nil || h.Number == nil {
			return 0, common.Hash{}, fmt.Errorf("eligible head %s: finalized tag returned no header", config.HostOnly(c.rawURL))
		}
		return h.Number.Uint64(), h.Hash(), nil
	case "confirmed":
		var head uint64
		if err := withRateLimitRetry(ctx, func() error {
			var err error
			head, err = c.ec.BlockNumber(ctx)
			return err
		}); err != nil {
			return 0, common.Hash{}, fmt.Errorf("eth_blockNumber %s: %w", config.HostOnly(c.rawURL), err)
		}
		height := uint64(0) // head shallower than the depth: nothing is eligible
		if head > cfg.ConfirmationBlocks {
			height = head - cfg.ConfirmationBlocks
		}
		h, err := c.Header(ctx, height)
		if err != nil {
			return 0, common.Hash{}, err
		}
		return height, h.Hash(), nil
	default:
		return 0, common.Hash{}, fmt.Errorf("eligible head %s: unknown head_policy %q (want \"finalized\" or \"confirmed\")", config.HostOnly(c.rawURL), cfg.HeadPolicy)
	}
}
