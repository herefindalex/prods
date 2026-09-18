import { describe, expect, it } from "vitest";
import {
  DEV_ORIGIN,
  GO_ORIGIN,
  rewriteDevHtml,
  rewriteLocationHeader,
  rewriteUpstreamBrowserOrigin,
} from "./dev-html-rewrite";

describe("rewriteDevHtml", () => {
  it("rewrites admin shell for Vite HMR", () => {
    const html = `<!doctype html><html><head>
<link rel="stylesheet" href="/static/admin/style.css"></head>
<body><div id="admin-root"></div>
<script type="module" src="/static/admin/admin.js"></script></body></html>`;
    const out = rewriteDevHtml(html);
    expect(out).not.toContain("/static/admin/style.css");
    expect(out).not.toContain("/static/admin/admin.js");
    expect(out).toContain('src="/src/admin/main.tsx"');
    expect(out).toContain("/@vite/client");
    expect(out).toContain("/src/dev/hmr-preamble.js");
    expect(out).not.toContain("injectIntoGlobalHook");
  });

  it("rewrites public islands and stylesheet", () => {
    const html = `<!doctype html><html><head>
<link rel="stylesheet" href="/static/public/public.css"></head>
<body><script type="module" src="/static/public/public-islands.js"></script></body></html>`;
    const out = rewriteDevHtml(html);
    expect(out).toContain('href="/src/public/style.css"');
    expect(out).toContain('src="/src/public/main.tsx"');
    expect(out).toContain("/@vite/client");
    expect(out).toContain("/src/dev/hmr-preamble.js");
  });

  it("leaves unrelated HTML unchanged", () => {
    const html = `<!doctype html><html><body><p>ok</p></body></html>`;
    expect(rewriteDevHtml(html)).toBe(html);
  });
});

describe("rewriteLocationHeader", () => {
  it("maps Go origin to Vite origin", () => {
    expect(rewriteLocationHeader(`${GO_ORIGIN}/admin`)).toBe(`${DEV_ORIGIN}/admin`);
    expect(rewriteLocationHeader("http://localhost:3310/")).toBe(`${DEV_ORIGIN}/`);
  });
});

describe("rewriteUpstreamBrowserOrigin", () => {
  it("maps Vite browser origins to the Go base URL origin", () => {
    expect(rewriteUpstreamBrowserOrigin("http://localhost:5173")).toBe(GO_ORIGIN);
    expect(rewriteUpstreamBrowserOrigin("http://127.0.0.1:5173")).toBe(GO_ORIGIN);
    expect(rewriteUpstreamBrowserOrigin("http://localhost:5173/admin/login")).toBe(
      `${GO_ORIGIN}/admin/login`,
    );
    expect(rewriteUpstreamBrowserOrigin("https://evil.example")).toBe("https://evil.example");
  });
});
