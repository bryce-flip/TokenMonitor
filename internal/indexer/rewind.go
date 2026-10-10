package indexer

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"TOkenMonitor/internal/config"
)

// Verified rewind — the SYNC-05 recovery path (D-03). The sync loop only ever
// DETECTS the mismatch and halts (CheckpointMismatchError); this file is the
// only code that deletes events, it runs exclusively from the operator-gated
// `rewind` subcommand, and it deletes strictly above the verified rewind
// point. A reorg deep enough to displace a checkpointed finalized block is a
// catastrophic-consensus or hostile-provider event: the walk is bounded and
// never auto-deep-dives.

// DefaultRewindMaxWalk bounds the ancestor walk below the checkpoint.
const DefaultRewindMaxWalk = 1000

// Rewind errors. All are named so the CLI and the tests can branch on them;
// none of them ever leaves stored data touched.
var (
	// ErrRewindNoCheckpoint: the chain has no recorded checkpoint.
	ErrRewindNoCheckpoint = errors.New("rewind: no checkpoint recorded for this chain — nothing to rewind")
	// ErrRewindWalkTooDeep: no agreeing ancestor within the bounded walk.
	ErrRewindWalkTooDeep = errors.New("rewind: no agreeing ancestor within the bounded walk — a displaced finalized checkpoint is a catastrophic or hostile-provider event (D-03); refusing to rewind further without operator investigation")
	// ErrRewindToMismatch: the operator's --to override failed verification.
	ErrRewindToMismatch = errors.New("rewind: --to override rejected")
	// ErrRewindUnconfirmed: the computed plan was not executed (no --yes).
	// The non-nil report alongside it IS the plan; the CLI prints it, prompts,
	// and re-invokes with Yes set.
	ErrRewindUnconfirmed = errors.New("rewind: plan computed but not confirmed (Yes/--yes executes it)")
)

// RewindStore is the storage surface the verified rewind needs.
// *storage.Store satisfies it structurally (same shape as EventStore plus the
// rewind queries; rewind.go must not touch sync.go — parallel-plan ownership).
type RewindStore interface {
	ReadCheckpoint(ctx context.Context, chain string) (height uint64, blockHash string, found bool, err error)
	WriteCheckpoint(ctx context.Context, chain string, height uint64, blockHash string) error
	// DeleteCheckpointAbove drops checkpoint rows strictly above after. The
	// re-anchor insert alone is invisible: ReadCheckpoint returns the max
	// height, so the orphaned chain's rows above the rewind point would keep
	// winning the read pre-merge and post-merge alike.
	DeleteCheckpointAbove(ctx context.Context, chain string, after uint64) error
	DeleteEventsFrom(ctx context.Context, chain string, after uint64) error
	StoredBlockHashes(ctx context.Context, chain string, from, to uint64) (map[uint64]string, error)
	RangeTotals(ctx context.Context, chain string, from, to uint64) (count uint64, sum *big.Int, err error)
}

// RangeTotalsResult is one FINAL (count, sum(raw_amount)) measurement over
// the affected range, shown before and after the rewind's delete.
type RangeTotalsResult struct {
	Count uint64
	Sum   *big.Int
}

// RewindOpts parameterizes Rewind. To overrides the walked rewind point after
// verification against the stored rows at that height; Yes gates the
// destructive steps; MaxWalk bounds the ancestor walk (0 = the 1000 default).
type RewindOpts struct {
	To      uint64
	Yes     bool
	MaxWalk uint64
}

// RewindReport carries everything the operator-facing plan/report prints.
type RewindReport struct {
	CheckpointHeight uint64
	ExpectedHash     string // the hash the checkpoint records
	ObservedHash     string // the provider's canonical hash at that height
	RewindPoint      uint64
	RewindHash       string // the provider-canonical hash re-anchored at the point
	Before           RangeTotalsResult
	After            RangeTotalsResult // Sum == nil until the destructive steps ran
	// AfterTotalsUnavailable marks a report whose delete and re-anchor DID
	// run but whose after-totals measurement failed: the report is partial
	// on purpose so the operator still sees exactly what was destroyed
	// (WR-03) — the accompanying error names the failed measurement.
	AfterTotalsUnavailable bool
	DeletedRange           string
}

// Rewind performs the verified rewind (D-03, SYNC-05):
//
//  1. read the checkpoint (none -> named error, nothing happens);
//  2. record expected (stored) vs observed (canonical) hashes at its height;
//  3. walk down from the checkpoint while the stored per-block hashes
//     disagree with the provider's canonical headers — the rewind point is
//     the first height where they agree, or the first height with no stored
//     rows (nothing there can be orphaned); the walk aborts with a named
//     error past MaxWalk;
//  4. an explicit To overrides the walked point only after the stored rows at
//     that height (if any) are verified to match the canonical hash there;
//  5. collect before-totals over (point, checkpoint]; without Yes return the
//     plan with ErrRewindUnconfirmed (the CLI prompts — Rewind itself never
//     asks);
//  6. delete events strictly above the point, drop the superseded checkpoint
//     rows strictly above the point, and re-anchor the checkpoint at the
//     point with the provider-canonical hash; collect after-totals.
//
// cfg is accepted for signature stability with the sync-side seams; the chain
// key is the package constant.
func Rewind(ctx context.Context, cr ChainReader, rs RewindStore, cfg *config.Config, opts RewindOpts) (*RewindReport, error) {
	_ = cfg

	cpHeight, cpHash, found, err := rs.ReadCheckpoint(ctx, Chain)
	if err != nil {
		return nil, fmt.Errorf("rewind: read checkpoint: %w", err)
	}
	if !found {
		return nil, ErrRewindNoCheckpoint
	}
	observed, err := cr.Header(ctx, cpHeight)
	if err != nil {
		return nil, fmt.Errorf("rewind: canonical header at checkpoint %d: %w", cpHeight, err)
	}

	maxWalk := opts.MaxWalk
	if maxWalk == 0 {
		maxWalk = DefaultRewindMaxWalk
	}
	lower := uint64(0)
	if cpHeight > maxWalk {
		lower = cpHeight - maxWalk
	}
	stored, err := rs.StoredBlockHashes(ctx, Chain, lower, cpHeight)
	if err != nil {
		return nil, fmt.Errorf("rewind: stored block hashes %d-%d: %w", lower, cpHeight, err)
	}

	// The walk steps down from the checkpoint, comparing the stored per-block
	// hash against the provider's canonical header at each height. A height
	// with no stored rows only terminates the walk when no stored rows remain
	// AT OR BELOW it (minStored): row-less heights above stored rows are
	// passed through, because those lower rows may still be orphaned.
	minStored := cpHeight + 1 // no stored rows in the window at all
	for h := range stored {
		if h < minStored {
			minStored = h
		}
	}
	point, pointHash := uint64(0), ""
	for h, walked := cpHeight, uint64(0); ; h, walked = h-1, walked+1 {
		if walked > maxWalk {
			return nil, fmt.Errorf("%w (walked %d blocks below checkpoint %d)", ErrRewindWalkTooDeep, walked, cpHeight)
		}
		if sh, ok := stored[h]; !ok {
			if h > minStored {
				continue // pass through: stored rows below may still be orphaned
			}
			// First height with no stored rows at or below it: nothing
			// further down can be orphaned; re-anchoring here is safe and
			// the next sync pass re-verifies the anchor.
			canonical, err := cr.Header(ctx, h)
			if err != nil {
				return nil, fmt.Errorf("rewind: canonical header during walk at %d: %w", h, err)
			}
			point, pointHash = h, canonical.Hash().Hex()
			break
		} else {
			canonical, err := cr.Header(ctx, h)
			if err != nil {
				return nil, fmt.Errorf("rewind: canonical header during walk at %d: %w", h, err)
			}
			if sh == canonical.Hash().Hex() {
				point, pointHash = h, sh
				break
			}
		}
		if h == 0 {
			return nil, fmt.Errorf("%w (reached genesis below checkpoint %d without agreement)", ErrRewindWalkTooDeep, cpHeight)
		}
	}

	if opts.To != 0 {
		if opts.To > cpHeight {
			return nil, fmt.Errorf("%w: --to %d is above the checkpoint %d", ErrRewindToMismatch, opts.To, cpHeight)
		}
		canonical, err := cr.Header(ctx, opts.To)
		if err != nil {
			return nil, fmt.Errorf("rewind: canonical header at --to %d: %w", opts.To, err)
		}
		if sh, ok := stored[opts.To]; ok && sh != canonical.Hash().Hex() {
			return nil, fmt.Errorf("%w: stored rows at %d carry %s but the provider's canonical header is %s — nothing was deleted or re-anchored",
				ErrRewindToMismatch, opts.To, sh, canonical.Hash().Hex())
		}
		point, pointHash = opts.To, canonical.Hash().Hex()
	}

	beforeN, beforeSum, err := rs.RangeTotals(ctx, Chain, point+1, cpHeight)
	if err != nil {
		return nil, fmt.Errorf("rewind: before totals %d-%d: %w", point+1, cpHeight, err)
	}
	if beforeSum == nil {
		beforeSum = new(big.Int)
	}
	report := &RewindReport{
		CheckpointHeight: cpHeight,
		ExpectedHash:     cpHash,
		ObservedHash:     observed.Hash().Hex(),
		RewindPoint:      point,
		RewindHash:       pointHash,
		Before:           RangeTotalsResult{Count: beforeN, Sum: beforeSum},
		DeletedRange:     fmt.Sprintf("block_number > %d", point),
	}
	if !opts.Yes {
		return report, ErrRewindUnconfirmed
	}

	// Destructive steps — strictly above the verified point only: the delete
	// predicate is block_number > point, so rows at or below the point are
	// never touched (must_have prohibition).
	if err := rs.DeleteEventsFrom(ctx, Chain, point); err != nil {
		return nil, fmt.Errorf("rewind: delete events above %d: %w", point, err)
	}
	// The checkpoint rows above the point are the orphaned chain's advance
	// history; ReadCheckpoint returns the max height, so they must go before
	// the re-anchor is written or the re-anchor loses the read forever
	// (pre-merge to the ordered read, post-merge to the ReplacingMergeTree
	// survivor). Same strictly-above rule as the events delete.
	if err := rs.DeleteCheckpointAbove(ctx, Chain, point); err != nil {
		return nil, fmt.Errorf("rewind: delete checkpoint above %d: %w", point, err)
	}
	if err := rs.WriteCheckpoint(ctx, Chain, point, pointHash); err != nil {
		return nil, fmt.Errorf("rewind: re-anchor checkpoint at %d: %w", point, err)
	}
	// Phase 4 adds derived-rollup rebuild over the rewound range at this seam
	// (Phase 2 has no derived tables — events-only rewind is complete here).
	afterN, afterSum, err := rs.RangeTotals(ctx, Chain, point+1, cpHeight)
	if err != nil {
		// The destructive steps already ran — never swallow the report: the
		// operator must see the rewind point, the delete range, and the
		// before-totals even when the closing measurement fails (WR-03).
		report.AfterTotalsUnavailable = true
		return report, fmt.Errorf("rewind: delete and re-anchor completed at %d, but after-totals failed: %w", point, err)
	}
	if afterSum == nil {
		afterSum = new(big.Int)
	}
	report.After = RangeTotalsResult{Count: afterN, Sum: afterSum}
	return report, nil
}
