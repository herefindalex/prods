import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { Refine, type AuthProvider } from "@refinedev/core";
import routerProvider from "@refinedev/react-router";
import { QueryClient } from "@tanstack/react-query";
import { BrowserRouter } from "react-router";
import { Alert, Button, Space } from "antd";
import { api, APIError, bindSessionScope, expireSession, onSessionExpired } from "./api";
import { dataProvider } from "./dataProvider";
import { allowed, resources } from "./resources";

type AdminIdentity = { id: string; cache_scope: string; capabilities: Record<string, boolean> };
const IdentityContext = createContext<AdminIdentity | undefined>(undefined);
export function useAdminIdentity(): AdminIdentity {
  const identity = useContext(IdentityContext);
  if (!identity) throw new Error("Admin identity is unavailable");
  return identity;
}

export const authProvider: AuthProvider = {
  // Login remains the existing server form POST; credentials are never cached.
  login: async () => { window.location.assign("/admin/login"); return { success: true }; },
  logout: async () => {
    await api<void>("/admin/logout", { method: "POST" });
    expireSession();
    window.location.assign(`/admin/login?lang=${encodeURIComponent(document.documentElement.lang)}`);
    return { success: true };
  },
  check: async () => {
    try { await api<AdminIdentity>("/admin/api/session"); return { authenticated: true }; }
    catch (error) { return { authenticated: false, error: error as Error }; }
  },
  getIdentity: () => api<AdminIdentity>("/admin/api/session"),
  onError: async (error) => { if (error instanceof APIError && error.status === 401) expireSession(); return { error }; },
};

export function AdminProviders({ children }: { children: ReactNode }) {
  const [client] = useState(() => new QueryClient({ defaultOptions: {
    queries: { retry: false, refetchOnWindowFocus: false }, mutations: { retry: false },
  } }));
  const [identity, setIdentity] = useState<AdminIdentity>();
  const [error, setError] = useState<Error>();
  const [expired, setExpired] = useState(false);
  const [attempt, setAttempt] = useState(0);
  useEffect(() => onSessionExpired(() => {
    setExpired(true);
    setIdentity(undefined);
    void client.cancelQueries();
    client.clear();
  }), [client]);
  useEffect(() => {
    const controller = new AbortController();
    setError(undefined);
    void api<AdminIdentity>("/admin/api/session", { signal: controller.signal }).then((identity) => {
      bindSessionScope(identity.cache_scope);
      setIdentity(identity);
    }).catch((error: Error) => {
      if (!controller.signal.aborted) setError(error);
    });
    return () => controller.abort();
  }, [attempt]);
  useEffect(() => {
    if (!identity || expired) return;
    const check = () => { void api<AdminIdentity>("/admin/api/session").then((next) => {
      if (next.id !== identity.id || next.cache_scope !== identity.cache_scope) { expireSession(); return; }
      setIdentity(next);
    }).catch(() => { /* Transport handles 401; outages are not logout. */ }); };
    window.addEventListener("focus", check);
    return () => window.removeEventListener("focus", check);
  }, [identity, expired]);
  if (expired) return <Alert type="warning" title="Session ended" description={<a href="/admin/login">Sign in again</a>} />;
  if (error) return <Space direction="vertical"><Alert type="error" title={error.message} /><Button onClick={() => setAttempt((value) => value + 1)}>Retry</Button></Space>;
  if (!identity) return <p role="status">Loading Prods…</p>;
  return <IdentityContext.Provider value={identity}>
    <BrowserRouter basename="/admin">
      <Refine dataProvider={dataProvider} authProvider={authProvider} routerProvider={routerProvider} resources={resources}
        accessControlProvider={{ can: async ({ resource, action }) => ({ can: allowed(identity.capabilities, resource ?? "", action) }) }}
        options={{ disableTelemetry: true, mutationMode: "pessimistic", reactQuery: { clientConfig: client } }}>
        {children}
      </Refine>
    </BrowserRouter>
  </IdentityContext.Provider>;
}
