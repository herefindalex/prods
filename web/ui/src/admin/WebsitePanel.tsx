import { useEffect, useState } from "react";
import {
  Alert,
  Button,
  Card,
  Checkbox,
  Descriptions,
  Form,
  Input,
  InputNumber,
  Popconfirm,
  Select,
  Space,
  Table,
  Typography,
  Upload,
} from "antd";
import { api, postJSON, putJSON } from "./api";
import { localeLabel, localeSelectOptions } from "./locales";
import type { AdminLocale } from "./locales";
import { brandImportCopy } from "./brandImportLabels";
import { replaceFormValues } from "./formValues";
import type {
  Asset,
  BrandImportFieldChange,
  BrandImportRequest,
  BrandImportValidation,
  SiteConfiguration,
  SiteRouteConfig,
  SiteRouteIssue,
  SiteRoutePreview,
  SiteRouteState,
  SearchIntegrationSettings,
  WebsiteLocalization,
  WebsiteState,
  WebsiteVersion,
} from "./types";

const websiteLocales = [
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
];

const websiteLocaleOptions = localeSelectOptions(websiteLocales);

type Props = {
  locale: AdminLocale;
  onError: (error: unknown) => void;
  onMessage: (message: string) => void;
};

type NavigationEditorCopy = {
  targetType: string;
  link: string;
  systemAction: string;
  group: string;
  action: string;
  catalog: string;
  catalogSearch: string;
  rfq: string;
  presentation: string;
  direct: string;
  dropdown: string;
};

const navigationEditorCopy: Record<AdminLocale, NavigationEditorCopy> = {
  "en-US": { targetType: "Target type", link: "Link", systemAction: "System action", group: "Group", action: "Action", catalog: "Catalog", catalogSearch: "Catalog search", rfq: "Request quote", presentation: "Presentation", direct: "Direct", dropdown: "Dropdown" },
  "zh-TW": { targetType: "目標類型", link: "連結", systemAction: "系統動作", group: "群組", action: "動作", catalog: "產品目錄", catalogSearch: "目錄搜尋", rfq: "詢價", presentation: "呈現方式", direct: "直接顯示", dropdown: "下拉選單" },
  "zh-CN": { targetType: "目标类型", link: "链接", systemAction: "系统操作", group: "分组", action: "操作", catalog: "产品目录", catalogSearch: "目录搜索", rfq: "询价", presentation: "展示方式", direct: "直接显示", dropdown: "下拉菜单" },
  "ja-JP": { targetType: "リンク先の種類", link: "リンク", systemAction: "システム操作", group: "グループ", action: "操作", catalog: "カタログ", catalogSearch: "カタログ検索", rfq: "見積依頼", presentation: "表示方法", direct: "直接表示", dropdown: "ドロップダウン" },
  "ko-KR": { targetType: "대상 유형", link: "링크", systemAction: "시스템 작업", group: "그룹", action: "작업", catalog: "카탈로그", catalogSearch: "카탈로그 검색", rfq: "견적 요청", presentation: "표시 방식", direct: "직접 표시", dropdown: "드롭다운" },
  "de-DE": { targetType: "Zieltyp", link: "Link", systemAction: "Systemaktion", group: "Gruppe", action: "Aktion", catalog: "Katalog", catalogSearch: "Katalogsuche", rfq: "Angebot anfordern", presentation: "Darstellung", direct: "Direkt", dropdown: "Dropdown" },
  "fr-FR": { targetType: "Type de cible", link: "Lien", systemAction: "Action système", group: "Groupe", action: "Action", catalog: "Catalogue", catalogSearch: "Recherche catalogue", rfq: "Demande de devis", presentation: "Présentation", direct: "Directe", dropdown: "Menu déroulant" },
  "it-IT": { targetType: "Tipo di destinazione", link: "Link", systemAction: "Azione di sistema", group: "Gruppo", action: "Azione", catalog: "Catalogo", catalogSearch: "Ricerca catalogo", rfq: "Richiedi preventivo", presentation: "Presentazione", direct: "Diretta", dropdown: "Menu a discesa" },
  "es-ES": { targetType: "Tipo de destino", link: "Enlace", systemAction: "Acción del sistema", group: "Grupo", action: "Acción", catalog: "Catálogo", catalogSearch: "Buscar en catálogo", rfq: "Solicitar cotización", presentation: "Presentación", direct: "Directa", dropdown: "Menú desplegable" },
  "pt-BR": { targetType: "Tipo de destino", link: "Link", systemAction: "Ação do sistema", group: "Grupo", action: "Ação", catalog: "Catálogo", catalogSearch: "Pesquisa no catálogo", rfq: "Solicitar cotação", presentation: "Apresentação", direct: "Direta", dropdown: "Menu suspenso" },
};

const labels = {
  "en-US": {
    saved: (revision: number) => `Saved Website working revision ${revision}. The public site is unchanged.`,
    uploaded: (name: string) => `Uploaded ${name}. Save the working copy to reference it.`,
    routesChecked: (affected: number, revision: number) => `Checked ${affected} Published product routes against Website working revision ${revision}.`,
    published: (epoch: number) => `Published Website configuration and every Product representation at site epoch ${epoch}.`,
    restored: (version: number, revision: number) => `Restored Website version ${version} into working revision ${revision}. Preview and Publish are still required.`,
    absoluteURL: "Enter an absolute HTTP or HTTPS URL.", validURL: "Enter a valid Website URL.",
    searchSaved: "Search integration settings saved. External submissions run independently.",
    cssState: (disabled: boolean) => disabled ? "Custom CSS Safe Mode is active for all new public requests." : "Custom CSS was re-enabled for new public requests.",
workingConfiguration: "Website working configuration", workingRevision: "Working revision", activeVersion: "Active version", activeEpoch: "Active site epoch",
localization: "Languages and content editing", localizationHelp: "These are Website working settings. Public languages change only after Preview and Publish; disabling editing never unpublishes saved translations.",
defaultLocale: "Site default locale", enabledLocales: "Published locales after next Publish", contentEditingEnabled: "Enable Customer Content translation editing", activeLocales: "Currently public locales", saveLocalization: "Save language working copy", localizationSaved: (revision: number) => `Saved Website language working revision ${revision}. The public site is unchanged.`,
    cssSafeMode: "Custom CSS Safe Mode is active", cssSafeDescription: "Public requests cannot retrieve the active custom stylesheet. Admin and system pages are unaffected.",
    reenableCSS: "Re-enable Custom CSS", disableCSSConfirm: "Immediately stop serving Custom CSS to new public requests?", disableCSS: "Disable Custom CSS now",
    workingHelp: "Save changes into the durable working copy. Preview is private. Only Publish changes the public Website and Product artifacts.", dirtyCapture: "Save or reload local form edits before re-capturing.",
    dirtyCaptureDescription: "The server compares a capture with the durable Website working revision, not unsaved browser fields.", reloadWorking: "Reload saved working",
    organization: "Organization", field: "Field",
    displayName: "Display name", legalName: "Legal name", officialWebsite: "Official website", privacyURL: "Privacy URL", termsURL: "Terms URL",
    primaryLogo: "Primary logo", darkLogo: "Dark-background logo", favicon: "Favicon", socialImage: "Social image", assetID: "asset ID", upload: "Upload",
    contactID: "Contact ID", label: "Label", url: "URL", order: "Order", removeContact: "Remove contact", addContact: "Add contact",
    navigation: "Navigation", id: "ID", parentID: "Parent ID", visible: "Visible", newWindow: "New window", removeLink: "Remove link", addLink: "Add navigation link",
    themeSEO: "Theme and SEO", primaryColor: "Primary color", secondaryColor: "Secondary color", fontFamily: "Font family", contentWidth: "Content width",
    applyCSS: "Apply Custom CSS on public pages", customCSS: "Custom CSS", defaultTitle: "Default title", defaultDescription: "Default description",
    saveWorking: "Save working copy", previewWorking: "Preview saved working copy",
    searchExposure: "Search and AI exposure", optionalEnhancement: "Optional enhancement",
    searchHelp: "HTML, JSON-LD, robots.txt, Sitemap, JSON, manifest, Markdown, and llms.txt remain available without either integration. A successful submission means accepted by the service, never indexed.",
    indexNowTitle: "Bing and participating IndexNow engines", enableIndexNow: "Enable IndexNow change notifications", indexNowUnavailable: "Configure a public HTTPS Base URL before enabling IndexNow.", keyURL: "Public key verification URL",
    googleTitle: "Google Search Console", enableGoogle: "Submit the Sitemap through Search Console", googleUnavailable: "Configure the host OAuth client ID, client-secret file, refresh-token file, and a public HTTPS Base URL first.",
    siteProperty: "Verified Search Console site property", sitePropertyHelp: "Use the exact URL-prefix property such as https://catalog.example.com/ or sc-domain:example.com.", sitePropertyRequired: "A verified site property is required.", saveSearch: "Save search integrations",
    publicRoutes: "Public product routes", activePrefix: "Active prefix", activePattern: "Active pattern",
    routesHelp: "Preview checks every Published Product. Publish installs the working Website, routes, and representations at one site-wide visibility boundary.",
    productPrefix: "Product prefix", safeSegment: "Use one safe path segment beginning with /.", pattern: "Pattern", compact: "Compact", manufacturerPath: "Manufacturer path", brandPath: "Brand path", categoryPath: "Category path", previewSite: "Preview entire site",
    candidateEpoch: (epoch: number) => `Candidate site epoch ${epoch}`, routesReady: (count: number, revision: number) => `${count} routes and Website working revision ${revision} are ready.`,
    cannotPublish: "This candidate cannot be published.", cannotPublishDescription: "Resolve every missing namespace, route conflict, or stale working revision, then preview again.",
    partNumber: "Part number", productID: "Product ID", targetRoute: "Target route", issue: "Issue",
    publishConfirm: "Publish the Website working copy and switch every Published Product at one new site epoch?", publishWebsite: "Publish Website configuration",
    publishedVersions: "Published Website versions", version: "Version", siteEpoch: "Site epoch", created: "Created", action: "Action",
    restoreConfirm: (version: number) => `Restore version ${version} into the working copy? It will not publish automatically.`, restoreWorking: "Restore as working",
  },
  "zh-TW": {
    saved: (revision: number) => `已儲存 Website 工作修訂 ${revision}；公開網站未變更。`,
    uploaded: (name: string) => `已上傳 ${name}；儲存工作副本後才會參照它。`,
    routesChecked: (affected: number, revision: number) => `已用 Website 工作修訂 ${revision} 檢查 ${affected} 條已發布產品路由。`,
    published: (epoch: number) => `已在站點 epoch ${epoch} 發布 Website 設定與所有 Product 表示。`,
    restored: (version: number, revision: number) => `已將 Website 版本 ${version} 還原到工作修訂 ${revision}；仍須預覽及發布。`,
    absoluteURL: "請輸入完整的 HTTP 或 HTTPS URL。", validURL: "請輸入有效的 Website URL。",
    searchSaved: "已儲存搜尋整合設定；外部提交會獨立執行。",
    cssState: (disabled: boolean) => disabled ? "所有新的公開請求已啟用 Custom CSS 安全模式。" : "新的公開請求已重新啟用 Custom CSS。",
workingConfiguration: "Website 工作設定", workingRevision: "工作修訂", activeVersion: "啟用版本", activeEpoch: "啟用站點 epoch",
localization: "語系與內容翻譯編輯", localizationHelp: "這些是 Website 工作設定。公開語系只會在預覽並發布後改變；停用翻譯編輯不會取消已儲存翻譯的公開狀態。",
defaultLocale: "站點預設語系", enabledLocales: "下次發布後的公開語系", contentEditingEnabled: "啟用 Customer Content 翻譯編輯", activeLocales: "目前公開語系", saveLocalization: "儲存語系工作副本", localizationSaved: (revision: number) => `已儲存 Website 語系工作修訂 ${revision}；公開網站未變更。`,
    cssSafeMode: "Custom CSS 安全模式已啟用", cssSafeDescription: "公開請求無法取得啟用中的自訂樣式；Admin 與系統頁面不受影響。",
    reenableCSS: "重新啟用 Custom CSS", disableCSSConfirm: "要立即停止向新的公開請求提供 Custom CSS 嗎？", disableCSS: "立即停用 Custom CSS",
    workingHelp: "先將變更儲存到持久工作副本。預覽是私有的；只有發布才會變更公開 Website 與 Product artifacts。", dirtyCapture: "重新擷取前，請儲存或重新載入本機表單修改。",
    dirtyCaptureDescription: "伺服器會將擷取結果與持久的 Website 工作修訂比較，而不是與未儲存的瀏覽器欄位比較。", reloadWorking: "重新載入已儲存工作副本",
    organization: "組織", field: "欄位",
    displayName: "顯示名稱", legalName: "法定名稱", officialWebsite: "官方網站", privacyURL: "隱私權 URL", termsURL: "條款 URL",
    primaryLogo: "主要 Logo", darkLogo: "深色背景 Logo", favicon: "Favicon", socialImage: "社群圖片", assetID: "資產 ID", upload: "上傳",
    contactID: "聯絡方式 ID", label: "標籤", url: "URL", order: "順序", removeContact: "移除聯絡方式", addContact: "新增聯絡方式",
    navigation: "導覽", id: "ID", parentID: "上層 ID", visible: "顯示", newWindow: "新視窗", removeLink: "移除連結", addLink: "新增導覽連結",
    themeSEO: "主題與 SEO", primaryColor: "主色", secondaryColor: "輔色", fontFamily: "字型", contentWidth: "內容寬度",
    applyCSS: "在公開頁面套用 Custom CSS", customCSS: "Custom CSS", defaultTitle: "預設標題", defaultDescription: "預設描述",
    saveWorking: "儲存工作副本", previewWorking: "預覽已儲存工作副本",
    searchExposure: "搜尋與 AI 曝光", optionalEnhancement: "選用增強功能",
    searchHelp: "即使不啟用任一整合，HTML、JSON-LD、robots.txt、Sitemap、JSON、manifest、Markdown 與 llms.txt 仍可使用。提交成功只表示服務已接受，不代表已建立索引。",
    indexNowTitle: "Bing 與參與 IndexNow 的搜尋引擎", enableIndexNow: "啟用 IndexNow 變更通知", indexNowUnavailable: "請先設定公開 HTTPS Base URL 再啟用 IndexNow。", keyURL: "公開金鑰驗證 URL",
    googleTitle: "Google Search Console", enableGoogle: "透過 Search Console 提交 Sitemap", googleUnavailable: "請先設定主機 OAuth client ID、client-secret 檔、refresh-token 檔及公開 HTTPS Base URL。",
    siteProperty: "已驗證的 Search Console 站點資源", sitePropertyHelp: "請使用完整 URL-prefix 資源，例如 https://catalog.example.com/，或 sc-domain:example.com。", sitePropertyRequired: "必須提供已驗證的站點資源。", saveSearch: "儲存搜尋整合",
    publicRoutes: "公開產品路由", activePrefix: "啟用 prefix", activePattern: "啟用 pattern",
    routesHelp: "預覽會檢查每項已發布 Product。發布會在同一個全站 visibility boundary 啟用工作 Website、路由及表示。",
    productPrefix: "產品 prefix", safeSegment: "請使用以 / 開頭的一個安全路徑片段。", pattern: "Pattern", compact: "簡潔", manufacturerPath: "製造商路徑", brandPath: "品牌路徑", categoryPath: "分類路徑", previewSite: "預覽整個站點",
    candidateEpoch: (epoch: number) => `候選站點 epoch ${epoch}`, routesReady: (count: number, revision: number) => `${count} 條路由與 Website 工作修訂 ${revision} 已就緒。`,
    cannotPublish: "這個候選版本無法發布。", cannotPublishDescription: "請解決所有缺少的命名空間、路由衝突或過期工作修訂，再重新預覽。",
    partNumber: "料號", productID: "Product ID", targetRoute: "目標路由", issue: "問題",
    publishConfirm: "要發布 Website 工作副本，並在新的站點 epoch 切換所有已發布 Product 嗎？", publishWebsite: "發布 Website 設定",
    publishedVersions: "已發布 Website 版本", version: "版本", siteEpoch: "站點 epoch", created: "建立時間", action: "操作",
    restoreConfirm: (version: number) => `要將版本 ${version} 還原到工作副本嗎？這不會自動發布。`, restoreWorking: "還原為工作副本",
  },
"zh-CN": {
    saved: (revision: number) => `\u5DF2\u4FDD\u5B58 Website \u5DE5\u4F5C\u4FEE\u8BA2\u7248${revision}\u3002\u516C\u5171\u7AD9\u70B9\u6CA1\u6709\u53D8\u5316\u3002`,
    uploaded: (name: string) => `\u5DF2\u4E0A\u4F20${name}\u3002\u4FDD\u5B58\u5DE5\u4F5C\u526F\u672C\u4EE5\u4F9B\u53C2\u8003\u3002`,
    routesChecked: (affected: number, revision: number) => `\u5DF2\u68C0\u67E5${affected}\u9488\u5BF9 Website \u5DE5\u4F5C\u4FEE\u8BA2\u7248\u7684 Published \u4EA7\u54C1\u8DEF\u7EBF${revision}.`,
    published: (epoch: number) => `Published Website \u914D\u7F6E\u548C\u7AD9\u70B9\u5386\u5143\u7684\u6BCF\u4E2A Product \u8868\u793A${epoch}.`,
    restored: (version: number, revision: number) => `\u6062\u590DWebsite\u7248\u672C${version}\u8FDB\u5165\u5DE5\u4F5C\u4FEE\u8BA2${revision}\u3002\u4ECD\u7136\u9700\u8981 Preview \u548C Publish\u3002`,
    absoluteURL: "\u8F93\u5165\u7EDD\u5BF9 HTTP \u6216 HTTPS URL\u3002", validURL: "\u8F93\u5165\u6709\u6548\u7684 Website URL\u3002",
    searchSaved: "\u5DF2\u4FDD\u5B58\u641C\u7D22\u96C6\u6210\u8BBE\u7F6E\u3002\u5916\u90E8\u63D0\u4EA4\u72EC\u7ACB\u8FD0\u884C\u3002",
    cssState: (disabled: boolean) => disabled ? "Custom CSS \u5B89\u5168\u6A21\u5F0F\u9002\u7528\u4E8E\u6240\u6709\u65B0\u7684\u516C\u5171\u8BF7\u6C42\u3002" : "Custom CSS \u5DF2\u9488\u5BF9\u65B0\u7684\u516C\u4F17\u8BF7\u6C42\u91CD\u65B0\u542F\u7528\u3002",
    workingConfiguration: "Website\u5DE5\u4F5C\u914D\u7F6E", workingRevision: "\u5DE5\u4F5C\u4FEE\u8BA2", activeVersion: "\u6D3B\u52A8\u7248\u672C", activeEpoch: "\u6D3B\u52A8\u7AD9\u70B9\u7EAA\u5143",
    localization: "\u8BED\u8A00\u548C\u5185\u5BB9\u7F16\u8F91", localizationHelp: "\u8FD9\u4E9B\u662F Website \u7684\u5DE5\u4F5C\u8BBE\u7F6E\u3002\u516C\u5171\u8BED\u8A00\u4EC5\u5728Preview\u548CPublish\u4E4B\u540E\u53D1\u751F\u53D8\u5316\uFF1B\u7981\u7528\u7F16\u8F91\u6C38\u8FDC\u4E0D\u4F1A\u53D6\u6D88\u53D1\u5E03\u5DF2\u4FDD\u5B58\u7684\u7FFB\u8BD1\u3002",
    defaultLocale: "\u7AD9\u70B9\u9ED8\u8BA4\u533A\u57DF\u8BBE\u7F6E", enabledLocales: "\u4E0B\u4E00\u4E2A Publish \u4E4B\u540E\u7684 Published \u533A\u57DF\u8BBE\u7F6E", contentEditingEnabled: "\u542F\u7528\u5BA2\u6237\u5185\u5BB9\u7FFB\u8BD1\u7F16\u8F91", activeLocales: "\u5F53\u524D\u516C\u5171\u573A\u6240", saveLocalization: "\u4FDD\u5B58\u8BED\u8A00\u5DE5\u4F5C\u526F\u672C", localizationSaved: (revision: number) => `\u5DF2\u4FDD\u5B58 Website \u8BED\u8A00\u5DE5\u4F5C\u4FEE\u8BA2\u7248${revision}\u3002\u516C\u5171\u7AD9\u70B9\u6CA1\u6709\u53D8\u5316\u3002`,
    cssSafeMode: "Custom CSS \u5B89\u5168\u6A21\u5F0F\u5DF2\u6FC0\u6D3B", cssSafeDescription: "\u516C\u5171\u8BF7\u6C42\u65E0\u6CD5\u68C0\u7D22\u6D3B\u52A8\u7684\u81EA\u5B9A\u4E49\u6837\u5F0F\u8868\u3002 Admin \u548C\u7CFB\u7EDF\u9875\u9762\u4E0D\u53D7\u5F71\u54CD\u3002",
    reenableCSS: "\u91CD\u65B0\u542F\u7528 Custom CSS", disableCSSConfirm: "\u7ACB\u5373\u505C\u6B62\u4E3A\u65B0\u7684\u516C\u4F17\u8BF7\u6C42\u63D0\u4F9B Custom CSS \u670D\u52A1\u5417\uFF1F", disableCSS: "\u7ACB\u5373\u7981\u7528 Custom CSS",
    workingHelp: "\u5C06\u66F4\u6539\u4FDD\u5B58\u5230\u6301\u4E45\u5DE5\u4F5C\u526F\u672C\u4E2D\u3002 Preview \u662F\u79C1\u6709\u7684\u3002\u4EC5 Publish \u66F4\u6539\u4E86\u516C\u5171 Website \u548C Product \u5DE5\u4EF6\u3002", dirtyCapture: "\u5728\u91CD\u65B0\u6355\u83B7\u4E4B\u524D\u4FDD\u5B58\u6216\u91CD\u65B0\u52A0\u8F7D\u672C\u5730\u8868\u5355\u7F16\u8F91\u3002",
    dirtyCaptureDescription: "\u670D\u52A1\u5668\u5C06\u6355\u83B7\u4E0E\u6301\u4E45\u7684 Website \u5DE5\u4F5C\u7248\u672C\u8FDB\u884C\u6BD4\u8F83\uFF0C\u800C\u4E0D\u662F\u4E0E\u672A\u4FDD\u5B58\u7684\u6D4F\u89C8\u5668\u5B57\u6BB5\u8FDB\u884C\u6BD4\u8F83\u3002", reloadWorking: "\u91CD\u65B0\u52A0\u8F7D\u4FDD\u5B58\u7684\u5DE5\u4F5C",
    organization: "\u7EC4\u7EC7", field: "\u573A\u5730",
    displayName: "\u663E\u793A\u540D\u79F0", legalName: "\u6CD5\u5B9A\u540D\u79F0", officialWebsite: "\u5B98\u65B9\u7F51\u7AD9", privacyURL: "\u9690\u79C1 URL", termsURL: "\u6761\u6B3E URL",
    primaryLogo: "\u4E3B\u8981\u6807\u5FD7", darkLogo: "\u6DF1\u8272\u80CC\u666F\u6807\u5FD7", favicon: "\u7F51\u7AD9\u56FE\u6807", socialImage: "\u793E\u4F1A\u5F62\u8C61", assetID: "\u8D44\u4EA7ID", upload: "\u4E0A\u4F20",
    contactID: "\u8054\u7CFB\u65B9\u5F0F ID", label: "\u6807\u7B7E", url: "URL", order: "\u547D\u4EE4", removeContact: "\u5220\u9664\u8054\u7CFB\u4EBA", addContact: "\u6DFB\u52A0\u8054\u7CFB\u4EBA",
    navigation: "\u5BFC\u822A", id: "ID", parentID: "\u7236ID", visible: "\u53EF\u89C1\u7684", newWindow: "\u65B0\u7A97\u53E3", removeLink: "\u5220\u9664\u94FE\u63A5", addLink: "\u6DFB\u52A0\u5BFC\u822A\u94FE\u63A5",
    themeSEO: "\u4E3B\u9898\u548C\u641C\u7D22\u5F15\u64CE\u4F18\u5316", primaryColor: "\u539F\u8272", secondaryColor: "\u6B21\u8981\u989C\u8272", fontFamily: "\u5B57\u4F53\u5BB6\u65CF", contentWidth: "\u5185\u5BB9\u5BBD\u5EA6",
    applyCSS: "\u5728\u516C\u5171\u9875\u9762\u5E94\u7528Custom CSS", customCSS: "Custom CSS", defaultTitle: "\u9ED8\u8BA4\u6807\u9898", defaultDescription: "\u9ED8\u8BA4\u63CF\u8FF0",
    saveWorking: "", previewWorking: "Preview \u4FDD\u5B58\u7684\u5DE5\u4F5C\u526F\u672C",
    searchExposure: "\u641C\u7D22\u548CAI\u66DD\u5149", optionalEnhancement: "",
    searchHelp: "",
    indexNowTitle: "Bing \u548C\u53C2\u4E0E\u7684 IndexNow \u5F15\u64CE", enableIndexNow: "\u542F\u7528 IndexNow \u66F4\u6539\u901A\u77E5", indexNowUnavailable: "\u5728\u542F\u7528 IndexNow \u4E4B\u524D\u914D\u7F6E\u516C\u5171 HTTPS \u57FA\u7840 URL\u3002", keyURL: "\u516C\u94A5\u9A8C\u8BC1 URL",
    googleTitle: "Google Search Console", enableGoogle: "\u901A\u8FC7 Search Console \u63D0\u4EA4\u7AD9\u70B9\u5730\u56FE", googleUnavailable: "\u9996\u5148\u914D\u7F6E\u4E3B\u673A OAuth \u5BA2\u6237\u7AEF ID\u3001\u5BA2\u6237\u7AEF\u673A\u5BC6\u6587\u4EF6\u3001\u5237\u65B0\u4EE4\u724C\u6587\u4EF6\u548C\u516C\u5171 HTTPS \u57FA\u7840 URL\u3002",
    siteProperty: "\u5DF2\u9A8C\u8BC1\u7684 Search Console \u7F51\u7AD9\u5C5E\u6027", sitePropertyHelp: "\u4F7F\u7528\u786E\u5207\u7684 URL \u524D\u7F00\u5C5E\u6027\uFF0C\u4F8B\u5982 https://catalog.example.com/ \u6216 sc-domain:example.com\u3002", sitePropertyRequired: "\u9700\u8981\u7ECF\u8FC7\u9A8C\u8BC1\u7684\u7AD9\u70B9\u5C5E\u6027\u3002", saveSearch: "\u4FDD\u5B58\u641C\u7D22\u96C6\u6210",
    publicRoutes: "\u516C\u5171\u4EA7\u54C1\u8DEF\u7EBF", activePrefix: "\u6D3B\u52A8\u524D\u7F00", activePattern: "\u6D3B\u52A8\u6A21\u5F0F",
    routesHelp: "Preview \u68C0\u67E5\u6BCF\u4E2A Published Product\u3002 Publish \u5728\u4E00\u4E2A\u7AD9\u70B9\u8303\u56F4\u7684\u53EF\u89C1\u6027\u8FB9\u754C\u4E0A\u5B89\u88C5\u5DE5\u4F5C Website\u3001\u8DEF\u7EBF\u548C\u8868\u793A\u3002",
    productPrefix: "Product \u524D\u7F00", safeSegment: "\u4F7F\u7528\u4E00\u4E2A\u4EE5 / \u5F00\u5934\u7684\u5B89\u5168\u8DEF\u5F84\u6BB5\u3002", pattern: "\u56FE\u6848", compact: "\u8896\u73CD\u7684", manufacturerPath: "\u5382\u5546\u8DEF\u5F84", brandPath: "\u54C1\u724C\u4E4B\u8DEF", categoryPath: "\u7C7B\u522B\u8DEF\u5F84", previewSite: "Preview \u6574\u4E2A\u7F51\u7AD9",
    candidateEpoch: (epoch: number) => `\u5019\u9009\u7AD9\u70B9\u7EAA\u5143${epoch}`, routesReady: (count: number, revision: number) => `${count}\u8DEF\u7EBF\u548CWebsite\u5DE5\u4F5C\u4FEE\u8BA2${revision}\u51C6\u5907\u597D\u4E86\u3002`,
    cannotPublish: "\u8BE5\u5019\u9009\u4EBA\u65E0\u6CD5\u53D1\u8868\u3002", cannotPublishDescription: "\u89E3\u51B3\u6BCF\u4E2A\u4E22\u5931\u7684\u547D\u540D\u7A7A\u95F4\u3001\u8DEF\u7531\u51B2\u7A81\u6216\u8FC7\u65F6\u7684\u5DE5\u4F5C\u4FEE\u8BA2\uFF0C\u7136\u540E\u518D\u6B21\u9884\u89C8\u3002",
    partNumber: "\u96F6\u4EF6\u7F16\u53F7", productID: "Product ID", targetRoute: "\u76EE\u6807\u8DEF\u7EBF", issue: "\u95EE\u9898",
    publishConfirm: "Publish Website \u5DE5\u4F5C\u526F\u672C\u5E76\u5728\u4E00\u4E2A\u65B0\u7AD9\u70B9\u7EAA\u5143\u5207\u6362\u6BCF\u4E2A Published Product \u5417\uFF1F", publishWebsite: "Publish Website \u914D\u7F6E",
    publishedVersions: "Published Website \u7248\u672C", version: "\u7248\u672C", siteEpoch: "\u7AD9\u70B9\u65F6\u4EE3", created: "\u5DF2\u521B\u5EFA", action: "\u884C\u52A8",
    restoreConfirm: (version: number) => `\u6062\u590D\u7248\u672C${version}\u8FDB\u5165\u5DE5\u4F5C\u526F\u672C\uFF1F\u5B83\u4E0D\u4F1A\u81EA\u52A8\u53D1\u5E03\u3002`, restoreWorking: "\u6062\u590D\u6B63\u5E38\u5DE5\u4F5C",
},
"ja-JP": {
    saved: (revision: number) => `\u4FDD\u5B58\u3055\u308C\u305F Website \u4F5C\u696D\u30EA\u30D3\u30B8\u30E7\u30F3${revision}\u3002\u516C\u958B\u30B5\u30A4\u30C8\u306F\u5909\u66F4\u3042\u308A\u307E\u305B\u3093\u3002`,
    uploaded: (name: string) => `\u30A2\u30C3\u30D7\u30ED\u30FC\u30C9\u3055\u308C\u307E\u3057\u305F${name}\u3002\u4F5C\u696D\u30B3\u30D4\u30FC\u3092\u4FDD\u5B58\u3057\u3066\u53C2\u7167\u3057\u307E\u3059\u3002`,
    routesChecked: (affected: number, revision: number) => `\u30C1\u30A7\u30C3\u30AF\u6E08\u307F${affected}Published \u88FD\u54C1\u306F Website \u4F5C\u696D\u30EA\u30D3\u30B8\u30E7\u30F3\u306B\u5BFE\u3057\u3066\u30EB\u30FC\u30C6\u30A3\u30F3\u30B0\u3055\u308C\u307E\u3059${revision}.`,
    published: (epoch: number) => `Published Website \u69CB\u6210\u3068\u30B5\u30A4\u30C8 \u30A8\u30DD\u30C3\u30AF\u3067\u306E\u3059\u3079\u3066\u306E Product \u8868\u73FE${epoch}.`,
    restored: (version: number, revision: number) => `\u5FA9\u5143\u3055\u308C\u305FWebsite\u30D0\u30FC\u30B8\u30E7\u30F3${version}\u5B9F\u7528\u7684\u306A\u30EA\u30D3\u30B8\u30E7\u30F3\u306B${revision}\u3002 Preview \u304A\u3088\u3073 Publish \u306F\u5F15\u304D\u7D9A\u304D\u5FC5\u8981\u3067\u3059\u3002`,
    absoluteURL: "\u7D76\u5BFE HTTP \u307E\u305F\u306F HTTPS URL \u3092\u5165\u529B\u3057\u307E\u3059\u3002", validURL: "\u6709\u52B9\u306A Website URL \u3092\u5165\u529B\u3057\u3066\u304F\u3060\u3055\u3044\u3002",
    searchSaved: "\u691C\u7D22\u7D71\u5408\u8A2D\u5B9A\u304C\u4FDD\u5B58\u3055\u308C\u307E\u3057\u305F\u3002\u5916\u90E8\u304B\u3089\u306E\u63D0\u51FA\u306F\u72EC\u7ACB\u3057\u3066\u5B9F\u884C\u3055\u308C\u307E\u3059\u3002",
    cssState: (disabled: boolean) => disabled ? "Custom CSS \u30BB\u30FC\u30D5 \u30E2\u30FC\u30C9\u306F\u3001\u3059\u3079\u3066\u306E\u65B0\u3057\u3044\u30D1\u30D6\u30EA\u30C3\u30AF \u30EA\u30AF\u30A8\u30B9\u30C8\u306B\u5BFE\u3057\u3066\u30A2\u30AF\u30C6\u30A3\u30D6\u306B\u306A\u308A\u307E\u3059\u3002" : "Custom CSS \u304C\u65B0\u3057\u3044\u30D1\u30D6\u30EA\u30C3\u30AF \u30EA\u30AF\u30A8\u30B9\u30C8\u306B\u5BFE\u3057\u3066\u518D\u3073\u6709\u52B9\u306B\u306A\u308A\u307E\u3057\u305F\u3002",
    workingConfiguration: "Website \u52D5\u4F5C\u69CB\u6210", workingRevision: "\u4F5C\u696D\u30EA\u30D3\u30B8\u30E7\u30F3", activeVersion: "\u30A2\u30AF\u30C6\u30A3\u30D6\u306A\u30D0\u30FC\u30B8\u30E7\u30F3", activeEpoch: "\u30A2\u30AF\u30C6\u30A3\u30D6\u30B5\u30A4\u30C8\u30A8\u30DD\u30C3\u30AF",
    localization: "\u8A00\u8A9E\u3068\u30B3\u30F3\u30C6\u30F3\u30C4\u306E\u7DE8\u96C6", localizationHelp: "\u3053\u308C\u3089\u306F Website \u306E\u52D5\u4F5C\u8A2D\u5B9A\u3067\u3059\u3002\u516C\u958B\u8A00\u8A9E\u306F\u3001Preview \u304A\u3088\u3073 Publish \u306E\u5F8C\u306B\u306E\u307F\u5909\u66F4\u3055\u308C\u307E\u3059\u3002\u7DE8\u96C6\u3092\u7121\u52B9\u306B\u3057\u3066\u3082\u3001\u4FDD\u5B58\u3055\u308C\u305F\u7FFB\u8A33\u304C\u975E\u516C\u958B\u306B\u306A\u308B\u3053\u3068\u306F\u3042\u308A\u307E\u305B\u3093\u3002",
    defaultLocale: "\u30B5\u30A4\u30C8\u306E\u30C7\u30D5\u30A9\u30EB\u30C8\u306E\u30ED\u30B1\u30FC\u30EB", enabledLocales: "\u6B21\u306E Publish \u4EE5\u964D\u306E Published \u30ED\u30B1\u30FC\u30EB", contentEditingEnabled: "\u9867\u5BA2\u30B3\u30F3\u30C6\u30F3\u30C4\u306E\u7FFB\u8A33\u7DE8\u96C6\u3092\u6709\u52B9\u306B\u3059\u308B", activeLocales: "\u73FE\u5728\u516C\u958B\u3055\u308C\u3066\u3044\u308B\u30ED\u30B1\u30FC\u30EB", saveLocalization: "\u8A00\u8A9E\u306E\u4F5C\u696D\u30B3\u30D4\u30FC\u3092\u4FDD\u5B58\u3059\u308B", localizationSaved: (revision: number) => `\u4FDD\u5B58\u3055\u308C\u305F Website \u8A00\u8A9E\u306E\u4F5C\u696D\u30EA\u30D3\u30B8\u30E7\u30F3${revision}\u3002\u516C\u958B\u30B5\u30A4\u30C8\u306F\u5909\u66F4\u3042\u308A\u307E\u305B\u3093\u3002`,
    cssSafeMode: "Custom CSS \u30BB\u30FC\u30D5 \u30E2\u30FC\u30C9\u304C\u30A2\u30AF\u30C6\u30A3\u30D6\u3067\u3059", cssSafeDescription: "\u30D1\u30D6\u30EA\u30C3\u30AF \u30EA\u30AF\u30A8\u30B9\u30C8\u3067\u306F\u3001\u30A2\u30AF\u30C6\u30A3\u30D6\u306A\u30AB\u30B9\u30BF\u30E0 \u30B9\u30BF\u30A4\u30EB\u30B7\u30FC\u30C8\u3092\u53D6\u5F97\u3067\u304D\u307E\u305B\u3093\u3002 Admin \u3068\u30B7\u30B9\u30C6\u30E0 \u30DA\u30FC\u30B8\u306F\u5F71\u97FF\u3092\u53D7\u3051\u307E\u305B\u3093\u3002",
    reenableCSS: "Custom CSS\u3092\u518D\u5EA6\u6709\u52B9\u306B\u3059\u308B", disableCSSConfirm: "\u65B0\u3057\u3044\u30D1\u30D6\u30EA\u30C3\u30AF\u30EA\u30AF\u30A8\u30B9\u30C8\u306B\u5BFE\u3059\u308B Custom CSS \u306E\u63D0\u4F9B\u3092\u76F4\u3061\u306B\u505C\u6B62\u3057\u307E\u3059\u304B?", disableCSS: "\u4ECA\u3059\u3050 Custom CSS \u3092\u7121\u52B9\u306B\u3057\u3066\u304F\u3060\u3055\u3044",
    workingHelp: "\u5909\u66F4\u3092\u6C38\u7D9A\u7684\u306A\u4F5C\u696D\u30B3\u30D4\u30FC\u306B\u4FDD\u5B58\u3057\u307E\u3059\u3002 Preview\u306F\u975E\u516C\u958B\u3067\u3059\u3002 Publish \u306E\u307F\u304C\u30D1\u30D6\u30EA\u30C3\u30AF Website \u304A\u3088\u3073 Product \u30A2\u30FC\u30C6\u30A3\u30D5\u30A1\u30AF\u30C8\u3092\u5909\u66F4\u3057\u307E\u3059\u3002", dirtyCapture: "\u518D\u30AD\u30E3\u30D7\u30C1\u30E3\u3059\u308B\u524D\u306B\u3001\u30ED\u30FC\u30AB\u30EB \u30D5\u30A9\u30FC\u30E0\u306E\u7DE8\u96C6\u3092\u4FDD\u5B58\u307E\u305F\u306F\u518D\u30ED\u30FC\u30C9\u3057\u307E\u3059\u3002",
    dirtyCaptureDescription: "\u30B5\u30FC\u30D0\u30FC\u306F\u3001\u4FDD\u5B58\u3055\u308C\u3066\u3044\u306A\u3044\u30D6\u30E9\u30A6\u30B6 \u30D5\u30A3\u30FC\u30EB\u30C9\u3067\u306F\u306A\u304F\u3001\u30AD\u30E3\u30D7\u30C1\u30E3\u3092\u6C38\u7D9A\u7684\u306A Website \u4F5C\u696D\u30EA\u30D3\u30B8\u30E7\u30F3\u3068\u6BD4\u8F03\u3057\u307E\u3059\u3002", reloadWorking: "\u4FDD\u5B58\u3055\u308C\u305F\u4F5C\u696D\u3092\u30EA\u30ED\u30FC\u30C9\u3059\u308B",
    organization: "\u7D44\u7E54", field: "\u5206\u91CE",
    displayName: "\u8868\u793A\u540D", legalName: "\u6B63\u5F0F\u540D\u79F0", officialWebsite: "\u516C\u5F0F\u30B5\u30A4\u30C8", privacyURL: "\u30D7\u30E9\u30A4\u30D0\u30B7\u30FC URL", termsURL: "\u898F\u7D04 URL",
    primaryLogo: "\u30D7\u30E9\u30A4\u30DE\u30EA\u30ED\u30B4", darkLogo: "\u6697\u3044\u80CC\u666F\u306E\u30ED\u30B4", favicon: "\u30D5\u30A1\u30D3\u30B3\u30F3", socialImage: "\u793E\u4F1A\u7684\u30A4\u30E1\u30FC\u30B8", assetID: "\u8CC7\u7523 ID", upload: "\u30A2\u30C3\u30D7\u30ED\u30FC\u30C9",
    contactID: "\u304A\u554F\u3044\u5408\u308F\u305B", label: "\u30E9\u30D9\u30EB", url: "URL", order: "\u6CE8\u6587", removeContact: "\u9023\u7D61\u5148\u3092\u524A\u9664\u3059\u308B", addContact: "\u9023\u7D61\u5148\u3092\u8FFD\u52A0",
    navigation: "\u30CA\u30D3\u30B2\u30FC\u30B7\u30E7\u30F3", id: "ID", parentID: "\u89AA ID", visible: "\u898B\u3048\u308B", newWindow: "\u65B0\u3057\u3044\u30A6\u30A3\u30F3\u30C9\u30A6", removeLink: "\u30EA\u30F3\u30AF\u3092\u524A\u9664\u3059\u308B", addLink: "\u30CA\u30D3\u30B2\u30FC\u30B7\u30E7\u30F3\u30EA\u30F3\u30AF\u3092\u8FFD\u52A0",
    themeSEO: "\u30C6\u30FC\u30DE\u3068SEO", primaryColor: "\u539F\u8272", secondaryColor: "\u4E8C\u6B21\u8272", fontFamily: "\u30D5\u30A9\u30F3\u30C8\u30D5\u30A1\u30DF\u30EA\u30FC", contentWidth: "\u30B3\u30F3\u30C6\u30F3\u30C4\u306E\u5E45",
    applyCSS: "\u516C\u958B\u30DA\u30FC\u30B8\u306B Custom CSS \u3092\u9069\u7528\u3059\u308B", customCSS: "Custom CSS", defaultTitle: "\u30C7\u30D5\u30A9\u30EB\u30C8\u306E\u30BF\u30A4\u30C8\u30EB", defaultDescription: "\u30C7\u30D5\u30A9\u30EB\u30C8\u306E\u8AAC\u660E",
    saveWorking: "\u4F5C\u696D\u30B3\u30D4\u30FC\u3092\u4FDD\u5B58\u3059\u308B", previewWorking: "Preview \u306B\u4FDD\u5B58\u3055\u308C\u305F\u4F5C\u696D\u30B3\u30D4\u30FC",
    searchExposure: "\u691C\u7D22\u3068 AI \u306E\u9732\u51FA", optionalEnhancement: "\u30AA\u30D7\u30B7\u30E7\u30F3\u306E\u5F37\u5316",
    searchHelp: "HTML\u3001JSON-LD\u3001robots.txt\u3001\u30B5\u30A4\u30C8\u30DE\u30C3\u30D7\u3001JSON\u3001\u30DE\u30CB\u30D5\u30A7\u30B9\u30C8\u3001Markdown\u3001\u304A\u3088\u3073 llms.txt \u306F\u3001\u3044\u305A\u308C\u306E\u7D71\u5408\u3082\u884C\u308F\u306A\u304F\u3066\u3082\u5F15\u304D\u7D9A\u304D\u4F7F\u7528\u3067\u304D\u307E\u3059\u3002\u9001\u4FE1\u304C\u6210\u529F\u3059\u308B\u3068\u3001\u30B5\u30FC\u30D3\u30B9\u306B\u3088\u3063\u3066\u53D7\u3051\u5165\u308C\u3089\u308C\u3001\u30A4\u30F3\u30C7\u30C3\u30AF\u30B9\u304C\u4F5C\u6210\u3055\u308C\u306A\u3044\u3053\u3068\u3092\u610F\u5473\u3057\u307E\u3059\u3002",
    indexNowTitle: "Bing \u304A\u3088\u3073\u53C2\u52A0\u3057\u3066\u3044\u308B IndexNow \u30A8\u30F3\u30B8\u30F3", enableIndexNow: "IndexNow \u5909\u66F4\u901A\u77E5\u3092\u6709\u52B9\u306B\u3059\u308B", indexNowUnavailable: "IndexNow \u3092\u6709\u52B9\u306B\u3059\u308B\u524D\u306B\u3001\u30D1\u30D6\u30EA\u30C3\u30AF HTTPS \u30D9\u30FC\u30B9 URL \u3092\u69CB\u6210\u3057\u307E\u3059\u3002", keyURL: "\u516C\u958B\u9375\u691C\u8A3C URL",
    googleTitle: "Google Search Console", enableGoogle: "Search Console \u304B\u3089\u30B5\u30A4\u30C8\u30DE\u30C3\u30D7\u3092\u9001\u4FE1\u3059\u308B", googleUnavailable: "\u307E\u305A\u3001\u30DB\u30B9\u30C8 OAuth \u30AF\u30E9\u30A4\u30A2\u30F3\u30C8 ID\u3001\u30AF\u30E9\u30A4\u30A2\u30F3\u30C8 \u30B7\u30FC\u30AF\u30EC\u30C3\u30C8 \u30D5\u30A1\u30A4\u30EB\u3001\u30EA\u30D5\u30EC\u30C3\u30B7\u30E5 \u30C8\u30FC\u30AF\u30F3 \u30D5\u30A1\u30A4\u30EB\u3001\u304A\u3088\u3073\u30D1\u30D6\u30EA\u30C3\u30AF HTTPS \u30D9\u30FC\u30B9 URL \u3092\u69CB\u6210\u3057\u307E\u3059\u3002",
    siteProperty: "Search Console \u30B5\u30A4\u30C8\u306E\u30D7\u30ED\u30D1\u30C6\u30A3\u3092\u78BA\u8A8D\u3059\u308B", sitePropertyHelp: "https://catalog.example.com/ \u3084 sc-domain:example.com \u306A\u3069\u3001\u6B63\u78BA\u306A URL-prefix \u30D7\u30ED\u30D1\u30C6\u30A3\u3092\u4F7F\u7528\u3057\u307E\u3059\u3002", sitePropertyRequired: "\u691C\u8A3C\u6E08\u307F\u306E\u30B5\u30A4\u30C8 \u30D7\u30ED\u30D1\u30C6\u30A3\u304C\u5FC5\u8981\u3067\u3059\u3002", saveSearch: "\u691C\u7D22\u7D71\u5408\u306E\u4FDD\u5B58",
    publicRoutes: "\u30D1\u30D6\u30EA\u30C3\u30AF\u30D7\u30ED\u30C0\u30AF\u30C8\u30EB\u30FC\u30C8", activePrefix: "\u30A2\u30AF\u30C6\u30A3\u30D6\u306A\u30D7\u30EC\u30D5\u30A3\u30C3\u30AF\u30B9", activePattern: "\u30A2\u30AF\u30C6\u30A3\u30D6\u30D1\u30BF\u30FC\u30F3",
    routesHelp: "Preview \u306F\u3001\u3059\u3079\u3066\u306E Published Product \u3092\u30C1\u30A7\u30C3\u30AF\u3057\u307E\u3059\u3002 Publish \u306F\u3001\u30B5\u30A4\u30C8\u5168\u4F53\u306E 1 \u3064\u306E\u53EF\u8996\u6027\u5883\u754C\u306B\u3001\u52D5\u4F5C\u3059\u308B Website\u3001\u30EB\u30FC\u30C8\u3001\u304A\u3088\u3073\u8868\u73FE\u3092\u30A4\u30F3\u30B9\u30C8\u30FC\u30EB\u3057\u307E\u3059\u3002",
    productPrefix: "Product \u30D7\u30EC\u30D5\u30A3\u30C3\u30AF\u30B9", safeSegment: "/ \u3067\u59CB\u307E\u308B 1 \u3064\u306E\u5B89\u5168\u306A\u30D1\u30B9 \u30BB\u30B0\u30E1\u30F3\u30C8\u3092\u4F7F\u7528\u3057\u307E\u3059\u3002", pattern: "\u30D1\u30BF\u30FC\u30F3", compact: "\u30B3\u30F3\u30D1\u30AF\u30C8", manufacturerPath: "\u30E1\u30FC\u30AB\u30FC\u30D1\u30B9", brandPath: "\u30D6\u30E9\u30F3\u30C9\u30D1\u30B9", categoryPath: "\u30AB\u30C6\u30B4\u30EA\u30D1\u30B9", previewSite: "Preview \u30B5\u30A4\u30C8\u5168\u4F53",
    candidateEpoch: (epoch: number) => `\u5019\u88DC\u5730\u306E\u30A8\u30DD\u30C3\u30AF${epoch}`, routesReady: (count: number, revision: number) => `${count}\u30EB\u30FC\u30C8\u3068 Website \u4F5C\u696D\u30EA\u30D3\u30B8\u30E7\u30F3${revision}\u6E96\u5099\u304C\u3067\u304D\u3066\u3044\u307E\u3059\u3002`,
    cannotPublish: "\u3053\u306E\u5019\u88DC\u306F\u516C\u958B\u3067\u304D\u307E\u305B\u3093\u3002", cannotPublishDescription: "\u6B20\u843D\u3057\u3066\u3044\u308B\u540D\u524D\u7A7A\u9593\u3001\u30EB\u30FC\u30C8\u306E\u7AF6\u5408\u3001\u307E\u305F\u306F\u53E4\u3044\u4F5C\u696D\u30EA\u30D3\u30B8\u30E7\u30F3\u3092\u3059\u3079\u3066\u89E3\u6C7A\u3057\u3066\u304B\u3089\u3001\u518D\u5EA6\u30D7\u30EC\u30D3\u30E5\u30FC\u3057\u307E\u3059\u3002",
    partNumber: "\u90E8\u54C1\u756A\u53F7", productID: "Product ID", targetRoute: "\u5BFE\u8C61\u30EB\u30FC\u30C8", issue: "\u554F\u984C",
    publishConfirm: "Publish Website \u4F5C\u696D\u30B3\u30D4\u30FC\u3092\u4F5C\u6210\u3057\u3001\u65B0\u3057\u3044\u30B5\u30A4\u30C8 \u30A8\u30DD\u30C3\u30AF\u3054\u3068\u306B Published Product \u3054\u3068\u306B\u5207\u308A\u66FF\u3048\u307E\u3059\u304B?", publishWebsite: "Publish Website \u69CB\u6210",
    publishedVersions: "Published Website \u30D0\u30FC\u30B8\u30E7\u30F3", version: "\u30D0\u30FC\u30B8\u30E7\u30F3", siteEpoch: "\u30B5\u30A4\u30C8\u30A8\u30DD\u30C3\u30AF", created: "\u4F5C\u6210\u3055\u308C\u307E\u3057\u305F", action: "\u30A2\u30AF\u30B7\u30E7\u30F3",
    restoreConfirm: (version: number) => `\u30D0\u30FC\u30B8\u30E7\u30F3\u3092\u5FA9\u5143\u3059\u308B${version}\u4F5C\u696D\u30B3\u30D4\u30FC\u306B\u5165\u308C\u307E\u3059\u304B\uFF1F\u81EA\u52D5\u7684\u306B\u306F\u516C\u958B\u3055\u308C\u307E\u305B\u3093\u3002`, restoreWorking: "\u52D5\u4F5C\u4E2D\u306E\u3082\u306E\u3068\u3057\u3066\u5FA9\u5143",
},
"ko-KR": {
    saved: (revision: number) => `Website \uC791\uC5C5 \uAC1C\uC815\uD310\uC744 \uC800\uC7A5\uD588\uC2B5\uB2C8\uB2E4.${revision}. \uACF5\uAC1C \uC0AC\uC774\uD2B8\uB294 \uBCC0\uACBD\uB418\uC9C0 \uC54A\uC558\uC2B5\uB2C8\uB2E4.`,
    uploaded: (name: string) => `\uC5C5\uB85C\uB4DC\uB428${name}. \uCC38\uC870\uD560 \uC218 \uC788\uB3C4\uB85D \uC791\uC5C5 \uBCF5\uC0AC\uBCF8\uC744 \uC800\uC7A5\uD569\uB2C8\uB2E4.`,
    routesChecked: (affected: number, revision: number) => `\uCCB4\uD06C\uB428${affected}Website \uC791\uC5C5 \uAC1C\uC815\uD310\uC5D0 \uB300\uD55C Published \uC81C\uD488 \uACBD\uB85C${revision}.`,
    published: (epoch: number) => `Published Website \uAD6C\uC131 \uBC0F \uC0AC\uC774\uD2B8 \uC5D0\uD3EC\uD06C\uC758 \uBAA8\uB4E0 Product \uD45C\uD604${epoch}.`,
    restored: (version: number, revision: number) => `Website \uBC84\uC804 \uBCF5\uC6D0${version}\uC791\uC5C5 \uAC1C\uC815\uC5D0${revision}. Preview \uBC0F Publish\uB294 \uC5EC\uC804\uD788 \uD544\uC694\uD569\uB2C8\uB2E4.`,
    absoluteURL: "\uC808\uB300 HTTP \uB610\uB294 HTTPS URL\uB97C \uC785\uB825\uD558\uC138\uC694.", validURL: "\uC720\uD6A8\uD55C Website URL\uB97C \uC785\uB825\uD558\uC138\uC694.",
    searchSaved: "\uAC80\uC0C9 \uD1B5\uD569 \uC124\uC815\uC774 \uC800\uC7A5\uB418\uC5C8\uC2B5\uB2C8\uB2E4. \uC678\uBD80 \uC81C\uCD9C\uC740 \uB3C5\uB9BD\uC801\uC73C\uB85C \uC2E4\uD589\uB429\uB2C8\uB2E4.",
    cssState: (disabled: boolean) => disabled ? "Custom CSS \uC548\uC804 \uBAA8\uB4DC\uB294 \uBAA8\uB4E0 \uC0C8\uB85C\uC6B4 \uACF5\uAC1C \uC694\uCCAD\uC5D0 \uB300\uD574 \uD65C\uC131\uD654\uB429\uB2C8\uB2E4." : "\uC0C8\uB85C\uC6B4 \uACF5\uAC1C \uC694\uCCAD\uC5D0 \uB300\uD574 Custom CSS\uAC00 \uB2E4\uC2DC \uD65C\uC131\uD654\uB418\uC5C8\uC2B5\uB2C8\uB2E4.",
    workingConfiguration: "Website \uC791\uC5C5 \uAD6C\uC131", workingRevision: "\uC791\uC5C5 \uAC1C\uC815", activeVersion: "\uD65C\uC131 \uBC84\uC804", activeEpoch: "\uD65C\uC131 \uC0AC\uC774\uD2B8 \uC2DC\uB300",
    localization: "\uC5B8\uC5B4 \uBC0F \uCF58\uD150\uCE20 \uD3B8\uC9D1", localizationHelp: "\uC774\uB294 Website \uC791\uC5C5 \uC124\uC815\uC785\uB2C8\uB2E4. \uACF5\uC6A9 \uC5B8\uC5B4\uB294 Preview \uBC0F Publish \uC774\uD6C4\uC5D0\uB9CC \uBCC0\uACBD\uB429\uB2C8\uB2E4. \uD3B8\uC9D1\uC744 \uBE44\uD65C\uC131\uD654\uD574\uB3C4 \uC800\uC7A5\uB41C \uBC88\uC5ED\uC740 \uAC8C\uC2DC \uCDE8\uC18C\uB418\uC9C0 \uC54A\uC2B5\uB2C8\uB2E4.",
    defaultLocale: "\uC0AC\uC774\uD2B8 \uAE30\uBCF8 \uB85C\uCF00\uC77C", enabledLocales: "\uB2E4\uC74C Publish \uC774\uD6C4\uC758 Published \uB85C\uCF00\uC77C", contentEditingEnabled: "\uACE0\uAC1D \uCF58\uD150\uCE20 \uBC88\uC5ED \uD3B8\uC9D1 \uD65C\uC131\uD654", activeLocales: "\uD604\uC7AC \uACF5\uAC1C \uB85C\uCF00\uC77C", saveLocalization: "\uC5B8\uC5B4 \uC791\uC5C5 \uC0AC\uBCF8 \uC800\uC7A5", localizationSaved: (revision: number) => `\uC800\uC7A5\uB41C Website \uC5B8\uC5B4 \uC791\uC5C5 \uAC1C\uC815\uD310${revision}. \uACF5\uAC1C \uC0AC\uC774\uD2B8\uB294 \uBCC0\uACBD\uB418\uC9C0 \uC54A\uC558\uC2B5\uB2C8\uB2E4.`,
    cssSafeMode: "Custom CSS \uC548\uC804 \uBAA8\uB4DC\uAC00 \uD65C\uC131\uD654\uB418\uC5C8\uC2B5\uB2C8\uB2E4.", cssSafeDescription: "\uACF5\uAC1C \uC694\uCCAD\uC740 \uD65C\uC131 \uC0AC\uC6A9\uC790 \uC815\uC758 \uC2A4\uD0C0\uC77C\uC2DC\uD2B8\uB97C \uAC80\uC0C9\uD560 \uC218 \uC5C6\uC2B5\uB2C8\uB2E4. Admin \uBC0F \uC2DC\uC2A4\uD15C \uD398\uC774\uC9C0\uB294 \uC601\uD5A5\uC744 \uBC1B\uC9C0 \uC54A\uC2B5\uB2C8\uB2E4.",
    reenableCSS: "Custom CSS\uB97C \uB2E4\uC2DC \uD65C\uC131\uD654\uD569\uB2C8\uB2E4.", disableCSSConfirm: "\uC0C8\uB85C\uC6B4 \uACF5\uAC1C \uC694\uCCAD\uC5D0 \uB300\uD574 Custom CSS \uC81C\uACF5\uC744 \uC989\uC2DC \uC911\uB2E8\uD558\uC2DC\uACA0\uC2B5\uB2C8\uAE4C?", disableCSS: "\uC9C0\uAE08 Custom CSS \uBE44\uD65C\uC131\uD654",
    workingHelp: "\uC9C0\uC18D \uAC00\uB2A5\uD55C \uC791\uC5C5 \uBCF5\uC0AC\uBCF8\uC5D0 \uBCC0\uACBD \uC0AC\uD56D\uC744 \uC800\uC7A5\uD569\uB2C8\uB2E4. Preview\uB294 \uBE44\uACF5\uAC1C\uC785\uB2C8\uB2E4. Publish\uB9CC\uC774 \uACF5\uAC1C Website \uBC0F Product \uC544\uD2F0\uD329\uD2B8\uB97C \uBCC0\uACBD\uD569\uB2C8\uB2E4.", dirtyCapture: "\uB2E4\uC2DC \uCEA1\uCC98\uD558\uAE30 \uC804\uC5D0 \uB85C\uCEEC \uC591\uC2DD \uD3B8\uC9D1 \uB0B4\uC6A9\uC744 \uC800\uC7A5\uD558\uAC70\uB098 \uB2E4\uC2DC \uB85C\uB4DC\uD558\uC138\uC694.",
    dirtyCaptureDescription: "\uC11C\uBC84\uB294 \uC800\uC7A5\uB418\uC9C0 \uC54A\uC740 \uBE0C\uB77C\uC6B0\uC800 \uD544\uB4DC\uAC00 \uC544\uB2CC \uB0B4\uAD6C\uC131 \uC788\uB294 Website \uC791\uC5C5 \uAC1C\uC815\uACFC \uCEA1\uCC98\uB97C \uBE44\uAD50\uD569\uB2C8\uB2E4.", reloadWorking: "\uC800\uC7A5\uB41C \uC791\uC5C5 \uB2E4\uC2DC \uB85C\uB4DC",
    organization: "\uC870\uC9C1", field: "\uD544\uB4DC",
    displayName: "\uD45C\uC2DC \uC774\uB984", legalName: "\uBC95\uC801 \uC774\uB984", officialWebsite: "\uACF5\uC2DD \uD648\uD398\uC774\uC9C0", privacyURL: "\uAC1C\uC778 \uC815\uBCF4 \uBCF4\uD638 URL", termsURL: "\uC774\uC6A9\uC57D\uAD00 URL",
    primaryLogo: "\uAE30\uBCF8 \uB85C\uACE0", darkLogo: "\uC5B4\uB450\uC6B4 \uBC30\uACBD\uC758 \uB85C\uACE0", favicon: "\uD30C\uBE44\uCF58", socialImage: "\uC18C\uC15C \uC774\uBBF8\uC9C0", assetID: "\uC790\uC0B0 ID", upload: "\uC5C5\uB85C\uB4DC",
    contactID: "ID\uC5D0 \uBB38\uC758\uD558\uC138\uC694", label: "\uC0C1\uD45C", url: "URL", order: "\uC8FC\uBB38\uD558\uB2E4", removeContact: "\uC5F0\uB77D\uCC98 \uC0AD\uC81C", addContact: "\uC5F0\uB77D\uCC98 \uCD94\uAC00",
    navigation: "\uD56D\uD574", id: "ID", parentID: "\uC0C1\uC704 ID", visible: "\uBCF4\uC774\uB294", newWindow: "\uC0C8 \uCC3D", removeLink: "\uB9C1\uD06C \uC0AD\uC81C", addLink: "\uD0D0\uC0C9 \uB9C1\uD06C \uCD94\uAC00",
    themeSEO: "\uD14C\uB9C8 \uBC0F SEO", primaryColor: "\uC6D0\uC0C9", secondaryColor: "\uBCF4\uC870 \uC0C9\uC0C1", fontFamily: "\uAE00\uAF34 \uACC4\uC5F4", contentWidth: "\uCF58\uD150\uCE20 \uB108\uBE44",
    applyCSS: "\uACF5\uAC1C \uD398\uC774\uC9C0\uC5D0 Custom CSS \uC801\uC6A9", customCSS: "Custom CSS", defaultTitle: "\uAE30\uBCF8 \uC81C\uBAA9", defaultDescription: "\uAE30\uBCF8 \uC124\uBA85",
    saveWorking: "\uC791\uC5C5 \uBCF5\uC0AC\uBCF8 \uC800\uC7A5", previewWorking: "Preview \uC800\uC7A5\uB41C \uC791\uC5C5 \uBCF5\uC0AC\uBCF8",
    searchExposure: "\uAC80\uC0C9\uACFC AI \uB178\uCD9C", optionalEnhancement: "\uC120\uD0DD\uC801 \uAC1C\uC120",
    searchHelp: "HTML, JSON-LD, robots.txt, Sitemap, JSON, \uB9E4\uB2C8\uD398\uC2A4\uD2B8, Markdown \uBC0F llms.txt\uB294 \uD1B5\uD569 \uC5C6\uC774\uB3C4 \uACC4\uC18D \uC0AC\uC6A9\uD560 \uC218 \uC788\uC2B5\uB2C8\uB2E4. \uC131\uACF5\uC801\uC778 \uC81C\uCD9C\uC740 \uC11C\uBE44\uC2A4\uC5D0\uC11C \uC2B9\uC778\uB418\uC5C8\uC73C\uBA70 \uC0C9\uC778\uC774 \uC0DD\uC131\uB418\uC9C0 \uC54A\uC558\uC74C\uC744 \uC758\uBBF8\uD569\uB2C8\uB2E4.",
    indexNowTitle: "Bing \uBC0F \uCC38\uC5EC IndexNow \uC5D4\uC9C4", enableIndexNow: "IndexNow \uBCC0\uACBD \uC54C\uB9BC \uD65C\uC131\uD654", indexNowUnavailable: "IndexNow\uB97C \uD65C\uC131\uD654\uD558\uAE30 \uC804\uC5D0 \uACF5\uAC1C HTTPS \uBCA0\uC774\uC2A4 URL\uB97C \uAD6C\uC131\uD558\uC2ED\uC2DC\uC624.", keyURL: "\uACF5\uAC1C\uD0A4 \uAC80\uC99D URL",
    googleTitle: "Google Search Console", enableGoogle: "Search Console\uC744 \uD1B5\uD574 \uC0AC\uC774\uD2B8\uB9F5 \uC81C\uCD9C", googleUnavailable: "\uBA3C\uC800 \uD638\uC2A4\uD2B8 OAuth \uD074\uB77C\uC774\uC5B8\uD2B8 ID, \uD074\uB77C\uC774\uC5B8\uD2B8 \uBE44\uBC00 \uD30C\uC77C, \uC0C8\uB85C \uACE0\uCE68 \uD1A0\uD070 \uD30C\uC77C \uBC0F \uACF5\uC6A9 HTTPS \uAE30\uBCF8 URL\uB97C \uAD6C\uC131\uD569\uB2C8\uB2E4.",
    siteProperty: "\uD655\uC778\uB41C Search Console \uC0AC\uC774\uD2B8 \uC18D\uC131", sitePropertyHelp: "https://catalog.example.com/ \uB610\uB294 sc-domain:example.com\uACFC \uAC19\uC740 \uC815\uD655\uD55C URL \uC811\uB450\uC0AC \uC18D\uC131\uC744 \uC0AC\uC6A9\uD558\uC138\uC694.", sitePropertyRequired: "\uD655\uC778\uB41C \uC0AC\uC774\uD2B8 \uC18D\uC131\uC774 \uD544\uC694\uD569\uB2C8\uB2E4.", saveSearch: "\uAC80\uC0C9 \uD1B5\uD569 \uC800\uC7A5",
    publicRoutes: "\uACF5\uAC1C \uC81C\uD488 \uACBD\uB85C", activePrefix: "\uD65C\uC131 \uC811\uB450\uC0AC", activePattern: "\uD65C\uC131 \uD328\uD134",
    routesHelp: "Preview\uB294 \uBAA8\uB4E0 Published Product\uB97C \uD655\uC778\uD569\uB2C8\uB2E4. Publish\uB294 \uD558\uB098\uC758 \uC0AC\uC774\uD2B8 \uC804\uCCB4 \uAC00\uC2DC\uC131 \uACBD\uACC4\uC5D0 \uC791\uB3D9\uD558\uB294 Website, \uACBD\uB85C \uBC0F \uD45C\uD604\uC744 \uC124\uCE58\uD569\uB2C8\uB2E4.",
    productPrefix: "Product \uC811\uB450\uC0AC", safeSegment: "/\uB85C \uC2DC\uC791\uD558\uB294 \uD558\uB098\uC758 \uC548\uC804\uD55C \uACBD\uB85C \uC138\uADF8\uBA3C\uD2B8\uB97C \uC0AC\uC6A9\uD558\uC2ED\uC2DC\uC624.", pattern: "\uBB34\uB2AC", compact: "\uCF64\uD329\uD2B8", manufacturerPath: "\uC81C\uC870\uC5C5\uCCB4 \uACBD\uB85C", brandPath: "\uBE0C\uB79C\uB4DC \uACBD\uB85C", categoryPath: "\uCE74\uD14C\uACE0\uB9AC \uACBD\uB85C", previewSite: "Preview \uC804\uCCB4 \uC0AC\uC774\uD2B8",
    candidateEpoch: (epoch: number) => `\uD6C4\uBCF4 \uC0AC\uC774\uD2B8 \uC2DC\uB300${epoch}`, routesReady: (count: number, revision: number) => `${count}\uB178\uC120 \uBC0F Website \uC791\uC5C5 \uAC1C\uC815${revision}\uC900\uBE44\uB418\uC5C8\uC2B5\uB2C8\uB2E4.`,
    cannotPublish: "\uC774 \uD6C4\uBCF4\uB294 \uAC8C\uC2DC\uD560 \uC218 \uC5C6\uC2B5\uB2C8\uB2E4.", cannotPublishDescription: "\uB204\uB77D\uB41C \uBAA8\uB4E0 \uB124\uC784\uC2A4\uD398\uC774\uC2A4, \uACBD\uB85C \uCDA9\uB3CC \uB610\uB294 \uC624\uB798\uB41C \uC791\uC5C5 \uAC1C\uC815\uC744 \uD574\uACB0\uD55C \uB2E4\uC74C \uB2E4\uC2DC \uBBF8\uB9AC \uBD05\uB2C8\uB2E4.",
    partNumber: "\uBD80\uD488 \uBC88\uD638", productID: "Product ID", targetRoute: "\uB300\uC0C1 \uACBD\uB85C", issue: "\uBB38\uC81C",
    publishConfirm: "Publish Website \uC791\uC5C5 \uBCF5\uC0AC\uBCF8\uC744 \uD558\uB098\uC758 \uC0C8\uB85C\uC6B4 \uC0AC\uC774\uD2B8 \uC2DC\uB300\uC5D0 Published Product\uB9C8\uB2E4 \uC804\uD658\uD569\uB2C8\uAE4C?", publishWebsite: "Publish Website \uAD6C\uC131",
    publishedVersions: "Published Website \uBC84\uC804", version: "\uBC84\uC804", siteEpoch: "\uC0AC\uC774\uD2B8 \uC2DC\uB300", created: "\uC0DD\uC131\uB428", action: "\uD589\uB3D9",
    restoreConfirm: (version: number) => `\uBC84\uC804 \uBCF5\uC6D0${version}\uC791\uC5C5 \uBCF5\uC0AC\uBCF8\uC5D0? \uC790\uB3D9\uC73C\uB85C \uAC8C\uC2DC\uB418\uC9C0 \uC54A\uC2B5\uB2C8\uB2E4.`, restoreWorking: "\uC791\uC5C5 \uC911\uC73C\uB85C \uBCF5\uC6D0",
},
"de-DE": {
    saved: (revision: number) => `Die funktionierende Website-Revision wurde gespeichert${revision}. Die \u00F6ffentliche Seite bleibt unver\u00E4ndert.`,
    uploaded: (name: string) => `Hochgeladen${name}. Speichern Sie die Arbeitskopie, um darauf zu verweisen.`,
    routesChecked: (affected: number, revision: number) => `Gepr\u00FCft${affected}Published-Produktrouten gegen die Website-Arbeitsrevision${revision}.`,
    published: (epoch: number) => `Published Website-Konfiguration und jede Product-Darstellung in der Standortepoche${epoch}.`,
    restored: (version: number, revision: number) => `Wiederhergestellte Website-Version${version}in die Arbeitsrevision${revision}. Preview und Publish sind weiterhin erforderlich.`,
    absoluteURL: "Geben Sie ein absolutes HTTP oder HTTPS URL ein.", validURL: "Geben Sie ein g\u00FCltiges Website URL ein.",
    searchSaved: "Suchintegrationseinstellungen gespeichert. Externe Einreichungen laufen unabh\u00E4ngig voneinander.",
    cssState: (disabled: boolean) => disabled ? "Der abgesicherte Modus Custom CSS ist f\u00FCr alle neuen \u00F6ffentlichen Anfragen aktiv." : "Custom CSS wurde f\u00FCr neue \u00F6ffentliche Anfragen wieder aktiviert.",
    workingConfiguration: "Website Arbeitskonfiguration", workingRevision: "Arbeitsrevision", activeVersion: "Aktive Version", activeEpoch: "Epoche der aktiven Website",
    localization: "Bearbeitung von Sprachen und Inhalten", localizationHelp: "Dies sind Website Arbeitseinstellungen. \u00D6ffentliche Sprachen \u00E4ndern sich erst nach Preview und Publish; Durch das Deaktivieren der Bearbeitung wird die Ver\u00F6ffentlichung gespeicherter \u00DCbersetzungen niemals r\u00FCckg\u00E4ngig gemacht.",
    defaultLocale: "Standardgebietsschema der Site", enabledLocales: "Published-Gebietsschemata nach dem n\u00E4chsten Publish", contentEditingEnabled: "Aktivieren Sie die Bearbeitung der \u00DCbersetzung von Kundeninhalten", activeLocales: "Derzeit \u00F6ffentliche Orte", saveLocalization: "Arbeitskopie in der Sprache speichern", localizationSaved: (revision: number) => `Arbeitsversion der Website-Sprache gespeichert${revision}. Die \u00F6ffentliche Seite bleibt unver\u00E4ndert.`,
    cssSafeMode: "Custom CSS Der abgesicherte Modus ist aktiv", cssSafeDescription: "\u00D6ffentliche Anfragen k\u00F6nnen das aktive benutzerdefinierte Stylesheet nicht abrufen. Admin und Systemseiten sind davon nicht betroffen.",
    reenableCSS: "Custom CSS erneut aktivieren", disableCSSConfirm: "Die Bereitstellung von Custom CSS f\u00FCr neue \u00F6ffentliche Anfragen sofort einstellen?", disableCSS: "Deaktivieren Sie Custom CSS jetzt",
    workingHelp: "Speichern Sie \u00C4nderungen in der dauerhaften Arbeitskopie. Preview ist privat. Nur Publish \u00E4ndert die \u00F6ffentlichen Website- und Product-Artefakte.", dirtyCapture: "Speichern Sie lokale Formular\u00E4nderungen oder laden Sie sie neu, bevor Sie sie erneut erfassen.",
    dirtyCaptureDescription: "Der Server vergleicht eine Erfassung mit der dauerhaften Website-Arbeitsrevision, nicht mit nicht gespeicherten Browserfeldern.", reloadWorking: "Gespeicherte Arbeit neu laden",
    organization: "Organisation", field: "Feld",
    displayName: "Anzeigename", legalName: "Offizieller Name", officialWebsite: "Offizielle Website", privacyURL: "Datenschutz URL", termsURL: "Bedingungen URL",
    primaryLogo: "Prim\u00E4res Logo", darkLogo: "Logo mit dunklem Hintergrund", favicon: "Favicon", socialImage: "Soziales Image", assetID: "Anlage ID", upload: "Hochladen",
    contactID: "Kontaktieren Sie ID", label: "Etikett", url: "URL", order: "Befehl", removeContact: "Kontakt entfernen", addContact: "Kontakt hinzuf\u00FCgen",
    navigation: "Navigation", id: "ID", parentID: "\u00DCbergeordnetes Element ID", visible: "Sichtbar", newWindow: "Neues Fenster", removeLink: "Link entfernen", addLink: "Navigationslink hinzuf\u00FCgen",
    themeSEO: "Thema und SEO", primaryColor: "Grundfarbe", secondaryColor: "Sekund\u00E4rfarbe", fontFamily: "Schriftfamilie", contentWidth: "Inhaltsbreite",
    applyCSS: "Wenden Sie Custom CSS auf \u00F6ffentlichen Seiten an", customCSS: "Custom CSS", defaultTitle: "Standardtitel", defaultDescription: "Standardbeschreibung",
    saveWorking: "Arbeitskopie speichern", previewWorking: "Preview hat die Arbeitskopie gespeichert",
    searchExposure: "Suche und KI-Pr\u00E4senz", optionalEnhancement: "Optionale Erweiterung",
    searchHelp: "HTML, JSON-LD, robots.txt, Sitemap, JSON, manifest, Markdown und llms.txt bleiben ohne Integration verf\u00FCgbar. Eine erfolgreiche \u00DCbermittlung bedeutet, dass sie vom Dienst akzeptiert und nie indiziert wird.",
    indexNowTitle: "Bing und teilnehmende IndexNow-Engines", enableIndexNow: "Aktivieren Sie IndexNow-\u00C4nderungsbenachrichtigungen", indexNowUnavailable: "Konfigurieren Sie eine \u00F6ffentliche HTTPS-Basis URL, bevor Sie IndexNow aktivieren.", keyURL: "\u00DCberpr\u00FCfung des \u00F6ffentlichen Schl\u00FCssels URL",
    googleTitle: "Google Search Console", enableGoogle: "Senden Sie die Sitemap \u00FCber die Search Console", googleUnavailable: "Konfigurieren Sie zun\u00E4chst den Host-OAuth-Client ID, die geheime Clientdatei, die Aktualisierungstokendatei und eine \u00F6ffentliche HTTPS-Basis URL.",
    siteProperty: "Verifizierte Search Console-Site-Eigenschaft", sitePropertyHelp: "Verwenden Sie die genaue URL-Pr\u00E4fixeigenschaft, z. B. https://catalog.example.com/ oder sc-domain:example.com.", sitePropertyRequired: "Eine verifizierte Site-Eigenschaft ist erforderlich.", saveSearch: "Suchintegrationen speichern",
    publicRoutes: "\u00D6ffentliche Produktrouten", activePrefix: "Aktives Pr\u00E4fix", activePattern: "Aktives Muster",
    routesHelp: "Preview pr\u00FCft jeden Published Product. Publish installiert das funktionierende Website, Routen und Darstellungen an einer standortweiten Sichtbarkeitsgrenze.",
    productPrefix: "Product-Pr\u00E4fix", safeSegment: "Verwenden Sie ein sicheres Pfadsegment, das mit / beginnt.", pattern: "Muster", compact: "Kompakt", manufacturerPath: "Herstellerpfad", brandPath: "Markenweg", categoryPath: "Kategoriepfad", previewSite: "Preview gesamte Website",
    candidateEpoch: (epoch: number) => `Epoche der Kandidatenseite${epoch}`, routesReady: (count: number, revision: number) => `${count}Routen und Website Arbeitsrevision${revision}sind bereit.`,
    cannotPublish: "Dieser Kandidat kann nicht ver\u00F6ffentlicht werden.", cannotPublishDescription: "Beheben Sie alle fehlenden Namespaces, Routenkonflikte oder veralteten Arbeitsrevisionen und zeigen Sie dann erneut eine Vorschau an.",
    partNumber: "Teilenummer", productID: "Product ID", targetRoute: "Zielroute", issue: "Ausgabe",
    publishConfirm: "Publish die Website-Arbeitskopie und jeden Published Product in einer neuen Site-Epoche wechseln?", publishWebsite: "Publish Website Konfiguration",
    publishedVersions: "Published Website-Versionen", version: "Version", siteEpoch: "Epoche der Website", created: "Erstellt", action: "Aktion",
    restoreConfirm: (version: number) => `Version wiederherstellen${version}in die Arbeitskopie? Es wird nicht automatisch ver\u00F6ffentlicht.`, restoreWorking: "Als funktionierend wiederherstellen",
},
"fr-FR": {
    saved: (revision: number) => `R\u00E9vision de travail Website enregistr\u00E9e${revision}. Le site public est inchang\u00E9.`,
    uploaded: (name: string) => `T\u00E9l\u00E9charg\u00E9${name}. Enregistrez la copie de travail pour la r\u00E9f\u00E9rencer.`,
    routesChecked: (affected: number, revision: number) => `\u00C0 carreaux${affected}Le produit Published est achemin\u00E9 vers la r\u00E9vision de travail Website${revision}.`,
    published: (epoch: number) => `Configuration Published Website et chaque repr\u00E9sentation Product \u00E0 l'\u00E9poque du site${epoch}.`,
    restored: (version: number, revision: number) => `Version Website restaur\u00E9e${version}en r\u00E9vision de travail${revision}. Preview et Publish sont toujours requis.`,
    absoluteURL: "Entrez un HTTP ou HTTPS URL absolu.", validURL: "Entrez un Website URL valide.",
    searchSaved: "Param\u00E8tres d'int\u00E9gration de recherche enregistr\u00E9s. Les soumissions externes s'ex\u00E9cutent de mani\u00E8re ind\u00E9pendante.",
    cssState: (disabled: boolean) => disabled ? "Le mode sans \u00E9chec Custom CSS est actif pour toutes les nouvelles demandes publiques." : "Custom CSS a \u00E9t\u00E9 r\u00E9activ\u00E9 pour les nouvelles demandes publiques.",
    workingConfiguration: "Configuration de travail Website", workingRevision: "R\u00E9vision de travail", activeVersion: "Version active", activeEpoch: "\u00C9poque du site actif",
    localization: "Langues et \u00E9dition de contenu", localizationHelp: "Ce sont les param\u00E8tres de travail Website. Les langues publiques ne changent qu'apr\u00E8s Preview et Publish\u00A0; la d\u00E9sactivation de l'\u00E9dition ne d\u00E9publie jamais les traductions enregistr\u00E9es.",
    defaultLocale: "Param\u00E8tres r\u00E9gionaux par d\u00E9faut du site", enabledLocales: "Param\u00E8tres r\u00E9gionaux Published apr\u00E8s le prochain Publish", contentEditingEnabled: "Activer la modification de la traduction du contenu client", activeLocales: "Lieux actuellement publics", saveLocalization: "Enregistrer la copie de travail de la langue", localizationSaved: (revision: number) => `R\u00E9vision de travail du langage Website enregistr\u00E9e${revision}. Le site public est inchang\u00E9.`,
    cssSafeMode: "Le mode sans \u00E9chec Custom CSS est actif", cssSafeDescription: "Les requ\u00EAtes publiques ne peuvent pas r\u00E9cup\u00E9rer la feuille de style personnalis\u00E9e active. Admin et les pages syst\u00E8me ne sont pas affect\u00E9s.",
    reenableCSS: "R\u00E9activer Custom CSS", disableCSSConfirm: "Arr\u00EAter imm\u00E9diatement de r\u00E9pondre \u00E0 Custom CSS aux nouvelles demandes publiques\u00A0?", disableCSS: "D\u00E9sactivez Custom CSS maintenant",
    workingHelp: "Enregistrez les modifications dans la copie de travail durable. Preview est priv\u00E9. Seul Publish modifie les artefacts publics Website et Product.", dirtyCapture: "Enregistrez ou rechargez les modifications du formulaire local avant de les capturer \u00E0 nouveau.",
    dirtyCaptureDescription: "Le serveur compare une capture avec la r\u00E9vision de travail durable Website, et non avec les champs du navigateur non enregistr\u00E9s.", reloadWorking: "Recharger le travail enregistr\u00E9",
    organization: "Organisation", field: "Champ",
    displayName: "Nom d'affichage", legalName: "Nom l\u00E9gal", officialWebsite: "Site officiel", privacyURL: "Confidentialit\u00E9 URL", termsURL: "Conditions URL",
    primaryLogo: "Logo principal", darkLogo: "Logo sur fond sombre", favicon: "Ic\u00F4ne de favori", socialImage: "Image sociale", assetID: "actif ID", upload: "T\u00E9l\u00E9charger",
    contactID: "Contacter ID", label: "\u00C9tiquette", url: "URL", order: "Commande", removeContact: "Supprimer le contact", addContact: "Ajouter un contact",
    navigation: "Navigation", id: "ID", parentID: "Parent ID", visible: "Visible", newWindow: "Nouvelle fen\u00EAtre", removeLink: "Supprimer le lien", addLink: "Ajouter un lien de navigation",
    themeSEO: "Th\u00E8me et r\u00E9f\u00E9rencement", primaryColor: "Couleur primaire", secondaryColor: "Couleur secondaire", fontFamily: "Famille de polices", contentWidth: "Largeur du contenu",
    applyCSS: "Appliquer Custom CSS sur les pages publiques", customCSS: "Custom CSS", defaultTitle: "Titre par d\u00E9faut", defaultDescription: "Description par d\u00E9faut",
    saveWorking: "Enregistrer la copie de travail", previewWorking: "Preview a enregistr\u00E9 une copie de travail",
    searchExposure: "Exposition \u00E0 la recherche et \u00E0 l\u2019IA", optionalEnhancement: "Am\u00E9lioration facultative",
    searchHelp: "HTML, JSON-LD, robots.txt, Sitemap, JSON, manifest, Markdown et llms.txt restent disponibles sans aucune des deux int\u00E9grations. Une soumission r\u00E9ussie signifie accept\u00E9e par le service, jamais index\u00E9e.",
    indexNowTitle: "Bing et moteurs IndexNow participants", enableIndexNow: "Activer les notifications de modification IndexNow", indexNowUnavailable: "Configurez une base HTTPS publique URL avant d'activer IndexNow.", keyURL: "V\u00E9rification de la cl\u00E9 publique URL",
    googleTitle: "Google Search Console", enableGoogle: "Soumettez le plan du site via la Search Console", googleUnavailable: "Configurez d'abord le client h\u00F4te OAuth ID, le fichier secret client, le fichier de jeton d'actualisation et une base HTTPS publique URL.",
    siteProperty: "Propri\u00E9t\u00E9 du site Search\u00A0Console v\u00E9rifi\u00E9e", sitePropertyHelp: "Utilisez la propri\u00E9t\u00E9 exacte de pr\u00E9fixe URL telle que https://catalog.example.com/ ou sc-domain:example.com.", sitePropertyRequired: "Une propri\u00E9t\u00E9 de site v\u00E9rifi\u00E9e est requise.", saveSearch: "Enregistrer les int\u00E9grations de recherche",
    publicRoutes: "Itin\u00E9raires de produits publics", activePrefix: "Pr\u00E9fixe actif", activePattern: "Mod\u00E8le actif",
    routesHelp: "Preview v\u00E9rifie chaque Published Product. Publish installe le Website fonctionnel, les itin\u00E9raires et les repr\u00E9sentations sur une limite de visibilit\u00E9 \u00E0 l'\u00E9chelle du site.",
    productPrefix: "Pr\u00E9fixe Product", safeSegment: "Utilisez un segment de chemin s\u00E9curis\u00E9 commen\u00E7ant par /.", pattern: "Mod\u00E8le", compact: "Compact", manufacturerPath: "Chemin du fabricant", brandPath: "Chemin de la marque", categoryPath: "Chemin de cat\u00E9gorie", previewSite: "Preview site entier",
    candidateEpoch: (epoch: number) => `\u00C9poque du site candidat${epoch}`, routesReady: (count: number, revision: number) => `${count}itin\u00E9raires et r\u00E9vision de travail Website${revision}sont pr\u00EAts.`,
    cannotPublish: "Ce candidat ne peut pas \u00EAtre publi\u00E9.", cannotPublishDescription: "R\u00E9solvez chaque espace de noms manquant, conflit de route ou r\u00E9vision de travail obsol\u00E8te, puis pr\u00E9visualisez \u00E0 nouveau.",
    partNumber: "Num\u00E9ro de pi\u00E8ce", productID: "Product ID", targetRoute: "Itin\u00E9raire cible", issue: "Probl\u00E8me",
    publishConfirm: "Publish la copie de travail Website et changer chaque Published Product \u00E0 une nouvelle \u00E9poque de site\u00A0?", publishWebsite: "Configuration Publish Website",
    publishedVersions: "Versions Published Website", version: "Version", siteEpoch: "\u00C9poque du site", created: "Cr\u00E9\u00E9", action: "Action",
    restoreConfirm: (version: number) => `Restaurer la version${version}dans la copie de travail ? Il ne sera pas publi\u00E9 automatiquement.`, restoreWorking: "Restaurer comme fonctionnant",
},
"it-IT": {
    saved: (revision: number) => `Revisione funzionante Website salvata${revision}. Il sito pubblico \u00E8 invariato.`,
    uploaded: (name: string) => `Caricato${name}. Salvare la copia di lavoro per farvi riferimento.`,
    routesChecked: (affected: number, revision: number) => `Controllato${affected}Il prodotto Published si confronta con la revisione operativa Website${revision}.`,
    published: (epoch: number) => `Configurazione Published Website e ogni rappresentazione Product all'epoca del sito${epoch}.`,
    restored: (version: number, revision: number) => `Versione Website restaurata${version}nella revisione operativa${revision}. Preview e Publish sono ancora necessari.`,
    absoluteURL: "Inserisci un HTTP o HTTPS assoluto URL.", validURL: "Inserisci un Website URL valido.",
    searchSaved: "Impostazioni di integrazione della ricerca salvate. Gli invii esterni vengono eseguiti in modo indipendente.",
    cssState: (disabled: boolean) => disabled ? "Custom CSS La modalit\u00E0 provvisoria \u00E8 attiva per tutte le nuove richieste pubbliche." : "Custom CSS \u00E8 stato riabilitato per nuove richieste pubbliche.",
    workingConfiguration: "Configurazione di lavoro Website", workingRevision: "Revisione funzionante", activeVersion: "Versione attiva", activeEpoch: "Epoca del sito attivo",
    localization: "Lingue e modifica dei contenuti", localizationHelp: "Queste sono le impostazioni di lavoro di Website. Le lingue pubbliche cambiano solo dopo Preview e Publish; disabilitare la modifica non annulla mai la pubblicazione delle traduzioni salvate.",
    defaultLocale: "Impostazioni locali predefinite del sito", enabledLocales: "Impostazioni locali Published dopo il successivo Publish", contentEditingEnabled: "Abilita la modifica della traduzione del contenuto del cliente", activeLocales: "Localit\u00E0 attualmente pubbliche", saveLocalization: "Salva la copia di lavoro della lingua", localizationSaved: (revision: number) => `Revisione operativa della lingua Website salvata${revision}. Il sito pubblico \u00E8 invariato.`,
    cssSafeMode: "Custom CSS La modalit\u00E0 provvisoria \u00E8 attiva", cssSafeDescription: "Le richieste pubbliche non possono recuperare il foglio di stile personalizzato attivo. Admin e le pagine di sistema non sono interessate.",
    reenableCSS: "Riabilitare Custom CSS", disableCSSConfirm: "Interrompere immediatamente la fornitura di Custom CSS alle nuove richieste pubbliche?", disableCSS: "Disabilita Custom CSS ora",
    workingHelp: "Salva le modifiche nella copia di lavoro durevole. Preview \u00E8 privato. Solo Publish modifica gli artefatti pubblici Website e Product.", dirtyCapture: "Salva o ricarica le modifiche del modulo locale prima di ripetere l'acquisizione.",
    dirtyCaptureDescription: "Il server confronta un'acquisizione con la revisione operativa durevole Website, non con i campi del browser non salvati.", reloadWorking: "Ricarica la lavorazione salvata",
    organization: "Organizzazione", field: "Campo",
    displayName: "Nome da visualizzare", legalName: "Nome legale", officialWebsite: "Sito ufficiale", privacyURL: "Privacy URL", termsURL: "Termini URL",
    primaryLogo: "Marchio primario", darkLogo: "Logo con sfondo scuro", favicon: "Favicon", socialImage: "Immagine sociale", assetID: "risorsa ID", upload: "Caricamento",
    contactID: "Contatta ID", label: "Etichetta", url: "URL", order: "Ordine", removeContact: "Rimuovi contatto", addContact: "Aggiungi contatto",
    navigation: "Navigazione", id: "ID", parentID: "Genitore ID", visible: "Visibile", newWindow: "Nuova finestra", removeLink: "Rimuovi collegamento", addLink: "Aggiungi collegamento di navigazione",
    themeSEO: "Tema e SEO", primaryColor: "Colore primario", secondaryColor: "Colore secondario", fontFamily: "Famiglia di caratteri", contentWidth: "Larghezza del contenuto",
    applyCSS: "Applica Custom CSS sulle pagine pubbliche", customCSS: "Custom CSS", defaultTitle: "Titolo predefinito", defaultDescription: "Descrizione predefinita",
    saveWorking: "Salva copia di lavoro", previewWorking: "Preview ha salvato la copia di lavoro",
    searchExposure: "Ricerca ed esposizione all'intelligenza artificiale", optionalEnhancement: "Miglioramento facoltativo",
    searchHelp: "HTML, JSON-LD, robots.txt, Sitemap, JSON, manifest, Markdown e llms.txt rimangono disponibili senza alcuna integrazione. Un invio riuscito significa accettato dal servizio, mai indicizzato.",
    indexNowTitle: "Bing e i motori IndexNow partecipanti", enableIndexNow: "Abilita le notifiche di modifica IndexNow", indexNowUnavailable: "Configurare una base HTTPS pubblica URL prima di abilitare IndexNow.", keyURL: "Verifica della chiave pubblica URL",
    googleTitle: "Google Search Console", enableGoogle: "Invia la Sitemap tramite Search Console", googleUnavailable: "Configurare prima il client host OAuth ID, il file segreto del client, il file del token di aggiornamento e una base HTTPS pubblica URL.",
    siteProperty: "Propriet\u00E0 del sito Verificato della Search Console", sitePropertyHelp: "Utilizza la propriet\u00E0 esatta del prefisso URL, ad esempio https://catalog.example.com/ o sc-domain:example.com.", sitePropertyRequired: "\u00C8 obbligatoria una propriet\u00E0 del sito verificata.", saveSearch: "Salva le integrazioni di ricerca",
    publicRoutes: "Percorsi dei prodotti pubblici", activePrefix: "Prefisso attivo", activePattern: "Modello attivo",
    routesHelp: "Preview controlla ogni Published Product. Publish installa Website, percorsi e rappresentazioni funzionanti in un limite di visibilit\u00E0 a livello di sito.",
    productPrefix: "Prefisso Product", safeSegment: "Utilizzare un segmento di percorso sicuro che inizia con /.", pattern: "Modello", compact: "Compatto", manufacturerPath: "Percorso del produttore", brandPath: "Percorso del marchio", categoryPath: "Percorso delle categorie", previewSite: "Preview intero sito",
    candidateEpoch: (epoch: number) => `Epoca del sito candidato${epoch}`, routesReady: (count: number, revision: number) => `${count}percorsi e revisione operativa Website${revision}sono pronti.`,
    cannotPublish: "Questo candidato non pu\u00F2 essere pubblicato.", cannotPublishDescription: "Risolvi ogni spazio dei nomi mancante, conflitto di instradamento o revisione funzionante non aggiornata, quindi visualizza nuovamente l'anteprima.",
    partNumber: "Numero di parte", productID: "Product ID", targetRoute: "Percorso di destinazione", issue: "Problema",
    publishConfirm: "Publish la copia funzionante di Website e cambia ogni Published Product in una nuova epoca di sito?", publishWebsite: "Configurazione Publish Website",
    publishedVersions: "Versioni Published Website", version: "Versione", siteEpoch: "Epoca del sito", created: "Creato", action: "Azione",
    restoreConfirm: (version: number) => `Ripristina versione${version}nella copia di lavoro? Non verr\u00E0 pubblicato automaticamente.`, restoreWorking: "Ripristinare come funzionante",
},
"es-ES": {
    saved: (revision: number) => `Revisi\u00F3n de trabajo Website guardada${revision}. El sitio p\u00FAblico no ha cambiado.`,
    uploaded: (name: string) => `subido${name}. Guarde la copia de trabajo para consultarla.`,
    routesChecked: (affected: number, revision: number) => `Comprobado${affected}Rutas del producto Published contra la revisi\u00F3n de trabajo Website${revision}.`,
    published: (epoch: number) => `Configuraci\u00F3n de Published Website y cada representaci\u00F3n de Product en la \u00E9poca del sitio${epoch}.`,
    restored: (version: number, revision: number) => `Versi\u00F3n Website restaurada${version}en revisi\u00F3n de trabajo${revision}. A\u00FAn se requieren Preview y Publish.`,
    absoluteURL: "Ingrese un HTTP absoluto o HTTPS URL.", validURL: "Introduzca un Website URL v\u00E1lido.",
    searchSaved: "Se guard\u00F3 la configuraci\u00F3n de integraci\u00F3n de b\u00FAsqueda. Los env\u00EDos externos se realizan de forma independiente.",
    cssState: (disabled: boolean) => disabled ? "Custom CSS El modo seguro est\u00E1 activo para todas las solicitudes p\u00FAblicas nuevas." : "Custom CSS se volvi\u00F3 a habilitar para nuevas solicitudes p\u00FAblicas.",
    workingConfiguration: "Configuraci\u00F3n de trabajo Website", workingRevision: "Revisi\u00F3n de trabajo", activeVersion: "Versi\u00F3n activa", activeEpoch: "\u00C9poca del sitio activo",
    localization: "Idiomas y edici\u00F3n de contenidos.", localizationHelp: "Estas son las configuraciones de trabajo de Website. Los idiomas p\u00FAblicos cambian s\u00F3lo despu\u00E9s de Preview y Publish; Deshabilitar la edici\u00F3n nunca anula la publicaci\u00F3n de las traducciones guardadas.",
    defaultLocale: "Configuraci\u00F3n regional predeterminada del sitio", enabledLocales: "Configuraciones locales Published despu\u00E9s del siguiente Publish", contentEditingEnabled: "Habilitar la edici\u00F3n de traducci\u00F3n del contenido del cliente", activeLocales: "Lugares p\u00FAblicos actualmente", saveLocalization: "Guardar copia de trabajo del idioma", localizationSaved: (revision: number) => `Revisi\u00F3n de trabajo del lenguaje Website guardada${revision}. El sitio p\u00FAblico no ha cambiado.`,
    cssSafeMode: "Custom CSS El modo seguro est\u00E1 activo", cssSafeDescription: "Las solicitudes p\u00FAblicas no pueden recuperar la hoja de estilo personalizada activa. Admin y las p\u00E1ginas del sistema no se ven afectadas.",
    reenableCSS: "Vuelva a habilitar Custom CSS", disableCSSConfirm: "\u00BFDejar de servir inmediatamente Custom CSS a nuevas solicitudes p\u00FAblicas?", disableCSS: "Desactivar Custom CSS ahora",
    workingHelp: "Guarde los cambios en la copia de trabajo duradera. Preview es privado. S\u00F3lo Publish cambia los artefactos p\u00FAblicos Website y Product.", dirtyCapture: "Guarde o vuelva a cargar las ediciones del formulario local antes de volver a capturarlas.",
    dirtyCaptureDescription: "", reloadWorking: "Recargar guardado trabajando",
    organization: "Organizaci\u00F3n", field: "Campo",
    displayName: "Nombre para mostrar", legalName: "Nombre legal", officialWebsite: "Sitio web oficial", privacyURL: "Privacidad URL", termsURL: "T\u00E9rminos URL",
    primaryLogo: "Logotipo principal", darkLogo: "Logotipo de fondo oscuro", favicon: "favicon", socialImage: "Imagen social", assetID: "activo ID", upload: "Subir",
    contactID: "Contacto ID", label: "Etiqueta", url: "URL", order: "Orden", removeContact: "Eliminar contacto", addContact: "A\u00F1adir contacto",
    navigation: "Navegaci\u00F3n", id: "ID", parentID: "Padre ID", visible: "Visible", newWindow: "Nueva ventana", removeLink: "Quitar enlace", addLink: "Agregar enlace de navegaci\u00F3n",
    themeSEO: "Tema y SEO", primaryColor: "color primario", secondaryColor: "color secundario", fontFamily: "Familia de fuentes", contentWidth: "Ancho del contenido",
    applyCSS: "Aplicar Custom CSS en p\u00E1ginas p\u00FAblicas", customCSS: "Custom CSS", defaultTitle: "T\u00EDtulo predeterminado", defaultDescription: "Descripci\u00F3n predeterminada",
    saveWorking: "Guardar copia de trabajo", previewWorking: "Preview copia de trabajo guardada",
    searchExposure: "B\u00FAsqueda y exposici\u00F3n a la IA", optionalEnhancement: "Mejora opcional",
    searchHelp: "HTML, JSON-LD, robots.txt, Sitemap, JSON, manifest, Markdown y llms.txt siguen estando disponibles sin ninguna integraci\u00F3n. Un env\u00EDo exitoso significa aceptado por el servicio, nunca indexado.",
    indexNowTitle: "Bing y motores IndexNow participantes", enableIndexNow: "Habilitar notificaciones de cambios IndexNow", indexNowUnavailable: "Configure una base HTTPS p\u00FAblica URL antes de habilitar IndexNow.", keyURL: "Verificaci\u00F3n de clave p\u00FAblica URL",
    googleTitle: "Google Search Console", enableGoogle: "Env\u00EDe el mapa del sitio a trav\u00E9s de Search Console", googleUnavailable: "Primero configure el cliente OAuth del host ID, el archivo secreto del cliente, el archivo de token de actualizaci\u00F3n y una base HTTPS p\u00FAblica URL.",
    siteProperty: "Propiedad del sitio de Search Console verificada", sitePropertyHelp: "Utilice la propiedad exacta del prefijo URL, como https://catalog.example.com/ o sc-domain:example.com.", sitePropertyRequired: "Se requiere una propiedad de sitio verificada.", saveSearch: "Guardar integraciones de b\u00FAsqueda",
    publicRoutes: "Rutas p\u00FAblicas de productos", activePrefix: "Prefijo activo", activePattern: "Patr\u00F3n activo",
    routesHelp: "Preview comprueba cada Published Product. Publish instala el Website en funcionamiento, las rutas y las representaciones en un l\u00EDmite de visibilidad en todo el sitio.",
    productPrefix: "Prefijo Product", safeSegment: "Utilice un segmento de ruta segura que comience con /.", pattern: "Patr\u00F3n", compact: "Compacto", manufacturerPath: "Ruta del fabricante", brandPath: "Camino de la marca", categoryPath: "Ruta de categor\u00EDa", previewSite: "Preview sitio completo",
    candidateEpoch: (epoch: number) => `\u00C9poca del sitio candidato${epoch}`, routesReady: (count: number, revision: number) => `${count}rutas y revisi\u00F3n de trabajo Website${revision}est\u00E1n listos.`,
    cannotPublish: "Este candidato no se puede publicar.", cannotPublishDescription: "Resuelva cada espacio de nombres faltante, conflicto de ruta o revisi\u00F3n de trabajo obsoleta y luego obtenga una vista previa nuevamente.",
    partNumber: "N\u00FAmero de pieza", productID: "Product ID", targetRoute: "Ruta objetivo", issue: "Asunto",
    publishConfirm: "\u00BFPublish la copia de trabajo Website y cambiar cada Published Product en una nueva \u00E9poca del sitio?", publishWebsite: "Configuraci\u00F3n Publish Website",
    publishedVersions: "Versiones Published Website", version: "Versi\u00F3n", siteEpoch: "\u00C9poca del sitio", created: "Creado", action: "Acci\u00F3n",
    restoreConfirm: (version: number) => `Restaurar versi\u00F3n${version}en la copia de trabajo? No se publicar\u00E1 autom\u00E1ticamente.`, restoreWorking: "Restaurar como funcionando",
},
"pt-BR": {
    saved: (revision: number) => `Revis\u00E3o de trabalho Website salva${revision}. O site p\u00FAblico permanece inalterado.`,
    uploaded: (name: string) => `Enviado${name}. Salve a c\u00F3pia de trabalho para referenci\u00E1-la.`,
    routesChecked: (affected: number, revision: number) => `Verificado${affected}Rotas do produto Published em rela\u00E7\u00E3o \u00E0 revis\u00E3o de trabalho Website${revision}.`,
    published: (epoch: number) => `Configura\u00E7\u00E3o Published Website e cada representa\u00E7\u00E3o Product na \u00E9poca do site${epoch}.`,
    restored: (version: number, revision: number) => `Vers\u00E3o Website restaurada${version}em revis\u00E3o de trabalho${revision}. Preview e Publish ainda s\u00E3o necess\u00E1rios.`,
    absoluteURL: "Insira um HTTP ou HTTPS absoluto URL.", validURL: "Insira um Website URL v\u00E1lido.",
    searchSaved: "Configura\u00E7\u00F5es de integra\u00E7\u00E3o de pesquisa salvas. Os envios externos s\u00E3o executados de forma independente.",
    cssState: (disabled: boolean) => disabled ? "O modo de seguran\u00E7a Custom CSS est\u00E1 ativo para todas as novas solicita\u00E7\u00F5es p\u00FAblicas." : "Custom CSS foi reativado para novas solicita\u00E7\u00F5es p\u00FAblicas.",
    workingConfiguration: "Configura\u00E7\u00E3o de trabalho Website", workingRevision: "Revis\u00E3o de trabalho", activeVersion: "Vers\u00E3o ativa", activeEpoch: "\u00C9poca do site ativo",
    localization: "Idiomas e edi\u00E7\u00E3o de conte\u00FAdo", localizationHelp: "Estas s\u00E3o configura\u00E7\u00F5es de trabalho Website. Os idiomas p\u00FAblicos mudam somente ap\u00F3s Preview e Publish; desabilitar a edi\u00E7\u00E3o nunca cancela a publica\u00E7\u00E3o das tradu\u00E7\u00F5es salvas.",
    defaultLocale: "Local padr\u00E3o do site", enabledLocales: "Localidades Published ap\u00F3s o pr\u00F3ximo Publish", contentEditingEnabled: "Habilitar edi\u00E7\u00E3o de tradu\u00E7\u00E3o de conte\u00FAdo do cliente", activeLocales: "Atualmente locais p\u00FAblicos", saveLocalization: "Salvar c\u00F3pia de trabalho do idioma", localizationSaved: (revision: number) => `Revis\u00E3o de trabalho da linguagem Website salva${revision}. O site p\u00FAblico permanece inalterado.`,
    cssSafeMode: "O modo de seguran\u00E7a Custom CSS est\u00E1 ativo", cssSafeDescription: "As solicita\u00E7\u00F5es p\u00FAblicas n\u00E3o podem recuperar a folha de estilo personalizada ativa. Admin e as p\u00E1ginas do sistema n\u00E3o s\u00E3o afetadas.",
    reenableCSS: "Reativar Custom CSS", disableCSSConfirm: "Parar imediatamente de servir Custom CSS para novas solicita\u00E7\u00F5es p\u00FAblicas?", disableCSS: "Desative Custom CSS agora",
    workingHelp: "Salve as altera\u00E7\u00F5es na c\u00F3pia de trabalho dur\u00E1vel. Preview \u00E9 privado. Somente Publish altera os artefatos p\u00FAblicos Website e Product.", dirtyCapture: "Salve ou recarregue as edi\u00E7\u00F5es locais do formul\u00E1rio antes de captur\u00E1-las novamente.",
    dirtyCaptureDescription: "O servidor compara uma captura com a revis\u00E3o de trabalho dur\u00E1vel Website, e n\u00E3o com campos n\u00E3o salvos do navegador.", reloadWorking: "Recarregar salvo funcionando",
    organization: "Organiza\u00E7\u00E3o", field: "Campo",
    displayName: "Nome de exibi\u00E7\u00E3o", legalName: "Nome legal", officialWebsite: "Site oficial", privacyURL: "Privacidade URL", termsURL: "Termos URL",
    primaryLogo: "Logotipo principal", darkLogo: "Logotipo com fundo escuro", favicon: "Favicon", socialImage: "Imagem social", assetID: "ativo ID", upload: "Carregar",
    contactID: "Contato ID", label: "R\u00F3tulo", url: "URL", order: "Ordem", removeContact: "Remover contato", addContact: "Adicionar contato",
    navigation: "Navega\u00E7\u00E3o", id: "ID", parentID: "Pai ID", visible: "Vis\u00EDvel", newWindow: "Nova janela", removeLink: "Remover link", addLink: "Adicionar link de navega\u00E7\u00E3o",
    themeSEO: "Tema e SEO", primaryColor: "Cor prim\u00E1ria", secondaryColor: "Cor secund\u00E1ria", fontFamily: "Fam\u00EDlia de fontes", contentWidth: "Largura do conte\u00FAdo",
    applyCSS: "Aplicar Custom CSS em p\u00E1ginas p\u00FAblicas", customCSS: "Custom CSS", defaultTitle: "T\u00EDtulo padr\u00E3o", defaultDescription: "Descri\u00E7\u00E3o padr\u00E3o",
    saveWorking: "Salvar c\u00F3pia de trabalho", previewWorking: "Preview salvou c\u00F3pia de trabalho",
    searchExposure: "Pesquisa e exposi\u00E7\u00E3o de IA", optionalEnhancement: "Aprimoramento opcional",
    searchHelp: "HTML, JSON-LD, robots.txt, Sitemap, JSON, manifest, Markdown e llms.txt permanecem dispon\u00EDveis sem qualquer integra\u00E7\u00E3o. Um envio bem-sucedido significa aceito pelo servi\u00E7o, nunca indexado.",
    indexNowTitle: "Bing e mecanismos IndexNow participantes", enableIndexNow: "Habilitar notifica\u00E7\u00F5es de altera\u00E7\u00E3o IndexNow", indexNowUnavailable: "Configure uma base HTTPS p\u00FAblica URL antes de ativar IndexNow.", keyURL: "Verifica\u00E7\u00E3o de chave p\u00FAblica URL",
    googleTitle: "Google Search Console", enableGoogle: "Envie o Sitemap por meio do Search Console", googleUnavailable: "Configure primeiro o cliente host OAuth ID, o arquivo secreto do cliente, o arquivo de token de atualiza\u00E7\u00E3o e uma base HTTPS p\u00FAblica URL.",
    siteProperty: "Propriedade verificada do site do Search Console", sitePropertyHelp: "Use a propriedade exata do prefixo URL, como https://catalog.example.com/ ou sc-domain:example.com.", sitePropertyRequired: "\u00C9 necess\u00E1ria uma propriedade de site verificada.", saveSearch: "Salvar integra\u00E7\u00F5es de pesquisa",
    publicRoutes: "Rotas p\u00FAblicas de produtos", activePrefix: "Prefixo ativo", activePattern: "Padr\u00E3o ativo",
    routesHelp: "Preview verifica cada Published Product. Publish instala o Website funcional, rotas e representa\u00E7\u00F5es em um limite de visibilidade em todo o site.",
    productPrefix: "Prefixo Product", safeSegment: "Use um segmento de caminho seguro come\u00E7ando com /.", pattern: "Padr\u00E3o", compact: "Compactar", manufacturerPath: "Caminho do fabricante", brandPath: "Caminho da marca", categoryPath: "Caminho da categoria", previewSite: "Preview site inteiro",
    candidateEpoch: (epoch: number) => `\u00C9poca do site candidato${epoch}`, routesReady: (count: number, revision: number) => `${count}rotas e revis\u00E3o de trabalho Website${revision}est\u00E3o prontos.`,
    cannotPublish: "Este candidato n\u00E3o pode ser publicado.", cannotPublishDescription: "Resolva todos os namespaces ausentes, conflitos de rota ou revis\u00F5es de trabalho obsoletas e visualize novamente.",
    partNumber: "N\u00FAmero da pe\u00E7a", productID: "Product ID", targetRoute: "Rota alvo", issue: "Emitir",
    publishConfirm: "Publish a c\u00F3pia de trabalho Website e trocar cada Published Product em uma nova \u00E9poca de site?", publishWebsite: "Configura\u00E7\u00E3o Publish Website",
    publishedVersions: "Vers\u00F5es Published Website", version: "Vers\u00E3o", siteEpoch: "\u00C9poca do site", created: "Criado", action: "A\u00E7\u00E3o",
    restoreConfirm: (version: number) => `Restaurar vers\u00E3o${version}na c\u00F3pia de trabalho? N\u00E3o ser\u00E1 publicado automaticamente.`, restoreWorking: "Restaurar como funcionando",
},
} as const;

function NavigationTargetFields({ fieldName, copy, urlLabel, newWindowLabel }: {
  fieldName: number;
  copy: NavigationEditorCopy;
  urlLabel: string;
  newWindowLabel: string;
}) {
  const targetType = Form.useWatch(["navigation", fieldName, "target_type"]) ?? "link";

  return (
    <>
      <Form.Item name={[fieldName, "target_type"]} label={copy.targetType} rules={[{ required: true }]}>
        <Select
          options={[
            { value: "link", label: copy.link },
            { value: "system_action", label: copy.systemAction },
            { value: "group", label: copy.group },
          ]}
        />
      </Form.Item>
      {targetType === "link" ? (
        <>
          <Form.Item name={[fieldName, "url"]} label={urlLabel} rules={[{ required: true }]}><Input /></Form.Item>
          <Form.Item name={[fieldName, "open_new_window"]} valuePropName="checked"><Checkbox>{newWindowLabel}</Checkbox></Form.Item>
        </>
      ) : null}
      {targetType === "system_action" ? (
        <Form.Item name={[fieldName, "system_action"]} label={copy.action} rules={[{ required: true }]}>
          <Select
            options={[
              { value: "catalog", label: copy.catalog },
              { value: "catalog_search", label: copy.catalogSearch },
              { value: "rfq", label: copy.rfq },
            ]}
          />
        </Form.Item>
      ) : null}
      <Form.Item name={[fieldName, "presentation"]} label={copy.presentation} rules={[{ required: true }]}>
        <Select
          options={[
            { value: "direct", label: copy.direct },
            { value: "dropdown", label: copy.dropdown },
          ]}
        />
      </Form.Item>
    </>
  );
}

export function WebsitePanel({ locale, onError, onMessage }: Props) {
  const text = labels[locale];
  const brandText = brandImportCopy(locale);
  const navigationText = navigationEditorCopy[locale];
  const [routeState, setRouteState] = useState<SiteRouteState>();
  const [websiteState, setWebsiteState] = useState<WebsiteState>();
  const [searchIntegrations, setSearchIntegrations] = useState<SearchIntegrationSettings>();
  const [versions, setVersions] = useState<WebsiteVersion[]>([]);
  const [routePreview, setRoutePreview] = useState<SiteRoutePreview>();
	const [brandRequest, setBrandRequest] = useState<BrandImportRequest>();
	const [brandPayload, setBrandPayload] = useState("");
	const [brandValidation, setBrandValidation] = useState<BrandImportValidation>();
	const [brandSourceURL, setBrandSourceURL] = useState("");
  const [brandRightsConfirmed, setBrandRightsConfirmed] = useState(false);
  const [websiteFormDirty, setWebsiteFormDirty] = useState(false);
  const [loading, setLoading] = useState(false);
  const [previewing, setPreviewing] = useState(false);
  const [routeForm] = Form.useForm<SiteRouteConfig>();
  const [websiteForm] = Form.useForm<SiteConfiguration>();
  const [localizationForm] = Form.useForm<WebsiteLocalization>();
  const [searchForm] = Form.useForm<SearchIntegrationSettings>();

  const load = async () => {
    setLoading(true);
    try {
      const [routes, website, history, integrations] = await Promise.all([
        api<SiteRouteState>("/admin/api/website/routes"),
        api<WebsiteState>("/admin/api/website/configuration"),
        api<WebsiteVersion[]>("/admin/api/website/versions"),
        api<SearchIntegrationSettings>("/admin/api/search-integrations"),
      ]);
      setRouteState(routes);
      setWebsiteState(website);
      setVersions(history);
      setSearchIntegrations(integrations);
      routeForm.setFieldsValue(routes.config);
      replaceFormValues(websiteForm, website.working);
      localizationForm.setFieldsValue(website.working_localization);
      searchForm.setFieldsValue(integrations);
      setRoutePreview(undefined);
		setBrandRequest(undefined);
		setBrandPayload("");
		setBrandValidation(undefined);
		setBrandRightsConfirmed(false);
		setWebsiteFormDirty(false);
    } catch (error) {
      onError(error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

	const saveWorking = async (configuration: SiteConfiguration) => {
		const currentWebsiteState = websiteState;
		if (!currentWebsiteState) return;
		setLoading(true);
		try {
			const navigation = (configuration.navigation ?? currentWebsiteState.working.navigation ?? []).map((item) => {
        const targetType = item.target_type ?? (item.system_action ? "system_action" : "link");
        return {
          ...item,
          target_type: targetType,
          url: targetType === "link" ? item.url : "",
          system_action: targetType === "system_action" ? item.system_action : undefined,
          open_new_window: targetType === "link" && item.open_new_window,
          presentation: item.presentation ?? "direct",
        };
      });
      const completeConfiguration: SiteConfiguration = {
				...currentWebsiteState.working,
				...configuration,
				organization: { ...currentWebsiteState.working.organization, ...configuration.organization },
				header: configuration.header ?? currentWebsiteState.working.header,
        navigation,
				footer: configuration.footer ?? currentWebsiteState.working.footer,
				theme: { ...currentWebsiteState.working.theme, ...configuration.theme },
				seo: { ...currentWebsiteState.working.seo, ...configuration.seo },
				category_listing_profiles: configuration.category_listing_profiles ?? currentWebsiteState.working.category_listing_profiles,
				brand_import: configuration.brand_import ?? currentWebsiteState.working.brand_import,
			};
			const next = await putJSON<WebsiteState>("/admin/api/website/configuration", {
				expected_revision: currentWebsiteState.working_revision,
				configuration: completeConfiguration,
			});
      setWebsiteState(next);
      replaceFormValues(websiteForm, next.working);
      localizationForm.setFieldsValue(next.working_localization);
      setRoutePreview(undefined);
		setBrandRequest(undefined);
		setBrandPayload("");
		setBrandValidation(undefined);
		setBrandRightsConfirmed(false);
		setWebsiteFormDirty(false);
      onMessage(text.saved(next.working_revision));
    } catch (error) {
      onError(error);
      await load();
    } finally {
      setLoading(false);
    }
  };

  const saveLocalization = async (localization: WebsiteLocalization) => {
    if (!websiteState) return;
    setLoading(true);
    try {
      const next = await putJSON<WebsiteState>(
        "/admin/api/website/localization",
        {
          expected_working_revision: websiteState.working_revision,
          localization: {
            ...localization,
            public_copy_overrides:
              websiteState.working_localization.public_copy_overrides ?? {},
          },
        },
      );
      setWebsiteState(next);
      localizationForm.setFieldsValue(next.working_localization);
      setRoutePreview(undefined);
      onMessage(text.localizationSaved(next.working_revision));
    } catch (error) {
      onError(error);
      await load();
    } finally {
      setLoading(false);
    }
  };

  const previewWebsite = async () => {
    if (!websiteState) return;
    setPreviewing(true);
    setLoading(true);
    try {
      const receipt = await postJSON<{ url: string }>("/admin/api/website/preview", {
        expected_working_revision: websiteState.working_revision,
      });
      window.location.assign(receipt.url);
    } catch (error) {
      onError(error);
    } finally {
      setPreviewing(false);
      setLoading(false);
    }
  };

  const uploadWebsiteImage = async (file: File, field: "primary_logo_asset_id" | "dark_logo_asset_id" | "favicon_asset_id" | "social_image_asset_id") => {
    setLoading(true);
    try {
      const formData = new FormData();
      formData.set("file", file);
      const asset = await api<Asset>("/admin/api/website/assets", { method: "POST", body: formData });
      websiteForm.setFieldValue(["organization", field], asset.id);
      onMessage(text.uploaded(asset.original_filename));
    } catch (error) {
      onError(error);
    } finally {
      setLoading(false);
    }
    return false;
  };

  const previewRoutes = async (config: SiteRouteConfig) => {
    setLoading(true);
    try {
      const next = await postJSON<SiteRoutePreview>("/admin/api/website/routes/preview", config);
      setRoutePreview(next);
      onMessage(text.routesChecked(next.affected, next.working_revision));
    } catch (error) {
      setRoutePreview(undefined);
      onError(error);
    } finally {
      setLoading(false);
    }
  };

  const publish = async () => {
    if (!routeState || !websiteState || !routePreview || routePreview.missing.length || routePreview.conflicts.length) return;
    setLoading(true);
    try {
      await postJSON<SiteRoutePreview>("/admin/api/website/routes/publish", {
        expected_epoch: routeState.current_epoch,
        expected_working_revision: websiteState.working_revision,
        config: routeForm.getFieldsValue(),
      });
      onMessage(text.published(routePreview.candidate_epoch));
      await load();
    } catch (error) {
      onError(error);
      await load();
    } finally {
      setLoading(false);
    }
  };

  const restoreVersion = async (version: number) => {
    if (!websiteState) return;
    setLoading(true);
    try {
      const next = await postJSON<WebsiteState>("/admin/api/website/versions/restore", {
        expected_working_revision: websiteState.working_revision,
        version,
      });
      setWebsiteState(next);
      replaceFormValues(websiteForm, next.working);
      localizationForm.setFieldsValue(next.working_localization);
      setRoutePreview(undefined);
		setBrandRequest(undefined);
		setBrandPayload("");
		setBrandValidation(undefined);
		setBrandRightsConfirmed(false);
		setWebsiteFormDirty(false);
      onMessage(text.restored(version, next.working_revision));
    } catch (error) {
      onError(error);
      await load();
    } finally {
      setLoading(false);
    }
  };

	const generateBrandPrompt = async () => {
		if (!websiteState || websiteFormDirty) return;
		const sourceURL = brandSourceURL.trim();
		try {
			const parsed = new URL(sourceURL);
			if (parsed.protocol !== "http:" && parsed.protocol !== "https:") throw new Error(text.absoluteURL);
		} catch (error) {
			onError(error instanceof Error ? error : new Error(text.validURL));
			return;
		}
		setLoading(true);
		try {
			const result = await postJSON<BrandImportRequest>("/admin/api/website/brand-import/requests", { source_url: sourceURL });
			setBrandRequest(result);
			setBrandPayload("");
			setBrandValidation(undefined);
			setBrandRightsConfirmed(false);
		} catch (error) {
			onError(error);
		} finally {
			setLoading(false);
		}
	};

	const copyBrandPrompt = async () => {
		if (!brandRequest) return;
		try {
			await navigator.clipboard.writeText(brandRequest.prompt);
			onMessage(brandText.copied);
		} catch (error) {
			onError(error);
		}
	};

	const validateBrandPayload = async () => {
		if (!brandRequest || !brandPayload.trim() || websiteFormDirty) return;
		setLoading(true);
		try {
			const result = await postJSON<BrandImportValidation>("/admin/api/website/brand-import/validate", {
				request_id: brandRequest.request_id,
				payload: brandPayload,
			});
			setBrandValidation(result);
			setBrandRightsConfirmed(false);
		} catch (error) {
			setBrandValidation(undefined);
			setBrandRightsConfirmed(false);
			onError(error);
		} finally {
			setLoading(false);
		}
	};

	const applyBrandImport = async () => {
		if (!brandRequest || !brandValidation?.proposed_configuration || !brandRightsConfirmed || websiteFormDirty || brandValidation.working_revision !== websiteState?.working_revision) return;
		setLoading(true);
		try {
			const next = await postJSON<WebsiteState>("/admin/api/website/brand-import/apply", {
				request_id: brandRequest.request_id,
				expected_revision: brandValidation.working_revision,
				rights_confirmed: true,
			});
			setWebsiteState(next);
      replaceFormValues(websiteForm, next.working);
			localizationForm.setFieldsValue(next.working_localization);
			setWebsiteFormDirty(false);
			setRoutePreview(undefined);
			onMessage(brandText.applied);
		} catch (error) {
			onError(error);
			await load();
		} finally {
			setLoading(false);
		}
	};

  const saveSearchIntegrations = async (values: SearchIntegrationSettings) => {
    if (!searchIntegrations) return;
    setLoading(true);
    try {
      const next = await putJSON<SearchIntegrationSettings>("/admin/api/search-integrations", {
        revision: searchIntegrations.revision,
        indexnow_enabled: values.indexnow_enabled,
        google_enabled: values.google_enabled,
        google_site_url: values.google_site_url ?? "",
      });
      setSearchIntegrations(next);
      searchForm.setFieldsValue(next);
      onMessage(text.searchSaved);
    } catch (error) {
      onError(error);
      const latest = await api<SearchIntegrationSettings>("/admin/api/search-integrations").catch(() => undefined);
      if (latest) {
        setSearchIntegrations(latest);
        searchForm.setFieldsValue(latest);
      }
    } finally {
      setLoading(false);
    }
  };

  const setCustomCSSDisabled = async (disabled: boolean) => {
    setLoading(true);
    try {
      const next = await postJSON<WebsiteState>("/admin/api/website/custom-css", { disabled });
      setWebsiteState(next);
      onMessage(text.cssState(disabled));
    } catch (error) {
      onError(error);
      await load();
    } finally {
      setLoading(false);
    }
  };

  const issues = [...(routePreview?.missing ?? []), ...(routePreview?.conflicts ?? [])];
  const clean =
    routePreview !== undefined &&
    issues.length === 0 &&
    routePreview.current_epoch === routeState?.current_epoch &&
    routePreview.working_revision === websiteState?.working_revision;

  return (
    <Space direction="vertical" size="large" className="panel-stack">
      <Card title={text.workingConfiguration} loading={!websiteState && loading}>
        {websiteState && (
          <Descriptions
            size="small"
            column={3}
            items={[
              { key: "working", label: text.workingRevision, children: websiteState.working_revision },
              { key: "active", label: text.activeVersion, children: websiteState.active_version },
              { key: "epoch", label: text.activeEpoch, children: websiteState.active_epoch },
            ]}
          />
        )}
        {websiteState?.custom_css_disabled ? (
          <Alert
            type="warning"
            showIcon
            message={text.cssSafeMode}
            description={text.cssSafeDescription}
            action={<Button onClick={() => void setCustomCSSDisabled(false)} loading={loading}>{text.reenableCSS}</Button>}
          />
        ) : (
          <Popconfirm title={text.disableCSSConfirm} onConfirm={() => void setCustomCSSDisabled(true)}>
            <Button danger loading={loading}>{text.disableCSS}</Button>
          </Popconfirm>
        )}
        <Typography.Paragraph type="secondary">
          {text.workingHelp}
        </Typography.Paragraph>
		<Form
			form={localizationForm}
			layout="vertical"
			onFinish={(values) => void saveLocalization(values)}
		>
			<Card size="small" title={text.localization}>
				<Alert
					className="bottom-gap"
					type="info"
					showIcon
					message={text.localizationHelp}
					description={`${text.activeLocales}: ${websiteState?.active_localization.enabled_locales.map(localeLabel).join(", ") ?? ""}`}
				/>
				<Form.Item
					name="default_locale"
					label={text.defaultLocale}
					rules={[{ required: true }]}
				>
					<Select options={websiteLocaleOptions} />
				</Form.Item>
				<Form.Item
					name="enabled_locales"
					label={text.enabledLocales}
					dependencies={["default_locale"]}
					rules={[
						{ required: true },
						({ getFieldValue }) => ({
							validator: async (_, value: string[]) => {
								if (!value?.includes(getFieldValue("default_locale"))) {
									throw new Error(`${text.defaultLocale}: ${localeLabel(getFieldValue("default_locale"))}`);
								}
							},
						}),
					]}
				>
					<Select mode="multiple" options={websiteLocaleOptions} />
				</Form.Item>
				<Form.Item name="content_editing_enabled" valuePropName="checked">
					<Checkbox>{text.contentEditingEnabled}</Checkbox>
				</Form.Item>
				<Button type="primary" htmlType="submit" loading={loading}>
					{text.saveLocalization}
				</Button>
			</Card>
		</Form>
		<Card size="small" title={brandText.title}>
			<Typography.Paragraph type="secondary">{brandText.help}</Typography.Paragraph>
			<Alert type="info" showIcon message={brandText.exactSource} />
			<Space.Compact block>
				<Input type="url" value={brandSourceURL} onChange={(event) => setBrandSourceURL(event.target.value)} placeholder="https://www.example.com/" aria-label={brandText.sourceURL} />
				<Button onClick={() => void generateBrandPrompt()} disabled={!brandSourceURL.trim() || websiteFormDirty} loading={loading}>{brandText.generate}</Button>
			</Space.Compact>
			{websiteFormDirty && <Alert type="warning" showIcon message={text.dirtyCapture} description={text.dirtyCaptureDescription} action={<Button onClick={() => void load()} disabled={loading}>{text.reloadWorking}</Button>} />}
			{brandRequest && (
				<Space direction="vertical" className="panel-stack">
					<Descriptions size="small" column={2} items={[
						{ key: "source", label: brandText.sourceURL, children: brandRequest.source_url },
						{ key: "expires", label: brandText.expires, children: new Date(brandRequest.expires_at).toLocaleString(locale) },
					]} />
					<Typography.Title level={5}>{brandText.prompt}</Typography.Title>
					<Input.TextArea value={brandRequest.prompt} readOnly autoSize={{ minRows: 10, maxRows: 18 }} />
					<Button onClick={() => void copyBrandPrompt()}>{brandText.copyPrompt}</Button>
					<Typography.Title level={5}>{brandText.pasteResult}</Typography.Title>
					<Input.TextArea value={brandPayload} onChange={(event) => { setBrandPayload(event.target.value); setBrandValidation(undefined); setBrandRightsConfirmed(false); }} autoSize={{ minRows: 10, maxRows: 22 }} placeholder="{ ... }" />
					<Button type="primary" onClick={() => void validateBrandPayload()} disabled={!brandPayload.trim() || websiteFormDirty} loading={loading}>{brandText.validate}</Button>
				</Space>
			)}
			{brandValidation && (
				<Space direction="vertical" className="panel-stack">
					<Descriptions size="small" items={[{ key: "status", label: brandText.status, children: brandValidation.status }]} />
					{brandValidation.status === "unavailable" && <Alert type="warning" showIcon message={brandText.unavailable} />}
					{(brandValidation.limitations ?? []).map((limitation) => <Alert key={limitation} type="warning" showIcon message={brandText.limitations} description={limitation} />)}
					<Table<BrandImportFieldChange>
						rowKey="field" size="small" pagination={false} dataSource={brandValidation.diff} locale={{ emptyText: brandText.noChanges }}
						columns={[
							{ title: text.field, dataIndex: "field" },
							{ title: brandText.current, dataIndex: "before", render: (value: unknown) => <Typography.Text code>{JSON.stringify(value, null, 2)}</Typography.Text> },
							{ title: brandText.proposed, dataIndex: "after", render: (value: unknown) => <Typography.Text code>{JSON.stringify(value, null, 2)}</Typography.Text> },
						]}
					/>
					{brandValidation.proposed_configuration && (
						<>
							<Checkbox checked={brandRightsConfirmed} onChange={(event) => setBrandRightsConfirmed(event.target.checked)}>{brandText.rights}</Checkbox>
							<Space>
								<Button type="primary" onClick={() => void applyBrandImport()} disabled={!brandRightsConfirmed || websiteFormDirty || brandValidation.working_revision !== websiteState?.working_revision} loading={loading}>{brandText.apply}</Button>
								{brandValidation.working_revision !== websiteState?.working_revision && <Button onClick={() => void previewWebsite()} loading={previewing}>{previewing ? `${brandText.preview}…` : brandText.preview}</Button>}
							</Space>
						</>
					)}
				</Space>
			)}
		</Card>
        <Form form={websiteForm} layout="vertical" onFinish={(values) => void saveWorking(values)} onValuesChange={() => setWebsiteFormDirty(true)}>
          <Card size="small" title={text.organization}>
            <Form.Item name={["organization", "display_name"]} label={text.displayName} rules={[{ required: true }]}>
              <Input />
            </Form.Item>
            <Space wrap align="start">
              <Form.Item name={["organization", "legal_name"]} label={text.legalName}><Input /></Form.Item>
              <Form.Item name={["organization", "official_website"]} label={text.officialWebsite} rules={[{ type: "url" }]}><Input /></Form.Item>
              <Form.Item name={["organization", "privacy_url"]} label={text.privacyURL} rules={[{ type: "url" }]}><Input /></Form.Item>
              <Form.Item name={["organization", "terms_url"]} label={text.termsURL} rules={[{ type: "url" }]}><Input /></Form.Item>
            </Space>
            <Space wrap align="start">
              {([
                ["primary_logo_asset_id", text.primaryLogo],
                ["dark_logo_asset_id", text.darkLogo],
                ["favicon_asset_id", text.favicon],
                ["social_image_asset_id", text.socialImage],
              ] as const).map(([field, label]) => (
                <Space direction="vertical" key={field}>
                  <Form.Item name={["organization", field]} label={`${label} ${text.assetID}`}><Input readOnly allowClear /></Form.Item>
                  <Upload accept=".jpg,.jpeg,.png,.webp,.svg" showUploadList={false} beforeUpload={(file) => { void uploadWebsiteImage(file, field); return false; }}>
                    <Button loading={loading}>{text.upload} {label}</Button>
                  </Upload>
                </Space>
              ))}
            </Space>
            <Form.List name={["organization", "contact_links"]}>
              {(fields, { add, remove }) => (
                <Space direction="vertical" className="panel-stack">
                  {fields.map((field) => (
                    <Space key={field.key} wrap align="start">
                      <Form.Item {...field} name={[field.name, "id"]} label={text.contactID} rules={[{ required: true }]}><Input /></Form.Item>
                      <Form.Item {...field} name={[field.name, "label"]} label={text.label} rules={[{ required: true }]}><Input /></Form.Item>
                      <Form.Item {...field} name={[field.name, "url"]} label={text.url} rules={[{ required: true }, { type: "url" }]}><Input /></Form.Item>
                      <Form.Item {...field} name={[field.name, "sort_order"]} label={text.order}><InputNumber /></Form.Item>
                      <Button danger onClick={() => remove(field.name)}>{text.removeContact}</Button>
                    </Space>
                  ))}
                  <Button onClick={() => add({ id: "", label: "", url: "", sort_order: fields.length * 10 })}>{text.addContact}</Button>
                </Space>
              )}
            </Form.List>
          </Card>

          <Card size="small" title={text.navigation}>
            <Form.List name="navigation">
              {(fields, { add, remove }) => (
                <Space direction="vertical" className="panel-stack">
                  {fields.map((field) => (
                    <Space key={field.key} wrap align="start">
                      <Form.Item {...field} name={[field.name, "id"]} label={text.id} rules={[{ required: true }]}><Input /></Form.Item>
                      <Form.Item {...field} name={[field.name, "parent_id"]} label={text.parentID}><Input /></Form.Item>
                      <Form.Item {...field} name={[field.name, "label"]} label={text.label} rules={[{ required: true }]}><Input /></Form.Item>
                      <NavigationTargetFields fieldName={field.name} copy={navigationText} urlLabel={text.url} newWindowLabel={text.newWindow} />
                      <Form.Item {...field} name={[field.name, "sort_order"]} label={text.order}><InputNumber /></Form.Item>
                      <Form.Item {...field} name={[field.name, "visible"]} valuePropName="checked"><Checkbox>{text.visible}</Checkbox></Form.Item>
                      <Button danger onClick={() => remove(field.name)}>{text.removeLink}</Button>
                    </Space>
                  ))}
                  <Button onClick={() => add({ id: "", label: "", url: "/", target_type: "link", presentation: "direct", sort_order: fields.length * 10, visible: true, open_new_window: false })}>{text.addLink}</Button>
                </Space>
              )}
            </Form.List>
          </Card>

          <Card size="small" title={text.themeSEO}>
            <Space wrap align="start">
              <Form.Item name={["theme", "primary_color"]} label={text.primaryColor} rules={[{ required: true }, { pattern: /^#[0-9a-fA-F]{6}$/ }]}><Input type="color" /></Form.Item>
              <Form.Item name={["theme", "secondary_color"]} label={text.secondaryColor} rules={[{ required: true }, { pattern: /^#[0-9a-fA-F]{6}$/ }]}><Input type="color" /></Form.Item>
              <Form.Item name={["theme", "font_family"]} label={text.fontFamily} rules={[{ required: true }]}><Input /></Form.Item>
              <Form.Item name={["theme", "content_width_px"]} label={text.contentWidth} rules={[{ required: true }]}><InputNumber min={640} max={1920} /></Form.Item>
            </Space>
            <Form.Item name={["theme", "custom_css_enabled"]} valuePropName="checked"><Checkbox>{text.applyCSS}</Checkbox></Form.Item>
            <Form.Item name={["theme", "custom_css"]} label={text.customCSS}><Input.TextArea rows={6} /></Form.Item>
            <Form.Item name={["seo", "default_title"]} label={text.defaultTitle}><Input /></Form.Item>
            <Form.Item name={["seo", "default_description"]} label={text.defaultDescription}><Input.TextArea rows={3} /></Form.Item>
          </Card>

          <Space>
            <Button type="primary" htmlType="submit" loading={loading}>{text.saveWorking}</Button>
            <Button onClick={() => void previewWebsite()} disabled={!websiteState} loading={previewing}>{previewing ? `${text.previewWorking}…` : text.previewWorking}</Button>
          </Space>
        </Form>
      </Card>

      <Card title={text.searchExposure} loading={!searchIntegrations && loading}>
        <Space direction="vertical" className="panel-stack">
          <Alert
            showIcon
            type="info"
            message={text.optionalEnhancement}
            description={text.searchHelp}
          />
          {searchIntegrations && (
            <Form form={searchForm} layout="vertical" onFinish={(values) => void saveSearchIntegrations(values)}>
              <Card size="small" title={text.indexNowTitle}>
                <Form.Item name="indexnow_enabled" valuePropName="checked">
                  <Checkbox disabled={!searchIntegrations.indexnow_available}>{text.enableIndexNow}</Checkbox>
                </Form.Item>
                {!searchIntegrations.indexnow_available && <Alert showIcon type="warning" message={text.indexNowUnavailable} />}
                {searchIntegrations.indexnow_key_url && (
                  <Descriptions size="small" items={[{ key: "key", label: text.keyURL, children: <Typography.Text copyable>{searchIntegrations.indexnow_key_url}</Typography.Text> }]} />
                )}
              </Card>
              <Card size="small" title={text.googleTitle}>
                <Form.Item name="google_enabled" valuePropName="checked">
                  <Checkbox disabled={!searchIntegrations.google_available}>{text.enableGoogle}</Checkbox>
                </Form.Item>
                {!searchIntegrations.google_available && <Alert showIcon type="warning" message={text.googleUnavailable} />}
                <Form.Item
                  name="google_site_url"
                  label={text.siteProperty}
                  extra={text.sitePropertyHelp}
                  rules={[({ getFieldValue }) => ({ validator(_, value?: string) { return !getFieldValue("google_enabled") || value?.trim() ? Promise.resolve() : Promise.reject(new Error(text.sitePropertyRequired)); } })]}
                >
                  <Input disabled={!searchIntegrations.google_available} />
                </Form.Item>
              </Card>
              <Button type="primary" htmlType="submit" loading={loading}>{text.saveSearch}</Button>
            </Form>
          )}
        </Space>
      </Card>

      <Card title={text.publicRoutes} loading={!routeState && loading}>
        {routeState && (
          <Descriptions size="small" column={3} items={[
            { key: "epoch", label: text.activeEpoch, children: routeState.current_epoch },
            { key: "prefix", label: text.activePrefix, children: routeState.config.product_prefix },
            { key: "pattern", label: text.activePattern, children: routeState.config.url_pattern },
          ]} />
        )}
        <Typography.Paragraph type="secondary">
          {text.routesHelp}
        </Typography.Paragraph>
        <Form form={routeForm} layout="inline" onFinish={(values) => void previewRoutes(values)} onValuesChange={() => setRoutePreview(undefined)}>
          <Form.Item name="product_prefix" label={text.productPrefix} rules={[{ required: true }, { pattern: /^\/[A-Za-z0-9._-]+$/, message: text.safeSegment }]}>
            <Input placeholder="/products" />
          </Form.Item>
          <Form.Item name="url_pattern" label={text.pattern} rules={[{ required: true }]}>
            <Select style={{ width: 190 }} options={[
              { value: "compact", label: text.compact },
              { value: "manufacturer", label: text.manufacturerPath },
              { value: "brand", label: text.brandPath },
              { value: "category", label: text.categoryPath },
            ]} />
          </Form.Item>
          <Button htmlType="submit" loading={loading}>{text.previewSite}</Button>
        </Form>
      </Card>

      {routePreview && (
        <Card title={text.candidateEpoch(routePreview.candidate_epoch)}>
          {clean ? (
            <Alert type="success" showIcon message={text.routesReady(routePreview.affected, routePreview.working_revision)} />
          ) : (
            <Alert type="error" showIcon message={text.cannotPublish} description={text.cannotPublishDescription} />
          )}
          {issues.length > 0 && (
            <Table<SiteRouteIssue>
              rowKey={(row) => `${row.product_id}:${row.reason}:${row.route ?? ""}`}
              pagination={false}
              dataSource={issues}
              columns={[
                { title: text.partNumber, dataIndex: "part_number" },
                { title: text.productID, dataIndex: "product_id" },
                { title: text.targetRoute, dataIndex: "route", render: (value?: string) => value || "—" },
                { title: text.issue, dataIndex: "reason" },
              ]}
            />
          )}
          <Popconfirm title={text.publishConfirm} onConfirm={() => void publish()} disabled={!clean}>
            <Button type="primary" disabled={!clean} loading={loading}>{text.publishWebsite}</Button>
          </Popconfirm>
        </Card>
      )}

      <Card title={text.publishedVersions}>
        <Table<WebsiteVersion>
          rowKey="version"
          pagination={false}
          dataSource={versions}
          columns={[
            { title: text.version, dataIndex: "version" },
            { title: text.siteEpoch, dataIndex: "site_epoch" },
            { title: text.organization, render: (_, row) => row.configuration.organization.display_name },
            { title: text.created, dataIndex: "created_at", render: (value: string) => new Date(value).toLocaleString(locale) },
            {
              title: text.action,
              render: (_, row) => (
                <Popconfirm title={text.restoreConfirm(row.version)} onConfirm={() => void restoreVersion(row.version)}>
                  <Button disabled={loading}>{text.restoreWorking}</Button>
                </Popconfirm>
              ),
            },
          ]}
        />
      </Card>
    </Space>
  );
}
