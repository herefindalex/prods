import { useEffect, useMemo, useState } from "react";
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Empty,
  Input,
  Popconfirm,
  Select,
  Space,
  Table,
  Tag,
  Typography,
  Upload,
} from "antd";
import { api, downloadFile, postJSON, putJSON } from "./api";
import type {
  PublicCopyDefault,
  PublicCopyDefinition,
  PublicCopyEditorState,
  PublicCopyExchangeChange,
  PublicCopyExchangeIssue,
  PublicCopyExchangePreview,
} from "./types";

type Props = {
  locale: "en-US" | "zh-TW";
  onError: (error: unknown) => void;
  onMessage: (message: string) => void;
};

const labels = {
  "en-US": {
    title: "Public Copy",
    help: "Edit official interface text in the durable Website working copy. Public output does not change until Website Preview and Publish complete.",
    disclaimerTitle: "Translation review status",
    disclaimer: "The ten bundled translations are machine-generated and have not been reviewed by professional native, legal, or marketing reviewers. Review them before production use.",
    workingRevision: "Website working revision",
    locale: "Locale",
    scope: "Scope",
    allScopes: "All scopes",
    search: "Filter by key or description",
    key: "Key",
    description: "Description",
    kind: "Value kind",
    official: "Official default",
    override: "Working override",
    placeholders: "Placeholders",
    sample: "Sample values",
    save: "Save override",
    reset: "Reset this locale",
    resetConfirm: "Reset only this key and locale to the official default?",
    noOverride: "Using official default",
    saved: (key: string, locale: string, revision: number) =>
      `Saved ${key} (${locale}) in Website working revision ${revision}. Preview and Publish are still required.`,
    resetDone: (key: string, locale: string, revision: number) =>
      `Reset ${key} (${locale}) in Website working revision ${revision}. Preview and Publish are still required.`,
    required: "Required",
    allowed: "Allowed",
    none: "None",
    selectEntry: "Select a copy entry to edit.",
    reviewStatus: "Bundle review status",
    bundleVersion: "Official bundle version",
    exchangeTitle: "Translation CSV/XLSX exchange",
    exchangeHelp: "Export a selected locale/scope, edit externally, then validate every row. Import only updates the Website working copy; Preview and Publish remain required. Use action=reset with a blank Value to remove one override.",
    exportCSV: "Export CSV",
    exportXLSX: "Export XLSX",
    chooseExchange: "Choose CSV/XLSX",
    validateExchange: "Validate import",
    commitExchange: "Commit validated changes",
    exchangeDone: (revision: number) => `Translation import committed to Website working revision ${revision}. Preview and Publish are still required.`,
    line: "Line",
    action: "Action",
    before: "Before",
    after: "After",
    code: "Code",
    message: "Message",
    whereUsed: "Where used",
    contextPreview: "Safe context preview",
    desktop: "Desktop",
    mobile: "Mobile",
  },
  "zh-TW": {
    title: "公開介面文案",
    help: "在持久的 Website 工作副本編輯官方介面文字。必須完成 Website 預覽與發布，公開輸出才會改變。",
    disclaimerTitle: "翻譯審閱狀態",
    disclaimer: "內建十種語系為機器產生，尚未經母語、法律或行銷專業人員審閱；正式上線前必須自行審閱。",
    workingRevision: "Website 工作修訂",
    locale: "語系",
    scope: "範圍",
    allScopes: "全部範圍",
    search: "依 key 或描述篩選",
    key: "Key",
    description: "說明",
    kind: "值類型",
    official: "官方預設值",
    override: "工作副本覆寫值",
    placeholders: "佔位符",
    sample: "範例值",
    save: "儲存覆寫",
    reset: "重設此語系",
    resetConfirm: "只將這個 key 與語系重設為官方預設值嗎？",
    noOverride: "目前使用官方預設值",
    saved: (key: string, locale: string, revision: number) =>
      `已將 ${key}（${locale}）儲存到 Website 工作修訂 ${revision}；仍須預覽並發布。`,
    resetDone: (key: string, locale: string, revision: number) =>
      `已在 Website 工作修訂 ${revision} 重設 ${key}（${locale}）；仍須預覽並發布。`,
    required: "必要",
    allowed: "允許",
    none: "無",
    selectEntry: "請選擇一個文案項目進行編輯。",
    reviewStatus: "套件審閱狀態",
    bundleVersion: "官方套件版本",
    exchangeTitle: "翻譯 CSV／XLSX 交換",
    exchangeHelp: "匯出選定語系／範圍，在外部編輯後逐列完整驗證。匯入只更新 Website 工作副本，仍須預覽與發布。要移除單一覆寫時，使用 action=reset 並將 Value 留白。",
    exportCSV: "匯出 CSV",
    exportXLSX: "匯出 XLSX",
    chooseExchange: "選擇 CSV／XLSX",
    validateExchange: "驗證匯入",
    commitExchange: "提交已驗證變更",
    exchangeDone: (revision: number) => `翻譯已提交到 Website 工作修訂 ${revision}；仍須預覽並發布。`,
    line: "列",
    action: "操作",
    before: "原值",
    after: "新值",
    code: "代碼",
    message: "訊息",
    whereUsed: "使用位置",
    contextPreview: "安全情境預覽",
    desktop: "桌面",
    mobile: "行動裝置",
  },
} as const;

function scopeOf(key: string): string {
  return key.includes(".") ? key.slice(0, key.indexOf(".")) : key;
}

function officialDefault(
  defaults: PublicCopyDefault[],
  key: string,
  locale: string,
): PublicCopyDefault | undefined {
  return defaults.find((item) => item.key === key && item.locale === locale);
}

export function PublicCopyPanel({ locale, onError, onMessage }: Props) {
  const text = labels[locale];
  const [state, setState] = useState<PublicCopyEditorState>();
  const [selectedLocale, setSelectedLocale] = useState("en-US");
  const [selectedScope, setSelectedScope] = useState<string>();
  const [filter, setFilter] = useState("");
  const [selectedKey, setSelectedKey] = useState<string>();
  const [value, setValue] = useState("");
  const [saving, setSaving] = useState(false);
  const [exchangeFile, setExchangeFile] = useState<File>();
  const [exchangePreview, setExchangePreview] = useState<PublicCopyExchangePreview>();
  const [exchangeBusy, setExchangeBusy] = useState(false);
  const [previewViewport, setPreviewViewport] = useState<"desktop" | "mobile">("desktop");

  const load = async () => {
    try {
      const next = await api<PublicCopyEditorState>("/admin/api/website/public-copy");
      setState(next);
      setSelectedLocale((current) =>
        next.enabled_locales.includes(current) ? current : next.default_locale,
      );
    } catch (error) {
      onError(error);
    }
  };

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const definitions = state?.catalog.definitions ?? [];
  const scopes = useMemo(
    () => Array.from(new Set(definitions.map((definition) => scopeOf(definition.key)))).sort(),
    [definitions],
  );
  const filteredDefinitions = useMemo(() => {
    const needle = filter.trim().toLocaleLowerCase();
    return definitions.filter(
      (definition) =>
        (!selectedScope || scopeOf(definition.key) === selectedScope) &&
        (!needle ||
          definition.key.toLocaleLowerCase().includes(needle) ||
          definition.description.toLocaleLowerCase().includes(needle)),
    );
  }, [definitions, filter, selectedScope]);
  const selected = definitions.find((definition) => definition.key === selectedKey);

  useEffect(() => {
    if (!state || !selectedKey) return;
    const override = state.overrides[selectedKey]?.[selectedLocale];
    const fallback = officialDefault(state.catalog.defaults, selectedKey, selectedLocale);
    setValue(override?.value ?? fallback?.value ?? "");
  }, [selectedKey, selectedLocale, state]);

  const save = async () => {
    if (!state || !selected) return;
    setSaving(true);
    try {
      const updated = await putJSON<{ working_revision: number }>(
        `/admin/api/website/public-copy/${encodeURIComponent(selected.key)}/${encodeURIComponent(selectedLocale)}`,
        {
          expected_working_revision: state.working_revision,
          value,
          definition_version: selected.definition_version,
        },
      );
      await load();
      onMessage(text.saved(selected.key, selectedLocale, updated.working_revision));
    } catch (error) {
      onError(error);
    } finally {
      setSaving(false);
    }
  };

  const reset = async () => {
    if (!state || !selected) return;
    setSaving(true);
    try {
      const updated = await api<{ working_revision: number }>(
        `/admin/api/website/public-copy/${encodeURIComponent(selected.key)}/${encodeURIComponent(selectedLocale)}`,
        {
          method: "DELETE",
          body: JSON.stringify({ expected_working_revision: state.working_revision }),
        },
      );
      await load();
      onMessage(text.resetDone(selected.key, selectedLocale, updated.working_revision));
    } catch (error) {
      onError(error);
    } finally {
      setSaving(false);
    }
  };

  const currentOverride = selectedKey ? state?.overrides[selectedKey]?.[selectedLocale] : undefined;
  const defaultValue = selectedKey
    ? officialDefault(state?.catalog.defaults ?? [], selectedKey, selectedLocale)?.value
    : undefined;
  const contextValue = Object.entries(selected?.sample ?? {}).reduce(
    (message, [name, sample]) => message.replaceAll(`{${name}}`, sample),
    value,
  );

  const exportExchange = async (format: "csv" | "xlsx") => {
    const query = new URLSearchParams({ locales: selectedLocale });
    if (selectedScope) query.set("scope", selectedScope);
    try {
      await downloadFile(`/admin/api/website/public-copy/export/${format}?${query}`, `prods-public-copy.${format}`);
    } catch (error) {
      onError(error);
    }
  };

  const previewExchange = async () => {
    if (!exchangeFile) return;
    setExchangeBusy(true);
    try {
      const body = new FormData();
      body.append("file", exchangeFile, exchangeFile.name);
      setExchangePreview(await api<PublicCopyExchangePreview>("/admin/api/website/public-copy/import/preview", { method: "POST", body }));
    } catch (error) {
      onError(error);
    } finally {
      setExchangeBusy(false);
    }
  };

  const commitExchange = async () => {
    if (!exchangePreview?.fully_validated) return;
    setExchangeBusy(true);
    try {
      const updated = await postJSON<{ working_revision: number }>("/admin/api/website/public-copy/import/commit", {
        expected_working_revision: exchangePreview.working_revision,
        rows: exchangePreview.rows,
      });
      setExchangePreview(undefined);
      setExchangeFile(undefined);
      await load();
      onMessage(text.exchangeDone(updated.working_revision));
    } catch (error) {
      onError(error);
    } finally {
      setExchangeBusy(false);
    }
  };

  return (
    <Space direction="vertical" size="middle" className="panel-stack">
      <Card title={text.title}>
        <Space direction="vertical" size="middle" className="panel-stack">
          <Typography.Paragraph>{text.help}</Typography.Paragraph>
          <Alert type="warning" showIcon message={text.disclaimerTitle} description={text.disclaimer} />
          {state && (
            <Descriptions size="small" column={{ xs: 1, sm: 3 }}>
              <Descriptions.Item label={text.workingRevision}>{state.working_revision}</Descriptions.Item>
              <Descriptions.Item label={text.bundleVersion}>
                {state.catalog.official_bundle_version}
              </Descriptions.Item>
              <Descriptions.Item label={text.reviewStatus}>{state.catalog.review_status}</Descriptions.Item>
            </Descriptions>
          )}
          <Space wrap>
            <Select
              aria-label={text.locale}
              value={selectedLocale}
              onChange={setSelectedLocale}
              options={(state?.enabled_locales ?? []).map((item) => ({ value: item, label: item }))}
              style={{ minWidth: 150 }}
            />
            <Select
              allowClear
              aria-label={text.scope}
              placeholder={text.allScopes}
              value={selectedScope}
              onChange={setSelectedScope}
              options={scopes.map((item) => ({ value: item, label: item }))}
              style={{ minWidth: 170 }}
            />
            <Input.Search
              allowClear
              aria-label={text.search}
              placeholder={text.search}
              value={filter}
              onChange={(event) => setFilter(event.target.value)}
              style={{ width: 320 }}
            />
          </Space>
          <Table<PublicCopyDefinition>
            size="small"
            rowKey="key"
            dataSource={filteredDefinitions}
            pagination={{ pageSize: 12, showSizeChanger: false }}
            rowSelection={{
              type: "radio",
              selectedRowKeys: selectedKey ? [selectedKey] : [],
              onChange: (keys) => setSelectedKey(String(keys[0] ?? "")),
            }}
            onRow={(record) => ({ onClick: () => setSelectedKey(record.key) })}
            columns={[
              { title: text.key, dataIndex: "key" },
              { title: text.description, dataIndex: "description" },
              {
                title: text.kind,
                dataIndex: "value_kind",
                width: 110,
                render: (kind: string) => <Tag>{kind}</Tag>,
              },
            ]}
          />
        </Space>
      </Card>

      {selected ? (
        <Card title={`${selected.key} · ${selectedLocale}`}>
          <Space direction="vertical" size="middle" className="panel-stack">
            <Descriptions size="small" column={{ xs: 1, sm: 2 }}>
              <Descriptions.Item label={text.description}>{selected.description}</Descriptions.Item>
              <Descriptions.Item label={text.kind}>{selected.value_kind}</Descriptions.Item>
              <Descriptions.Item label={`${text.placeholders} (${text.required})`}>
                {selected.required_placeholders.join(", ") || text.none}
              </Descriptions.Item>
              <Descriptions.Item label={`${text.placeholders} (${text.allowed})`}>
                {selected.allowed_placeholders.join(", ") || text.none}
              </Descriptions.Item>
              <Descriptions.Item label={text.sample} span={2}>
                {selected.sample ? JSON.stringify(selected.sample) : text.none}
              </Descriptions.Item>
              <Descriptions.Item label={text.official} span={2}>
                <Typography.Text code>{defaultValue ?? "—"}</Typography.Text>
              </Descriptions.Item>
              <Descriptions.Item label={text.whereUsed} span={2}>
                {(selected.where_used ?? []).map((usage) => (
                  <div key={`${usage.surface}-${usage.section}`}>
                    <strong>{usage.surface}</strong> · {usage.section} — {usage.purpose}
                    <br /><Typography.Text type="secondary">{usage.safe_context}</Typography.Text>
                  </div>
                ))}
              </Descriptions.Item>
            </Descriptions>
            {!currentOverride && <Alert type="info" showIcon message={text.noOverride} />}
            <Typography.Text strong>{text.override}</Typography.Text>
            <Input.TextArea
              autoSize={{ minRows: 3, maxRows: 10 }}
              value={value}
              onChange={(event) => setValue(event.target.value)}
            />
            <Space>
              <Typography.Text strong>{text.contextPreview}</Typography.Text>
              <Select
                value={previewViewport}
                onChange={setPreviewViewport}
                options={[{ value: "desktop", label: text.desktop }, { value: "mobile", label: text.mobile }]}
              />
            </Space>
            <div
              aria-label={text.contextPreview}
              style={{
                width: previewViewport === "mobile" ? 360 : "100%",
                maxWidth: "100%",
                padding: 16,
                border: "1px solid #d9d9d9",
                borderRadius: 8,
                background: "#fff",
              }}
            >
              <Typography.Text>{contextValue}</Typography.Text>
            </div>
            <Space>
              <Button type="primary" loading={saving} onClick={() => void save()}>
                {text.save}
              </Button>
              <Popconfirm title={text.resetConfirm} onConfirm={() => void reset()}>
                <Button disabled={!currentOverride || saving}>{text.reset}</Button>
              </Popconfirm>
            </Space>
          </Space>
        </Card>
      ) : (
        <Card>
          <Empty description={text.selectEntry} />
        </Card>
      )}

      <Card title={text.exchangeTitle}>
        <Space direction="vertical" size="middle" className="panel-stack">
          <Typography.Paragraph>{text.exchangeHelp}</Typography.Paragraph>
          <Space wrap>
            <Button onClick={() => void exportExchange("csv")}>{text.exportCSV}</Button>
            <Button onClick={() => void exportExchange("xlsx")}>{text.exportXLSX}</Button>
            <Upload
              accept=".csv,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
              maxCount={1}
              beforeUpload={(file) => { setExchangeFile(file); setExchangePreview(undefined); return false; }}
              onRemove={() => { setExchangeFile(undefined); setExchangePreview(undefined); }}
            >
              <Button>{text.chooseExchange}</Button>
            </Upload>
            <Button type="primary" disabled={!exchangeFile} loading={exchangeBusy} onClick={() => void previewExchange()}>
              {text.validateExchange}
            </Button>
            <Button
              type="primary"
              disabled={!exchangePreview?.fully_validated}
              loading={exchangeBusy}
              onClick={() => void commitExchange()}
            >
              {text.commitExchange}
            </Button>
          </Space>
          {exchangePreview && (
            <>
              <Alert
                showIcon
                type={exchangePreview.fully_validated ? "success" : "error"}
                message={exchangePreview.fully_validated
                  ? `${exchangePreview.changes.length} validated change(s)`
                  : `${exchangePreview.issues.length} validation issue(s)`}
              />
              {exchangePreview.issues.length > 0 && (
                <Table<PublicCopyExchangeIssue>
                  size="small"
                  rowKey={(item, index) => `${item.line}-${item.code}-${index}`}
                  dataSource={exchangePreview.issues}
                  pagination={false}
                  columns={[
                    { title: text.line, dataIndex: "line" },
                    { title: text.key, dataIndex: "key" },
                    { title: text.locale, dataIndex: "locale" },
                    { title: text.code, dataIndex: "code" },
                    { title: text.message, dataIndex: "message" },
                  ]}
                />
              )}
              <Table<PublicCopyExchangeChange>
                size="small"
                rowKey={(item, index) => `${item.line}-${item.key}-${item.locale}-${index}`}
                dataSource={exchangePreview.changes}
                pagination={{ pageSize: 20 }}
                columns={[
                  { title: text.line, dataIndex: "line" },
                  { title: text.key, dataIndex: "key" },
                  { title: text.locale, dataIndex: "locale" },
                  { title: text.action, dataIndex: "action" },
                  { title: text.before, dataIndex: "before", ellipsis: true },
                  { title: text.after, dataIndex: "after", ellipsis: true },
                ]}
              />
            </>
          )}
        </Space>
      </Card>
    </Space>
  );
}
