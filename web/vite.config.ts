/// <reference types="vitest/config" />
import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Em desenvolvimento (npm run dev), /auth e /api são encaminhados ao gateway.
// Use API_URL para apontar para outro endereço (ex.: http://localhost:8082 sem gateway).
const apiTarget = process.env.API_URL ?? "http://localhost:8080";

export default defineConfig({
  plugins: [react()],
  resolve: {
    // "@/..." aponta para src/ (o mesmo alias está em tsconfig.json).
    alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) },
  },
  server: {
    port: 5173,
    proxy: {
      "/auth": apiTarget,
      "/api": apiTarget,
    },
  },
  build: {
    outDir: "dist",
    sourcemap: false,
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    css: false,
    restoreMocks: true,
  },
});
