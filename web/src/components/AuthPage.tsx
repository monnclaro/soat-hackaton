// Tela de acesso: abas Entrar / Criar conta.
import { useState, type FormEvent } from "react";
import { api } from "../api/client";
import { useSession } from "../session/SessionContext";
import { useToast } from "../toast/ToastContext";
import { PasswordField } from "./PasswordField";

type Tab = "login" | "register";

/** Lê os campos do formulário e valida com as regras HTML (required, type=email, minlength). */
function readForm(form: HTMLFormElement): Record<string, string> | string {
  for (const input of Array.from(form.querySelectorAll("input"))) {
    const invalid = !input.checkValidity();
    input.setAttribute("aria-invalid", String(invalid));
    if (invalid) {
      input.focus();
      return input.validationMessage || "Preencha os campos corretamente.";
    }
  }
  return Object.fromEntries(new FormData(form)) as Record<string, string>;
}

function useSubmit(action: (fields: Record<string, string>) => Promise<void>) {
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const fields = readForm(e.currentTarget);
    if (typeof fields === "string") return setError(fields);
    setBusy(true);
    setError("");
    try {
      await action(fields);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }
  return { error, busy, onSubmit };
}

function LoginForm() {
  const { start } = useSession();
  const { error, busy, onSubmit } = useSubmit(async ({ email = "", password = "" }) => {
    const res = await api.login(email, password);
    start({ token: res.access_token, user: res.user });
  });

  return (
    <form className="form" onSubmit={onSubmit} noValidate aria-label="Entrar">
      <label className="field">
        <span className="field__label">E-mail</span>
        <input className="field__input" name="email" type="email" autoComplete="email" required />
      </label>
      <PasswordField label="Senha" name="password" autoComplete="current-password" />
      <p className="form__error" role="alert">
        {error}
      </p>
      <button className="btn btn--primary btn--block" type="submit" disabled={busy}>
        Entrar
      </button>
    </form>
  );
}

function RegisterForm() {
  const { start } = useSession();
  const notify = useToast();
  const { error, busy, onSubmit } = useSubmit(async ({ name = "", email = "", password = "" }) => {
    await api.register(name, email, password);
    // Conta criada: entra automaticamente com as mesmas credenciais.
    const res = await api.login(email, password);
    start({ token: res.access_token, user: res.user });
    notify(`Bem-vindo(a), ${res.user.name}!`, "ok");
  });

  return (
    <form className="form" onSubmit={onSubmit} noValidate aria-label="Criar conta">
      <label className="field">
        <span className="field__label">Nome</span>
        <input className="field__input" name="name" autoComplete="name" maxLength={120} required />
      </label>
      <label className="field">
        <span className="field__label">E-mail</span>
        <input className="field__input" name="email" type="email" autoComplete="email" required />
      </label>
      <PasswordField
        label="Senha (mínimo de 8 caracteres)"
        name="password"
        autoComplete="new-password"
        minLength={8}
        maxLength={72}
      />
      <p className="form__error" role="alert">
        {error}
      </p>
      <button className="btn btn--primary btn--block" type="submit" disabled={busy}>
        Criar conta
      </button>
    </form>
  );
}

export function AuthPage() {
  const [tab, setTab] = useState<Tab>("login");
  return (
    <section className="auth">
      <div className="auth__intro">
        <h1>Transforme vídeos em imagens.</h1>
        <p>
          Envie um ou vários vídeos e receba um <strong>.zip com os frames</strong> de cada um. O processamento roda em
          paralelo e você acompanha o status em tempo real.
        </p>
        <ul className="feature-list">
          <li>Vários vídeos ao mesmo tempo</li>
          <li>Status de cada envio</li>
          <li>Aviso por e-mail se algo der errado</li>
        </ul>
      </div>

      <div className="card auth__card">
        <div className="tabs" role="tablist" aria-label="Acesso">
          {(["login", "register"] as const).map((t) => (
            <button
              key={t}
              className={`tabs__tab${tab === t ? " is-active" : ""}`}
              role="tab"
              type="button"
              aria-selected={tab === t}
              onClick={() => setTab(t)}
            >
              {t === "login" ? "Entrar" : "Criar conta"}
            </button>
          ))}
        </div>
        {tab === "login" ? <LoginForm /> : <RegisterForm />}
      </div>
    </section>
  );
}
