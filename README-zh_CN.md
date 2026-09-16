# Prods

[English](README.md) | [繁體中文](README-zh_TW.md) | 简体中文

**把产品数据变成正式、可搜索、可询价的官方网站，不必运维一整套应用系统。**

Prods 是为制造商、经销商与其他 B2B 产品团队打造的自托管产品目录与 RFQ（询价请求）系统。小型团队可导入并维护技术产品数据，安全发布给访客、搜索引擎与机器使用，再从单个便携式 Go 可执行文件接收真实买家需求。

运行时包含 Admin 后台、公开网站、SQLite 支持、搜索、发布、RFQ、备份、恢复与运维工具。Node.js 只在从源代码构建时需要；部署后不需要 Node.js、独立数据库服务器、Docker、队列或外部搜索服务。

## 为什么选择 Prods

- **沿用现有数据开始。** 管理产品、分类、规格、字典、文件、翻译与 Website 配置。XLSX 导入／导出及批量操作可维护大型产品目录，不必逐条开表单修改。
- **精确控制公开状态。** 产品页、结构化数据、JSON、Markdown 与路由状态一起切换。更新已发布产品时，旧的有效版本会保留到新版本完成；Hide 与 Archive 会立即停止新的公开访问。
- **同时服务工程师、采购、搜索引擎与 AI 工具。** 产品目录浏览、搜索、分页、文件及 RFQ 都有服务器端语意 HTML，没有 JavaScript 仍能使用。同一份已发布模型也供应 JSON-LD、JSON、Markdown、Sitemap、manifest 与 `llms.txt`。
- **先获取需求，不假装成电商。** 访客可以询问一个或多个产品目录产品；搜索无结果时也能主动提交 Requested Part。Prods 以可重播且耐久的方式记录 RFQ，价格、供货、资格审查与后续联系仍由企业决定。
- **运维方式清楚可掌握。** 单个进程负责 migration、写入节流、持久化任务、审计、备份、restore journal 与健康检查。Terminal 会显示当前状态、下一步、URL、Admin 路径、主机信息、资源路径与实时日志。
- **掌握部署与数据。** Prods Community 采 AGPL 许可，以 SQLite 储存主要状态，将资产与备份放在明确的本机目录，可在 Windows 或 Linux 上执行并搭配自选的反向代理。

## 适合的团队

Prods 适合需要官方产品目录与 RFQ 管道，但不想自行拼装并运维 CMS、定制数据库应用、搜索服务与独立后台的产品型企业。尤其适合元器件、工业、技术型与 B2B 产品目录，这些情境通常重视料号、分类、规格、文件、公开控制与询价脉络。

V1 刻意不包含结帐、定价、库存 Availability、CRM、通用页面编辑器、任意自定义 JavaScript，也不会由 RFQ 自动创建产品。这些边界让产品目录真实数据、公开内容与买家需求维持清楚可控。

## 主要能力

| 领域 | Prods 提供的能力 |
| --- | --- |
| 产品目录作业 | Current／Archived 产品、任意深度分类、规格与 Spec Set、字典、图片／文件、分页、排序、XLSX 导入／导出，以及批量 Publish／Hide／Archive／分类／生命周期操作 |
| Website 与多语 | 版本化 Website 配置、预览／发布流程、Public Copy 覆写、导览与主题控制、独立 Admin UI 语言，以及十种内建公开语言 |
| 公开探索 | 服务器端搜索与产品目录页、产品／分类／制造商／品牌／应用路由、Unicode folding 搜索、JSON-LD、JSON、Markdown、Sitemap、robots、manifest 与 `llms.txt` |
| RFQ | 多产品询价、搜索无结果后的 Requested Part、high-entropy idempotency key、持久化回执、Admin 查看，以及由用户明确触发的可选 SMTP 发送 |
| 权限与可追溯性 | 不透明 server-side session、CSRF 防护、能力型角色、Admin scope，以及必要管理操作记录 |
| 运维 | Liveness／readiness、runtime log、排程与单次备份、验证式 restore、Recovery、Owner 救援、Maintenance、资产清理与手动版本检查 |
| 发布 | 默认空白安装、可选且精确对应版本的 sample data、嵌入源代码 revision／版本、checksum，以及 Linux／Windows amd64 构建产物 |

内建语言为 `en-US`、`zh-TW`、`zh-CN`、`ja-JP`、`ko-KR`、`de-DE`、`fr-FR`、`it-IT`、`es-ES`、`pt-BR`。内置默认文案已通过 key、placeholder、plural、格式与版面检查，但尚未经专业母语、法律或行销审核；正式发布前请审阅所有面向公网文案。

## 快速开始

从 [GitHub Releases](https://github.com/herefindalex/prods/releases) 下载同一版本的文件：

- `prods-linux-amd64`、`prods-windows-amd64.exe`、`prods-darwin-arm64` 或 `prods-darwin-amd64`
- `SHA256SUMS` 与 `BUILD_INFO.txt`
- `LICENSE`、`LICENSE_POLICY.md`、各语言 README 与 `THIRD_PARTY_NOTICES.md`

执行前请先验证下载档。Linux：

```sh
sha256sum --check SHA256SUMS
chmod +x prods-linux-amd64
./prods-linux-amd64
```

Windows PowerShell：

```powershell
.\prods-windows-amd64.exe
```

Apple Silicon macOS：

```sh
chmod +x prods-darwin-arm64
./prods-darwin-arm64
```

Intel macOS 使用 `prods-darwin-amd64`。**目前两个 macOS 可执行文件都只有 cross-compile 结果，尚未在 macOS 实际运行、code sign 或 notarize；native runtime、Installer、Admin、backup／recovery 与 shutdown 验收完成前，macOS 版本均视为未验证。**

数据目录为空时，Terminal 会显示一次性的本机 Installer URL。打开URL、认领安装流程、创建第一位 Owner、选择站点站点语言和时区，再完成安装。Ready marker 交易完成后 Prods 会离开；重新启动同一个可执行文件，打开画面显示的 Admin URL，并以 Owner 登录。

公开产品目录从 `/` 开始；Admin 后台位于 `/admin`。

### Terminal Console

在交互式 terminal 中，Prods 使用 Bubble Tea 呈现三个视觉区域：

1. 单行 `Prods` 产品标题。
2. 状态与操作指引，包含 Installer／Recovery URL、公开 URL、Admin URL、版本、主机、监听地址与资源路径。
3. 可滚动的实时日志。

使用上／下方向键、`PgUp`／`PgDn`、`Home`、`End` 查看 log；`Ctrl+C` 会开始 graceful shutdown。URL使用 OSC 8 hyperlink，支持的 terminal 可直接点击，不支持时仍会显示可复制文字。Windows Service 与 redirected output 会改用 newline-delimited JSON，不显示交互界面。

### 空白或 sample data 安装

新安装默认没有任何产品测试数据，只有 Owner 明确选择 sample data 时才会导入。

只有可执行文件嵌入的精确版本，在相同 GitHub Release tag 同时提供下列三个资产时，sample data 选项才会启用：

- `prods-sample-data-v1.json`
- `prods-sample-data-v1.json.sha256`
- `schema-v1.json`

Prods 绝不拿 `latest` 的数据替代。系统会验证 checksum、schema version、release version 与 payload 限制，再把 Owner、最小站点配置、sample records、audit entry 与 Ready marker 放在同一个 SQLite transaction 中提交。下载内容保留于 `data/sample-data/`。

如果 GitHub 离线、repo 为 private、缺少 tag 或任一资产，sample 选项会显示不可用，但空白安装仍可正常完成。

## 便携式执行环境与配置

默认路径以可执行文件所在目录为基准，方便检查、搬移、备份或注册成服务：

| 配置 | 默认值 |
| --- | --- |
| 监听地址 | `:8080` |
| Public Base URL | `http://127.0.0.1:8080` |
| 配置档 | `prods.ini` |
| 数据目录 | `data/` |
| 备份目录 | `backups/` |
| SQLite 数据库 | `data/prods.db` |

第一次安装若默认 port 已被占用，Prods 可以选择并保存下一个可用 port。已安装或明确配置的站点会直接报错，不会在未告知的情况下改用其他地址。

配置优先顺序：

```text
CLI flag > environment variable > 单一 prods.ini > 便携式默认值
```

执行 `prods --help` 查看所有参数。常见主机配置：

```ini
[prods]
listen=127.0.0.1:8080
base_url=https://catalog.example.com
data_dir=data
backup_dir=backups
trusted_proxies=127.0.0.1/32
asset_gc_grace_days=7
```

对应环境变数包含 `PRODS_LISTEN`、`PRODS_BASE_URL`、`PRODS_DATA_DIR`、`PRODS_BACKUP_DIR`、`PRODS_CONFIG`、`PRODS_TRUSTED_PROXIES`。

面向公网服务时，请由受信任的反向代理终止 HTTPS，明确配置正式 HTTPS Base URL，只接受已知 proxy IP／CIDR 提供的 forwarded client 信息。不要用通用文件服务器公开 data、backup、secret 或 generated-history 目录。

## 发布与 RFQ 行为

Prods 将 Domain Truth、Website 配置、Public Copy 与产生后的公开 representation 分开管理。

发布产品时会创建 immutable public representation；路由、HTML、JSON-LD、JSON、Markdown 与 public revision 以同一单位启用。搜索、分类、制造商、品牌与 Sitemap aggregate 可在之后收敛，但不会暴露已撤销产品，也不会发布指向不可用内容的链接。全站 URL pattern 或 prefix 变更使用 atomic site epoch switch。

RFQ 不会创建 Product、不会虚构价格或 availability，也不会自动寄信。SMTP 是可选集成；只有许可的 Admin 用户明确操作时才发送，并逐一记录每位收件人的结果。

## 备份、恢复与 Maintenance

可在 Admin 创建与监看备份，或执行单次备份：

```sh
./prods-linux-amd64 --backup-now
```

离线 restore 会选择已验证的 backup ID 或路径，完成 verified roll-forward 后离开：

```sh
./prods-linux-amd64 --restore-backup <backup-id-or-path>
```

默认情况下，restore 正常站点前必须先完成可验证的 pre-restore backup。只有当前状态已无法备份时，主机管理者才能明确使用 `--allow-restore-without-prebackup` 例外。

数据库损坏或 prepared operation 中断时，Prods 会进入 Recovery Required，不会覆盖现有数据重新初始化。Terminal 会提供另一个有期限的 Recovery URL；Recovery 不依赖正常数据库、Admin session、Public artifacts 或 Custom CSS。

Owner 忘记密码时，可在不重开 Installer 的情况下创建一次性本机救援链接：

```sh
./prods-linux-amd64 --recover-owner owner@example.com
```

Manual Maintenance 从 Admin System Health 开始或结束。它会对新的 Public／RFQ admission 返回 HTTP 503，同时保留 Admin、Recovery、system assets 与 liveness。

## 可选集成与版本更新

SMTP credential 与 Google Search Console OAuth secret 从 private file 载入，不把 bearer value 存在 SQLite。Secret file 应位于 `data/secrets`，让 backup／restore 把它们视为必要 root。

IndexNow 与 Google Search Console submission 都是可选功能，并要求公开 HTTPS Base URL。成功 receipt 只代表 provider 接受请求，不代表页面已被收录；没有这些 provider 时，公开渲染与 machine-readable output 仍可正常工作。

Admin System Health 只在用户要求时检查 GitHub 最新 stable release。若较新的 semantic version 提供当前平台的 binary，Admin 会显示直接下载链接。网络错误、private／unavailable repo、无效 release metadata 或缺少平台资产都不会阻塞启动与健康状态。

## 架构

```text
Browser / crawler
       │
       ▼
单一 Prods Go process
  ├─ Public：semantic HTML + JSON-LD + JSON + Markdown + Sitemap/manifest/llms.txt
  ├─ Admin：嵌入 React + headless Refine + Ant Design
  ├─ System：嵌入 Installer / Recovery / Maintenance
  ├─ application services：catalog、RFQ、import/export、publication、backup/restore
  ├─ SQLite：authoritative state、migration、durable job、receipt、audit
  └─ private roots：immutable assets、secrets/config、public-state、backups
```

主要边界：

- 可执行文件是唯一必要的应用进程；Node.js 是 build dependency，不是用户 runtime dependency。
- SQLite 是主要数据库；Prods 自行管理 migration，所有写入共用 bounded admission。
- 公开输出只能来自明确的 `PublicView` allowlist，不能直接使用 Admin DTO 或 database row。
- 产品发布与撤销以 durable intent 与 generated representation 完成；没有持久化证据就不回报成功。
- RFQ idempotency 会在同一 transaction 保存 key、canonical payload hash、RFQ ID 与可重播 receipt。
- Restore 使用 durable journal；`prepared` 之前可安全放弃，之后只能用同一 operation 依各 root 证据向前完成。
- Admin 使用不透明 server-side session、server-side authorization、same-origin CSRF 与可信任 Host／proxy 配置。

## 从源代码构建与验证

所需工具：

- Go 1.27.1
- Node.js 24
- pnpm 10.28.1，由 `packageManager` 固定

```sh
pnpm --dir web/ui install --frozen-lockfile
python3 scripts/check-license-policy.py
pnpm --dir web/ui typecheck
pnpm --dir web/ui test
pnpm --dir web/ui build
go test ./... -count=1
go test -tags poc ./... -count=1 -timeout=5m
go vet ./...
CGO_ENABLED=0 go build -trimpath -o prods ./cmd/prods
```

前端构建会写入 `internal/webapp/static/` 的嵌入资产；源代码变更时应一起提交产生档。

创建含嵌入版本、对应 sample data、源代码 metadata、许可文件与 checksum 的 Linux amd64、Windows amd64、macOS Intel 与 macOS Apple Silicon release：

```sh
./scripts/build-release.sh v0.6.6
```

输出位于 `dist/<version>/`。根目录 [`VERSION`](VERSION) 是唯一版本来源；script 会拒绝不同参数，GitHub Actions 也会拒绝不同 tag。同一版本会写入每个 binary、sample payload、`BUILD_INFO.txt`、产物目录与 GitHub Release tag。Cross-build 只能证明可编译；Windows 必须在 Windows 实际运行后才能宣称通过 runtime 验证，两个 macOS 产物在 native macOS 验收前均明确列为未测试。

## GitHub Actions 与 Release

`.github/workflows/ci.yml` 会在每次 push 与 pull request 执行：安装 pinned frontend dependency、检查 dependency license policy、执行 typecheck 与前端测试、重建并检查 embedded assets、执行一般与 PoC Go suite、`go vet`、sample contract 验证，以及 Linux amd64、Windows amd64、macOS Intel 与 macOS Apple Silicon cross-build，最后上传短期 CI artifacts。

Push `v*` tag 会启动 `.github/workflows/release.yml`。Workflow 会再次验证源代码，以精确 tag 呼叫 `scripts/build-release.sh`、验证 `SHA256SUMS`、上传 workflow artifact，并将所有文件发布到相同 tag 的 GitHub Release。含连字号的 tag 会标成 prerelease，不会进入 Admin 的 latest-stable 更新路径。

Release script 要求 tracked working tree干净，并将版本与 source revision 写入 binary 与 `BUILD_INFO.txt`。它会重新生成 tracked sample payload 与 schema checksum，逐位元相同才允许发布。

## 许可与商用兼容性

Prods Community 采用 [GNU Affero General Public License v3.0 only](LICENSE)，SPDX 标识为 `AGPL-3.0-only`。通过网络使用修改版 Community 程序的用户，有权获取对应源代码。

当前 release closure 的第三方组件仅使用 MIT、BSD、Apache-2.0、ISC／0BSD 或 public-domain 类型许可，未发现 GPL、AGPL、LGPL、SSPL、BSL、Commons Clause、PolyForm、非商用或禁止衍生许可。完整范围、build-only 例外、未来闭源商业发行条件与自动检查方式请见 [LICENSE_POLICY.md](LICENSE_POLICY.md)；保留声明与许可文字请见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。

第三方许可不会自动阻止未来另行许可的 proprietary edition；主要条件是 Prods 本身所有贡献都必须具有可重新许可的权利。在正式 Contributor License Agreement、接受 CLA 的法律实体及记录流程创建前，不应合并实质外部贡献。Product data、图片、datasheet、RFQ、backup 与其他客户内容不会因使用 Prods 而自动改采 AGPL。
