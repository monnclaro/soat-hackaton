import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "@/shared/toast/ToastProvider";
import { FakeXHR, installFakeXHR } from "@/test/fakes";
import { UploadPanel } from "./UploadPanel";

function setup() {
  const onUploaded = vi.fn();
  const onUnauthorized = vi.fn();
  render(
    <ToastProvider>
      <UploadPanel token="tok-123" onUploaded={onUploaded} onUnauthorized={onUnauthorized} />
    </ToastProvider>,
  );
  return { onUploaded, onUnauthorized, input: screen.getByLabelText("Selecionar vídeos") };
}

const file = (name: string, content = "x") => new File([content], name, { type: "video/mp4" });

describe("UploadPanel", () => {
  beforeEach(installFakeXHR);

  it("envia vários vídeos em paralelo com o token", async () => {
    const { onUploaded, input } = setup();
    await userEvent.upload(input, [file("a.mp4"), file("b.mkv")]);

    expect(await screen.findAllByText("Enviado ✓ (na fila de processamento)")).toHaveLength(2);
    expect(FakeXHR.instances).toHaveLength(2);
    expect(FakeXHR.instances[0]?.url).toBe("/api/videos");
    expect(FakeXHR.instances[0]?.headers.Authorization).toBe("Bearer tok-123");
    expect(onUploaded).toHaveBeenCalledTimes(2);
    expect(screen.getByText("2 vídeo(s) enviados para processamento.")).toBeInTheDocument();
  });

  it("recusa arquivos inválidos sem enviar", async () => {
    const { input } = setup();
    // applyAccept: false simula arquivos que passariam pelo seletor (ex.: arrastados).
    await userEvent.upload(input, [file("doc.pdf"), file("vazio.mp4", "")], { applyAccept: false });

    expect(await screen.findByText("Recusado: formato não suportado")).toBeInTheDocument();
    expect(screen.getByText("Recusado: arquivo vazio")).toBeInTheDocument();
    expect(FakeXHR.instances).toHaveLength(0);
    expect(screen.getByText("2 arquivo(s) não foram enviados.")).toBeInTheDocument();
  });

  it("mostra o erro do servidor e trata sessão expirada", async () => {
    FakeXHR.respondWith = { status: 401, body: { error: "token inválido ou expirado" } };
    const { input, onUnauthorized, onUploaded } = setup();
    await userEvent.upload(input, [file("a.mp4")]);

    expect(await screen.findByText("Erro: token inválido ou expirado")).toBeInTheDocument();
    expect(onUnauthorized).toHaveBeenCalled();
    expect(onUploaded).not.toHaveBeenCalled();
  });
});
