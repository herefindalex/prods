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
} from "antd";
import { api, putJSON } from "./api";
import type {
  PublicCopyDefault,
  PublicCopyDefinition,
  PublicCopyEditorState,
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
            </Descriptions>
            {!currentOverride && <Alert type="info" showIcon message={text.noOverride} />}
            <Typography.Text strong>{text.override}</Typography.Text>
            <Input.TextArea
              autoSize={{ minRows: 3, maxRows: 10 }}
              value={value}
              onChange={(event) => setValue(event.target.value)}
            />
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
    </Space>
  );
}
