import { useCallback, useEffect, useRef, useState } from "react";
import { Alert, Button, Card, Form, Input, Space, Typography } from "antd";
import { api, putJSON } from "./api";
import type { SiteSettings } from "./types";

type Props = {
	locale: "en-US" | "zh-TW";
	onError(error: unknown): void;
	onMessage(message: string): void;
};

const labels = {
	"en-US": {
		title: "Site settings",
		defaultLocale: "Default interface locale",
		supportedLocales: "Supported interface locales",
		timeZone: "Site time zone",
		timeZoneHelp: "Use an IANA time zone such as America/New_York, Asia/Taipei, or UTC.",
		invalidTimeZone: "Enter a valid IANA time zone.",
		save: "Save site time zone",
		saved: "Site time zone saved. Stored event timestamps remain unchanged in UTC.",
		explanation: "The site time zone controls Admin/RFQ display and the built-in backup schedule. It is independent of interface language and never rewrites stored event times.",
	},
	"zh-TW": {
		title: "站點設定",
		defaultLocale: "預設介面語系",
		supportedLocales: "支援的介面語系",
		timeZone: "站點時區",
		timeZoneHelp: "請使用 IANA 時區，例如 America/New_York、Asia/Taipei 或 UTC。",
		invalidTimeZone: "請輸入有效的 IANA 時區。",
		save: "儲存站點時區",
		saved: "站點時區已儲存；既有事件時間仍以 UTC 原值保存。",
		explanation: "站點時區用於 Admin／RFQ 顯示及內建備份排程；它與介面語言分離，也不會改寫既有事件時間。",
	},
} as const;

function validTimeZone(value: string): boolean {
	if (value === "Local") return true;
	try {
		new Intl.DateTimeFormat("en-US", { timeZone: value }).format();
		return true;
	} catch {
		return false;
	}
}

export function SettingsPanel({ locale, onError, onMessage }: Props) {
	const text = labels[locale];
	const [settings, setSettings] = useState<SiteSettings>();
	const [loading, setLoading] = useState(false);
	const [form] = Form.useForm<{ time_zone: string }>();
	const onErrorRef = useRef(onError);
	const onMessageRef = useRef(onMessage);
	onErrorRef.current = onError;
	onMessageRef.current = onMessage;

	const load = useCallback(async () => {
		setLoading(true);
		try {
			const next = await api<SiteSettings>("/admin/api/system/settings");
			setSettings(next);
			form.setFieldsValue({ time_zone: next.time_zone });
		} catch (error) {
			onErrorRef.current(error);
		} finally {
			setLoading(false);
		}
	}, [form]);

	useEffect(() => {
		void load();
	}, [load]);

	const save = async (values: { time_zone: string }) => {
		if (!settings) return;
		setLoading(true);
		try {
			const updated = await putJSON<SiteSettings>("/admin/api/system/settings", {
				expected_revision: settings.revision,
				time_zone: values.time_zone.trim(),
			});
			setSettings(updated);
			form.setFieldsValue({ time_zone: updated.time_zone });
			onMessageRef.current(text.saved);
		} catch (error) {
			onErrorRef.current(error);
		} finally {
			setLoading(false);
		}
	};

	return (
		<Card title={text.title} extra={<Button onClick={() => void load()} loading={loading}>Refresh</Button>}>
			<Space direction="vertical" size="large" className="panel-stack">
				<Alert type="info" showIcon message={text.explanation} />
				<Form form={form} layout="vertical" onFinish={(values) => void save(values)}>
					<div className="form-grid three-columns">
						<Form.Item label={text.defaultLocale}>
							<Input value={settings?.default_locale} readOnly />
						</Form.Item>
						<Form.Item label={text.supportedLocales}>
							<Input value={settings?.supported_locales.join(", ")} readOnly />
						</Form.Item>
						<Form.Item
							name="time_zone"
							label={text.timeZone}
							extra={text.timeZoneHelp}
							rules={[
								{ required: true },
								{
									validator: async (_, value: string) => {
										if (value && !validTimeZone(value.trim())) throw new Error(text.invalidTimeZone);
									},
								},
							]}
						>
							<Input placeholder="UTC" />
						</Form.Item>
					</div>
					<Button type="primary" htmlType="submit" loading={loading}>{text.save}</Button>
				</Form>
				{settings && <Typography.Text type="secondary">Revision {settings.revision} · {new Date(settings.updated_at).toLocaleString(locale)}</Typography.Text>}
			</Space>
		</Card>
	);
}
