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

export function AccessPanel({ onError, onMessage }: Feedback) {
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
      onMessage(`Created role ${values.name}.`);
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
      onMessage(`Created ${response.user.email}. Copy the one-time link now.`);
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
			onMessage(`Changed ${updated.email} to ${roles.find((role) => role.id === updated.role_id)?.name ?? updated.role_id}. Existing sessions and unused set-password links were revoked.`);
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
			onMessage(`Generated a new one-time set-password link for ${response.user.email}. Earlier unused links are invalid.`);
		} catch (error) {
			onError(error);
		}
	};

	const reactivateUser = async (user: User) => {
		try {
			const response = await postJSON<GrantResponse>(`/admin/api/users/${user.id}/reactivate`, {});
			showGrant(response);
			onMessage(`Reactivated ${response.user.email}. They must use the new one-time link before signing in.`);
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
      onMessage(`Disabled ${user.email}; existing sessions and grants were revoked.`);
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
        onMessage(`SMTP accepted the invitation for ${attempt.recipient_email}.`);
      } else if (attempt.status === "unknown") {
        onMessage(`SMTP outcome for ${attempt.recipient_email} is unknown. Generate a new password link before sending again.`);
      } else {
        onMessage(`Invitation delivery for ${attempt.recipient_email} failed. The durable result is recorded.`);
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
          message="One-time set-password link"
          description={
            <Space direction="vertical" className="panel-stack">
              <Typography.Text>
                This bearer link for {visibleGrant.user.email} is displayed only in this response. Copy it to a private channel, or explicitly send it through the configured SMTP server.
              </Typography.Text>
              <Input
                value={visibleGrant.set_password_url}
                readOnly
                addonAfter={<Button type="link" onClick={() => void navigator.clipboard.writeText(visibleGrant.set_password_url)}>Copy</Button>}
              />
              {invitationAttempt && (
                <Alert
                  showIcon
                  type={invitationAttempt.status === "accepted" ? "success" : invitationAttempt.status === "failed" ? "error" : "warning"}
                  message={invitationAttempt.status === "accepted" ? "SMTP accepted the invitation" : invitationAttempt.status === "failed" ? "Invitation delivery failed" : "Invitation outcome is unknown"}
                  description={invitationAttempt.status === "unknown"
                    ? "Do not resend this same link. Generate a new password link before another explicit send."
                    : invitationAttempt.error_message || `Durable result recorded at ${invitationAttempt.completed_at ?? invitationAttempt.created_at}.`}
                />
              )}
              <Space wrap>
                <Popconfirm
                  title="Send this bearer link by email?"
                  description="This is an explicit external SMTP send. Its accepted, failed, or unknown outcome will be recorded durably."
                  onConfirm={() => void sendInvitation()}
                >
                  <Button
                    type="primary"
                    loading={sendingInvitation}
                    disabled={invitationAttempt?.status === "accepted" || invitationAttempt?.status === "unknown"}
                  >
                    Send invitation email
                  </Button>
                </Popconfirm>
                <Button size="small" onClick={() => { setVisibleGrant(undefined); setInvitationAttempt(undefined); }}>Dismiss</Button>
              </Space>
            </Space>
          }
        />
      )}
      <Card title="New role">
        <Form form={roleForm} layout="vertical" onFinish={(values) => void createRole(values)}>
          <div className="form-grid">
            <Form.Item name="name" label="Role name" rules={[{ required: true }]}><Input /></Form.Item>
            <Form.Item name="capabilities" label="Capabilities" rules={[{ required: true }]}>
              <Select mode="multiple" options={capabilities.map((value) => ({ value, label: value }))} />
            </Form.Item>
          </div>
          <Button type="primary" htmlType="submit">Create role</Button>
        </Form>
      </Card>
      <Card title="New user">
        <Form form={userForm} layout="vertical" onFinish={(values) => void createUser(values)}>
          <div className="form-grid three-columns">
            <Form.Item name="email" label="Email" rules={[{ required: true, type: "email" }]}><Input /></Form.Item>
            <Form.Item name="display_name" label="Display name" rules={[{ required: true }]}><Input /></Form.Item>
            <Form.Item name="role_id" label="Role" rules={[{ required: true }]}>
              <Select options={roles.filter((role) => role.status === "active").map((role) => ({ value: role.id, label: role.name }))} />
            </Form.Item>
          </div>
          <Button type="primary" htmlType="submit">Create user</Button>
        </Form>
      </Card>
      <Card title="Users and roles" extra={<Button onClick={() => void load()}>Refresh</Button>}>
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
            { title: "Email", dataIndex: "email" },
            { title: "Display name", dataIndex: "display_name" },
            { title: "Role", render: (_, user) => roles.find((role) => role.id === user.role_id)?.name ?? user.role_id },
            { title: "Status", render: (_, user) => <Tag color={user.status === "active" ? "green" : "default"}>{user.status}</Tag> },
            { title: "Auth revision", dataIndex: "auth_revision" },
			{
			  title: "Actions",
			  render: (_, user) => (
				<Space wrap>
				  <Button size="small" onClick={() => beginRoleChange(user)}>Change role</Button>
				  <Button size="small" onClick={() => void showInvitationHistory(user)}>Invitation history</Button>
				  {user.status === "active" ? (
					<>
					  <Popconfirm
						title="Generate a new set-password link?"
						description="Every earlier unused set-password link for this user will become invalid."
						onConfirm={() => void issueSetPasswordGrant(user)}
					  >
						<Button size="small">New password link</Button>
					  </Popconfirm>
					  <Popconfirm title="Disable this user and revoke all sessions and grants?" onConfirm={() => void disableUser(user)}>
						<Button size="small" danger>Disable</Button>
					  </Popconfirm>
					</>
				  ) : (
					<Popconfirm
					  title="Reactivate this user?"
					  description="A new one-time set-password link will be required; the old password stays unusable."
					  onConfirm={() => void reactivateUser(user)}
					>
					  <Button size="small" type="primary">Reactivate</Button>
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
        title={invitationHistoryUser ? `Invitation history for ${invitationHistoryUser.email}` : "Invitation history"}
        footer={null}
        width={860}
        onCancel={() => { setInvitationHistoryUser(undefined); setInvitationHistory([]); }}
        destroyOnHidden
      >
        <Alert
          className="bottom-gap"
          type="info"
          showIcon
          message="Delivery evidence does not contain the bearer link"
          description="Accepted means the SMTP server accepted the message. Unknown is never retried automatically; generate a new password link before another send."
        />
        <Table<InvitationMailAttempt>
          rowKey="id"
          loading={loadingInvitationHistory}
          dataSource={invitationHistory}
          pagination={false}
          locale={{ emptyText: "No invitation email attempts recorded." }}
          columns={[
            { title: "Created", render: (_, attempt) => new Date(attempt.created_at).toLocaleString() },
            { title: "Recipient", dataIndex: "recipient_email" },
            {
              title: "Status",
              render: (_, attempt) => (
                <Tag color={attempt.status === "accepted" ? "green" : attempt.status === "failed" ? "red" : attempt.status === "unknown" ? "orange" : "blue"}>
                  {attempt.status}
                </Tag>
              ),
            },
            { title: "Result", render: (_, attempt) => attempt.error_message || (attempt.completed_at ? "Durably completed" : "In progress") },
          ]}
        />
      </Modal>

	 <Modal
		open={roleUser !== undefined}
		title={roleUser ? `Change role for ${roleUser.email}` : "Change role"}
		okText="Change role"
		onOk={() => roleEditForm.submit()}
		onCancel={() => { setRoleUser(undefined); roleEditForm.resetFields(); }}
		destroyOnHidden
	  >
		<Alert
		  className="bottom-gap"
		  type="warning"
		  showIcon
		  message="Access changes immediately"
		  description="Changing a role revokes this user's current sessions and unused set-password links. The last usable Active Owner cannot be downgraded."
		/>
		<Form form={roleEditForm} layout="vertical" onFinish={(values) => void changeRole(values)}>
		  <Form.Item name="role_id" label="Role" rules={[{ required: true }]}>
			<Select options={roles.filter((role) => role.status === "active").map((role) => ({ value: role.id, label: role.name }))} />
		  </Form.Item>
		</Form>
	  </Modal>
	</Space>
  );
}
