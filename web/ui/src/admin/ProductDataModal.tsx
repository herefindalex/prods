import { useEffect, useMemo, useState } from "react";
import { Alert, Button, Card, Checkbox, Form, Input, InputNumber, List, Modal, Popconfirm, Radio, Select, Space, Tag, Typography, Upload } from "antd";
import { api, APIError, clientID, postJSON, putJSON } from "./api";
import type { AdminLocale } from "./locales";
import type {
	Asset,
	DictionaryEntry,
  Product,
  ProductDocument,
	ProductImage,
  SpecDefinition,
  SpecSet,
  SpecValue,
  SpecValueDetail,
} from "./types";

type Props = {
 locale: AdminLocale;
  product?: Product;
  open: boolean;
  onClose: () => void;
  onChanged: () => Promise<void>;
  onError: (error: unknown) => void;
  onMessage: (message: string) => void;
};

const labels = {
 "en-US": {
  title: (part?: string, revision?: number) => part ? `Specs and documents — ${part} · r${revision}` : "Specs and documents",
  noSpecSet: "This product's category has no Spec Set assigned.", specs: "Specification values", noValues: "No values saved",
  active: "active", inactive: "inactive", sourceRevision: "source r", specification: "Specification", rawValue: "Raw value",
  localeOptional: "Locale (optional)", saveValue: "Save value", images: "Product images", noImages: "No product images",
  setPrimary: "Set primary", removeImageConfirm: "Remove this image reference?", remove: "Remove", primary: "Primary",
  altFallback: "Alt falls back to product name", order: "order", externalImage: "External image", managedAsset: "Managed asset",
  source: "Source", externalURL: "External URL", uploadImage: "Upload image", externalHTTP: "External HTTP(S) URL",
  imageTypes: "JPEG, PNG, or WebP", chooseUpload: "Choose and upload", altText: "Alt text",
  altHelp: "Optional; product name is the public fallback.", sortOrder: "Sort order", primaryImage: "Primary image", addImage: "Add image",
  documents: "Documents", noDocuments: "No documents", localPDF: "Local PDF", uploadPDF: "Upload PDF", label: "Label",
  documentType: "Document type", language: "Language", addDocument: "Add document",
  savedValue: (name: string) => `Saved ${name} raw value.`, uploadedDocument: (name: string) => `Uploaded ${name}. Add a document to publish its reference.`,
  uploadedImage: (name: string) => `Uploaded ${name}. Add it to the product image list to publish its reference.`,
  uploadImageFirst: "Upload an image before adding it.", imageAdded: "Product image added.",
  imageUpdated: (primary: boolean) => primary ? "Primary image updated." : "Product image updated.",
  imageRemoved: "Product image reference removed; the immutable asset remains available under the retention/GC policy.",
  uploadPDFFirst: "Upload a PDF before adding this document.", documentAdded: (label: string) => `Added ${label}.`,
 },
 "zh-TW": {
  title: (part?: string, revision?: number) => part ? `規格與文件 — ${part} · r${revision}` : "規格與文件",
  noSpecSet: "這項產品的分類尚未指派 Spec Set。", specs: "規格值", noValues: "尚未儲存規格值",
  active: "有效", inactive: "停用", sourceRevision: "來源修訂 r", specification: "規格", rawValue: "原始值",
  localeOptional: "語系（選填）", saveValue: "儲存規格值", images: "產品圖片", noImages: "尚無產品圖片",
  setPrimary: "設為主圖", removeImageConfirm: "要移除這個圖片參照嗎？", remove: "移除", primary: "主圖",
  altFallback: "Alt 文字將使用產品名稱", order: "順序", externalImage: "外部圖片", managedAsset: "受管理資產",
  source: "來源", externalURL: "外部 URL", uploadImage: "上傳圖片", externalHTTP: "外部 HTTP(S) URL",
  imageTypes: "JPEG、PNG 或 WebP", chooseUpload: "選擇並上傳", altText: "Alt 文字",
  altHelp: "選填；公開頁未填時使用產品名稱。", sortOrder: "排序", primaryImage: "主圖", addImage: "新增圖片",
  documents: "文件", noDocuments: "尚無文件", localPDF: "本機 PDF", uploadPDF: "上傳 PDF", label: "標籤",
  documentType: "文件類型", language: "語言", addDocument: "新增文件",
  savedValue: (name: string) => `已儲存「${name}」的原始值。`, uploadedDocument: (name: string) => `已上傳 ${name}；新增文件後才會發布其參照。`,
  uploadedImage: (name: string) => `已上傳 ${name}；加入產品圖片清單後才會發布其參照。`,
  uploadImageFirst: "請先上傳圖片再新增。", imageAdded: "已新增產品圖片。",
  imageUpdated: (primary: boolean) => primary ? "已更新主圖。" : "已更新產品圖片。",
  imageRemoved: "已移除產品圖片參照；immutable asset 仍依 retention／GC 政策保留。",
  uploadPDFFirst: "請先上傳 PDF 再新增文件。", documentAdded: (label: string) => `已新增「${label}」。`,
 },
"zh-CN": {
    title: (part?: string, revision?: number) => part ? `\u89C4\u683C\u548C\u6587\u4EF6 \u2014${part} \u00B7 r${revision}` : "\u89C4\u683C\u548C\u6587\u4EF6",
    noSpecSet: "\u8BE5\u4EA7\u54C1\u7684\u7C7B\u522B\u6CA1\u6709\u6307\u5B9A\u89C4\u683C\u96C6\u3002", specs: "\u89C4\u683C\u503C", noValues: "\u672A\u4FDD\u5B58\u4EFB\u4F55\u503C",
    active: "\u79EF\u6781\u7684", inactive: "\u4E0D\u6D3B\u8DC3\u7684", sourceRevision: "\u6E90r", specification: "\u89C4\u683C", rawValue: "\u539F\u59CB\u503C",
    localeOptional: "\u533A\u57DF\u8BBE\u7F6E\uFF08\u53EF\u9009\uFF09", saveValue: "\u8282\u7701\u4EF7\u503C", images: "Product \u56FE\u7247", noImages: "\u6CA1\u6709\u4EA7\u54C1\u56FE\u7247",
    setPrimary: "\u8BBE\u7F6E\u4E3B\u8981", removeImageConfirm: "\u5220\u9664\u6B64\u56FE\u50CF\u53C2\u8003\u5417\uFF1F", remove: "\u6D88\u9664", primary: "\u57FA\u672C\u7684",
    altFallback: "Alt \u56DE\u9000\u5230\u4EA7\u54C1\u540D\u79F0", order: "\u547D\u4EE4", externalImage: "\u5916\u90E8\u5F62\u8C61", managedAsset: "\u7BA1\u7406\u8D44\u4EA7",
    source: "\u6765\u6E90", externalURL: "\u5916\u7F6EURL", uploadImage: "\u4E0A\u4F20\u56FE\u7247", externalHTTP: "\u5916\u90E8 HTTP(S) URL",
    imageTypes: "JPEG\u3001PNG \u6216 WebP", chooseUpload: "\u9009\u62E9\u5E76\u4E0A\u4F20", altText: "\u66FF\u4EE3\u6587\u672C",
    altHelp: "\u9009\u4FEE\u7684;\u4EA7\u54C1\u540D\u79F0\u662F\u516C\u5171\u540E\u5907\u540D\u79F0\u3002", sortOrder: "\u6392\u5E8F\u987A\u5E8F", primaryImage: "\u4E3B\u56FE\u50CF", addImage: "\u6DFB\u52A0\u56FE\u7247",
    documents: "\u6587\u4EF6", noDocuments: "\u6CA1\u6709\u6587\u4EF6", localPDF: "\u672C\u5730PDF", uploadPDF: "\u4E0A\u4F20 PDF", label: "\u6807\u7B7E",
    documentType: "\u6587\u4EF6\u7C7B\u578B", language: "\u8BED\u8A00", addDocument: "\u6DFB\u52A0\u6587\u6863",
    savedValue: (name: string) => `\u5DF2\u4FDD\u5B58${name}\u539F\u59CB\u503C\u3002`, uploadedDocument: (name: string) => `\u5DF2\u4E0A\u4F20${name}\u3002\u6DFB\u52A0\u6587\u6863\u4EE5\u53D1\u5E03\u5176\u53C2\u8003\u3002`,
    uploadedImage: (name: string) => `\u5DF2\u4E0A\u4F20${name}\u3002\u5C06\u5176\u6DFB\u52A0\u5230\u4EA7\u54C1\u56FE\u7247\u5217\u8868\u4EE5\u53D1\u5E03\u5176\u53C2\u8003\u3002`,
    uploadImageFirst: "\u6DFB\u52A0\u56FE\u50CF\u4E4B\u524D\u5148\u4E0A\u4F20\u56FE\u50CF\u3002", imageAdded: "\u6DFB\u52A0\u4E86 Product \u56FE\u50CF\u3002",
    imageUpdated: (primary: boolean) => primary ? "\u4E3B\u56FE\u50CF\u5DF2\u66F4\u65B0\u3002" : "Product \u56FE\u50CF\u5DF2\u66F4\u65B0\u3002",
    imageRemoved: "Product \u56FE\u50CF\u53C2\u8003\u5DF2\u5220\u9664\uFF1B\u6839\u636E\u4FDD\u7559/GC \u653F\u7B56\uFF0C\u4E0D\u53EF\u53D8\u8D44\u4EA7\u4ECD\u7136\u53EF\u7528\u3002",
    uploadPDFFirst: "\u6DFB\u52A0\u6B64\u6587\u6863\u4E4B\u524D\u5148\u4E0A\u4F20 PDF\u3002", documentAdded: (label: string) => `\u989D\u5916${label}.`,
},
"ja-JP": {
    title: (part?: string, revision?: number) => part ? `\u4ED5\u69D8\u3068\u30C9\u30AD\u30E5\u30E1\u30F3\u30C8 \u2014${part} \u00B7 r${revision}` : "\u4ED5\u69D8\u3068\u30C9\u30AD\u30E5\u30E1\u30F3\u30C8",
    noSpecSet: "\u3053\u306E\u88FD\u54C1\u306E\u30AB\u30C6\u30B4\u30EA\u306B\u306F\u30B9\u30DA\u30C3\u30AF \u30BB\u30C3\u30C8\u304C\u5272\u308A\u5F53\u3066\u3089\u308C\u3066\u3044\u307E\u305B\u3093\u3002", specs: "\u4ED5\u69D8\u5024", noValues: "\u5024\u304C\u4FDD\u5B58\u3055\u308C\u3066\u3044\u307E\u305B\u3093",
    active: "\u30A2\u30AF\u30C6\u30A3\u30D6", inactive: "\u975E\u30A2\u30AF\u30C6\u30A3\u30D6\u306A", sourceRevision: "\u30BD\u30FC\u30B9r", specification: "\u4ED5\u69D8", rawValue: "\u751F\u306E\u5024",
    localeOptional: "\u30ED\u30B1\u30FC\u30EB (\u30AA\u30D7\u30B7\u30E7\u30F3)", saveValue: "\u5024\u3092\u4FDD\u5B58\u3059\u308B", images: "Product\u753B\u50CF", noImages: "\u5546\u54C1\u753B\u50CF\u306F\u3042\u308A\u307E\u305B\u3093",
    setPrimary: "\u30D7\u30E9\u30A4\u30DE\u30EA\u3092\u8A2D\u5B9A\u3059\u308B", removeImageConfirm: "\u3053\u306E\u753B\u50CF\u53C2\u7167\u3092\u524A\u9664\u3057\u307E\u3059\u304B?", remove: "\u53D6\u308A\u9664\u304F", primary: "\u4E3B\u8981\u306A",
    altFallback: "Alt \u3092\u62BC\u3059\u3068\u88FD\u54C1\u540D\u306B\u623B\u308A\u307E\u3059", order: "\u6CE8\u6587", externalImage: "\u5916\u89B3\u30A4\u30E1\u30FC\u30B8", managedAsset: "\u7BA1\u7406\u8CC7\u7523",
    source: "\u30BD\u30FC\u30B9", externalURL: "\u5916\u90E8URL", uploadImage: "\u753B\u50CF\u3092\u30A2\u30C3\u30D7\u30ED\u30FC\u30C9\u3059\u308B", externalHTTP: "\u5916\u90E8HTTP(S) URL",
    imageTypes: "JPEG\u3001PNG\u3001\u307E\u305F\u306F WebP", chooseUpload: "\u9078\u629E\u3057\u3066\u30A2\u30C3\u30D7\u30ED\u30FC\u30C9", altText: "\u4EE3\u66FF\u30C6\u30AD\u30B9\u30C8",
    altHelp: "\u30AA\u30D7\u30B7\u30E7\u30F3\u3002\u88FD\u54C1\u540D\u306F\u30D1\u30D6\u30EA\u30C3\u30AF\u30D5\u30A9\u30FC\u30EB\u30D0\u30C3\u30AF\u3067\u3059\u3002", sortOrder: "\u4E26\u3079\u66FF\u3048\u9806\u5E8F", primaryImage: "\u30D7\u30E9\u30A4\u30DE\u30EA\u753B\u50CF", addImage: "\u753B\u50CF\u3092\u8FFD\u52A0",
    documents: "\u66F8\u985E", noDocuments: "\u66F8\u985E\u304C\u3042\u308A\u307E\u305B\u3093", localPDF: "\u30ED\u30FC\u30AB\u30EB PDF", uploadPDF: "PDF\u3092\u30A2\u30C3\u30D7\u30ED\u30FC\u30C9\u3059\u308B", label: "\u30E9\u30D9\u30EB",
    documentType: "\u6587\u66F8\u306E\u7A2E\u985E", language: "\u8A00\u8A9E", addDocument: "\u30C9\u30AD\u30E5\u30E1\u30F3\u30C8\u306E\u8FFD\u52A0",
    savedValue: (name: string) => `\u4FDD\u5B58\u3055\u308C\u307E\u3057\u305F${name}\u751F\u306E\u5024\u3002`, uploadedDocument: (name: string) => `\u30A2\u30C3\u30D7\u30ED\u30FC\u30C9\u3055\u308C\u307E\u3057\u305F${name}\u3002\u30C9\u30AD\u30E5\u30E1\u30F3\u30C8\u3092\u8FFD\u52A0\u3057\u3066\u305D\u306E\u53C2\u7167\u3092\u516C\u958B\u3057\u307E\u3059\u3002`,
    uploadedImage: (name: string) => `\u30A2\u30C3\u30D7\u30ED\u30FC\u30C9\u3055\u308C\u307E\u3057\u305F${name}\u3002\u53C2\u7167\u3092\u516C\u958B\u3059\u308B\u306B\u306F\u3001\u88FD\u54C1\u753B\u50CF\u30EA\u30B9\u30C8\u306B\u8FFD\u52A0\u3057\u307E\u3059\u3002`,
    uploadImageFirst: "\u753B\u50CF\u3092\u8FFD\u52A0\u3059\u308B\u524D\u306B\u30A2\u30C3\u30D7\u30ED\u30FC\u30C9\u3057\u3066\u304F\u3060\u3055\u3044\u3002", imageAdded: "Product\u753B\u50CF\u3092\u8FFD\u52A0\u3057\u307E\u3057\u305F\u3002",
    imageUpdated: (primary: boolean) => primary ? "\u30D7\u30E9\u30A4\u30DE\u30EA\u753B\u50CF\u304C\u66F4\u65B0\u3055\u308C\u307E\u3057\u305F\u3002" : "Product\u753B\u50CF\u304C\u66F4\u65B0\u3055\u308C\u307E\u3057\u305F\u3002",
    imageRemoved: "Product \u753B\u50CF\u53C2\u7167\u304C\u524A\u9664\u3055\u308C\u307E\u3057\u305F\u3002\u4E0D\u5909\u306E\u8CC7\u7523\u306F\u3001\u4FDD\u6301/GC \u30DD\u30EA\u30B7\u30FC\u306E\u4E0B\u3067\u5F15\u304D\u7D9A\u304D\u5229\u7528\u53EF\u80FD\u3067\u3059\u3002",
    uploadPDFFirst: "\u3053\u306E\u30C9\u30AD\u30E5\u30E1\u30F3\u30C8\u3092\u8FFD\u52A0\u3059\u308B\u524D\u306B PDF \u3092\u30A2\u30C3\u30D7\u30ED\u30FC\u30C9\u3057\u3066\u304F\u3060\u3055\u3044\u3002", documentAdded: (label: string) => `\u8FFD\u52A0\u3057\u305F${label}.`,
},
"ko-KR": {
    title: (part?: string, revision?: number) => part ? `\uC0AC\uC591 \uBC0F \uBB38\uC11C \u2014${part} \u00B7 r${revision}` : "\uC0AC\uC591 \uBC0F \uBB38\uC11C",
    noSpecSet: "\uC774 \uC81C\uD488 \uCE74\uD14C\uACE0\uB9AC\uC5D0\uB294 \uC9C0\uC815\uB41C \uC0AC\uC591 \uC138\uD2B8\uAC00 \uC5C6\uC2B5\uB2C8\uB2E4.", specs: "\uC0AC\uC591 \uAC12", noValues: "\uC800\uC7A5\uB41C \uAC12\uC774 \uC5C6\uC2B5\uB2C8\uB2E4.",
    active: "\uD65C\uB3D9\uC801\uC778", inactive: "\uBE44\uD65C\uC131", sourceRevision: "\uC18C\uC2A4 r", specification: "\uC0AC\uC591", rawValue: "\uC6D0\uC2DC \uAC00\uCE58",
    localeOptional: "\uB85C\uCF00\uC77C(\uC120\uD0DD\uC0AC\uD56D)", saveValue: "\uAC12 \uC800\uC7A5", images: "Product \uC774\uBBF8\uC9C0", noImages: "\uC81C\uD488 \uC774\uBBF8\uC9C0\uAC00 \uC5C6\uC2B5\uB2C8\uB2E4.",
    setPrimary: "\uAE30\uBCF8 \uC124\uC815", removeImageConfirm: "\uC774 \uC774\uBBF8\uC9C0 \uCC38\uC870\uB97C \uC0AD\uC81C\uD558\uC2DC\uACA0\uC2B5\uB2C8\uAE4C?", remove: "\uC81C\uAC70\uD558\uB2E4", primary: "\uC8FC\uC694\uD55C",
    altFallback: "Alt\uB294 \uC81C\uD488 \uC774\uB984\uC73C\uB85C \uB300\uCCB4\uB429\uB2C8\uB2E4.", order: "\uC8FC\uBB38\uD558\uB2E4", externalImage: "\uC678\uBD80 \uC774\uBBF8\uC9C0", managedAsset: "\uAD00\uB9AC \uC790\uC0B0",
    source: "\uC6D0\uCC9C", externalURL: "\uC678\uBD80 URL", uploadImage: "\uC774\uBBF8\uC9C0 \uC5C5\uB85C\uB4DC", externalHTTP: "\uC678\uBD80 HTTP(S) URL",
    imageTypes: "JPEG, PNG \uB610\uB294 WebP", chooseUpload: "\uC120\uD0DD\uD558\uACE0 \uC5C5\uB85C\uB4DC\uD558\uC138\uC694", altText: "\uB300\uCCB4 \uD14D\uC2A4\uD2B8",
    altHelp: "\uC120\uD0DD \uACFC\uBAA9; \uC81C\uD488 \uC774\uB984\uC740 \uACF5\uAC1C \uD3F4\uBC31\uC785\uB2C8\uB2E4.", sortOrder: "\uC815\uB82C \uC21C\uC11C", primaryImage: "\uAE30\uBCF8 \uC774\uBBF8\uC9C0", addImage: "\uC774\uBBF8\uC9C0 \uCD94\uAC00",
    documents: "\uC11C\uB958", noDocuments: "\uBB38\uC11C \uC5C6\uC74C", localPDF: "\uB85C\uCEEC PDF", uploadPDF: "PDF \uC5C5\uB85C\uB4DC", label: "\uC0C1\uD45C",
    documentType: "\uBB38\uC11C \uC720\uD615", language: "\uC5B8\uC5B4", addDocument: "\uBB38\uC11C \uCD94\uAC00",
    savedValue: (name: string) => `\uC800\uC7A5\uB428${name}\uC6D0\uC2DC \uAC00\uCE58.`, uploadedDocument: (name: string) => `\uC5C5\uB85C\uB4DC\uB428${name}. \uCC38\uC870\uB97C \uAC8C\uC2DC\uD558\uB824\uBA74 \uBB38\uC11C\uB97C \uCD94\uAC00\uD558\uC138\uC694.`,
    uploadedImage: (name: string) => `\uC5C5\uB85C\uB4DC\uB428${name}. \uCC38\uC870\uB97C \uAC8C\uC2DC\uD558\uB824\uBA74 \uC81C\uD488 \uC774\uBBF8\uC9C0 \uBAA9\uB85D\uC5D0 \uCD94\uAC00\uD558\uC138\uC694.`,
    uploadImageFirst: "\uC774\uBBF8\uC9C0\uB97C \uCD94\uAC00\uD558\uAE30 \uC804\uC5D0 \uC5C5\uB85C\uB4DC\uD558\uC138\uC694.", imageAdded: "Product \uC774\uBBF8\uC9C0\uAC00 \uCD94\uAC00\uB418\uC5C8\uC2B5\uB2C8\uB2E4.",
    imageUpdated: (primary: boolean) => primary ? "\uAE30\uBCF8 \uC774\uBBF8\uC9C0\uAC00 \uC5C5\uB370\uC774\uD2B8\uB418\uC5C8\uC2B5\uB2C8\uB2E4." : "Product \uC774\uBBF8\uC9C0\uAC00 \uC5C5\uB370\uC774\uD2B8\uB418\uC5C8\uC2B5\uB2C8\uB2E4.",
    imageRemoved: "Product \uC774\uBBF8\uC9C0 \uCC38\uC870\uAC00 \uC81C\uAC70\uB418\uC5C8\uC2B5\uB2C8\uB2E4. \uBD88\uBCC0 \uC790\uC0B0\uC740 \uBCF4\uC874/GC \uC815\uCC45\uC5D0 \uB530\uB77C \uACC4\uC18D \uC0AC\uC6A9\uD560 \uC218 \uC788\uC2B5\uB2C8\uB2E4.",
    uploadPDFFirst: "\uC774 \uBB38\uC11C\uB97C \uCD94\uAC00\uD558\uAE30 \uC804\uC5D0 PDF\uB97C \uC5C5\uB85C\uB4DC\uD558\uC138\uC694.", documentAdded: (label: string) => `\uCD94\uAC00\uB428${label}.`,
},
"de-DE": {
    title: (part?: string, revision?: number) => part ? `Spezifikationen und Dokumente \u2013${part} \u00B7 r${revision}` : "Spezifikationen und Dokumente",
    noSpecSet: "Der Kategorie dieses Produkts ist kein Spezifikationssatz zugewiesen.", specs: "Spezifikationswerte", noValues: "Keine Werte gespeichert",
    active: "aktiv", inactive: "inaktiv", sourceRevision: "Quelle r", specification: "Spezifikation", rawValue: "Rohwert",
    localeOptional: "Gebietsschema (optional)", saveValue: "Wert sparen", images: "Product-Bilder", noImages: "Keine Produktbilder",
    setPrimary: "Prim\u00E4r festlegen", removeImageConfirm: "Diesen Bildverweis entfernen?", remove: "Entfernen", primary: "Prim\u00E4r",
    altFallback: "Alt greift auf den Produktnamen zur\u00FCck", order: "Befehl", externalImage: "Au\u00DFenbild", managedAsset: "Verwaltetes Verm\u00F6gen",
    source: "Quelle", externalURL: "Extern URL", uploadImage: "Bild hochladen", externalHTTP: "Externes HTTP(S) URL",
    imageTypes: "JPEG, PNG oder WebP", chooseUpload: "Ausw\u00E4hlen und hochladen", altText: "Alt-Text",
    altHelp: "Optional; Der Produktname ist der \u00F6ffentliche Fallback.", sortOrder: "Sortierreihenfolge", primaryImage: "Prim\u00E4res Bild", addImage: "Bild hinzuf\u00FCgen",
    documents: "Unterlagen", noDocuments: "Keine Dokumente", localPDF: "Lokales PDF", uploadPDF: "PDF hochladen", label: "Etikett",
    documentType: "Dokumenttyp", language: "Sprache", addDocument: "Dokument hinzuf\u00FCgen",
    savedValue: (name: string) => `Gespeichert${name}Rohwert.`, uploadedDocument: (name: string) => `Hochgeladen${name}. F\u00FCgen Sie ein Dokument hinzu, um seine Referenz zu ver\u00F6ffentlichen.`,
    uploadedImage: (name: string) => `Hochgeladen${name}. F\u00FCgen Sie es zur Produktbildliste hinzu, um seine Referenz zu ver\u00F6ffentlichen.`,
    uploadImageFirst: "Laden Sie ein Bild hoch, bevor Sie es hinzuf\u00FCgen.", imageAdded: "Product-Bild hinzugef\u00FCgt.",
    imageUpdated: (primary: boolean) => primary ? "Prim\u00E4res Bild aktualisiert." : "Product-Image aktualisiert.",
    imageRemoved: "Product-Bildreferenz entfernt; Der unver\u00E4nderliche Verm\u00F6genswert bleibt im Rahmen der Aufbewahrungs-/GC-Richtlinie verf\u00FCgbar.",
    uploadPDFFirst: "Laden Sie ein PDF hoch, bevor Sie dieses Dokument hinzuf\u00FCgen.", documentAdded: (label: string) => `Hinzugef\u00FCgt${label}.`,
},
"fr-FR": {
    title: (part?: string, revision?: number) => part ? `Sp\u00E9cifications et documents \u2014${part} \u00B7 r${revision}` : "Sp\u00E9cifications et documents",
    noSpecSet: "Aucun ensemble de sp\u00E9cifications n'est attribu\u00E9 \u00E0 la cat\u00E9gorie de ce produit.", specs: "Valeurs de sp\u00E9cification", noValues: "Aucune valeur enregistr\u00E9e",
    active: "actif", inactive: "inactif", sourceRevision: "source r", specification: "Sp\u00E9cification", rawValue: "Valeur brute",
    localeOptional: "Param\u00E8tres r\u00E9gionaux (facultatif)", saveValue: "Enregistrer de la valeur", images: "Images Product", noImages: "Aucune image de produit",
    setPrimary: "D\u00E9finir le primaire", removeImageConfirm: "Supprimer cette r\u00E9f\u00E9rence d'image\u00A0?", remove: "Retirer", primary: "Primaire",
    altFallback: "Alt revient au nom du produit", order: "commande", externalImage: "Image externe", managedAsset: "Actif g\u00E9r\u00E9",
    source: "Source", externalURL: "Externe URL", uploadImage: "T\u00E9l\u00E9charger une image", externalHTTP: "HTTP(S) externe URL",
    imageTypes: "JPEG, PNG ou WebP", chooseUpload: "Choisissez et t\u00E9l\u00E9chargez", altText: "Texte alternatif",
    altHelp: "Facultatif; le nom du produit est la solution de secours publique.", sortOrder: "Ordre de tri", primaryImage: "Image principale", addImage: "Ajouter une image",
    documents: "Documents", noDocuments: "Aucun document", localPDF: "PDF local", uploadPDF: "T\u00E9l\u00E9charger le PDF", label: "\u00C9tiquette",
    documentType: "Type de document", language: "Langue", addDocument: "Ajouter un document",
    savedValue: (name: string) => `Enregistr\u00E9${name}valeur brute.`, uploadedDocument: (name: string) => `T\u00E9l\u00E9charg\u00E9${name}. Ajoutez un document pour publier sa r\u00E9f\u00E9rence.`,
    uploadedImage: (name: string) => `T\u00E9l\u00E9charg\u00E9${name}. Ajoutez-le \u00E0 la liste des images du produit pour publier sa r\u00E9f\u00E9rence.`,
    uploadImageFirst: "T\u00E9l\u00E9chargez une image avant de l'ajouter.", imageAdded: "Image Product ajout\u00E9e.",
    imageUpdated: (primary: boolean) => primary ? "Image principale mise \u00E0 jour." : "Image Product mise \u00E0 jour.",
    imageRemoved: "R\u00E9f\u00E9rence d'image Product supprim\u00E9e\u00A0; l'actif immuable reste disponible dans le cadre de la politique de r\u00E9tention/GC.",
    uploadPDFFirst: "T\u00E9l\u00E9chargez un PDF avant d'ajouter ce document.", documentAdded: (label: string) => `Ajout\u00E9${label}.`,
},
"it-IT": {
    title: (part?: string, revision?: number) => part ? `Specifiche e documenti \u2014${part} \u00B7 r${revision}` : "Specifiche e documenti",
    noSpecSet: "Alla categoria di questo prodotto non \u00E8 assegnato alcun set di specifiche.", specs: "Valori di specifica", noValues: "Nessun valore salvato",
    active: "attivo", inactive: "inattivo", sourceRevision: "fonte r", specification: "Specifica", rawValue: "Valore grezzo",
    localeOptional: "Impostazioni internazionali (facoltativo)", saveValue: "Risparmia valore", images: "Immagini Product", noImages: "Nessuna immagine del prodotto",
    setPrimary: "Imposta primario", removeImageConfirm: "Rimuovere questo riferimento all'immagine?", remove: "Rimuovere", primary: "Primario",
    altFallback: "Alt torna al nome del prodotto", order: "ordine", externalImage: "Immagine esterna", managedAsset: "Risorsa gestita",
    source: "Fonte", externalURL: "Esterno URL", uploadImage: "Carica immagine", externalHTTP: "HTTP(S) esterno URL",
    imageTypes: "JPEG, PNG o WebP", chooseUpload: "Scegli e carica", altText: "Testo alternativo",
    altHelp: "Opzionale; il nome del prodotto \u00E8 il fallback pubblico.", sortOrder: "Ordinamento", primaryImage: "Immagine primaria", addImage: "Aggiungi immagine",
    documents: "Documenti", noDocuments: "Nessun documento", localPDF: "PDF locale", uploadPDF: "Carica PDF", label: "Etichetta",
    documentType: "Tipo di documento", language: "Lingua", addDocument: "Aggiungi documento",
    savedValue: (name: string) => `Salvato${name}valore grezzo.`, uploadedDocument: (name: string) => `Caricato${name}. Aggiungi un documento per pubblicarne il riferimento.`,
    uploadedImage: (name: string) => `Caricato${name}. Aggiungilo all'elenco delle immagini del prodotto per pubblicare il suo riferimento.`,
    uploadImageFirst: "Carica un'immagine prima di aggiungerla.", imageAdded: "Aggiunta immagine Product.",
    imageUpdated: (primary: boolean) => primary ? "Immagine principale aggiornata." : "Immagine Product aggiornata.",
    imageRemoved: "Riferimento immagine Product rimosso; la risorsa immutabile rimane disponibile in base alla politica di conservazione/GC.",
    uploadPDFFirst: "Carica un PDF prima di aggiungere questo documento.", documentAdded: (label: string) => `Aggiunto${label}.`,
},
"es-ES": {
    title: (part?: string, revision?: number) => part ? `Especificaciones y documentos.${part} \u00B7 r${revision}` : "Especificaciones y documentos",
    noSpecSet: "La categor\u00EDa de este producto no tiene ning\u00FAn conjunto de especificaciones asignado.", specs: "Valores de especificaci\u00F3n", noValues: "No hay valores guardados",
    active: "activo", inactive: "inactivo", sourceRevision: "fuente r", specification: "Especificaci\u00F3n", rawValue: "Valor bruto",
    localeOptional: "Configuraci\u00F3n regional (opcional)", saveValue: "Guardar valor", images: "Im\u00E1genes Product", noImages: "Sin im\u00E1genes de producto",
    setPrimary: "Establecer primario", removeImageConfirm: "\u00BFEliminar esta referencia de imagen?", remove: "Eliminar", primary: "Primario",
    altFallback: "Alt vuelve al nombre del producto", order: "orden", externalImage: "Imagen externa", managedAsset: "Activo gestionado",
    source: "Fuente", externalURL: "Externo URL", uploadImage: "Subir imagen", externalHTTP: "HTTP(S) externo URL",
    imageTypes: "JPEG, PNG o WebP", chooseUpload: "Elige y sube", altText: "Texto alternativo",
    altHelp: "Opcional; El nombre del producto es el recurso p\u00FAblico.", sortOrder: "orden de clasificaci\u00F3n", primaryImage: "Imagen principal", addImage: "Agregar imagen",
    documents: "Documentos", noDocuments: "Sin documentos", localPDF: "PDF locales", uploadPDF: "Subir PDF", label: "Etiqueta",
    documentType: "Tipo de documento", language: "Idioma", addDocument: "Agregar documento",
    savedValue: (name: string) => `Guardado${name}valor bruto.`, uploadedDocument: (name: string) => `subido${name}. Agregue un documento para publicar su referencia.`,
    uploadedImage: (name: string) => `subido${name}. Agr\u00E9guelo a la lista de im\u00E1genes del producto para publicar su referencia.`,
    uploadImageFirst: "Sube una imagen antes de agregarla.", imageAdded: "Imagen Product agregada.",
    imageUpdated: (primary: boolean) => primary ? "Imagen principal actualizada." : "Imagen Product actualizada.",
    imageRemoved: "Se elimin\u00F3 la referencia de imagen Product; el activo inmutable permanece disponible bajo la pol\u00EDtica de retenci\u00F3n/GC.",
    uploadPDFFirst: "Cargue un PDF antes de agregar este documento.", documentAdded: (label: string) => `Agregado${label}.`,
},
"pt-BR": {
    title: (part?: string, revision?: number) => part ? `Especifica\u00E7\u00F5es e documentos \u2014${part} \u00B7 r${revision}` : "Especifica\u00E7\u00F5es e documentos",
    noSpecSet: "A categoria deste produto n\u00E3o possui nenhum conjunto de especifica\u00E7\u00F5es atribu\u00EDdo.", specs: "Valores de especifica\u00E7\u00E3o", noValues: "Nenhum valor salvo",
    active: "ativo", inactive: "inativo", sourceRevision: "fonte r", specification: "Especifica\u00E7\u00E3o", rawValue: "Valor bruto",
    localeOptional: "Local (opcional)", saveValue: "Salvar valor", images: "Imagens Product", noImages: "Nenhuma imagem de produto",
    setPrimary: "Definir prim\u00E1rio", removeImageConfirm: "Remover esta refer\u00EAncia de imagem?", remove: "Remover", primary: "Prim\u00E1rio",
    altFallback: "Alt volta ao nome do produto", order: "ordem", externalImage: "Imagem externa", managedAsset: "Ativo gerenciado",
    source: "Fonte", externalURL: "URL externo", uploadImage: "Carregar imagem", externalHTTP: "HTTP(S) externo URL",
    imageTypes: "JPEG, PNG ou WebP", chooseUpload: "Escolha e carregue", altText: "Texto alternativo",
    altHelp: "Opcional; o nome do produto \u00E9 o substituto p\u00FAblico.", sortOrder: "Ordem de classifica\u00E7\u00E3o", primaryImage: "Imagem principal", addImage: "Adicionar imagem",
    documents: "Documentos", noDocuments: "Nenhum documento", localPDF: "PDF local", uploadPDF: "Carregar PDF", label: "R\u00F3tulo",
    documentType: "Tipo de documento", language: "Linguagem", addDocument: "Adicionar documento",
    savedValue: (name: string) => `Salvo${name}valor bruto.`, uploadedDocument: (name: string) => `Enviado${name}. Adicione um documento para publicar sua refer\u00EAncia.`,
    uploadedImage: (name: string) => `Enviado${name}. Adicione-o \u00E0 lista de imagens do produto para publicar sua refer\u00EAncia.`,
    uploadImageFirst: "Carregue uma imagem antes de adicion\u00E1-la.", imageAdded: "Imagem Product adicionada.",
    imageUpdated: (primary: boolean) => primary ? "Imagem prim\u00E1ria atualizada." : "Imagem Product atualizada.",
    imageRemoved: "Refer\u00EAncia de imagem Product removida; o ativo imut\u00E1vel permanece dispon\u00EDvel sob a pol\u00EDtica de reten\u00E7\u00E3o/GC.",
    uploadPDFFirst: "Carregue um PDF antes de adicionar este documento.", documentAdded: (label: string) => `Adicionado${label}.`,
},
};

export function ProductDataModal({ locale, product, open, onClose, onChanged, onError, onMessage }: Props) {
 const text = labels[locale];
  const [current, setCurrent] = useState<Product>();
  const [specs, setSpecs] = useState<SpecDefinition[]>([]);
  const [specSet, setSpecSet] = useState<SpecSet>();
  const [values, setValues] = useState<SpecValueDetail[]>([]);
  const [documents, setDocuments] = useState<ProductDocument[]>([]);
	const [images, setImages] = useState<ProductImage[]>([]);
	const [documentTypes, setDocumentTypes] = useState<DictionaryEntry[]>([]);
	const [documentSource, setDocumentSource] = useState<"external" | "upload">("external");
	const [uploadedAsset, setUploadedAsset] = useState<Asset>();
	const [uploading, setUploading] = useState(false);
	const [imageSource, setImageSource] = useState<"external" | "upload">("external");
	const [uploadedImageAsset, setUploadedImageAsset] = useState<Asset>();
	const [imageUploading, setImageUploading] = useState(false);
  const [valueForm] = Form.useForm<{ spec_id: string; raw_value: string; source_locale?: string }>();
	const [documentForm] = Form.useForm<{ label: string; document_type_id: string; external_url?: string; language?: string; sort_order: number }>();
	const [imageForm] = Form.useForm<{ external_url?: string; alt_text?: string; sort_order: number; primary?: boolean }>();

  const load = async () => {
    if (!product) return;
    try {
		const [nextProduct, nextSpecs, nextValues, nextDocuments, nextDocumentTypes, nextImages] = await Promise.all([
        api<Product>(`/admin/api/products/${product.id}`),
        api<SpecDefinition[]>("/admin/api/specs"),
        api<SpecValueDetail[]>(`/admin/api/products/${product.id}/spec-values`),
        api<ProductDocument[]>(`/admin/api/products/${product.id}/documents`),
        api<DictionaryEntry[]>("/admin/api/dictionaries?kind=document_type"),
		api<ProductImage[]>(`/admin/api/products/${product.id}/images`),
      ]);
      setCurrent(nextProduct);
      setSpecs(nextSpecs ?? []);
      setValues(nextValues ?? []);
      setDocuments(nextDocuments ?? []);
      setDocumentTypes(nextDocumentTypes ?? []);
		setImages(nextImages ?? []);
      try {
        setSpecSet(await api<SpecSet>(`/admin/api/categories/${nextProduct.category_id}/spec-set`));
      } catch (error) {
        if (error instanceof APIError && error.status === 404) setSpecSet(undefined);
        else throw error;
      }
    } catch (error) {
      onError(error);
    }
  };

  useEffect(() => {
    if (open) void load();
  }, [open, product?.id]);

  const applicableSpecs = useMemo(
    () => specs.filter((spec) => spec.status === "active" && specSet?.spec_ids.includes(spec.id)),
    [specs, specSet],
  );

  const saveValue = async (input: { spec_id: string; raw_value: string; source_locale?: string }) => {
    if (!current) return;
    try {
      const value = await postJSON<SpecValue>(`/admin/api/products/${current.id}/spec-values`, {
        expected_revision: current.revision,
        ...input,
        source_locale: input.source_locale || "",
      });
      valueForm.resetFields();
      onMessage(text.savedValue(specs.find((spec) => spec.id === value.spec_id)?.name ?? value.spec_id));
      await load();
      await onChanged();
    } catch (error) {
      onError(error);
      await load();
    }
  };

	const uploadPDF = async (file: File) => {
		if (!current) return;
		setUploading(true);
		try {
			const body = new FormData();
			body.append("file", file);
			const asset = await api<Asset>(`/admin/api/products/${current.id}/assets`, { method: "POST", body });
			setUploadedAsset(asset);
      onMessage(text.uploadedDocument(asset.original_filename));
		} catch (error) {
			onError(error);
		} finally {
			setUploading(false);
		}
	};

	const uploadProductImage = async (file: File) => {
		if (!current) return;
		setImageUploading(true);
		try {
			const body = new FormData();
			body.append("file", file);
			const asset = await api<Asset>(`/admin/api/products/${current.id}/assets`, { method: "POST", body });
			setUploadedImageAsset(asset);
      onMessage(text.uploadedImage(asset.original_filename));
		} catch (error) {
			onError(error);
		} finally {
			setImageUploading(false);
		}
	};

	const addImage = async (input: { external_url?: string; alt_text?: string; sort_order: number; primary?: boolean }) => {
		if (!current) return;
		if (imageSource === "upload" && !uploadedImageAsset) {
			onError(new Error(text.uploadImageFirst));
			return;
		}
		try {
			await postJSON<ProductImage>(`/admin/api/products/${current.id}/images`, {
				expected_revision: current.revision,
				image: {
					id: clientID("img"),
					asset_id: imageSource === "upload" ? uploadedImageAsset?.id : undefined,
					external_url: imageSource === "external" ? input.external_url : undefined,
					alt_text: input.alt_text || "",
					sort_order: input.sort_order,
					primary: input.primary ?? false,
				},
			});
			imageForm.resetFields();
			setUploadedImageAsset(undefined);
			onMessage(text.imageAdded);
			await load();
			await onChanged();
		} catch (error) {
			onError(error);
			await load();
		}
	};

	const updateImage = async (image: ProductImage, primary = image.primary) => {
		if (!current) return;
		try {
			await putJSON<ProductImage>(`/admin/api/products/${current.id}/images/${image.id}`, {
				expected_revision: current.revision,
				image: { ...image, primary },
			});
			onMessage(text.imageUpdated(primary));
			await load();
			await onChanged();
		} catch (error) {
			onError(error);
			await load();
		}
	};

	const deleteImage = async (image: ProductImage) => {
		if (!current) return;
		try {
			await postJSON<void>(`/admin/api/products/${current.id}/images/${image.id}/delete`, { expected_revision: current.revision });
			onMessage(text.imageRemoved);
			await load();
			await onChanged();
		} catch (error) {
			onError(error);
			await load();
		}
	};

	const addDocument = async (input: { label: string; document_type_id: string; external_url?: string; language?: string; sort_order: number }) => {
		if (!current) return;
		if (documentSource === "upload" && !uploadedAsset) {
			onError(new Error(text.uploadPDFFirst));
			return;
		}
		try {
			await postJSON<ProductDocument>(`/admin/api/products/${current.id}/documents`, {
				expected_revision: current.revision,
				document: {
					id: clientID("doc"),
					label: input.label,
					document_type_id: input.document_type_id,
					external_url: documentSource === "external" ? input.external_url : undefined,
					asset_id: documentSource === "upload" ? uploadedAsset?.id : undefined,
					language: input.language || "",
					sort_order: input.sort_order,
				},
			});
			documentForm.resetFields();
			setUploadedAsset(undefined);
      onMessage(text.documentAdded(input.label));
      await load();
      await onChanged();
    } catch (error) {
      onError(error);
      await load();
    }
  };

  return (
    <Modal open={open} onCancel={onClose} footer={null} width={900} title={text.title(current?.part_number, current?.revision)}>
      <Space direction="vertical" size="large" className="panel-stack">
        {!specSet && <Alert type="warning" showIcon message={text.noSpecSet} />}
        <Card title={text.specs} size="small">
          <List
            dataSource={values}
            locale={{ emptyText: text.noValues }}
            renderItem={(detail) => (
              <List.Item>
                <List.Item.Meta
                  title={specs.find((spec) => spec.id === detail.value.spec_id)?.name ?? detail.value.spec_id}
                  description={
                    <Space wrap>
                      <Typography.Text>{detail.value.raw_value}</Typography.Text>
                      <Tag color={detail.value.active ? "green" : "default"}>{detail.value.active ? text.active : text.inactive}</Tag>
                      <Tag>{text.sourceRevision}{detail.value.source_revision}</Tag>
                      {detail.normalized?.map((normalized) => <Tag key={normalized.id} color={normalized.status === "current" ? "blue" : "default"}>{normalized.source}:{normalized.status}</Tag>)}
                    </Space>
                  }
                />
              </List.Item>
            )}
          />
          <Form form={valueForm} layout="inline" onFinish={(input) => void saveValue(input)}>
            <Form.Item name="spec_id" rules={[{ required: true }]}><Select placeholder={text.specification} style={{ width: 220 }} options={applicableSpecs.map((spec) => ({ value: spec.id, label: `${spec.name}${spec.preferred_unit ? ` (${spec.preferred_unit})` : ""}` }))} /></Form.Item>
            <Form.Item name="raw_value" rules={[{ required: true }]}><Input placeholder={text.rawValue} /></Form.Item>
            <Form.Item name="source_locale"><Input placeholder={text.localeOptional} /></Form.Item>
            <Button type="primary" htmlType="submit" disabled={!applicableSpecs.length}>{text.saveValue}</Button>
          </Form>
        </Card>

		<Card title={text.images} size="small">
			<List
				dataSource={images}
				locale={{ emptyText: text.noImages }}
				renderItem={(image) => (
					<List.Item actions={[
						<Button key="primary" size="small" disabled={image.primary} onClick={() => void updateImage(image, true)}>{text.setPrimary}</Button>,
						<Popconfirm key="delete" title={text.removeImageConfirm} onConfirm={() => void deleteImage(image)}><Button size="small" danger>{text.remove}</Button></Popconfirm>,
					]}>
						<List.Item.Meta
							title={<Space>{image.primary && <Tag color="blue">{text.primary}</Tag>}<span>{image.alt_text || text.altFallback}</span></Space>}
							description={<Space wrap><Tag>{text.order} {image.sort_order}</Tag>{image.external_url ? <a href={image.external_url} target="_blank" rel="noreferrer">{text.externalImage}</a> : <span>{text.managedAsset} {image.asset_id}</span>}</Space>}
						/>
					</List.Item>
				)}
			/>
			<Form form={imageForm} layout="vertical" initialValues={{ sort_order: images.length + 1, primary: images.length === 0 }} onFinish={(input) => void addImage(input)}>
				<Form.Item label={text.source}>
					<Radio.Group optionType="button" value={imageSource} onChange={(event) => { setImageSource(event.target.value as "external" | "upload"); setUploadedImageAsset(undefined); }} options={[{ label: text.externalURL, value: "external" }, { label: text.uploadImage, value: "upload" }]} />
				</Form.Item>
				<div className="form-grid three-columns">
					{imageSource === "external" ? (
						<Form.Item name="external_url" label={text.externalHTTP} rules={[{ required: true }, { type: "url" }]}><Input /></Form.Item>
					) : (
						<Form.Item label={text.imageTypes} required>
							<Space><Upload accept=".jpg,.jpeg,.png,.webp,image/jpeg,image/png,image/webp" maxCount={1} showUploadList={false} beforeUpload={(file) => { void uploadProductImage(file); return false; }}><Button loading={imageUploading}>{text.chooseUpload}</Button></Upload>{uploadedImageAsset && <Typography.Text>{uploadedImageAsset.original_filename}</Typography.Text>}</Space>
						</Form.Item>
					)}
					<Form.Item name="alt_text" label={text.altText} extra={text.altHelp}><Input /></Form.Item>
					<Form.Item name="sort_order" label={text.sortOrder} rules={[{ required: true }]}><InputNumber min={0} precision={0} /></Form.Item>
					<Form.Item name="primary" valuePropName="checked"><Checkbox>{text.primaryImage}</Checkbox></Form.Item>
				</div>
				<Button type="primary" htmlType="submit">{text.addImage}</Button>
			</Form>
		</Card>

        <Card title={text.documents} size="small">
          <List
            dataSource={documents}
            locale={{ emptyText: text.noDocuments }}
            renderItem={(document) => (
              <List.Item>
                <List.Item.Meta
                  title={document.label}
                  description={
                    <Space wrap>
                      <Tag>{documentTypes.find((type) => type.id === document.document_type_id)?.name ?? document.document_type_id}</Tag>
                      {document.external_url && <a href={document.external_url} target="_blank" rel="noreferrer">{document.external_url}</a>}
							{document.asset_id && <a href={`/assets/${document.asset_id}`} target="_blank" rel="noreferrer">{text.localPDF}</a>}
                    </Space>
                  }
                />
              </List.Item>
            )}
          />
		  <Form form={documentForm} layout="vertical" initialValues={{ sort_order: documents.length + 1 }} onFinish={(input) => void addDocument(input)}>
			<Form.Item label={text.source}>
			  <Radio.Group
				optionType="button"
				value={documentSource}
				onChange={(event) => {
				  setDocumentSource(event.target.value as "external" | "upload");
				  setUploadedAsset(undefined);
				}}
				options={[{ label: text.externalURL, value: "external" }, { label: text.uploadPDF, value: "upload" }]}
			  />
			</Form.Item>
			<div className="form-grid three-columns">
			  <Form.Item name="label" label={text.label} rules={[{ required: true }]}><Input /></Form.Item>
			  <Form.Item name="document_type_id" label={text.documentType} rules={[{ required: true }]}><Select options={documentTypes.filter((type) => type.status === "active").map((type) => ({ value: type.id, label: type.name }))} /></Form.Item>
			  {documentSource === "external" ? (
				<Form.Item name="external_url" label={text.externalHTTP} rules={[{ required: true }, { type: "url" }]}><Input /></Form.Item>
			  ) : (
				<Form.Item label="PDF" required>
				  <Space>
					<Upload
					  accept=".pdf,application/pdf"
					  maxCount={1}
					  showUploadList={false}
					  beforeUpload={(file) => { void uploadPDF(file); return false; }}
					>
					  <Button loading={uploading}>{text.chooseUpload}</Button>
					</Upload>
					{uploadedAsset && <Typography.Text>{uploadedAsset.original_filename}</Typography.Text>}
				  </Space>
				</Form.Item>
			  )}
              <Form.Item name="language" label={text.language}><Input placeholder="en-US" /></Form.Item>
              <Form.Item name="sort_order" label={text.sortOrder} rules={[{ required: true }]}><InputNumber min={0} precision={0} /></Form.Item>
            </div>
            <Button type="primary" htmlType="submit">{text.addDocument}</Button>
          </Form>
        </Card>
      </Space>
    </Modal>
  );
}
