import type { AdminLocale } from "./locales";

export type BrandImportCopy = {
  title: string;
  help: string;
  sourceURL: string;
  generate: string;
  prompt: string;
  copyPrompt: string;
  copied: string;
  pasteResult: string;
  validate: string;
  status: string;
  limitations: string;
  diff: string;
  current: string;
  proposed: string;
  noChanges: string;
  rights: string;
  apply: string;
  applied: string;
  unavailable: string;
  preview: string;
  expires: string;
  exactSource: string;
};

const en: BrandImportCopy = {
  title: "Import Website brand with ChatGPT",
  help: "Generate a request-bound prompt, run it in an external browsing-capable ChatGPT, then paste the JSON result here. Prods validates the exact source site and saves an approved result only to Website Working Copy.",
  sourceURL: "Exact official Website URL",
  generate: "Generate prompt",
  prompt: "Prompt for external ChatGPT",
  copyPrompt: "Copy prompt",
  copied: "Prompt copied.",
  pasteResult: "Paste the JSON result",
  validate: "Validate result",
  status: "Result status",
  limitations: "Limitations",
  diff: "Changes to Website Working Copy",
  current: "Current working value",
  proposed: "Proposed value",
  noChanges: "The result makes no changes.",
  rights: "I confirm that we have the right to use this site's branding and content references.",
  apply: "Apply to Working Copy",
  applied: "Imported Website brand into Working Copy. The public site is unchanged until Preview and Publish.",
  unavailable: "The external model could not inspect the specified site. Generate a new prompt only after confirming the exact URL.",
  preview: "Open real Public Preview",
  expires: "Request expires",
  exactSource: "The model must inspect this exact URL. Cross-domain redirects are rejected.",
};

const zhTW: BrandImportCopy = {
  title: "透過 ChatGPT 匯入網站品牌",
  help: "產生綁定請求的 Prompt，在外部具瀏覽能力的 ChatGPT 執行，再將 JSON 結果貼回。Prods 會驗證指定來源，核准後只寫入 Website 工作副本。",
  sourceURL: "精確的官方網站網址",
  generate: "產生 Prompt",
  prompt: "交給外部 ChatGPT 的 Prompt",
  copyPrompt: "複製 Prompt",
  copied: "已複製 Prompt。",
  pasteResult: "貼上 JSON 結果",
  validate: "驗證結果",
  status: "結果狀態",
  limitations: "限制",
  diff: "Website 工作副本變更",
  current: "目前工作值",
  proposed: "建議值",
  noChanges: "這份結果沒有造成變更。",
  rights: "我確認我們有權使用此網站的品牌與內容參考。",
  apply: "套用到工作副本",
  applied: "已匯入 Website 品牌到工作副本。公開網站要等到預覽並發布後才會改變。",
  unavailable: "外部模型無法檢視指定網站。請先確認精確網址，再產生新 Prompt。",
  preview: "開啟真實公開預覽",
  expires: "請求到期時間",
  exactSource: "模型必須檢視這個精確網址；跨網域重新導向會被拒絕。",
};

const zhCN: BrandImportCopy = {
  ...zhTW,
  title: "通过 ChatGPT 导入网站品牌",
  help: "生成绑定请求的提示词，在外部具备浏览能力的 ChatGPT 中运行，再将 JSON 结果粘贴回来。Prods 验证指定来源，批准后只写入 Website 工作副本。",
  sourceURL: "准确的官方网站网址",
  generate: "生成提示词",
  copyPrompt: "复制提示词",
  copied: "已复制提示词。",
  pasteResult: "粘贴 JSON 结果",
  validate: "验证结果",
  current: "当前工作值",
  proposed: "建议值",
  rights: "我确认我们有权使用此网站的品牌和内容参考。",
  apply: "应用到工作副本",
  applied: "已将网站品牌导入工作副本。公开网站在预览并发布前不会改变。",
  unavailable: "外部模型无法检查指定网站。请确认准确网址后再生成新提示词。",
  preview: "打开真实公开预览",
  exactSource: "模型必须检查此准确网址；跨域重定向会被拒绝。",
};

const de: BrandImportCopy = {
  ...en,
  title: "Website-Marke mit ChatGPT importieren",
  sourceURL: "Exakte offizielle Website-URL",
  generate: "Prompt erzeugen",
  copyPrompt: "Prompt kopieren",
  copied: "Prompt kopiert.",
  pasteResult: "JSON-Ergebnis einfügen",
  validate: "Ergebnis prüfen",
  limitations: "Einschränkungen",
  current: "Aktueller Arbeitswert",
  proposed: "Vorgeschlagener Wert",
  rights: "Ich bestätige, dass wir die Marken- und Inhaltsreferenzen dieser Website verwenden dürfen.",
  apply: "Auf Arbeitskopie anwenden",
  preview: "Echte öffentliche Preview öffnen",
};

const fr: BrandImportCopy = {
  ...en,
  title: "Importer la marque du site avec ChatGPT",
  sourceURL: "URL exacte du site officiel",
  generate: "Générer le prompt",
  copyPrompt: "Copier le prompt",
  copied: "Prompt copié.",
  pasteResult: "Coller le résultat JSON",
  validate: "Valider le résultat",
  limitations: "Limites",
  current: "Valeur de travail actuelle",
  proposed: "Valeur proposée",
  rights: "Je confirme que nous avons le droit d'utiliser les références de marque et de contenu de ce site.",
  apply: "Appliquer à la copie de travail",
  preview: "Ouvrir la véritable Preview publique",
};

const es: BrandImportCopy = {
  ...en,
  title: "Importar la marca del sitio con ChatGPT",
  sourceURL: "URL exacta del sitio oficial",
  generate: "Generar prompt",
  copyPrompt: "Copiar prompt",
  copied: "Prompt copiado.",
  pasteResult: "Pegar resultado JSON",
  validate: "Validar resultado",
  limitations: "Limitaciones",
  current: "Valor de trabajo actual",
  proposed: "Valor propuesto",
  rights: "Confirmo que tenemos derecho a usar las referencias de marca y contenido de este sitio.",
  apply: "Aplicar a la copia de trabajo",
  preview: "Abrir la Preview pública real",
};

const variants: Record<AdminLocale, BrandImportCopy> = {
  "en-US": en,
  "zh-TW": zhTW,
  "zh-CN": zhCN,
  "ja-JP": { ...en, title: "ChatGPT で Web サイトブランドをインポート", sourceURL: "正確な公式 Web サイト URL", generate: "プロンプトを生成", copyPrompt: "プロンプトをコピー", copied: "プロンプトをコピーしました。", pasteResult: "JSON 結果を貼り付け", validate: "結果を検証", current: "現在の作業値", proposed: "提案値", rights: "このサイトのブランドとコンテンツ参照を使用する権利があることを確認します。", apply: "作業コピーに適用", preview: "実際の公開 Preview を開く" },
  "ko-KR": { ...en, title: "ChatGPT로 웹사이트 브랜드 가져오기", sourceURL: "정확한 공식 웹사이트 URL", generate: "프롬프트 생성", copyPrompt: "프롬프트 복사", copied: "프롬프트를 복사했습니다.", pasteResult: "JSON 결과 붙여넣기", validate: "결과 검증", current: "현재 작업 값", proposed: "제안 값", rights: "이 사이트의 브랜드 및 콘텐츠 참조를 사용할 권리가 있음을 확인합니다.", apply: "작업 사본에 적용", preview: "실제 공개 Preview 열기" },
  "de-DE": de,
  "fr-FR": fr,
  "it-IT": { ...en, title: "Importa il marchio del sito con ChatGPT", sourceURL: "URL esatto del sito ufficiale", generate: "Genera prompt", copyPrompt: "Copia prompt", copied: "Prompt copiato.", pasteResult: "Incolla il risultato JSON", validate: "Convalida risultato", current: "Valore di lavoro attuale", proposed: "Valore proposto", rights: "Confermo che abbiamo il diritto di usare i riferimenti al marchio e ai contenuti di questo sito.", apply: "Applica alla copia di lavoro", preview: "Apri la vera Preview pubblica" },
  "es-ES": es,
  "pt-BR": { ...en, title: "Importar a marca do site com ChatGPT", sourceURL: "URL exata do site oficial", generate: "Gerar prompt", copyPrompt: "Copiar prompt", copied: "Prompt copiado.", pasteResult: "Colar resultado JSON", validate: "Validar resultado", current: "Valor de trabalho atual", proposed: "Valor proposto", rights: "Confirmo que temos o direito de usar as referências de marca e conteúdo deste site.", apply: "Aplicar à cópia de trabalho", preview: "Abrir a Preview pública real" },
};

export function brandImportCopy(locale: AdminLocale): BrandImportCopy {
  return variants[locale];
}
