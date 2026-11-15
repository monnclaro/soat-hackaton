// Tela de acesso: apresentação + abas Entrar / Criar conta.
import { useState } from "react";
import { LoginForm } from "./LoginForm";
import { RegisterForm } from "./RegisterForm";

type Tab = "login" | "register";

const TABS: { id: Tab; label: string }[] = [
  { id: "login", label: "Entrar" },
  { id: "register", label: "Criar conta" },
];

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
          {TABS.map((t) => (
            <button
              key={t.id}
              className={`tabs__tab${tab === t.id ? " is-active" : ""}`}
              role="tab"
              type="button"
              aria-selected={tab === t.id}
              onClick={() => setTab(t.id)}
            >
              {t.label}
            </button>
          ))}
        </div>
        {tab === "login" ? <LoginForm /> : <RegisterForm />}
      </div>
    </section>
  );
}
