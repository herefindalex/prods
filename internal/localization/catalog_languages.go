package localization

// These built-in translations are the V1 official seed. They have passed
// structural validation, but have not been reviewed by professional native,
// legal, or marketing reviewers.
var simplifiedChinese = Messages{
	Catalog: "产品目录", RequestPart: "询问产品", SearchLabel: "料号或产品名称", Search: "搜索", All: "全部",
	Filters: "筛选条件", ClearFilters: "清除筛选", SelectForRFQ: "加入询价选择", SelectedProducts: "已选产品", ClearSelection: "清除选择", MoreDetails: "更多信息", LessDetails: "收起信息",
	NoProducts: "未找到产品", NoProductsPrefix: "没有已发布产品匹配", RequestThisPart: "询问此料号",
	PreviousPage: "上一页", NextPage: "下一页", PartNumber: "料号", Manufacturer: "制造商", Brand: "品牌",
	Category: "分类", Lifecycle: "生命周期", Applications: "应用", ProductImages: "产品图片",
	Description: "说明", Features: "特性", Specification: "规格", Specifications: "规格参数", Documents: "文档",
	RequestQuote: "请求报价", RFQTitle: "询价单", CatalogProduct: "目录产品", RequestedPart: "询问的产品",
	OriginalSearch: "原始搜索", Name: "姓名", Email: "电子邮箱", QuantityOptional: "数量（可选）", Notes: "备注",
	SubmitRFQ: "提交询价", RFQReceived: "已收到询价", Reference: "参考编号",
	ReplayNotice: "这是原先的成功结果；没有创建重复询价单。", Privacy: "隐私", Terms: "条款", Language: "语言",
}

var japanese = Messages{
	Catalog: "製品カタログ", RequestPart: "製品を問い合わせる", SearchLabel: "品番または製品名", Search: "検索", All: "すべて",
	Filters: "絞り込み", ClearFilters: "絞り込みを解除", SelectForRFQ: "見積対象に選択", SelectedProducts: "選択した製品", ClearSelection: "選択を解除", MoreDetails: "詳細を表示", LessDetails: "詳細を閉じる",
	NoProducts: "製品が見つかりません", NoProductsPrefix: "公開済み製品に一致するものがありません", RequestThisPart: "この品番を問い合わせる",
	PreviousPage: "前のページ", NextPage: "次のページ", PartNumber: "品番", Manufacturer: "メーカー", Brand: "ブランド",
	Category: "カテゴリー", Lifecycle: "ライフサイクル", Applications: "用途", ProductImages: "製品画像",
	Description: "説明", Features: "特長", Specification: "仕様", Specifications: "仕様一覧", Documents: "資料",
	RequestQuote: "見積を依頼", RFQTitle: "見積依頼", CatalogProduct: "カタログ製品", RequestedPart: "問い合わせ対象",
	OriginalSearch: "元の検索", Name: "氏名", Email: "メール", QuantityOptional: "数量（任意）", Notes: "備考",
	SubmitRFQ: "見積依頼を送信", RFQReceived: "見積依頼を受け付けました", Reference: "受付番号",
	ReplayNotice: "これは最初の成功結果です。重複した見積依頼は作成されていません。", Privacy: "プライバシー", Terms: "利用規約", Language: "言語",
}

var korean = Messages{
	Catalog: "제품 카탈로그", RequestPart: "제품 문의", SearchLabel: "부품 번호 또는 제품명", Search: "검색", All: "전체",
	Filters: "필터", ClearFilters: "필터 지우기", SelectForRFQ: "견적 대상 선택", SelectedProducts: "선택한 제품", ClearSelection: "선택 지우기", MoreDetails: "상세 정보 더 보기", LessDetails: "상세 정보 접기",
	NoProducts: "제품을 찾을 수 없음", NoProductsPrefix: "일치하는 공개 제품이 없습니다", RequestThisPart: "이 부품 문의",
	PreviousPage: "이전 페이지", NextPage: "다음 페이지", PartNumber: "부품 번호", Manufacturer: "제조사", Brand: "브랜드",
	Category: "카테고리", Lifecycle: "수명 주기", Applications: "응용 분야", ProductImages: "제품 이미지",
	Description: "설명", Features: "특징", Specification: "사양", Specifications: "사양 목록", Documents: "문서",
	RequestQuote: "견적 요청", RFQTitle: "견적 요청서", CatalogProduct: "카탈로그 제품", RequestedPart: "요청한 제품",
	OriginalSearch: "원래 검색어", Name: "이름", Email: "이메일", QuantityOptional: "수량(선택)", Notes: "메모",
	SubmitRFQ: "견적 요청 제출", RFQReceived: "견적 요청 접수됨", Reference: "참조 번호",
	ReplayNotice: "원래 성공 결과입니다. 중복 견적 요청은 생성되지 않았습니다.", Privacy: "개인정보", Terms: "약관", Language: "언어",
}

var german = Messages{
	Catalog: "Produktkatalog", RequestPart: "Produkt anfragen", SearchLabel: "Teilenummer oder Produktname", Search: "Suchen", All: "Alle",
	Filters: "Filter", ClearFilters: "Filter löschen", SelectForRFQ: "Für Anfrage auswählen", SelectedProducts: "Ausgewählte Produkte", ClearSelection: "Auswahl löschen", MoreDetails: "Mehr Details", LessDetails: "Weniger Details",
	NoProducts: "Keine Produkte gefunden", NoProductsPrefix: "Kein veröffentlichtes Produkt entspricht", RequestThisPart: "Dieses Teil anfragen",
	PreviousPage: "Vorherige Seite", NextPage: "Nächste Seite", PartNumber: "Teilenummer", Manufacturer: "Hersteller", Brand: "Marke",
	Category: "Kategorie", Lifecycle: "Lebenszyklus", Applications: "Anwendungen", ProductImages: "Produktbilder",
	Description: "Beschreibung", Features: "Merkmale", Specification: "Spezifikation", Specifications: "Spezifikationen", Documents: "Dokumente",
	RequestQuote: "Angebot anfordern", RFQTitle: "Angebotsanfrage", CatalogProduct: "Katalogprodukt", RequestedPart: "Angefragtes Teil",
	OriginalSearch: "Ursprüngliche Suche", Name: "Name", Email: "E-Mail", QuantityOptional: "Menge (optional)", Notes: "Hinweise",
	SubmitRFQ: "Anfrage senden", RFQReceived: "Anfrage eingegangen", Reference: "Referenz",
	ReplayNotice: "Dies ist das ursprüngliche erfolgreiche Ergebnis; es wurde keine doppelte Anfrage erstellt.", Privacy: "Datenschutz", Terms: "Bedingungen", Language: "Sprache",
}

var french = Messages{
	Catalog: "Catalogue produits", RequestPart: "Demander un produit", SearchLabel: "Référence ou nom du produit", Search: "Rechercher", All: "Tous",
	Filters: "Filtres", ClearFilters: "Effacer les filtres", SelectForRFQ: "Sélectionner pour le devis", SelectedProducts: "Produits sélectionnés", ClearSelection: "Effacer la sélection", MoreDetails: "Plus de détails", LessDetails: "Moins de détails",
	NoProducts: "Aucun produit trouvé", NoProductsPrefix: "Aucun produit publié ne correspond à", RequestThisPart: "Demander cette référence",
	PreviousPage: "Page précédente", NextPage: "Page suivante", PartNumber: "Référence", Manufacturer: "Fabricant", Brand: "Marque",
	Category: "Catégorie", Lifecycle: "Cycle de vie", Applications: "Applications", ProductImages: "Images du produit",
	Description: "Description", Features: "Caractéristiques", Specification: "Spécification", Specifications: "Spécifications", Documents: "Documents",
	RequestQuote: "Demander un devis", RFQTitle: "Demande de devis", CatalogProduct: "Produit du catalogue", RequestedPart: "Produit demandé",
	OriginalSearch: "Recherche initiale", Name: "Nom", Email: "E-mail", QuantityOptional: "Quantité (facultatif)", Notes: "Remarques",
	SubmitRFQ: "Envoyer la demande", RFQReceived: "Demande reçue", Reference: "Référence",
	ReplayNotice: "Voici le résultat initial réussi ; aucune demande en double n’a été créée.", Privacy: "Confidentialité", Terms: "Conditions", Language: "Langue",
}

var italian = Messages{
	Catalog: "Catalogo prodotti", RequestPart: "Richiedi un prodotto", SearchLabel: "Codice prodotto o nome", Search: "Cerca", All: "Tutti",
	Filters: "Filtri", ClearFilters: "Cancella filtri", SelectForRFQ: "Seleziona per il preventivo", SelectedProducts: "Prodotti selezionati", ClearSelection: "Cancella selezione", MoreDetails: "Altri dettagli", LessDetails: "Meno dettagli",
	NoProducts: "Nessun prodotto trovato", NoProductsPrefix: "Nessun prodotto pubblicato corrisponde a", RequestThisPart: "Richiedi questo codice",
	PreviousPage: "Pagina precedente", NextPage: "Pagina successiva", PartNumber: "Codice prodotto", Manufacturer: "Produttore", Brand: "Marchio",
	Category: "Categoria", Lifecycle: "Ciclo di vita", Applications: "Applicazioni", ProductImages: "Immagini del prodotto",
	Description: "Descrizione", Features: "Caratteristiche", Specification: "Specifica", Specifications: "Specifiche", Documents: "Documenti",
	RequestQuote: "Richiedi preventivo", RFQTitle: "Richiesta di preventivo", CatalogProduct: "Prodotto a catalogo", RequestedPart: "Prodotto richiesto",
	OriginalSearch: "Ricerca originale", Name: "Nome", Email: "E-mail", QuantityOptional: "Quantità (facoltativa)", Notes: "Note",
	SubmitRFQ: "Invia richiesta", RFQReceived: "Richiesta ricevuta", Reference: "Riferimento",
	ReplayNotice: "Questo è il risultato originale riuscito; non è stata creata una richiesta duplicata.", Privacy: "Privacy", Terms: "Condizioni", Language: "Lingua",
}

var spanish = Messages{
	Catalog: "Catálogo de productos", RequestPart: "Solicitar producto", SearchLabel: "Número de pieza o nombre", Search: "Buscar", All: "Todos",
	Filters: "Filtros", ClearFilters: "Borrar filtros", SelectForRFQ: "Seleccionar para cotización", SelectedProducts: "Productos seleccionados", ClearSelection: "Borrar selección", MoreDetails: "Más detalles", LessDetails: "Menos detalles",
	NoProducts: "No se encontraron productos", NoProductsPrefix: "Ningún producto publicado coincide con", RequestThisPart: "Solicitar esta pieza",
	PreviousPage: "Página anterior", NextPage: "Página siguiente", PartNumber: "Número de pieza", Manufacturer: "Fabricante", Brand: "Marca",
	Category: "Categoría", Lifecycle: "Ciclo de vida", Applications: "Aplicaciones", ProductImages: "Imágenes del producto",
	Description: "Descripción", Features: "Características", Specification: "Especificación", Specifications: "Especificaciones", Documents: "Documentos",
	RequestQuote: "Solicitar cotización", RFQTitle: "Solicitud de cotización", CatalogProduct: "Producto del catálogo", RequestedPart: "Producto solicitado",
	OriginalSearch: "Búsqueda original", Name: "Nombre", Email: "Correo electrónico", QuantityOptional: "Cantidad (opcional)", Notes: "Notas",
	SubmitRFQ: "Enviar solicitud", RFQReceived: "Solicitud recibida", Reference: "Referencia",
	ReplayNotice: "Este es el resultado correcto original; no se creó una solicitud duplicada.", Privacy: "Privacidad", Terms: "Términos", Language: "Idioma",
}

var brazilianPortuguese = Messages{
	Catalog: "Catálogo de produtos", RequestPart: "Solicitar produto", SearchLabel: "Número da peça ou nome", Search: "Pesquisar", All: "Todos",
	Filters: "Filtros", ClearFilters: "Limpar filtros", SelectForRFQ: "Selecionar para cotação", SelectedProducts: "Produtos selecionados", ClearSelection: "Limpar seleção", MoreDetails: "Mais detalhes", LessDetails: "Menos detalhes",
	NoProducts: "Nenhum produto encontrado", NoProductsPrefix: "Nenhum produto publicado corresponde a", RequestThisPart: "Solicitar esta peça",
	PreviousPage: "Página anterior", NextPage: "Próxima página", PartNumber: "Número da peça", Manufacturer: "Fabricante", Brand: "Marca",
	Category: "Categoria", Lifecycle: "Ciclo de vida", Applications: "Aplicações", ProductImages: "Imagens do produto",
	Description: "Descrição", Features: "Recursos", Specification: "Especificação", Specifications: "Especificações", Documents: "Documentos",
	RequestQuote: "Solicitar cotação", RFQTitle: "Solicitação de cotação", CatalogProduct: "Produto do catálogo", RequestedPart: "Produto solicitado",
	OriginalSearch: "Pesquisa original", Name: "Nome", Email: "E-mail", QuantityOptional: "Quantidade (opcional)", Notes: "Observações",
	SubmitRFQ: "Enviar solicitação", RFQReceived: "Solicitação recebida", Reference: "Referência",
	ReplayNotice: "Este é o resultado original bem-sucedido; nenhuma solicitação duplicada foi criada.", Privacy: "Privacidade", Terms: "Termos", Language: "Idioma",
}

var messageCatalogs = map[string]Messages{
	"en-US": english,
	"zh-TW": traditionalChinese,
	"zh-CN": simplifiedChinese,
	"ja-JP": japanese,
	"ko-KR": korean,
	"de-DE": german,
	"fr-FR": french,
	"it-IT": italian,
	"es-ES": spanish,
	"pt-BR": brazilianPortuguese,
}
