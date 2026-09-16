import { useEffect, useState } from "react";
import { Alert, Button, Card, Descriptions, Input, List, Modal, Popconfirm, Select, Space, Table, Tag, Typography, message } from "antd";
import { APIError, api, clientID, downloadFile, postJSON, putJSON } from "./api";
import type { AdminLocale } from "./locales";
import type { AuditEntry, RFQ, RFQDeliveryAttempt, RFQRecipientSettings, RFQRecipientUser } from "./types";

const transitions: Record<RFQ["Status"], RFQ["Status"][]> = {
  new: ["in_progress", "spam"],
  in_progress: ["closed", "spam"],
  closed: ["in_progress"],
  spam: ["new", "in_progress"],
};

type ActivityLocale = AdminLocale;

const labels = {
  "en-US": {
    status: { new: "New", in_progress: "In progress", closed: "Closed", spam: "Spam" } as Record<RFQ["Status"], string>,
    exportDownloaded: "RFQ export downloaded.", updateStatusFailed: "Could not update RFQ status",
    anonymized: "RFQ personal data was anonymized locally.", anonymizeFailed: "Could not anonymize RFQ personal data",
    recipientsUpdated: "RFQ recipients updated", recipientsFailed: "Could not update recipients",
    deliveryQueued: "Manual RFQ delivery was durably queued",
    assignmentNotice: "Recipient assignment does not grant Admin access and does not send email. Sending remains a separate explicit action.",
    defaultRecipients: "Default recipients", savedRFQs: "Saved RFQs", exportCSV: "Export CSV", refresh: "Refresh", noRFQs: "No RFQs yet",
    updateStatus: (id: string) => `Update status for ${id}`, recipients: "Recipients", deliveryHistory: "Delivery history", sendHistory: "Send / history",
    anonymizeConfirm: "Anonymize this RFQ's personal data?",
    anonymizeDescription: "This removes local contact details, free-form text, assignments, and stored mail content. Sent email and older backups cannot be recalled.",
    anonymize: "Anonymize", anonymizedRFQ: "Anonymized RFQ", personalDataRemoved: "personal data removed", revision: "rev",
    email: "Email", removed: "Removed", created: "Created", items: "Items", message: "Message", privacyProcessed: "Privacy processed",
    user: "user", external: "external", unassigned: "Unassigned", adminLog: "Admin Log", time: "Time", action: "Action", target: "Target", actor: "Actor", result: "Result", details: "Details", system: "system",
    defaultRecipientsTitle: "Default recipients for new RFQs", recipientsTitle: (id?: string) => id ? `Recipients — ${id}` : "Recipients",
    assignmentOnly: "Assignment only: this does not grant access or send email.", systemUsers: "System users", externalEmails: "External email addresses", onePerLine: "One address per line",
    deliveryTitle: (id?: string) => id ? `Manual delivery — ${id}` : "Manual delivery", sendNow: "Send now",
    sendWarning: "This is an explicit external send. SMTP acceptance does not prove inbox delivery or reading. Unknown outcomes are never retried automatically.",
    none: "None", rfqRevision: "RFQ revision", noAttempts: "No manual delivery attempts",
    attemptStatus: { pending: "Pending", accepted: "Accepted", failed: "Failed", unknown: "Unknown" } as Record<string, string>,
  },
  "zh-TW": {
    status: { new: "新詢價", in_progress: "處理中", closed: "已結案", spam: "垃圾詢價" } as Record<RFQ["Status"], string>,
    exportDownloaded: "已下載 RFQ 匯出檔。", updateStatusFailed: "無法更新 RFQ 狀態",
    anonymized: "已在本機匿名化 RFQ 個人資料。", anonymizeFailed: "無法匿名化 RFQ 個人資料",
    recipientsUpdated: "已更新 RFQ 收件人", recipientsFailed: "無法更新收件人",
    deliveryQueued: "已持久排入手動 RFQ 寄送",
    assignmentNotice: "指派收件人不會授予 Admin 存取權，也不會寄送電子郵件；寄送仍是另一個明確操作。",
    defaultRecipients: "預設收件人", savedRFQs: "已保存的 RFQ", exportCSV: "匯出 CSV", refresh: "重新整理", noRFQs: "尚無 RFQ",
    updateStatus: (id: string) => `更新 ${id} 的狀態`, recipients: "收件人", deliveryHistory: "寄送紀錄", sendHistory: "寄送／紀錄",
    anonymizeConfirm: "要匿名化這筆 RFQ 的個人資料嗎？",
    anonymizeDescription: "這會移除本機聯絡資料、自由文字、指派及已保存的郵件內容；已寄出的電子郵件與舊備份無法收回。",
    anonymize: "匿名化", anonymizedRFQ: "已匿名化 RFQ", personalDataRemoved: "個人資料已移除", revision: "修訂",
    email: "電子郵件", removed: "已移除", created: "建立時間", items: "項目", message: "訊息", privacyProcessed: "隱私處理時間",
    user: "使用者", external: "外部", unassigned: "未指派", adminLog: "管理紀錄", time: "時間", action: "動作", target: "目標", actor: "操作者", result: "結果", details: "詳細資料", system: "系統",
    defaultRecipientsTitle: "新 RFQ 的預設收件人", recipientsTitle: (id?: string) => id ? `收件人 — ${id}` : "收件人",
    assignmentOnly: "這只會指派收件人，不會授予存取權或寄送電子郵件。", systemUsers: "系統使用者", externalEmails: "外部電子郵件地址", onePerLine: "每行一個地址",
    deliveryTitle: (id?: string) => id ? `手動寄送 — ${id}` : "手動寄送", sendNow: "立即寄送",
    sendWarning: "這是明確的外部寄送。SMTP 接受不代表郵件已送達收件匣或被閱讀；結果不明時絕不自動重試。",
    none: "無", rfqRevision: "RFQ 修訂", noAttempts: "尚無手動寄送紀錄",
    attemptStatus: { pending: "等待中", accepted: "已接受", failed: "失敗", unknown: "結果不明" } as Record<string, string>,
  },
"zh-CN": {
    status: { new: "\u65B0\u7684", in_progress: "\u8FDB\u884C\u4E2D", closed: "\u5173\u95ED", spam: "\u5783\u573E\u90AE\u4EF6" } as Record<RFQ["Status"], string>,
    exportDownloaded: "RFQ \u5BFC\u51FA\u5DF2\u4E0B\u8F7D\u3002", updateStatusFailed: "\u65E0\u6CD5\u66F4\u65B0 RFQ \u72B6\u6001",
    anonymized: "RFQ \u4E2A\u4EBA\u6570\u636E\u5DF2\u5728\u672C\u5730\u8FDB\u884C\u533F\u540D\u5316\u5904\u7406\u3002", anonymizeFailed: "\u65E0\u6CD5\u533F\u540D\u5316 RFQ \u4E2A\u4EBA\u6570\u636E",
    recipientsUpdated: "RFQ \u6536\u4EF6\u4EBA\u5DF2\u66F4\u65B0", recipientsFailed: "\u65E0\u6CD5\u66F4\u65B0\u6536\u4EF6\u4EBA",
    deliveryQueued: "\u624B\u52A8 RFQ \u4EA4\u4ED8\u6301\u4E45\u6392\u961F",
    assignmentNotice: "\u6536\u4EF6\u4EBA\u5206\u914D\u4E0D\u6388\u4E88 Admin \u8BBF\u95EE\u6743\u9650\uFF0C\u4E5F\u4E0D\u53D1\u9001\u7535\u5B50\u90AE\u4EF6\u3002\u53D1\u9001\u4ECD\u7136\u662F\u4E00\u4E2A\u5355\u72EC\u7684\u663E\u5F0F\u64CD\u4F5C\u3002",
    defaultRecipients: "\u9ED8\u8BA4\u6536\u4EF6\u4EBA", savedRFQs: "\u5DF2\u4FDD\u5B58\u7684\u8BE2\u4EF7", exportCSV: "\u51FA\u53E3CSV", refresh: "\u5237\u65B0", noRFQs: "\u8FD8\u6CA1\u6709\u8BE2\u4EF7",
    updateStatus: (id: string) => `\u66F4\u65B0\u72B6\u6001${id}`, recipients: "\u6536\u4EF6\u4EBA", deliveryHistory: "\u4EA4\u8D27\u5386\u53F2", sendHistory: "\u53D1\u9001/\u5386\u53F2\u8BB0\u5F55",
    anonymizeConfirm: "\u533F\u540D\u5316\u6B64 RFQ \u7684\u4E2A\u4EBA\u6570\u636E\u5417\uFF1F",
    anonymizeDescription: "\u8FD9\u5C06\u5220\u9664\u672C\u5730\u8054\u7CFB\u4EBA\u8BE6\u7EC6\u4FE1\u606F\u3001\u81EA\u7531\u683C\u5F0F\u6587\u672C\u3001\u4F5C\u4E1A\u548C\u5B58\u50A8\u7684\u90AE\u4EF6\u5185\u5BB9\u3002\u5DF2\u53D1\u9001\u7684\u7535\u5B50\u90AE\u4EF6\u548C\u8F83\u65E7\u7684\u5907\u4EFD\u65E0\u6CD5\u64A4\u56DE\u3002",
    anonymize: "\u533F\u540D\u5316", anonymizedRFQ: "\u533F\u540D RFQ", personalDataRemoved: "\u4E2A\u4EBA\u6570\u636E\u5DF2\u5220\u9664", revision: "\u8F6C\u901F",
    email: "\u7535\u5B50\u90AE\u4EF6", removed: "\u5DF2\u5220\u9664", created: "\u5DF2\u521B\u5EFA", items: "\u9879\u76EE", message: "\u4FE1\u606F", privacyProcessed: "\u9690\u79C1\u5904\u7406",
    user: "\u7528\u6237", external: "\u5916\u90E8\u7684", unassigned: "\u672A\u5206\u914D", adminLog: "Admin \u65E5\u5FD7", time: "\u65F6\u95F4", action: "\u884C\u52A8", target: "\u76EE\u6807", actor: "\u6F14\u5458", result: "\u7ED3\u679C", details: "\u7EC6\u8282", system: "\u7CFB\u7EDF",
    defaultRecipientsTitle: "\u65B0\u8BE2\u4EF7\u7684\u9ED8\u8BA4\u6536\u4EF6\u4EBA", recipientsTitle: (id?: string) => id ? `\u6536\u4EF6\u4EBA\u2014\u2014${id}` : "\u6536\u4EF6\u4EBA",
    assignmentOnly: "\u4EC5\u5206\u914D\uFF1A\u8FD9\u4E0D\u4F1A\u6388\u4E88\u8BBF\u95EE\u6743\u9650\u6216\u53D1\u9001\u7535\u5B50\u90AE\u4EF6\u3002", systemUsers: "\u7CFB\u7EDF\u7528\u6237", externalEmails: "\u5916\u90E8\u7535\u5B50\u90AE\u4EF6\u5730\u5740", onePerLine: "\u6BCF\u884C\u4E00\u4E2A\u5730\u5740",
    deliveryTitle: (id?: string) => id ? `\u4EBA\u5DE5\u4EA4\u4ED8 \u2014${id}` : "\u4EBA\u5DE5\u53D1\u8D27", sendNow: "\u7ACB\u5373\u53D1\u9001",
    sendWarning: "\u8FD9\u662F\u663E\u5F0F\u7684\u5916\u90E8\u53D1\u9001\u3002 SMTP \u63A5\u53D7\u5E76\u4E0D\u8BC1\u660E\u6536\u4EF6\u7BB1\u5DF2\u9001\u8FBE\u6216\u5DF2\u9605\u8BFB\u3002\u672A\u77E5\u7ED3\u679C\u6C38\u8FDC\u4E0D\u4F1A\u81EA\u52A8\u91CD\u8BD5\u3002",
    none: "\u6CA1\u6709\u4EFB\u4F55", rfqRevision: "RFQ\u6539\u7248", noAttempts: "\u6CA1\u6709\u624B\u52A8\u4EA4\u4ED8\u5C1D\u8BD5",
    attemptStatus: { pending: "\u5F85\u529E\u7684", accepted: "\u516C\u8BA4", failed: "\u5931\u8D25\u7684", unknown: "\u672A\u77E5" } as Record<string, string>,
},
"ja-JP": {
    status: { new: "\u65B0\u3057\u3044", in_progress: "\u9032\u884C\u4E2D", closed: "\u9589\u5E97", spam: "\u30B9\u30D1\u30E0" } as Record<RFQ["Status"], string>,
    exportDownloaded: "RFQ \u30A8\u30AF\u30B9\u30DD\u30FC\u30C8\u304C\u30C0\u30A6\u30F3\u30ED\u30FC\u30C9\u3055\u308C\u307E\u3057\u305F\u3002", updateStatusFailed: "RFQ \u30B9\u30C6\u30FC\u30BF\u30B9\u3092\u66F4\u65B0\u3067\u304D\u307E\u305B\u3093\u3067\u3057\u305F",
    anonymized: "RFQ \u306E\u500B\u4EBA\u30C7\u30FC\u30BF\u306F\u30ED\u30FC\u30AB\u30EB\u3067\u533F\u540D\u5316\u3055\u308C\u307E\u3057\u305F\u3002", anonymizeFailed: "RFQ \u306E\u500B\u4EBA\u30C7\u30FC\u30BF\u3092\u533F\u540D\u5316\u3067\u304D\u307E\u305B\u3093\u3067\u3057\u305F",
    recipientsUpdated: "RFQ \u53D7\u4FE1\u8005\u304C\u66F4\u65B0\u3055\u308C\u307E\u3057\u305F", recipientsFailed: "\u53D7\u4FE1\u8005\u3092\u66F4\u65B0\u3067\u304D\u307E\u305B\u3093\u3067\u3057\u305F",
    deliveryQueued: "\u624B\u52D5 RFQ \u914D\u4FE1\u306F\u6C38\u7D9A\u7684\u306B\u30AD\u30E5\u30FC\u306B\u5165\u308C\u3089\u308C\u307E\u3057\u305F",
    assignmentNotice: "\u53D7\u4FE1\u8005\u306E\u5272\u308A\u5F53\u3066\u3067\u306F\u3001Admin \u30A2\u30AF\u30BB\u30B9\u306F\u8A31\u53EF\u3055\u308C\u305A\u3001\u96FB\u5B50\u30E1\u30FC\u30EB\u306F\u9001\u4FE1\u3055\u308C\u307E\u305B\u3093\u3002\u9001\u4FE1\u306F\u4F9D\u7136\u3068\u3057\u3066\u5225\u500B\u306E\u660E\u793A\u7684\u306A\u30A2\u30AF\u30B7\u30E7\u30F3\u3067\u3059\u3002",
    defaultRecipients: "\u30C7\u30D5\u30A9\u30EB\u30C8\u306E\u53D7\u4FE1\u8005", savedRFQs: "\u4FDD\u5B58\u3055\u308C\u305F\u898B\u7A4D\u4F9D\u983C", exportCSV: "CSV\u3092\u30A8\u30AF\u30B9\u30DD\u30FC\u30C8", refresh: "\u30EA\u30D5\u30EC\u30C3\u30B7\u30E5", noRFQs: "\u307E\u3060\u898B\u7A4D\u4F9D\u983C\u306F\u3042\u308A\u307E\u305B\u3093",
    updateStatus: (id: string) => `\u306E\u30B9\u30C6\u30FC\u30BF\u30B9\u3092\u66F4\u65B0\u3057\u307E\u3059${id}`, recipients: "\u53D7\u4FE1\u8005", deliveryHistory: "\u7D0D\u54C1\u5C65\u6B74", sendHistory: "\u9001\u4FE1\u30FB\u5C65\u6B74",
    anonymizeConfirm: "\u3053\u306E RFQ \u306E\u500B\u4EBA\u30C7\u30FC\u30BF\u3092\u533F\u540D\u5316\u3057\u307E\u3059\u304B?",
    anonymizeDescription: "\u3053\u308C\u306B\u3088\u308A\u3001\u30ED\u30FC\u30AB\u30EB\u306E\u9023\u7D61\u5148\u306E\u8A73\u7D30\u3001\u81EA\u7531\u5F62\u5F0F\u306E\u30C6\u30AD\u30B9\u30C8\u3001\u5272\u308A\u5F53\u3066\u3001\u304A\u3088\u3073\u4FDD\u5B58\u3055\u308C\u305F\u30E1\u30FC\u30EB\u306E\u5185\u5BB9\u304C\u524A\u9664\u3055\u308C\u307E\u3059\u3002\u9001\u4FE1\u3055\u308C\u305F\u96FB\u5B50\u30E1\u30FC\u30EB\u3068\u53E4\u3044\u30D0\u30C3\u30AF\u30A2\u30C3\u30D7\u3092\u547C\u3073\u51FA\u3059\u3053\u3068\u306F\u3067\u304D\u307E\u305B\u3093\u3002",
    anonymize: "\u533F\u540D\u5316", anonymizedRFQ: "\u533F\u540D\u5316\u3055\u308C\u305FRFQ", personalDataRemoved: "\u500B\u4EBA\u30C7\u30FC\u30BF\u304C\u524A\u9664\u3055\u308C\u307E\u3057\u305F", revision: "\u56DE\u8EE2\u6570",
    email: "\u96FB\u5B50\u30E1\u30FC\u30EB", removed: "\u524A\u9664\u3055\u308C\u307E\u3057\u305F", created: "\u4F5C\u6210\u3055\u308C\u307E\u3057\u305F", items: "\u30A2\u30A4\u30C6\u30E0", message: "\u30E1\u30C3\u30BB\u30FC\u30B8", privacyProcessed: "\u30D7\u30E9\u30A4\u30D0\u30B7\u30FC\u51E6\u7406\u6E08\u307F",
    user: "\u30E6\u30FC\u30B6\u30FC", external: "\u5916\u90E8\u306E", unassigned: "\u672A\u5272\u308A\u5F53\u3066", adminLog: "Admin\u30ED\u30B0", time: "\u6642\u9593", action: "\u30A2\u30AF\u30B7\u30E7\u30F3", target: "\u30BF\u30FC\u30B2\u30C3\u30C8", actor: "\u4FF3\u512A", result: "\u7D50\u679C", details: "\u8A73\u7D30", system: "\u30B7\u30B9\u30C6\u30E0",
    defaultRecipientsTitle: "\u65B0\u3057\u3044 RFQ \u306E\u30C7\u30D5\u30A9\u30EB\u30C8\u306E\u53D7\u4FE1\u8005", recipientsTitle: (id?: string) => id ? `\u53D7\u4FE1\u8005 -${id}` : "\u53D7\u4FE1\u8005",
    assignmentOnly: "\u5272\u308A\u5F53\u3066\u306E\u307F: \u30A2\u30AF\u30BB\u30B9\u6A29\u306E\u4ED8\u4E0E\u3084\u96FB\u5B50\u30E1\u30FC\u30EB\u306E\u9001\u4FE1\u306F\u884C\u308F\u308C\u307E\u305B\u3093\u3002", systemUsers: "\u30B7\u30B9\u30C6\u30E0\u30E6\u30FC\u30B6\u30FC", externalEmails: "\u5916\u90E8\u30E1\u30FC\u30EB\u30A2\u30C9\u30EC\u30B9", onePerLine: "1 \u884C\u306B 1 \u3064\u306E\u30A2\u30C9\u30EC\u30B9",
    deliveryTitle: (id?: string) => id ? `\u624B\u52D5\u914D\u4FE1 \u2014${id}` : "\u624B\u52D5\u914D\u4FE1", sendNow: "\u4ECA\u3059\u3050\u9001\u4FE1",
    sendWarning: "\u3053\u308C\u306F\u660E\u793A\u7684\u306A\u5916\u90E8\u9001\u4FE1\u3067\u3059\u3002 SMTP \u306E\u53D7\u8AFE\u306F\u3001\u53D7\u4FE1\u7BB1\u306E\u914D\u4FE1\u307E\u305F\u306F\u8AAD\u307F\u53D6\u308A\u3092\u8A3C\u660E\u3059\u308B\u3082\u306E\u3067\u306F\u3042\u308A\u307E\u305B\u3093\u3002\u4E0D\u660E\u306A\u7D50\u679C\u304C\u81EA\u52D5\u7684\u306B\u518D\u8A66\u884C\u3055\u308C\u308B\u3053\u3068\u306F\u3042\u308A\u307E\u305B\u3093\u3002",
    none: "\u306A\u3057", rfqRevision: "RFQ \u30EA\u30D3\u30B8\u30E7\u30F3", noAttempts: "\u624B\u52D5\u306B\u3088\u308B\u914D\u4FE1\u306F\u8A66\u884C\u3055\u308C\u307E\u305B\u3093",
    attemptStatus: { pending: "\u4FDD\u7559\u4E2D", accepted: "\u627F\u8A8D\u3055\u308C\u307E\u3057\u305F", failed: "\u5931\u6557\u3057\u305F", unknown: "\u672A\u77E5" } as Record<string, string>,
},
"ko-KR": {
    status: { new: "\uC0C8\uB85C\uC6B4", in_progress: "\uC9C4\uD589 \uC911", closed: "\uB2EB\uC740", spam: "\uC2A4\uD338" } as Record<RFQ["Status"], string>,
    exportDownloaded: "RFQ \uB0B4\uBCF4\uB0B4\uAE30\uAC00 \uB2E4\uC6B4\uB85C\uB4DC\uB418\uC5C8\uC2B5\uB2C8\uB2E4.", updateStatusFailed: "RFQ \uC0C1\uD0DC\uB97C \uC5C5\uB370\uC774\uD2B8\uD560 \uC218 \uC5C6\uC2B5\uB2C8\uB2E4.",
    anonymized: "RFQ \uAC1C\uC778 \uB370\uC774\uD130\uB294 \uB85C\uCEEC\uC5D0\uC11C \uC775\uBA85\uD654\uB418\uC5C8\uC2B5\uB2C8\uB2E4.", anonymizeFailed: "RFQ \uAC1C\uC778 \uB370\uC774\uD130\uB97C \uC775\uBA85\uD654\uD560 \uC218 \uC5C6\uC2B5\uB2C8\uB2E4.",
    recipientsUpdated: "RFQ \uC218\uC2E0\uC790 \uC5C5\uB370\uC774\uD2B8\uB428", recipientsFailed: "\uC218\uC2E0\uC790\uB97C \uC5C5\uB370\uC774\uD2B8\uD560 \uC218 \uC5C6\uC2B5\uB2C8\uB2E4.",
    deliveryQueued: "\uC218\uB3D9 RFQ \uC804\uB2EC\uC774 \uC9C0\uC18D\uC801\uC73C\uB85C \uB300\uAE30\uC5F4\uC5D0 \uCD94\uAC00\uB418\uC5C8\uC2B5\uB2C8\uB2E4.",
    assignmentNotice: "\uC218\uC2E0\uC790 \uD560\uB2F9\uC740 Admin \uC561\uC138\uC2A4 \uAD8C\uD55C\uC744 \uBD80\uC5EC\uD558\uC9C0 \uC54A\uC73C\uBA70 \uC774\uBA54\uC77C\uC744 \uBCF4\uB0B4\uC9C0 \uC54A\uC2B5\uB2C8\uB2E4. \uC804\uC1A1\uC740 \uBCC4\uB3C4\uC758 \uBA85\uC2DC\uC801 \uC791\uC5C5\uC73C\uB85C \uC720\uC9C0\uB429\uB2C8\uB2E4.",
    defaultRecipients: "\uAE30\uBCF8 \uC218\uC2E0\uC790", savedRFQs: "\uC800\uC7A5\uB41C \uACAC\uC801 \uC694\uCCAD", exportCSV: "CSV \uB0B4\uBCF4\uB0B4\uAE30", refresh: "\uC0C8\uB85C \uACE0\uCE58\uB2E4", noRFQs: "\uC544\uC9C1 RFQ\uAC00 \uC5C6\uC2B5\uB2C8\uB2E4.",
    updateStatus: (id: string) => `\uC0C1\uD0DC \uC5C5\uB370\uC774\uD2B8${id}`, recipients: "\uC218\uC2E0\uC790", deliveryHistory: "\uBC30\uC1A1 \uB0B4\uC5ED", sendHistory: "\uBCF4\uB0B4\uAE30 / \uB0B4\uC5ED",
    anonymizeConfirm: "\uC774 RFQ\uC758 \uAC1C\uC778 \uB370\uC774\uD130\uB97C \uC775\uBA85\uD654\uD558\uC2DC\uACA0\uC2B5\uB2C8\uAE4C?",
    anonymizeDescription: "\uC774\uB807\uAC8C \uD558\uBA74 \uB85C\uCEEC \uC5F0\uB77D\uCC98 \uC138\uBD80\uC815\uBCF4, \uC790\uC720 \uD615\uC2DD \uD14D\uC2A4\uD2B8, \uD560\uB2F9 \uBC0F \uC800\uC7A5\uB41C \uBA54\uC77C \uCF58\uD150\uCE20\uAC00 \uC81C\uAC70\uB429\uB2C8\uB2E4. \uC804\uC1A1\uB41C \uC774\uBA54\uC77C\uACFC \uC774\uC804 \uBC31\uC5C5\uC740 \uD68C\uC218\uD560 \uC218 \uC5C6\uC2B5\uB2C8\uB2E4.",
    anonymize: "\uC775\uBA85\uD654", anonymizedRFQ: "\uC775\uBA85\uD654\uB41C RFQ", personalDataRemoved: "\uAC1C\uC778 \uB370\uC774\uD130\uAC00 \uC81C\uAC70\uB418\uC5C8\uC2B5\uB2C8\uB2E4", revision: "\uD68C\uC804",
    email: "\uC774\uBA54\uC77C", removed: "\uC81C\uAC70\uB428", created: "\uC0DD\uC131\uB428", items: "\uD488\uBAA9", message: "\uBA54\uC2DC\uC9C0", privacyProcessed: "\uAC1C\uC778\uC815\uBCF4 \uCC98\uB9AC\uB428",
    user: "\uC0AC\uC6A9\uC790", external: "\uC678\uBD80", unassigned: "\uD560\uB2F9\uB418\uC9C0 \uC54A\uC74C", adminLog: "Admin \uB85C\uADF8", time: "\uC2DC\uAC04", action: "\uD589\uB3D9", target: "\uBAA9\uD45C", actor: "\uBC30\uC6B0", result: "\uACB0\uACFC", details: "\uC138\uBD80", system: "\uCCB4\uACC4",
    defaultRecipientsTitle: "\uC0C8 RFQ\uC758 \uAE30\uBCF8 \uC218\uC2E0\uC790", recipientsTitle: (id?: string) => id ? `\uC218\uC2E0\uC790 \u2014${id}` : "\uC218\uC2E0\uC790",
    assignmentOnly: "\uD560\uB2F9 \uC804\uC6A9: \uC561\uC138\uC2A4 \uAD8C\uD55C\uC744 \uBD80\uC5EC\uD558\uAC70\uB098 \uC774\uBA54\uC77C\uC744 \uBCF4\uB0B4\uC9C0 \uC54A\uC2B5\uB2C8\uB2E4.", systemUsers: "\uC2DC\uC2A4\uD15C \uC0AC\uC6A9\uC790", externalEmails: "\uC678\uBD80 \uC774\uBA54\uC77C \uC8FC\uC18C", onePerLine: "\uD55C \uC904\uC5D0 \uD558\uB098\uC758 \uC8FC\uC18C",
    deliveryTitle: (id?: string) => id ? `\uC218\uB3D9 \uC804\uB2EC \u2014${id}` : "\uC218\uB3D9 \uBC30\uC1A1", sendNow: "\uC9C0\uAE08 \uBCF4\uB0B4\uAE30",
    sendWarning: "\uC774\uB294 \uBA85\uC2DC\uC801\uC778 \uC678\uBD80 \uC804\uC1A1\uC785\uB2C8\uB2E4. SMTP \uC218\uB77D\uC740 \uBC1B\uC740 \uD3B8\uC9C0\uD568 \uBC30\uB2EC \uB610\uB294 \uC77D\uAE30\uB97C \uC99D\uBA85\uD558\uC9C0 \uC54A\uC2B5\uB2C8\uB2E4. \uC54C \uC218 \uC5C6\uB294 \uACB0\uACFC\uB294 \uC790\uB3D9\uC73C\uB85C \uC7AC\uC2DC\uB3C4\uB418\uC9C0 \uC54A\uC2B5\uB2C8\uB2E4.",
    none: "\uC5C6\uC74C", rfqRevision: "RFQ \uAC1C\uC815\uD310", noAttempts: "\uC218\uB3D9 \uC804\uB2EC \uC2DC\uB3C4 \uC5C6\uC74C",
    attemptStatus: { pending: "\uBCF4\uB958 \uC911", accepted: "\uC218\uB77D\uB428", failed: "\uC2E4\uD328\uD55C", unknown: "\uC54C\uB824\uC9C0\uC9C0 \uC54A\uC740" } as Record<string, string>,
},
"de-DE": {
    status: { new: "Neu", in_progress: "Im Gange", closed: "Geschlossen", spam: "Spam" } as Record<RFQ["Status"], string>,
    exportDownloaded: "RFQ-Export heruntergeladen.", updateStatusFailed: "Der RFQ-Status konnte nicht aktualisiert werden",
    anonymized: "RFQ personenbezogene Daten wurden lokal anonymisiert.", anonymizeFailed: "Pers\u00F6nliche Daten von RFQ konnten nicht anonymisiert werden",
    recipientsUpdated: "RFQ-Empf\u00E4nger aktualisiert", recipientsFailed: "Empf\u00E4nger konnten nicht aktualisiert werden",
    deliveryQueued: "Die manuelle RFQ-Zustellung stand dauerhaft in der Warteschlange",
    assignmentNotice: "Die Empf\u00E4ngerzuweisung gew\u00E4hrt Admin keinen Zugriff und sendet keine E-Mails. Das Senden bleibt eine separate explizite Aktion.",
    defaultRecipients: "Standardempf\u00E4nger", savedRFQs: "Gespeicherte RFQs", exportCSV: "Exportieren Sie CSV", refresh: "Aktualisieren", noRFQs: "Noch keine Ausschreibungen",
    updateStatus: (id: string) => `Aktualisierungsstatus f\u00FCr${id}`, recipients: "Empf\u00E4nger", deliveryHistory: "Lieferhistorie", sendHistory: "Senden / Verlauf",
    anonymizeConfirm: "Pers\u00F6nliche Daten dieses RFQ anonymisieren?",
    anonymizeDescription: "Dadurch werden lokale Kontaktdaten, Freitext, Aufgaben und gespeicherte E-Mail-Inhalte entfernt. Gesendete E-Mails und \u00E4ltere Backups k\u00F6nnen nicht zur\u00FCckgerufen werden.",
    anonymize: "Anonymisieren", anonymizedRFQ: "Anonymisierter RFQ", personalDataRemoved: "personenbezogene Daten entfernt", revision: "rev",
    email: "E-Mail", removed: "ENTFERNT", created: "Erstellt", items: "Artikel", message: "Nachricht", privacyProcessed: "Datenschutz verarbeitet",
    user: "Benutzer", external: "extern", unassigned: "Nicht zugewiesen", adminLog: "Admin Protokoll", time: "Zeit", action: "Aktion", target: "Ziel", actor: "Schauspieler", result: "Ergebnis", details: "Einzelheiten", system: "System",
    defaultRecipientsTitle: "Standardempf\u00E4nger f\u00FCr neue RFQs", recipientsTitle: (id?: string) => id ? `Empf\u00E4nger \u2013${id}` : "Empf\u00E4nger",
    assignmentOnly: "Nur Zuweisung: Dadurch wird kein Zugriff gew\u00E4hrt oder E-Mails gesendet.", systemUsers: "Systembenutzer", externalEmails: "Externe E-Mail-Adressen", onePerLine: "Eine Adresse pro Zeile",
    deliveryTitle: (id?: string) => id ? `Manuelle Lieferung \u2013${id}` : "Manuelle Lieferung", sendNow: "Jetzt senden",
    sendWarning: "Dies ist ein expliziter externer Versand. Die Annahme von SMTP stellt keinen Beweis f\u00FCr die Zustellung oder das Lesen im Posteingang dar. Unbekannte Ergebnisse werden nie automatisch wiederholt.",
    none: "Keiner", rfqRevision: "RFQ-Revision", noAttempts: "Keine manuellen Zustellversuche",
    attemptStatus: { pending: "Ausstehend", accepted: "Akzeptiert", failed: "Fehlgeschlagen", unknown: "Unbekannt" } as Record<string, string>,
},
"fr-FR": {
    status: { new: "Nouveau", in_progress: "En cours", closed: "Ferm\u00E9", spam: "Courrier ind\u00E9sirable" } as Record<RFQ["Status"], string>,
    exportDownloaded: "Exportation RFQ t\u00E9l\u00E9charg\u00E9e.", updateStatusFailed: "Impossible de mettre \u00E0 jour le statut RFQ",
    anonymized: "Les donn\u00E9es personnelles de RFQ ont \u00E9t\u00E9 anonymis\u00E9es localement.", anonymizeFailed: "Impossible d'anonymiser les donn\u00E9es personnelles de RFQ",
    recipientsUpdated: "Destinataires RFQ mis \u00E0 jour", recipientsFailed: "Impossible de mettre \u00E0 jour les destinataires",
    deliveryQueued: "La livraison manuelle du RFQ \u00E9tait durablement mise en file d'attente",
    assignmentNotice: "L'attribution du destinataire n'accorde pas l'acc\u00E8s \u00E0 Admin et n'envoie pas d'e-mail. L'envoi reste une action explicite distincte.",
    defaultRecipients: "Destinataires par d\u00E9faut", savedRFQs: "Demandes d'offres enregistr\u00E9es", exportCSV: "Exporter CSV", refresh: "Rafra\u00EEchir", noRFQs: "Aucune demande d'offre pour l'instant",
    updateStatus: (id: string) => `Mettre \u00E0 jour le statut pour${id}`, recipients: "Destinataires", deliveryHistory: "Historique de livraison", sendHistory: "Envoyer / historique",
    anonymizeConfirm: "Anonymiser les donn\u00E9es personnelles de ce RFQ\u00A0?",
    anonymizeDescription: "Cela supprime les coordonn\u00E9es locales, le texte libre, les affectations et le contenu du courrier stock\u00E9. Les e-mails envoy\u00E9s et les anciennes sauvegardes ne peuvent pas \u00EAtre rappel\u00E9s.",
    anonymize: "Anonymiser", anonymizedRFQ: "RFQ anonymis\u00E9", personalDataRemoved: "donn\u00E9es personnelles supprim\u00E9es", revision: "tour",
    email: "E-mail", removed: "Supprim\u00E9", created: "Cr\u00E9\u00E9", items: "Articles", message: "Message", privacyProcessed: "Confidentialit\u00E9 trait\u00E9e",
    user: "utilisateur", external: "externe", unassigned: "Non attribu\u00E9", adminLog: "Journal Admin", time: "Temps", action: "Action", target: "Cible", actor: "Acteur", result: "R\u00E9sultat", details: "D\u00E9tails", system: "syst\u00E8me",
    defaultRecipientsTitle: "Destinataires par d\u00E9faut pour les nouveaux appels d'offres", recipientsTitle: (id?: string) => id ? `Destinataires \u2014${id}` : "Destinataires",
    assignmentOnly: "Affectation uniquement\u00A0: cela n'accorde pas l'acc\u00E8s ni n'envoie d'e-mail.", systemUsers: "Utilisateurs du syst\u00E8me", externalEmails: "Adresses e-mail externes", onePerLine: "Une adresse par ligne",
    deliveryTitle: (id?: string) => id ? `Livraison manuelle \u2014${id}` : "Livraison manuelle", sendNow: "Envoyer maintenant",
    sendWarning: "Il s'agit d'un envoi externe explicite. L'acceptation de SMTP ne prouve pas la livraison ou la lecture de la bo\u00EEte de r\u00E9ception. Les r\u00E9sultats inconnus ne sont jamais r\u00E9essay\u00E9s automatiquement.",
    none: "Aucun", rfqRevision: "R\u00E9vision RFQ", noAttempts: "Aucune tentative de livraison manuelle",
    attemptStatus: { pending: "En attente", accepted: "Accept\u00E9", failed: "\u00C9chou\u00E9", unknown: "Inconnu" } as Record<string, string>,
},
"it-IT": {
    status: { new: "Nuovo", in_progress: "In corso", closed: "Chiuso", spam: "Spam" } as Record<RFQ["Status"], string>,
    exportDownloaded: "Esportazione RFQ scaricata.", updateStatusFailed: "Impossibile aggiornare lo stato RFQ",
    anonymized: "I dati personali RFQ sono stati resi anonimi a livello locale.", anonymizeFailed: "Impossibile anonimizzare i dati personali RFQ",
    recipientsUpdated: "Destinatari RFQ aggiornati", recipientsFailed: "Impossibile aggiornare i destinatari",
    deliveryQueued: "La consegna manuale di RFQ \u00E8 rimasta permanentemente in coda",
    assignmentNotice: "L'assegnazione del destinatario non garantisce l'accesso Admin e non invia messaggi di posta elettronica. L'invio rimane un'azione esplicita separata.",
    defaultRecipients: "Destinatari predefiniti", savedRFQs: "Richieste di offerta salvate", exportCSV: "Esporta CSV", refresh: "Aggiorna", noRFQs: "Nessuna richiesta di offerta ancora",
    updateStatus: (id: string) => `Aggiorna stato per${id}`, recipients: "Destinatari", deliveryHistory: "Cronologia delle consegne", sendHistory: "Invia / cronologia",
    anonymizeConfirm: "Rendere anonimi i dati personali di questo RFQ?",
    anonymizeDescription: "Verranno rimossi i dettagli di contatto locali, il testo in formato libero, i compiti e il contenuto della posta archiviato. Non \u00E8 possibile richiamare le e-mail inviate e i backup precedenti.",
    anonymize: "Anonimizzare", anonymizedRFQ: "RFQ anonimo", personalDataRemoved: "dati personali rimossi", revision: "rev",
    email: "E-mail", removed: "RIMOSSO", created: "Creato", items: "Elementi", message: "Messaggio", privacyProcessed: "Privacy elaborata",
    user: "utente", external: "esterno", unassigned: "Non assegnato", adminLog: "Admin Registro", time: "Tempo", action: "Azione", target: "Bersaglio", actor: "Attore", result: "Risultato", details: "Dettagli", system: "sistema",
    defaultRecipientsTitle: "Destinatari predefiniti per le nuove richieste di offerta", recipientsTitle: (id?: string) => id ? `Destinatari \u2014${id}` : "Destinatari",
    assignmentOnly: "Solo assegnazione: non garantisce l'accesso n\u00E9 l'invio di e-mail.", systemUsers: "Utenti del sistema", externalEmails: "Indirizzi email esterni", onePerLine: "Un indirizzo per riga",
    deliveryTitle: (id?: string) => id ? `Consegna manuale \u2014${id}` : "Consegna manuale", sendNow: "Invia ora",
    sendWarning: "Questo \u00E8 un invio esterno esplicito. L'accettazione di SMTP non costituisce prova dell'avvenuta consegna o lettura della posta in arrivo. I risultati sconosciuti non vengono mai ritentati automaticamente.",
    none: "Nessuno", rfqRevision: "Revisione RFQ", noAttempts: "Nessun tentativo di consegna manuale",
    attemptStatus: { pending: "In attesa di", accepted: "Accettato", failed: "Fallito", unknown: "Sconosciuto" } as Record<string, string>,
},
"es-ES": {
    status: { new: "Nuevo", in_progress: "En curso", closed: "Cerrado", spam: "Correo basura" } as Record<RFQ["Status"], string>,
    exportDownloaded: "Exportaci\u00F3n RFQ descargada.", updateStatusFailed: "No se pudo actualizar el estado de RFQ",
    anonymized: "Los datos personales de RFQ se anonimizaron localmente.", anonymizeFailed: "No se pudieron anonimizar los datos personales de RFQ",
    recipientsUpdated: "Destinatarios RFQ actualizados", recipientsFailed: "No se pudieron actualizar los destinatarios",
    deliveryQueued: "La entrega manual RFQ estaba permanentemente en cola",
    assignmentNotice: "La asignaci\u00F3n de destinatario no otorga acceso a Admin y no env\u00EDa correo electr\u00F3nico. El env\u00EDo sigue siendo una acci\u00F3n expl\u00EDcita separada.",
    defaultRecipients: "Destinatarios predeterminados", savedRFQs: "RFQ guardadas", exportCSV: "Exportar CSV", refresh: "Refrescar", noRFQs: "A\u00FAn no hay solicitudes de cotizaci\u00F3n",
    updateStatus: (id: string) => `Estado de actualizaci\u00F3n para${id}`, recipients: "Destinatarios", deliveryHistory: "Historial de entrega", sendHistory: "Enviar / historial",
    anonymizeConfirm: "\u00BFAnonimizar los datos personales de este RFQ?",
    anonymizeDescription: "Esto elimina los detalles de contacto locales, el texto de formato libre, las tareas y el contenido del correo almacenado. Los correos electr\u00F3nicos enviados y las copias de seguridad m\u00E1s antiguas no se pueden recuperar.",
    anonymize: "Anonimizar", anonymizedRFQ: "Anonimizado RFQ", personalDataRemoved: "datos personales eliminados", revision: "Rdo",
    email: "Correo electr\u00F3nico", removed: "Remoto", created: "Creado", items: "Elementos", message: "Mensaje", privacyProcessed: "Privacidad procesada",
    user: "usuario", external: "externo", unassigned: "No asignado", adminLog: "Registro Admin", time: "Tiempo", action: "Acci\u00F3n", target: "Objetivo", actor: "Actor", result: "Resultado", details: "Detalles", system: "sistema",
    defaultRecipientsTitle: "Destinatarios predeterminados para nuevas solicitudes de cotizaci\u00F3n", recipientsTitle: (id?: string) => id ? `Destinatarios -${id}` : "Destinatarios",
    assignmentOnly: "S\u00F3lo asignaci\u00F3n: esto no otorga acceso ni env\u00EDa correo electr\u00F3nico.", systemUsers: "Usuarios del sistema", externalEmails: "Direcciones de correo electr\u00F3nico externas", onePerLine: "Una direcci\u00F3n por l\u00EDnea",
    deliveryTitle: (id?: string) => id ? `Entrega manual${id}` : "Entrega manual", sendNow: "Enviar ahora",
    sendWarning: "Este es un env\u00EDo externo expl\u00EDcito. La aceptaci\u00F3n de SMTP no acredita la entrega por inbox ni la lectura. Los resultados desconocidos nunca se vuelven a intentar autom\u00E1ticamente.",
    none: "Ninguno", rfqRevision: "Revisi\u00F3n RFQ", noAttempts: "Sin intentos de entrega manual",
    attemptStatus: { pending: "Pendiente", accepted: "Aceptado", failed: "Fallido", unknown: "Desconocido" } as Record<string, string>,
},
"pt-BR": {
    status: { new: "Novo", in_progress: "Em andamento", closed: "Fechado", spam: "Spam" } as Record<RFQ["Status"], string>,
    exportDownloaded: "Exporta\u00E7\u00E3o RFQ baixada.", updateStatusFailed: "N\u00E3o foi poss\u00EDvel atualizar o status RFQ",
    anonymized: "Os dados pessoais de RFQ foram anonimizados localmente.", anonymizeFailed: "N\u00E3o foi poss\u00EDvel anonimizar os dados pessoais de RFQ",
    recipientsUpdated: "Destinat\u00E1rios RFQ atualizados", recipientsFailed: "N\u00E3o foi poss\u00EDvel atualizar os destinat\u00E1rios",
    deliveryQueued: "A entrega manual de RFQ foi enfileirada de forma duradoura",
    assignmentNotice: "A atribui\u00E7\u00E3o de destinat\u00E1rio n\u00E3o concede acesso Admin e n\u00E3o envia email. O envio continua sendo uma a\u00E7\u00E3o expl\u00EDcita separada.",
    defaultRecipients: "Destinat\u00E1rios padr\u00E3o", savedRFQs: "Solicita\u00E7\u00F5es de cota\u00E7\u00E3o salvas", exportCSV: "Exportar CSV", refresh: "Atualizar", noRFQs: "Ainda n\u00E3o h\u00E1 solicita\u00E7\u00F5es de cota\u00E7\u00E3o",
    updateStatus: (id: string) => `Atualizar status para${id}`, recipients: "Destinat\u00E1rios", deliveryHistory: "Hist\u00F3rico de entrega", sendHistory: "Enviar / hist\u00F3rico",
    anonymizeConfirm: "Anonimizar os dados pessoais deste RFQ?",
    anonymizeDescription: "Isso remove detalhes de contato local, texto de formato livre, tarefas e conte\u00FAdo de e-mail armazenado. E-mails enviados e backups mais antigos n\u00E3o podem ser recuperados.",
    anonymize: "Anonimizar", anonymizedRFQ: "RFQ anonimizado", personalDataRemoved: "dados pessoais removidos", revision: "rev",
    email: "E-mail", removed: "Removido", created: "Criado", items: "Unid", message: "Mensagem", privacyProcessed: "Privacidade processada",
    user: "usu\u00E1rio", external: "externo", unassigned: "N\u00E3o atribu\u00EDdo", adminLog: "Registro Admin", time: "Tempo", action: "A\u00E7\u00E3o", target: "Alvo", actor: "Ator", result: "Resultado", details: "Detalhes", system: "sistema",
    defaultRecipientsTitle: "Destinat\u00E1rios padr\u00E3o para novas RFQs", recipientsTitle: (id?: string) => id ? `Destinat\u00E1rios \u2014${id}` : "Destinat\u00E1rios",
    assignmentOnly: "Apenas atribui\u00E7\u00E3o: n\u00E3o concede acesso nem envia e-mail.", systemUsers: "Usu\u00E1rios do sistema", externalEmails: "Endere\u00E7os de e-mail externos", onePerLine: "Um endere\u00E7o por linha",
    deliveryTitle: (id?: string) => id ? `Entrega manual -${id}` : "Entrega manual", sendNow: "Enviar agora",
    sendWarning: "Este \u00E9 um envio externo expl\u00EDcito. A aceita\u00E7\u00E3o do SMTP n\u00E3o comprova entrega ou leitura na caixa de entrada. Resultados desconhecidos nunca s\u00E3o repetidos automaticamente.",
    none: "Nenhum", rfqRevision: "Revis\u00E3o RFQ", noAttempts: "Nenhuma tentativa de entrega manual",
    attemptStatus: { pending: "Pendente", accepted: "Aceito", failed: "Fracassado", unknown: "Desconhecido" } as Record<string, string>,
},
} as const;

export function ActivityPanel({ locale, onError }: { locale: ActivityLocale; onError: (next: unknown) => void }) {
  const text = labels[locale];
  const [rfqs, setRFQs] = useState<RFQ[]>([]);
  const [audit, setAudit] = useState<AuditEntry[]>([]);
  const [recipientUsers, setRecipientUsers] = useState<RFQRecipientUser[]>([]);
  const [recipientSettings, setRecipientSettings] = useState<RFQRecipientSettings>();
  const [canManage, setCanManage] = useState(false);
  const [editing, setEditing] = useState<RFQ>();
  const [editingDefaults, setEditingDefaults] = useState(false);
  const [selectedUsers, setSelectedUsers] = useState<string[]>([]);
  const [externalEmails, setExternalEmails] = useState("");
  const [saving, setSaving] = useState(false);
  const [deliveryRFQ, setDeliveryRFQ] = useState<RFQ>();
  const [deliveryKey, setDeliveryKey] = useState("");
  const [deliveries, setDeliveries] = useState<RFQDeliveryAttempt[]>([]);
	const [anonymizing, setAnonymizing] = useState<string>();
  const [sending, setSending] = useState(false);
  const [exporting, setExporting] = useState(false);

  const exportRFQs = async () => {
    setExporting(true);
    try {
      await downloadFile("/admin/api/exports/rfqs.csv", "prods-rfqs.csv");
      message.success(text.exportDownloaded);
    } catch (error) {
      onError(error);
    } finally {
      setExporting(false);
    }
  };

  const loadRFQs = async () => setRFQs((await api<RFQ[]>("/admin/api/rfqs")) ?? []);
  const loadAudit = async () => setAudit((await api<AuditEntry[]>("/admin/api/audit")) ?? []);

  const loadRecipientUsers = async () => {
    try {
      const [users, settings] = await Promise.all([
        api<RFQRecipientUser[]>("/admin/api/rfq-recipient-users"),
        api<RFQRecipientSettings>("/admin/api/rfq-recipient-settings"),
      ]);
      setRecipientUsers(users ?? []);
      setRecipientSettings(settings);
      setCanManage(true);
    } catch (error) {
      if (error instanceof APIError && error.status === 403) {
        setCanManage(false);
        return;
      }
      onError(error);
    }
  };

  useEffect(() => {
    void loadRFQs();
    void loadAudit();
    void loadRecipientUsers();
  }, []);

  const updateStatus = async (rfq: RFQ, status: RFQ["Status"]) => {
    try {
      const updated = await putJSON<RFQ>(`/admin/api/rfqs/${encodeURIComponent(rfq.ID)}/status`, {
        expected_revision: rfq.Revision,
        status,
      });
      setRFQs((current) => current.map((item) => (item.ID === updated.ID ? updated : item)));
      void loadAudit();
    } catch (error) {
      message.error(error instanceof Error ? error.message : text.updateStatusFailed);
      void loadRFQs();
    }
  };

	const anonymizeRFQ = async (rfq: RFQ) => {
		setAnonymizing(rfq.ID);
		try {
			const updated = await postJSON<RFQ>(`/admin/api/rfqs/${encodeURIComponent(rfq.ID)}/anonymize`, {
				expected_revision: rfq.Revision,
			});
			setRFQs((current) => current.map((item) => (item.ID === updated.ID ? updated : item)));
			void loadAudit();
			message.success(text.anonymized);
		} catch (error) {
			message.error(error instanceof Error ? error.message : text.anonymizeFailed);
			void loadRFQs();
		} finally {
			setAnonymizing(undefined);
		}
	};

  const openRecipients = (rfq: RFQ) => {
	setEditingDefaults(false);
    setEditing(rfq);
    setSelectedUsers(
      (rfq.Recipients ?? [])
        .filter((item) => item.kind === "user")
        .map((item) => item.user_id ?? "")
        .filter(Boolean),
    );
    setExternalEmails(
      (rfq.Recipients ?? [])
        .filter((item) => item.kind === "email")
        .map((item) => item.email ?? "")
        .filter(Boolean)
        .join("\n"),
    );
  };

  const openDefaultRecipients = () => {
    if (!recipientSettings) return;
    setEditing(undefined);
    setEditingDefaults(true);
    setSelectedUsers(
      (recipientSettings.recipients ?? [])
        .filter((item) => item.kind === "user")
        .map((item) => item.user_id ?? "")
        .filter(Boolean),
    );
    setExternalEmails(
      (recipientSettings.recipients ?? [])
        .filter((item) => item.kind === "email")
        .map((item) => item.email ?? "")
        .filter(Boolean)
        .join("\n"),
    );
  };

  const saveRecipients = async () => {
    if (!editing && !editingDefaults) return;
    const emails = externalEmails.split(/[\n,;]/).map((value) => value.trim()).filter(Boolean);
    setSaving(true);
    try {
      const recipients = [
        ...selectedUsers.map((user_id) => ({ kind: "user" as const, user_id })),
        ...emails.map((email) => ({ kind: "email" as const, email })),
      ];
      if (editingDefaults && recipientSettings) {
        const updated = await putJSON<RFQRecipientSettings>("/admin/api/rfq-recipient-settings", {
          expected_revision: recipientSettings.revision,
          recipients,
        });
        setRecipientSettings(updated);
      } else if (editing) {
        const updated = await putJSON<RFQ>(`/admin/api/rfqs/${encodeURIComponent(editing.ID)}/recipients`, {
          expected_revision: editing.Revision,
          recipients,
        });
        setRFQs((current) => current.map((item) => (item.ID === updated.ID ? updated : item)));
      }
      setEditing(undefined);
      setEditingDefaults(false);
      void loadAudit();
      message.success(text.recipientsUpdated);
    } catch (error) {
      message.error(error instanceof Error ? error.message : text.recipientsFailed);
      void loadRFQs();
    } finally {
      setSaving(false);
    }
  };

  const loadDeliveries = async (rfq: RFQ) => {
    const attempts = await api<RFQDeliveryAttempt[]>(`/admin/api/rfqs/${encodeURIComponent(rfq.ID)}/deliveries`);
    setDeliveries(attempts ?? []);
  };

  const openDelivery = async (rfq: RFQ) => {
    setDeliveryRFQ(rfq);
    setDeliveryKey(clientID("mail"));
    try {
      await loadDeliveries(rfq);
    } catch (error) {
      onError(error);
    }
  };

  const sendRFQ = async () => {
    if (!deliveryRFQ || !deliveryKey) return;
    setSending(true);
    try {
      await postJSON<RFQDeliveryAttempt>(`/admin/api/rfqs/${encodeURIComponent(deliveryRFQ.ID)}/deliveries`, {
        expected_revision: deliveryRFQ.Revision,
        delivery_key: deliveryKey,
      });
      await loadDeliveries(deliveryRFQ);
      void loadAudit();
      message.success(text.deliveryQueued);
      setDeliveryKey(clientID("mail"));
    } catch (error) {
      // Keep the same delivery key. A retry after a lost response must replay
      // the original durable attempt instead of creating another send.
      onError(error);
    } finally {
      setSending(false);
    }
  };

  return (
    <Space direction="vertical" size="large" className="panel-stack">
      <Alert
        message={text.assignmentNotice}
        type="info"
        showIcon
        action={canManage ? <Button size="small" onClick={openDefaultRecipients}>{text.defaultRecipients}</Button> : undefined}
      />
      <Card
        title={text.savedRFQs}
        extra={
          <Space>
            <Button loading={exporting} onClick={() => void exportRFQs()}>{text.exportCSV}</Button>
            <Button onClick={() => void loadRFQs()}>{text.refresh}</Button>
          </Space>
        }
      >
        <List
          dataSource={rfqs}
		locale={{ emptyText: text.noRFQs }}
          renderItem={(rfq) => (
            <List.Item
              actions={canManage ? [
                <Select
                  key="status"
                  aria-label={text.updateStatus(rfq.ID)}
                  value={rfq.Status}
                  style={{ width: 140 }}
                  options={[
                    { value: rfq.Status, label: text.status[rfq.Status] },
                    ...transitions[rfq.Status].map((value) => ({ value, label: text.status[value] })),
                  ]}
                  onChange={(value) => void updateStatus(rfq, value)}
                />,
					<Button key="recipients" disabled={rfq.PrivacyState === "anonymized"} onClick={() => openRecipients(rfq)}>{text.recipients}</Button>,
				<Button key="send" disabled={!rfq.Recipients?.length && rfq.PrivacyState !== "anonymized"} onClick={() => void openDelivery(rfq)}>
						{rfq.PrivacyState === "anonymized" ? text.deliveryHistory : text.sendHistory}
				</Button>,
				rfq.PrivacyState === "anonymized" ? null : (
					<Popconfirm
						key="anonymize"
							title={text.anonymizeConfirm}
							description={text.anonymizeDescription}
							okText={text.anonymize}
						okButtonProps={{ danger: true }}
						onConfirm={() => anonymizeRFQ(rfq)}
					>
							<Button danger loading={anonymizing === rfq.ID}>{text.anonymize}</Button>
					</Popconfirm>
				),
              ] : undefined}
            >
              <List.Item.Meta
                title={
                  <Space>
					<span>{`${rfq.ID} — ${rfq.Name || text.anonymizedRFQ}`}</span>
                    <Tag>{text.status[rfq.Status]}</Tag>
					{rfq.PrivacyState === "anonymized" ? <Tag color="default">{text.personalDataRemoved}</Tag> : null}
                    <Typography.Text type="secondary">{text.revision} {rfq.Revision}</Typography.Text>
                  </Space>
                }
                description={
                  <Descriptions size="small" column={{ xs: 1, sm: 2 }}>
						<Descriptions.Item label={text.email}>{rfq.Email || text.removed}</Descriptions.Item>
                    <Descriptions.Item label={text.created}>{new Date(rfq.CreatedAt).toLocaleString(locale)}</Descriptions.Item>
                    <Descriptions.Item label={text.items} span={2}>
                      {rfq.Items?.map((item) => item.product_id || item.requested || item.raw_query).join(", ")}
                    </Descriptions.Item>
                    <Descriptions.Item label={text.recipients} span={2}>
                      {rfq.Recipients?.length
                        ? rfq.Recipients.map((item) => item.kind === "user" ? `${item.display_name || item.email} (${text.user})` : `${item.email} (${text.external})`).join(", ")
                        : text.unassigned}
                    </Descriptions.Item>
                    {rfq.GeneralMessage ? <Descriptions.Item label={text.message} span={2}>{rfq.GeneralMessage}</Descriptions.Item> : null}
					{rfq.PrivacyAt ? <Descriptions.Item label={text.privacyProcessed} span={2}>{new Date(rfq.PrivacyAt).toLocaleString(locale)}</Descriptions.Item> : null}
                  </Descriptions>
                }
              />
            </List.Item>
          )}
        />
      </Card>
      <Card title={text.adminLog} extra={<Button onClick={() => void loadAudit()}>{text.refresh}</Button>}>
        <Table<AuditEntry>
          rowKey="id"
          dataSource={audit}
          pagination={{ pageSize: 25, hideOnSinglePage: true }}
          scroll={{ x: 900 }}
          columns={[
            { title: text.time, dataIndex: "created_at", render: (value: string) => value ? new Date(value).toLocaleString(locale) : "—" },
            { title: text.action, dataIndex: "action", render: (value: string) => <Tag>{value}</Tag> },
            { title: text.target, render: (_, row) => `${row.target_type}:${row.target_id}` },
            { title: text.actor, dataIndex: "actor_id", render: (value: string) => value || text.system },
            { title: text.result, dataIndex: "result" },
            { title: text.details, dataIndex: "details", render: (value: unknown) => <Typography.Text code>{typeof value === "string" ? value : JSON.stringify(value)}</Typography.Text> },
          ]}
        />
      </Card>
      <Modal
        title={editingDefaults ? text.defaultRecipientsTitle : text.recipientsTitle(editing?.ID)}
        open={Boolean(editing) || editingDefaults}
        onCancel={() => { setEditing(undefined); setEditingDefaults(false); }}
        onOk={() => void saveRecipients()}
        confirmLoading={saving}
        destroyOnHidden
      >
        <Space direction="vertical" className="panel-stack">
          <Alert message={text.assignmentOnly} type="warning" showIcon />
          <div>
            <Typography.Text strong>{text.systemUsers}</Typography.Text>
            <Select
              mode="multiple"
              value={selectedUsers}
              onChange={setSelectedUsers}
              style={{ width: "100%" }}
              options={recipientUsers.map((user) => ({ value: user.id, label: `${user.display_name} — ${user.email}` }))}
            />
          </div>
          <div>
            <Typography.Text strong>{text.externalEmails}</Typography.Text>
            <Input.TextArea
              rows={4}
              value={externalEmails}
              onChange={(event) => setExternalEmails(event.target.value)}
              placeholder={text.onePerLine}
            />
          </div>
        </Space>
      </Modal>
      <Modal
        title={text.deliveryTitle(deliveryRFQ?.ID)}
        open={Boolean(deliveryRFQ)}
        onCancel={() => setDeliveryRFQ(undefined)}
        onOk={() => void sendRFQ()}
        okText={text.sendNow}
        confirmLoading={sending}
        okButtonProps={{ disabled: !deliveryRFQ?.Recipients?.length }}
        width={760}
      >
        <Space direction="vertical" className="panel-stack">
          <Alert
            type="warning"
            showIcon
            message={text.sendWarning}
          />
          <Descriptions size="small" column={1}>
            <Descriptions.Item label={text.recipients}>
              {deliveryRFQ?.Recipients?.map((item) => item.email).join(", ") || text.none}
            </Descriptions.Item>
            <Descriptions.Item label={text.rfqRevision}>{deliveryRFQ?.Revision}</Descriptions.Item>
            <Descriptions.Item label={text.message}>{deliveryRFQ?.GeneralMessage || "—"}</Descriptions.Item>
          </Descriptions>
          <Card size="small" title={text.deliveryHistory} extra={<Button size="small" onClick={() => deliveryRFQ && void loadDeliveries(deliveryRFQ)}>{text.refresh}</Button>}>
            <List
              dataSource={deliveries}
              locale={{ emptyText: text.noAttempts }}
              renderItem={(attempt) => (
                <List.Item>
                  <List.Item.Meta
                    title={<Space><Tag>{text.attemptStatus[attempt.status] ?? attempt.status}</Tag><span>{new Date(attempt.created_at).toLocaleString(locale)}</span><Typography.Text type="secondary">{attempt.id}</Typography.Text></Space>}
                    description={attempt.recipients.map((recipient) => `${recipient.email}: ${recipient.status}${recipient.error_class ? ` (${recipient.error_class})` : ""}`).join(", ")}
                  />
                </List.Item>
              )}
            />
          </Card>
        </Space>
      </Modal>
    </Space>
  );
}
