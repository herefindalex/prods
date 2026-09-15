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
import type {
  Asset,
  BrandCaptureFieldChange,
  BrandCaptureResponse,
  SiteConfiguration,
  SiteRouteConfig,
  SiteRouteIssue,
  SiteRoutePreview,
  SiteRouteState,
  SearchIntegrationSettings,
  WebsiteState,
  WebsiteVersion,
} from "./types";

type Props = {
  locale: "en-US" | "zh-TW";
  onError: (error: unknown) => void;
  onMessage: (message: string) => void;
};

const labels = {
  "en-US": {
    saved: (revision: number) => `Saved Website working revision ${revision}. The public site is unchanged.`,
    uploaded: (name: string) => `Uploaded ${name}. Save the working copy to reference it.`,
    routesChecked: (affected: number, revision: number) => `Checked ${affected} Published product routes against Website working revision ${revision}.`,
    published: (epoch: number) => `Published Website configuration and every Product representation at site epoch ${epoch}.`,
    restored: (version: number, revision: number) => `Restored Website version ${version} into working revision ${revision}. Preview and Publish are still required.`,
    absoluteURL: "Enter an absolute HTTP or HTTPS URL.", validURL: "Enter a valid Website URL.",
    captured: (revision: number) => `Captured a review-only candidate against Website working revision ${revision}.`,
    applied: "Applied the candidate to this browser form only. Save working copy, Preview, and Publish are still required.",
    searchSaved: "Search integration settings saved. External submissions run independently.",
    cssState: (disabled: boolean) => disabled ? "Custom CSS Safe Mode is active for all new public requests." : "Custom CSS was re-enabled for new public requests.",
    workingConfiguration: "Website working configuration", workingRevision: "Working revision", activeVersion: "Active version", activeEpoch: "Active site epoch",
    cssSafeMode: "Custom CSS Safe Mode is active", cssSafeDescription: "Public requests cannot retrieve the active custom stylesheet. Admin and system pages are unaffected.",
    reenableCSS: "Re-enable Custom CSS", disableCSSConfirm: "Immediately stop serving Custom CSS to new public requests?", disableCSS: "Disable Custom CSS now",
    workingHelp: "Save changes into the durable working copy. Preview is private. Only Publish changes the public Website and Product artifacts.",
    captureTitle: "Brand Capture candidate", captureHelp: "Fetch bounded public HTML/CSS to suggest Organization, Theme, and Navigation. It never imports source scripts, canonical/SEO settings, analytics, raw HTML/CSS, or assets. Dynamic JavaScript-only content may remain unavailable.",
    sourceURL: "Source Website URL", analyze: "Analyze", dirtyCapture: "Save or reload local form edits before re-capturing.",
    dirtyCaptureDescription: "The server compares a capture with the durable Website working revision, not unsaved browser fields.", reloadWorking: "Reload saved working",
    dynamicUnavailable: "Some source content requires JavaScript and was not captured.", unavailable: "Unavailable", dynamicContent: "dynamic content", completeManually: "Complete these fields manually.", stylesheetUnavailable: "Stylesheet unavailable:",
    organization: "Organization", sourceTitle: "Source title", colors: "Colors", fonts: "Fonts", logoReferences: "Logo references", manualCorrections: "Manual corrections",
    manualInput: "Manual input required", unavailableValue: "Not available", manualUpload: "Manual upload required", noneIdentified: "None identified",
    noCaptureChanges: "The capture does not change the saved working configuration.", field: "Field", currentWorking: "Current working", candidate: "Candidate",
    rights: "I confirm that we have the right to use the selected branding and references.", applyInfo: "Applying only updates this browser form.",
    applyInfoDescription: "You must still Save working copy, Preview, and Publish. Logo URLs are references for review and are not downloaded or attached automatically.", applyCandidate: "Apply candidate to form",
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
    captured: (revision: number) => `已依 Website 工作修訂 ${revision} 擷取僅供檢視的候選內容。`,
    applied: "候選內容只套用到目前瀏覽器表單；仍須儲存工作副本、預覽及發布。",
    searchSaved: "已儲存搜尋整合設定；外部提交會獨立執行。",
    cssState: (disabled: boolean) => disabled ? "所有新的公開請求已啟用 Custom CSS 安全模式。" : "新的公開請求已重新啟用 Custom CSS。",
    workingConfiguration: "Website 工作設定", workingRevision: "工作修訂", activeVersion: "啟用版本", activeEpoch: "啟用站點 epoch",
    cssSafeMode: "Custom CSS 安全模式已啟用", cssSafeDescription: "公開請求無法取得啟用中的自訂樣式；Admin 與系統頁面不受影響。",
    reenableCSS: "重新啟用 Custom CSS", disableCSSConfirm: "要立即停止向新的公開請求提供 Custom CSS 嗎？", disableCSS: "立即停用 Custom CSS",
    workingHelp: "先將變更儲存到持久工作副本。預覽是私有的；只有發布才會變更公開 Website 與 Product artifacts。",
    captureTitle: "品牌擷取候選內容", captureHelp: "有限度地擷取公開 HTML／CSS，以建議組織、主題與導覽；不匯入來源 script、canonical／SEO 設定、analytics、原始 HTML／CSS 或資產。只由動態 JavaScript 產生的內容可能無法取得。",
    sourceURL: "來源 Website URL", analyze: "分析", dirtyCapture: "重新擷取前，請儲存或重新載入本機表單修改。",
    dirtyCaptureDescription: "伺服器會將擷取結果與持久的 Website 工作修訂比較，而不是與未儲存的瀏覽器欄位比較。", reloadWorking: "重新載入已儲存工作副本",
    dynamicUnavailable: "部分來源內容需要 JavaScript，因此未能擷取。", unavailable: "無法取得", dynamicContent: "動態內容", completeManually: "請手動完成這些欄位。", stylesheetUnavailable: "無法取得樣式表：",
    organization: "組織", sourceTitle: "來源標題", colors: "色彩", fonts: "字型", logoReferences: "Logo 參照", manualCorrections: "手動修正",
    manualInput: "需要手動輸入", unavailableValue: "無法取得", manualUpload: "需要手動上傳", noneIdentified: "未發現",
    noCaptureChanges: "擷取結果不會變更已儲存的工作設定。", field: "欄位", currentWorking: "目前工作值", candidate: "候選值",
    rights: "我確認我們有權使用所選品牌內容與參照。", applyInfo: "套用只會更新目前的瀏覽器表單。",
    applyInfoDescription: "仍須執行儲存工作副本、預覽與發布。Logo URL 僅供檢視，不會自動下載或附加。", applyCandidate: "將候選內容套用到表單",
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
} as const;

export function WebsitePanel({ locale, onError, onMessage }: Props) {
  const text = labels[locale];
  const brandFieldNames: Record<string, string> = {
    organization: text.organization,
    logo: text.primaryLogo,
    colors: text.colors,
    fonts: text.fonts,
    navigation: text.navigation,
  };
  const brandFieldList = (values?: string[]) => (values ?? []).map((value) => brandFieldNames[value] ?? value).join(", ");
  const brandWarning = (warning: string) => warning.startsWith("Stylesheet unavailable:")
    ? `${text.stylesheetUnavailable}${warning.slice("Stylesheet unavailable:".length)}`
    : warning;
  const [routeState, setRouteState] = useState<SiteRouteState>();
  const [websiteState, setWebsiteState] = useState<WebsiteState>();
  const [searchIntegrations, setSearchIntegrations] = useState<SearchIntegrationSettings>();
  const [versions, setVersions] = useState<WebsiteVersion[]>([]);
  const [routePreview, setRoutePreview] = useState<SiteRoutePreview>();
	const [brandCapture, setBrandCapture] = useState<BrandCaptureResponse>();
	const [brandSourceURL, setBrandSourceURL] = useState("");
	const [brandRightsConfirmed, setBrandRightsConfirmed] = useState(false);
	const [websiteFormDirty, setWebsiteFormDirty] = useState(false);
  const [loading, setLoading] = useState(false);
  const [routeForm] = Form.useForm<SiteRouteConfig>();
  const [websiteForm] = Form.useForm<SiteConfiguration>();
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
      websiteForm.setFieldsValue(website.working);
      searchForm.setFieldsValue(integrations);
      setRoutePreview(undefined);
		setBrandCapture(undefined);
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
    if (!websiteState) return;
    setLoading(true);
    try {
      const next = await putJSON<WebsiteState>("/admin/api/website/configuration", {
        expected_revision: websiteState.working_revision,
        configuration,
      });
      setWebsiteState(next);
      websiteForm.setFieldsValue(next.working);
      setRoutePreview(undefined);
		setBrandCapture(undefined);
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

  const previewWebsite = async () => {
    if (!websiteState) return;
    const previewWindow = window.open("about:blank", "_blank");
    setLoading(true);
    try {
      const receipt = await postJSON<{ url: string }>("/admin/api/website/preview", {
        expected_working_revision: websiteState.working_revision,
      });
      if (previewWindow) previewWindow.location.assign(receipt.url);
      else window.open(receipt.url, "_blank", "noopener,noreferrer");
    } catch (error) {
      previewWindow?.close();
      onError(error);
    } finally {
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
      websiteForm.setFieldsValue(next.working);
      setRoutePreview(undefined);
		setBrandCapture(undefined);
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

	const captureBrand = async () => {
		if (!websiteState || websiteFormDirty) return;
		const sourceURL = brandSourceURL.trim();
		try {
			const parsed = new URL(sourceURL);
			if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
        throw new Error(text.absoluteURL);
			}
		} catch (error) {
      onError(error instanceof Error ? error : new Error(text.validURL));
			return;
		}
		setLoading(true);
		try {
			const result = await postJSON<BrandCaptureResponse>("/admin/api/website/capture", { source_url: sourceURL });
			setBrandCapture(result);
			setBrandRightsConfirmed(false);
      onMessage(text.captured(result.working_revision));
		} catch (error) {
			setBrandCapture(undefined);
			setBrandRightsConfirmed(false);
			onError(error);
		} finally {
			setLoading(false);
		}
	};

	const applyBrandCandidate = () => {
		if (!brandCapture || !brandRightsConfirmed || websiteFormDirty || brandCapture.working_revision !== websiteState?.working_revision) return;
		websiteForm.setFieldsValue(brandCapture.proposed_configuration);
		setWebsiteFormDirty(true);
		setRoutePreview(undefined);
    onMessage(text.applied);
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
			<Card size="small" title={text.captureTitle}>
			<Typography.Paragraph type="secondary">
					{text.captureHelp}
			</Typography.Paragraph>
			<Space.Compact block>
				<Input
					type="url"
					value={brandSourceURL}
					onChange={(event) => setBrandSourceURL(event.target.value)}
					placeholder="https://www.example.com"
						aria-label={text.sourceURL}
				/>
					<Button onClick={() => void captureBrand()} disabled={!brandSourceURL.trim() || websiteFormDirty} loading={loading}>{text.analyze}</Button>
			</Space.Compact>
			{websiteFormDirty && (
					<Alert type="warning" showIcon message={text.dirtyCapture} description={text.dirtyCaptureDescription} action={<Button onClick={() => void load()} disabled={loading}>{text.reloadWorking}</Button>} />
			)}
			{brandCapture && (
				<Space direction="vertical" className="panel-stack">
					{brandCapture.candidate.requires_browser && (
							<Alert type="warning" showIcon message={text.dynamicUnavailable} description={`${text.unavailable}: ${brandFieldList(brandCapture.candidate.unavailable_dynamic_parts) || text.dynamicContent}. ${text.completeManually}`} />
					)}
						{(brandCapture.candidate.warnings ?? []).map((warning) => <Alert key={warning} type="warning" showIcon message={brandWarning(warning)} />)}
					<Descriptions size="small" column={2} items={[
							{ key: "organization", label: text.organization, children: brandCapture.candidate.organization || text.manualInput },
							{ key: "title", label: text.sourceTitle, children: brandCapture.candidate.title || text.unavailableValue },
							{ key: "colors", label: text.colors, children: (brandCapture.candidate.colors ?? []).join(", ") || text.manualInput },
							{ key: "fonts", label: text.fonts, children: (brandCapture.candidate.fonts ?? []).join(", ") || text.manualInput },
							{ key: "logos", label: text.logoReferences, children: (brandCapture.candidate.logo_urls ?? []).join(", ") || text.manualUpload },
							{ key: "manual", label: text.manualCorrections, children: brandFieldList(brandCapture.candidate.manual_corrections) || text.noneIdentified },
					]} />
					<Table<BrandCaptureFieldChange>
						rowKey="field"
						size="small"
						pagination={false}
						dataSource={brandCapture.diff}
							locale={{ emptyText: text.noCaptureChanges }}
						columns={[
								{ title: text.field, dataIndex: "field" },
								{ title: text.currentWorking, dataIndex: "before", render: (value: string) => <Typography.Text code>{value}</Typography.Text> },
								{ title: text.candidate, dataIndex: "after", render: (value: string) => <Typography.Text code>{value}</Typography.Text> },
						]}
					/>
					<Checkbox checked={brandRightsConfirmed} onChange={(event) => setBrandRightsConfirmed(event.target.checked)}>
							{text.rights}
					</Checkbox>
						<Alert type="info" showIcon message={text.applyInfo} description={text.applyInfoDescription} />
						<Button type="primary" onClick={applyBrandCandidate} disabled={!brandRightsConfirmed || websiteFormDirty || brandCapture.working_revision !== websiteState?.working_revision}>{text.applyCandidate}</Button>
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
                      <Form.Item {...field} name={[field.name, "url"]} label={text.url} rules={[{ required: true }]}><Input /></Form.Item>
                      <Form.Item {...field} name={[field.name, "sort_order"]} label={text.order}><InputNumber /></Form.Item>
                      <Form.Item {...field} name={[field.name, "visible"]} valuePropName="checked"><Checkbox>{text.visible}</Checkbox></Form.Item>
                      <Form.Item {...field} name={[field.name, "open_new_window"]} valuePropName="checked"><Checkbox>{text.newWindow}</Checkbox></Form.Item>
                      <Button danger onClick={() => remove(field.name)}>{text.removeLink}</Button>
                    </Space>
                  ))}
                  <Button onClick={() => add({ id: "", label: "", url: "/", sort_order: fields.length * 10, visible: true, open_new_window: false })}>{text.addLink}</Button>
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
            <Button onClick={() => void previewWebsite()} disabled={!websiteState} loading={loading}>{text.previewWorking}</Button>
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
