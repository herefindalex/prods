// These capabilities mirror the existing server use cases. Unknown actions
// deny by default; presentation never grants authority to the backend.
export const pageCapabilities: Record<string, string[]> = {
  catalog: ["catalog.view"], taxonomy: ["catalog.view"], imports: ["catalog.import"],
  "product-bulk": ["catalog.edit"], jobs: ["admin.access"], website: ["system.manage"],
  "listing-profiles": ["system.manage"], "public-copy": ["system.manage"], access: ["users.manage"],
  activity: ["rfq.view", "audit.view"], settings: ["system.manage"], health: ["system.manage"],
  backups: ["system.manage"], traffic: ["system.manage"],
};

export const resources = Object.keys(pageCapabilities).map((page) => ({ name: page === "catalog" ? "products" : page, list: `/${page}` }));

export function allowed(capabilities: Record<string, boolean>, resource: string, action: string): boolean {
  if (["categories", "dictionary-entries", "spec-definitions", "spec-sets"].includes(resource)) {
    const capability = action === "list" || action === "show" ? "catalog.view" : action === "create" || action === "edit" ? "catalog.edit" : "";
    return Boolean(capability && capabilities[capability]);
  }
  if (["roles", "users"].includes(resource)) {
    return ["list", "show", "create", "edit"].includes(action) && Boolean(capabilities["users.manage"]);
  }
  if (resource === "products") {
    if (action === "publish") return Boolean(capabilities["catalog.edit"] && capabilities["catalog.publish"]);
    const capability = ({ list: "catalog.view", show: "catalog.view", create: "catalog.edit", edit: "catalog.edit", clone: "catalog.edit", preview: "catalog.edit", publish: "catalog.publish", hide: "catalog.publish", archive: "catalog.publish", export: "catalog.export" } as Record<string, string>)[action];
    return Boolean(capability && capabilities[capability]);
  }
  return action === "list" && Boolean(pageCapabilities[resource]?.every((capability) => capabilities[capability]));
}
