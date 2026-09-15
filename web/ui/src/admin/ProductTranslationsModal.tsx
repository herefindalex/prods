import { useCallback, useEffect, useState } from "react";
import { Alert, Button, Form, Input, Modal, Spin, Tabs } from "antd";
import { api, putJSON } from "./api";
import type {
  Product,
  ProductContent,
  ProductTranslation,
  SiteSettings,
} from "./types";

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
  empty: string;
  name: string;
  description: string;
  features: string;
  specification: string;
  save: string;
};
const labels = {
  "en-US": {
    title: "Product translations",
    source: "Source locale",
    empty:
      "Translations are optional. Empty fields fall back to source content.",
    name: "Name",
    description: "Description",
    features: "Features",
    specification: "Specification",
    save: "Save translation",
  },
  "zh-TW": {
    title: "產品翻譯",
    source: "來源語系",
    empty: "翻譯皆為選填；空白欄位會 fallback 至來源內容。",
    name: "名稱",
    description: "Description",
    features: "Features",
    specification: "Specification",
    save: "儲存翻譯",
  },
} as const;

function LocaleForm({
  productID,
  locale,
  item,
  revision,
  text,
  onSaved,
  onError,
}: {
  productID: string;
  locale: string;
  item?: ProductTranslation;
  revision: number;
  text: TranslationText;
  onSaved(content: ProductContent): void;
  onError(error: unknown): void;
}) {
  const [saving, setSaving] = useState(false);
  const [form] =
    Form.useForm<Omit<ProductTranslation, "locale" | "revision">>();
  useEffect(() => {
    form.setFieldsValue(item ?? {});
  }, [form, item]);
  const submit = async (
    values: Omit<ProductTranslation, "locale" | "revision">,
  ) => {
    setSaving(true);
    try {
      const content = await putJSON<ProductContent>(
        `/admin/api/products/${productID}/translations/${encodeURIComponent(locale)}`,
        { expected_revision: revision, translation: values },
      );
      onSaved(content);
    } catch (error) {
      onError(error);
    } finally {
      setSaving(false);
    }
  };
  return (
    <Form form={form} layout="vertical" onFinish={(v) => void submit(v)}>
      <Form.Item name="name" label={text.name}>
        <Input />
      </Form.Item>
      <Form.Item name="description" label={text.description}>
        <Input.TextArea rows={3} />
      </Form.Item>
      <Form.Item name="features" label={text.features}>
        <Input.TextArea rows={3} />
      </Form.Item>
      <Form.Item name="specification" label={text.specification}>
        <Input.TextArea rows={3} />
      </Form.Item>
      <Button type="primary" htmlType="submit" loading={saving}>
        {text.save}
      </Button>
    </Form>
  );
}

export function ProductTranslationsModal({
  product,
  settings,
  locale,
  onClose,
  onError,
  onSaved,
}: Props) {
  const text = labels[locale];
  const [content, setContent] = useState<ProductContent>();
  const [loading, setLoading] = useState(false);
  const load = useCallback(async () => {
    if (!product) return;
    setLoading(true);
    try {
      setContent(
        await api<ProductContent>(`/admin/api/products/${product.id}/content`),
      );
    } catch (error) {
      onError(error);
    } finally {
      setLoading(false);
    }
  }, [onError, product]);
  useEffect(() => {
    void load();
  }, [load]);
  const locales = (settings?.supported_locales ?? []).filter(
    (item) => item !== content?.source_locale,
  );
  return (
    <Modal
      open={Boolean(product)}
      title={`${text.title} · ${product?.part_number ?? ""}`}
      footer={null}
      onCancel={onClose}
      width={760}
      destroyOnHidden
    >
      {loading || !content || !product ? (
        <Spin />
      ) : (
        <>
          <Alert
            className="bottom-gap"
            type="info"
            showIcon
            message={`${text.source}: ${content.source_locale}`}
            description={text.empty}
          />
          <Tabs
            items={locales.map((language) => {
              const item = content.translations?.find(
                (row) => row.locale === language,
              );
              return {
                key: language,
                label: language,
                children: (
                  <LocaleForm
                    productID={product.id}
                    locale={language}
                    item={item}
                    revision={content.product_revision}
                    text={text}
                    onError={onError}
                    onSaved={(next) => {
                      setContent(next);
                      onSaved();
                    }}
                  />
                ),
              };
            })}
          />
        </>
      )}
    </Modal>
  );
}
