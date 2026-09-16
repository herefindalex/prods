import { useState } from "react";
import { Alert, Button, Card, ConfigProvider, Form, Input, Select, Space, Typography } from "antd";
import { ADMIN_LOCALE_OPTIONS, antDesignLocale, normalizeAdminLocale, type AdminLocale } from "./locales";

type LoginRootData = {
  language: string;
  action: string;
  title: string;
  emailLabel: string;
  passwordLabel: string;
  tokenLabel: string;
  signIn: string;
  catalog: string;
  requestPart: string;
  footer: string;
  pocHelp: string;
  email: string;
  error: string;
  poc: boolean;
};

function readLoginData(root: HTMLElement): LoginRootData {
  const value = (name: string) => root.dataset[name] ?? "";
  return {
    language: value("language") || "en-US",
    action: value("action") || "/admin/login",
    title: value("title") || "Prods Admin",
    emailLabel: value("emailLabel") || "Email",
    passwordLabel: value("passwordLabel") || "Password",
    tokenLabel: value("tokenLabel") || "Temporary admin token",
    signIn: value("signIn") || "Sign in",
    catalog: value("catalog") || "Catalog",
    requestPart: value("requestPart") || "Request part",
    footer: value("footer"),
    pocHelp: value("pocHelp"),
    email: value("email"),
    error: value("error"),
    poc: value("poc") === "true",
  };
}

export function LoginPage({ root }: { root: HTMLElement }) {
  const data = readLoginData(root);
  const locale = normalizeAdminLocale(data.language);
  const [submitting, setSubmitting] = useState(false);
  const submit = () => {
    setSubmitting(true);
    (document.getElementById("admin-login-form") as HTMLFormElement | null)?.submit();
  };

  const switchLocale = (next: AdminLocale) => {
    const url = new URL(window.location.href);
    url.searchParams.set("lang", next);
    window.location.assign(url);
  };

  document.documentElement.lang = locale;

  return (
    <ConfigProvider
      locale={antDesignLocale(locale)}
      theme={{
        token: {
          colorPrimary: "#147985",
          borderRadius: 10,
          controlHeight: 44,
          fontSize: 15,
        },
      }}
    >
      <main className="admin-login-shell">
        <section className="admin-login-layout" aria-labelledby="admin-login-title">
          <div className="admin-login-intro">
            <div className="admin-login-mark" aria-hidden="true">P</div>
            <Typography.Title level={1}>Prods</Typography.Title>
            <Typography.Paragraph>{data.footer}</Typography.Paragraph>
            <Space size="large">
              <a href="/search">{data.catalog}</a>
              <a href="/rfq">{data.requestPart}</a>
            </Space>
          </div>
          <Card className="admin-login-card" bordered={false}>
            <Select
              className="admin-login-locale"
              aria-label="Language"
              value={locale}
              onChange={switchLocale}
              options={[...ADMIN_LOCALE_OPTIONS]}
            />
            <Typography.Text className="admin-login-eyebrow">PRODS</Typography.Text>
            <Typography.Title id="admin-login-title" level={2}>{data.title}</Typography.Title>
            <Typography.Paragraph type="secondary" className="admin-login-subtitle">
              {data.poc ? data.pocHelp : data.footer}
            </Typography.Paragraph>
            {data.error && <Alert showIcon type="error" message={data.error} role="alert" />}
            <Form
              id="admin-login-form"
              className="admin-login-form"
              layout="vertical"
              method="post"
              action={data.action}
              initialValues={{ email: data.email }}
              onFinish={submit}
              requiredMark={false}
            >
              {data.poc ? (
                <Form.Item name="token" label={data.tokenLabel} rules={[{ required: true, message: data.tokenLabel }]}>
                  <Input.Password name="token" autoComplete="one-time-code" autoFocus />
                </Form.Item>
              ) : (
                <>
                  <Form.Item
                    name="email"
                    label={data.emailLabel}
                    rules={[{ required: true, message: data.emailLabel }, { type: "email" }]}
                  >
                    <Input name="email" type="email" autoComplete="username" autoFocus inputMode="email" />
                  </Form.Item>
                  <Form.Item name="password" label={data.passwordLabel} rules={[{ required: true, message: data.passwordLabel }]}>
                    <Input.Password name="password" autoComplete="current-password" />
                  </Form.Item>
                </>
              )}
              <Button type="primary" htmlType="submit" block loading={submitting}>
                {data.signIn}
              </Button>
            </Form>
          </Card>
        </section>
      </main>
    </ConfigProvider>
  );
}
