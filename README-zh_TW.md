# Prods

[English](README.md) | 繁體中文 | [简体中文](README-zh_CN.md)

**把產品資料變成正式、可搜尋、可詢價的官方網站，不必維運一整套應用系統。**

Prods 是為製造商、經銷商與其他 B2B 產品團隊打造的自架產品型錄與 RFQ（詢價請求）系統。小型團隊可匯入並維護技術產品資料，安全發布給訪客、搜尋引擎與機器使用，再從同一個可攜式 Go 執行檔接收真實買家需求。

執行環境內含 Admin 後台、公開網站、SQLite 支援、搜尋、發布、RFQ、備份、復原與維運工具。Node.js 只在從原始碼建置時需要；部署後不需要 Node.js、獨立資料庫伺服器、Docker、佇列或外部搜尋服務。

## 為什麼選擇 Prods

- **沿用既有資料開始。** 管理產品、分類、規格、字典、文件、翻譯與 Website 設定。XLSX 匯入／匯出及批次操作可處理大量型錄，不必逐筆開表單修改。
- **精準控制公開狀態。** 產品頁、結構化資料、JSON、Markdown 與路由狀態一起切換。更新已發布產品時，舊的有效版本會保留到新版本完成；Hide 與 Archive 會立即停止新的公開存取。
- **同時服務工程師、採購、搜尋引擎與 AI 工具。** 型錄瀏覽、搜尋、分頁、文件及 RFQ 都有伺服器端語意 HTML，沒有 JavaScript 仍能使用。同一份已發布模型也供應 JSON-LD、JSON、Markdown、Sitemap、manifest 與 `llms.txt`。
- **先取得需求，不假裝成電商。** 訪客可以詢問一個或多個型錄產品；搜尋無結果時也能主動提交 Requested Part。Prods 以可重播且耐久的方式記錄 RFQ，價格、供貨、資格審查與後續聯絡仍由企業決定。
- **維運方式清楚可掌握。** 單一行程負責 migration、寫入節流、耐久工作、稽核、備份、restore journal 與健康檢查。Terminal 會顯示目前狀態、下一步、網址、Admin 路徑、主機資訊、資源路徑與即時 log。
- **掌握部署與資料。** Prods Community 採 AGPL 授權，以 SQLite 儲存主要狀態，將資產與備份放在明確的本機目錄，可在 Windows 或 Linux 上執行並搭配自選的反向代理。

## 適合的團隊

Prods 適合需要官方產品型錄與 RFQ 管道，但不想自行拼裝並維運 CMS、客製資料庫應用、搜尋服務與獨立後台的產品型企業。尤其適合零組件、工業、技術型與 B2B 型錄，這些情境通常重視料號、分類、規格、文件、公開控制與詢價脈絡。

V1 刻意不包含結帳、定價、庫存 Availability、CRM、通用頁面編輯器、任意自訂 JavaScript，也不會由 RFQ 自動建立產品。這些邊界讓型錄真實資料、公開內容與買家需求維持清楚可控。

## 主要能力

| 領域 | Prods 提供的能力 |
| --- | --- |
| 型錄作業 | Current／Archived 產品、任意深度分類、規格與 Spec Set、字典、圖片／文件、分頁、排序、XLSX 匯入／匯出，以及批次 Publish／Hide／Archive／分類／生命週期操作 |
| Website 與多語 | 版本化 Website 設定、預覽／發布流程、Public Copy 覆寫、導覽與主題控制、獨立 Admin UI 語系，以及十種內建公開語系 |
| 公開探索 | 伺服器端搜尋與型錄頁、產品／分類／製造商／品牌／應用路由、Unicode folding 搜尋、JSON-LD、JSON、Markdown、Sitemap、robots、manifest 與 `llms.txt` |
| RFQ | 多產品詢價、搜尋無結果後的 Requested Part、high-entropy idempotency key、耐久收據、Admin 檢視，以及明確觸發的選用 SMTP 寄送 |
| 權限與可追溯性 | 不透明 server-side session、CSRF 防護、能力型角色、Admin scope，以及必要管理操作記錄 |
| 維運 | Liveness／readiness、runtime log、排程與單次備份、驗證式 restore、Recovery、Owner 救援、Maintenance、資產清理與手動版本檢查 |
| 發布 | 預設空白安裝、選用且精確對應版本的 sample data、內嵌原始碼 revision／版本、checksum，以及 Linux／Windows amd64 成品 |

內建語系為 `en-US`、`zh-TW`、`zh-CN`、`ja-JP`、`ko-KR`、`de-DE`、`fr-FR`、`it-IT`、`es-ES`、`pt-BR`。預設文字已通過 key、placeholder、plural、格式與版面檢查，但尚未經專業母語、法律或行銷審校；正式上線前請審閱所有對外文案。

## 快速開始

從 [GitHub Releases](https://github.com/herefindalex/prods/releases) 下載同一版本的檔案：

- `prods-linux-amd64`、`prods-windows-amd64.exe`、`prods-darwin-arm64` 或 `prods-darwin-amd64`
- `SHA256SUMS` 與 `BUILD_INFO.txt`
- `LICENSE`、`LICENSE_POLICY.md`、各語言 README 與 `THIRD_PARTY_NOTICES.md`

執行前請先驗證下載檔。Linux：

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

Intel macOS 使用 `prods-darwin-amd64`。**目前兩個 macOS 執行檔都只有 cross-compile 結果，尚未在 macOS 實際執行、code sign 或 notarize；native runtime、Installer、Admin、backup／recovery 與 shutdown 驗收完成前，macOS 版本均視為未驗證。**

資料目錄全新時，Terminal 會顯示一次性的本機 Installer URL。開啟網址、認領安裝程序、建立第一位 Owner、選擇站點語系與時區，再完成安裝。Ready marker 交易完成後 Prods 會離開；重新啟動同一個執行檔，開啟畫面顯示的 Admin URL，並以 Owner 登入。

公開型錄從 `/` 開始；Admin 後台位於 `/admin`。

### Terminal Console

在互動式 terminal 中，Prods 使用 Bubble Tea 呈現三個視覺區域：

1. 單行 `Prods` 產品標題。
2. 狀態與操作指引，包含 Installer／Recovery URL、公開網址、Admin URL、版本、主機、監聽位址與資源路徑。
3. 可捲動的即時 log。

使用方向鍵、`PgUp`／`PgDn`、`Home`、`End` 檢視 log；`Ctrl+C` 會開始 graceful shutdown。網址使用 OSC 8 hyperlink，支援的 terminal 可直接點擊，不支援時仍會顯示可複製文字。Windows Service 與 redirected output 會改用 newline-delimited JSON，不顯示互動介面。

### 空白或 sample data 安裝

新安裝預設沒有任何產品測試資料，只有 Owner 明確選擇 sample data 時才會匯入。

每個 Prods 執行檔都會內建經壓縮、綁定發行版本的 sample payload。只有 payload 的 `release_version` 與 binary 編譯版本完全相同時，安裝器才會啟用這個選項；匯入前會驗證 schema、參照關係、重複 identity 與 payload 限制，再把 Owner、最小站點設定、sample records、audit entry 與 Ready marker 放在同一個 SQLite transaction 中提交。整個流程不需要網路，也不會在 runtime 另外保存 sample 檔案。

Binary 絕不改用其他 release 的資料。開發版或封裝錯誤而造成內建 payload 版本不符時，仍可完成空白安裝。Release 附帶的 JSON、schema 與 checksum 是供人審閱內建資料的發行證據，安裝器不會下載這些檔案。

## 可攜式執行環境與設定

預設路徑以執行檔所在目錄為基準，方便檢查、搬移、備份或註冊成服務：

| 設定 | 預設值 |
| --- | --- |
| 監聽位址 | `:8080` |
| Public Base URL | `http://127.0.0.1:8080` |
| 設定檔 | `prods.ini` |
| 資料目錄 | `data/` |
| 備份目錄 | `backups/` |
| SQLite 資料庫 | `data/prods.db` |

第一次安裝若預設 port 已被占用，Prods 可以選擇並保存下一個可用 port。已安裝或明確設定的站點會直接報錯，不會在未告知的情況下改用其他位址。

設定優先順序：

```text
CLI flag > environment variable > 單一 prods.ini > 可攜式預設值
```

執行 `prods --help` 查看所有參數。常見主機設定：

```ini
[prods]
listen=127.0.0.1:8080
base_url=https://catalog.example.com
data_dir=data
backup_dir=backups
trusted_proxies=127.0.0.1/32
asset_gc_grace_days=7
```

對應環境變數包含 `PRODS_LISTEN`、`PRODS_BASE_URL`、`PRODS_DATA_DIR`、`PRODS_BACKUP_DIR`、`PRODS_CONFIG`、`PRODS_TRUSTED_PROXIES`。

對外服務時，請由受信任的反向代理終止 HTTPS，明確設定正式 HTTPS Base URL，只接受已知 proxy IP／CIDR 提供的 forwarded client 資訊。不要用通用檔案伺服器公開 data、backup、secret 或 generated-history 目錄。

## 發布與 RFQ 行為

Prods 將 Domain Truth、Website 設定、Public Copy 與產生後的公開 representation 分開管理。

發布產品時會建立 immutable public representation；路由、HTML、JSON-LD、JSON、Markdown 與 public revision 以同一單位啟用。搜尋、分類、製造商、品牌與 Sitemap aggregate 可在之後收斂，但不會暴露已撤銷產品，也不會發布指向不可用內容的連結。全站 URL pattern 或 prefix 變更使用 atomic site epoch switch。

RFQ 不會建立 Product、不會虛構價格或 availability，也不會自動寄信。SMTP 是選用整合；只有授權的 Admin 使用者明確操作時才寄送，並逐一記錄收件結果。

## 備份、復原與 Maintenance

可在 Admin 建立與監看備份，或執行單次備份：

```sh
./prods-linux-amd64 --backup-now
```

離線 restore 會選擇已驗證的 backup ID 或路徑，完成 verified roll-forward 後離開：

```sh
./prods-linux-amd64 --restore-backup <backup-id-or-path>
```

預設情況下，restore 正常站點前必須先完成可驗證的 pre-restore backup。只有目前狀態已無法備份時，主機管理者才能明確使用 `--allow-restore-without-prebackup` 例外。

資料庫損壞或 prepared operation 中斷時，Prods 會進入 Recovery Required，不會覆蓋既有資料重新初始化。Terminal 會提供另一個有期限的 Recovery URL；Recovery 不依賴正常資料庫、Admin session、Public artifacts 或 Custom CSS。

Owner 忘記密碼時，可在不重開 Installer 的情況下建立一次性本機救援連結：

```sh
./prods-linux-amd64 --recover-owner owner@example.com
```

Manual Maintenance 從 Admin System Health 開始或結束。它會對新的 Public／RFQ admission 回傳 HTTP 503，同時保留 Admin、Recovery、system assets 與 liveness。

## 選用整合與版本更新

SMTP credential 與 Google Search Console OAuth secret 從 private file 載入，不把 bearer value 存在 SQLite。Secret file 應位於 `data/secrets`，讓 backup／restore 把它們視為必要 root。

IndexNow 與 Google Search Console submission 都是選用功能，並要求公開 HTTPS Base URL。成功 receipt 只代表 provider 接受請求，不代表頁面已被收錄；沒有這些 provider 時，公開渲染與 machine-readable output 仍可正常工作。

Admin System Health 只在使用者要求時檢查 GitHub 最新 stable release。若較新的 semantic version 提供目前平台的 binary，Admin 會顯示直接下載連結。網路錯誤、private／unavailable repo、無效 release metadata 或缺少平台資產都不會阻塞啟動與健康狀態。

## 架構

```text
Browser / crawler
       │
       ▼
單一 Prods Go process
  ├─ Public：semantic HTML + JSON-LD + JSON + Markdown + Sitemap/manifest/llms.txt
  ├─ Admin：內嵌 React + headless Refine + Ant Design
  ├─ System：內嵌 Installer / Recovery / Maintenance
  ├─ application services：catalog、RFQ、import/export、publication、backup/restore
  ├─ SQLite：authoritative state、migration、durable job、receipt、audit
  └─ private roots：immutable assets、secrets/config、public-state、backups
```

主要邊界：

- 執行檔是唯一必要的應用行程；Node.js 是 build dependency，不是使用者 runtime dependency。
- SQLite 是主要資料庫；Prods 自行管理 migration，所有寫入共用 bounded admission。
- 公開輸出只能來自明確的 `PublicView` allowlist，不能直接使用 Admin DTO 或 database row。
- 產品發布與撤銷以 durable intent 與 generated representation 完成；沒有耐久證據就不回報成功。
- RFQ idempotency 會在同一 transaction 保存 key、canonical payload hash、RFQ ID 與可重播 receipt。
- Restore 使用 durable journal；`prepared` 之前可安全放棄，之後只能用同一 operation 依各 root 證據向前完成。
- Admin 使用不透明 server-side session、server-side authorization、same-origin CSRF 與可信任 Host／proxy 設定。

## 從原始碼建置與驗證

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

前端建置會寫入 `internal/webapp/static/` 的內嵌資產；原始碼變更時應一起提交產生檔。

建立含內嵌版本、對應 sample data、原始碼 metadata、授權文件與 checksum 的 Linux amd64、Windows amd64、macOS Intel 與 macOS Apple Silicon release：

```sh
./scripts/build-release.sh v0.6.8
```

輸出位於 `dist/<version>/`。根目錄 [`VERSION`](VERSION) 是唯一版本來源；script 會拒絕不同參數，GitHub Actions 也會拒絕不同 tag。同一版本會寫入每個 binary、sample payload、`BUILD_INFO.txt`、產物目錄與 GitHub Release tag。Cross-build 只能證明可編譯；Windows 必須在 Windows 實際執行後才能宣稱通過 runtime 驗證，兩個 macOS 產物在 native macOS 驗收前均明確列為未測試。

## GitHub Actions 與 Release

`.github/workflows/ci.yml` 會在每次 push 與 pull request 執行：安裝 pinned frontend dependency、檢查 dependency license policy、執行 typecheck 與前端測試、重建並檢查 embedded assets、執行一般與 PoC Go suite、`go vet`、sample contract 驗證，以及 Linux amd64、Windows amd64、macOS Intel 與 macOS Apple Silicon cross-build，最後上傳短期 CI artifacts。

Push `v*` tag 會啟動 `.github/workflows/release.yml`。Workflow 會再次驗證原始碼，以精確 tag 呼叫 `scripts/build-release.sh`、驗證 `SHA256SUMS`、上傳 workflow artifact，並將所有檔案發布到相同 tag 的 GitHub Release。含連字號的 tag 會標成 prerelease，不會進入 Admin 的 latest-stable 更新路徑。

Release script 要求 tracked working tree 乾淨，並將版本與 source revision 寫入 binary 與 `BUILD_INFO.txt`。它會重新產生 tracked sample payload 與 schema checksum，逐位元相同才允許發布。

## 授權與商用相容性

Prods Community 採 [GNU Affero General Public License v3.0 only](LICENSE)，SPDX 識別為 `AGPL-3.0-only`。透過網路使用修改版 Community 程式的使用者，有權取得對應原始碼。

目前 release closure 的第三方元件僅使用 MIT、BSD、Apache-2.0、ISC／0BSD 或 public-domain 類型授權，未發現 GPL、AGPL、LGPL、SSPL、BSL、Commons Clause、PolyForm、非商用或禁止衍生授權。完整範圍、build-only 例外、未來封閉商用條件與自動檢查方式請見 [LICENSE_POLICY.md](LICENSE_POLICY.md)；保留聲明與授權文字請見 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。

第三方授權不會自動阻止未來另行授權的 proprietary edition；主要條件是 Prods 本身所有貢獻都必須具有可重新授權的權利。在正式 Contributor License Agreement、接受 CLA 的法律實體及紀錄流程建立前，不應合併實質外部貢獻。Product data、圖片、datasheet、RFQ、backup 與其他客戶內容不會因使用 Prods 而自動改採 AGPL。
