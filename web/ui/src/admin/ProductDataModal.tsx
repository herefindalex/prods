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
  product?: Product;
  open: boolean;
  onClose: () => void;
  onChanged: () => Promise<void>;
  onError: (error: unknown) => void;
  onMessage: (message: string) => void;
};

export function ProductDataModal({ product, open, onClose, onChanged, onError, onMessage }: Props) {
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
      onMessage(`Saved ${specs.find((spec) => spec.id === value.spec_id)?.name ?? value.spec_id} raw value.`);
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
			onMessage(`Uploaded ${asset.original_filename}. Add the document to publish its reference.`);
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
			onMessage(`Uploaded ${asset.original_filename}. Add it to the product image list to publish the reference.`);
		} catch (error) {
			onError(error);
		} finally {
			setImageUploading(false);
		}
	};

	const addImage = async (input: { external_url?: string; alt_text?: string; sort_order: number; primary?: boolean }) => {
		if (!current) return;
		if (imageSource === "upload" && !uploadedImageAsset) {
			onError(new Error("Upload an image before adding it."));
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
			onMessage("Product image added.");
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
			onMessage(primary ? "Primary image updated." : "Product image updated.");
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
			onMessage("Product image reference removed; the immutable asset remains available for retention/GC policy.");
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
			onError(new Error("Upload a PDF before adding this document."));
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
      onMessage(`Added ${input.label}.`);
      await load();
      await onChanged();
    } catch (error) {
      onError(error);
      await load();
    }
  };

  return (
    <Modal open={open} onCancel={onClose} footer={null} width={900} title={current ? `Specs and documents — ${current.part_number} · r${current.revision}` : "Specs and documents"}>
      <Space direction="vertical" size="large" className="panel-stack">
        {!specSet && <Alert type="warning" showIcon message="This product's category has no Spec Set assigned." />}
        <Card title="Specification values" size="small">
          <List
            dataSource={values}
            locale={{ emptyText: "No values saved" }}
            renderItem={(detail) => (
              <List.Item>
                <List.Item.Meta
                  title={specs.find((spec) => spec.id === detail.value.spec_id)?.name ?? detail.value.spec_id}
                  description={
                    <Space wrap>
                      <Typography.Text>{detail.value.raw_value}</Typography.Text>
                      <Tag color={detail.value.active ? "green" : "default"}>{detail.value.active ? "active" : "inactive"}</Tag>
                      <Tag>source r{detail.value.source_revision}</Tag>
                      {detail.normalized?.map((normalized) => <Tag key={normalized.id} color={normalized.status === "current" ? "blue" : "default"}>{normalized.source}:{normalized.status}</Tag>)}
                    </Space>
                  }
                />
              </List.Item>
            )}
          />
          <Form form={valueForm} layout="inline" onFinish={(input) => void saveValue(input)}>
            <Form.Item name="spec_id" rules={[{ required: true }]}><Select placeholder="Specification" style={{ width: 220 }} options={applicableSpecs.map((spec) => ({ value: spec.id, label: `${spec.name}${spec.preferred_unit ? ` (${spec.preferred_unit})` : ""}` }))} /></Form.Item>
            <Form.Item name="raw_value" rules={[{ required: true }]}><Input placeholder="Raw value" /></Form.Item>
            <Form.Item name="source_locale"><Input placeholder="Locale (optional)" /></Form.Item>
            <Button type="primary" htmlType="submit" disabled={!applicableSpecs.length}>Save value</Button>
          </Form>
        </Card>

		<Card title="Product images" size="small">
			<List
				dataSource={images}
				locale={{ emptyText: "No product images" }}
				renderItem={(image) => (
					<List.Item actions={[
						<Button key="primary" size="small" disabled={image.primary} onClick={() => void updateImage(image, true)}>Set primary</Button>,
						<Popconfirm key="delete" title="Remove this image reference?" onConfirm={() => void deleteImage(image)}><Button size="small" danger>Remove</Button></Popconfirm>,
					]}>
						<List.Item.Meta
							title={<Space>{image.primary && <Tag color="blue">Primary</Tag>}<span>{image.alt_text || "Alt falls back to product name"}</span></Space>}
							description={<Space wrap><Tag>order {image.sort_order}</Tag>{image.external_url ? <a href={image.external_url} target="_blank" rel="noreferrer">External image</a> : <span>Managed asset {image.asset_id}</span>}</Space>}
						/>
					</List.Item>
				)}
			/>
			<Form form={imageForm} layout="vertical" initialValues={{ sort_order: images.length + 1, primary: images.length === 0 }} onFinish={(input) => void addImage(input)}>
				<Form.Item label="Source">
					<Radio.Group optionType="button" value={imageSource} onChange={(event) => { setImageSource(event.target.value as "external" | "upload"); setUploadedImageAsset(undefined); }} options={[{ label: "External URL", value: "external" }, { label: "Upload image", value: "upload" }]} />
				</Form.Item>
				<div className="form-grid three-columns">
					{imageSource === "external" ? (
						<Form.Item name="external_url" label="External HTTP(S) URL" rules={[{ required: true }, { type: "url" }]}><Input /></Form.Item>
					) : (
						<Form.Item label="JPEG, PNG, or WebP" required>
							<Space><Upload accept=".jpg,.jpeg,.png,.webp,image/jpeg,image/png,image/webp" maxCount={1} showUploadList={false} beforeUpload={(file) => { void uploadProductImage(file); return false; }}><Button loading={imageUploading}>Choose and upload</Button></Upload>{uploadedImageAsset && <Typography.Text>{uploadedImageAsset.original_filename}</Typography.Text>}</Space>
						</Form.Item>
					)}
					<Form.Item name="alt_text" label="Alt text" extra="Optional; product name is the public fallback."><Input /></Form.Item>
					<Form.Item name="sort_order" label="Sort order" rules={[{ required: true }]}><InputNumber min={0} precision={0} /></Form.Item>
					<Form.Item name="primary" valuePropName="checked"><Checkbox>Primary image</Checkbox></Form.Item>
				</div>
				<Button type="primary" htmlType="submit">Add image</Button>
			</Form>
		</Card>

        <Card title="Documents" size="small">
          <List
            dataSource={documents}
            locale={{ emptyText: "No documents" }}
            renderItem={(document) => (
              <List.Item>
                <List.Item.Meta
                  title={document.label}
                  description={
                    <Space wrap>
                      <Tag>{documentTypes.find((type) => type.id === document.document_type_id)?.name ?? document.document_type_id}</Tag>
                      {document.external_url && <a href={document.external_url} target="_blank" rel="noreferrer">{document.external_url}</a>}
							{document.asset_id && <a href={`/assets/${document.asset_id}`} target="_blank" rel="noreferrer">Local PDF</a>}
                    </Space>
                  }
                />
              </List.Item>
            )}
          />
		  <Form form={documentForm} layout="vertical" initialValues={{ sort_order: documents.length + 1 }} onFinish={(input) => void addDocument(input)}>
			<Form.Item label="Source">
			  <Radio.Group
				optionType="button"
				value={documentSource}
				onChange={(event) => {
				  setDocumentSource(event.target.value as "external" | "upload");
				  setUploadedAsset(undefined);
				}}
				options={[{ label: "External URL", value: "external" }, { label: "Upload PDF", value: "upload" }]}
			  />
			</Form.Item>
			<div className="form-grid three-columns">
			  <Form.Item name="label" label="Label" rules={[{ required: true }]}><Input /></Form.Item>
			  <Form.Item name="document_type_id" label="Document type" rules={[{ required: true }]}><Select options={documentTypes.filter((type) => type.status === "active").map((type) => ({ value: type.id, label: type.name }))} /></Form.Item>
			  {documentSource === "external" ? (
				<Form.Item name="external_url" label="External HTTP(S) URL" rules={[{ required: true }, { type: "url" }]}><Input /></Form.Item>
			  ) : (
				<Form.Item label="PDF" required>
				  <Space>
					<Upload
					  accept=".pdf,application/pdf"
					  maxCount={1}
					  showUploadList={false}
					  beforeUpload={(file) => { void uploadPDF(file); return false; }}
					>
					  <Button loading={uploading}>Choose and upload</Button>
					</Upload>
					{uploadedAsset && <Typography.Text>{uploadedAsset.original_filename}</Typography.Text>}
				  </Space>
				</Form.Item>
			  )}
              <Form.Item name="language" label="Language"><Input placeholder="en-US" /></Form.Item>
              <Form.Item name="sort_order" label="Sort order" rules={[{ required: true }]}><InputNumber min={0} precision={0} /></Form.Item>
            </div>
            <Button type="primary" htmlType="submit">Add document</Button>
          </Form>
        </Card>
      </Space>
    </Modal>
  );
}
