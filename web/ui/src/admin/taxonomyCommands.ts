import { clientID, postJSON } from "./api";
import type { Category, DictionaryEntry, DictionaryKind, SpecDefinition, SpecSet, TaxonomyImpact } from "./types";

type CategoryValues = { name: string; slug: string; parent_id?: string };
type DictionaryValues = { name: string; slug?: string };
type SpecValues = { name: string; preferred_unit?: string; filterable: boolean; semantic_version: number };

const categoryPath = (id: string) => `/admin/api/categories/${encodeURIComponent(id)}`;
const dictionaryPath = (id: string) => `/admin/api/dictionaries/${encodeURIComponent(id)}`;

// Commands remain explicit because their revision, preview, publication and
// partial-effect contracts are domain behavior, not generic resource CRUD.
export const taxonomyCommands = {
  createCategory: (values: CategoryValues) => postJSON<Category>("/admin/api/categories", {
    id: clientID("cat"), ...values, parent_id: values.parent_id || "", status: "active", revision: 1,
  }),
  disableCategory: (category: Category) => postJSON<void>(`${categoryPath(category.id)}/disable`, { expected_revision: category.revision }),
  previewCategory: (category: Category, values: CategoryValues) => postJSON<TaxonomyImpact>(`${categoryPath(category.id)}/update-preview`, {
    expected_revision: category.revision, ...values, parent_id: values.parent_id || "",
  }),
  updateCategory: (category: Category, values: CategoryValues) => postJSON<Category>(`${categoryPath(category.id)}/update`, {
    expected_revision: category.revision, ...values, parent_id: values.parent_id || "",
  }),
  createDictionary: (kind: DictionaryKind, values: DictionaryValues) => postJSON<DictionaryEntry>("/admin/api/dictionaries", {
    id: clientID("dic"), kind, ...values, slug: values.slug || "", status: "active", revision: 1,
  }),
  disableDictionary: (entry: DictionaryEntry) => postJSON<void>(`${dictionaryPath(entry.id)}/disable`, { expected_revision: entry.revision }),
  previewDictionary: (entry: DictionaryEntry, values: DictionaryValues) => postJSON<TaxonomyImpact>(`${dictionaryPath(entry.id)}/update-preview`, {
    expected_revision: entry.revision, ...values, slug: values.slug || "",
  }),
  updateDictionary: (entry: DictionaryEntry, values: DictionaryValues) => postJSON<DictionaryEntry>(`${dictionaryPath(entry.id)}/update`, {
    expected_revision: entry.revision, ...values, slug: values.slug || "",
  }),
  createSpec: (values: SpecValues) => postJSON<SpecDefinition>("/admin/api/specs", {
    id: clientID("spc"), ...values, preferred_unit: values.preferred_unit || "", status: "active", revision: 1,
  }),
  createSpecSet: (values: { name: string; spec_ids: string[] }) => postJSON<SpecSet>("/admin/api/spec-sets", {
    id: clientID("sps"), ...values, status: "active", revision: 1,
  }),
  assignSpecSet: (category: Category, specSetID: string) => postJSON<void>(`${categoryPath(category.id)}/spec-set`, {
    expected_revision: category.revision, spec_set_id: specSetID,
  }),
};

export const taxonomyInvalidations = {
  category: ["categories", "products"],
  dictionary: ["dictionary-entries", "products"],
  specifications: ["spec-definitions", "spec-sets", "categories", "products"],
} as const;
