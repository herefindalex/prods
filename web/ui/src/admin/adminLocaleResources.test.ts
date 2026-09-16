import fs from "node:fs";
import path from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import { ADMIN_LOCALE_OPTIONS, normalizeAdminLocale } from "./locales";

const expectedLocales = ADMIN_LOCALE_OPTIONS.map(({ value }) => value);

const resources = [
  ["main.tsx", "adminText"],
  ["AccessPanel.tsx", "labels"],
  ["ActivityPanel.tsx", "labels"],
  ["BackupPanel.tsx", "labels"],
  ["CatalogPanel.tsx", "labels"],
  ["HealthPanel.tsx", "labels"],
  ["HealthPanel.tsx", "componentNames"],
  ["ImportPanel.tsx", "labels"],
  ["JobsPanel.tsx", "labels"],
  ["ListingProfilesPanel.tsx", "labels"],
  ["ProductBulkPanel.tsx", "labels"],
  ["ProductDataModal.tsx", "labels"],
  ["ProductTranslationsModal.tsx", "labels"],
  ["PublicCopyPanel.tsx", "labels"],
  ["SettingsPanel.tsx", "labels"],
  ["TaxonomyPanel.tsx", "labels"],
  ["TaxonomyTranslationsModal.tsx", "labels"],
  ["TrafficPanel.tsx", "labels"],
  ["WebsitePanel.tsx", "labels"],
] as const;

const scalarResources = [
  ["main.tsx", "openNavigationText"],
  ["main.tsx", "unavailablePageText"],
] as const;

function unwrap(node: ts.Expression): ts.Expression {
  let current = node;
  while (ts.isAsExpression(current) || ts.isSatisfiesExpression(current) || ts.isParenthesizedExpression(current)) {
    current = current.expression;
  }
  return current;
}

function propertyName(node: ts.ObjectLiteralElementLike): string | undefined {
  if (!node.name) return undefined;
  if (ts.isIdentifier(node.name) || ts.isStringLiteral(node.name) || ts.isNumericLiteral(node.name)) return node.name.text;
  return undefined;
}

function objectResource(fileName: string, variableName: string): ts.ObjectLiteralExpression {
  const filePath = path.join(import.meta.dirname, fileName);
  const source = fs.readFileSync(filePath, "utf8");
  const sourceFile = ts.createSourceFile(filePath, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  let result: ts.ObjectLiteralExpression | undefined;

  function visit(node: ts.Node) {
    if (ts.isVariableDeclaration(node) && ts.isIdentifier(node.name) && node.name.text === variableName && node.initializer) {
      const initializer = unwrap(node.initializer);
      if (ts.isObjectLiteralExpression(initializer)) result = initializer;
    }
    ts.forEachChild(node, visit);
  }
  visit(sourceFile);
  if (!result) throw new Error(fileName + " does not define object resource " + variableName);
  return result;
}

function localeObjects(resource: ts.ObjectLiteralExpression): Map<string, ts.ObjectLiteralExpression> {
  const result = new Map<string, ts.ObjectLiteralExpression>();
  for (const property of resource.properties) {
    if (!ts.isPropertyAssignment(property)) continue;
    const name = propertyName(property);
    const initializer = unwrap(property.initializer);
    if (name && ts.isObjectLiteralExpression(initializer)) result.set(name, initializer);
  }
  return result;
}

function shape(node: ts.Node): unknown {
  if (ts.isObjectLiteralExpression(node)) {
    return node.properties
      .filter(ts.isPropertyAssignment)
      .map((property) => [propertyName(property), shape(unwrap(property.initializer))]);
  }
  if (ts.isArrowFunction(node) || ts.isFunctionExpression(node)) {
    return {
      kind: "function",
      parameters: node.parameters.map((parameter) => parameter.name.getText()),
      body: shape(node.body),
    };
  }
  if (ts.isConditionalExpression(node)) {
    return { kind: "conditional", whenTrue: shape(node.whenTrue), whenFalse: shape(node.whenFalse) };
  }
  if (ts.isTemplateExpression(node)) {
    const identifiers = new Set<string>();
    for (const span of node.templateSpans) {
      function visit(expressionNode: ts.Node) {
        if (ts.isIdentifier(expressionNode)) identifiers.add(expressionNode.text);
        ts.forEachChild(expressionNode, visit);
      }
      visit(span.expression);
    }
    return {
      kind: "template",
      identifiers: [...identifiers].sort(),
    };
  }
  if (ts.isNoSubstitutionTemplateLiteral(node) || ts.isStringLiteral(node)) return { kind: "text" };
  return { kind: ts.SyntaxKind[node.kind] };
}

describe("Admin locale resources", () => {
  it("defines exactly the approved ten locales in the official order", () => {
    expect(expectedLocales).toEqual([
      "en-US",
      "zh-TW",
      "zh-CN",
      "ja-JP",
      "ko-KR",
      "de-DE",
      "fr-FR",
      "it-IT",
      "es-ES",
      "pt-BR",
    ]);
  });

  it.each(resources)("%s %s has complete locale and placeholder parity", (fileName, variableName) => {
    const locales = localeObjects(objectResource(fileName, variableName));
    expect([...locales.keys()]).toEqual(expectedLocales);
    const english = locales.get("en-US");
    expect(english).toBeDefined();
    const englishShape = shape(english!);
    for (const locale of expectedLocales) {
      expect(shape(locales.get(locale)!), fileName + ":" + variableName + ":" + locale).toEqual(englishShape);
    }
  });

  it.each(scalarResources)("%s %s covers every locale", (fileName, variableName) => {
    const resource = objectResource(fileName, variableName);
    expect(resource.properties.map(propertyName)).toEqual(expectedLocales);
  });

  it("contains no legacy two-locale application-message fallback", () => {
    const files = fs.readdirSync(import.meta.dirname).filter((name) => /\.(?:ts|tsx)$/.test(name) && name !== "adminLocaleResources.test.ts");
    const source = files.map((name) => fs.readFileSync(path.join(import.meta.dirname, name), "utf8")).join("\n");
    expect(source).not.toContain("implementedAdminMessageLocale");
    expect(source).not.toContain('locale === "zh-TW"');
    expect(source).not.toMatch(/locale:\s*"en-US"\s*\|\s*"zh-TW"/);
    expect(source).not.toMatch(/ZXQ\d+QXZ/);
  });

  it("formats dates and numbers for every Admin locale", () => {
    const date = new Date("2026-09-16T12:34:56Z");
    for (const locale of expectedLocales) {
      expect(new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short", timeZone: "UTC" }).format(date)).not.toBe("");
      expect(new Intl.NumberFormat(locale, { maximumFractionDigits: 2 }).format(1234567.89)).not.toBe("");
      expect(normalizeAdminLocale(locale.toLowerCase())).toBe(locale);
    }
  });
});
