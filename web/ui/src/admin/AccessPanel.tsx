import { useEffect, useState } from "react";
import { Alert, Button, Card, Form, Input, Modal, Popconfirm, Select, Space, Table, Tag, Typography } from "antd";
import { api, postJSON, putJSON } from "./api";
import { capabilities, type Capability, type Role, type User } from "./types";

type Feedback = { onError: (error: unknown) => void; onMessage: (message: string) => void };
type GrantResponse = { user: User; set_password_url: string; expires_at: string };
type InvitationMailStatus = "pending" | "accepted" | "failed" | "unknown";
type InvitationMailAttempt = {
  id: string;
  user_id: string;
  recipient_email: string;
  status: InvitationMailStatus;
  error_class?: string;
  error_message?: string;
  created_at: string;
  completed_at?: string;
};

type AccessLocale = "en-US" | "zh-TW";

const labels = {
  "en-US": {
    createdRole: (name: string) => `Created role ${name}.`,
    createdUser: (email: string) => `Created ${email}. Copy the one-time link now.`,
    changedRole: (email: string, role: string) => `Changed ${email} to ${role}. Existing sessions and unused set-password links were revoked.`,
    generatedLink: (email: string) => `Generated a new one-time set-password link for ${email}. Earlier unused links are invalid.`,
    reactivated: (email: string) => `Reactivated ${email}. They must use the new one-time link before signing in.`,
    disabled: (email: string) => `Disabled ${email}; existing sessions and grants were revoked.`,
    acceptedMessage: (email: string) => `SMTP accepted the invitation for ${email}.`,
    unknownMessage: (email: string) => `SMTP outcome for ${email} is unknown. Generate a new password link before sending again.`,
    failedMessage: (email: string) => `Invitation delivery for ${email} failed. The durable result is recorded.`,
    oneTimeLink: "One-time set-password link",
    bearerDescription: (email: string) => `This bearer link for ${email} is displayed only in this response. Copy it to a private channel, or explicitly send it through the configured SMTP server.`,
    copy: "Copy",
    acceptedTitle: "SMTP accepted the invitation",
    failedTitle: "Invitation delivery failed",
    unknownTitle: "Invitation outcome is unknown",
    unknownDescription: "Do not resend this same link. Generate a new password link before another explicit send.",
    durableAt: (value: string) => `Durable result recorded at ${value}.`,
    sendConfirm: "Send this bearer link by email?",
    sendDescription: "This is an explicit external SMTP send. Its accepted, failed, or unknown outcome will be recorded durably.",
    send: "Send invitation email",
    dismiss: "Dismiss",
    newRole: "New role",
    roleName: "Role name",
    capabilities: "Capabilities",
    createRole: "Create role",
    newUser: "New user",
    email: "Email",
    displayName: "Display name",
    role: "Role",
    createUser: "Create user",
    usersRoles: "Users and roles",
    refresh: "Refresh",
    status: "Status",
    authRevision: "Auth revision",
    actions: "Actions",
    changeRole: "Change role",
    history: "Invitation history",
    newPasswordLink: "New password link",
    newPasswordConfirm: "Generate a new set-password link?",
    newPasswordDescription: "Every earlier unused set-password link for this user will become invalid.",
    disableConfirm: "Disable this user and revoke all sessions and grants?",
    disable: "Disable",
    reactivateConfirm: "Reactivate this user?",
    reactivateDescription: "A new one-time set-password link will be required; the old password stays unusable.",
    reactivate: "Reactivate",
    historyFor: (email: string) => `Invitation history for ${email}`,
    evidenceTitle: "Delivery evidence does not contain the bearer link",
    evidenceDescription: "Accepted means the SMTP server accepted the message. Unknown is never retried automatically; generate a new password link before another send.",
    noAttempts: "No invitation email attempts recorded.",
    created: "Created",
    recipient: "Recipient",
    result: "Result",
    completed: "Durably completed",
    inProgress: "In progress",
    changeRoleFor: (email: string) => `Change role for ${email}`,
    accessImmediate: "Access changes immediately",
    accessDescription: "Changing a role revokes this user's current sessions and unused set-password links. The last usable Active Owner cannot be downgraded.",
    active: "active",
    disabledStatus: "disabled",
  },
  "zh-TW": {
    createdRole: (name: string) => `已建立角色「${name}」。`,
    createdUser: (email: string) => `已建立 ${email}；請立即複製一次性連結。`,
    changedRole: (email: string, role: string) => `已將 ${email} 改為「${role}」；既有工作階段與未使用設密碼連結已撤銷。`,
    generatedLink: (email: string) => `已為 ${email} 產生新的一次性設密碼連結；先前未使用的連結已失效。`,
    reactivated: (email: string) => `已重新啟用 ${email}；對方必須使用新的一次性連結設密碼後才能登入。`,
    disabled: (email: string) => `已停用 ${email}；既有工作階段與設密碼授權已撤銷。`,
    acceptedMessage: (email: string) => `SMTP 已接受寄給 ${email} 的邀請郵件。`,
    unknownMessage: (email: string) => `寄給 ${email} 的 SMTP 結果不明；再次寄送前請先產生新的設密碼連結。`,
    failedMessage: (email: string) => `寄給 ${email} 的邀請郵件失敗；結果已永久記錄。`,
    oneTimeLink: "一次性設密碼連結",
    bearerDescription: (email: string) => `這是 ${email} 的 bearer 連結，只會在此次回應顯示。請複製到適當的私密管道，或明確使用已設定的 SMTP 伺服器寄送。`,
    copy: "複製",
    acceptedTitle: "SMTP 已接受邀請郵件",
    failedTitle: "邀請郵件寄送失敗",
    unknownTitle: "邀請郵件結果不明",
    unknownDescription: "不要用同一個連結重送。再次明確寄送前，請先產生新的設密碼連結。",
    durableAt: (value: string) => `永久結果記錄時間：${value}。`,
    sendConfirm: "要用 Email 寄送這個 bearer 連結嗎？",
    sendDescription: "這會執行明確的外部 SMTP 寄送；accepted、failed 或 unknown 結果都會永久記錄。",
    send: "寄送邀請郵件",
    dismiss: "關閉",
    newRole: "新增角色",
    roleName: "角色名稱",
    capabilities: "權限能力",
    createRole: "建立角色",
    newUser: "新增使用者",
    email: "Email",
    displayName: "顯示名稱",
    role: "角色",
    createUser: "建立使用者",
    usersRoles: "使用者與角色",
    refresh: "重新整理",
    status: "狀態",
    authRevision: "認證修訂",
    actions: "操作",
    changeRole: "變更角色",
    history: "邀請郵件歷史",
    newPasswordLink: "新設密碼連結",
    newPasswordConfirm: "要產生新的設密碼連結嗎？",
    newPasswordDescription: "這位使用者先前所有未使用的設密碼連結都會失效。",
    disableConfirm: "要停用這位使用者，並撤銷所有工作階段與設密碼授權嗎？",
    disable: "停用",
    reactivateConfirm: "要重新啟用這位使用者嗎？",
    reactivateDescription: "必須建立新的一次性設密碼連結；舊密碼仍不可使用。",
    reactivate: "重新啟用",
    historyFor: (email: string) => `${email} 的邀請郵件歷史`,
    evidenceTitle: "寄送證據不包含 bearer 連結",
    evidenceDescription: "Accepted 表示 SMTP 伺服器已接受郵件。Unknown 絕不自動重送；再次寄送前請先產生新的設密碼連結。",
    noAttempts: "尚無邀請郵件寄送紀錄。",
    created: "建立時間",
    recipient: "收件人",
    result: "結果",
    completed: "已永久完成",
    inProgress: "處理中",
    changeRoleFor: (email: string) => `變更 ${email} 的角色`,
    accessImmediate: "存取權限會立即變更",
    accessDescription: "變更角色會撤銷這位使用者目前的工作階段與未使用設密碼連結。最後一位可正常登入的 Active Owner 不可降級。",
    active: "啟用中",
    disabledStatus: "已停用",
  },
};

export function AccessPanel({ locale, onError, onMessage }: Feedback & { locale: AccessLocale }) {
  const text = labels[locale];
  const [roles, setRoles] = useState<Role[]>([]);
  const [users, setUsers] = useState<User[]>([]);
  const [visibleGrant, setVisibleGrant] = useState<GrantResponse>();
  const [invitationAttempt, setInvitationAttempt] = useState<InvitationMailAttempt>();
  const [sendingInvitation, setSendingInvitation] = useState(false);
  const [invitationHistoryUser, setInvitationHistoryUser] = useState<User>();
  const [invitationHistory, setInvitationHistory] = useState<InvitationMailAttempt[]>([]);
  const [loadingInvitationHistory, setLoadingInvitationHistory] = useState(false);
  const [roleUser, setRoleUser] = useState<User>();
  const [roleForm] = Form.useForm<{ name: string; capabilities: Capability[] }>();
  const [userForm] = Form.useForm<{ email: string; display_name: string; role_id: string }>();
  const [roleEditForm] = Form.useForm<{ role_id: string }>();

  const load = async () => {
    try {
      const [nextRoles, nextUsers] = await Promise.all([
        api<Role[]>("/admin/api/roles"),
        api<User[]>("/admin/api/users"),
      ]);
      setRoles(nextRoles ?? []);
      setUsers(nextUsers ?? []);
    } catch (error) {
      onError(error);
    }
  };
  useEffect(() => void load(), []);
  const showGrant = (response: GrantResponse) => {
    setVisibleGrant(response);
    setInvitationAttempt(undefined);
  };
  const dismissGrantFor = (userID: string) => {
    if (visibleGrant?.user.id === userID) {
      setVisibleGrant(undefined);
      setInvitationAttempt(undefined);
    }
  };

  const createRole = async (values: { name: string; capabilities: Capability[] }) => {
    try {
      await postJSON<Role>("/admin/api/roles", values);
      roleForm.resetFields();
      onMessage(text.createdRole(values.name));
      await load();
    } catch (error) {
      onError(error);
    }
  };

  const createUser = async (values: { email: string; display_name: string; role_id: string }) => {
    try {
			const response = await postJSON<GrantResponse>("/admin/api/users", values);
      showGrant(response);
      userForm.resetFields();
      onMessage(text.createdUser(response.user.email));
      await load();
    } catch (error) {
      onError(error);
    }
	};

	const beginRoleChange = (user: User) => {
		setRoleUser(user);
		roleEditForm.setFieldsValue({ role_id: user.role_id });
	};

	const changeRole = async (values: { role_id: string }) => {
		if (!roleUser) return;
		try {
			const updated = await putJSON<User>(`/admin/api/users/${roleUser.id}/role`, values);
			dismissGrantFor(updated.id);
			setRoleUser(undefined);
			roleEditForm.resetFields();
			onMessage(text.changedRole(updated.email, roles.find((role) => role.id === updated.role_id)?.name ?? updated.role_id));
			await load();
		} catch (error) {
			onError(error);
			await load();
		}
	};

	const issueSetPasswordGrant = async (user: User) => {
		try {
			const response = await postJSON<GrantResponse>(`/admin/api/users/${user.id}/set-password-grant`, {});
			showGrant(response);
			onMessage(text.generatedLink(response.user.email));
		} catch (error) {
			onError(error);
		}
	};

	const reactivateUser = async (user: User) => {
		try {
			const response = await postJSON<GrantResponse>(`/admin/api/users/${user.id}/reactivate`, {});
			showGrant(response);
			onMessage(text.reactivated(response.user.email));
			await load();
		} catch (error) {
			onError(error);
			await load();
		}
	};

  const disableUser = async (user: User) => {
    try {
      await postJSON<void>(`/admin/api/users/${user.id}/disable`, {});
      dismissGrantFor(user.id);
      onMessage(text.disabled(user.email));
      await load();
    } catch (error) {
      onError(error);
    }
  };

  const sendInvitation = async () => {
    if (!visibleGrant) return;
    setSendingInvitation(true);
    try {
      const attempt = await postJSON<InvitationMailAttempt>(
        `/admin/api/users/${visibleGrant.user.id}/send-set-password`,
        { set_password_url: visibleGrant.set_password_url },
      );
      setInvitationAttempt(attempt);
      if (attempt.status === "accepted") {
        onMessage(text.acceptedMessage(attempt.recipient_email));
      } else if (attempt.status === "unknown") {
        onMessage(text.unknownMessage(attempt.recipient_email));
      } else {
        onMessage(text.failedMessage(attempt.recipient_email));
      }
    } catch (error) {
      onError(error);
    } finally {
      setSendingInvitation(false);
    }
  };

  const showInvitationHistory = async (user: User) => {
    setInvitationHistoryUser(user);
    setLoadingInvitationHistory(true);
    try {
      const attempts = await api<InvitationMailAttempt[]>(`/admin/api/users/${user.id}/invitation-mail-attempts`);
      setInvitationHistory(attempts ?? []);
    } catch (error) {
      setInvitationHistoryUser(undefined);
      onError(error);
    } finally {
      setLoadingInvitationHistory(false);
    }
  };

  return (
    <Space direction="vertical" size="large" className="panel-stack">
      {visibleGrant && (
        <Alert
          type="warning"
          showIcon
          message={text.oneTimeLink}
          description={
            <Space direction="vertical" className="panel-stack">
              <Typography.Text>
                {text.bearerDescription(visibleGrant.user.email)}
              </Typography.Text>
              <Input
                value={visibleGrant.set_password_url}
                readOnly
                addonAfter={<Button type="link" onClick={() => void navigator.clipboard.writeText(visibleGrant.set_password_url)}>{text.copy}</Button>}
              />
              {invitationAttempt && (
                <Alert
                  showIcon
                  type={invitationAttempt.status === "accepted" ? "success" : invitationAttempt.status === "failed" ? "error" : "warning"}
                  message={invitationAttempt.status === "accepted" ? text.acceptedTitle : invitationAttempt.status === "failed" ? text.failedTitle : text.unknownTitle}
                  description={invitationAttempt.status === "unknown"
                    ? text.unknownDescription
                    : invitationAttempt.error_message || text.durableAt(invitationAttempt.completed_at ?? invitationAttempt.created_at)}
                />
              )}
              <Space wrap>
                <Popconfirm
                  title={text.sendConfirm}
                  description={text.sendDescription}
                  onConfirm={() => void sendInvitation()}
                >
                  <Button
                    type="primary"
                    loading={sendingInvitation}
                    disabled={invitationAttempt?.status === "accepted" || invitationAttempt?.status === "unknown"}
                  >
                    {text.send}
                  </Button>
                </Popconfirm>
                <Button size="small" onClick={() => { setVisibleGrant(undefined); setInvitationAttempt(undefined); }}>{text.dismiss}</Button>
              </Space>
            </Space>
          }
        />
      )}
      <Card title={text.newRole}>
        <Form form={roleForm} layout="vertical" onFinish={(values) => void createRole(values)}>
          <div className="form-grid">
            <Form.Item name="name" label={text.roleName} rules={[{ required: true }]}><Input /></Form.Item>
            <Form.Item name="capabilities" label={text.capabilities} rules={[{ required: true }]}>
              <Select mode="multiple" options={capabilities.map((value) => ({ value, label: value }))} />
            </Form.Item>
          </div>
          <Button type="primary" htmlType="submit">{text.createRole}</Button>
        </Form>
      </Card>
      <Card title={text.newUser}>
        <Form form={userForm} layout="vertical" onFinish={(values) => void createUser(values)}>
          <div className="form-grid three-columns">
            <Form.Item name="email" label={text.email} rules={[{ required: true, type: "email" }]}><Input /></Form.Item>
            <Form.Item name="display_name" label={text.displayName} rules={[{ required: true }]}><Input /></Form.Item>
            <Form.Item name="role_id" label={text.role} rules={[{ required: true }]}>
              <Select options={roles.filter((role) => role.status === "active").map((role) => ({ value: role.id, label: role.name }))} />
            </Form.Item>
          </div>
          <Button type="primary" htmlType="submit">{text.createUser}</Button>
        </Form>
      </Card>
      <Card title={text.usersRoles} extra={<Button onClick={() => void load()}>{text.refresh}</Button>}>
        <Table<User>
          rowKey="id"
          dataSource={users}
          pagination={false}
          expandable={{
            expandedRowRender: (user) => {
              const role = roles.find((item) => item.id === user.role_id);
              return <Space wrap>{role?.capabilities.map((capability) => <Tag key={capability}>{capability}</Tag>)}</Space>;
            },
          }}
          columns={[
            { title: text.email, dataIndex: "email" },
            { title: text.displayName, dataIndex: "display_name" },
            { title: text.role, render: (_, user) => roles.find((role) => role.id === user.role_id)?.name ?? user.role_id },
            { title: text.status, render: (_, user) => <Tag color={user.status === "active" ? "green" : "default"}>{user.status === "active" ? text.active : text.disabledStatus}</Tag> },
            { title: text.authRevision, dataIndex: "auth_revision" },
			{
			  title: text.actions,
			  render: (_, user) => (
				<Space wrap>
				  <Button size="small" onClick={() => beginRoleChange(user)}>{text.changeRole}</Button>
				  <Button size="small" onClick={() => void showInvitationHistory(user)}>{text.history}</Button>
				  {user.status === "active" ? (
					<>
					  <Popconfirm
						title={text.newPasswordConfirm}
						description={text.newPasswordDescription}
						onConfirm={() => void issueSetPasswordGrant(user)}
					  >
						<Button size="small">{text.newPasswordLink}</Button>
					  </Popconfirm>
					  <Popconfirm title={text.disableConfirm} onConfirm={() => void disableUser(user)}>
						<Button size="small" danger>{text.disable}</Button>
					  </Popconfirm>
					</>
				  ) : (
					<Popconfirm
					  title={text.reactivateConfirm}
					  description={text.reactivateDescription}
					  onConfirm={() => void reactivateUser(user)}
					>
					  <Button size="small" type="primary">{text.reactivate}</Button>
					</Popconfirm>
				  )}
				</Space>
			  ),
			},
          ]}
		/>
	 </Card>

      <Modal
        open={invitationHistoryUser !== undefined}
        title={invitationHistoryUser ? text.historyFor(invitationHistoryUser.email) : text.history}
        footer={null}
        width={860}
        onCancel={() => { setInvitationHistoryUser(undefined); setInvitationHistory([]); }}
        destroyOnHidden
      >
        <Alert
          className="bottom-gap"
          type="info"
          showIcon
          message={text.evidenceTitle}
          description={text.evidenceDescription}
        />
        <Table<InvitationMailAttempt>
          rowKey="id"
          loading={loadingInvitationHistory}
          dataSource={invitationHistory}
          pagination={false}
          locale={{ emptyText: text.noAttempts }}
          columns={[
            { title: text.created, render: (_, attempt) => new Date(attempt.created_at).toLocaleString(locale) },
            { title: text.recipient, dataIndex: "recipient_email" },
            {
              title: text.status,
              render: (_, attempt) => (
                <Tag color={attempt.status === "accepted" ? "green" : attempt.status === "failed" ? "red" : attempt.status === "unknown" ? "orange" : "blue"}>
                  {attempt.status === "accepted" ? text.acceptedTitle : attempt.status === "failed" ? text.failedTitle : attempt.status === "unknown" ? text.unknownTitle : text.inProgress}
                </Tag>
              ),
            },
            { title: text.result, render: (_, attempt) => attempt.error_message || (attempt.completed_at ? text.completed : text.inProgress) },
          ]}
        />
      </Modal>

	 <Modal
		open={roleUser !== undefined}
		title={roleUser ? text.changeRoleFor(roleUser.email) : text.changeRole}
		okText={text.changeRole}
		onOk={() => roleEditForm.submit()}
		onCancel={() => { setRoleUser(undefined); roleEditForm.resetFields(); }}
		destroyOnHidden
	  >
		<Alert
		  className="bottom-gap"
		  type="warning"
		  showIcon
		  message={text.accessImmediate}
		  description={text.accessDescription}
		/>
		<Form form={roleEditForm} layout="vertical" onFinish={(values) => void changeRole(values)}>
		  <Form.Item name="role_id" label={text.role} rules={[{ required: true }]}>
			<Select options={roles.filter((role) => role.status === "active").map((role) => ({ value: role.id, label: role.name }))} />
		  </Form.Item>
		</Form>
	  </Modal>
	</Space>
  );
}
