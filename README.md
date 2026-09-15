# Arc Mainnet Monitor

Arc 主网上线期的轻量级链上监控器。使用 GitHub Actions 每 5 分钟运行一次，无需服务器。

## MVP 当前监控

- 读取 Arc EVM RPC 最新区块
- 扫描最近 `LOOKBACK_BLOCKS` 个区块中的新合约部署
- 对新合约调用 ERC-20 `name()` / `symbol()` 做初步识别
- Telegram 推送合约地址、部署者、交易哈希、区块高度
- 每次最多 `MAX_ALERTS` 条，防止刷屏
- 支持手动 Telegram 测试

> 当前版本是 Launch Radar MVP，不会把“新合约”当作买入信号，也还没有 DEX 成交量、LP、Holder、Smart Money 等指标。后续可在主网上线后根据实际 DEX/Explorer API 增加。

## GitHub Secrets

进入仓库 `Settings → Secrets and variables → Actions → New repository secret`，添加：

- `ARC_RPC_URL`：Arc 主网 JSON-RPC HTTPS 地址。请使用 Arc 官方/可信 RPC，不要使用来路不明的节点。
- `TELEGRAM_BOT_TOKEN`：BotFather 创建 Bot 后获得的 token。
- `TELEGRAM_CHAT_ID`：接收提醒的 Telegram chat id。

不要把 token 写进代码、README 或 issue。

## 测试 Telegram

进入 `Actions → Arc Mainnet Monitor → Run workflow`，勾选 `Send Telegram test only` 后运行。收到 `Arc Mainnet Monitor 测试成功` 即表示推送链路正常。

## 自动运行

合并到默认分支 `main` 后，GitHub Actions 使用：

```yaml
cron: "*/5 * * * *"
```

GitHub schedule 最小间隔为 5 分钟，但不保证精确准点，高峰期可能延迟。

## 本地运行

```bash
export ARC_RPC_URL='https://...'
export TELEGRAM_BOT_TOKEN='...'
export TELEGRAM_CHAT_ID='...'
go run .
```

## 风险说明

新部署合约可能是仿盘、钓鱼、honeypot、恶意授权或 rug pull。本项目只做链上信息提醒，不执行自动交易，也不构成投资建议。
