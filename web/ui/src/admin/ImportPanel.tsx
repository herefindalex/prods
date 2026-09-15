import { useEffect, useState } from "react";
import { Alert, Button, Card, Form, Input, InputNumber, Progress, Select, Space, Table, Tag, Upload } from "antd";
import { api, postJSON } from "./api";
import type { ImportIssue, ImportJob, ImportJobResponse, ImportMapping, ImportPreview } from "./types";

type Feedback = { onError: (error: unknown) => void; onMessage: (message: string) => void };
type ImportForm = {
  sheet?: string;
  header_row: number;
  identity_mode: "part_number" | "manufacturer_part_number";
  mappings: ImportMapping[];
};

const targets = [
  "part_number",
  "product_name",
  "manufacturer_id",
  "brand_id",
  "category_id",
  "package_form_factor",
  "description",
  "features",
  "lifecycle_id",
	"application_ids",
];

const activeStatuses = new Set(["queued", "parsing", "committing"]);

type ImportLocale = "en-US" | "zh-TW";

const labels = {
  "en-US": {
    accepted: (id: string) => `Import ${id} was accepted for validation.`,
    committed: (created: number, updated: number, unchanged: number) => `Import committed: ${created} created, ${updated} updated, ${unchanged} unchanged.`,
    cancelRequested: (id: string) => `Cancellation requested for import ${id}.`,
    title: "Atomic XLSX import",
    chooseFileFirst: "Choose an .xlsx file before starting the import.",
    description: "Validation scans every row and produces the complete error report. Commit rechecks identity, references, and revisions before one all-or-nothing transaction.",
    uploadMapping: "Upload and mapping", workbook: "Workbook", choose: "Choose .xlsx", sheet: "Sheet",
    sheetPlaceholder: "First sheet when blank", headerRow: "Header row", identityMode: "Identity mode",
    partNumber: "Part number", manufacturerPart: "Manufacturer + part number", columnIndex: "Column index",
    headerName: "Header name", productField: "Product field", remove: "Remove", addMapping: "Add mapping",
    validate: "Validate workbook", importTitle: (id: string) => `Import ${id}`, refresh: "Refresh", cancel: "Cancel",
    commit: "Commit atomically", download: "Download error CSV", rows: "rows", create: "create", update: "update",
    unchanged: "unchanged", issues: "issues", sheetColumn: "Sheet", row: "Row", column: "Column", code: "Code", message: "Message",
    target: {
      part_number: "Part number", product_name: "Product name", manufacturer_id: "Manufacturer",
      brand_id: "Brand", category_id: "Category", package_form_factor: "Package / form factor",
      description: "Description", features: "Features", lifecycle_id: "Lifecycle", application_ids: "Applications",
    } as Record<string, string>,
  },
  "zh-TW": {
    accepted: (id: string) => `匯入工作 ${id} 已受理並開始驗證。`,
    committed: (created: number, updated: number, unchanged: number) => `匯入已提交：新增 ${created} 筆、更新 ${updated} 筆、未變更 ${unchanged} 筆。`,
    cancelRequested: (id: string) => `已要求取消匯入工作 ${id}。`,
    title: "原子 XLSX 匯入",
    chooseFileFirst: "請先選擇 .xlsx 檔案再開始匯入。",
    description: "驗證會掃描每一列並產生完整錯誤報告。提交前會重新檢查識別、參照與修訂，最後以單一全成或全敗交易寫入。",
    uploadMapping: "上傳與欄位映射", workbook: "活頁簿", choose: "選擇 .xlsx", sheet: "工作表",
    sheetPlaceholder: "留空時使用第一個工作表", headerRow: "標題列", identityMode: "識別模式",
    partNumber: "料號", manufacturerPart: "Manufacturer + 料號", columnIndex: "欄位索引",
    headerName: "標題名稱", productField: "產品欄位", remove: "移除", addMapping: "新增映射",
    validate: "驗證活頁簿", importTitle: (id: string) => `匯入 ${id}`, refresh: "重新整理", cancel: "取消",
    commit: "原子提交", download: "下載錯誤 CSV", rows: "列數", create: "新增", update: "更新",
    unchanged: "未變更", issues: "問題", sheetColumn: "工作表", row: "列", column: "欄", code: "代碼", message: "訊息",
    target: {
      part_number: "料號", product_name: "產品名稱", manufacturer_id: "Manufacturer",
      brand_id: "品牌", category_id: "分類", package_form_factor: "封裝／外型",
      description: "描述", features: "特色", lifecycle_id: "生命週期", application_ids: "應用",
    } as Record<string, string>,
  },
};

export function ImportPanel({ locale, onError, onMessage }: Feedback & { locale: ImportLocale }) {
	const text = labels[locale];
	const targetOptions = targets.map((value) => ({ value, label: text.target[value] ?? value }));
  const [file, setFile] = useState<File>();
  const [job, setJob] = useState<ImportJob>();
  const [preview, setPreview] = useState<ImportPreview>();
  const [submitting, setSubmitting] = useState(false);
  const [form] = Form.useForm<ImportForm>();

  const refreshJob = async (jobID = job?.id) => {
    if (!jobID) return;
    try {
      const response = await api<ImportJobResponse>(`/admin/api/imports/${jobID}`);
      setJob(response.job);
      setPreview(response.preview);
    } catch (error) {
      onError(error);
    }
  };

  useEffect(() => {
    if (!job || !activeStatuses.has(job.status)) return;
    const timer = window.setInterval(() => void refreshJob(job.id), 1000);
    return () => window.clearInterval(timer);
  }, [job?.id, job?.status]);

  const upload = async (values: ImportForm) => {
    if (!file) {
      onError(new Error(text.chooseFileFirst));
      return;
    }
    setSubmitting(true);
    try {
      const body = new FormData();
      body.append("template", JSON.stringify({
        sheet: values.sheet || "",
        header_row: values.header_row,
        identity_mode: values.identity_mode,
        mappings: values.mappings,
      }));
      body.append("file", file, file.name);
      const created = await api<ImportJob>("/admin/api/imports", { method: "POST", body });
      setJob(created);
      setPreview(undefined);
			onMessage(text.accepted(created.id));
    } catch (error) {
      onError(error);
    } finally {
      setSubmitting(false);
    }
  };

  const commit = async () => {
    if (!job) return;
    try {
      const receipt = await postJSON<{ created: number; updated: number; no_change: number; replay: boolean }>(
        `/admin/api/imports/${job.id}/commit`,
        {},
      );
		onMessage(text.committed(receipt.created, receipt.updated, receipt.no_change));
      await refreshJob(job.id);
    } catch (error) {
      onError(error);
      await refreshJob(job.id);
    }
  };

  const cancel = async () => {
    if (!job) return;
    try {
      await postJSON<void>(`/admin/api/imports/${job.id}/cancel`, {});
		onMessage(text.cancelRequested(job.id));
      await refreshJob(job.id);
    } catch (error) {
      onError(error);
    }
  };

  const percent = job?.total_rows ? Math.min(100, Math.round((job.checked_rows / job.total_rows) * 100)) : 0;

  return (
    <Space direction="vertical" size="large" className="panel-stack">
      <Alert
        showIcon
        type="info"
        message={text.title}
        description={text.description}
      />
      <Card title={text.uploadMapping}>
        <Form<ImportForm>
          form={form}
          layout="vertical"
          initialValues={{
            header_row: 1,
            identity_mode: "part_number",
            mappings: [
              { source_index: 0, source_name: "Part Number", target: "part_number" },
              { source_index: 1, source_name: "Product Name", target: "product_name" },
            ],
          }}
          onFinish={(values) => void upload(values)}
        >
          <div className="form-grid three-columns">
            <Form.Item label={text.workbook}>
              <Upload
                accept=".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
                maxCount={1}
                beforeUpload={(nextFile) => { setFile(nextFile); return false; }}
                onRemove={() => { setFile(undefined); }}
              >
                <Button>{text.choose}</Button>
              </Upload>
            </Form.Item>
            <Form.Item name="sheet" label={text.sheet}><Input placeholder={text.sheetPlaceholder} /></Form.Item>
            <Form.Item name="header_row" label={text.headerRow} rules={[{ required: true }]}><InputNumber min={1} precision={0} /></Form.Item>
            <Form.Item name="identity_mode" label={text.identityMode} rules={[{ required: true }]}>
              <Select options={[
                { value: "part_number", label: text.partNumber },
                { value: "manufacturer_part_number", label: text.manufacturerPart },
              ]} />
            </Form.Item>
          </div>
          <Form.List name="mappings">
            {(fields, { add, remove }) => (
              <Space direction="vertical" className="panel-stack">
                {fields.map((field) => (
                  <Space key={field.key} wrap align="baseline">
                    <Form.Item {...field} name={[field.name, "source_index"]} label={text.columnIndex} rules={[{ required: true }]}><InputNumber min={0} precision={0} /></Form.Item>
                    <Form.Item {...field} name={[field.name, "source_name"]} label={text.headerName} rules={[{ required: true }]}><Input /></Form.Item>
                    <Form.Item {...field} name={[field.name, "target"]} label={text.productField} rules={[{ required: true }]}><Select options={targetOptions} style={{ width: 190 }} /></Form.Item>
                    <Button danger onClick={() => remove(field.name)}>{text.remove}</Button>
                  </Space>
                ))}
                <Button onClick={() => add({ source_index: fields.length, source_name: "", target: "" })}>{text.addMapping}</Button>
              </Space>
            )}
          </Form.List>
          <Button type="primary" htmlType="submit" loading={submitting} disabled={!file} className="top-gap">{text.validate}</Button>
        </Form>
      </Card>

      {job && (
        <Card
          title={text.importTitle(job.id)}
          extra={
            <Space>
              <Button onClick={() => void refreshJob()}>{text.refresh}</Button>
              {activeStatuses.has(job.status) && <Button danger onClick={() => void cancel()}>{text.cancel}</Button>}
              {job.status === "preview_ready" && preview?.fully_scanned && preview.fully_validated && (
                <Button type="primary" onClick={() => void commit()}>{text.commit}</Button>
              )}
              {preview && <Button href={`/admin/api/imports/${job.id}/report`} target="_blank">{text.download}</Button>}
            </Space>
          }
        >
          <Space direction="vertical" className="panel-stack">
            <Space wrap>
              <Tag color={job.status === "failed" ? "red" : job.status === "committed" ? "green" : "blue"}>{job.status}</Tag>
              <span>{job.phase}</span>
              <span>{job.original_filename}</span>
            </Space>
            {activeStatuses.has(job.status) && <Progress percent={percent} status="active" />}
            {job.error_message && <Alert type="error" message={job.error_message} showIcon />}
            <Space wrap>
              <Tag>{text.rows} {preview?.total_rows ?? job.total_rows}</Tag>
              <Tag color="green">{text.create} {preview?.create_count ?? job.create_count}</Tag>
              <Tag color="blue">{text.update} {preview?.update_count ?? job.update_count}</Tag>
              <Tag>{text.unchanged} {preview?.no_change_count ?? job.no_change_count}</Tag>
              <Tag color={(preview?.issues?.length ?? job.failed_count) > 0 ? "red" : "default"}>{text.issues} {preview?.issues?.length ?? job.failed_count}</Tag>
            </Space>
            {preview && (
              <Table<ImportIssue>
                rowKey={(issue, index) => `${issue.sheet}-${issue.row}-${issue.column}-${issue.code}-${index}`}
                dataSource={preview.issues ?? []}
                pagination={{ pageSize: 20, hideOnSinglePage: true }}
                columns={[
                  { title: text.sheetColumn, dataIndex: "sheet" },
                  { title: text.row, dataIndex: "row" },
                  { title: text.column, dataIndex: "column" },
                  { title: text.code, dataIndex: "code" },
                  { title: text.message, dataIndex: "message" },
                ]}
              />
            )}
          </Space>
        </Card>
      )}
    </Space>
  );
}
