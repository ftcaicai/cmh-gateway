// Load real TypeScript modules, including read-only official exports, in Node tests.
import { registerHooks } from "node:module";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import ts from "typescript";

const appRoot = new URL("../", import.meta.url);
registerHooks({
  resolve(specifier, context, nextResolve) {
    if (specifier.startsWith("@/")) {
      return nextResolve(new URL(`../web/src/${specifier.slice(2)}.ts`, appRoot).href, context);
    }
    if (specifier.startsWith("~/")) {
      return nextResolve(new URL(`src/${specifier.slice(2)}.ts`, appRoot).href, context);
    }
    if (
      context.parentURL?.startsWith(new URL("../web/src/", appRoot).href) &&
      !specifier.startsWith(".") &&
      !specifier.startsWith("node:") &&
      !specifier.includes(":")
    ) {
      return nextResolve(specifier, {
        ...context,
        parentURL: new URL("package.json", appRoot).href,
      });
    }
    return nextResolve(specifier, context);
  },
  load(url, context, nextLoad) {
    if (!url.endsWith(".ts")) return nextLoad(url, context);
    const source = readFileSync(fileURLToPath(url), "utf8");
    return {
      format: "module",
      source: ts.transpileModule(source, {
        compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext },
      }).outputText,
      shortCircuit: true,
    };
  },
});
