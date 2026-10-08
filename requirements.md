# Ethereum Stablecoin Monitor

## 1. 项目目标

构建一个稳定币监控系统，第一阶段只监控 Ethereum Mainnet 上的：

* USDT
* USDC

系统通过 Ethereum 链上原始数据识别稳定币的：

* Mint
* Burn
* Total Supply
* Net Issuance
* Daily Issuance
* 7D Net Issuance
* 30D Net Issuance

最终通过 Grafana 提供监控 Dashboard。

### 核心目标

系统需要能够回答：

1. Ethereum 上当前有多少 USDT / USDC？
2. 今天发行了多少？
3. 今天销毁了多少？
4. 今天净增加多少？
5. 最近 7 天净增加多少？
6. 最近 30 天净增加多少？
7. 最近是否发生了大额 Mint / Burn？
8. 链上计算出来的 Supply 是否与 Token Contract 的 `totalSupply()` 一致？

---

# 2. MVP 范围

## 2.1 支持链

第一阶段：

```text
Ethereum Mainnet
```

暂不支持：

```text
Tron
Solana
Base
Arbitrum
Avalanche
其他链
```

后续通过 Adapter 扩展。

---

# 3. 支持 Token

第一阶段：

| Token | Issuer | Ethereum Contract                            | Decimals |
| ----- | ------ | -------------------------------------------- | -------: |
| USDT  | Tether | `0xdAC17F958D2ee523a2206206994597C13D831ec7` |        6 |
| USDC  | Circle | `0xA0b86991c6218b36c1d19d4a2e9eb0ce3606eb48` |        6 |

Token 配置不能硬编码到业务逻辑中，应使用配置文件。

示例：

```yaml
chain:
  name: ethereum
  chain_id: 1

tokens:
  - symbol: USDT
    issuer: Tether
    contract: "0xdAC17F958D2ee523a2206206994597C13D831ec7"
    decimals: 6

  - symbol: USDC
    issuer: Circle
    contract: "0xA0b86991c6218b36c1d19d4a2e9eb0ce3606eb48"
    decimals: 6
```

---

# 4. 核心数据来源

## 4.1 Ethereum RPC

系统需要 Ethereum RPC Provider。

支持：

```text
HTTP RPC
WebSocket RPC
```

MVP 至少支持 HTTP RPC。

推荐通过环境变量配置：

```bash
ETH_RPC_URL=
ETH_WS_URL=
```

不要把 RPC URL 写死。

---

# 5. 链上数据采集

## 5.1 ERC20 Transfer Event

核心 Event：

```solidity
event Transfer(
    address indexed from,
    address indexed to,
    uint256 value
);
```

Topic：

```text
keccak256("Transfer(address,address,uint256)")
```

系统只需要过滤：

```text
USDT contract
USDC contract
```

不需要扫描整个 Ethereum 的 ERC20 Transfer。

---

# 6. Mint / Burn 识别

## 6.1 Mint

如果：

```text
from == 0x0000000000000000000000000000000000000000
```

则认为：

```text
MINT
```

例如：

```text
Transfer(
    0x0000000000000000000000000000000000000000,
    0x1234...,
    100000000
)
```

表示：

```text
Mint = 100 USDT
```

具体数量需要根据 token decimals 转换。

---

## 6.2 Burn

如果：

```text
to == 0x0000000000000000000000000000000000000000
```

则认为：

```text
BURN
```

---

## 6.3 普通 Transfer

如果：

```text
from != zero
to != zero
```

则：

```text
NORMAL_TRANSFER
```

MVP 不需要保存普通 Transfer。

但 Indexer 可以保留扩展能力。

---

# 7. Raw Event 数据

建立原始 Mint/Burn Event 表。

建议：

```sql
CREATE TABLE stablecoin_events
(
    chain String,
    token String,
    contract_address String,

    block_number UInt64,
    block_hash String,
    block_time DateTime,

    tx_hash String,
    log_index UInt32,

    event_type LowCardinality(String),

    from_address String,
    to_address String,

    raw_amount UInt256,
    amount Decimal(38, 6),

    created_at DateTime DEFAULT now()
)
ENGINE = ReplacingMergeTree(created_at)
ORDER BY (
    chain,
    token,
    block_number,
    tx_hash,
    log_index
);
```

`event_type`：

```text
MINT
BURN
```

---

# 8. 数据唯一性

Event 唯一标识：

```text
chain
+
block_number
+
tx_hash
+
log_index
```

必须保证重复执行 Indexer 不会产生重复数据。

Indexer 必须支持：

```text
idempotency
```

---

# 9. Block Indexing

Indexer 按 Block Range 工作。

例如：

```text
fromBlock = 20,000,000
toBlock   = 20,001,000
```

通过：

```text
eth_getLogs
```

获取 USDT / USDC Transfer Events。

流程：

```text
Latest Block
     ↓
Calculate Block Range
     ↓
eth_getLogs
     ↓
Decode Logs
     ↓
Filter Mint/Burn
     ↓
Write ClickHouse
     ↓
Update Checkpoint
```

---

# 10. Checkpoint

Indexer 必须记录处理进度。

例如：

```sql
CREATE TABLE indexer_checkpoint
(
    chain String,
    indexer String,
    last_processed_block UInt64,
    updated_at DateTime
)
ENGINE = ReplacingMergeTree(updated_at)
ORDER BY (
    chain,
    indexer
);
```

Indexer 重启后：

```text
读取 last_processed_block
        ↓
继续处理
```

而不是从 Genesis 重新开始。

---

# 11. Reorg 处理

Ethereum 存在短暂 Chain Reorganization。

MVP 必须预留 confirmation 机制。

例如：

```text
latest block
     ↓
latest - N
     ↓
只处理 finalized-safe blocks
```

建议 MVP 使用：

```text
confirmation_blocks = 20
```

作为配置，而不是写死。

例如：

```yaml
ethereum:
  confirmations: 20
```

后续可以支持 Ethereum finalized block。

---

# 12. Supply 计算

核心公式：

```text
Supply(t)
=
Initial Supply
+
Σ Mint
-
Σ Burn
```

但 MVP 不建议只依赖累计 Mint/Burn 计算 Supply。

系统需要定期调用：

```text
ERC20.totalSupply()
```

进行校验。

---

# 13. Supply Snapshot

建立 Supply Snapshot：

```sql
CREATE TABLE stablecoin_supply_snapshot
(
    timestamp DateTime,

    chain String,
    token String,

    calculated_supply Decimal(38, 6),
    contract_supply Decimal(38, 6),

    difference Decimal(38, 6),

    created_at DateTime DEFAULT now()
)
ENGINE = MergeTree
ORDER BY (
    chain,
    token,
    timestamp
);
```

其中：

```text
calculated_supply
```

来自：

```text
Mint - Burn
```

而：

```text
contract_supply
```

来自：

```text
eth_call -> totalSupply()
```

---

# 14. Supply 校验

核心监控：

```text
calculated_supply
        VS
contract.totalSupply()
```

正常：

```text
difference = 0
```

如果：

```text
difference != 0
```

则产生异常。

例如：

```text
USDT
Calculated: 81,500,000,000
Contract:   81,510,000,000

Difference: 10,000,000
```

需要报警。

---

# 15. Issuance Metrics

## Daily Mint

```text
Daily Mint
=
当天所有 MINT amount 之和
```

## Daily Burn

```text
Daily Burn
=
当天所有 BURN amount 之和
```

## Daily Net Issuance

```text
Daily Net Issuance
=
Daily Mint
-
Daily Burn
```

---

# 16. Rolling Metrics

需要支持：

```text
24H Net Issuance
7D Net Issuance
30D Net Issuance
```

公式：

```text
7D Net Issuance
=
7D Mint
-
7D Burn
```

---

# 17. 大额事件

支持配置：

```yaml
alerts:
  mint_threshold_usd: 100000000
  burn_threshold_usd: 100000000
```

例如：

```text
USDT Mint
Amount: $500M
Block: 23,xxx,xxx
TX: 0x...
Time: 2026-10-08 10:30:00
```

MVP 可以先只记录，不实现通知。

后续支持：

```text
Telegram
Discord
Email
Webhook
```

---

# 18. ClickHouse 数据模型

建议至少有三类数据：

```text
stablecoin_events
        ↓
原始 Mint/Burn

stablecoin_supply_snapshot
        ↓
Supply 时间序列

indexer_checkpoint
        ↓
Indexer 状态
```

后续可以增加：

```text
stablecoin_daily_metrics
```

用于 Dashboard 查询。

---

# 19. Daily Metrics

建议建立：

```sql
CREATE TABLE stablecoin_daily_metrics
(
    date Date,

    chain String,
    token String,

    mint_amount Decimal(38, 6),
    burn_amount Decimal(38, 6),
    net_issuance Decimal(38, 6),

    end_supply Decimal(38, 6)
)
ENGINE = MergeTree
ORDER BY (
    chain,
    token,
    date
);
```

---

# 20. Grafana Dashboard

第一版 Dashboard 分为 4 个区域。

## 20.1 Current Supply

```text
Ethereum Stablecoin Supply

USDT    $XXX B
USDC    $XXX B
Total   $XXX B
```

---

## 20.2 Supply Trend

```text
USDT Supply
USDC Supply
Total Supply
```

时间范围：

```text
24H
7D
30D
90D
1Y
```

---

## 20.3 Issuance

展示：

```text
Daily Mint
Daily Burn
Daily Net Issuance
```

---

## 20.4 Large Events

例如：

```text
Time        Token   Type   Amount      TX
10:30       USDT    MINT   $500M       0x...
09:20       USDC    BURN   $200M       0x...
```

---

# 21. API

Indexer 和 Dashboard 可以先不做复杂 Backend。

MVP 可以直接：

```text
ClickHouse
    ↓
Grafana
```

如果后续需要 Web Dashboard，再增加：

```text
Go API
    ↓
ClickHouse
```

API 可以设计为：

```text
GET /api/v1/stablecoins
GET /api/v1/stablecoins/{symbol}/supply
GET /api/v1/stablecoins/{symbol}/issuance
GET /api/v1/stablecoins/{symbol}/events
```

---

# 22. 项目结构

推荐：

```text
stablecoin-monitor/
│
├── cmd/
│   └── indexer/
│       └── main.go
│
├── config/
│   └── tokens.yaml
│
├── internal/
│   ├── ethereum/
│   │   ├── client.go
│   │   ├── logs.go
│   │   └── blocks.go
│   │
│   ├── indexer/
│   │   ├── indexer.go
│   │   ├── decoder.go
│   │   └── checkpoint.go
│   │
│   ├── token/
│   │   ├── token.go
│   │   └── erc20.go
│   │
│   ├── storage/
│   │   └── clickhouse.go
│   │
│   └── metrics/
│       └── issuance.go
│
├── migrations/
│   └── clickhouse.sql
│
├── deployments/
│   ├── docker-compose.yml
│   └── grafana/
│
├── configs/
│   └── config.yaml
│
├── Dockerfile
├── Makefile
└── README.md
```

---

# 23. 技术要求

## Programming Language

```text
Go
```

## Database

```text
ClickHouse
```

## Visualization

```text
Grafana
```

## Blockchain

```text
Ethereum Mainnet
```

## RPC

使用标准 Ethereum JSON-RPC。

主要使用：

```text
eth_blockNumber
eth_getLogs
eth_getBlockByNumber
eth_call
```

---

# 24. Indexer 要求

Indexer 必须满足：

### 可靠性

* 服务重启后可以继续同步
* 不重复写入 Event
* 支持 Block Range
* 支持 RPC Error Retry
* 支持 Rate Limit
* 支持 Reorg

### 可观测性

至少提供：

```text
current_block
latest_block
sync_lag
processed_blocks
processed_events
rpc_errors
indexer_errors
```

可以使用 Prometheus Metrics。

---

# 25. Docker Compose

MVP 使用：

```text
Docker Compose

├── stablecoin-indexer
├── clickhouse
└── grafana
```

RPC 不需要自己部署 Ethereum Node。

第一阶段使用外部 RPC Provider。

RPC URL：

```bash
ETH_RPC_URL=
```

---

# 26. 数据源验证

系统启动后需要执行：

```text
1. 获取当前 Ethereum block

2. 获取 USDT totalSupply()

3. 获取 USDC totalSupply()

4. 从历史 Mint/Burn 计算 Supply

5. 对比两者

6. 输出 difference
```

如果 difference 不为 0：

```text
WARN supply mismatch
```

---

# 27. MVP Acceptance Criteria

项目完成必须满足：

### AC1

能够从 Ethereum 获取 USDT / USDC Transfer Logs。

### AC2

能够正确识别：

```text
Mint
Burn
```

### AC3

Indexer 重启不会重复数据。

### AC4

能够持续同步最新 Ethereum Block。

### AC5

能够计算：

```text
Daily Mint
Daily Burn
Daily Net Issuance
7D Net Issuance
30D Net Issuance
```

### AC6

能够获取：

```text
USDT totalSupply()
USDC totalSupply()
```

### AC7

Calculated Supply 与 Contract Supply 能够进行校验。

### AC8

Grafana 可以展示：

```text
Current Supply
Supply Trend
Mint
Burn
Net Issuance
Large Events
```

### AC9

出现 RPC 错误、Indexer 停止同步、Supply mismatch 时能够记录明确日志。

---

# 28. 后续扩展

MVP 完成后，不修改核心数据模型，通过 Adapter 扩展。

## Chain Adapter

```text
EthereumAdapter
TronAdapter
SolanaAdapter
```

## Token Adapter

```text
USDT
USDC
DAI
USDS
PYUSD
USDe
FDUSD
```

最终：

```text
                    Stablecoin Monitor
                           │
             ┌─────────────┴─────────────┐
             │                           │
          Chain Layer                Token Layer
             │                           │
      Ethereum / Tron / Solana     USDT / USDC / ...
             │                           │
             └─────────────┬─────────────┘
                           │
                       Normalized
                         Events
                           │
                      ClickHouse
                           │
                        Grafana
```

---

# 29. 第一阶段明确不做

为了控制 MVP 范围，以下内容暂时不实现：

* 多链
* 多稳定币
* Ethereum 全节点部署
* 普通 ERC20 Transfer 全量索引
* 钱包地址标签
* 交易所识别
* Whale tracking
* Telegram 通知
* Web 前端
* 跨链 USDC CCTP 分析
* 跨链 USDT 分析
* DeFi Protocol 分析

---

# 30. 开发顺序

推荐 Codex 按以下顺序实现：

### Step 1

项目初始化：

```text
Go
Docker Compose
ClickHouse
Grafana
```

### Step 2

Ethereum RPC Client。

### Step 3

Block Sync。

### Step 4

USDT / USDC Transfer Log Indexer。

### Step 5

Mint / Burn Decoder。

### Step 6

ClickHouse Storage。

### Step 7

Checkpoint + Restart Recovery。

### Step 8

Reorg / Confirmation。

### Step 9

Supply Calculator。

### Step 10

Contract `totalSupply()` 校验。

### Step 11

Daily / 7D / 30D Metrics。

### Step 12

Grafana Dashboard。

### Step 13

Metrics / Logging / Alert。

---

# 31. 核心设计原则

整个系统遵循：

```text
Raw Blockchain Data
        ↓
Normalized Events
        ↓
Derived Metrics
        ↓
Dashboard
```

不要直接：

```text
Ethereum
   ↓
Grafana
```

所有计算指标都应该能够从 Raw Event 重新计算。

这样未来如果：

```text
计算逻辑改变
Bug 修复
Token 增加
统计口径改变
```

可以重新生成指标，而不需要重新扫描 Ethereum。

---

# 32. MVP 最终产出

完成后系统应该能够提供：

```text
Ethereum Stablecoin Monitor
```

并回答：

┌──────────────────────────────────┐
│ Ethereum Stablecoin Monitor      │
├──────────────────────────────────┤
│                                  │
│ USDT Supply          $XXX B       │
│ USDC Supply          $XXX B       │
│                                  │
│ 24H Net Issuance     +$XXX M      │
│ 7D Net Issuance      +$XXX B      │
│ 30D Net Issuance     +$XXX B      │
│                                  │
│ ───── Supply Trend ───────────    │
│                                  │
│ USDT    ╱────────────             │
│ USDC   ╱─────────────             │
│                                  │
│ ───── Large Events ───────────   │
│                                  │
│ USDT MINT   $500M                 │
│ USDC BURN   $200M                 │
│                                  │
└──────────────────────────────────┘

```

这就是第一阶段完整的 MVP。

我建议你**不要直接让 Codex 一次性把整个需求实现完**。把上面的文档放进项目后，让 Codex 按 **Step 1 → Step 13** 逐步开发，每一步跑测试、验证数据，再进入下一步。

尤其第一关先要求它完成：

> **Ethereum RPC → USDT/USDC Transfer Logs → 正确识别 Mint/Burn → ClickHouse**

这一关跑通以后，整个项目的核心技术路线基本就验证了。
```

