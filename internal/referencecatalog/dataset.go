package referencecatalog

import (
	"fmt"
	"math/rand"
	"slices"
	"sort"
	"strings"
	"time"

	"prods/internal/catalog"
)

const (
	Version         = "a01-r1"
	DefaultSeed     = int64(20260916)
	ProductCount    = 1200
	PublishedCount  = 1020
	HiddenCount     = 140
	ArchivedCount   = 40
	ReferenceTime   = "2026-09-16T12:00:00Z"
	SourceStatement = "Deterministic synthetic electronic-component data; no customer records, third-party catalog copy, or production assets."
)

type DesiredState string

const (
	DesiredPublished DesiredState = "published"
	DesiredHidden    DesiredState = "hidden"
	DesiredArchived  DesiredState = "archived"
)

type ProductSeed struct {
	Product      catalog.Product
	SpecValues   []SpecValueSeed
	Translations []catalog.ProductTranslation
	Documents    []catalog.ProductDocument
	DesiredState DesiredState
	ContentLevel string
}

type SpecValueSeed struct {
	SpecID       string
	RawValue     string
	SourceLocale string
}

type Dataset struct {
	Version         string
	Seed            int64
	ReferenceTime   time.Time
	Categories      []catalog.Category
	Dictionaries    []catalog.DictionaryEntry
	Specs           []catalog.SpecDefinition
	SpecSets        []catalog.SpecSet
	CategorySpecSet map[string]string
	Products        []ProductSeed
}

type categoryDefinition struct {
	Key    string
	Parent string
	Name   string
}

var categoryDefinitions = []categoryDefinition{
	{"semiconductors", "", "Semiconductors"},
	{"semi-analog", "semiconductors", "Analog ICs"},
	{"semi-regulators", "semi-analog", "Voltage Regulators"},
	{"semi-ldo", "semi-regulators", "LDO Regulators"},
	{"semi-ldo-auto", "semi-ldo", "Automotive LDO Regulators"},
	{"semi-buck", "semi-regulators", "Buck Regulators"},
	{"semi-amplifiers", "semi-analog", "Amplifiers"},
	{"semi-digital", "semiconductors", "Digital ICs"},
	{"semi-logic", "semi-digital", "Logic"},
	{"semi-memory", "semi-digital", "Memory ICs"},
	{"semi-discrete", "semiconductors", "Discrete Semiconductors"},
	{"semi-mosfet", "semi-discrete", "MOSFETs"},
	{"semi-diodes", "semi-discrete", "Diodes"},
	{"power", "", "Power"},
	{"power-supplies", "power", "Power Supplies"},
	{"power-acdc", "power-supplies", "AC-DC Power Supplies"},
	{"power-dcdc", "power-supplies", "DC-DC Modules"},
	{"power-battery", "power", "Battery Management"},
	{"power-controllers", "power", "Power Controllers"},
	{"sensors", "", "Sensors"},
	{"sensor-temperature", "sensors", "Temperature Sensors"},
	{"sensor-pressure", "sensors", "Pressure Sensors"},
	{"sensor-motion", "sensors", "Motion Sensors"},
	{"sensor-accelerometers", "sensor-motion", "Accelerometers"},
	{"sensor-gyroscopes", "sensor-motion", "Gyroscopes"},
	{"sensor-environmental", "sensors", "Environmental Sensors"},
	{"connectors", "", "Connectors"},
	{"connector-board", "connectors", "Board-to-Board Connectors"},
	{"connector-wire", "connectors", "Wire-to-Board Connectors"},
	{"connector-circular", "connectors", "Circular Connectors"},
	{"connector-rf", "connectors", "RF Connectors"},
	{"passives", "", "Passive Components"},
	{"passive-resistors", "passives", "Resistors"},
	{"passive-capacitors", "passives", "Capacitors"},
	{"passive-inductors", "passives", "Inductors"},
	{"passive-crystals", "passives", "Crystals and Oscillators"},
	{"optoelectronics", "", "Optoelectronics"},
	{"opto-leds", "optoelectronics", "LEDs"},
	{"opto-displays", "optoelectronics", "Displays"},
	{"opto-photodetectors", "optoelectronics", "Photodetectors"},
	{"opto-lasers", "optoelectronics", "Laser Diodes"},
	{"electromechanical", "", "Electromechanical"},
	{"electro-relays", "electromechanical", "Relays"},
	{"electro-switches", "electromechanical", "Switches"},
	{"electro-motors", "electromechanical", "Motors"},
	{"electro-fans", "electromechanical", "Fans"},
	{"embedded", "", "Embedded and Modules"},
	{"embedded-mcu", "embedded", "Microcontrollers"},
	{"embedded-sbc", "embedded", "Single-board Computers"},
	{"embedded-wireless", "embedded", "Wireless Modules"},
	{"embedded-memory", "embedded", "Memory Modules"},
	{"test-protection", "", "Test and Protection"},
	{"protection-circuit", "test-protection", "Circuit Protection"},
	{"protection-fuses", "protection-circuit", "Fuses"},
	{"protection-tvs", "protection-circuit", "TVS Diodes"},
	{"test-points", "test-protection", "Test Points"},
	{"protection-isolation", "test-protection", "Isolation Components"},
	{"empty-evaluation", "test-protection", "Evaluation Samples"},
}

func Build(seed int64) (Dataset, Manifest, error) {
	if seed == 0 {
		seed = DefaultSeed
	}
	referenceTime, err := time.Parse(time.RFC3339, ReferenceTime)
	if err != nil {
		return Dataset{}, Manifest{}, err
	}
	dataset := Dataset{
		Version:         Version,
		Seed:            seed,
		ReferenceTime:   referenceTime,
		Categories:      buildCategories(),
		Dictionaries:    buildDictionaries(),
		Specs:           buildSpecs(),
		SpecSets:        buildSpecSets(),
		CategorySpecSet: make(map[string]string),
	}
	for _, definition := range categoryDefinitions {
		dataset.CategorySpecSet[categoryID(definition.Key)] = specSetForCategory(definition.Key)
	}
	dataset.Products = buildProducts(seed, dataset)
	manifest := buildManifest(dataset)
	if err := validate(dataset, manifest); err != nil {
		return Dataset{}, Manifest{}, err
	}
	return dataset, manifest, nil
}

func buildCategories() []catalog.Category {
	categories := make([]catalog.Category, 0, len(categoryDefinitions))
	for _, definition := range categoryDefinitions {
		parentID := "cat_root"
		if definition.Parent != "" {
			parentID = categoryID(definition.Parent)
		}
		categories = append(categories, catalog.Category{
			ID:           categoryID(definition.Key),
			ParentID:     parentID,
			Name:         definition.Name,
			Description:  "Synthetic reference category for responsive catalog verification.",
			SourceLocale: "en-US",
			Slug:         definition.Key,
			Status:       catalog.EntryActive,
		})
	}
	return categories
}

func buildDictionaries() []catalog.DictionaryEntry {
	var entries []catalog.DictionaryEntry
	for index := 1; index <= 20; index++ {
		entries = append(entries, dictionaryEntry(catalog.DictionaryManufacturer, fmt.Sprintf("mfr_ref_%02d", index), fmt.Sprintf("Reference Manufacturer %02d", index)))
	}
	for index := 1; index <= 15; index++ {
		entries = append(entries, dictionaryEntry(catalog.DictionaryBrand, fmt.Sprintf("brd_ref_%02d", index), fmt.Sprintf("Reference Brand %02d", index)))
	}
	applicationNames := []string{"Automotive", "Industrial Control", "Medical Instrumentation", "Consumer Electronics", "Renewable Energy", "Robotics", "Aerospace Test", "Building Automation", "Data Center", "Telecommunications", "Audio", "Lighting", "Motor Control", "Battery Systems", "Smart Metering", "Security", "Agriculture", "Rail", "Marine", "Laboratory", "Wearables", "Networking", "Factory Automation", "Education"}
	for index, name := range applicationNames {
		entries = append(entries, dictionaryEntry(catalog.DictionaryApplication, fmt.Sprintf("app_ref_%02d", index+1), name))
	}
	for index, name := range []string{"Active", "NRND", "LTB", "EOL", "Obsolete"} {
		entries = append(entries, dictionaryEntry(catalog.DictionaryLifecycle, fmt.Sprintf("life_ref_%02d", index+1), name))
	}
	return entries
}

func dictionaryEntry(kind catalog.DictionaryKind, id, name string) catalog.DictionaryEntry {
	return catalog.DictionaryEntry{ID: id, Kind: kind, Name: name, Description: "Synthetic reference value.", SourceLocale: "en-US", Slug: strings.ReplaceAll(strings.ToLower(id), "_", "-"), Status: catalog.EntryActive}
}

func buildSpecs() []catalog.SpecDefinition {
	definitions := []struct {
		id, name, unit string
		filterable     bool
	}{
		{"spc_input_voltage", "Input voltage range", "V", true}, {"spc_output_voltage", "Output voltage", "V", true},
		{"spc_output_current", "Output current", "A", true}, {"spc_quiescent_current", "Quiescent current", "uA", true},
		{"spc_dropout", "Dropout voltage", "mV", true}, {"spc_switching_frequency", "Switching frequency", "kHz", true},
		{"spc_efficiency", "Efficiency", "%", true}, {"spc_gain", "Gain", "dB", true},
		{"spc_bandwidth", "Bandwidth", "MHz", true}, {"spc_logic_family", "Logic family", "", true},
		{"spc_memory_density", "Memory density", "Mbit", true}, {"spc_rds_on", "On resistance", "mOhm", true},
		{"spc_reverse_voltage", "Reverse voltage", "V", true}, {"spc_power", "Rated power", "W", true},
		{"spc_measurement_range", "Measurement range", "", true}, {"spc_interface", "Interface", "", true},
		{"spc_accuracy", "Accuracy", "%", true}, {"spc_positions", "Positions", "", true},
		{"spc_pitch", "Pitch", "mm", true}, {"spc_rated_contact_current", "Rated contact current", "A", true},
		{"spc_mounting", "Mounting style", "", true}, {"spc_resistance", "Resistance", "ohm", true},
		{"spc_capacitance", "Capacitance", "uF", true}, {"spc_inductance", "Inductance", "uH", true},
		{"spc_tolerance", "Tolerance", "%", true}, {"spc_wavelength", "Wavelength", "nm", true},
		{"spc_luminous_intensity", "Luminous intensity", "mcd", true}, {"spc_coil_voltage", "Coil voltage", "V", true},
		{"spc_contact_rating", "Contact rating", "A", true}, {"spc_core", "Processor core", "", true},
		{"spc_clock", "Clock frequency", "MHz", true}, {"spc_wireless_standard", "Wireless standard", "", true},
		{"spc_breakdown_voltage", "Breakdown voltage", "V", true}, {"spc_package", "Package", "", false},
	}
	specs := make([]catalog.SpecDefinition, 0, len(definitions))
	for _, definition := range definitions {
		specs = append(specs, catalog.SpecDefinition{ID: definition.id, Name: definition.name, PreferredUnit: definition.unit, Filterable: definition.filterable, SemanticVer: 1, Status: catalog.EntryActive})
	}
	return specs
}

func buildSpecSets() []catalog.SpecSet {
	sets := []catalog.SpecSet{
		specSet("ldo", "LDO", "spc_input_voltage", "spc_output_voltage", "spc_output_current", "spc_quiescent_current", "spc_dropout", "spc_package"),
		specSet("buck", "Buck regulator", "spc_input_voltage", "spc_output_voltage", "spc_output_current", "spc_switching_frequency", "spc_efficiency", "spc_package"),
		specSet("amplifier", "Amplifier", "spc_input_voltage", "spc_gain", "spc_bandwidth", "spc_quiescent_current", "spc_package"),
		specSet("logic", "Logic", "spc_input_voltage", "spc_logic_family", "spc_output_current", "spc_package"),
		specSet("memory", "Memory", "spc_input_voltage", "spc_memory_density", "spc_interface", "spc_clock", "spc_package"),
		specSet("mosfet", "MOSFET", "spc_input_voltage", "spc_output_current", "spc_rds_on", "spc_power", "spc_package"),
		specSet("diode", "Diode", "spc_reverse_voltage", "spc_output_current", "spc_power", "spc_package"),
		specSet("acdc", "AC-DC supply", "spc_input_voltage", "spc_output_voltage", "spc_output_current", "spc_efficiency", "spc_power"),
		specSet("dcdc", "DC-DC module", "spc_input_voltage", "spc_output_voltage", "spc_output_current", "spc_efficiency", "spc_package"),
		specSet("battery", "Battery management", "spc_input_voltage", "spc_output_current", "spc_interface", "spc_package"),
		specSet("power-controller", "Power controller", "spc_input_voltage", "spc_output_current", "spc_switching_frequency", "spc_package"),
		specSet("temperature", "Temperature sensor", "spc_input_voltage", "spc_measurement_range", "spc_interface", "spc_accuracy", "spc_package"),
		specSet("pressure", "Pressure sensor", "spc_input_voltage", "spc_measurement_range", "spc_interface", "spc_accuracy", "spc_package"),
		specSet("motion", "Motion sensor", "spc_input_voltage", "spc_measurement_range", "spc_interface", "spc_bandwidth", "spc_package"),
		specSet("environmental", "Environmental sensor", "spc_input_voltage", "spc_measurement_range", "spc_interface", "spc_accuracy", "spc_package"),
		specSet("board-connector", "Board connector", "spc_positions", "spc_pitch", "spc_rated_contact_current", "spc_mounting", "spc_package"),
		specSet("wire-connector", "Wire connector", "spc_positions", "spc_pitch", "spc_rated_contact_current", "spc_mounting", "spc_package"),
		specSet("rf-connector", "RF connector", "spc_interface", "spc_rated_contact_current", "spc_mounting", "spc_package"),
		specSet("passive", "Passive component", "spc_resistance", "spc_capacitance", "spc_inductance", "spc_tolerance", "spc_power", "spc_package"),
		specSet("opto", "Optoelectronic", "spc_input_voltage", "spc_output_current", "spc_wavelength", "spc_luminous_intensity", "spc_package"),
		specSet("electromechanical", "Electromechanical", "spc_input_voltage", "spc_coil_voltage", "spc_contact_rating", "spc_power", "spc_package"),
		specSet("microcontroller", "Microcontroller", "spc_input_voltage", "spc_core", "spc_clock", "spc_memory_density", "spc_interface", "spc_package"),
		specSet("module", "Module", "spc_input_voltage", "spc_core", "spc_clock", "spc_interface", "spc_wireless_standard", "spc_package"),
		specSet("protection", "Protection", "spc_input_voltage", "spc_reverse_voltage", "spc_breakdown_voltage", "spc_output_current", "spc_package"),
	}
	return sets
}

func specSet(key, name string, specIDs ...string) catalog.SpecSet {
	return catalog.SpecSet{ID: "sps_ref_" + key, Name: name, Status: catalog.EntryActive, SpecIDs: specIDs}
}

func specSetForCategory(key string) string {
	switch {
	case strings.Contains(key, "ldo"):
		return "sps_ref_ldo"
	case strings.Contains(key, "buck"):
		return "sps_ref_buck"
	case strings.Contains(key, "amplifier"), key == "semi-analog":
		return "sps_ref_amplifier"
	case strings.Contains(key, "logic"), key == "semi-digital", key == "semiconductors":
		return "sps_ref_logic"
	case strings.Contains(key, "memory"):
		return "sps_ref_memory"
	case strings.Contains(key, "mosfet"), key == "semi-discrete":
		return "sps_ref_mosfet"
	case strings.Contains(key, "diode"):
		return "sps_ref_diode"
	case strings.Contains(key, "acdc"), key == "power-supplies":
		return "sps_ref_acdc"
	case strings.Contains(key, "dcdc"):
		return "sps_ref_dcdc"
	case strings.Contains(key, "battery"):
		return "sps_ref_battery"
	case key == "power", strings.Contains(key, "power-controller"):
		return "sps_ref_power-controller"
	case strings.Contains(key, "temperature"), key == "sensors":
		return "sps_ref_temperature"
	case strings.Contains(key, "pressure"):
		return "sps_ref_pressure"
	case strings.Contains(key, "motion"), strings.Contains(key, "accelerometer"), strings.Contains(key, "gyroscope"):
		return "sps_ref_motion"
	case strings.Contains(key, "environmental"):
		return "sps_ref_environmental"
	case key == "connectors", strings.Contains(key, "board"), strings.Contains(key, "circular"):
		return "sps_ref_board-connector"
	case strings.Contains(key, "wire"):
		return "sps_ref_wire-connector"
	case strings.Contains(key, "connector-rf"):
		return "sps_ref_rf-connector"
	case strings.Contains(key, "passive"):
		return "sps_ref_passive"
	case strings.Contains(key, "opto"):
		return "sps_ref_opto"
	case strings.Contains(key, "electro"):
		return "sps_ref_electromechanical"
	case strings.Contains(key, "mcu"):
		return "sps_ref_microcontroller"
	case strings.Contains(key, "embedded"):
		return "sps_ref_module"
	default:
		return "sps_ref_protection"
	}
}

func buildProducts(seed int64, dataset Dataset) []ProductSeed {
	counts := productCounts()
	setByID := make(map[string]catalog.SpecSet, len(dataset.SpecSets))
	for _, set := range dataset.SpecSets {
		setByID[set.ID] = set
	}
	products := make([]ProductSeed, 0, ProductCount)
	products = append(products, productForIndex(1, catalog.UncategorizedCategoryID, catalog.SpecSet{}, dataset))
	index := 2
	for _, definition := range categoryDefinitions {
		category := categoryID(definition.Key)
		set := setByID[dataset.CategorySpecSet[category]]
		for local := 0; local < counts[definition.Key]; local++ {
			products = append(products, productForIndex(index, category, set, dataset))
			index++
		}
	}
	assignDesiredStates(seed, products)
	return products
}

func productCounts() map[string]int {
	counts := make(map[string]int, len(categoryDefinitions))
	counts["semi-ldo-auto"] = 220
	counts["sensor-gyroscopes"] = 1
	counts["empty-evaluation"] = 0
	remaining := 0
	for _, definition := range categoryDefinitions {
		if _, fixed := counts[definition.Key]; fixed {
			continue
		}
		count := 17
		if remaining < 43 {
			count++
		}
		remaining++
		counts[definition.Key] = count
	}
	return counts
}

func productForIndex(index int, categoryID string, set catalog.SpecSet, dataset Dataset) ProductSeed {
	id := fmt.Sprintf("prd_ref_%04d", index)
	partNumber := fmt.Sprintf("REF-%s-%04d", shortFamily(set.ID), index)
	if index == 2 {
		partNumber = "abc123"
	} else if index == 3 {
		partNumber = "ABC123"
	} else if index == 4 {
		partNumber = "LITERAL%_REF"
	} else if index == 5 {
		partNumber = "ÉCO-SENSOR-ß"
	}
	level := []string{"minimal", "basic", "basic", "typical", "typical", "typical", "typical", "rich", "rich", "edge"}[(index-1)%10]
	manufacturerIndex := (index-1)%20 + 1
	brandIndex := (index-1)%15 + 1
	product := catalog.Product{
		ID:                id,
		PartNumber:        partNumber,
		CategoryID:        categoryID,
		SourceLocale:      "en-US",
		PackageFormFactor: []string{"QFN-24", "SOT-23-5", "0603", "Through-hole", "Module"}[(index-1)%5],
		Status:            catalog.Hidden,
	}
	if level != "minimal" {
		product.Name = fmt.Sprintf("Synthetic %s component %04d", strings.TrimPrefix(set.ID, "sps_ref_"), index)
	}
	if index%17 != 0 {
		product.ManufacturerID = fmt.Sprintf("mfr_ref_%02d", manufacturerIndex)
	}
	if index%13 != 0 {
		product.BrandID = fmt.Sprintf("brd_ref_%02d", brandIndex)
	}
	if index%6 != 0 {
		product.LifecycleID = fmt.Sprintf("life_ref_%02d", (index-1)%5+1)
	}
	applicationCount := index % 4
	for application := 0; application < applicationCount; application++ {
		product.ApplicationIDs = append(product.ApplicationIDs, fmt.Sprintf("app_ref_%02d", (index+application)%24+1))
	}
	switch level {
	case "basic":
		product.Description = "Synthetic basic component for sparse-row verification."
	case "typical":
		product.Description = "Synthetic reference component with representative catalog content."
		product.Features = "Stable synthetic identity\nRepresentative electrical data"
		product.DocumentURL = fmt.Sprintf("https://docs.example.test/reference/%s.pdf", strings.ToLower(id))
	case "rich", "edge":
		product.Description = "Synthetic reference component with multiple paragraphs, conditional engineering values, and intentionally varied content.\n\nThis text is test material and is not suitable for real component selection."
		product.Features = "Deterministic fixture content\nLong-form responsive layout case\nMultiple application relationships"
		product.DocumentURL = fmt.Sprintf("https://docs.example.test/reference/%s.pdf", strings.ToLower(id))
	}
	if level == "edge" {
		product.Name += " — 超長名稱 für responsive Prüfung 日本語"
	}
	seed := ProductSeed{Product: product, DesiredState: DesiredHidden, ContentLevel: level}
	limit := len(set.SpecIDs)
	if level == "minimal" {
		limit = 0
	} else if level == "basic" && limit > 3 {
		limit = 3
	} else if level == "typical" && limit > 6 {
		limit = 6
	}
	for specIndex, specID := range set.SpecIDs[:limit] {
		if (index+specIndex)%19 == 0 {
			continue
		}
		seed.SpecValues = append(seed.SpecValues, SpecValueSeed{SpecID: specID, RawValue: rawSpecValue(specID, index), SourceLocale: "en-US"})
	}
	if level == "rich" || level == "edge" {
		seed.Documents = []catalog.ProductDocument{
			{ID: fmt.Sprintf("doc_ref_%04d_a", index), ProductID: id, Label: "Reference datasheet", ExternalURL: fmt.Sprintf("https://docs.example.test/reference/%s-datasheet.pdf", id), Language: "en-US", SortOrder: 0},
			{ID: fmt.Sprintf("doc_ref_%04d_b", index), ProductID: id, Label: "Reference application note", ExternalURL: fmt.Sprintf("https://docs.example.test/reference/%s-note.pdf", id), Language: "en-US", SortOrder: 1},
		}
	}
	if index%10 == 0 {
		seed.Translations = append(seed.Translations, catalog.ProductTranslation{Locale: "zh-TW", Name: fmt.Sprintf("合成參考元件 %04d", index), Description: "用於多語回退與響應式列表驗證的合成內容。"})
	}
	if index%25 == 0 {
		seed.Translations = append(seed.Translations, catalog.ProductTranslation{Locale: "ja-JP", Name: fmt.Sprintf("合成リファレンス部品 %04d", index), Description: "多言語表示を確認するための合成データです。"})
	}
	return seed
}

func assignDesiredStates(seed int64, products []ProductSeed) {
	for index := 0; index < 4 && index < len(products); index++ {
		products[index].DesiredState = DesiredPublished
	}
	indices := make([]int, 0, len(products)-4)
	for index := 4; index < len(products); index++ {
		indices = append(indices, index)
	}
	random := rand.New(rand.NewSource(seed))
	random.Shuffle(len(indices), func(left, right int) { indices[left], indices[right] = indices[right], indices[left] })
	publishedRemaining := PublishedCount - min(4, len(products))
	for position, index := range indices {
		switch {
		case position < publishedRemaining:
			products[index].DesiredState = DesiredPublished
		case position < publishedRemaining+ArchivedCount:
			products[index].DesiredState = DesiredArchived
		default:
			products[index].DesiredState = DesiredHidden
		}
	}
}

func rawSpecValue(specID string, index int) string {
	switch specID {
	case "spc_input_voltage":
		if index%9 == 0 {
			return "3.0–3.6 V, 4.5–5.5 V"
		}
		return fmt.Sprintf("%d–%d V", 2+index%5, 8+index%17)
	case "spc_output_voltage":
		return []string{"0.8 V", "1.2 V", "1.8 V", "3.3 V", "5 V"}[index%5]
	case "spc_output_current", "spc_rated_contact_current":
		return fmt.Sprintf("%d.%d A", index%5, index%10)
	case "spc_measurement_range":
		return []string{"−40…125 °C", "0…100 kPa", "±2 g / ±4 g", "10–90 %RH"}[index%4]
	case "spc_interface":
		return []string{"I²C", "SPI", "UART", "Analog"}[index%4]
	case "spc_package":
		return []string{"QFN-24", "SOT-23-5", "0603", "Module"}[index%4]
	case "spc_positions":
		return fmt.Sprintf("%d", 2+(index%40))
	case "spc_pitch":
		return []string{"0.50 mm", "1.00 mm", "2.00 mm", "2.54 mm"}[index%4]
	case "spc_tolerance":
		return []string{"±0.1 %", "±1 %", "±5 %", "asymmetric +10/−5 %"}[index%4]
	default:
		return fmt.Sprintf("%d %s", 1+index%97, strings.TrimPrefix(specID, "spc_"))
	}
}

func shortFamily(specSetID string) string {
	value := strings.TrimPrefix(specSetID, "sps_ref_")
	value = strings.ToUpper(strings.ReplaceAll(value, "-", ""))
	if value == "" {
		return "MIN"
	}
	if len(value) > 8 {
		return value[:8]
	}
	return value
}

func categoryID(key string) string { return "cat_ref_" + strings.ReplaceAll(key, "-", "_") }

func validate(dataset Dataset, manifest Manifest) error {
	if len(dataset.Categories)+2 != 60 {
		return fmt.Errorf("reference categories including system categories = %d, want 60", len(dataset.Categories)+2)
	}
	if len(dataset.SpecSets) != 24 || len(dataset.Products) != ProductCount {
		return fmt.Errorf("reference dimensions spec_sets=%d products=%d", len(dataset.SpecSets), len(dataset.Products))
	}
	counts := map[DesiredState]int{}
	for _, product := range dataset.Products {
		counts[product.DesiredState]++
	}
	if counts[DesiredPublished] != PublishedCount || counts[DesiredHidden] != HiddenCount || counts[DesiredArchived] != ArchivedCount {
		return fmt.Errorf("reference state distribution = %+v", counts)
	}
	if manifest.Summary.Products != ProductCount || len(manifest.Taxonomy) != len(dataset.Categories)+2 {
		return fmt.Errorf("manifest dimensions do not match dataset")
	}
	return nil
}

func sortedProductIDs(products []ProductSeed, state DesiredState) []string {
	ids := make([]string, 0)
	for _, product := range products {
		if product.DesiredState == state {
			ids = append(ids, product.Product.ID)
		}
	}
	slices.Sort(ids)
	return ids
}

func sortSearchResults(products []ProductSeed, query string) []string {
	folded := catalog.FoldSearch(query)
	type result struct {
		id, part string
		rank     int
	}
	var results []result
	for _, seed := range products {
		if seed.DesiredState != DesiredPublished {
			continue
		}
		part := catalog.FoldSearch(seed.Product.PartNumber)
		name := catalog.FoldSearch(seed.Product.Name)
		manufacturer := catalog.FoldSearch(seed.Product.Manufacturer)
		brand := catalog.FoldSearch(seed.Product.Brand)
		rank := 4
		switch {
		case part == folded:
			rank = 0
		case strings.HasPrefix(part, folded):
			rank = 1
		case strings.Contains(part, folded):
			rank = 2
		case strings.Contains(name, folded) || strings.Contains(manufacturer, folded) || strings.Contains(brand, folded):
			rank = 3
		default:
			continue
		}
		results = append(results, result{id: seed.Product.ID, part: part, rank: rank})
	}
	sort.Slice(results, func(left, right int) bool {
		if results[left].rank != results[right].rank {
			return results[left].rank < results[right].rank
		}
		if results[left].part != results[right].part {
			return results[left].part < results[right].part
		}
		return results[left].id < results[right].id
	})
	ids := make([]string, len(results))
	for index, result := range results {
		ids[index] = result.id
	}
	return ids
}
