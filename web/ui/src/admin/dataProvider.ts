import type { BaseRecord, DataProvider, GetListParams, GetListResponse } from "@refinedev/core";
import { api, APIError, postJSON, putJSON } from "./api";
import { dictionaryKinds, type DictionaryKind } from "./types";

function productsOnly(resource: string): void {
  if (resource !== "products") throw new APIError("Unsupported resource", 400);
}

export function productListURL({ resource, pagination, filters = [], sorters = [] }: GetListParams): string {
  productsOnly(resource);
  const page = pagination?.currentPage ?? 1;
  const size = pagination?.pageSize ?? 20;
  if (pagination?.mode === "off" || !Number.isInteger(page) || page < 1 || page > 1_000_000 || !Number.isInteger(size) || size < 1 || size > 100) {
    throw new APIError("Invalid pagination", 400);
  }
  if (sorters.length > 1 || (sorters[0] && (!["updated_at", "part_number", "name"].includes(sorters[0].field) || !["asc", "desc"].includes(sorters[0].order)))) {
    throw new APIError("Unsupported sort", 400);
  }
  const params = new URLSearchParams({ page: String(page), page_size: String(size), sort: sorters[0]?.field ?? "updated_at", order: sorters[0]?.order ?? "desc" });
  if (filters.length > 1) throw new APIError("Unsupported filters", 400);
  for (const filter of filters) {
    if (!("field" in filter) || filter.field !== "include_archived" || filter.operator !== "eq" || typeof filter.value !== "boolean") throw new APIError("Unsupported filter", 400);
    params.set("include_archived", String(filter.value));
  }
  return `/admin/api/products?${params}`;
}

const flatListPaths = {
  categories: "/admin/api/categories",
  "spec-definitions": "/admin/api/specs",
  "spec-sets": "/admin/api/spec-sets",
  roles: "/admin/api/roles",
  users: "/admin/api/users",
} as const;

function dictionaryKind(meta: GetListParams["meta"]): DictionaryKind {
  const kind = meta?.kind;
  if (typeof kind !== "string" || !dictionaryKinds.includes(kind as DictionaryKind)) {
    throw new APIError("Dictionary kind is required", 400);
  }
  return kind as DictionaryKind;
}

export function flatListURL({ resource, pagination, filters = [], sorters = [], meta }: GetListParams): string {
  if (pagination && pagination.mode !== "off") throw new APIError("Flat resources do not support server pagination", 400);
  if (filters.length) throw new APIError("Unsupported filters", 400);
  if (sorters.length) throw new APIError("Unsupported sort", 400);
  if (resource === "dictionary-entries") {
    return `/admin/api/dictionaries?kind=${encodeURIComponent(dictionaryKind(meta))}`;
  }
  const path = flatListPaths[resource as keyof typeof flatListPaths];
  if (!path) throw new APIError("Unsupported resource", 400);
  return path;
}

export const dataProvider: DataProvider = {
  getApiUrl: () => "/admin/api",
  getList: async <TData extends BaseRecord = BaseRecord>(params: GetListParams): Promise<GetListResponse<TData>> => {
    if (params.resource === "products") return api<GetListResponse<TData>>(productListURL(params));
    const data = await api<TData[]>(flatListURL(params));
    return { data, total: data.length };
  },
  getOne: async ({ resource, id }) => {
    productsOnly(resource);
    return { data: await api(`/admin/api/products/${encodeURIComponent(id)}`) };
  },
  create: async ({ resource, variables }) => {
    productsOnly(resource);
    return { data: await postJSON("/admin/api/products", { ...variables, status: "hidden" }) };
  },
  update: async ({ resource, id, variables }) => {
    productsOnly(resource);
    const revision = (variables as { expected_revision?: number } | undefined)?.expected_revision;
    if (!Number.isInteger(revision) || (revision ?? 0) < 1) throw new APIError("Expected revision is required", 400);
    return { data: await putJSON(`/admin/api/products/${encodeURIComponent(id)}`, variables) };
  },
  deleteOne: async () => { throw new APIError("Products cannot be deleted. Use the Archive command.", 405); },
};
