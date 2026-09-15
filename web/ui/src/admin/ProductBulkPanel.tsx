import { useEffect, useState, type Key } from "react";
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Form,
  Popconfirm,
  Select,
  Space,
  Table,
  Tag,
  Typography,
} from "antd";
import { api, postJSON } from "./api";
import type {
  Category,
  DictionaryEntry,
  Product,
  ProductBulkAction,
  ProductBulkPlanItem,
  ProductBulkReceipt,
  ProductBulkRun,
} from "./types";

type Props = {
  locale: "en-US" | "zh-TW";
  onError: (error: unknown) => void;
  onMessage: (message: string) => void;
};

const labels = {
  "en-US": {
    title: "Product Bulk Operations",
    help: "Preflight freezes the selection and each Product revision. Execution rechecks and commits each Product atomically; other successful Products remain when one conflicts.",
    selection: "Selected Products",
    action: "Operation",
    target: "Target",
    preflight: "Preflight fixed selection",
    execute: "Execute reviewed plan",
    confirm: "Execute this fixed preflight plan? Each Product will be revalidated.",
    partNumber: "Part number",
    state: "State",
    revision: "Revision",
    eligible: "Eligible",
    result: "Result",
    message: "Message",
    yes: "Yes",
    no: "No",
    selectAtLeastOne: "Select at least one Product.",
    noUnresolved: "There are no unresolved Products to retry.",
    previewSummary: (eligible: number, total: number) => `${eligible} of ${total} Products are eligible in this fixed preflight.`,
    done: (receipt: ProductBulkReceipt) =>
      `Bulk completed: ${receipt.succeeded} succeeded, ${receipt.no_change} unchanged, ${receipt.conflicts} conflicts, ${receipt.invalid} invalid, ${receipt.failed} failed.`,
    retryUnresolved: "Preflight unresolved only",
    actions: {
      publish: "Publish",
      hide: "Hide",
      archive: "Archive",
      change_category: "Change category",
      change_lifecycle: "Change lifecycle",
    } as Record<ProductBulkAction, string>,
  },
  "zh-TW": {
    title: "Product 批次操作",
    help: "預檢會固定選取範圍與每個 Product 修訂。執行時逐項重驗並以單一 Product 原子提交；某項衝突不會回滾其他成功項目。",
    selection: "已選 Product",
    action: "操作",
    target: "目標",
    preflight: "固定選取並預檢",
    execute: "執行已檢視計畫",
    confirm: "執行這份固定預檢計畫？每個 Product 都會重新驗證。",
    partNumber: "料號",
    state: "狀態",
    revision: "修訂",
    eligible: "可執行",
    result: "結果",
    message: "訊息",
    yes: "是",
    no: "否",
    selectAtLeastOne: "請至少選取一個 Product。",
    noUnresolved: "沒有需要重試的未解決 Product。",
    previewSummary: (eligible: number, total: number) => `固定預檢中 ${total} 個 Product 有 ${eligible} 個可執行。`,
    done: (receipt: ProductBulkReceipt) =>
      `批次完成：成功 ${receipt.succeeded}、無需變更 ${receipt.no_change}、衝突 ${receipt.conflicts}、無效 ${receipt.invalid}、失敗 ${receipt.failed}。`,
    retryUnresolved: "只重新預檢未解決項目",
    actions: {
      publish: "發布",
      hide: "隱藏",
      archive: "封存",
      change_category: "變更分類",
      change_lifecycle: "變更生命週期",
    } as Record<ProductBulkAction, string>,
  },
} as const;

type PreflightValues = { action: ProductBulkAction; target_id?: string };

function isUnresolved(status: string): boolean {
  return status === "conflict" || status === "invalid" || status === "failed";
}

export function ProductBulkPanel({ locale, onError, onMessage }: Props) {
  const text = labels[locale];
  const [products, setProducts] = useState<Product[]>([]);
  const [categories, setCategories] = useState<Category[]>([]);
  const [lifecycles, setLifecycles] = useState<DictionaryEntry[]>([]);
  const [selected, setSelected] = useState<Key[]>([]);
  const [run, setRun] = useState<ProductBulkRun>();
  const [busy, setBusy] = useState(false);
  const [form] = Form.useForm<PreflightValues>();
  const action = Form.useWatch("action", form);

  const load = async () => {
    try {
      const [productRows, categoryRows, lifecycleRows] = await Promise.all([
        api<Product[]>("/admin/api/products?include_archived=1"),
        api<Category[]>("/admin/api/categories"),
        api<DictionaryEntry[]>("/admin/api/dictionaries?kind=lifecycle"),
      ]);
      setProducts(productRows ?? []);
      setCategories((categoryRows ?? []).filter((item) => item.status === "active"));
      setLifecycles((lifecycleRows ?? []).filter((item) => item.status === "active"));
    } catch (error) {
      onError(error);
    }
  };

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const preflight = async (values: PreflightValues, productIDs = selected.map(String)) => {
    if (productIDs.length === 0) {
      onError(new Error(text.selectAtLeastOne));
      return;
    }
    setBusy(true);
    try {
      setRun(await postJSON<ProductBulkRun>("/admin/api/product-bulk/preflight", {
        product_ids: productIDs,
        action: values.action,
        target_id: values.target_id ?? "",
      }));
    } catch (error) {
      onError(error);
    } finally {
      setBusy(false);
    }
  };

  const execute = async () => {
    if (!run) return;
    setBusy(true);
    try {
      const receipt = await postJSON<ProductBulkReceipt>(`/admin/api/product-bulk/${run.preview.run_id}/execute`, {});
      setRun({ ...run, status: "completed", receipt });
      onMessage(text.done(receipt));
      await load();
    } catch (error) {
      onError(error);
      try {
        setRun(await api<ProductBulkRun>(`/admin/api/product-bulk/${run.preview.run_id}`));
      } catch {
        // Keep the last durable plan visible if its status cannot be refreshed.
      }
    } finally {
      setBusy(false);
    }
  };

  const retryUnresolved = async () => {
    if (!run?.receipt) return;
    const productIDs = run.receipt.results.filter((item) => isUnresolved(item.status)).map((item) => item.product_id);
    if (productIDs.length === 0) {
      onError(new Error(text.noUnresolved));
      return;
    }
    setSelected(productIDs);
    await preflight({ action: run.preview.action, target_id: run.preview.target_id }, productIDs);
  };

  const requiresTarget = action === "change_category" || action === "change_lifecycle";
  const targetOptions = action === "change_category"
    ? categories.map((item) => ({ value: item.id, label: item.name }))
    : lifecycles.map((item) => ({ value: item.id, label: item.name }));
  const unresolved = run?.receipt?.results.filter((item) => isUnresolved(item.status)) ?? [];

  return (
    <Space direction="vertical" size="large" className="panel-stack">
      <Alert showIcon type="info" message={text.title} description={text.help} />
      <Card title={text.selection}>
        <Table<Product>
          size="small"
          rowKey="id"
          dataSource={products}
          pagination={{ pageSize: 20 }}
          rowSelection={{ selectedRowKeys: selected, onChange: setSelected }}
          columns={[
            { title: text.partNumber, dataIndex: "part_number" },
            { title: text.state, render: (_, item) => `${item.record_state} / ${item.status}` },
            { title: text.revision, dataIndex: "revision", width: 100 },
          ]}
        />
        <Form form={form} layout="inline" initialValues={{ action: "publish" }} onFinish={(values) => void preflight(values)}>
          <Form.Item name="action" label={text.action} rules={[{ required: true }]}>
            <Select
              style={{ width: 210 }}
              options={(Object.keys(text.actions) as ProductBulkAction[]).map((value) => ({ value, label: text.actions[value] }))}
              onChange={() => form.setFieldValue("target_id", undefined)}
            />
          </Form.Item>
          {requiresTarget && (
            <Form.Item name="target_id" label={text.target} rules={[{ required: true }]}>
              <Select showSearch optionFilterProp="label" style={{ width: 240 }} options={targetOptions} />
            </Form.Item>
          )}
          <Button htmlType="submit" type="primary" loading={busy}>{text.preflight}</Button>
        </Form>
      </Card>
      {run && (
        <Card
          title={`${text.actions[run.preview.action]} · ${run.preview.run_id}`}
          extra={run.status !== "completed" && (
            <Popconfirm title={text.confirm} onConfirm={() => void execute()}>
              <Button type="primary" loading={busy}>{text.execute}</Button>
            </Popconfirm>
          )}
        >
          <Space direction="vertical" size="middle" className="panel-stack">
            <Descriptions size="small" column={{ xs: 1, sm: 3 }}>
              <Descriptions.Item label={text.selection}>{run.preview.selection_count}</Descriptions.Item>
              <Descriptions.Item label={text.eligible}>{run.preview.eligible_count}</Descriptions.Item>
              <Descriptions.Item label={text.state}>{run.status}</Descriptions.Item>
            </Descriptions>
            <Typography.Text>{text.previewSummary(run.preview.eligible_count, run.preview.selection_count)}</Typography.Text>
            {run.preview.warning && <Alert type="warning" showIcon message={run.preview.warning} />}
            <Table<ProductBulkPlanItem>
              size="small"
              rowKey="product_id"
              dataSource={run.preview.items}
              pagination={false}
              columns={[
                { title: text.partNumber, dataIndex: "part_number" },
                { title: text.revision, dataIndex: "expected_revision" },
                { title: text.state, render: (_, item) => `${item.record_state} / ${item.publishing_state}` },
                { title: text.eligible, render: (_, item) => <Tag color={item.eligible ? "green" : "red"}>{item.eligible ? text.yes : text.no}</Tag> },
                { title: text.message, dataIndex: "message" },
              ]}
            />
            {run.receipt && (
              <>
                <Table
                  size="small"
                  rowKey="product_id"
                  dataSource={run.receipt.results}
                  pagination={false}
                  columns={[
                    { title: text.partNumber, dataIndex: "part_number" },
                    { title: text.result, dataIndex: "status", render: (value: string) => <Tag color={value === "succeeded" ? "green" : isUnresolved(value) ? "red" : "blue"}>{value}</Tag> },
                    { title: text.revision, dataIndex: "result_revision" },
                    { title: text.message, dataIndex: "message" },
                  ]}
                />
                <Button disabled={unresolved.length === 0} onClick={() => void retryUnresolved()}>
                  {text.retryUnresolved} ({unresolved.length})
                </Button>
              </>
            )}
          </Space>
        </Card>
      )}
    </Space>
  );
}
