import type { Plugin } from "vite";

// Fail the build on transitive imports too, including shared barrels that
// accidentally pull Admin orchestration into a public or recovery entry point.
export function bundleBoundary(surface: "public" | "system"): Plugin {
  return {
    name: `prods-${surface}-boundary`,
    generateBundle() {
      for (const id of this.getModuleIds()) {
        if (id.includes("/src/admin/") || id.includes("/node_modules/@refinedev/") || (surface === "public" && id.includes("/node_modules/antd/"))) {
          this.error(`${surface} bundle cannot import ${id}`);
        }
      }
    },
  };
}
