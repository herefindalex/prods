import react from "@vitejs/plugin-react";
import type { IncomingMessage, ServerResponse } from "node:http";
import type { Socket } from "node:net";
import { defineConfig, type Plugin, type ProxyOptions } from "vite";
import {
  DEV_ORIGIN,
  GO_ORIGIN,
  rewriteDevHtml,
  rewriteLocationHeader,
  rewriteUpstreamBrowserOrigin,
} from "./scripts/dev-html-rewrite";

const goTarget = process.env.PRODS_DEV_PROXY_TARGET ?? GO_ORIGIN;

function collectProxyBody(proxyRes: IncomingMessage): Promise<Buffer> {
  return new Promise((resolve, reject) => {
    const chunks: Buffer[] = [];
    proxyRes.on("data", (chunk: Buffer | string) => {
      chunks.push(typeof chunk === "string" ? Buffer.from(chunk) : chunk);
    });
    proxyRes.on("end", () => resolve(Buffer.concat(chunks)));
    proxyRes.on("error", reject);
  });
}

function writeProxiedResponse(
  proxyRes: IncomingMessage,
  res: ServerResponse,
  body: Buffer | undefined,
  headers: Record<string, string | string[] | undefined>,
): void {
  const status = proxyRes.statusCode ?? 200;
  for (const [key, value] of Object.entries(headers)) {
    if (value === undefined) continue;
    if (key.toLowerCase() === "transfer-encoding") continue;
    res.setHeader(key, value);
  }
  if (body) {
    res.setHeader("content-length", Buffer.byteLength(body));
    res.writeHead(status);
    res.end(body);
    return;
  }
  res.writeHead(status);
  proxyRes.pipe(res);
}

function createGoProxy(): Record<string, string | ProxyOptions> {
  const options: ProxyOptions = {
    target: goTarget,
    changeOrigin: true,
    // Keep WebSockets on Vite for HMR; Prods does not need a WS proxy here.
    ws: false,
    selfHandleResponse: true,
    configure(proxy) {
      proxy.on("proxyReq", (proxyReq, req) => {
        const origin = rewriteUpstreamBrowserOrigin(
          typeof req.headers.origin === "string" ? req.headers.origin : undefined,
          goTarget,
        );
        if (origin) proxyReq.setHeader("Origin", origin);
        const referer = rewriteUpstreamBrowserOrigin(
          typeof req.headers.referer === "string" ? req.headers.referer : undefined,
          goTarget,
        );
        if (referer) proxyReq.setHeader("Referer", referer);
      });

      proxy.on("proxyRes", (proxyRes, _req, res) => {
        const headers: Record<string, string | string[] | undefined> = {
          ...proxyRes.headers,
        };
        const location = headers.location;
        if (typeof location === "string") {
          headers.location = rewriteLocationHeader(location);
        } else if (Array.isArray(location)) {
          headers.location = location.map((value) => rewriteLocationHeader(value) ?? value);
        }

        const contentType = String(headers["content-type"] ?? "");
        const encoding = String(headers["content-encoding"] ?? "");
        const canRewriteHtml =
          contentType.includes("text/html") &&
          (!encoding || encoding === "identity");

        if (!canRewriteHtml) {
          writeProxiedResponse(proxyRes, res as ServerResponse, undefined, headers);
          return;
        }

        void collectProxyBody(proxyRes)
          .then((raw) => {
            delete headers["content-encoding"];
            delete headers["content-length"];
            // Local HMR only: drop Go CSP so Vite client / WS are not blocked.
            delete headers["content-security-policy"];
            const rewritten = rewriteDevHtml(raw.toString("utf8"));
            writeProxiedResponse(
              proxyRes,
              res as ServerResponse,
              Buffer.from(rewritten, "utf8"),
              headers,
            );
          })
          .catch((error: unknown) => {
            const message = error instanceof Error ? error.message : String(error);
            const response = res as ServerResponse;
            if (!response.headersSent) {
              response.writeHead(502, { "content-type": "text/plain; charset=utf-8" });
            }
            response.end(`prods HMR proxy failed to read Go HTML: ${message}`);
          });
      });

      proxy.on("error", (error, _req, res) => {
        const message = error instanceof Error ? error.message : String(error);
        if (
          res &&
          "writeHead" in (res as ServerResponse | Socket) &&
          typeof (res as ServerResponse).writeHead === "function"
        ) {
          const response = res as ServerResponse;
          if (!response.headersSent) {
            response.writeHead(502, { "content-type": "text/plain; charset=utf-8" });
          }
          response.end(
            `prods HMR proxy cannot reach ${goTarget}. Start ./prods first.\n${message}`,
          );
        }
      });
    },
  };

  return {
    "^/(?!src/|@vite/|@id/|@react-refresh|node_modules/|@fs/).*": options,
  };
}

function prodsDevBanner(): Plugin {
  return {
    name: "prods-dev-banner",
    configureServer(server) {
      server.httpServer?.once("listening", () => {
        console.log(`[prods-dev] browse ${DEV_ORIGIN}/ and ${DEV_ORIGIN}/admin (Go target ${goTarget})`);
      });
    },
  };
}

export default defineConfig({
  plugins: [react(), prodsDevBanner()],
  server: {
    host: "127.0.0.1",
    port: 5173,
    strictPort: true,
    proxy: createGoProxy(),
  },
  clearScreen: false,
});
