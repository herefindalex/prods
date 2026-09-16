import { useEffect, useMemo, useState } from "react";
import { Alert, Button, Card, Form, Select, Space, Spin, Typography } from "antd";
import { api, putJSON } from "./api";
import type { AdminLocale } from "./locales";
import type {
  Category,
  CategoryListingProfile,
  SiteConfiguration,
  SpecDefinition,
  SpecSet,
  WebsiteState,
} from "./types";

type Props = {
  locale: AdminLocale;
  onError: (error: unknown) => void;
  onMessage: (message: string) => void;
};

const labels = {
  "en-US": {
    title: "Category listing profiles",
    help: "Choose the columns, default sort, and phone key specifications for each Category. Saving changes only updates the Website working revision; Preview and Publish in Website are still required.",
    category: "Category",
    visible: "Visible columns",
    sort: "Default sort",
    direction: "Direction",
    mobile: "Phone key specifications",
    save: "Save working profile",
    remove: "Use inherited defaults",
    saved: (revision: number) => `Saved Website working revision ${revision}. The public listing is unchanged until Website Publish.`,
    removed: (revision: number) => `Removed the Category override in working revision ${revision}. Publish Website to activate it.`,
    noSpecSet: "This Category has no active SpecSet. Only base columns can be configured.",
    requiredPart: "Part number must remain visible.",
    asc: "Ascending",
    desc: "Descending",
  },
  "zh-TW": {
    title: "分類列表設定檔",
    help: "設定各分類的欄位、預設排序與手機重點規格。儲存只會更新 Website 工作修訂；仍須到 Website 預覽並發布才會公開。",
    category: "分類",
    visible: "顯示欄位",
    sort: "預設排序",
    direction: "方向",
    mobile: "手機重點規格",
    save: "儲存工作設定檔",
    remove: "使用繼承預設值",
    saved: (revision: number) => `已儲存 Website 工作修訂 ${revision}；公開列表要等 Website 發布後才會變更。`,
    removed: (revision: number) => `已在工作修訂 ${revision} 移除分類覆寫；請發布 Website 以啟用。`,
    noSpecSet: "此分類沒有啟用中的 SpecSet，只能設定基本欄位。",
    requiredPart: "料號欄位必須保留。",
    asc: "遞增",
    desc: "遞減",
  },
"zh-CN": {
    title: "\u7C7B\u522B\u5217\u8868\u914D\u7F6E\u6587\u4EF6",
    help: "\u9009\u62E9\u6BCF\u4E2A\u7C7B\u522B\u7684\u5217\u3001\u9ED8\u8BA4\u6392\u5E8F\u548C\u7535\u8BDD\u952E\u89C4\u8303\u3002\u4FDD\u5B58\u66F4\u6539\u4EC5\u66F4\u65B0 Website \u5DE5\u4F5C\u4FEE\u8BA2\u7248\uFF1B Website \u4E2D\u7684 Preview \u548C Publish \u4ECD\u7136\u662F\u5FC5\u9700\u7684\u3002",
    category: "\u7C7B\u522B",
    visible: "\u53EF\u89C1\u5217",
    sort: "\u9ED8\u8BA4\u6392\u5E8F",
    direction: "\u65B9\u5411",
    mobile: "\u7535\u8BDD\u6309\u952E\u89C4\u683C",
    save: "\u4FDD\u5B58\u5DE5\u4F5C\u8D44\u6599",
    remove: "\u4F7F\u7528\u7EE7\u627F\u7684\u9ED8\u8BA4\u503C",
    saved: (revision: number) => `\u5DF2\u4FDD\u5B58 Website \u5DE5\u4F5C\u4FEE\u8BA2\u7248${revision}\u3002\u516C\u5F00\u4E0A\u5E02\u76F4\u5230Website Publish \u4E3A\u6B62\u6CA1\u6709\u53D8\u5316\u3002`,
    removed: (revision: number) => `\u5220\u9664\u4E86\u5DE5\u4F5C\u4FEE\u8BA2\u7248\u4E2D\u7684\u7C7B\u522B\u8986\u76D6${revision}\u3002 Publish Website \u6FC0\u6D3B\u5B83\u3002`,
    noSpecSet: "\u6B64\u7C7B\u522B\u6CA1\u6709\u6D3B\u52A8\u7684\u89C4\u683C\u96C6\u3002\u53EA\u80FD\u914D\u7F6E\u57FA\u7840\u5217\u3002",
    requiredPart: "\u96F6\u4EF6\u53F7\u5FC5\u987B\u4FDD\u6301\u53EF\u89C1\u3002",
    asc: "\u5347\u5E8F",
    desc: "\u964D\u5E8F",
},
"ja-JP": {
    title: "\u30AB\u30C6\u30B4\u30EA\u30EA\u30B9\u30C8\u306E\u30D7\u30ED\u30D5\u30A3\u30FC\u30EB",
    help: "\u30AB\u30C6\u30B4\u30EA\u3054\u3068\u306B\u5217\u3001\u30C7\u30D5\u30A9\u30EB\u30C8\u306E\u4E26\u3079\u66FF\u3048\u3001\u304A\u3088\u3073\u96FB\u8A71\u30AD\u30FC\u306E\u4ED5\u69D8\u3092\u9078\u629E\u3057\u307E\u3059\u3002\u5909\u66F4\u3092\u4FDD\u5B58\u3059\u308B\u3068\u3001Website \u4F5C\u696D\u30EA\u30D3\u30B8\u30E7\u30F3\u306E\u307F\u304C\u66F4\u65B0\u3055\u308C\u307E\u3059\u3002 Website \u306E Preview \u304A\u3088\u3073 Publish \u306F\u5F15\u304D\u7D9A\u304D\u5FC5\u8981\u3067\u3059\u3002",
    category: "\u30AB\u30C6\u30B4\u30EA",
    visible: "\u8868\u793A\u3055\u308C\u308B\u5217",
    sort: "\u30C7\u30D5\u30A9\u30EB\u30C8\u306E\u4E26\u3079\u66FF\u3048",
    direction: "\u65B9\u5411",
    mobile: "\u96FB\u8A71\u30AD\u30FC\u306E\u4ED5\u69D8",
    save: "\u4F5C\u696D\u30D7\u30ED\u30D5\u30A1\u30A4\u30EB\u3092\u4FDD\u5B58\u3059\u308B",
    remove: "\u7D99\u627F\u3055\u308C\u305F\u30C7\u30D5\u30A9\u30EB\u30C8\u3092\u4F7F\u7528\u3059\u308B",
    saved: (revision: number) => `\u4FDD\u5B58\u3055\u308C\u305F Website \u4F5C\u696D\u30EA\u30D3\u30B8\u30E7\u30F3${revision}\u3002\u516C\u958B\u30EA\u30B9\u30C8\u306F\u3001Website Publish \u307E\u3067\u5909\u66F4\u3055\u308C\u307E\u305B\u3093\u3002`,
    removed: (revision: number) => `\u4F5C\u696D\u30EA\u30D3\u30B8\u30E7\u30F3\u3067\u30AB\u30C6\u30B4\u30EA\u306E\u4E0A\u66F8\u304D\u3092\u524A\u9664\u3057\u307E\u3057\u305F${revision}\u3002 Publish Website \u3092\u62BC\u3057\u3066\u6709\u52B9\u306B\u3057\u307E\u3059\u3002`,
    noSpecSet: "\u3053\u306E\u30AB\u30C6\u30B4\u30EA\u306B\u306F\u30A2\u30AF\u30C6\u30A3\u30D6\u306A SpecSet \u304C\u3042\u308A\u307E\u305B\u3093\u3002\u69CB\u6210\u3067\u304D\u308B\u306E\u306F\u30D9\u30FC\u30B9\u5217\u306E\u307F\u3067\u3059\u3002",
    requiredPart: "\u90E8\u54C1\u756A\u53F7\u306F\u8868\u793A\u3055\u308C\u305F\u307E\u307E\u306B\u3057\u3066\u304A\u304F\u5FC5\u8981\u304C\u3042\u308A\u307E\u3059\u3002",
    asc: "\u4E0A\u6607",
    desc: "\u964D\u9806",
},
"ko-KR": {
    title: "\uCE74\uD14C\uACE0\uB9AC \uBAA9\uB85D \uD504\uB85C\uD544",
    help: "\uAC01 \uBC94\uC8FC\uC5D0 \uB300\uD55C \uC5F4, \uAE30\uBCF8 \uC815\uB82C \uBC0F \uC804\uD654 \uD0A4 \uC0AC\uC591\uC744 \uC120\uD0DD\uD569\uB2C8\uB2E4. \uBCC0\uACBD \uC0AC\uD56D\uC744 \uC800\uC7A5\uD558\uBA74 Website \uC791\uC5C5 \uAC1C\uC815\uB9CC \uC5C5\uB370\uC774\uD2B8\uB429\uB2C8\uB2E4. Website\uC758 Preview \uBC0F Publish\uB294 \uC5EC\uC804\uD788 \uD544\uC694\uD569\uB2C8\uB2E4.",
    category: "\uBC94\uC8FC",
    visible: "\uBCF4\uC774\uB294 \uC5F4",
    sort: "\uAE30\uBCF8 \uC815\uB82C",
    direction: "\uBC29\uD5A5",
    mobile: "\uC804\uD654 \uD0A4 \uC0AC\uC591",
    save: "\uC791\uC5C5 \uD504\uB85C\uD544 \uC800\uC7A5",
    remove: "\uC0C1\uC18D\uB41C \uAE30\uBCF8\uAC12 \uC0AC\uC6A9",
    saved: (revision: number) => `Website \uC791\uC5C5 \uAC1C\uC815\uD310\uC744 \uC800\uC7A5\uD588\uC2B5\uB2C8\uB2E4.${revision}. \uACF5\uAC1C \uBAA9\uB85D\uC740 Website Publish\uAE4C\uC9C0 \uBCC0\uACBD\uB418\uC9C0 \uC54A\uC2B5\uB2C8\uB2E4.`,
    removed: (revision: number) => `\uC791\uC5C5 \uAC1C\uC815\uC5D0\uC11C \uCE74\uD14C\uACE0\uB9AC \uC7AC\uC815\uC758\uB97C \uC81C\uAC70\uD588\uC2B5\uB2C8\uB2E4.${revision}. Publish Website\uB97C \uD65C\uC131\uD654\uD569\uB2C8\uB2E4.`,
    noSpecSet: "\uC774 \uCE74\uD14C\uACE0\uB9AC\uC5D0\uB294 \uD65C\uC131 SpecSet\uC774 \uC5C6\uC2B5\uB2C8\uB2E4. \uAE30\uBCF8 \uC5F4\uB9CC \uAD6C\uC131\uD560 \uC218 \uC788\uC2B5\uB2C8\uB2E4.",
    requiredPart: "\uBD80\uD488 \uBC88\uD638\uB294 \uACC4\uC18D \uD45C\uC2DC\uB418\uC5B4\uC57C \uD569\uB2C8\uB2E4.",
    asc: "\uC624\uB984\uCC28\uC21C",
    desc: "\uB0B4\uB9BC\uCC28\uC21C",
},
"de-DE": {
    title: "Kategorielistenprofile",
    help: "W\u00E4hlen Sie die Spalten, die Standardsortierung und die Telefontastenspezifikationen f\u00FCr jede Kategorie aus. Durch das Speichern von \u00C4nderungen wird nur die Arbeitsrevision Website aktualisiert. Preview und Publish in Website sind weiterhin erforderlich.",
    category: "Kategorie",
    visible: "Sichtbare Spalten",
    sort: "Standardsortierung",
    direction: "Richtung",
    mobile: "Spezifikationen der Telefontasten",
    save: "Arbeitsprofil speichern",
    remove: "Geerbte Standardwerte verwenden",
    saved: (revision: number) => `Die funktionierende Website-Revision wurde gespeichert${revision}. Die \u00F6ffentliche Notierung bleibt bis Website Publish unver\u00E4ndert.`,
    removed: (revision: number) => `Die Kategorie\u00FCberschreibung wurde in der Arbeitsrevision entfernt${revision}. Publish Website, um es zu aktivieren.`,
    noSpecSet: "Diese Kategorie hat kein aktives SpecSet. Es k\u00F6nnen nur Basisspalten konfiguriert werden.",
    requiredPart: "Teilenummer muss sichtbar bleiben.",
    asc: "Aufsteigend",
    desc: "Absteigend",
},
"fr-FR": {
    title: "Profils de liste de cat\u00E9gories",
    help: "Choisissez les colonnes, le tri par d\u00E9faut et les sp\u00E9cifications des touches de t\u00E9l\u00E9phone pour chaque cat\u00E9gorie. L'enregistrement des modifications met uniquement \u00E0 jour la r\u00E9vision de travail Website\u00A0; Preview et Publish dans Website sont toujours requis.",
    category: "Cat\u00E9gorie",
    visible: "Colonnes visibles",
    sort: "Tri par d\u00E9faut",
    direction: "Direction",
    mobile: "Sp\u00E9cifications de la cl\u00E9 du t\u00E9l\u00E9phone",
    save: "Enregistrer le profil de travail",
    remove: "Utiliser les valeurs par d\u00E9faut h\u00E9rit\u00E9es",
    saved: (revision: number) => `R\u00E9vision de travail Website enregistr\u00E9e${revision}. La cotation publique reste inchang\u00E9e jusqu'\u00E0 Website Publish.`,
    removed: (revision: number) => `Suppression du remplacement de cat\u00E9gorie dans la r\u00E9vision de travail${revision}. Publish Website pour l'activer.`,
    noSpecSet: "Cette cat\u00E9gorie n'a pas de SpecSet actif. Seules les colonnes de base peuvent \u00EAtre configur\u00E9es.",
    requiredPart: "Le num\u00E9ro de pi\u00E8ce doit rester visible.",
    asc: "Ascendant",
    desc: "Descendant",
},
"it-IT": {
    title: "Profili di elenco delle categorie",
    help: "Scegli le colonne, l'ordinamento predefinito e le specifiche dei tasti del telefono per ciascuna categoria. Il salvataggio delle modifiche aggiorna solo la revisione operativa Website; Preview e Publish in Website sono ancora necessari.",
    category: "Categoria",
    visible: "Colonne visibili",
    sort: "Ordinamento predefinito",
    direction: "Direzione",
    mobile: "Specifiche dei tasti del telefono",
    save: "Salva profilo di lavoro",
    remove: "Utilizza le impostazioni predefinite ereditate",
    saved: (revision: number) => `Revisione funzionante Website salvata${revision}. La quotazione pubblica rimane invariata fino a Website Publish.`,
    removed: (revision: number) => `Rimosso l'override della categoria nella revisione funzionante${revision}. Publish Website per attivarlo.`,
    noSpecSet: "Questa categoria non ha uno SpecSet attivo. \u00C8 possibile configurare solo le colonne di base.",
    requiredPart: "Il numero di parte deve rimanere visibile.",
    asc: "Ascendente",
    desc: "Discendente",
},
"es-ES": {
    title: "Perfiles de listado de categor\u00EDas",
    help: "Elija las columnas, el orden predeterminado y las especificaciones de las teclas del tel\u00E9fono para cada categor\u00EDa. Al guardar los cambios solo se actualiza la revisi\u00F3n de trabajo Website; A\u00FAn se requieren Preview y Publish en Website.",
    category: "Categor\u00EDa",
    visible: "Columnas visibles",
    sort: "Orden predeterminado",
    direction: "Direcci\u00F3n",
    mobile: "Especificaciones de las teclas del tel\u00E9fono",
    save: "Guardar perfil de trabajo",
    remove: "Usar valores predeterminados heredados",
    saved: (revision: number) => `Revisi\u00F3n de trabajo Website guardada${revision}. La cotizaci\u00F3n p\u00FAblica no cambia hasta Website Publish.`,
    removed: (revision: number) => `Se elimin\u00F3 la anulaci\u00F3n de categor\u00EDa en la revisi\u00F3n de trabajo.${revision}. Publish Website para activarlo.`,
    noSpecSet: "Esta categor\u00EDa no tiene ning\u00FAn SpecSet activo. S\u00F3lo se pueden configurar columnas base.",
    requiredPart: "El n\u00FAmero de pieza debe permanecer visible.",
    asc: "Ascendente",
    desc: "Descendente",
},
"pt-BR": {
    title: "Perfis de listagem de categorias",
    help: "Escolha as colunas, a classifica\u00E7\u00E3o padr\u00E3o e as especifica\u00E7\u00F5es da tecla telef\u00F4nica para cada categoria. Salvar altera\u00E7\u00F5es apenas atualiza a revis\u00E3o de trabalho Website; Preview e Publish em Website ainda s\u00E3o necess\u00E1rios.",
    category: "Categoria",
    visible: "Colunas vis\u00EDveis",
    sort: "Classifica\u00E7\u00E3o padr\u00E3o",
    direction: "Dire\u00E7\u00E3o",
    mobile: "Especifica\u00E7\u00F5es da tecla do telefone",
    save: "Salvar perfil de trabalho",
    remove: "Usar padr\u00F5es herdados",
    saved: (revision: number) => `Revis\u00E3o de trabalho Website salva${revision}. A listagem p\u00FAblica permanece inalterada at\u00E9 Website Publish.`,
    removed: (revision: number) => `Removida a substitui\u00E7\u00E3o de categoria na revis\u00E3o de trabalho${revision}. Publish Website para ativ\u00E1-lo.`,
    noSpecSet: "Esta categoria n\u00E3o possui nenhum SpecSet ativo. Somente colunas base podem ser configuradas.",
    requiredPart: "O n\u00FAmero da pe\u00E7a deve permanecer vis\u00EDvel.",
    asc: "Ascendente",
    desc: "Descendente",
},
} as const;

const baseColumns = [
  ["part_number", "Part number / 料號"],
  ["name", "Product name / 產品名稱"],
  ["manufacturer", "Manufacturer / 製造商"],
  ["brand", "Brand / 品牌"],
  ["package", "Package / 封裝"],
  ["lifecycle", "Lifecycle / 生命週期"],
  ["documents", "Documents / 文件"],
  ["rfq", "RFQ"],
] as const;

const defaultProfile: CategoryListingProfile = {
  visible_columns: ["part_number", "name", "manufacturer", "package", "lifecycle", "documents", "rfq"],
  default_sort: "part_number",
  default_sort_direction: "asc",
  mobile_key_specs: [],
};

export function ListingProfilesPanel({ locale, onError, onMessage }: Props) {
  const text = labels[locale];
  const [form] = Form.useForm<CategoryListingProfile>();
  const [website, setWebsite] = useState<WebsiteState>();
  const [categories, setCategories] = useState<Category[]>([]);
  const [specs, setSpecs] = useState<SpecDefinition[]>([]);
  const [specSet, setSpecSet] = useState<SpecSet>();
  const [categoryID, setCategoryID] = useState<string>();
  const [loading, setLoading] = useState(false);

  const load = async () => {
    setLoading(true);
    try {
      const [nextWebsite, nextCategories, nextSpecs] = await Promise.all([
        api<WebsiteState>("/admin/api/website/configuration"),
        api<Category[]>("/admin/api/categories"),
        api<SpecDefinition[]>("/admin/api/specs"),
      ]);
      setWebsite(nextWebsite);
      setCategories(nextCategories);
      setSpecs(nextSpecs);
      const selectable = nextCategories.filter((category) => category.status === "active" && category.system_key !== "root");
      const configured = selectable.find((category) => nextWebsite.working.category_listing_profiles?.[category.id]);
      setCategoryID((current) => current && selectable.some((category) => category.id === current) ? current : configured?.id ?? selectable.find((category) => !category.system_key)?.id ?? selectable[0]?.id);
    } catch (error) {
      onError(error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (!categoryID || !website) return;
    form.setFieldsValue(website.working.category_listing_profiles?.[categoryID] ?? defaultProfile);
    setSpecSet(undefined);
		void api<SpecSet | null>(`/admin/api/categories/${encodeURIComponent(categoryID)}/spec-set`)
			.then((next) => setSpecSet(next ?? undefined))
			.catch(onError);
  }, [categoryID, form, onError, website]);

  const specOptions = useMemo(() => {
    const allowed = new Set(specSet?.spec_ids ?? []);
    return specs
      .filter((spec) => spec.status === "active" && allowed.has(spec.id))
      .map((spec) => ({ value: `spec:${spec.id}`, label: `${spec.name}${spec.preferred_unit ? ` (${spec.preferred_unit})` : ""}` }));
  }, [specSet, specs]);
  const visible = Form.useWatch("visible_columns", form) ?? [];
  const mobileOptions = specOptions.filter((option) => visible.includes(option.value));

  const saveConfiguration = async (configuration: SiteConfiguration, message: "saved" | "removed") => {
    if (!website) return;
    setLoading(true);
    try {
      const next = await putJSON<WebsiteState>("/admin/api/website/configuration", {
        expected_revision: website.working_revision,
        configuration,
      });
      setWebsite(next);
      onMessage(text[message](next.working_revision));
    } catch (error) {
      onError(error);
      await load();
    } finally {
      setLoading(false);
    }
  };

  const save = async (profile: CategoryListingProfile) => {
    if (!website || !categoryID) return;
    await saveConfiguration({
      ...website.working,
      category_listing_profiles: {
        ...(website.working.category_listing_profiles ?? {}),
        [categoryID]: profile,
      },
    }, "saved");
  };

  const remove = async () => {
    if (!website || !categoryID) return;
    const profiles = { ...(website.working.category_listing_profiles ?? {}) };
    delete profiles[categoryID];
    await saveConfiguration({ ...website.working, category_listing_profiles: profiles }, "removed");
    form.setFieldsValue(defaultProfile);
  };

  return (
    <Spin spinning={loading}>
      <Space direction="vertical" size="large" className="panel-stack">
        <Card title={text.title}>
          <Typography.Paragraph type="secondary">{text.help}</Typography.Paragraph>
          <Form form={form} layout="vertical" initialValues={defaultProfile} onFinish={(values) => void save(values)}>
            <Form.Item label={text.category} required>
              <Select
                showSearch
                optionFilterProp="label"
                value={categoryID}
                onChange={setCategoryID}
                options={categories.filter((category) => category.status === "active" && category.system_key !== "root").map((category) => ({ value: category.id, label: category.name }))}
              />
            </Form.Item>
            {!specSet ? <Alert type="info" showIcon message={text.noSpecSet} /> : null}
            <Form.Item
              name="visible_columns"
              label={text.visible}
              rules={[
                { required: true },
                { validator: (_, value: string[]) => value?.includes("part_number") ? Promise.resolve() : Promise.reject(new Error(text.requiredPart)) },
              ]}
            >
              <Select mode="multiple" options={[...baseColumns.map(([value, label]) => ({ value, label })), ...specOptions]} />
            </Form.Item>
            <Space align="start" wrap>
              <Form.Item name="default_sort" label={text.sort} rules={[{ required: true }]}>
                <Select style={{ minWidth: 220 }} options={baseColumns.filter(([value]) => ["part_number", "name", "manufacturer", "brand", "lifecycle"].includes(value)).map(([value, label]) => ({ value, label }))} />
              </Form.Item>
              <Form.Item name="default_sort_direction" label={text.direction} rules={[{ required: true }]}>
                <Select style={{ minWidth: 160 }} options={[{ value: "asc", label: text.asc }, { value: "desc", label: text.desc }]} />
              </Form.Item>
            </Space>
            <Form.Item name="mobile_key_specs" label={text.mobile}>
              <Select mode="multiple" maxCount={5} options={mobileOptions} />
            </Form.Item>
            <Space>
              <Button type="primary" htmlType="submit" disabled={!categoryID}>{text.save}</Button>
              <Button onClick={() => void remove()} disabled={!categoryID || !website?.working.category_listing_profiles?.[categoryID]}>{text.remove}</Button>
            </Space>
          </Form>
        </Card>
      </Space>
    </Spin>
  );
}
