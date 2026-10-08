// Package rpc is a thin Ethereum JSON-RPC reader over go-ethereum's
// ethclient. Every error message renders the endpoint through
// config.HostOnly so provider credentials embedded in the URL never reach
// logs (D-01, T-01-02).
package rpc

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"

	"TOkenMonitor/internal/config"
	"TOkenMonitor/internal/indexer"
)

// rateLimitBackoffs paces retries of throttled RPC reads. Free-tier
// providers reject bursty requests (observed live: HTTP 429 / JSON-RPC
// -32005); requirements.md §24 requires the indexer to tolerate them.
var rateLimitBackoffs = []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}

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
	_, err = c.ec.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(from),
		ToBlock:   new(big.Int).SetUint64(from),
		Addresses: []common.Address{common.HexToAddress(token.Contract)},
		Topics:    [][]common.Hash{indexer.SupplyTopics(token)},
	})
	if err != nil {
		return fmt.Errorf("probe %s: sample eth_getLogs at block %d for %s: %w (provider may not serve historical logs)",
			config.HostOnly(c.rawURL), from, token.Symbol, err)
	}
	return nil
}

// FetchLogs returns the logs of token's supply topics over [from, to],
// filtered by the token's configured contract address.
func (c *Client) FetchLogs(ctx context.Context, token config.Token, topics []common.Hash, from, to uint64) ([]types.Log, error) {
	logs, err := c.ec.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(from),
		ToBlock:   new(big.Int).SetUint64(to),
		Addresses: []common.Address{common.HexToAddress(token.Contract)},
		Topics:    [][]common.Hash{topics},
	})
	if err != nil {
		return nil, fmt.Errorf("eth_getLogs %s %s blocks %d-%d: %w", token.Symbol, config.HostOnly(c.rawURL), from, to, err)
	}
	return logs, nil
}

// Header returns the canonical block header (hash + timestamp) at number.
func (c *Client) Header(ctx context.Context, number uint64) (*types.Header, error) {
	h, err := c.ec.HeaderByNumber(ctx, new(big.Int).SetUint64(number))
	if err != nil {
		return nil, fmt.Errorf("eth_getBlockByNumber %s block %d: %w", config.HostOnly(c.rawURL), number, err)
	}
	return h, nil
}
