import { useEffect, useState } from "react";
import {
  Alert,
  Button,
  Card,
  Checkbox,
  Descriptions,
  Form,
  Input,
  InputNumber,
  Popconfirm,
  Select,
  Space,
  Table,
  Typography,
  Upload,
} from "antd";
import { api, postJSON, putJSON } from "./api";
import type {
  Asset,
  BrandCaptureFieldChange,
  BrandCaptureResponse,
  SiteConfiguration,
  SiteRouteConfig,
  SiteRouteIssue,
  SiteRoutePreview,
  SiteRouteState,
  SearchIntegrationSettings,
  WebsiteState,
  WebsiteVersion,
} from "./types";

type Props = {
  onError: (error: unknown) => void;
  onMessage: (message: string) => void;
};

export function WebsitePanel({ onError, onMessage }: Props) {
  const [routeState, setRouteState] = useState<SiteRouteState>();
  const [websiteState, setWebsiteState] = useState<WebsiteState>();
  const [searchIntegrations, setSearchIntegrations] = useState<SearchIntegrationSettings>();
  const [versions, setVersions] = useState<WebsiteVersion[]>([]);
  const [routePreview, setRoutePreview] = useState<SiteRoutePreview>();
	const [brandCapture, setBrandCapture] = useState<BrandCaptureResponse>();
	const [brandSourceURL, setBrandSourceURL] = useState("");
	const [brandRightsConfirmed, setBrandRightsConfirmed] = useState(false);
	const [websiteFormDirty, setWebsiteFormDirty] = useState(false);
  const [loading, setLoading] = useState(false);
  const [routeForm] = Form.useForm<SiteRouteConfig>();
  const [websiteForm] = Form.useForm<SiteConfiguration>();
  const [searchForm] = Form.useForm<SearchIntegrationSettings>();

  const load = async () => {
    setLoading(true);
    try {
      const [routes, website, history, integrations] = await Promise.all([
        api<SiteRouteState>("/admin/api/website/routes"),
        api<WebsiteState>("/admin/api/website/configuration"),
        api<WebsiteVersion[]>("/admin/api/website/versions"),
        api<SearchIntegrationSettings>("/admin/api/search-integrations"),
      ]);
      setRouteState(routes);
      setWebsiteState(website);
      setVersions(history);
      setSearchIntegrations(integrations);
      routeForm.setFieldsValue(routes.config);
      websiteForm.setFieldsValue(website.working);
      searchForm.setFieldsValue(integrations);
      setRoutePreview(undefined);
		setBrandCapture(undefined);
		setBrandRightsConfirmed(false);
		setWebsiteFormDirty(false);
    } catch (error) {
      onError(error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const saveWorking = async (configuration: SiteConfiguration) => {
    if (!websiteState) return;
    setLoading(true);
    try {
      const next = await putJSON<WebsiteState>("/admin/api/website/configuration", {
        expected_revision: websiteState.working_revision,
        configuration,
      });
      setWebsiteState(next);
      websiteForm.setFieldsValue(next.working);
      setRoutePreview(undefined);
		setBrandCapture(undefined);
		setBrandRightsConfirmed(false);
		setWebsiteFormDirty(false);
      onMessage(`Saved Website working revision ${next.working_revision}. The public site is unchanged.`);
    } catch (error) {
      onError(error);
      await load();
    } finally {
      setLoading(false);
    }
  };

  const previewWebsite = async () => {
    if (!websiteState) return;
    const previewWindow = window.open("about:blank", "_blank");
    setLoading(true);
    try {
      const receipt = await postJSON<{ url: string }>("/admin/api/website/preview", {
        expected_working_revision: websiteState.working_revision,
      });
      if (previewWindow) previewWindow.location.assign(receipt.url);
      else window.open(receipt.url, "_blank", "noopener,noreferrer");
    } catch (error) {
      previewWindow?.close();
      onError(error);
    } finally {
      setLoading(false);
    }
  };

  const uploadWebsiteImage = async (file: File, field: "primary_logo_asset_id" | "dark_logo_asset_id" | "favicon_asset_id" | "social_image_asset_id") => {
    setLoading(true);
    try {
      const formData = new FormData();
      formData.set("file", file);
      const asset = await api<Asset>("/admin/api/website/assets", { method: "POST", body: formData });
      websiteForm.setFieldValue(["organization", field], asset.id);
      onMessage(`Uploaded ${asset.original_filename}. Save the working copy to reference it.`);
    } catch (error) {
      onError(error);
    } finally {
      setLoading(false);
    }
    return false;
  };

  const previewRoutes = async (config: SiteRouteConfig) => {
    setLoading(true);
    try {
      const next = await postJSON<SiteRoutePreview>("/admin/api/website/routes/preview", config);
      setRoutePreview(next);
      onMessage(`Checked ${next.affected} Published product routes against Website working revision ${next.working_revision}.`);
    } catch (error) {
      setRoutePreview(undefined);
      onError(error);
    } finally {
      setLoading(false);
    }
  };

  const publish = async () => {
    if (!routeState || !websiteState || !routePreview || routePreview.missing.length || routePreview.conflicts.length) return;
    setLoading(true);
    try {
      await postJSON<SiteRoutePreview>("/admin/api/website/routes/publish", {
        expected_epoch: routeState.current_epoch,
        expected_working_revision: websiteState.working_revision,
        config: routeForm.getFieldsValue(),
      });
      onMessage(`Published Website configuration and every Product representation at site epoch ${routePreview.candidate_epoch}.`);
      await load();
    } catch (error) {
      onError(error);
      await load();
    } finally {
      setLoading(false);
    }
  };

  const restoreVersion = async (version: number) => {
    if (!websiteState) return;
    setLoading(true);
    try {
      const next = await postJSON<WebsiteState>("/admin/api/website/versions/restore", {
        expected_working_revision: websiteState.working_revision,
        version,
      });
      setWebsiteState(next);
      websiteForm.setFieldsValue(next.working);
      setRoutePreview(undefined);
		setBrandCapture(undefined);
		setBrandRightsConfirmed(false);
		setWebsiteFormDirty(false);
      onMessage(`Restored Website version ${version} into working revision ${next.working_revision}. Preview and Publish are still required.`);
    } catch (error) {
      onError(error);
      await load();
    } finally {
      setLoading(false);
    }
  };

	const captureBrand = async () => {
		if (!websiteState || websiteFormDirty) return;
		const sourceURL = brandSourceURL.trim();
		try {
			const parsed = new URL(sourceURL);
			if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
				throw new Error("Enter an absolute HTTP or HTTPS URL.");
			}
		} catch (error) {
			onError(error instanceof Error ? error : new Error("Enter a valid Website URL."));
			return;
		}
		setLoading(true);
		try {
			const result = await postJSON<BrandCaptureResponse>("/admin/api/website/capture", { source_url: sourceURL });
			setBrandCapture(result);
			setBrandRightsConfirmed(false);
			onMessage(`Captured a review-only candidate against Website working revision ${result.working_revision}.`);
		} catch (error) {
			setBrandCapture(undefined);
			setBrandRightsConfirmed(false);
			onError(error);
		} finally {
			setLoading(false);
		}
	};

	const applyBrandCandidate = () => {
		if (!brandCapture || !brandRightsConfirmed || websiteFormDirty || brandCapture.working_revision !== websiteState?.working_revision) return;
		websiteForm.setFieldsValue(brandCapture.proposed_configuration);
		setWebsiteFormDirty(true);
		setRoutePreview(undefined);
		onMessage("Applied the candidate to this browser form only. Save working copy, Preview, and Publish are still required.");
	};

  const saveSearchIntegrations = async (values: SearchIntegrationSettings) => {
    if (!searchIntegrations) return;
    setLoading(true);
    try {
      const next = await putJSON<SearchIntegrationSettings>("/admin/api/search-integrations", {
        revision: searchIntegrations.revision,
        indexnow_enabled: values.indexnow_enabled,
        google_enabled: values.google_enabled,
        google_site_url: values.google_site_url ?? "",
      });
      setSearchIntegrations(next);
      searchForm.setFieldsValue(next);
      onMessage("Search integration settings saved. External submissions run independently; accepted never means indexed.");
    } catch (error) {
      onError(error);
      const latest = await api<SearchIntegrationSettings>("/admin/api/search-integrations").catch(() => undefined);
      if (latest) {
        setSearchIntegrations(latest);
        searchForm.setFieldsValue(latest);
      }
    } finally {
      setLoading(false);
    }
  };

  const setCustomCSSDisabled = async (disabled: boolean) => {
    setLoading(true);
    try {
      const next = await postJSON<WebsiteState>("/admin/api/website/custom-css", { disabled });
      setWebsiteState(next);
      onMessage(disabled ? "Custom CSS Safe Mode is active for all new public requests." : "Custom CSS public serving is enabled again.");
    } catch (error) {
      onError(error);
      await load();
    } finally {
      setLoading(false);
    }
  };

  const issues = [...(routePreview?.missing ?? []), ...(routePreview?.conflicts ?? [])];
  const clean =
    routePreview !== undefined &&
    issues.length === 0 &&
    routePreview.current_epoch === routeState?.current_epoch &&
    routePreview.working_revision === websiteState?.working_revision;

  return (
    <Space direction="vertical" size="large" className="panel-stack">
      <Card title="Website working configuration" loading={!websiteState && loading}>
        {websiteState && (
          <Descriptions
            size="small"
            column={3}
            items={[
              { key: "working", label: "Working revision", children: websiteState.working_revision },
              { key: "active", label: "Active version", children: websiteState.active_version },
              { key: "epoch", label: "Active site epoch", children: websiteState.active_epoch },
            ]}
          />
        )}
        {websiteState?.custom_css_disabled ? (
          <Alert
            type="warning"
            showIcon
            message="Custom CSS Safe Mode is active"
            description="Public requests cannot retrieve the active custom stylesheet. Admin and system pages are unaffected."
            action={<Button onClick={() => void setCustomCSSDisabled(false)} loading={loading}>Re-enable Custom CSS</Button>}
          />
        ) : (
          <Popconfirm title="Immediately stop serving Custom CSS to new public requests?" onConfirm={() => void setCustomCSSDisabled(true)}>
            <Button danger loading={loading}>Disable Custom CSS now</Button>
          </Popconfirm>
        )}
        <Typography.Paragraph type="secondary">
          Save changes into the durable working copy. Preview is private. Only Publish changes the public Website and Product artifacts.
        </Typography.Paragraph>
		<Card size="small" title="Brand Capture candidate">
			<Typography.Paragraph type="secondary">
				Fetch bounded public HTML/CSS to suggest Organization, Theme, and Navigation. It never imports source scripts, canonical/SEO settings, analytics, raw HTML/CSS, or assets. Dynamic JavaScript-only content may remain unavailable.
			</Typography.Paragraph>
			<Space.Compact block>
				<Input
					type="url"
					value={brandSourceURL}
					onChange={(event) => setBrandSourceURL(event.target.value)}
					placeholder="https://www.example.com"
					aria-label="Source Website URL"
				/>
				<Button onClick={() => void captureBrand()} disabled={!brandSourceURL.trim() || websiteFormDirty} loading={loading}>Analyze</Button>
			</Space.Compact>
			{websiteFormDirty && (
				<Alert type="warning" showIcon message="Save or reload local form edits before re-capturing." description="The server compares a capture with the durable Website working revision, not unsaved browser fields." action={<Button onClick={() => void load()} disabled={loading}>Reload saved working</Button>} />
			)}
			{brandCapture && (
				<Space direction="vertical" className="panel-stack">
					{brandCapture.candidate.requires_browser && (
						<Alert type="warning" showIcon message="Some source content requires JavaScript and was not captured." description={`Unavailable: ${(brandCapture.candidate.unavailable_dynamic_parts ?? []).join(", ") || "dynamic content"}. Complete these fields manually.`} />
					)}
					{(brandCapture.candidate.warnings ?? []).map((warning) => <Alert key={warning} type="warning" showIcon message={warning} />)}
					<Descriptions size="small" column={2} items={[
						{ key: "organization", label: "Organization", children: brandCapture.candidate.organization || "Manual input required" },
						{ key: "title", label: "Source title", children: brandCapture.candidate.title || "Not available" },
						{ key: "colors", label: "Colors", children: (brandCapture.candidate.colors ?? []).join(", ") || "Manual input required" },
						{ key: "fonts", label: "Fonts", children: (brandCapture.candidate.fonts ?? []).join(", ") || "Manual input required" },
						{ key: "logos", label: "Logo references", children: (brandCapture.candidate.logo_urls ?? []).join(", ") || "Manual upload required" },
						{ key: "manual", label: "Manual corrections", children: (brandCapture.candidate.manual_corrections ?? []).join(", ") || "None identified" },
					]} />
					<Table<BrandCaptureFieldChange>
						rowKey="field"
						size="small"
						pagination={false}
						dataSource={brandCapture.diff}
						locale={{ emptyText: "The capture does not change the saved working configuration." }}
						columns={[
							{ title: "Field", dataIndex: "field" },
							{ title: "Current working", dataIndex: "before", render: (value: string) => <Typography.Text code>{value}</Typography.Text> },
							{ title: "Candidate", dataIndex: "after", render: (value: string) => <Typography.Text code>{value}</Typography.Text> },
						]}
					/>
					<Checkbox checked={brandRightsConfirmed} onChange={(event) => setBrandRightsConfirmed(event.target.checked)}>
						I confirm that we have the right to use the selected branding and references.
					</Checkbox>
					<Alert type="info" showIcon message="Applying only updates this browser form." description="You must still Save working copy, Preview, and Publish. Logo URLs are references for review and are not downloaded or attached automatically." />
					<Button type="primary" onClick={applyBrandCandidate} disabled={!brandRightsConfirmed || websiteFormDirty || brandCapture.working_revision !== websiteState?.working_revision}>Apply candidate to form</Button>
				</Space>
			)}
		</Card>
        <Form form={websiteForm} layout="vertical" onFinish={(values) => void saveWorking(values)} onValuesChange={() => setWebsiteFormDirty(true)}>
          <Card size="small" title="Organization">
            <Form.Item name={["organization", "display_name"]} label="Display name" rules={[{ required: true }]}>
              <Input />
            </Form.Item>
            <Space wrap align="start">
              <Form.Item name={["organization", "legal_name"]} label="Legal name"><Input /></Form.Item>
              <Form.Item name={["organization", "official_website"]} label="Official website" rules={[{ type: "url" }]}><Input /></Form.Item>
              <Form.Item name={["organization", "privacy_url"]} label="Privacy URL" rules={[{ type: "url" }]}><Input /></Form.Item>
              <Form.Item name={["organization", "terms_url"]} label="Terms URL" rules={[{ type: "url" }]}><Input /></Form.Item>
            </Space>
            <Space wrap align="start">
              {([
                ["primary_logo_asset_id", "Primary logo"],
                ["dark_logo_asset_id", "Dark-background logo"],
                ["favicon_asset_id", "Favicon"],
                ["social_image_asset_id", "Social image"],
              ] as const).map(([field, label]) => (
                <Space direction="vertical" key={field}>
                  <Form.Item name={["organization", field]} label={`${label} asset ID`}><Input readOnly allowClear /></Form.Item>
                  <Upload accept=".jpg,.jpeg,.png,.webp,.svg" showUploadList={false} beforeUpload={(file) => { void uploadWebsiteImage(file, field); return false; }}>
                    <Button loading={loading}>Upload {label}</Button>
                  </Upload>
                </Space>
              ))}
            </Space>
            <Form.List name={["organization", "contact_links"]}>
              {(fields, { add, remove }) => (
                <Space direction="vertical" className="panel-stack">
                  {fields.map((field) => (
                    <Space key={field.key} wrap align="start">
                      <Form.Item {...field} name={[field.name, "id"]} label="Contact ID" rules={[{ required: true }]}><Input /></Form.Item>
                      <Form.Item {...field} name={[field.name, "label"]} label="Label" rules={[{ required: true }]}><Input /></Form.Item>
                      <Form.Item {...field} name={[field.name, "url"]} label="URL" rules={[{ required: true }, { type: "url" }]}><Input /></Form.Item>
                      <Form.Item {...field} name={[field.name, "sort_order"]} label="Order"><InputNumber /></Form.Item>
                      <Button danger onClick={() => remove(field.name)}>Remove contact</Button>
                    </Space>
                  ))}
                  <Button onClick={() => add({ id: "", label: "", url: "", sort_order: fields.length * 10 })}>Add contact</Button>
                </Space>
              )}
            </Form.List>
          </Card>

          <Card size="small" title="Navigation">
            <Form.List name="navigation">
              {(fields, { add, remove }) => (
                <Space direction="vertical" className="panel-stack">
                  {fields.map((field) => (
                    <Space key={field.key} wrap align="start">
                      <Form.Item {...field} name={[field.name, "id"]} label="ID" rules={[{ required: true }]}><Input /></Form.Item>
                      <Form.Item {...field} name={[field.name, "parent_id"]} label="Parent ID"><Input /></Form.Item>
                      <Form.Item {...field} name={[field.name, "label"]} label="Label" rules={[{ required: true }]}><Input /></Form.Item>
                      <Form.Item {...field} name={[field.name, "url"]} label="URL" rules={[{ required: true }]}><Input /></Form.Item>
                      <Form.Item {...field} name={[field.name, "sort_order"]} label="Order"><InputNumber /></Form.Item>
                      <Form.Item {...field} name={[field.name, "visible"]} valuePropName="checked"><Checkbox>Visible</Checkbox></Form.Item>
                      <Form.Item {...field} name={[field.name, "open_new_window"]} valuePropName="checked"><Checkbox>New window</Checkbox></Form.Item>
                      <Button danger onClick={() => remove(field.name)}>Remove link</Button>
                    </Space>
                  ))}
                  <Button onClick={() => add({ id: "", label: "", url: "/", sort_order: fields.length * 10, visible: true, open_new_window: false })}>Add navigation link</Button>
                </Space>
              )}
            </Form.List>
          </Card>

          <Card size="small" title="Theme and SEO">
            <Space wrap align="start">
              <Form.Item name={["theme", "primary_color"]} label="Primary color" rules={[{ required: true }, { pattern: /^#[0-9a-fA-F]{6}$/ }]}><Input type="color" /></Form.Item>
              <Form.Item name={["theme", "secondary_color"]} label="Secondary color" rules={[{ required: true }, { pattern: /^#[0-9a-fA-F]{6}$/ }]}><Input type="color" /></Form.Item>
              <Form.Item name={["theme", "font_family"]} label="Font family" rules={[{ required: true }]}><Input /></Form.Item>
              <Form.Item name={["theme", "content_width_px"]} label="Content width" rules={[{ required: true }]}><InputNumber min={640} max={1920} /></Form.Item>
            </Space>
            <Form.Item name={["theme", "custom_css_enabled"]} valuePropName="checked"><Checkbox>Apply Custom CSS on public pages</Checkbox></Form.Item>
            <Form.Item name={["theme", "custom_css"]} label="Custom CSS"><Input.TextArea rows={6} /></Form.Item>
            <Form.Item name={["seo", "default_title"]} label="Default title"><Input /></Form.Item>
            <Form.Item name={["seo", "default_description"]} label="Default description"><Input.TextArea rows={3} /></Form.Item>
          </Card>

          <Space>
            <Button type="primary" htmlType="submit" loading={loading}>Save working copy</Button>
            <Button onClick={() => void previewWebsite()} disabled={!websiteState} loading={loading}>Preview saved working copy</Button>
          </Space>
        </Form>
      </Card>

      <Card title="Search and AI exposure" loading={!searchIntegrations && loading}>
        <Space direction="vertical" className="panel-stack">
          <Alert
            showIcon
            type="info"
            message="Optional enhancement"
            description="HTML, JSON-LD, robots.txt, Sitemap, JSON, manifest, Markdown, and llms.txt remain available without either integration. A successful submission means accepted by the service, never indexed."
          />
          {searchIntegrations && (
            <Form form={searchForm} layout="vertical" onFinish={(values) => void saveSearchIntegrations(values)}>
              <Card size="small" title="Bing and participating IndexNow engines">
                <Form.Item name="indexnow_enabled" valuePropName="checked">
                  <Checkbox disabled={!searchIntegrations.indexnow_available}>Enable IndexNow change notifications</Checkbox>
                </Form.Item>
                {!searchIntegrations.indexnow_available && <Alert showIcon type="warning" message="Configure a public HTTPS Base URL before enabling IndexNow." />}
                {searchIntegrations.indexnow_key_url && (
                  <Descriptions size="small" items={[{ key: "key", label: "Public key verification URL", children: <Typography.Text copyable>{searchIntegrations.indexnow_key_url}</Typography.Text> }]} />
                )}
              </Card>
              <Card size="small" title="Google Search Console">
                <Form.Item name="google_enabled" valuePropName="checked">
                  <Checkbox disabled={!searchIntegrations.google_available}>Submit the Sitemap through Search Console</Checkbox>
                </Form.Item>
                {!searchIntegrations.google_available && <Alert showIcon type="warning" message="Configure the host OAuth client ID, client-secret file, refresh-token file, and a public HTTPS Base URL first." />}
                <Form.Item
                  name="google_site_url"
                  label="Verified Search Console site property"
                  extra="Use the exact URL-prefix property such as https://catalog.example.com/ or sc-domain:example.com."
                  rules={[({ getFieldValue }) => ({ validator(_, value?: string) { return !getFieldValue("google_enabled") || value?.trim() ? Promise.resolve() : Promise.reject(new Error("A verified site property is required.")); } })]}
                >
                  <Input disabled={!searchIntegrations.google_available} />
                </Form.Item>
              </Card>
              <Button type="primary" htmlType="submit" loading={loading}>Save search integrations</Button>
            </Form>
          )}
        </Space>
      </Card>

      <Card title="Public product routes" loading={!routeState && loading}>
        {routeState && (
          <Descriptions size="small" column={3} items={[
            { key: "epoch", label: "Active site epoch", children: routeState.current_epoch },
            { key: "prefix", label: "Active prefix", children: routeState.config.product_prefix },
            { key: "pattern", label: "Active pattern", children: routeState.config.url_pattern },
          ]} />
        )}
        <Typography.Paragraph type="secondary">
          Preview checks every Published Product. Publish installs the working Website, routes, and representations at one site-wide visibility boundary.
        </Typography.Paragraph>
        <Form form={routeForm} layout="inline" onFinish={(values) => void previewRoutes(values)} onValuesChange={() => setRoutePreview(undefined)}>
          <Form.Item name="product_prefix" label="Product prefix" rules={[{ required: true }, { pattern: /^\/[A-Za-z0-9._-]+$/, message: "Use one safe path segment beginning with /." }]}>
            <Input placeholder="/products" />
          </Form.Item>
          <Form.Item name="url_pattern" label="Pattern" rules={[{ required: true }]}>
            <Select style={{ width: 190 }} options={[
              { value: "compact", label: "Compact" },
              { value: "manufacturer", label: "Manufacturer path" },
              { value: "brand", label: "Brand path" },
              { value: "category", label: "Category path" },
            ]} />
          </Form.Item>
          <Button htmlType="submit" loading={loading}>Preview entire site</Button>
        </Form>
      </Card>

      {routePreview && (
        <Card title={`Candidate site epoch ${routePreview.candidate_epoch}`}>
          {clean ? (
            <Alert type="success" showIcon message={`${routePreview.affected} routes and Website working revision ${routePreview.working_revision} are ready.`} />
          ) : (
            <Alert type="error" showIcon message="This candidate cannot be published." description="Resolve every missing namespace, route conflict, or stale working revision, then preview again." />
          )}
          {issues.length > 0 && (
            <Table<SiteRouteIssue>
              rowKey={(row) => `${row.product_id}:${row.reason}:${row.route ?? ""}`}
              pagination={false}
              dataSource={issues}
              columns={[
                { title: "Part number", dataIndex: "part_number" },
                { title: "Product ID", dataIndex: "product_id" },
                { title: "Target route", dataIndex: "route", render: (value?: string) => value || "—" },
                { title: "Issue", dataIndex: "reason" },
              ]}
            />
          )}
          <Popconfirm title="Publish the Website working copy and switch every Published Product at one new site epoch?" onConfirm={() => void publish()} disabled={!clean}>
            <Button type="primary" disabled={!clean} loading={loading}>Publish Website configuration</Button>
          </Popconfirm>
        </Card>
      )}

      <Card title="Published Website versions">
        <Table<WebsiteVersion>
          rowKey="version"
          pagination={false}
          dataSource={versions}
          columns={[
            { title: "Version", dataIndex: "version" },
            { title: "Site epoch", dataIndex: "site_epoch" },
            { title: "Organization", render: (_, row) => row.configuration.organization.display_name },
            { title: "Created", dataIndex: "created_at" },
            {
              title: "Action",
              render: (_, row) => (
                <Popconfirm title={`Restore version ${row.version} into the working copy? It will not publish automatically.`} onConfirm={() => void restoreVersion(row.version)}>
                  <Button disabled={loading}>Restore as working</Button>
                </Popconfirm>
              ),
            },
          ]}
        />
      </Card>
    </Space>
  );
}
