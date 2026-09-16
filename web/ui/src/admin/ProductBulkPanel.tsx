import { useEffect, useState, type Key } from "react";
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Form,
  Popconfirm,
  Select,
  Space,
  Table,
  Tag,
  Typography,
} from "antd";
import { api, postJSON } from "./api";
import type { AdminLocale } from "./locales";
import type {
  Category,
  DictionaryEntry,
  Product,
  ProductBulkAction,
  ProductBulkPlanItem,
  ProductBulkReceipt,
  ProductBulkRun,
} from "./types";

type Props = {
  locale: AdminLocale;
  onError: (error: unknown) => void;
  onMessage: (message: string) => void;
};

const labels = {
  "en-US": {
    title: "Product Bulk Operations",
    help: "Preflight freezes the selection and each Product revision. Execution rechecks and commits each Product atomically; other successful Products remain when one conflicts.",
    selection: "Selected Products",
    action: "Operation",
    target: "Target",
    preflight: "Preflight fixed selection",
    execute: "Execute reviewed plan",
    confirm: "Execute this fixed preflight plan? Each Product will be revalidated.",
    partNumber: "Part number",
    state: "State",
    revision: "Revision",
    eligible: "Eligible",
    result: "Result",
    message: "Message",
    yes: "Yes",
    no: "No",
    selectAtLeastOne: "Select at least one Product.",
    noUnresolved: "There are no unresolved Products to retry.",
    previewSummary: (eligible: number, total: number) => `${eligible} of ${total} Products are eligible in this fixed preflight.`,
    done: (receipt: ProductBulkReceipt) =>
      `Bulk completed: ${receipt.succeeded} succeeded, ${receipt.no_change} unchanged, ${receipt.conflicts} conflicts, ${receipt.invalid} invalid, ${receipt.failed} failed.`,
    retryUnresolved: "Preflight unresolved only",
    actions: {
      publish: "Publish",
      hide: "Hide",
      archive: "Archive",
      change_category: "Change category",
      change_lifecycle: "Change lifecycle",
    } as Record<ProductBulkAction, string>,
  },
  "zh-TW": {
    title: "Product 批次操作",
    help: "預檢會固定選取範圍與每個 Product 修訂。執行時逐項重驗並以單一 Product 原子提交；某項衝突不會回滾其他成功項目。",
    selection: "已選 Product",
    action: "操作",
    target: "目標",
    preflight: "固定選取並預檢",
    execute: "執行已檢視計畫",
    confirm: "執行這份固定預檢計畫？每個 Product 都會重新驗證。",
    partNumber: "料號",
    state: "狀態",
    revision: "修訂",
    eligible: "可執行",
    result: "結果",
    message: "訊息",
    yes: "是",
    no: "否",
    selectAtLeastOne: "請至少選取一個 Product。",
    noUnresolved: "沒有需要重試的未解決 Product。",
    previewSummary: (eligible: number, total: number) => `固定預檢中 ${total} 個 Product 有 ${eligible} 個可執行。`,
    done: (receipt: ProductBulkReceipt) =>
      `批次完成：成功 ${receipt.succeeded}、無需變更 ${receipt.no_change}、衝突 ${receipt.conflicts}、無效 ${receipt.invalid}、失敗 ${receipt.failed}。`,
    retryUnresolved: "只重新預檢未解決項目",
    actions: {
      publish: "發布",
      hide: "隱藏",
      archive: "封存",
      change_category: "變更分類",
      change_lifecycle: "變更生命週期",
    } as Record<ProductBulkAction, string>,
  },
"zh-CN": {
    title: "Product \u6279\u91CF\u64CD\u4F5C",
    help: "\u9884\u68C0\u4F1A\u51BB\u7ED3\u9009\u62E9\u548C\u6BCF\u4E2A Product \u4FEE\u8BA2\u3002\u6267\u884C\u4EE5\u539F\u5B50\u65B9\u5F0F\u91CD\u65B0\u68C0\u67E5\u5E76\u63D0\u4EA4\u6BCF\u4E2A Product\uFF1B\u5F53\u4E00\u79CD\u4EA7\u54C1\u53D1\u751F\u51B2\u7A81\u65F6\uFF0C\u5176\u4ED6\u6210\u529F\u7684\u4EA7\u54C1\u4ECD\u7136\u5B58\u5728\u3002",
    selection: "\u7CBE\u9009\u4EA7\u54C1",
    action: "\u624B\u672F",
    target: "\u76EE\u6807",
    preflight: "\u9884\u68C0\u56FA\u5B9A\u9009\u62E9",
    execute: "\u6267\u884C\u7ECF\u5BA1\u67E5\u7684\u8BA1\u5212",
    confirm: "\u6267\u884C\u8FD9\u4E2A\u56FA\u5B9A\u7684\u98DE\u884C\u524D\u8BA1\u5212\u5417\uFF1F\u6BCF\u4E2A Product \u5C06\u88AB\u91CD\u65B0\u9A8C\u8BC1\u3002",
    partNumber: "\u96F6\u4EF6\u7F16\u53F7",
    state: "\u72B6\u6001",
    revision: "\u4FEE\u8BA2",
    eligible: "\u6709\u8D44\u683C\u7684",
    result: "\u7ED3\u679C",
    message: "\u4FE1\u606F",
    yes: "\u662F\u7684",
    no: "\u4E0D",
    selectAtLeastOne: "\u81F3\u5C11\u9009\u62E9\u4E00\u4E2A Product\u3002",
    noUnresolved: "\u6CA1\u6709\u672A\u89E3\u51B3\u7684\u4EA7\u54C1\u53EF\u4F9B\u91CD\u8BD5\u3002",
    previewSummary: (eligible: number, total: number) => `${eligible}\u7684${total}\u4EA7\u54C1\u7B26\u5408\u6B64\u56FA\u5B9A\u9884\u68C0\u8D44\u683C\u200B\u200B\u3002`,
    done: (receipt: ProductBulkReceipt) => `\u6279\u91CF\u5B8C\u6210\uFF1A${receipt.succeeded}\u6210\u529F\u4E86\uFF0C${receipt.no_change}\u4E0D\u53D8\uFF0C${receipt.conflicts}\u51B2\u7A81\uFF0C${receipt.invalid}\u65E0\u6548\u7684\uFF0C${receipt.failed}\u5931\u8D25\u7684\u3002`,
    retryUnresolved: "\u4EC5\u9884\u68C0\u672A\u89E3\u51B3",
    actions: {
        publish: "Publish",
        hide: "\u9690\u85CF",
        archive: "\u6863\u6848",
        change_category: "\u66F4\u6539\u7C7B\u522B",
        change_lifecycle: "\u6539\u53D8\u751F\u547D\u5468\u671F",
    } as Record<ProductBulkAction, string>,
},
"ja-JP": {
    title: "Product \u4E00\u62EC\u64CD\u4F5C",
    help: "\u30D7\u30EA\u30D5\u30E9\u30A4\u30C8\u306F\u3001\u9078\u629E\u7BC4\u56F2\u3068\u5404 Product \u30EA\u30D3\u30B8\u30E7\u30F3\u3092\u30D5\u30EA\u30FC\u30BA\u3057\u307E\u3059\u3002\u5B9F\u884C\u3067\u306F\u3001\u5404 Product \u304C\u30A2\u30C8\u30DF\u30C3\u30AF\u306B\u518D\u30C1\u30A7\u30C3\u30AF\u3055\u308C\u3001\u30B3\u30DF\u30C3\u30C8\u3055\u308C\u307E\u3059\u3002 1 \u3064\u306E\u88FD\u54C1\u304C\u7AF6\u5408\u3057\u3066\u3082\u3001\u4ED6\u306E\u6210\u529F\u3057\u305F\u88FD\u54C1\u306F\u6B8B\u308A\u307E\u3059\u3002",
    selection: "\u53B3\u9078\u3055\u308C\u305F\u88FD\u54C1",
    action: "\u624B\u8853",
    target: "\u30BF\u30FC\u30B2\u30C3\u30C8",
    preflight: "\u30D7\u30EA\u30D5\u30E9\u30A4\u30C8\u56FA\u5B9A\u9078\u629E",
    execute: "\u691C\u8A0E\u3057\u305F\u8A08\u753B\u3092\u5B9F\u884C\u3059\u308B",
    confirm: "\u3053\u306E\u56FA\u5B9A\u3055\u308C\u305F\u30D7\u30EA\u30D5\u30E9\u30A4\u30C8\u8A08\u753B\u3092\u5B9F\u884C\u3057\u307E\u3059\u304B?\u5404 Product \u304C\u518D\u691C\u8A3C\u3055\u308C\u307E\u3059\u3002",
    partNumber: "\u90E8\u54C1\u756A\u53F7",
    state: "\u5DDE",
    revision: "\u30EA\u30D3\u30B8\u30E7\u30F3",
    eligible: "\u9069\u683C",
    result: "\u7D50\u679C",
    message: "\u30E1\u30C3\u30BB\u30FC\u30B8",
    yes: "\u306F\u3044",
    no: "\u3044\u3044\u3048",
    selectAtLeastOne: "\u5C11\u306A\u304F\u3068\u3082 1 \u3064\u306E Product \u3092\u9078\u629E\u3057\u307E\u3059\u3002",
    noUnresolved: "\u518D\u8A66\u884C\u3059\u308B\u672A\u89E3\u6C7A\u306E\u88FD\u54C1\u306F\u3042\u308A\u307E\u305B\u3093\u3002",
    previewSummary: (eligible: number, total: number) => `${eligible}\u306E${total}\u88FD\u54C1\u306F\u3053\u306E\u56FA\u5B9A\u30D7\u30EC\u30D5\u30E9\u30A4\u30C8\u306E\u5BFE\u8C61\u3068\u306A\u308A\u307E\u3059\u3002`,
    done: (receipt: ProductBulkReceipt) => `\u4E00\u62EC\u5B8C\u4E86:${receipt.succeeded}\u6210\u529F\u3057\u307E\u3057\u305F\u3001${receipt.no_change}\u5909\u308F\u3089\u305A\u3001${receipt.conflicts}\u7D1B\u4E89\u3001${receipt.invalid}\u7121\u52B9\u3001${receipt.failed}\u5931\u6557\u3057\u305F\u3002`,
    retryUnresolved: "\u30D7\u30EA\u30D5\u30E9\u30A4\u30C8\u672A\u89E3\u6C7A\u306E\u307F",
    actions: {
        publish: "Publish",
        hide: "\u96A0\u308C\u308B",
        archive: "\u30A2\u30FC\u30AB\u30A4\u30D6",
        change_category: "\u30AB\u30C6\u30B4\u30EA\u3092\u5909\u66F4\u3059\u308B",
        change_lifecycle: "\u30E9\u30A4\u30D5\u30B5\u30A4\u30AF\u30EB\u306E\u5909\u66F4",
    } as Record<ProductBulkAction, string>,
},
"ko-KR": {
    title: "Product \uB300\uB7C9 \uC791\uC5C5",
    help: "Preflight\uB294 \uC120\uD0DD \uD56D\uBAA9\uACFC \uAC01 Product \uAC1C\uC815\uC744 \uACE0\uC815\uD569\uB2C8\uB2E4. \uC2E4\uD589\uC740 \uAC01 Product\uB97C \uC6D0\uC790\uC801\uC73C\uB85C \uB2E4\uC2DC \uD655\uC778\uD558\uACE0 \uCEE4\uBC0B\uD569\uB2C8\uB2E4. \uB2E4\uB978 \uC131\uACF5\uC801\uC778 \uC81C\uD488\uC740 \uD558\uB098\uAC00 \uCDA9\uB3CC\uD558\uB354\uB77C\uB3C4 \uADF8\uB300\uB85C \uC720\uC9C0\uB429\uB2C8\uB2E4.",
    selection: "\uC120\uD0DD\uB41C \uC81C\uD488",
    action: "\uC791\uC5C5",
    target: "\uBAA9\uD45C",
    preflight: "\uD504\uB9AC\uD50C\uB77C\uC774\uD2B8 \uACE0\uC815 \uC120\uD0DD",
    execute: "\uAC80\uD1A0\uB41C \uACC4\uD68D \uC2E4\uD589",
    confirm: "\uC774 \uACE0\uC815\uB41C \uBE44\uD589 \uC804 \uACC4\uD68D\uC744 \uC2E4\uD589\uD558\uC2DC\uACA0\uC2B5\uB2C8\uAE4C? \uAC01 Product\uB294 \uC7AC\uAC80\uC99D\uB429\uB2C8\uB2E4.",
    partNumber: "\uBD80\uD488 \uBC88\uD638",
    state: "\uC0C1\uD0DC",
    revision: "\uAC1C\uC815",
    eligible: "\uC790\uACA9\uC774 \uC788\uB294",
    result: "\uACB0\uACFC",
    message: "\uBA54\uC2DC\uC9C0",
    yes: "\uC608",
    no: "\uC544\uB2C8\uC694",
    selectAtLeastOne: "Product\uB97C \uD558\uB098 \uC774\uC0C1 \uC120\uD0DD\uD558\uC138\uC694.",
    noUnresolved: "\uC7AC\uC2DC\uB3C4\uD560 \uD574\uACB0\uB418\uC9C0 \uC54A\uC740 \uC81C\uD488\uC774 \uC5C6\uC2B5\uB2C8\uB2E4.",
    previewSummary: (eligible: number, total: number) => `${eligible}~\uC758${total}\uC81C\uD488\uC740 \uC774 \uACE0\uC815 \uD504\uB9AC\uD50C\uB77C\uC774\uD2B8\uC5D0 \uC801\uD569\uD569\uB2C8\uB2E4.`,
    done: (receipt: ProductBulkReceipt) => `\uB300\uB7C9 \uC644\uB8CC\uB428:${receipt.succeeded}\uC131\uACF5\uD588\uB2E4${receipt.no_change}\uBCC0\uD558\uC9C0 \uC54A\uC740,${receipt.conflicts}\uAC08\uB4F1,${receipt.invalid}\uC720\uD6A8\uD558\uC9C0 \uC54A\uC740,${receipt.failed}\uC2E4\uD328\uD55C.`,
    retryUnresolved: "\uD504\uB9AC\uD50C\uB77C\uC774\uD2B8 \uBBF8\uD574\uACB0\uB9CC",
    actions: {
        publish: "Publish",
        hide: "\uC228\uB2E4",
        archive: "\uBCF4\uAD00\uC18C",
        change_category: "\uCE74\uD14C\uACE0\uB9AC \uBCC0\uACBD",
        change_lifecycle: "\uC218\uBA85\uC8FC\uAE30 \uBCC0\uACBD",
    } as Record<ProductBulkAction, string>,
},
"de-DE": {
    title: "Product Massenoperationen",
    help: "Preflight friert die Auswahl und jede Product-Revision ein. Bei der Ausf\u00FChrung wird jedes Product atomar erneut \u00FCberpr\u00FCft und festgeschrieben. Andere erfolgreiche Produkte bleiben bestehen, wenn eines in Konflikt steht.",
    selection: "Ausgew\u00E4hlte Produkte",
    action: "Betrieb",
    target: "Ziel",
    preflight: "Feste Preflight-Auswahl",
    execute: "F\u00FChren Sie den \u00FCberpr\u00FCften Plan aus",
    confirm: "Diesen festen Preflight-Plan ausf\u00FChren? Jeder Product wird erneut validiert.",
    partNumber: "Teilenummer",
    state: "Zustand",
    revision: "Revision",
    eligible: "Berechtigt",
    result: "Ergebnis",
    message: "Nachricht",
    yes: "Ja",
    no: "NEIN",
    selectAtLeastOne: "W\u00E4hlen Sie mindestens einen Product aus.",
    noUnresolved: "Es sind keine ungel\u00F6sten Produkte vorhanden, die erneut versucht werden k\u00F6nnten.",
    previewSummary: (eligible: number, total: number) => `${eligible}von${total}Produkte sind f\u00FCr diesen festen Preflight berechtigt.`,
    done: (receipt: ProductBulkReceipt) => `Bulk abgeschlossen:${receipt.succeeded}gelungen,${receipt.no_change}unver\u00E4ndert,${receipt.conflicts}Konflikte,${receipt.invalid}ung\u00FCltig,${receipt.failed}fehlgeschlagen.`,
    retryUnresolved: "Nur Preflight ungel\u00F6st",
    actions: {
        publish: "Publish",
        hide: "Verstecken",
        archive: "Archiv",
        change_category: "Kategorie \u00E4ndern",
        change_lifecycle: "Lebenszyklus \u00E4ndern",
    } as Record<ProductBulkAction, string>,
},
"fr-FR": {
    title: "Product Op\u00E9rations group\u00E9es",
    help: "Le contr\u00F4le en amont g\u00E8le la s\u00E9lection et chaque r\u00E9vision Product. L'ex\u00E9cution rev\u00E9rifie et valide chaque Product de mani\u00E8re atomique\u00A0; les autres produits \u00E0 succ\u00E8s restent en cas de conflit.",
    selection: "Produits s\u00E9lectionn\u00E9s",
    action: "Op\u00E9ration",
    target: "Cible",
    preflight: "S\u00E9lection fixe de contr\u00F4le en amont",
    execute: "Ex\u00E9cuter le plan r\u00E9vis\u00E9",
    confirm: "Ex\u00E9cuter ce plan de contr\u00F4le en amont fixe\u00A0? Chaque Product sera revalid\u00E9.",
    partNumber: "Num\u00E9ro de pi\u00E8ce",
    state: "\u00C9tat",
    revision: "R\u00E9vision",
    eligible: "Admissible",
    result: "R\u00E9sultat",
    message: "Message",
    yes: "Oui",
    no: "Non",
    selectAtLeastOne: "S\u00E9lectionnez au moins un Product.",
    noUnresolved: "Il n'y a aucun produit non r\u00E9solu \u00E0 r\u00E9essayer.",
    previewSummary: (eligible: number, total: number) => `${eligible}de${total}Les produits sont \u00E9ligibles dans ce contr\u00F4le en amont fixe.`,
    done: (receipt: ProductBulkReceipt) => `En vrac termin\u00E9\u00A0:${receipt.succeeded}r\u00E9ussi,${receipt.no_change}inchang\u00E9,${receipt.conflicts}conflits,${receipt.invalid}invalide,${receipt.failed}\u00E9chou\u00E9.`,
    retryUnresolved: "Contr\u00F4le en amont non r\u00E9solu uniquement",
    actions: {
        publish: "Publish",
        hide: "Cacher",
        archive: "Archive",
        change_category: "Changer de cat\u00E9gorie",
        change_lifecycle: "Changer le cycle de vie",
    } as Record<ProductBulkAction, string>,
},
"it-IT": {
    title: "Product Operazioni collettive",
    help: "Il preflight congela la selezione e ogni revisione Product. L'esecuzione ricontrolla e impegna atomicamente ogni Product; altri prodotti di successo rimangono quando uno \u00E8 in conflitto.",
    selection: "Prodotti selezionati",
    action: "Operazione",
    target: "Bersaglio",
    preflight: "Selezione fissa di preflight",
    execute: "Eseguire il piano rivisto",
    confirm: "Eseguire questo piano di preflight fisso? Ogni Product verr\u00E0 riconvalidato.",
    partNumber: "Numero di parte",
    state: "Stato",
    revision: "Revisione",
    eligible: "Idoneo",
    result: "Risultato",
    message: "Messaggio",
    yes: "S\u00CC",
    no: "NO",
    selectAtLeastOne: "Seleziona almeno uno Product.",
    noUnresolved: "Non sono presenti prodotti irrisolti da riprovare.",
    previewSummary: (eligible: number, total: number) => `${eligible}Di${total}I prodotti sono idonei in questo preflight fisso.`,
    done: (receipt: ProductBulkReceipt) => `Completato in blocco:${receipt.succeeded}\u00E8 riuscito,${receipt.no_change}invariato,${receipt.conflicts}conflitti,${receipt.invalid}non valido,${receipt.failed}fallito.`,
    retryUnresolved: "Solo verifica preliminare irrisolta",
    actions: {
        publish: "Publish",
        hide: "Nascondere",
        archive: "Archivio",
        change_category: "Cambia categoria",
        change_lifecycle: "Cambiare il ciclo di vita",
    } as Record<ProductBulkAction, string>,
},
"es-ES": {
    title: "Operaciones masivas Product",
    help: "La verificaci\u00F3n previa congela la selecci\u00F3n y cada revisi\u00F3n de Product. La ejecuci\u00F3n vuelve a verificar y confirma cada Product de forma at\u00F3mica; otros Productos exitosos permanecen cuando uno entra en conflicto.",
    selection: "Productos seleccionados",
    action: "Operaci\u00F3n",
    target: "Objetivo",
    preflight: "Selecci\u00F3n fija de verificaci\u00F3n previa",
    execute: "Ejecutar el plan revisado",
    confirm: "\u00BFEjecutar este plan de verificaci\u00F3n previa fijo? Cada Product ser\u00E1 revalidado.",
    partNumber: "N\u00FAmero de pieza",
    state: "Estado",
    revision: "Revisi\u00F3n",
    eligible: "Elegible",
    result: "Resultado",
    message: "Mensaje",
    yes: "S\u00ED",
    no: "No",
    selectAtLeastOne: "Seleccione al menos un Product.",
    noUnresolved: "No hay productos sin resolver para volver a intentarlo.",
    previewSummary: (eligible: number, total: number) => `${eligible}de${total}Los productos son elegibles en esta verificaci\u00F3n previa fija.`,
    done: (receipt: ProductBulkReceipt) => `Completado en masa:${receipt.succeeded}tuvo \u00E9xito,${receipt.no_change}sin alterar,${receipt.conflicts}conflictos,${receipt.invalid}inv\u00E1lido,${receipt.failed}fallido.`,
    retryUnresolved: "Verificaci\u00F3n previa solo sin resolver",
    actions: {
        publish: "Publish",
        hide: "Esconder",
        archive: "Archivo",
        change_category: "Cambiar categor\u00EDa",
        change_lifecycle: "Cambiar ciclo de vida",
    } as Record<ProductBulkAction, string>,
},
"pt-BR": {
    title: "Opera\u00E7\u00F5es em massa Product",
    help: "O Preflight congela a sele\u00E7\u00E3o e cada revis\u00E3o Product. A execu\u00E7\u00E3o verifica novamente e confirma cada Product atomicamente; outros Produtos de sucesso permanecem quando um entra em conflito.",
    selection: "Produtos Selecionados",
    action: "Opera\u00E7\u00E3o",
    target: "Alvo",
    preflight: "Sele\u00E7\u00E3o fixa de comprova\u00E7\u00E3o",
    execute: "Executar plano revisado",
    confirm: "Executar este plano de comprova\u00E7\u00E3o fixo? Cada Product ser\u00E1 revalidado.",
    partNumber: "N\u00FAmero da pe\u00E7a",
    state: "Estado",
    revision: "Revis\u00E3o",
    eligible: "Eleg\u00EDvel",
    result: "Resultado",
    message: "Mensagem",
    yes: "Sim",
    no: "N\u00E3o",
    selectAtLeastOne: "Selecione pelo menos um Product.",
    noUnresolved: "N\u00E3o h\u00E1 produtos n\u00E3o resolvidos para tentar novamente.",
    previewSummary: (eligible: number, total: number) => `${eligible}de${total}Os produtos s\u00E3o eleg\u00EDveis nesta simula\u00E7\u00E3o fixa.`,
    done: (receipt: ProductBulkReceipt) => `Conclu\u00EDdo em massa:${receipt.succeeded}teve sucesso,${receipt.no_change}inalterado,${receipt.conflicts}conflitos,${receipt.invalid}inv\u00E1lido,${receipt.failed}fracassado.`,
    retryUnresolved: "Apenas comprova\u00E7\u00E3o n\u00E3o resolvida",
    actions: {
        publish: "Publish",
        hide: "Esconder",
        archive: "Arquivo",
        change_category: "Alterar categoria",
        change_lifecycle: "Alterar ciclo de vida",
    } as Record<ProductBulkAction, string>,
},
} as const;

type PreflightValues = { action: ProductBulkAction; target_id?: string };

function isUnresolved(status: string): boolean {
  return status === "conflict" || status === "invalid" || status === "failed";
}

export function ProductBulkPanel({ locale, onError, onMessage }: Props) {
  const text = labels[locale];
  const [products, setProducts] = useState<Product[]>([]);
  const [categories, setCategories] = useState<Category[]>([]);
  const [lifecycles, setLifecycles] = useState<DictionaryEntry[]>([]);
  const [selected, setSelected] = useState<Key[]>([]);
  const [run, setRun] = useState<ProductBulkRun>();
  const [busy, setBusy] = useState(false);
  const [form] = Form.useForm<PreflightValues>();
  const action = Form.useWatch("action", form);

  const load = async () => {
    try {
      const [productRows, categoryRows, lifecycleRows] = await Promise.all([
        api<Product[]>("/admin/api/products?include_archived=1"),
        api<Category[]>("/admin/api/categories"),
        api<DictionaryEntry[]>("/admin/api/dictionaries?kind=lifecycle"),
      ]);
      setProducts(productRows ?? []);
      setCategories((categoryRows ?? []).filter((item) => item.status === "active"));
      setLifecycles((lifecycleRows ?? []).filter((item) => item.status === "active"));
    } catch (error) {
      onError(error);
    }
  };

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const preflight = async (values: PreflightValues, productIDs = selected.map(String)) => {
    if (productIDs.length === 0) {
      onError(new Error(text.selectAtLeastOne));
      return;
    }
    setBusy(true);
    try {
      setRun(await postJSON<ProductBulkRun>("/admin/api/product-bulk/preflight", {
        product_ids: productIDs,
        action: values.action,
        target_id: values.target_id ?? "",
      }));
    } catch (error) {
      onError(error);
    } finally {
      setBusy(false);
    }
  };

  const execute = async () => {
    if (!run) return;
    setBusy(true);
    try {
      const receipt = await postJSON<ProductBulkReceipt>(`/admin/api/product-bulk/${run.preview.run_id}/execute`, {});
      setRun({ ...run, status: "completed", receipt });
      onMessage(text.done(receipt));
      await load();
    } catch (error) {
      onError(error);
      try {
        setRun(await api<ProductBulkRun>(`/admin/api/product-bulk/${run.preview.run_id}`));
      } catch {
        // Keep the last durable plan visible if its status cannot be refreshed.
      }
    } finally {
      setBusy(false);
    }
  };

  const retryUnresolved = async () => {
    if (!run?.receipt) return;
    const productIDs = run.receipt.results.filter((item) => isUnresolved(item.status)).map((item) => item.product_id);
    if (productIDs.length === 0) {
      onError(new Error(text.noUnresolved));
      return;
    }
    setSelected(productIDs);
    await preflight({ action: run.preview.action, target_id: run.preview.target_id }, productIDs);
  };

  const requiresTarget = action === "change_category" || action === "change_lifecycle";
  const targetOptions = action === "change_category"
    ? categories.map((item) => ({ value: item.id, label: item.name }))
    : lifecycles.map((item) => ({ value: item.id, label: item.name }));
  const unresolved = run?.receipt?.results.filter((item) => isUnresolved(item.status)) ?? [];

  return (
    <Space direction="vertical" size="large" className="panel-stack">
      <Alert showIcon type="info" message={text.title} description={text.help} />
      <Card title={text.selection}>
        <Table<Product>
          size="small"
          rowKey="id"
          dataSource={products}
          pagination={{ pageSize: 20 }}
          rowSelection={{ selectedRowKeys: selected, onChange: setSelected }}
          columns={[
            { title: text.partNumber, dataIndex: "part_number" },
            { title: text.state, render: (_, item) => `${item.record_state} / ${item.status}` },
            { title: text.revision, dataIndex: "revision", width: 100 },
          ]}
        />
        <Form form={form} layout="inline" initialValues={{ action: "publish" }} onFinish={(values) => void preflight(values)}>
          <Form.Item name="action" label={text.action} rules={[{ required: true }]}>
            <Select
              style={{ width: 210 }}
              options={(Object.keys(text.actions) as ProductBulkAction[]).map((value) => ({ value, label: text.actions[value] }))}
              onChange={() => form.setFieldValue("target_id", undefined)}
            />
          </Form.Item>
          {requiresTarget && (
            <Form.Item name="target_id" label={text.target} rules={[{ required: true }]}>
              <Select showSearch optionFilterProp="label" style={{ width: 240 }} options={targetOptions} />
            </Form.Item>
          )}
          <Button htmlType="submit" type="primary" loading={busy}>{text.preflight}</Button>
        </Form>
      </Card>
      {run && (
        <Card
          title={`${text.actions[run.preview.action]} · ${run.preview.run_id}`}
          extra={run.status !== "completed" && (
            <Popconfirm title={text.confirm} onConfirm={() => void execute()}>
              <Button type="primary" loading={busy}>{text.execute}</Button>
            </Popconfirm>
          )}
        >
          <Space direction="vertical" size="middle" className="panel-stack">
            <Descriptions size="small" column={{ xs: 1, sm: 3 }}>
              <Descriptions.Item label={text.selection}>{run.preview.selection_count}</Descriptions.Item>
              <Descriptions.Item label={text.eligible}>{run.preview.eligible_count}</Descriptions.Item>
              <Descriptions.Item label={text.state}>{run.status}</Descriptions.Item>
            </Descriptions>
            <Typography.Text>{text.previewSummary(run.preview.eligible_count, run.preview.selection_count)}</Typography.Text>
            {run.preview.warning && <Alert type="warning" showIcon message={run.preview.warning} />}
            <Table<ProductBulkPlanItem>
              size="small"
              rowKey="product_id"
              dataSource={run.preview.items}
              pagination={false}
              columns={[
                { title: text.partNumber, dataIndex: "part_number" },
                { title: text.revision, dataIndex: "expected_revision" },
                { title: text.state, render: (_, item) => `${item.record_state} / ${item.publishing_state}` },
                { title: text.eligible, render: (_, item) => <Tag color={item.eligible ? "green" : "red"}>{item.eligible ? text.yes : text.no}</Tag> },
                { title: text.message, dataIndex: "message" },
              ]}
            />
            {run.receipt && (
              <>
                <Table
                  size="small"
                  rowKey="product_id"
                  dataSource={run.receipt.results}
                  pagination={false}
                  columns={[
                    { title: text.partNumber, dataIndex: "part_number" },
                    { title: text.result, dataIndex: "status", render: (value: string) => <Tag color={value === "succeeded" ? "green" : isUnresolved(value) ? "red" : "blue"}>{value}</Tag> },
                    { title: text.revision, dataIndex: "result_revision" },
                    { title: text.message, dataIndex: "message" },
                  ]}
                />
                <Button disabled={unresolved.length === 0} onClick={() => void retryUnresolved()}>
                  {text.retryUnresolved} ({unresolved.length})
                </Button>
              </>
            )}
          </Space>
        </Card>
      )}
    </Space>
  );
}
