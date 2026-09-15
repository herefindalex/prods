import { useCallback, useEffect, useState } from "react";
import { Alert, Button, Card, Progress, Space, Table, Tag, Typography } from "antd";
import { api, postJSON } from "./api";
import type { AdminJob, AdminJobsResponse } from "./types";

type Props = {
  locale: "en-US" | "zh-TW";
  onError: (error: unknown) => void;
  onMessage: (message: string) => void;
};

const activeStatuses = new Set(["pending", "processing", "queued", "parsing", "committing", "running", "sending"]);

function statusColor(status: string): string {
  if (["failed", "unknown", "partial"].includes(status)) return "red";
  if (["completed", "committed", "succeeded", "accepted"].includes(status)) return "green";
  if (["superseded", "cancelled", "interrupted"].includes(status)) return "default";
  return "blue";
}

function jobProgress(job: AdminJob) {
  if (!job.progress_total || job.progress_total < 1) return "—";
  const percent = Math.min(100, Math.round(((job.progress_current ?? 0) / job.progress_total) * 100));
  return <Progress percent={percent} size="small" status={job.status === "failed" ? "exception" : undefined} />;
}

export function JobsPanel({ locale, onError, onMessage }: Props) {
  const [response, setResponse] = useState<AdminJobsResponse>();
  const [loading, setLoading] = useState(false);
  const text = locale === "zh-TW" ? {
    title: "背景工作",
    refresh: "重新整理",
      policy: "公開生成只重試仍指向相同 Current／Published revision 的工作；搜尋提交可重試，但 accepted 只代表服務已接收，不代表已收錄。Import commit、SMTP 不確定結果、Restore 與 Migration 不會自動重試。",
    retry: "重試",
      retryQueued: "已排入安全重試。",
  } : {
    title: "Background jobs",
    refresh: "Refresh",
      policy: "Publication retries remain tied to the same Current, Published revision. Search submissions may be retried, but accepted means received—not indexed. Import commits, uncertain SMTP outcomes, restores, and migrations are never retried automatically.",
    retry: "Retry",
      retryQueued: "A safe retry was queued.",
  };

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      setResponse(await api<AdminJobsResponse>("/admin/api/jobs?limit=100"));
    } catch (error) {
      onError(error);
    } finally {
      setLoading(false);
    }
  }, [onError]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    if (!response?.jobs.some((job) => activeStatuses.has(job.status))) return;
    const timer = window.setInterval(() => void refresh(), 2000);
    return () => window.clearInterval(timer);
  }, [refresh, response]);

  const retry = async (job: AdminJob) => {
    try {
      const kind = job.kind === "search_submission" ? "search" : "publication";
      await postJSON<AdminJob>(`/admin/api/jobs/${kind}/${encodeURIComponent(job.id)}/retry`, {});
      onMessage(text.retryQueued);
      await refresh();
    } catch (error) {
      onError(error);
      await refresh();
    }
  };

  return (
    <Space direction="vertical" size="large" className="panel-stack">
      <Alert showIcon type="info" message={text.policy} />
      <Card
        title={text.title}
        extra={<Button loading={loading} onClick={() => void refresh()}>{text.refresh}</Button>}
      >
        <Table<AdminJob>
          rowKey="id"
          loading={loading && !response}
          dataSource={response?.jobs ?? []}
          pagination={{ pageSize: 20, hideOnSinglePage: true }}
          expandable={{
            rowExpandable: (job) => Boolean(job.error_message || job.outputs?.length),
            expandedRowRender: (job) => (
              <Space direction="vertical" className="panel-stack">
                {job.error_message && <Alert showIcon type="error" message={job.error_message} />}
                {job.outputs?.length ? (
                  <Space wrap>
                    <Typography.Text type="secondary">Outputs:</Typography.Text>
                    {job.outputs.map((output) => <Tag key={output}>{output}</Tag>)}
                  </Space>
                ) : null}
              </Space>
            ),
          }}
          columns={[
            { title: "Type", dataIndex: "kind", render: (kind: string) => <Tag>{kind}</Tag> },
            {
              title: "Job / target",
              render: (_, job) => (
                <Space direction="vertical" size={0}>
                  <Typography.Text code>{job.id}</Typography.Text>
                  <Typography.Text type="secondary">
                    {job.target_type && job.target_id ? `${job.target_type}: ${job.target_id}` : job.label || "—"}
                    {job.desired_revision ? ` · revision ${job.desired_revision}` : ""}
                  </Typography.Text>
                </Space>
              ),
            },
            { title: "Status", dataIndex: "status", render: (status: string) => <Tag color={statusColor(status)}>{status}</Tag> },
            { title: "Stage", dataIndex: "stage" },
            { title: "Progress", width: 150, render: (_, job) => jobProgress(job) },
            { title: "Updated", dataIndex: "updated_at", render: (value: string) => new Date(value).toLocaleString(locale) },
            {
              title: "Action",
              render: (_, job) => (job.kind === "publication" || job.kind === "search_submission") && job.retryable
                ? <Button onClick={() => void retry(job)}>{text.retry}</Button>
                : "—",
            },
          ]}
        />
      </Card>
    </Space>
  );
}
