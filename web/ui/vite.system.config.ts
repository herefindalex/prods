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
    outDir: "../../internal/webapp/static/system",
    emptyOutDir: true,
    cssCodeSplit: false,
    lib: {
      entry: resolve(here, "src/system/main.tsx"),
      formats: ["es"],
      fileName: () => "system.js",
      cssFileName: "system",
    },
    rollupOptions: { output: { inlineDynamicImports: true } },
  },
});
