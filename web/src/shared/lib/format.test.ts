import { describe, expect, it } from "vitest";
import { formatBytes, initials, timeAgo } from "./format";

describe("format", () => {
  it("formata bytes", () => {
    expect(formatBytes(0)).toBe("0 B");
    expect(formatBytes(512)).toBe("512 B");
    expect(formatBytes(1536)).toBe("1.5 KB");
    expect(formatBytes(5 * 1024 ** 3)).toBe("5.0 GB");
  });

  it("gera tempo relativo", () => {
    const now = Date.parse("2026-11-14T12:00:00Z");
    expect(timeAgo("2026-11-14T11:59:30Z", now)).toMatch(/30 segundos/);
    expect(timeAgo("2026-11-14T11:55:00Z", now)).toMatch(/5 minutos/);
    expect(timeAgo("2026-11-14T09:00:00Z", now)).toMatch(/3 horas/);
    expect(timeAgo("2026-11-13T12:00:00Z", now)).toBe("ontem");
    expect(timeAgo("2026-01-01T12:00:00Z", now)).toMatch(/2026/);
  });

  it("gera iniciais", () => {
    expect(initials("Ana Souza Lima")).toBe("AS");
    expect(initials("  bia ")).toBe("B");
    expect(initials()).toBe("");
  });
});
