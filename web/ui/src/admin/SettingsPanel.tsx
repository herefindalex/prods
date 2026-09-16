import { useCallback, useEffect, useRef, useState } from "react";
import {
  Alert,
  Button,
  Card,
  Form,
  Input,
  Space,
  Typography,
} from "antd";
import { api, putJSON } from "./api";
import { localeLabel, type AdminLocale } from "./locales";
import type { SiteSettings } from "./types";

type Props = {
  locale: AdminLocale;
  onError(error: unknown): void;
  onMessage(message: string): void;
};
const labels = {
  "en-US": {
    title: "Site settings",
    refresh: "Refresh",
    revision: "Revision",
    defaultLocale: "Default interface locale",
    supportedLocales: "Supported locales",
    timeZone: "Site time zone",
    timeZoneHelp:
      "Use an IANA time zone such as America/New_York, Asia/Taipei, or UTC.",
    invalidTimeZone: "Enter a valid IANA time zone.",
    save: "Save site time zone",
    saved:
      "Site time zone saved. Stored event timestamps remain unchanged in UTC.",
    explanation:
      "The site time zone controls Admin/RFQ display and backup schedules. It is independent from interface language.",
    contentTitle: "Multilingual content",
    contentEnabled: "Enable multilingual content editing",
    contentHelp:
      "Language tabs appear only while enabled. Removing a locale preserves saved translations but revokes their public representations.",
    saveContent: "Save content languages",
    contentSaved: "Content language settings saved.",
  },
  "zh-TW": {
    title: "站點設定",
    refresh: "重新整理",
    revision: "修訂",
    defaultLocale: "預設介面語系",
    supportedLocales: "支援語系",
    timeZone: "站點時區",
    timeZoneHelp:
      "請使用 IANA 時區，例如 America/New_York、Asia/Taipei 或 UTC。",
    invalidTimeZone: "請輸入有效的 IANA 時區。",
    save: "儲存站點時區",
    saved: "站點時區已儲存；既有事件時間仍以 UTC 原值保存。",
    explanation: "站點時區用於 Admin／RFQ 顯示及備份排程；它與介面語言分離。",
    contentTitle: "多語內容",
    contentEnabled: "啟用多語內容編輯",
    contentHelp:
      "只有啟用後才顯示語言頁籤。移除語系會保留既有翻譯，但撤銷該語系公開表示。",
    saveContent: "儲存內容語系",
    contentSaved: "內容語系設定已儲存。",
  },
"zh-CN": {
    title: "\u7AD9\u70B9\u8BBE\u7F6E",
    refresh: "\u5237\u65B0",
    revision: "\u4FEE\u8BA2\u7248",
    defaultLocale: "\u9ED8\u8BA4\u754C\u9762\u533A\u57DF\u8BBE\u7F6E",
    supportedLocales: "\u652F\u6301\u7684\u533A\u57DF\u8BBE\u7F6E",
    timeZone: "\u7AD9\u70B9\u65F6\u533A",
    timeZoneHelp: "\u4F7F\u7528 IANA \u65F6\u533A\uFF0C\u4F8B\u5982 America/New_York\u3001Asia/Taipei \u6216 UTC\u3002",
    invalidTimeZone: "\u8F93\u5165\u6709\u6548\u7684 IANA \u65F6\u533A\u3002",
    save: "\u4FDD\u5B58\u7AD9\u70B9\u65F6\u533A",
    saved: "\u5DF2\u4FDD\u5B58\u7AD9\u70B9\u65F6\u533A\u3002\u5B58\u50A8\u7684\u4E8B\u4EF6\u65F6\u95F4\u6233\u5728 UTC \u4E2D\u4FDD\u6301\u4E0D\u53D8\u3002",
    explanation: "\u7AD9\u70B9\u65F6\u533A\u63A7\u5236Admin/RFQ \u663E\u793A\u548C\u5907\u4EFD\u8BA1\u5212\u3002\u5B83\u72EC\u7ACB\u4E8E\u754C\u9762\u8BED\u8A00\u3002",
    contentTitle: "\u591A\u8BED\u8A00\u5185\u5BB9",
    contentEnabled: "\u542F\u7528\u591A\u8BED\u8A00\u5185\u5BB9\u7F16\u8F91",
    contentHelp: "\u8BED\u8A00\u9009\u9879\u5361\u4EC5\u5728\u542F\u7528\u65F6\u51FA\u73B0\u3002\u5220\u9664\u8BED\u8A00\u73AF\u5883\u4F1A\u4FDD\u7559\u5DF2\u4FDD\u5B58\u7684\u7FFB\u8BD1\uFF0C\u4F46\u4F1A\u64A4\u9500\u5176\u516C\u5F00\u8868\u793A\u3002",
    saveContent: "\u4FDD\u5B58\u5185\u5BB9\u8BED\u8A00",
    contentSaved: "\u5DF2\u4FDD\u5B58\u5185\u5BB9\u8BED\u8A00\u8BBE\u7F6E\u3002",
},
"ja-JP": {
    title: "\u30B5\u30A4\u30C8\u8A2D\u5B9A",
    refresh: "\u66F4\u65B0",
    revision: "\u30EA\u30D3\u30B8\u30E7\u30F3",
    defaultLocale: "\u30C7\u30D5\u30A9\u30EB\u30C8\u306E\u30A4\u30F3\u30BF\u30FC\u30D5\u30A7\u30A4\u30B9 \u30ED\u30B1\u30FC\u30EB",
    supportedLocales: "\u30B5\u30DD\u30FC\u30C8\u3055\u308C\u3066\u3044\u308B\u30ED\u30B1\u30FC\u30EB",
    timeZone: "\u30B5\u30A4\u30C8\u306E\u30BF\u30A4\u30E0\u30BE\u30FC\u30F3",
    timeZoneHelp: "\u30A2\u30E1\u30EA\u30AB/\u30CB\u30E5\u30FC\u30E8\u30FC\u30AF\u3001\u30A2\u30B8\u30A2/\u53F0\u5317\u3001UTC \u306A\u3069\u306E IANA \u30BF\u30A4\u30E0 \u30BE\u30FC\u30F3\u3092\u4F7F\u7528\u3057\u307E\u3059\u3002",
    invalidTimeZone: "\u6709\u52B9\u306A IANA \u30BF\u30A4\u30E0\u30BE\u30FC\u30F3\u3092\u5165\u529B\u3057\u307E\u3059\u3002",
    save: "\u30B5\u30A4\u30C8\u306E\u30BF\u30A4\u30E0\u30BE\u30FC\u30F3\u3092\u4FDD\u5B58",
    saved: "\u30B5\u30A4\u30C8\u306E\u30BF\u30A4\u30E0\u30BE\u30FC\u30F3\u304C\u4FDD\u5B58\u3055\u308C\u307E\u3057\u305F\u3002\u4FDD\u5B58\u3055\u308C\u305F\u30A4\u30D9\u30F3\u30C8\u306E\u30BF\u30A4\u30E0\u30B9\u30BF\u30F3\u30D7\u306F UTC \u3067\u5909\u66F4\u3055\u308C\u307E\u305B\u3093\u3002",
    explanation: "\u30B5\u30A4\u30C8\u306E\u30BF\u30A4\u30E0\u30BE\u30FC\u30F3\u306F\u3001Admin/RFQ \u306E\u8868\u793A\u3068\u30D0\u30C3\u30AF\u30A2\u30C3\u30D7\u306E\u30B9\u30B1\u30B8\u30E5\u30FC\u30EB\u3092\u5236\u5FA1\u3057\u307E\u3059\u3002\u30A4\u30F3\u30BF\u30FC\u30D5\u30A7\u30FC\u30B9\u8A00\u8A9E\u304B\u3089\u72EC\u7ACB\u3057\u3066\u3044\u307E\u3059\u3002",
    contentTitle: "\u591A\u8A00\u8A9E\u30B3\u30F3\u30C6\u30F3\u30C4",
    contentEnabled: "\u591A\u8A00\u8A9E\u30B3\u30F3\u30C6\u30F3\u30C4\u7DE8\u96C6\u3092\u6709\u52B9\u306B\u3059\u308B",
    contentHelp: "\u8A00\u8A9E\u30BF\u30D6\u306F\u3001\u6709\u52B9\u306B\u306A\u3063\u3066\u3044\u308B\u5834\u5408\u306B\u306E\u307F\u8868\u793A\u3055\u308C\u307E\u3059\u3002\u30ED\u30B1\u30FC\u30EB\u3092\u524A\u9664\u3059\u308B\u3068\u3001\u4FDD\u5B58\u3055\u308C\u305F\u7FFB\u8A33\u306F\u4FDD\u6301\u3055\u308C\u307E\u3059\u304C\u3001\u305D\u306E\u516C\u958B\u8868\u73FE\u306F\u53D6\u308A\u6D88\u3055\u308C\u307E\u3059\u3002",
    saveContent: "\u30B3\u30F3\u30C6\u30F3\u30C4\u306E\u8A00\u8A9E\u3092\u4FDD\u5B58\u3059\u308B",
    contentSaved: "\u30B3\u30F3\u30C6\u30F3\u30C4\u306E\u8A00\u8A9E\u8A2D\u5B9A\u304C\u4FDD\u5B58\u3055\u308C\u307E\u3057\u305F\u3002",
},
"ko-KR": {
    title: "\uC0AC\uC774\uD2B8 \uC124\uC815",
    refresh: "\uC0C8\uB85C \uACE0\uCE68",
    revision: "\uAC1C\uC815",
    defaultLocale: "\uAE30\uBCF8 \uC778\uD130\uD398\uC774\uC2A4 \uB85C\uCF00\uC77C",
    supportedLocales: "\uC9C0\uC6D0\uB418\uB294 \uB85C\uCF00\uC77C",
    timeZone: "\uC0AC\uC774\uD2B8 \uC2DC\uAC04\uB300",
    timeZoneHelp: "America/New_York, Asia/Taipei \uB610\uB294 UTC\uC640 \uAC19\uC740 IANA \uC2DC\uAC04\uB300\uB97C \uC0AC\uC6A9\uD569\uB2C8\uB2E4.",
    invalidTimeZone: "\uC720\uD6A8\uD55C IANA \uC2DC\uAC04\uB300\uB97C \uC785\uB825\uD558\uC138\uC694.",
    save: "\uC0AC\uC774\uD2B8 \uC2DC\uAC04\uB300 \uC800\uC7A5",
    saved: "\uC0AC\uC774\uD2B8 \uC2DC\uAC04\uB300\uAC00 \uC800\uC7A5\uB418\uC5C8\uC2B5\uB2C8\uB2E4. \uC800\uC7A5\uB41C \uC774\uBCA4\uD2B8 \uD0C0\uC784\uC2A4\uD0EC\uD504\uB294 UTC \uAE30\uC900\uC73C\uB85C \uBCC0\uACBD\uB418\uC9C0 \uC54A\uC2B5\uB2C8\uB2E4.",
    explanation: "\uC0AC\uC774\uD2B8 \uC2DC\uAC04\uB300\uB294 Admin/RFQ \uD45C\uC2DC \uBC0F \uBC31\uC5C5 \uC77C\uC815\uC744 \uC81C\uC5B4\uD569\uB2C8\uB2E4. \uC778\uD130\uD398\uC774\uC2A4 \uC5B8\uC5B4\uC640\uB294 \uB3C5\uB9BD\uC801\uC785\uB2C8\uB2E4.",
    contentTitle: "\uB2E4\uAD6D\uC5B4 \uCF58\uD150\uCE20",
    contentEnabled: "\uB2E4\uAD6D\uC5B4 \uCF58\uD150\uCE20 \uD3B8\uC9D1 \uD65C\uC131\uD654",
    contentHelp: "\uC5B8\uC5B4 \uD0ED\uC740 \uD65C\uC131\uD654\uB41C \uB3D9\uC548\uC5D0\uB9CC \uB098\uD0C0\uB0A9\uB2C8\uB2E4. \uB85C\uCE98\uC744 \uC81C\uAC70\uD558\uBA74 \uC800\uC7A5\uB41C \uBC88\uC5ED\uC740 \uC720\uC9C0\uB418\uC9C0\uB9CC \uACF5\uAC1C \uD45C\uD604\uC740 \uCDE8\uC18C\uB429\uB2C8\uB2E4.",
    saveContent: "\uCF58\uD150\uCE20 \uC5B8\uC5B4 \uC800\uC7A5",
    contentSaved: "\uCF58\uD150\uCE20 \uC5B8\uC5B4 \uC124\uC815\uC774 \uC800\uC7A5\uB418\uC5C8\uC2B5\uB2C8\uB2E4.",
},
"de-DE": {
    title: "Site-Einstellungen",
    refresh: "Aktualisieren",
    revision: "-Revision",
    defaultLocale: "Standard-Schnittstellengebietsschema",
    supportedLocales: "Unterst\u00FCtzte Gebietsschemas",
    timeZone: "Zeitzone der Website",
    timeZoneHelp: "Verwenden Sie eine IANA-Zeitzone wie America/New_York, Asia/Taipei oder UTC.",
    invalidTimeZone: "Geben Sie eine g\u00FCltige IANA-Zeitzone ein.",
    save: "Speichern Sie die Zeitzone der Website",
    saved: "Site-Zeitzone gespeichert. Gespeicherte Ereigniszeitstempel bleiben in UTC unver\u00E4ndert.",
    explanation: "Die Site-Zeitzone steuert die Anzeige- und Sicherungszeitpl\u00E4ne von Admin/RFQ. Es ist unabh\u00E4ngig von der Sprache der Benutzeroberfl\u00E4che.",
    contentTitle: "Mehrsprachiger Inhalt",
    contentEnabled: "Erm\u00F6glichen Sie die mehrsprachige Inhaltsbearbeitung",
    contentHelp: "Sprachregisterkarten werden nur angezeigt, wenn sie aktiviert sind. Durch das Entfernen eines Gebietsschemas bleiben gespeicherte \u00DCbersetzungen erhalten, ihre \u00F6ffentlichen Darstellungen werden jedoch widerrufen.",
    saveContent: "Inhaltssprachen speichern",
    contentSaved: "Inhaltsspracheinstellungen gespeichert.",
},
"fr-FR": {
    title: "Param\u00E8tres du site",
    refresh: "Actualiser",
    revision: "Révision",
    defaultLocale: "Param\u00E8tres r\u00E9gionaux de l'interface par d\u00E9faut",
    supportedLocales: "Param\u00E8tres r\u00E9gionaux pris en charge",
    timeZone: "Fuseau horaire du site",
    timeZoneHelp: "Utilisez un fuseau horaire IANA tel que America/New_York, Asia/Taipei ou UTC.",
    invalidTimeZone: "Entrez un fuseau horaire IANA valide.",
    save: "Enregistrer le fuseau horaire du site",
    saved: "Fuseau horaire du site enregistr\u00E9. Les horodatages des \u00E9v\u00E9nements stock\u00E9s restent inchang\u00E9s en UTC.",
    explanation: "Le fuseau horaire du site contr\u00F4le les planifications d'affichage et de sauvegarde des Admin/RFQ. Il est ind\u00E9pendant du langage de l'interface.",
    contentTitle: "Contenu multilingue",
    contentEnabled: "Activer l’édition de contenu multilingue",
    contentHelp: "Les onglets de langue apparaissent uniquement lorsque cette option est activée. La suppression d’une langue conserve les traductions enregistrées, mais révoque leurs représentations publiques.",
    saveContent: "Enregistrer les langues du contenu",
    contentSaved: "Param\u00E8tres de langue du contenu enregistr\u00E9s.",
},
"it-IT": {
    title: "Impostazioni del sito",
    refresh: "Aggiorna",
    revision: "Revisione",
    defaultLocale: "Impostazioni locali predefinite dell'interfaccia",
    supportedLocales: "Localit\u00E0 supportate",
    timeZone: "Fuso orario del sito",
    timeZoneHelp: "Utilizza un fuso orario IANA come America/New_York, Asia/Taipei o UTC.",
    invalidTimeZone: "Inserisci un fuso orario IANA valido.",
    save: "Salva il fuso orario del sito",
    saved: "Fuso orario del sito salvato. I timestamp degli eventi memorizzati rimangono invariati in UTC.",
    explanation: "Il fuso orario del sito controlla la visualizzazione Admin/RFQ e le pianificazioni di backup. \u00C8 indipendente dalla lingua dell'interfaccia.",
    contentTitle: "Contenuti multilingue",
    contentEnabled: "Abilita la modifica dei contenuti multilingue",
    contentHelp: "Le schede della lingua vengono visualizzate solo quando sono abilitate. La rimozione di una lingua preserva le traduzioni salvate ma revoca le loro rappresentazioni pubbliche.",
    saveContent: "Salva le lingue dei contenuti",
    contentSaved: "Impostazioni della lingua del contenuto salvate.",
},
"es-ES": {
    title: "Configuraci\u00F3n del sitio",
    refresh: "Actualizar",
    revision: "Revisi\u00F3n",
    defaultLocale: "Configuraci\u00F3n regional de la interfaz predeterminada",
    supportedLocales: "Configuraci\u00F3n regional compatible",
    timeZone: "Zona horaria del sitio",
    timeZoneHelp: "Utilice una zona horaria de IANA como Am\u00E9rica/Nueva_York, Asia/Taipei o UTC.",
    invalidTimeZone: "Ingrese una zona horaria IANA v\u00E1lida.",
    save: "Guardar zona horaria del sitio",
    saved: "Zona horaria del sitio guardada. Las marcas de tiempo de los eventos almacenados permanecen sin cambios en UTC.",
    explanation: "La zona horaria del sitio controla los horarios de visualizaci\u00F3n y respaldo de Admin/RFQ. Es independiente del idioma de la interfaz.",
    contentTitle: "Contenido multiling\u00FCe",
    contentEnabled: "Habilitar la edici\u00F3n de contenido multiling\u00FCe",
    contentHelp: "Las pesta\u00F1as de idioma aparecen solo cuando est\u00E1n habilitadas. Eliminar una configuraci\u00F3n regional conserva las traducciones guardadas pero revoca sus representaciones p\u00FAblicas.",
    saveContent: "Guardar idiomas de contenido",
    contentSaved: "Se guard\u00F3 la configuraci\u00F3n de idioma del contenido.",
},
"pt-BR": {
 title: "Configurações do site",
 refresh: "Atualizar",
 revision: "Revisão",
    defaultLocale: "Local da interface padr\u00E3o",
    supportedLocales: "Localidades suportadas",
    timeZone: "Fuso hor\u00E1rio do site",
    timeZoneHelp: "Use um fuso hor\u00E1rio IANA, como Am\u00E9rica/Nova_Iorque, \u00C1sia/Taipei ou UTC.",
    invalidTimeZone: "Insira um fuso hor\u00E1rio IANA v\u00E1lido.",
    save: "Salvar fuso hor\u00E1rio do site",
    saved: "Fuso hor\u00E1rio do site salvo. Os carimbos de data/hora dos eventos armazenados permanecem inalterados em UTC.",
    explanation: "O fuso hor\u00E1rio do site controla a exibi\u00E7\u00E3o Admin/RFQ e os agendamentos de backup. \u00C9 independente do idioma da interface.",
    contentTitle: "Conte\u00FAdo multil\u00EDngue",
    contentEnabled: "Habilitar edi\u00E7\u00E3o de conte\u00FAdo multil\u00EDngue",
    contentHelp: "As guias de idioma aparecem apenas quando ativadas. A remo\u00E7\u00E3o de uma localidade preserva as tradu\u00E7\u00F5es salvas, mas revoga suas representa\u00E7\u00F5es p\u00FAblicas.",
    saveContent: "Salvar idiomas de conte\u00FAdo",
    contentSaved: "Configura\u00E7\u00F5es de idioma do conte\u00FAdo salvas.",
},
} as const;

function validTimeZone(value: string) {
  if (value === "Local") return true;
  try {
    new Intl.DateTimeFormat("en-US", { timeZone: value }).format();
    return true;
  } catch {
    return false;
  }
}

export function SettingsPanel({ locale, onError, onMessage }: Props) {
  const text = labels[locale];
  const [settings, setSettings] = useState<SiteSettings>();
  const [loading, setLoading] = useState(false);
  const [timeForm] = Form.useForm<{ time_zone: string }>();
  const onErrorRef = useRef(onError);
  const onMessageRef = useRef(onMessage);
  onErrorRef.current = onError;
  onMessageRef.current = onMessage;
  const load = useCallback(async () => {
    setLoading(true);
    try {
      const next = await api<SiteSettings>("/admin/api/system/settings");
      setSettings(next);
      timeForm.setFieldsValue({ time_zone: next.time_zone });
    } catch (error) {
      onErrorRef.current(error);
    } finally {
      setLoading(false);
    }
  }, [timeForm]);
  useEffect(() => {
    void load();
  }, [load]);
  const saveTime = async (values: { time_zone: string }) => {
    if (!settings) return;
    setLoading(true);
    try {
      const updated = await putJSON<SiteSettings>(
        "/admin/api/system/settings",
        {
          expected_revision: settings.revision,
          time_zone: values.time_zone.trim(),
        },
      );
      setSettings(updated);
      onMessageRef.current(text.saved);
    } catch (error) {
      onErrorRef.current(error);
    } finally {
      setLoading(false);
    }
  };
  return (
    <Space direction="vertical" size="large" className="panel-stack">
      <Card
        title={text.title}
        extra={
          <Button onClick={() => void load()} loading={loading}>
            {text.refresh}
          </Button>
        }
      >
        <Space direction="vertical" size="large" className="panel-stack">
          <Alert type="info" showIcon message={text.explanation} />
          <Form
            form={timeForm}
            layout="vertical"
            onFinish={(v) => void saveTime(v)}
          >
            <div className="form-grid three-columns">
              <Form.Item label={text.defaultLocale}>
                <Input value={settings ? localeLabel(settings.default_locale) : ""} readOnly />
              </Form.Item>
              <Form.Item label={text.supportedLocales}>
                <Input
                  value={settings?.supported_locales.map(localeLabel).join(", ")}
                  readOnly
                />
              </Form.Item>
              <Form.Item
                name="time_zone"
                label={text.timeZone}
                extra={text.timeZoneHelp}
                rules={[
                  { required: true },
                  {
                    validator: async (_, v: string) => {
                      if (v && !validTimeZone(v.trim()))
                        throw new Error(text.invalidTimeZone);
                    },
                  },
                ]}
              >
                <Input placeholder="UTC" />
              </Form.Item>
            </div>
            <Button type="primary" htmlType="submit" loading={loading}>
              {text.save}
            </Button>
          </Form>
          {settings && (
            <Typography.Text type="secondary">
              {text.revision} {settings.revision} ·{" "}
              {new Date(settings.updated_at).toLocaleString(locale)}
            </Typography.Text>
          )}
        </Space>
      </Card>
    </Space>
  );
}
