export type InstallerStage = "claim" | "claimed" | "setup" | "complete";

export type InstallerPayloadValues = {
  token?: string;
  owner_email?: string;
  owner_display_name?: string;
  password?: string;
  password_confirm?: string;
  default_locale?: string;
  supported_locales?: string[];
  time_zone?: string;
  use_sample_data?: boolean;
};

export function installerRequestBody(stage: InstallerStage, values: InstallerPayloadValues, locale: string): URLSearchParams {
  if (stage === "claim") {
    return new URLSearchParams({ token: values.token ?? "", interface_locale: locale });
  }
  return new URLSearchParams({
    owner_email: values.owner_email ?? "",
    owner_display_name: values.owner_display_name ?? "",
    password: values.password ?? "",
    password_confirm: values.password_confirm ?? "",
    default_locale: values.default_locale ?? "",
    supported_locales: values.supported_locales?.join(",") ?? "",
    time_zone: values.time_zone ?? "",
    use_sample_data: String(values.use_sample_data ?? false),
    interface_locale: locale,
  });
}
