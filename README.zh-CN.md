[English](README.md) | **简体中文**

# grok-switch

<p align="center">
  <strong>Grok Build 中间站切换工具</strong><br>
  单二进制 · 零依赖 · 自动更新 · CLI + 全屏 TUI · Linux amd64 / arm64
</p>

<p align="center">
  <a href="https://github.com/Gelmezon/grok-switch/releases/latest"><img src="https://img.shields.io/github/v/release/Gelmezon/grok-switch?style=flat-square" alt="release"></a>
  <a href="https://github.com/Gelmezon/grok-switch/releases"><img src="https://img.shields.io/github/downloads/Gelmezon/grok-switch/total?style=flat-square" alt="downloads"></a>
  <a href="./LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue?style=flat-square" alt="license"></a>
</p>

---

## 我为什么写 grok-switch

我最初写 `grok-switch`，是因为自己需要在 Linux 服务器上频繁切换 Grok Build 的官方认证和不同中间站。表面看，这件事似乎只需要修改一个 API 地址和一个 Key；真正使用后才会发现，问题远不止于此。

Grok Build 的配置同时包含默认模型、Web Search 模型、子代理模型和每个模型自己的认证与路由字段。尤其在 Grok Build `0.2.106` 中，仅修改旧的 `[endpoints].api_url` 并不足以让中间站真正生效：每个 `[model.'模型名']` 还需要正确的 `base_url` 和 `api_backend`。这也是为什么中间站的 `/v1/models` 和 `/v1/chat/completions` 明明测试正常，Grok 里却仍然可能出现 `Authentication required`。

另一个现实问题是，中间站提供的模型并不是固定的。模型可能增加、改名或下线。如果继续维护一份手写模型列表，配置很快就会与服务端不一致；如果直接反复编辑 `~/.grok/config.toml`，又容易覆盖原有设置、泄露 API Key，或者在写入中断后留下无法解析的文件。

因此，我把这套容易出错的手工操作做成了 `grok-switch`：让“保存一个中间站、同步它当前提供的模型、安全切换配置、出错时恢复”成为一次可重复、可检查的操作，而不是每次都重新排查 TOML。

## 它解决什么问题

| 实际问题 | grok-switch 的处理方式 |
|---|---|
| 修改了中间站地址，但 Grok 仍要求 `/login` | 为每个模型生成 Grok 当前版本需要的 `base_url`、`api_backend = "chat_completions"` 和认证字段，而不只写旧版全局端点。 |
| 中间站模型经常变化 | 添加、编辑和切换 Profile 时自动请求 `/v1/models`，去重并生成全部模型配置；已有默认模型仍可用时会继续保留。 |
| 多个中间站的 URL、Key 和模型容易混淆 | 每个供应商保存为独立 Profile，可以按名称查看、测试和切换。 |
| “能访问 API”不等于“Grok 能对话” | `test` 分别检查认证、Models 端点和 Chat Completions，避免只凭一次 HTTP 成功误判。 |
| 手工修改 TOML 容易损坏或覆盖其他设置 | 只管理与中间站有关的字段，保留未知配置；使用原子写入、写后解析校验和切换前备份。 |
| 想临时恢复 Grok 官方认证 | `official` 会移除由本工具管理的中间站字段，同时保留其他 Grok 设置。 |
| 服务器没有桌面环境 | 提供一个 Linux 单二进制，同时支持全屏 TUI、普通 CLI、管道和 CI。 |
| API Key 可能出现在 history 或 `ps` | 不提供 `--api-key` 参数；支持隐藏输入、标准输入和环境变量，配置文件使用严格权限。 |

## 一次切换实际做了什么

当你执行 `grok-switch use relay-a` 时，工具会：

1. 读取该 Profile 的 Base URL 和 API Key。
2. 请求中间站的 `/v1/models`，同步当前真实可用的模型列表。
3. 保留仍然有效的默认模型；否则按优先级选择一个可用模型。
4. 为每个模型生成独立的 Grok 路由、认证和推理配置。
5. 备份现有 `~/.grok/config.toml`，再通过原子写入更新受管字段并重新解析校验。
6. 将 Profile 标记为当前配置；之后 Grok Build 直接请求你配置的中间站。

`grok-switch` **不是中间站，也不是网络代理**。它不会转发、保存或查看 Grok 的日常对话流量；它只在本机管理配置，并在添加、编辑、切换或测试时主动访问中间站。它也不能让失效的 API Key 或不兼容的接口变得可用：当前面向的是兼容 OpenAI Chat Completions、提供 `/v1/models` 与 `/v1/chat/completions` 的服务。

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

安装脚本会同时下载 `SHA256SUMS` 并在写入目标目录前完成校验。

如需无需 `sudo` 的自动更新，建议安装到用户目录并确认该目录已加入 `PATH`：

```bash
INSTALL_DIR="$HOME/.local/bin" bash scripts/install.sh
```

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

## 日常使用概览

日常使用时，可以通过下面这些命令管理多个 **Grok Build 中间站 Profile**，并在它们与官方认证之间安全切换 `~/.grok/config.toml`。

- 无桌面 / Web / OAuth 依赖，**一个静态二进制**
- TTY 下全屏 TUI；管道 / CI 自动纯文本
- 切换前自动备份，失败可回滚

```bash
grok-switch add relay-a      # 添加中间站供应商
grok-switch list             # 列出供应商（含默认官方）
grok-switch use relay-a      # 切换供应商
grok-switch test relay-a     # 测试模型连通性
grok-switch status           # 查看状态
grok-switch official         # 切回官方（默认配置）
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
| `U` | 检查更新并在 TUI 内安装 |
| `/` | 搜索 |
| `Tab` | 切换面板焦点 |
| `?` | 帮助 |
| `q` | 退出 |

**注意**：添加、编辑或切换供应商时，grok-switch 会使用 API Key 请求中间站的 `/v1/models`，自动生成全部模型配置并选择默认模型。当前默认模型仍可用时会保持不变。

### CLI 常用命令

```bash
# 交互添加
grok-switch add relay-a

# 非交互添加（CI）
GROK_SWITCH_API_KEY='sk-xxx' grok-switch add relay-a \
  --base-url https://relay.example.com/v1

# 切换 / 状态 / 官方
grok-switch use relay-a
grok-switch status
grok-switch official

# 备份
grok-switch backup list
grok-switch backup restore <文件名>
grok-switch backup prune --keep 10

# 程序更新
grok-switch update check
grok-switch update
grok-switch update rollback
```

> 不支持 `--api-key` 参数，避免 Key 进入 shell history / `ps`。

---

## 命令一览

| 命令 | 说明 |
|---|---|
| `grok-switch` / `tui` | 全屏 TUI |
| `add` / `edit` / `delete` / `show` / `list` | Profile 管理 |
| `use` / `status` / `official` | 切换与状态 |
| `test <name-or-id>` | 分别测试认证、Models 端点和 Chat Completions |
| `import-current <name>` | 从当前 config.toml 导入 |
| `backup list\|restore\|prune` | 备份 |
| `update check` / `update` / `update rollback` | 检查、安装和回退程序版本 |
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
| `GROK_SWITCH_UPDATE_MODE` | 更新模式：`notify` / `auto` / `off` | `notify` |
| `GROK_SWITCH_NO_UPDATE_CHECK` | 禁止 TUI 启动时检查更新 | — |
| `NO_COLOR` | 禁用颜色 | — |
| `GROK_SWITCH_NO_TUI` | 禁用全屏 TUI | — |

---

## 安全要点

- API Key **绝不**入参数/日志/history（推荐用 `GROK_SWITCH_API_KEY` 或交互提示）。
- 目录 `0700` / 文件 `0600` / 入口 `umask(0077)`。
- 原子写入 + 自动备份 + 回滚。
- `status` 显示 `official` / `managed-relay` / `unmanaged`。
- `test` 分别验证认证、Models、Chat Completions。

## 自动更新

TUI 启动异步检查最新 Release（缓存 24h）。按 `U` 强制检查 → 直接弹出确认更新。

```bash
grok-switch update          # 检查 + 更新（交互）
grok-switch update --yes   # 非交互更新
grok-switch update rollback # 回退上一版本
```

`grok-switch.previous` 保留。自动模式：`GROK_SWITCH_UPDATE_MODE=auto`。

## 退出码

| 码 | 含义 |
|---:|---|
| 0 | 成功 |
| 1 | 运行错误 |
| 2 | 用法错误 |
| 3 | `status`：未托管/未知或 Profile 不匹配 |
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

## License

MIT

**作者：** DavidZhao · **仓库：** [Gelmezon/grok-switch](https://github.com/Gelmezon/grok-switch)
