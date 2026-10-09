// Continuous checkpointed sync (Phase 2, plans 02-01): follow the configured
// eligible head in bounded windows, verify the durable checkpoint against the
// provider before each window, and advance the checkpoint only after the
// window's events are durably accepted (SYNC-01/SYNC-03/SYNC-04).
package indexer

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"TOkenMonitor/internal/config"
	"TOkenMonitor/internal/storage"
)

// ChainReader is the RPC surface RunSync needs. *rpc.Client satisfies it
// structurally; this package cannot import internal/rpc (rpc imports
// indexer for SupplyTopics — that would be an import cycle), so the loop
// depends on the seam, never the concrete client.
type ChainReader interface {
	EligibleHead(ctx context.Context, cfg *config.Config) (uint64, common.Hash, error)
	Header(ctx context.Context, number uint64) (*types.Header, error)
	// FetchLogsResilient carries the D-05 window policy (sub-range halving
	// under result caps, fail-closed floor, bounded throttle backoff); the
	// loop never advances the checkpoint on its failure (SYNC-02).
	FetchLogsResilient(ctx context.Context, token config.Token, topics []common.Hash, from, to, window, floor uint64) ([]types.Log, error)
}

// EventStore is the storage surface RunSync needs. *storage.Store satisfies
// it structurally (indexer may import storage; no cycle).
type EventStore interface {
	InsertEvents(ctx context.Context, rows []storage.EventRow) error
	ReadCheckpoint(ctx context.Context, chain string) (height uint64, blockHash string, found bool, err error)
	WriteCheckpoint(ctx context.Context, chain string, height uint64, blockHash string) error
}

// Chain is the stablecoin_events/indexer_checkpoint chain key (Mainnet only,
// CHAIN-01).
const Chain = "ethereum"

// CheckpointMismatchError reports that the provider's canonical header at
// the checkpointed height no longer carries the checkpointed hash (SYNC-05
// detection seed). Ordinary indexing halts before any window is ingested;
// recovery is the operator-gated rewind (D-03), never an automatic reset.
type CheckpointMismatchError struct {
	Height        uint64
	ExpectedHash  string
	ObservedHash  string
}

func (e *CheckpointMismatchError) Error() string {
	return fmt.Sprintf("checkpoint hash mismatch at block %d: checkpoint records %s but the provider's canonical header is %s; ordinary indexing halted — verify the chain and run the rewind subcommand (D-03)",
		e.Height, e.ExpectedHash, e.ObservedHash)
}

// RunSync maintains the continuous ingest loop: resolve the eligible head
// (D-01), seed or verify the durable checkpoint (D-04/SYNC-05), ingest one
// bounded window per pass capped at the head (SYNC-01), and advance the
// checkpoint strictly after the window's InsertEvents is acked (SYNC-03).
// It returns only on error or context cancellation, never between windows.
func RunSync(ctx context.Context, cr ChainReader, es EventStore, cfg *config.Config) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		head, headHash, err := cr.EligibleHead(ctx, cfg)
		if err != nil {
			return fmt.Errorf("eligible head: %w", err)
		}
		cpHeight, cpHash, found, err := es.ReadCheckpoint(ctx, Chain)
		if err != nil {
			return fmt.Errorf("read checkpoint: %w", err)
		}
		if !found {
			// D-04: first run seeds at start_block when configured (fetching
			// its canonical hash), otherwise at the current eligible head —
			// indexing forward only, never the seed block itself.
			seed, seedHash := head, headHash.Hex()
			if cfg.StartBlock != 0 {
				if cfg.StartBlock > head {
					return fmt.Errorf("sync seed: start_block %d is above the eligible head %d; nothing before the head may be ingested", cfg.StartBlock, head)
				}
				h, err := cr.Header(ctx, cfg.StartBlock)
				if err != nil {
					return fmt.Errorf("sync seed header at start_block %d: %w", cfg.StartBlock, err)
				}
				seed, seedHash = cfg.StartBlock, h.Hash().Hex()
			}
			if err := es.WriteCheckpoint(ctx, Chain, seed, seedHash); err != nil {
				return fmt.Errorf("seed checkpoint at %d: %w", seed, err)
			}
			slog.Info("seeded sync checkpoint", "height", seed, "hash", seedHash, "head", head, "start_block", cfg.StartBlock)
			continue
		}
		if cpHeight >= head {
			// Caught up: sleep, do not error, do not insert, do not advance
			// (SYNC-01 idle). Verification is skipped while idle (02-RESEARCH
			// Open Question 1 resolution).
			select {
			case <-time.After(time.Duration(cfg.PollSeconds) * time.Second):
			case <-ctx.Done():
				return ctx.Err()
			}
			continue
		}
		// Verify the checkpoint against the provider's canonical header
		// before ingesting (both at resume and before every window ingest).
		vh, err := cr.Header(ctx, cpHeight)
		if err != nil {
			return fmt.Errorf("verify checkpoint header at %d: %w", cpHeight, err)
		}
		if vh.Hash().Hex() != cpHash {
			return &CheckpointMismatchError{Height: cpHeight, ExpectedHash: cpHash, ObservedHash: vh.Hash().Hex()}
		}
		windowEnd := min(cpHeight+cfg.WindowBlocks, head)
		if err := ingestWindow(ctx, cr, es, cfg, cpHeight+1, windowEnd, head); err != nil {
			return err
		}
	}
}

// ingestWindow fetches, decodes, and stores one window [start, end] and then
// advances the checkpoint to the window-end height with that header's hash.
func ingestWindow(ctx context.Context, cr ChainReader, es EventStore, cfg *config.Config, start, end, head uint64) error {
	type tokenLog struct {
		token config.Token
		log   types.Log
	}
	var fetched []tokenLog
	blocks := map[uint64]struct{}{}
	for _, token := range cfg.Tokens {
		topics := SupplyTopics(token)
		if topics == nil {
			slog.Warn("no supply topics for token; skipping", "token", token.Symbol)
			continue
		}
		logs, err := cr.FetchLogsResilient(ctx, token, topics, start, end, cfg.WindowBlocks, cfg.WindowFloor)
		if err != nil {
			return fmt.Errorf("fetch %s blocks %d-%d: %w", token.Symbol, start, end, err)
		}
		for _, lg := range logs {
			fetched = append(fetched, tokenLog{token, lg})
			blocks[lg.BlockNumber] = struct{}{}
		}
	}

	// One guaranteed header per window: the window-end block anchors the
	// checkpoint (02-RESEARCH Pitfall 6). Per-event-block headers follow only
	// where events exist (Phase 1 WR-01 behavior).
	headers := map[uint64]*types.Header{}
	endHeader, err := cr.Header(ctx, end)
	if err != nil {
		return fmt.Errorf("window-end header at %d: %w", end, err)
	}
	headers[end] = endHeader
	for bn := range blocks {
		if _, ok := headers[bn]; ok {
			continue
		}
		h, err := cr.Header(ctx, bn)
		if err != nil {
			return fmt.Errorf("event-block header at %d: %w", bn, err)
		}
		headers[bn] = h
	}

	var rows []storage.EventRow
	for _, tl := range fetched {
		ev, err := DecodeSupplyEvent(tl.token, tl.log)
		if err != nil {
			return fmt.Errorf("decode tx %s log %d: %w", tl.log.TxHash.Hex(), tl.log.Index, err)
		}
		if ev == nil {
			continue
		}
		h := headers[tl.log.BlockNumber]
		if h == nil {
			return fmt.Errorf("missing header for event block %d", tl.log.BlockNumber)
		}
		// WR-01: the provider's header must attest the log's block hash.
		if h.Hash() != tl.log.BlockHash {
			return fmt.Errorf("reorg detected in window %d-%d: header hash %s at block %d does not match the log's block hash %s; the window stays un-checkpointed and will be retried",
				start, end, h.Hash().Hex(), tl.log.BlockNumber, tl.log.BlockHash.Hex())
		}
		rows = append(rows, storage.EventRow{
			Chain:           Chain,
			Token:           ev.Token,
			ContractAddress: strings.ToLower(common.HexToAddress(tl.token.Contract).Hex()),
			BlockNumber:     tl.log.BlockNumber,
			BlockHash:       tl.log.BlockHash.Hex(), // the hash the provider attests emitted this log
			BlockTime:       time.Unix(int64(h.Time), 0).UTC(),
			TxHash:          tl.log.TxHash.Hex(),
			LogIndex:        uint32(tl.log.Index),
			EventType:       ev.EventType,
			FromAddress:     ev.FromAddress,
			ToAddress:       ev.ToAddress,
			RawAmount:       ev.Amount,
		})
	}

	// Advance-after-accept (SYNC-03): the checkpoint moves only after the
	// window's events are durably accepted. On insert failure the checkpoint
	// stays put and the next pass re-processes this exact window; FINAL dedup
	// collapses the replay (SYNC-04). Never per-log inserts.
	if err := es.InsertEvents(ctx, rows); err != nil {
		return fmt.Errorf("insert events window %d-%d: %w", start, end, err)
	}
	if err := es.WriteCheckpoint(ctx, Chain, end, endHeader.Hash().Hex()); err != nil {
		return fmt.Errorf("write checkpoint at %d: %w", end, err)
	}
	slog.Info("window stored",
		"window_from", start, "window_to", end,
		"eligible_head", head, "checkpoint", end, "rows", len(rows))
	return nil
}
