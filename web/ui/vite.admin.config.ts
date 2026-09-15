import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = fileURLToPath(new URL(".", import.meta.url));

export default defineConfig({
  plugins: [react()],
  define: { "process.env.NODE_ENV": JSON.stringify("production") },
  build: {
    minify: "esbuild",
    outDir: "../../internal/webapp/static/admin",
    emptyOutDir: true,
    cssCodeSplit: false,
    lib: {
      entry: resolve(here, "src/admin/main.tsx"),
      formats: ["es"],
      fileName: () => "admin.js",
      cssFileName: "style",
    },
    rollupOptions: { output: { inlineDynamicImports: true } },
  },
});
