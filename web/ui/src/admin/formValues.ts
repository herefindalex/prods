export interface ReplaceableForm<T extends object> {
  resetFields(): void;
  setFieldsValue(values: T): void;
}

export function replaceFormValues<T extends object>(form: ReplaceableForm<T>, values: T): void {
  form.resetFields();
  form.setFieldsValue(values);
}
