import { api, downloadFile, postJSON, putJSON } from "./api";
import type { Category, DictionaryEntry, Product, ProductForm, SiteSettings } from "./types";

const productPath = (id: string) => `/admin/api/products/${encodeURIComponent(id)}`;

// Named use cases retain their backend contracts; no updateMany/import helper
// or generic delete method can stand in for these commands.
export const catalogCommands = {
  lookups: async () => Promise.all([
    api<Category[]>("/admin/api/categories"),
    api<DictionaryEntry[]>("/admin/api/dictionaries?kind=manufacturer"),
    api<DictionaryEntry[]>("/admin/api/dictionaries?kind=brand"),
    api<DictionaryEntry[]>("/admin/api/dictionaries?kind=application"),
    api<DictionaryEntry[]>("/admin/api/dictionaries?kind=lifecycle"),
    api<SiteSettings>("/admin/api/products/content-settings"),
  ] as const),
  export: () => downloadFile("/admin/api/exports/products.xlsx", "prods-products.xlsx"),
  clone: (source: Product, values: ProductForm) => postJSON<Product>(`${productPath(source.id)}/clone`, { expected_revision: source.revision, product: { ...values, status: "hidden" } }),
  changeURL: (product: Product, values: ProductForm) => postJSON<Product>(`${productPath(product.id)}/url`, { expected_revision: product.revision, slug: values.slug, custom_path: values.custom_path ?? "" }),
  preview: (product: Product | undefined, values: ProductForm) => postJSON<{ url: string }>("/admin/api/previews/products", { expected_revision: product?.revision ?? 0, product: product ? { ...product, ...values } : values }),
  hide: (product: Product) => postJSON<void>(`${productPath(product.id)}/hide`, { expected_revision: product.revision }),
  archive: (product: Product) => postJSON<void>(`${productPath(product.id)}/archive`, { expected_revision: product.revision }),
  publish: (product: Product) => putJSON<Product>(productPath(product.id), { expected_revision: product.revision, product: { ...product, status: "published" } }),
};
