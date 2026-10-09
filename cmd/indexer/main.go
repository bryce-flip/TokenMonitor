// Command indexer continuously maintains the Mainnet supply-event history:
// load config -> fail-fast provider probe (D-02) -> follow the configured
// eligible head in bounded windows -> decode -> ClickHouse insert -> advance
// the durable checkpoint (SYNC-01/SYNC-03). The default subcommand is `run`.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"TOkenMonitor/internal/config"
	"TOkenMonitor/internal/indexer"
	"TOkenMonitor/internal/rpc"
	"TOkenMonitor/internal/storage"
)

func main() {
	configPath := flag.String("config", "config/tokens.json", "path to token config JSON")
	fromFlag := flag.Uint64("from", 0, "seed the sync checkpoint at this block (backfill opt-in, D-04); 0 starts at the current eligible head")
	flag.Parse()

	sub := "run"
	if args := flag.Args(); len(args) > 0 {
		sub = args[0]
	}
	if sub != "run" {
		fmt.Fprintf(os.Stderr, "error: unknown subcommand %q; usage: indexer [run]\n", sub)
		os.Exit(2)
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}
	if *fromFlag != 0 {
		cfg.StartBlock = *fromFlag
	}
	if err := cfg.Validate(); err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}

	rpcURL, err := config.RPCURL()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	client, err := rpc.New(ctx, rpcURL)
	if err != nil {
		slog.Error("rpc", "err", err)
		os.Exit(1)
	}

	// Probe at the eligible head (or the seed when start_block sits below
	// it), so the Phase 1 confirmed-range assertion stays satisfied under
	// both head policies (D-01).
	eligible, _, err := client.EligibleHead(ctx, cfg)
	if err != nil {
		slog.Error("eligible head", "err", err)
		os.Exit(1)
	}
	probeAt := eligible
	if cfg.StartBlock != 0 && cfg.StartBlock < eligible {
		probeAt = cfg.StartBlock
	}
	if err := client.Probe(ctx, cfg, probeAt, probeAt); err != nil {
		slog.Error("provider probe failed", "err", err)
		os.Exit(1)
	}
	slog.Info("provider probe passed", "probed_block", probeAt, "eligible_head", eligible, "head_policy", cfg.HeadPolicy)

	store, err := storage.Open(ctx, config.ClickHouseURL())
	if err != nil {
		slog.Error("storage", "err", err)
		os.Exit(1)
	}
	if err := store.EnsureSchema(ctx); err != nil {
		slog.Error("storage", "err", err)
		os.Exit(1)
	}

	slog.Info("starting continuous sync",
		"head_policy", cfg.HeadPolicy, "window_blocks", cfg.WindowBlocks,
		"window_floor", cfg.WindowFloor, "poll_seconds", cfg.PollSeconds,
		"start_block", cfg.StartBlock)
	err = indexer.RunSync(ctx, client, store, cfg)
	if errors.Is(err, context.Canceled) {
		slog.Info("sync stopped (interrupt)")
		return
	}
	if err != nil {
		var mm *indexer.CheckpointMismatchError
		if errors.As(err, &mm) {
			slog.Error("checkpoint hash mismatch: ordinary indexing and reporting halted; verify the chain and run the rewind subcommand (D-03)",
				"block", mm.Height, "expected_hash", mm.ExpectedHash, "observed_hash", mm.ObservedHash)
			os.Exit(1)
		}
		slog.Error("sync stopped", "err", err)
		os.Exit(1)
	}
}
