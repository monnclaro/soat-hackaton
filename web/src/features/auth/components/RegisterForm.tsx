import { useToast } from "@/shared/toast/ToastProvider";
import { authApi } from "../api";
import { useFormSubmit } from "../hooks/useFormSubmit";
import { useSession } from "../session";
import { PasswordField } from "./PasswordField";

export function RegisterForm() {
  const { start } = useSession();
  const notify = useToast();
  const { error, busy, onSubmit } = useFormSubmit(async ({ name = "", email = "", password = "" }) => {
    await authApi.register(name, email, password);
    // Conta criada: entra automaticamente com as mesmas credenciais.
    const res = await authApi.login(email, password);
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
