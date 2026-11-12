// Build da UI: compila o SCSS e copia HTML/JS para dist/ (servido pelo nginx do gateway).
//   npm run build          → build único (usado no Dockerfile do gateway)
//   npm run watch          → recompila a cada alteração em src/
import { compile } from "sass";
import { cpSync, mkdirSync, rmSync, watch, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = dirname(fileURLToPath(import.meta.url));
const src = join(root, "src");
const dist = join(root, "dist");

function build() {
  const started = Date.now();
  rmSync(dist, { recursive: true, force: true });
  mkdirSync(join(dist, "assets"), { recursive: true });

  const css = compile(join(src, "styles", "main.scss"), { style: "compressed" });
  writeFileSync(join(dist, "assets", "app.css"), css.css);

  cpSync(join(src, "js"), join(dist, "assets", "js"), { recursive: true });
  cpSync(join(src, "index.html"), join(dist, "index.html"));
  cpSync(join(src, "favicon.svg"), join(dist, "favicon.svg"));

  console.log(`web: build concluído em ${Date.now() - started}ms → dist/`);
}

build();

if (process.argv.includes("--watch")) {
  console.log("web: observando src/ ...");
  let timer;
  watch(src, { recursive: true }, () => {
    clearTimeout(timer);
    timer = setTimeout(() => {
      try {
        build();
      } catch (err) {
        console.error(err.message);
      }
    }, 100);
  });
}
