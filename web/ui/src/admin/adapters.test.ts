import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";

vi.mock("./locales", () => ({ normalizeAdminLocale: (locale: string) => locale }));

beforeEach(() => {
  vi.resetModules();
  vi.stubGlobal("document", { documentElement: { lang: "zh-TW" }, querySelector: () => ({ content: "test-csrf" }) });
});
afterEach(() => { vi.unstubAllGlobals(); });

describe("shared Admin transport", () => {
  it("binds requests to their original cookie session and rejects scope changes", async () => {
    const fetch = vi.fn().mockResolvedValue(new Response('{"data":[],"total":0}'));
    vi.stubGlobal("fetch", fetch);
    const { api, bindSessionScope } = await import("./api");
    bindSessionScope("original-session-scope");
    await api("/admin/api/products?page=1");
    expect(fetch.mock.calls[0][1].headers.get("X-Prods-Session-Scope")).toBe("original-session-scope");
    expect(() => bindSessionScope("replacement-session-scope")).toThrow("Session changed");
    await expect(api("/admin/api/products?page=1")).rejects.toMatchObject({ status: 401 });
    expect(fetch).toHaveBeenCalledTimes(1);
  });
  it("sends cookie, CSRF and locale through one transport", async () => {
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ revision: 2 })));
    vi.stubGlobal("fetch", fetch);
    const { putJSON } = await import("./api");
    await expect(putJSON("/admin/api/products/opaque-id", { expected_revision: 1 })).resolves.toEqual({ revision: 2 });
    const [url, init] = fetch.mock.calls[0];
    expect(url).toBe("/admin/api/products/opaque-id");
    expect(init.credentials).toBe("same-origin");
    expect(init.cache).toBe("no-store");
    expect(init.headers.get("X-CSRF-Token")).toBe("test-csrf");
    expect(init.headers.get("Accept-Language")).toBe("zh-TW");
  });

  it.each([403, 409, 503])("preserves %s without logout or mutation retry", async (status) => {
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ code: "specific_code", error: "Readable error" }), { status }));
    vi.stubGlobal("fetch", fetch);
    const { putJSON, onSessionExpired } = await import("./api");
    const expired = vi.fn();
    onSessionExpired(expired);
    await expect(putJSON("/admin/api/products/opaque-id", {})).rejects.toMatchObject({ status, statusCode: status, code: "specific_code", message: "Readable error" });
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(expired).not.toHaveBeenCalled();
  });

  it("rejects late responses after session expiration and blocks further requests", async () => {
    let finish!: (value: unknown) => void;
    const fetch = vi.fn().mockResolvedValue({ ok: true, status: 200, json: () => new Promise((resolve) => { finish = resolve; }) });
    vi.stubGlobal("fetch", fetch);
    const { api, expireSession, onSessionExpired } = await import("./api");
    const expired = vi.fn();
    onSessionExpired(expired);
    const pending = api("/admin/api/products");
    await vi.waitFor(() => expect(finish).toBeTypeOf("function"));
    expireSession();
    finish([{ id: "old-account-private-record" }]);
    await expect(pending).rejects.toMatchObject({ status: 401 });
    await expect(api("/admin/api/products")).rejects.toMatchObject({ status: 401 });
    expect(expired).toHaveBeenCalledTimes(1);
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(fetch.mock.calls[0][1].signal.aborted).toBe(true);
  });

  it("expires a revoked session on 401", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response('{"code":"unauthorized"}', { status: 401 })));
    const { api, onSessionExpired } = await import("./api");
    const expired = vi.fn();
    onSessionExpired(expired);
    await expect(api("/admin/api/session")).rejects.toMatchObject({ status: 401 });
    expect(expired).toHaveBeenCalledTimes(1);
  });

  it("reports a lost write response as unknown, with no replay", async () => {
    const fetch = vi.fn().mockRejectedValue(new TypeError("network disconnected"));
    vi.stubGlobal("fetch", fetch);
    const { postJSON } = await import("./api");
    await expect(postJSON("/admin/api/products/opaque-id/archive", {})).rejects.toMatchObject({ code: "outcome_unknown" });
    expect(fetch).toHaveBeenCalledTimes(1);
  });
});

describe("Prods resource adapter", () => {
  it("creates Hidden products and leaves publication to its named command", async () => {
    const fetch = vi.fn().mockResolvedValue(new Response('{"id":"new-product","status":"hidden"}'));
    vi.stubGlobal("fetch", fetch);
    const { dataProvider } = await import("./dataProvider");
    await dataProvider.create({ resource: "products", variables: { part_number: "NEW", status: "published" } });
    expect(JSON.parse(fetch.mock.calls[0][1].body).status).toBe("hidden");
  });
  it("maps actual pages/total and archive filtering without changing stable IDs", async () => {
    const data = { data: [{ id: "opaque-A", part_number: "same" }, { id: "opaque-B", part_number: "same" }], total: 505 };
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify(data)));
    vi.stubGlobal("fetch", fetch);
    const { dataProvider } = await import("./dataProvider");
    await expect(dataProvider.getList({ resource: "products", pagination: { currentPage: 26, pageSize: 20 }, sorters: [{ field: "part_number", order: "asc" }], filters: [{ field: "include_archived", operator: "eq", value: true }] })).resolves.toEqual(data);
    expect(fetch.mock.calls[0][0]).toBe("/admin/api/products?page=26&page_size=20&sort=part_number&order=asc&include_archived=true");
  });

  it("maps allowlisted taxonomy arrays to Refine lists and requires a dictionary kind", async () => {
    const fetch = vi.fn()
      .mockResolvedValueOnce(new Response('[{"id":"cat-a"}]'))
      .mockResolvedValueOnce(new Response('[{"id":"dic-a"},{"id":"dic-b"}]'))
      .mockResolvedValueOnce(new Response('[{"id":"spec-a"}]'))
      .mockResolvedValueOnce(new Response('[{"id":"set-a"}]'));
    vi.stubGlobal("fetch", fetch);
    const { dataProvider } = await import("./dataProvider");
    const flat = { mode: "off" as const };
    await expect(dataProvider.getList({ resource: "categories", pagination: flat })).resolves.toEqual({ data: [{ id: "cat-a" }], total: 1 });
    await expect(dataProvider.getList({ resource: "dictionary-entries", pagination: flat, meta: { kind: "manufacturer" } })).resolves.toEqual({ data: [{ id: "dic-a" }, { id: "dic-b" }], total: 2 });
    await expect(dataProvider.getList({ resource: "spec-definitions", pagination: flat })).resolves.toEqual({ data: [{ id: "spec-a" }], total: 1 });
    await expect(dataProvider.getList({ resource: "spec-sets", pagination: flat })).resolves.toEqual({ data: [{ id: "set-a" }], total: 1 });
    expect(fetch.mock.calls.map(([url]) => url)).toEqual([
      "/admin/api/categories",
      "/admin/api/dictionaries?kind=manufacturer",
      "/admin/api/specs",
      "/admin/api/spec-sets",
    ]);
  });

  it("maps roles and users into Refine lists without caching command receipts", async () => {
    const fetch = vi.fn()
      .mockResolvedValueOnce(new Response('[{"id":"role-owner","name":"Owner"}]'))
      .mockResolvedValueOnce(new Response('[{"id":"user-a","email":"owner@example.test"}]'));
    vi.stubGlobal("fetch", fetch);
    const { dataProvider } = await import("./dataProvider");
    const flat = { mode: "off" as const };
    await expect(dataProvider.getList({ resource: "roles", pagination: flat })).resolves.toEqual({ data: [{ id: "role-owner", name: "Owner" }], total: 1 });
    await expect(dataProvider.getList({ resource: "users", pagination: flat })).resolves.toEqual({ data: [{ id: "user-a", email: "owner@example.test" }], total: 1 });
    expect(fetch.mock.calls.map(([url]) => url)).toEqual(["/admin/api/roles", "/admin/api/users"]);
  });

  it("rejects missing dictionary kinds and unsupported flat-list query features before HTTP", async () => {
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    const { dataProvider } = await import("./dataProvider");
    const flat = { mode: "off" as const };
    await expect(dataProvider.getList({ resource: "dictionary-entries", pagination: flat })).rejects.toMatchObject({ status: 400 });
    await expect(dataProvider.getList({ resource: "dictionary-entries", pagination: flat, meta: { kind: "unknown" } })).rejects.toMatchObject({ status: 400 });
    await expect(dataProvider.getList({ resource: "categories", filters: [{ field: "status", operator: "eq", value: "active" }], pagination: flat })).rejects.toMatchObject({ status: 400 });
    await expect(dataProvider.getList({ resource: "categories", sorters: [{ field: "name", order: "asc" }], pagination: flat })).rejects.toMatchObject({ status: 400 });
    await expect(dataProvider.getList({ resource: "categories", pagination: { currentPage: 1, pageSize: 10 } })).rejects.toMatchObject({ status: 400 });
    expect(fetch).not.toHaveBeenCalled();
  });

  it("does not convert failed list queries to an empty successful list", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response('{"error":"Unavailable"}', { status: 503 })));
    const { dataProvider } = await import("./dataProvider");
    await expect(dataProvider.getList({ resource: "products" })).rejects.toMatchObject({ status: 503 });
  });

  it("does not convert a failed taxonomy query to an empty successful list", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response('{"error":"Unavailable"}', { status: 503 })));
    const { dataProvider } = await import("./dataProvider");
    await expect(dataProvider.getList({ resource: "categories", pagination: { mode: "off" } })).rejects.toMatchObject({ status: 503 });
  });

  it("rejects unsupported operations and missing revision without HTTP writes", async () => {
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    const { dataProvider } = await import("./dataProvider");
    await expect(dataProvider.deleteOne({ resource: "products", id: "p" })).rejects.toMatchObject({ status: 405 });
    await expect(dataProvider.update({ resource: "products", id: "p", variables: { product: {} } })).rejects.toMatchObject({ status: 400 });
    await expect(dataProvider.getList({ resource: "widgets" })).rejects.toMatchObject({ status: 400 });
    await expect(dataProvider.getList({ resource: "products", pagination: { mode: "off" } })).rejects.toMatchObject({ status: 400 });
    await expect(dataProvider.getList({ resource: "products", sorters: [{ field: "unsupported", order: "asc" }] })).rejects.toMatchObject({ status: 400 });
    expect(fetch).not.toHaveBeenCalled();
  });

  it("keeps the caller revision and dirty payload on conflict", async () => {
    const fetch = vi.fn().mockResolvedValue(new Response('{"code":"revision_conflict"}', { status: 409 }));
    vi.stubGlobal("fetch", fetch);
    const { dataProvider } = await import("./dataProvider");
    const variables = { expected_revision: 4, product: { name: "Unsaved input" } };
    await expect(dataProvider.update({ resource: "products", id: "p", variables })).rejects.toMatchObject({ status: 409 });
    expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual(variables);
    expect(variables.product.name).toBe("Unsaved input");
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it("keeps taxonomy preview inputs available when apply conflicts and never retries", async () => {
    const fetch = vi.fn()
      .mockResolvedValueOnce(new Response('{"affected_products":[]}'))
      .mockResolvedValueOnce(new Response('{"code":"revision_conflict"}', { status: 409 }));
    vi.stubGlobal("fetch", fetch);
    const { taxonomyCommands } = await import("./taxonomyCommands");
    const category = { id: "cat-a", name: "Original", slug: "original", parent_id: "", status: "active", revision: 7 } as const;
    const dirty = { name: "Unsaved name", slug: "unsaved", parent_id: "cat-parent" };
    await expect(taxonomyCommands.previewCategory(category, dirty)).resolves.toEqual({ affected_products: [] });
    await expect(taxonomyCommands.updateCategory(category, dirty)).rejects.toMatchObject({ status: 409 });
    expect(dirty).toEqual({ name: "Unsaved name", slug: "unsaved", parent_id: "cat-parent" });
    expect(JSON.parse(fetch.mock.calls[1][1].body)).toEqual({ expected_revision: 7, ...dirty });
    expect(fetch).toHaveBeenCalledTimes(2);
  });

  it("declares domain-specific invalidation targets", async () => {
    const { taxonomyInvalidations } = await import("./taxonomyCommands");
    expect(taxonomyInvalidations.category).toEqual(["categories", "products"]);
    expect(taxonomyInvalidations.dictionary).toEqual(["dictionary-entries", "products"]);
    expect(taxonomyInvalidations.specifications).toEqual(["spec-definitions", "spec-sets", "categories", "products"]);
  });

  it("keeps access lifecycle operations as non-retrying commands", async () => {
    const fetch = vi.fn()
      .mockResolvedValueOnce(new Response('{"id":"user-a","email":"owner@example.test","role_id":"role-editor"}'))
      .mockResolvedValueOnce(new Response('{"code":"last_owner"}', { status: 409 }));
    vi.stubGlobal("fetch", fetch);
    const { accessCommands } = await import("./accessCommands");
    const user = { id: "user-a", email: "owner@example.test", role_id: "role-owner" } as never;
    await expect(accessCommands.changeRole(user, "role-editor")).resolves.toMatchObject({ role_id: "role-editor" });
    await expect(accessCommands.disableUser(user)).rejects.toMatchObject({ status: 409, code: "last_owner" });
    expect(fetch.mock.calls[0][0]).toBe("/admin/api/users/user-a/role");
    expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual({ role_id: "role-editor" });
    expect(fetch).toHaveBeenCalledTimes(2);
  });

  it("keeps one-time set-password bearer data outside list resources", async () => {
    const grant = { user: { id: "user-a", email: "new@example.test" }, set_password_url: "/admin/set-password?token=secret", expires_at: "2030-01-01T00:00:00Z" };
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify(grant)));
    vi.stubGlobal("fetch", fetch);
    const { accessCommands } = await import("./accessCommands");
    await expect(accessCommands.createUser({ email: "new@example.test", display_name: "New", role_id: "role-editor" })).resolves.toEqual(grant);
    expect(fetch.mock.calls[0][0]).toBe("/admin/api/users");
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it("declares access invalidation targets", async () => {
    const { accessInvalidations } = await import("./accessCommands");
    expect(accessInvalidations.roles).toEqual(["roles"]);
    expect(accessInvalidations.users).toEqual(["users"]);
  });

  it("fails closed for unknown UI permissions", async () => {
    const { allowed } = await import("./resources");
    expect(allowed({}, "products", "edit")).toBe(false);
    expect(allowed({ "catalog.edit": true }, "products", "delete")).toBe(false);
    expect(allowed({ "catalog.view": true }, "products", "list")).toBe(true);
    expect(allowed({ "catalog.edit": true }, "products", "archive")).toBe(false);
    expect(allowed({ "system.manage": true }, "health", "list")).toBe(true);
    expect(allowed({ "catalog.view": true }, "categories", "list")).toBe(true);
    expect(allowed({ "catalog.view": true }, "categories", "edit")).toBe(false);
    expect(allowed({ "catalog.edit": true }, "categories", "edit")).toBe(true);
    expect(allowed({ "catalog.edit": true }, "categories", "delete")).toBe(false);
    expect(allowed({ "users.manage": true }, "users", "list")).toBe(true);
    expect(allowed({ "users.manage": true }, "roles", "edit")).toBe(true);
    expect(allowed({ "users.manage": true }, "users", "delete")).toBe(false);
    expect(allowed({ "catalog.edit": true }, "users", "list")).toBe(false);
  });
});
