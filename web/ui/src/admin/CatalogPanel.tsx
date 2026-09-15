import { useEffect, useMemo, useState } from "react";
import {
  Button,
  Card,
  Checkbox,
  Form,
  Input,
  Modal,
  Popconfirm,
  Select,
  Space,
  Table,
  Tag,
  Typography,
} from "antd";
import { api, downloadFile, postJSON, putJSON } from "./api";
import { ProductDataModal } from "./ProductDataModal";
import type { Category, DictionaryEntry, Product, ProductForm } from "./types";

type Feedback = { onError: (error: unknown) => void; onMessage: (message: string) => void };

const blankProduct: ProductForm = {
	slug: "",
	custom_path: "",
	part_number: "",
  name: "",
  category_id: "cat_uncategorized",
  description: "",
  specification: "",
  document_url: "",
  status: "hidden",
};

export function CatalogPanel({ onError, onMessage }: Feedback) {
  const [products, setProducts] = useState<Product[]>([]);
  const [categories, setCategories] = useState<Category[]>([]);
  const [manufacturers, setManufacturers] = useState<DictionaryEntry[]>([]);
  const [brands, setBrands] = useState<DictionaryEntry[]>([]);
	const [applications, setApplications] = useState<DictionaryEntry[]>([]);
	const [lifecycles, setLifecycles] = useState<DictionaryEntry[]>([]);
  const [includeArchived, setIncludeArchived] = useState(false);
  const [loading, setLoading] = useState(false);
  const [exporting, setExporting] = useState(false);
  const [editing, setEditing] = useState<Product>();
  const [dataProduct, setDataProduct] = useState<Product>();
  const [formOpen, setFormOpen] = useState(false);
  const [form] = Form.useForm<ProductForm>();

  const exportProducts = async () => {
    setExporting(true);
    try {
      await downloadFile("/admin/api/exports/products.xlsx", "prods-products.xlsx");
      onMessage("Product export downloaded.");
    } catch (error) {
      onError(error);
    } finally {
      setExporting(false);
    }
  };

  const load = async () => {
    setLoading(true);
    try {
		const [productRows, categoryRows, manufacturerRows, brandRows, applicationRows, lifecycleRows] = await Promise.all([
        api<Product[]>(`/admin/api/products?include_archived=${includeArchived}`),
        api<Category[]>("/admin/api/categories"),
        api<DictionaryEntry[]>("/admin/api/dictionaries?kind=manufacturer"),
			api<DictionaryEntry[]>("/admin/api/dictionaries?kind=brand"),
			api<DictionaryEntry[]>("/admin/api/dictionaries?kind=application"),
			api<DictionaryEntry[]>("/admin/api/dictionaries?kind=lifecycle"),
      ]);
      setProducts(productRows ?? []);
      setCategories(categoryRows ?? []);
      setManufacturers(manufacturerRows ?? []);
		setBrands(brandRows ?? []);
		setApplications(applicationRows ?? []);
		setLifecycles(lifecycleRows ?? []);
    } catch (error) {
      onError(error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [includeArchived]);

  const categoryOptions = useMemo(
    () => categories.filter((item) => item.status === "active").map((item) => ({ value: item.id, label: item.name })),
    [categories],
  );
  const dictionaryOptions = (rows: DictionaryEntry[]) =>
    rows.filter((item) => item.status === "active").map((item) => ({ value: item.id, label: item.name }));

  const beginCreate = () => {
    setEditing(undefined);
    form.setFieldsValue(blankProduct);
    setFormOpen(true);
  };

  const beginEdit = (product: Product) => {
    setEditing(product);
	form.setFieldsValue({
		slug: product.slug,
		custom_path: product.custom_path,
		part_number: product.part_number,
      name: product.name,
      manufacturer_id: product.manufacturer_id,
      manufacturer: product.manufacturer,
      brand_id: product.brand_id,
		brand: product.brand,
		application_ids: product.application_ids ?? [],
      lifecycle_id: product.lifecycle_id,
      category_id: product.category_id,
      package_form_factor: product.package_form_factor,
      description: product.description,
      features: product.features,
      specification: product.specification,
      document_url: product.document_url,
      status: product.status,
    });
    setFormOpen(true);
  };

	const save = async (values: ProductForm) => {
    try {
		if (editing) {
		  let updated = await putJSON<Product>(`/admin/api/products/${editing.id}`, {
			expected_revision: editing.revision,
			product: { ...editing, ...values },
		  });
		  if (values.slug !== editing.slug || (values.custom_path ?? "") !== (editing.custom_path ?? "")) {
			updated = await postJSON<Product>(`/admin/api/products/${editing.id}/url`, {
			  expected_revision: updated.revision,
			  slug: values.slug,
			  custom_path: values.custom_path ?? "",
			});
		  }
        onMessage(`Saved ${updated.part_number} revision ${updated.revision}.`);
      } else {
        const created = await postJSON<Product>("/admin/api/products", values);
        onMessage(`Created ${created.part_number} as Hidden.`);
      }
      setEditing(undefined);
      setFormOpen(false);
      form.resetFields();
      await load();
    } catch (error) {
      onError(error);
      await load();
    }
	};

	const previewUnsaved = async () => {
		let previewWindow: Window | null = null;
		try {
			const values = await form.validateFields();
			previewWindow = window.open("about:blank", "_blank");
			const receipt = await postJSON<{ url: string }>("/admin/api/previews/products", {
				expected_revision: editing?.revision ?? 0,
				product: editing ? { ...editing, ...values } : values,
			});
			if (previewWindow) {
				previewWindow.location.assign(receipt.url);
			} else {
				window.open(receipt.url, "_blank", "noopener,noreferrer");
			}
		} catch (error) {
			previewWindow?.close();
			onError(error);
		}
	};

  const mutate = async (product: Product, action: "hide" | "archive") => {
    try {
      await postJSON<void>(`/admin/api/products/${product.id}/${action}`, { expected_revision: product.revision });
      onMessage(`${product.part_number} ${action === "hide" ? "is now Hidden" : "was archived"}.`);
      await load();
    } catch (error) {
      onError(error);
      await load();
    }
  };

  return (
    <Space direction="vertical" size="large" className="panel-stack">
      <Card
        title="Products"
        extra={
          <Space wrap>
            <Checkbox checked={includeArchived} onChange={(event) => setIncludeArchived(event.target.checked)}>
              Include archived
            </Checkbox>
            <Button loading={exporting} onClick={() => void exportProducts()}>Export XLSX</Button>
            <Button onClick={() => void load()}>Refresh</Button>
            <Button type="primary" onClick={beginCreate}>New product</Button>
          </Space>
        }
      >
        <Table<Product>
          rowKey="id"
          loading={loading}
          dataSource={products}
          pagination={{ pageSize: 20, hideOnSinglePage: true }}
          scroll={{ x: 880 }}
          columns={[
            { title: "Part number", dataIndex: "part_number", sorter: (a, b) => a.part_number.localeCompare(b.part_number) },
            { title: "Name", dataIndex: "name", render: (value: string) => value || <Typography.Text type="secondary">—</Typography.Text> },
            { title: "Manufacturer", render: (_, row) => row.manufacturer || row.manufacturer_id || "—" },
            { title: "Category", render: (_, row) => categories.find((item) => item.id === row.category_id)?.name ?? row.category_id },
            {
              title: "State",
              render: (_, row) => (
                <Space>
                  <Tag color={row.record_state === "archived" ? "default" : "blue"}>{row.record_state}</Tag>
                  <Tag color={row.status === "published" ? "green" : "gold"}>{row.status}</Tag>
                  <Typography.Text type="secondary">r{row.revision}</Typography.Text>
                </Space>
              ),
            },
            {
              title: "Actions",
              fixed: "right",
              render: (_, row) => (
                <Space>
                  <Button size="small" disabled={row.record_state === "archived"} onClick={() => beginEdit(row)}>Edit</Button>
                  <Button size="small" disabled={row.record_state === "archived"} onClick={() => setDataProduct(row)}>Data</Button>
                  {row.record_state !== "archived" && row.status === "published" && (
                    <Popconfirm title="Hide this product from new public requests?" onConfirm={() => void mutate(row, "hide")}>
                      <Button size="small">Hide</Button>
                    </Popconfirm>
                  )}
                  {row.record_state !== "archived" && (
                    <Popconfirm title="Archive this product? Archived records are read-only." onConfirm={() => void mutate(row, "archive")}>
                      <Button size="small" danger>Archive</Button>
                    </Popconfirm>
                  )}
                </Space>
              ),
            },
          ]}
        />
      </Card>

      <Modal
        open={formOpen}
        title={editing ? `Edit ${editing.part_number}` : "New product"}
        okText={editing ? "Save" : "Create Hidden product"}
        onOk={() => form.submit()}
        onCancel={() => { setEditing(undefined); setFormOpen(false); form.resetFields(); }}
        width={760}
        destroyOnHidden
      >
        <Form form={form} layout="vertical" onFinish={(values) => void save(values)} initialValues={blankProduct}>
		  <div className="form-grid">
			<Form.Item name="part_number" label="Part number" rules={[{ required: true }]}><Input /></Form.Item>
			<Form.Item name="slug" label="URL slug" extra={editing ? "Changing this changes the public URL." : "Leave blank to generate a stable suggestion once."}><Input placeholder="auto-generated" /></Form.Item>
			<Form.Item name="custom_path" label="Custom public path" extra="Optional full internal path; it stays fixed when the global pattern changes."><Input placeholder="/featured/example" /></Form.Item>
            <Form.Item name="name" label="Product name"><Input /></Form.Item>
            <Form.Item name="manufacturer_id" label="Manufacturer"><Select allowClear showSearch optionFilterProp="label" options={dictionaryOptions(manufacturers)} /></Form.Item>
			<Form.Item name="brand_id" label="Brand"><Select allowClear showSearch optionFilterProp="label" options={dictionaryOptions(brands)} /></Form.Item>
			<Form.Item name="application_ids" label="Applications"><Select mode="multiple" allowClear showSearch optionFilterProp="label" options={dictionaryOptions(applications)} /></Form.Item>
			<Form.Item name="lifecycle_id" label="Lifecycle"><Select allowClear showSearch optionFilterProp="label" options={dictionaryOptions(lifecycles)} /></Form.Item>
            <Form.Item name="category_id" label="Category" rules={[{ required: true }]}><Select showSearch optionFilterProp="label" options={categoryOptions} /></Form.Item>
            <Form.Item name="package_form_factor" label="Package / form factor"><Input /></Form.Item>
            <Form.Item name="status" label="Visibility" rules={[{ required: true }]}>
              <Select disabled={editing?.status === "published"} options={[{ value: "hidden", label: "Hidden" }, { value: "published", label: "Published" }]} />
            </Form.Item>
            <Form.Item name="document_url" label="Legacy document URL"><Input type="url" /></Form.Item>
          </div>
          <Form.Item name="description" label="Description"><Input.TextArea rows={3} /></Form.Item>
          <Form.Item name="features" label="Features"><Input.TextArea rows={3} /></Form.Item>
		  <Form.Item name="specification" label="Specification"><Input.TextArea rows={3} /></Form.Item>
		  <Button onClick={() => void previewUnsaved()}>Preview unsaved changes</Button>
		</Form>
      </Modal>
      <ProductDataModal
        product={dataProduct}
        open={dataProduct !== undefined}
        onClose={() => setDataProduct(undefined)}
        onChanged={load}
        onError={onError}
        onMessage={onMessage}
      />
    </Space>
  );
}
