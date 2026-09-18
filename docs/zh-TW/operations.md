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
