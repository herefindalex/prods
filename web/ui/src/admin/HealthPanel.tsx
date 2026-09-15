import { useCallback, useEffect, useRef, useState } from "react";
import { Alert, Button, Card, Input, Popconfirm, Select, Space, Table, Tag, Typography, message } from "antd";
import { api, putJSON } from "./api";
import type { RuntimeLog, SiteMaintenance, SystemComponentHealth, SystemHealth, SystemResourceHealth } from "./types";

type Props = {
  locale: "en-US" | "zh-TW";
  onError(error: unknown): void;
};

const labels = {
  "en-US": {
    title: "System health",
    refresh: "Refresh",
    healthy: "Database, publication, jobs, backup, search, integrations, and storage checks are healthy.",
    degraded: "One or more capabilities need attention. Unaffected safe operations remain available.",
    components: "Capabilities",
    component: "Capability",
    summary: "Summary",
    details: "Evidence",
    resources: "Storage resources",
    resource: "Resource",
    status: "Status",
    capacity: "Capacity",
    operations: "Affected operations",
    checked: "Checked",
    unavailable: "Unavailable",
    configured: "configured",
    notConfigured: "optional / not configured",
    lastSuccess: "last success",
    lastRun: "last run",
    nextRun: "next run",
    due: "due now",
    runtimeRunning: "runtime running",
    runtimeStopped: "runtime stopped",
    lastRuntimeAttempt: "last runtime attempt",
    lastRuntimeSuccess: "last runtime success",
    lastRuntimeFailure: "last runtime failure",
    runtimeStage: "failure stage",
    consecutiveFailures: "consecutive runtime failures",
		maintenanceTitle: "Manual site maintenance",
		maintenanceInactive: "Public catalog and RFQ submission are available.",
		maintenanceActive: "Public requests and new RFQs are paused with HTTP 503. Admin and recovery entry points remain available.",
		maintenanceMessage: "Public maintenance message",
		enableMaintenance: "Start maintenance",
		disableMaintenance: "End maintenance",
		confirmMaintenance: "Start maintenance after all already-admitted public writes drain?",
		runtimeLog: "Runtime log",
		logGeneration: "Log file",
		currentLog: "Current",
		previousLog: "Previous",
		logUnavailable: "This log generation is not available.",
		logTruncated: "Only the bounded tail is shown.",
  },
  "zh-TW": {
    title: "系統健康狀態",
    refresh: "重新檢查",
    healthy: "資料庫、公開生成、背景工作、備份、搜尋、整合與儲存檢查均正常。",
    degraded: "一項或多項能力需要處理；未受影響且安全的操作仍保持可用。",
    components: "系統能力",
    component: "能力",
    summary: "摘要",
    details: "證據",
    resources: "儲存資源",
    resource: "資源",
    status: "狀態",
    capacity: "容量",
    operations: "受影響操作",
    checked: "檢查時間",
    unavailable: "無法取得",
    configured: "已設定",
    notConfigured: "選填／未設定",
    lastSuccess: "最後成功",
    lastRun: "最後執行",
    nextRun: "下次執行",
    due: "目前已到期",
    runtimeRunning: "執行中",
    runtimeStopped: "已停止",
    lastRuntimeAttempt: "最近背景嘗試",
    lastRuntimeSuccess: "最近背景成功",
    lastRuntimeFailure: "最近背景失敗",
    runtimeStage: "失敗階段",
    consecutiveFailures: "連續背景失敗",
		maintenanceTitle: "手動站點維護",
		maintenanceInactive: "公開型錄與 RFQ 提交目前可用。",
		maintenanceActive: "公開請求與新 RFQ 已暫停並回應 HTTP 503；Admin 與恢復入口仍可使用。",
		maintenanceMessage: "公開維護訊息",
		enableMaintenance: "開始維護",
		disableMaintenance: "結束維護",
		confirmMaintenance: "排空已接受的公開寫入後開始維護？",
		runtimeLog: "執行期日誌",
		logGeneration: "日誌檔案",
		currentLog: "目前",
		previousLog: "較舊",
		logUnavailable: "此代日誌目前不存在。",
		logTruncated: "僅顯示受限大小的尾端內容。",
  },
} as const;

const componentNames = {
  "en-US": {
    database: "Database",
		maintenance: "Maintenance",
    public_generation: "Public generation",
    background_jobs: "Background jobs",
    backup: "Backup",
    asset_gc: "Asset cleanup",
    search: "Search",
    smtp: "SMTP",
  },
  "zh-TW": {
    database: "資料庫",
		maintenance: "維護模式",
    public_generation: "公開生成",
    background_jobs: "背景工作",
    backup: "備份",
    asset_gc: "資產清理",
    search: "搜尋",
    smtp: "SMTP",
  },
} as const;

function bytes(value?: number): string {
  if (value === undefined) return "—";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let amount = value;
  let unit = 0;
  while (amount >= 1024 && unit < units.length - 1) {
    amount /= 1024;
    unit += 1;
  }
  return `${amount.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
}

function statusTag(status: SystemComponentHealth["status"]) {
  return <Tag color={status === "Critical" ? "red" : status === "Warning" ? "gold" : "green"}>{status}</Tag>;
}

export function HealthPanel({ locale, onError }: Props) {
  const text = labels[locale];
  const [health, setHealth] = useState<SystemHealth>();
	const [maintenance, setMaintenance] = useState<SiteMaintenance>();
	const [maintenanceMessage, setMaintenanceMessage] = useState("");
	const [runtimeLog, setRuntimeLog] = useState<RuntimeLog>();
	const [runtimeGeneration, setRuntimeGeneration] = useState(0);
  const [loading, setLoading] = useState(false);
	const [updatingMaintenance, setUpdatingMaintenance] = useState(false);
  const onErrorRef = useRef(onError);
  onErrorRef.current = onError;

  const load = useCallback(async () => {
    setLoading(true);
    try {
		const [nextHealth, nextMaintenance, nextRuntimeLog] = await Promise.all([
			api<SystemHealth>("/admin/api/system/health"),
			api<SiteMaintenance>("/admin/api/system/maintenance"),
			api<RuntimeLog>(`/admin/api/system/runtime-log?generation=${runtimeGeneration}`),
		]);
		setHealth(nextHealth);
		setMaintenance(nextMaintenance);
		setMaintenanceMessage(nextMaintenance.message);
		setRuntimeLog(nextRuntimeLog);
    } catch (error) {
      onErrorRef.current(error);
    } finally {
      setLoading(false);
    }
	}, [runtimeGeneration]);

	const updateMaintenance = async (active: boolean) => {
		if (!maintenance) return;
		setUpdatingMaintenance(true);
		try {
			const updated = await putJSON<SiteMaintenance>("/admin/api/system/maintenance", {
				expected_revision: maintenance.revision,
				active,
				message: maintenanceMessage,
			});
			setMaintenance(updated);
			setMaintenanceMessage(updated.message);
			message.success(active ? text.enableMaintenance : text.disableMaintenance);
			void load();
		} catch (error) {
			onErrorRef.current(error);
		} finally {
			setUpdatingMaintenance(false);
		}
	};

  useEffect(() => {
    void load();
  }, [load]);

  const componentColumns = [
    {
      title: text.component,
      dataIndex: "component",
      key: "component",
      render: (component: SystemComponentHealth["component"]) => componentNames[locale][component],
    },
    {
      title: text.status,
      dataIndex: "status",
      key: "status",
      render: statusTag,
    },
    { title: text.summary, dataIndex: "summary", key: "summary" },
    {
      title: text.details,
      key: "details",
      render: (_: unknown, item: SystemComponentHealth) => {
        const evidence: string[] = [];
        if (item.configured !== undefined) evidence.push(item.configured ? text.configured : text.notConfigured);
        if (item.last_success_utc) evidence.push(`${text.lastSuccess}: ${new Date(item.last_success_utc).toLocaleString(locale)}`);
        if (item.last_run_status) evidence.push(`${text.lastRun}: ${item.last_run_status}`);
        if (item.next_run_utc) evidence.push(`${text.nextRun}: ${new Date(item.next_run_utc).toLocaleString(locale)}`);
        if (item.due) evidence.push(text.due);
        if (item.runtime_running !== undefined) evidence.push(item.runtime_running ? text.runtimeRunning : text.runtimeStopped);
        if (item.last_runtime_attempt_utc) evidence.push(`${text.lastRuntimeAttempt}: ${new Date(item.last_runtime_attempt_utc).toLocaleString(locale)}`);
        if (item.last_runtime_success_utc) evidence.push(`${text.lastRuntimeSuccess}: ${new Date(item.last_runtime_success_utc).toLocaleString(locale)}`);
        if (item.last_runtime_failure_utc) evidence.push(`${text.lastRuntimeFailure}: ${new Date(item.last_runtime_failure_utc).toLocaleString(locale)}`);
        if (item.runtime_failure_stage) evidence.push(`${text.runtimeStage}: ${item.runtime_failure_stage}`);
        if (item.consecutive_runtime_failures) evidence.push(`${text.consecutiveFailures}: ${item.consecutive_runtime_failures}`);
        for (const [name, value] of Object.entries(item.counters ?? {})) evidence.push(`${name}: ${value}`);
        return evidence.length > 0 ? (
          <Space direction="vertical" size={0}>
            {evidence.map((value) => <span key={value}>{value}</span>)}
          </Space>
        ) : "—";
      },
    },
  ];

  const resourceColumns = [
    { title: text.resource, dataIndex: "resource", key: "resource" },
    {
      title: text.status,
      dataIndex: "status",
      key: "status",
      render: (status: SystemResourceHealth["status"]) => statusTag(status),
    },
    {
      title: text.capacity,
      key: "capacity",
      render: (_: unknown, item: SystemResourceHealth) => item.reason ? (
        <Space direction="vertical" size={0}>
          <Typography.Text type="danger">{text.unavailable}</Typography.Text>
          <Typography.Text type="secondary">{item.reason}</Typography.Text>
        </Space>
      ) : `${bytes(item.free_bytes)} free / ${bytes(item.total_bytes)} total (${bytes(item.reserved_bytes)} reserved)`,
    },
    {
      title: text.operations,
      dataIndex: "affected_operations",
      key: "operations",
      render: (items: string[]) => items.join(", "),
    },
    {
      title: text.checked,
      dataIndex: "checked_utc",
      key: "checked",
      render: (value: string) => new Date(value).toLocaleString(locale),
    },
  ];

  return (
    <Card title={text.title} extra={<Button onClick={() => void load()} loading={loading}>{text.refresh}</Button>}>
      {health && (
        <Alert
          showIcon
          type={health.status === "Critical" ? "error" : health.status === "Warning" ? "warning" : "success"}
          message={health.status === "Normal" ? text.healthy : text.degraded}
          style={{ marginBottom: 16 }}
        />
      )}
		<Card size="small" type="inner" title={text.maintenanceTitle} style={{ marginBottom: 24 }}>
			<Space direction="vertical" style={{ width: "100%" }}>
				<Alert
					showIcon
					type={maintenance?.active ? "warning" : "success"}
					message={maintenance?.active ? text.maintenanceActive : text.maintenanceInactive}
				/>
				<Typography.Text strong>{text.maintenanceMessage}</Typography.Text>
				<Input.TextArea
					value={maintenanceMessage}
					onChange={(event) => setMaintenanceMessage(event.target.value)}
					maxLength={280}
					showCount
					rows={2}
					disabled={updatingMaintenance}
				/>
				{maintenance?.active ? (
					<Button loading={updatingMaintenance} onClick={() => void updateMaintenance(false)}>{text.disableMaintenance}</Button>
				) : (
					<Popconfirm title={text.confirmMaintenance} onConfirm={() => updateMaintenance(true)} okText={text.enableMaintenance}>
						<Button danger loading={updatingMaintenance}>{text.enableMaintenance}</Button>
					</Popconfirm>
				)}
			</Space>
		</Card>
		<Card size="small" type="inner" title={text.runtimeLog} style={{ marginBottom: 24 }}>
			<Space direction="vertical" style={{ width: "100%" }}>
				<Select
					aria-label={text.logGeneration}
					value={runtimeGeneration}
					onChange={setRuntimeGeneration}
					options={Array.from({ length: (runtimeLog?.max_generation ?? 5) + 1 }, (_, generation) => ({
						value: generation,
						label: generation === 0 ? text.currentLog : `${text.previousLog} ${generation}`,
					}))}
				/>
				{runtimeLog?.available ? (
					<>
						<Typography.Text type="secondary">
							{runtimeLog.file_name} · {bytes(runtimeLog.size_bytes)}
							{runtimeLog.truncated ? ` · ${text.logTruncated}` : ""}
						</Typography.Text>
						<pre style={{ maxHeight: 360, overflow: "auto", padding: 12, background: "#111827", color: "#e5e7eb", whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}>
							{runtimeLog.lines.join("\n")}
						</pre>
					</>
				) : <Alert type="info" showIcon message={text.logUnavailable} />}
			</Space>
		</Card>
      <Typography.Title level={5}>{text.components}</Typography.Title>
      <Table<SystemComponentHealth>
        rowKey="component"
        loading={loading}
        dataSource={health?.components ?? []}
        columns={componentColumns}
        pagination={false}
        scroll={{ x: true }}
      />
      <Typography.Title level={5} style={{ marginTop: 24 }}>{text.resources}</Typography.Title>
      <Table<SystemResourceHealth>
        rowKey="resource"
        loading={loading}
        dataSource={health?.resources ?? []}
        columns={resourceColumns}
        pagination={false}
        scroll={{ x: true }}
      />
    </Card>
  );
}
