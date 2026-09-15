import { mkdir, readFile, rename, writeFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const outputs = [
  ["src/public/style.css", "../../internal/webapp/static/public/public.css"],
];

for (const [sourceName, outputName] of outputs) {
  const source = await readFile(resolve(root, sourceName));
  const output = resolve(root, outputName);
  const temporary = `${output}.tmp`;
  await mkdir(dirname(output), { recursive: true });
  await writeFile(temporary, source, { mode: 0o644 });
  await rename(temporary, output);
}
