import { useCallback, useEffect, useRef, useState } from "react";
import { Alert, Button, Card, Form, Input, InputNumber, Space, Switch, Table, Tag, Typography } from "antd";
import { api, postJSON, putJSON } from "./api";
import type { AdminLocale } from "./locales";
import type { BackupRun, BackupSettings, BackupStatus } from "./types";

type Props = {
  locale: AdminLocale;
  onError(error: unknown): void;
  onMessage(message: string): void;
};

const labels = {
  "en-US": {
    title: "Backups",
    schedule: "Built-in daily schedule",
    enabled: "Enabled",
    localTime: "Local time",
    timeZone: "Schedule time zone",
    daily: "Daily copies",
    weekly: "Weekly copies",
    monthly: "Monthly copies",
    preUpgrade: "Pre-upgrade copies",
    preRestore: "Pre-restore copies",
    save: "Save schedule",
    run: "Run backup now",
    refresh: "Refresh",
    next: "Next scheduled run",
    overdue: "The current scheduled backup is due and waiting to run.",
    sensitive: "Backup restore points contain the database and may contain RFQ personal data, credentials, and local secrets. Store and copy them as sensitive data.",
    external: "Recorded but not included; restore requires these external runtime settings:",
    history: "Recent runs",
    kind: "Kind",
    status: "Status",
    started: "Started",
    completed: "Completed",
    size: "Size",
    evidence: "Completion evidence",
    retention: "Retention reasons",
    verified: "Content verified",
    protected: "Read-only applied",
    saved: "Backup settings saved.",
    completedMessage: "Backup completed.",
  },
  "zh-TW": {
    title: "備份",
    schedule: "內建每日排程",
    enabled: "啟用",
    localTime: "當地時間",
    timeZone: "排程時區",
    daily: "每日保留",
    weekly: "每週保留",
    monthly: "每月保留",
    preUpgrade: "升級前保留",
    preRestore: "還原前保留",
    save: "儲存排程",
    run: "立即執行備份",
    refresh: "重新整理",
    next: "下次排程",
    overdue: "目前排程備份已到期，正在等待執行。",
    sensitive: "備份還原點包含資料庫，且可能包含 RFQ 個資、登入憑證與本機 Secrets；保存或複製時必須視為敏感資料。",
    external: "以下外部 runtime 設定只會記錄、不會包含於備份；還原後仍須提供：",
    history: "最近執行紀錄",
    kind: "種類",
    status: "狀態",
    started: "開始時間",
    completed: "完成時間",
    size: "大小",
    evidence: "完成證據",
    retention: "保留理由",
    verified: "內容已驗證",
    protected: "已套用唯讀保護",
    saved: "備份設定已儲存。",
    completedMessage: "備份已完成。",
  },
"zh-CN": {
    title: "\u5907\u4EFD",
    schedule: "\u5185\u7F6E\u6BCF\u65E5\u65F6\u95F4\u8868",
    enabled: "\u542F\u7528",
    localTime: "\u5F53\u5730\u65F6\u95F4",
    timeZone: "\u5B89\u6392\u65F6\u533A",
    daily: "\u6BCF\u65E5\u4EFD\u6570",
    weekly: "\u6BCF\u5468\u4EFD\u6570",
    monthly: "\u6BCF\u6708\u4EFD\u6570",
    preUpgrade: "\u5347\u7EA7\u524D\u526F\u672C",
    preRestore: "\u9884\u6062\u590D\u526F\u672C",
    save: "\u4FDD\u5B58\u65E5\u7A0B",
    run: "\u7ACB\u5373\u8FD0\u884C\u5907\u4EFD",
    refresh: "\u5237\u65B0",
    next: "\u4E0B\u4E00\u6B21\u8BA1\u5212\u8FD0\u884C",
    overdue: "\u5F53\u524D\u8BA1\u5212\u7684\u5907\u4EFD\u5DF2\u5230\u671F\u5E76\u7B49\u5F85\u8FD0\u884C\u3002",
    sensitive: "\u5907\u4EFD\u8FD8\u539F\u70B9\u5305\u542B\u6570\u636E\u5E93\uFF0C\u5E76\u4E14\u53EF\u80FD\u5305\u542B RFQ \u4E2A\u4EBA\u6570\u636E\u3001\u51ED\u636E\u548C\u672C\u5730\u673A\u5BC6\u3002\u5C06\u5B83\u4EEC\u4F5C\u4E3A\u654F\u611F\u6570\u636E\u5B58\u50A8\u548C\u590D\u5236\u3002",
    external: "\u5DF2\u8BB0\u5F55\u4F46\u672A\u5305\u542B\u5728\u5185\uFF1B\u6062\u590D\u9700\u8981\u8FD9\u4E9B\u5916\u90E8\u8FD0\u884C\u65F6\u8BBE\u7F6E\uFF1A",
    history: "\u6700\u8FD1\u7684\u8DD1\u6B65",
    kind: "\u79CD\u7C7B",
    status: "\u5730\u4F4D",
    started: "\u5F00\u59CB",
    completed: "\u5B8C\u5168\u7684",
    size: "\u5C3A\u5BF8",
    evidence: "\u5B8C\u6210\u8BC1\u636E",
    retention: "\u4FDD\u7559\u539F\u56E0",
    verified: "\u5185\u5BB9\u5DF2\u9A8C\u8BC1",
    protected: "\u53EA\u8BFB\u5E94\u7528",
    saved: "\u5DF2\u4FDD\u5B58\u5907\u4EFD\u8BBE\u7F6E\u3002",
    completedMessage: "\u5907\u4EFD\u5B8C\u6210\u3002",
},
"ja-JP": {
    title: "\u30D0\u30C3\u30AF\u30A2\u30C3\u30D7",
    schedule: "\u5185\u8535\u306E\u6BCE\u65E5\u306E\u30B9\u30B1\u30B8\u30E5\u30FC\u30EB",
    enabled: "\u6709\u52B9",
    localTime: "\u73FE\u5730\u6642\u9593",
    timeZone: "\u30B9\u30B1\u30B8\u30E5\u30FC\u30EB\u306E\u30BF\u30A4\u30E0\u30BE\u30FC\u30F3",
    daily: "\u6BCE\u65E5\u306E\u30B3\u30D4\u30FC",
    weekly: "\u6BCE\u9031\u306E\u90E8\u6570",
    monthly: "\u6BCE\u6708\u306E\u90E8\u6570",
    preUpgrade: "\u30A2\u30C3\u30D7\u30B0\u30EC\u30FC\u30C9\u524D\u306E\u30B3\u30D4\u30FC",
    preRestore: "\u5FA9\u5143\u524D\u306E\u30B3\u30D4\u30FC",
    save: "\u30B9\u30B1\u30B8\u30E5\u30FC\u30EB\u306E\u4FDD\u5B58",
    run: "\u4ECA\u3059\u3050\u30D0\u30C3\u30AF\u30A2\u30C3\u30D7\u3092\u5B9F\u884C",
    refresh: "\u30EA\u30D5\u30EC\u30C3\u30B7\u30E5",
    next: "\u6B21\u306B\u30B9\u30B1\u30B8\u30E5\u30FC\u30EB\u3055\u308C\u305F\u5B9F\u884C",
    overdue: "\u73FE\u5728\u30B9\u30B1\u30B8\u30E5\u30FC\u30EB\u3055\u308C\u3066\u3044\u308B\u30D0\u30C3\u30AF\u30A2\u30C3\u30D7\u306E\u671F\u9650\u304C\u8FEB\u3063\u3066\u304A\u308A\u3001\u5B9F\u884C\u3092\u5F85\u6A5F\u3057\u3066\u3044\u307E\u3059\u3002",
    sensitive: "\u30D0\u30C3\u30AF\u30A2\u30C3\u30D7\u5FA9\u5143\u30DD\u30A4\u30F3\u30C8\u306B\u306F\u30C7\u30FC\u30BF\u30D9\u30FC\u30B9\u304C\u542B\u307E\u308C\u3066\u304A\u308A\u3001RFQ \u306E\u500B\u4EBA\u30C7\u30FC\u30BF\u3001\u8CC7\u683C\u60C5\u5831\u3001\u30ED\u30FC\u30AB\u30EB \u30B7\u30FC\u30AF\u30EC\u30C3\u30C8\u304C\u542B\u307E\u308C\u308B\u5834\u5408\u304C\u3042\u308A\u307E\u3059\u3002\u3053\u308C\u3089\u3092\u6A5F\u5BC6\u30C7\u30FC\u30BF\u3068\u3057\u3066\u4FDD\u5B58\u304A\u3088\u3073\u30B3\u30D4\u30FC\u3057\u307E\u3059\u3002",
    external: "\u8A18\u9332\u3055\u308C\u3066\u3044\u307E\u3059\u304C\u542B\u307E\u308C\u3066\u3044\u307E\u305B\u3093\u3002\u5FA9\u5143\u306B\u306F\u6B21\u306E\u5916\u90E8\u30E9\u30F3\u30BF\u30A4\u30E0\u8A2D\u5B9A\u304C\u5FC5\u8981\u3067\u3059\u3002",
    history: "\u6700\u8FD1\u306E\u30E9\u30F3\u30CB\u30F3\u30B0",
    kind: "\u89AA\u5207",
    status: "\u72B6\u614B",
    started: "\u958B\u59CB\u3057\u307E\u3057\u305F",
    completed: "\u5B8C\u4E86",
    size: "\u30B5\u30A4\u30BA",
    evidence: "\u5B8C\u4E86\u8A3C\u62E0",
    retention: "\u4FDD\u6301\u7406\u7531",
    verified: "\u5185\u5BB9\u78BA\u8A8D\u6E08\u307F",
    protected: "\u8AAD\u307F\u53D6\u308A\u5C02\u7528\u304C\u9069\u7528\u3055\u308C\u307E\u3059",
    saved: "\u30D0\u30C3\u30AF\u30A2\u30C3\u30D7\u8A2D\u5B9A\u304C\u4FDD\u5B58\u3055\u308C\u307E\u3057\u305F\u3002",
    completedMessage: "\u30D0\u30C3\u30AF\u30A2\u30C3\u30D7\u304C\u5B8C\u4E86\u3057\u307E\u3057\u305F\u3002",
},
"ko-KR": {
    title: "\uBC31\uC5C5",
    schedule: "\uB0B4\uC7A5\uB41C \uC77C\uC77C \uC77C\uC815",
    enabled: "\uD65C\uC131\uD654\uB428",
    localTime: "\uD604\uC9C0 \uC2DC\uAC04",
    timeZone: "\uC2DC\uAC04\uB300 \uC608\uC57D",
    daily: "\uC77C\uC77C \uC0AC\uBCF8",
    weekly: "\uC8FC\uAC04 \uC0AC\uBCF8",
    monthly: "\uC6D4\uBCC4 \uC0AC\uBCF8",
    preUpgrade: "\uC5C5\uADF8\uB808\uC774\uB4DC \uC804 \uC0AC\uBCF8",
    preRestore: "\uC0AC\uC804 \uBCF5\uC6D0 \uBCF5\uC0AC\uBCF8",
    save: "\uC77C\uC815 \uC800\uC7A5",
    run: "\uC9C0\uAE08 \uBC31\uC5C5 \uC2E4\uD589",
    refresh: "\uC0C8\uB85C \uACE0\uCE58\uB2E4",
    next: "\uB2E4\uC74C \uC608\uC57D \uC2E4\uD589",
    overdue: "\uD604\uC7AC \uC608\uC57D\uB41C \uBC31\uC5C5\uC774 \uC608\uC815\uB418\uC5B4 \uC2E4\uD589 \uB300\uAE30 \uC911\uC785\uB2C8\uB2E4.",
    sensitive: "\uBC31\uC5C5 \uBCF5\uC6D0 \uC9C0\uC810\uC5D0\uB294 \uB370\uC774\uD130\uBCA0\uC774\uC2A4\uAC00 \uD3EC\uD568\uB418\uC5B4 \uC788\uC73C\uBA70 RFQ \uAC1C\uC778 \uB370\uC774\uD130, \uC790\uACA9 \uC99D\uBA85 \uBC0F \uB85C\uCEEC \uBE44\uBC00\uC774 \uD3EC\uD568\uB420 \uC218 \uC788\uC2B5\uB2C8\uB2E4. \uBBFC\uAC10\uD55C \uB370\uC774\uD130\uB85C \uC800\uC7A5\uD558\uACE0 \uBCF5\uC0AC\uD558\uC138\uC694.",
    external: "\uAE30\uB85D\uB418\uC5C8\uC9C0\uB9CC \uD3EC\uD568\uB418\uC9C0 \uC54A\uC558\uC2B5\uB2C8\uB2E4. \uBCF5\uC6D0\uC5D0\uB294 \uB2E4\uC74C\uACFC \uAC19\uC740 \uC678\uBD80 \uB7F0\uD0C0\uC784 \uC124\uC815\uC774 \uD544\uC694\uD569\uB2C8\uB2E4.",
    history: "\uCD5C\uADFC \uC2E4\uD589",
    kind: "\uCE5C\uC808\uD55C",
    status: "\uC0C1\uD0DC",
    started: "\uC2DC\uC791\uB428",
    completed: "\uC644\uC804\uD55C",
    size: "\uD06C\uAE30",
    evidence: "\uC644\uB8CC \uC99D\uAC70",
    retention: "\uBCF4\uC874 \uC774\uC720",
    verified: "\uCF58\uD150\uCE20\uAC00 \uD655\uC778\uB418\uC5C8\uC2B5\uB2C8\uB2E4.",
    protected: "\uC77D\uAE30 \uC804\uC6A9 \uC801\uC6A9\uB428",
    saved: "\uBC31\uC5C5 \uC124\uC815\uC774 \uC800\uC7A5\uB418\uC5C8\uC2B5\uB2C8\uB2E4.",
    completedMessage: "\uBC31\uC5C5\uC774 \uC644\uB8CC\uB418\uC5C8\uC2B5\uB2C8\uB2E4.",
},
"de-DE": {
    title: "Backups",
    schedule: "Integrierter Tagesplan",
    enabled: "Erm\u00F6glicht",
    localTime: "Ortszeit",
    timeZone: "Zeitzone planen",
    daily: "T\u00E4gliche Exemplare",
    weekly: "W\u00F6chentliche Exemplare",
    monthly: "Monatliche Exemplare",
    preUpgrade: "Kopien vor dem Upgrade",
    preRestore: "Kopien vor der Wiederherstellung",
    save: "Zeitplan speichern",
    run: "F\u00FChren Sie jetzt ein Backup durch",
    refresh: "Aktualisieren",
    next: "N\u00E4chster geplanter Lauf",
    overdue: "Die aktuell geplante Sicherung ist f\u00E4llig und wartet auf ihre Ausf\u00FChrung.",
    sensitive: "Sicherungswiederherstellungspunkte enthalten die Datenbank und k\u00F6nnen pers\u00F6nliche Daten, Anmeldeinformationen und lokale Geheimnisse enthalten. Speichern und kopieren Sie sie als sensible Daten.",
    external: "Aufgenommen, aber nicht enthalten; F\u00FCr die Wiederherstellung sind diese externen Laufzeiteinstellungen erforderlich:",
    history: "Aktuelle L\u00E4ufe",
    kind: "Art",
    status: "Status",
    started: "Begonnen",
    completed: "Vollendet",
    size: "Gr\u00F6\u00DFe",
    evidence: "Abschlussnachweis",
    retention: "Aufbewahrungsgr\u00FCnde",
    verified: "Inhalt \u00FCberpr\u00FCft",
    protected: "Schreibgesch\u00FCtzt angewendet",
    saved: "Sicherungseinstellungen gespeichert.",
    completedMessage: "Sicherung abgeschlossen.",
},
"fr-FR": {
    title: "Sauvegardes",
    schedule: "Programme quotidien int\u00E9gr\u00E9",
    enabled: "Activ\u00E9",
    localTime: "Heure locale",
    timeZone: "Planifier le fuseau horaire",
    daily: "Copies quotidiennes",
    weekly: "Copies hebdomadaires",
    monthly: "Copies mensuelles",
    preUpgrade: "Copies pr\u00E9alables \u00E0 la mise \u00E0 niveau",
    preRestore: "Copies de pr\u00E9-restauration",
    save: "Enregistrer le planning",
    run: "Ex\u00E9cutez la sauvegarde maintenant",
    refresh: "Rafra\u00EEchir",
    next: "Prochaine ex\u00E9cution planifi\u00E9e",
    overdue: "La sauvegarde planifi\u00E9e actuelle est due et en attente d'ex\u00E9cution.",
    sensitive: "Les points de restauration de sauvegarde contiennent la base de donn\u00E9es et peuvent contenir des donn\u00E9es personnelles, des informations d'identification et des secrets locaux RFQ. Stockez-les et copiez-les en tant que donn\u00E9es sensibles.",
    external: "Enregistr\u00E9 mais non inclus\u00A0; la restauration n\u00E9cessite ces param\u00E8tres d'ex\u00E9cution externes\u00A0:",
    history: "Courses r\u00E9centes",
    kind: "Gentil",
    status: "Statut",
    started: "Commenc\u00E9",
    completed: "Compl\u00E9t\u00E9",
    size: "Taille",
    evidence: "Preuve d'ach\u00E8vement",
    retention: "Raisons de conservation",
    verified: "Contenu v\u00E9rifi\u00E9",
    protected: "Appliqu\u00E9 en lecture seule",
    saved: "Param\u00E8tres de sauvegarde enregistr\u00E9s.",
    completedMessage: "Sauvegarde termin\u00E9e.",
},
"it-IT": {
    title: "Backup",
    schedule: "Programma giornaliero integrato",
    enabled: "Abilitato",
    localTime: "Ora locale",
    timeZone: "Pianifica il fuso orario",
    daily: "Copie giornaliere",
    weekly: "Copie settimanali",
    monthly: "Copie mensili",
    preUpgrade: "Copie pre-aggiornamento",
    preRestore: "Copie pre-ripristino",
    save: "Salva programma",
    run: "Esegui il backup adesso",
    refresh: "Aggiorna",
    next: "Prossima corsa programmata",
    overdue: "Il backup pianificato corrente \u00E8 in scadenza ed \u00E8 in attesa di essere eseguito.",
    sensitive: "I punti di ripristino del backup contengono il database e possono contenere dati personali, credenziali e segreti locali RFQ. Archiviarli e copiarli come dati sensibili.",
    external: "Registrato ma non incluso; il ripristino richiede queste impostazioni di runtime esterne:",
    history: "Esecuzioni recenti",
    kind: "Tipo",
    status: "Stato",
    started: "Iniziato",
    completed: "Completato",
    size: "Misurare",
    evidence: "Prove di completamento",
    retention: "Motivi di conservazione",
    verified: "Contenuto verificato",
    protected: "Applicata di sola lettura",
    saved: "Impostazioni di backup salvate.",
    completedMessage: "Backup completato.",
},
"es-ES": {
    title: "Copias de seguridad",
    schedule: "Horario diario incorporado",
    enabled: "Activado",
    localTime: "Hora local",
    timeZone: "Horario zona horaria",
    daily: "Copias diarias",
    weekly: "Copias semanales",
    monthly: "Copias mensuales",
    preUpgrade: "Copias previas a la actualizaci\u00F3n",
    preRestore: "Copias previas a la restauraci\u00F3n",
    save: "Guardar horario",
    run: "Ejecute la copia de seguridad ahora",
    refresh: "Refrescar",
    next: "Pr\u00F3xima ejecuci\u00F3n programada",
    overdue: "La copia de seguridad programada actual est\u00E1 pendiente de ejecuci\u00F3n.",
    sensitive: "Los puntos de restauraci\u00F3n de la copia de seguridad contienen la base de datos y pueden contener datos personales, credenciales y secretos locales de RFQ. Gu\u00E1rdelos y c\u00F3pielos como datos confidenciales.",
    external: "Grabado pero no incluido; La restauraci\u00F3n requiere estas configuraciones de tiempo de ejecuci\u00F3n externas:",
    history: "Ejecuciones recientes",
    kind: "Amable",
    status: "Estado",
    started: "Comenz\u00F3",
    completed: "Terminado",
    size: "Tama\u00F1o",
    evidence: "Evidencia de finalizaci\u00F3n",
    retention: "Razones de retenci\u00F3n",
    verified: "Contenido verificado",
    protected: "Aplicado solo lectura",
    saved: "Configuraci\u00F3n de copia de seguridad guardada.",
    completedMessage: "Copia de seguridad completada.",
},
"pt-BR": {
    title: "C\u00F3pias de seguran\u00E7a",
    schedule: "Programa\u00E7\u00E3o di\u00E1ria integrada",
    enabled: "Habilitado",
    localTime: "Hora local",
    timeZone: "Agendar fuso hor\u00E1rio",
    daily: "C\u00F3pias di\u00E1rias",
    weekly: "C\u00F3pias semanais",
    monthly: "C\u00F3pias mensais",
    preUpgrade: "C\u00F3pias de pr\u00E9-atualiza\u00E7\u00E3o",
    preRestore: "C\u00F3pias pr\u00E9-restauradas",
    save: "Salvar programa\u00E7\u00E3o",
    run: "Execute o backup agora",
    refresh: "Atualizar",
    next: "Pr\u00F3xima execu\u00E7\u00E3o agendada",
    overdue: "O backup agendado atual est\u00E1 vencido e aguardando para ser executado.",
    sensitive: "Os pontos de restaura\u00E7\u00E3o de backup cont\u00EAm o banco de dados e podem conter dados pessoais RFQ, credenciais e segredos locais. Armazene e copie-os como dados confidenciais.",
    external: "Gravado, mas n\u00E3o inclu\u00EDdo; A restaura\u00E7\u00E3o requer estas configura\u00E7\u00F5es de tempo de execu\u00E7\u00E3o externo:",
    history: "Execu\u00E7\u00F5es recentes",
    kind: "Tipo",
    status: "Status",
    started: "Iniciado",
    completed: "Conclu\u00EDdo",
    size: "Tamanho",
    evidence: "Evid\u00EAncia de conclus\u00E3o",
    retention: "Raz\u00F5es de reten\u00E7\u00E3o",
    verified: "Conte\u00FAdo verificado",
    protected: "Somente leitura aplicado",
    saved: "Configura\u00E7\u00F5es de backup salvas.",
    completedMessage: "C\u00F3pia de seguran\u00E7a conclu\u00EDda.",
},
} as const;

function formatBytes(value?: number): string {
  if (!value) return "—";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let amount = value;
  let index = 0;
  while (amount >= 1024 && index < units.length - 1) {
    amount /= 1024;
    index += 1;
  }
  return `${amount.toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

export function BackupPanel({ locale, onError, onMessage }: Props) {
  const text = labels[locale];
  const [form] = Form.useForm<BackupSettings>();
  const [status, setStatus] = useState<BackupStatus>();
  const [loading, setLoading] = useState(false);
  const [running, setRunning] = useState(false);
  const onErrorRef = useRef(onError);
  const onMessageRef = useRef(onMessage);
  onErrorRef.current = onError;
  onMessageRef.current = onMessage;

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const next = await api<BackupStatus>("/admin/api/backups");
      setStatus(next);
      form.setFieldsValue(next.settings);
    } catch (error) {
      onErrorRef.current(error);
    } finally {
      setLoading(false);
    }
  }, [form]);

  useEffect(() => {
    void load();
  }, [load]);

  const save = async (values: BackupSettings) => {
    if (!status) return;
    setLoading(true);
    try {
      await putJSON<BackupSettings>("/admin/api/backups/settings", {
        expected_version: status.settings.version,
        settings: values,
      });
      onMessageRef.current(text.saved);
      await load();
    } catch (error) {
      onErrorRef.current(error);
    } finally {
      setLoading(false);
    }
  };

  const runNow = async () => {
    setRunning(true);
    try {
      await postJSON<BackupRun>("/admin/api/backups/run", {});
      onMessageRef.current(text.completedMessage);
      await load();
    } catch (error) {
      onErrorRef.current(error);
    } finally {
      setRunning(false);
    }
  };

  const columns = [
    { title: text.kind, dataIndex: "kind", key: "kind" },
    {
      title: text.status,
      dataIndex: "status",
      key: "status",
      render: (value: BackupRun["status"]) => (
        <Tag color={value === "succeeded" ? "green" : value === "failed" ? "red" : "blue"}>{value}</Tag>
      ),
    },
    {
      title: text.started,
      dataIndex: "started_at",
      key: "started",
      render: (value: string) => new Date(value).toLocaleString(locale),
    },
    {
      title: text.completed,
      dataIndex: "completed_at",
      key: "completed",
      render: (value?: string) => (value ? new Date(value).toLocaleString(locale) : "—"),
    },
    { title: text.size, dataIndex: "size_bytes", key: "size", render: formatBytes },
    {
      title: text.evidence,
      key: "evidence",
      render: (_: unknown, run: BackupRun) => (
        <Space direction="vertical" size={0}>
          <Typography.Text type={run.content_verified ? "success" : undefined}>
            {run.content_verified ? text.verified : "—"}
          </Typography.Text>
          <Typography.Text type={run.read_only_applied ? "success" : "secondary"}>
            {run.read_only_applied ? text.protected : "—"}
          </Typography.Text>
          {run.error_message && <Typography.Text type="danger">{run.error_message}</Typography.Text>}
          {run.warning_message && <Typography.Text type="warning">{run.warning_message}</Typography.Text>}
        </Space>
      ),
    },
    {
      title: text.retention,
      key: "retention",
      render: (_: unknown, run: BackupRun) =>
        run.backup_id ? (status?.retention.reasons[run.backup_id] ?? []).join(", ") || "—" : "—",
    },
  ];

  return (
    <Space direction="vertical" size="middle" className="panel-stack">
      {status?.due && <Alert type="warning" showIcon message={text.overdue} />}
		{status?.contains_sensitive_data && <Alert type="warning" showIcon message={text.sensitive} />}
		{Boolean(status?.external_requirements?.length) && (
			<Alert type="info" showIcon message={text.external} description={status?.external_requirements?.join(", ")} />
		)}
      <Card
        title={text.title}
        extra={
          <Space>
            <Button onClick={() => void load()} loading={loading}>{text.refresh}</Button>
            <Button type="primary" onClick={() => void runNow()} loading={running}>{text.run}</Button>
          </Space>
        }
      >
        {status?.next_run_utc && (
          <Typography.Paragraph>
            {text.next}: {new Date(status.next_run_utc).toLocaleString(locale)}
          </Typography.Paragraph>
        )}
        <Form form={form} layout="vertical" onFinish={(values) => void save(values)}>
          <Card type="inner" title={text.schedule}>
            <Space wrap align="start">
              <Form.Item name="enabled" label={text.enabled} valuePropName="checked">
                <Switch />
              </Form.Item>
              <Form.Item name="local_time" label={text.localTime} rules={[{ required: true }]}>
                <Input type="time" />
              </Form.Item>
              <Form.Item label={text.timeZone}>
                <Input value={status?.settings.time_zone} readOnly />
              </Form.Item>
              {([
                ["retention_daily", text.daily],
                ["retention_weekly", text.weekly],
                ["retention_monthly", text.monthly],
                ["retention_pre_upgrade", text.preUpgrade],
                ["retention_pre_restore", text.preRestore],
              ] as const).map(([name, label]) => (
                <Form.Item key={name} name={name} label={label} rules={[{ required: true }]}>
                  <InputNumber min={1} max={3660} precision={0} />
                </Form.Item>
              ))}
            </Space>
            <div><Button htmlType="submit" loading={loading}>{text.save}</Button></div>
          </Card>
        </Form>
      </Card>
      <Card title={text.history}>
        <Table<BackupRun>
          rowKey="id"
          loading={loading}
          dataSource={status?.runs ?? []}
          columns={columns}
          pagination={false}
          scroll={{ x: true }}
        />
      </Card>
    </Space>
  );
}
