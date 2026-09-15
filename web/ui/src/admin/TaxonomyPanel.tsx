import { useEffect, useState } from "react";
import { Button, Card, Checkbox, Form, Input, InputNumber, Modal, Popconfirm, Select, Space, Table, Tabs, Tag } from "antd";
import { api, clientID, postJSON } from "./api";
import { dictionaryKinds, type Category, type DictionaryEntry, type DictionaryKind, type SpecDefinition, type SpecSet, type TaxonomyImpact } from "./types";

type Feedback = { onError: (error: unknown) => void; onMessage: (message: string) => void };

export function TaxonomyPanel({ onError, onMessage }: Feedback) {
  const [categories, setCategories] = useState<Category[]>([]);
  const [kind, setKind] = useState<DictionaryKind>("manufacturer");
  const [entries, setEntries] = useState<DictionaryEntry[]>([]);
  const [specs, setSpecs] = useState<SpecDefinition[]>([]);
	const [specSets, setSpecSets] = useState<SpecSet[]>([]);
	const [editingCategory, setEditingCategory] = useState<Category>();
	const [editingDictionary, setEditingDictionary] = useState<DictionaryEntry>();
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
      onMessage(`Created category ${values.name}.`);
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
      onMessage(`Created ${kind.replace("_", " ")} ${values.name}.`);
      await loadDictionary();
    } catch (error) {
      onError(error);
    }
  };

  const disableCategory = async (category: Category) => {
    try {
      await postJSON<void>(`/admin/api/categories/${category.id}/disable`, { expected_revision: category.revision });
      onMessage(`Disabled category ${category.name}.`);
      await loadCategories();
    } catch (error) {
      onError(error);
    }
  };

	const disableDictionary = async (entry: DictionaryEntry) => {
    try {
      await postJSON<void>(`/admin/api/dictionaries/${entry.id}/disable`, { expected_revision: entry.revision });
      onMessage(`Disabled ${entry.name}. Existing references remain valid.`);
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
			onMessage(`Updated ${editingCategory.name}; ${categoryImpact.affected_products.length} product(s) queued for publication.`);
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
			onMessage(`Updated ${editingDictionary.name}; ${dictionaryImpact.affected_products.length} product(s) queued for publication.`);
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
      onMessage(`Created specification ${values.name}.`);
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
      onMessage(`Created Spec Set ${values.name}.`);
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
      onMessage(`Assigned a Spec Set to ${category.name}.`);
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
          label: "Categories",
          children: (
            <Space direction="vertical" size="large" className="panel-stack">
              <Card title="New category">
                <Form form={categoryForm} layout="inline" onFinish={(values) => void createCategory(values)}>
                  <Form.Item name="name" rules={[{ required: true }]}><Input placeholder="Name" /></Form.Item>
                  <Form.Item name="slug" rules={[{ required: true }]}><Input placeholder="slug" /></Form.Item>
                  <Form.Item name="parent_id"><Select allowClear showSearch optionFilterProp="label" placeholder="Parent" style={{ minWidth: 180 }} options={categories.filter((row) => row.status === "active").map((row) => ({ value: row.id, label: row.name }))} /></Form.Item>
                  <Button type="primary" htmlType="submit">Create</Button>
                </Form>
              </Card>
              <Card title="Category tree data" extra={<Button onClick={() => void loadCategories()}>Refresh</Button>}>
                <Table<Category>
                  rowKey="id"
                  dataSource={categories}
                  pagination={false}
                  columns={[
                    { title: "Name", dataIndex: "name" },
                    { title: "Slug", dataIndex: "slug" },
                    { title: "Parent", render: (_, row) => categories.find((item) => item.id === row.parent_id)?.name ?? row.parent_id ?? "Root" },
                    { title: "Status", render: (_, row) => <Tag color={row.status === "active" ? "green" : "default"}>{row.status}</Tag> },
                    { title: "Revision", dataIndex: "revision" },
                    {
                      title: "Actions",
                      render: (_, row) => row.system_key === "root" ? null : (
                        <Space>
                          <Button size="small" onClick={() => openCategoryEdit(row)}>Edit</Button>
                          {!row.system_key && row.status !== "disabled" ? (
                            <Popconfirm title="Disable this category? Existing references are preserved." onConfirm={() => void disableCategory(row)}>
                              <Button size="small" danger>Disable</Button>
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
          label: "Dictionaries",
          children: (
            <Space direction="vertical" size="large" className="panel-stack">
              <Card title="Dictionary">
                <Space direction="vertical" className="panel-stack">
                  <Select<DictionaryKind>
                    value={kind}
                    onChange={setKind}
                    options={dictionaryKinds.map((value) => ({ value, label: value.replace("_", " ") }))}
                    style={{ width: 220 }}
                  />
                  <Form form={dictionaryForm} layout="inline" onFinish={(values) => void createDictionaryEntry(values)}>
                    <Form.Item name="name" rules={[{ required: true }]}><Input placeholder="Name" /></Form.Item>
                    <Form.Item name="slug"><Input placeholder="Optional slug" /></Form.Item>
                    <Button type="primary" htmlType="submit">Create</Button>
                  </Form>
                </Space>
              </Card>
              <Card title={kind.replace("_", " ")} extra={<Button onClick={() => void loadDictionary()}>Refresh</Button>}>
                <Table<DictionaryEntry>
                  rowKey="id"
                  dataSource={entries}
                  pagination={false}
                  columns={[
                    { title: "Name", dataIndex: "name" },
                    { title: "Slug", dataIndex: "slug", render: (value: string) => value || "—" },
                    { title: "Status", render: (_, row) => <Tag color={row.status === "active" ? "green" : "default"}>{row.status}</Tag> },
                    { title: "Revision", dataIndex: "revision" },
                    {
                      title: "Actions",
                      render: (_, row) => (
                        <Space>
                          <Button size="small" onClick={() => openDictionaryEdit(row)}>Edit</Button>
                          {row.status !== "disabled" ? (
                            <Popconfirm title="Disable this value? Existing references remain valid." onConfirm={() => void disableDictionary(row)}>
                              <Button size="small" danger>Disable</Button>
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
          label: "Specifications",
          children: (
            <Space direction="vertical" size="large" className="panel-stack">
              <Card title="New specification">
                <Form form={specForm} layout="inline" initialValues={{ filterable: false, semantic_version: 1 }} onFinish={(values) => void createSpec(values)}>
                  <Form.Item name="name" rules={[{ required: true }]}><Input placeholder="Name" /></Form.Item>
                  <Form.Item name="preferred_unit"><Input placeholder="Preferred unit" /></Form.Item>
                  <Form.Item name="semantic_version" rules={[{ required: true }]}><InputNumber min={1} precision={0} /></Form.Item>
                  <Form.Item name="filterable" valuePropName="checked"><Checkbox>Filterable</Checkbox></Form.Item>
                  <Button type="primary" htmlType="submit">Create</Button>
                </Form>
              </Card>
              <Card title="New Spec Set">
                <Form form={specSetForm} layout="inline" onFinish={(values) => void createSpecSet(values)}>
                  <Form.Item name="name" rules={[{ required: true }]}><Input placeholder="Name" /></Form.Item>
                  <Form.Item name="spec_ids" rules={[{ required: true }]}><Select mode="multiple" placeholder="Specifications" style={{ minWidth: 320 }} options={specs.filter((spec) => spec.status === "active").map((spec) => ({ value: spec.id, label: spec.name }))} /></Form.Item>
                  <Button type="primary" htmlType="submit">Create</Button>
                </Form>
              </Card>
              <Card title="Assign exact category Spec Set">
                <Form form={assignmentForm} layout="inline" onFinish={(values) => void assignSpecSet(values)}>
                  <Form.Item name="category_id" rules={[{ required: true }]}><Select placeholder="Category" style={{ minWidth: 220 }} options={categories.filter((category) => category.status === "active" && !category.system_key).map((category) => ({ value: category.id, label: category.name }))} /></Form.Item>
                  <Form.Item name="spec_set_id" rules={[{ required: true }]}><Select placeholder="Spec Set" style={{ minWidth: 220 }} options={specSets.filter((set) => set.status === "active").map((set) => ({ value: set.id, label: set.name }))} /></Form.Item>
                  <Button type="primary" htmlType="submit">Assign</Button>
                </Form>
              </Card>
              <Card title="Spec definitions and sets" extra={<Button onClick={() => void loadSpecs()}>Refresh</Button>}>
                <Table<SpecDefinition>
                  rowKey="id"
                  dataSource={specs}
                  pagination={false}
                  columns={[
                    { title: "Name", dataIndex: "name" },
                    { title: "Unit", dataIndex: "preferred_unit", render: (value: string) => value || "—" },
                    { title: "Semantic version", dataIndex: "semantic_version" },
                    { title: "Filterable", dataIndex: "filterable", render: (value: boolean) => value ? "Yes" : "No" },
                    { title: "Status", dataIndex: "status" },
                  ]}
                />
                <Table<SpecSet>
                  className="top-gap"
                  rowKey="id"
                  dataSource={specSets}
                  pagination={false}
                  columns={[
                    { title: "Spec Set", dataIndex: "name" },
                    { title: "Members", dataIndex: "spec_ids", render: (ids: string[]) => ids.map((id) => specs.find((spec) => spec.id === id)?.name ?? id).join(", ") },
                    { title: "Revision", dataIndex: "revision" },
                    { title: "Status", dataIndex: "status" },
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
      title="Edit category"
      okText="Apply reviewed change"
      okButtonProps={{ disabled: !categoryImpact }}
      confirmLoading={taxonomySaving}
      onOk={() => void applyCategoryEdit()}
      onCancel={() => { setEditingCategory(undefined); setCategoryImpact(undefined); }}
    >
      <Form form={categoryEditForm} layout="vertical" onValuesChange={() => setCategoryImpact(undefined)}>
        <Form.Item name="name" label="Name" rules={[{ required: true }]}><Input disabled={editingCategory?.system_key === "uncategorized"} /></Form.Item>
        <Form.Item name="slug" label="Slug" rules={[{ required: true }]}><Input /></Form.Item>
        <Form.Item name="parent_id" label="Parent">
          <Select
            disabled={editingCategory?.system_key === "uncategorized"}
            showSearch
            optionFilterProp="label"
            options={categories.filter((row) => row.status === "active" && row.id !== editingCategory?.id).map((row) => ({ value: row.id, label: row.name }))}
          />
        </Form.Item>
      </Form>
      <Button onClick={() => void previewCategoryEdit()}>Preview affected products and routes</Button>
      {categoryImpact ? <ImpactSummary impact={categoryImpact} /> : null}
    </Modal>
    <Modal
      open={Boolean(editingDictionary)}
      title="Edit dictionary value"
      okText="Apply reviewed change"
      okButtonProps={{ disabled: !dictionaryImpact }}
      confirmLoading={taxonomySaving}
      onOk={() => void applyDictionaryEdit()}
      onCancel={() => { setEditingDictionary(undefined); setDictionaryImpact(undefined); }}
    >
      <Form form={dictionaryEditForm} layout="vertical" onValuesChange={() => setDictionaryImpact(undefined)}>
        <Form.Item name="name" label="Name" rules={[{ required: true }]}><Input /></Form.Item>
        <Form.Item name="slug" label="Slug"><Input /></Form.Item>
      </Form>
      <Button onClick={() => void previewDictionaryEdit()}>Preview affected products and routes</Button>
      {dictionaryImpact ? <ImpactSummary impact={dictionaryImpact} /> : null}
    </Modal>
    </>
  );
}

function ImpactSummary({ impact }: { impact: TaxonomyImpact }) {
  return (
    <div className="top-gap">
      <strong>{impact.affected_products.length} affected product(s)</strong>
      {impact.affected_products.length ? (
        <ul>
          {impact.affected_products.map((item) => (
            <li key={item.product_id}>
              {item.part_number}: {item.current_route || "not currently public"}
              {item.proposed_route && item.proposed_route !== item.current_route ? ` → ${item.proposed_route}` : ""}
            </li>
          ))}
        </ul>
      ) : <p>No Product publication changes are required.</p>}
    </div>
  );
}
