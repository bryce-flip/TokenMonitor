---
quick_id: 261016-bmv
type: execute
autonomous: true
files_modified:
  - docs/DEPLOYMENT.md
  - README.md
---

# Deployment guide and pull request

Create a Chinese deployment guide for the implemented Go indexer and ClickHouse path,
then commit, push, and open a PR from `docs/deployment-guide` to `main`.

## Scope

- Base the branch on `origin/main`; preserve the six local main-only planning commits.
- Preserve pre-existing uncommitted planning changes and exclude them from commits.
- Ground commands in `cmd/indexer/main.go`, `internal/config/config.go`,
  `internal/indexer/sync.go`, `internal/storage/clickhouse.go`,
  `migrations/clickhouse.sql`, `config/tokens.json`, and `go.mod`.
- Update the README entry point and its stale description of range halving.
- Document the current single-host deployment; Compose, Grafana, supply reconciliation,
  and resource sizing remain future work.

## Tasks

1. Write `docs/DEPLOYMENT.md`: prerequisites, build, persistent authenticated ClickHouse,
   external credentials/config, systemd with SIGINT, verification, restart, upgrade,
   rollback, and verified rewind. Explain exclusive seed semantics and single-writer use.
2. Validate shell blocks, relative links, build and offline tests; exercise the pinned
   ClickHouse setup and SQL in an isolated container when the daemon is available.
3. Commit only task-owned changes, record the quick task in `.planning/STATE.md`,
   push `docs/deployment-guide`, and create a reviewable PR targeting `main`.

<threat_model>
ASVS level 1; block on high severity. Keep RPC credentials outside Git, bind the database
to loopback, require a database password, persist events and checkpoints in one volume,
and require chain verification plus explicit confirmation before destructive rewind.
Validation must use isolated storage, with production RPC and database variables unset.
</threat_model>

## Acceptance

- Documented flags, paths, tables, defaults, and recovery behavior match the source.
- Build/tests and available deployment checks have recorded outcomes and limits.
- PR diff contains the guide, README updates, and this quick task's planning records only.
- Remote branch exists and an open PR targets `main`; merging is left for review.
