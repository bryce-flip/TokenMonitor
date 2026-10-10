// Command indexer continuously maintains the Mainnet supply-event history:
// load config -> fail-fast provider probe (D-02) -> follow the configured
// eligible head in bounded windows -> decode -> ClickHouse insert -> advance
// the durable checkpoint (SYNC-01/SYNC-03). The default subcommand is `run`;
// `rewind` is the operator-gated recovery path for a checkpoint hash mismatch
// (D-03, SYNC-05).
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"os"
	"os/signal"
	"strings"
	"text/tabwriter"

	"TOkenMonitor/internal/config"
	"TOkenMonitor/internal/indexer"
	"TOkenMonitor/internal/rpc"
	"TOkenMonitor/internal/storage"
)

func main() {
	configPath := flag.String("config", "config/tokens.json", "path to token config JSON")
	fromFlag := flag.Uint64("from", 0, "seed the sync checkpoint at this block (backfill opt-in, D-04); 0 starts at the current eligible head")
	flag.Parse()

	sub, subArgs := "run", []string(nil)
	if args := flag.Args(); len(args) > 0 {
		sub, subArgs = args[0], args[1:]
	}
	switch sub {
	case "run":
		runCommand(*configPath, *fromFlag)
	case "rewind":
		rewindCommand(*configPath, subArgs)
	default:
		fmt.Fprintf(os.Stderr, "error: unknown subcommand %q; usage: indexer [run | rewind]\n", sub)
		os.Exit(2)
	}
}

// runCommand is the continuous ingest path (SYNC-01..SYNC-05 detection).
func runCommand(configPath string, from uint64) {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}
	if from != 0 {
		cfg.StartBlock = from
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
	defer client.Close()

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

// rewindCommand is the operator-gated verified rewind (D-03, SYNC-05): it
// computes the plan (checkpoint expected/observed hashes, walked rewind
// point, before totals), requires an explicit interactive confirmation
// unless --yes is passed, executes the delete strictly above the verified
// point, re-anchors the checkpoint, and prints the before/after report. Any
// verification failure exits non-zero with nothing deleted past the last
// reported step.
func rewindCommand(configPath string, args []string) {
	fs := flag.NewFlagSet("rewind", flag.ExitOnError)
	toFlag := fs.Uint64("to", 0, "rewind point override; verified against stored rows before use (0 = walk to the first agreeing ancestor)")
	yesFlag := fs.Bool("yes", false, "skip the interactive confirmation prompt (scripted use)")
	_ = fs.Parse(args)

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
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
	defer client.Close()

	// Minimal startup check (no ingest, no sample window): the probe's chain
	// and head-boundary assertions against the eligible head.
	eligible, _, err := client.EligibleHead(ctx, cfg)
	if err != nil {
		slog.Error("eligible head", "err", err)
		os.Exit(1)
	}
	if err := client.Probe(ctx, cfg, eligible, eligible); err != nil {
		slog.Error("provider probe failed", "err", err)
		os.Exit(1)
	}

	store, err := storage.Open(ctx, config.ClickHouseURL())
	if err != nil {
		slog.Error("storage", "err", err)
		os.Exit(1)
	}
	if err := store.EnsureSchema(ctx); err != nil {
		slog.Error("storage", "err", err)
		os.Exit(1)
	}

	opts := indexer.RewindOpts{To: *toFlag, Yes: *yesFlag}
	report, err := indexer.Rewind(ctx, client, store, cfg, opts)
	if errors.Is(err, indexer.ErrRewindUnconfirmed) {
		printRewindReport(os.Stdout, report)
		if !confirmRewind(os.Stdin, os.Stderr) {
			slog.Error("rewind aborted: explicit \"yes\" required; nothing was deleted")
			os.Exit(1)
		}
		opts.Yes = true
		report, err = indexer.Rewind(ctx, client, store, cfg, opts)
	}
	if err != nil {
		// A non-nil report means the destructive steps already ran (WR-03):
		// print what happened before exiting on the error — the operator
		// must see the rewind point, the delete range, and the before-totals
		// even when the closing measurement failed.
		if report != nil {
			printRewindReport(os.Stdout, report)
		}
		slog.Error("rewind failed", "err", err)
		os.Exit(1)
	}
	printRewindReport(os.Stdout, report)
	slog.Info("rewind complete; restart the indexer to resume ordinary sync",
		"rewind_point", report.RewindPoint, "rewind_hash", report.RewindHash)
}

// printRewindReport renders the rewind plan/result in the Phase 1 tabwriter
// style. After.Sum is nil until the destructive steps have run, so a
// not-yet-confirmed plan simply shows the before side; a report whose
// after-totals measurement failed shows the unavailable marker instead
// (WR-03) — the delete and re-anchor still ran.
func printRewindReport(w io.Writer, r *indexer.RewindReport) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "field\tvalue")
	fmt.Fprintf(tw, "checkpoint height\t%d\n", r.CheckpointHeight)
	fmt.Fprintf(tw, "checkpoint hash (expected)\t%s\n", r.ExpectedHash)
	fmt.Fprintf(tw, "provider canonical hash (observed)\t%s\n", r.ObservedHash)
	fmt.Fprintf(tw, "verified rewind point\t%d\n", r.RewindPoint)
	fmt.Fprintf(tw, "rewind point hash (canonical)\t%s\n", r.RewindHash)
	fmt.Fprintf(tw, "delete range\t%s\n", r.DeletedRange)
	fmt.Fprintf(tw, "FINAL count in range (before)\t%d\n", r.Before.Count)
	fmt.Fprintf(tw, "FINAL sum in range (before)\t%s\n", amountString(r.Before.Sum))
	switch {
	case r.AfterTotalsUnavailable:
		fmt.Fprintln(tw, "FINAL totals in range (after)\tunavailable — the measurement failed; the delete and re-anchor DID run")
	case r.After.Sum != nil:
		fmt.Fprintf(tw, "FINAL count in range (after)\t%d\n", r.After.Count)
		fmt.Fprintf(tw, "FINAL sum in range (after)\t%s\n", amountString(r.After.Sum))
	}
	tw.Flush()
}

func amountString(v *big.Int) string {
	if v == nil {
		return "0"
	}
	return v.String()
}

// confirmRewind prompts on stdin and accepts nothing but an explicit yes.
func confirmRewind(in io.Reader, out io.Writer) bool {
	fmt.Fprintln(out, "The delete above the verified rewind point is destructive and trusts the provider's canonical chain.")
	fmt.Fprint(out, `Type "yes" to proceed: `)
	line, err := bufio.NewReader(in).ReadString('\n')
	return err == nil && strings.TrimSpace(line) == "yes"
}
