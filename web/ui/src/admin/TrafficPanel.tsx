import { useCallback, useEffect, useRef, useState } from "react";
import { Alert, Button, Card, Form, InputNumber, Space, Typography } from "antd";
import { api, putJSON } from "./api";
import type { AdminLocale } from "./locales";
import type { TrafficSettings } from "./types";

type Props = {
  locale: AdminLocale;
  onError(error: unknown): void;
  onMessage(message: string): void;
};

const labels = {
  "en-US": {
    title: "Public traffic protection",
    explanation: "The limit applies only to new RFQ submissions from the same direct connection source. Public catalog reads are not limited. A retry of an already completed submission key still replays its durable receipt.",
    proxy: "Forwarded client headers are not trusted by this setting; until a trusted proxy is explicitly configured at the host boundary, requests behind a proxy share that proxy's direct source limit.",
    proxyConfigured: "Forwarded client addresses are accepted only when the transport peer matches a host-configured trusted proxy. Multi-hop chains are evaluated from the trusted edge inward.",
    limit: "New RFQs per window",
    window: "Window (seconds)",
    save: "Save protection settings",
    saved: "Traffic protection settings saved.",
  },
  "zh-TW": {
    title: "公開流量保護",
    explanation: "限制只套用於同一直接連線來源的新 RFQ 提交，不限制公開型錄讀取。已完成 submission key 的重試仍會重播 durable receipt。",
    proxy: "此設定不信任轉送的客戶端標頭；主機邊界尚未明確設定 trusted proxy 前，代理後方的請求會共用該代理的直接來源額度。",
    proxyConfigured: "只有傳輸端符合主機設定的 trusted proxy 時才接受轉送客戶端位址；多跳鏈會從可信任邊界向內判定。",
    limit: "每個窗口的新 RFQ 數",
    window: "窗口秒數",
    save: "儲存保護設定",
    saved: "流量保護設定已儲存。",
  },
"zh-CN": {
    title: "公共流量保护",
    explanation: "\u8BE5\u9650\u5236\u4EC5\u9002\u7528\u4E8E\u6765\u81EA\u540C\u4E00\u76F4\u63A5\u8FDE\u63A5\u6E90\u7684\u65B0 RFQ \u63D0\u4EA4\u3002\u516C\u5171\u76EE\u5F55\u8BFB\u53D6\u4E0D\u53D7\u9650\u5236\u3002\u91CD\u8BD5\u5DF2\u5B8C\u6210\u7684\u63D0\u4EA4\u5BC6\u94A5\u4ECD\u4F1A\u91CD\u64AD\u5176\u6301\u4E45\u6536\u636E\u3002",
    proxy: "\u6B64\u8BBE\u7F6E\u4E0D\u4FE1\u4EFB\u8F6C\u53D1\u7684\u5BA2\u6237\u7AEF\u6807\u5934\uFF1B\u9664\u975E\u5728\u4E3B\u673A\u8FB9\u754C\u663E\u5F0F\u914D\u7F6E\u53D7\u4FE1\u4EFB\u4EE3\u7406\uFF0C\u5426\u5219\u4EE3\u7406\u540E\u9762\u7684\u8BF7\u6C42\u5C06\u5171\u4EAB\u8BE5\u4EE3\u7406\u7684\u76F4\u63A5\u6E90\u9650\u5236\u3002",
    proxyConfigured: "\u4EC5\u5F53\u4F20\u8F93\u5BF9\u7B49\u65B9\u4E0E\u4E3B\u673A\u914D\u7F6E\u7684\u53EF\u4FE1\u4EE3\u7406\u5339\u914D\u65F6\uFF0C\u624D\u4F1A\u63A5\u53D7\u8F6C\u53D1\u7684\u5BA2\u6237\u7AEF\u5730\u5740\u3002\u591A\u8DF3\u94FE\u662F\u4ECE\u53EF\u4FE1\u8FB9\u7F18\u5411\u5185\u8BC4\u4F30\u7684\u3002",
    limit: "\u6BCF\u4E2A\u7A97\u53E3\u7684\u65B0\u8BE2\u4EF7",
    window: "\u7A97\u53E3\uFF08\u79D2\uFF09",
    save: "\u4FDD\u5B58\u4FDD\u62A4\u8BBE\u7F6E",
    saved: "\u5DF2\u4FDD\u5B58\u6D41\u91CF\u4FDD\u62A4\u8BBE\u7F6E\u3002",
},
"ja-JP": {
    title: "公開トラフィック保護",
    explanation: "\u3053\u306E\u5236\u9650\u306F\u3001\u540C\u3058\u76F4\u63A5\u63A5\u7D9A\u30BD\u30FC\u30B9\u304B\u3089\u306E\u65B0\u3057\u3044 RFQ \u9001\u4FE1\u306B\u306E\u307F\u9069\u7528\u3055\u308C\u307E\u3059\u3002\u30D1\u30D6\u30EA\u30C3\u30AF \u30AB\u30BF\u30ED\u30B0\u306E\u8AAD\u307F\u53D6\u308A\u306B\u306F\u5236\u9650\u304C\u3042\u308A\u307E\u305B\u3093\u3002\u3059\u3067\u306B\u5B8C\u4E86\u3057\u305F\u9001\u4FE1\u30AD\u30FC\u3092\u518D\u8A66\u884C\u3057\u3066\u3082\u3001\u6C38\u7D9A\u7684\u306A\u53D7\u4FE1\u304C\u518D\u751F\u3055\u308C\u307E\u3059\u3002",
    proxy: "\u8EE2\u9001\u3055\u308C\u305F\u30AF\u30E9\u30A4\u30A2\u30F3\u30C8 \u30D8\u30C3\u30C0\u30FC\u306F\u3001\u3053\u306E\u8A2D\u5B9A\u3067\u306F\u4FE1\u983C\u3055\u308C\u307E\u305B\u3093\u3002\u4FE1\u983C\u3055\u308C\u305F\u30D7\u30ED\u30AD\u30B7\u304C\u30DB\u30B9\u30C8\u5883\u754C\u3067\u660E\u793A\u7684\u306B\u8A2D\u5B9A\u3055\u308C\u308B\u307E\u3067\u3001\u30D7\u30ED\u30AD\u30B7\u306E\u80CC\u5F8C\u306B\u3042\u308B\u30EA\u30AF\u30A8\u30B9\u30C8\u306F\u3001\u305D\u306E\u30D7\u30ED\u30AD\u30B7\u306E\u76F4\u63A5\u30BD\u30FC\u30B9\u5236\u9650\u3092\u5171\u6709\u3057\u307E\u3059\u3002",
    proxyConfigured: "\u8EE2\u9001\u3055\u308C\u305F\u30AF\u30E9\u30A4\u30A2\u30F3\u30C8 \u30A2\u30C9\u30EC\u30B9\u306F\u3001\u30C8\u30E9\u30F3\u30B9\u30DD\u30FC\u30C8 \u30D4\u30A2\u304C\u30DB\u30B9\u30C8\u3067\u69CB\u6210\u3055\u308C\u305F\u4FE1\u983C\u3067\u304D\u308B\u30D7\u30ED\u30AD\u30B7\u3068\u4E00\u81F4\u3059\u308B\u5834\u5408\u306B\u306E\u307F\u53D7\u3051\u5165\u308C\u3089\u308C\u307E\u3059\u3002\u30DE\u30EB\u30C1\u30DB\u30C3\u30D7 \u30C1\u30A7\u30FC\u30F3\u306F\u3001\u4FE1\u983C\u3067\u304D\u308B\u30A8\u30C3\u30B8\u304B\u3089\u5185\u5074\u306B\u5411\u200B\u200B\u304B\u3063\u3066\u8A55\u4FA1\u3055\u308C\u307E\u3059\u3002",
    limit: "\u30A6\u30A3\u30F3\u30C9\u30A6\u3054\u3068\u306E\u65B0\u3057\u3044 RFQ",
    window: "\u30A6\u30A3\u30F3\u30C9\u30A6 (\u79D2)",
    save: "\u4FDD\u5B58\u4FDD\u8B77\u8A2D\u5B9A",
    saved: "\u30C8\u30E9\u30D5\u30A3\u30C3\u30AF\u4FDD\u8B77\u8A2D\u5B9A\u304C\u4FDD\u5B58\u3055\u308C\u307E\u3057\u305F\u3002",
},
"ko-KR": {
    title: "공개 트래픽 보호",
    explanation: "\uC81C\uD55C\uC740 \uB3D9\uC77C\uD55C \uC9C1\uC811 \uC5F0\uACB0 \uC18C\uC2A4\uC758 \uC0C8\uB85C\uC6B4 RFQ \uC81C\uCD9C\uC5D0\uB9CC \uC801\uC6A9\uB429\uB2C8\uB2E4. \uACF5\uAC1C \uCE74\uD0C8\uB85C\uADF8 \uC77D\uAE30\uB294 \uC81C\uD55C\uB418\uC9C0 \uC54A\uC2B5\uB2C8\uB2E4. \uC774\uBBF8 \uC644\uB8CC\uB41C \uC81C\uCD9C \uD0A4\uB97C \uB2E4\uC2DC \uC2DC\uB3C4\uD558\uBA74 \uC9C0\uC18D \uAC00\uB2A5\uD55C \uC218\uC2E0\uC774 \uACC4\uC18D \uC7AC\uC0DD\uB429\uB2C8\uB2E4.",
    proxy: "\uC804\uB2EC\uB41C \uD074\uB77C\uC774\uC5B8\uD2B8 \uD5E4\uB354\uB294 \uC774 \uC124\uC815\uC73C\uB85C \uC2E0\uB8B0\uB418\uC9C0 \uC54A\uC2B5\uB2C8\uB2E4. \uC2E0\uB8B0\uD560 \uC218 \uC788\uB294 \uD504\uB85D\uC2DC\uAC00 \uD638\uC2A4\uD2B8 \uACBD\uACC4\uC5D0 \uBA85\uC2DC\uC801\uC73C\uB85C \uAD6C\uC131\uB420 \uB54C\uAE4C\uC9C0 \uD504\uB85D\uC2DC \uB4A4\uC758 \uC694\uCCAD\uC740 \uD574\uB2F9 \uD504\uB85D\uC2DC\uC758 \uC9C1\uC811 \uC18C\uC2A4 \uC81C\uD55C\uC744 \uACF5\uC720\uD569\uB2C8\uB2E4.",
    proxyConfigured: "\uC804\uB2EC\uB41C \uD074\uB77C\uC774\uC5B8\uD2B8 \uC8FC\uC18C\uB294 \uC804\uC1A1 \uD53C\uC5B4\uAC00 \uD638\uC2A4\uD2B8\uC5D0\uC11C \uAD6C\uC131\uD55C \uC2E0\uB8B0\uD560 \uC218 \uC788\uB294 \uD504\uB85D\uC2DC\uC640 \uC77C\uCE58\uD558\uB294 \uACBD\uC6B0\uC5D0\uB9CC \uD5C8\uC6A9\uB429\uB2C8\uB2E4. \uB2E4\uC911 \uD649 \uCCB4\uC778\uC740 \uC2E0\uB8B0\uD560 \uC218 \uC788\uB294 \uAC00\uC7A5\uC790\uB9AC\uC5D0\uC11C \uC548\uCABD\uC73C\uB85C \uD3C9\uAC00\uB429\uB2C8\uB2E4.",
    limit: "\uCC3D\uB2F9 \uC0C8 RFQ",
    window: "\uCC3D(\uCD08)",
    save: "\uBCF4\uD638 \uC124\uC815 \uC800\uC7A5",
    saved: "\uAD50\uD1B5 \uBCF4\uD638 \uC124\uC815\uC774 \uC800\uC7A5\uB418\uC5C8\uC2B5\uB2C8\uB2E4.",
},
"de-DE": {
    title: "Schutz des öffentlichen Datenverkehrs",
    explanation: "Das Limit gilt nur f\u00FCr neue RFQ-\u00DCbermittlungen von derselben direkten Verbindungsquelle. Das Lesen \u00F6ffentlicher Kataloge ist nicht beschr\u00E4nkt. Bei einem erneuten Versuch eines bereits abgeschlossenen \u00DCbermittlungsschl\u00FCssels wird dessen dauerhafter Empfang noch einmal abgespielt.",
    proxy: "Weitergeleitete Client-Header werden von dieser Einstellung nicht als vertrauensw\u00FCrdig eingestuft. Bis ein vertrauensw\u00FCrdiger Proxy explizit an der Hostgrenze konfiguriert wird, teilen sich Anforderungen hinter einem Proxy die direkte Quellbeschr\u00E4nkung dieses Proxys.",
    proxyConfigured: "Weitergeleitete Clientadressen werden nur akzeptiert, wenn der Transport-Peer mit einem vom Host konfigurierten vertrauensw\u00FCrdigen Proxy \u00FCbereinstimmt. Multi-Hop-Ketten werden von der vertrauensw\u00FCrdigen Kante nach innen ausgewertet.",
    limit: "Neue RFQs pro Fenster",
    window: "Fenster (Sekunden)",
    save: "Schutzeinstellungen speichern",
    saved: "Verkehrsschutzeinstellungen gespeichert.",
},
"fr-FR": {
    title: "Protection du trafic public",
    explanation: "La limite s'applique uniquement aux nouvelles soumissions RFQ provenant de la m\u00EAme source de connexion directe. Les lectures du catalogue public ne sont pas limit\u00E9es. Une nouvelle tentative d'une cl\u00E9 de soumission d\u00E9j\u00E0 compl\u00E9t\u00E9e rejoue toujours sa r\u00E9ception durable.",
    proxy: "Les en-t\u00EAtes client transf\u00E9r\u00E9s ne sont pas approuv\u00E9s par ce param\u00E8tre ; jusqu'\u00E0 ce qu'un proxy de confiance soit explicitement configur\u00E9 \u00E0 la limite de l'h\u00F4te, les requ\u00EAtes derri\u00E8re un proxy partagent la limite de source directe de ce proxy.",
    proxyConfigured: "Les adresses client transf\u00E9r\u00E9es sont accept\u00E9es uniquement lorsque l'homologue de transport correspond \u00E0 un proxy de confiance configur\u00E9 par l'h\u00F4te. Les cha\u00EEnes multi-sauts sont \u00E9valu\u00E9es depuis le bord de confiance vers l'int\u00E9rieur.",
    limit: "Nouveaux appels d'offres par fenêtre",
    window: "Fenêtre (secondes)",
    save: "Enregistrer les param\u00E8tres de protection",
    saved: "Param\u00E8tres de protection du trafic enregistr\u00E9s.",
},
"it-IT": {
    title: "Protezione del traffico pubblico",
    explanation: "Il limite si applica solo ai nuovi invii RFQ dalla stessa fonte di connessione diretta. Le letture del catalogo pubblico non sono limitate. Un nuovo tentativo di una chiave di invio gi\u00E0 completata riproduce comunque la sua ricevuta durevole.",
    proxy: "Le intestazioni client inoltrate non sono considerate attendibili da questa impostazione; finch\u00E9 un proxy attendibile non viene configurato esplicitamente al limite dell'host, le richieste dietro un proxy condividono il limite di origine diretta del proxy.",
    proxyConfigured: "Gli indirizzi client inoltrati vengono accettati solo quando il peer di trasporto corrisponde a un proxy attendibile configurato dall'host. Le catene multi-hop vengono valutate dal perimetro attendibile verso l'interno.",
    limit: "Nuove richieste di offerta per finestra",
    window: "Finestra (secondi)",
    save: "Salva le impostazioni di protezione",
    saved: "Impostazioni di protezione del traffico salvate.",
},
"es-ES": {
    title: "Protecci\u00F3n del tr\u00E1fico p\u00FAblico",
    explanation: "El l\u00EDmite se aplica solo a nuevos env\u00EDos de RFQ desde la misma fuente de conexi\u00F3n directa. Las lecturas del cat\u00E1logo p\u00FAblico no est\u00E1n limitadas. Un reintento de una clave de env\u00EDo ya completada a\u00FAn reproduce su recibo duradero.",
    proxy: "Esta configuraci\u00F3n no conf\u00EDa en los encabezados de cliente reenviados; Hasta que un proxy confiable se configure expl\u00EDcitamente en el l\u00EDmite del host, las solicitudes detr\u00E1s de un proxy comparten el l\u00EDmite de origen directo de ese proxy.",
    proxyConfigured: "Las direcciones de cliente reenviadas se aceptan solo cuando el par de transporte coincide con un proxy confiable configurado por el host. Las cadenas de m\u00FAltiples saltos se eval\u00FAan desde el borde de confianza hacia adentro.",
    limit: "Nuevas RFQ por ventana",
    window: "Ventana (segundos)",
    save: "Guardar configuraci\u00F3n de protecci\u00F3n",
    saved: "Configuraci\u00F3n de protecci\u00F3n de tr\u00E1fico guardada.",
},
"pt-BR": {
    title: "Prote\u00E7\u00E3o de tr\u00E1fego p\u00FAblico",
    explanation: "O limite se aplica apenas a novos envios RFQ da mesma fonte de conex\u00E3o direta. As leituras do cat\u00E1logo p\u00FAblico n\u00E3o s\u00E3o limitadas. Uma nova tentativa de uma chave de envio j\u00E1 conclu\u00EDda ainda reproduz seu recebimento dur\u00E1vel.",
    proxy: "Os cabe\u00E7alhos de cliente encaminhados n\u00E3o s\u00E3o confi\u00E1veis \u200B\u200Bpara esta configura\u00E7\u00E3o; at\u00E9 que um proxy confi\u00E1vel seja explicitamente configurado no limite do host, as solicita\u00E7\u00F5es atr\u00E1s de um proxy compartilham o limite de origem direta desse proxy.",
    proxyConfigured: "Endere\u00E7os de clientes encaminhados s\u00E3o aceitos somente quando o peer de transporte corresponde a um proxy confi\u00E1vel configurado pelo host. As cadeias multi-hop s\u00E3o avaliadas da borda confi\u00E1vel para dentro.",
    limit: "Novas RFQs por janela",
    window: "Janela (segundos)",
    save: "Salvar configura\u00E7\u00F5es de prote\u00E7\u00E3o",
    saved: "Configura\u00E7\u00F5es de prote\u00E7\u00E3o de tr\u00E1fego salvas.",
},
} as const;

export function TrafficPanel({ locale, onError, onMessage }: Props) {
  const text = labels[locale];
  const [form] = Form.useForm<TrafficSettings>();
  const [settings, setSettings] = useState<TrafficSettings>();
  const [loading, setLoading] = useState(false);
  const onErrorRef = useRef(onError);
  onErrorRef.current = onError;

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const next = await api<TrafficSettings>("/admin/api/traffic-settings");
      setSettings(next);
      form.setFieldsValue(next);
    } catch (error) {
      onErrorRef.current(error);
    } finally {
      setLoading(false);
    }
  }, [form]);

  useEffect(() => {
    void load();
  }, [load]);

  const save = async (values: TrafficSettings) => {
    if (!settings) return;
    setLoading(true);
    try {
      const next = await putJSON<TrafficSettings>("/admin/api/traffic-settings", {
        expected_version: settings.version,
        settings: values,
      });
      setSettings(next);
      form.setFieldsValue(next);
      onMessage(text.saved);
    } catch (error) {
      onError(error);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Card title={text.title} loading={!settings && loading}>
      <Space direction="vertical" size="middle" className="panel-stack">
        <Typography.Paragraph>{text.explanation}</Typography.Paragraph>
        <Alert type="info" showIcon message={settings?.trusted_proxy_configured ? text.proxyConfigured : text.proxy} />
        <Form form={form} layout="vertical" onFinish={(values) => void save(values)}>
          <Space wrap align="start">
            <Form.Item name="rfq_limit" label={text.limit} rules={[{ required: true }]}>
              <InputNumber min={1} max={10000} precision={0} />
            </Form.Item>
            <Form.Item name="rfq_window_seconds" label={text.window} rules={[{ required: true }]}>
              <InputNumber min={1} max={86400} precision={0} />
            </Form.Item>
          </Space>
          <div><Button type="primary" htmlType="submit" loading={loading}>{text.save}</Button></div>
        </Form>
      </Space>
    </Card>
  );
}
