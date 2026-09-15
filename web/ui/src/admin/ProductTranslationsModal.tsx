import { useCallback, useEffect, useState } from "react";
import { Alert, Button, Card, Form, Input, Modal, Select, Spin, Tabs } from "antd";
import { api, putJSON } from "./api";
import type { Product, ProductContent, ProductTranslation, SiteSettings } from "./types";

type Props = {
  product?: Product;
  settings?: SiteSettings;
  locale: "en-US" | "zh-TW";
  onClose(): void;
  onError(error: unknown): void;
  onSaved(): void;
};

type TranslationText = {
  title: string;
  source: string;
  sourceHelp: string;
  perField: string;
  empty: string;
  name: string;
  description: string;
  features: string;
  specification: string;
  save: string;
  saveSource: string;
};

const labels = {
  "en-US": {
    title: "Product translations", source: "Default Source Locale",
    sourceHelp: "Source Locale describes the language of each authoritative value. It defaults from the Website Site Default at creation and is independent of your Admin interface language.",
    perField: "Per-field Source Locale", empty: "Translations are optional. Empty fields fall back through Site Default to each field Source Locale.",
    name: "Name", description: "Description", features: "Features", specification: "Specification",
    save: "Save translation", saveSource: "Save Source Locale metadata",
  },
  "zh-TW": {
    title: "產品翻譯", source: "預設來源語系",
    sourceHelp: "Source Locale 用來描述每個權威值的語言；建立時預設取 Website Site Default，與你的 Admin 介面語系無關。",
    perField: "逐欄來源語系", empty: "翻譯皆為選填；空白欄位會依序 fallback 至 Site Default 與該欄 Source Locale。",
    name: "名稱", description: "說明", features: "特色", specification: "規格",
    save: "儲存翻譯", saveSource: "儲存來源語系資訊",
  },
} as const;

function LocaleForm({ productID, locale, item, revision, text, onSaved, onError }: {
  productID: string; locale: string; item?: ProductTranslation; revision: number; text: TranslationText;
  onSaved(content: ProductContent): void; onError(error: unknown): void;
}) {
  const [saving, setSaving] = useState(false);
  const [form] = Form.useForm<Omit<ProductTranslation, "locale" | "revision">>();
  useEffect(() => { form.setFieldsValue(item ?? {}); }, [form, item]);
  const submit = async (values: Omit<ProductTranslation, "locale" | "revision">) => {
    setSaving(true);
    try {
      const content = await putJSON<ProductContent>(`/admin/api/products/${productID}/translations/${encodeURIComponent(locale)}`, {
        expected_revision: revision,
        translation: values,
      });
      onSaved(content);
    } catch (error) { onError(error); } finally { setSaving(false); }
  };
  return (
    <Form form={form} layout="vertical" onFinish={(values) => void submit(values)}>
      <Form.Item name="name" label={text.name}><Input /></Form.Item>
      <Form.Item name="description" label={text.description}><Input.TextArea rows={3} /></Form.Item>
      <Form.Item name="features" label={text.features}><Input.TextArea rows={3} /></Form.Item>
      <Form.Item name="specification" label={text.specification}><Input.TextArea rows={3} /></Form.Item>
      <Button type="primary" htmlType="submit" loading={saving}>{text.save}</Button>
    </Form>
  );
}

function SourceLocaleForm({ productID, content, locales, text, onSaved, onError }: {
  productID: string; content: ProductContent; locales: string[]; text: TranslationText;
  onSaved(content: ProductContent): void; onError(error: unknown): void;
}) {
  const [saving, setSaving] = useState(false);
  const [form] = Form.useForm<{ source_locale: string; source_locales: Record<string, string> }>();
  const options = locales.map((value) => ({ value, label: value }));
  useEffect(() => {
    form.setFieldsValue({ source_locale: content.source_locale, source_locales: content.source_locales });
  }, [content, form]);
  const submit = async (values: { source_locale: string; source_locales: Record<string, string> }) => {
    setSaving(true);
    try {
      const next = await putJSON<ProductContent>(`/admin/api/products/${productID}/content/source-locales`, {
        expected_revision: content.product_revision,
        source_locale: values.source_locale,
        source_locales: values.source_locales,
      });
      onSaved(next);
    } catch (error) { onError(error); } finally { setSaving(false); }
  };
  return (
    <Form form={form} layout="vertical" onFinish={(values) => void submit(values)}>
      <Alert className="bottom-gap" type="info" showIcon message={text.sourceHelp} />
      <Form.Item name="source_locale" label={text.source} rules={[{ required: true }]}><Select options={options} /></Form.Item>
      <Card size="small" title={text.perField}>
        {(["name", "description", "features", "specification"] as const).map((field) => (
          <Form.Item key={field} name={["source_locales", field]} label={text[field]} rules={[{ required: true }]}>
            <Select options={options} />
          </Form.Item>
        ))}
      </Card>
      <Button className="top-gap" type="primary" htmlType="submit" loading={saving}>{text.saveSource}</Button>
    </Form>
  );
}

export function ProductTranslationsModal({ product, settings, locale, onClose, onError, onSaved }: Props) {
  const text = labels[locale];
  const [loading, setLoading] = useState(false);
  const [content, setContent] = useState<ProductContent>();
  const load = useCallback(async () => {
    if (!product) { setContent(undefined); return; }
    setLoading(true);
    try { setContent(await api<ProductContent>(`/admin/api/products/${product.id}/content`)); }
    catch (error) { onError(error); }
    finally { setLoading(false); }
  }, [onError, product]);
  useEffect(() => { void load(); }, [load]);
  const locales = settings?.supported_locales ?? [];
  const acceptSaved = (next: ProductContent) => { setContent(next); onSaved(); };
  return (
    <Modal open={Boolean(product)} title={`${text.title} · ${product?.part_number ?? ""}`} footer={null} onCancel={onClose} width={800} destroyOnHidden>
      {loading || !content || !product ? <Spin /> : (
        <Tabs items={[
          {
            key: "source-locales", label: text.perField,
            children: <SourceLocaleForm productID={product.id} content={content} locales={locales} text={text} onError={onError} onSaved={acceptSaved} />,
          },
          ...locales.filter((language) => language !== content.source_locale).map((language) => {
            const item = content.translations?.find((row) => row.locale === language);
            return {
              key: language, label: language,
              children: <><Alert className="bottom-gap" type="info" showIcon message={`${text.source}: ${content.source_locale}`} description={text.empty} />
                <LocaleForm productID={product.id} locale={language} item={item} revision={content.product_revision} text={text} onError={onError} onSaved={acceptSaved} /></>,
            };
          }),
        ]} />
      )}
    </Modal>
  );
}
