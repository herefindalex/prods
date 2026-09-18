# Prods 工程案例研究

[English](../en/engineering-case-study.md) | 繁體中文 | [简体中文](../zh-CN/engineering-case-study.md)

[README](../../README-zh_TW.md) · [架構](architecture.md) · [維運與證據限制](operations.md)

## 情境與證據

技術型 B2B 型錄必須保留一般名稱／描述／圖片清單無法表達的差異。半導體的電壓範圍、連接器的接點數與模組的介面是不同規格；原始值可能帶有條件或容差。製造商身分、料號、生命週期與 datasheet 有助於辨識產品，但不代表即時庫存或可作為替代料。

Prods 將管理員維護的型錄連接到公開探索與 RFQ。相同詢價流程接受型錄產品及訪客輸入的 Requested Part，因此型錄不完整時也不必捏造產品紀錄。實作包含分類／規格模型、公開發布協議、Admin 應用程式及安裝／復原路徑；這些不構成客戶採用或正式部署正在運行的證據。

本文於 2026-09-17 對照工作目錄查核（`VERSION`：`v0.6.8`）。以下連結指向公開原始碼與測試。內部需求及歷史 ADR 留在本機；本文不重新公開其內容，也不事後補造 ADR。替代方案是架構比較，不表示每個方案都曾做過原型。演進觸發條件是本指南提出的評估準則。

## 真正影響實作的需求

| 需求 | 可觀察的實作結果 |
| --- | --- |
| 小型團隊可以自架維運型錄 | 單一執行檔內嵌 UI，執行以 SQLite 為基礎的應用服務。 |
| 買家與爬蟲無 JavaScript 也能讀取核心內容 | Go 產生可用 HTML；React 增強部分控制。 |
| 已儲存資料與公開可見性必須可區分 | 發布意圖與啟用 revision 分開編輯與公開啟用。 |
| 重複 RFQ 提交需要可查核的結果 | Canonical payload hash 與持久收據和 RFQ 共用交易。 |
| 技術資料因分類而異，也可能不完整 | Spec Set、原始值與衍生正規化紀錄分開。 |
| 損壞狀態不能被當成新站點 | 啟動檢查、Installer 與 journaled Recovery 有明確邊界。 |

本文沒有把流量、營收、客戶數、高可用性或分散式服務需求描述成已確認需求或實測結果。

## 1. 單行程與明確模組

**決策。** 在一個 Go 執行檔內執行 HTTP、應用服務與有界背景工作。

**情境。** 自架型錄已經需要持久資料、資產檔案與復原程序；再要求獨立 API、renderer、queue 與 search 部署，會增加維運相依性。

**替代方案。** 元件需要獨立擴展或故障隔離時，可比較獨立服務與外部佇列；目前沒有證據確立這種正式環境需求。

**選擇理由。** [`main.go`](../../cmd/prods/main.go) 組裝服務，[`publishing.Repository`](../../internal/publishing/protocol.go) 與 [`backup Snapshotter`](../../internal/recovery/backup.go) 明確表達持久化相依性。SQL 工作紀錄在重啟後保留意圖，不需 queue server。

**取捨。** 封裝與本機診斷較簡單，但 CPU、記憶體、行程故障及維護仍共用。套件邊界有助推理與測試，不表示未來拆服務沒有成本，尤其涉及交易與可見性時。

**失敗／演進觸發條件。** 代表性量測顯示 worker 無法在資源限制內與互動流量共存，或部署確實需要元件獨立可用。先建立證據，再引入網路邊界。

## 2. SQLite 與受控寫入

**決策。** 使用 SQLite 作為主要資料庫，以 SQL-first repository 與序列化寫入准入協調操作。

**情境。** 型錄讀取、短 Admin／RFQ 寫入及偶爾的大型匯入成本不同。Atomic Import 與持久詢價保存需要明確的資源競爭契約。

**替代方案。** 資料庫伺服器可提供不同的並行與維運安排，也增加佈建、存取及備份責任。拆分匯入交易可縮短個別交易，卻會改變全成全敗契約。

**選擇理由。** [`beginWrite`](../../internal/storage/sqlite/store.go) 提供單行程共用寫入准入點。[`import_store.go`](../../internal/storage/sqlite/import_store.go) 在提交時驗證並重新檢查匯入。備份將一致 DB 快照與引用資產及其他必要 root 配對。

**取捨。** 不需維運資料庫伺服器，但寫入會競爭。長時間最終匯入交易可能延遲 RFQ；writer channel 不保證優先級或公平性。紀錄引用外部檔案與 secrets 時，只備份 DB 檔不足以復原。

**失敗／演進觸發條件。** 依操作者同意的目標量測排隊等待、RFQ 延遲、匯入交易時間與還原時間。負載無法符合目標時，再調整準備階段、排程或儲存。本指南不捏造吞吐量上限或 SLA。

**可查核證據。** [`Atomic Import 測試`](../../internal/storage/sqlite/import_store_test.go) 涵蓋 revision 重驗、收據重播與 rollback；[`備份測試`](../../internal/recovery/backup_files_test.go) 涵蓋檔案配對。帶標籤的 [`POC-02 harness`](../../internal/storage/sqlite/poc02_test.go) 是實驗，不是正式容量證據。

## 3. Public 呈現與 Admin 互動分離

**決策。** 由 Go 生成 Public HTML，以局部 React 增強；Admin 使用 React／Refine／Ant Design。

**情境。** 訪客與爬蟲需要在 JavaScript 執行前取得產品資訊、搜尋連結及基本 RFQ 表單；管理員需要互動列表、表單、預覽與工作狀態。

**替代方案。** 單一整頁 SPA 會讓核心 Public 行為依賴客戶端執行。獨立 Node renderer 會改變執行環境的交付模型。純伺服器 Admin 表單可避免客戶端狀態，但會放棄目前選用的管理框架。

**選擇理由。** [`Public template`](../../internal/webapp/templates/pages.tmpl) 與 [`發布呈現`](../../internal/publishing/render.go) 產生內容；[`Public roots`](../../web/ui/src/public/main.tsx) 加入 RFQ 選取／列表控制。分離的 Vite 入口內嵌 Admin 與 system 應用，不把 Public 變成 Admin bundle。

**取捨。** 兩種呈現模型需要一致的多語與行為。共用已發布資料可降低 HTML／JSON／Markdown 差異，卻不會自動證明所有呈現路徑正確。Go HTML 不是 React SSR。

**失敗／演進觸發條件。** 新互動無法在不重複核心資料行為下局部增強，或實測呈現／維護成本足以支持另一模型；仍須保留無 JavaScript 的核心旅程。

**可查核證據。** [`搜尋／列表 HTML 測試`](../../internal/webapp/search_test.go)、[`機器格式測試`](../../internal/publishing/machine_test.go) 及 [`多語單元測試`](../../internal/publishing/localized_unit_test.go)。

## 4. 領域命令保留各自結果語意

**決策。** Refine DataProvider 處理支援的 resource；具業務意義的狀態轉換走具名 adapter。發布及 RFQ 的成功判定各自依據持久證據。

**情境。** 編輯欄位、封存產品、發布網站與重試詢價，效果不能互換。回應遺失不代表寫入失敗。

**替代方案。** 全部映射成通用 CRUD、樂觀 UI 或自動 mutation retry，能簡化部分客戶端程式碼，卻會掩蓋 revision、部分成功及結果不確定性。

**選擇理由。** [`dataProvider.ts`](../../web/ui/src/admin/dataProvider.ts) 拒絕刪除 Product；[`catalogCommands.ts`](../../web/ui/src/admin/catalogCommands.ts) 命名 Archive、Hide、Clone 與 preview。Publish 目前使用帶 revision 的 Product PUT：這是語意邊界，不保證每個命令都有獨立 endpoint。[`api.ts`](../../web/ui/src/admin/api.ts) 在寫入結果未知時引導使用者查核已保存狀態；[`AdminProviders.tsx`](../../web/ui/src/admin/AdminProviders.tsx) 停用 mutation retry 與樂觀 mutation mode。

持久化也遵循相同原則。[`發布`](../../internal/publishing/protocol.go) 先準備單元再啟用，撤銷則在 ETag／Range 前拒絕新的准入。[`SubmitRFQ`](../../internal/storage/sqlite/store.go) 在 RFQ 交易中保存 canonical payload hash 與收據。相同 key／內容重播與衝突編輯是不同情況。Product Bulk 允許逐產品部分成功；Excel Import 則整批原子提交。

**取捨。** Adapter 與明確結果處理增加程式碼。儲存發布操作可能早於公開啟用，使用者需要可查閱狀態。SMTP 結果可能不確定，不能當作 exactly-once delivery。

**失敗／演進觸發條件。** 命令繞過共用 transport、收據不足以釐清中斷結果，或測試出現過期資料公開時，應先修復契約與證據，不用自動重試掩蓋問題。

**可查核證據。** [`Adapter 測試`](../../web/ui/src/admin/adapters.test.ts)、[`正常發布測試`](../../internal/webapp/publication_test.go)、[`POC-03 協議測試`](../../internal/publishing/poc03_test.go)、[`RFQ 收據測試`](../../internal/storage/sqlite/rfq_receipt_test.go)。Renderer feasibility 與 publication correctness 是不同門檻，兩者都不等於正式環境就緒。

## 5. 分類專屬資料與受控呈現

**決策。** 規格與 Product 分開建模，重用 Spec Set、保留原始值，並把正規化結果視為衍生資料。分類列表 profile 與資料值分開設定。

**情境。** 巨大的扁平產品 schema 會混合不相干的元件屬性；把所有原始字串當成可靠純量，會誤解單位、範圍、條件與缺值。

**替代方案。** 每種屬性一個欄位會造成 schema 頻繁變更；只有自由格式 blob 又難以支援適用性、來源及受控篩選。任意逐產品頁面 layout 會把資料維護與呈現設計混在一起。

**選擇理由。** [`maintenance.go`](../../internal/catalog/maintenance.go) 分別定義 SpecValue 與 NormalizedValue，包含來源 revision、語意版本與狀態。[`spec_document_store.go`](../../internal/storage/sqlite/spec_document_store.go) 調整適用性。[`listing.go`](../../internal/cataloglisting/listing.go) 依完整範圍的適用性產生 Common 欄位，不只看第一列或目前頁面。產品身分仍與路徑及搜尋 folding 分離。

**取捨。** 適用性、過期正規化及分類變更需要明確 reconciliation。資料模型與正規化紀錄不代表通用電氣解析器，也不能證明兩顆料相容。結構化設定比任意 template 少一些彈性，但保留可審查的發布邊界。

**失敗／演進觸發條件。** 真實分類無法表達必要屬性，或使用者反覆需要未支援計算時，先定義附範例及失敗行為的資料／正規化契約，再擴展 layout 或推論能力。

**可查核證據。** [`Common／profile 測試`](../../internal/cataloglisting/listing_test.go)、[`型錄維護測試`](../../internal/catalog/maintenance_test.go)、[`空白製造商身分測試`](../../internal/storage/sqlite/d9_identity_test.go)。

## 6. 安裝與復原是生命週期邊界

**決策。** 區分 fresh、installing、ready、upgrade-required 與 recovery-required。Restore 使用 journal，準備完成後向前完成同一操作。

**情境。** 初次沒有資料庫與既有資料庫損壞，需要相反處理。跨 root 還原 DB、資產與 secrets，不能用單一 filesystem rename 表達。

**替代方案。** 任何開啟錯誤都自動初始化，可能覆蓋既有站點證據。沒有 journal 的 root 複製，無法區分某一步已完成或被中斷。回到更早時間點是另一個 restore，不是準備後的隱含 undo。

**選擇理由。** [`Inspect 與 CompleteInstallation`](../../internal/storage/sqlite/store.go) 分類狀態，並一起提交初始 Owner／Ready。[`recovery_ui.go`](../../cmd/prods/recovery_ui.go) 提供獨立修復入口。[`restore.go`](../../internal/recovery/restore.go) 記錄逐 root hash 與進度，恢復同一操作，完成前驗證全部 root。

**取捨。** 啟動／復原有更多狀態與程式碼。Prepared 操作可能需要修復後才能回到正常服務。本機 manifest 與唯讀檔案不提供異地主機災難復原、防竄改儲存或復原時間保證。

**失敗／演進觸發條件。** 中斷步驟無法從持久證據釐清、平台檔案操作行為不同，或操作者復原目標超出實測行為時，應測試實際平台／儲存拓撲，再明確修訂協議。

**可查核證據。** [`安裝測試`](../../internal/storage/sqlite/installation_test.go)、[`restore 故障邊界測試`](../../internal/recovery/restore_fault_test.go)、[`shutdown 測試`](../../cmd/prods/shutdown_test.go)。模擬故障與交叉編譯不能代替原生斷電、檔案系統或 service manager 驗收。

## 範圍與 AI 定位

**尚未發布的本機工作，不包含於本次文件提交：** 查核的工作目錄包含 `internal/webapp/brand_import.go` 與 `web/ui/src/admin/brandImportLabels.ts`，實作由人操作的外部 ChatGPT 流程。Prods 產生綁定請求的 prompt／schema；操作者在 Prods 外執行，再貼回 JSON。驗證檢查 request／version／source URL 綁定並產生建議差異。明確核准且帶入 expected revision 後只儲存 Website Working Copy；發布仍是另一操作。結構驗證不會獨立證實外部模型的觀察。這些程式碼在本次查核時已處於開發中，不宣稱已發布或完成外部模型端到端驗收，也不是內建推論或 RAG 服務。

- **已實作：** 前述型錄維護、分類／規格資料、公開探索、發布、RFQ、Admin 與生命週期／維運程式碼；JSON／Markdown／`llms.txt` 是可讀輸出。
- **規劃或延後：** 更廣的內容編排／headless 能力不屬目前 V1 交付；本文不承諾日期或實作設計。
- **探索性：** AI 輔助 RFQ 擷取只有在真實工作流程、可信度、驗證及失敗邊界明確時才值得評估；不是已實作功能。
- **範圍外：** 結帳、價格／庫存承諾、CRM、任意逐主體 layout 或 JavaScript、外部寫入 token、權威跨廠替代料推薦、AI 聊天助理或 RAG 服務。語意相似不代表電氣相容。

儲存庫能支持產品邊界、實作與維運機制的具體說明。要進一步證明正式系統正在運行，還需要部署專屬證據；缺口列於[維運指南](operations.md)。
