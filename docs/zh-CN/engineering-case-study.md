# Prods 工程案例研究

[English](../en/engineering-case-study.md) | [繁體中文](../zh-TW/engineering-case-study.md) | 简体中文

[README](../../README-zh_CN.md) · [架构](architecture.md) · [运维与证据限制](operations.md)

## 情境与证据

技术型 B2B 产品目录必须保留一般名称／描述／图片清单无法表达的差异。半导体的电压范围、连接器的接点数与模块的界面是不同规格；原始值可能带有条件或容差。制造商身份、型号、生命周期与 datasheet 有助于辨识产品，但不代表即时库存或可作为替代料。

Prods 将管理员维护的产品目录连接到公开探索与 RFQ。相同询价流程接受产品目录产品及访客输入的 Requested Part，因此产品目录不完整时也不必捏造产品记录。实现包含分类／规格模型、公开发布协议、Admin 应用程序及安装／恢复路径；这些不构成客户采用或生产部署正在运行的证据。

本文于 2026-09-17 对照工作目录核查（`VERSION`：`v0.6.8`）。以下链接指向公开源代码与测试。内部需求及历史 ADR 留在本机；本文不重新公开其内容，也不事后补造 ADR。替代方案是架构比较，不表示每个方案都曾做过原型。演进触发条件是本指南提出的评估准则。

## 真正影响实现的需求

| 需求 | 可观察的实现结果 |
| --- | --- |
| 小型团队可以自托管运维产品目录 | 单一可执行文件内嵌 UI，运行以 SQLite 为基础的应用服务。 |
| 买家与爬虫无 JavaScript 也能读取核心内容 | Go 产生可用 HTML；React 增强部分控制。 |
| 已存储数据与公开可见性必须可区分 | 发布意图与启用 revision 分开编辑与公开启用。 |
| 重复 RFQ 提交需要可核查的结果 | Canonical payload hash 与持久回执和 RFQ 共用事务。 |
| 技术数据因分类而异，也可能不完整 | Spec Set、原始值与衍生规范化记录分开。 |
| 损坏状态不能被当成新站点 | 启动检查、Installer 与 journaled Recovery 有明确边界。 |

本文没有把流量、营收、客户数、高可用性或分散式服务需求描述成已确认需求或实测结果。

## 1. 单进程与明确模块

**决策。** 在一个 Go 可执行文件内运行 HTTP、应用服务与有界后台任务。

**情境。** 自托管产品目录已经需要持久数据、资产文件与恢复程序；再要求独立 API、renderer、queue 与 search 部署，会增加运维依赖性。

**替代方案。** 组件需要独立扩展或故障隔离时，可比较独立服务与外部队列；目前没有证据确立这种生产环境需求。

**选择理由。** [`main.go`](../../cmd/prods/main.go) 组装服务，[`publishing.Repository`](../../internal/publishing/protocol.go) 与 [`backup Snapshotter`](../../internal/recovery/backup.go) 明确表达持久化依赖性。SQL 工作记录在重启后保留意图，不需 queue server。

**取舍。** 封装与本机诊断较简单，但 CPU、内存、进程故障及维护仍共用。软件包边界有助推理与测试，不表示未来拆服务没有成本，尤其涉及事务与可见性时。

**失败／演进触发条件。** 代表性测量显示 worker 无法在资源限制内与交互流量共存，或部署确实需要组件独立可用。先建立证据，再引入网络边界。

## 2. SQLite 与受控写入

**决策。** 使用 SQLite 作为主要数据库，以 SQL-first repository 与序列化写入准入协调操作。

**情境。** 产品目录读取、短 Admin／RFQ 写入及偶尔的大型导入成本不同。Atomic Import 与持久询价保存需要明确的资源竞争契约。

**替代方案。** 数据库服务器可提供不同的并发与运维安排，也增加部署、访问及备份责任。拆分导入事务可缩短个别事务，却会改变全成全败契约。

**选择理由。** [`beginWrite`](../../internal/storage/sqlite/store.go) 提供单进程共用写入准入点。[`import_store.go`](../../internal/storage/sqlite/import_store.go) 在提交时验证并重新检查导入。备份将一致 DB 快照与引用资产及其他必要 root 配对。

**取舍。** 不需运维数据库服务器，但写入会竞争。长时间最终导入事务可能延迟 RFQ；writer channel 不保证优先级或公平性。记录引用外部文件与 secrets 时，只备份 DB 档不足以恢复。

**失败／演进触发条件。** 依操作者同意的目标测量排队等待、RFQ 延迟、导入事务时间与恢复时间。负载无法符合目标时，再调整准备阶段、调度或存储。本指南不捏造吞吐量上限或 SLA。

**可核查证据。** [`Atomic Import 测试`](../../internal/storage/sqlite/import_store_test.go) 涵盖 revision 重验、回执重放与 rollback；[`备份测试`](../../internal/recovery/backup_files_test.go) 涵盖文件配对。带标签的 [`POC-02 harness`](../../internal/storage/sqlite/poc02_test.go) 是实验，不是生产容量证据。

## 3. Public 呈现与 Admin 交互分离

**决策。** 由 Go 生成 Public HTML，以局部 React 增强；Admin 使用 React／Refine／Ant Design。

**情境。** 访客与爬虫需要在 JavaScript 运行前取得产品信息、搜索链接及基本 RFQ 表单；管理员需要交互列表、表单、预览与工作状态。

**替代方案。** 单一整页 SPA 会让核心 Public 行为依赖客户端运行。独立 Node renderer 会改变运行环境的交付模型。纯服务器 Admin 表单可避免客户端状态，但会放弃当前选用的管理框架。

**选择理由。** [`Public template`](../../internal/webapp/templates/pages.tmpl) 与 [`发布呈现`](../../internal/publishing/render.go) 产生内容；[`Public roots`](../../web/ui/src/public/main.tsx) 加入 RFQ 选取／列表控制。分离的 Vite 入口内嵌 Admin 与 system 应用，不把 Public 变成 Admin bundle。

**取舍。** 两种呈现模型需要一致的多语与行为。共用已发布数据可降低 HTML／JSON／Markdown 差异，却不会自动证明所有呈现路径正确。Go HTML 不是 React SSR。

**失败／演进触发条件。** 新交互无法在不重复核心数据行为下局部增强，或实测呈现／维护成本足以支持另一模型；仍须保留无 JavaScript 的核心旅程。

**可核查证据。** [`搜索／列表 HTML 测试`](../../internal/webapp/search_test.go)、[`机器格式测试`](../../internal/publishing/machine_test.go) 及 [`多语单元测试`](../../internal/publishing/localized_unit_test.go)。

## 4. 领域命令保留各自结果语义

**决策。** Refine DataProvider 处理支持的 resource；具业务意义的状态转换走具名 adapter。发布及 RFQ 的成功判定各自依据持久证据。

**情境。** 编辑字段、封存产品、发布网站与重试询价，效果不能互换。响应遗失不代表写入失败。

**替代方案。** 全部映射成通用 CRUD、乐观 UI 或自动 mutation retry，能简化部分客户端代码，却会掩盖 revision、部分成功及结果不确定性。

**选择理由。** [`dataProvider.ts`](../../web/ui/src/admin/dataProvider.ts) 拒绝删除 Product；[`catalogCommands.ts`](../../web/ui/src/admin/catalogCommands.ts) 命名 Archive、Hide、Clone 与 preview。Publish 目前使用带 revision 的 Product PUT：这是语义边界，不保证每个命令都有独立 endpoint。[`api.ts`](../../web/ui/src/admin/api.ts) 在写入结果未知时引导用户核查已保存状态；[`AdminProviders.tsx`](../../web/ui/src/admin/AdminProviders.tsx) 停用 mutation retry 与乐观 mutation mode。

持久化也遵循相同原则。[`发布`](../../internal/publishing/protocol.go) 先准备单元再启用，撤销则在 ETag／Range 前拒绝新的准入。[`SubmitRFQ`](../../internal/storage/sqlite/store.go) 在 RFQ 事务中保存 canonical payload hash 与回执。相同 key／内容重放与冲突编辑是不同情况。Product Bulk 允许逐产品部分成功；Excel Import 则整批原子提交。

**取舍。** Adapter 与明确结果处理增加代码。存储发布操作可能早于公开启用，用户需要可查看状态。SMTP 结果可能不确定，不能当作 exactly-once delivery。

**失败／演进触发条件。** 命令绕过共用 transport、回执不足以厘清中断结果，或测试出现过期数据公开时，应先修复契约与证据，不用自动重试掩盖问题。

**可核查证据。** [`Adapter 测试`](../../web/ui/src/admin/adapters.test.ts)、[`正常发布测试`](../../internal/webapp/publication_test.go)、[`POC-03 协议测试`](../../internal/publishing/poc03_test.go)、[`RFQ 回执测试`](../../internal/storage/sqlite/rfq_receipt_test.go)。Renderer feasibility 与 publication correctness 是不同门槛，两者都不等于生产环境就绪。

## 5. 分类专属数据与受控呈现

**决策。** 规格与 Product 分开建模，重用 Spec Set、保留原始值，并把规范化结果视为衍生数据。分类列表 profile 与数据值分开配置。

**情境。** 巨大的扁平产品 schema 会混合不相干的组件属性；把所有原始字符串当成可靠标量，会误解单位、范围、条件与缺值。

**替代方案。** 每种属性一个字段会造成 schema 频繁变更；只有自由格式 blob 又难以支持适用性、来源及受控筛选。任意逐产品页面 layout 会把数据维护与呈现设计混在一起。

**选择理由。** [`maintenance.go`](../../internal/catalog/maintenance.go) 分别定义 SpecValue 与 NormalizedValue，包含来源 revision、语义版本与状态。[`spec_document_store.go`](../../internal/storage/sqlite/spec_document_store.go) 调整适用性。[`listing.go`](../../internal/cataloglisting/listing.go) 依完整范围的适用性产生 Common 字段，不只看第一列或目前页面。产品身份仍与路径及搜索 folding 分离。

**取舍。** 适用性、过期规范化及分类变更需要明确 reconciliation。数据模型与规范化记录不代表通用电气解析器，也不能证明两个器件兼容。结构化配置比任意 template 少一些弹性，但保留可审查的发布边界。

**失败／演进触发条件。** 真实分类无法表达必要属性，或用户反复需要未支持计算时，先定义附示例及失败行为的数据／规范化契约，再扩展 layout 或推论能力。

**可核查证据。** [`Common／profile 测试`](../../internal/cataloglisting/listing_test.go)、[`产品目录维护测试`](../../internal/catalog/maintenance_test.go)、[`空白制造商身份测试`](../../internal/storage/sqlite/d9_identity_test.go)。

## 6. 安装与恢复是生命周期边界

**决策。** 区分 fresh、installing、ready、upgrade-required 与 recovery-required。Restore 使用 journal，准备完成后向前完成同一操作。

**情境。** 初次没有数据库与既有数据库损坏，需要相反处理。跨 root 恢复 DB、资产与 secrets，不能用单一 filesystem rename 表达。

**替代方案。** 任何开启错误都自动初始化，可能覆盖既有站点证据。没有 journal 的 root 复制，无法区分某一步已完成或被中断。回到更早时间点是另一个 restore，不是准备后的隐含 undo。

**选择理由。** [`Inspect 与 CompleteInstallation`](../../internal/storage/sqlite/store.go) 分类状态，并一起提交初始 Owner／Ready。[`recovery_ui.go`](../../cmd/prods/recovery_ui.go) 提供独立修复入口。[`restore.go`](../../internal/recovery/restore.go) 记录逐 root hash 与进度，恢复同一操作，完成前验证全部 root。

**取舍。** 启动／恢复有更多状态与代码。Prepared 操作可能需要修复后才能回到正常服务。本机 manifest 与只读文件不提供异地主机灾难恢复、防篡改存储或恢复时间保证。

**失败／演进触发条件。** 中断步骤无法从持久证据厘清、平台文件操作行为不同，或操作者恢复目标超出实测行为时，应测试实际平台／存储拓扑，再明确修订协议。

**可核查证据。** [`安装测试`](../../internal/storage/sqlite/installation_test.go)、[`restore 故障边界测试`](../../internal/recovery/restore_fault_test.go)、[`shutdown 测试`](../../cmd/prods/shutdown_test.go)。模拟故障与交叉编译不能代替真实平台断电、文件系统或 service manager 验收。

## 范围与 AI 定位

**尚未发布的本地工作，不包含在本次文档提交中：** 核查的工作目录包含 `internal/webapp/brand_import.go` 与 `web/ui/src/admin/brandImportLabels.ts`，实现由人操作的外部 ChatGPT 流程。Prods 生成绑定请求的 prompt／schema；操作者在 Prods 外执行，再粘贴 JSON。验证检查 request／version／source URL 绑定并生成建议差异。明确批准并带入 expected revision 后只保存 Website Working Copy；发布仍是另一项操作。结构验证不会独立证实外部模型的观察。这些代码在本次核查时已处于开发中，不声称已发布或完成外部模型端到端验收，也不是内置推理或 RAG 服务。

- **已实现：** 前述产品目录维护、分类／规格数据、公开探索、发布、RFQ、Admin 与生命周期／运维代码；JSON／Markdown／`llms.txt` 是可读输出。
- **规划或延后：** 更广的内容编排／headless 能力不属目前 V1 交付；本文不承诺日期或实现设计。
- **探索性：** AI 辅助 RFQ 撷取只有在真实工作流程、可信度、验证及失败边界明确时才值得评估；不是已实现功能。
- **范围外：** 结账、价格／库存承诺、CRM、任意逐主体 layout 或 JavaScript、外部写入 token、权威跨厂替代料推荐、AI 聊天助理或 RAG 服务。语义相似不代表电气兼容。

代码仓库能支持产品边界、实现与运维机制的具体说明。要进一步证明生产系统正在运行，还需要部署专属证据；缺口列于[运维指南](operations.md)。
