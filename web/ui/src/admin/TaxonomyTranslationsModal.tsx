import { useCallback, useEffect, useState } from "react";
import { Alert, Button, Card, Form, Input, Modal, Select, Spin, Tabs } from "antd";
import { api, putJSON } from "./api";
import { localeLabel, localeSelectOptions, type AdminLocale } from "./locales";
import type { SiteSettings, TaxonomyContent, TaxonomyTranslation } from "./types";

export type TaxonomyTranslationTarget = {
  type: "category" | "dictionary";
  id: string;
  name: string;
};

type Props = {
  target?: TaxonomyTranslationTarget;
  locale: AdminLocale;
  canEdit: boolean;
  onClose(): void;
  onError(error: unknown): void;
  onSaved(): void;
};

const labels = {
  "en-US": { title: "Translations", source: "Default Source Locale", sourceHelp: "Source Locale follows the Website Site Default when created and is independent of Admin UI language.", perField: "Per-field Source Locale", name: "Name", description: "Description", saveSource: "Save Source Locale metadata", empty: "Empty translated values fall back to the source value.", save: "Save translation" },
  "zh-TW": { title: "翻譯", source: "預設來源語系", sourceHelp: "建立時 Source Locale 取 Website Site Default，與 Admin UI 語系無關。", perField: "逐欄來源語系", name: "名稱", description: "說明", saveSource: "儲存來源語系資訊", empty: "翻譯值留空時會 fallback 至來源值。", save: "儲存翻譯" },
"zh-CN": { title: "\u7FFB\u8BD1", source: "\u9ED8\u8BA4Source Locale", sourceHelp: "Source Locale \u5728\u521B\u5EFA\u65F6\u9075\u5FAA Website Site Default\uFF0C\u5E76\u4E14\u72EC\u7ACB\u4E8E Admin UI \u8BED\u8A00\u3002", perField: "\u6BCF\u573A Source Locale", name: "\u59D3\u540D", description: "\u63CF\u8FF0", saveSource: "\u4FDD\u5B58 Source Locale \u5143\u6570\u636E", empty: "\u7A7A\u7684\u7FFB\u8BD1\u503C\u4F1A\u56DE\u9000\u5230\u6E90\u503C\u3002", save: "\u4FDD\u5B58\u7FFB\u8BD1" },
"ja-JP": { title: "\u7FFB\u8A33", source: "\u30C7\u30D5\u30A9\u30EB\u30C8\u306ESource Locale", sourceHelp: "Source Locale \u306F\u3001\u4F5C\u6210\u6642\u306B Website Site Default \u306B\u5F93\u3044\u3001Admin UI \u8A00\u8A9E\u304B\u3089\u72EC\u7ACB\u3057\u3066\u3044\u307E\u3059\u3002", perField: "\u30D5\u30A3\u30FC\u30EB\u30C9\u3054\u3068\u306E Source Locale", name: "\u540D\u524D", description: "\u8AAC\u660E", saveSource: "Source Locale \u30E1\u30BF\u30C7\u30FC\u30BF\u3092\u4FDD\u5B58\u3059\u308B", empty: "\u7A7A\u306E\u5909\u63DB\u5024\u306F\u30BD\u30FC\u30B9\u5024\u306B\u623B\u308A\u307E\u3059\u3002", save: "\u7FFB\u8A33\u3092\u4FDD\u5B58\u3059\u308B" },
"ko-KR": { title: "\uBC88\uC5ED", source: "\uAE30\uBCF8 Source Locale", sourceHelp: "Source Locale\uB294 \uC0DD\uC131 \uC2DC Website Site Default\uB97C \uB530\uB974\uBA70 Admin UI \uC5B8\uC5B4\uC640 \uB3C5\uB9BD\uC801\uC785\uB2C8\uB2E4.", perField: "\uD544\uB4DC\uBCC4 Source Locale", name: "\uC774\uB984", description: "\uC124\uBA85", saveSource: "Source Locale \uBA54\uD0C0\uB370\uC774\uD130 \uC800\uC7A5", empty: "\uBE44\uC5B4 \uC788\uB294 \uBC88\uC5ED\uB41C \uAC12\uC740 \uC18C\uC2A4 \uAC12\uC73C\uB85C \uB300\uCCB4\uB429\uB2C8\uB2E4.", save: "\uBC88\uC5ED \uC800\uC7A5" },
"de-DE": { title: "\u00DCbersetzungen", source: "Standard Source Locale", sourceHelp: "Source Locale folgt bei der Erstellung dem Website Site Default und ist unabh\u00E4ngig von der Admin-Benutzeroberfl\u00E4chensprache.", perField: "Pro Feld Source Locale", name: "Name", description: "Beschreibung", saveSource: "Speichern Sie Source Locale-Metadaten", empty: "Leere \u00FCbersetzte Werte fallen auf den Quellwert zur\u00FCck.", save: "\u00DCbersetzung speichern" },
"fr-FR": { title: "Traductions", source: "Par d\u00E9faut Source Locale", sourceHelp: "Source Locale suit le Website Site Default lors de sa cr\u00E9ation et est ind\u00E9pendant du langage de l'interface utilisateur Admin.", perField: "Par champ Source Locale", name: "Nom", description: "Description", saveSource: "Enregistrer les m\u00E9tadonn\u00E9es Source Locale", empty: "Les valeurs traduites vides reviennent \u00E0 la valeur source.", save: "Enregistrer la traduction" },
"it-IT": { title: "Traduzioni", source: "Source Locale predefinito", sourceHelp: "Source Locale segue Website Site Default quando viene creato ed \u00E8 indipendente dal linguaggio dell'interfaccia utente Admin.", perField: "Per campo Source Locale", name: "Nome", description: "Descrizione", saveSource: "Salva i metadati Source Locale", empty: "I valori tradotti vuoti ritornano al valore di origine.", save: "Salva la traduzione" },
"es-ES": { title: "Traducciones", source: "Predeterminado Source Locale", sourceHelp: "Source Locale sigue a Website Site Default cuando se crea y es independiente del lenguaje de interfaz de usuario de Admin.", perField: "Por campo Source Locale", name: "Nombre", description: "Descripci\u00F3n", saveSource: "Guardar metadatos Source Locale", empty: "Los valores traducidos vac\u00EDos vuelven al valor de origen.", save: "Guardar traducci\u00F3n" },
"pt-BR": { title: "Tradu\u00E7\u00F5es", source: "Source Locale padr\u00E3o", sourceHelp: "Source Locale segue o Website Site Default quando criado e \u00E9 independente da linguagem da interface do usu\u00E1rio Admin.", perField: "Source Locale por campo", name: "Nome", description: "Descri\u00E7\u00E3o", saveSource: "Salvar metadados Source Locale", empty: "Os valores traduzidos vazios retornam ao valor de origem.", save: "Salvar tradu\u00E7\u00E3o" },
} as const;

export function TaxonomyTranslationsModal({ target, locale, canEdit, onClose, onError, onSaved }: Props) {
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
  const options = localeSelectOptions(locales);
  const accept = (next: TaxonomyContent) => { setContent(next); onSaved(); };
  const saveSource = async (values: { source_locale: string; source_locales: Record<string, string> }) => {
    if (!canEdit || !target || !content) return;
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
              <Form.Item name="source_locale" label={text.source} rules={[{ required: true }]}><Select disabled={!canEdit} options={options} /></Form.Item>
              <Card size="small" title={text.perField}>
                {(["name", "description"] as const).map((field) => <Form.Item key={field} name={["source_locales", field]} label={text[field]} rules={[{ required: true }]}><Select disabled={!canEdit} options={options} /></Form.Item>)}
              </Card>
              {canEdit ? <Button type="primary" htmlType="submit" loading={loading}>{text.saveSource}</Button> : null}
            </Form>
          ) },
          ...locales.filter((language) => language !== content.source_locale).map((language) => ({
            key: language, label: localeLabel(language),
            children: <TaxonomyLocaleForm target={target} content={content} locale={language} item={content.translations?.find((row) => row.locale === language)} text={text} canEdit={canEdit} onSaved={accept} onError={onError} />,
          })),
        ]} />
      )}
    </Modal>
  );
}

function TaxonomyLocaleForm({ target, content, locale, item, text, canEdit, onSaved, onError }: {
  target: TaxonomyTranslationTarget; content: TaxonomyContent; locale: string; item?: TaxonomyTranslation;
  text: (typeof labels)[AdminLocale];
  canEdit: boolean;
  onSaved(content: TaxonomyContent): void; onError(error: unknown): void;
}) {
  const [saving, setSaving] = useState(false);
  const [form] = Form.useForm<{ name?: string; description?: string }>();
  useEffect(() => { form.setFieldsValue(item ?? {}); }, [form, item]);
  const submit = async (translation: { name?: string; description?: string }) => {
    if (!canEdit) return;
    setSaving(true);
    try {
      onSaved(await putJSON<TaxonomyContent>(`/admin/api/taxonomy/${target.type}/${target.id}/translations/${encodeURIComponent(locale)}`, { expected_revision: content.subject_revision, translation }));
    } catch (error) { onError(error); } finally { setSaving(false); }
  };
  return (
    <Form form={form} layout="vertical" onFinish={(values) => void submit(values)}>
      <Alert className="bottom-gap" type="info" showIcon message={text.empty} />
      <Form.Item name="name" label={text.name}><Input disabled={!canEdit} /></Form.Item>
      <Form.Item name="description" label={text.description}><Input.TextArea disabled={!canEdit} rows={4} /></Form.Item>
      {canEdit ? <Button type="primary" htmlType="submit" loading={saving}>{text.save}</Button> : null}
    </Form>
  );
}
