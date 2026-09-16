import { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import { Alert, Button, ConfigProvider, Layout, Select, Space, Tabs, Typography } from "antd";
import { AccessPanel } from "./AccessPanel";
import { ActivityPanel } from "./ActivityPanel";
import { api, csrf, setAPILocale } from "./api";
import { BackupPanel } from "./BackupPanel";
import { CatalogPanel } from "./CatalogPanel";
import { HealthPanel } from "./HealthPanel";
import { ImportPanel } from "./ImportPanel";
import { JobsPanel } from "./JobsPanel";
import { LoginPage } from "./LoginPage";
import {
  ADMIN_LOCALE_OPTIONS,
  antDesignLocale,
  implementedAdminMessageLocale,
  isAdminLocale,
  normalizeAdminLocale,
  type AdminLocale,
} from "./locales";
import { ListingProfilesPanel } from "./ListingProfilesPanel";
import { PublicCopyPanel } from "./PublicCopyPanel";
import { ProductBulkPanel } from "./ProductBulkPanel";
import { SettingsPanel } from "./SettingsPanel";
import { TaxonomyPanel } from "./TaxonomyPanel";
import { TrafficPanel } from "./TrafficPanel";
import { WebsitePanel } from "./WebsitePanel";
import type { SystemHealth } from "./types";
import "./style.css";

const adminText = {
  "en-US": {
    subtitle: "Catalog operations",
    signOut: "Sign out",
    requestFailed: "Request failed",
    catalog: "Catalog",
    taxonomy: "Taxonomy",
    imports: "Imports",
    productBulk: "Product bulk",
    jobs: "Jobs",
    listingProfiles: "Listing profiles",
    website: "Website",
    publicCopy: "Public copy",
    access: "Users & roles",
    activity: "RFQs & Admin Log",
		health: "System health",
		settings: "Site settings",
    backups: "Backups",
    traffic: "Traffic protection",
    language: "Interface language",
    healthWarning: "System capacity needs attention",
    healthWarningDescription: "Normal work remains available, but one or more resources are approaching their admission floor. Open System health for the affected operations and remediation.",
  },
  "zh-TW": {
    subtitle: "型錄營運管理",
    signOut: "登出",
    requestFailed: "請求失敗",
    catalog: "產品型錄",
    taxonomy: "分類與字典",
    imports: "匯入",
    productBulk: "Product 批次",
    jobs: "工作",
    listingProfiles: "列表設定檔",
    website: "網站",
    publicCopy: "公開介面文案",
    access: "使用者與角色",
    activity: "詢價與管理紀錄",
		health: "系統健康狀態",
		settings: "站點設定",
    backups: "備份",
    traffic: "流量保護",
    language: "介面語言",
    healthWarning: "系統容量需要處理",
    healthWarningDescription: "一般操作仍可使用，但一項或多項資源已接近准入下限。請開啟系統健康狀態，查看受影響操作與修復建議。",
  },
} as const;

function messageFrom(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function initialLocale(): AdminLocale {
  const saved = window.localStorage.getItem("prods-admin-locale");
  if (isAdminLocale(saved)) return saved;
  return normalizeAdminLocale(document.documentElement.lang);
}

function AdminApp() {
  const [locale, setLocale] = useState<AdminLocale>(initialLocale);
  const [message, setMessage] = useState<string>();
  const [error, setError] = useState<string>();
  const [activeTab, setActiveTab] = useState("catalog");
  const [systemHealth, setSystemHealth] = useState<SystemHealth>();
  const messageLocale = implementedAdminMessageLocale(locale);
  const text = adminText[messageLocale];
  const showMessage = (next: string) => {
    setError(undefined);
    setMessage(next);
  };
  const showError = (next: unknown) => {
    setMessage(undefined);
    setError(messageFrom(next));
  };
  const logout = async () => {
    await fetch("/admin/logout", {
      method: "POST",
      headers: { "Accept-Language": locale, "X-CSRF-Token": csrf },
    });
    window.location.assign(`/admin/login?lang=${encodeURIComponent(locale)}`);
  };

  useEffect(() => {
    document.documentElement.lang = locale;
    window.localStorage.setItem("prods-admin-locale", locale);
    setAPILocale(locale);
  }, [locale]);

	useEffect(() => {
		let cancelled = false;
		void api<SystemHealth>("/admin/api/system/health")
			.then((health) => {
				if (cancelled) return;
				setSystemHealth(health);
				if (health.status === "Critical") setActiveTab("health");
			})
			.catch(() => {
				// The panel exposes the actionable error when the user opens it.
			});
		return () => {
			cancelled = true;
		};
	}, []);

  return (
    <ConfigProvider
      locale={antDesignLocale(locale)}
      theme={{
        token: {
          colorPrimary: "#147985",
          borderRadius: 6,
          colorBgLayout: "#f2f5f7",
        },
      }}
    >
      <Layout className="admin-shell">
        <Layout.Header className="admin-header">
          <Space className="header-content" align="center">
            <div>
              <Typography.Title level={2} className="brand-title">
                Prods
              </Typography.Title>
              <Typography.Text className="brand-subtitle">{text.subtitle}</Typography.Text>
            </div>
            <Space>
              <Select
                aria-label={text.language}
                value={locale}
                onChange={setLocale}
                options={[...ADMIN_LOCALE_OPTIONS]}
              />
              <Button ghost onClick={() => void logout()}>
                {text.signOut}
              </Button>
            </Space>
          </Space>
        </Layout.Header>
        <Layout.Content className="content">
          <Space direction="vertical" size="middle" className="panel-stack">
            {message && (
              <Alert closable type="success" showIcon message={message} onClose={() => setMessage(undefined)} />
            )}
            {error && (
              <Alert
                closable
                type="error"
                showIcon
                message={text.requestFailed}
                description={error}
                onClose={() => setError(undefined)}
              />
            )}
            {systemHealth?.status === "Warning" && (
              <Alert
                type="warning"
                showIcon
                message={text.healthWarning}
                description={text.healthWarningDescription}
                action={<Button size="small" onClick={() => setActiveTab("health")}>{text.health}</Button>}
              />
            )}
            <Tabs
			  activeKey={activeTab}
			  onChange={setActiveTab}
              destroyOnHidden
              items={[
                {
                  key: "catalog",
                  label: text.catalog,
                  children: <CatalogPanel locale={messageLocale} onError={showError} onMessage={showMessage} />,
                },
                {
                  key: "taxonomy",
                  label: text.taxonomy,
                  children: <TaxonomyPanel locale={messageLocale} onError={showError} onMessage={showMessage} />,
                },
                {
                  key: "imports",
                  label: text.imports,
                  children: <ImportPanel locale={messageLocale} onError={showError} onMessage={showMessage} />,
                },
                {
                  key: "product-bulk",
                  label: text.productBulk,
                  children: <ProductBulkPanel locale={messageLocale} onError={showError} onMessage={showMessage} />,
                },
                {
                  key: "jobs",
                  label: text.jobs,
                  children: <JobsPanel locale={messageLocale} onError={showError} onMessage={showMessage} />,
                },
                {
                  key: "website",
                  label: text.website,
                  children: <WebsitePanel locale={messageLocale} onError={showError} onMessage={showMessage} />,
                },
                {
                  key: "listing-profiles",
                  label: text.listingProfiles,
                  children: <ListingProfilesPanel locale={messageLocale} onError={showError} onMessage={showMessage} />,
                },
                {
                  key: "public-copy",
                  label: text.publicCopy,
                  children: <PublicCopyPanel locale={messageLocale} onError={showError} onMessage={showMessage} />,
                },
                {
                  key: "access",
                  label: text.access,
                  children: <AccessPanel locale={messageLocale} onError={showError} onMessage={showMessage} />,
                },
                {
                  key: "activity",
                  label: text.activity,
                  children: <ActivityPanel locale={messageLocale} onError={showError} />,
                },
				{
					key: "settings",
					label: text.settings,
                  children: <SettingsPanel locale={messageLocale} onError={showError} onMessage={showMessage} />,
				},
				{
					key: "health",
				  label: text.health,
                  children: <HealthPanel locale={messageLocale} onError={showError} />,
				},
				{
				  key: "backups",
				  label: text.backups,
                  children: <BackupPanel locale={messageLocale} onError={showError} onMessage={showMessage} />,
				},
				{
				  key: "traffic",
				  label: text.traffic,
                  children: <TrafficPanel locale={messageLocale} onError={showError} onMessage={showMessage} />,
				},
              ]}
            />
          </Space>
        </Layout.Content>
      </Layout>
    </ConfigProvider>
  );
}

const root = document.getElementById("admin-root");
if (root) createRoot(root).render(<AdminApp />);
const loginRoot = document.getElementById("admin-login-root");
if (loginRoot) createRoot(loginRoot).render(<LoginPage root={loginRoot} />);
