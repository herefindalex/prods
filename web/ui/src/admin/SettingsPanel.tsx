import { useCallback, useEffect, useRef, useState } from "react";
import {
  Alert,
  Button,
  Card,
  Form,
  Input,
  Select,
  Space,
  Switch,
  Typography,
} from "antd";
import { api, putJSON } from "./api";
import type { SiteSettings } from "./types";

type Props = {
  locale: "en-US" | "zh-TW";
  onError(error: unknown): void;
  onMessage(message: string): void;
};
const localeOptions = ["en-US", "zh-TW"].map((value) => ({
  value,
  label: value,
}));
const labels = {
  "en-US": {
    title: "Site settings",
    refresh: "Refresh",
    revision: "Revision",
    defaultLocale: "Default interface locale",
    supportedLocales: "Supported locales",
    timeZone: "Site time zone",
    timeZoneHelp:
      "Use an IANA time zone such as America/New_York, Asia/Taipei, or UTC.",
    invalidTimeZone: "Enter a valid IANA time zone.",
    save: "Save site time zone",
    saved:
      "Site time zone saved. Stored event timestamps remain unchanged in UTC.",
    explanation:
      "The site time zone controls Admin/RFQ display and backup schedules. It is independent from interface language.",
    contentTitle: "Multilingual content",
    contentEnabled: "Enable multilingual content editing",
    contentHelp:
      "Language tabs appear only while enabled. Removing a locale preserves saved translations but revokes their public representations.",
    saveContent: "Save content languages",
    contentSaved: "Content language settings saved.",
  },
  "zh-TW": {
    title: "站點設定",
    refresh: "重新整理",
    revision: "修訂",
    defaultLocale: "預設介面語系",
    supportedLocales: "支援語系",
    timeZone: "站點時區",
    timeZoneHelp:
      "請使用 IANA 時區，例如 America/New_York、Asia/Taipei 或 UTC。",
    invalidTimeZone: "請輸入有效的 IANA 時區。",
    save: "儲存站點時區",
    saved: "站點時區已儲存；既有事件時間仍以 UTC 原值保存。",
    explanation: "站點時區用於 Admin／RFQ 顯示及備份排程；它與介面語言分離。",
    contentTitle: "多語內容",
    contentEnabled: "啟用多語內容編輯",
    contentHelp:
      "只有啟用後才顯示語言頁籤。移除語系會保留既有翻譯，但撤銷該語系公開表示。",
    saveContent: "儲存內容語系",
    contentSaved: "內容語系設定已儲存。",
  },
} as const;

function validTimeZone(value: string) {
  if (value === "Local") return true;
  try {
    new Intl.DateTimeFormat("en-US", { timeZone: value }).format();
    return true;
  } catch {
    return false;
  }
}

export function SettingsPanel({ locale, onError, onMessage }: Props) {
  const text = labels[locale];
  const [settings, setSettings] = useState<SiteSettings>();
  const [loading, setLoading] = useState(false);
  const [timeForm] = Form.useForm<{ time_zone: string }>();
  const [contentForm] = Form.useForm<{
    enabled: boolean;
    supported_locales: string[];
  }>();
  const onErrorRef = useRef(onError);
  const onMessageRef = useRef(onMessage);
  onErrorRef.current = onError;
  onMessageRef.current = onMessage;
  const load = useCallback(async () => {
    setLoading(true);
    try {
      const next = await api<SiteSettings>("/admin/api/system/settings");
      setSettings(next);
      timeForm.setFieldsValue({ time_zone: next.time_zone });
      contentForm.setFieldsValue({
        enabled: next.content_multilingual_enabled,
        supported_locales: next.supported_locales,
      });
    } catch (error) {
      onErrorRef.current(error);
    } finally {
      setLoading(false);
    }
  }, [contentForm, timeForm]);
  useEffect(() => {
    void load();
  }, [load]);
  const saveTime = async (values: { time_zone: string }) => {
    if (!settings) return;
    setLoading(true);
    try {
      const updated = await putJSON<SiteSettings>(
        "/admin/api/system/settings",
        {
          expected_revision: settings.revision,
          time_zone: values.time_zone.trim(),
        },
      );
      setSettings(updated);
      onMessageRef.current(text.saved);
    } catch (error) {
      onErrorRef.current(error);
    } finally {
      setLoading(false);
    }
  };
  const saveContent = async (values: {
    enabled: boolean;
    supported_locales: string[];
  }) => {
    if (!settings) return;
    setLoading(true);
    try {
      const updated = await putJSON<SiteSettings>(
        "/admin/api/system/settings/content-localization",
        {
          expected_revision: settings.revision,
          enabled: values.enabled,
          supported_locales: values.supported_locales,
        },
      );
      setSettings(updated);
      contentForm.setFieldsValue({
        enabled: updated.content_multilingual_enabled,
        supported_locales: updated.supported_locales,
      });
      onMessageRef.current(text.contentSaved);
    } catch (error) {
      onErrorRef.current(error);
    } finally {
      setLoading(false);
    }
  };
  return (
    <Space direction="vertical" size="large" className="panel-stack">
      <Card
        title={text.title}
        extra={
          <Button onClick={() => void load()} loading={loading}>
            {text.refresh}
          </Button>
        }
      >
        <Space direction="vertical" size="large" className="panel-stack">
          <Alert type="info" showIcon message={text.explanation} />
          <Form
            form={timeForm}
            layout="vertical"
            onFinish={(v) => void saveTime(v)}
          >
            <div className="form-grid three-columns">
              <Form.Item label={text.defaultLocale}>
                <Input value={settings?.default_locale} readOnly />
              </Form.Item>
              <Form.Item label={text.supportedLocales}>
                <Input
                  value={settings?.supported_locales.join(", ")}
                  readOnly
                />
              </Form.Item>
              <Form.Item
                name="time_zone"
                label={text.timeZone}
                extra={text.timeZoneHelp}
                rules={[
                  { required: true },
                  {
                    validator: async (_, v: string) => {
                      if (v && !validTimeZone(v.trim()))
                        throw new Error(text.invalidTimeZone);
                    },
                  },
                ]}
              >
                <Input placeholder="UTC" />
              </Form.Item>
            </div>
            <Button type="primary" htmlType="submit" loading={loading}>
              {text.save}
            </Button>
          </Form>
          {settings && (
            <Typography.Text type="secondary">
              {text.revision} {settings.revision} ·{" "}
              {new Date(settings.updated_at).toLocaleString(locale)}
            </Typography.Text>
          )}
        </Space>
      </Card>
      <Card title={text.contentTitle}>
        <Form
          form={contentForm}
          layout="vertical"
          onFinish={(v) => void saveContent(v)}
        >
          <Alert
            className="bottom-gap"
            type="info"
            showIcon
            message={text.contentHelp}
          />
          <Form.Item
            name="enabled"
            label={text.contentEnabled}
            valuePropName="checked"
          >
            <Switch />
          </Form.Item>
          <Form.Item
            name="supported_locales"
            label={text.supportedLocales}
            rules={[
              { required: true },
              {
                validator: async (_, v: string[]) => {
                  if (!v?.includes(settings?.default_locale ?? ""))
                    throw new Error(
                      `${text.defaultLocale}: ${settings?.default_locale}`,
                    );
                },
              },
            ]}
          >
            <Select mode="multiple" options={localeOptions} />
          </Form.Item>
          <Button type="primary" htmlType="submit" loading={loading}>
            {text.saveContent}
          </Button>
        </Form>
      </Card>
    </Space>
  );
}
