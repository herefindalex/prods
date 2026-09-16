import { useEffect, useState } from "react";
import { Alert, Button, Card, Form, Input, InputNumber, Progress, Select, Space, Table, Tag, Upload } from "antd";
import { api, postJSON } from "./api";
import { localeSelectOptions } from "./locales";
import type { AdminLocale } from "./locales";
import type { ImportIssue, ImportJob, ImportJobResponse, ImportMapping, ImportPreview } from "./types";

type Feedback = { onError: (error: unknown) => void; onMessage: (message: string) => void };
type ImportForm = {
  sheet?: string;
  header_row: number;
  identity_mode: "part_number" | "manufacturer_part_number";
  source_locale_override?: string;
  mappings: ImportMapping[];
};

const targets = [
  "part_number",
  "product_name",
  "manufacturer_id",
  "brand_id",
  "category_id",
  "package_form_factor",
  "description",
  "features",
  "specification",
  "lifecycle_id",
  "application_ids",
  "source_locale",
  "name_source_locale",
  "description_source_locale",
  "features_source_locale",
  "specification_source_locale",
];

const localeOptions = localeSelectOptions([
  "en-US",
  "zh-TW",
  "zh-CN",
  "ja-JP",
  "ko-KR",
  "de-DE",
  "fr-FR",
  "it-IT",
  "es-ES",
  "pt-BR",
]);

const activeStatuses = new Set(["queued", "parsing", "committing"]);

type ImportLocale = AdminLocale;

const labels = {
  "en-US": {
    accepted: (id: string) => `Import ${id} was accepted for validation.`,
    committed: (created: number, updated: number, unchanged: number) => `Import committed: ${created} created, ${updated} updated, ${unchanged} unchanged.`,
    cancelRequested: (id: string) => `Cancellation requested for import ${id}.`,
    title: "Atomic XLSX import",
    chooseFileFirst: "Choose an .xlsx file before starting the import.",
    description: "Validation scans every row and produces the complete error report. Commit rechecks identity, references, and revisions before one all-or-nothing transaction.",
    uploadMapping: "Upload and mapping", workbook: "Workbook", choose: "Choose .xlsx", sheet: "Sheet",
    sheetPlaceholder: "First sheet when blank", headerRow: "Header row", identityMode: "Identity mode",
    sourceLocaleOverride: "Import Source Locale override",
    sourceLocaleHelp: "Optional explicit fallback for this operation. Otherwise new content uses the currently Published Website Site Default. Per-row and per-field locale columns take precedence.",
    partNumber: "Part number", manufacturerPart: "Manufacturer + part number", columnIndex: "Column index",
    headerName: "Header name", productField: "Product field", remove: "Remove", addMapping: "Add mapping",
    validate: "Validate workbook", importTitle: (id: string) => `Import ${id}`, refresh: "Refresh", cancel: "Cancel",
    commit: "Commit atomically", download: "Download error CSV", rows: "rows", create: "create", update: "update",
    unchanged: "unchanged", issues: "issues", sheetColumn: "Sheet", row: "Row", column: "Column", code: "Code", message: "Message",
    target: {
      part_number: "Part number", product_name: "Product name", manufacturer_id: "Manufacturer",
      brand_id: "Brand", category_id: "Category", package_form_factor: "Package / form factor",
      description: "Description", features: "Features", specification: "Specification", lifecycle_id: "Lifecycle", application_ids: "Applications",
      source_locale: "Source Locale", name_source_locale: "Name Source Locale",
      description_source_locale: "Description Source Locale", features_source_locale: "Features Source Locale",
      specification_source_locale: "Specification Source Locale",
    } as Record<string, string>,
  },
  "zh-TW": {
    accepted: (id: string) => `匯入工作 ${id} 已受理並開始驗證。`,
    committed: (created: number, updated: number, unchanged: number) => `匯入已提交：新增 ${created} 筆、更新 ${updated} 筆、未變更 ${unchanged} 筆。`,
    cancelRequested: (id: string) => `已要求取消匯入工作 ${id}。`,
    title: "原子 XLSX 匯入",
    chooseFileFirst: "請先選擇 .xlsx 檔案再開始匯入。",
    description: "驗證會掃描每一列並產生完整錯誤報告。提交前會重新檢查識別、參照與修訂，最後以單一全成或全敗交易寫入。",
    uploadMapping: "上傳與欄位映射", workbook: "活頁簿", choose: "選擇 .xlsx", sheet: "工作表",
    sheetPlaceholder: "留空時使用第一個工作表", headerRow: "標題列", identityMode: "識別模式",
    sourceLocaleOverride: "本次匯入 Source Locale 覆寫",
    sourceLocaleHelp: "可選的明確整批 fallback；未指定時，新內容使用當時已發布的 Website Site Default。逐列及逐欄語系欄位優先。",
    partNumber: "料號", manufacturerPart: "Manufacturer + 料號", columnIndex: "欄位索引",
    headerName: "標題名稱", productField: "產品欄位", remove: "移除", addMapping: "新增映射",
    validate: "驗證活頁簿", importTitle: (id: string) => `匯入 ${id}`, refresh: "重新整理", cancel: "取消",
    commit: "原子提交", download: "下載錯誤 CSV", rows: "列數", create: "新增", update: "更新",
    unchanged: "未變更", issues: "問題", sheetColumn: "工作表", row: "列", column: "欄", code: "代碼", message: "訊息",
    target: {
      part_number: "料號", product_name: "產品名稱", manufacturer_id: "Manufacturer",
      brand_id: "品牌", category_id: "分類", package_form_factor: "封裝／外型",
      description: "描述", features: "特色", specification: "規格", lifecycle_id: "生命週期", application_ids: "應用",
      source_locale: "Source Locale", name_source_locale: "名稱 Source Locale",
      description_source_locale: "描述 Source Locale", features_source_locale: "特色 Source Locale",
      specification_source_locale: "規格 Source Locale",
    } as Record<string, string>,
  },
"zh-CN": {
    accepted: (id: string) => `\u8FDB\u53E3${id}\u88AB\u63A5\u53D7\u8FDB\u884C\u9A8C\u8BC1\u3002`,
    committed: (created: number, updated: number, unchanged: number) => `\u8FDB\u53E3\u627F\u8BFA\uFF1A${created}\u521B\u5EFA\uFF0C${updated}\u66F4\u65B0\u4E86\uFF0C${unchanged}\u4E0D\u53D8\u3002`,
    cancelRequested: (id: string) => `\u8FDB\u53E3\u7533\u8BF7\u53D6\u6D88${id}.`,
    title: "\u539F\u5B50XLSX\u5BFC\u5165",
    chooseFileFirst: "\u5728\u5F00\u59CB\u5BFC\u5165\u4E4B\u524D\u9009\u62E9\u4E00\u4E2A .xlsx \u6587\u4EF6\u3002",
    description: "\u9A8C\u8BC1\u626B\u63CF\u6BCF\u4E00\u884C\u5E76\u751F\u6210\u5B8C\u6574\u7684\u9519\u8BEF\u62A5\u544A\u3002\u63D0\u4EA4\u5728\u4E00\u7B14\u5168\u6709\u6216\u5168\u65E0\u7684\u4E8B\u52A1\u4E4B\u524D\u91CD\u65B0\u68C0\u67E5\u8EAB\u4EFD\u3001\u5F15\u7528\u548C\u4FEE\u8BA2\u3002",
    uploadMapping: "\u4E0A\u4F20\u548C\u6620\u5C04", workbook: "\u7EC3\u4E60\u518C", choose: "\u9009\u62E9.xlsx", sheet: "\u5E8A\u5355",
    sheetPlaceholder: "\u7B2C\u4E00\u5F20\u7EB8\u4E3A\u7A7A\u767D", headerRow: "\u6807\u9898\u884C", identityMode: "\u8EAB\u4EFD\u6A21\u5F0F",
    sourceLocaleOverride: "\u5BFC\u5165 Source Locale \u8986\u76D6",
    sourceLocaleHelp: "\u6B64\u64CD\u4F5C\u7684\u53EF\u9009\u663E\u5F0F\u56DE\u9000\u3002\u5426\u5219\u65B0\u5185\u5BB9\u4F7F\u7528\u5F53\u524D\u7684 Published Website Site Default\u3002\u6BCF\u884C\u548C\u6BCF\u5B57\u6BB5\u533A\u57DF\u8BBE\u7F6E\u5217\u4F18\u5148\u3002",
    partNumber: "\u96F6\u4EF6\u7F16\u53F7", manufacturerPart: "\u5236\u9020\u5546+\u96F6\u4EF6\u53F7", columnIndex: "\u680F\u76EE\u7D22\u5F15",
    headerName: "\u6807\u5934\u540D\u79F0", productField: "Product\u5B57\u6BB5", remove: "\u6D88\u9664", addMapping: "\u6DFB\u52A0\u6620\u5C04",
    validate: "\u9A8C\u8BC1\u5DE5\u4F5C\u7C3F", importTitle: (id: string) => `\u8FDB\u53E3${id}`, refresh: "\u5237\u65B0", cancel: "\u53D6\u6D88",
    commit: "\u539F\u5B50\u63D0\u4EA4", download: "\u4E0B\u8F7D\u9519\u8BEFCSV", rows: "\u884C", create: "\u521B\u9020", update: "\u66F4\u65B0",
    unchanged: "\u4E0D\u53D8", issues: "\u95EE\u9898", sheetColumn: "\u5E8A\u5355", row: "\u6392", column: "\u67F1\u5B50", code: "\u4EE3\u7801", message: "\u4FE1\u606F",
    target: {
        part_number: "\u96F6\u4EF6\u7F16\u53F7", product_name: "Product \u540D\u79F0", manufacturer_id: "\u5236\u9020\u5546",
        brand_id: "\u54C1\u724C", category_id: "\u7C7B\u522B", package_form_factor: "\u5C01\u88C5/\u5916\u5F62\u5C3A\u5BF8",
        description: "\u63CF\u8FF0", features: "\u7279\u5F81", specification: "\u89C4\u683C", lifecycle_id: "\u751F\u547D\u5468\u671F", application_ids: "\u5E94\u7528\u9886\u57DF",
        source_locale: "Source Locale", name_source_locale: "\u540D\u79F0 Source Locale",
        description_source_locale: "\u63CF\u8FF0 Source Locale", features_source_locale: "\u7279\u70B9 Source Locale",
        specification_source_locale: "\u89C4\u683C Source Locale",
    } as Record<string, string>,
},
"ja-JP": {
    accepted: (id: string) => `\u8F38\u5165${id}\u691C\u8A3C\u306E\u305F\u3081\u306B\u53D7\u3051\u5165\u308C\u3089\u308C\u307E\u3057\u305F\u3002`,
    committed: (created: number, updated: number, unchanged: number) => `\u30A4\u30F3\u30DD\u30FC\u30C8\u304C\u30B3\u30DF\u30C3\u30C8\u3055\u308C\u307E\u3057\u305F:${created}\u4F5C\u6210\u3055\u308C\u305F\u3001${updated}\u66F4\u65B0\u3055\u308C\u307E\u3057\u305F\u3001${unchanged}\u5909\u308F\u3089\u306A\u3044\u3002`,
    cancelRequested: (id: string) => `\u8F38\u5165\u306E\u30AD\u30E3\u30F3\u30BB\u30EB\u8981\u6C42${id}.`,
    title: "\u30A2\u30C8\u30DF\u30C3\u30AF XLSX \u30A4\u30F3\u30DD\u30FC\u30C8",
    chooseFileFirst: "\u30A4\u30F3\u30DD\u30FC\u30C8\u3092\u958B\u59CB\u3059\u308B\u524D\u306B .xlsx \u30D5\u30A1\u30A4\u30EB\u3092\u9078\u629E\u3057\u3066\u304F\u3060\u3055\u3044\u3002",
    description: "\u691C\u8A3C\u3067\u306F\u3059\u3079\u3066\u306E\u884C\u304C\u30B9\u30AD\u30E3\u30F3\u3055\u308C\u3001\u5B8C\u5168\u306A\u30A8\u30E9\u30FC \u30EC\u30DD\u30FC\u30C8\u304C\u751F\u6210\u3055\u308C\u307E\u3059\u3002\u30B3\u30DF\u30C3\u30C8\u306F\u3001\u30AA\u30FC\u30EB \u30AA\u30A2\u30CA\u30C3\u30B7\u30F3\u30B0 \u30C8\u30E9\u30F3\u30B6\u30AF\u30B7\u30E7\u30F3\u306E\u524D\u306B\u3001\u30A2\u30A4\u30C7\u30F3\u30C6\u30A3\u30C6\u30A3\u3001\u53C2\u7167\u3001\u304A\u3088\u3073\u30EA\u30D3\u30B8\u30E7\u30F3\u3092\u518D\u30C1\u30A7\u30C3\u30AF\u3057\u307E\u3059\u3002",
    uploadMapping: "\u30A2\u30C3\u30D7\u30ED\u30FC\u30C9\u3068\u30DE\u30C3\u30D4\u30F3\u30B0", workbook: "\u30EF\u30FC\u30AF\u30D6\u30C3\u30AF", choose: ".xlsx\u3092\u9078\u629E\u3057\u307E\u3059", sheet: "\u30B7\u30FC\u30C8",
    sheetPlaceholder: "\u7A7A\u767D\u306E\u5834\u5408\u306E\u6700\u521D\u306E\u30B7\u30FC\u30C8", headerRow: "\u30D8\u30C3\u30C0\u30FC\u884C", identityMode: "\u30A2\u30A4\u30C7\u30F3\u30C6\u30A3\u30C6\u30A3\u30E2\u30FC\u30C9",
    sourceLocaleOverride: "Source Locale \u30AA\u30FC\u30D0\u30FC\u30E9\u30A4\u30C9\u3092\u30A4\u30F3\u30DD\u30FC\u30C8",
    sourceLocaleHelp: "\u3053\u306E\u64CD\u4F5C\u306E\u30AA\u30D7\u30B7\u30E7\u30F3\u306E\u660E\u793A\u7684\u306A\u30D5\u30A9\u30FC\u30EB\u30D0\u30C3\u30AF\u3002\u305D\u308C\u4EE5\u5916\u306E\u5834\u5408\u3001\u65B0\u3057\u3044\u30B3\u30F3\u30C6\u30F3\u30C4\u306F\u73FE\u5728\u306E Published Website Site Default \u3092\u4F7F\u7528\u3057\u307E\u3059\u3002\u884C\u3054\u3068\u304A\u3088\u3073\u30D5\u30A3\u30FC\u30EB\u30C9\u3054\u3068\u306E\u30ED\u30B1\u30FC\u30EB\u5217\u304C\u512A\u5148\u3055\u308C\u307E\u3059\u3002",
    partNumber: "\u90E8\u54C1\u756A\u53F7", manufacturerPart: "\u30E1\u30FC\u30AB\u30FC+\u54C1\u756A", columnIndex: "\u5217\u30A4\u30F3\u30C7\u30C3\u30AF\u30B9",
    headerName: "\u30D8\u30C3\u30C0\u30FC\u540D", productField: "Product\u30D5\u30A3\u30FC\u30EB\u30C9", remove: "\u53D6\u308A\u9664\u304F", addMapping: "\u30DE\u30C3\u30D4\u30F3\u30B0\u306E\u8FFD\u52A0",
    validate: "\u30EF\u30FC\u30AF\u30D6\u30C3\u30AF\u306E\u691C\u8A3C", importTitle: (id: string) => `\u8F38\u5165${id}`, refresh: "\u30EA\u30D5\u30EC\u30C3\u30B7\u30E5", cancel: "\u30AD\u30E3\u30F3\u30BB\u30EB",
    commit: "\u30A2\u30C8\u30DF\u30C3\u30AF\u306B\u30B3\u30DF\u30C3\u30C8\u3059\u308B", download: "\u30C0\u30A6\u30F3\u30ED\u30FC\u30C9 \u30A8\u30E9\u30FC CSV", rows: "\u884C", create: "\u4F5C\u6210\u3059\u308B", update: "\u30A2\u30C3\u30D7\u30C7\u30FC\u30C8",
    unchanged: "\u5909\u308F\u3089\u306A\u3044", issues: "\u554F\u984C", sheetColumn: "\u30B7\u30FC\u30C8", row: "\u884C", column: "\u30AB\u30E9\u30E0", code: "\u30B3\u30FC\u30C9", message: "\u30E1\u30C3\u30BB\u30FC\u30B8",
    target: {
        part_number: "\u90E8\u54C1\u756A\u53F7", product_name: "Product \u540D\u524D", manufacturer_id: "\u30E1\u30FC\u30AB\u30FC",
        brand_id: "\u30D6\u30E9\u30F3\u30C9", category_id: "\u30AB\u30C6\u30B4\u30EA", package_form_factor: "\u30D1\u30C3\u30B1\u30FC\u30B8/\u30D5\u30A9\u30FC\u30E0\u30D5\u30A1\u30AF\u30BF\u30FC",
        description: "\u8AAC\u660E", features: "\u7279\u5FB4", specification: "\u4ED5\u69D8", lifecycle_id: "\u30E9\u30A4\u30D5\u30B5\u30A4\u30AF\u30EB", application_ids: "\u30A2\u30D7\u30EA\u30B1\u30FC\u30B7\u30E7\u30F3",
        source_locale: "Source Locale", name_source_locale: "\u540D\u524D Source Locale",
        description_source_locale: "\u8AAC\u660EDescription", features_source_locale: "\u7279\u5FB4",
        specification_source_locale: "\u4ED5\u69D8 Source Locale",
    } as Record<string, string>,
},
"ko-KR": {
    accepted: (id: string) => `\uC218\uC785${id}\uD655\uC778\uC744 \uC704\uD574 \uC2B9\uC778\uB418\uC5C8\uC2B5\uB2C8\uB2E4.`,
    committed: (created: number, updated: number, unchanged: number) => `\uAC00\uC838\uC624\uAE30 \uCEE4\uBC0B\uB428:${created}\uC0DD\uC131,${updated}\uC5C5\uB370\uC774\uD2B8,${unchanged}\uBCC0\uD558\uC9C0 \uC54A\uC740.`,
    cancelRequested: (id: string) => `\uC218\uC785 \uCDE8\uC18C \uC694\uCCAD${id}.`,
    title: "\uC6D0\uC790 XLSX \uAC00\uC838\uC624\uAE30",
    chooseFileFirst: "\uAC00\uC838\uC624\uAE30\uB97C \uC2DC\uC791\uD558\uAE30 \uC804\uC5D0 .xlsx \uD30C\uC77C\uC744 \uC120\uD0DD\uD558\uC138\uC694.",
    description: "\uC720\uD6A8\uC131 \uAC80\uC0AC\uB294 \uBAA8\uB4E0 \uD589\uC744 \uAC80\uC0AC\uD558\uACE0 \uC644\uC804\uD55C \uC624\uB958 \uBCF4\uACE0\uC11C\uB97C \uC0DD\uC131\uD569\uB2C8\uB2E4. \uCEE4\uBC0B\uC740 \uD558\uB098\uC758 \uC804\uBD80 \uC544\uB2C8\uBA74 \uC804\uBB34 \uD2B8\uB79C\uC7AD\uC158 \uC804\uC5D0 ID, \uCC38\uC870 \uBC0F \uAC1C\uC815\uC744 \uB2E4\uC2DC \uD655\uC778\uD569\uB2C8\uB2E4.",
    uploadMapping: "\uC5C5\uB85C\uB4DC \uBC0F \uB9E4\uD551", workbook: "\uD559\uC2B5\uC7A5", choose: ".xlsx\uB97C \uC120\uD0DD\uD558\uC138\uC694.", sheet: "\uC2DC\uD2B8",
    sheetPlaceholder: "\uBE44\uC5B4 \uC788\uB294 \uACBD\uC6B0 \uCCAB \uBC88\uC9F8 \uC2DC\uD2B8", headerRow: "\uD5E4\uB354 \uD589", identityMode: "\uC2E0\uC6D0 \uBAA8\uB4DC",
    sourceLocaleOverride: "Source Locale \uC7AC\uC815\uC758 \uAC00\uC838\uC624\uAE30",
    sourceLocaleHelp: "\uC774 \uC791\uC5C5\uC5D0 \uB300\uD55C \uC120\uD0DD\uC801 \uBA85\uC2DC\uC801 \uB300\uCCB4\uC785\uB2C8\uB2E4. \uADF8\uB807\uC9C0 \uC54A\uC73C\uBA74 \uC0C8 \uCF58\uD150\uCE20\uB294 \uD604\uC7AC Published Website Site Default\uB97C \uC0AC\uC6A9\uD569\uB2C8\uB2E4. \uD589\uBCC4 \uBC0F \uD544\uB4DC\uBCC4 \uB85C\uCE98 \uC5F4\uC774 \uC6B0\uC120\uC801\uC73C\uB85C \uC801\uC6A9\uB429\uB2C8\uB2E4.",
    partNumber: "\uBD80\uD488 \uBC88\uD638", manufacturerPart: "\uC81C\uC870\uC5C5\uCCB4 + \uBD80\uD488 \uBC88\uD638", columnIndex: "\uC5F4 \uC778\uB371\uC2A4",
    headerName: "\uD5E4\uB354 \uC774\uB984", productField: "Product \uD544\uB4DC", remove: "\uC81C\uAC70\uD558\uB2E4", addMapping: "\uB9E4\uD551 \uCD94\uAC00",
    validate: "\uD1B5\uD569 \uBB38\uC11C \uC720\uD6A8\uC131 \uAC80\uC0AC", importTitle: (id: string) => `\uC218\uC785${id}`, refresh: "\uC0C8\uB85C \uACE0\uCE58\uB2E4", cancel: "\uCDE8\uC18C",
    commit: "\uC6D0\uC790\uC801\uC73C\uB85C \uCEE4\uBC0B", download: "\uB2E4\uC6B4\uB85C\uB4DC \uC624\uB958 CSV", rows: "\uD589", create: "\uB9CC\uB4E4\uB2E4", update: "\uC5C5\uB370\uC774\uD2B8",
    unchanged: "\uBCC0\uD558\uC9C0 \uC54A\uC740", issues: "\uBB38\uC81C", sheetColumn: "\uC2DC\uD2B8", row: "\uC5F4", column: "\uC5F4", code: "\uC554\uD638", message: "\uBA54\uC2DC\uC9C0",
    target: {
        part_number: "\uBD80\uD488 \uBC88\uD638", product_name: "Product \uC774\uB984", manufacturer_id: "\uC81C\uC870\uC5C5\uCCB4",
        brand_id: "\uC0C1\uD45C", category_id: "\uBC94\uC8FC", package_form_factor: "\uD328\uD0A4\uC9C0/\uD3FC \uD329\uD130",
        description: "\uC124\uBA85", features: "\uD2B9\uC9D5", specification: "\uC0AC\uC591", lifecycle_id: "\uC218\uBA85\uC8FC\uAE30", application_ids: "\uC751\uC6A9",
        source_locale: "Source Locale", name_source_locale: "\uC774\uB984 Source Locale",
        description_source_locale: "\uC124\uBA85 Source Locale", features_source_locale: "\uD2B9\uC9D5 Source Locale",
        specification_source_locale: "\uC0AC\uC591 Source Locale",
    } as Record<string, string>,
},
"de-DE": {
    accepted: (id: string) => `Import${id}wurde zur Validierung angenommen.`,
    committed: (created: number, updated: number, unchanged: number) => `Import festgeschrieben:${created}erstellt,${updated}aktualisiert,${unchanged}unver\u00E4ndert.`,
    cancelRequested: (id: string) => `Stornierung f\u00FCr den Import beantragt${id}.`,
    title: "Atomarer XLSX-Import",
    chooseFileFirst: "W\u00E4hlen Sie eine XLSX-Datei aus, bevor Sie mit dem Import beginnen.",
    description: "Die Validierung scannt jede Zeile und erstellt den vollst\u00E4ndigen Fehlerbericht. Commit \u00FCberpr\u00FCft Identit\u00E4t, Referenzen und Revisionen noch einmal vor einer Alles-oder-Nichts-Transaktion.",
    uploadMapping: "Hochladen und Mapping", workbook: "Arbeitsbuch", choose: "W\u00E4hlen Sie .xlsx", sheet: "Blatt",
    sheetPlaceholder: "Erstes Blatt, wenn es leer ist", headerRow: "Kopfzeile", identityMode: "Identit\u00E4tsmodus",
    sourceLocaleOverride: "Importieren Sie die Source Locale-\u00DCberschreibung",
    sourceLocaleHelp: "Optionaler expliziter Fallback f\u00FCr diesen Vorgang. Ansonsten wird f\u00FCr neue Inhalte das aktuelle Published Website Site Default verwendet. Lokalisierungsspalten pro Zeile und pro Feld haben Vorrang.",
    partNumber: "Teilenummer", manufacturerPart: "Hersteller + Teilenummer", columnIndex: "Spaltenindex",
    headerName: "Headername", productField: "Product-Feld", remove: "Entfernen", addMapping: "Zuordnung hinzuf\u00FCgen",
    validate: "Arbeitsmappe validieren", importTitle: (id: string) => `Import${id}`, refresh: "Aktualisieren", cancel: "Stornieren",
    commit: "Atomar begehen", download: "Download-Fehler CSV", rows: "Reihen", create: "erstellen", update: "aktualisieren",
    unchanged: "unver\u00E4ndert", issues: "Probleme", sheetColumn: "Blatt", row: "Reihe", column: "Spalte", code: "Code", message: "Nachricht",
    target: {
        part_number: "Teilenummer", product_name: "Product-Name", manufacturer_id: "Hersteller",
        brand_id: "Marke", category_id: "Kategorie", package_form_factor: "Paket/Formfaktor",
        description: "Beschreibung", features: "Merkmale", specification: "Spezifikation", lifecycle_id: "Lebenszyklus", application_ids: "Anwendungen",
        source_locale: "Source Locale", name_source_locale: "Name Source Locale",
        description_source_locale: "Beschreibung Source Locale", features_source_locale: "Merkmale Source Locale",
        specification_source_locale: "Spezifikation Source Locale",
    } as Record<string, string>,
},
"fr-FR": {
    accepted: (id: string) => `Importer${id}a \u00E9t\u00E9 accept\u00E9 pour validation.`,
    committed: (created: number, updated: number, unchanged: number) => `Importation valid\u00E9e\u00A0:${created}cr\u00E9\u00E9,${updated}mis \u00E0 jour,${unchanged}inchang\u00E9.`,
    cancelRequested: (id: string) => `Annulation demand\u00E9e pour l'importation${id}.`,
    title: "Importation atomique XLSX",
    chooseFileFirst: "Choisissez un fichier .xlsx avant de lancer l'importation.",
    description: "La validation analyse chaque ligne et produit le rapport d'erreur complet. Commit rev\u00E9rifie l'identit\u00E9, les r\u00E9f\u00E9rences et les r\u00E9visions avant une transaction tout ou rien.",
    uploadMapping: "T\u00E9l\u00E9chargement et mappage", workbook: "Cahier d'exercices", choose: "Choisissez .xlsx", sheet: "Feuille",
    sheetPlaceholder: "Premi\u00E8re feuille vierge", headerRow: "Ligne d'en-t\u00EAte", identityMode: "Mode identit\u00E9",
    sourceLocaleOverride: "Importer le remplacement Source Locale",
    sourceLocaleHelp: "Repli explicite facultatif pour cette op\u00E9ration. Sinon, le nouveau contenu utilise le Published Website Site Default actuel. Les colonnes de param\u00E8tres r\u00E9gionaux par ligne et par champ sont prioritaires.",
    partNumber: "Num\u00E9ro de pi\u00E8ce", manufacturerPart: "Fabricant + num\u00E9ro de pi\u00E8ce", columnIndex: "Index de colonne",
    headerName: "Nom de l'en-t\u00EAte", productField: "Champ Product", remove: "Retirer", addMapping: "Ajouter un mappage",
    validate: "Valider le classeur", importTitle: (id: string) => `Importer${id}`, refresh: "Rafra\u00EEchir", cancel: "Annuler",
    commit: "S'engager atomiquement", download: "Erreur de t\u00E9l\u00E9chargement CSV", rows: "lignes", create: "cr\u00E9er", update: "mise \u00E0 jour",
    unchanged: "inchang\u00E9", issues: "probl\u00E8mes", sheetColumn: "Feuille", row: "Rang\u00E9e", column: "Colonne", code: "Code", message: "Message",
    target: {
        part_number: "Num\u00E9ro de pi\u00E8ce", product_name: "Nom Product", manufacturer_id: "Fabricant",
        brand_id: "Marque", category_id: "Cat\u00E9gorie", package_form_factor: "Emballage/facteur de forme",
        description: "Description", features: "Caract\u00E9ristiques", specification: "Sp\u00E9cification", lifecycle_id: "Cycle de vie", application_ids: "Applications",
        source_locale: "Source Locale", name_source_locale: "Pr\u00E9nom Source Locale",
        description_source_locale: "Description Source Locale", features_source_locale: "Caract\u00E9ristiques Source Locale",
        specification_source_locale: "Sp\u00E9cification Source Locale",
    } as Record<string, string>,
},
"it-IT": {
    accepted: (id: string) => `Importare${id}\u00E8 stato accettato per la convalida.`,
    committed: (created: number, updated: number, unchanged: number) => `Importazione impegnata:${created}creato,${updated}aggiornato,${unchanged}invariato.`,
    cancelRequested: (id: string) => `Richiesto annullamento per l'importazione${id}.`,
    title: "Importazione atomica XLSX",
    chooseFileFirst: "Scegli un file .xlsx prima di iniziare l'importazione.",
    description: "La convalida esegue la scansione di ogni riga e produce il rapporto errori completo. Il commit ricontrolla identit\u00E0, riferimenti e revisioni prima di una transazione tutto o niente.",
    uploadMapping: "Caricamento e mappatura", workbook: "Cartella di lavoro", choose: "Scegli .xlsx", sheet: "Foglio",
    sheetPlaceholder: "Primo foglio vuoto", headerRow: "Riga di intestazione", identityMode: "Modalit\u00E0 identit\u00E0",
    sourceLocaleOverride: "Importa la sostituzione Source Locale",
    sourceLocaleHelp: "Fallback esplicito facoltativo per questa operazione. Altrimenti i nuovi contenuti utilizzano attualmente Published Website Site Default. Le colonne delle impostazioni locali per riga e per campo hanno la precedenza.",
    partNumber: "Numero di parte", manufacturerPart: "Produttore + codice articolo", columnIndex: "Indice delle colonne",
    headerName: "Nome dell'intestazione", productField: "Campo Product", remove: "Rimuovere", addMapping: "Aggiungi mappatura",
    validate: "Convalidare la cartella di lavoro", importTitle: (id: string) => `Importare${id}`, refresh: "Aggiorna", cancel: "Cancellare",
    commit: "Impegnarsi atomicamente", download: "Errore di download CSV", rows: "righe", create: "creare", update: "aggiornamento",
    unchanged: "invariato", issues: "problemi", sheetColumn: "Foglio", row: "Riga", column: "Colonna", code: "Codice", message: "Messaggio",
    target: {
        part_number: "Numero di parte", product_name: "Nome Product", manufacturer_id: "Produttore",
        brand_id: "Marca", category_id: "Categoria", package_form_factor: "Pacchetto/fattore di forma",
        description: "Descrizione", features: "Caratteristiche", specification: "Specifica", lifecycle_id: "Ciclo vitale", application_ids: "Applicazioni",
        source_locale: "Source Locale", name_source_locale: "Nome Source Locale",
        description_source_locale: "DescrizioneSource Locale", features_source_locale: "Caratteristiche Source Locale",
        specification_source_locale: "Specifica Source Locale",
    } as Record<string, string>,
},
"es-ES": {
    accepted: (id: string) => `Importar${id}fue aceptado para su validaci\u00F3n.`,
    committed: (created: number, updated: number, unchanged: number) => `Importaci\u00F3n comprometida:${created}creado,${updated}actualizado,${unchanged}sin alterar.`,
    cancelRequested: (id: string) => `Cancelaci\u00F3n solicitada para importaci\u00F3n${id}.`,
    title: "Importaci\u00F3n at\u00F3mica XLSX",
    chooseFileFirst: "Elija un archivo .xlsx antes de iniciar la importaci\u00F3n.",
    description: "La validaci\u00F3n escanea cada fila y produce el informe de error completo. La confirmaci\u00F3n vuelve a verificar la identidad, las referencias y las revisiones antes de una transacci\u00F3n de todo o nada.",
    uploadMapping: "Carga y mapeo", workbook: "Libro de trabajo", choose: "Elija .xlsx", sheet: "Hoja",
    sheetPlaceholder: "Primera hoja cuando est\u00E1 en blanco", headerRow: "fila de encabezado", identityMode: "Modo de identidad",
    sourceLocaleOverride: "Importar anulaci\u00F3n de Source Locale",
    sourceLocaleHelp: "Reserva expl\u00EDcita opcional para esta operaci\u00F3n. De lo contrario, el contenido nuevo utiliza el Published Website Site Default actual. Las columnas de configuraci\u00F3n regional por fila y por campo tienen prioridad.",
    partNumber: "N\u00FAmero de pieza", manufacturerPart: "Fabricante + n\u00FAmero de pieza", columnIndex: "\u00CDndice de columnas",
    headerName: "Nombre del encabezado", productField: "Campo Product", remove: "Eliminar", addMapping: "Agregar mapeo",
    validate: "Validar libro de trabajo", importTitle: (id: string) => `Importar${id}`, refresh: "Refrescar", cancel: "Cancelar",
    commit: "Comprometerse at\u00F3micamente", download: "Error de descarga CSV", rows: "filas", create: "crear", update: "actualizar",
    unchanged: "sin alterar", issues: "asuntos", sheetColumn: "Hoja", row: "Fila", column: "Columna", code: "C\u00F3digo", message: "Mensaje",
    target: {
        part_number: "N\u00FAmero de pieza", product_name: "Nombre Product", manufacturer_id: "Fabricante",
        brand_id: "Marca", category_id: "Categor\u00EDa", package_form_factor: "Paquete/factor de forma",
        description: "Descripci\u00F3n", features: "Caracter\u00EDsticas", specification: "Especificaci\u00F3n", lifecycle_id: "Ciclo vital", application_ids: "Aplicaciones",
        source_locale: "Source Locale", name_source_locale: "Nombre Source Locale",
        description_source_locale: "Descripci\u00F3n Source Locale", features_source_locale: "Caracter\u00EDsticas Source Locale",
        specification_source_locale: "Especificaci\u00F3n Source Locale",
    } as Record<string, string>,
},
"pt-BR": {
    accepted: (id: string) => `Importar${id}foi aceito para valida\u00E7\u00E3o.`,
    committed: (created: number, updated: number, unchanged: number) => `Importa\u00E7\u00E3o confirmada:${created}criado,${updated}atualizado,${unchanged}inalterado.`,
    cancelRequested: (id: string) => `Cancelamento solicitado para importa\u00E7\u00E3o${id}.`,
    title: "Importa\u00E7\u00E3o at\u00F4mica XLSX",
    chooseFileFirst: "Escolha um arquivo .xlsx antes de iniciar a importa\u00E7\u00E3o.",
    description: "A valida\u00E7\u00E3o verifica cada linha e produz o relat\u00F3rio de erros completo. O commit verifica novamente a identidade, as refer\u00EAncias e as revis\u00F5es antes de uma transa\u00E7\u00E3o do tipo tudo ou nada.",
    uploadMapping: "Carregar e mapear", workbook: "Pasta de trabalho", choose: "Escolha .xlsx", sheet: "Folha",
    sheetPlaceholder: "Primeira folha quando em branco", headerRow: "Linha de cabe\u00E7alho", identityMode: "Modo de identidade",
    sourceLocaleOverride: "Substitui\u00E7\u00E3o de importa\u00E7\u00E3o Source Locale",
    sourceLocaleHelp: "Fallback expl\u00EDcito opcional para esta opera\u00E7\u00E3o. Caso contr\u00E1rio, o novo conte\u00FAdo usa o Published Website Site Default atualmente. As colunas de localidade por linha e por campo t\u00EAm preced\u00EAncia.",
    partNumber: "N\u00FAmero da pe\u00E7a", manufacturerPart: "Fabricante + n\u00FAmero da pe\u00E7a", columnIndex: "\u00CDndice de coluna",
    headerName: "Nome do cabe\u00E7alho", productField: "Campo Product", remove: "Remover", addMapping: "Adicionar mapeamento",
    validate: "Validar pasta de trabalho", importTitle: (id: string) => `Importar${id}`, refresh: "Atualizar", cancel: "Cancelar",
    commit: "Comprometer-se atomicamente", download: "Erro de download CSV", rows: "linhas", create: "criar", update: "atualizar",
    unchanged: "inalterado", issues: "problemas", sheetColumn: "Folha", row: "Linha", column: "Coluna", code: "C\u00F3digo", message: "Mensagem",
    target: {
        part_number: "N\u00FAmero da pe\u00E7a", product_name: "Nome Product", manufacturer_id: "Fabricante",
        brand_id: "Marca", category_id: "Categoria", package_form_factor: "Pacote/fator de forma",
        description: "Descri\u00E7\u00E3o", features: "Caracter\u00EDsticas", specification: "Especifica\u00E7\u00E3o", lifecycle_id: "Vida \u00FAtil", application_ids: "Aplicativos",
        source_locale: "Source Locale", name_source_locale: "Nome Source Locale",
        description_source_locale: "Descri\u00E7\u00E3o Source Locale", features_source_locale: "Caracter\u00EDsticas Source Locale",
        specification_source_locale: "Especifica\u00E7\u00E3o Source Locale",
    } as Record<string, string>,
},
};

export function ImportPanel({ locale, onError, onMessage }: Feedback & { locale: ImportLocale }) {
	const text = labels[locale];
	const targetOptions = targets.map((value) => ({ value, label: text.target[value] ?? value }));
  const [file, setFile] = useState<File>();
  const [job, setJob] = useState<ImportJob>();
  const [preview, setPreview] = useState<ImportPreview>();
  const [submitting, setSubmitting] = useState(false);
  const [form] = Form.useForm<ImportForm>();

  const refreshJob = async (jobID = job?.id) => {
    if (!jobID) return;
    try {
      const response = await api<ImportJobResponse>(`/admin/api/imports/${jobID}`);
      setJob(response.job);
      setPreview(response.preview);
    } catch (error) {
      onError(error);
    }
  };

  useEffect(() => {
    if (!job || !activeStatuses.has(job.status)) return;
    const timer = window.setInterval(() => void refreshJob(job.id), 1000);
    return () => window.clearInterval(timer);
  }, [job?.id, job?.status]);

  const upload = async (values: ImportForm) => {
    if (!file) {
      onError(new Error(text.chooseFileFirst));
      return;
    }
    setSubmitting(true);
    try {
      const body = new FormData();
      body.append("template", JSON.stringify({
        sheet: values.sheet || "",
        header_row: values.header_row,
        identity_mode: values.identity_mode,
        source_locale_override: values.source_locale_override,
        mappings: values.mappings,
      }));
      body.append("file", file, file.name);
      const created = await api<ImportJob>("/admin/api/imports", { method: "POST", body });
      setJob(created);
      setPreview(undefined);
			onMessage(text.accepted(created.id));
    } catch (error) {
      onError(error);
    } finally {
      setSubmitting(false);
    }
  };

  const commit = async () => {
    if (!job) return;
    try {
      const receipt = await postJSON<{ created: number; updated: number; no_change: number; replay: boolean }>(
        `/admin/api/imports/${job.id}/commit`,
        {},
      );
		onMessage(text.committed(receipt.created, receipt.updated, receipt.no_change));
      await refreshJob(job.id);
    } catch (error) {
      onError(error);
      await refreshJob(job.id);
    }
  };

  const cancel = async () => {
    if (!job) return;
    try {
      await postJSON<void>(`/admin/api/imports/${job.id}/cancel`, {});
		onMessage(text.cancelRequested(job.id));
      await refreshJob(job.id);
    } catch (error) {
      onError(error);
    }
  };

  const percent = job?.total_rows ? Math.min(100, Math.round((job.checked_rows / job.total_rows) * 100)) : 0;

  return (
    <Space direction="vertical" size="large" className="panel-stack">
      <Alert
        showIcon
        type="info"
        message={text.title}
        description={text.description}
      />
      <Card title={text.uploadMapping}>
        <Form<ImportForm>
          form={form}
          layout="vertical"
          initialValues={{
            header_row: 1,
            identity_mode: "part_number",
            mappings: [
              { source_index: 0, source_name: "Part Number", target: "part_number" },
              { source_index: 1, source_name: "Product Name", target: "product_name" },
            ],
          }}
          onFinish={(values) => void upload(values)}
        >
          <div className="form-grid three-columns">
            <Form.Item label={text.workbook}>
              <Upload
                accept=".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
                maxCount={1}
                beforeUpload={(nextFile) => { setFile(nextFile); return false; }}
                onRemove={() => { setFile(undefined); }}
              >
                <Button>{text.choose}</Button>
              </Upload>
            </Form.Item>
            <Form.Item name="sheet" label={text.sheet}><Input placeholder={text.sheetPlaceholder} /></Form.Item>
            <Form.Item name="header_row" label={text.headerRow} rules={[{ required: true }]}><InputNumber min={1} precision={0} /></Form.Item>
            <Form.Item name="identity_mode" label={text.identityMode} rules={[{ required: true }]}>
              <Select options={[
                { value: "part_number", label: text.partNumber },
                { value: "manufacturer_part_number", label: text.manufacturerPart },
              ]} />
            </Form.Item>
            <Form.Item name="source_locale_override" label={text.sourceLocaleOverride} extra={text.sourceLocaleHelp}>
              <Select allowClear options={localeOptions} />
            </Form.Item>
          </div>
          <Form.List name="mappings">
            {(fields, { add, remove }) => (
              <Space direction="vertical" className="panel-stack">
                {fields.map((field) => (
                  <Space key={field.key} wrap align="baseline">
                    <Form.Item {...field} name={[field.name, "source_index"]} label={text.columnIndex} rules={[{ required: true }]}><InputNumber min={0} precision={0} /></Form.Item>
                    <Form.Item {...field} name={[field.name, "source_name"]} label={text.headerName} rules={[{ required: true }]}><Input /></Form.Item>
                    <Form.Item {...field} name={[field.name, "target"]} label={text.productField} rules={[{ required: true }]}><Select options={targetOptions} style={{ width: 190 }} /></Form.Item>
                    <Button danger onClick={() => remove(field.name)}>{text.remove}</Button>
                  </Space>
                ))}
                <Button onClick={() => add({ source_index: fields.length, source_name: "", target: "" })}>{text.addMapping}</Button>
              </Space>
            )}
          </Form.List>
          <Button type="primary" htmlType="submit" loading={submitting} disabled={!file} className="top-gap">{text.validate}</Button>
        </Form>
      </Card>

      {job && (
        <Card
          title={text.importTitle(job.id)}
          extra={
            <Space>
              <Button onClick={() => void refreshJob()}>{text.refresh}</Button>
              {activeStatuses.has(job.status) && <Button danger onClick={() => void cancel()}>{text.cancel}</Button>}
              {job.status === "preview_ready" && preview?.fully_scanned && preview.fully_validated && (
                <Button type="primary" onClick={() => void commit()}>{text.commit}</Button>
              )}
              {preview && <Button href={`/admin/api/imports/${job.id}/report`} target="_blank">{text.download}</Button>}
            </Space>
          }
        >
          <Space direction="vertical" className="panel-stack">
            <Space wrap>
              <Tag color={job.status === "failed" ? "red" : job.status === "committed" ? "green" : "blue"}>{job.status}</Tag>
              <span>{job.phase}</span>
              <span>{job.original_filename}</span>
            </Space>
            {activeStatuses.has(job.status) && <Progress percent={percent} status="active" />}
            {job.error_message && <Alert type="error" message={job.error_message} showIcon />}
            <Space wrap>
              <Tag>{text.rows} {preview?.total_rows ?? job.total_rows}</Tag>
              <Tag color="green">{text.create} {preview?.create_count ?? job.create_count}</Tag>
              <Tag color="blue">{text.update} {preview?.update_count ?? job.update_count}</Tag>
              <Tag>{text.unchanged} {preview?.no_change_count ?? job.no_change_count}</Tag>
              <Tag color={(preview?.issues?.length ?? job.failed_count) > 0 ? "red" : "default"}>{text.issues} {preview?.issues?.length ?? job.failed_count}</Tag>
            </Space>
            {preview && (
              <Table<ImportIssue>
                rowKey={(issue, index) => `${issue.sheet}-${issue.row}-${issue.column}-${issue.code}-${index}`}
                dataSource={preview.issues ?? []}
                pagination={{ pageSize: 20, hideOnSinglePage: true }}
                columns={[
                  { title: text.sheetColumn, dataIndex: "sheet" },
                  { title: text.row, dataIndex: "row" },
                  { title: text.column, dataIndex: "column" },
                  { title: text.code, dataIndex: "code" },
                  { title: text.message, dataIndex: "message" },
                ]}
              />
            )}
          </Space>
        </Card>
      )}
    </Space>
  );
}
