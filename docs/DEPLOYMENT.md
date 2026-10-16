# 部署指南

本文部署当前已实现的 **Ethereum Mainnet USDT / USDC 事件索引器**：Go 二进制运行在宿主机，ClickHouse 运行在同机 Docker 中，systemd 管理索引进程。当前可查询铸造、赎回、销毁事件和同步检查点；供应量锚定、`totalSupply()` 对账、Grafana 和 Docker Compose 尚未交付，不能据此宣称已得到绝对供应量。

```text
外部 Ethereum Mainnet HTTP RPC
             ↓
      Go indexer（systemd）
             ↓ native TCP / 127.0.0.1:9000
      ClickHouse（Docker）
             ↓
      tm-clickhouse-data 持久卷
```

## 1. 环境与构建

需要使用 systemd 的 Linux 主机、Bash、Git、Go **1.27.1**（见 [go.mod](../go.mod)）、Docker Engine，以及可运行 Docker 的部署账号。RPC 由部署者提供，必须连接 Ethereum Mainnet（chain ID 1），支持历史 `eth_getLogs` 和区块头查询；默认策略还需要 `finalized` 标签。镜像沿用仓库固定版本 `clickhouse/clickhouse-server:26.8.20.9`。

以下使用 `/opt/tokenmonitor` 存放源码及迁移文件。已有检出可复用；若更换目录，必须同步修改后文的 `WorkingDirectory`。

```bash
sudo install -d -m 0755 -o "$(id -un)" -g "$(id -gn)" /opt/tokenmonitor
git clone https://github.com/bryce-flip/TokenMonitor.git /opt/tokenmonitor
cd /opt/tokenmonitor
git rev-parse HEAD
go version
docker version
go mod download
env -u ETH_RPC_URL -u CLICKHOUSE_URL -u DEVNET_RPC_URL go test ./...
go build -trimpath -o /tmp/tokenmonitor-indexer ./cmd/indexer
sudo install -m 0755 /tmp/tokenmonitor-indexer /usr/local/bin/tokenmonitor-indexer
```

私有仓库需要先配置 GitHub 访问权限。记录本次部署的提交 SHA。离线测试会跳过 Mainnet、ClickHouse 和 devnet 集成验证；完整测试入口见 [README](../README.md)。

**必须保留工作目录下的 `migrations/clickhouse.sql`**：程序启动时从相对路径读取它并自动执行幂等建表，单独复制二进制无法完成部署。

## 2. 启动持久化 ClickHouse

以下仅用于首次创建。输入密码管理器生成的高强度字母数字密码，后续 DSN 使用同一密码；若使用特殊字符，需要对 DSN 中的密码进行 URL 编码。密码通过环境传入，不写入仓库或命令历史。

```bash
read -r -s -p 'ClickHouse password: ' CLICKHOUSE_PASSWORD
printf '\n'
export CLICKHOUSE_PASSWORD
docker volume create tm-clickhouse-data
docker run -d --name tm-clickhouse \
  --restart unless-stopped \
  --ulimit nofile=262144:262144 \
  -p 127.0.0.1:9000:9000 \
  -v tm-clickhouse-data:/var/lib/clickhouse \
  -e CLICKHOUSE_DB=tokenmonitor \
  -e CLICKHOUSE_USER=tokenmonitor \
  -e CLICKHOUSE_PASSWORD \
  clickhouse/clickhouse-server:26.8.20.9
unset CLICKHOUSE_PASSWORD
```

只发布本机 native 协议端口，不需要开放 HTTP 8123。事件与检查点都存放在同一持久卷中，容器重建时应复用该卷。数据卷和用户初始化参数依据 [ClickHouse 官方 Docker 文档](https://clickhouse.com/docs/get-started/setup/self-managed/docker)。

等待初始化完成后检查连接；若尚未就绪，查看 `docker logs --tail 50 tm-clickhouse` 后重试：

```bash
docker exec tm-clickhouse sh -c 'exec clickhouse-client \
  --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" \
  --database "$CLICKHOUSE_DB" --query "SELECT version(), currentDatabase()"'
```

预期返回版本 `26.8.20.9`、数据库 `tokenmonitor`。已有同名容器时先核对挂载、数据库及认证配置；后续启停使用 `docker start tm-clickhouse` / `docker stop tm-clickhouse`，不要重复执行首次创建命令。README 的无密码本地调试容器不能直接当作此部署复用。

## 3. 配置运行账号与凭据

创建专用系统用户（已存在时跳过 `useradd`），复制配置到仓库外：

```bash
sudo useradd --system --user-group --no-create-home --shell /usr/sbin/nologin tokenmonitor
sudo install -d -o root -g tokenmonitor -m 0750 /etc/tokenmonitor
sudo install -o root -g tokenmonitor -m 0640 config/tokens.json /etc/tokenmonitor/tokens.json
sudo -u tokenmonitor test -r /opt/tokenmonitor/migrations/clickhouse.sql
sudoedit /etc/tokenmonitor/indexer.env
```

在 `indexer.env` 中写入以下两项，并替换占位内容；不加 `export`，也不要提交此文件：

```ini
ETH_RPC_URL=REPLACE_WITH_ETHEREUM_MAINNET_HTTP_RPC_URL
CLICKHOUSE_URL=clickhouse://tokenmonitor:REPLACE_WITH_PASSWORD@127.0.0.1:9000/tokenmonitor
```

```bash
sudo chown root:root /etc/tokenmonitor/indexer.env
sudo chmod 0600 /etc/tokenmonitor/indexer.env
sudoedit /etc/tokenmonitor/tokens.json
```

systemd 会读取环境文件并传给服务进程。应用本身不自动加载 `.env`。部署必须显式设置 `CLICKHOUSE_URL`，否则会回落到本地 `default` 用户和 `default` 数据库。

配置定义见 [config.go](../internal/config/config.go)，初始值见 [tokens.json](../config/tokens.json)：

| 字段 | 仓库值 / 默认行为 | 操作说明 |
| --- | --- | --- |
| `chain_id`、`tokens` | Mainnet；USDT / USDC，均为 6 位小数 | 保留已核对的合约地址；当前仅支持这两种代币 |
| `head_policy` | `finalized` | 优先使用；提供者不支持时，显式改为 `confirmed` 并设置确认深度 |
| `confirmation_blocks` | 20 | 仅对 `confirmed` 生效，必须大于 0 |
| `window_blocks` / `window_floor` | 5000 / 100 | 遇到已识别的日志结果上限或查询超时会缩小范围，到下限仍失败则退出 |
| `poll_seconds` | 60 | 追平 eligible head 后的轮询间隔 |
| `start_block` | 未设置 | 首次以当前 eligible head 建检查点，只索引之后的区块，不补历史 |

**首次回填要在启动前决定。** `start_block = N` 是检查点种子，实际从 **N + 1** 开始；若要包含区块 B，应设置为 B − 1。CLI 的 `-from N` 语义相同，只在没有检查点时决定种子。已有检查点时会继续原进度，修改起点不会触发历史重建；不要通过删除检查点来补历史。确需独立回填时使用新的空数据库，并单独核对覆盖范围。

全局选项必须放在子命令前，例如 `tokenmonitor-indexer -config /etc/tokenmonitor/tokens.json -from 12345678 run`；当前没有 `-to` 一次性结束参数。同一个数据库仅运行一个索引进程，维护或回填前先停止现有实例。

## 4. 使用 systemd 运行

用 `sudoedit /etc/systemd/system/tokenmonitor.service` 创建以下 unit：

```ini
[Unit]
Description=Ethereum Stablecoin Monitor indexer
Wants=network-online.target
After=network-online.target docker.service
Requires=docker.service
StartLimitIntervalSec=300
StartLimitBurst=3

[Service]
Type=simple
User=tokenmonitor
Group=tokenmonitor
WorkingDirectory=/opt/tokenmonitor
EnvironmentFile=/etc/tokenmonitor/indexer.env
ExecStart=/usr/local/bin/tokenmonitor-indexer -config /etc/tokenmonitor/tokens.json run
KillSignal=SIGINT
TimeoutStopSec=90
Restart=on-failure
RestartSec=15

[Install]
WantedBy=multi-user.target
```

`WorkingDirectory` 保证迁移文件可读，`-config` 指向外部配置。当前程序只处理 `os.Interrupt`，因此停机信号必须使用 `SIGINT`。重试次数受启动频率限制，持续故障需人工处理。unit 语义参见 [systemd.service](https://www.freedesktop.org/software/systemd/man/latest/systemd.service.html)。

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now tokenmonitor
sudo systemctl status tokenmonitor --no-pager
sudo journalctl -u tokenmonitor -n 50 --no-pager
```

正常启动会出现 `provider probe passed`、`starting continuous sync`；空数据库还会出现 `seeded sync checkpoint`。有新的 eligible 区块后应出现 `window stored`，其中包含窗口边界、检查点和事件行数。没有新块时进程会等待；`rows=0` 也可以正常推进检查点。

## 5. 验收与日常启停

检查表、检查点和最近事件：

```bash
docker exec -i tm-clickhouse sh -c 'exec clickhouse-client \
  --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" \
  --database "$CLICKHOUSE_DB" --multiquery' <<'SQL'
SHOW TABLES;

SELECT chain, height, block_hash, updated_at
FROM indexer_checkpoint
WHERE chain = 'ethereum'
ORDER BY height DESC LIMIT 1;

SELECT token, event_type, toString(raw_amount) AS raw_amount,
       block_number, block_hash, block_time, tx_hash, log_index
FROM stablecoin_events FINAL
WHERE chain = 'ethereum'
ORDER BY block_number DESC, log_index DESC LIMIT 20;
SQL
```

应存在 `stablecoin_events` 和 `indexer_checkpoint`。在 eligible head 推进后重复查询，确认检查点跟进并结合日志核对延迟；不能只看事件表是否增加，因为供给事件可能稀疏。首次从链头启动时，事件结果暂时为空是正常现象。

事件读取必须使用 `FINAL` 去除重放重复项；检查点按 `height DESC LIMIT 1` 读取。`raw_amount` 是精确原始整数，`1000000` 对两种代币都表示 1 token，不要转成浮点数做精确核算。事件分类为 `mint`、`redeem`、`destroyed_black_funds`、`burn`。

```bash
sudo systemctl stop tokenmonitor
sudo systemctl start tokenmonitor
sudo systemctl restart tokenmonitor
sudo journalctl -u tokenmonitor -f
```

启停后从持久检查点恢复；写入事件后、写入检查点前发生中断会重放该窗口，查询时由 `FINAL` 去重。停止整套服务时先停索引器，再停 ClickHouse；启动顺序相反，并先确认数据库可连接。

不要把存储或 rewind 集成测试指向运行数据库：测试会插入夹具并执行删除。需要验证时使用独立测试容器或数据库。

## 6. 升级、备份与回滚

升级前记录旧提交 SHA，备份 `/etc/tokenmonitor`，并对事件表和检查点做同一时间点的数据库备份。采用存储卷快照时，先停索引器和 ClickHouse，再快照 `tm-clickhouse-data`，完成后先恢复数据库。持久卷本身不等于备份。

以下在 `/opt/tokenmonitor` 的同一 Bash 会话中执行，以经过审核的提交 SHA 发布。先停服务再切换迁移源码；构建失败时按下方回滚步骤恢复旧源码及二进制后启动：

```bash
cd /opt/tokenmonitor
PREVIOUS_COMMIT=$(git rev-parse HEAD)
read -r -p 'Target commit SHA: ' RELEASE_COMMIT
git fetch origin
sudo systemctl stop tokenmonitor
sudo cp /usr/local/bin/tokenmonitor-indexer /usr/local/bin/tokenmonitor-indexer.previous
git checkout --detach "$RELEASE_COMMIT"
env -u ETH_RPC_URL -u CLICKHOUSE_URL -u DEVNET_RPC_URL go test ./...
go build -trimpath -o /tmp/tokenmonitor-indexer ./cmd/indexer
```

仅在测试和构建成功后替换并重启：

```bash
sudo install -m 0755 /tmp/tokenmonitor-indexer /usr/local/bin/tokenmonitor-indexer
sudo systemctl reset-failed tokenmonitor
sudo systemctl start tokenmonitor
sudo journalctl -u tokenmonitor -n 50 --no-pager
```

按第 5 节重新验收。若新旧版本的数据库结构和数据语义兼容，可回滚二进制及对应迁移源码：

```bash
sudo systemctl stop tokenmonitor
git checkout --detach "$PREVIOUS_COMMIT"
sudo install -m 0755 /usr/local/bin/tokenmonitor-indexer.previous /usr/local/bin/tokenmonitor-indexer
sudo systemctl reset-failed tokenmonitor
sudo systemctl start tokenmonitor
```

若发布包含不兼容迁移，先按该版本的恢复方案处理数据库；不能仅换旧二进制。恢复备份时必须让事件与检查点保持同一时间点，不能只恢复其中一项。不要删除数据卷来修复启动错误。

## 7. 故障排查与重组恢复

| 现象 | 检查 / 处理 |
| --- | --- |
| `ETH_RPC_URL is not set`、chain ID 不符、provider probe 失败 | 检查环境文件、RPC Mainnet 网络、历史日志权限和 `finalized` 支持；配置修改后重启 |
| `clickhouse migration not found` | 核对 `WorkingDirectory` 和 `migrations/clickhouse.sql`；`-config` 不改变迁移路径 |
| 数据库连接、认证或建表失败 | 先运行第 2 节连接检查，核对 DSN、密码编码、数据库名、权限和容器状态 |
| RPC 429、日志结果过多或超时 | 限流会做有界退避；检查 provider 配额，必要时同时调低窗口和下限，保持 `0 < window_floor <= window_blocks` |
| 检查点持续不前进 | 结合 eligible head 和 `window stored` 日志核对；检查网络、最终确认停滞、RPC 配额及磁盘空间 |
| systemd 启动过于频繁而停止重试 | 修复原因后执行 `sudo systemctl reset-failed tokenmonitor`，再启动并验收 |
| `checkpoint hash mismatch` | 停止服务，按下面流程确认规范链并人工 rewind；不要清表或重置检查点 |

遇到检查点哈希不匹配时：

1. `sudo systemctl stop tokenmonitor`，保留日志中的高度和 expected / observed 哈希，备份当前数据库。
2. 使用独立 RPC 或区块浏览器确认规范链，再确认配置的 provider 可信。哈希检查在下一窗口摄取前进行，追平后的空闲等待期不持续检查。
3. 在工作目录中用相同外部环境执行交互式恢复。下面通过临时 systemd unit 复用服务账号和环境，凭据不进入命令行：

   ```bash
   sudo systemd-run --pty --wait --collect \
     --property=User=tokenmonitor --property=Group=tokenmonitor \
     --property=WorkingDirectory=/opt/tokenmonitor \
     --property=EnvironmentFile=/etc/tokenmonitor/indexer.env \
     --property=KillSignal=SIGINT \
     /usr/local/bin/tokenmonitor-indexer -config /etc/tokenmonitor/tokens.json rewind
   ```

4. 核对输出的原检查点、规范哈希、rewind point、删除范围及删除前统计，再输入 `yes`。操作只删除 **高于** 已验证恢复点的事件并重设检查点；祖先查找最多 1000 块，验证失败应停止调查。可用 `rewind --to N` 指定需验证的恢复点；`--yes` 会跳过交互确认，日常人工恢复不要使用。
5. 成功后启动服务并按第 5 节验证重放。若报错说明删除 / 重设已经执行但后续统计失败，保留输出并核查数据库，不要假定发生了事务回滚。

实现与完整恢复语义见 [main.go](../cmd/indexer/main.go)、[sync.go](../internal/indexer/sync.go) 和 [rewind.go](../internal/indexer/rewind.go)。
