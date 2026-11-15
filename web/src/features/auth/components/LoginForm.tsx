import { authApi } from "../api";
import { useFormSubmit } from "../hooks/useFormSubmit";
import { useSession } from "../session";
import { PasswordField } from "./PasswordField";

export function LoginForm() {
  const { start } = useSession();
  const { error, busy, onSubmit } = useFormSubmit(async ({ email = "", password = "" }) => {
    const res = await authApi.login(email, password);
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
