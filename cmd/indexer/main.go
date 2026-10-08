// Command indexer runs one bounded Mainnet ingest over a configured block
// range: load config -> fail-fast provider probe (D-02) -> per-token
// filtered eth_getLogs -> decode -> ClickHouse insert -> SQL inspection.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"TOkenMonitor/internal/config"
	"TOkenMonitor/internal/indexer"
	"TOkenMonitor/internal/rpc"
	"TOkenMonitor/internal/storage"
)

func main() {
	configPath := flag.String("config", "config/tokens.json", "path to token config JSON")
	fromFlag := flag.Uint64("from", 0, "first block of the range (inclusive)")
	toFlag := flag.Uint64("to", 0, "last block of the range (inclusive)")
	flag.Parse()

	from, to := *fromFlag, *toFlag
	set := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { set[f.Name] = true })
	switch {
	case set["from"] != set["to"]:
		fmt.Fprintln(os.Stderr, "error: -from and -to must be provided together")
		os.Exit(1)
	case !set["from"]:
		fmt.Fprintln(os.Stderr, "error: -from and -to are required (bounded block range)")
		os.Exit(1)
	case from > to:
		fmt.Fprintf(os.Stderr, "error: -from (%d) must be <= -to (%d)\n", from, to)
		os.Exit(1)
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	ctx := context.Background()

	cfg, err := config.Load(*configPath)
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
	if err := client.Probe(ctx, cfg, from, to); err != nil {
		slog.Error("provider probe failed", "err", err)
		os.Exit(1)
	}
	slog.Info("provider probe passed", "from", from, "to", to)

	type tokenLog struct {
		token config.Token
		log   types.Log
	}
	var fetched []tokenLog
	blocks := map[uint64]struct{}{}
	for _, token := range cfg.Tokens {
		topics := indexer.SupplyTopics(token)
		if topics == nil {
			slog.Warn("no supply topics for token; skipping", "token", token.Symbol)
			continue
		}
		logs, err := client.FetchLogs(ctx, token, topics, from, to)
		if err != nil {
			slog.Error("fetch", "err", err)
			os.Exit(1)
		}
		slog.Info("fetched logs", "token", token.Symbol, "count", len(logs))
		for _, lg := range logs {
			fetched = append(fetched, tokenLog{token, lg})
			blocks[lg.BlockNumber] = struct{}{}
		}
	}

	headers := map[uint64]*types.Header{}
	for bn := range blocks {
		h, err := client.Header(ctx, bn)
		if err != nil {
			slog.Error("header", "block", bn, "err", err)
			os.Exit(1)
		}
		headers[bn] = h
	}

	chain := "ethereum"
	var rows []storage.EventRow
	for _, tl := range fetched {
		ev, err := indexer.DecodeSupplyEvent(tl.token, tl.log)
		if err != nil {
			slog.Error("decode failed", "tx", tl.log.TxHash.Hex(), "log_index", tl.log.Index, "err", err)
			os.Exit(1)
		}
		if ev == nil {
			continue
		}
		h := headers[tl.log.BlockNumber]
		if h.Hash() != tl.log.BlockHash {
			slog.Error("reorg detected: header hash does not match the log's block hash; re-run the range",
				"block", tl.log.BlockNumber, "header_hash", h.Hash().Hex(), "log_block_hash", tl.log.BlockHash.Hex())
			os.Exit(1)
		}
		rows = append(rows, storage.EventRow{
			Chain:           chain,
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

	store, err := storage.Open(ctx, config.ClickHouseURL())
	if err != nil {
		slog.Error("storage", "err", err)
		os.Exit(1)
	}
	if err := store.EnsureSchema(ctx); err != nil {
		slog.Error("storage", "err", err)
		os.Exit(1)
	}
	if err := store.InsertEvents(ctx, rows); err != nil {
		slog.Error("storage", "err", err)
		os.Exit(1)
	}
	inspected, err := store.Inspect(ctx, from, to, 20)
	if err != nil {
		slog.Error("storage", "err", err)
		os.Exit(1)
	}

	fmt.Printf("stored %d supply events for range %d-%d\n", len(rows), from, to)
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "block\ttime (UTC)\ttoken\tevent\traw_amount\tfrom\tto\ttx_hash\tlog_index")
	for _, r := range inspected {
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\n",
			r.BlockNumber, r.BlockTime.Format(time.RFC3339), r.Token, r.EventType,
			r.RawAmount.String(), orDash(r.FromAddress), orDash(r.ToAddress),
			r.TxHash, r.LogIndex)
	}
	w.Flush()
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
