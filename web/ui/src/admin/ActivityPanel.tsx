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

const statusLabel: Record<RFQ["Status"], string> = {
  new: "New",
  in_progress: "In Progress",
  closed: "Closed",
  spam: "Spam",
};

export function ActivityPanel({ onError }: { onError: (next: unknown) => void }) {
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
      message.success("RFQ export downloaded.");
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
      message.error(error instanceof Error ? error.message : "Could not update RFQ status");
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
			message.success("RFQ personal data was anonymized locally.");
		} catch (error) {
			message.error(error instanceof Error ? error.message : "Could not anonymize RFQ personal data");
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
      message.success("RFQ recipients updated");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "Could not update recipients");
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
      message.success("Manual RFQ delivery was durably queued");
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
        message="Recipient assignment does not grant Admin access and does not send email. Sending remains a separate explicit action."
        type="info"
        showIcon
        action={canManage ? <Button size="small" onClick={openDefaultRecipients}>Default recipients</Button> : undefined}
      />
      <Card
        title="Saved RFQs"
        extra={
          <Space>
            <Button loading={exporting} onClick={() => void exportRFQs()}>Export CSV</Button>
            <Button onClick={() => void loadRFQs()}>Refresh</Button>
          </Space>
        }
      >
        <List
          dataSource={rfqs}
          locale={{ emptyText: "No RFQs yet" }}
          renderItem={(rfq) => (
            <List.Item
              actions={canManage ? [
                <Select
                  key="status"
                  aria-label={`Update status for ${rfq.ID}`}
                  value={rfq.Status}
                  style={{ width: 140 }}
                  options={[
                    { value: rfq.Status, label: statusLabel[rfq.Status] },
                    ...transitions[rfq.Status].map((value) => ({ value, label: statusLabel[value] })),
                  ]}
                  onChange={(value) => void updateStatus(rfq, value)}
                />,
				<Button key="recipients" disabled={rfq.PrivacyState === "anonymized"} onClick={() => openRecipients(rfq)}>Recipients</Button>,
				<Button key="send" disabled={!rfq.Recipients?.length && rfq.PrivacyState !== "anonymized"} onClick={() => void openDelivery(rfq)}>
					{rfq.PrivacyState === "anonymized" ? "Delivery history" : "Send / history"}
				</Button>,
				rfq.PrivacyState === "anonymized" ? null : (
					<Popconfirm
						key="anonymize"
						title="Anonymize this RFQ's personal data?"
						description="This removes local contact details, free-form text, assignments, and stored mail content. Sent email and older backups cannot be recalled."
						okText="Anonymize"
						okButtonProps={{ danger: true }}
						onConfirm={() => anonymizeRFQ(rfq)}
					>
						<Button danger loading={anonymizing === rfq.ID}>Anonymize</Button>
					</Popconfirm>
				),
              ] : undefined}
            >
              <List.Item.Meta
                title={
                  <Space>
					<span>{`${rfq.ID} — ${rfq.Name || "Anonymized RFQ"}`}</span>
                    <Tag>{statusLabel[rfq.Status]}</Tag>
					{rfq.PrivacyState === "anonymized" ? <Tag color="default">personal data removed</Tag> : null}
                    <Typography.Text type="secondary">rev {rfq.Revision}</Typography.Text>
                  </Space>
                }
                description={
                  <Descriptions size="small" column={{ xs: 1, sm: 2 }}>
					<Descriptions.Item label="Email">{rfq.Email || "Removed"}</Descriptions.Item>
                    <Descriptions.Item label="Created">{rfq.CreatedAt}</Descriptions.Item>
                    <Descriptions.Item label="Items" span={2}>
                      {rfq.Items?.map((item) => item.product_id || item.requested || item.raw_query).join(", ")}
                    </Descriptions.Item>
                    <Descriptions.Item label="Recipients" span={2}>
                      {rfq.Recipients?.length
                        ? rfq.Recipients.map((item) => item.kind === "user" ? `${item.display_name || item.email} (user)` : `${item.email} (external)`).join(", ")
                        : "Unassigned"}
                    </Descriptions.Item>
                    {rfq.GeneralMessage ? <Descriptions.Item label="Message" span={2}>{rfq.GeneralMessage}</Descriptions.Item> : null}
					{rfq.PrivacyAt ? <Descriptions.Item label="Privacy processed" span={2}>{rfq.PrivacyAt}</Descriptions.Item> : null}
                  </Descriptions>
                }
              />
            </List.Item>
          )}
        />
      </Card>
      <Card title="Admin Log" extra={<Button onClick={() => void loadAudit()}>Refresh</Button>}>
        <Table<AuditEntry>
          rowKey="id"
          dataSource={audit}
          pagination={{ pageSize: 25, hideOnSinglePage: true }}
          scroll={{ x: 900 }}
          columns={[
            { title: "Time", dataIndex: "created_at", render: (value: string) => value ? new Date(value).toLocaleString() : "—" },
            { title: "Action", dataIndex: "action", render: (value: string) => <Tag>{value}</Tag> },
            { title: "Target", render: (_, row) => `${row.target_type}:${row.target_id}` },
            { title: "Actor", dataIndex: "actor_id", render: (value: string) => value || "system" },
            { title: "Result", dataIndex: "result" },
            { title: "Details", dataIndex: "details", render: (value: unknown) => <Typography.Text code>{typeof value === "string" ? value : JSON.stringify(value)}</Typography.Text> },
          ]}
        />
      </Card>
      <Modal
        title={editingDefaults ? "Default recipients for new RFQs" : editing ? `Recipients — ${editing.ID}` : "Recipients"}
        open={Boolean(editing) || editingDefaults}
        onCancel={() => { setEditing(undefined); setEditingDefaults(false); }}
        onOk={() => void saveRecipients()}
        confirmLoading={saving}
        destroyOnHidden
      >
        <Space direction="vertical" className="panel-stack">
          <Alert message="Assignment only: this does not grant access or send email." type="warning" showIcon />
          <div>
            <Typography.Text strong>System users</Typography.Text>
            <Select
              mode="multiple"
              value={selectedUsers}
              onChange={setSelectedUsers}
              style={{ width: "100%" }}
              options={recipientUsers.map((user) => ({ value: user.id, label: `${user.display_name} — ${user.email}` }))}
            />
          </div>
          <div>
            <Typography.Text strong>External email addresses</Typography.Text>
            <Input.TextArea
              rows={4}
              value={externalEmails}
              onChange={(event) => setExternalEmails(event.target.value)}
              placeholder="One address per line"
            />
          </div>
        </Space>
      </Modal>
      <Modal
        title={deliveryRFQ ? `Manual delivery — ${deliveryRFQ.ID}` : "Manual delivery"}
        open={Boolean(deliveryRFQ)}
        onCancel={() => setDeliveryRFQ(undefined)}
        onOk={() => void sendRFQ()}
        okText="Send now"
        confirmLoading={sending}
        okButtonProps={{ disabled: !deliveryRFQ?.Recipients?.length }}
        width={760}
      >
        <Space direction="vertical" className="panel-stack">
          <Alert
            type="warning"
            showIcon
            message="This is an explicit external send. SMTP acceptance does not prove inbox delivery or reading. Unknown outcomes are never retried automatically."
          />
          <Descriptions size="small" column={1}>
            <Descriptions.Item label="Recipients">
              {deliveryRFQ?.Recipients?.map((item) => item.email).join(", ") || "None"}
            </Descriptions.Item>
            <Descriptions.Item label="RFQ revision">{deliveryRFQ?.Revision}</Descriptions.Item>
            <Descriptions.Item label="Message">{deliveryRFQ?.GeneralMessage || "—"}</Descriptions.Item>
          </Descriptions>
          <Card size="small" title="Delivery history" extra={<Button size="small" onClick={() => deliveryRFQ && void loadDeliveries(deliveryRFQ)}>Refresh</Button>}>
            <List
              dataSource={deliveries}
              locale={{ emptyText: "No manual delivery attempts" }}
              renderItem={(attempt) => (
                <List.Item>
                  <List.Item.Meta
                    title={<Space><Tag>{attempt.status}</Tag><span>{attempt.created_at}</span><Typography.Text type="secondary">{attempt.id}</Typography.Text></Space>}
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
