// Tela de acesso: abas Entrar/Criar conta, mostrar senha e envio dos formulários.
import { $, $$ } from "./dom.js";
import { api } from "./api.js";
import { session } from "./session.js";
import { toast } from "./toast.js";

function showTab(name) {
  const isLogin = name === "login";
  $("#tabLogin").classList.toggle("is-active", isLogin);
  $("#tabRegister").classList.toggle("is-active", !isLogin);
  $("#tabLogin").setAttribute("aria-selected", String(isLogin));
  $("#tabRegister").setAttribute("aria-selected", String(!isLogin));
  $("#loginForm").hidden = !isLogin;
  $("#registerForm").hidden = isLogin;
  $(isLogin ? "#loginForm" : "#registerForm").querySelector("input")?.focus();
}

function setError(form, message, success = false) {
  const box = form.querySelector(".form__error");
  box.textContent = message;
  box.classList.toggle("is-success", success);
}

function validate(form) {
  for (const input of form.querySelectorAll("input")) {
    const invalid = !input.checkValidity();
    input.setAttribute("aria-invalid", String(invalid));
    if (invalid) {
      input.focus();
      return input.validationMessage;
    }
  }
  return "";
}

async function submit(form, action) {
  const message = validate(form);
  if (message) return setError(form, message);
  const button = form.querySelector('button[type="submit"]');
  button.disabled = true;
  setError(form, "");
  try {
    await action(Object.fromEntries(new FormData(form)));
  } catch (err) {
    setError(form, err.message);
  } finally {
    button.disabled = false;
  }
}

/** @param {() => void} onLogin chamado após login bem-sucedido */
export function initAuth(onLogin) {
  $("#tabLogin").addEventListener("click", () => showTab("login"));
  $("#tabRegister").addEventListener("click", () => showTab("register"));

  for (const btn of $$("[data-toggle-password]")) {
    btn.addEventListener("click", () => {
      const input = btn.parentElement.querySelector("input");
      const show = input.type === "password";
      input.type = show ? "text" : "password";
      btn.setAttribute("aria-pressed", String(show));
      btn.setAttribute("aria-label", show ? "Ocultar senha" : "Mostrar senha");
    });
  }

  const loginForm = $("#loginForm");
  loginForm.addEventListener("submit", (e) => {
    e.preventDefault();
    submit(loginForm, async ({ email, password }) => {
      const res = await api.login(email, password);
      session.start(res.access_token, res.user);
      loginForm.reset();
      onLogin();
    });
  });

  const registerForm = $("#registerForm");
  registerForm.addEventListener("submit", (e) => {
    e.preventDefault();
    submit(registerForm, async ({ name, email, password }) => {
      await api.register(name, email, password);
      // Conta criada: faz login automaticamente com as mesmas credenciais.
      const res = await api.login(email, password);
      session.start(res.access_token, res.user);
      registerForm.reset();
      toast(`Bem-vindo(a), ${res.user.name}!`, "ok");
      onLogin();
    });
  });
}
