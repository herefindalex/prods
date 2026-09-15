import { useCallback, useEffect, useRef, useState } from "react";
import { Alert, Button, Card, Form, InputNumber, Space, Typography } from "antd";
import { api, putJSON } from "./api";
import type { TrafficSettings } from "./types";

type Props = {
  locale: "en-US" | "zh-TW";
  onError(error: unknown): void;
  onMessage(message: string): void;
};

const labels = {
  "en-US": {
    title: "Public traffic protection",
    explanation: "The limit applies only to new RFQ submissions from the same direct connection source. Public catalog reads are not limited. A retry of an already completed submission key still replays its durable receipt.",
    proxy: "Forwarded client headers are not trusted by this setting; until a trusted proxy is explicitly configured at the host boundary, requests behind a proxy share that proxy's direct source limit.",
    proxyConfigured: "Forwarded client addresses are accepted only when the transport peer matches a host-configured trusted proxy. Multi-hop chains are evaluated from the trusted edge inward.",
    limit: "New RFQs per window",
    window: "Window (seconds)",
    save: "Save protection settings",
    saved: "Traffic protection settings saved.",
  },
  "zh-TW": {
    title: "公開流量保護",
    explanation: "限制只套用於同一直接連線來源的新 RFQ 提交，不限制公開型錄讀取。已完成 submission key 的重試仍會重播 durable receipt。",
    proxy: "此設定不信任轉送的客戶端標頭；主機邊界尚未明確設定 trusted proxy 前，代理後方的請求會共用該代理的直接來源額度。",
    proxyConfigured: "只有傳輸端符合主機設定的 trusted proxy 時才接受轉送客戶端位址；多跳鏈會從可信任邊界向內判定。",
    limit: "每個窗口的新 RFQ 數",
    window: "窗口秒數",
    save: "儲存保護設定",
    saved: "流量保護設定已儲存。",
  },
} as const;

export function TrafficPanel({ locale, onError, onMessage }: Props) {
  const text = labels[locale];
  const [form] = Form.useForm<TrafficSettings>();
  const [settings, setSettings] = useState<TrafficSettings>();
  const [loading, setLoading] = useState(false);
  const onErrorRef = useRef(onError);
  onErrorRef.current = onError;

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const next = await api<TrafficSettings>("/admin/api/traffic-settings");
      setSettings(next);
      form.setFieldsValue(next);
    } catch (error) {
      onErrorRef.current(error);
    } finally {
      setLoading(false);
    }
  }, [form]);

  useEffect(() => {
    void load();
  }, [load]);

  const save = async (values: TrafficSettings) => {
    if (!settings) return;
    setLoading(true);
    try {
      const next = await putJSON<TrafficSettings>("/admin/api/traffic-settings", {
        expected_version: settings.version,
        settings: values,
      });
      setSettings(next);
      form.setFieldsValue(next);
      onMessage(text.saved);
    } catch (error) {
      onError(error);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Card title={text.title} loading={!settings && loading}>
      <Space direction="vertical" size="middle" className="panel-stack">
        <Typography.Paragraph>{text.explanation}</Typography.Paragraph>
        <Alert type="info" showIcon message={settings?.trusted_proxy_configured ? text.proxyConfigured : text.proxy} />
        <Form form={form} layout="vertical" onFinish={(values) => void save(values)}>
          <Space wrap align="start">
            <Form.Item name="rfq_limit" label={text.limit} rules={[{ required: true }]}>
              <InputNumber min={1} max={10000} precision={0} />
            </Form.Item>
            <Form.Item name="rfq_window_seconds" label={text.window} rules={[{ required: true }]}>
              <InputNumber min={1} max={86400} precision={0} />
            </Form.Item>
          </Space>
          <div><Button type="primary" htmlType="submit" loading={loading}>{text.save}</Button></div>
        </Form>
      </Space>
    </Card>
  );
}
