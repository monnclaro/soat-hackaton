// Envio de formulário com validação HTML (required, type=email, minlength), estado de
// "enviando" e mensagem de erro.
import { useState, type FormEvent } from "react";
import { errorMessage } from "@/shared/api/http";

export type FormFields = Record<string, string>;

/** Valida os campos com as regras HTML. Retorna os valores ou a mensagem do primeiro erro. */
export function readForm(form: HTMLFormElement): FormFields | string {
  for (const input of Array.from(form.querySelectorAll("input"))) {
    const invalid = !input.checkValidity();
    input.setAttribute("aria-invalid", String(invalid));
    if (invalid) {
      input.focus();
      return input.validationMessage || "Preencha os campos corretamente.";
    }
  }
  return Object.fromEntries(new FormData(form)) as FormFields;
}

export function useFormSubmit(action: (fields: FormFields) => Promise<void>) {
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
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return { error, busy, onSubmit };
}
