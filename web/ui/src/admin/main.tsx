import { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import { Alert, Button, ConfigProvider, Layout, Menu, Select, Space, Typography } from "antd";
import { AccessPanel } from "./AccessPanel";
import { ActivityPanel } from "./ActivityPanel";
import { api, setAPILocale } from "./api";
import { useLocation, useNavigate } from "react-router";
import { AdminProviders, authProvider, useAdminIdentity } from "./AdminProviders";
import { pageCapabilities } from "./resources";
import { BackupPanel } from "./BackupPanel";
import { CatalogPanel } from "./CatalogPanel";
import { HealthPanel } from "./HealthPanel";
import { ImportPanel } from "./ImportPanel";
import { JobsPanel } from "./JobsPanel";
import { LoginPage } from "./LoginPage";
import {
  ADMIN_LOCALE_OPTIONS,
  antDesignLocale,
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
  "zh-CN": {
    subtitle: "目录运营管理", signOut: "退出登录", requestFailed: "请求失败", catalog: "产品目录",
    taxonomy: "分类与字典", imports: "导入", productBulk: "产品批量操作", jobs: "任务",
    listingProfiles: "列表配置", website: "网站", publicCopy: "公开界面文案", access: "用户与角色",
    activity: "询价与管理日志", health: "系统健康状态", settings: "站点设置", backups: "备份",
    traffic: "流量防护", language: "界面语言", healthWarning: "系统容量需要处理",
    healthWarningDescription: "常规操作仍可使用，但一项或多项资源已接近准入下限。请打开系统健康状态，查看受影响的操作和修复建议。",
  },
  "ja-JP": {
    subtitle: "カタログ運用管理", signOut: "ログアウト", requestFailed: "リクエストに失敗しました", catalog: "製品カタログ",
    taxonomy: "分類と辞書", imports: "インポート", productBulk: "製品の一括操作", jobs: "ジョブ",
    listingProfiles: "一覧プロファイル", website: "ウェブサイト", publicCopy: "公開画面の文言", access: "ユーザーとロール",
    activity: "見積依頼と管理ログ", health: "システム状態", settings: "サイト設定", backups: "バックアップ",
    traffic: "トラフィック保護", language: "表示言語", healthWarning: "システム容量を確認してください",
    healthWarningDescription: "通常の操作は利用できますが、1つ以上のリソースが受付下限に近づいています。影響を受ける操作と対処方法はシステム状態で確認してください。",
  },
  "ko-KR": {
    subtitle: "카탈로그 운영 관리", signOut: "로그아웃", requestFailed: "요청 실패", catalog: "제품 카탈로그",
    taxonomy: "분류 및 사전", imports: "가져오기", productBulk: "제품 일괄 작업", jobs: "작업",
    listingProfiles: "목록 프로필", website: "웹사이트", publicCopy: "공개 화면 문구", access: "사용자 및 역할",
    activity: "견적 요청 및 관리 로그", health: "시스템 상태", settings: "사이트 설정", backups: "백업",
    traffic: "트래픽 보호", language: "인터페이스 언어", healthWarning: "시스템 용량을 확인해야 합니다",
    healthWarningDescription: "일반 작업은 계속 사용할 수 있지만 하나 이상의 리소스가 허용 하한에 가까워졌습니다. 영향을 받는 작업과 해결 방법은 시스템 상태에서 확인하세요.",
  },
  "de-DE": {
    subtitle: "Katalogverwaltung", signOut: "Abmelden", requestFailed: "Anfrage fehlgeschlagen", catalog: "Produktkatalog",
    taxonomy: "Kategorien und Wörterbücher", imports: "Importe", productBulk: "Produktstapel", jobs: "Aufträge",
    listingProfiles: "Listenprofile", website: "Website", publicCopy: "Öffentliche Texte", access: "Benutzer und Rollen",
    activity: "Anfragen und Admin-Protokoll", health: "Systemzustand", settings: "Website-Einstellungen", backups: "Sicherungen",
    traffic: "Datenverkehrsschutz", language: "Oberflächensprache", healthWarning: "Systemkapazität erfordert Aufmerksamkeit",
    healthWarningDescription: "Normale Arbeiten sind weiterhin möglich, aber mindestens eine Ressource nähert sich der Zulassungsgrenze. Öffnen Sie den Systemzustand für betroffene Vorgänge und Abhilfen.",
  },
  "fr-FR": {
    subtitle: "Gestion du catalogue", signOut: "Se déconnecter", requestFailed: "Échec de la requête", catalog: "Catalogue produits",
    taxonomy: "Catégories et dictionnaires", imports: "Importations", productBulk: "Opérations groupées", jobs: "Tâches",
    listingProfiles: "Profils de liste", website: "Site web", publicCopy: "Textes publics", access: "Utilisateurs et rôles",
    activity: "Demandes de devis et journal admin", health: "État du système", settings: "Paramètres du site", backups: "Sauvegardes",
    traffic: "Protection du trafic", language: "Langue de l’interface", healthWarning: "La capacité du système nécessite une intervention",
    healthWarningDescription: "Le travail normal reste disponible, mais une ou plusieurs ressources approchent de leur seuil d’admission. Consultez l’état du système pour les opérations concernées et les mesures correctives.",
  },
  "it-IT": {
    subtitle: "Gestione del catalogo", signOut: "Esci", requestFailed: "Richiesta non riuscita", catalog: "Catalogo prodotti",
    taxonomy: "Categorie e dizionari", imports: "Importazioni", productBulk: "Operazioni in blocco", jobs: "Attività",
    listingProfiles: "Profili elenco", website: "Sito web", publicCopy: "Testi pubblici", access: "Utenti e ruoli",
    activity: "Richieste di offerta e registro admin", health: "Stato del sistema", settings: "Impostazioni del sito", backups: "Backup",
    traffic: "Protezione del traffico", language: "Lingua dell’interfaccia", healthWarning: "La capacità del sistema richiede attenzione",
    healthWarningDescription: "Le normali operazioni restano disponibili, ma una o più risorse si stanno avvicinando alla soglia di ammissione. Apri Stato del sistema per le operazioni interessate e le azioni correttive.",
  },
  "es-ES": {
    subtitle: "Gestión del catálogo", signOut: "Cerrar sesión", requestFailed: "Error en la solicitud", catalog: "Catálogo de productos",
    taxonomy: "Categorías y diccionarios", imports: "Importaciones", productBulk: "Operaciones masivas", jobs: "Tareas",
    listingProfiles: "Perfiles de listado", website: "Sitio web", publicCopy: "Textos públicos", access: "Usuarios y roles",
    activity: "Solicitudes de oferta y registro de administración", health: "Estado del sistema", settings: "Configuración del sitio", backups: "Copias de seguridad",
    traffic: "Protección del tráfico", language: "Idioma de la interfaz", healthWarning: "La capacidad del sistema requiere atención",
    healthWarningDescription: "El trabajo normal sigue disponible, pero uno o más recursos se acercan al límite de admisión. Abre Estado del sistema para consultar las operaciones afectadas y las medidas correctivas.",
  },
  "pt-BR": {
    subtitle: "Gestão do catálogo", signOut: "Sair", requestFailed: "Falha na solicitação", catalog: "Catálogo de produtos",
    taxonomy: "Categorias e dicionários", imports: "Importações", productBulk: "Operações em massa", jobs: "Tarefas",
    listingProfiles: "Perfis de listagem", website: "Site", publicCopy: "Textos públicos", access: "Usuários e funções",
    activity: "Solicitações de cotação e registro administrativo", health: "Estado do sistema", settings: "Configurações do site", backups: "Backups",
    traffic: "Proteção de tráfego", language: "Idioma da interface", healthWarning: "A capacidade do sistema requer atenção",
    healthWarningDescription: "O trabalho normal continua disponível, mas um ou mais recursos estão próximos do limite de admissão. Abra Estado do sistema para ver as operações afetadas e as medidas corretivas.",
  },
} as const;

const openNavigationText: Record<AdminLocale, string> = {
  "en-US": "Open navigation menu",
  "zh-TW": "開啟導覽選單",
  "zh-CN": "打开导航菜单",
  "ja-JP": "ナビゲーションメニューを開く",
  "ko-KR": "탐색 메뉴 열기",
  "de-DE": "Navigationsmenü öffnen",
  "fr-FR": "Ouvrir le menu de navigation",
  "it-IT": "Apri il menu di navigazione",
  "es-ES": "Abrir el menú de navegación",
  "pt-BR": "Abrir menu de navegação",
};

const unavailablePageText: Record<AdminLocale, string> = {
  "en-US": "This page is unavailable. Choose an accessible page.",
  "zh-TW": "此頁面不存在或無權存取。請選擇可用頁面。",
  "zh-CN": "此页面不存在或无权访问。请选择可用页面。",
  "ja-JP": "このページは利用できません。アクセス可能なページを選択してください。",
  "ko-KR": "이 페이지를 사용할 수 없습니다. 접근 가능한 페이지를 선택하세요.",
  "de-DE": "Diese Seite ist nicht verfügbar. Wählen Sie eine zugängliche Seite.",
  "fr-FR": "Cette page est indisponible. Choisissez une page accessible.",
  "it-IT": "Questa pagina non è disponibile. Scegli una pagina accessibile.",
  "es-ES": "Esta página no está disponible. Elige una página accesible.",
  "pt-BR": "Esta página não está disponível. Escolha uma página acessível.",
};

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
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const identity = useAdminIdentity();
  const location = useLocation();
  const navigate = useNavigate();
  const requestedTab = location.pathname.replace(/^\//, "") || "catalog";
  const availablePages = Object.keys(pageCapabilities).filter((page) => pageCapabilities[page].every((capability) => identity.capabilities[capability]));
  const activeTab = availablePages.includes(requestedTab) ? requestedTab : "";
  const setActiveTab = (page: string) => { setMessage(undefined); setError(undefined); void navigate(`/${page}`); };
  const [systemHealth, setSystemHealth] = useState<SystemHealth>();
  const text = adminText[locale];
  const showMessage = (next: string) => {
    setError(undefined);
    setMessage(next);
  };
  const showError = (next: unknown) => {
    setError(messageFrom(next));
  };
  const logout = async () => {
    try { await authProvider.logout({}); } catch (error) { showError(error); }
  };

  useEffect(() => {
    document.documentElement.lang = locale;
    window.localStorage.setItem("prods-admin-locale", locale);
    setAPILocale(locale);
  }, [locale]);

	useEffect(() => {
		let cancelled = false;
    if (!identity.capabilities["system.manage"]) return;
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

  const pages = [
    {
      key: "catalog",
      label: text.catalog,
      content: <CatalogPanel locale={locale} onError={showError} onMessage={showMessage} />,
    },
    {
      key: "taxonomy",
      label: text.taxonomy,
      content: <TaxonomyPanel locale={locale} onError={showError} onMessage={showMessage} />,
    },
    {
      key: "imports",
      label: text.imports,
      content: <ImportPanel locale={locale} onError={showError} onMessage={showMessage} />,
    },
    {
      key: "product-bulk",
      label: text.productBulk,
      content: <ProductBulkPanel locale={locale} onError={showError} onMessage={showMessage} />,
    },
    {
      key: "jobs",
      label: text.jobs,
      content: <JobsPanel locale={locale} onError={showError} onMessage={showMessage} />,
    },
    {
      key: "website",
      label: text.website,
      content: <WebsitePanel locale={locale} onError={showError} onMessage={showMessage} />,
    },
    {
      key: "listing-profiles",
      label: text.listingProfiles,
      content: <ListingProfilesPanel locale={locale} onError={showError} onMessage={showMessage} />,
    },
    {
      key: "public-copy",
      label: text.publicCopy,
      content: <PublicCopyPanel locale={locale} onError={showError} onMessage={showMessage} />,
    },
    {
      key: "access",
      label: text.access,
      content: <AccessPanel locale={locale} onError={showError} onMessage={showMessage} />,
    },
    {
      key: "activity",
      label: text.activity,
      content: <ActivityPanel locale={locale} onError={showError} />,
    },
    {
      key: "settings",
      label: text.settings,
      content: <SettingsPanel locale={locale} onError={showError} onMessage={showMessage} />,
    },
    {
      key: "health",
      label: text.health,
      content: <HealthPanel locale={locale} onError={showError} />,
    },
    {
      key: "backups",
      label: text.backups,
      content: <BackupPanel locale={locale} onError={showError} onMessage={showMessage} />,
    },
    {
      key: "traffic",
      label: text.traffic,
      content: <TrafficPanel locale={locale} onError={showError} onMessage={showMessage} />,
    },
  ].filter((page) => availablePages.includes(page.key));
  const activePage = pages.find((page) => page.key === activeTab);

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
        <Layout.Sider
          className="admin-sidebar"
          width={248}
          breakpoint="lg"
          collapsedWidth={0}
          collapsed={sidebarCollapsed}
          onCollapse={setSidebarCollapsed}
          trigger={null}
        >
          <div className="sidebar-brand">
            <Typography.Title level={2} className="brand-title">
              Prods
            </Typography.Title>
            <Typography.Text className="brand-subtitle">{text.subtitle}</Typography.Text>
          </div>
          <Menu
            className="admin-menu"
            theme="dark"
            mode="inline"
            selectedKeys={activeTab ? [activeTab] : []}
            items={pages.map(({ key, label }) => ({ key, label }))}
            onClick={({ key }) => {
              setActiveTab(key);
              if (window.matchMedia("(max-width: 991px)").matches) setSidebarCollapsed(true);
            }}
          />
        </Layout.Sider>
        <Layout className="admin-main">
          <Layout.Header className="admin-header">
            <div className="header-content">
              <Space size="small">
                <Button
                  className="sidebar-toggle"
                  type="text"
                  aria-label={openNavigationText[locale]}
                  onClick={() => setSidebarCollapsed((collapsed) => !collapsed)}
                >
                  ☰
                </Button>
                <Typography.Title level={4} className="section-title">
                  {activePage?.label ?? "Admin"}
                </Typography.Title>
              </Space>
              <Space className="header-actions">
                <Select
                  aria-label={text.language}
                  value={locale}
                  onChange={setLocale}
                  options={[...ADMIN_LOCALE_OPTIONS]}
                />
                <Button onClick={() => void logout()}>{text.signOut}</Button>
              </Space>
            </div>
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
              {!activeTab && <Alert type="warning" title={unavailablePageText[locale]} />}
              {activePage?.content}
            </Space>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}

const root = document.getElementById("admin-root");
if (root) createRoot(root).render(<AdminProviders><AdminApp /></AdminProviders>);
const loginRoot = document.getElementById("admin-login-root");
if (loginRoot) createRoot(loginRoot).render(<LoginPage root={loginRoot} />);
