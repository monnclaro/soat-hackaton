import { useId, useState } from "react";

interface Props {
  label: string;
  name: string;
  autoComplete: string;
  minLength?: number;
  maxLength?: number;
}

/** Campo de senha com botão para mostrar/ocultar (o botão fica fora do rótulo do campo). */
export function PasswordField({ label, name, autoComplete, minLength, maxLength }: Props) {
  const [visible, setVisible] = useState(false);
  const id = useId();
  return (
    <div className="field">
      <label className="field__label" htmlFor={id}>
        {label}
      </label>
      <span className="field__password">
        <input
          id={id}
          className="field__input"
          name={name}
          type={visible ? "text" : "password"}
          autoComplete={autoComplete}
          minLength={minLength}
          maxLength={maxLength}
          required
        />
        <button
          className="field__toggle"
          type="button"
          aria-pressed={visible}
          aria-label={visible ? "Ocultar senha" : "Mostrar senha"}
          onClick={() => setVisible((v) => !v)}
        >
          👁
        </button>
      </span>
    </div>
  );
}
