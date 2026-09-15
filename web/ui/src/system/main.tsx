import { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import { Alert, Button, Card, Checkbox, ConfigProvider, Descriptions, Form, Input, Popconfirm, Progress, Result, Select, Space, Spin, Typography } from "antd";
import "antd/dist/reset.css";
import "./style.css";
import {
  antdLocale,
  extraCommonText,
  extraInstallerText,
  extraMaintenanceText,
  extraRecoveryText,
  systemLocaleCodes,
  systemLocaleOptions,
  type SystemLocale,
} from "./i18n";

type SystemMode = "installer" | "recovery" | "maintenance";
type TextOf<T> = { [K in keyof T]: string };

class APIError extends Error {
  constructor(message: string, readonly status: number, readonly field?: string) {
    super(message);
  }
}

async function requestJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, { credentials: "same-origin", ...init });
  const body = (await response.json().catch(() => ({}))) as { error?: string; field?: string } & T;
  if (!response.ok) throw new APIError(body.error || `${response.status} ${response.statusText}`, response.status, body.field);
  return body;
}

const commonText = {
  "en-US": { language: "Interface language", loading: "Loading system state…", requestFailed: "Request failed", retry: "Retry", setup: "Setup", recovery: "Recovery", maintenance: "System maintenance" },
  "zh-TW": { language: "介面語言", loading: "正在載入系統狀態…", requestFailed: "請求失敗", retry: "重試", setup: "系統安裝", recovery: "系統復原", maintenance: "系統維護" },
} as const;
const localizedCommonText = { ...commonText, ...extraCommonText } as unknown as Record<SystemLocale, TextOf<(typeof commonText)["en-US"]>>;

function initialLocale(): SystemLocale {
  const requested = new URL(window.location.href).searchParams.get("lang");
	const requestedLocale = systemLocaleCodes.find((locale) => locale.toLowerCase() === requested?.toLowerCase());
	if (requestedLocale) return requestedLocale;
	const documentLocale = systemLocaleCodes.find((locale) => locale.toLowerCase() === document.documentElement.lang.toLowerCase());
	return documentLocale ?? "en-US";
}

function localeQuery(locale: SystemLocale): string {
  return `lang=${encodeURIComponent(locale)}`;
}

function SystemFrame({ mode, children }: { mode: SystemMode; children: React.ReactNode }) {
  const [locale, setLocale] = useState<SystemLocale>(initialLocale);
	const copy = localizedCommonText[locale];
  useEffect(() => { document.documentElement.lang = locale; }, [locale]);
  const changeLocale = (next: SystemLocale) => {
    const location = new URL(window.location.href);
    location.searchParams.set("lang", next);
    window.history.replaceState(null, "", location);
    setLocale(next);
    window.dispatchEvent(new CustomEvent("prods-system-locale", { detail: next }));
  };
  return (
    <ConfigProvider locale={antdLocale[locale]} theme={{ token: { colorPrimary: "#147985", borderRadius: 8, colorBgLayout: "#eef3f5" } }}>
      <main className={`system-shell system-${mode}`}>
        <header className="system-header">
          <Space className="system-header-content" align="center">
            <div>
              <Typography.Title level={2}>Prods</Typography.Title>
              <Typography.Text type="secondary">{mode === "installer" ? copy.setup : mode === "recovery" ? copy.recovery : copy.maintenance}</Typography.Text>
            </div>
            <Select<SystemLocale> aria-label={localizedCommonText[locale].language} value={locale} onChange={changeLocale} options={systemLocaleOptions} />
          </Space>
        </header>
        {children}
      </main>
    </ConfigProvider>
  );
}

type InstallerState = {
  stage: "claim" | "claimed" | "setup" | "complete";
  csrf_token?: string;
  default_locale: string;
  supported_locales: string;
  time_zone: string;
};

type InstallerValues = {
  token: string;
  owner_email: string;
  owner_display_name: string;
  password: string;
  password_confirm: string;
  default_locale: SystemLocale;
  supported_locales: SystemLocale[];
  time_zone: string;
};

const installerText = {
  "en-US": {
    title: "Set up Prods", claim: "Claim this installation", claimHelp: "Enter the one-time bootstrap token printed by Prods on this host.", token: "Bootstrap token",
    claimed: "This installer is already claimed in another browser. Return to that browser or restart Prods to issue a new token.",
    setup: "Create the first Owner and minimum site settings. Product data, Website configuration, SMTP, and the public URL can be configured later.",
    email: "Owner email", name: "Owner display name", password: "Password", confirm: "Confirm password",
    passwordHelp: "Use at least 8 characters with at least one letter and one number.", defaultLocale: "Default locale", supportedLocales: "Supported locales",
    localesHelp: "Choose one or more interface languages available in this binary; include the default locale.", timeZone: "Site time zone", finish: "Complete installation",
    complete: "Installation was saved atomically. Restart Prods to enter Normal mode.",
  },
  "zh-TW": {
    title: "設定 Prods", claim: "認領此安裝程序", claimHelp: "輸入 Prods 在這台主機終端機顯示的一次性 bootstrap token。", token: "Bootstrap token",
    claimed: "此安裝程序已由另一個瀏覽器認領。請回到原瀏覽器，或重新啟動 Prods 取得新 token。",
    setup: "建立第一位 Owner 與最低限度站點設定。產品、網站設定、SMTP 與正式公開網址可稍後設定。",
    email: "Owner 電子郵件", name: "Owner 顯示名稱", password: "密碼", confirm: "確認密碼",
    passwordHelp: "至少 8 個字元，並至少包含一個英文字母與一個數字。", defaultLocale: "預設語系", supportedLocales: "支援語系",
    localesHelp: "選擇此 binary 提供的一個或多個介面語系，且必須包含預設語系。", timeZone: "站點時區", finish: "完成安裝",
    complete: "安裝資料已原子保存。請重新啟動 Prods 進入正常模式。",
  },
} as const;
const localizedInstallerText = { ...installerText, ...extraInstallerText } as unknown as Record<SystemLocale, TextOf<(typeof installerText)["en-US"]>>;

function InstallerApp() {
  const [locale, setLocale] = useState<SystemLocale>(initialLocale);
  const [state, setState] = useState<InstallerState>();
  const [error, setError] = useState<string>();
  const [loading, setLoading] = useState(false);
  const [form] = Form.useForm<InstallerValues>();
  const copy = localizedInstallerText[locale];
  const parseSupportedLocales = (value: string): SystemLocale[] => value
		.split(",")
		.map((item) => item.trim())
		.filter((item): item is SystemLocale => systemLocaleCodes.includes(item as SystemLocale));

  const load = async () => {
    setLoading(true);
    setError(undefined);
    try {
      const next = await requestJSON<InstallerState>(`/install/api/state?${localeQuery(locale)}`);
      setState(next);
      form.setFieldsValue({
        token: new URL(window.location.href).searchParams.get("token") ?? form.getFieldValue("token") ?? "",
        default_locale: form.getFieldValue("default_locale") ?? next.default_locale as SystemLocale,
        supported_locales: form.getFieldValue("supported_locales") ?? parseSupportedLocales(next.supported_locales),
        time_zone: form.getFieldValue("time_zone") ?? next.time_zone,
      });
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : String(nextError));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
    const listener = (event: Event) => setLocale((event as CustomEvent<SystemLocale>).detail);
    window.addEventListener("prods-system-locale", listener);
    return () => window.removeEventListener("prods-system-locale", listener);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  useEffect(() => { if (state) void load(); /* eslint-disable-next-line react-hooks/exhaustive-deps */ }, [locale]);

  const submit = async (values: InstallerValues) => {
    setLoading(true);
    setError(undefined);
    const body = new URLSearchParams({ ...values, supported_locales: values.supported_locales.join(","), interface_locale: locale });
    try {
      if (state?.stage === "claim") {
        const next = await requestJSON<InstallerState>("/install/claim", { method: "POST", headers: { Accept: "application/json", "Content-Type": "application/x-www-form-urlencoded" }, body });
        setState(next);
        form.setFieldsValue({ default_locale: next.default_locale as SystemLocale, supported_locales: parseSupportedLocales(next.supported_locales), time_zone: next.time_zone });
        const location = new URL(window.location.href);
        location.searchParams.delete("token");
        window.history.replaceState(null, "", location);
      } else {
        body.set("csrf_token", state?.csrf_token ?? "");
        const next = await requestJSON<InstallerState>("/install/complete", { method: "POST", headers: { Accept: "application/json", "Content-Type": "application/x-www-form-urlencoded" }, body });
        setState(next);
      }
    } catch (nextError) {
      const apiError = nextError instanceof APIError ? nextError : undefined;
      if (apiError?.field) form.setFields([{ name: apiError.field as keyof InstallerValues, errors: [apiError.message] }]);
      setError(nextError instanceof Error ? nextError.message : String(nextError));
    } finally {
      setLoading(false);
    }
  };

  if (!state && loading) return <Spin tip={localizedCommonText[locale].loading} fullscreen />;
  if (!state) return <Alert type="error" showIcon message={localizedCommonText[locale].requestFailed} description={error} action={<Button onClick={() => void load()}>{localizedCommonText[locale].retry}</Button>} />;
  if (state.stage === "complete") return <Result status="success" title={copy.title} subTitle={copy.complete} />;
  if (state.stage === "claimed") return <Result status="warning" title={copy.title} subTitle={copy.claimed} />;
  return (
    <Card title={copy.title}>
      <Space direction="vertical" className="system-stack">
        <Alert type="info" showIcon message={state.stage === "claim" ? copy.claimHelp : copy.setup} />
        {error && <Alert closable type="error" showIcon message={localizedCommonText[locale].requestFailed} description={error} onClose={() => setError(undefined)} />}
        <Form form={form} layout="vertical" onFinish={(values) => void submit(values)} requiredMark="optional">
          {state.stage === "claim" ? (
            <Form.Item name="token" label={copy.token} rules={[{ required: true }]}><Input.Password autoComplete="off" /></Form.Item>
          ) : (
            <>
              <Form.Item name="owner_email" label={copy.email} rules={[{ required: true }, { type: "email" }]}><Input autoComplete="username" /></Form.Item>
              <Form.Item name="owner_display_name" label={copy.name}><Input /></Form.Item>
              <Form.Item name="password" label={copy.password} extra={copy.passwordHelp} rules={[{ required: true }, { min: 8 }, { pattern: /[A-Za-z]/, message: copy.passwordHelp }, { pattern: /[0-9]/, message: copy.passwordHelp }]}><Input.Password autoComplete="new-password" /></Form.Item>
              <Form.Item name="password_confirm" label={copy.confirm} dependencies={["password"]} rules={[{ required: true }, ({ getFieldValue }) => ({ validator(_, value) { return !value || value === getFieldValue("password") ? Promise.resolve() : Promise.reject(new Error(copy.confirm)); } })]}><Input.Password autoComplete="new-password" /></Form.Item>
              <Form.Item name="default_locale" label={copy.defaultLocale} rules={[{ required: true }]}><Select options={systemLocaleOptions} /></Form.Item>
              <Form.Item name="supported_locales" label={copy.supportedLocales} extra={copy.localesHelp} dependencies={["default_locale"]} rules={[{ required: true }, ({ getFieldValue }) => ({ validator(_, value?: SystemLocale[]) { return value?.includes(getFieldValue("default_locale")) ? Promise.resolve() : Promise.reject(new Error(copy.localesHelp)); } })]}><Select mode="multiple" options={systemLocaleOptions} /></Form.Item>
              <Form.Item name="time_zone" label={copy.timeZone} rules={[{ required: true }]}><Input placeholder="UTC" /></Form.Item>
            </>
          )}
          <Button type="primary" htmlType="submit" loading={loading}>{state.stage === "claim" ? copy.claim : copy.finish}</Button>
        </Form>
      </Space>
    </Card>
  );
}

type RecoveryManifest = { id: string; created_utc: string; schema_version: number; content_verified: boolean };
type RecoveryState = { diagnostic: string; csrf_token: string; backups: RecoveryManifest[]; success: boolean; restoring: boolean; restore_disabled: boolean };

const recoveryText = {
  "en-US": {
    title: "Recovery Required", blocked: "The existing site cannot enter Normal mode.", sensitive: "Restore points contain sensitive data.",
    sensitiveHelp: "A restore may bring back RFQ personal data removed after the selected point. Already-sent email cannot be recalled.", failed: "Recovery did not complete",
    disabled: "Restore is disabled while recovery control records are ambiguous.", none: "No readable verified restore point was found.", diagnostic: "Diagnostic",
    points: "Verified restore points", confirm: "Type RESTORE to confirm", understood: "I understand this is a fixed-order restore operation.",
    popTitle: "Restore the selected verified point?", popHelp: "After the durable journal reaches prepared, recovery can only roll forward.", restore: "Restore selected point",
    complete: "Restore completed and verified", restart: "Restart Prods normally. This recovery session is now closed.", schema: "schema",
  },
  "zh-TW": {
    title: "需要系統復原", blocked: "現有站點無法進入正常模式。", sensitive: "還原點包含敏感資料。",
    sensitiveHelp: "還原可能恢復在所選時間點之後已移除的 RFQ 個人資料；已寄出的電子郵件無法收回。", failed: "系統復原未完成",
    disabled: "復原控制記錄不明確，因此目前禁止執行還原。", none: "找不到可讀且已驗證的還原點。", diagnostic: "診斷資訊",
    points: "已驗證還原點", confirm: "輸入 RESTORE 以確認", understood: "我了解這是依固定順序執行的還原作業。",
    popTitle: "要還原所選的已驗證還原點嗎？", popHelp: "durable journal 進入 prepared 後，復原只能單向向前完成。", restore: "還原所選時間點",
    complete: "還原已完成並通過驗證", restart: "請正常重新啟動 Prods；此次 recovery session 已關閉。", schema: "schema",
  },
} as const;
const localizedRecoveryText = { ...recoveryText, ...extraRecoveryText } as unknown as Record<SystemLocale, TextOf<(typeof recoveryText)["en-US"]>>;

function RecoveryApp() {
  const [locale, setLocale] = useState<SystemLocale>(initialLocale);
  const [state, setState] = useState<RecoveryState>();
  const [error, setError] = useState<string>();
  const [loading, setLoading] = useState(false);
  const [backupID, setBackupID] = useState<string>();
  const [confirmation, setConfirmation] = useState("");
  const copy = localizedRecoveryText[locale];
  const load = async () => {
    setLoading(true);
    try {
      const next = await requestJSON<RecoveryState>("/recovery/api/state");
      setState(next);
      setBackupID((current) => current ?? next.backups[0]?.id);
      setError(undefined);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : String(nextError));
    } finally {
      setLoading(false);
    }
  };
  useEffect(() => {
    void load();
    const listener = (event: Event) => setLocale((event as CustomEvent<SystemLocale>).detail);
    window.addEventListener("prods-system-locale", listener);
    return () => window.removeEventListener("prods-system-locale", listener);
  }, []);
  const restore = async () => {
    if (!state || !backupID || confirmation !== "RESTORE") return;
    setLoading(true);
    setError(undefined);
    try {
      await requestJSON<{ success: boolean }>("/recovery/api/restore", { method: "POST", headers: { "Content-Type": "application/json", "X-CSRF-Token": state.csrf_token }, body: JSON.stringify({ backup_id: backupID, confirmation }) });
      setState({ ...state, success: true, restoring: false });
    } catch (nextError) {
      const message = nextError instanceof Error ? nextError.message : String(nextError);
      setState((current) => current ? { ...current, restoring: false } : current);
      setError(message);
    } finally {
      setLoading(false);
    }
  };
  if (!state && loading) return <Spin tip={localizedCommonText[locale].loading} fullscreen />;
  if (!state) return <Alert type="error" showIcon message={localizedCommonText[locale].requestFailed} description={error} action={<Button onClick={() => void load()}>{localizedCommonText[locale].retry}</Button>} />;
  if (state.success) return <Result status="success" title={copy.complete} subTitle={copy.restart} />;
  return (
    <Card title={copy.title}>
      <Space direction="vertical" className="system-stack">
        <Alert type="error" showIcon message={copy.blocked} description={state.diagnostic} />
        <Alert type="warning" showIcon message={copy.sensitive} description={copy.sensitiveHelp} />
        {error && <Alert closable type="error" showIcon message={copy.failed} description={error} onClose={() => setError(undefined)} />}
        {loading && <Progress percent={50} status="active" showInfo={false} />}
        {state.restore_disabled ? <Alert type="error" showIcon message={copy.disabled} /> : state.backups.length === 0 ? (
          <Alert type="error" showIcon message={copy.none} />
        ) : (
          <>
            <Descriptions bordered size="small" items={[{ key: "diagnostic", label: copy.diagnostic, children: state.diagnostic }, { key: "points", label: copy.points, children: state.backups.length }]} />
            <Select className="system-field" value={backupID} onChange={setBackupID} options={state.backups.map((backup) => ({ value: backup.id, label: `${backup.id} — ${backup.created_utc} — ${copy.schema} ${backup.schema_version}` }))} />
            <Input value={confirmation} onChange={(event) => setConfirmation(event.target.value)} placeholder={copy.confirm} status={confirmation && confirmation !== "RESTORE" ? "error" : undefined} />
            <Checkbox checked={confirmation === "RESTORE"} disabled>{copy.understood}</Checkbox>
            <Popconfirm title={copy.popTitle} description={copy.popHelp} onConfirm={() => void restore()} okText={copy.restore} okButtonProps={{ danger: true }}>
              <Button danger type="primary" disabled={confirmation !== "RESTORE" || !backupID} loading={loading}>{copy.restore}</Button>
            </Popconfirm>
          </>
        )}
      </Space>
    </Card>
  );
}

type MaintenanceState = { active: boolean; message: string; retry_after_seconds: number };

const maintenanceText = {
  "en-US": { unavailable: "Temporarily unavailable", fallback: "The public catalog is temporarily unavailable.", retry: "Retry now", restored: "Service restored", restoredHelp: "The public catalog is available again.", catalog: "Return to catalog" },
  "zh-TW": { unavailable: "服務暫時無法使用", fallback: "公開型錄目前暫時無法使用。", retry: "立即重試", restored: "服務已恢復", restoredHelp: "公開型錄已恢復使用。", catalog: "返回型錄" },
} as const;
const localizedMaintenanceText = { ...maintenanceText, ...extraMaintenanceText } as unknown as Record<SystemLocale, TextOf<(typeof maintenanceText)["en-US"]>>;

function MaintenanceApp() {
  const [locale, setLocale] = useState<SystemLocale>(initialLocale);
  const [state, setState] = useState<MaintenanceState>();
  const [error, setError] = useState<string>();
  const copy = localizedMaintenanceText[locale];
  const load = async () => {
    try {
      setState(await requestJSON<MaintenanceState>("/maintenance/api/state"));
      setError(undefined);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : String(nextError));
    }
  };
  useEffect(() => {
    void load();
    const listener = (event: Event) => setLocale((event as CustomEvent<SystemLocale>).detail);
    window.addEventListener("prods-system-locale", listener);
    return () => window.removeEventListener("prods-system-locale", listener);
  }, []);
  return (
    <Card>
      {error ? <Alert type="error" showIcon message={localizedCommonText[locale].requestFailed} description={error} action={<Button onClick={() => void load()}>{localizedCommonText[locale].retry}</Button>} /> : !state ? <Spin /> : state.active ? (
        <Result status="warning" title={copy.unavailable} subTitle={state.message || copy.fallback} extra={<Button type="primary" onClick={() => window.location.reload()}>{copy.retry}</Button>} />
      ) : (
        <Result status="success" title={copy.restored} subTitle={copy.restoredHelp} extra={<Button type="primary" onClick={() => window.location.assign("/")}>{copy.catalog}</Button>} />
      )}
    </Card>
  );
}

const root = document.getElementById("system-root");
if (!root) throw new Error("system root missing");
const mode = root.dataset.mode as SystemMode;
const application = mode === "installer" ? <InstallerApp /> : mode === "recovery" ? <RecoveryApp /> : <MaintenanceApp />;
createRoot(root).render(<SystemFrame mode={mode}>{application}</SystemFrame>);
