import { useEffect, useState } from "react";
import { Alert, Button, Card, Form, Input, Popconfirm, Select, Space, Table, Tag, Typography } from "antd";
import { api, postJSON } from "./api";
import { capabilities, type Capability, type Role, type User } from "./types";

type Feedback = { onError: (error: unknown) => void; onMessage: (message: string) => void };

export function AccessPanel({ onError, onMessage }: Feedback) {
  const [roles, setRoles] = useState<Role[]>([]);
  const [users, setUsers] = useState<User[]>([]);
  const [setPasswordURL, setSetPasswordURL] = useState("");
  const [roleForm] = Form.useForm<{ name: string; capabilities: Capability[] }>();
  const [userForm] = Form.useForm<{ email: string; display_name: string; role_id: string }>();

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
      const response = await postJSON<{ user: User; set_password_url: string; expires_at: string }>("/admin/api/users", values);
      setSetPasswordURL(response.set_password_url);
      userForm.resetFields();
      onMessage(`Created ${response.user.email}. Copy the one-time link now.`);
      await load();
    } catch (error) {
      onError(error);
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
              render: (_, user) => user.status !== "active" ? null : (
                <Popconfirm title="Disable this user and revoke all sessions and grants?" onConfirm={() => void disableUser(user)}>
                  <Button size="small" danger>Disable</Button>
                </Popconfirm>
              ),
            },
          ]}
        />
      </Card>
    </Space>
  );
}
