# grok-switch

<p align="center">
  <strong>Grok Build 中间站切换工具</strong><br>
  单二进制 · 零依赖 · CLI + 全屏 TUI · Linux amd64 / arm64
</p>

<p align="center">
  <a href="https://github.com/Gelmezon/grok-switch/releases/latest"><img src="https://img.shields.io/github/v/release/Gelmezon/grok-switch?style=flat-square" alt="release"></a>
  <a href="https://github.com/Gelmezon/grok-switch/releases"><img src="https://img.shields.io/github/downloads/Gelmezon/grok-switch/total?style=flat-square" alt="downloads"></a>
  <a href="./LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue?style=flat-square" alt="license"></a>
</p>

---

## 一键安装（推荐）

在 **Linux** 上执行下面这一行即可：自动识别架构、从最新 Release 下载二进制并安装到 `/usr/local/bin`。

```bash
curl -fsSL https://raw.githubusercontent.com/Gelmezon/grok-switch/main/scripts/install.sh | sudo bash
```

装好后验证：

```bash
grok-switch version
grok-switch          # 启动全屏 TUI
```

| 可选参数 | 说明 | 示例 |
|---|---|---|
| `VERSION` | 指定版本（默认 `latest`） | `VERSION=v0.1.0 sudo -E bash -c 'curl -fsSL ... \| bash'` |
| `INSTALL_DIR` | 安装目录（默认 `/usr/local/bin`） | `INSTALL_DIR=$HOME/.local/bin bash scripts/install.sh` |
| `GROK_SWITCH_REPO` | 覆盖仓库 | 默认 `Gelmezon/grok-switch` |

脚本源码：[`scripts/install.sh`](scripts/install.sh)

---

## 从 Release 手动下载

最新包：[Releases](https://github.com/Gelmezon/grok-switch/releases/latest)

| 架构 | 文件 |
|---|---|
| Linux x86_64 | [`grok-switch-linux-amd64`](https://github.com/Gelmezon/grok-switch/releases/latest/download/grok-switch-linux-amd64) |
| Linux arm64 | [`grok-switch-linux-arm64`](https://github.com/Gelmezon/grok-switch/releases/latest/download/grok-switch-linux-arm64) |
| 校验 | [`SHA256SUMS`](https://github.com/Gelmezon/grok-switch/releases/latest/download/SHA256SUMS) |

```bash
# amd64 示例
wget https://github.com/Gelmezon/grok-switch/releases/latest/download/grok-switch-linux-amd64
chmod +x grok-switch-linux-amd64
sudo mv grok-switch-linux-amd64 /usr/local/bin/grok-switch

# 可选校验
wget https://github.com/Gelmezon/grok-switch/releases/latest/download/SHA256SUMS
sha256sum -c SHA256SUMS --ignore-missing
```

---

## 这是什么？

`grok-switch` 管理多个 **Grok Build 中间站 Profile**，在它们与官方认证之间安全切换 `~/.grok/config.toml`。

- 无桌面 / Web / OAuth 依赖，**一个静态二进制**
- TTY 下全屏 TUI；管道 / CI 自动纯文本
- 切换前自动备份，失败可回滚

```bash
grok-switch add relay-a      # 添加中间站
grok-switch list             # 列出 Profile
grok-switch use relay-a      # 切换
grok-switch status           # 查看状态
grok-switch official         # 切回官方
grok-switch backup list      # 备份
```

---

## 快速开始

### 全屏 TUI（推荐）

```bash
grok-switch
# 或
grok-switch tui
```

| 按键 | 操作 |
|---|---|
| `↑↓` / `jk` | 选择 Profile |
| `Enter` | 切换到选中项 |
| `a` / `e` / `d` | 添加 / 编辑 / 删除 |
| `o` | 切回官方认证 |
| `b` | 备份管理 |
| `s` | 当前状态 |
| `/` | 搜索 |
| `Tab` | 切换面板焦点 |
| `?` | 帮助 |
| `q` | 退出 |

### CLI 常用命令

```bash
# 交互添加
grok-switch add relay-a

# 非交互添加（CI）
GROK_SWITCH_API_KEY='sk-xxx' grok-switch add relay-a \
  --base-url https://relay.example.com/v1 \
  --model grok-4

# 切换 / 状态 / 官方
grok-switch use relay-a
grok-switch status
grok-switch official

# 备份
grok-switch backup list
grok-switch backup restore <文件名>
grok-switch backup prune --keep 10
```

> 不支持 `--api-key` 参数，避免 Key 进入 shell history / `ps`。

---

## 命令一览

| 命令 | 说明 |
|---|---|
| `grok-switch` / `tui` | 全屏 TUI |
| `add` / `edit` / `delete` / `show` / `list` | Profile 管理 |
| `use` / `status` / `official` | 切换与状态 |
| `import-current <name>` | 从当前 config.toml 导入 |
| `backup list\|restore\|prune` | 备份 |
| `version` / `help` / `completion` | 杂项 |

**全局标志：** `--no-interactive`、`--json`（list 等）

---

## 环境变量

| 变量 | 说明 | 默认 |
|---|---|---|
| `GROK_HOME` | Grok 配置目录 | `~/.grok` |
| `GROK_CONFIG` | 配置文件路径 | `$GROK_HOME/config.toml` |
| `GROK_SWITCH_HOME` | 本工具数据目录 | `~/.grok_switch` |
| `GROK_SWITCH_API_KEY` | 非交互 API Key | — |
| `NO_COLOR` | 禁用颜色 | — |
| `GROK_SWITCH_NO_TUI` | 禁用全屏 TUI | — |

---

## 安全要点

- API Key 不入参数、日志、history
- 数据目录 `0700`，敏感文件 `0600`，入口 `umask(0077)`
- 原子写入 + 切换前备份 + `flock` 并发保护

## 退出码

| 码 | 含义 |
|---:|---|
| 0 | 成功 |
| 1 | 运行错误 |
| 2 | 用法错误 |
| 3 | status：磁盘与 Profile 不一致 |
| 4 | Profile 不存在 |
| 5 | 锁超时 |
| 6 | 配置解析失败 |
| 7 | 备份/恢复失败 |

---

## 从源码构建

```bash
git clone https://github.com/Gelmezon/grok-switch.git
cd grok-switch
make build          # dist/grok-switch
make release        # dist/grok-switch-linux-amd64|arm64 + SHA256SUMS
make test && make vet
sudo make install   # → /usr/local/bin/grok-switch
```

**要求：** Go 1.24+

推送 tag `v*` 会触发 GitHub Actions，自动构建并发布 Release（含上述三个文件）。

---

## 文档

- [project.md](./project.md) — 实施与架构
- [project-ui.md](./project-ui.md) — 终端 UI 设计
- [LINUX_SWITCHER_DEVELOPMENT.md](./LINUX_SWITCHER_DEVELOPMENT.md) — 开发规范

---

## License

MIT

**作者：** DavidZhao · **仓库：** [Gelmezon/grok-switch](https://github.com/Gelmezon/grok-switch)
