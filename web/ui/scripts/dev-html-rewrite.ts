/** HTML / Location / Origin rewrites for the Vite-fronted local HMR proxy. */

export const GO_ORIGIN = "http://127.0.0.1:3310";
export const DEV_ORIGIN = "http://127.0.0.1:5173";

/** External only — Go CSP allows script-src 'self', not unsafe-inline. */
const HMR_SCRIPTS = [
  `<script type="module" src="/src/dev/hmr-preamble.js"></script>`,
  `<script type="module" src="/@vite/client"></script>`,
].join("");

/** Rewrite Go-served HTML so Admin / Public load Vite source entries with HMR. */
export function rewriteDevHtml(html: string): string {
  let out = html;
  const before = out;

  out = out.replace(/<link\s+rel="stylesheet"\s+href="\/static\/admin\/style\.css"\s*>/gi, "");
  out = out.replace(
    /(<script\s+type="module"\s+src=")\/static\/admin\/admin\.js("><\/script>)/gi,
    "$1/src/admin/main.tsx$2",
  );
  out = out.replace(
    /(<script\s+type="module"\s+src=")\/static\/public\/public-islands\.js("><\/script>)/gi,
    "$1/src/public/main.tsx$2",
  );
  out = out.replace(
    /(<link\s+rel="stylesheet"\s+href=")\/static\/public\/public\.css("\s*>)/gi,
    "$1/src/public/style.css$2",
  );

  if (out === before) {
    return out;
  }

  if (!out.includes("/@vite/client")) {
    if (/<\/head>/i.test(out)) {
      out = out.replace(/<\/head>/i, `${HMR_SCRIPTS}</head>`);
    } else if (/<body[^>]*>/i.test(out)) {
      out = out.replace(/<body([^>]*)>/i, `<body$1>${HMR_SCRIPTS}`);
    } else {
      out = `${HMR_SCRIPTS}${out}`;
    }
  }

  return out;
}

/** Keep redirects on the Vite origin during HMR sessions. */
export function rewriteLocationHeader(location: string | undefined): string | undefined {
  if (!location) return location;
  return location
    .replaceAll(GO_ORIGIN, DEV_ORIGIN)
    .replaceAll("http://localhost:3310", DEV_ORIGIN)
    .replaceAll("http://[::1]:3310", DEV_ORIGIN);
}

/**
 * Map browser Vite Origin/Referer to the Go base_url origin so EnforceHost
 * accepts state-changing requests (e.g. POST /admin/login).
 */
export function rewriteUpstreamBrowserOrigin(
  headerValue: string | undefined,
  goOrigin: string = GO_ORIGIN,
): string | undefined {
  if (!headerValue) return headerValue;
  try {
    const parsed = new URL(headerValue);
    if (parsed.port !== "5173") return headerValue;
    const host = parsed.hostname.toLowerCase();
    if (host !== "127.0.0.1" && host !== "localhost" && host !== "::1") {
      return headerValue;
    }
    const go = new URL(goOrigin);
    parsed.protocol = go.protocol;
    parsed.host = go.host;
    // Origin has no path; Referer keeps path/query.
    if (parsed.pathname === "/" && parsed.search === "" && parsed.hash === "") {
      return goOrigin;
    }
    return parsed.toString();
  } catch {
    return headerValue;
  }
}
