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

const targetOptions = [
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
].map((value) => ({ value, label: value.replaceAll("_", " ") }));

const activeStatuses = new Set(["queued", "parsing", "committing"]);

export function ImportPanel({ onError, onMessage }: Feedback) {
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
      onError(new Error("Choose an .xlsx file before starting the import."));
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
      onMessage(`Import ${created.id} was accepted for validation.`);
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
      onMessage(`Import committed: ${receipt.created} created, ${receipt.updated} updated, ${receipt.no_change} unchanged.`);
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
      onMessage(`Cancellation requested for import ${job.id}.`);
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
        message="Atomic XLSX import"
        description="Validation scans every row and produces the complete error report. Commit rechecks identity, references, and revisions before one all-or-nothing transaction."
      />
      <Card title="Upload and mapping">
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
            <Form.Item label="Workbook">
              <Upload
                accept=".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
                maxCount={1}
                beforeUpload={(nextFile) => { setFile(nextFile); return false; }}
                onRemove={() => { setFile(undefined); }}
              >
                <Button>Choose .xlsx</Button>
              </Upload>
            </Form.Item>
            <Form.Item name="sheet" label="Sheet"><Input placeholder="First sheet when blank" /></Form.Item>
            <Form.Item name="header_row" label="Header row" rules={[{ required: true }]}><InputNumber min={1} precision={0} /></Form.Item>
            <Form.Item name="identity_mode" label="Identity mode" rules={[{ required: true }]}>
              <Select options={[
                { value: "part_number", label: "Part number" },
                { value: "manufacturer_part_number", label: "Manufacturer + part number" },
              ]} />
            </Form.Item>
          </div>
          <Form.List name="mappings">
            {(fields, { add, remove }) => (
              <Space direction="vertical" className="panel-stack">
                {fields.map((field) => (
                  <Space key={field.key} wrap align="baseline">
                    <Form.Item {...field} name={[field.name, "source_index"]} label="Column index" rules={[{ required: true }]}><InputNumber min={0} precision={0} /></Form.Item>
                    <Form.Item {...field} name={[field.name, "source_name"]} label="Header name" rules={[{ required: true }]}><Input /></Form.Item>
                    <Form.Item {...field} name={[field.name, "target"]} label="Product field" rules={[{ required: true }]}><Select options={targetOptions} style={{ width: 190 }} /></Form.Item>
                    <Button danger onClick={() => remove(field.name)}>Remove</Button>
                  </Space>
                ))}
                <Button onClick={() => add({ source_index: fields.length, source_name: "", target: "" })}>Add mapping</Button>
              </Space>
            )}
          </Form.List>
          <Button type="primary" htmlType="submit" loading={submitting} disabled={!file} className="top-gap">Validate workbook</Button>
        </Form>
      </Card>

      {job && (
        <Card
          title={`Import ${job.id}`}
          extra={
            <Space>
              <Button onClick={() => void refreshJob()}>Refresh</Button>
              {activeStatuses.has(job.status) && <Button danger onClick={() => void cancel()}>Cancel</Button>}
              {job.status === "preview_ready" && preview?.fully_scanned && preview.fully_validated && (
                <Button type="primary" onClick={() => void commit()}>Commit atomically</Button>
              )}
              {preview && <Button href={`/admin/api/imports/${job.id}/report`} target="_blank">Download error CSV</Button>}
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
              <Tag>rows {preview?.total_rows ?? job.total_rows}</Tag>
              <Tag color="green">create {preview?.create_count ?? job.create_count}</Tag>
              <Tag color="blue">update {preview?.update_count ?? job.update_count}</Tag>
              <Tag>unchanged {preview?.no_change_count ?? job.no_change_count}</Tag>
              <Tag color={(preview?.issues?.length ?? job.failed_count) > 0 ? "red" : "default"}>issues {preview?.issues?.length ?? job.failed_count}</Tag>
            </Space>
            {preview && (
              <Table<ImportIssue>
                rowKey={(issue, index) => `${issue.sheet}-${issue.row}-${issue.column}-${issue.code}-${index}`}
                dataSource={preview.issues ?? []}
                pagination={{ pageSize: 20, hideOnSinglePage: true }}
                columns={[
                  { title: "Sheet", dataIndex: "sheet" },
                  { title: "Row", dataIndex: "row" },
                  { title: "Column", dataIndex: "column" },
                  { title: "Code", dataIndex: "code" },
                  { title: "Message", dataIndex: "message" },
                ]}
              />
            )}
          </Space>
        </Card>
      )}
    </Space>
  );
}
