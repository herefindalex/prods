import { useEffect, useState } from "react";
import { Alert, Button, Card, Descriptions, Input, List, Modal, Popconfirm, Select, Space, Table, Tag, Typography, message } from "antd";
import { APIError, api, clientID, downloadFile, postJSON, putJSON } from "./api";
import type { AuditEntry, RFQ, RFQDeliveryAttempt, RFQRecipientSettings, RFQRecipientUser } from "./types";

const transitions: Record<RFQ["Status"], RFQ["Status"][]> = {
  new: ["in_progress", "spam"],
  in_progress: ["closed", "spam"],
  closed: ["in_progress"],
  spam: ["new", "in_progress"],
};

type ActivityLocale = "en-US" | "zh-TW";

const labels = {
  "en-US": {
    status: { new: "New", in_progress: "In progress", closed: "Closed", spam: "Spam" } as Record<RFQ["Status"], string>,
    exportDownloaded: "RFQ export downloaded.", updateStatusFailed: "Could not update RFQ status",
    anonymized: "RFQ personal data was anonymized locally.", anonymizeFailed: "Could not anonymize RFQ personal data",
    recipientsUpdated: "RFQ recipients updated", recipientsFailed: "Could not update recipients",
    deliveryQueued: "Manual RFQ delivery was durably queued",
    assignmentNotice: "Recipient assignment does not grant Admin access and does not send email. Sending remains a separate explicit action.",
    defaultRecipients: "Default recipients", savedRFQs: "Saved RFQs", exportCSV: "Export CSV", refresh: "Refresh", noRFQs: "No RFQs yet",
    updateStatus: (id: string) => `Update status for ${id}`, recipients: "Recipients", deliveryHistory: "Delivery history", sendHistory: "Send / history",
    anonymizeConfirm: "Anonymize this RFQ's personal data?",
    anonymizeDescription: "This removes local contact details, free-form text, assignments, and stored mail content. Sent email and older backups cannot be recalled.",
    anonymize: "Anonymize", anonymizedRFQ: "Anonymized RFQ", personalDataRemoved: "personal data removed", revision: "rev",
    email: "Email", removed: "Removed", created: "Created", items: "Items", message: "Message", privacyProcessed: "Privacy processed",
    user: "user", external: "external", unassigned: "Unassigned", adminLog: "Admin Log", time: "Time", action: "Action", target: "Target", actor: "Actor", result: "Result", details: "Details", system: "system",
    defaultRecipientsTitle: "Default recipients for new RFQs", recipientsTitle: (id?: string) => id ? `Recipients — ${id}` : "Recipients",
    assignmentOnly: "Assignment only: this does not grant access or send email.", systemUsers: "System users", externalEmails: "External email addresses", onePerLine: "One address per line",
    deliveryTitle: (id?: string) => id ? `Manual delivery — ${id}` : "Manual delivery", sendNow: "Send now",
    sendWarning: "This is an explicit external send. SMTP acceptance does not prove inbox delivery or reading. Unknown outcomes are never retried automatically.",
    none: "None", rfqRevision: "RFQ revision", noAttempts: "No manual delivery attempts",
    attemptStatus: { pending: "Pending", accepted: "Accepted", failed: "Failed", unknown: "Unknown" } as Record<string, string>,
  },
  "zh-TW": {
    status: { new: "新詢價", in_progress: "處理中", closed: "已結案", spam: "垃圾詢價" } as Record<RFQ["Status"], string>,
    exportDownloaded: "已下載 RFQ 匯出檔。", updateStatusFailed: "無法更新 RFQ 狀態",
    anonymized: "已在本機匿名化 RFQ 個人資料。", anonymizeFailed: "無法匿名化 RFQ 個人資料",
    recipientsUpdated: "已更新 RFQ 收件人", recipientsFailed: "無法更新收件人",
    deliveryQueued: "已持久排入手動 RFQ 寄送",
    assignmentNotice: "指派收件人不會授予 Admin 存取權，也不會寄送電子郵件；寄送仍是另一個明確操作。",
    defaultRecipients: "預設收件人", savedRFQs: "已保存的 RFQ", exportCSV: "匯出 CSV", refresh: "重新整理", noRFQs: "尚無 RFQ",
    updateStatus: (id: string) => `更新 ${id} 的狀態`, recipients: "收件人", deliveryHistory: "寄送紀錄", sendHistory: "寄送／紀錄",
    anonymizeConfirm: "要匿名化這筆 RFQ 的個人資料嗎？",
    anonymizeDescription: "這會移除本機聯絡資料、自由文字、指派及已保存的郵件內容；已寄出的電子郵件與舊備份無法收回。",
    anonymize: "匿名化", anonymizedRFQ: "已匿名化 RFQ", personalDataRemoved: "個人資料已移除", revision: "修訂",
    email: "電子郵件", removed: "已移除", created: "建立時間", items: "項目", message: "訊息", privacyProcessed: "隱私處理時間",
    user: "使用者", external: "外部", unassigned: "未指派", adminLog: "管理紀錄", time: "時間", action: "動作", target: "目標", actor: "操作者", result: "結果", details: "詳細資料", system: "系統",
    defaultRecipientsTitle: "新 RFQ 的預設收件人", recipientsTitle: (id?: string) => id ? `收件人 — ${id}` : "收件人",
    assignmentOnly: "這只會指派收件人，不會授予存取權或寄送電子郵件。", systemUsers: "系統使用者", externalEmails: "外部電子郵件地址", onePerLine: "每行一個地址",
    deliveryTitle: (id?: string) => id ? `手動寄送 — ${id}` : "手動寄送", sendNow: "立即寄送",
    sendWarning: "這是明確的外部寄送。SMTP 接受不代表郵件已送達收件匣或被閱讀；結果不明時絕不自動重試。",
    none: "無", rfqRevision: "RFQ 修訂", noAttempts: "尚無手動寄送紀錄",
    attemptStatus: { pending: "等待中", accepted: "已接受", failed: "失敗", unknown: "結果不明" } as Record<string, string>,
  },
} as const;

export function ActivityPanel({ locale, onError }: { locale: ActivityLocale; onError: (next: unknown) => void }) {
  const text = labels[locale];
  const [rfqs, setRFQs] = useState<RFQ[]>([]);
  const [audit, setAudit] = useState<AuditEntry[]>([]);
  const [recipientUsers, setRecipientUsers] = useState<RFQRecipientUser[]>([]);
  const [recipientSettings, setRecipientSettings] = useState<RFQRecipientSettings>();
  const [canManage, setCanManage] = useState(false);
  const [editing, setEditing] = useState<RFQ>();
  const [editingDefaults, setEditingDefaults] = useState(false);
  const [selectedUsers, setSelectedUsers] = useState<string[]>([]);
  const [externalEmails, setExternalEmails] = useState("");
  const [saving, setSaving] = useState(false);
  const [deliveryRFQ, setDeliveryRFQ] = useState<RFQ>();
  const [deliveryKey, setDeliveryKey] = useState("");
  const [deliveries, setDeliveries] = useState<RFQDeliveryAttempt[]>([]);
	const [anonymizing, setAnonymizing] = useState<string>();
  const [sending, setSending] = useState(false);
  const [exporting, setExporting] = useState(false);

  const exportRFQs = async () => {
    setExporting(true);
    try {
      await downloadFile("/admin/api/exports/rfqs.csv", "prods-rfqs.csv");
      message.success(text.exportDownloaded);
    } catch (error) {
      onError(error);
    } finally {
      setExporting(false);
    }
  };

  const loadRFQs = async () => setRFQs((await api<RFQ[]>("/admin/api/rfqs")) ?? []);
  const loadAudit = async () => setAudit((await api<AuditEntry[]>("/admin/api/audit")) ?? []);

  const loadRecipientUsers = async () => {
    try {
      const [users, settings] = await Promise.all([
        api<RFQRecipientUser[]>("/admin/api/rfq-recipient-users"),
        api<RFQRecipientSettings>("/admin/api/rfq-recipient-settings"),
      ]);
      setRecipientUsers(users ?? []);
      setRecipientSettings(settings);
      setCanManage(true);
    } catch (error) {
      if (error instanceof APIError && error.status === 403) {
        setCanManage(false);
        return;
      }
      onError(error);
    }
  };

  useEffect(() => {
    void loadRFQs();
    void loadAudit();
    void loadRecipientUsers();
  }, []);

  const updateStatus = async (rfq: RFQ, status: RFQ["Status"]) => {
    try {
      const updated = await putJSON<RFQ>(`/admin/api/rfqs/${encodeURIComponent(rfq.ID)}/status`, {
        expected_revision: rfq.Revision,
        status,
      });
      setRFQs((current) => current.map((item) => (item.ID === updated.ID ? updated : item)));
      void loadAudit();
    } catch (error) {
      message.error(error instanceof Error ? error.message : text.updateStatusFailed);
      void loadRFQs();
    }
  };

	const anonymizeRFQ = async (rfq: RFQ) => {
		setAnonymizing(rfq.ID);
		try {
			const updated = await postJSON<RFQ>(`/admin/api/rfqs/${encodeURIComponent(rfq.ID)}/anonymize`, {
				expected_revision: rfq.Revision,
			});
			setRFQs((current) => current.map((item) => (item.ID === updated.ID ? updated : item)));
			void loadAudit();
			message.success(text.anonymized);
		} catch (error) {
			message.error(error instanceof Error ? error.message : text.anonymizeFailed);
			void loadRFQs();
		} finally {
			setAnonymizing(undefined);
		}
	};

  const openRecipients = (rfq: RFQ) => {
	setEditingDefaults(false);
    setEditing(rfq);
    setSelectedUsers(
      (rfq.Recipients ?? [])
        .filter((item) => item.kind === "user")
        .map((item) => item.user_id ?? "")
        .filter(Boolean),
    );
    setExternalEmails(
      (rfq.Recipients ?? [])
        .filter((item) => item.kind === "email")
        .map((item) => item.email ?? "")
        .filter(Boolean)
        .join("\n"),
    );
  };

  const openDefaultRecipients = () => {
    if (!recipientSettings) return;
    setEditing(undefined);
    setEditingDefaults(true);
    setSelectedUsers(
      (recipientSettings.recipients ?? [])
        .filter((item) => item.kind === "user")
        .map((item) => item.user_id ?? "")
        .filter(Boolean),
    );
    setExternalEmails(
      (recipientSettings.recipients ?? [])
        .filter((item) => item.kind === "email")
        .map((item) => item.email ?? "")
        .filter(Boolean)
        .join("\n"),
    );
  };

  const saveRecipients = async () => {
    if (!editing && !editingDefaults) return;
    const emails = externalEmails.split(/[\n,;]/).map((value) => value.trim()).filter(Boolean);
    setSaving(true);
    try {
      const recipients = [
        ...selectedUsers.map((user_id) => ({ kind: "user" as const, user_id })),
        ...emails.map((email) => ({ kind: "email" as const, email })),
      ];
      if (editingDefaults && recipientSettings) {
        const updated = await putJSON<RFQRecipientSettings>("/admin/api/rfq-recipient-settings", {
          expected_revision: recipientSettings.revision,
          recipients,
        });
        setRecipientSettings(updated);
      } else if (editing) {
        const updated = await putJSON<RFQ>(`/admin/api/rfqs/${encodeURIComponent(editing.ID)}/recipients`, {
          expected_revision: editing.Revision,
          recipients,
        });
        setRFQs((current) => current.map((item) => (item.ID === updated.ID ? updated : item)));
      }
      setEditing(undefined);
      setEditingDefaults(false);
      void loadAudit();
      message.success(text.recipientsUpdated);
    } catch (error) {
      message.error(error instanceof Error ? error.message : text.recipientsFailed);
      void loadRFQs();
    } finally {
      setSaving(false);
    }
  };

  const loadDeliveries = async (rfq: RFQ) => {
    const attempts = await api<RFQDeliveryAttempt[]>(`/admin/api/rfqs/${encodeURIComponent(rfq.ID)}/deliveries`);
    setDeliveries(attempts ?? []);
  };

  const openDelivery = async (rfq: RFQ) => {
    setDeliveryRFQ(rfq);
    setDeliveryKey(clientID("mail"));
    try {
      await loadDeliveries(rfq);
    } catch (error) {
      onError(error);
    }
  };

  const sendRFQ = async () => {
    if (!deliveryRFQ || !deliveryKey) return;
    setSending(true);
    try {
      await postJSON<RFQDeliveryAttempt>(`/admin/api/rfqs/${encodeURIComponent(deliveryRFQ.ID)}/deliveries`, {
        expected_revision: deliveryRFQ.Revision,
        delivery_key: deliveryKey,
      });
      await loadDeliveries(deliveryRFQ);
      void loadAudit();
      message.success(text.deliveryQueued);
      setDeliveryKey(clientID("mail"));
    } catch (error) {
      // Keep the same delivery key. A retry after a lost response must replay
      // the original durable attempt instead of creating another send.
      onError(error);
    } finally {
      setSending(false);
    }
  };

  return (
    <Space direction="vertical" size="large" className="panel-stack">
      <Alert
        message={text.assignmentNotice}
        type="info"
        showIcon
        action={canManage ? <Button size="small" onClick={openDefaultRecipients}>{text.defaultRecipients}</Button> : undefined}
      />
      <Card
        title={text.savedRFQs}
        extra={
          <Space>
            <Button loading={exporting} onClick={() => void exportRFQs()}>{text.exportCSV}</Button>
            <Button onClick={() => void loadRFQs()}>{text.refresh}</Button>
          </Space>
        }
      >
        <List
          dataSource={rfqs}
		locale={{ emptyText: text.noRFQs }}
          renderItem={(rfq) => (
            <List.Item
              actions={canManage ? [
                <Select
                  key="status"
                  aria-label={text.updateStatus(rfq.ID)}
                  value={rfq.Status}
                  style={{ width: 140 }}
                  options={[
                    { value: rfq.Status, label: text.status[rfq.Status] },
                    ...transitions[rfq.Status].map((value) => ({ value, label: text.status[value] })),
                  ]}
                  onChange={(value) => void updateStatus(rfq, value)}
                />,
					<Button key="recipients" disabled={rfq.PrivacyState === "anonymized"} onClick={() => openRecipients(rfq)}>{text.recipients}</Button>,
				<Button key="send" disabled={!rfq.Recipients?.length && rfq.PrivacyState !== "anonymized"} onClick={() => void openDelivery(rfq)}>
						{rfq.PrivacyState === "anonymized" ? text.deliveryHistory : text.sendHistory}
				</Button>,
				rfq.PrivacyState === "anonymized" ? null : (
					<Popconfirm
						key="anonymize"
							title={text.anonymizeConfirm}
							description={text.anonymizeDescription}
							okText={text.anonymize}
						okButtonProps={{ danger: true }}
						onConfirm={() => anonymizeRFQ(rfq)}
					>
							<Button danger loading={anonymizing === rfq.ID}>{text.anonymize}</Button>
					</Popconfirm>
				),
              ] : undefined}
            >
              <List.Item.Meta
                title={
                  <Space>
					<span>{`${rfq.ID} — ${rfq.Name || text.anonymizedRFQ}`}</span>
                    <Tag>{text.status[rfq.Status]}</Tag>
					{rfq.PrivacyState === "anonymized" ? <Tag color="default">{text.personalDataRemoved}</Tag> : null}
                    <Typography.Text type="secondary">{text.revision} {rfq.Revision}</Typography.Text>
                  </Space>
                }
                description={
                  <Descriptions size="small" column={{ xs: 1, sm: 2 }}>
						<Descriptions.Item label={text.email}>{rfq.Email || text.removed}</Descriptions.Item>
                    <Descriptions.Item label={text.created}>{new Date(rfq.CreatedAt).toLocaleString(locale)}</Descriptions.Item>
                    <Descriptions.Item label={text.items} span={2}>
                      {rfq.Items?.map((item) => item.product_id || item.requested || item.raw_query).join(", ")}
                    </Descriptions.Item>
                    <Descriptions.Item label={text.recipients} span={2}>
                      {rfq.Recipients?.length
                        ? rfq.Recipients.map((item) => item.kind === "user" ? `${item.display_name || item.email} (${text.user})` : `${item.email} (${text.external})`).join(", ")
                        : text.unassigned}
                    </Descriptions.Item>
                    {rfq.GeneralMessage ? <Descriptions.Item label={text.message} span={2}>{rfq.GeneralMessage}</Descriptions.Item> : null}
					{rfq.PrivacyAt ? <Descriptions.Item label={text.privacyProcessed} span={2}>{new Date(rfq.PrivacyAt).toLocaleString(locale)}</Descriptions.Item> : null}
                  </Descriptions>
                }
              />
            </List.Item>
          )}
        />
      </Card>
      <Card title={text.adminLog} extra={<Button onClick={() => void loadAudit()}>{text.refresh}</Button>}>
        <Table<AuditEntry>
          rowKey="id"
          dataSource={audit}
          pagination={{ pageSize: 25, hideOnSinglePage: true }}
          scroll={{ x: 900 }}
          columns={[
            { title: text.time, dataIndex: "created_at", render: (value: string) => value ? new Date(value).toLocaleString(locale) : "—" },
            { title: text.action, dataIndex: "action", render: (value: string) => <Tag>{value}</Tag> },
            { title: text.target, render: (_, row) => `${row.target_type}:${row.target_id}` },
            { title: text.actor, dataIndex: "actor_id", render: (value: string) => value || text.system },
            { title: text.result, dataIndex: "result" },
            { title: text.details, dataIndex: "details", render: (value: unknown) => <Typography.Text code>{typeof value === "string" ? value : JSON.stringify(value)}</Typography.Text> },
          ]}
        />
      </Card>
      <Modal
        title={editingDefaults ? text.defaultRecipientsTitle : text.recipientsTitle(editing?.ID)}
        open={Boolean(editing) || editingDefaults}
        onCancel={() => { setEditing(undefined); setEditingDefaults(false); }}
        onOk={() => void saveRecipients()}
        confirmLoading={saving}
        destroyOnHidden
      >
        <Space direction="vertical" className="panel-stack">
          <Alert message={text.assignmentOnly} type="warning" showIcon />
          <div>
            <Typography.Text strong>{text.systemUsers}</Typography.Text>
            <Select
              mode="multiple"
              value={selectedUsers}
              onChange={setSelectedUsers}
              style={{ width: "100%" }}
              options={recipientUsers.map((user) => ({ value: user.id, label: `${user.display_name} — ${user.email}` }))}
            />
          </div>
          <div>
            <Typography.Text strong>{text.externalEmails}</Typography.Text>
            <Input.TextArea
              rows={4}
              value={externalEmails}
              onChange={(event) => setExternalEmails(event.target.value)}
              placeholder={text.onePerLine}
            />
          </div>
        </Space>
      </Modal>
      <Modal
        title={text.deliveryTitle(deliveryRFQ?.ID)}
        open={Boolean(deliveryRFQ)}
        onCancel={() => setDeliveryRFQ(undefined)}
        onOk={() => void sendRFQ()}
        okText={text.sendNow}
        confirmLoading={sending}
        okButtonProps={{ disabled: !deliveryRFQ?.Recipients?.length }}
        width={760}
      >
        <Space direction="vertical" className="panel-stack">
          <Alert
            type="warning"
            showIcon
            message={text.sendWarning}
          />
          <Descriptions size="small" column={1}>
            <Descriptions.Item label={text.recipients}>
              {deliveryRFQ?.Recipients?.map((item) => item.email).join(", ") || text.none}
            </Descriptions.Item>
            <Descriptions.Item label={text.rfqRevision}>{deliveryRFQ?.Revision}</Descriptions.Item>
            <Descriptions.Item label={text.message}>{deliveryRFQ?.GeneralMessage || "—"}</Descriptions.Item>
          </Descriptions>
          <Card size="small" title={text.deliveryHistory} extra={<Button size="small" onClick={() => deliveryRFQ && void loadDeliveries(deliveryRFQ)}>{text.refresh}</Button>}>
            <List
              dataSource={deliveries}
              locale={{ emptyText: text.noAttempts }}
              renderItem={(attempt) => (
                <List.Item>
                  <List.Item.Meta
                    title={<Space><Tag>{text.attemptStatus[attempt.status] ?? attempt.status}</Tag><span>{new Date(attempt.created_at).toLocaleString(locale)}</span><Typography.Text type="secondary">{attempt.id}</Typography.Text></Space>}
                    description={attempt.recipients.map((recipient) => `${recipient.email}: ${recipient.status}${recipient.error_class ? ` (${recipient.error_class})` : ""}`).join(", ")}
                  />
                </List.Item>
              )}
            />
          </Card>
        </Space>
      </Modal>
    </Space>
  );
}
