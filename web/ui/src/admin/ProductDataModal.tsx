import { useEffect, useMemo, useState } from "react";
import { Alert, Button, Card, Checkbox, Form, Input, InputNumber, List, Modal, Popconfirm, Radio, Select, Space, Tag, Typography, Upload } from "antd";
import { api, APIError, clientID, postJSON, putJSON } from "./api";
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
 locale: "en-US" | "zh-TW";
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
