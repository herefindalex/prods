import { useEffect, useMemo, useState } from "react";
import { Alert, Button, Card, Form, Select, Space, Spin, Typography } from "antd";
import { APIError, api, putJSON } from "./api";
import type {
  Category,
  CategoryListingProfile,
  SiteConfiguration,
  SpecDefinition,
  SpecSet,
  WebsiteState,
} from "./types";

type Props = {
  locale: "en-US" | "zh-TW";
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
    void api<SpecSet>(`/admin/api/categories/${encodeURIComponent(categoryID)}/spec-set`)
      .then(setSpecSet)
      .catch((error: unknown) => {
        if (!(error instanceof APIError) || error.status !== 404) onError(error);
      });
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
