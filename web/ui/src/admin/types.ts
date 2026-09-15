export type ProductStatus = "published" | "hidden";
export type RecordState = "current" | "archived";

export type Product = {
  id: string;
  slug: string;
  custom_path?: string;
  part_number: string;
  name: string;
  manufacturer_id?: string;
  manufacturer?: string;
  brand_id?: string;
  brand?: string;
  lifecycle_id?: string;
  lifecycle?: string;
  application_ids?: string[];
  applications?: Array<{ id: string; name: string; slug: string }>;
  category_id: string;
  package_form_factor?: string;
  description: string;
  features?: string;
  specification: string;
  document_url: string;
  record_state: RecordState;
  status: ProductStatus;
  revision: number;
};

export type ProductTranslation = {
  locale: string;
  name?: string;
  description?: string;
  features?: string;
  specification?: string;
  revision: number;
  updated_by?: string;
  updated_at?: string;
};
export type ProductContent = {
  product_id: string;
  product_revision: number;
  source_locale: string;
  translations: ProductTranslation[];
};

export type ProductForm = Omit<
  Product,
  "id" | "record_state" | "revision" | "applications"
>;

export type Category = {
  id: string;
  parent_id?: string;
  system_key?: string;
  name: string;
  slug: string;
  status: "active" | "disabled";
  revision: number;
};

export const dictionaryKinds = [
  "manufacturer",
  "brand",
  "application",
  "lifecycle",
  "document_type",
] as const;
export type DictionaryKind = (typeof dictionaryKinds)[number];
export type DictionaryEntry = {
  id: string;
  kind: DictionaryKind;
  name: string;
  slug?: string;
  status: "active" | "disabled";
  revision: number;
};

export type TaxonomyProductImpact = {
  product_id: string;
  part_number: string;
  current_route?: string;
  proposed_route?: string;
};

export type TaxonomyImpact = {
  entity_type: string;
  entity_id: string;
  affected_products: TaxonomyProductImpact[];
};

export type SpecDefinition = {
  id: string;
  name: string;
  preferred_unit?: string;
  filterable: boolean;
  semantic_version: number;
  status: "active" | "disabled";
  revision: number;
};
export type SpecSet = {
  id: string;
  name: string;
  status: "active" | "disabled";
  revision: number;
  spec_ids: string[];
};
export type SpecValue = {
  id: string;
  product_id: string;
  spec_id: string;
  raw_value: string;
  source_locale?: string;
  source_revision: number;
  active: boolean;
};
export type NormalizedValue = {
  id: string;
  spec_value_id: string;
  value_json?: string;
  source_revision: number;
  spec_semantic_version: number;
  normalizer_version: string;
  source: "automatic" | "manual";
  status: "current" | "stale" | "failed";
  failure_reason?: string;
};
export type SpecValueDetail = {
  value: SpecValue;
  normalized: NormalizedValue[];
};
export type ProductDocument = {
  id: string;
  product_id: string;
  label: string;
  document_type_id: string;
  asset_id?: string;
  external_url?: string;
  language?: string;
  sort_order: number;
};

export type ProductImage = {
  id: string;
  product_id: string;
  asset_id?: string;
  external_url?: string;
  alt_text?: string;
  sort_order: number;
  primary: boolean;
};

export type Asset = {
  id: string;
  owner_type: "product" | "website";
  owner_id: string;
  original_filename: string;
  mime_type: string;
  size_bytes: number;
  checksum: string;
  created_at: string;
};

export type SiteRouteConfig = {
  product_prefix: string;
  url_pattern: "compact" | "manufacturer" | "brand" | "category";
};

export type SiteRouteState = {
  current_epoch: number;
  config: SiteRouteConfig;
};

export type SiteRouteIssue = {
  product_id: string;
  part_number: string;
  route?: string;
  reason: string;
};

export type SiteRoutePreview = {
  current_epoch: number;
  candidate_epoch: number;
  working_revision: number;
  affected: number;
  missing: SiteRouteIssue[];
  conflicts: SiteRouteIssue[];
};

export type SiteContactLink = {
  id: string;
  label: string;
  url: string;
  sort_order: number;
};

export type SiteNavigationItem = {
  id: string;
  parent_id?: string;
  label: string;
  url: string;
  sort_order: number;
  open_new_window: boolean;
  visible: boolean;
};

export type SiteConfiguration = {
  organization: {
    display_name: string;
    legal_name?: string;
    official_website?: string;
    privacy_url?: string;
    terms_url?: string;
    primary_logo_asset_id?: string;
    dark_logo_asset_id?: string;
    favicon_asset_id?: string;
    social_image_asset_id?: string;
    contact_links?: SiteContactLink[];
  };
  navigation?: SiteNavigationItem[];
  theme: {
    primary_color: string;
    secondary_color: string;
    font_family: string;
    content_width_px: number;
    custom_css?: string;
    custom_css_enabled: boolean;
  };
  seo: {
    default_title?: string;
    default_description?: string;
  };
};

export type WebsiteState = {
  working_revision: number;
  active_version: number;
  active_epoch: number;
  custom_css_disabled: boolean;
  runtime_generation: number;
  working: SiteConfiguration;
  active: SiteConfiguration;
};

export type WebsiteVersion = {
  version: number;
  source_working_revision: number;
  site_epoch: number;
  configuration: SiteConfiguration;
  created_at: string;
};

export type BrandCaptureNavigation = {
  label: string;
  url: string;
};

export type BrandCaptureCandidate = {
  source_url: string;
  organization?: string;
  title?: string;
  logo_urls?: string[];
  colors?: string[];
  fonts?: string[];
  navigation?: BrandCaptureNavigation[];
  footer_text?: string;
  stylesheet_urls?: string[];
  warnings?: string[];
  requires_browser: boolean;
  unavailable_dynamic_parts?: string[];
  manual_corrections?: string[];
  needs_rights_confirmation: boolean;
  working_copy_only: boolean;
};

export type BrandCaptureFieldChange = {
  field: string;
  before: string;
  after: string;
};

export type BrandCaptureResponse = {
  working_revision: number;
  candidate: BrandCaptureCandidate;
  proposed_configuration: SiteConfiguration;
  diff: BrandCaptureFieldChange[];
};

export type SystemResourceHealth = {
  resource: string;
  status: "Normal" | "Warning" | "Critical";
  reason?: string;
  checked_utc: string;
  total_bytes?: number;
  free_bytes?: number;
  reserved_bytes?: number;
  free_inodes?: number;
  reserved_inodes?: number;
  supports_inodes: boolean;
  affected_operations: string[];
};

export type SystemComponentHealth = {
  component:
    | "database"
    | "maintenance"
    | "public_generation"
    | "background_jobs"
    | "backup"
    | "asset_gc"
    | "search"
    | "smtp";
  status: "Normal" | "Warning" | "Critical";
  summary: string;
  checked_utc: string;
  configured?: boolean;
  last_success_utc?: string;
  last_run_status?: string;
  next_run_utc?: string;
  due?: boolean;
  counters?: Record<string, number>;
  runtime_running?: boolean;
  last_runtime_attempt_utc?: string;
  last_runtime_success_utc?: string;
  last_runtime_failure_utc?: string;
  runtime_failure_stage?: string;
  consecutive_runtime_failures?: number;
};

export type SystemHealth = {
  status: "Normal" | "Warning" | "Critical";
  checked_utc: string;
  components: SystemComponentHealth[];
  resources: SystemResourceHealth[];
};

export type RuntimeLog = {
  available: boolean;
  generation: number;
  max_generation: number;
  file_name?: string;
  size_bytes?: number;
  truncated: boolean;
  lines: string[];
};

export type SiteMaintenance = {
  active: boolean;
  message: string;
  revision: number;
  updated_by?: string;
  updated_at: string;
};

export type SiteSettings = {
  default_locale: string;
  supported_locales: string[];
  content_multilingual_enabled: boolean;
  time_zone: string;
  revision: number;
  updated_at: string;
};

export type BackupSettings = {
  enabled: boolean;
  local_time: string;
  time_zone: string;
  retention_daily: number;
  retention_weekly: number;
  retention_monthly: number;
  retention_pre_upgrade: number;
  retention_pre_restore: number;
  version: number;
  updated_at: string;
};

export type BackupRun = {
  id: string;
  kind: "scheduled" | "manual" | "pre-upgrade" | "pre-restore";
  status: "running" | "succeeded" | "failed";
  scheduled_for?: string;
  started_at: string;
  completed_at?: string;
  backup_id?: string;
  size_bytes?: number;
  content_verified: boolean;
  read_only_applied: boolean;
  error_message?: string;
  warning_message?: string;
};

export type BackupStatus = {
  settings: BackupSettings;
  runs: BackupRun[];
  next_run_utc?: string;
  due: boolean;
  contains_sensitive_data: boolean;
  external_requirements?: string[];
  retention: {
    reasons: Record<string, string[]>;
    expired: string[];
    deleted: string[];
  };
};

export type TrafficSettings = {
  rfq_limit: number;
  rfq_window_seconds: number;
  version: number;
  updated_at: string;
  trusted_proxy_configured?: boolean;
};

export const capabilities = [
  "admin.access",
  "catalog.view",
  "catalog.edit",
  "catalog.publish",
  "catalog.import",
  "catalog.export",
  "rfq.view",
  "rfq.manage",
  "rfq.export",
  "users.manage",
  "system.manage",
  "audit.view",
] as const;
export type Capability = (typeof capabilities)[number];
export type Role = {
  id: string;
  name: string;
  status: string;
  revision: number;
  capabilities: Capability[];
};
export type User = {
  id: string;
  email: string;
  display_name: string;
  role_id: string;
  status: string;
  auth_revision: number;
};

export type RFQ = {
  ID: string;
  Name: string;
  Email: string;
  Company: string;
  Phone: string;
  Country: string;
  GeneralMessage: string;
  Status: "new" | "in_progress" | "closed" | "spam";
  Revision: number;
  UpdatedBy: string;
  UpdatedAt: string;
  CreatedAt: string;
  PrivacyState: "retained" | "anonymized";
  PrivacyAt: string;
  PrivacyBy: string;
  Items: Array<{
    kind: string;
    product_id?: string;
    requested?: string;
    raw_query?: string;
  }>;
  Recipients: Array<{
    kind: "user" | "email";
    user_id?: string;
    email?: string;
    display_name?: string;
  }>;
};

export type RFQRecipientUser = {
  id: string;
  email: string;
  display_name: string;
};
export type RFQRecipientSettings = {
  revision: number;
  updated_by?: string;
  updated_at: string;
  recipients: RFQ["Recipients"];
};

export type RFQDeliveryAttempt = {
  id: string;
  rfq_id: string;
  rfq_revision: number;
  content_version: string;
  content_hash: string;
  status: "pending" | "sending" | "accepted" | "partial" | "failed" | "unknown";
  created_by: string;
  created_at: string;
  updated_at: string;
  replay?: boolean;
  recipients: Array<{
    index: number;
    kind: "user" | "email";
    source_user_id?: string;
    email: string;
    status: "pending" | "sending" | "accepted" | "failed" | "unknown";
    error_class?: string;
    error_message?: string;
  }>;
};

export type AuditEntry = {
  id: string;
  actor_id?: string;
  action: string;
  target_type: string;
  target_id: string;
  result: string;
  details: unknown;
  created_at: string;
};

export type ImportMapping = {
  source_index: number;
  source_name: string;
  target: string;
};
export type ImportJob = {
  id: string;
  status: string;
  phase: string;
  original_filename: string;
  checked_rows: number;
  total_rows: number;
  create_count: number;
  update_count: number;
  no_change_count: number;
  failed_count: number;
  error_message?: string;
  cancel_requested: boolean;
};
export type ImportIssue = {
  sheet: string;
  row: number;
  column?: string;
  value?: string;
  code: string;
  message: string;
};
export type ImportPreview = {
  operation_id: string;
  fully_scanned: boolean;
  fully_validated: boolean;
  checked_rows: number;
  total_rows: number;
  create_count: number;
  update_count: number;
  no_change_count: number;
  issues: ImportIssue[];
};
export type ImportJobResponse = { job: ImportJob; preview?: ImportPreview };

export type AdminJob = {
  id: string;
  kind: "publication" | "import" | "backup" | "smtp" | "search_submission";
  status: string;
  stage: string;
  target_type?: string;
  target_id?: string;
  label?: string;
  desired_revision?: number;
  progress_current?: number;
  progress_total?: number;
  error_message?: string;
  retryable: boolean;
  outputs?: string[];
  created_at: string;
  updated_at: string;
};

export type AdminJobsResponse = {
  jobs: AdminJob[];
  retry_policy: {
    safe_kinds: string[];
    new_operation_only: string[];
    never_automatic: string[];
  };
};

export type SearchIntegrationSettings = {
  revision: number;
  indexnow_enabled: boolean;
  indexnow_key?: string;
  indexnow_available: boolean;
  indexnow_key_url?: string;
  google_enabled: boolean;
  google_site_url?: string;
  google_available: boolean;
  updated_at: string;
};
