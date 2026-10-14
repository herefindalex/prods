# 維運與正式環境證據

[English](../en/operations.md) | 繁體中文 | [简体中文](../zh-CN/operations.md)

[README](../../README-zh_TW.md) · [架構](architecture.md) · [工程案例研究](engineering-case-study.md)

## 正式環境狀態

於 2026-09-17 對照工作目錄檢視（`VERSION`：`v0.6.8`）。儲存庫包含可執行應用程式、發布封裝、維運控制及自動化測試，這些是實作證據。本次文件查核未驗證正式 endpoint、其部署 revision、真實客戶負載、正式 TLS 設定或異地主機還原演練。這裡的「正式環境」指預期由操作者管理、處理真實型錄／RFQ 資料的安裝，不是已達成的部署里程碑。

以下原始碼與測試連結可供檢視機制。測試存在不代表每個平台都通過；workflow 設定也不等於 CI 執行成功。目前 checkout 包含進行中的變更，本身不是乾淨的發布成品。

## 執行與部署邊界

發布模型為平台專屬 Go 執行檔、內嵌前端資產、本機 SQLite 及受管理檔案。[`main.go`](../../cmd/prods/main.go) 在正常或單次操作前取得 OS 層級資料庫 ownership lock。[`service_linux.go`](../../internal/platform/service_linux.go) 產生選用 systemd unit；[`service_windows.go`](../../internal/platform/service_windows.go) 實作 Windows service 整合。Service 定義不代表已在真實主機驗收 service manager。

對外部署預期由操作者提供 TLS 反向代理及明確 HTTPS Base URL。Prods 檢查 request authority，支援設定 trusted proxy 位址。儲存庫沒有特定正式 proxy／TLS 拓撲的執行證據。反向代理應轉送至應用，不應公開 data、backup、secrets 或歷史生成目錄。參見 [`proxy_trust.go`](../../internal/webapp/proxy_trust.go)。

預設路徑相對於執行檔：

| 設定 | 預設值／用途 |
| --- | --- |
| Listen | `:8080`；並非只監聽 loopback |
| Base URL | `http://127.0.0.1:8080` |
| Config | `prods.ini` |
| Data／DB | `data/`／`data/prods.db` |
| Backups | `backups/` |
| 受管理 data 子目錄 | 資產、work、生成輸出、secrets、control journals 與 logs |

優先序為 CLI flag → 環境變數 → 單一 config → 預設值。[`hostconfig/config.go`](../../internal/hostconfig/config.go) 定義 flag 與驗證。請使用 README 範例與實際執行檔的 `--help`；不要從開發者本機設定推測進階路徑或憑證。

## 如何啟動隔離的本機安裝

使用 Linux amd64 發布執行檔與新目錄，或依 [README](../../README-zh_TW.md) 從原始碼建置。以下 Linux shell 範例假設執行檔在目前目錄，且 port 18080 可用：

```sh
demo_dir="$(mktemp -d)"
cp ./prods-linux-amd64 "$demo_dir/prods"
chmod +x "$demo_dir/prods"
"$demo_dir/prods" --listen 127.0.0.1:18080 --base-url http://127.0.0.1:18080
```

1. 開啟 terminal 印出的一次性 Installer URL，建立 Owner 並完成安裝。空型錄是有效狀態；sample data 為選用且綁定執行檔發布版本。
2. Prods 結束後，重複最後一個命令。在 `http://127.0.0.1:18080/admin` 登入，並於 `/` 檢視公開網站。
3. 檢查 `http://127.0.0.1:18080/health/live` 與 `/health/ready`。Readiness 與行程 liveness 必須分開判斷。完成後以 `Ctrl+C` 停止。

若明確指定的 port 已占用，請換 port 並同步修改 listen／Base URL。啟動若回報資料損壞或無法辨識，保留目錄並使用 Recovery，不要當作重新安裝請求。Installer 與正常啟動行為由 [`main_test.go`](../../cmd/prods/main_test.go)、[`Installer 測試`](../../internal/webapp/installer_test.go) 及 [`安裝測試`](../../internal/storage/sqlite/installation_test.go) 涵蓋。

## 如何備份與還原

運行中的站點使用 Admin Backup 或排程器。以下命令列單次操作前，先停止 service／前景行程，並使用相同執行檔、config 與 data 路徑。Ownership lock 防止第二個行程共用 instance。

```sh
./prods-linux-amd64 --backup-now
./prods-linux-amd64 --restore-backup <backup-id-or-path>
```

以選定備份取代 restore placeholder。Restore 會變更持久狀態；第二個命令是另一項操作，不是每次備份後都必須執行。預設要求先取得目前站點的已驗證 pre-restore backup。`--allow-restore-without-prebackup` 是針對無法備份狀態的主機授權例外，不是預設流程。

Backup 將一致資料庫快照與其引用資產、指定 root／檔案及獨立 manifest 配對。Manifest 分開記錄內容驗證與唯讀套用結果。Secrets 使備份具有敏感性；checksum／唯讀旗標不代表加密、防竄改或主機損失後仍可復原。外部 secret 需求也必須滿足。

Restore 準備並驗證各 root、持久記錄 `prepared`，再依逐 root 證據向前完成同一操作。最終驗證後正常重啟，檢查 readiness、Admin、公開頁面與必要資產。Recovery 若回報缺失證據或 root 不符，保留 journal 與來源備份；不要刪除 journal 強迫正常啟動。參見 [`backup.go`](../../internal/recovery/backup.go)、[`restore.go`](../../internal/recovery/restore.go)、[`restore 故障測試`](../../internal/recovery/restore_fault_test.go) 及 [`backup／restore CLI 測試`](../../cmd/prods/backup_restore_test.go)。

## 如何復原 Owner 存取

正常 instance 停止後，以相同設定及既有 active Owner 的位址執行：

```sh
./prods-linux-amd64 --recover-owner owner@example.com
```

命令印出有期限的一次性連結後結束。正常重啟 Prods，在到期前開啟連結，並保持連結私密。命令要求已完成安裝的站點資料庫，不會重開 Installer，也不修復無法讀取的資料庫。證據：[`main.go`](../../cmd/prods/main.go)、[`identity_store.go`](../../internal/storage/sqlite/identity_store.go)、[`使用者生命週期測試`](../../internal/webapp/user_lifecycle_test.go)。

## 啟動、升級、關閉與診斷

- **啟動：** 持久狀態檢查區分 fresh、installing、ready、upgrade 與 recovery。未完成安裝取得新 claim，成功安裝要求重啟。
- **Migration：** 具版本 SQL 與 migration history 隨執行檔交付。升級啟動先建立／驗證備份並記錄進度，再允許正常執行。中斷操作必須釐清或進入 Recovery，不會默默接受。
- **Shutdown：** 關閉與釋放元件前先停止接收新工作並排空。HTTP shutdown 有 timeout，可能強制關閉，不保證所有故障下每個請求都完成。[`shutdown_test.go`](../../cmd/prods/shutdown_test.go) 涵蓋有界行為。
- **Maintenance：** 新 Public／RFQ 准入回 503，保留允許的 Admin、Recovery、system assets 與 liveness。這是受控停機，不是高可用性。
- **診斷：** JSON runtime log 與互動 terminal 呈現狀態；Admin 提供工作與 health 檢視。[`runtime_log.go`](../../internal/platform/runtime_log.go) 及 [`runtime_health.go`](../../internal/platform/runtime_health.go) 實作本機機制；這裡沒有外部監控／值班部署證據。
- **Secrets／整合：** 正常站點從私有檔案讀取 SMTP 與 Google 憑證。選用 mail／indexing 操作與型錄服務分開；provider 收據不證明已索引或 exactly-once 郵件投遞。參見 [`maildelivery`](../../internal/maildelivery/manager.go) 與 [`searchnotify`](../../internal/searchnotify/searchnotify.go)。

## 建置、發布與驗證範圍

[`ci.yml`](../../.github/workflows/ci.yml) 定義前端 typecheck／test、內嵌資產差異檢查、Go normal／PoC test、vet、license／sample 檢查及 cross-build。[`release.yml`](../../.github/workflows/release.yml) 驗證 `v*` tag 並發布檔案，不部署運行中的站點。[`build-release.sh`](../../scripts/build-release.sh) 檢查 `VERSION`、Go 版本、source revision、乾淨工作目錄與精確 sample 成品，再輸出執行檔、建置資訊與 checksum。

建置目標為 Linux amd64、Windows amd64 及 macOS amd64／arm64。Windows／macOS 原生 runtime、service、簽章及平台專屬復原，需針對同一成品另備證據，不能靠 Linux 測試結案。POC-01 renderer feasibility 與 POC-03 publication protocol correctness 是不同門檻；呈現 smoke test 不能證明發布正確性或正式環境就緒。

完整原始碼驗證命令保留在 [README](../../README-zh_TW.md)。前端建置會重寫內嵌檔案，release packaging 要求乾淨 checkout。請在適當 checkout 執行，不要只為驗證文件而覆寫其他進行中工作。

## 目前維運邊界

| 儲存庫證據 | 宣稱邊界 |
| --- | --- |
| Instance lock 與單行程執行 | 不保證多節點協調、failover 或零停機升級。 |
| 序列化 SQLite 寫入與 import 測試 | 沒有正式容量實測，也不保證 atomic import 期間 RFQ 零等待。 |
| 發布准入與協議測試 | 測試涵蓋指定情境；已准入傳輸可在撤銷後完成。 |
| 收據與稽核紀錄 | 是持久應用證據，不是外部服務 exactly-once 保證。 |
| Backup manifest 與 restore journal | 沒有經驗證的異地主機備份政策、RTO／RPO 或所有檔案系統的斷電保證。 |
| Session、CSRF、capability 與檔案驗證 | 是已實作控制，不是安全認證或全面安全保證。 |
| 十種內建語系資源 | 不宣稱已經專業母語、法律或行銷審校。 |
| 建置／發布 workflow 與 sample 型錄 | 不從 fixture 或自動化推論客戶數、商業成效、正式流量、SLA 或實際部署。 |

## 正式環境敘事仍需的證據

操作者可補充部署版本／source revision、主機／平台、service／proxy 設定、TLS 驗證、持久 root 對照、備份保留／異地主機位置、有日期的還原演練及量測負載／事件紀錄。憑證與客戶資料保持私密。在證據齊備前，描述實作與測試，不使用尚未驗證的正式環境成功敘事。

## 數值規格搜尋（後端）

本節描述 2026-10-14 查核的 `v0.7.0` 後端。前述章節與架構／工程案例指南保留歷史 `v0.6.8` 查核範圍。數值 profile 編輯、共用來源／採用控制及 Public 數值篩選 UI 延後提供；既有文字搜尋獨立運作。

### 設定候選後端

預設為 `bleve`。Flag `--spec-search-backend`、環境變數 `PRODS_SPEC_SEARCH_BACKEND` 與 INI key `[prods] spec_search_backend` 接受 `bleve` 或 `sqlite`，優先序為 CLI > ENV > INI > 預設值。選擇下列其中一種設定，使用相同站點路徑重啟：

```ini
[prods]
spec_search_backend=bleve
```

```sh
PRODS_SPEC_SEARCH_BACKEND=sqlite ./prods-linux-amd64
./prods-linux-amd64 --spec-search-backend sqlite
```

Bleve 內嵌於 Go 行程，在 `<data_dir>/generated/search/specs` 寫入私有、可重建候選索引。SQLite 仍為主要儲存；`sqlite` 後端改用其衍生 Spec-ID 投影。不要透過 filesystem server 公開 generated/search 目錄。公開啟用狀態變更後，由 publication worker 替換／重建候選。

候選失敗、不完整或與已准入 inventory／revision 不符時，查詢改為完整掃描目前公開 inventory 並精確判定。這保留結果與計數，但增加掃描成本，不保證延遲。Log 會記錄候選索引失敗。已開啟 adapter 的 refresh 會在後續 worker tick 重試；初次開啟失敗的 Bleve adapter 不會由 tick 重新開啟。修正 filesystem／ownership／resource 原因後重啟，或以 `sqlite` 重啟。`SpecSearchDiagnostics` 與 `RebuildSpecSearchIndex` 是內部方法，沒有 Admin／Public 診斷或重建 endpoint。正式公開索引本身不健康時仍回 503。

### 準備與採用數值事實

每個 Product + Spec 選擇一份共用 Raw 來源。`source_origin_locale` 記錄來源 provenance，不為每個公開語系建立另一份數值事實；語系展示文字獨立保存。Migration 19 保留舊 Raw／history，只為 Product + Spec 恰有一列的情況選擇來源；多來源舊群組保持未選擇，直到明確命令處理。它不從舊 display unit／history 建立數值 profile 或採用事實。Published Current Product 取得新 source revision 與 publication intent；替換完成前保留舊的有效公開 activation。內建合成樣本也只提供 Raw 規格，不包含設定好的 profile／已採用數值事實。

使用合法同源 Admin 登入與私有 cookie jar（`admin_cookie_jar`），從已登入 Admin 頁面的 `csrf-token` meta 值取得 `admin_csrf`。`base` 設為指定 Base URL，ID 變數設為既有 domain ID。Admin 讀取需要 `catalog.view`；寫入需要 `catalog.edit`、session cookie 及 `X-CSRF-Token`。這是 browser Admin 命令，不是外部 write token。以下 body 是示意檔案，請填入操作開啟時觀察到的 ID／revision。每個 mutation 範例是獨立操作，不是可盲目依序執行的 script。

| 方法／路徑 | 權限／body |
| --- | --- |
| `PUT /admin/api/specs/{id}/numeric-profile` | `catalog.edit`; `expected_revision`, `profile` |
| `GET /admin/api/products/{id}/spec-values` | `catalog.view`; 檢視已保存 Raw 來源 |
| `POST /admin/api/products/{id}/spec-source` | `catalog.edit`; `spec_id`, `spec_value_id`, `expected_product_revision`, `expected_source_revision`, `expected_source_selection_revision` |
| `GET /admin/api/spec-values/{id}/normalization-context` | `catalog.view`; 凍結 Raw、profile、`expected`、`selection`、`source_selection` |
| `POST /admin/api/spec-values/{id}/normalize` | `catalog.edit`; `expected` |
| `POST /admin/api/spec-values/{id}/normalized` | `catalog.edit`; `expected`, `value_json` (以字串編碼的 JSON) |
| `POST /admin/api/spec-values/{id}/normalization/automatic` | `catalog.edit`; `expected`, `candidate_id` |

Operating-voltage Spec 的 `profile-command.json` 範例（`expected_revision` 使用 Spec definition 的實際 `revision`）：

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

若 context 尚未選定共用來源，先用 `source-command.json` 選擇。三個 expectation 分別採用 context 的 `expected.product_revision`、`expected.source_revision` 與 `source_selection.revision`；成功後重新開啟 normalization context：

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

將下列存為 `normalize-command.json`，用 `normalization-context` 回傳的完整 `expected` 物件取代五個值：`product_revision`、`source_revision`、`spec_semantic_version`、`profile_revision`、`selection_revision`。前四個須為正數，selection revision 可為零。寫入交易重驗所有 expectation 及選定共用來源。

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

正規化可能採用有效 automatic 結果、在 manual preference 有效時記錄 candidate，或對無法解析 Raw 回 HTTP 200、`state: "failed"` 與 `reason_code`；不覆寫手動採用的 selection。Manual adoption 以 `value_json` 字串提供經驗證、canonical-unit 的 `Value`。上述 voltage profile 可用以下 `manual-command.json`：

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

若要將 manual preference 改為目前 automatic candidate，明確選擇伺服器已記錄的 ID，並將重新檢視的 context 填入 `automatic-command.json`；任意、過期或不相容 candidate 會被拒絕：

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

採用成功會增加 Product／selection revision，可能回 `publication_pending: true`。已保存或已排程不代表公開啟用。不要自動重試寫入；409 時保留編輯，讀取新 context 並檢視變更版本後再發出新命令。

### 查詢凍結公開事實

`POST /api/search/specs` 是有界讀取命令，不要求 Admin session 或 Admin CSRF token。將示意 Spec／category ID 換成已公開、profile 允許該 operator／unit 的 ID。Output-current 範例另需允許 `>=` 的 current profile：

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

請求需有 1–16 個 predicate、正數 `page` 及 1–100 的 `page_size`。Decimal operand 是 JSON 字串，必須明確指定 unit。Point operator（`contains_point`、`=`、`<`、`<=`、`>`、`>=`）只用 `value`；range operator（`contains_range`、`overlaps_range`、`between`）只用 `lower`／`upper`。不要傳不相關 operand key，即使值為 `null`。選填 `category_id` 包含該分類及子孫；`manufacturer_id` 以精確 ID 篩選。`locale` 必須已公開啟用，省略時解析 active Site Default。

所有 predicate 須對已准入 active `PublicView` 中採用的事實／凍結 profile 判定為 `MATCH`。缺失、過期、有條件或不相容事實為 `UNKNOWN`，不納入結果。分離 operating range 保留缺口；nominal／tolerance 與單向 limit claim 遵循宣告 role，不推斷為 operating range。候選索引不決定數值真相。在同一准入 snapshot 完成精確判定、可見性／scope 檢查後才計算 `total` 與分頁。結果為 `{rows,total,page,page_size}`，依 Part Number 再 stable Product ID 排序；超出範圍的頁面回空 rows，保留精確 total。待發布 source／profile 編輯影響未來 publication，不改目前 active 凍結事實。

### 解讀 API 結果

Admin：400 `invalid_json` 包含 body／expectation 格式錯誤及不合法 controlled value JSON；401 `unauthorized` 表示無有效 session；403 `forbidden` 或 `invalid_csrf` 表示權限／安全拒絕；404 `not_found` 包含缺少 source／profile context；409 `revision_conflict` 包含過期 expectation 或未選定共用來源；422 `validation_failed` 表示 domain／profile 不相容。Manual unit／quantity／role／allowed-shape 不相容回 422，tagged Value 格式錯誤回 400。Raw 解析失敗是上述成功 200 outcome，不是傳輸錯誤。

Public search：JSON／operand shape、decimal encoding 或頁面範圍錯誤回 400 `invalid_json`；明確指定 disabled／unknown locale 回 400 `validation_failed`（准入 locale-policy 競態可能回 400 `invalid_json`）。未知 operator 或與 scope 內凍結 definition 不相容的 operator／unit 回 422 `validation_failed`。Scope 內無 definition／fact 時不成立 match，不虛構 profile 語意。正式 publication 不可用回 503 `service_unavailable`；非預期 server error 回 500 `internal_error`。Maintenance 會攔截路由，回 HTTP 503 HTML 與 `Retry-After: 300`；draining 可能在 API 處理前回純文字 503。候選索引降級本身觸發精確 fallback，不回近似答案。

證據：[`spec_numeric.go`](../../internal/webapp/spec_numeric.go)、[`spec_search.go`](../../internal/webapp/spec_search.go)、[數值 profile](../../internal/specnumeric/profile.go)、[數值 value](../../internal/specnumeric/value.go)、[精確判定](../../internal/specnumeric/predicate.go)、[搜尋引擎](../../internal/publishing/spec_search.go)、[後端生命週期](../../internal/publishing/spec_search_backend.go) 與 [migration 19](../../internal/storage/sqlite/migrations.go)。這些連結說明實作；正式規模與 Windows／macOS 原生驗收仍需獨立證據。
