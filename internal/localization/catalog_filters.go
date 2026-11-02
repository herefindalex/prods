package localization

// Versioned defaults; professional language review remains outstanding.
func init() {
	labels := map[string][]string{
		"en-US": {"Package / Form factor", "Equals", "Contains text", "Supports value", "Covers entire range", "Between", "Value", "Lower bound", "Upper bound", "Unit", "Check your filters", "Unavailable in this scope", "Use Value for a single value; use both bounds for a range."},
		"zh-TW": {"封裝／外形", "等於", "包含文字", "支援指定值", "涵蓋整段區間", "介於", "數值", "下限", "上限", "單位", "請檢查篩選條件", "此範圍不適用", "查單值請填數值；查區間請填下限與上限。"},
		"zh-CN": {"封装／外形", "等于", "包含文字", "支持指定值", "涵盖整个区间", "介于", "数值", "下限", "上限", "单位", "请检查筛选条件", "此范围不适用", "查单值请填数值；查区间请填下限与上限。"},
		"ja-JP": {"パッケージ／形状", "等しい", "文字を含む", "指定値に対応", "範囲全体に対応", "範囲内", "値", "下限", "上限", "単位", "絞り込み条件を確認してください", "この範囲では使用できません", "単一値には値を、範囲には下限と上限を入力してください。"},
		"ko-KR": {"패키지 / 형태", "같음", "텍스트 포함", "지정 값 지원", "전체 범위 지원", "범위 내", "값", "하한", "상한", "단위", "필터를 확인하세요", "이 범위에서 사용할 수 없음", "단일 값은 값에, 범위는 하한과 상한에 입력하세요."},
		"de-DE": {"Gehäuse / Bauform", "Gleich", "Text enthält", "Unterstützt Wert", "Deckt gesamten Bereich ab", "Zwischen", "Wert", "Untergrenze", "Obergrenze", "Einheit", "Filter prüfen", "In diesem Bereich nicht verfügbar", "Für einen Einzelwert das Wertfeld, für einen Bereich beide Grenzen ausfüllen."},
		"fr-FR": {"Boîtier / Format", "Égal à", "Contient le texte", "Prend en charge la valeur", "Couvre toute la plage", "Entre", "Valeur", "Borne inférieure", "Borne supérieure", "Unité", "Vérifiez les filtres", "Indisponible dans ce périmètre", "Pour une valeur, renseignez Valeur ; pour une plage, renseignez les deux bornes."},
		"it-IT": {"Contenitore / Formato", "Uguale a", "Contiene testo", "Supporta il valore", "Copre tutto l’intervallo", "Tra", "Valore", "Limite inferiore", "Limite superiore", "Unità", "Controlla i filtri", "Non disponibile in questo ambito", "Per un valore compila Valore; per un intervallo compila entrambi i limiti."},
		"es-ES": {"Encapsulado / Formato", "Igual a", "Contiene texto", "Admite el valor", "Cubre todo el intervalo", "Entre", "Valor", "Límite inferior", "Límite superior", "Unidad", "Comprueba los filtros", "No disponible en este ámbito", "Para un valor, rellena Valor; para un intervalo, rellena ambos límites."},
		"pt-BR": {"Encapsulamento / Formato", "Igual a", "Contém texto", "Suporta o valor", "Cobre todo o intervalo", "Entre", "Valor", "Limite inferior", "Limite superior", "Unidade", "Verifique os filtros", "Indisponível neste escopo", "Para um valor, preencha Valor; para um intervalo, preencha os dois limites."},
	}
	for locale, values := range labels {
		m := messageCatalogs[locale]
		m.Package = values[0]
		m.FilterEquals = values[1]
		m.FilterContains = values[2]
		m.FilterSupports = values[3]
		m.FilterCovers = values[4]
		m.FilterBetween = values[5]
		m.FilterValue = values[6]
		m.FilterLower = values[7]
		m.FilterUpper = values[8]
		m.FilterUnit = values[9]
		m.FilterError = values[10]
		m.FilterUnavailable = values[11]
		m.FilterNumericHelp = values[12]
		messageCatalogs[locale] = m
	}
}
