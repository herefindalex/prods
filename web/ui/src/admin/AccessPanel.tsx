import { useEffect, useState } from "react";
import { Alert, Button, Card, Form, Input, Modal, Popconfirm, Select, Space, Table, Tag, Typography } from "antd";
import { useInvalidate, useList } from "@refinedev/core";
import { APIError } from "./api";
import { accessCommands, accessInvalidations, type GrantResponse, type InvitationMailAttempt } from "./accessCommands";
import { capabilities, type Capability, type Role, type User } from "./types";
import type { AdminLocale } from "./locales";

type Feedback = { onError: (error: unknown) => void; onMessage: (message: string) => void };
type AccessLocale = AdminLocale;

const labels = {
  "en-US": {
    createdRole: (name: string) => `Created role ${name}.`,
    createdUser: (email: string) => `Created ${email}. Copy the one-time link now.`,
    changedRole: (email: string, role: string) => `Changed ${email} to ${role}. Existing sessions and unused set-password links were revoked.`,
    generatedLink: (email: string) => `Generated a new one-time set-password link for ${email}. Earlier unused links are invalid.`,
    reactivated: (email: string) => `Reactivated ${email}. They must use the new one-time link before signing in.`,
    disabled: (email: string) => `Disabled ${email}; existing sessions and grants were revoked.`,
    acceptedMessage: (email: string) => `SMTP accepted the invitation for ${email}.`,
    unknownMessage: (email: string) => `SMTP outcome for ${email} is unknown. Generate a new password link before sending again.`,
    failedMessage: (email: string) => `Invitation delivery for ${email} failed. The durable result is recorded.`,
    oneTimeLink: "One-time set-password link",
    bearerDescription: (email: string) => `This bearer link for ${email} is displayed only in this response. Copy it to a private channel, or explicitly send it through the configured SMTP server.`,
    copy: "Copy",
    acceptedTitle: "SMTP accepted the invitation",
    failedTitle: "Invitation delivery failed",
    unknownTitle: "Invitation outcome is unknown",
    unknownDescription: "Do not resend this same link. Generate a new password link before another explicit send.",
    durableAt: (value: string) => `Durable result recorded at ${value}.`,
    sendConfirm: "Send this bearer link by email?",
    sendDescription: "This is an explicit external SMTP send. Its accepted, failed, or unknown outcome will be recorded durably.",
    send: "Send invitation email",
    dismiss: "Dismiss",
    newRole: "New role",
    roleName: "Role name",
    capabilities: "Capabilities",
    createRole: "Create role",
    newUser: "New user",
    email: "Email",
    displayName: "Display name",
    role: "Role",
    createUser: "Create user",
    usersRoles: "Users and roles",
    refresh: "Refresh",
    status: "Status",
    authRevision: "Auth revision",
    actions: "Actions",
    changeRole: "Change role",
    history: "Invitation history",
    newPasswordLink: "New password link",
    newPasswordConfirm: "Generate a new set-password link?",
    newPasswordDescription: "Every earlier unused set-password link for this user will become invalid.",
    disableConfirm: "Disable this user and revoke all sessions and grants?",
    disable: "Disable",
    reactivateConfirm: "Reactivate this user?",
    reactivateDescription: "A new one-time set-password link will be required; the old password stays unusable.",
    reactivate: "Reactivate",
    historyFor: (email: string) => `Invitation history for ${email}`,
    evidenceTitle: "Delivery evidence does not contain the bearer link",
    evidenceDescription: "Accepted means the SMTP server accepted the message. Unknown is never retried automatically; generate a new password link before another send.",
    noAttempts: "No invitation email attempts recorded.",
    created: "Created",
    recipient: "Recipient",
    result: "Result",
    completed: "Durably completed",
    inProgress: "In progress",
    changeRoleFor: (email: string) => `Change role for ${email}`,
    accessImmediate: "Access changes immediately",
    accessDescription: "Changing a role revokes this user's current sessions and unused set-password links. The last usable Active Owner cannot be downgraded.",
    active: "active",
    disabledStatus: "disabled",
  },
  "zh-TW": {
    createdRole: (name: string) => `已建立角色「${name}」。`,
    createdUser: (email: string) => `已建立 ${email}；請立即複製一次性連結。`,
    changedRole: (email: string, role: string) => `已將 ${email} 改為「${role}」；既有工作階段與未使用設密碼連結已撤銷。`,
    generatedLink: (email: string) => `已為 ${email} 產生新的一次性設密碼連結；先前未使用的連結已失效。`,
    reactivated: (email: string) => `已重新啟用 ${email}；對方必須使用新的一次性連結設密碼後才能登入。`,
    disabled: (email: string) => `已停用 ${email}；既有工作階段與設密碼授權已撤銷。`,
    acceptedMessage: (email: string) => `SMTP 已接受寄給 ${email} 的邀請郵件。`,
    unknownMessage: (email: string) => `寄給 ${email} 的 SMTP 結果不明；再次寄送前請先產生新的設密碼連結。`,
    failedMessage: (email: string) => `寄給 ${email} 的邀請郵件失敗；結果已永久記錄。`,
    oneTimeLink: "一次性設密碼連結",
    bearerDescription: (email: string) => `這是 ${email} 的 bearer 連結，只會在此次回應顯示。請複製到適當的私密管道，或明確使用已設定的 SMTP 伺服器寄送。`,
    copy: "複製",
    acceptedTitle: "SMTP 已接受邀請郵件",
    failedTitle: "邀請郵件寄送失敗",
    unknownTitle: "邀請郵件結果不明",
    unknownDescription: "不要用同一個連結重送。再次明確寄送前，請先產生新的設密碼連結。",
    durableAt: (value: string) => `永久結果記錄時間：${value}。`,
    sendConfirm: "要用 Email 寄送這個 bearer 連結嗎？",
    sendDescription: "這會執行明確的外部 SMTP 寄送；accepted、failed 或 unknown 結果都會永久記錄。",
    send: "寄送邀請郵件",
    dismiss: "關閉",
    newRole: "新增角色",
    roleName: "角色名稱",
    capabilities: "權限能力",
    createRole: "建立角色",
    newUser: "新增使用者",
    email: "Email",
    displayName: "顯示名稱",
    role: "角色",
    createUser: "建立使用者",
    usersRoles: "使用者與角色",
    refresh: "重新整理",
    status: "狀態",
    authRevision: "認證修訂",
    actions: "操作",
    changeRole: "變更角色",
    history: "邀請郵件歷史",
    newPasswordLink: "新設密碼連結",
    newPasswordConfirm: "要產生新的設密碼連結嗎？",
    newPasswordDescription: "這位使用者先前所有未使用的設密碼連結都會失效。",
    disableConfirm: "要停用這位使用者，並撤銷所有工作階段與設密碼授權嗎？",
    disable: "停用",
    reactivateConfirm: "要重新啟用這位使用者嗎？",
    reactivateDescription: "必須建立新的一次性設密碼連結；舊密碼仍不可使用。",
    reactivate: "重新啟用",
    historyFor: (email: string) => `${email} 的邀請郵件歷史`,
    evidenceTitle: "寄送證據不包含 bearer 連結",
    evidenceDescription: "Accepted 表示 SMTP 伺服器已接受郵件。Unknown 絕不自動重送；再次寄送前請先產生新的設密碼連結。",
    noAttempts: "尚無邀請郵件寄送紀錄。",
    created: "建立時間",
    recipient: "收件人",
    result: "結果",
    completed: "已永久完成",
    inProgress: "處理中",
    changeRoleFor: (email: string) => `變更 ${email} 的角色`,
    accessImmediate: "存取權限會立即變更",
    accessDescription: "變更角色會撤銷這位使用者目前的工作階段與未使用設密碼連結。最後一位可正常登入的 Active Owner 不可降級。",
    active: "啟用中",
    disabledStatus: "已停用",
  },
"zh-CN": {
    createdRole: (name: string) => `\u5DF2\u521B\u5EFA\u89D2\u8272${name}.`,
    createdUser: (email: string) => `\u5DF2\u521B\u5EFA${email}\u3002\u7ACB\u5373\u590D\u5236\u4E00\u6B21\u6027\u94FE\u63A5\u3002`,
    changedRole: (email: string, role: string) => `\u6539\u53D8\u4E86${email}\u5230${role}\u3002\u73B0\u6709\u4F1A\u8BDD\u548C\u672A\u4F7F\u7528\u7684\u8BBE\u7F6E\u5BC6\u7801\u94FE\u63A5\u5DF2\u88AB\u64A4\u9500\u3002`,
    generatedLink: (email: string) => `\u751F\u6210\u4E86\u4E00\u4E2A\u65B0\u7684\u4E00\u6B21\u6027\u8BBE\u7F6E\u5BC6\u7801\u94FE\u63A5${email}\u3002\u65E9\u671F\u672A\u4F7F\u7528\u7684\u94FE\u63A5\u65E0\u6548\u3002`,
    reactivated: (email: string) => `\u91CD\u65B0\u6FC0\u6D3B${email}\u3002\u4ED6\u4EEC\u5FC5\u987B\u5728\u767B\u5F55\u524D\u4F7F\u7528\u65B0\u7684\u4E00\u6B21\u6027\u94FE\u63A5\u3002`,
    disabled: (email: string) => `\u6B8B\u75BE\u4EBA${email};\u73B0\u6709\u7684\u4F1A\u8BAE\u548C\u8D60\u6B3E\u88AB\u64A4\u9500\u3002`,
    acceptedMessage: (email: string) => `SMTP \u63A5\u53D7\u9080\u8BF7${email}.`,
    unknownMessage: (email: string) => `SMTP \u7ED3\u679C\u4E3A${email}\u672A\u77E5\u3002\u518D\u6B21\u53D1\u9001\u4E4B\u524D\u751F\u6210\u65B0\u5BC6\u7801\u94FE\u63A5\u3002`,
    failedMessage: (email: string) => `\u9080\u8BF7\u51FD\u9012\u9001${email}\u5931\u8D25\u7684\u3002\u8BB0\u5F55\u6301\u4E45\u7684\u7ED3\u679C\u3002`,
    oneTimeLink: "\u4E00\u6B21\u6027\u8BBE\u7F6E\u5BC6\u7801\u94FE\u63A5",
    bearerDescription: (email: string) => `\u6B64\u4E0D\u8BB0\u540D\u94FE\u63A5\u4E3A${email}\u4EC5\u5728\u6B64\u54CD\u5E94\u4E2D\u663E\u793A\u3002\u5C06\u5176\u590D\u5236\u5230\u4E13\u7528\u901A\u9053\uFF0C\u6216\u901A\u8FC7\u914D\u7F6E\u7684 SMTP \u670D\u52A1\u5668\u663E\u5F0F\u53D1\u9001\u3002`,
    copy: "\u590D\u5236",
    acceptedTitle: "SMTP \u63A5\u53D7\u9080\u8BF7",
    failedTitle: "\u9080\u8BF7\u53D1\u9001\u5931\u8D25",
    unknownTitle: "\u9080\u8BF7\u7ED3\u679C\u672A\u77E5",
    unknownDescription: "\u4E0D\u8981\u91CD\u65B0\u53D1\u9001\u540C\u4E00\u94FE\u63A5\u3002\u5728\u518D\u6B21\u663E\u5F0F\u53D1\u9001\u4E4B\u524D\u751F\u6210\u65B0\u5BC6\u7801\u94FE\u63A5\u3002",
    durableAt: (value: string) => `\u6301\u4E45\u7ED3\u679C\u8BB0\u5F55\u4E8E${value}.`,
    sendConfirm: "\u901A\u8FC7\u7535\u5B50\u90AE\u4EF6\u53D1\u9001\u6B64\u4E0D\u8BB0\u540D\u94FE\u63A5\uFF1F",
    sendDescription: "\u8FD9\u662F\u663E\u5F0F\u5916\u90E8 SMTP \u53D1\u9001\u3002\u5176\u63A5\u53D7\u7684\u3001\u5931\u8D25\u7684\u6216\u672A\u77E5\u7684\u7ED3\u679C\u5C06\u88AB\u6301\u4E45\u8BB0\u5F55\u3002",
    send: "\u53D1\u9001\u9080\u8BF7\u90AE\u4EF6",
    dismiss: "\u89E3\u96C7",
    newRole: "\u65B0\u89D2\u8272",
    roleName: "\u89D2\u8272\u540D\u79F0",
    capabilities: "\u80FD\u529B",
    createRole: "\u521B\u5EFA\u89D2\u8272",
    newUser: "\u65B0\u7528\u6237",
    email: "\u7535\u5B50\u90AE\u4EF6",
    displayName: "\u663E\u793A\u540D\u79F0",
    role: "\u89D2\u8272",
    createUser: "\u521B\u5EFA\u7528\u6237",
    usersRoles: "\u7528\u6237\u548C\u89D2\u8272",
    refresh: "\u5237\u65B0",
    status: "\u5730\u4F4D",
    authRevision: "\u6388\u6743\u4FEE\u8BA2",
    actions: "\u884C\u52A8",
    changeRole: "\u6539\u53D8\u89D2\u8272",
    history: "\u9080\u8BF7\u5386\u53F2",
    newPasswordLink: "\u65B0\u5BC6\u7801\u94FE\u63A5",
    newPasswordConfirm: "\u751F\u6210\u65B0\u7684\u8BBE\u7F6E\u5BC6\u7801\u94FE\u63A5\uFF1F",
    newPasswordDescription: "\u8BE5\u7528\u6237\u4E4B\u524D\u672A\u4F7F\u7528\u7684\u6BCF\u4E2A\u8BBE\u7F6E\u5BC6\u7801\u94FE\u63A5\u90FD\u5C06\u53D8\u5F97\u65E0\u6548\u3002",
    disableConfirm: "\u7981\u7528\u8BE5\u7528\u6237\u5E76\u64A4\u9500\u6240\u6709\u4F1A\u8BDD\u548C\u6388\u6743\uFF1F",
    disable: "\u7981\u7528",
    reactivateConfirm: "\u91CD\u65B0\u6FC0\u6D3B\u8BE5\u7528\u6237\uFF1F",
    reactivateDescription: "\u9700\u8981\u4E00\u4E2A\u65B0\u7684\u4E00\u6B21\u6027\u8BBE\u7F6E\u5BC6\u7801\u94FE\u63A5\uFF1B\u65E7\u5BC6\u7801\u4ECD\u7136\u65E0\u6CD5\u4F7F\u7528\u3002",
    reactivate: "\u91CD\u65B0\u6FC0\u6D3B",
    historyFor: (email: string) => `\u9080\u8BF7\u5386\u53F2\u8BB0\u5F55${email}`,
    evidenceTitle: "\u4EA4\u4ED8\u8BC1\u636E\u4E0D\u5305\u542B\u4E0D\u8BB0\u540D\u94FE\u63A5",
    evidenceDescription: "\u63A5\u53D7\u610F\u5473\u7740SMTP\u670D\u52A1\u5668\u63A5\u53D7\u4E86\u8BE5\u6D88\u606F\u3002\u672A\u77E5\u6C38\u8FDC\u4E0D\u4F1A\u81EA\u52A8\u91CD\u8BD5\uFF1B\u5728\u518D\u6B21\u53D1\u9001\u4E4B\u524D\u751F\u6210\u65B0\u5BC6\u7801\u94FE\u63A5\u3002",
    noAttempts: "\u6CA1\u6709\u8BB0\u5F55\u9080\u8BF7\u7535\u5B50\u90AE\u4EF6\u5C1D\u8BD5\u3002",
    created: "\u5DF2\u521B\u5EFA",
    recipient: "\u63A5\u53D7\u8005",
    result: "\u7ED3\u679C",
    completed: "\u6301\u4E45\u5B8C\u6210",
    inProgress: "\u8FDB\u884C\u4E2D",
    changeRoleFor: (email: string) => `\u66F4\u6539\u89D2\u8272\u4E3A${email}`,
    accessImmediate: "\u7ACB\u5373\u8BBF\u95EE\u66F4\u6539",
    accessDescription: "\u66F4\u6539\u89D2\u8272\u4F1A\u64A4\u9500\u8BE5\u7528\u6237\u7684\u5F53\u524D\u4F1A\u8BDD\u548C\u672A\u4F7F\u7528\u7684\u8BBE\u7F6E\u5BC6\u7801\u94FE\u63A5\u3002\u6700\u540E\u53EF\u7528\u7684\u6D3B\u52A8\u6240\u6709\u8005\u65E0\u6CD5\u964D\u7EA7\u3002",
    active: "\u79EF\u6781\u7684",
    disabledStatus: "\u6B8B\u75BE\u4EBA",
},
"ja-JP": {
    createdRole: (name: string) => `\u4F5C\u6210\u3055\u308C\u305F\u30ED\u30FC\u30EB${name}.`,
    createdUser: (email: string) => `\u4F5C\u6210\u3055\u308C\u307E\u3057\u305F${email}\u3002\u4ECA\u3059\u3050\u30EF\u30F3\u30BF\u30A4\u30E0 \u30EA\u30F3\u30AF\u3092\u30B3\u30D4\u30FC\u3057\u307E\u3059\u3002`,
    changedRole: (email: string, role: string) => `\u5909\u66F4\u3055\u308C\u307E\u3057\u305F${email}\u306B${role}\u3002\u65E2\u5B58\u306E\u30BB\u30C3\u30B7\u30E7\u30F3\u3068\u672A\u4F7F\u7528\u306E\u30D1\u30B9\u30EF\u30FC\u30C9\u8A2D\u5B9A\u30EA\u30F3\u30AF\u306F\u53D6\u308A\u6D88\u3055\u308C\u307E\u3057\u305F\u3002`,
    generatedLink: (email: string) => `\u65B0\u3057\u3044\u30EF\u30F3\u30BF\u30A4\u30E0\u30D1\u30B9\u30EF\u30FC\u30C9\u8A2D\u5B9A\u30EA\u30F3\u30AF\u3092\u751F\u6210\u3057\u307E\u3057\u305F${email}\u3002\u4EE5\u524D\u306E\u672A\u4F7F\u7528\u306E\u30EA\u30F3\u30AF\u306F\u7121\u52B9\u3067\u3059\u3002`,
    reactivated: (email: string) => `\u518D\u30A2\u30AF\u30C6\u30A3\u30D6\u5316\u3055\u308C\u307E\u3057\u305F${email}\u3002\u30B5\u30A4\u30F3\u30A4\u30F3\u3059\u308B\u524D\u306B\u3001\u65B0\u3057\u3044\u30EF\u30F3\u30BF\u30A4\u30E0 \u30EA\u30F3\u30AF\u3092\u4F7F\u7528\u3059\u308B\u5FC5\u8981\u304C\u3042\u308A\u307E\u3059\u3002`,
    disabled: (email: string) => `\u7121\u52B9${email};\u65E2\u5B58\u306E\u30BB\u30C3\u30B7\u30E7\u30F3\u3068\u8A31\u53EF\u306F\u53D6\u308A\u6D88\u3055\u308C\u307E\u3057\u305F\u3002`,
    acceptedMessage: (email: string) => `SMTP \u304C\u62DB\u5F85\u3092\u627F\u8AFE\u3057\u307E\u3057\u305F${email}.`,
    unknownMessage: (email: string) => `SMTP \u306E\u7D50\u679C${email}\u306F\u4E0D\u660E\u3067\u3059\u3002\u518D\u9001\u4FE1\u3059\u308B\u524D\u306B\u3001\u65B0\u3057\u3044\u30D1\u30B9\u30EF\u30FC\u30C9 \u30EA\u30F3\u30AF\u3092\u751F\u6210\u3057\u3066\u304F\u3060\u3055\u3044\u3002`,
    failedMessage: (email: string) => `\u3078\u306E\u62DB\u5F85\u72B6\u306E\u914D\u4FE1${email}\u5931\u6557\u3057\u305F\u3002\u8010\u4E45\u7D50\u679C\u304C\u8A18\u9332\u3055\u308C\u307E\u3059\u3002`,
    oneTimeLink: "\u30EF\u30F3\u30BF\u30A4\u30E0\u30D1\u30B9\u30EF\u30FC\u30C9\u8A2D\u5B9A\u30EA\u30F3\u30AF",
    bearerDescription: (email: string) => `\u3053\u306E\u30D9\u30A2\u30E9\u30FC\u30EA\u30F3\u30AF\u306F\u3001${email}\u306F\u3053\u306E\u5FDC\u7B54\u3067\u306E\u307F\u8868\u793A\u3055\u308C\u307E\u3059\u3002\u3053\u308C\u3092\u30D7\u30E9\u30A4\u30D9\u30FC\u30C8 \u30C1\u30E3\u30CD\u30EB\u306B\u30B3\u30D4\u30FC\u3059\u308B\u304B\u3001\u69CB\u6210\u3055\u308C\u305F SMTP \u30B5\u30FC\u30D0\u30FC\u7D4C\u7531\u3067\u660E\u793A\u7684\u306B\u9001\u4FE1\u3057\u307E\u3059\u3002`,
    copy: "\u30B3\u30D4\u30FC",
    acceptedTitle: "SMTP \u306F\u62DB\u5F85\u3092\u53D7\u3051\u5165\u308C\u307E\u3057\u305F",
    failedTitle: "\u62DB\u5F85\u72B6\u306E\u914D\u4FE1\u306B\u5931\u6557\u3057\u307E\u3057\u305F",
    unknownTitle: "\u62DB\u5F85\u7D50\u679C\u306F\u4E0D\u660E",
    unknownDescription: "\u3053\u306E\u540C\u3058\u30EA\u30F3\u30AF\u3092\u518D\u9001\u4FE1\u3057\u306A\u3044\u3067\u304F\u3060\u3055\u3044\u3002\u5225\u306E\u660E\u793A\u7684\u306A\u9001\u4FE1\u306E\u524D\u306B\u3001\u65B0\u3057\u3044\u30D1\u30B9\u30EF\u30FC\u30C9 \u30EA\u30F3\u30AF\u3092\u751F\u6210\u3057\u307E\u3059\u3002",
    durableAt: (value: string) => `\u8010\u4E45\u6027\u306E\u3042\u308B\u7D50\u679C\u304C\u8A18\u9332\u3055\u308C\u307E\u3057\u305F${value}.`,
    sendConfirm: "\u3053\u306E\u30D9\u30A2\u30E9\u30FC\u30EA\u30F3\u30AF\u3092\u96FB\u5B50\u30E1\u30FC\u30EB\u3067\u9001\u4FE1\u3057\u307E\u3059\u304B?",
    sendDescription: "\u3053\u308C\u306F\u660E\u793A\u7684\u306A\u5916\u90E8 SMTP \u9001\u4FE1\u3067\u3059\u3002\u53D7\u3051\u5165\u308C\u3089\u308C\u305F\u304B\u3001\u5931\u6557\u3057\u305F\u304B\u3001\u4E0D\u660E\u306A\u7D50\u679C\u306F\u6C38\u7D9A\u7684\u306B\u8A18\u9332\u3055\u308C\u307E\u3059\u3002",
    send: "\u62DB\u5F85\u30E1\u30FC\u30EB\u3092\u9001\u4FE1\u3059\u308B",
    dismiss: "\u5374\u4E0B\u3059\u308B",
    newRole: "\u65B0\u3057\u3044\u5F79\u5272",
    roleName: "\u5F79\u5272\u540D",
    capabilities: "\u80FD\u529B",
    createRole: "\u30ED\u30FC\u30EB\u306E\u4F5C\u6210",
    newUser: "\u65B0\u898F\u30E6\u30FC\u30B6\u30FC",
    email: "\u96FB\u5B50\u30E1\u30FC\u30EB",
    displayName: "\u8868\u793A\u540D",
    role: "\u5F79\u5272",
    createUser: "\u30E6\u30FC\u30B6\u30FC\u306E\u4F5C\u6210",
    usersRoles: "\u30E6\u30FC\u30B6\u30FC\u3068\u5F79\u5272",
    refresh: "\u30EA\u30D5\u30EC\u30C3\u30B7\u30E5",
    status: "\u72B6\u614B",
    authRevision: "\u8A8D\u8A3C\u30EA\u30D3\u30B8\u30E7\u30F3",
    actions: "\u30A2\u30AF\u30B7\u30E7\u30F3",
    changeRole: "\u5F79\u5272\u3092\u5909\u66F4\u3059\u308B",
    history: "\u62DB\u5F85\u5C65\u6B74",
    newPasswordLink: "\u65B0\u3057\u3044\u30D1\u30B9\u30EF\u30FC\u30C9\u306E\u30EA\u30F3\u30AF",
    newPasswordConfirm: "\u65B0\u3057\u3044\u30D1\u30B9\u30EF\u30FC\u30C9\u8A2D\u5B9A\u30EA\u30F3\u30AF\u3092\u751F\u6210\u3057\u307E\u3059\u304B?",
    newPasswordDescription: "\u3053\u306E\u30E6\u30FC\u30B6\u30FC\u306E\u4EE5\u524D\u306E\u672A\u4F7F\u7528\u306E\u30D1\u30B9\u30EF\u30FC\u30C9\u8A2D\u5B9A\u30EA\u30F3\u30AF\u306F\u3059\u3079\u3066\u7121\u52B9\u306B\u306A\u308A\u307E\u3059\u3002",
    disableConfirm: "\u3053\u306E\u30E6\u30FC\u30B6\u30FC\u3092\u7121\u52B9\u306B\u3057\u3066\u3001\u3059\u3079\u3066\u306E\u30BB\u30C3\u30B7\u30E7\u30F3\u3068\u8A31\u53EF\u3092\u53D6\u308A\u6D88\u3057\u307E\u3059\u304B?",
    disable: "\u7121\u52B9\u306B\u3059\u308B",
    reactivateConfirm: "\u3053\u306E\u30E6\u30FC\u30B6\u30FC\u3092\u518D\u30A2\u30AF\u30C6\u30A3\u30D6\u5316\u3057\u307E\u3059\u304B?",
    reactivateDescription: "\u65B0\u3057\u3044\u30EF\u30F3\u30BF\u30A4\u30E0\u30D1\u30B9\u30EF\u30FC\u30C9\u8A2D\u5B9A\u30EA\u30F3\u30AF\u304C\u5FC5\u8981\u306B\u306A\u308A\u307E\u3059\u3002\u53E4\u3044\u30D1\u30B9\u30EF\u30FC\u30C9\u306F\u4F7F\u7528\u3067\u304D\u306A\u3044\u307E\u307E\u306B\u306A\u308A\u307E\u3059\u3002",
    reactivate: "\u518D\u30A2\u30AF\u30C6\u30A3\u30D6\u5316",
    historyFor: (email: string) => `\u62DB\u5F85\u5C65\u6B74${email}`,
    evidenceTitle: "\u914D\u4FE1\u8A3C\u62E0\u306B\u30D9\u30A2\u30E9\u30FC\u30EA\u30F3\u30AF\u304C\u542B\u307E\u308C\u3066\u3044\u306A\u3044",
    evidenceDescription: "Accepted \u306F\u3001SMTP \u30B5\u30FC\u30D0\u30FC\u304C\u30E1\u30C3\u30BB\u30FC\u30B8\u3092\u53D7\u3051\u5165\u308C\u305F\u3053\u3068\u3092\u610F\u5473\u3057\u307E\u3059\u3002\u4E0D\u660E\u306F\u81EA\u52D5\u7684\u306B\u518D\u8A66\u884C\u3055\u308C\u308B\u3053\u3068\u306F\u3042\u308A\u307E\u305B\u3093\u3002\u5225\u306E\u9001\u4FE1\u306E\u524D\u306B\u65B0\u3057\u3044\u30D1\u30B9\u30EF\u30FC\u30C9 \u30EA\u30F3\u30AF\u3092\u751F\u6210\u3057\u307E\u3059\u3002",
    noAttempts: "\u62DB\u5F85\u30E1\u30FC\u30EB\u306E\u8A66\u884C\u306F\u8A18\u9332\u3055\u308C\u307E\u305B\u3093\u3067\u3057\u305F\u3002",
    created: "\u4F5C\u6210\u3055\u308C\u307E\u3057\u305F",
    recipient: "\u53D7\u4FE1\u8005",
    result: "\u7D50\u679C",
    completed: "\u8010\u4E45\u6027\u306E\u3042\u308B\u5B8C\u6210\u54C1",
    inProgress: "\u9032\u884C\u4E2D",
    changeRoleFor: (email: string) => `\u306E\u5F79\u5272\u3092\u5909\u66F4\u3059\u308B${email}`,
    accessImmediate: "\u5909\u66F4\u306B\u3059\u3050\u306B\u30A2\u30AF\u30BB\u30B9",
    accessDescription: "\u30ED\u30FC\u30EB\u3092\u5909\u66F4\u3059\u308B\u3068\u3001\u3053\u306E\u30E6\u30FC\u30B6\u30FC\u306E\u73FE\u5728\u306E\u30BB\u30C3\u30B7\u30E7\u30F3\u3068\u672A\u4F7F\u7528\u306E\u30D1\u30B9\u30EF\u30FC\u30C9\u8A2D\u5B9A\u30EA\u30F3\u30AF\u304C\u53D6\u308A\u6D88\u3055\u308C\u307E\u3059\u3002\u6700\u5F8C\u306B\u4F7F\u7528\u53EF\u80FD\u306A\u30A2\u30AF\u30C6\u30A3\u30D6 \u30AA\u30FC\u30CA\u30FC\u3092\u30C0\u30A6\u30F3\u30B0\u30EC\u30FC\u30C9\u3059\u308B\u3053\u3068\u306F\u3067\u304D\u307E\u305B\u3093\u3002",
    active: "\u30A2\u30AF\u30C6\u30A3\u30D6",
    disabledStatus: "\u7121\u52B9",
},
"ko-KR": {
    createdRole: (name: string) => `\uC0DD\uC131\uB41C \uC5ED\uD560${name}.`,
    createdUser: (email: string) => `\uC0DD\uC131\uB428${email}. \uC9C0\uAE08 \uC77C\uD68C\uC131 \uB9C1\uD06C\uB97C \uBCF5\uC0AC\uD558\uC138\uC694.`,
    changedRole: (email: string, role: string) => `\uBCC0\uACBD\uB428${email}\uC5D0\uAC8C${role}. \uAE30\uC874 \uC138\uC158\uACFC \uC0AC\uC6A9\uB418\uC9C0 \uC54A\uC740 \uBE44\uBC00\uBC88\uD638 \uC124\uC815 \uB9C1\uD06C\uAC00 \uCDE8\uC18C\uB418\uC5C8\uC2B5\uB2C8\uB2E4.`,
    generatedLink: (email: string) => `\uC5D0 \uB300\uD55C \uC0C8\uB85C\uC6B4 \uC77C\uD68C\uC6A9 \uBE44\uBC00\uBC88\uD638 \uC124\uC815 \uB9C1\uD06C\uB97C \uC0DD\uC131\uD588\uC2B5\uB2C8\uB2E4.${email}. \uC774\uC804\uC5D0 \uC0AC\uC6A9\uD558\uC9C0 \uC54A\uC740 \uB9C1\uD06C\uB294 \uC720\uD6A8\uD558\uC9C0 \uC54A\uC2B5\uB2C8\uB2E4.`,
    reactivated: (email: string) => `\uC7AC\uD65C\uC131\uD654\uB428${email}. \uB85C\uADF8\uC778\uD558\uAE30 \uC804\uC5D0 \uC0C8\uB85C\uC6B4 \uC77C\uD68C\uC131 \uB9C1\uD06C\uB97C \uC0AC\uC6A9\uD574\uC57C \uD569\uB2C8\uB2E4.`,
    disabled: (email: string) => `\uC7A5\uC560\uAC00 \uC788\uB294${email}; \uAE30\uC874 \uC138\uC158\uACFC \uBCF4\uC870\uAE08\uC774 \uCDE8\uC18C\uB418\uC5C8\uC2B5\uB2C8\uB2E4.`,
    acceptedMessage: (email: string) => `SMTP\uAC00 \uCD08\uB300\uB97C \uC218\uB77D\uD588\uC2B5\uB2C8\uB2E4.${email}.`,
    unknownMessage: (email: string) => `\uB2E4\uC74C\uC5D0 \uB300\uD55C SMTP \uACB0\uACFC${email}\uC54C \uC218 \uC5C6\uC2B5\uB2C8\uB2E4. \uB2E4\uC2DC \uBCF4\uB0B4\uAE30 \uC804\uC5D0 \uC0C8 \uBE44\uBC00\uBC88\uD638 \uB9C1\uD06C\uB97C \uC0DD\uC131\uD558\uC138\uC694.`,
    failedMessage: (email: string) => `\uCD08\uB300\uC7A5 \uC804\uB2EC \uB300\uC0C1${email}\uC2E4\uD328\uD55C. \uB0B4\uAD6C\uC131 \uACB0\uACFC\uAC00 \uAE30\uB85D\uB429\uB2C8\uB2E4.`,
    oneTimeLink: "\uC77C\uD68C\uC131 \uBE44\uBC00\uBC88\uD638 \uC124\uC815 \uB9C1\uD06C",
    bearerDescription: (email: string) => `\uC774 \uBB34\uAE30\uBA85 \uB9C1\uD06C\uB294${email}\uC774 \uC751\uB2F5\uC5D0\uB9CC \uD45C\uC2DC\uB429\uB2C8\uB2E4. \uBE44\uACF5\uAC1C \uCC44\uB110\uC5D0 \uBCF5\uC0AC\uD558\uAC70\uB098 \uAD6C\uC131\uB41C SMTP \uC11C\uBC84\uB97C \uD1B5\uD574 \uBA85\uC2DC\uC801\uC73C\uB85C \uBCF4\uB0C5\uB2C8\uB2E4.`,
    copy: "\uBCF5\uC0AC",
    acceptedTitle: "SMTP\uAC00 \uCD08\uB300\uB97C \uC218\uB77D\uD588\uC2B5\uB2C8\uB2E4.",
    failedTitle: "\uCD08\uB300\uC7A5 \uC804\uB2EC \uC2E4\uD328",
    unknownTitle: "\uCD08\uB300 \uACB0\uACFC\uB294 \uC54C \uC218 \uC5C6\uC2B5\uB2C8\uB2E4",
    unknownDescription: "\uB3D9\uC77C\uD55C \uB9C1\uD06C\uB97C \uB2E4\uC2DC \uBCF4\uB0B4\uC9C0 \uB9C8\uC2ED\uC2DC\uC624. \uB610 \uB2E4\uB978 \uBA85\uC2DC\uC801 \uC804\uC1A1 \uC804\uC5D0 \uC0C8 \uBE44\uBC00\uBC88\uD638 \uB9C1\uD06C\uB97C \uC0DD\uC131\uD558\uC2ED\uC2DC\uC624.",
    durableAt: (value: string) => `\uC5D0 \uAE30\uB85D\uB41C \uB0B4\uAD6C\uC131 \uC788\uB294 \uACB0\uACFC${value}.`,
    sendConfirm: "\uC774 \uC804\uB2EC\uC790 \uB9C1\uD06C\uB97C \uC774\uBA54\uC77C\uB85C \uBCF4\uB0B4\uC2DC\uACA0\uC2B5\uB2C8\uAE4C?",
    sendDescription: "\uC774\uB294 \uBA85\uC2DC\uC801\uC778 \uC678\uBD80 SMTP \uC804\uC1A1\uC785\uB2C8\uB2E4. \uC2B9\uC778, \uC2E4\uD328 \uB610\uB294 \uC54C \uC218 \uC5C6\uB294 \uACB0\uACFC\uB294 \uC9C0\uC18D\uC801\uC73C\uB85C \uAE30\uB85D\uB429\uB2C8\uB2E4.",
    send: "\uCD08\uB300 \uC774\uBA54\uC77C \uBCF4\uB0B4\uAE30",
    dismiss: "\uD574\uACE0\uD558\uB2E4",
    newRole: "\uC0C8\uB85C\uC6B4 \uC5ED\uD560",
    roleName: "\uC5ED\uD560 \uC774\uB984",
    capabilities: "\uAE30\uB2A5",
    createRole: "\uC5ED\uD560 \uB9CC\uB4E4\uAE30",
    newUser: "\uC2E0\uADDC \uC0AC\uC6A9\uC790",
    email: "\uC774\uBA54\uC77C",
    displayName: "\uD45C\uC2DC \uC774\uB984",
    role: "\uC5ED\uD560",
    createUser: "\uC0AC\uC6A9\uC790 \uC0DD\uC131",
    usersRoles: "\uC0AC\uC6A9\uC790 \uBC0F \uC5ED\uD560",
    refresh: "\uC0C8\uB85C \uACE0\uCE58\uB2E4",
    status: "\uC0C1\uD0DC",
    authRevision: "\uC778\uC99D \uAC1C\uC815",
    actions: "\uD589\uC704",
    changeRole: "\uC5ED\uD560 \uBCC0\uACBD",
    history: "\uCD08\uB300 \uB0B4\uC5ED",
    newPasswordLink: "\uC0C8 \uBE44\uBC00\uBC88\uD638 \uB9C1\uD06C",
    newPasswordConfirm: "\uC0C8\uB85C\uC6B4 \uBE44\uBC00\uBC88\uD638 \uC124\uC815 \uB9C1\uD06C\uB97C \uC0DD\uC131\uD558\uC2DC\uACA0\uC2B5\uB2C8\uAE4C?",
    newPasswordDescription: "\uC774 \uC0AC\uC6A9\uC790\uC5D0 \uB300\uD574 \uC774\uC804\uC5D0 \uC0AC\uC6A9\uB418\uC9C0 \uC54A\uC740 \uBAA8\uB4E0 \uBE44\uBC00\uBC88\uD638 \uC124\uC815 \uB9C1\uD06C\uB294 \uC720\uD6A8\uD558\uC9C0 \uC54A\uAC8C \uB429\uB2C8\uB2E4.",
    disableConfirm: "\uC774 \uC0AC\uC6A9\uC790\uB97C \uBE44\uD65C\uC131\uD654\uD558\uACE0 \uBAA8\uB4E0 \uC138\uC158\uACFC \uAD8C\uD55C \uBD80\uC5EC\uB97C \uCDE8\uC18C\uD558\uC2DC\uACA0\uC2B5\uB2C8\uAE4C?",
    disable: "\uC7A5\uC560\uB97C \uC785\uD788\uB2E4",
    reactivateConfirm: "\uC774 \uC0AC\uC6A9\uC790\uB97C \uC7AC\uD65C\uC131\uD654\uD558\uC2DC\uACA0\uC2B5\uB2C8\uAE4C?",
    reactivateDescription: "\uC0C8\uB85C\uC6B4 \uC77C\uD68C\uC131 \uBE44\uBC00\uBC88\uD638 \uC124\uC815 \uB9C1\uD06C\uAC00 \uD544\uC694\uD569\uB2C8\uB2E4. \uC774\uC804 \uBE44\uBC00\uBC88\uD638\uB294 \uC0AC\uC6A9\uD560 \uC218 \uC5C6\uC2B5\uB2C8\uB2E4.",
    reactivate: "\uC7AC\uD65C\uC131\uD654",
    historyFor: (email: string) => `\uCD08\uB300 \uB0B4\uC5ED${email}`,
    evidenceTitle: "\uBC30\uB2EC \uC99D\uAC70\uC5D0 \uC804\uB2EC\uC790 \uB9C1\uD06C\uAC00 \uD3EC\uD568\uB418\uC5B4 \uC788\uC9C0 \uC54A\uC2B5\uB2C8\uB2E4.",
    evidenceDescription: "\uC218\uB77D\uB428\uC740 SMTP \uC11C\uBC84\uAC00 \uBA54\uC2DC\uC9C0\uB97C \uC218\uB77D\uD588\uC74C\uC744 \uC758\uBBF8\uD569\uB2C8\uB2E4. Unknown\uC740 \uC790\uB3D9\uC73C\uB85C \uC7AC\uC2DC\uB3C4\uB418\uC9C0 \uC54A\uC2B5\uB2C8\uB2E4. \uB2E4\uB978 \uBCF4\uB0B4\uAE30 \uC804\uC5D0 \uC0C8 \uBE44\uBC00\uBC88\uD638 \uB9C1\uD06C\uB97C \uC0DD\uC131\uD558\uC2ED\uC2DC\uC624.",
    noAttempts: "\uCD08\uB300 \uC774\uBA54\uC77C \uC2DC\uB3C4\uAC00 \uAE30\uB85D\uB418\uC9C0 \uC54A\uC558\uC2B5\uB2C8\uB2E4.",
    created: "\uC0DD\uC131\uB428",
    recipient: "\uBC1B\uB294 \uC0AC\uB78C",
    result: "\uACB0\uACFC",
    completed: "\uB0B4\uAD6C\uC131 \uC788\uAC8C \uC644\uC131\uB428",
    inProgress: "\uC9C4\uD589 \uC911",
    changeRoleFor: (email: string) => `\uC5ED\uD560 \uBCC0\uACBD${email}`,
    accessImmediate: "\uBCC0\uACBD \uC0AC\uD56D\uC5D0 \uC989\uC2DC \uC561\uC138\uC2A4",
    accessDescription: "\uC5ED\uD560\uC744 \uBCC0\uACBD\uD558\uBA74 \uC774 \uC0AC\uC6A9\uC790\uC758 \uD604\uC7AC \uC138\uC158\uACFC \uC0AC\uC6A9\uB418\uC9C0 \uC54A\uC740 \uBE44\uBC00\uBC88\uD638 \uC124\uC815 \uB9C1\uD06C\uAC00 \uCDE8\uC18C\uB429\uB2C8\uB2E4. \uB9C8\uC9C0\uB9C9\uC73C\uB85C \uC0AC\uC6A9 \uAC00\uB2A5\uD55C \uD65C\uC131 \uC18C\uC720\uC790\uB294 \uB2E4\uC6B4\uADF8\uB808\uC774\uB4DC\uD560 \uC218 \uC5C6\uC2B5\uB2C8\uB2E4.",
    active: "\uD65C\uB3D9\uC801\uC778",
    disabledStatus: "\uC7A5\uC560\uAC00 \uC788\uB294",
},
"de-DE": {
    createdRole: (name: string) => `Rolle erstellt${name}.`,
    createdUser: (email: string) => `Erstellt${email}. Kopieren Sie jetzt den einmaligen Link.`,
    changedRole: (email: string, role: string) => `Ge\u00E4ndert${email}Zu${role}. Bestehende Sitzungen und ungenutzte Set-Password-Links wurden widerrufen.`,
    generatedLink: (email: string) => `Erstellt einen neuen Link zum einmaligen Festlegen eines Passworts f\u00FCr${email}. Zuvor nicht verwendete Links sind ung\u00FCltig.`,
    reactivated: (email: string) => `Reaktiviert${email}. Sie m\u00FCssen den neuen einmaligen Link verwenden, bevor sie sich anmelden.`,
    disabled: (email: string) => `Deaktiviert${email}; Bestehende Sitzungen und Zusch\u00FCsse wurden widerrufen.`,
    acceptedMessage: (email: string) => `SMTP hat die Einladung angenommen${email}.`,
    unknownMessage: (email: string) => `SMTP Ergebnis f\u00FCr${email}ist unbekannt. Generieren Sie einen neuen Passwort-Link, bevor Sie ihn erneut senden.`,
    failedMessage: (email: string) => `Einladungszustellung f\u00FCr${email}fehlgeschlagen. Das dauerhafte Ergebnis wird protokolliert.`,
    oneTimeLink: "Einmaliger Link zum Festlegen eines Passworts",
    bearerDescription: (email: string) => `Dieser Tr\u00E4gerlink f\u00FCr${email}wird nur in dieser Antwort angezeigt. Kopieren Sie es in einen privaten Kanal oder senden Sie es explizit \u00FCber den konfigurierten SMTP-Server.`,
    copy: "Kopie",
    acceptedTitle: "SMTP nahm die Einladung an",
    failedTitle: "Die Zustellung der Einladung ist fehlgeschlagen",
    unknownTitle: "Das Ergebnis der Einladung ist unbekannt",
    unknownDescription: "Senden Sie denselben Link nicht erneut. Generieren Sie vor einem weiteren expliziten Versand einen neuen Passwort-Link.",
    durableAt: (value: string) => `Dauerhaftes Ergebnis aufgezeichnet bei${value}.`,
    sendConfirm: "Diesen Tr\u00E4gerlink per E-Mail senden?",
    sendDescription: "Dies ist ein expliziter externer SMTP-Versand. Das akzeptierte, fehlgeschlagene oder unbekannte Ergebnis wird dauerhaft aufgezeichnet.",
    send: "Einladungs-E-Mail senden",
    dismiss: "Zur\u00FCckweisen",
    newRole: "Neue Rolle",
    roleName: "Rollenname",
    capabilities: "F\u00E4higkeiten",
    createRole: "Rolle erstellen",
    newUser: "Neuer Benutzer",
    email: "E-Mail",
    displayName: "Anzeigename",
    role: "Rolle",
    createUser: "Benutzer anlegen",
    usersRoles: "Benutzer und Rollen",
    refresh: "Aktualisieren",
    status: "Status",
    authRevision: "Auth-Revision",
    actions: "Aktionen",
    changeRole: "Rolle wechseln",
    history: "Einladungshistorie",
    newPasswordLink: "Neuer Passwort-Link",
    newPasswordConfirm: "Einen neuen Link zum Festlegen des Passworts generieren?",
    newPasswordDescription: "Jeder zuvor nicht verwendete Link zum Festlegen des Passworts f\u00FCr diesen Benutzer wird ung\u00FCltig.",
    disableConfirm: "Diesen Benutzer deaktivieren und alle Sitzungen und Gew\u00E4hrungen widerrufen?",
    disable: "Deaktivieren",
    reactivateConfirm: "Diesen Benutzer reaktivieren?",
    reactivateDescription: "Es ist ein neuer Link zum einmaligen Festlegen eines Passworts erforderlich. Das alte Passwort bleibt unbrauchbar.",
    reactivate: "Reaktivieren",
    historyFor: (email: string) => `Einladungsverlauf f\u00FCr${email}`,
    evidenceTitle: "Der Zustellnachweis enth\u00E4lt keinen Tr\u00E4gerlink",
    evidenceDescription: "Akzeptiert bedeutet, dass der SMTP-Server die Nachricht akzeptiert hat. \u201EUnbekannt\u201C wird nie automatisch erneut versucht; Generieren Sie einen neuen Passwort-Link, bevor Sie ihn erneut senden.",
    noAttempts: "Es wurden keine Versuche mit Einladungs-E-Mails aufgezeichnet.",
    created: "Erstellt",
    recipient: "Empf\u00E4nger",
    result: "Ergebnis",
    completed: "Dauerhaft abgeschlossen",
    inProgress: "Im Gange",
    changeRoleFor: (email: string) => `Rolle \u00E4ndern f\u00FCr${email}`,
    accessImmediate: "Greifen Sie sofort auf \u00C4nderungen zu",
    accessDescription: "Durch das \u00C4ndern einer Rolle werden die aktuellen Sitzungen dieses Benutzers und nicht verwendete Links zum Festlegen von Passw\u00F6rtern widerrufen. Der letzte verwendbare aktive Besitzer kann nicht herabgestuft werden.",
    active: "aktiv",
    disabledStatus: "deaktiviert",
},
"fr-FR": {
    createdRole: (name: string) => `R\u00F4le cr\u00E9\u00E9${name}.`,
    createdUser: (email: string) => `Cr\u00E9\u00E9${email}. Copiez le lien unique maintenant.`,
    changedRole: (email: string, role: string) => `Modifi\u00E9${email}\u00E0${role}. Les sessions existantes et les liens de d\u00E9finition de mot de passe inutilis\u00E9s ont \u00E9t\u00E9 r\u00E9voqu\u00E9s.`,
    generatedLink: (email: string) => `G\u00E9n\u00E9ration d'un nouveau lien de d\u00E9finition de mot de passe unique pour${email}. Les liens pr\u00E9c\u00E9demment inutilis\u00E9s ne sont pas valides.`,
    reactivated: (email: string) => `R\u00E9activ\u00E9${email}. Ils doivent utiliser le nouveau lien unique avant de se connecter.`,
    disabled: (email: string) => `D\u00E9sactiv\u00E9${email}; les sessions et les subventions existantes ont \u00E9t\u00E9 r\u00E9voqu\u00E9es.`,
    acceptedMessage: (email: string) => `SMTP a accept\u00E9 l'invitation pour${email}.`,
    unknownMessage: (email: string) => `R\u00E9sultat SMTP pour${email}est inconnu. G\u00E9n\u00E9rez un nouveau lien de mot de passe avant de renvoyer.`,
    failedMessage: (email: string) => `Remise des invitations pour${email}\u00E9chou\u00E9. Le r\u00E9sultat durable est enregistr\u00E9.`,
    oneTimeLink: "Lien unique pour d\u00E9finir un mot de passe",
    bearerDescription: (email: string) => `Ce lien porteur pour${email}est affich\u00E9 uniquement dans cette r\u00E9ponse. Copiez-le sur un canal priv\u00E9 ou envoyez-le explicitement via le serveur SMTP configur\u00E9.`,
    copy: "Copie",
    acceptedTitle: "SMTP a accept\u00E9 l'invitation",
    failedTitle: "\u00C9chec de la livraison de l'invitation",
    unknownTitle: "Le r\u00E9sultat de l'invitation est inconnu",
    unknownDescription: "Ne renvoyez pas ce m\u00EAme lien. G\u00E9n\u00E9rez un nouveau lien de mot de passe avant un autre envoi explicite.",
    durableAt: (value: string) => `R\u00E9sultat durable enregistr\u00E9 \u00E0${value}.`,
    sendConfirm: "Envoyer ce lien porteur par email ?",
    sendDescription: "Il s\u2019agit d\u2019un envoi SMTP externe explicite. Son r\u00E9sultat accept\u00E9, \u00E9chou\u00E9 ou inconnu sera enregistr\u00E9 durablement.",
    send: "Envoyer un e-mail d'invitation",
    dismiss: "Rejeter",
    newRole: "Nouveau r\u00F4le",
    roleName: "Nom du r\u00F4le",
    capabilities: "Capacit\u00E9s",
    createRole: "Cr\u00E9er un r\u00F4le",
    newUser: "Nouvel utilisateur",
    email: "E-mail",
    displayName: "Nom d'affichage",
    role: "R\u00F4le",
    createUser: "Cr\u00E9er un utilisateur",
    usersRoles: "Utilisateurs et r\u00F4les",
    refresh: "Rafra\u00EEchir",
    status: "Statut",
    authRevision: "R\u00E9vision d'authentification",
    actions: "Actes",
    changeRole: "Changer de r\u00F4le",
    history: "Historique des invitations",
    newPasswordLink: "Lien nouveau mot de passe",
    newPasswordConfirm: "G\u00E9n\u00E9rer un nouveau lien de d\u00E9finition de mot de passe\u00A0?",
    newPasswordDescription: "Tout lien de d\u00E9finition de mot de passe inutilis\u00E9 pr\u00E9c\u00E9demment pour cet utilisateur deviendra invalide.",
    disableConfirm: "D\u00E9sactiver cet utilisateur et r\u00E9voquer toutes les sessions et autorisations\u00A0?",
    disable: "D\u00E9sactiver",
    reactivateConfirm: "R\u00E9activer cet utilisateur\u00A0?",
    reactivateDescription: "Un nouveau lien de mot de passe unique sera requis\u00A0; l'ancien mot de passe reste inutilisable.",
    reactivate: "R\u00E9activer",
    historyFor: (email: string) => `Historique des invitations pour${email}`,
    evidenceTitle: "Le justificatif de livraison ne contient pas le lien porteur",
    evidenceDescription: "Accept\u00E9 signifie que le serveur SMTP a accept\u00E9 le message. Inconnu n'est jamais r\u00E9essay\u00E9 automatiquement\u00A0; g\u00E9n\u00E9rer un nouveau lien de mot de passe avant un nouvel envoi.",
    noAttempts: "Aucune tentative d'envoi d'e-mail d'invitation enregistr\u00E9e.",
    created: "Cr\u00E9\u00E9",
    recipient: "Destinataire",
    result: "R\u00E9sultat",
    completed: "Durablement achev\u00E9",
    inProgress: "En cours",
    changeRoleFor: (email: string) => `Changer de r\u00F4le pour${email}`,
    accessImmediate: "Acc\u00E9dez imm\u00E9diatement aux modifications",
    accessDescription: "La modification d'un r\u00F4le r\u00E9voque les sessions en cours de cet utilisateur et les liens de d\u00E9finition de mot de passe inutilis\u00E9s. Le dernier propri\u00E9taire actif utilisable ne peut pas \u00EAtre r\u00E9trograd\u00E9.",
    active: "actif",
    disabledStatus: "d\u00E9sactiv\u00E9",
},
"it-IT": {
    createdRole: (name: string) => `Ruolo creato${name}.`,
    createdUser: (email: string) => `Creato${email}. Copia ora il collegamento unico.`,
    changedRole: (email: string, role: string) => `Cambiato${email}A${role}. Le sessioni esistenti e i collegamenti con la password impostata inutilizzati sono stati revocati.`,
    generatedLink: (email: string) => `Generato un nuovo collegamento per l'impostazione della password una tantum per${email}. I collegamenti precedentemente non utilizzati non sono validi.`,
    reactivated: (email: string) => `Riattivato${email}. Devono utilizzare il nuovo collegamento monouso prima di accedere.`,
    disabled: (email: string) => `Disabilitato${email}; le sessioni e le sovvenzioni esistenti sono state revocate.`,
    acceptedMessage: (email: string) => `SMTP ha accettato l'invito per${email}.`,
    unknownMessage: (email: string) => `Risultato SMTP per${email}\u00E8 sconosciuto. Genera un nuovo collegamento per la password prima di inviare nuovamente.`,
    failedMessage: (email: string) => `Consegna inviti per${email}fallito. Il risultato duraturo viene registrato.`,
    oneTimeLink: "Collegamento con password impostata una tantum",
    bearerDescription: (email: string) => `Questo collegamento al portatore per${email}viene visualizzato solo in questa risposta. Copialo su un canale privato o invialo esplicitamente tramite il server SMTP configurato.`,
    copy: "Copia",
    acceptedTitle: "SMTP ha accettato l'invito",
    failedTitle: "La consegna dell'invito non \u00E8 riuscita",
    unknownTitle: "L'esito dell'invito non \u00E8 noto",
    unknownDescription: "Non inviare nuovamente lo stesso collegamento. Genera un nuovo collegamento password prima di un altro invio esplicito.",
    durableAt: (value: string) => `Risultato durevole registrato a${value}.`,
    sendConfirm: "Inviare questo collegamento tramite e-mail?",
    sendDescription: "Questo \u00E8 un invio SMTP esterno esplicito. Il suo esito accettato, fallito o sconosciuto verr\u00E0 registrato in modo duraturo.",
    send: "Invia e-mail di invito",
    dismiss: "Congedare",
    newRole: "Nuovo ruolo",
    roleName: "Nome del ruolo",
    capabilities: "Capacit\u00E0",
    createRole: "Crea ruolo",
    newUser: "Nuovo utente",
    email: "E-mail",
    displayName: "Nome da visualizzare",
    role: "Ruolo",
    createUser: "Crea utente",
    usersRoles: "Utenti e ruoli",
    refresh: "Aggiorna",
    status: "Stato",
    authRevision: "Revisione dell'autenticazione",
    actions: "Azioni",
    changeRole: "Cambia ruolo",
    history: "Cronologia degli inviti",
    newPasswordLink: "Nuovo collegamento per la password",
    newPasswordConfirm: "Generare un nuovo collegamento con la password impostata?",
    newPasswordDescription: "Ogni precedente collegamento di impostazione della password non utilizzato per questo utente non sar\u00E0 pi\u00F9 valido.",
    disableConfirm: "Disabilitare questo utente e revocare tutte le sessioni e le concessioni?",
    disable: "Disabilita",
    reactivateConfirm: "Riattivare questo utente?",
    reactivateDescription: "Sar\u00E0 richiesto un nuovo collegamento con la password impostata una tantum; la vecchia password rimane inutilizzabile.",
    reactivate: "Riattivare",
    historyFor: (email: string) => `Cronologia degli inviti per${email}`,
    evidenceTitle: "La prova di consegna non contiene il collegamento al portatore",
    evidenceDescription: "Accettato significa che il server SMTP ha accettato il messaggio. Sconosciuto non viene mai ritentato automaticamente; generare un nuovo collegamento password prima di un altro invio.",
    noAttempts: "Nessun tentativo di e-mail di invito registrato.",
    created: "Creato",
    recipient: "Destinatario",
    result: "Risultato",
    completed: "Completato durevolmente",
    inProgress: "In corso",
    changeRoleFor: (email: string) => `Cambia ruolo per${email}`,
    accessImmediate: "Accedi immediatamente alle modifiche",
    accessDescription: "La modifica di un ruolo revoca le sessioni correnti di questo utente e i collegamenti di impostazione della password non utilizzati. L'ultimo proprietario attivo utilizzabile non pu\u00F2 essere declassato.",
    active: "attivo",
    disabledStatus: "disabilitato",
},
"es-ES": {
    createdRole: (name: string) => `Rol creado${name}.`,
    createdUser: (email: string) => `Creado${email}. Copie el enlace \u00FAnico ahora.`,
    changedRole: (email: string, role: string) => `Cambi\u00F3${email}a${role}. Se revocaron las sesiones existentes y los enlaces de configuraci\u00F3n de contrase\u00F1a no utilizados.`,
    generatedLink: (email: string) => `Se gener\u00F3 un nuevo enlace \u00FAnico para establecer una contrase\u00F1a para${email}. Los enlaces no utilizados anteriormente no son v\u00E1lidos.`,
    reactivated: (email: string) => `Reactivado${email}. Deben utilizar el nuevo enlace \u00FAnico antes de iniciar sesi\u00F3n.`,
    disabled: (email: string) => `Desactivado${email}; Las sesiones y subvenciones existentes fueron revocadas.`,
    acceptedMessage: (email: string) => `SMTP acept\u00F3 la invitaci\u00F3n para${email}.`,
    unknownMessage: (email: string) => `Resultado SMTP para${email}es desconocido. Genere un nuevo enlace de contrase\u00F1a antes de volver a enviar.`,
    failedMessage: (email: string) => `Entrega de invitaci\u00F3n para${email}fallido. Se registra el resultado duradero.`,
    oneTimeLink: "Enlace \u00FAnico para establecer contrase\u00F1a",
    bearerDescription: (email: string) => `Este enlace portador para${email}se muestra solo en esta respuesta. C\u00F3pielo a un canal privado o env\u00EDelo expl\u00EDcitamente a trav\u00E9s del servidor SMTP configurado.`,
    copy: "Copiar",
    acceptedTitle: "SMTP acept\u00F3 la invitaci\u00F3n.",
    failedTitle: "Error en la entrega de la invitaci\u00F3n",
    unknownTitle: "Se desconoce el resultado de la invitaci\u00F3n",
    unknownDescription: "No reenv\u00EDes este mismo enlace. Genere un nuevo enlace de contrase\u00F1a antes de otro env\u00EDo expl\u00EDcito.",
    durableAt: (value: string) => `Resultado duradero registrado en${value}.`,
    sendConfirm: "\u00BFEnviar este enlace al portador por correo electr\u00F3nico?",
    sendDescription: "Este es un env\u00EDo externo expl\u00EDcito de SMTP. Su resultado aceptado, fallido o desconocido quedar\u00E1 registrado de forma duradera.",
    send: "Enviar correo electr\u00F3nico de invitaci\u00F3n",
    dismiss: "Despedir",
    newRole: "Nuevo rol",
    roleName: "Nombre del rol",
    capabilities: "Capacidades",
    createRole: "Crear rol",
    newUser: "Nuevo usuario",
    email: "Correo electr\u00F3nico",
    displayName: "Nombre para mostrar",
    role: "Role",
    createUser: "Crear usuario",
    usersRoles: "Usuarios y roles",
    refresh: "Refrescar",
    status: "Estado",
    authRevision: "revisi\u00F3n de autenticaci\u00F3n",
    actions: "Comportamiento",
    changeRole: "Cambiar rol",
    history: "Historial de invitaciones",
    newPasswordLink: "Nuevo enlace de contrase\u00F1a",
    newPasswordConfirm: "\u00BFGenerar un nuevo enlace para establecer contrase\u00F1a?",
    newPasswordDescription: "Todos los enlaces de configuraci\u00F3n de contrase\u00F1a no utilizados anteriormente para este usuario dejar\u00E1n de ser v\u00E1lidos.",
    disableConfirm: "\u00BFDesactivar a este usuario y revocar todas las sesiones y subvenciones?",
    disable: "Desactivar",
    reactivateConfirm: "\u00BFReactivar este usuario?",
    reactivateDescription: "Se requerir\u00E1 un nuevo enlace \u00FAnico para establecer una contrase\u00F1a; la contrase\u00F1a anterior permanece inutilizable.",
    reactivate: "Reactivar",
    historyFor: (email: string) => `Historial de invitaciones para${email}`,
    evidenceTitle: "La evidencia de entrega no contiene el enlace al portador.",
    evidenceDescription: "Aceptado significa que el servidor SMTP acept\u00F3 el mensaje. Desconocido nunca se vuelve a intentar autom\u00E1ticamente; generar un nuevo enlace de contrase\u00F1a antes de otro env\u00EDo.",
    noAttempts: "No se registraron intentos de invitaci\u00F3n por correo electr\u00F3nico.",
    created: "Creado",
    recipient: "Beneficiario",
    result: "Resultado",
    completed: "Completado de forma duradera",
    inProgress: "En curso",
    changeRoleFor: (email: string) => `Cambiar rol para${email}`,
    accessImmediate: "Accede a los cambios inmediatamente",
    accessDescription: "Cambiar una funci\u00F3n revoca las sesiones actuales de este usuario y los enlaces de configuraci\u00F3n de contrase\u00F1a no utilizados. El \u00FAltimo propietario activo utilizable no se puede degradar.",
    active: "activo",
    disabledStatus: "desactivado",
},
"pt-BR": {
    createdRole: (name: string) => `Fun\u00E7\u00E3o criada${name}.`,
    createdUser: (email: string) => `Criado${email}. Copie o link \u00FAnico agora.`,
    changedRole: (email: string, role: string) => `Mudado${email}para${role}. Sess\u00F5es existentes e links de configura\u00E7\u00E3o de senha n\u00E3o utilizados foram revogados.`,
    generatedLink: (email: string) => `Gerou um novo link de configura\u00E7\u00E3o de senha \u00FAnica para${email}. Links n\u00E3o utilizados anteriormente s\u00E3o inv\u00E1lidos.`,
    reactivated: (email: string) => `Reativado${email}. Eles devem usar o novo link \u00FAnico antes de fazer login.`,
    disabled: (email: string) => `Desabilitado${email}; sess\u00F5es e concess\u00F5es existentes foram revogadas.`,
    acceptedMessage: (email: string) => `SMTP aceitou o convite para${email}.`,
    unknownMessage: (email: string) => `Resultado SMTP para${email}\u00E9 desconhecido. Gere um novo link de senha antes de enviar novamente.`,
    failedMessage: (email: string) => `Entrega de convite para${email}fracassado. O resultado duradouro \u00E9 registrado.`,
    oneTimeLink: "Link de configura\u00E7\u00E3o de senha \u00FAnica",
    bearerDescription: (email: string) => `Este link de portador para${email}\u00E9 exibido apenas nesta resposta. Copie-o para um canal privado ou envie-o explicitamente por meio do servidor SMTP configurado.`,
    copy: "C\u00F3pia",
    acceptedTitle: "SMTP aceitou o convite",
    failedTitle: "Falha na entrega do convite",
    unknownTitle: "O resultado do convite \u00E9 desconhecido",
    unknownDescription: "N\u00E3o reenvie este mesmo link. Gere um novo link de senha antes de outro envio expl\u00EDcito.",
    durableAt: (value: string) => `Resultado dur\u00E1vel registrado em${value}.`,
    sendConfirm: "Enviar este link do portador por e-mail?",
    sendDescription: "Este \u00E9 um envio SMTP externo expl\u00EDcito. Seu resultado aceito, reprovado ou desconhecido ser\u00E1 registrado de forma duradoura.",
    send: "Enviar e-mail de convite",
    dismiss: "Liberar",
    newRole: "Nova fun\u00E7\u00E3o",
    roleName: "Nome da fun\u00E7\u00E3o",
    capabilities: "Capacidades",
    createRole: "Criar fun\u00E7\u00E3o",
    newUser: "Novo usu\u00E1rio",
    email: "E-mail",
    displayName: "Nome de exibi\u00E7\u00E3o",
    role: "Papel",
    createUser: "Criar usu\u00E1rio",
    usersRoles: "Usu\u00E1rios e fun\u00E7\u00F5es",
    refresh: "Atualizar",
    status: "Status",
    authRevision: "Revis\u00E3o de autentica\u00E7\u00E3o",
    actions: "A\u00E7\u00F5es",
    changeRole: "Mudar fun\u00E7\u00E3o",
    history: "Hist\u00F3rico de convites",
    newPasswordLink: "Link para nova senha",
    newPasswordConfirm: "Gerar um novo link de configura\u00E7\u00E3o de senha?",
    newPasswordDescription: "Cada link set-password n\u00E3o utilizado anteriormente para este usu\u00E1rio se tornar\u00E1 inv\u00E1lido.",
    disableConfirm: "Desabilitar este usu\u00E1rio e revogar todas as sess\u00F5es e concess\u00F5es?",
    disable: "Desativar",
    reactivateConfirm: "Reativar este usu\u00E1rio?",
    reactivateDescription: "Ser\u00E1 necess\u00E1rio um novo link de configura\u00E7\u00E3o de senha \u00FAnica; a senha antiga permanece inutiliz\u00E1vel.",
    reactivate: "Reativar",
    historyFor: (email: string) => `Hist\u00F3rico de convites para${email}`,
    evidenceTitle: "A prova de entrega n\u00E3o cont\u00E9m o link do portador",
    evidenceDescription: "Aceito significa que o servidor SMTP aceitou a mensagem. Desconhecido nunca \u00E9 repetido automaticamente; gerar um novo link de senha antes de outro envio.",
    noAttempts: "Nenhuma tentativa de e-mail de convite registrada.",
    created: "Criado",
    recipient: "Destinat\u00E1rio",
    result: "Resultado",
    completed: "Conclu\u00EDdo de forma dur\u00E1vel",
    inProgress: "Em andamento",
    changeRoleFor: (email: string) => `Alterar fun\u00E7\u00E3o para${email}`,
    accessImmediate: "Acesse as altera\u00E7\u00F5es imediatamente",
    accessDescription: "A altera\u00E7\u00E3o de uma fun\u00E7\u00E3o revoga as sess\u00F5es atuais deste usu\u00E1rio e os links de defini\u00E7\u00E3o de senha n\u00E3o utilizados. O \u00FAltimo propriet\u00E1rio ativo utiliz\u00E1vel n\u00E3o pode ser rebaixado.",
    active: "ativo",
    disabledStatus: "desabilitado",
},
};

export function AccessPanel({ locale, onError, onMessage }: Feedback & { locale: AccessLocale }) {
  const text = labels[locale];
  const invalidate = useInvalidate();
  const rolesList = useList<Role, APIError>({ resource: "roles", pagination: { mode: "off" }, queryOptions: { retry: false } });
  const usersList = useList<User, APIError>({ resource: "users", pagination: { mode: "off" }, queryOptions: { retry: false } });
  const roles = rolesList.result.data;
  const users = usersList.result.data;
  const [visibleGrant, setVisibleGrant] = useState<GrantResponse>();
  const [invitationAttempt, setInvitationAttempt] = useState<InvitationMailAttempt>();
  const [sendingInvitation, setSendingInvitation] = useState(false);
  const [invitationHistoryUser, setInvitationHistoryUser] = useState<User>();
  const [invitationHistory, setInvitationHistory] = useState<InvitationMailAttempt[]>([]);
  const [loadingInvitationHistory, setLoadingInvitationHistory] = useState(false);
  const [roleUser, setRoleUser] = useState<User>();
  const [roleForm] = Form.useForm<{ name: string; capabilities: Capability[] }>();
  const [userForm] = Form.useForm<{ email: string; display_name: string; role_id: string }>();
  const [roleEditForm] = Form.useForm<{ role_id: string }>();

  const queryError = rolesList.query.error ?? usersList.query.error;
  useEffect(() => { if (queryError) onError(queryError); }, [queryError]);
  const invalidateResources = (resources: readonly string[]) => Promise.all(
    resources.map((resource) => invalidate({ resource, invalidates: ["list", "detail"] })),
  );
  const load = () => invalidateResources(["roles", "users"]);
  const showGrant = (response: GrantResponse) => {
    setVisibleGrant(response);
    setInvitationAttempt(undefined);
  };
  const dismissGrantFor = (userID: string) => {
    if (visibleGrant?.user.id === userID) {
      setVisibleGrant(undefined);
      setInvitationAttempt(undefined);
    }
  };

  const createRole = async (values: { name: string; capabilities: Capability[] }) => {
    try {
      await accessCommands.createRole(values);
      roleForm.resetFields();
      onMessage(text.createdRole(values.name));
      await invalidateResources(accessInvalidations.roles);
    } catch (error) {
      onError(error);
    }
  };

  const createUser = async (values: { email: string; display_name: string; role_id: string }) => {
    try {
			const response = await accessCommands.createUser(values);
      showGrant(response);
      userForm.resetFields();
      onMessage(text.createdUser(response.user.email));
      await invalidateResources(accessInvalidations.users);
    } catch (error) {
      onError(error);
    }
	};

	const beginRoleChange = (user: User) => {
		setRoleUser(user);
		roleEditForm.setFieldsValue({ role_id: user.role_id });
	};

	const changeRole = async (values: { role_id: string }) => {
		if (!roleUser) return;
		try {
			const updated = await accessCommands.changeRole(roleUser, values.role_id);
			dismissGrantFor(updated.id);
			setRoleUser(undefined);
			roleEditForm.resetFields();
			onMessage(text.changedRole(updated.email, roles.find((role) => role.id === updated.role_id)?.name ?? updated.role_id));
			await invalidateResources(accessInvalidations.users);
		} catch (error) {
			onError(error);
			await invalidateResources(accessInvalidations.users);
		}
	};

	const issueSetPasswordGrant = async (user: User) => {
		try {
			const response = await accessCommands.issueSetPasswordGrant(user);
			showGrant(response);
			onMessage(text.generatedLink(response.user.email));
		} catch (error) {
			onError(error);
		}
	};

	const reactivateUser = async (user: User) => {
		try {
			const response = await accessCommands.reactivateUser(user);
			showGrant(response);
			onMessage(text.reactivated(response.user.email));
			await invalidateResources(accessInvalidations.users);
		} catch (error) {
			onError(error);
			await invalidateResources(accessInvalidations.users);
		}
	};

  const disableUser = async (user: User) => {
    try {
      await accessCommands.disableUser(user);
      dismissGrantFor(user.id);
      onMessage(text.disabled(user.email));
      await invalidateResources(accessInvalidations.users);
    } catch (error) {
      onError(error);
    }
  };

  const sendInvitation = async () => {
    if (!visibleGrant) return;
    setSendingInvitation(true);
    try {
      const attempt = await accessCommands.sendInvitation(visibleGrant);
      setInvitationAttempt(attempt);
      if (attempt.status === "accepted") {
        onMessage(text.acceptedMessage(attempt.recipient_email));
      } else if (attempt.status === "unknown") {
        onMessage(text.unknownMessage(attempt.recipient_email));
      } else {
        onMessage(text.failedMessage(attempt.recipient_email));
      }
    } catch (error) {
      onError(error);
    } finally {
      setSendingInvitation(false);
    }
  };

  const showInvitationHistory = async (user: User) => {
    setInvitationHistoryUser(user);
    setLoadingInvitationHistory(true);
    try {
      const attempts = await accessCommands.invitationHistory(user);
      setInvitationHistory(attempts ?? []);
    } catch (error) {
      setInvitationHistoryUser(undefined);
      onError(error);
    } finally {
      setLoadingInvitationHistory(false);
    }
  };

  return (
    <Space direction="vertical" size="large" className="panel-stack">
      {visibleGrant && (
        <Alert
          type="warning"
          showIcon
          message={text.oneTimeLink}
          description={
            <Space direction="vertical" className="panel-stack">
              <Typography.Text>
                {text.bearerDescription(visibleGrant.user.email)}
              </Typography.Text>
              <Input
                value={visibleGrant.set_password_url}
                readOnly
                addonAfter={<Button type="link" onClick={() => void navigator.clipboard.writeText(visibleGrant.set_password_url)}>{text.copy}</Button>}
              />
              {invitationAttempt && (
                <Alert
                  showIcon
                  type={invitationAttempt.status === "accepted" ? "success" : invitationAttempt.status === "failed" ? "error" : "warning"}
                  message={invitationAttempt.status === "accepted" ? text.acceptedTitle : invitationAttempt.status === "failed" ? text.failedTitle : text.unknownTitle}
                  description={invitationAttempt.status === "unknown"
                    ? text.unknownDescription
                    : invitationAttempt.error_message || text.durableAt(invitationAttempt.completed_at ?? invitationAttempt.created_at)}
                />
              )}
              <Space wrap>
                <Popconfirm
                  title={text.sendConfirm}
                  description={text.sendDescription}
                  onConfirm={() => void sendInvitation()}
                >
                  <Button
                    type="primary"
                    loading={sendingInvitation}
                    disabled={invitationAttempt?.status === "accepted" || invitationAttempt?.status === "unknown"}
                  >
                    {text.send}
                  </Button>
                </Popconfirm>
                <Button size="small" onClick={() => { setVisibleGrant(undefined); setInvitationAttempt(undefined); }}>{text.dismiss}</Button>
              </Space>
            </Space>
          }
        />
      )}
      <Card title={text.newRole}>
        <Form form={roleForm} layout="vertical" onFinish={(values) => void createRole(values)}>
          <div className="form-grid">
            <Form.Item name="name" label={text.roleName} rules={[{ required: true }]}><Input /></Form.Item>
            <Form.Item name="capabilities" label={text.capabilities} rules={[{ required: true }]}>
              <Select mode="multiple" options={capabilities.map((value) => ({ value, label: value }))} />
            </Form.Item>
          </div>
          <Button type="primary" htmlType="submit">{text.createRole}</Button>
        </Form>
      </Card>
      <Card title={text.newUser}>
        <Form form={userForm} layout="vertical" onFinish={(values) => void createUser(values)}>
          <div className="form-grid three-columns">
            <Form.Item name="email" label={text.email} rules={[{ required: true, type: "email" }]}><Input /></Form.Item>
            <Form.Item name="display_name" label={text.displayName} rules={[{ required: true }]}><Input /></Form.Item>
            <Form.Item name="role_id" label={text.role} rules={[{ required: true }]}>
              <Select options={roles.filter((role) => role.status === "active").map((role) => ({ value: role.id, label: role.name }))} />
            </Form.Item>
          </div>
          <Button type="primary" htmlType="submit">{text.createUser}</Button>
        </Form>
      </Card>
      <Card title={text.usersRoles} extra={<Button onClick={() => void load()}>{text.refresh}</Button>}>
        <Table<User>
          rowKey="id"
          dataSource={users}
          loading={rolesList.query.isFetching || usersList.query.isFetching}
          pagination={false}
          expandable={{
            expandedRowRender: (user) => {
              const role = roles.find((item) => item.id === user.role_id);
              return <Space wrap>{role?.capabilities.map((capability) => <Tag key={capability}>{capability}</Tag>)}</Space>;
            },
          }}
          columns={[
            { title: text.email, dataIndex: "email" },
            { title: text.displayName, dataIndex: "display_name" },
            { title: text.role, render: (_, user) => roles.find((role) => role.id === user.role_id)?.name ?? user.role_id },
            { title: text.status, render: (_, user) => <Tag color={user.status === "active" ? "green" : "default"}>{user.status === "active" ? text.active : text.disabledStatus}</Tag> },
            { title: text.authRevision, dataIndex: "auth_revision" },
			{
			  title: text.actions,
			  render: (_, user) => (
				<Space wrap>
				  <Button size="small" onClick={() => beginRoleChange(user)}>{text.changeRole}</Button>
				  <Button size="small" onClick={() => void showInvitationHistory(user)}>{text.history}</Button>
				  {user.status === "active" ? (
					<>
					  <Popconfirm
						title={text.newPasswordConfirm}
						description={text.newPasswordDescription}
						onConfirm={() => void issueSetPasswordGrant(user)}
					  >
						<Button size="small">{text.newPasswordLink}</Button>
					  </Popconfirm>
					  <Popconfirm title={text.disableConfirm} onConfirm={() => void disableUser(user)}>
						<Button size="small" danger>{text.disable}</Button>
					  </Popconfirm>
					</>
				  ) : (
					<Popconfirm
					  title={text.reactivateConfirm}
					  description={text.reactivateDescription}
					  onConfirm={() => void reactivateUser(user)}
					>
					  <Button size="small" type="primary">{text.reactivate}</Button>
					</Popconfirm>
				  )}
				</Space>
			  ),
			},
          ]}
		/>
	 </Card>

      <Modal
        open={invitationHistoryUser !== undefined}
        title={invitationHistoryUser ? text.historyFor(invitationHistoryUser.email) : text.history}
        footer={null}
        width={860}
        onCancel={() => { setInvitationHistoryUser(undefined); setInvitationHistory([]); }}
        destroyOnHidden
      >
        <Alert
          className="bottom-gap"
          type="info"
          showIcon
          message={text.evidenceTitle}
          description={text.evidenceDescription}
        />
        <Table<InvitationMailAttempt>
          rowKey="id"
          loading={loadingInvitationHistory}
          dataSource={invitationHistory}
          pagination={false}
          locale={{ emptyText: text.noAttempts }}
          columns={[
            { title: text.created, render: (_, attempt) => new Date(attempt.created_at).toLocaleString(locale) },
            { title: text.recipient, dataIndex: "recipient_email" },
            {
              title: text.status,
              render: (_, attempt) => (
                <Tag color={attempt.status === "accepted" ? "green" : attempt.status === "failed" ? "red" : attempt.status === "unknown" ? "orange" : "blue"}>
                  {attempt.status === "accepted" ? text.acceptedTitle : attempt.status === "failed" ? text.failedTitle : attempt.status === "unknown" ? text.unknownTitle : text.inProgress}
                </Tag>
              ),
            },
            { title: text.result, render: (_, attempt) => attempt.error_message || (attempt.completed_at ? text.completed : text.inProgress) },
          ]}
        />
      </Modal>

	 <Modal
		open={roleUser !== undefined}
		title={roleUser ? text.changeRoleFor(roleUser.email) : text.changeRole}
		okText={text.changeRole}
		onOk={() => roleEditForm.submit()}
		onCancel={() => { setRoleUser(undefined); roleEditForm.resetFields(); }}
		destroyOnHidden
	  >
		<Alert
		  className="bottom-gap"
		  type="warning"
		  showIcon
		  message={text.accessImmediate}
		  description={text.accessDescription}
		/>
		<Form form={roleEditForm} layout="vertical" onFinish={(values) => void changeRole(values)}>
		  <Form.Item name="role_id" label={text.role} rules={[{ required: true }]}>
			<Select options={roles.filter((role) => role.status === "active").map((role) => ({ value: role.id, label: role.name }))} />
		  </Form.Item>
		</Form>
	  </Modal>
	</Space>
  );
}
