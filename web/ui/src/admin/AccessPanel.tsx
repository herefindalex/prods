import { useEffect, useState } from "react";
import { Alert, Button, Card, Form, Input, Modal, Popconfirm, Select, Space, Table, Tag, Typography } from "antd";
import { api, postJSON, putJSON } from "./api";
import { capabilities, type Capability, type Role, type User } from "./types";

type Feedback = { onError: (error: unknown) => void; onMessage: (message: string) => void };
type GrantResponse = { user: User; set_password_url: string; expires_at: string };

export function AccessPanel({ onError, onMessage }: Feedback) {
  const [roles, setRoles] = useState<Role[]>([]);
	const [users, setUsers] = useState<User[]>([]);
	const [setPasswordURL, setSetPasswordURL] = useState("");
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
      setSetPasswordURL(response.set_password_url);
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
			setSetPasswordURL(response.set_password_url);
			onMessage(`Generated a new one-time set-password link for ${response.user.email}. Earlier unused links are invalid.`);
		} catch (error) {
			onError(error);
		}
	};

	const reactivateUser = async (user: User) => {
		try {
			const response = await postJSON<GrantResponse>(`/admin/api/users/${user.id}/reactivate`, {});
			setSetPasswordURL(response.set_password_url);
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
      onMessage(`Disabled ${user.email}; existing sessions and grants were revoked.`);
      await load();
    } catch (error) {
      onError(error);
    }
  };

  return (
    <Space direction="vertical" size="large" className="panel-stack">
      {setPasswordURL && (
        <Alert
          type="warning"
          showIcon
          message="One-time set-password link"
          description={
            <Space direction="vertical" className="panel-stack">
              <Typography.Text>This bearer link is displayed only in this response. Share it through an appropriate private channel.</Typography.Text>
              <Input value={setPasswordURL} readOnly addonAfter={<Button type="link" onClick={() => void navigator.clipboard.writeText(setPasswordURL)}>Copy</Button>} />
              <Button size="small" onClick={() => setSetPasswordURL("")}>Dismiss</Button>
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
