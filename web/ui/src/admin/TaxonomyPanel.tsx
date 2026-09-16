import { useEffect, useState } from "react";
import { Button, Card, Checkbox, Form, Input, InputNumber, Modal, Popconfirm, Select, Space, Table, Tabs, Tag } from "antd";
import { useInvalidate, useList } from "@refinedev/core";
import { APIError } from "./api";
import { useAdminIdentity } from "./AdminProviders";
import { taxonomyCommands, taxonomyInvalidations } from "./taxonomyCommands";
import { TaxonomyTranslationsModal, type TaxonomyTranslationTarget } from "./TaxonomyTranslationsModal";
import { dictionaryKinds, type Category, type DictionaryEntry, type DictionaryKind, type SpecDefinition, type SpecSet, type TaxonomyImpact } from "./types";
import type { AdminLocale } from "./locales";

type Feedback = { onError: (error: unknown) => void; onMessage: (message: string) => void };

type TaxonomyLocale = AdminLocale;

const labels = {
  "en-US": {
    categories: "Categories", dictionaries: "Dictionaries", specifications: "Specifications",
    createdCategory: (name: string) => `Created category ${name}.`,
    createdDictionary: (kind: string, name: string) => `Created ${kind} ${name}.`,
    disabledCategory: (name: string) => `Disabled category ${name}.`,
    disabledDictionary: (name: string) => `Disabled ${name}. Existing references remain valid.`,
    updated: (name: string, count: number) => `Updated ${name}; ${count} product(s) queued for publication.`,
    createdSpec: (name: string) => `Created specification ${name}.`,
    createdSpecSet: (name: string) => `Created Spec Set ${name}.`,
    assignedSpecSet: (name: string) => `Assigned Spec Set to ${name}.`,
    newCategory: "New category", name: "Name", slug: "Slug", parent: "Parent", create: "Create",
    categoryData: "Category tree data", refresh: "Refresh", root: "Root", status: "Status", revision: "Revision", actions: "Actions",
    edit: "Edit", translate: "Translations", disable: "Disable", disableCategoryConfirm: "Disable this category? Existing references are preserved.",
    dictionary: "Dictionary", optionalSlug: "Optional slug", disableValueConfirm: "Disable this value? Existing references remain valid.",
    newSpec: "New specification", preferredUnit: "Preferred unit", filterable: "Filterable", newSpecSet: "New Spec Set",
    assignSpecSet: "Assign exact category Spec Set", category: "Category", specSet: "Spec Set", assign: "Assign",
    definitions: "Spec definitions and sets", unit: "Unit", semanticVersion: "Semantic version", yes: "Yes", no: "No", members: "Members",
    editCategory: "Edit category", editDictionary: "Edit dictionary value", applyReviewed: "Apply reviewed change",
    previewAffected: "Preview affected products and routes", active: "Active", disabled: "Disabled",
    affected: (count: number) => `${count} affected product(s)`, notPublic: "not currently public",
    noChanges: "No Product publication changes required.",
    kinds: { manufacturer: "Manufacturer", brand: "Brand", lifecycle: "Lifecycle", application: "Application", document_type: "Document type" } as Record<DictionaryKind, string>,
  },
  "zh-TW": {
    categories: "分類", dictionaries: "字典", specifications: "規格",
    createdCategory: (name: string) => `已建立分類 ${name}。`,
    createdDictionary: (kind: string, name: string) => `已建立${kind} ${name}。`,
    disabledCategory: (name: string) => `已停用分類 ${name}。`,
    disabledDictionary: (name: string) => `已停用 ${name}；既有參照仍然有效。`,
    updated: (name: string, count: number) => `已更新 ${name}；${count} 項產品已排入重新發布。`,
    createdSpec: (name: string) => `已建立規格 ${name}。`,
    createdSpecSet: (name: string) => `已建立 Spec Set ${name}。`,
    assignedSpecSet: (name: string) => `已將 Spec Set 指派給 ${name}。`,
    newCategory: "新增分類", name: "名稱", slug: "Slug", parent: "上層分類", create: "建立",
    categoryData: "分類樹資料", refresh: "重新整理", root: "根分類", status: "狀態", revision: "修訂", actions: "操作",
    edit: "編輯", translate: "翻譯", disable: "停用", disableCategoryConfirm: "要停用這個分類嗎？既有參照會保留。",
    dictionary: "字典", optionalSlug: "選填 Slug", disableValueConfirm: "要停用這個值嗎？既有參照仍然有效。",
    newSpec: "新增規格", preferredUnit: "偏好單位", filterable: "可篩選", newSpecSet: "新增 Spec Set",
    assignSpecSet: "指派分類專屬 Spec Set", category: "分類", specSet: "Spec Set", assign: "指派",
    definitions: "規格定義與集合", unit: "單位", semanticVersion: "語意版本", yes: "是", no: "否", members: "成員",
    editCategory: "編輯分類", editDictionary: "編輯字典值", applyReviewed: "套用已檢視的變更",
    previewAffected: "預覽受影響的產品與路由", active: "有效", disabled: "停用",
    affected: (count: number) => `${count} 項受影響產品`, notPublic: "目前未公開",
    noChanges: "不需要變更任何 Product publication。",
    kinds: { manufacturer: "製造商", brand: "品牌", lifecycle: "生命週期", application: "應用", document_type: "文件類型" } as Record<DictionaryKind, string>,
  },
"zh-CN": {
    categories: "\u7C7B\u522B", dictionaries: "\u8BCD\u5178", specifications: "\u89C4\u683C",
    createdCategory: (name: string) => `\u5DF2\u521B\u5EFA\u7C7B\u522B${name}.`,
    createdDictionary: (kind: string, name: string) => `\u5DF2\u521B\u5EFA${kind} ${name}.`,
    disabledCategory: (name: string) => `\u6B8B\u75BE\u4EBA\u7C7B\u522B${name}.`,
    disabledDictionary: (name: string) => `\u6B8B\u75BE\u4EBA${name}\u3002\u73B0\u6709\u53C2\u8003\u4ECD\u7136\u6709\u6548\u3002`,
    updated: (name: string, count: number) => `\u5DF2\u66F4\u65B0${name}; ${count}\u6392\u961F\u7B49\u5F85\u53D1\u5E03\u7684\u4EA7\u54C1\u3002`,
    createdSpec: (name: string) => `\u521B\u5EFA\u89C4\u683C${name}.`,
    createdSpecSet: (name: string) => `\u521B\u5EFA\u89C4\u683C\u96C6${name}.`,
    assignedSpecSet: (name: string) => `\u6307\u5B9A\u89C4\u683C\u96C6\u81F3${name}.`,
    newCategory: "\u65B0\u7C7B\u522B", name: "\u59D3\u540D", slug: "\u86DE\u8753", parent: "\u5BB6\u957F", create: "\u521B\u9020",
    categoryData: "\u7C7B\u522B\u6811\u6570\u636E", refresh: "\u5237\u65B0", root: "\u6839", status: "\u5730\u4F4D", revision: "\u4FEE\u8BA2", actions: "\u884C\u52A8",
    edit: "\u7F16\u8F91", translate: "\u7FFB\u8BD1", disable: "\u7981\u7528", disableCategoryConfirm: "\u7981\u7528\u8BE5\u7C7B\u522B\uFF1F\u73B0\u6709\u53C2\u8003\u6587\u732E\u5C06\u88AB\u4FDD\u7559\u3002",
    dictionary: "\u5B57\u5178", optionalSlug: "\u53EF\u9009\u7684\u6BB5\u5934", disableValueConfirm: "\u7981\u7528\u8BE5\u503C\uFF1F\u73B0\u6709\u53C2\u8003\u4ECD\u7136\u6709\u6548\u3002",
    newSpec: "\u65B0\u89C4\u683C", preferredUnit: "\u9996\u9009\u5355\u4F4D", filterable: "\u53EF\u8FC7\u6EE4", newSpecSet: "\u65B0\u89C4\u683C\u96C6",
    assignSpecSet: "\u6307\u5B9A\u51C6\u786E\u7684\u7C7B\u522B\u89C4\u683C\u96C6", category: "\u7C7B\u522B", specSet: "\u89C4\u683C\u96C6", assign: "\u5206\u914D",
    definitions: "\u89C4\u683C\u5B9A\u4E49\u548C\u96C6", unit: "\u5355\u5143", semanticVersion: "\u8BED\u4E49\u7248\u672C", yes: "\u662F\u7684", no: "\u4E0D", members: "\u4F1A\u5458",
    editCategory: "\u7F16\u8F91\u7C7B\u522B", editDictionary: "\u7F16\u8F91\u5B57\u5178\u503C", applyReviewed: "\u5E94\u7528\u5DF2\u5BA1\u6838\u7684\u53D8\u66F4",
    previewAffected: "Preview \u53D7\u5F71\u54CD\u7684\u4EA7\u54C1\u548C\u8DEF\u7EBF", active: "\u79EF\u6781\u7684", disabled: "\u6B8B\u75BE\u4EBA",
    affected: (count: number) => `${count}\u53D7\u5F71\u54CD\u7684\u4EA7\u54C1`, notPublic: "\u76EE\u524D\u5C1A\u672A\u516C\u5F00",
    noChanges: "\u65E0\u9700\u66F4\u6539 Product \u51FA\u7248\u7269\u3002",
    kinds: { manufacturer: "\u5236\u9020\u5546", brand: "\u54C1\u724C", lifecycle: "\u751F\u547D\u5468\u671F", application: "\u5E94\u7528", document_type: "\u6587\u4EF6\u7C7B\u578B" } as Record<DictionaryKind, string>,
},
"ja-JP": {
    categories: "\u30AB\u30C6\u30B4\u30EA\u30FC", dictionaries: "\u8F9E\u66F8", specifications: "\u4ED5\u69D8",
    createdCategory: (name: string) => `\u4F5C\u6210\u3057\u305F\u30AB\u30C6\u30B4\u30EA${name}.`,
    createdDictionary: (kind: string, name: string) => `\u4F5C\u6210\u3055\u308C\u307E\u3057\u305F${kind} ${name}.`,
    disabledCategory: (name: string) => `\u969C\u5BB3\u8005\u306E\u30AB\u30C6\u30B4\u30EA${name}.`,
    disabledDictionary: (name: string) => `\u7121\u52B9${name}\u3002\u65E2\u5B58\u306E\u53C2\u7167\u306F\u5F15\u304D\u7D9A\u304D\u6709\u52B9\u3067\u3059\u3002`,
    updated: (name: string, count: number) => `\u66F4\u65B0\u3055\u308C\u307E\u3057\u305F${name}; ${count}\u516C\u958B\u5F85\u3061\u306E\u88FD\u54C1\u3002`,
    createdSpec: (name: string) => `\u4F5C\u6210\u3057\u305F\u4ED5\u69D8\u66F8${name}.`,
    createdSpecSet: (name: string) => `\u4F5C\u6210\u3055\u308C\u305F\u4ED5\u69D8\u30BB\u30C3\u30C8${name}.`,
    assignedSpecSet: (name: string) => `\u5272\u308A\u5F53\u3066\u3089\u308C\u305F\u4ED5\u69D8\u30BB\u30C3\u30C8${name}.`,
    newCategory: "\u65B0\u3057\u3044\u30AB\u30C6\u30B4\u30EA", name: "\u540D\u524D", slug: "\u30CA\u30E1\u30AF\u30B8", parent: "\u89AA", create: "\u4F5C\u6210\u3059\u308B",
    categoryData: "\u30AB\u30C6\u30B4\u30EA\u30C4\u30EA\u30FC\u30C7\u30FC\u30BF", refresh: "\u30EA\u30D5\u30EC\u30C3\u30B7\u30E5", root: "\u6839", status: "\u72B6\u614B", revision: "\u30EA\u30D3\u30B8\u30E7\u30F3", actions: "\u30A2\u30AF\u30B7\u30E7\u30F3",
    edit: "\u7DE8\u96C6", translate: "\u7FFB\u8A33", disable: "\u7121\u52B9\u306B\u3059\u308B", disableCategoryConfirm: "\u3053\u306E\u30AB\u30C6\u30B4\u30EA\u3092\u7121\u52B9\u306B\u3057\u307E\u3059\u304B?\u65E2\u5B58\u306E\u53C2\u7167\u306F\u4FDD\u6301\u3055\u308C\u307E\u3059\u3002",
    dictionary: "\u8F9E\u66F8", optionalSlug: "\u30AA\u30D7\u30B7\u30E7\u30F3\u306E\u30B9\u30E9\u30C3\u30B0", disableValueConfirm: "\u3053\u306E\u5024\u3092\u7121\u52B9\u306B\u3057\u307E\u3059\u304B?\u65E2\u5B58\u306E\u53C2\u7167\u306F\u5F15\u304D\u7D9A\u304D\u6709\u52B9\u3067\u3059\u3002",
    newSpec: "\u65B0\u4ED5\u69D8", preferredUnit: "\u512A\u5148\u5358\u4F4D", filterable: "\u30D5\u30A3\u30EB\u30BF\u30FC\u53EF\u80FD", newSpecSet: "\u65B0\u3057\u3044\u4ED5\u69D8\u30BB\u30C3\u30C8",
    assignSpecSet: "\u6B63\u78BA\u306A\u30AB\u30C6\u30B4\u30EA\u4ED5\u69D8\u30BB\u30C3\u30C8\u3092\u5272\u308A\u5F53\u3066\u308B", category: "\u30AB\u30C6\u30B4\u30EA", specSet: "\u30B9\u30DA\u30C3\u30AF\u30BB\u30C3\u30C8", assign: "\u5272\u308A\u5F53\u3066\u308B",
    definitions: "\u4ED5\u69D8\u306E\u5B9A\u7FA9\u3068\u30BB\u30C3\u30C8", unit: "\u30E6\u30CB\u30C3\u30C8", semanticVersion: "\u30BB\u30DE\u30F3\u30C6\u30A3\u30C3\u30AF\u30D0\u30FC\u30B8\u30E7\u30F3", yes: "\u306F\u3044", no: "\u3044\u3044\u3048", members: "\u30E1\u30F3\u30D0\u30FC",
    editCategory: "\u30AB\u30C6\u30B4\u30EA\u3092\u7DE8\u96C6\u3059\u308B", editDictionary: "\u8F9E\u66F8\u306E\u5024\u3092\u7DE8\u96C6\u3059\u308B", applyReviewed: "\u30EC\u30D3\u30E5\u30FC\u3055\u308C\u305F\u5909\u66F4\u3092\u9069\u7528\u3059\u308B",
    previewAffected: "Preview \u5F71\u97FF\u3092\u53D7\u3051\u308B\u5546\u54C1\u3068\u30EB\u30FC\u30C8", active: "\u30A2\u30AF\u30C6\u30A3\u30D6", disabled: "\u7121\u52B9",
    affected: (count: number) => `${count}\u5F71\u97FF\u3092\u53D7\u3051\u308B\u88FD\u54C1`, notPublic: "\u73FE\u5728\u975E\u516C\u958B",
    noChanges: "Product \u30D1\u30D6\u30EA\u30B1\u30FC\u30B7\u30E7\u30F3\u306E\u5909\u66F4\u306F\u5FC5\u8981\u3042\u308A\u307E\u305B\u3093\u3002",
    kinds: { manufacturer: "\u30E1\u30FC\u30AB\u30FC", brand: "\u30D6\u30E9\u30F3\u30C9", lifecycle: "\u30E9\u30A4\u30D5\u30B5\u30A4\u30AF\u30EB", application: "\u5FDC\u7528", document_type: "\u6587\u66F8\u306E\u7A2E\u985E" } as Record<DictionaryKind, string>,
},
"ko-KR": {
    categories: "\uCE74\uD14C\uACE0\uB9AC", dictionaries: "\uC0AC\uC804", specifications: "\uBA85\uC138\uC11C",
    createdCategory: (name: string) => `\uC0DD\uC131\uB41C \uCE74\uD14C\uACE0\uB9AC${name}.`,
    createdDictionary: (kind: string, name: string) => `\uC0DD\uC131\uB428${kind} ${name}.`,
    disabledCategory: (name: string) => `\uC7A5\uC560\uC778 \uCE74\uD14C\uACE0\uB9AC${name}.`,
    disabledDictionary: (name: string) => `\uC7A5\uC560\uAC00 \uC788\uB294${name}. \uAE30\uC874 \uCC38\uC870\uB294 \uACC4\uC18D \uC720\uD6A8\uD569\uB2C8\uB2E4.`,
    updated: (name: string, count: number) => `\uC5C5\uB370\uC774\uD2B8\uB428${name}; ${count}\uAC8C\uC2DC \uB300\uAE30 \uC911\uC778 \uC81C\uD488\uC785\uB2C8\uB2E4.`,
    createdSpec: (name: string) => `\uC0DD\uC131\uB41C \uC0AC\uC591${name}.`,
    createdSpecSet: (name: string) => `\uC0AC\uC591 \uC138\uD2B8\uAC00 \uC0DD\uC131\uB418\uC5C8\uC2B5\uB2C8\uB2E4.${name}.`,
    assignedSpecSet: (name: string) => `\uD560\uB2F9\uB41C \uC0AC\uC591 \uC138\uD2B8${name}.`,
    newCategory: "\uC0C8\uB85C\uC6B4 \uCE74\uD14C\uACE0\uB9AC", name: "\uC774\uB984", slug: "\uAC15\uD0C0", parent: "\uC870\uC0C1", create: "\uB9CC\uB4E4\uB2E4",
    categoryData: "\uCE74\uD14C\uACE0\uB9AC \uD2B8\uB9AC \uB370\uC774\uD130", refresh: "\uC0C8\uB85C \uACE0\uCE58\uB2E4", root: "\uBFCC\uB9AC", status: "\uC0C1\uD0DC", revision: "\uAC1C\uC815", actions: "\uD589\uC704",
    edit: "\uD3B8\uC9D1\uD558\uB2E4", translate: "\uBC88\uC5ED", disable: "\uC7A5\uC560\uB97C \uC785\uD788\uB2E4", disableCategoryConfirm: "\uC774 \uCE74\uD14C\uACE0\uB9AC\uB97C \uBE44\uD65C\uC131\uD654\uD558\uC2DC\uACA0\uC2B5\uB2C8\uAE4C? \uAE30\uC874 \uCC38\uC870\uB294 \uC720\uC9C0\uB429\uB2C8\uB2E4.",
    dictionary: "\uC0AC\uC804", optionalSlug: "\uC120\uD0DD\uC801 \uC2AC\uB7EC\uADF8", disableValueConfirm: "\uC774 \uAC12\uC744 \uBE44\uD65C\uC131\uD654\uD558\uC2DC\uACA0\uC2B5\uB2C8\uAE4C? \uAE30\uC874 \uCC38\uC870\uB294 \uACC4\uC18D \uC720\uD6A8\uD569\uB2C8\uB2E4.",
    newSpec: "\uC0C8\uB85C\uC6B4 \uC0AC\uC591", preferredUnit: "\uC120\uD638\uD558\uB294 \uB2E8\uC704", filterable: "\uD544\uD130\uB9C1 \uAC00\uB2A5", newSpecSet: "\uC0C8\uB85C\uC6B4 \uC0AC\uC591 \uC138\uD2B8",
    assignSpecSet: "\uC815\uD655\uD55C \uCE74\uD14C\uACE0\uB9AC \uC0AC\uC591 \uC138\uD2B8 \uC9C0\uC815", category: "\uBC94\uC8FC", specSet: "\uC0AC\uC591 \uC138\uD2B8", assign: "\uC591\uC218\uC778",
    definitions: "\uC0AC\uC591 \uC815\uC758 \uBC0F \uC138\uD2B8", unit: "\uB2E8\uC704", semanticVersion: "\uC758\uBBF8\uB860\uC801 \uBC84\uC804", yes: "\uC608", no: "\uC544\uB2C8\uC694", members: "\uD68C\uC6D0",
    editCategory: "\uCE74\uD14C\uACE0\uB9AC \uC218\uC815", editDictionary: "\uC0AC\uC804 \uAC12 \uD3B8\uC9D1", applyReviewed: "\uAC80\uD1A0\uB41C \uBCC0\uACBD\uC0AC\uD56D \uC801\uC6A9",
    previewAffected: "Preview \uC601\uD5A5\uC744 \uBC1B\uB294 \uC81C\uD488 \uBC0F \uACBD\uB85C", active: "\uD65C\uB3D9\uC801\uC778", disabled: "\uC7A5\uC560\uAC00 \uC788\uB294",
    affected: (count: number) => `${count}\uC601\uD5A5\uC744 \uBC1B\uB294 \uC81C\uD488`, notPublic: "\uD604\uC7AC \uACF5\uAC1C\uB418\uC9C0 \uC54A\uC74C",
    noChanges: "Product \uAC8C\uC2DC \uBCC0\uACBD\uC774 \uD544\uC694\uD558\uC9C0 \uC54A\uC2B5\uB2C8\uB2E4.",
    kinds: { manufacturer: "\uC81C\uC870\uC5C5\uCCB4", brand: "\uC0C1\uD45C", lifecycle: "\uC218\uBA85\uC8FC\uAE30", application: "\uC560\uD50C\uB9AC\uCF00\uC774\uC158", document_type: "\uBB38\uC11C \uC720\uD615" } as Record<DictionaryKind, string>,
},
"de-DE": {
    categories: "Kategorien", dictionaries: "W\u00F6rterb\u00FCcher", specifications: "Spezifikationen",
    createdCategory: (name: string) => `Kategorie erstellt${name}.`,
    createdDictionary: (kind: string, name: string) => `Erstellt${kind} ${name}.`,
    disabledCategory: (name: string) => `Kategorie Behinderte${name}.`,
    disabledDictionary: (name: string) => `Deaktiviert${name}. Bestehende Referenzen behalten ihre G\u00FCltigkeit.`,
    updated: (name: string, count: number) => `Aktualisiert${name}; ${count}Produkt(e) zur Ver\u00F6ffentlichung in der Warteschlange.`,
    createdSpec: (name: string) => `Spezifikation erstellt${name}.`,
    createdSpecSet: (name: string) => `Spezifikationssatz erstellt${name}.`,
    assignedSpecSet: (name: string) => `Zugewiesener Spezifikationssatz zu${name}.`,
    newCategory: "Neue Kategorie", name: "Name", slug: "Schnecke", parent: "Elternteil", create: "Erstellen",
    categoryData: "Kategoriebaumdaten", refresh: "Aktualisieren", root: "Wurzel", status: "Status", revision: "Revision", actions: "Aktionen",
    edit: "Bearbeiten", translate: "\u00DCbersetzungen", disable: "Deaktivieren", disableCategoryConfirm: "Diese Kategorie deaktivieren? Vorhandene Referenzen bleiben erhalten.",
    dictionary: "W\u00F6rterbuch", optionalSlug: "Optionaler Slug", disableValueConfirm: "Diesen Wert deaktivieren? Bestehende Referenzen behalten ihre G\u00FCltigkeit.",
    newSpec: "Neue Spezifikation", preferredUnit: "Bevorzugte Einheit", filterable: "Filterbar", newSpecSet: "Neues Spezifikationsset",
    assignSpecSet: "Weisen Sie den Spezifikationssatz der genauen Kategorie zu", category: "Kategorie", specSet: "Spezifikationssatz", assign: "Zuordnen",
    definitions: "Spezifikationsdefinitionen und -s\u00E4tze", unit: "Einheit", semanticVersion: "Semantische Version", yes: "Ja", no: "NEIN", members: "Mitglieder",
    editCategory: "Kategorie bearbeiten", editDictionary: "W\u00F6rterbuchwert bearbeiten", applyReviewed: "\u00DCberpr\u00FCfte \u00C4nderung anwenden",
    previewAffected: "Von Preview betroffene Produkte und Routen", active: "Aktiv", disabled: "Deaktiviert",
    affected: (count: number) => `${count}betroffene(s) Produkt(e)`, notPublic: "derzeit nicht \u00F6ffentlich",
    noChanges: "Keine Product-Ver\u00F6ffentlichungs\u00E4nderungen erforderlich.",
    kinds: { manufacturer: "Hersteller", brand: "Marke", lifecycle: "Lebenszyklus", application: "Anwendung", document_type: "Dokumenttyp" } as Record<DictionaryKind, string>,
},
"fr-FR": {
    categories: "Cat\u00E9gories", dictionaries: "Dictionnaires", specifications: "Caract\u00E9ristiques",
    createdCategory: (name: string) => `Cat\u00E9gorie cr\u00E9\u00E9e${name}.`,
    createdDictionary: (kind: string, name: string) => `Cr\u00E9\u00E9${kind} ${name}.`,
    disabledCategory: (name: string) => `Cat\u00E9gorie d\u00E9sactiv\u00E9e${name}.`,
    disabledDictionary: (name: string) => `D\u00E9sactiv\u00E9${name}. Les r\u00E9f\u00E9rences existantes restent valables.`,
    updated: (name: string, count: number) => `Mis \u00E0 jour${name}; ${count}produit(s) en attente de publication.`,
    createdSpec: (name: string) => `Sp\u00E9cification cr\u00E9\u00E9e${name}.`,
    createdSpecSet: (name: string) => `Ensemble de sp\u00E9cifications cr\u00E9\u00E9${name}.`,
    assignedSpecSet: (name: string) => `Ensemble de sp\u00E9cifications attribu\u00E9 \u00E0${name}.`,
    newCategory: "Nouvelle cat\u00E9gorie", name: "Nom", slug: "Limace", parent: "M\u00E8re", create: "Cr\u00E9er",
    categoryData: "Donn\u00E9es de l'arborescence des cat\u00E9gories", refresh: "Rafra\u00EEchir", root: "Racine", status: "Statut", revision: "R\u00E9vision", actions: "Actes",
    edit: "Modifier", translate: "Traductions", disable: "D\u00E9sactiver", disableCategoryConfirm: "D\u00E9sactiver cette cat\u00E9gorie\u00A0? Les r\u00E9f\u00E9rences existantes sont conserv\u00E9es.",
    dictionary: "Dictionnaire", optionalSlug: "Limace en option", disableValueConfirm: "D\u00E9sactiver cette valeur\u00A0? Les r\u00E9f\u00E9rences existantes restent valables.",
    newSpec: "Nouvelle sp\u00E9cification", preferredUnit: "Unit\u00E9 pr\u00E9f\u00E9r\u00E9e", filterable: "Filtrable", newSpecSet: "Nouvel ensemble de sp\u00E9cifications",
    assignSpecSet: "Attribuer un ensemble de sp\u00E9cifications de cat\u00E9gorie exacte", category: "Cat\u00E9gorie", specSet: "Ensemble de sp\u00E9cifications", assign: "Attribuer",
    definitions: "D\u00E9finitions et ensembles de sp\u00E9cifications", unit: "Unit\u00E9", semanticVersion: "Version s\u00E9mantique", yes: "Oui", no: "Non", members: "Membres",
    editCategory: "Modifier la cat\u00E9gorie", editDictionary: "Modifier la valeur du dictionnaire", applyReviewed: "Appliquer la modification r\u00E9vis\u00E9e",
    previewAffected: "Produits et itin\u00E9raires concern\u00E9s par Preview", active: "Actif", disabled: "D\u00E9sactiv\u00E9",
    affected: (count: number) => `${count}produit(s) concern\u00E9(s)`, notPublic: "pas actuellement public",
    noChanges: "Aucune modification de publication Product requise.",
    kinds: { manufacturer: "Fabricant", brand: "Marque", lifecycle: "Cycle de vie", application: "Application", document_type: "Type de document" } as Record<DictionaryKind, string>,
},
"it-IT": {
    categories: "Categorie", dictionaries: "Dizionari", specifications: "Specifiche",
    createdCategory: (name: string) => `Categoria creata${name}.`,
    createdDictionary: (kind: string, name: string) => `Creato${kind} ${name}.`,
    disabledCategory: (name: string) => `Categoria disabili${name}.`,
    disabledDictionary: (name: string) => `Disabilitato${name}. I riferimenti esistenti rimangono validi.`,
    updated: (name: string, count: number) => `Aggiornato${name}; ${count}prodotto(i) in coda per la pubblicazione.`,
    createdSpec: (name: string) => `Specifica creata${name}.`,
    createdSpecSet: (name: string) => `Set di specifiche creato${name}.`,
    assignedSpecSet: (name: string) => `Specifica assegnata impostata su${name}.`,
    newCategory: "Nuova categoria", name: "Nome", slug: "Lumaca", parent: "Genitore", create: "Creare",
    categoryData: "Dati dell'albero delle categorie", refresh: "Aggiorna", root: "Radice", status: "Stato", revision: "Revisione", actions: "Azioni",
    edit: "Modificare", translate: "Traduzioni", disable: "Disabilita", disableCategoryConfirm: "Disattivare questa categoria? I riferimenti esistenti vengono conservati.",
    dictionary: "Dizionario", optionalSlug: "Lumaca opzionale", disableValueConfirm: "Disabilitare questo valore? I riferimenti esistenti rimangono validi.",
    newSpec: "Nuova specifica", preferredUnit: "Unit\u00E0 preferita", filterable: "Filtrabile", newSpecSet: "Nuovo set di specifiche",
    assignSpecSet: "Assegnare la categoria esatta del set di specifiche", category: "Categoria", specSet: "Insieme delle specifiche", assign: "Assegnare",
    definitions: "Definizioni e insiemi di specifiche", unit: "Unit\u00E0", semanticVersion: "Versione semantica", yes: "S\u00CC", no: "NO", members: "Membri",
    editCategory: "Modifica categoria", editDictionary: "Modifica il valore del dizionario", applyReviewed: "Applica la modifica rivista",
    previewAffected: "", active: "Attivo", disabled: "Disabilitato",
    affected: (count: number) => `${count}prodotti interessati`, notPublic: "attualmente non pubblico",
    noChanges: "",
    kinds: { manufacturer: "Produttore", brand: "Marca", lifecycle: "Ciclo vitale", application: "Applicazione", document_type: "Tipo di documento" } as Record<DictionaryKind, string>,
},
"es-ES": {
    categories: "Categor\u00EDas", dictionaries: "Diccionarios", specifications: "Presupuesto",
    createdCategory: (name: string) => `Categor\u00EDa creada${name}.`,
    createdDictionary: (kind: string, name: string) => `Creado${kind} ${name}.`,
    disabledCategory: (name: string) => `Categor\u00EDa deshabilitada${name}.`,
    disabledDictionary: (name: string) => `Desactivado${name}. Las referencias existentes siguen siendo v\u00E1lidas.`,
    updated: (name: string, count: number) => `Actualizado${name}; ${count}producto(s) en cola para su publicaci\u00F3n.`,
    createdSpec: (name: string) => `Especificaci\u00F3n creada${name}.`,
    createdSpecSet: (name: string) => `Conjunto de especificaciones creado${name}.`,
    assignedSpecSet: (name: string) => `Conjunto de especificaciones asignado a${name}.`,
    newCategory: "Nueva categor\u00EDa", name: "Nombre", slug: "Babosa", parent: "Padre", create: "Crear",
    categoryData: "Datos del \u00E1rbol de categor\u00EDas", refresh: "Refrescar", root: "Ra\u00EDz", status: "Estado", revision: "Revisi\u00F3n", actions: "Comportamiento",
    edit: "Editar", translate: "Traducciones", disable: "Desactivar", disableCategoryConfirm: "\u00BFDesactivar esta categor\u00EDa? Se conservan las referencias existentes.",
    dictionary: "Diccionario", optionalSlug: "Babosa opcional", disableValueConfirm: "\u00BFDesactivar este valor? Las referencias existentes siguen siendo v\u00E1lidas.",
    newSpec: "Nueva especificaci\u00F3n", preferredUnit: "Unidad preferida", filterable: "Filtrable", newSpecSet: "Nuevo conjunto de especificaciones",
    assignSpecSet: "Asignar conjunto de especificaciones de categor\u00EDa exacta", category: "Categor\u00EDa", specSet: "Conjunto de especificaciones", assign: "Asignar",
    definitions: "Definiciones y conjuntos de especificaciones", unit: "Unidad", semanticVersion: "Versi\u00F3n sem\u00E1ntica", yes: "S\u00ED", no: "No", members: "Miembros",
    editCategory: "Editar categor\u00EDa", editDictionary: "Editar valor del diccionario", applyReviewed: "Aplicar el cambio revisado",
    previewAffected: "Preview productos y rutas afectados", active: "Activo", disabled: "Desactivado",
    affected: (count: number) => `${count}producto(s) afectado(s)`, notPublic: "actualmente no p\u00FAblico",
    noChanges: "No se requieren cambios en la publicaci\u00F3n Product.",
    kinds: { manufacturer: "Fabricante", brand: "Marca", lifecycle: "Ciclo vital", application: "Solicitud", document_type: "Tipo de documento" } as Record<DictionaryKind, string>,
},
"pt-BR": {
    categories: "Categorias", dictionaries: "Dicion\u00E1rios", specifications: "Especifica\u00E7\u00F5es",
    createdCategory: (name: string) => `Categoria criada${name}.`,
    createdDictionary: (kind: string, name: string) => `Criado${kind} ${name}.`,
    disabledCategory: (name: string) => `Categoria desativada${name}.`,
    disabledDictionary: (name: string) => `Desabilitado${name}. As refer\u00EAncias existentes permanecem v\u00E1lidas.`,
    updated: (name: string, count: number) => `Atualizado${name}; ${count}produto(s) na fila para publica\u00E7\u00E3o.`,
    createdSpec: (name: string) => `Especifica\u00E7\u00E3o criada${name}.`,
    createdSpecSet: (name: string) => `Conjunto de especifica\u00E7\u00F5es criado${name}.`,
    assignedSpecSet: (name: string) => `Conjunto de especifica\u00E7\u00F5es atribu\u00EDdo a${name}.`,
    newCategory: "Nova categoria", name: "Nome", slug: "Lesma", parent: "Pai", create: "Criar",
    categoryData: "Dados da \u00E1rvore de categorias", refresh: "Atualizar", root: "Raiz", status: "Status", revision: "Revis\u00E3o", actions: "A\u00E7\u00F5es",
    edit: "Editar", translate: "Tradu\u00E7\u00F5es", disable: "Desativar", disableCategoryConfirm: "Desativar esta categoria? As refer\u00EAncias existentes s\u00E3o preservadas.",
    dictionary: "Dicion\u00E1rio", optionalSlug: "Slug opcional", disableValueConfirm: "Desativar este valor? As refer\u00EAncias existentes permanecem v\u00E1lidas.",
    newSpec: "Nova especifica\u00E7\u00E3o", preferredUnit: "Unidade preferida", filterable: "Filtr\u00E1vel", newSpecSet: "Novo conjunto de especifica\u00E7\u00F5es",
    assignSpecSet: "Atribuir categoria exata do conjunto de especifica\u00E7\u00F5es", category: "Categoria", specSet: "Conjunto de especifica\u00E7\u00F5es", assign: "Atribuir",
    definitions: "Defini\u00E7\u00F5es e conjuntos de especifica\u00E7\u00F5es", unit: "Unidade", semanticVersion: "Vers\u00E3o sem\u00E2ntica", yes: "Sim", no: "N\u00E3o", members: "Membros",
    editCategory: "Editar categoria", editDictionary: "Editar valor do dicion\u00E1rio", applyReviewed: "Aplicar altera\u00E7\u00E3o revisada",
    previewAffected: "Produtos e rotas afetados Preview", active: "Ativo", disabled: "Desabilitado",
    affected: (count: number) => `${count}produto(s) afetado(s)`, notPublic: "atualmente n\u00E3o \u00E9 p\u00FAblico",
    noChanges: "Nenhuma altera\u00E7\u00E3o na publica\u00E7\u00E3o Product \u00E9 necess\u00E1ria.",
    kinds: { manufacturer: "Fabricante", brand: "Marca", lifecycle: "Vida \u00FAtil", application: "Aplicativo", document_type: "Tipo de documento" } as Record<DictionaryKind, string>,
},
} as const;

export function TaxonomyPanel({ locale, onError, onMessage }: Feedback & { locale: TaxonomyLocale }) {
  const text = labels[locale];
  const { capabilities } = useAdminIdentity();
  const canEdit = Boolean(capabilities["catalog.edit"]);
  const invalidate = useInvalidate();
  const [kind, setKind] = useState<DictionaryKind>("manufacturer");
	const categoriesList = useList<Category, APIError>({ resource: "categories", pagination: { mode: "off" }, queryOptions: { retry: false } });
	const dictionaryList = useList<DictionaryEntry, APIError>({ resource: "dictionary-entries", pagination: { mode: "off" }, meta: { kind }, queryOptions: { retry: false } });
	const specsList = useList<SpecDefinition, APIError>({ resource: "spec-definitions", pagination: { mode: "off" }, queryOptions: { retry: false } });
	const specSetsList = useList<SpecSet, APIError>({ resource: "spec-sets", pagination: { mode: "off" }, queryOptions: { retry: false } });
	const categories = categoriesList.result.data;
	const entries = dictionaryList.result.data;
	const specs = specsList.result.data;
	const specSets = specSetsList.result.data;
	const [editingCategory, setEditingCategory] = useState<Category>();
	const [editingDictionary, setEditingDictionary] = useState<DictionaryEntry>();
	const [translationTarget, setTranslationTarget] = useState<TaxonomyTranslationTarget>();
	const [categoryImpact, setCategoryImpact] = useState<TaxonomyImpact>();
	const [dictionaryImpact, setDictionaryImpact] = useState<TaxonomyImpact>();
	const [taxonomySaving, setTaxonomySaving] = useState(false);
	const [categoryForm] = Form.useForm<{ name: string; slug: string; parent_id?: string }>();
	const [dictionaryForm] = Form.useForm<{ name: string; slug?: string }>();
	const [categoryEditForm] = Form.useForm<{ name: string; slug: string; parent_id?: string }>();
	const [dictionaryEditForm] = Form.useForm<{ name: string; slug?: string }>();
  const [specForm] = Form.useForm<{ name: string; preferred_unit?: string; filterable: boolean; semantic_version: number }>();
  const [specSetForm] = Form.useForm<{ name: string; spec_ids: string[] }>();
  const [assignmentForm] = Form.useForm<{ category_id: string; spec_set_id: string }>();

  const queryError = categoriesList.query.error ?? dictionaryList.query.error ?? specsList.query.error ?? specSetsList.query.error;
  useEffect(() => { if (queryError) onError(queryError); }, [queryError]);

  const invalidateResources = (resources: readonly string[]) => Promise.all(
    resources.map((resource) => invalidate({ resource, invalidates: ["list", "detail"] })),
  );
  const loadCategories = () => invalidateResources(["categories"]);
  const loadDictionary = () => invalidateResources(["dictionary-entries"]);
  const loadSpecs = () => invalidateResources(["spec-definitions", "spec-sets"]);

  const createCategory = async (values: { name: string; slug: string; parent_id?: string }) => {
    try {
      await taxonomyCommands.createCategory(values);
      categoryForm.resetFields();
      onMessage(text.createdCategory(values.name));
      await invalidateResources(taxonomyInvalidations.category);
    } catch (error) {
      onError(error);
    }
  };

  const createDictionaryEntry = async (values: { name: string; slug?: string }) => {
    try {
      await taxonomyCommands.createDictionary(kind, values);
      dictionaryForm.resetFields();
      onMessage(text.createdDictionary(text.kinds[kind], values.name));
      await invalidateResources(taxonomyInvalidations.dictionary);
    } catch (error) {
      onError(error);
    }
  };

  const disableCategory = async (category: Category) => {
    try {
      await taxonomyCommands.disableCategory(category);
      onMessage(text.disabledCategory(category.name));
      await invalidateResources(taxonomyInvalidations.category);
    } catch (error) {
      onError(error);
    }
  };

	const disableDictionary = async (entry: DictionaryEntry) => {
    try {
		await taxonomyCommands.disableDictionary(entry);
      onMessage(text.disabledDictionary(entry.name));
      await invalidateResources(taxonomyInvalidations.dictionary);
    } catch (error) {
      onError(error);
    }
	};

	const openCategoryEdit = (category: Category) => {
		setEditingCategory(category);
		setCategoryImpact(undefined);
		categoryEditForm.setFieldsValue({ name: category.name, slug: category.slug, parent_id: category.parent_id });
	};

	const previewCategoryEdit = async () => {
		if (!editingCategory) return;
		try {
			const values = await categoryEditForm.validateFields();
			setCategoryImpact(await taxonomyCommands.previewCategory(editingCategory, values));
		} catch (error) {
			onError(error);
		}
	};

	const applyCategoryEdit = async () => {
		if (!editingCategory || !categoryImpact) return;
		setTaxonomySaving(true);
		try {
			const values = await categoryEditForm.validateFields();
			await taxonomyCommands.updateCategory(editingCategory, values);
      onMessage(text.updated(editingCategory.name, categoryImpact.affected_products.length));
			setEditingCategory(undefined);
			setCategoryImpact(undefined);
			await invalidateResources(taxonomyInvalidations.category);
		} catch (error) {
			onError(error);
		} finally {
			setTaxonomySaving(false);
		}
	};

	const openDictionaryEdit = (entry: DictionaryEntry) => {
		setEditingDictionary(entry);
		setDictionaryImpact(undefined);
		dictionaryEditForm.setFieldsValue({ name: entry.name, slug: entry.slug });
	};

	const previewDictionaryEdit = async () => {
		if (!editingDictionary) return;
		try {
			const values = await dictionaryEditForm.validateFields();
			setDictionaryImpact(await taxonomyCommands.previewDictionary(editingDictionary, values));
		} catch (error) {
			onError(error);
		}
	};

	const applyDictionaryEdit = async () => {
		if (!editingDictionary || !dictionaryImpact) return;
		setTaxonomySaving(true);
		try {
			const values = await dictionaryEditForm.validateFields();
			await taxonomyCommands.updateDictionary(editingDictionary, values);
      onMessage(text.updated(editingDictionary.name, dictionaryImpact.affected_products.length));
			setEditingDictionary(undefined);
			setDictionaryImpact(undefined);
			await invalidateResources(taxonomyInvalidations.dictionary);
		} catch (error) {
			onError(error);
		} finally {
			setTaxonomySaving(false);
		}
	};

  const createSpec = async (values: { name: string; preferred_unit?: string; filterable: boolean; semantic_version: number }) => {
    try {
      await taxonomyCommands.createSpec(values);
      specForm.resetFields();
      onMessage(text.createdSpec(values.name));
      await invalidateResources(taxonomyInvalidations.specifications);
    } catch (error) {
      onError(error);
    }
  };

  const createSpecSet = async (values: { name: string; spec_ids: string[] }) => {
    try {
      await taxonomyCommands.createSpecSet(values);
      specSetForm.resetFields();
      onMessage(text.createdSpecSet(values.name));
      await invalidateResources(taxonomyInvalidations.specifications);
    } catch (error) {
      onError(error);
    }
  };

  const assignSpecSet = async (values: { category_id: string; spec_set_id: string }) => {
    const category = categories.find((item) => item.id === values.category_id);
    if (!category) return;
    try {
      await taxonomyCommands.assignSpecSet(category, values.spec_set_id);
      assignmentForm.resetFields();
      onMessage(text.assignedSpecSet(category.name));
      await invalidateResources(taxonomyInvalidations.specifications);
    } catch (error) {
      onError(error);
      await invalidateResources(["categories"]);
    }
  };

  return (
    <>
    <Tabs
      items={[
        {
          key: "categories",
          label: text.categories,
          children: (
            <Space direction="vertical" size="large" className="panel-stack">
              {canEdit ? <Card title={text.newCategory}>
                <Form form={categoryForm} layout="inline" onFinish={(values) => void createCategory(values)}>
                  <Form.Item name="name" rules={[{ required: true }]}><Input placeholder={text.name} /></Form.Item>
                  <Form.Item name="slug" rules={[{ required: true }]}><Input placeholder={text.slug} /></Form.Item>
                  <Form.Item name="parent_id"><Select allowClear showSearch optionFilterProp="label" placeholder={text.parent} style={{ minWidth: 180 }} options={categories.filter((row) => row.status === "active").map((row) => ({ value: row.id, label: row.name }))} /></Form.Item>
                  <Button type="primary" htmlType="submit">{text.create}</Button>
                </Form>
              </Card> : null}
              <Card title={text.categoryData} extra={<Button onClick={() => void loadCategories()}>{text.refresh}</Button>}>
                <Table<Category>
                  rowKey="id"
                  dataSource={categories}
                  loading={categoriesList.query.isFetching}
                  pagination={false}
                  columns={[
                    { title: text.name, dataIndex: "name" },
                    { title: text.slug, dataIndex: "slug" },
                    { title: text.parent, render: (_, row) => categories.find((item) => item.id === row.parent_id)?.name ?? row.parent_id ?? text.root },
                    { title: text.status, render: (_, row) => <Tag color={row.status === "active" ? "green" : "default"}>{row.status === "active" ? text.active : text.disabled}</Tag> },
                    { title: text.revision, dataIndex: "revision" },
                    {
                      title: text.actions,
                      render: (_, row) => row.system_key === "root" ? null : (
                        <Space>
                          {canEdit ? <Button size="small" onClick={() => openCategoryEdit(row)}>{text.edit}</Button> : null}
                          <Button size="small" onClick={() => setTranslationTarget({ type: "category", id: row.id, name: row.name })}>{text.translate}</Button>
                          {canEdit && !row.system_key && row.status !== "disabled" ? (
                            <Popconfirm title={text.disableCategoryConfirm} onConfirm={() => void disableCategory(row)}>
                              <Button size="small" danger>{text.disable}</Button>
                            </Popconfirm>
                          ) : null}
                        </Space>
                      ),
                    },
                  ]}
                />
              </Card>
            </Space>
          ),
        },
        {
          key: "dictionaries",
          label: text.dictionaries,
          children: (
            <Space direction="vertical" size="large" className="panel-stack">
              <Card title={text.dictionary}>
                <Space direction="vertical" className="panel-stack">
                  <Select<DictionaryKind>
                    value={kind}
                    onChange={setKind}
                    options={dictionaryKinds.map((value) => ({ value, label: text.kinds[value] }))}
                    style={{ width: 220 }}
                  />
                  {canEdit ? <Form form={dictionaryForm} layout="inline" onFinish={(values) => void createDictionaryEntry(values)}>
                    <Form.Item name="name" rules={[{ required: true }]}><Input placeholder={text.name} /></Form.Item>
                    <Form.Item name="slug"><Input placeholder={text.optionalSlug} /></Form.Item>
                    <Button type="primary" htmlType="submit">{text.create}</Button>
                  </Form> : null}
                </Space>
              </Card>
              <Card title={text.kinds[kind]} extra={<Button onClick={() => void loadDictionary()}>{text.refresh}</Button>}>
                <Table<DictionaryEntry>
                  rowKey="id"
                  dataSource={entries}
                  loading={dictionaryList.query.isFetching}
                  pagination={false}
                  columns={[
                    { title: text.name, dataIndex: "name" },
                    { title: text.slug, dataIndex: "slug", render: (value: string) => value || "—" },
                    { title: text.status, render: (_, row) => <Tag color={row.status === "active" ? "green" : "default"}>{row.status === "active" ? text.active : text.disabled}</Tag> },
                    { title: text.revision, dataIndex: "revision" },
                    {
                      title: text.actions,
                      render: (_, row) => (
                        <Space>
                          {canEdit ? <Button size="small" onClick={() => openDictionaryEdit(row)}>{text.edit}</Button> : null}
                          <Button size="small" onClick={() => setTranslationTarget({ type: "dictionary", id: row.id, name: row.name })}>{text.translate}</Button>
                          {canEdit && row.status !== "disabled" ? (
                            <Popconfirm title={text.disableValueConfirm} onConfirm={() => void disableDictionary(row)}>
                              <Button size="small" danger>{text.disable}</Button>
                            </Popconfirm>
                          ) : null}
                        </Space>
                      ),
                    },
                  ]}
                />
              </Card>
            </Space>
          ),
        },
        {
          key: "specifications",
          label: text.specifications,
          children: (
            <Space direction="vertical" size="large" className="panel-stack">
              {canEdit ? <Card title={text.newSpec}>
                <Form form={specForm} layout="inline" initialValues={{ filterable: false, semantic_version: 1 }} onFinish={(values) => void createSpec(values)}>
                  <Form.Item name="name" rules={[{ required: true }]}><Input placeholder={text.name} /></Form.Item>
                  <Form.Item name="preferred_unit"><Input placeholder={text.preferredUnit} /></Form.Item>
                  <Form.Item name="semantic_version" rules={[{ required: true }]}><InputNumber min={1} precision={0} /></Form.Item>
                  <Form.Item name="filterable" valuePropName="checked"><Checkbox>{text.filterable}</Checkbox></Form.Item>
                  <Button type="primary" htmlType="submit">{text.create}</Button>
                </Form>
              </Card> : null}
              {canEdit ? <Card title={text.newSpecSet}>
                <Form form={specSetForm} layout="inline" onFinish={(values) => void createSpecSet(values)}>
                  <Form.Item name="name" rules={[{ required: true }]}><Input placeholder={text.name} /></Form.Item>
                  <Form.Item name="spec_ids" rules={[{ required: true }]}><Select mode="multiple" placeholder={text.specifications} style={{ minWidth: 320 }} options={specs.filter((spec) => spec.status === "active").map((spec) => ({ value: spec.id, label: spec.name }))} /></Form.Item>
                  <Button type="primary" htmlType="submit">{text.create}</Button>
                </Form>
              </Card> : null}
              {canEdit ? <Card title={text.assignSpecSet}>
                <Form form={assignmentForm} layout="inline" onFinish={(values) => void assignSpecSet(values)}>
                  <Form.Item name="category_id" rules={[{ required: true }]}><Select placeholder={text.category} style={{ minWidth: 220 }} options={categories.filter((category) => category.status === "active" && !category.system_key).map((category) => ({ value: category.id, label: category.name }))} /></Form.Item>
                  <Form.Item name="spec_set_id" rules={[{ required: true }]}><Select placeholder={text.specSet} style={{ minWidth: 220 }} options={specSets.filter((set) => set.status === "active").map((set) => ({ value: set.id, label: set.name }))} /></Form.Item>
                  <Button type="primary" htmlType="submit">{text.assign}</Button>
                </Form>
              </Card> : null}
              <Card title={text.definitions} extra={<Button onClick={() => void loadSpecs()}>{text.refresh}</Button>}>
                <Table<SpecDefinition>
                  rowKey="id"
                  dataSource={specs}
                  loading={specsList.query.isFetching}
                  pagination={false}
                  columns={[
                    { title: text.name, dataIndex: "name" },
                    { title: text.unit, dataIndex: "preferred_unit", render: (value: string) => value || "—" },
                    { title: text.semanticVersion, dataIndex: "semantic_version" },
                    { title: text.filterable, dataIndex: "filterable", render: (value: boolean) => value ? text.yes : text.no },
                    { title: text.status, dataIndex: "status", render: (value: string) => value === "active" ? text.active : text.disabled },
                  ]}
                />
                <Table<SpecSet>
                  className="top-gap"
                  rowKey="id"
                  dataSource={specSets}
                  loading={specSetsList.query.isFetching}
                  pagination={false}
                  columns={[
                    { title: text.specSet, dataIndex: "name" },
                    { title: text.members, dataIndex: "spec_ids", render: (ids: string[]) => ids.map((id) => specs.find((spec) => spec.id === id)?.name ?? id).join(", ") },
                    { title: text.revision, dataIndex: "revision" },
                    { title: text.status, dataIndex: "status", render: (value: string) => value === "active" ? text.active : text.disabled },
                  ]}
                />
              </Card>
            </Space>
          ),
        },
      ]}
    />
    <Modal
      open={Boolean(editingCategory)}
      title={text.editCategory}
      okText={text.applyReviewed}
      okButtonProps={{ disabled: !categoryImpact }}
      confirmLoading={taxonomySaving}
      onOk={() => void applyCategoryEdit()}
      onCancel={() => { setEditingCategory(undefined); setCategoryImpact(undefined); }}
    >
      <Form form={categoryEditForm} layout="vertical" onValuesChange={() => setCategoryImpact(undefined)}>
        <Form.Item name="name" label={text.name} rules={[{ required: true }]}><Input disabled={editingCategory?.system_key === "uncategorized"} /></Form.Item>
        <Form.Item name="slug" label={text.slug} rules={[{ required: true }]}><Input /></Form.Item>
        <Form.Item name="parent_id" label={text.parent}>
          <Select
            disabled={editingCategory?.system_key === "uncategorized"}
            showSearch
            optionFilterProp="label"
            options={categories.filter((row) => row.status === "active" && row.id !== editingCategory?.id).map((row) => ({ value: row.id, label: row.name }))}
          />
        </Form.Item>
      </Form>
      <Button onClick={() => void previewCategoryEdit()}>{text.previewAffected}</Button>
      {categoryImpact ? <ImpactSummary impact={categoryImpact} locale={locale} /> : null}
    </Modal>
    <Modal
      open={Boolean(editingDictionary)}
      title={text.editDictionary}
      okText={text.applyReviewed}
      okButtonProps={{ disabled: !dictionaryImpact }}
      confirmLoading={taxonomySaving}
      onOk={() => void applyDictionaryEdit()}
      onCancel={() => { setEditingDictionary(undefined); setDictionaryImpact(undefined); }}
    >
      <Form form={dictionaryEditForm} layout="vertical" onValuesChange={() => setDictionaryImpact(undefined)}>
        <Form.Item name="name" label={text.name} rules={[{ required: true }]}><Input /></Form.Item>
        <Form.Item name="slug" label={text.slug}><Input /></Form.Item>
      </Form>
      <Button onClick={() => void previewDictionaryEdit()}>{text.previewAffected}</Button>
      {dictionaryImpact ? <ImpactSummary impact={dictionaryImpact} locale={locale} /> : null}
    </Modal>
    <TaxonomyTranslationsModal
      target={translationTarget}
      locale={locale}
      canEdit={canEdit}
      onClose={() => setTranslationTarget(undefined)}
      onError={onError}
      onSaved={() => {
        void loadCategories();
        void loadDictionary();
      }}
    />
    </>
  );
}

function ImpactSummary({ impact, locale }: { impact: TaxonomyImpact; locale: TaxonomyLocale }) {
  const text = labels[locale];
  return (
    <div className="top-gap">
      <strong>{text.affected(impact.affected_products.length)}</strong>
      {impact.affected_products.length ? (
        <ul>
          {impact.affected_products.map((item) => (
            <li key={item.product_id}>
              {item.part_number}: {item.current_route || text.notPublic}
              {item.proposed_route && item.proposed_route !== item.current_route ? ` → ${item.proposed_route}` : ""}
            </li>
          ))}
        </ul>
      ) : <p>{text.noChanges}</p>}
    </div>
  );
}
