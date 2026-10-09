// Package storage persists supply events in ClickHouse over the native
// protocol. Raw amounts travel as *big.Int end to end — never float — so the
// UInt256 column round-trips exactly.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"TOkenMonitor/internal/config"
)

// EventRow mirrors the stablecoin_events columns.
type EventRow struct {
	Chain           string
	Token           string
	ContractAddress string
	BlockNumber     uint64
	BlockHash       string
	BlockTime       time.Time
	TxHash          string
	LogIndex        uint32
	EventType       string
	FromAddress     string
	ToAddress       string
	RawAmount       *big.Int
}

// Store is a lazily connected ClickHouse handle.
type Store struct {
	conn driver.Conn
}

// Open connects from a clickhouse:// URL (native protocol).
func Open(ctx context.Context, url string) (*Store, error) {
	opts, err := clickhouse.ParseDSN(url)
	if err != nil {
		return nil, fmt.Errorf("clickhouse parse %s: %w", config.HostOnly(url), err)
	}
	conn, err := clickhouse.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("clickhouse open %s: %w", config.HostOnly(url), err)
	}
	return &Store{conn: conn}, nil
}

// migrationCandidates lets EnsureSchema resolve migrations/clickhouse.sql
// both from the repo root (the indexer command) and from internal/storage
// during tests.
var migrationCandidates = []string{"migrations/clickhouse.sql", "../../migrations/clickhouse.sql"}

// EnsureSchema applies migrations/clickhouse.sql (idempotent DDL). The file
// is split into individual statements: the native protocol rejects
// multi-statement requests ("Multi-statements are not allowed").
func (s *Store) EnsureSchema(ctx context.Context) error {
	var ddl []byte
	for _, p := range migrationCandidates {
		if b, err := os.ReadFile(p); err == nil {
			ddl = b
			break
		}
	}
	if ddl == nil {
		return fmt.Errorf("clickhouse migration not found in %v", migrationCandidates)
	}
	for _, stmt := range splitStatements(string(ddl)) {
		if err := s.conn.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("clickhouse ensure schema: %w", err)
		}
	}
	return nil
}

// splitStatements splits a semicolon-terminated SQL file into statements.
// Comment lines are dropped before splitting: the file's comments contain
// both semicolons and apostrophes, which would otherwise cut a comment in
// half and poison the parser. The migration convention is full-line "--"
// comments only — no inline trailing comments.
func splitStatements(ddl string) []string {
	var code []string
	for _, line := range strings.Split(ddl, "\n") {
		if t := strings.TrimSpace(line); t == "" || strings.HasPrefix(t, "--") {
			continue
		}
		code = append(code, line)
	}
	var stmts []string
	for _, stmt := range strings.Split(strings.Join(code, "\n"), ";") {
		if strings.TrimSpace(stmt) != "" {
			stmts = append(stmts, stmt)
		}
	}
	return stmts
}

// InsertEvents batch-inserts rows; an empty slice is a no-op.
func (s *Store) InsertEvents(ctx context.Context, rows []EventRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, `INSERT INTO stablecoin_events
		(chain, token, contract_address, block_number, block_hash, block_time,
		 tx_hash, log_index, event_type, from_address, to_address, raw_amount)`)
	if err != nil {
		return fmt.Errorf("clickhouse prepare batch: %w", err)
	}
	for _, r := range rows {
		if err := batch.Append(
			r.Chain, r.Token, r.ContractAddress,
			r.BlockNumber, r.BlockHash, r.BlockTime,
			r.TxHash, r.LogIndex, r.EventType,
			r.FromAddress, r.ToAddress, r.RawAmount,
		); err != nil {
			return fmt.Errorf("clickhouse append %s log %d: %w", r.TxHash, r.LogIndex, err)
		}
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("clickhouse send batch: %w", err)
	}
	return nil
}

// Inspect returns up to limit rows in the inclusive block range [from, to],
// newest first (block_number then log_index descending). FINAL is mandatory:
// ReplacingMergeTree background replacement is eventual.
func (s *Store) Inspect(ctx context.Context, from, to uint64, limit uint32) ([]EventRow, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT chain, token, contract_address, block_number, block_hash, block_time,
		       tx_hash, log_index, event_type, from_address, to_address, raw_amount
		FROM stablecoin_events FINAL
		WHERE block_number BETWEEN ? AND ?
		ORDER BY block_number DESC, log_index DESC
		LIMIT ?`, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("clickhouse inspect: %w", err)
	}
	defer rows.Close()
	var out []EventRow
	for rows.Next() {
		var r EventRow
		amount := new(big.Int)
		if err := rows.Scan(
			&r.Chain, &r.Token, &r.ContractAddress,
			&r.BlockNumber, &r.BlockHash, &r.BlockTime,
			&r.TxHash, &r.LogIndex, &r.EventType,
			&r.FromAddress, &r.ToAddress, amount,
		); err != nil {
			return nil, fmt.Errorf("clickhouse inspect scan: %w", err)
		}
		r.RawAmount = amount
		out = append(out, r)
	}
	return out, rows.Err()
}

// WriteCheckpoint records one durable sync checkpoint for chain at
// (height, blockHash). The sync loop calls it only after the window's
// InsertEvents returned nil — advance-after-accept (SYNC-03).
func (s *Store) WriteCheckpoint(ctx context.Context, chain string, height uint64, blockHash string) error {
	batch, err := s.conn.PrepareBatch(ctx, `INSERT INTO indexer_checkpoint (chain, height, block_hash)`)
	if err != nil {
		return fmt.Errorf("clickhouse prepare checkpoint batch: %w", err)
	}
	if err := batch.Append(chain, height, blockHash); err != nil {
		return fmt.Errorf("clickhouse append checkpoint height %d: %w", height, err)
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("clickhouse send checkpoint batch: %w", err)
	}
	return nil
}

// ReadCheckpoint returns the newest durable checkpoint for chain. The read
// is ORDER BY height DESC LIMIT 1 — never FINAL and never an unordered scan:
// ReplacingMergeTree replacement is eventual, so pre-merge multiple physical
// rows are normal, and the ordered read returns the max height both pre-merge
// and post-merge (02-RESEARCH Pattern 2, probe-verified on 26.8.20.9).
func (s *Store) ReadCheckpoint(ctx context.Context, chain string) (height uint64, blockHash string, found bool, err error) {
	err = s.conn.QueryRow(ctx,
		`SELECT height, block_hash FROM indexer_checkpoint WHERE chain = ? ORDER BY height DESC LIMIT 1`,
		chain,
	).Scan(&height, &blockHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, "", false, nil
		}
		return 0, "", false, fmt.Errorf("clickhouse read checkpoint: %w", err)
	}
	return height, blockHash, true, nil
}
