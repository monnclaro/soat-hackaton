import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";
import { App } from "./App";
import { mockFetch, user, video } from "./test/fakes";

const session = { token: "tok-123", user };

describe("acesso", () => {
  it("faz login e mostra os vídeos do usuário", async () => {
    const fetchMock = mockFetch({
      "POST /auth/login": () => ({ status: 200, body: { access_token: "tok-123", token_type: "Bearer", expires_at: "", user } }),
      "GET /api/videos": () => ({
        status: 200,
        body: {
          videos: [
            video({ id: "v1", original_name: "aula.mp4", status: "COMPLETED", frame_count: 12, download_url: "/api/videos/v1/download" }),
            video({ id: "v2", original_name: "ruim.mp4", status: "FAILED", error: "ffmpeg não conseguiu processar" }),
          ],
        },
      }),
    });
    render(<App />);

    const form = screen.getByRole("form", { name: "Entrar" });
    await userEvent.type(within(form).getByLabelText("E-mail"), "ana@fiapx.com");
    await userEvent.type(within(form).getByLabelText("Senha"), "senha-segura");
    await userEvent.click(within(form).getByRole("button", { name: "Entrar" }));

    expect(await screen.findByText("aula.mp4")).toBeInTheDocument();
    expect(screen.getByText("Ana Souza")).toBeInTheDocument();
    expect(screen.getByText("12 frames")).toBeInTheDocument();
    expect(screen.getByText("Concluído")).toBeInTheDocument();
    expect(screen.getByText("ffmpeg não conseguiu processar")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Baixar ZIP" })).toBeInTheDocument();

    // O token é enviado nas chamadas à API e a sessão fica salva.
    const listCall = fetchMock.mock.calls.find(([url]) => url === "/api/videos");
    expect((listCall?.[1]?.headers as Record<string, string>).Authorization).toBe("Bearer tok-123");
    expect(JSON.parse(localStorage.getItem("fiapx_session") ?? "{}").token).toBe("tok-123");
  });

  it("mostra o erro de credenciais inválidas", async () => {
    mockFetch({ "POST /auth/login": () => ({ status: 401, body: { error: "e-mail ou senha inválidos" } }) });
    render(<App />);
    const form = screen.getByRole("form", { name: "Entrar" });
    await userEvent.type(within(form).getByLabelText("E-mail"), "ana@fiapx.com");
    await userEvent.type(within(form).getByLabelText("Senha"), "errada123");
    await userEvent.click(within(form).getByRole("button", { name: "Entrar" }));
    expect(await within(form).findByRole("alert")).toHaveTextContent("e-mail ou senha inválidos");
  });

  it("valida os campos antes de enviar", async () => {
    const fetchMock = mockFetch({});
    render(<App />);
    const form = screen.getByRole("form", { name: "Entrar" });
    await userEvent.click(within(form).getByRole("button", { name: "Entrar" }));
    expect(within(form).getByRole("alert")).not.toBeEmptyDOMElement();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("cadastra e já entra logado", async () => {
    const fetchMock = mockFetch({
      "POST /auth/register": () => ({ status: 201, body: user }),
      "POST /auth/login": () => ({ status: 200, body: { access_token: "tok-new", token_type: "Bearer", expires_at: "", user } }),
      "GET /api/videos": () => ({ status: 200, body: { videos: [] } }),
    });
    render(<App />);
    await userEvent.click(screen.getByRole("tab", { name: "Criar conta" }));
    const form = screen.getByRole("form", { name: "Criar conta" });
    await userEvent.type(within(form).getByLabelText("Nome"), "Ana Souza");
    await userEvent.type(within(form).getByLabelText("E-mail"), "ana@fiapx.com");
    await userEvent.type(within(form).getByLabelText(/Senha/), "senha-segura");
    await userEvent.click(within(form).getByRole("button", { name: "Criar conta" }));

    expect(await screen.findByText(/Nenhum vídeo enviado ainda/)).toBeInTheDocument();
    expect(screen.getByText("Bem-vindo(a), Ana Souza!")).toBeInTheDocument();
    const registerBody = JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body));
    expect(registerBody).toEqual({ name: "Ana Souza", email: "ana@fiapx.com", password: "senha-segura" });
  });

  it("mostra e oculta a senha", async () => {
    render(<App />);
    const form = screen.getByRole("form", { name: "Entrar" });
    const input = within(form).getByLabelText("Senha");
    expect(input).toHaveAttribute("type", "password");
    await userEvent.click(within(form).getByRole("button", { name: "Mostrar senha" }));
    expect(input).toHaveAttribute("type", "text");
  });
});

describe("sessão", () => {
  beforeEach(() => localStorage.setItem("fiapx_session", JSON.stringify(session)));

  it("restaura a sessão salva e permite sair", async () => {
    mockFetch({ "GET /api/videos": () => ({ status: 200, body: { videos: [] } }) });
    render(<App />);
    expect(await screen.findByText(/Nenhum vídeo enviado ainda/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Sair" }));
    expect(screen.getByRole("form", { name: "Entrar" })).toBeInTheDocument();
    expect(localStorage.getItem("fiapx_session")).toBeNull();
  });

  it("volta para o login quando o token expira", async () => {
    mockFetch({ "GET /api/videos": () => ({ status: 401, body: { error: "token inválido ou expirado" } }) });
    render(<App />);
    expect(await screen.findByText("Sua sessão expirou. Entre novamente.")).toBeInTheDocument();
    expect(screen.getByRole("form", { name: "Entrar" })).toBeInTheDocument();
  });

  it("mostra os contadores por status", async () => {
    mockFetch({
      "GET /api/videos": () => ({
        status: 200,
        body: {
          videos: [
            video({ id: "a", status: "PENDING" }),
            video({ id: "b", status: "PROCESSING" }),
            video({ id: "c", status: "COMPLETED" }),
          ],
        },
      }),
    });
    render(<App />);
    const stats = await screen.findByLabelText("Resumo dos seus vídeos");
    await waitFor(() => expect(within(stats).getByText("Vídeos").previousSibling).toHaveTextContent("3"));
    expect(within(stats).getByText("Na fila / processando").previousSibling).toHaveTextContent("2");
    expect(within(stats).getByText("Concluídos").previousSibling).toHaveTextContent("1");
    expect(screen.getByText("atualizando")).toBeInTheDocument();
  });
});
