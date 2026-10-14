# 运维与生产环境证据

[English](../en/operations.md) | [繁體中文](../zh-TW/operations.md) | 简体中文

[README](../../README-zh_CN.md) · [架构](architecture.md) · [工程案例研究](engineering-case-study.md)

## 生产环境状态

于 2026-09-17 对照工作目录查看（`VERSION`：`v0.6.8`）。代码仓库包含可运行应用程序、发布封装、运维控制及自动化测试，这些是实现证据。本次文档核查未验证生产 endpoint、其部署 revision、真实客户负载、生产 TLS 配置或异地主机恢复演练。这里的「生产环境」指预期由操作者管理、处理真实产品目录／RFQ 数据的安装，不是已达成的部署里程碑。

以下源代码与测试链接可供查看机制。测试存在不代表每个平台都通过；workflow 配置也不等于 CI 运行成功。目前 checkout 包含进行中的变更，本身不是干净的发布成品。

## 运行与部署边界

发布模型为平台专属 Go 可执行文件、内嵌前端资产、本机 SQLite 及受管理文件。[`main.go`](../../cmd/prods/main.go) 在正常或单次操作前取得 OS 层级数据库 ownership lock。[`service_linux.go`](../../internal/platform/service_linux.go) 产生可选 systemd unit；[`service_windows.go`](../../internal/platform/service_windows.go) 实现 Windows service 集成。Service 定义不代表已在真实主机验收 service manager。

对外部署预期由操作者提供 TLS 反向代理及明确 HTTPS Base URL。Prods 检查 request authority，支持配置 trusted proxy 地址。代码仓库没有特定生产 proxy／TLS 拓扑的运行证据。反向代理应转送至应用，不应公开 data、backup、secrets 或历史生成目录。参见 [`proxy_trust.go`](../../internal/webapp/proxy_trust.go)。

默认路径相对于可执行文件：

| 配置 | 默认值／用途 |
| --- | --- |
| Listen | `:8080`；并非只监听 loopback |
| Base URL | `http://127.0.0.1:8080` |
| Config | `prods.ini` |
| Data／DB | `data/`／`data/prods.db` |
| Backups | `backups/` |
| 受管理 data 子目录 | 资产、work、生成输出、secrets、control journals 与 logs |

优先序为 CLI flag → 环境变量 → 单一 config → 默认值。[`hostconfig/config.go`](../../internal/hostconfig/config.go) 定义 flag 与验证。请使用 README 示例与实际可执行文件的 `--help`；不要从开发者本机配置推测高级路径或凭证。

## 如何启动隔离的本机安装

使用 Linux amd64 发布可执行文件与新目录，或依 [README](../../README-zh_CN.md) 从源代码构建。以下 Linux shell 示例假设可执行文件在目前目录，且 port 18080 可用：

```sh
demo_dir="$(mktemp -d)"
cp ./prods-linux-amd64 "$demo_dir/prods"
chmod +x "$demo_dir/prods"
"$demo_dir/prods" --listen 127.0.0.1:18080 --base-url http://127.0.0.1:18080
```

1. 开启 terminal 输出的一次性 Installer URL，建立 Owner 并完成安装。空产品目录是有效状态；sample data 为可选且绑定可执行文件发布版本。
2. Prods 结束后，重复最后一个命令。在 `http://127.0.0.1:18080/admin` 登入，并于 `/` 查看公开网站。
3. 检查 `http://127.0.0.1:18080/health/live` 与 `/health/ready`。Readiness 与进程 liveness 必须分开判断。完成后以 `Ctrl+C` 停止。

若明确指定的 port 已占用，请换 port 并同步修改 listen／Base URL。启动若报告数据损坏或无法辨识，保留目录并使用 Recovery，不要当作重新安装请求。Installer 与正常启动行为由 [`main_test.go`](../../cmd/prods/main_test.go)、[`Installer 测试`](../../internal/webapp/installer_test.go) 及 [`安装测试`](../../internal/storage/sqlite/installation_test.go) 涵盖。

## 如何备份与恢复

运行中的站点使用 Admin Backup 或调度器。以下命令列单次操作前，先停止 service／前台进程，并使用相同可执行文件、config 与 data 路径。Ownership lock 防止第二个进程共用 instance。

```sh
./prods-linux-amd64 --backup-now
./prods-linux-amd64 --restore-backup <backup-id-or-path>
```

以选定备份取代 restore placeholder。Restore 会变更持久状态；第二个命令是另一项操作，不是每次备份后都必须运行。默认要求先取得目前站点的已验证 pre-restore backup。`--allow-restore-without-prebackup` 是针对无法备份状态的主机授权例外，不是默认流程。

Backup 将一致数据库快照与其引用资产、指定 root／文件及独立 manifest 配对。Manifest 分开记录内容验证与只读应用结果。Secrets 使备份具有敏感性；checksum／只读标志不代表加密、防篡改或主机损失后仍可恢复。外部 secret 需求也必须满足。

Restore 准备并验证各 root、持久记录 `prepared`，再依逐 root 证据向前完成同一操作。最终验证后正常重启，检查 readiness、Admin、公开页面与必要资产。Recovery 若报告缺失证据或 root 不符，保留 journal 与来源备份；不要删除 journal 强迫正常启动。参见 [`backup.go`](../../internal/recovery/backup.go)、[`restore.go`](../../internal/recovery/restore.go)、[`restore 故障测试`](../../internal/recovery/restore_fault_test.go) 及 [`backup／restore CLI 测试`](../../cmd/prods/backup_restore_test.go)。

## 如何恢复 Owner 访问

正常 instance 停止后，以相同配置及既有 active Owner 的地址运行：

```sh
./prods-linux-amd64 --recover-owner owner@example.com
```

命令输出有期限的一次性链接后结束。正常重启 Prods，在到期前开启链接，并保持链接私密。命令要求已完成安装的站点数据库，不会重开 Installer，也不修复无法读取的数据库。证据：[`main.go`](../../cmd/prods/main.go)、[`identity_store.go`](../../internal/storage/sqlite/identity_store.go)、[`用户生命周期测试`](../../internal/webapp/user_lifecycle_test.go)。

## 启动、升级、关闭与诊断

- **启动：** 持久状态检查区分 fresh、installing、ready、upgrade 与 recovery。未完成安装取得新 claim，成功安装要求重启。
- **Migration：** 具版本 SQL 与 migration history 随可执行文件交付。升级启动先建立／验证备份并记录进度，再允许正常运行。中断操作必须厘清或进入 Recovery，不会默默接受。
- **Shutdown：** 关闭与释放组件前先停止接收新工作并排空。HTTP shutdown 有 timeout，可能强制关闭，不保证所有故障下每个请求都完成。[`shutdown_test.go`](../../cmd/prods/shutdown_test.go) 涵盖有界行为。
- **Maintenance：** 新 Public／RFQ 准入回 503，保留允许的 Admin、Recovery、system assets 与 liveness。这是受控停机，不是高可用性。
- **诊断：** JSON runtime log 与交互 terminal 呈现状态；Admin 提供工作与 health 查看。[`runtime_log.go`](../../internal/platform/runtime_log.go) 及 [`runtime_health.go`](../../internal/platform/runtime_health.go) 实现本机机制；这里没有外部监控／值班部署证据。
- **Secrets／集成：** 正常站点从私有文件读取 SMTP 与 Google 凭证。可选 mail／indexing 操作与产品目录服务分开；provider 回执不证明已索引或 exactly-once 邮件投递。参见 [`maildelivery`](../../internal/maildelivery/manager.go) 与 [`searchnotify`](../../internal/searchnotify/searchnotify.go)。

## 构建、发布与验证范围

[`ci.yml`](../../.github/workflows/ci.yml) 定义前端 typecheck／test、内嵌资产差异检查、Go normal／PoC test、vet、license／sample 检查及 cross-build。[`release.yml`](../../.github/workflows/release.yml) 验证 `v*` tag 并发布文件，不部署运行中的站点。[`build-release.sh`](../../scripts/build-release.sh) 检查 `VERSION`、Go 版本、source revision、干净工作目录与精确 sample 成品，再输出可执行文件、构建信息与 checksum。

构建目标为 Linux amd64、Windows amd64 及 macOS amd64／arm64。Windows／macOS 原生 runtime、service、签名及平台专属恢复，需针对同一成品另备证据，不能靠 Linux 测试结案。POC-01 renderer feasibility 与 POC-03 publication protocol correctness 是不同门槛；呈现 smoke test 不能证明发布正确性或生产环境就绪。

完整源代码验证命令保留在 [README](../../README-zh_CN.md)。前端构建会重写内嵌文件，release packaging 要求干净 checkout。请在适当 checkout 运行，不要只为验证文件而覆盖其他进行中工作。

## 目前运维边界

| 代码仓库证据 | 宣称边界 |
| --- | --- |
| Instance lock 与单进程运行 | 不保证多节点协调、failover 或零停机升级。 |
| 序列化 SQLite 写入与 import 测试 | 没有生产容量实测，也不保证 atomic import 期间 RFQ 零等待。 |
| 发布准入与协议测试 | 测试涵盖指定情境；已准入传输可在撤销后完成。 |
| 回执与审计记录 | 是持久应用证据，不是外部服务 exactly-once 保证。 |
| Backup manifest 与 restore journal | 没有经验证的异地主机备份政策、RTO／RPO 或所有文件系统的断电保证。 |
| Session、CSRF、capability 与文件验证 | 是已实现控制，不是安全认证或全面安全保证。 |
| 十种内建语言资源 | 不宣称已经专业母语、法律或行销审校。 |
| 构建／发布 workflow 与 sample 产品目录 | 不从 fixture 或自动化推论客户数、商业成效、生产流量、SLA 或实际部署。 |

## 生产环境叙事仍需的证据

操作者可补充部署版本／source revision、主机／平台、service／proxy 配置、TLS 验证、持久 root 对照、备份保留／异地主机位置、有日期的恢复演练及测量负载／事件记录。凭证与客户数据保持私密。在证据齐备前，描述实现与测试，不使用尚未验证的生产环境成功叙事。

## 数值规格搜索（后端）

本节描述 2026-10-14 核查的 `v0.7.0` 后端。前述章节与架构／工程案例指南保留历史 `v0.6.8` 核查范围。数值 profile 编辑、共享来源／采用控件及 Public 数值筛选 UI 延后提供；现有文本搜索独立运行。

### 配置候选后端

默认为 `bleve`。Flag `--spec-search-backend`、环境变量 `PRODS_SPEC_SEARCH_BACKEND` 与 INI key `[prods] spec_search_backend` 接受 `bleve` 或 `sqlite`，优先序为 CLI > ENV > INI > 默认值。选择下列其中一种配置，使用相同站点路径重启：

```ini
[prods]
spec_search_backend=bleve
```

```sh
PRODS_SPEC_SEARCH_BACKEND=sqlite ./prods-linux-amd64
./prods-linux-amd64 --spec-search-backend sqlite
```

Bleve 内嵌于 Go 进程，在 `<data_dir>/generated/search/specs` 写入私有、可重建候选索引。SQLite 仍为主存储；`sqlite` 后端改用其派生 Spec-ID 投影。不要通过 filesystem server 公开 generated/search 目录。公开激活状态变更后，由 publication worker 替换／重建候选。

候选失败、不完整或与已准入 inventory／revision 不符时，查询改为完整扫描当前公开 inventory 并精确判定。这保留结果与计数，但增加扫描成本，不保证延迟。Log 会记录候选索引失败。已打开 adapter 的 refresh 会在后续 worker tick 重试；初次打开失败的 Bleve adapter 不会由 tick 重新打开。修正 filesystem／ownership／resource 原因后重启，或以 `sqlite` 重启。`SpecSearchDiagnostics` 与 `RebuildSpecSearchIndex` 是内部方法，没有 Admin／Public 诊断或重建 endpoint。正式公开索引本身不健康时仍返回 503。

### 准备与采用数值事实

每个 Product + Spec 选择一份共享 Raw 来源。`source_origin_locale` 记录来源 provenance，不为每个公开语言创建另一份数值事实；本地化展示文字独立保存。Migration 19 保留旧 Raw／history，只为 Product + Spec 恰有一行的情况选择来源；多来源旧分组保持未选择，直到明确命令处理。它不从旧 display unit／history 创建数值 profile 或采用事实。Published Current Product 获得新 source revision 与 publication intent；替换完成前保留旧的有效公开 activation。内置合成样本也只提供 Raw 规格，不包含已配置 profile／已采用数值事实。

使用合法同源 Admin 登录与私有 cookie jar（`admin_cookie_jar`），从已登录 Admin 页面的 `csrf-token` meta 值取得 `admin_csrf`。`base` 设为指定 Base URL，ID 变量设为现有 domain ID。Admin 读取需要 `catalog.view`；写入需要 `catalog.edit`、session cookie 及 `X-CSRF-Token`。这是 browser Admin 命令，不是外部 write token。以下 body 是示意文件，请填入操作打开时观察到的 ID／revision。每个 mutation 示例是独立操作，不是可盲目依次执行的 script。

| 方法／路径 | 权限／body |
| --- | --- |
| `PUT /admin/api/specs/{id}/numeric-profile` | `catalog.edit`; `expected_revision`, `profile` |
| `GET /admin/api/products/{id}/spec-values` | `catalog.view`; 查看已保存 Raw 来源 |
| `POST /admin/api/products/{id}/spec-source` | `catalog.edit`; `spec_id`, `spec_value_id`, `expected_product_revision`, `expected_source_revision`, `expected_source_selection_revision` |
| `GET /admin/api/spec-values/{id}/normalization-context` | `catalog.view`; 冻结 Raw、profile、`expected`、`selection`、`source_selection` |
| `POST /admin/api/spec-values/{id}/normalize` | `catalog.edit`; `expected` |
| `POST /admin/api/spec-values/{id}/normalized` | `catalog.edit`; `expected`, `value_json` (以字符串编码的 JSON) |
| `POST /admin/api/spec-values/{id}/normalization/automatic` | `catalog.edit`; `expected`, `candidate_id` |

Operating-voltage Spec 的 `profile-command.json` 示例（`expected_revision` 使用 Spec definition 的实际 `revision`）：

```json
{
  "expected_revision": 1,
  "profile": {
    "quantity": "voltage",
    "semantic_role": "operating_support",
    "allowed_shapes": ["scalar", "discrete_set", "interval_set"],
    "allowed_operators": ["contains_point", "contains_range", "overlaps_range"],
    "canonical_unit": "V",
    "declared_input_unit": "V",
    "numeric_format": "decimal-dot",
    "schema_version": 1,
    "parser_version": "electrical-parser-1",
    "unit_registry_version": "electrical-1"
  }
}
```

```sh
curl -b "$admin_cookie_jar" -H "X-CSRF-Token: $admin_csrf" \
  -H 'Content-Type: application/json' -X PUT   "$base/admin/api/specs/$spec_id/numeric-profile" --data-binary @profile-command.json
curl -b "$admin_cookie_jar" \
  "$base/admin/api/products/$product_id/spec-values"
curl -b "$admin_cookie_jar" \
  "$base/admin/api/spec-values/$spec_value_id/normalization-context"
```

若 context 尚未选定共享来源，先用 `source-command.json` 选择。三个 expectation 分别采用 context 的 `expected.product_revision`、`expected.source_revision` 与 `source_selection.revision`；成功后重新打开 normalization context：

```json
{
  "spec_id": "spec_input_voltage",
  "spec_value_id": "spv_selected_source",
  "expected_product_revision": 7,
  "expected_source_revision": 1,
  "expected_source_selection_revision": 0
}
```

```sh
curl -b "$admin_cookie_jar" -H "X-CSRF-Token: $admin_csrf" \
  -H 'Content-Type: application/json'   "$base/admin/api/products/$product_id/spec-source" --data-binary @source-command.json
```

将下列保存为 `normalize-command.json`，用 `normalization-context` 返回的完整 `expected` 对象替换五个值：`product_revision`、`source_revision`、`spec_semantic_version`、`profile_revision`、`selection_revision`。前四个须为正数，selection revision 可为零。写入事务重新验证所有 expectation 及选定共享来源。

```json
{
  "expected": {
    "product_revision": 7,
    "source_revision": 1,
    "spec_semantic_version": 2,
    "profile_revision": 1,
    "selection_revision": 0
  }
}
```

```sh
curl -b "$admin_cookie_jar" -H "X-CSRF-Token: $admin_csrf" \
  -H 'Content-Type: application/json'   "$base/admin/api/spec-values/$spec_value_id/normalize" --data-binary @normalize-command.json
```

规范化可能采用有效 automatic 结果、在 manual preference 有效时记录 candidate，或对无法解析 Raw 返回 HTTP 200、`state: "failed"` 与 `reason_code`；不覆盖手动采用的 selection。Manual adoption 以 `value_json` 字符串提供经过验证、canonical-unit 的 `Value`。上述 voltage profile 可用以下 `manual-command.json`：

```json
{
  "expected": {"product_revision": 7, "source_revision": 1, "spec_semantic_version": 2, "profile_revision": 1, "selection_revision": 0},
  "value_json": "{\"schema_version\":1,\"quantity\":\"voltage\",\"semantic_role\":\"operating_support\",\"shape\":\"scalar\",\"unit\":\"V\",\"unit_registry_version\":\"electrical-1\",\"scalar\":\"5\",\"context\":{\"kind\":\"unconditional\"}}"
}
```

```sh
curl -b "$admin_cookie_jar" -H "X-CSRF-Token: $admin_csrf" \
  -H 'Content-Type: application/json'   "$base/admin/api/spec-values/$spec_value_id/normalized" --data-binary @manual-command.json
```

若要将 manual preference 改为当前 automatic candidate，明确选择服务器已记录的 ID，并将重新查看的 context 填入 `automatic-command.json`；任意、过期或不兼容 candidate 会被拒绝：

```json
{
  "expected": {"product_revision": 8, "source_revision": 1, "spec_semantic_version": 2, "profile_revision": 1, "selection_revision": 1},
  "candidate_id": "nrm_current_automatic_candidate"
}
```

```sh
curl -b "$admin_cookie_jar" -H "X-CSRF-Token: $admin_csrf" \
  -H 'Content-Type: application/json'   "$base/admin/api/spec-values/$spec_value_id/normalization/automatic" --data-binary @automatic-command.json
```

采用成功会增加 Product／selection revision，可能返回 `publication_pending: true`。已保存或已排程不代表公开激活。不要自动重试写入；409 时保留编辑，读取新 context 并查看变更版本后再发出新命令。

### 查询冻结公开事实

`POST /api/search/specs` 是有界读取命令，不要求 Admin session 或 Admin CSRF token。将示意 Spec／category ID 换成已公开、profile 允许该 operator／unit 的 ID。Output-current 示例另需允许 `>=` 的 current profile：

```sh
curl -H 'Content-Type: application/json' "$base/api/search/specs" --data-binary '{
  "predicates": [
    {"spec_id":"spec_input_voltage","operator":"contains_point","value":"5","unit":"V"},
    {"spec_id":"spec_output_current","operator":">=","value":"0.5","unit":"A"}
  ],
  "page":1,"page_size":20
}'
curl -H 'Content-Type: application/json' "$base/api/search/specs" --data-binary '{
  "predicates":[{"spec_id":"spec_input_voltage","operator":"contains_range","lower":"3","upper":"5","unit":"V"}],
  "category_id":"cat_power","locale":"en-US","page":1,"page_size":20
}'
```

请求需有 1–16 个 predicate、正数 `page` 及 1–100 的 `page_size`。Decimal operand 是 JSON 字符串，必须明确指定 unit。Point operator（`contains_point`、`=`、`<`、`<=`、`>`、`>=`）只用 `value`；range operator（`contains_range`、`overlaps_range`、`between`）只用 `lower`／`upper`。不要传不相关 operand key，即使值为 `null`。选填 `category_id` 包含该分类及后代；`manufacturer_id` 按精确 ID 筛选。`locale` 必须已公开启用，省略时解析 active Site Default。

所有 predicate 须对已准入 active `PublicView` 中采用的事实／冻结 profile 判定为 `MATCH`。缺失、过期、有条件或不兼容事实为 `UNKNOWN`，不纳入结果。分离 operating range 保留间隙；nominal／tolerance 与单向 limit claim 遵循声明 role，不推断为 operating range。候选索引不决定数值真相。在同一准入 snapshot 完成精确判定、可见性／scope 检查后才计算 `total` 与分页。结果为 `{rows,total,page,page_size}`，按 Part Number 再 stable Product ID 排序；超出范围的页面返回空 rows，保留精确 total。待发布 source／profile 编辑影响未来 publication，不改当前 active 冻结事实。

### 解读 API 结果

Admin：400 `invalid_json` 包含 body／expectation 格式错误及不合法 controlled value JSON；401 `unauthorized` 表示无有效 session；403 `forbidden` 或 `invalid_csrf` 表示权限／安全拒绝；404 `not_found` 包含缺少 source／profile context；409 `revision_conflict` 包含过期 expectation 或未选定共享来源；422 `validation_failed` 表示 domain／profile 不兼容。Manual unit／quantity／role／allowed-shape 不兼容返回 422，tagged Value 格式错误返回 400。Raw 解析失败是上述成功 200 outcome，不是传输错误。

Public search：JSON／operand shape、decimal encoding 或页面范围错误返回 400 `invalid_json`；明确指定 disabled／unknown locale 返回 400 `validation_failed`（准入 locale-policy 竞争可能返回 400 `invalid_json`）。未知 operator 或与 scope 内冻结 definition 不兼容的 operator／unit 返回 422 `validation_failed`。Scope 内无 definition／fact 时不成立 match，不虚构 profile 语义。正式 publication 不可用返回 503 `service_unavailable`；非预期 server error 返回 500 `internal_error`。Maintenance 会拦截路由，返回 HTTP 503 HTML 与 `Retry-After: 300`；draining 可能在 API 处理前返回纯文本 503。候选索引降级本身触发精确 fallback，不返回近似答案。

证据：[`spec_numeric.go`](../../internal/webapp/spec_numeric.go)、[`spec_search.go`](../../internal/webapp/spec_search.go)、[数值 profile](../../internal/specnumeric/profile.go)、[数值 value](../../internal/specnumeric/value.go)、[精确判定](../../internal/specnumeric/predicate.go)、[搜索引擎](../../internal/publishing/spec_search.go)、[后端生命周期](../../internal/publishing/spec_search_backend.go) 与 [migration 19](../../internal/storage/sqlite/migrations.go)。这些链接说明实现；生产规模与 Windows／macOS 原生验收仍需独立证据。
