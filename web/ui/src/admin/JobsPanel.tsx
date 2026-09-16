import { useCallback, useEffect, useState } from "react";
import { Alert, Button, Card, Progress, Space, Table, Tag, Typography } from "antd";
import { api, postJSON } from "./api";
import type { AdminLocale } from "./locales";
import type { AdminJob, AdminJobsResponse } from "./types";

type Props = {
  locale: AdminLocale;
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

const labels = {
  "en-US": {
    title: "Background jobs",
    refresh: "Refresh",
    policy: "Publication retries remain tied to the same Current, Published revision. Search submissions may be retried, but accepted means received—not indexed. Import commits, uncertain SMTP outcomes, restores, and migrations are never retried automatically.",
    retry: "Retry",
    retryQueued: "A safe retry was queued.",
    outputs: "Outputs:",
    type: "Type",
    jobTarget: "Job / target",
    revision: "revision",
    status: "Status",
    stage: "Stage",
    progress: "Progress",
    updated: "Updated",
    action: "Action",
  },
  "zh-TW": {
    title: "背景工作",
    refresh: "重新整理",
    policy: "公開生成只重試仍指向相同 Current／Published revision 的工作；搜尋提交可重試，但 accepted 只代表服務已接收，不代表已收錄。Import commit、SMTP 不確定結果、Restore 與 Migration 不會自動重試。",
    retry: "重試",
    retryQueued: "已排入安全重試。",
    outputs: "輸出：",
    type: "類型",
    jobTarget: "工作／目標",
    revision: "修訂",
    status: "狀態",
    stage: "階段",
    progress: "進度",
    updated: "更新時間",
    action: "操作",
  },
"zh-CN": {
    title: "\u540E\u53F0\u5DE5\u4F5C",
    refresh: "\u5237\u65B0",
    policy: "\u53D1\u5E03\u91CD\u8BD5\u4ECD\u7136\u4E0E\u76F8\u540C\u7684 Current\u3001Published \u7248\u672C\u76F8\u5173\u3002\u641C\u7D22\u63D0\u4EA4\u53EF\u4EE5\u91CD\u8BD5\uFF0C\u4F46\u63A5\u53D7\u610F\u5473\u7740\u5DF2\u6536\u5230\uFF0C\u4F46\u672A\u7F16\u5165\u7D22\u5F15\u3002\u5BFC\u5165\u63D0\u4EA4\u3001\u4E0D\u786E\u5B9A\u7684 SMTP \u7ED3\u679C\u3001\u6062\u590D\u548C\u8FC1\u79FB\u6C38\u8FDC\u4E0D\u4F1A\u81EA\u52A8\u91CD\u8BD5\u3002",
    retry: "\u91CD\u8BD5",
    retryQueued: "\u5B89\u5168\u91CD\u8BD5\u5DF2\u6392\u961F\u3002",
    outputs: "\u8F93\u51FA\uFF1A",
    type: "\u7C7B\u578B",
    jobTarget: "\u5DE5\u4F5C/\u76EE\u6807",
    revision: "\u4FEE\u8BA2",
    status: "\u5730\u4F4D",
    stage: "\u9636\u6BB5",
    progress: "\u8FDB\u6B65",
    updated: "\u5DF2\u66F4\u65B0",
    action: "\u884C\u52A8",
},
"ja-JP": {
    title: "\u30D0\u30C3\u30AF\u30B0\u30E9\u30A6\u30F3\u30C9\u30B8\u30E7\u30D6",
    refresh: "\u30EA\u30D5\u30EC\u30C3\u30B7\u30E5",
    policy: "\u30D1\u30D6\u30EA\u30B1\u30FC\u30B7\u30E7\u30F3\u306E\u518D\u8A66\u884C\u306F\u3001\u540C\u3058 Current\u3001Published \u30EA\u30D3\u30B8\u30E7\u30F3\u306B\u95A2\u9023\u4ED8\u3051\u3089\u308C\u305F\u307E\u307E\u306B\u306A\u308A\u307E\u3059\u3002\u691C\u7D22\u306E\u9001\u4FE1\u306F\u518D\u8A66\u884C\u3067\u304D\u307E\u3059\u304C\u3001\u53D7\u3051\u5165\u308C\u3089\u308C\u305F\u3068\u3044\u3046\u3053\u3068\u306F\u30A4\u30F3\u30C7\u30C3\u30AF\u30B9\u5316\u3055\u308C\u3066\u3044\u306A\u3044\u53D7\u4FE1\u3055\u308C\u305F\u3053\u3068\u3092\u610F\u5473\u3057\u307E\u3059\u3002\u30A4\u30F3\u30DD\u30FC\u30C8\u306E\u30B3\u30DF\u30C3\u30C8\u3001\u4E0D\u78BA\u5B9F\u306A SMTP \u306E\u7D50\u679C\u3001\u5FA9\u5143\u3001\u79FB\u884C\u304C\u81EA\u52D5\u7684\u306B\u518D\u8A66\u884C\u3055\u308C\u308B\u3053\u3068\u306F\u3042\u308A\u307E\u305B\u3093\u3002",
    retry: "\u30EA\u30C8\u30E9\u30A4",
    retryQueued: "\u5B89\u5168\u306A\u518D\u8A66\u884C\u304C\u30AD\u30E5\u30FC\u306B\u5165\u308C\u3089\u308C\u307E\u3057\u305F\u3002",
    outputs: "\u51FA\u529B:",
    type: "\u30BF\u30A4\u30D7",
    jobTarget: "\u4ED5\u4E8B\u30FB\u76EE\u6A19",
    revision: "\u30EA\u30D3\u30B8\u30E7\u30F3",
    status: "\u72B6\u614B",
    stage: "\u30B9\u30C6\u30FC\u30B8",
    progress: "\u9032\u6357",
    updated: "\u66F4\u65B0\u3055\u308C\u307E\u3057\u305F",
    action: "\u30A2\u30AF\u30B7\u30E7\u30F3",
},
"ko-KR": {
    title: "\uBC31\uADF8\uB77C\uC6B4\uB4DC \uC791\uC5C5",
    refresh: "\uC0C8\uB85C \uACE0\uCE58\uB2E4",
    policy: "\uAC8C\uC2DC \uC7AC\uC2DC\uB3C4\uB294 \uB3D9\uC77C\uD55C Current, Published \uAC1C\uC815\uD310\uC5D0 \uACC4\uC18D \uC5F0\uACB0\uB418\uC5B4 \uC788\uC2B5\uB2C8\uB2E4. \uAC80\uC0C9 \uC81C\uCD9C\uC740 \uB2E4\uC2DC \uC2DC\uB3C4\uB420 \uC218 \uC788\uC9C0\uB9CC \uC218\uB77D\uC740 \uC0C9\uC778\uD654\uB418\uC9C0 \uC54A\uC740 \uC218\uC2E0\uC744 \uC758\uBBF8\uD569\uB2C8\uB2E4. \uAC00\uC838\uC624\uAE30 \uCEE4\uBC0B, \uBD88\uD655\uC2E4\uD55C SMTP \uACB0\uACFC, \uBCF5\uC6D0 \uBC0F \uB9C8\uC774\uADF8\uB808\uC774\uC158\uC740 \uC790\uB3D9\uC73C\uB85C \uB2E4\uC2DC \uC2DC\uB3C4\uB418\uC9C0 \uC54A\uC2B5\uB2C8\uB2E4.",
    retry: "\uB2E4\uC2DC \uD574 \uBCF4\uB2E4",
    retryQueued: "\uC548\uC804\uD55C \uC7AC\uC2DC\uB3C4\uAC00 \uB300\uAE30\uC5F4\uC5D0 \uCD94\uAC00\uB418\uC5C8\uC2B5\uB2C8\uB2E4.",
    outputs: "\uCD9C\uB825:",
    type: "\uC720\uD615",
    jobTarget: "\uC9C1\uC5C5/\uB300\uC0C1",
    revision: "\uAC1C\uC815",
    status: "\uC0C1\uD0DC",
    stage: "\uB2E8\uACC4",
    progress: "\uC9C4\uC804",
    updated: "\uC5C5\uB370\uC774\uD2B8\uB428",
    action: "\uD589\uB3D9",
},
"de-DE": {
    title: "Hintergrundjobs",
    refresh: "Aktualisieren",
    policy: "Ver\u00F6ffentlichungswiederholungsversuche bleiben an dieselbe Current-, Published-Revision gebunden. Sucheingaben k\u00F6nnen wiederholt werden, akzeptiert bedeutet jedoch \u201Eempfangen\u201C \u2013 nicht indiziert. Import-Commits, unsichere SMTP-Ergebnisse, Wiederherstellungen und Migrationen werden nie automatisch wiederholt.",
    retry: "Wiederholen",
    retryQueued: "Ein sicherer Wiederholungsversuch wurde in die Warteschlange gestellt.",
    outputs: "Ausg\u00E4nge:",
    type: "Typ",
    jobTarget: "Auftrag / Ziel",
    revision: "Revision",
    status: "Status",
    stage: "B\u00FChne",
    progress: "Fortschritt",
    updated: "Aktualisiert",
    action: "Aktion",
},
"fr-FR": {
    title: "Travaux en arri\u00E8re-plan",
    refresh: "Rafra\u00EEchir",
    policy: "Les tentatives de publication restent li\u00E9es \u00E0 la m\u00EAme r\u00E9vision Current, Published. Les soumissions de recherche peuvent \u00EAtre r\u00E9essay\u00E9es, mais accept\u00E9es signifie re\u00E7ues et non index\u00E9es. Les validations d\u2019importation, les r\u00E9sultats incertains de SMTP, les restaurations et les migrations ne sont jamais r\u00E9essay\u00E9s automatiquement.",
    retry: "R\u00E9essayer",
    retryQueued: "Une nouvelle tentative s\u00E9curis\u00E9e a \u00E9t\u00E9 mise en file d'attente.",
    outputs: "Sorties\u00A0:",
    type: "Taper",
    jobTarget: "Emploi/cible",
    revision: "r\u00E9vision",
    status: "Statut",
    stage: "Sc\u00E8ne",
    progress: "Progr\u00E8s",
    updated: "Mis \u00E0 jour",
    action: "Action",
},
"it-IT": {
    title: "Lavori in background",
    refresh: "Aggiorna",
    policy: "I tentativi di pubblicazione rimangono legati alla stessa revisione Current, Published. Gli invii di ricerca possono essere ritentati, ma accettati significa ricevuti, non indicizzati. I commit di importazione, i risultati SMTP incerti, i ripristini e le migrazioni non vengono mai ritentati automaticamente.",
    retry: "Riprova",
    retryQueued: "\u00C8 stato messo in coda un nuovo tentativo sicuro.",
    outputs: "Uscite:",
    type: "Tipo",
    jobTarget: "Lavoro/obiettivo",
    revision: "revisione",
    status: "Stato",
    stage: "Palcoscenico",
    progress: "Progressi",
    updated: "Aggiornato",
    action: "Azione",
},
"es-ES": {
    title: "Trabajos en segundo plano",
    refresh: "Refrescar",
    policy: "Los reintentos de publicaci\u00F3n permanecen vinculados a la misma revisi\u00F3n Current, Published. Los env\u00EDos de b\u00FAsqueda se pueden volver a intentar, pero aceptado significa recibido, no indexado. Las confirmaciones de importaci\u00F3n, los resultados inciertos de SMTP, las restauraciones y las migraciones nunca se reintentan autom\u00E1ticamente.",
    retry: "Rever",
    retryQueued: "Se puso en cola un reintento seguro.",
    outputs: "Salidas:",
    type: "Tipo",
    jobTarget: "Trabajo / objetivo",
    revision: "revisi\u00F3n",
    status: "Estado",
    stage: "Escenario",
    progress: "Progreso",
    updated: "Actualizado",
    action: "Acci\u00F3n",
},
"pt-BR": {
    title: "Trabalhos em segundo plano",
    refresh: "Atualizar",
    policy: "As novas tentativas de publica\u00E7\u00E3o permanecem vinculadas \u00E0 mesma revis\u00E3o Current, Published. Os envios de pesquisa podem ser repetidos, mas aceitos significam recebidos \u2013 n\u00E3o indexados. Confirma\u00E7\u00F5es de importa\u00E7\u00E3o, resultados SMTP incertos, restaura\u00E7\u00F5es e migra\u00E7\u00F5es nunca s\u00E3o repetidas automaticamente.",
    retry: "Tentar novamente",
    retryQueued: "Uma nova tentativa segura foi colocada na fila.",
    outputs: "Resultados:",
    type: "Tipo",
    jobTarget: "Trabalho/alvo",
    revision: "revis\u00E3o",
    status: "Status",
    stage: "Est\u00E1gio",
    progress: "Progresso",
    updated: "Atualizado",
    action: "A\u00E7\u00E3o",
},
};

export function JobsPanel({ locale, onError, onMessage }: Props) {
  const [response, setResponse] = useState<AdminJobsResponse>();
  const [loading, setLoading] = useState(false);
  const text = labels[locale];

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
                    <Typography.Text type="secondary">{text.outputs}</Typography.Text>
                    {job.outputs.map((output) => <Tag key={output}>{output}</Tag>)}
                  </Space>
                ) : null}
              </Space>
            ),
          }}
          columns={[
            { title: text.type, dataIndex: "kind", render: (kind: string) => <Tag>{kind}</Tag> },
            {
              title: text.jobTarget,
              render: (_, job) => (
                <Space direction="vertical" size={0}>
                  <Typography.Text code>{job.id}</Typography.Text>
                  <Typography.Text type="secondary">
                    {job.target_type && job.target_id ? `${job.target_type}: ${job.target_id}` : job.label || "—"}
                    {job.desired_revision ? ` · ${text.revision} ${job.desired_revision}` : ""}
                  </Typography.Text>
                </Space>
              ),
            },
            { title: text.status, dataIndex: "status", render: (status: string) => <Tag color={statusColor(status)}>{status}</Tag> },
            { title: text.stage, dataIndex: "stage" },
            { title: text.progress, width: 150, render: (_, job) => jobProgress(job) },
            { title: text.updated, dataIndex: "updated_at", render: (value: string) => new Date(value).toLocaleString(locale) },
            {
              title: text.action,
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
