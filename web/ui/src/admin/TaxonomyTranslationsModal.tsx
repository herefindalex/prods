import { useCallback, useEffect, useState } from "react";
import { Alert, Button, Card, Form, Input, Modal, Select, Spin, Tabs } from "antd";
import { api, putJSON } from "./api";
import type { SiteSettings, TaxonomyContent, TaxonomyTranslation } from "./types";

export type TaxonomyTranslationTarget = {
  type: "category" | "dictionary";
  id: string;
  name: string;
};

type Props = {
  target?: TaxonomyTranslationTarget;
  locale: "en-US" | "zh-TW";
  onClose(): void;
  onError(error: unknown): void;
  onSaved(): void;
};

const labels = {
  "en-US": { title: "Translations", source: "Default Source Locale", sourceHelp: "Source Locale follows the Website Site Default when created and is independent of Admin UI language.", perField: "Per-field Source Locale", name: "Name", description: "Description", saveSource: "Save Source Locale metadata", empty: "Empty translated values fall back to the source value.", save: "Save translation" },
  "zh-TW": { title: "翻譯", source: "預設來源語系", sourceHelp: "建立時 Source Locale 取 Website Site Default，與 Admin UI 語系無關。", perField: "逐欄來源語系", name: "名稱", description: "說明", saveSource: "儲存來源語系資訊", empty: "翻譯值留空時會 fallback 至來源值。", save: "儲存翻譯" },
} as const;

export function TaxonomyTranslationsModal({ target, locale, onClose, onError, onSaved }: Props) {
  const text = labels[locale];
  const [loading, setLoading] = useState(false);
  const [content, setContent] = useState<TaxonomyContent>();
  const [settings, setSettings] = useState<SiteSettings>();
  const [sourceForm] = Form.useForm<{ source_locale: string; source_locales: Record<string, string> }>();

  const load = useCallback(async () => {
    if (!target) { setContent(undefined); return; }
    setLoading(true);
    try {
      const [next, nextSettings] = await Promise.all([
        api<TaxonomyContent>(`/admin/api/taxonomy/${target.type}/${target.id}/content`),
        api<SiteSettings>("/admin/api/products/content-settings"),
      ]);
      setContent(next);
      setSettings(nextSettings);
      sourceForm.setFieldsValue({ source_locale: next.source_locale, source_locales: next.source_locales });
    } catch (error) { onError(error); } finally { setLoading(false); }
  }, [onError, sourceForm, target]);
  useEffect(() => { void load(); }, [load]);

  const locales = settings?.supported_locales ?? [];
  const options = locales.map((value) => ({ value, label: value }));
  const accept = (next: TaxonomyContent) => { setContent(next); onSaved(); };
  const saveSource = async (values: { source_locale: string; source_locales: Record<string, string> }) => {
    if (!target || !content) return;
    setLoading(true);
    try {
      accept(await putJSON<TaxonomyContent>(`/admin/api/taxonomy/${target.type}/${target.id}/content/source-locales`, { expected_revision: content.subject_revision, ...values }));
    } catch (error) { onError(error); } finally { setLoading(false); }
  };

  return (
    <Modal open={Boolean(target)} title={`${text.title} · ${target?.name ?? ""}`} footer={null} onCancel={onClose} width={760} destroyOnHidden>
      {loading || !content || !target ? <Spin /> : (
        <Tabs items={[
          { key: "source", label: text.perField, children: (
            <Form form={sourceForm} layout="vertical" onFinish={(values) => void saveSource(values)}>
              <Alert className="bottom-gap" type="info" showIcon message={text.sourceHelp} />
              <Form.Item name="source_locale" label={text.source} rules={[{ required: true }]}><Select options={options} /></Form.Item>
              <Card size="small" title={text.perField}>
                {(["name", "description"] as const).map((field) => <Form.Item key={field} name={["source_locales", field]} label={text[field]} rules={[{ required: true }]}><Select options={options} /></Form.Item>)}
              </Card>
              <Button type="primary" htmlType="submit" loading={loading}>{text.saveSource}</Button>
            </Form>
          ) },
          ...locales.filter((language) => language !== content.source_locale).map((language) => ({
            key: language, label: language,
            children: <TaxonomyLocaleForm target={target} content={content} locale={language} item={content.translations?.find((row) => row.locale === language)} text={text} onSaved={accept} onError={onError} />,
          })),
        ]} />
      )}
    </Modal>
  );
}

function TaxonomyLocaleForm({ target, content, locale, item, text, onSaved, onError }: {
  target: TaxonomyTranslationTarget; content: TaxonomyContent; locale: string; item?: TaxonomyTranslation;
  text: (typeof labels)["en-US"] | (typeof labels)["zh-TW"];
  onSaved(content: TaxonomyContent): void; onError(error: unknown): void;
}) {
  const [saving, setSaving] = useState(false);
  const [form] = Form.useForm<{ name?: string; description?: string }>();
  useEffect(() => { form.setFieldsValue(item ?? {}); }, [form, item]);
  const submit = async (translation: { name?: string; description?: string }) => {
    setSaving(true);
    try {
      onSaved(await putJSON<TaxonomyContent>(`/admin/api/taxonomy/${target.type}/${target.id}/translations/${encodeURIComponent(locale)}`, { expected_revision: content.subject_revision, translation }));
    } catch (error) { onError(error); } finally { setSaving(false); }
  };
  return (
    <Form form={form} layout="vertical" onFinish={(values) => void submit(values)}>
      <Alert className="bottom-gap" type="info" showIcon message={text.empty} />
      <Form.Item name="name" label={text.name}><Input /></Form.Item>
      <Form.Item name="description" label={text.description}><Input.TextArea rows={4} /></Form.Item>
      <Button type="primary" htmlType="submit" loading={saving}>{text.save}</Button>
    </Form>
  );
}
