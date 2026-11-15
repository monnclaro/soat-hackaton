// Garante a organização em camadas do front-end:
//   app/      → pode usar features e shared
//   features/ → pode usar shared e a API pública (index.ts) de outras features
//   shared/   → não depende de features nem de app
import { describe, expect, it } from "vitest";

// Lê o código-fonte de todos os módulos (recurso do Vite, sem depender do Node).
const sources = import.meta.glob<string>(["/src/**/*.{ts,tsx}", "!/src/**/*.test.{ts,tsx}", "!/src/test/**"], {
  query: "?raw",
  import: "default",
  eager: true,
});

const IMPORT_RE = /^\s*import\s[^"']*["']([^"']+)["']/gm;

/** Resolve um import para um caminho a partir de /src (ex.: "@/shared/x" → "/src/shared/x"). */
function resolve(fromFile: string, spec: string): string | null {
  if (spec.startsWith("@/")) return `/src/${spec.slice(2)}`;
  if (!spec.startsWith(".")) return null; // pacote npm
  const parts = fromFile.split("/").slice(0, -1);
  for (const seg of spec.split("/")) {
    if (seg === "..") parts.pop();
    else if (seg !== ".") parts.push(seg);
  }
  return parts.join("/");
}

const featureOf = (path: string) => /^\/src\/features\/([^/]+)/.exec(path)?.[1];

function violations(): string[] {
  const out: string[] = [];
  for (const [file, code] of Object.entries(sources)) {
    for (const [, spec] of code.matchAll(IMPORT_RE)) {
      const target = spec && resolve(file, spec);
      if (!target) continue;
      const from = featureOf(file);
      const to = featureOf(target);

      if (file.startsWith("/src/shared/") && (target.startsWith("/src/features/") || target.startsWith("/src/app/"))) {
        out.push(`${file} → ${spec}: shared/ não pode depender de features/ ou app/`);
      }
      if (from && target.startsWith("/src/app/")) {
        out.push(`${file} → ${spec}: features/ não pode depender de app/`);
      }
      if (to && to !== from && target !== `/src/features/${to}` && target !== `/src/features/${to}/index`) {
        out.push(`${file} → ${spec}: use a API pública "@/features/${to}" em vez de arquivos internos`);
      }
    }
  }
  return out;
}

describe("arquitetura do front-end", () => {
  it("encontra os módulos para verificar", () => {
    expect(Object.keys(sources).length).toBeGreaterThan(10);
  });

  it("respeita as dependências entre app/, features/ e shared/", () => {
    expect(violations()).toEqual([]);
  });

  it("detecta violações (autoteste das regras)", () => {
    expect(resolve("/src/features/videos/components/A.tsx", "../api")).toBe("/src/features/videos/api");
    expect(resolve("/src/features/videos/components/A.tsx", "@/features/auth/session")).toBe("/src/features/auth/session");
    expect(resolve("/src/app/App.tsx", "react")).toBeNull();
    expect(featureOf("/src/features/auth/components/X.tsx")).toBe("auth");
    expect(featureOf("/src/shared/lib/format.ts")).toBeUndefined();
  });
});
