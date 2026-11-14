import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ToastProvider } from "../toast/ToastContext";
import { mockFetch, video } from "../test/fakes";
import { VideoList } from "./VideoList";

function renderList(videos = [video()], extra: Partial<Parameters<typeof VideoList>[0]> = {}) {
  const props = { token: "tok", videos, loaded: true, active: false, onRefresh: vi.fn(), onUnauthorized: vi.fn(), ...extra };
  render(
    <ToastProvider>
      <VideoList {...props} />
    </ToastProvider>,
  );
  return props;
}

describe("VideoList", () => {
  it("mostra carregando e lista vazia", () => {
    const { unmount } = render(
      <ToastProvider>
        <VideoList token="t" videos={[]} loaded={false} active={false} onRefresh={vi.fn()} onUnauthorized={vi.fn()} />
      </ToastProvider>,
    );
    expect(screen.getByText("Carregando...")).toBeInTheDocument();
    unmount();
    renderList([]);
    expect(screen.getByText(/Nenhum vídeo enviado ainda/)).toBeInTheDocument();
  });

  it("mostra o status e só oferece download quando concluído", () => {
    renderList([
      video({ id: "a", original_name: "fila.mp4", status: "PENDING" }),
      video({ id: "b", original_name: "pronto.mp4", status: "COMPLETED", download_url: "/api/videos/b/download" }),
    ]);
    expect(screen.getByText("Na fila")).toBeInTheDocument();
    expect(screen.getByText("Concluído")).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Baixar ZIP" })).toHaveLength(1);
  });

  it("baixa o ZIP com o token", async () => {
    const fetchMock = mockFetch({ "GET /api/videos/b/download": () => ({ status: 200, body: "zip" }) });
    URL.createObjectURL = vi.fn(() => "blob:fake");
    URL.revokeObjectURL = vi.fn();
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});

    renderList([video({ id: "b", original_name: "aula.mp4", status: "COMPLETED", download_url: "/api/videos/b/download" })]);
    await userEvent.click(screen.getByRole("button", { name: "Baixar ZIP" }));

    expect(click).toHaveBeenCalled();
    const headers = fetchMock.mock.calls[0]?.[1]?.headers as Record<string, string>;
    expect(headers.Authorization).toBe("Bearer tok");
  });

  it("avisa erro de download", async () => {
    mockFetch({ "GET /api/videos/b/download": () => ({ status: 409, body: { error: "vídeo ainda não foi processado" } }) });
    renderList([video({ id: "b", status: "COMPLETED", download_url: "/api/videos/b/download" })]);
    await userEvent.click(screen.getByRole("button", { name: "Baixar ZIP" }));
    expect(await screen.findByText("vídeo ainda não foi processado")).toBeInTheDocument();
  });

  it("encerra a sessão se o download retornar 401", async () => {
    mockFetch({ "GET /api/videos/b/download": () => ({ status: 401, body: { error: "token inválido" } }) });
    const props = renderList([video({ id: "b", status: "COMPLETED", download_url: "/api/videos/b/download" })]);
    await userEvent.click(screen.getByRole("button", { name: "Baixar ZIP" }));
    await vi.waitFor(() => expect(props.onUnauthorized).toHaveBeenCalled());
  });

  it("chama atualizar e mostra o indicador ao vivo", async () => {
    const props = renderList([video()], { active: true });
    expect(screen.getByText("atualizando")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Atualizar" }));
    expect(props.onRefresh).toHaveBeenCalled();
  });
});
