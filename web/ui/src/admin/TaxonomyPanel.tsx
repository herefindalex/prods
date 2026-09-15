import { useEffect, useState } from "react";
import { Button, Card, Checkbox, Form, Input, InputNumber, Modal, Popconfirm, Select, Space, Table, Tabs, Tag } from "antd";
import { api, clientID, postJSON } from "./api";
import { TaxonomyTranslationsModal, type TaxonomyTranslationTarget } from "./TaxonomyTranslationsModal";
import { dictionaryKinds, type Category, type DictionaryEntry, type DictionaryKind, type SpecDefinition, type SpecSet, type TaxonomyImpact } from "./types";

type Feedback = { onError: (error: unknown) => void; onMessage: (message: string) => void };

type TaxonomyLocale = "en-US" | "zh-TW";

const labels = {
  "en-US": {
    categories: "Categories", dictionaries: "Dictionaries", specifications: "Specifications",
    createdCategory: (name: string) => `Created category ${name}.`,
    createdDictionary: (kind: string, name: string) => `Created ${kind} ${name}.`,
    disabledCategory: (name: string) => `Disabled category ${name}.`,
    disabledDictionary: (name: string) => `Disabled ${name}. Existing references remain valid.`,
    updated: (name: string, count: number) => `Updated ${name}; ${count} product(s) queued for publication.`,
    createdSpec: (name: string) => `Created specification ${name}.`,
    createdSpecSet: (name: string) => `Created Spec Set ${name}.`,
    assignedSpecSet: (name: string) => `Assigned Spec Set to ${name}.`,
    newCategory: "New category", name: "Name", slug: "Slug", parent: "Parent", create: "Create",
    categoryData: "Category tree data", refresh: "Refresh", root: "Root", status: "Status", revision: "Revision", actions: "Actions",
    edit: "Edit", translate: "Translations", disable: "Disable", disableCategoryConfirm: "Disable this category? Existing references are preserved.",
    dictionary: "Dictionary", optionalSlug: "Optional slug", disableValueConfirm: "Disable this value? Existing references remain valid.",
    newSpec: "New specification", preferredUnit: "Preferred unit", filterable: "Filterable", newSpecSet: "New Spec Set",
    assignSpecSet: "Assign exact category Spec Set", category: "Category", specSet: "Spec Set", assign: "Assign",
    definitions: "Spec definitions and sets", unit: "Unit", semanticVersion: "Semantic version", yes: "Yes", no: "No", members: "Members",
    editCategory: "Edit category", editDictionary: "Edit dictionary value", applyReviewed: "Apply reviewed change",
    previewAffected: "Preview affected products and routes", active: "Active", disabled: "Disabled",
    affected: (count: number) => `${count} affected product(s)`, notPublic: "not currently public",
    noChanges: "No Product publication changes required.",
    kinds: { manufacturer: "Manufacturer", brand: "Brand", lifecycle: "Lifecycle", application: "Application", document_type: "Document type" } as Record<DictionaryKind, string>,
  },
  "zh-TW": {
    categories: "分類", dictionaries: "字典", specifications: "規格",
    createdCategory: (name: string) => `已建立分類 ${name}。`,
    createdDictionary: (kind: string, name: string) => `已建立${kind} ${name}。`,
    disabledCategory: (name: string) => `已停用分類 ${name}。`,
    disabledDictionary: (name: string) => `已停用 ${name}；既有參照仍然有效。`,
    updated: (name: string, count: number) => `已更新 ${name}；${count} 項產品已排入重新發布。`,
    createdSpec: (name: string) => `已建立規格 ${name}。`,
    createdSpecSet: (name: string) => `已建立 Spec Set ${name}。`,
    assignedSpecSet: (name: string) => `已將 Spec Set 指派給 ${name}。`,
    newCategory: "新增分類", name: "名稱", slug: "Slug", parent: "上層分類", create: "建立",
    categoryData: "分類樹資料", refresh: "重新整理", root: "根分類", status: "狀態", revision: "修訂", actions: "操作",
    edit: "編輯", translate: "翻譯", disable: "停用", disableCategoryConfirm: "要停用這個分類嗎？既有參照會保留。",
    dictionary: "字典", optionalSlug: "選填 Slug", disableValueConfirm: "要停用這個值嗎？既有參照仍然有效。",
    newSpec: "新增規格", preferredUnit: "偏好單位", filterable: "可篩選", newSpecSet: "新增 Spec Set",
    assignSpecSet: "指派分類專屬 Spec Set", category: "分類", specSet: "Spec Set", assign: "指派",
    definitions: "規格定義與集合", unit: "單位", semanticVersion: "語意版本", yes: "是", no: "否", members: "成員",
    editCategory: "編輯分類", editDictionary: "編輯字典值", applyReviewed: "套用已檢視的變更",
    previewAffected: "預覽受影響的產品與路由", active: "有效", disabled: "停用",
    affected: (count: number) => `${count} 項受影響產品`, notPublic: "目前未公開",
    noChanges: "不需要變更任何 Product publication。",
    kinds: { manufacturer: "製造商", brand: "品牌", lifecycle: "生命週期", application: "應用", document_type: "文件類型" } as Record<DictionaryKind, string>,
  },
} as const;

export function TaxonomyPanel({ locale, onError, onMessage }: Feedback & { locale: TaxonomyLocale }) {
  const text = labels[locale];
  const [categories, setCategories] = useState<Category[]>([]);
  const [kind, setKind] = useState<DictionaryKind>("manufacturer");
  const [entries, setEntries] = useState<DictionaryEntry[]>([]);
  const [specs, setSpecs] = useState<SpecDefinition[]>([]);
	const [specSets, setSpecSets] = useState<SpecSet[]>([]);
	const [editingCategory, setEditingCategory] = useState<Category>();
	const [editingDictionary, setEditingDictionary] = useState<DictionaryEntry>();
	const [translationTarget, setTranslationTarget] = useState<TaxonomyTranslationTarget>();
	const [categoryImpact, setCategoryImpact] = useState<TaxonomyImpact>();
	const [dictionaryImpact, setDictionaryImpact] = useState<TaxonomyImpact>();
	const [taxonomySaving, setTaxonomySaving] = useState(false);
	const [categoryForm] = Form.useForm<{ name: string; slug: string; parent_id?: string }>();
	const [dictionaryForm] = Form.useForm<{ name: string; slug?: string }>();
	const [categoryEditForm] = Form.useForm<{ name: string; slug: string; parent_id?: string }>();
	const [dictionaryEditForm] = Form.useForm<{ name: string; slug?: string }>();
  const [specForm] = Form.useForm<{ name: string; preferred_unit?: string; filterable: boolean; semantic_version: number }>();
  const [specSetForm] = Form.useForm<{ name: string; spec_ids: string[] }>();
  const [assignmentForm] = Form.useForm<{ category_id: string; spec_set_id: string }>();

  const loadCategories = async () => {
    try {
      setCategories((await api<Category[]>("/admin/api/categories")) ?? []);
    } catch (error) {
      onError(error);
    }
  };
  const loadDictionary = async (nextKind = kind) => {
    try {
      setEntries((await api<DictionaryEntry[]>(`/admin/api/dictionaries?kind=${nextKind}`)) ?? []);
    } catch (error) {
      onError(error);
    }
  };
  const loadSpecs = async () => {
    try {
      const [nextSpecs, nextSets] = await Promise.all([
        api<SpecDefinition[]>("/admin/api/specs"),
        api<SpecSet[]>("/admin/api/spec-sets"),
      ]);
      setSpecs(nextSpecs ?? []);
      setSpecSets(nextSets ?? []);
    } catch (error) {
      onError(error);
    }
  };
  useEffect(() => void loadCategories(), []);
  useEffect(() => void loadDictionary(kind), [kind]);
  useEffect(() => void loadSpecs(), []);

  const createCategory = async (values: { name: string; slug: string; parent_id?: string }) => {
    try {
      await postJSON<Category>("/admin/api/categories", {
        id: clientID("cat"),
        ...values,
        parent_id: values.parent_id || "",
        status: "active",
        revision: 1,
      });
      categoryForm.resetFields();
      onMessage(text.createdCategory(values.name));
      await loadCategories();
    } catch (error) {
      onError(error);
    }
  };

  const createDictionaryEntry = async (values: { name: string; slug?: string }) => {
    try {
      await postJSON<DictionaryEntry>("/admin/api/dictionaries", {
        id: clientID("dic"),
        kind,
        ...values,
        status: "active",
        revision: 1,
      });
      dictionaryForm.resetFields();
      onMessage(text.createdDictionary(text.kinds[kind], values.name));
      await loadDictionary();
    } catch (error) {
      onError(error);
    }
  };

  const disableCategory = async (category: Category) => {
    try {
      await postJSON<void>(`/admin/api/categories/${category.id}/disable`, { expected_revision: category.revision });
      onMessage(text.disabledCategory(category.name));
      await loadCategories();
    } catch (error) {
      onError(error);
    }
  };

	const disableDictionary = async (entry: DictionaryEntry) => {
    try {
      await postJSON<void>(`/admin/api/dictionaries/${entry.id}/disable`, { expected_revision: entry.revision });
      onMessage(text.disabledDictionary(entry.name));
      await loadDictionary();
    } catch (error) {
      onError(error);
    }
	};

	const openCategoryEdit = (category: Category) => {
		setEditingCategory(category);
		setCategoryImpact(undefined);
		categoryEditForm.setFieldsValue({ name: category.name, slug: category.slug, parent_id: category.parent_id });
	};

	const previewCategoryEdit = async () => {
		if (!editingCategory) return;
		try {
			const values = await categoryEditForm.validateFields();
			setCategoryImpact(await postJSON<TaxonomyImpact>(`/admin/api/categories/${editingCategory.id}/update-preview`, {
				expected_revision: editingCategory.revision,
				...values,
				parent_id: values.parent_id || "",
			}));
		} catch (error) {
			onError(error);
		}
	};

	const applyCategoryEdit = async () => {
		if (!editingCategory || !categoryImpact) return;
		setTaxonomySaving(true);
		try {
			const values = await categoryEditForm.validateFields();
			await postJSON(`/admin/api/categories/${editingCategory.id}/update`, {
				expected_revision: editingCategory.revision,
				...values,
				parent_id: values.parent_id || "",
			});
      onMessage(text.updated(editingCategory.name, categoryImpact.affected_products.length));
			setEditingCategory(undefined);
			setCategoryImpact(undefined);
			await loadCategories();
		} catch (error) {
			onError(error);
		} finally {
			setTaxonomySaving(false);
		}
	};

	const openDictionaryEdit = (entry: DictionaryEntry) => {
		setEditingDictionary(entry);
		setDictionaryImpact(undefined);
		dictionaryEditForm.setFieldsValue({ name: entry.name, slug: entry.slug });
	};

	const previewDictionaryEdit = async () => {
		if (!editingDictionary) return;
		try {
			const values = await dictionaryEditForm.validateFields();
			setDictionaryImpact(await postJSON<TaxonomyImpact>(`/admin/api/dictionaries/${editingDictionary.id}/update-preview`, {
				expected_revision: editingDictionary.revision,
				...values,
				slug: values.slug || "",
			}));
		} catch (error) {
			onError(error);
		}
	};

	const applyDictionaryEdit = async () => {
		if (!editingDictionary || !dictionaryImpact) return;
		setTaxonomySaving(true);
		try {
			const values = await dictionaryEditForm.validateFields();
			await postJSON(`/admin/api/dictionaries/${editingDictionary.id}/update`, {
				expected_revision: editingDictionary.revision,
				...values,
				slug: values.slug || "",
			});
      onMessage(text.updated(editingDictionary.name, dictionaryImpact.affected_products.length));
			setEditingDictionary(undefined);
			setDictionaryImpact(undefined);
			await loadDictionary();
		} catch (error) {
			onError(error);
		} finally {
			setTaxonomySaving(false);
		}
	};

  const createSpec = async (values: { name: string; preferred_unit?: string; filterable: boolean; semantic_version: number }) => {
    try {
      await postJSON<SpecDefinition>("/admin/api/specs", {
        id: clientID("spc"),
        ...values,
        preferred_unit: values.preferred_unit || "",
        status: "active",
        revision: 1,
      });
      specForm.resetFields();
      onMessage(text.createdSpec(values.name));
      await loadSpecs();
    } catch (error) {
      onError(error);
    }
  };

  const createSpecSet = async (values: { name: string; spec_ids: string[] }) => {
    try {
      await postJSON<SpecSet>("/admin/api/spec-sets", {
        id: clientID("sps"),
        ...values,
        status: "active",
        revision: 1,
      });
      specSetForm.resetFields();
      onMessage(text.createdSpecSet(values.name));
      await loadSpecs();
    } catch (error) {
      onError(error);
    }
  };

  const assignSpecSet = async (values: { category_id: string; spec_set_id: string }) => {
    const category = categories.find((item) => item.id === values.category_id);
    if (!category) return;
    try {
      await postJSON<void>(`/admin/api/categories/${category.id}/spec-set`, {
        expected_revision: category.revision,
        spec_set_id: values.spec_set_id,
      });
      assignmentForm.resetFields();
      onMessage(text.assignedSpecSet(category.name));
      await loadCategories();
    } catch (error) {
      onError(error);
      await loadCategories();
    }
  };

  return (
    <>
    <Tabs
      items={[
        {
          key: "categories",
          label: text.categories,
          children: (
            <Space direction="vertical" size="large" className="panel-stack">
              <Card title={text.newCategory}>
                <Form form={categoryForm} layout="inline" onFinish={(values) => void createCategory(values)}>
                  <Form.Item name="name" rules={[{ required: true }]}><Input placeholder={text.name} /></Form.Item>
                  <Form.Item name="slug" rules={[{ required: true }]}><Input placeholder={text.slug} /></Form.Item>
                  <Form.Item name="parent_id"><Select allowClear showSearch optionFilterProp="label" placeholder={text.parent} style={{ minWidth: 180 }} options={categories.filter((row) => row.status === "active").map((row) => ({ value: row.id, label: row.name }))} /></Form.Item>
                  <Button type="primary" htmlType="submit">{text.create}</Button>
                </Form>
              </Card>
              <Card title={text.categoryData} extra={<Button onClick={() => void loadCategories()}>{text.refresh}</Button>}>
                <Table<Category>
                  rowKey="id"
                  dataSource={categories}
                  pagination={false}
                  columns={[
                    { title: text.name, dataIndex: "name" },
                    { title: text.slug, dataIndex: "slug" },
                    { title: text.parent, render: (_, row) => categories.find((item) => item.id === row.parent_id)?.name ?? row.parent_id ?? text.root },
                    { title: text.status, render: (_, row) => <Tag color={row.status === "active" ? "green" : "default"}>{row.status === "active" ? text.active : text.disabled}</Tag> },
                    { title: text.revision, dataIndex: "revision" },
                    {
                      title: text.actions,
                      render: (_, row) => row.system_key === "root" ? null : (
                        <Space>
                          <Button size="small" onClick={() => openCategoryEdit(row)}>{text.edit}</Button>
                          <Button size="small" onClick={() => setTranslationTarget({ type: "category", id: row.id, name: row.name })}>{text.translate}</Button>
                          {!row.system_key && row.status !== "disabled" ? (
                            <Popconfirm title={text.disableCategoryConfirm} onConfirm={() => void disableCategory(row)}>
                              <Button size="small" danger>{text.disable}</Button>
                            </Popconfirm>
                          ) : null}
                        </Space>
                      ),
                    },
                  ]}
                />
              </Card>
            </Space>
          ),
        },
        {
          key: "dictionaries",
          label: text.dictionaries,
          children: (
            <Space direction="vertical" size="large" className="panel-stack">
              <Card title={text.dictionary}>
                <Space direction="vertical" className="panel-stack">
                  <Select<DictionaryKind>
                    value={kind}
                    onChange={setKind}
                    options={dictionaryKinds.map((value) => ({ value, label: text.kinds[value] }))}
                    style={{ width: 220 }}
                  />
                  <Form form={dictionaryForm} layout="inline" onFinish={(values) => void createDictionaryEntry(values)}>
                    <Form.Item name="name" rules={[{ required: true }]}><Input placeholder={text.name} /></Form.Item>
                    <Form.Item name="slug"><Input placeholder={text.optionalSlug} /></Form.Item>
                    <Button type="primary" htmlType="submit">{text.create}</Button>
                  </Form>
                </Space>
              </Card>
              <Card title={text.kinds[kind]} extra={<Button onClick={() => void loadDictionary()}>{text.refresh}</Button>}>
                <Table<DictionaryEntry>
                  rowKey="id"
                  dataSource={entries}
                  pagination={false}
                  columns={[
                    { title: text.name, dataIndex: "name" },
                    { title: text.slug, dataIndex: "slug", render: (value: string) => value || "—" },
                    { title: text.status, render: (_, row) => <Tag color={row.status === "active" ? "green" : "default"}>{row.status === "active" ? text.active : text.disabled}</Tag> },
                    { title: text.revision, dataIndex: "revision" },
                    {
                      title: text.actions,
                      render: (_, row) => (
                        <Space>
                          <Button size="small" onClick={() => openDictionaryEdit(row)}>{text.edit}</Button>
                          <Button size="small" onClick={() => setTranslationTarget({ type: "dictionary", id: row.id, name: row.name })}>{text.translate}</Button>
                          {row.status !== "disabled" ? (
                            <Popconfirm title={text.disableValueConfirm} onConfirm={() => void disableDictionary(row)}>
                              <Button size="small" danger>{text.disable}</Button>
                            </Popconfirm>
                          ) : null}
                        </Space>
                      ),
                    },
                  ]}
                />
              </Card>
            </Space>
          ),
        },
        {
          key: "specifications",
          label: text.specifications,
          children: (
            <Space direction="vertical" size="large" className="panel-stack">
              <Card title={text.newSpec}>
                <Form form={specForm} layout="inline" initialValues={{ filterable: false, semantic_version: 1 }} onFinish={(values) => void createSpec(values)}>
                  <Form.Item name="name" rules={[{ required: true }]}><Input placeholder={text.name} /></Form.Item>
                  <Form.Item name="preferred_unit"><Input placeholder={text.preferredUnit} /></Form.Item>
                  <Form.Item name="semantic_version" rules={[{ required: true }]}><InputNumber min={1} precision={0} /></Form.Item>
                  <Form.Item name="filterable" valuePropName="checked"><Checkbox>{text.filterable}</Checkbox></Form.Item>
                  <Button type="primary" htmlType="submit">{text.create}</Button>
                </Form>
              </Card>
              <Card title={text.newSpecSet}>
                <Form form={specSetForm} layout="inline" onFinish={(values) => void createSpecSet(values)}>
                  <Form.Item name="name" rules={[{ required: true }]}><Input placeholder={text.name} /></Form.Item>
                  <Form.Item name="spec_ids" rules={[{ required: true }]}><Select mode="multiple" placeholder={text.specifications} style={{ minWidth: 320 }} options={specs.filter((spec) => spec.status === "active").map((spec) => ({ value: spec.id, label: spec.name }))} /></Form.Item>
                  <Button type="primary" htmlType="submit">{text.create}</Button>
                </Form>
              </Card>
              <Card title={text.assignSpecSet}>
                <Form form={assignmentForm} layout="inline" onFinish={(values) => void assignSpecSet(values)}>
                  <Form.Item name="category_id" rules={[{ required: true }]}><Select placeholder={text.category} style={{ minWidth: 220 }} options={categories.filter((category) => category.status === "active" && !category.system_key).map((category) => ({ value: category.id, label: category.name }))} /></Form.Item>
                  <Form.Item name="spec_set_id" rules={[{ required: true }]}><Select placeholder={text.specSet} style={{ minWidth: 220 }} options={specSets.filter((set) => set.status === "active").map((set) => ({ value: set.id, label: set.name }))} /></Form.Item>
                  <Button type="primary" htmlType="submit">{text.assign}</Button>
                </Form>
              </Card>
              <Card title={text.definitions} extra={<Button onClick={() => void loadSpecs()}>{text.refresh}</Button>}>
                <Table<SpecDefinition>
                  rowKey="id"
                  dataSource={specs}
                  pagination={false}
                  columns={[
                    { title: text.name, dataIndex: "name" },
                    { title: text.unit, dataIndex: "preferred_unit", render: (value: string) => value || "—" },
                    { title: text.semanticVersion, dataIndex: "semantic_version" },
                    { title: text.filterable, dataIndex: "filterable", render: (value: boolean) => value ? text.yes : text.no },
                    { title: text.status, dataIndex: "status", render: (value: string) => value === "active" ? text.active : text.disabled },
                  ]}
                />
                <Table<SpecSet>
                  className="top-gap"
                  rowKey="id"
                  dataSource={specSets}
                  pagination={false}
                  columns={[
                    { title: text.specSet, dataIndex: "name" },
                    { title: text.members, dataIndex: "spec_ids", render: (ids: string[]) => ids.map((id) => specs.find((spec) => spec.id === id)?.name ?? id).join(", ") },
                    { title: text.revision, dataIndex: "revision" },
                    { title: text.status, dataIndex: "status", render: (value: string) => value === "active" ? text.active : text.disabled },
                  ]}
                />
              </Card>
            </Space>
          ),
        },
      ]}
    />
    <Modal
      open={Boolean(editingCategory)}
      title={text.editCategory}
      okText={text.applyReviewed}
      okButtonProps={{ disabled: !categoryImpact }}
      confirmLoading={taxonomySaving}
      onOk={() => void applyCategoryEdit()}
      onCancel={() => { setEditingCategory(undefined); setCategoryImpact(undefined); }}
    >
      <Form form={categoryEditForm} layout="vertical" onValuesChange={() => setCategoryImpact(undefined)}>
        <Form.Item name="name" label={text.name} rules={[{ required: true }]}><Input disabled={editingCategory?.system_key === "uncategorized"} /></Form.Item>
        <Form.Item name="slug" label={text.slug} rules={[{ required: true }]}><Input /></Form.Item>
        <Form.Item name="parent_id" label={text.parent}>
          <Select
            disabled={editingCategory?.system_key === "uncategorized"}
            showSearch
            optionFilterProp="label"
            options={categories.filter((row) => row.status === "active" && row.id !== editingCategory?.id).map((row) => ({ value: row.id, label: row.name }))}
          />
        </Form.Item>
      </Form>
      <Button onClick={() => void previewCategoryEdit()}>{text.previewAffected}</Button>
      {categoryImpact ? <ImpactSummary impact={categoryImpact} locale={locale} /> : null}
    </Modal>
    <Modal
      open={Boolean(editingDictionary)}
      title={text.editDictionary}
      okText={text.applyReviewed}
      okButtonProps={{ disabled: !dictionaryImpact }}
      confirmLoading={taxonomySaving}
      onOk={() => void applyDictionaryEdit()}
      onCancel={() => { setEditingDictionary(undefined); setDictionaryImpact(undefined); }}
    >
      <Form form={dictionaryEditForm} layout="vertical" onValuesChange={() => setDictionaryImpact(undefined)}>
        <Form.Item name="name" label={text.name} rules={[{ required: true }]}><Input /></Form.Item>
        <Form.Item name="slug" label={text.slug}><Input /></Form.Item>
      </Form>
      <Button onClick={() => void previewDictionaryEdit()}>{text.previewAffected}</Button>
      {dictionaryImpact ? <ImpactSummary impact={dictionaryImpact} locale={locale} /> : null}
    </Modal>
    <TaxonomyTranslationsModal
      target={translationTarget}
      locale={locale}
      onClose={() => setTranslationTarget(undefined)}
      onError={onError}
      onSaved={() => {
        void loadCategories();
        void loadDictionary();
      }}
    />
    </>
  );
}

function ImpactSummary({ impact, locale }: { impact: TaxonomyImpact; locale: TaxonomyLocale }) {
  const text = labels[locale];
  return (
    <div className="top-gap">
      <strong>{text.affected(impact.affected_products.length)}</strong>
      {impact.affected_products.length ? (
        <ul>
          {impact.affected_products.map((item) => (
            <li key={item.product_id}>
              {item.part_number}: {item.current_route || text.notPublic}
              {item.proposed_route && item.proposed_route !== item.current_route ? ` → ${item.proposed_route}` : ""}
            </li>
          ))}
        </ul>
      ) : <p>{text.noChanges}</p>}
    </div>
  );
}
