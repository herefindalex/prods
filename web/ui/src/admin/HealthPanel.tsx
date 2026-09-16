import { useCallback, useEffect, useRef, useState } from "react";
import { Alert, Button, Card, Input, Popconfirm, Select, Space, Table, Tag, Typography, message } from "antd";
import { api, apiText, putJSON } from "./api";
import type { AdminLocale } from "./locales";
import type { RuntimeLog, SiteMaintenance, SystemComponentHealth, SystemHealth, SystemResourceHealth } from "./types";

type Props = {
  locale: AdminLocale;
  onError(error: unknown): void;
};

type SystemUpdate = {
  status: "current" | "update_available" | "unavailable" | "development";
  current_version: string;
  latest_version?: string;
  release_page_url?: string;
  download_url?: string;
  checked_utc: string;
};

const updateText: Record<AdminLocale, { title: string; description(version: string): string; download: string }> = {
  "en-US": { title: "A newer Prods release is available", description: (version) => `Version ${version} is ready for this platform.`, download: "Download latest" },
  "zh-TW": { title: "Prods 有新版本", description: (version) => `此平台可下載 ${version}。`, download: "下載最新版" },
  "zh-CN": { title: "Prods 有新版本", description: (version) => `此平台可下载 ${version}。`, download: "下载最新版" },
  "ja-JP": { title: "Prods の新しいバージョンがあります", description: (version) => `このプラットフォーム用の ${version} をダウンロードできます。`, download: "最新版をダウンロード" },
  "ko-KR": { title: "새 Prods 버전이 있습니다", description: (version) => `이 플랫폼용 ${version}을 다운로드할 수 있습니다.`, download: "최신 버전 다운로드" },
  "de-DE": { title: "Eine neue Prods-Version ist verfügbar", description: (version) => `Version ${version} ist für diese Plattform verfügbar.`, download: "Neueste Version laden" },
  "fr-FR": { title: "Une nouvelle version de Prods est disponible", description: (version) => `La version ${version} est disponible pour cette plateforme.`, download: "Télécharger la dernière version" },
  "it-IT": { title: "È disponibile una nuova versione di Prods", description: (version) => `La versione ${version} è disponibile per questa piattaforma.`, download: "Scarica l’ultima versione" },
  "es-ES": { title: "Hay una nueva versión de Prods", description: (version) => `La versión ${version} está disponible para esta plataforma.`, download: "Descargar la última versión" },
  "pt-BR": { title: "Uma nova versão do Prods está disponível", description: (version) => `A versão ${version} está disponível para esta plataforma.`, download: "Baixar versão mais recente" },
};


const runtimeLogCopyText: Record<AdminLocale, { copy: string; copied: string; failed: string }> = {
  "en-US": { copy: "Copy all", copied: "Runtime log copied.", failed: "Could not copy the runtime log." },
  "zh-TW": { copy: "複製全部", copied: "已複製執行期日誌。", failed: "無法複製執行期日誌。" },
  "zh-CN": { copy: "复制全部", copied: "已复制运行时日志。", failed: "无法复制运行时日志。" },
  "ja-JP": { copy: "すべてコピー", copied: "実行時ログをコピーしました。", failed: "実行時ログをコピーできませんでした。" },
  "ko-KR": { copy: "전체 복사", copied: "런타임 로그를 복사했습니다.", failed: "런타임 로그를 복사할 수 없습니다." },
  "de-DE": { copy: "Alles kopieren", copied: "Laufzeitprotokoll kopiert.", failed: "Laufzeitprotokoll konnte nicht kopiert werden." },
  "fr-FR": { copy: "Tout copier", copied: "Journal d’exécution copié.", failed: "Impossible de copier le journal d’exécution." },
  "it-IT": { copy: "Copia tutto", copied: "Registro di runtime copiato.", failed: "Impossibile copiare il registro di runtime." },
  "es-ES": { copy: "Copiar todo", copied: "Registro de ejecución copiado.", failed: "No se pudo copiar el registro de ejecución." },
  "pt-BR": { copy: "Copiar tudo", copied: "Log de execução copiado.", failed: "Não foi possível copiar o log de execução." },
};
const labels = {
  "en-US": {
    title: "System health",
    refresh: "Refresh",
    healthy: "Database, publication, jobs, backup, search, integrations, and storage checks are healthy.",
    degraded: "One or more capabilities need attention. Unaffected safe operations remain available.",
    components: "Capabilities",
    component: "Capability",
    summary: "Summary",
    details: "Evidence",
    resources: "Storage resources",
    resource: "Resource",
    status: "Status",
    capacity: "Capacity",
    operations: "Affected operations",
    checked: "Checked",
    unavailable: "Unavailable",
    admissionFloor: "hard-stop floor",
    warningFloor: "warning floor",
    recommendedAction: "Recommended action",
    configured: "configured",
    notConfigured: "optional / not configured",
    lastSuccess: "last success",
    lastRun: "last run",
    nextRun: "next run",
    due: "due now",
    runtimeRunning: "runtime running",
    runtimeStopped: "runtime stopped",
    lastRuntimeAttempt: "last runtime attempt",
    lastRuntimeSuccess: "last runtime success",
    lastRuntimeFailure: "last runtime failure",
    runtimeStage: "failure stage",
    consecutiveFailures: "consecutive runtime failures",
		maintenanceTitle: "Manual site maintenance",
		maintenanceInactive: "Public catalog and RFQ submission are available.",
		maintenanceActive: "Public requests and new RFQs are paused with HTTP 503. Admin and recovery entry points remain available.",
		maintenanceMessage: "Public maintenance message",
		enableMaintenance: "Start maintenance",
		disableMaintenance: "End maintenance",
		confirmMaintenance: "Start maintenance after all already-admitted public writes drain?",
		runtimeLog: "Runtime log",
		logGeneration: "Log file",
		currentLog: "Current",
		previousLog: "Previous",
		logUnavailable: "This log generation is not available.",
		logTruncated: "Only the bounded tail is shown.",
  },
  "zh-TW": {
    title: "系統健康狀態",
    refresh: "重新檢查",
    healthy: "資料庫、公開生成、背景工作、備份、搜尋、整合與儲存檢查均正常。",
    degraded: "一項或多項能力需要處理；未受影響且安全的操作仍保持可用。",
    components: "系統能力",
    component: "能力",
    summary: "摘要",
    details: "證據",
    resources: "儲存資源",
    resource: "資源",
    status: "狀態",
    capacity: "容量",
    operations: "受影響操作",
    checked: "檢查時間",
    unavailable: "無法取得",
    admissionFloor: "硬性停止下限",
    warningFloor: "預警下限",
    recommendedAction: "建議處置",
    configured: "已設定",
    notConfigured: "選填／未設定",
    lastSuccess: "最後成功",
    lastRun: "最後執行",
    nextRun: "下次執行",
    due: "目前已到期",
    runtimeRunning: "執行中",
    runtimeStopped: "已停止",
    lastRuntimeAttempt: "最近背景嘗試",
    lastRuntimeSuccess: "最近背景成功",
    lastRuntimeFailure: "最近背景失敗",
    runtimeStage: "失敗階段",
    consecutiveFailures: "連續背景失敗",
		maintenanceTitle: "手動站點維護",
		maintenanceInactive: "公開型錄與 RFQ 提交目前可用。",
		maintenanceActive: "公開請求與新 RFQ 已暫停並回應 HTTP 503；Admin 與恢復入口仍可使用。",
		maintenanceMessage: "公開維護訊息",
		enableMaintenance: "開始維護",
		disableMaintenance: "結束維護",
		confirmMaintenance: "排空已接受的公開寫入後開始維護？",
		runtimeLog: "執行期日誌",
		logGeneration: "日誌檔案",
		currentLog: "目前",
		previousLog: "較舊",
		logUnavailable: "此代日誌目前不存在。",
		logTruncated: "僅顯示受限大小的尾端內容。",
  },
"zh-CN": {
    title: "\u7CFB\u7EDF\u5065\u5EB7\u72B6\u51B5",
    refresh: "\u5237\u65B0",
    healthy: "\u6570\u636E\u5E93\u3001\u53D1\u5E03\u3001\u4F5C\u4E1A\u3001\u5907\u4EFD\u3001\u641C\u7D22\u3001\u96C6\u6210\u548C\u5B58\u50A8\u68C0\u67E5\u5747\u6B63\u5E38\u3002",
    degraded: "\u9700\u8981\u5173\u6CE8\u4E00\u9879\u6216\u591A\u9879\u80FD\u529B\u3002\u4E0D\u53D7\u5F71\u54CD\u7684\u5B89\u5168\u64CD\u4F5C\u4ECD\u7136\u53EF\u7528\u3002",
    components: "\u80FD\u529B",
    component: "\u80FD\u529B",
    summary: "\u6982\u62EC",
    details: "\u8BC1\u636E",
    resources: "\u5B58\u50A8\u8D44\u6E90",
    resource: "\u8D44\u6E90",
    status: "\u5730\u4F4D",
    capacity: "\u5BB9\u91CF",
    operations: "\u53D7\u5F71\u54CD\u7684\u4E1A\u52A1",
    checked: "\u5DF2\u68C0\u67E5",
    unavailable: "\u4E0D\u53EF\u7528",
    admissionFloor: "\u786C\u505C\u5730\u677F",
    warningFloor: "\u8B66\u544A\u5C42",
    recommendedAction: "\u5EFA\u8BAE\u91C7\u53D6\u7684\u884C\u52A8",
    configured: "\u914D\u7F6E\u597D\u7684",
    notConfigured: "\u53EF\u9009/\u672A\u914D\u7F6E",
    lastSuccess: "\u6700\u540E\u7684\u6210\u529F",
    lastRun: "\u6700\u540E\u4E00\u6B21\u8FD0\u884C",
    nextRun: "\u4E0B\u4E00\u6B21\u8FD0\u884C",
    due: "\u73B0\u5728\u5230\u671F",
    runtimeRunning: "\u8FD0\u884C\u65F6\u8FD0\u884C",
    runtimeStopped: "\u8FD0\u884C\u65F6\u505C\u6B62",
    lastRuntimeAttempt: "\u6700\u540E\u4E00\u6B21\u8FD0\u884C\u65F6\u5C1D\u8BD5",
    lastRuntimeSuccess: "\u4E0A\u6B21\u8FD0\u884C\u65F6\u6210\u529F",
    lastRuntimeFailure: "\u4E0A\u6B21\u8FD0\u884C\u65F6\u5931\u8D25",
    runtimeStage: "\u5931\u8D25\u9636\u6BB5",
    consecutiveFailures: "\u8FDE\u7EED\u8FD0\u884C\u65F6\u5931\u8D25",
    maintenanceTitle: "\u624B\u52A8\u7AD9\u70B9\u7EF4\u62A4",
    maintenanceInactive: "\u63D0\u4F9B\u516C\u5171\u76EE\u5F55\u548C RFQ \u63D0\u4EA4\u3002",
    maintenanceActive: "\u516C\u5171\u8BF7\u6C42\u548C\u65B0 RFQ \u901A\u8FC7 HTTP 503 \u6682\u505C\u3002Admin \u548C\u6062\u590D\u5165\u53E3\u70B9\u4ECD\u7136\u53EF\u7528\u3002",
    maintenanceMessage: "\u516C\u5171\u7EF4\u62A4\u6D88\u606F",
    enableMaintenance: "\u5F00\u59CB\u7EF4\u62A4",
    disableMaintenance: "\u7ED3\u675F\u7EF4\u62A4",
    confirmMaintenance: "\u5728\u6240\u6709\u5DF2\u627F\u8BA4\u7684\u516C\u5171\u5199\u5165\u8017\u5C3D\u540E\u5F00\u59CB\u7EF4\u62A4\uFF1F",
    runtimeLog: "\u8FD0\u884C\u65F6\u65E5\u5FD7",
    logGeneration: "\u65E5\u5FD7\u6863\u6848",
    currentLog: "Current",
    previousLog: "\u4EE5\u524D\u7684",
    logUnavailable: "\u6B64\u65E5\u5FD7\u751F\u6210\u4E0D\u53EF\u7528\u3002",
    logTruncated: "\u4EC5\u663E\u793A\u6709\u754C\u5C3E\u90E8\u3002",
},
"ja-JP": {
    title: "\u30B7\u30B9\u30C6\u30E0\u306E\u5065\u5168\u6027",
    refresh: "\u30EA\u30D5\u30EC\u30C3\u30B7\u30E5",
    healthy: "\u30C7\u30FC\u30BF\u30D9\u30FC\u30B9\u3001\u30D1\u30D6\u30EA\u30B1\u30FC\u30B7\u30E7\u30F3\u3001\u30B8\u30E7\u30D6\u3001\u30D0\u30C3\u30AF\u30A2\u30C3\u30D7\u3001\u691C\u7D22\u3001\u7D71\u5408\u3001\u30B9\u30C8\u30EC\u30FC\u30B8 \u30C1\u30A7\u30C3\u30AF\u306F\u6B63\u5E38\u3067\u3059\u3002",
    degraded: "1 \u3064\u4EE5\u4E0A\u306E\u6A5F\u80FD\u306B\u6CE8\u610F\u304C\u5FC5\u8981\u3067\u3059\u3002\u5F71\u97FF\u3092\u53D7\u3051\u305A\u306B\u5B89\u5168\u306A\u64CD\u4F5C\u304C\u53EF\u80FD\u3067\u3059\u3002",
    components: "\u80FD\u529B",
    component: "\u80FD\u529B",
    summary: "\u307E\u3068\u3081",
    details: "\u8A3C\u62E0",
    resources: "\u30B9\u30C8\u30EC\u30FC\u30B8\u30EA\u30BD\u30FC\u30B9",
    resource: "\u30EA\u30BD\u30FC\u30B9",
    status: "\u72B6\u614B",
    capacity: "\u5BB9\u91CF",
    operations: "\u5F71\u97FF\u3092\u53D7\u3051\u308B\u696D\u52D9",
    checked: "\u30C1\u30A7\u30C3\u30AF\u6E08\u307F",
    unavailable: "\u5229\u7528\u4E0D\u53EF",
    admissionFloor: "\u30CF\u30FC\u30C9\u30B9\u30C8\u30C3\u30D7\u30D5\u30ED\u30A2",
    warningFloor: "\u8B66\u6212\u30D5\u30ED\u30A2",
    recommendedAction: "\u63A8\u5968\u3055\u308C\u308B\u30A2\u30AF\u30B7\u30E7\u30F3",
    configured: "\u8A2D\u5B9A\u6E08\u307F",
    notConfigured: "\u30AA\u30D7\u30B7\u30E7\u30F3/\u672A\u69CB\u6210",
    lastSuccess: "\u6700\u5F8C\u306E\u6210\u529F",
    lastRun: "\u6700\u5F8C\u306E\u5B9F\u884C",
    nextRun: "\u6B21\u306E\u5B9F\u884C",
    due: "\u4ECA\u671F\u9650",
    runtimeRunning: "\u30E9\u30F3\u30BF\u30A4\u30E0\u5B9F\u884C\u4E2D",
    runtimeStopped: "\u30E9\u30F3\u30BF\u30A4\u30E0\u304C\u505C\u6B62\u3057\u307E\u3057\u305F",
    lastRuntimeAttempt: "\u6700\u5F8C\u306E\u5B9F\u884C\u6642\u306E\u8A66\u884C",
    lastRuntimeSuccess: "\u524D\u56DE\u306E\u5B9F\u884C\u6642\u306E\u6210\u529F",
    lastRuntimeFailure: "\u6700\u5F8C\u306E\u5B9F\u884C\u6642\u306E\u5931\u6557",
    runtimeStage: "\u5931\u6557\u6BB5\u968E",
    consecutiveFailures: "\u9023\u7D9A\u3057\u305F\u30E9\u30F3\u30BF\u30A4\u30E0\u30A8\u30E9\u30FC",
    maintenanceTitle: "\u624B\u52D5\u306B\u3088\u308B\u30B5\u30A4\u30C8\u306E\u30E1\u30F3\u30C6\u30CA\u30F3\u30B9",
    maintenanceInactive: "\u516C\u958B\u30AB\u30BF\u30ED\u30B0\u3068 RFQ \u306E\u63D0\u51FA\u304C\u53EF\u80FD\u3067\u3059\u3002",
    maintenanceActive: "\u30D1\u30D6\u30EA\u30C3\u30AF \u30EA\u30AF\u30A8\u30B9\u30C8\u3068\u65B0\u3057\u3044 RFQ \u306F HTTP 503 \u3067\u4E00\u6642\u505C\u6B62\u3055\u308C\u307E\u3059\u3002Admin \u3068\u30EA\u30AB\u30D0\u30EA \u30A8\u30F3\u30C8\u30EA \u30DD\u30A4\u30F3\u30C8\u306F\u5F15\u304D\u7D9A\u304D\u5229\u7528\u53EF\u80FD\u3067\u3059\u3002",
    maintenanceMessage: "\u516C\u958B\u30E1\u30F3\u30C6\u30CA\u30F3\u30B9\u30E1\u30C3\u30BB\u30FC\u30B8",
    enableMaintenance: "\u30E1\u30F3\u30C6\u30CA\u30F3\u30B9\u958B\u59CB",
    disableMaintenance: "\u30E1\u30F3\u30C6\u30CA\u30F3\u30B9\u7D42\u4E86",
    confirmMaintenance: "\u3059\u3067\u306B\u627F\u8A8D\u3055\u308C\u3066\u3044\u308B\u30D1\u30D6\u30EA\u30C3\u30AF\u66F8\u304D\u8FBC\u307F\u304C\u3059\u3079\u3066\u306A\u304F\u306A\u3063\u305F\u5F8C\u3067\u30E1\u30F3\u30C6\u30CA\u30F3\u30B9\u3092\u958B\u59CB\u3057\u307E\u3059\u304B?",
    runtimeLog: "\u5B9F\u884C\u6642\u30ED\u30B0",
    logGeneration: "\u30ED\u30B0\u30D5\u30A1\u30A4\u30EB",
    currentLog: "Current",
    previousLog: "\u524D\u306E",
    logUnavailable: "\u3053\u306E\u30ED\u30B0\u751F\u6210\u306F\u5229\u7528\u3067\u304D\u307E\u305B\u3093\u3002",
    logTruncated: "\u5883\u754C\u306E\u3042\u308B\u5C3E\u90E8\u306E\u307F\u304C\u8868\u793A\u3055\u308C\u307E\u3059\u3002",
},
"ko-KR": {
    title: "\uC2DC\uC2A4\uD15C \uC0C1\uD0DC",
    refresh: "\uC0C8\uB85C \uACE0\uCE58\uB2E4",
    healthy: "\uB370\uC774\uD130\uBCA0\uC774\uC2A4, \uAC8C\uC2DC, \uC791\uC5C5, \uBC31\uC5C5, \uAC80\uC0C9, \uD1B5\uD569 \uBC0F \uC800\uC7A5\uC18C \uD655\uC778\uC774 \uC815\uC0C1\uC785\uB2C8\uB2E4.",
    degraded: "\uD558\uB098 \uC774\uC0C1\uC758 \uAE30\uB2A5\uC5D0 \uC8FC\uC758\uAC00 \uD544\uC694\uD569\uB2C8\uB2E4. \uC601\uD5A5\uC744 \uBC1B\uC9C0 \uC54A\uC740 \uC548\uC804\uD55C \uC791\uB3D9\uC740 \uACC4\uC18D \uAC00\uB2A5\uD569\uB2C8\uB2E4.",
    components: "\uAE30\uB2A5",
    component: "\uB2A5\uB825",
    summary: "\uC694\uC57D",
    details: "\uC99D\uAC70",
    resources: "\uC2A4\uD1A0\uB9AC\uC9C0 \uB9AC\uC18C\uC2A4",
    resource: "\uC758\uC9C0",
    status: "\uC0C1\uD0DC",
    capacity: "\uC6A9\uB7C9",
    operations: "\uC601\uD5A5\uC744 \uBC1B\uB294 \uC791\uC5C5",
    checked: "\uCCB4\uD06C\uB428",
    unavailable: "\uC5C6\uB294",
    admissionFloor: "\uD558\uB4DC \uC2A4\uD1B1 \uBC14\uB2E5",
    warningFloor: "\uACBD\uACE0\uCE35",
    recommendedAction: "\uAD8C\uC7A5 \uC870\uCE58",
    configured: "\uAD6C\uC131\uB41C",
    notConfigured: "\uC120\uD0DD \uC0AC\uD56D / \uAD6C\uC131\uB418\uC9C0 \uC54A\uC74C",
    lastSuccess: "\uB9C8\uC9C0\uB9C9 \uC131\uACF5",
    lastRun: "\uB9C8\uC9C0\uB9C9 \uC2E4\uD589",
    nextRun: "\uB2E4\uC74C \uC2E4\uD589",
    due: "\uC9C0\uAE08 \uB9C8\uAC10",
    runtimeRunning: "\uB7F0\uD0C0\uC784 \uC2E4\uD589 \uC911",
    runtimeStopped: "\uB7F0\uD0C0\uC784\uC774 \uC911\uC9C0\uB428",
    lastRuntimeAttempt: "\uB9C8\uC9C0\uB9C9 \uB7F0\uD0C0\uC784 \uC2DC\uB3C4",
    lastRuntimeSuccess: "\uB9C8\uC9C0\uB9C9 \uB7F0\uD0C0\uC784 \uC131\uACF5",
    lastRuntimeFailure: "\uB9C8\uC9C0\uB9C9 \uB7F0\uD0C0\uC784 \uC2E4\uD328",
    runtimeStage: "\uC2E4\uD328 \uB2E8\uACC4",
    consecutiveFailures: "\uC5F0\uC18D\uC801\uC778 \uB7F0\uD0C0\uC784 \uC2E4\uD328",
    maintenanceTitle: "\uC218\uB3D9 \uC0AC\uC774\uD2B8 \uC720\uC9C0 \uAD00\uB9AC",
    maintenanceInactive: "\uACF5\uAC1C \uCE74\uD0C8\uB85C\uADF8 \uBC0F RFQ \uC81C\uCD9C\uC774 \uAC00\uB2A5\uD569\uB2C8\uB2E4.",
    maintenanceActive: "\uACF5\uAC1C \uC694\uCCAD \uBC0F \uC0C8 RFQ\uB294 HTTP 503\uC73C\uB85C \uC77C\uC2DC \uC911\uC9C0\uB429\uB2C8\uB2E4. Admin \uBC0F \uBCF5\uAD6C \uC9C4\uC785\uC810\uC740 \uACC4\uC18D \uC0AC\uC6A9\uD560 \uC218 \uC788\uC2B5\uB2C8\uB2E4.",
    maintenanceMessage: "\uACF5\uAC1C \uC720\uC9C0\uBCF4\uC218 \uBA54\uC2DC\uC9C0",
    enableMaintenance: "\uC720\uC9C0\uBCF4\uC218 \uC2DC\uC791",
    disableMaintenance: "\uC720\uC9C0\uBCF4\uC218 \uC885\uB8CC",
    confirmMaintenance: "\uC774\uBBF8 \uC2B9\uC778\uB41C \uBAA8\uB4E0 \uACF5\uAC1C \uC4F0\uAE30\uAC00 \uC0AD\uC81C\uB41C \uD6C4 \uC720\uC9C0 \uAD00\uB9AC\uB97C \uC2DC\uC791\uD558\uC2DC\uACA0\uC2B5\uB2C8\uAE4C?",
    runtimeLog: "\uB7F0\uD0C0\uC784 \uB85C\uADF8",
    logGeneration: "\uB85C\uADF8 \uD30C\uC77C",
    currentLog: "Current",
    previousLog: "\uC774\uC804\uC758",
    logUnavailable: "\uC774 \uB85C\uADF8 \uC0DD\uC131\uC744 \uC0AC\uC6A9\uD560 \uC218 \uC5C6\uC2B5\uB2C8\uB2E4.",
    logTruncated: "\uC81C\uD55C\uB41C \uAF2C\uB9AC\uB9CC \uD45C\uC2DC\uB429\uB2C8\uB2E4.",
},
"de-DE": {
    title: "Systemgesundheit",
    refresh: "Aktualisieren",
    healthy: "Datenbank, Ver\u00F6ffentlichung, Jobs, Sicherung, Suche, Integrationen und Speicherpr\u00FCfungen sind fehlerfrei.",
    degraded: "Eine oder mehrere F\u00E4higkeiten erfordern Aufmerksamkeit. Der sichere Betrieb bleibt davon unber\u00FChrt.",
    components: "F\u00E4higkeiten",
    component: "F\u00E4higkeit",
    summary: "Zusammenfassung",
    details: "Beweis",
    resources: "Speicherressourcen",
    resource: "Ressource",
    status: "Status",
    capacity: "Kapazit\u00E4t",
    operations: "Betroffene Vorg\u00E4nge",
    checked: "Gepr\u00FCft",
    unavailable: "Nicht verf\u00FCgbar",
    admissionFloor: "Hartboden",
    warningFloor: "Warnboden",
    recommendedAction: "Empfohlene Ma\u00DFnahme",
    configured: "konfiguriert",
    notConfigured: "optional / nicht konfiguriert",
    lastSuccess: "letzter Erfolg",
    lastRun: "letzter Lauf",
    nextRun: "n\u00E4chster Lauf",
    due: "jetzt f\u00E4llig",
    runtimeRunning: "Laufzeit l\u00E4uft",
    runtimeStopped: "Laufzeit gestoppt",
    lastRuntimeAttempt: "letzter Laufzeitversuch",
    lastRuntimeSuccess: "letzter Laufzeiterfolg",
    lastRuntimeFailure: "letzter Laufzeitfehler",
    runtimeStage: "Stadium des Scheiterns",
    consecutiveFailures: "aufeinanderfolgende Laufzeitfehler",
    maintenanceTitle: "Manuelle Site-Wartung",
    maintenanceInactive: "\u00D6ffentlicher Katalog und RFQ-Einreichung sind verf\u00FCgbar.",
    maintenanceActive: "\u00D6ffentliche Anfragen und neue RFQs werden mit HTTP 503 angehalten. Admin und Wiederherstellungseinstiegspunkte bleiben verf\u00FCgbar.",
    maintenanceMessage: "\u00D6ffentliche Wartungsmeldung",
    enableMaintenance: "Starten Sie die Wartung",
    disableMaintenance: "Beenden Sie die Wartung",
    confirmMaintenance: "Mit der Wartung beginnen, nachdem alle bereits zugelassenen \u00F6ffentlichen Schreibvorg\u00E4nge abgelaufen sind?",
    runtimeLog: "Laufzeitprotokoll",
    logGeneration: "Protokolldatei",
    currentLog: "Current",
    previousLog: "Vorherige",
    logUnavailable: "Diese Protokollgenerierung ist nicht verf\u00FCgbar.",
    logTruncated: "Es wird nur der begrenzte Schwanz angezeigt.",
},
"fr-FR": {
    title: "Sant\u00E9 du syst\u00E8me",
    refresh: "Rafra\u00EEchir",
    healthy: "Les contr\u00F4les de base de donn\u00E9es, de publication, de t\u00E2ches, de sauvegarde, de recherche, d\u2019int\u00E9gration et de stockage sont sains.",
    degraded: "Une ou plusieurs fonctionnalit\u00E9s n\u00E9cessitent une attention particuli\u00E8re. Les op\u00E9rations s\u00FBres non affect\u00E9es restent disponibles.",
    components: "Capacit\u00E9s",
    component: "Capacit\u00E9",
    summary: "R\u00E9sum\u00E9",
    details: "Preuve",
    resources: "Ressources de stockage",
    resource: "Ressource",
    status: "Statut",
    capacity: "Capacit\u00E9",
    operations: "Op\u00E9rations concern\u00E9es",
    checked: "\u00C0 carreaux",
    unavailable: "Indisponible",
    admissionFloor: "sol dur",
    warningFloor: "plancher d'avertissement",
    recommendedAction: "Action recommand\u00E9e",
    configured: "configur\u00E9",
    notConfigured: "facultatif / non configur\u00E9",
    lastSuccess: "dernier succ\u00E8s",
    lastRun: "derni\u00E8re course",
    nextRun: "prochaine course",
    due: "\u00E0 payer maintenant",
    runtimeRunning: "ex\u00E9cution en cours d'ex\u00E9cution",
    runtimeStopped: "l'ex\u00E9cution s'est arr\u00EAt\u00E9e",
    lastRuntimeAttempt: "derni\u00E8re tentative d'ex\u00E9cution",
    lastRuntimeSuccess: "succ\u00E8s de la derni\u00E8re ex\u00E9cution",
    lastRuntimeFailure: "dernier \u00E9chec d'ex\u00E9cution",
    runtimeStage: "\u00E9tape de d\u00E9faillance",
    consecutiveFailures: "\u00E9checs d'ex\u00E9cution cons\u00E9cutifs",
    maintenanceTitle: "Maintenance manuelle du site",
    maintenanceInactive: "Le catalogue public et la soumission RFQ sont disponibles.",
    maintenanceActive: "Les demandes publiques et les nouveaux appels d'offres sont suspendus avec HTTP 503. Admin et les points d'entr\u00E9e de r\u00E9cup\u00E9ration restent disponibles.",
    maintenanceMessage: "Message de maintenance publique",
    enableMaintenance: "D\u00E9marrer la maintenance",
    disableMaintenance: "Fin de la maintenance",
    confirmMaintenance: "D\u00E9marrer la maintenance une fois que toutes les \u00E9critures publiques d\u00E9j\u00E0 admises ont \u00E9t\u00E9 \u00E9puis\u00E9es\u00A0?",
    runtimeLog: "Journal d'ex\u00E9cution",
    logGeneration: "Fichier journal",
    currentLog: "Current",
    previousLog: "Pr\u00E9c\u00E9dent",
    logUnavailable: "Cette g\u00E9n\u00E9ration de journaux n'est pas disponible.",
    logTruncated: "Seule la queue d\u00E9limit\u00E9e est affich\u00E9e.",
},
"it-IT": {
    title: "Salute del sistema",
    refresh: "Aggiorna",
    healthy: "I controlli di database, pubblicazione, processi, backup, ricerca, integrazioni e archiviazione sono integri.",
    degraded: "Una o pi\u00F9 funzionalit\u00E0 richiedono attenzione. Le operazioni sicure non interessate rimangono disponibili.",
    components: "Capacit\u00E0",
    component: "Capacit\u00E0",
    summary: "Riepilogo",
    details: "Prova",
    resources: "Risorse di archiviazione",
    resource: "Risorsa",
    status: "Stato",
    capacity: "Capacit\u00E0",
    operations: "Operazioni interessate",
    checked: "Controllato",
    unavailable: "Non disponibile",
    admissionFloor: "pavimento hard-stop",
    warningFloor: "piano di avvertimento",
    recommendedAction: "Azione consigliata",
    configured: "configurato",
    notConfigured: "opzionale/non configurato",
    lastSuccess: "ultimo successo",
    lastRun: "ultima corsa",
    nextRun: "prossima corsa",
    due: "dovuto adesso",
    runtimeRunning: "esecuzione in esecuzione",
    runtimeStopped: "il tempo di esecuzione \u00E8 stato interrotto",
    lastRuntimeAttempt: "ultimo tentativo di esecuzione",
    lastRuntimeSuccess: "ultimo successo di runtime",
    lastRuntimeFailure: "ultimo errore di runtime",
    runtimeStage: "fase di fallimento",
    consecutiveFailures: "errori di runtime consecutivi",
    maintenanceTitle: "Manutenzione manuale del sito",
    maintenanceInactive: "Sono disponibili il catalogo pubblico e l'invio RFQ.",
    maintenanceActive: "Le richieste pubbliche e le nuove richieste di offerta vengono sospese con HTTP 503. Admin e i punti di ingresso di ripristino rimangono disponibili.",
    maintenanceMessage: "Messaggio di manutenzione pubblica",
    enableMaintenance: "Inizia la manutenzione",
    disableMaintenance: "Terminare la manutenzione",
    confirmMaintenance: "Avviare la manutenzione dopo che tutte le scritture pubbliche gi\u00E0 ammesse si sono esaurite?",
    runtimeLog: "Registro di esecuzione",
    logGeneration: "File di registro",
    currentLog: "Current",
    previousLog: "Precedente",
    logUnavailable: "Questa generazione di log non \u00E8 disponibile.",
    logTruncated: "Viene mostrata solo la coda delimitata.",
},
"es-ES": {
    title: "Salud del sistema",
    refresh: "Refrescar",
    healthy: "Las comprobaciones de bases de datos, publicaciones, trabajos, copias de seguridad, b\u00FAsquedas, integraciones y almacenamiento est\u00E1n en buen estado.",
    degraded: "Una o m\u00E1s capacidades necesitan atenci\u00F3n. Las operaciones seguras no afectadas permanecen disponibles.",
    components: "Capacidades",
    component: "Capacidad",
    summary: "Resumen",
    details: "Evidencia",
    resources: "Recursos de almacenamiento",
    resource: "Recurso",
    status: "Estado",
    capacity: "Capacidad",
    operations: "Operaciones afectadas",
    checked: "Comprobado",
    unavailable: "Indisponible",
    admissionFloor: "piso duro",
    warningFloor: "piso de advertencia",
    recommendedAction: "Acci\u00F3n recomendada",
    configured: "configurado",
    notConfigured: "opcional / no configurado",
    lastSuccess: "\u00FAltimo \u00E9xito",
    lastRun: "\u00FAltima ejecuci\u00F3n",
    nextRun: "pr\u00F3xima ejecuci\u00F3n",
    due: "debido ahora",
    runtimeRunning: "tiempo de ejecuci\u00F3n corriendo",
    runtimeStopped: "tiempo de ejecuci\u00F3n detenido",
    lastRuntimeAttempt: "\u00FAltimo intento de ejecuci\u00F3n",
    lastRuntimeSuccess: "\u00FAltimo \u00E9xito en tiempo de ejecuci\u00F3n",
    lastRuntimeFailure: "\u00FAltimo fallo en tiempo de ejecuci\u00F3n",
    runtimeStage: "etapa de falla",
    consecutiveFailures: "fallos consecutivos en tiempo de ejecuci\u00F3n",
    maintenanceTitle: "mantenimiento manual del sitio",
    maintenanceInactive: "El cat\u00E1logo p\u00FAblico y el env\u00EDo de RFQ est\u00E1n disponibles.",
    maintenanceActive: "Las solicitudes p\u00FAblicas y las nuevas RFQ se pausan con HTTP 503. Admin y los puntos de entrada de recuperaci\u00F3n permanecen disponibles.",
    maintenanceMessage: "Mensaje de mantenimiento p\u00FAblico",
    enableMaintenance: "Iniciar mantenimiento",
    disableMaintenance: "Finalizar el mantenimiento",
    confirmMaintenance: "\u00BFIniciar el mantenimiento despu\u00E9s de que se agoten todas las escrituras p\u00FAblicas ya admitidas?",
    runtimeLog: "Registro de tiempo de ejecuci\u00F3n",
    logGeneration: "Archivo de registro",
    currentLog: "Current",
    previousLog: "Anterior",
    logUnavailable: "Esta generaci\u00F3n de registros no est\u00E1 disponible.",
    logTruncated: "S\u00F3lo se muestra la cola delimitada.",
},
"pt-BR": {
    title: "Sa\u00FAde do sistema",
    refresh: "Atualizar",
    healthy: "Banco de dados, publica\u00E7\u00E3o, trabalhos, backup, pesquisa, integra\u00E7\u00F5es e verifica\u00E7\u00F5es de armazenamento est\u00E3o \u00EDntegros.",
    degraded: "Um ou mais recursos precisam de aten\u00E7\u00E3o. As opera\u00E7\u00F5es seguras n\u00E3o afetadas permanecem dispon\u00EDveis.",
    components: "Capacidades",
    component: "Capacidade",
    summary: "Resumo",
    details: "Evid\u00EAncia",
    resources: "Recursos de armazenamento",
    resource: "Recurso",
    status: "Status",
    capacity: "Capacidade",
    operations: "Opera\u00E7\u00F5es afetadas",
    checked: "Verificado",
    unavailable: "Indispon\u00EDvel",
    admissionFloor: "piso r\u00EDgido",
    warningFloor: "piso de alerta",
    recommendedAction: "A\u00E7\u00E3o recomendada",
    configured: "configurado",
    notConfigured: "opcional/n\u00E3o configurado",
    lastSuccess: "\u00FAltimo sucesso",
    lastRun: "\u00FAltima corrida",
    nextRun: "pr\u00F3xima corrida",
    due: "devido agora",
    runtimeRunning: "tempo de execu\u00E7\u00E3o em execu\u00E7\u00E3o",
    runtimeStopped: "tempo de execu\u00E7\u00E3o interrompido",
    lastRuntimeAttempt: "\u00FAltima tentativa de tempo de execu\u00E7\u00E3o",
    lastRuntimeSuccess: "\u00FAltimo sucesso em tempo de execu\u00E7\u00E3o",
    lastRuntimeFailure: "\u00FAltima falha de tempo de execu\u00E7\u00E3o",
    runtimeStage: "est\u00E1gio de falha",
    consecutiveFailures: "falhas consecutivas de tempo de execu\u00E7\u00E3o",
    maintenanceTitle: "Manuten\u00E7\u00E3o manual do site",
    maintenanceInactive: "Cat\u00E1logo p\u00FAblico e envio RFQ est\u00E3o dispon\u00EDveis.",
    maintenanceActive: "Solicita\u00E7\u00F5es p\u00FAblicas e novas RFQs s\u00E3o pausadas com HTTP 503. Admin e pontos de entrada de recupera\u00E7\u00E3o permanecem dispon\u00EDveis.",
    maintenanceMessage: "Mensagem de manuten\u00E7\u00E3o p\u00FAblica",
    enableMaintenance: "Iniciar manuten\u00E7\u00E3o",
    disableMaintenance: "Terminar manuten\u00E7\u00E3o",
    confirmMaintenance: "Iniciar a manuten\u00E7\u00E3o depois que todas as grava\u00E7\u00F5es p\u00FAblicas j\u00E1 admitidas forem drenadas?",
    runtimeLog: "Registro de tempo de execu\u00E7\u00E3o",
    logGeneration: "Arquivo de registro",
    currentLog: "Current",
    previousLog: "Anterior",
    logUnavailable: "Esta gera\u00E7\u00E3o de log n\u00E3o est\u00E1 dispon\u00EDvel.",
    logTruncated: "Apenas a cauda delimitada \u00E9 mostrada.",
},
} as const;

const componentNames = {
  "en-US": {
    database: "Database",
		maintenance: "Maintenance",
    public_generation: "Public generation",
    background_jobs: "Background jobs",
    backup: "Backup",
    asset_gc: "Asset cleanup",
    search: "Search",
    smtp: "SMTP",
  },
  "zh-TW": {
    database: "資料庫",
		maintenance: "維護模式",
    public_generation: "公開生成",
    background_jobs: "背景工作",
    backup: "備份",
    asset_gc: "資產清理",
    search: "搜尋",
    smtp: "SMTP",
  },
"zh-CN": {
    database: "\u6570\u636E\u5E93",
    maintenance: "\u7EF4\u62A4",
    public_generation: "\u516C\u5171\u4E00\u4EE3",
    background_jobs: "\u540E\u53F0\u5DE5\u4F5C",
    backup: "\u5907\u4EFD",
    asset_gc: "\u8D44\u4EA7\u6E05\u7406",
    search: "\u641C\u7D22",
    smtp: "SMTP",
},
"ja-JP": {
    database: "\u30C7\u30FC\u30BF\u30D9\u30FC\u30B9",
    maintenance: "\u30E1\u30F3\u30C6\u30CA\u30F3\u30B9",
    public_generation: "\u516C\u5171\u767A\u96FB",
    background_jobs: "\u30D0\u30C3\u30AF\u30B0\u30E9\u30A6\u30F3\u30C9\u30B8\u30E7\u30D6",
    backup: "\u30D0\u30C3\u30AF\u30A2\u30C3\u30D7",
    asset_gc: "\u8CC7\u7523\u306E\u30AF\u30EA\u30FC\u30F3\u30A2\u30C3\u30D7",
    search: "\u691C\u7D22",
    smtp: "SMTP",
},
"ko-KR": {
    database: "\uB370\uC774\uD130 \uBCA0\uC774\uC2A4",
    maintenance: "\uC720\uC9C0",
    public_generation: "\uACF5\uACF5\uC138\uB300",
    background_jobs: "\uBC31\uADF8\uB77C\uC6B4\uB4DC \uC791\uC5C5",
    backup: "\uC9C0\uC6D0",
    asset_gc: "\uC790\uC0B0 \uC815\uB9AC",
    search: "\uCC3E\uB2E4",
    smtp: "SMTP",
},
"de-DE": {
    database: "Datenbank",
    maintenance: "Wartung",
    public_generation: "\u00D6ffentliche Generation",
    background_jobs: "Hintergrundjobs",
    backup: "Sicherung",
    asset_gc: "Bereinigung von Verm\u00F6genswerten",
    search: "Suchen",
    smtp: "SMTP",
},
"fr-FR": {
    database: "Base de donn\u00E9es",
    maintenance: "Entretien",
    public_generation: "G\u00E9n\u00E9ration publique",
    background_jobs: "Travaux en arri\u00E8re-plan",
    backup: "Sauvegarde",
    asset_gc: "Nettoyage des actifs",
    search: "Recherche",
    smtp: "SMTP",
},
"it-IT": {
    database: "Banca dati",
    maintenance: "Manutenzione",
    public_generation: "Generazione pubblica",
    background_jobs: "Lavori in background",
    backup: "Backup",
    asset_gc: "Pulizia delle risorse",
    search: "Ricerca",
    smtp: "SMTP",
},
"es-ES": {
    database: "Base de datos",
    maintenance: "Mantenimiento",
    public_generation: "generaci\u00F3n p\u00FAblica",
    background_jobs: "Trabajos en segundo plano",
    backup: "Respaldo",
    asset_gc: "Limpieza de activos",
    search: "Buscar",
    smtp: "SMTP",
},
"pt-BR": {
    database: "Banco de dados",
    maintenance: "Manuten\u00E7\u00E3o",
    public_generation: "Gera\u00E7\u00E3o p\u00FAblica",
    background_jobs: "Trabalhos em segundo plano",
    backup: "Backup",
    asset_gc: "Limpeza de ativos",
    search: "Procurar",
    smtp: "SMTP",
},
} as const;

function bytes(value?: number): string {
  if (value === undefined) return "—";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let amount = value;
  let unit = 0;
  while (amount >= 1024 && unit < units.length - 1) {
    amount /= 1024;
    unit += 1;
  }
  return `${amount.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
}

function statusTag(status: SystemComponentHealth["status"]) {
  return <Tag color={status === "Critical" ? "red" : status === "Warning" ? "gold" : "green"}>{status}</Tag>;
}


async function copyToClipboard(value: string): Promise<void> {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(value);
    return;
  }
  const textarea = document.createElement("textarea");
  textarea.value = value;
  textarea.style.position = "fixed";
  textarea.style.opacity = "0";
  document.body.appendChild(textarea);
  textarea.select();
  const copied = document.execCommand("copy");
  textarea.remove();
  if (!copied) throw new Error("clipboard copy was rejected");
}

export function HealthPanel({ locale, onError }: Props) {
  const text = labels[locale];
  const [health, setHealth] = useState<SystemHealth>();
	const [maintenance, setMaintenance] = useState<SiteMaintenance>();
	const [maintenanceMessage, setMaintenanceMessage] = useState("");
	const [runtimeLog, setRuntimeLog] = useState<RuntimeLog>();
	const [update, setUpdate] = useState<SystemUpdate>();
	const [runtimeGeneration, setRuntimeGeneration] = useState(0);
  const [loading, setLoading] = useState(false);
  const [copyingLog, setCopyingLog] = useState(false);
	const [updatingMaintenance, setUpdatingMaintenance] = useState(false);
  const onErrorRef = useRef(onError);
  onErrorRef.current = onError;

  const load = useCallback(async () => {
    setLoading(true);
    try {
		const [nextHealth, nextMaintenance, nextRuntimeLog, nextUpdate] = await Promise.all([
			api<SystemHealth>("/admin/api/system/health"),
			api<SiteMaintenance>("/admin/api/system/maintenance"),
			api<RuntimeLog>(`/admin/api/system/runtime-log?generation=${runtimeGeneration}`),
			api<SystemUpdate>("/admin/api/system/update").catch(() => undefined),
		]);
		setHealth(nextHealth);
		setMaintenance(nextMaintenance);
		setMaintenanceMessage(nextMaintenance.message);
		setRuntimeLog(nextRuntimeLog);
		setUpdate(nextUpdate);
    } catch (error) {
      onErrorRef.current(error);
    } finally {
      setLoading(false);
    }
	}, [runtimeGeneration]);

	const updateMaintenance = async (active: boolean) => {
		if (!maintenance) return;
		setUpdatingMaintenance(true);
		try {
			const updated = await putJSON<SiteMaintenance>("/admin/api/system/maintenance", {
				expected_revision: maintenance.revision,
				active,
				message: maintenanceMessage,
			});
			setMaintenance(updated);
			setMaintenanceMessage(updated.message);
			message.success(active ? text.enableMaintenance : text.disableMaintenance);
			void load();
		} catch (error) {
			onErrorRef.current(error);
		} finally {
			setUpdatingMaintenance(false);
		}
	};

  useEffect(() => {
    void load();
  }, [load]);
  const copyRuntimeLog = async () => {
    if (!runtimeLog?.available) return;
    setCopyingLog(true);
    try {
	  const content = await apiText(`/admin/api/system/runtime-log/text?generation=${runtimeGeneration}`);
	  await copyToClipboard(content);
      message.success(runtimeLogCopyText[locale].copied);
    } catch {
      message.error(runtimeLogCopyText[locale].failed);
    } finally {
      setCopyingLog(false);
    }
  };


  const componentColumns = [
    {
      title: text.component,
      dataIndex: "component",
      key: "component",
      render: (component: SystemComponentHealth["component"]) => componentNames[locale][component],
    },
    {
      title: text.status,
      dataIndex: "status",
      key: "status",
      render: statusTag,
    },
    { title: text.summary, dataIndex: "summary", key: "summary" },
    {
      title: text.details,
      key: "details",
      render: (_: unknown, item: SystemComponentHealth) => {
        const evidence: string[] = [];
        if (item.configured !== undefined) evidence.push(item.configured ? text.configured : text.notConfigured);
        if (item.last_success_utc) evidence.push(`${text.lastSuccess}: ${new Date(item.last_success_utc).toLocaleString(locale)}`);
        if (item.last_run_status) evidence.push(`${text.lastRun}: ${item.last_run_status}`);
        if (item.next_run_utc) evidence.push(`${text.nextRun}: ${new Date(item.next_run_utc).toLocaleString(locale)}`);
        if (item.due) evidence.push(text.due);
        if (item.runtime_running !== undefined) evidence.push(item.runtime_running ? text.runtimeRunning : text.runtimeStopped);
        if (item.last_runtime_attempt_utc) evidence.push(`${text.lastRuntimeAttempt}: ${new Date(item.last_runtime_attempt_utc).toLocaleString(locale)}`);
        if (item.last_runtime_success_utc) evidence.push(`${text.lastRuntimeSuccess}: ${new Date(item.last_runtime_success_utc).toLocaleString(locale)}`);
        if (item.last_runtime_failure_utc) evidence.push(`${text.lastRuntimeFailure}: ${new Date(item.last_runtime_failure_utc).toLocaleString(locale)}`);
        if (item.runtime_failure_stage) evidence.push(`${text.runtimeStage}: ${item.runtime_failure_stage}`);
        if (item.consecutive_runtime_failures) evidence.push(`${text.consecutiveFailures}: ${item.consecutive_runtime_failures}`);
        for (const [name, value] of Object.entries(item.counters ?? {})) evidence.push(`${name}: ${value}`);
        return evidence.length > 0 ? (
          <Space direction="vertical" size={0}>
            {evidence.map((value) => <span key={value}>{value}</span>)}
          </Space>
        ) : "—";
      },
    },
  ];

  const resourceColumns = [
    { title: text.resource, dataIndex: "resource", key: "resource" },
    {
      title: text.status,
      dataIndex: "status",
      key: "status",
      render: (status: SystemResourceHealth["status"]) => statusTag(status),
    },
    {
      title: text.capacity,
      key: "capacity",
      render: (_: unknown, item: SystemResourceHealth) => (
        <Space direction="vertical" size={0}>
          {item.status === "Critical" && <Typography.Text type="danger">{text.unavailable}</Typography.Text>}
          <span>{`${bytes(item.free_bytes)} free / ${bytes(item.total_bytes)} total (${bytes(item.reserved_bytes)} reserved)`}</span>
          {item.reason && <Typography.Text type="secondary">{item.reason}</Typography.Text>}
          <Typography.Text type="secondary">
            {text.admissionFloor}: {bytes(item.admission_floor_bytes)} · {text.warningFloor}: {bytes(item.warning_floor_bytes)}
          </Typography.Text>
          {item.recommended_action && <Typography.Text>{text.recommendedAction}: {item.recommended_action}</Typography.Text>}
        </Space>
      ),
    },
    {
      title: text.operations,
      dataIndex: "affected_operations",
      key: "operations",
      render: (items: string[]) => items.join(", "),
    },
    {
      title: text.checked,
      dataIndex: "checked_utc",
      key: "checked",
      render: (value: string) => new Date(value).toLocaleString(locale),
    },
  ];

  return (
    <Card title={text.title} extra={<Button onClick={() => void load()} loading={loading}>{text.refresh}</Button>}>
      {update?.status === "update_available" && update.download_url && (
        <Alert
          showIcon
          type="info"
          message={updateText[locale].title}
          description={updateText[locale].description(update.latest_version ?? "")}
          action={<Button type="primary" href={update.download_url} target="_blank" rel="noopener noreferrer">{updateText[locale].download}</Button>}
          style={{ marginBottom: 16 }}
        />
      )}
      {health && (
        <Alert
          showIcon
          type={health.status === "Critical" ? "error" : health.status === "Warning" ? "warning" : "success"}
          message={health.status === "Normal" ? text.healthy : text.degraded}
          style={{ marginBottom: 16 }}
        />
      )}
		<Card size="small" type="inner" title={text.maintenanceTitle} style={{ marginBottom: 24 }}>
			<Space direction="vertical" style={{ width: "100%" }}>
				<Alert
					showIcon
					type={maintenance?.active ? "warning" : "success"}
					message={maintenance?.active ? text.maintenanceActive : text.maintenanceInactive}
				/>
				<Typography.Text strong>{text.maintenanceMessage}</Typography.Text>
				<Input.TextArea
					value={maintenanceMessage}
					onChange={(event) => setMaintenanceMessage(event.target.value)}
					maxLength={280}
					showCount
					rows={2}
					disabled={updatingMaintenance}
				/>
				{maintenance?.active ? (
					<Button loading={updatingMaintenance} onClick={() => void updateMaintenance(false)}>{text.disableMaintenance}</Button>
				) : (
					<Popconfirm title={text.confirmMaintenance} onConfirm={() => updateMaintenance(true)} okText={text.enableMaintenance}>
						<Button danger loading={updatingMaintenance}>{text.enableMaintenance}</Button>
					</Popconfirm>
				)}
			</Space>
		</Card>
		<Card size="small" type="inner" title={text.runtimeLog} style={{ marginBottom: 24 }}>
			<Space direction="vertical" style={{ width: "100%" }}>
				<Space wrap>
					<Select
						aria-label={text.logGeneration}
						value={runtimeGeneration}
						onChange={setRuntimeGeneration}
						options={Array.from({ length: (runtimeLog?.max_generation ?? 5) + 1 }, (_, generation) => ({
							value: generation,
							label: generation === 0 ? text.currentLog : `${text.previousLog} ${generation}`,
						}))}
					/>
					<Button
						onClick={() => void copyRuntimeLog()}
						loading={copyingLog}
						disabled={!runtimeLog?.available}
					>
						{runtimeLogCopyText[locale].copy}
					</Button>
				</Space>
				{runtimeLog?.available ? (
					<>
						<Typography.Text type="secondary">
							{runtimeLog.file_name} · {bytes(runtimeLog.size_bytes)}
							{runtimeLog.truncated ? ` · ${text.logTruncated}` : ""}
						</Typography.Text>
						<pre style={{ maxHeight: 360, overflow: "auto", padding: 12, background: "#111827", color: "#e5e7eb", whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}>
							{runtimeLog.lines.join("\n")}
						</pre>
					</>
				) : <Alert type="info" showIcon message={text.logUnavailable} />}
			</Space>
		</Card>
      <Typography.Title level={5}>{text.components}</Typography.Title>
      <Table<SystemComponentHealth>
        rowKey="component"
        loading={loading}
        dataSource={health?.components ?? []}
        columns={componentColumns}
        pagination={false}
        scroll={{ x: true }}
      />
      <Typography.Title level={5} style={{ marginTop: 24 }}>{text.resources}</Typography.Title>
      <Table<SystemResourceHealth>
        rowKey="resource"
        loading={loading}
        dataSource={health?.resources ?? []}
        columns={resourceColumns}
        pagination={false}
        scroll={{ x: true }}
      />
    </Card>
  );
}
