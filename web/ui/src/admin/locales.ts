import deDE from "antd/locale/de_DE";
import enUS from "antd/locale/en_US";
import esES from "antd/locale/es_ES";
import frFR from "antd/locale/fr_FR";
import itIT from "antd/locale/it_IT";
import jaJP from "antd/locale/ja_JP";
import koKR from "antd/locale/ko_KR";
import ptBR from "antd/locale/pt_BR";
import zhCN from "antd/locale/zh_CN";
import zhTW from "antd/locale/zh_TW";

export const ADMIN_LOCALE_OPTIONS = [
  { value: "en-US", label: "English" },
  { value: "zh-TW", label: "繁中" },
  { value: "zh-CN", label: "简中" },
  { value: "ja-JP", label: "日本語" },
  { value: "ko-KR", label: "한국어" },
  { value: "de-DE", label: "Deutsch" },
  { value: "fr-FR", label: "Français" },
  { value: "it-IT", label: "Italiano" },
  { value: "es-ES", label: "Español" },
  { value: "pt-BR", label: "Português (Brasil)" },
] as const;

export type AdminLocale = (typeof ADMIN_LOCALE_OPTIONS)[number]["value"];
export type ImplementedAdminMessageLocale = "en-US" | "zh-TW";

const adminLocaleCodes = new Set<string>(ADMIN_LOCALE_OPTIONS.map(({ value }) => value));

export function isAdminLocale(value: string | null): value is AdminLocale {
  return value !== null && adminLocaleCodes.has(value);
}

export function normalizeAdminLocale(value: string): AdminLocale {
  const normalized = value.trim().toLowerCase();
  const exact = ADMIN_LOCALE_OPTIONS.find(({ value: candidate }) => normalized === candidate.toLowerCase());
  if (exact) return exact.value;
  const language = normalized.split("-")[0];
  return ADMIN_LOCALE_OPTIONS.find(({ value: candidate }) => candidate.toLowerCase().startsWith(`${language}-`))?.value ?? "en-US";
}

export function implementedAdminMessageLocale(locale: AdminLocale): ImplementedAdminMessageLocale {
  return locale === "zh-TW" ? "zh-TW" : "en-US";
}

export function antDesignLocale(locale: AdminLocale) {
  switch (locale) {
    case "zh-TW": return zhTW;
    case "zh-CN": return zhCN;
    case "ja-JP": return jaJP;
    case "ko-KR": return koKR;
    case "de-DE": return deDE;
    case "fr-FR": return frFR;
    case "it-IT": return itIT;
    case "es-ES": return esES;
    case "pt-BR": return ptBR;
    default: return enUS;
  }
}
