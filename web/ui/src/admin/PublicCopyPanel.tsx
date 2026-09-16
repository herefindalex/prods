import { useEffect, useMemo, useState } from "react";
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Empty,
  Input,
  Popconfirm,
  Select,
  Space,
  Table,
  Tag,
  Typography,
  Upload,
} from "antd";
import { api, downloadFile, postJSON, putJSON } from "./api";
import { localeSelectOptions, type AdminLocale } from "./locales";
import type {
  PublicCopyDefault,
  PublicCopyDefinition,
  PublicCopyEditorState,
  PublicCopyExchangeChange,
  PublicCopyExchangeIssue,
  PublicCopyExchangePreview,
} from "./types";

type Props = {
  locale: AdminLocale;
  onError: (error: unknown) => void;
  onMessage: (message: string) => void;
};

const labels = {
  "en-US": {
    title: "Public Copy",
    help: "Edit official interface text in the durable Website working copy. Public output does not change until Website Preview and Publish complete.",
    disclaimerTitle: "Translation review status",
    disclaimer: "The ten bundled translations are machine-generated and have not been reviewed by professional native, legal, or marketing reviewers. Review them before production use.",
    workingRevision: "Website working revision",
    locale: "Locale",
    scope: "Scope",
    allScopes: "All scopes",
    search: "Filter by key or description",
    key: "Key",
    description: "Description",
    kind: "Value kind",
    official: "Official default",
    override: "Working override",
    placeholders: "Placeholders",
    sample: "Sample values",
    save: "Save override",
    reset: "Reset this locale",
    resetConfirm: "Reset only this key and locale to the official default?",
    noOverride: "Using official default",
    saved: (key: string, locale: string, revision: number) =>
      `Saved ${key} (${locale}) in Website working revision ${revision}. Preview and Publish are still required.`,
    resetDone: (key: string, locale: string, revision: number) =>
      `Reset ${key} (${locale}) in Website working revision ${revision}. Preview and Publish are still required.`,
    required: "Required",
    allowed: "Allowed",
    none: "None",
    selectEntry: "Select a copy entry to edit.",
    reviewStatus: "Bundle review status",
    bundleVersion: "Official bundle version",
    exchangeTitle: "Translation CSV/XLSX exchange",
    exchangeHelp: "Export a selected locale/scope, edit externally, then validate every row. Import only updates the Website working copy; Preview and Publish remain required. Use action=reset with a blank Value to remove one override.",
    exportCSV: "Export CSV",
    exportXLSX: "Export XLSX",
    chooseExchange: "Choose CSV/XLSX",
    validateExchange: "Validate import",
    commitExchange: "Commit validated changes",
    exchangeDone: (revision: number) => `Translation import committed to Website working revision ${revision}. Preview and Publish are still required.`,
    line: "Line",
    action: "Action",
    before: "Before",
    after: "After",
    code: "Code",
    message: "Message",
    whereUsed: "Where used",
    contextPreview: "Safe context preview",
    desktop: "Desktop",
    mobile: "Mobile",
  },
  "zh-TW": {
    title: "公開介面文案",
    help: "在持久的 Website 工作副本編輯官方介面文字。必須完成 Website 預覽與發布，公開輸出才會改變。",
    disclaimerTitle: "翻譯審閱狀態",
    disclaimer: "內建十種語系為機器產生，尚未經母語、法律或行銷專業人員審閱；正式上線前必須自行審閱。",
    workingRevision: "Website 工作修訂",
    locale: "語系",
    scope: "範圍",
    allScopes: "全部範圍",
    search: "依 key 或描述篩選",
    key: "Key",
    description: "說明",
    kind: "值類型",
    official: "官方預設值",
    override: "工作副本覆寫值",
    placeholders: "佔位符",
    sample: "範例值",
    save: "儲存覆寫",
    reset: "重設此語系",
    resetConfirm: "只將這個 key 與語系重設為官方預設值嗎？",
    noOverride: "目前使用官方預設值",
    saved: (key: string, locale: string, revision: number) =>
      `已將 ${key}（${locale}）儲存到 Website 工作修訂 ${revision}；仍須預覽並發布。`,
    resetDone: (key: string, locale: string, revision: number) =>
      `已在 Website 工作修訂 ${revision} 重設 ${key}（${locale}）；仍須預覽並發布。`,
    required: "必要",
    allowed: "允許",
    none: "無",
    selectEntry: "請選擇一個文案項目進行編輯。",
    reviewStatus: "套件審閱狀態",
    bundleVersion: "官方套件版本",
    exchangeTitle: "翻譯 CSV／XLSX 交換",
    exchangeHelp: "匯出選定語系／範圍，在外部編輯後逐列完整驗證。匯入只更新 Website 工作副本，仍須預覽與發布。要移除單一覆寫時，使用 action=reset 並將 Value 留白。",
    exportCSV: "匯出 CSV",
    exportXLSX: "匯出 XLSX",
    chooseExchange: "選擇 CSV／XLSX",
    validateExchange: "驗證匯入",
    commitExchange: "提交已驗證變更",
    exchangeDone: (revision: number) => `翻譯已提交到 Website 工作修訂 ${revision}；仍須預覽並發布。`,
    line: "列",
    action: "操作",
    before: "原值",
    after: "新值",
    code: "代碼",
    message: "訊息",
    whereUsed: "使用位置",
    contextPreview: "安全情境預覽",
    desktop: "桌面",
    mobile: "行動裝置",
  },
"zh-CN": {
    title: "Public Copy",
    help: "\u5728\u6301\u4E45\u7684Website\u5DE5\u4F5C\u526F\u672C\u4E2D\u7F16\u8F91\u5B98\u65B9\u754C\u9762\u6587\u672C\u3002\u5728 Website\u3001Preview \u548C Publish \u5B8C\u6210\u4E4B\u524D\uFF0C\u516C\u5171\u8F93\u51FA\u4E0D\u4F1A\u66F4\u6539\u3002",
    disclaimerTitle: "\u7FFB\u8BD1\u5BA1\u6838\u200B\u200B\u72B6\u6001",
    disclaimer: "\u5341\u4E2A\u6346\u7ED1\u7FFB\u8BD1\u662F\u673A\u5668\u751F\u6210\u7684\uFF0C\u672A\u7ECF\u4E13\u4E1A\u7684\u672C\u5730\u3001\u6CD5\u5F8B\u6216\u8425\u9500\u5BA1\u9605\u8005\u5BA1\u9605\u3002\u5728\u751F\u4EA7\u4F7F\u7528\u4E4B\u524D\u68C0\u67E5\u5B83\u4EEC\u3002",
    workingRevision: "Website \u5DE5\u4F5C\u4FEE\u8BA2\u7248",
    locale: "\u8BED\u8A00\u73AF\u5883",
    scope: "\u8303\u56F4",
    allScopes: "\u6240\u6709\u8303\u56F4",
    search: "\u6309\u5173\u952E\u5B57\u6216\u63CF\u8FF0\u8FC7\u6EE4",
    key: "\u94A5\u5319",
    description: "\u63CF\u8FF0",
    kind: "\u4EF7\u503C\u79CD\u7C7B",
    official: "\u5B98\u65B9\u9ED8\u8BA4",
    override: "\u5DE5\u4F5C\u500D\u7387",
    placeholders: "\u5360\u4F4D\u7B26",
    sample: "\u6837\u672C\u503C",
    save: "\u4FDD\u5B58\u8986\u76D6",
    reset: "\u91CD\u7F6E\u6B64\u533A\u57DF\u8BBE\u7F6E",
    resetConfirm: "\u4EC5\u5C06\u6B64\u5BC6\u94A5\u548C\u533A\u57DF\u8BBE\u7F6E\u91CD\u7F6E\u4E3A\u5B98\u65B9\u9ED8\u8BA4\u503C\uFF1F",
    noOverride: "\u4F7F\u7528\u5B98\u65B9\u9ED8\u8BA4",
    saved: (key: string, locale: string, revision: number) => `\u5DF2\u4FDD\u5B58${key} (${locale}\uFF09\u5728 Website \u5DE5\u4F5C\u4FEE\u8BA2\u7248\u4E2D${revision}\u3002\u4ECD\u7136\u9700\u8981 Preview \u548C Publish\u3002`,
    resetDone: (key: string, locale: string, revision: number) => `\u91CD\u7F6E${key} (${locale}\uFF09\u5728 Website \u5DE5\u4F5C\u4FEE\u8BA2\u7248\u4E2D${revision}\u3002\u4ECD\u7136\u9700\u8981 Preview \u548C Publish\u3002`,
    required: "\u5FC5\u9700\u7684",
    allowed: "\u5141\u8BB8",
    none: "\u6CA1\u6709\u4EFB\u4F55",
    selectEntry: "\u9009\u62E9\u8981\u7F16\u8F91\u7684\u526F\u672C\u6761\u76EE\u3002",
    reviewStatus: "\u6346\u7ED1\u5305\u5BA1\u6838\u72B6\u6001",
    bundleVersion: "\u5B98\u65B9\u6346\u7ED1\u7248",
    exchangeTitle: "\u7FFB\u8BD1CSV/XLSX\u4EA4\u6362",
    exchangeHelp: "\u5BFC\u51FA\u9009\u5B9A\u7684\u533A\u57DF\u8BBE\u7F6E/\u8303\u56F4\uFF0C\u5728\u5916\u90E8\u8FDB\u884C\u7F16\u8F91\uFF0C\u7136\u540E\u9A8C\u8BC1\u6BCF\u4E00\u884C\u3002\u5BFC\u5165\u4EC5\u66F4\u65B0Website\u5DE5\u4F5C\u526F\u672C\uFF1B Preview \u548C Publish \u4ECD\u7136\u662F\u5FC5\u9700\u7684\u3002\u4F7F\u7528\u5E26\u6709\u7A7A\u767D\u503C\u7684 action=reset \u53EF\u5220\u9664\u4E00\u9879\u8986\u76D6\u3002",
    exportCSV: "\u51FA\u53E3CSV",
    exportXLSX: "\u51FA\u53E3XLSX",
    chooseExchange: "\u9009\u62E9CSV/XLSX",
    validateExchange: "\u9A8C\u8BC1\u5BFC\u5165",
    commitExchange: "\u63D0\u4EA4\u7ECF\u8FC7\u9A8C\u8BC1\u7684\u66F4\u6539",
    exchangeDone: (revision: number) => `\u7FFB\u8BD1\u5BFC\u5165\u81F4\u529B\u4E8EWebsite\u5DE5\u4F5C\u4FEE\u8BA2${revision}\u3002\u4ECD\u7136\u9700\u8981 Preview \u548C Publish\u3002`,
    line: "\u7EBF",
    action: "\u884C\u52A8",
    before: "\u524D",
    after: "\u540E",
    code: "\u4EE3\u7801",
    message: "\u4FE1\u606F",
    whereUsed: "\u4F7F\u7528\u5730\u70B9",
    contextPreview: "\u5B89\u5168\u4E0A\u4E0B\u6587\u9884\u89C8",
    desktop: "\u684C\u9762",
    mobile: "\u79FB\u52A8\u7684",
},
"ja-JP": {
    title: "Public Copy",
    help: "\u8010\u4E45\u6027\u306E\u3042\u308B Website \u4F5C\u696D\u30B3\u30D4\u30FC\u3067\u516C\u5F0F\u30A4\u30F3\u30BF\u30FC\u30D5\u30A7\u30A4\u30B9\u306E\u30C6\u30AD\u30B9\u30C8\u3092\u7DE8\u96C6\u3057\u307E\u3059\u3002\u30D1\u30D6\u30EA\u30C3\u30AF\u51FA\u529B\u306F\u3001Website Preview \u304A\u3088\u3073 Publish \u304C\u5B8C\u4E86\u3059\u308B\u307E\u3067\u5909\u66F4\u3055\u308C\u307E\u305B\u3093\u3002",
    disclaimerTitle: "\u7FFB\u8A33\u30EC\u30D3\u30E5\u30FC\u306E\u30B9\u30C6\u30FC\u30BF\u30B9",
    disclaimer: "10 \u500B\u306E\u30D0\u30F3\u30C9\u30EB\u3055\u308C\u305F\u7FFB\u8A33\u306F\u6A5F\u68B0\u306B\u3088\u3063\u3066\u751F\u6210\u3055\u308C\u3066\u304A\u308A\u3001\u5C02\u9580\u306E\u30CD\u30A4\u30C6\u30A3\u30D6\u3001\u6CD5\u5F8B\u3001\u307E\u305F\u306F\u30DE\u30FC\u30B1\u30C6\u30A3\u30F3\u30B0\u306E\u30EC\u30D3\u30E5\u30FC\u62C5\u5F53\u8005\u306B\u3088\u308B\u30EC\u30D3\u30E5\u30FC\u306F\u53D7\u3051\u3066\u3044\u307E\u305B\u3093\u3002\u904B\u7528\u74B0\u5883\u3067\u4F7F\u7528\u3059\u308B\u524D\u306B\u78BA\u8A8D\u3057\u3066\u304F\u3060\u3055\u3044\u3002",
    workingRevision: "Website \u4F5C\u696D\u30EA\u30D3\u30B8\u30E7\u30F3",
    locale: "\u30ED\u30B1\u30FC\u30EB",
    scope: "\u7BC4\u56F2",
    allScopes: "\u3059\u3079\u3066\u306E\u30B9\u30B3\u30FC\u30D7",
    search: "\u30AD\u30FC\u307E\u305F\u306F\u8AAC\u660E\u306B\u3088\u308B\u30D5\u30A3\u30EB\u30BF\u30FC",
    key: "\u9375",
    description: "\u8AAC\u660E",
    kind: "\u5024\u306E\u7A2E\u985E",
    official: "\u516C\u5F0F\u30C7\u30D5\u30A9\u30EB\u30C8",
    override: "\u4F5C\u696D\u30AA\u30FC\u30D0\u30FC\u30E9\u30A4\u30C9",
    placeholders: "\u30D7\u30EC\u30FC\u30B9\u30DB\u30EB\u30C0\u30FC",
    sample: "\u30B5\u30F3\u30D7\u30EB\u5024",
    save: "\u30AA\u30FC\u30D0\u30FC\u30E9\u30A4\u30C9\u3092\u4FDD\u5B58",
    reset: "\u3053\u306E\u30ED\u30B1\u30FC\u30EB\u3092\u30EA\u30BB\u30C3\u30C8\u3059\u308B",
    resetConfirm: "\u3053\u306E\u30AD\u30FC\u3068\u30ED\u30B1\u30FC\u30EB\u306E\u307F\u3092\u516C\u5F0F\u306E\u30C7\u30D5\u30A9\u30EB\u30C8\u306B\u30EA\u30BB\u30C3\u30C8\u3057\u307E\u3059\u304B?",
    noOverride: "\u516C\u5F0F\u30C7\u30D5\u30A9\u30EB\u30C8\u3092\u4F7F\u7528\u3059\u308B",
    saved: (key: string, locale: string, revision: number) => `\u4FDD\u5B58\u3055\u308C\u307E\u3057\u305F${key} (${locale}) Website \u4F5C\u696D\u30EA\u30D3\u30B8\u30E7\u30F3${revision}\u3002 Preview \u304A\u3088\u3073 Publish \u306F\u5F15\u304D\u7D9A\u304D\u5FC5\u8981\u3067\u3059\u3002`,
    resetDone: (key: string, locale: string, revision: number) => `\u30EA\u30BB\u30C3\u30C8${key} (${locale}) Website \u4F5C\u696D\u30EA\u30D3\u30B8\u30E7\u30F3${revision}\u3002 Preview \u304A\u3088\u3073 Publish \u306F\u5F15\u304D\u7D9A\u304D\u5FC5\u8981\u3067\u3059\u3002`,
    required: "\u5FC5\u9808",
    allowed: "\u8A31\u53EF\u3055\u308C\u305F",
    none: "\u306A\u3057",
    selectEntry: "\u7DE8\u96C6\u3059\u308B\u30B3\u30D4\u30FC \u30A8\u30F3\u30C8\u30EA\u3092\u9078\u629E\u3057\u307E\u3059\u3002",
    reviewStatus: "\u30D0\u30F3\u30C9\u30EB\u306E\u30EC\u30D3\u30E5\u30FC\u30B9\u30C6\u30FC\u30BF\u30B9",
    bundleVersion: "\u516C\u5F0F\u30D0\u30F3\u30C9\u30EB\u7248",
    exchangeTitle: "\u7FFB\u8A33 CSV/XLSX \u4EA4\u63DB",
    exchangeHelp: "\u9078\u629E\u3057\u305F\u30ED\u30B1\u30FC\u30EB/\u30B9\u30B3\u30FC\u30D7\u3092\u30A8\u30AF\u30B9\u30DD\u30FC\u30C8\u3057\u3001\u5916\u90E8\u3067\u7DE8\u96C6\u3057\u3066\u304B\u3089\u3001\u3059\u3079\u3066\u306E\u884C\u3092\u691C\u8A3C\u3057\u307E\u3059\u3002\u30A4\u30F3\u30DD\u30FC\u30C8\u3067\u306F\u3001Website \u4F5C\u696D\u30B3\u30D4\u30FC\u306E\u307F\u304C\u66F4\u65B0\u3055\u308C\u307E\u3059\u3002 Preview \u304A\u3088\u3073 Publish \u306F\u5F15\u304D\u7D9A\u304D\u5FC5\u8981\u3067\u3059\u3002 1 \u3064\u306E\u30AA\u30FC\u30D0\u30FC\u30E9\u30A4\u30C9\u3092\u524A\u9664\u3059\u308B\u306B\u306F\u3001\u5024\u3092\u7A7A\u767D\u306B\u3057\u3066 action=reset \u3092\u4F7F\u7528\u3057\u307E\u3059\u3002",
    exportCSV: "CSV\u3092\u30A8\u30AF\u30B9\u30DD\u30FC\u30C8",
    exportXLSX: "XLSX\u3092\u30A8\u30AF\u30B9\u30DD\u30FC\u30C8",
    chooseExchange: "CSV/XLSX\u3092\u9078\u629E\u3057\u3066\u304F\u3060\u3055\u3044",
    validateExchange: "\u30A4\u30F3\u30DD\u30FC\u30C8\u3092\u691C\u8A3C\u3059\u308B",
    commitExchange: "\u691C\u8A3C\u3055\u308C\u305F\u5909\u66F4\u3092\u30B3\u30DF\u30C3\u30C8\u3059\u308B",
    exchangeDone: (revision: number) => `\u7FFB\u8A33\u30A4\u30F3\u30DD\u30FC\u30C8\u306F Website \u4F5C\u696D\u30EA\u30D3\u30B8\u30E7\u30F3\u306B\u30B3\u30DF\u30C3\u30C8\u3055\u308C\u307E\u3057\u305F${revision}\u3002 Preview \u304A\u3088\u3073 Publish \u306F\u5F15\u304D\u7D9A\u304D\u5FC5\u8981\u3067\u3059\u3002`,
    line: "\u30E9\u30A4\u30F3",
    action: "\u30A2\u30AF\u30B7\u30E7\u30F3",
    before: "\u524D\u306B",
    after: "\u5F8C",
    code: "\u30B3\u30FC\u30C9",
    message: "\u30E1\u30C3\u30BB\u30FC\u30B8",
    whereUsed: "\u4F7F\u7528\u3055\u308C\u308B\u5834\u6240",
    contextPreview: "\u5B89\u5168\u306A\u30B3\u30F3\u30C6\u30AD\u30B9\u30C8\u306E\u30D7\u30EC\u30D3\u30E5\u30FC",
    desktop: "\u30C7\u30B9\u30AF\u30C8\u30C3\u30D7",
    mobile: "\u643A\u5E2F",
},
"ko-KR": {
    title: "Public Copy",
    help: "\uB0B4\uAD6C\uC131\uC774 \uB6F0\uC5B4\uB09C Website \uC791\uC5C5 \uBCF5\uC0AC\uBCF8\uC5D0\uC11C \uACF5\uC2DD \uC778\uD130\uD398\uC774\uC2A4 \uD14D\uC2A4\uD2B8\uB97C \uD3B8\uC9D1\uD558\uC138\uC694. Website Preview \uBC0F Publish\uAC00 \uC644\uB8CC\uB420 \uB54C\uAE4C\uC9C0 \uACF5\uAC1C \uCD9C\uB825\uC740 \uBCC0\uACBD\uB418\uC9C0 \uC54A\uC2B5\uB2C8\uB2E4.",
    disclaimerTitle: "\uBC88\uC5ED \uAC80\uD1A0 \uC0C1\uD0DC",
    disclaimer: "10\uAC1C\uC758 \uBC88\uB4E4 \uBC88\uC5ED\uC740 \uAE30\uACC4\uB85C \uC0DD\uC131\uB418\uC5C8\uC73C\uBA70 \uC804\uBB38\uC801\uC778 \uC6D0\uC5B4\uBBFC, \uBC95\uB960 \uB610\uB294 \uB9C8\uCF00\uD305 \uAC80\uD1A0\uC790\uC758 \uAC80\uD1A0\uB97C \uAC70\uCE58\uC9C0 \uC54A\uC558\uC2B5\uB2C8\uB2E4. \uD504\uB85C\uB355\uC158\uC5D0 \uC0AC\uC6A9\uD558\uAE30 \uC804\uC5D0 \uAC80\uD1A0\uD558\uC138\uC694.",
    workingRevision: "Website \uC791\uC5C5 \uAC1C\uC815\uD310",
    locale: "\uC7A5\uC18C",
    scope: "\uBC94\uC704",
    allScopes: "\uBAA8\uB4E0 \uBC94\uC704",
    search: "\uD0A4 \uB610\uB294 \uC124\uBA85\uC73C\uB85C \uD544\uD130\uB9C1",
    key: "\uC5F4\uC1E0",
    description: "\uC124\uBA85",
    kind: "\uAC00\uCE58 \uC885\uB958",
    official: "\uACF5\uC2DD \uAE30\uBCF8\uAC12",
    override: "\uC791\uC5C5 \uC7AC\uC815\uC758",
    placeholders: "\uC790\uB9AC\uD45C\uC2DC\uC790",
    sample: "\uC0D8\uD50C \uAC12",
    save: "\uC7AC\uC815\uC758 \uC800\uC7A5",
    reset: "\uC774 \uB85C\uCE98 \uC7AC\uC124\uC815",
    resetConfirm: "\uC774 \uD0A4\uC640 \uB85C\uCE98\uB9CC \uACF5\uC2DD \uAE30\uBCF8\uAC12\uC73C\uB85C \uC7AC\uC124\uC815\uD558\uC2DC\uACA0\uC2B5\uB2C8\uAE4C?",
    noOverride: "\uACF5\uC2DD \uAE30\uBCF8\uAC12 \uC0AC\uC6A9",
    saved: (key: string, locale: string, revision: number) => `\uC800\uC7A5\uB428${key} (${locale}) Website \uC791\uC5C5 \uAC1C\uC815\uC5D0\uC11C${revision}. Preview \uBC0F Publish\uB294 \uC5EC\uC804\uD788 \uD544\uC694\uD569\uB2C8\uB2E4.`,
    resetDone: (key: string, locale: string, revision: number) => `\uB2E4\uC2DC \uB193\uAE30${key} (${locale}) Website \uC791\uC5C5 \uAC1C\uC815\uC5D0\uC11C${revision}. Preview \uBC0F Publish\uB294 \uC5EC\uC804\uD788 \uD544\uC694\uD569\uB2C8\uB2E4.`,
    required: "\uD544\uC218\uC758",
    allowed: "\uD5C8\uC6A9\uB41C",
    none: "\uC5C6\uC74C",
    selectEntry: "\uD3B8\uC9D1\uD560 \uBCF5\uC0AC \uD56D\uBAA9\uC744 \uC120\uD0DD\uD569\uB2C8\uB2E4.",
    reviewStatus: "\uBC88\uB4E4 \uAC80\uD1A0 \uC0C1\uD0DC",
    bundleVersion: "\uACF5\uC2DD \uBC88\uB4E4 \uBC84\uC804",
    exchangeTitle: "\uBC88\uC5ED CSV/XLSX \uAD50\uD658",
    exchangeHelp: "\uC120\uD0DD\uD55C \uB85C\uCE98/\uBC94\uC704\uB97C \uB0B4\uBCF4\uB0B4\uACE0 \uC678\uBD80\uC5D0\uC11C \uD3B8\uC9D1\uD55C \uB2E4\uC74C \uBAA8\uB4E0 \uD589\uC758 \uC720\uD6A8\uC131\uC744 \uAC80\uC0AC\uD569\uB2C8\uB2E4. \uAC00\uC838\uC624\uAE30\uB294 Website \uC791\uC5C5 \uBCF5\uC0AC\uBCF8\uB9CC \uC5C5\uB370\uC774\uD2B8\uD569\uB2C8\uB2E4. Preview \uBC0F Publish\uB294 \uC5EC\uC804\uD788 \uD544\uC218\uC785\uB2C8\uB2E4. \uD558\uB098\uC758 \uC7AC\uC815\uC758\uB97C \uC81C\uAC70\uD558\uB824\uBA74 \uBE48 \uAC12\uACFC \uD568\uAED8 action=reset\uC744 \uC0AC\uC6A9\uD558\uC138\uC694.",
    exportCSV: "CSV \uB0B4\uBCF4\uB0B4\uAE30",
    exportXLSX: "XLSX \uB0B4\uBCF4\uB0B4\uAE30",
    chooseExchange: "CSV/XLSX\uB97C \uC120\uD0DD\uD558\uC138\uC694",
    validateExchange: "\uAC00\uC838\uC624\uAE30 \uAC80\uC99D",
    commitExchange: "\uAC80\uC99D\uB41C \uBCC0\uACBD \uC0AC\uD56D \uCEE4\uBC0B",
    exchangeDone: (revision: number) => `Website \uC791\uC5C5 \uAC1C\uC815\uC5D0 \uB300\uD55C \uBC88\uC5ED \uAC00\uC838\uC624\uAE30\uAC00 \uC644\uB8CC\uB418\uC5C8\uC2B5\uB2C8\uB2E4.${revision}. Preview \uBC0F Publish\uB294 \uC5EC\uC804\uD788 \uD544\uC694\uD569\uB2C8\uB2E4.`,
    line: "\uC120",
    action: "\uD589\uB3D9",
    before: "\uC804\uC5D0",
    after: "\uD6C4\uC5D0",
    code: "\uC554\uD638",
    message: "\uBA54\uC2DC\uC9C0",
    whereUsed: "\uC0AC\uC6A9\uCC98",
    contextPreview: "\uC548\uC804\uD55C \uCEE8\uD14D\uC2A4\uD2B8 \uBBF8\uB9AC\uBCF4\uAE30",
    desktop: "\uB370\uC2A4\uD06C\uD0D1",
    mobile: "\uC774\uB3D9\uD558\uB294",
},
"de-DE": {
    title: "Public Copy",
    help: "Bearbeiten Sie den offiziellen Schnittstellentext in der dauerhaften Website-Arbeitskopie. Die \u00F6ffentliche Ausgabe \u00E4ndert sich erst, wenn Website Preview und Publish abgeschlossen sind.",
    disclaimerTitle: "Status der \u00DCbersetzungs\u00FCberpr\u00FCfung",
    disclaimer: "Die zehn geb\u00FCndelten \u00DCbersetzungen werden maschinell erstellt und nicht von professionellen muttersprachlichen, juristischen oder Marketing-Rezensenten \u00FCberpr\u00FCft. \u00DCberpr\u00FCfen Sie sie vor dem Produktionseinsatz.",
    workingRevision: "Website Arbeitsrevision",
    locale: "Gebietsschema",
    scope: "Umfang",
    allScopes: "Alle Bereiche",
    search: "Filtern Sie nach Schl\u00FCssel oder Beschreibung",
    key: "Schl\u00FCssel",
    description: "Beschreibung",
    kind: "Wertvolle Art",
    official: "Offizieller Standard",
    override: "Arbeits\u00FCberbr\u00FCckung",
    placeholders: "Platzhalter",
    sample: "Beispielwerte",
    save: "Au\u00DFerkraftsetzung speichern",
    reset: "Setzen Sie dieses Gebietsschema zur\u00FCck",
    resetConfirm: "Nur diesen Schl\u00FCssel und dieses Gebietsschema auf die offizielle Standardeinstellung zur\u00FCcksetzen?",
    noOverride: "Verwendung der offiziellen Standardeinstellung",
    saved: (key: string, locale: string, revision: number) => `Gespeichert${key} (${locale}) in der Arbeitsrevision Website${revision}. Preview und Publish sind weiterhin erforderlich.`,
    resetDone: (key: string, locale: string, revision: number) => `Zur\u00FCcksetzen${key} (${locale}) in der Arbeitsrevision Website${revision}. Preview und Publish sind weiterhin erforderlich.`,
    required: "Erforderlich",
    allowed: "Erlaubt",
    none: "Keiner",
    selectEntry: "W\u00E4hlen Sie einen Kopiereintrag zum Bearbeiten aus.",
    reviewStatus: "Bundle-\u00DCberpr\u00FCfungsstatus",
    bundleVersion: "Offizielle Bundle-Version",
    exchangeTitle: "\u00DCbersetzung CSV/XLSX Austausch",
    exchangeHelp: "Exportieren Sie ein ausgew\u00E4hltes Gebietsschema/einen ausgew\u00E4hlten Bereich, bearbeiten Sie es extern und validieren Sie dann jede Zeile. Beim Import wird nur die Website-Arbeitskopie aktualisiert. Preview und Publish bleiben erforderlich. Verwenden Sie action=reset mit einem leeren Wert, um eine \u00DCberschreibung zu entfernen.",
    exportCSV: "Exportieren Sie CSV",
    exportXLSX: "Exportieren Sie XLSX",
    chooseExchange: "W\u00E4hlen Sie CSV/XLSX",
    validateExchange: "Import validieren",
    commitExchange: "\u00DCbernehmen Sie validierte \u00C4nderungen",
    exchangeDone: (revision: number) => `Der \u00DCbersetzungsimport wurde an die Arbeitsrevision Website \u00FCbergeben${revision}. Preview und Publish sind weiterhin erforderlich.`,
    line: "Linie",
    action: "Aktion",
    before: "Vor",
    after: "Nach",
    code: "Code",
    message: "Nachricht",
    whereUsed: "Wo verwendet",
    contextPreview: "Sichere Kontextvorschau",
    desktop: "Desktop",
    mobile: "Mobile",
},
"fr-FR": {
    title: "Public Copy",
    help: "Modifiez le texte officiel de l'interface dans la copie de travail durable Website. La sortie publique ne change pas tant que Website, Preview et Publish ne sont pas termin\u00E9s.",
    disclaimerTitle: "Statut de r\u00E9vision de la traduction",
    disclaimer: "Les dix traductions group\u00E9es sont g\u00E9n\u00E9r\u00E9es automatiquement et n\u2019ont pas \u00E9t\u00E9 r\u00E9vis\u00E9es par des r\u00E9viseurs professionnels natifs, juridiques ou marketing. Examinez-les avant leur utilisation en production.",
    workingRevision: "R\u00E9vision de travail Website",
    locale: "Lieu",
    scope: "Port\u00E9e",
    allScopes: "Toutes les port\u00E9es",
    search: "Filtrer par cl\u00E9 ou description",
    key: "Cl\u00E9",
    description: "Description",
    kind: "Type de valeur",
    official: "Par d\u00E9faut officiel",
    override: "Remplacement de travail",
    placeholders: "Espaces r\u00E9serv\u00E9s",
    sample: "Exemples de valeurs",
    save: "Enregistrer le remplacement",
    reset: "R\u00E9initialiser ces param\u00E8tres r\u00E9gionaux",
    resetConfirm: "R\u00E9initialiser uniquement cette cl\u00E9 et ces param\u00E8tres r\u00E9gionaux aux valeurs par d\u00E9faut officielles\u00A0?",
    noOverride: "Utilisation des valeurs par d\u00E9faut officielles",
    saved: (key: string, locale: string, revision: number) => `Enregistr\u00E9${key} (${locale}) dans la r\u00E9vision de travail Website${revision}. Preview et Publish sont toujours requis.`,
    resetDone: (key: string, locale: string, revision: number) => `R\u00E9initialiser${key} (${locale}) dans la r\u00E9vision de travail Website${revision}. Preview et Publish sont toujours requis.`,
    required: "Requis",
    allowed: "Autoris\u00E9",
    none: "Aucun",
    selectEntry: "S\u00E9lectionnez une entr\u00E9e de copie \u00E0 modifier.",
    reviewStatus: "\u00C9tat de l'examen du lot",
    bundleVersion: "Version group\u00E9e officielle",
    exchangeTitle: "Traduction \u00E9change CSV/XLSX",
    exchangeHelp: "Exportez un param\u00E8tre r\u00E9gional/port\u00E9e s\u00E9lectionn\u00E9, modifiez-le en externe, puis validez chaque ligne. L'importation met uniquement \u00E0 jour la copie de travail Website\u00A0; Preview et Publish restent obligatoires. Utilisez action=reset avec une valeur vide pour supprimer un remplacement.",
    exportCSV: "Exporter CSV",
    exportXLSX: "Exporter XLSX",
    chooseExchange: "Choisissez CSV/XLSX",
    validateExchange: "Valider l'importation",
    commitExchange: "Valider les modifications valid\u00E9es",
    exchangeDone: (revision: number) => `Importation de traduction engag\u00E9e dans la r\u00E9vision de travail Website${revision}. Preview et Publish sont toujours requis.`,
    line: "Doubler",
    action: "Action",
    before: "Avant",
    after: "Apr\u00E8s",
    code: "Code",
    message: "Message",
    whereUsed: "O\u00F9 utilis\u00E9",
    contextPreview: "Aper\u00E7u du contexte s\u00E9curis\u00E9",
    desktop: "Bureau",
    mobile: "Mobile",
},
"it-IT": {
    title: "Public Copy",
    help: "Modifica il testo dell'interfaccia ufficiale nella copia di lavoro durevole Website. L'output pubblico non cambia fino al completamento di Website Preview e Publish.",
    disclaimerTitle: "Stato di revisione della traduzione",
    disclaimer: "Le dieci traduzioni in bundle sono generate automaticamente e non sono state revisionate da revisori professionisti madrelingua, legali o di marketing. Esaminarli prima dell'uso in produzione.",
    workingRevision: "Revisione funzionante Website",
    locale: "Locale",
    scope: "Ambito",
    allScopes: "Tutti gli ambiti",
    search: "Filtra per chiave o descrizione",
    key: "Chiave",
    description: "Descrizione",
    kind: "Valore gentile",
    official: "Default ufficiale",
    override: "Override di lavoro",
    placeholders: "Segnaposto",
    sample: "Valori campione",
    save: "Salva override",
    reset: "Reimposta questa impostazione locale",
    resetConfirm: "Reimpostare solo questa chiave e questa impostazione locale sui valori predefiniti ufficiali?",
    noOverride: "Utilizzando l'impostazione predefinita ufficiale",
    saved: (key: string, locale: string, revision: number) => `Salvato${key} (${locale}) nella revisione operativa Website${revision}. Preview e Publish sono ancora necessari.`,
    resetDone: (key: string, locale: string, revision: number) => `Reset${key} (${locale}) nella revisione operativa Website${revision}. Preview e Publish sono ancora necessari.`,
    required: "Necessario",
    allowed: "Consentito",
    none: "Nessuno",
    selectEntry: "Seleziona una voce di copia da modificare.",
    reviewStatus: "Stato di revisione del pacchetto",
    bundleVersion: "Versione bundle ufficiale",
    exchangeTitle: "Scambio traduzione CSV/XLSX",
    exchangeHelp: "Esporta un'impostazione locale/ambito selezionata, modificala esternamente, quindi convalida ogni riga. L'importazione aggiorna solo la copia di lavoro Website; Preview e Publish rimangono obbligatori. Utilizzare action=reset con un valore vuoto per rimuovere una sostituzione.",
    exportCSV: "Esporta CSV",
    exportXLSX: "Esporta XLSX",
    chooseExchange: "Scegli CSV/XLSX",
    validateExchange: "Convalida l'importazione",
    commitExchange: "Effettua il commit delle modifiche convalidate",
    exchangeDone: (revision: number) => `Importazione della traduzione impegnata nella revisione operativa Website${revision}. Preview e Publish sono ancora necessari.`,
    line: "Linea",
    action: "Azione",
    before: "Prima",
    after: "Dopo",
    code: "Codice",
    message: "Messaggio",
    whereUsed: "Dove utilizzato",
    contextPreview: "Anteprima del contesto sicuro",
    desktop: "Desktop",
    mobile: "Mobile",
},
"es-ES": {
    title: "Public Copy",
    help: "Edite el texto de la interfaz oficial en la duradera copia de trabajo Website. La producci\u00F3n p\u00FAblica no cambia hasta que se completen Website, Preview y Publish.",
    disclaimerTitle: "Estado de revisi\u00F3n de traducci\u00F3n",
    disclaimer: "Las diez traducciones incluidas se generan autom\u00E1ticamente y no han sido revisadas por revisores profesionales nativos, legales o de marketing. Rev\u00EDselos antes de su uso en producci\u00F3n.",
    workingRevision: "Revisi\u00F3n de trabajo Website",
    locale: "Lugar",
    scope: "Alcance",
    allScopes: "Todos los alcances",
    search: "Filtrar por clave o descripci\u00F3n",
    key: "Llave",
    description: "Descripci\u00F3n",
    kind: "tipo de valor",
    official: "Por defecto oficial",
    override: "Anulaci\u00F3n de trabajo",
    placeholders: "Marcadores de posici\u00F3n",
    sample: "Valores de muestra",
    save: "Guardar anulaci\u00F3n",
    reset: "Restablecer esta configuraci\u00F3n regional",
    resetConfirm: "\u00BFRestablecer solo esta clave y configuraci\u00F3n regional a los valores predeterminados oficiales?",
    noOverride: "Usando el valor predeterminado oficial",
    saved: (key: string, locale: string, revision: number) => `Guardado${key} (${locale}) en la revisi\u00F3n de trabajo Website${revision}. A\u00FAn se requieren Preview y Publish.`,
    resetDone: (key: string, locale: string, revision: number) => `Reiniciar${key} (${locale}) en la revisi\u00F3n de trabajo Website${revision}. A\u00FAn se requieren Preview y Publish.`,
    required: "Requerido",
    allowed: "Permitido",
    none: "Ninguno",
    selectEntry: "Seleccione una entrada de copia para editar.",
    reviewStatus: "Estado de revisi\u00F3n del paquete",
    bundleVersion: "Versi\u00F3n oficial del paquete",
    exchangeTitle: "Traducci\u00F3n CSV/XLSX intercambio",
    exchangeHelp: "Exporte una configuraci\u00F3n regional/\u00E1mbito seleccionado, ed\u00EDtelo externamente y luego valide cada fila. Importar s\u00F3lo actualiza la copia de trabajo Website; Preview y Publish siguen siendo necesarios. Utilice action=reset con un valor en blanco para eliminar una anulaci\u00F3n.",
    exportCSV: "Exportar CSV",
    exportXLSX: "Exportar XLSX",
    chooseExchange: "Elija CSV/XLSX",
    validateExchange: "Validar importaci\u00F3n",
    commitExchange: "Confirmar cambios validados",
    exchangeDone: (revision: number) => `Importaci\u00F3n de traducci\u00F3n comprometida con la revisi\u00F3n de trabajo Website${revision}. A\u00FAn se requieren Preview y Publish.`,
    line: "L\u00EDnea",
    action: "Acci\u00F3n",
    before: "Antes",
    after: "Despu\u00E9s",
    code: "C\u00F3digo",
    message: "Mensaje",
    whereUsed: "donde se usa",
    contextPreview: "Vista previa de contexto seguro",
    desktop: "De oficina",
    mobile: "M\u00F3vil",
},
"pt-BR": {
    title: "Public Copy",
    help: "Edite o texto oficial da interface na c\u00F3pia de trabalho dur\u00E1vel Website. A sa\u00EDda p\u00FAblica n\u00E3o muda at\u00E9 que Website Preview e Publish sejam conclu\u00EDdos.",
    disclaimerTitle: "Status da revis\u00E3o da tradu\u00E7\u00E3o",
    disclaimer: "As dez tradu\u00E7\u00F5es agrupadas s\u00E3o geradas automaticamente e n\u00E3o foram revisadas por revisores profissionais nativos, jur\u00EDdicos ou de marketing. Revise-os antes do uso em produ\u00E7\u00E3o.",
    workingRevision: "Revis\u00E3o de trabalho Website",
    locale: "Localidade",
    scope: "Escopo",
    allScopes: "Todos os escopos",
    search: "Filtrar por chave ou descri\u00E7\u00E3o",
    key: "Chave",
    description: "Descri\u00E7\u00E3o",
    kind: "Tipo de valor",
    official: "Padr\u00E3o oficial",
    override: "Substitui\u00E7\u00E3o de trabalho",
    placeholders: "Espa\u00E7os reservados",
    sample: "Valores de amostra",
    save: "Salvar substitui\u00E7\u00E3o",
    reset: "Redefinir esta localidade",
    resetConfirm: "Redefinir apenas esta chave e localidade para o padr\u00E3o oficial?",
    noOverride: "Usando o padr\u00E3o oficial",
    saved: (key: string, locale: string, revision: number) => `Salvo${key} (${locale}) na revis\u00E3o de trabalho Website${revision}. Preview e Publish ainda s\u00E3o necess\u00E1rios.`,
    resetDone: (key: string, locale: string, revision: number) => `Reiniciar${key} (${locale}) na revis\u00E3o de trabalho Website${revision}. Preview e Publish ainda s\u00E3o necess\u00E1rios.`,
    required: "Obrigat\u00F3rio",
    allowed: "Permitido",
    none: "Nenhum",
    selectEntry: "Selecione uma entrada de c\u00F3pia para editar.",
    reviewStatus: "Status de revis\u00E3o do pacote",
    bundleVersion: "Vers\u00E3o oficial do pacote",
    exchangeTitle: "Troca de convers\u00E3o CSV/XLSX",
    exchangeHelp: "Exporte uma localidade/escopo selecionado, edite externamente e valide cada linha. A importa\u00E7\u00E3o atualiza apenas a c\u00F3pia de trabalho Website; Preview e Publish permanecem obrigat\u00F3rios. Use action=reset com um valor em branco para remover uma substitui\u00E7\u00E3o.",
    exportCSV: "Exportar CSV",
    exportXLSX: "Exportar XLSX",
    chooseExchange: "Escolha CSV/XLSX",
    validateExchange: "Validar importa\u00E7\u00E3o",
    commitExchange: "Confirmar altera\u00E7\u00F5es validadas",
    exchangeDone: (revision: number) => `Importa\u00E7\u00E3o de tradu\u00E7\u00E3o comprometida com a revis\u00E3o de trabalho Website${revision}. Preview e Publish ainda s\u00E3o necess\u00E1rios.`,
    line: "Linha",
    action: "A\u00E7\u00E3o",
    before: "Antes",
    after: "Depois",
    code: "C\u00F3digo",
    message: "Mensagem",
    whereUsed: "Onde usado",
    contextPreview: "Pr\u00E9-visualiza\u00E7\u00E3o segura do contexto",
    desktop: "\u00C1rea de trabalho",
    mobile: "M\u00F3vel",
},
} as const;

function scopeOf(key: string): string {
  return key.includes(".") ? key.slice(0, key.indexOf(".")) : key;
}

function officialDefault(
  defaults: PublicCopyDefault[],
  key: string,
  locale: string,
): PublicCopyDefault | undefined {
  return defaults.find((item) => item.key === key && item.locale === locale);
}

export function PublicCopyPanel({ locale, onError, onMessage }: Props) {
  const text = labels[locale];
  const [state, setState] = useState<PublicCopyEditorState>();
  const [selectedLocale, setSelectedLocale] = useState("en-US");
  const [selectedScope, setSelectedScope] = useState<string>();
  const [filter, setFilter] = useState("");
  const [selectedKey, setSelectedKey] = useState<string>();
  const [value, setValue] = useState("");
  const [saving, setSaving] = useState(false);
  const [exchangeFile, setExchangeFile] = useState<File>();
  const [exchangePreview, setExchangePreview] = useState<PublicCopyExchangePreview>();
  const [exchangeBusy, setExchangeBusy] = useState(false);
  const [previewViewport, setPreviewViewport] = useState<"desktop" | "mobile">("desktop");

  const load = async () => {
    try {
      const next = await api<PublicCopyEditorState>("/admin/api/website/public-copy");
      setState(next);
      setSelectedLocale((current) =>
        next.enabled_locales.includes(current) ? current : next.default_locale,
      );
    } catch (error) {
      onError(error);
    }
  };

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const definitions = state?.catalog.definitions ?? [];
  const scopes = useMemo(
    () => Array.from(new Set(definitions.map((definition) => scopeOf(definition.key)))).sort(),
    [definitions],
  );
  const filteredDefinitions = useMemo(() => {
    const needle = filter.trim().toLocaleLowerCase();
    return definitions.filter(
      (definition) =>
        (!selectedScope || scopeOf(definition.key) === selectedScope) &&
        (!needle ||
          definition.key.toLocaleLowerCase().includes(needle) ||
          definition.description.toLocaleLowerCase().includes(needle)),
    );
  }, [definitions, filter, selectedScope]);
  const selected = definitions.find((definition) => definition.key === selectedKey);

  useEffect(() => {
    if (!state || !selectedKey) return;
    const override = state.overrides[selectedKey]?.[selectedLocale];
    const fallback = officialDefault(state.catalog.defaults, selectedKey, selectedLocale);
    setValue(override?.value ?? fallback?.value ?? "");
  }, [selectedKey, selectedLocale, state]);

  const save = async () => {
    if (!state || !selected) return;
    setSaving(true);
    try {
      const updated = await putJSON<{ working_revision: number }>(
        `/admin/api/website/public-copy/${encodeURIComponent(selected.key)}/${encodeURIComponent(selectedLocale)}`,
        {
          expected_working_revision: state.working_revision,
          value,
          definition_version: selected.definition_version,
        },
      );
      await load();
      onMessage(text.saved(selected.key, selectedLocale, updated.working_revision));
    } catch (error) {
      onError(error);
    } finally {
      setSaving(false);
    }
  };

  const reset = async () => {
    if (!state || !selected) return;
    setSaving(true);
    try {
      const updated = await api<{ working_revision: number }>(
        `/admin/api/website/public-copy/${encodeURIComponent(selected.key)}/${encodeURIComponent(selectedLocale)}`,
        {
          method: "DELETE",
          body: JSON.stringify({ expected_working_revision: state.working_revision }),
        },
      );
      await load();
      onMessage(text.resetDone(selected.key, selectedLocale, updated.working_revision));
    } catch (error) {
      onError(error);
    } finally {
      setSaving(false);
    }
  };

  const currentOverride = selectedKey ? state?.overrides[selectedKey]?.[selectedLocale] : undefined;
  const defaultValue = selectedKey
    ? officialDefault(state?.catalog.defaults ?? [], selectedKey, selectedLocale)?.value
    : undefined;
  const contextValue = Object.entries(selected?.sample ?? {}).reduce(
    (message, [name, sample]) => message.replaceAll(`{${name}}`, sample),
    value,
  );

  const exportExchange = async (format: "csv" | "xlsx") => {
    const query = new URLSearchParams({ locales: selectedLocale });
    if (selectedScope) query.set("scope", selectedScope);
    try {
      await downloadFile(`/admin/api/website/public-copy/export/${format}?${query}`, `prods-public-copy.${format}`);
    } catch (error) {
      onError(error);
    }
  };

  const previewExchange = async () => {
    if (!exchangeFile) return;
    setExchangeBusy(true);
    try {
      const body = new FormData();
      body.append("file", exchangeFile, exchangeFile.name);
      setExchangePreview(await api<PublicCopyExchangePreview>("/admin/api/website/public-copy/import/preview", { method: "POST", body }));
    } catch (error) {
      onError(error);
    } finally {
      setExchangeBusy(false);
    }
  };

  const commitExchange = async () => {
    if (!exchangePreview?.fully_validated) return;
    setExchangeBusy(true);
    try {
      const updated = await postJSON<{ working_revision: number }>("/admin/api/website/public-copy/import/commit", {
        expected_working_revision: exchangePreview.working_revision,
        rows: exchangePreview.rows,
      });
      setExchangePreview(undefined);
      setExchangeFile(undefined);
      await load();
      onMessage(text.exchangeDone(updated.working_revision));
    } catch (error) {
      onError(error);
    } finally {
      setExchangeBusy(false);
    }
  };

  return (
    <Space direction="vertical" size="middle" className="panel-stack">
      <Card title={text.title}>
        <Space direction="vertical" size="middle" className="panel-stack">
          <Typography.Paragraph>{text.help}</Typography.Paragraph>
          <Alert type="warning" showIcon message={text.disclaimerTitle} description={text.disclaimer} />
          {state && (
            <Descriptions size="small" column={{ xs: 1, sm: 3 }}>
              <Descriptions.Item label={text.workingRevision}>{state.working_revision}</Descriptions.Item>
              <Descriptions.Item label={text.bundleVersion}>
                {state.catalog.official_bundle_version}
              </Descriptions.Item>
              <Descriptions.Item label={text.reviewStatus}>{state.catalog.review_status}</Descriptions.Item>
            </Descriptions>
          )}
          <Space wrap>
            <Select
              aria-label={text.locale}
              value={selectedLocale}
              onChange={setSelectedLocale}
              options={localeSelectOptions(state?.enabled_locales ?? [])}
              style={{ minWidth: 150 }}
            />
            <Select
              allowClear
              aria-label={text.scope}
              placeholder={text.allScopes}
              value={selectedScope}
              onChange={setSelectedScope}
              options={scopes.map((item) => ({ value: item, label: item }))}
              style={{ minWidth: 170 }}
            />
            <Input.Search
              allowClear
              aria-label={text.search}
              placeholder={text.search}
              value={filter}
              onChange={(event) => setFilter(event.target.value)}
              style={{ width: 320 }}
            />
          </Space>
          <Table<PublicCopyDefinition>
            size="small"
            rowKey="key"
            dataSource={filteredDefinitions}
            pagination={{ pageSize: 12, showSizeChanger: false }}
            rowSelection={{
              type: "radio",
              selectedRowKeys: selectedKey ? [selectedKey] : [],
              onChange: (keys) => setSelectedKey(String(keys[0] ?? "")),
            }}
            onRow={(record) => ({ onClick: () => setSelectedKey(record.key) })}
            columns={[
              { title: text.key, dataIndex: "key" },
              { title: text.description, dataIndex: "description" },
              {
                title: text.kind,
                dataIndex: "value_kind",
                width: 110,
                render: (kind: string) => <Tag>{kind}</Tag>,
              },
            ]}
          />
        </Space>
      </Card>

      {selected ? (
        <Card title={`${selected.key} · ${selectedLocale}`}>
          <Space direction="vertical" size="middle" className="panel-stack">
            <Descriptions size="small" column={{ xs: 1, sm: 2 }}>
              <Descriptions.Item label={text.description}>{selected.description}</Descriptions.Item>
              <Descriptions.Item label={text.kind}>{selected.value_kind}</Descriptions.Item>
              <Descriptions.Item label={`${text.placeholders} (${text.required})`}>
                {selected.required_placeholders.join(", ") || text.none}
              </Descriptions.Item>
              <Descriptions.Item label={`${text.placeholders} (${text.allowed})`}>
                {selected.allowed_placeholders.join(", ") || text.none}
              </Descriptions.Item>
              <Descriptions.Item label={text.sample} span={2}>
                {selected.sample ? JSON.stringify(selected.sample) : text.none}
              </Descriptions.Item>
              <Descriptions.Item label={text.official} span={2}>
                <Typography.Text code>{defaultValue ?? "—"}</Typography.Text>
              </Descriptions.Item>
              <Descriptions.Item label={text.whereUsed} span={2}>
                {(selected.where_used ?? []).map((usage) => (
                  <div key={`${usage.surface}-${usage.section}`}>
                    <strong>{usage.surface}</strong> · {usage.section} — {usage.purpose}
                    <br /><Typography.Text type="secondary">{usage.safe_context}</Typography.Text>
                  </div>
                ))}
              </Descriptions.Item>
            </Descriptions>
            {!currentOverride && <Alert type="info" showIcon message={text.noOverride} />}
            <Typography.Text strong>{text.override}</Typography.Text>
            <Input.TextArea
              autoSize={{ minRows: 3, maxRows: 10 }}
              value={value}
              onChange={(event) => setValue(event.target.value)}
            />
            <Space>
              <Typography.Text strong>{text.contextPreview}</Typography.Text>
              <Select
                value={previewViewport}
                onChange={setPreviewViewport}
                options={[{ value: "desktop", label: text.desktop }, { value: "mobile", label: text.mobile }]}
              />
            </Space>
            <div
              aria-label={text.contextPreview}
              style={{
                width: previewViewport === "mobile" ? 360 : "100%",
                maxWidth: "100%",
                padding: 16,
                border: "1px solid #d9d9d9",
                borderRadius: 8,
                background: "#fff",
              }}
            >
              <Typography.Text>{contextValue}</Typography.Text>
            </div>
            <Space>
              <Button type="primary" loading={saving} onClick={() => void save()}>
                {text.save}
              </Button>
              <Popconfirm title={text.resetConfirm} onConfirm={() => void reset()}>
                <Button disabled={!currentOverride || saving}>{text.reset}</Button>
              </Popconfirm>
            </Space>
          </Space>
        </Card>
      ) : (
        <Card>
          <Empty description={text.selectEntry} />
        </Card>
      )}

      <Card title={text.exchangeTitle}>
        <Space direction="vertical" size="middle" className="panel-stack">
          <Typography.Paragraph>{text.exchangeHelp}</Typography.Paragraph>
          <Space wrap>
            <Button onClick={() => void exportExchange("csv")}>{text.exportCSV}</Button>
            <Button onClick={() => void exportExchange("xlsx")}>{text.exportXLSX}</Button>
            <Upload
              accept=".csv,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
              maxCount={1}
              beforeUpload={(file) => { setExchangeFile(file); setExchangePreview(undefined); return false; }}
              onRemove={() => { setExchangeFile(undefined); setExchangePreview(undefined); }}
            >
              <Button>{text.chooseExchange}</Button>
            </Upload>
            <Button type="primary" disabled={!exchangeFile} loading={exchangeBusy} onClick={() => void previewExchange()}>
              {text.validateExchange}
            </Button>
            <Button
              type="primary"
              disabled={!exchangePreview?.fully_validated}
              loading={exchangeBusy}
              onClick={() => void commitExchange()}
            >
              {text.commitExchange}
            </Button>
          </Space>
          {exchangePreview && (
            <>
              <Alert
                showIcon
                type={exchangePreview.fully_validated ? "success" : "error"}
                message={exchangePreview.fully_validated
                  ? `${exchangePreview.changes.length} validated change(s)`
                  : `${exchangePreview.issues.length} validation issue(s)`}
              />
              {exchangePreview.issues.length > 0 && (
                <Table<PublicCopyExchangeIssue>
                  size="small"
                  rowKey={(item, index) => `${item.line}-${item.code}-${index}`}
                  dataSource={exchangePreview.issues}
                  pagination={false}
                  columns={[
                    { title: text.line, dataIndex: "line" },
                    { title: text.key, dataIndex: "key" },
                    { title: text.locale, dataIndex: "locale" },
                    { title: text.code, dataIndex: "code" },
                    { title: text.message, dataIndex: "message" },
                  ]}
                />
              )}
              <Table<PublicCopyExchangeChange>
                size="small"
                rowKey={(item, index) => `${item.line}-${item.key}-${item.locale}-${index}`}
                dataSource={exchangePreview.changes}
                pagination={{ pageSize: 20 }}
                columns={[
                  { title: text.line, dataIndex: "line" },
                  { title: text.key, dataIndex: "key" },
                  { title: text.locale, dataIndex: "locale" },
                  { title: text.action, dataIndex: "action" },
                  { title: text.before, dataIndex: "before", ellipsis: true },
                  { title: text.after, dataIndex: "after", ellipsis: true },
                ]}
              />
            </>
          )}
        </Space>
      </Card>
    </Space>
  );
}
