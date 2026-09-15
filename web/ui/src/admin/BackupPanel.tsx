import { useCallback, useEffect, useRef, useState } from "react";
import { Alert, Button, Card, Form, Input, InputNumber, Space, Switch, Table, Tag, Typography } from "antd";
import { api, postJSON, putJSON } from "./api";
import type { BackupRun, BackupSettings, BackupStatus } from "./types";

type Props = {
  locale: "en-US" | "zh-TW";
  onError(error: unknown): void;
  onMessage(message: string): void;
};

const labels = {
  "en-US": {
    title: "Backups",
    schedule: "Built-in daily schedule",
    enabled: "Enabled",
    localTime: "Local time",
    timeZone: "Schedule time zone",
    daily: "Daily copies",
    weekly: "Weekly copies",
    monthly: "Monthly copies",
    preUpgrade: "Pre-upgrade copies",
    preRestore: "Pre-restore copies",
    save: "Save schedule",
    run: "Run backup now",
    refresh: "Refresh",
    next: "Next scheduled run",
    overdue: "The current scheduled backup is due and waiting to run.",
    sensitive: "Backup restore points contain the database and may contain RFQ personal data, credentials, and local secrets. Store and copy them as sensitive data.",
    external: "Recorded but not included; restore requires these external runtime settings:",
    history: "Recent runs",
    kind: "Kind",
    status: "Status",
    started: "Started",
    completed: "Completed",
    size: "Size",
    evidence: "Completion evidence",
    retention: "Retention reasons",
    verified: "Content verified",
    protected: "Read-only applied",
    saved: "Backup settings saved.",
    completedMessage: "Backup completed.",
  },
  "zh-TW": {
    title: "備份",
    schedule: "內建每日排程",
    enabled: "啟用",
    localTime: "當地時間",
    timeZone: "排程時區",
    daily: "每日保留",
    weekly: "每週保留",
    monthly: "每月保留",
    preUpgrade: "升級前保留",
    preRestore: "還原前保留",
    save: "儲存排程",
    run: "立即執行備份",
    refresh: "重新整理",
    next: "下次排程",
    overdue: "目前排程備份已到期，正在等待執行。",
    sensitive: "備份還原點包含資料庫，且可能包含 RFQ 個資、登入憑證與本機 Secrets；保存或複製時必須視為敏感資料。",
    external: "以下外部 runtime 設定只會記錄、不會包含於備份；還原後仍須提供：",
    history: "最近執行紀錄",
    kind: "種類",
    status: "狀態",
    started: "開始時間",
    completed: "完成時間",
    size: "大小",
    evidence: "完成證據",
    retention: "保留理由",
    verified: "內容已驗證",
    protected: "已套用唯讀保護",
    saved: "備份設定已儲存。",
    completedMessage: "備份已完成。",
  },
} as const;

function formatBytes(value?: number): string {
  if (!value) return "—";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let amount = value;
  let index = 0;
  while (amount >= 1024 && index < units.length - 1) {
    amount /= 1024;
    index += 1;
  }
  return `${amount.toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

export function BackupPanel({ locale, onError, onMessage }: Props) {
  const text = labels[locale];
  const [form] = Form.useForm<BackupSettings>();
  const [status, setStatus] = useState<BackupStatus>();
  const [loading, setLoading] = useState(false);
  const [running, setRunning] = useState(false);
  const onErrorRef = useRef(onError);
  const onMessageRef = useRef(onMessage);
  onErrorRef.current = onError;
  onMessageRef.current = onMessage;

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const next = await api<BackupStatus>("/admin/api/backups");
      setStatus(next);
      form.setFieldsValue(next.settings);
    } catch (error) {
      onErrorRef.current(error);
    } finally {
      setLoading(false);
    }
  }, [form]);

  useEffect(() => {
    void load();
  }, [load]);

  const save = async (values: BackupSettings) => {
    if (!status) return;
    setLoading(true);
    try {
      await putJSON<BackupSettings>("/admin/api/backups/settings", {
        expected_version: status.settings.version,
        settings: values,
      });
      onMessageRef.current(text.saved);
      await load();
    } catch (error) {
      onErrorRef.current(error);
    } finally {
      setLoading(false);
    }
  };

  const runNow = async () => {
    setRunning(true);
    try {
      await postJSON<BackupRun>("/admin/api/backups/run", {});
      onMessageRef.current(text.completedMessage);
      await load();
    } catch (error) {
      onErrorRef.current(error);
    } finally {
      setRunning(false);
    }
  };

  const columns = [
    { title: text.kind, dataIndex: "kind", key: "kind" },
    {
      title: text.status,
      dataIndex: "status",
      key: "status",
      render: (value: BackupRun["status"]) => (
        <Tag color={value === "succeeded" ? "green" : value === "failed" ? "red" : "blue"}>{value}</Tag>
      ),
    },
    {
      title: text.started,
      dataIndex: "started_at",
      key: "started",
      render: (value: string) => new Date(value).toLocaleString(locale),
    },
    {
      title: text.completed,
      dataIndex: "completed_at",
      key: "completed",
      render: (value?: string) => (value ? new Date(value).toLocaleString(locale) : "—"),
    },
    { title: text.size, dataIndex: "size_bytes", key: "size", render: formatBytes },
    {
      title: text.evidence,
      key: "evidence",
      render: (_: unknown, run: BackupRun) => (
        <Space direction="vertical" size={0}>
          <Typography.Text type={run.content_verified ? "success" : undefined}>
            {run.content_verified ? text.verified : "—"}
          </Typography.Text>
          <Typography.Text type={run.read_only_applied ? "success" : "secondary"}>
            {run.read_only_applied ? text.protected : "—"}
          </Typography.Text>
          {run.error_message && <Typography.Text type="danger">{run.error_message}</Typography.Text>}
          {run.warning_message && <Typography.Text type="warning">{run.warning_message}</Typography.Text>}
        </Space>
      ),
    },
    {
      title: text.retention,
      key: "retention",
      render: (_: unknown, run: BackupRun) =>
        run.backup_id ? (status?.retention.reasons[run.backup_id] ?? []).join(", ") || "—" : "—",
    },
  ];

  return (
    <Space direction="vertical" size="middle" className="panel-stack">
      {status?.due && <Alert type="warning" showIcon message={text.overdue} />}
		{status?.contains_sensitive_data && <Alert type="warning" showIcon message={text.sensitive} />}
		{Boolean(status?.external_requirements?.length) && (
			<Alert type="info" showIcon message={text.external} description={status?.external_requirements?.join(", ")} />
		)}
      <Card
        title={text.title}
        extra={
          <Space>
            <Button onClick={() => void load()} loading={loading}>{text.refresh}</Button>
            <Button type="primary" onClick={() => void runNow()} loading={running}>{text.run}</Button>
          </Space>
        }
      >
        {status?.next_run_utc && (
          <Typography.Paragraph>
            {text.next}: {new Date(status.next_run_utc).toLocaleString(locale)}
          </Typography.Paragraph>
        )}
        <Form form={form} layout="vertical" onFinish={(values) => void save(values)}>
          <Card type="inner" title={text.schedule}>
            <Space wrap align="start">
              <Form.Item name="enabled" label={text.enabled} valuePropName="checked">
                <Switch />
              </Form.Item>
              <Form.Item name="local_time" label={text.localTime} rules={[{ required: true }]}>
                <Input type="time" />
              </Form.Item>
              <Form.Item label={text.timeZone}>
                <Input value={status?.settings.time_zone} readOnly />
              </Form.Item>
              {([
                ["retention_daily", text.daily],
                ["retention_weekly", text.weekly],
                ["retention_monthly", text.monthly],
                ["retention_pre_upgrade", text.preUpgrade],
                ["retention_pre_restore", text.preRestore],
              ] as const).map(([name, label]) => (
                <Form.Item key={name} name={name} label={label} rules={[{ required: true }]}>
                  <InputNumber min={1} max={3660} precision={0} />
                </Form.Item>
              ))}
            </Space>
            <div><Button htmlType="submit" loading={loading}>{text.save}</Button></div>
          </Card>
        </Form>
      </Card>
      <Card title={text.history}>
        <Table<BackupRun>
          rowKey="id"
          loading={loading}
          dataSource={status?.runs ?? []}
          columns={columns}
          pagination={false}
          scroll={{ x: true }}
        />
      </Card>
    </Space>
  );
}
