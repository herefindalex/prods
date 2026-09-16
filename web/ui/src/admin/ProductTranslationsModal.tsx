import { useCallback, useEffect, useState } from "react";
import { Alert, Button, Card, Form, Input, Modal, Select, Spin, Tabs } from "antd";
import { api, putJSON } from "./api";
import { localeLabel, localeSelectOptions, type AdminLocale } from "./locales";
import type { Product, ProductContent, ProductTranslation, SiteSettings } from "./types";

type Props = {
  product?: Product;
  settings?: SiteSettings;
  locale: AdminLocale;
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
"zh-CN": {
    title: "Product \u7FFB\u8BD1", source: "\u9ED8\u8BA4Source Locale",
    sourceHelp: "Source Locale\u63CF\u8FF0\u4E86\u6BCF\u4E2A\u6743\u5A01\u503C\u7684\u8BED\u8A00\u3002\u5B83\u5728\u521B\u5EFA\u65F6\u9ED8\u8BA4\u4E3A Website Site Default\uFF0C\u5E76\u4E14\u72EC\u7ACB\u4E8E\u60A8\u7684 Admin \u754C\u9762\u8BED\u8A00\u3002",
    perField: "\u6BCF\u573A Source Locale", empty: "\u7FFB\u8BD1\u662F\u53EF\u9009\u7684\u3002\u7A7A\u5B57\u6BB5\u901A\u8FC7 Site Default \u56DE\u9000\u5230\u6BCF\u4E2A\u5B57\u6BB5 Source Locale\u3002",
    name: "\u59D3\u540D", description: "\u63CF\u8FF0", features: "\u7279\u5F81", specification: "\u89C4\u683C",
    save: "\u4FDD\u5B58\u7FFB\u8BD1", saveSource: "\u4FDD\u5B58 Source Locale \u5143\u6570\u636E",
},
"ja-JP": {
    title: "Product \u7FFB\u8A33", source: "\u30C7\u30D5\u30A9\u30EB\u30C8\u306ESource Locale",
    sourceHelp: "Source Locale \u306F\u3001\u5404\u6A29\u9650\u306E\u3042\u308B\u5024\u306E\u8A00\u8A9E\u3092\u8AAC\u660E\u3057\u307E\u3059\u3002\u3053\u308C\u306F\u4F5C\u6210\u6642\u306E Website Site Default \u304B\u3089\u30C7\u30D5\u30A9\u30EB\u30C8\u3068\u306A\u308A\u3001Admin \u30A4\u30F3\u30BF\u30FC\u30D5\u30A7\u30A4\u30B9\u8A00\u8A9E\u306B\u306F\u4F9D\u5B58\u3057\u307E\u305B\u3093\u3002",
    perField: "\u30D5\u30A3\u30FC\u30EB\u30C9\u3054\u3068\u306E Source Locale", empty: "\u7FFB\u8A33\u306F\u30AA\u30D7\u30B7\u30E7\u30F3\u3067\u3059\u3002\u7A7A\u306E\u30D5\u30A3\u30FC\u30EB\u30C9\u306F\u3001Site Default \u3092\u4ECB\u3057\u3066\u5404\u30D5\u30A3\u30FC\u30EB\u30C9 Source Locale \u306B\u30D5\u30A9\u30FC\u30EB\u30D0\u30C3\u30AF\u3057\u307E\u3059\u3002",
    name: "\u540D\u524D", description: "\u8AAC\u660E", features: "\u7279\u5FB4", specification: "\u4ED5\u69D8",
    save: "\u7FFB\u8A33\u3092\u4FDD\u5B58\u3059\u308B", saveSource: "Source Locale \u30E1\u30BF\u30C7\u30FC\u30BF\u3092\u4FDD\u5B58\u3059\u308B",
},
"ko-KR": {
    title: "Product \uBC88\uC5ED", source: "\uAE30\uBCF8 Source Locale",
    sourceHelp: "Source Locale\uB294 \uAC01 \uAD8C\uC704 \uC788\uB294 \uAC12\uC758 \uC5B8\uC5B4\uB97C \uC124\uBA85\uD569\uB2C8\uB2E4. \uC0DD\uC131 \uC2DC Website Site Default\uC758 \uAE30\uBCF8\uAC12\uC774\uBA70 Admin \uC778\uD130\uD398\uC774\uC2A4 \uC5B8\uC5B4\uC640 \uB3C5\uB9BD\uC801\uC785\uB2C8\uB2E4.",
    perField: "\uD544\uB4DC\uBCC4 Source Locale", empty: "\uBC88\uC5ED\uC740 \uC120\uD0DD\uC0AC\uD56D\uC785\uB2C8\uB2E4. \uBE48 \uD544\uB4DC\uB294 Site Default\uB97C \uD1B5\uD574 \uAC01 \uD544\uB4DC Source Locale\uB85C \uB300\uCCB4\uB429\uB2C8\uB2E4.",
    name: "\uC774\uB984", description: "\uC124\uBA85", features: "\uD2B9\uC9D5", specification: "\uC0AC\uC591",
    save: "\uBC88\uC5ED \uC800\uC7A5", saveSource: "Source Locale \uBA54\uD0C0\uB370\uC774\uD130 \uC800\uC7A5",
},
"de-DE": {
    title: "Product-\u00DCbersetzungen", source: "Standard Source Locale",
    sourceHelp: "Source Locale beschreibt die Sprache jedes ma\u00DFgeblichen Werts. Es wird bei der Erstellung standardm\u00E4\u00DFig vom Website Site Default \u00FCbernommen und ist unabh\u00E4ngig von der Sprache Ihrer Admin-Benutzeroberfl\u00E4che.",
    perField: "Pro Feld Source Locale", empty: "\u00DCbersetzungen sind optional. Leere Felder fallen \u00FCber Site Default auf jedes Feld Source Locale zur\u00FCck.",
    name: "Name", description: "Beschreibung", features: "Merkmale", specification: "Spezifikation",
    save: "\u00DCbersetzung speichern", saveSource: "Speichern Sie Source Locale-Metadaten",
},
"fr-FR": {
    title: "Product traductions", source: "Par d\u00E9faut Source Locale",
    sourceHelp: "Source Locale d\u00E9crit la langue de chaque valeur faisant autorit\u00E9. Il est d\u00E9fini par d\u00E9faut sur le Website Site Default lors de la cr\u00E9ation et est ind\u00E9pendant de la langue de votre interface Admin.",
    perField: "Par champ Source Locale", empty: "Les traductions sont facultatives. Les champs vides passent par Site Default vers chaque champ Source Locale.",
    name: "Nom", description: "Description", features: "Caract\u00E9ristiques", specification: "Sp\u00E9cification",
    save: "Enregistrer la traduction", saveSource: "Enregistrer les m\u00E9tadonn\u00E9es Source Locale",
},
"it-IT": {
    title: "Traduzioni Product", source: "Source Locale predefinito",
    sourceHelp: "Source Locale descrive la lingua di ciascun valore autorevole. Il valore predefinito \u00E8 Website Site Default al momento della creazione ed \u00E8 indipendente dalla lingua dell'interfaccia Admin.",
    perField: "Per campo Source Locale", empty: "Le traduzioni sono facoltative. I campi vuoti ricadono attraverso Site Default su ciascun campo Source Locale.",
    name: "Nome", description: "Descrizione", features: "Caratteristiche", specification: "Specifica",
    save: "Salva la traduzione", saveSource: "Salva i metadati Source Locale",
},
"es-ES": {
    title: "Product traducciones", source: "Predeterminado Source Locale",
    sourceHelp: "Source Locale describe el lenguaje de cada valor autorizado. Su valor predeterminado es Website Site Default en el momento de la creaci\u00F3n y es independiente del idioma de su interfaz Admin.",
    perField: "Por campo Source Locale", empty: "Las traducciones son opcionales. Los campos vac\u00EDos retroceden a trav\u00E9s de Site Default a cada campo Source Locale.",
    name: "Nombre", description: "Descripci\u00F3n", features: "Caracter\u00EDsticas", specification: "Especificaci\u00F3n",
    save: "Guardar traducci\u00F3n", saveSource: "Guardar metadatos Source Locale",
},
"pt-BR": {
    title: "Tradu\u00E7\u00F5es Product", source: "Source Locale padr\u00E3o",
    sourceHelp: "Source Locale descreve o idioma de cada valor oficial. O padr\u00E3o \u00E9 Website Site Default na cria\u00E7\u00E3o e \u00E9 independente do idioma da interface Admin.",
    perField: "Source Locale por campo", empty: "As tradu\u00E7\u00F5es s\u00E3o opcionais. Os campos vazios retornam atrav\u00E9s de Site Default para cada campo Source Locale.",
    name: "Nome", description: "Descri\u00E7\u00E3o", features: "Caracter\u00EDsticas", specification: "Especifica\u00E7\u00E3o",
    save: "Salvar tradu\u00E7\u00E3o", saveSource: "Salvar metadados Source Locale",
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
  const options = localeSelectOptions(locales);
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
              key: language, label: localeLabel(language),
              children: <><Alert className="bottom-gap" type="info" showIcon message={`${text.source}: ${localeLabel(content.source_locale)}`} description={text.empty} />
                <LocaleForm productID={product.id} locale={language} item={item} revision={content.product_revision} text={text} onError={onError} onSaved={acceptSaved} /></>,
            };
          }),
        ]} />
      )}
    </Modal>
  );
}
