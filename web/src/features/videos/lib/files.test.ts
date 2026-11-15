import { describe, expect, it } from "vitest";
import { extensionOf, validateVideoFile, zipFileName } from "./files";

describe("files", () => {
  it("valida extensão, tamanho e arquivo vazio", () => {
    expect(extensionOf("Aula.MP4")).toBe(".mp4");
    expect(extensionOf("sem-extensao")).toBe("");
    expect(validateVideoFile({ name: "aula.mkv", size: 10 })).toBeNull();
    expect(validateVideoFile({ name: "doc.pdf", size: 10 })).toBe("formato não suportado");
    expect(validateVideoFile({ name: "a.mp4", size: 0 })).toBe("arquivo vazio");
    expect(validateVideoFile({ name: "a.mp4", size: 2 * 1024 ** 3 })).toBe("maior que 1 GB");
  });

  it("gera o nome do zip", () => {
    expect(zipFileName("aula.mp4")).toBe("aula_frames.zip");
    expect(zipFileName(".mp4")).toBe("video_frames.zip");
  });
});
