import { useId } from "react";
import { FormFieldLabel, formInputClassName } from "@/components/layout/form-field";
import { Input } from "@/components/ui/input";

interface TextFieldProps {
  label: string;
  value: string;
  onChange: (value: string) => void;
  required?: boolean;
  type?: "text" | "email" | "date";
  className?: string;
  disabled?: boolean;
  /** Keterangan di bawah field, mis. alasan field dikunci. */
  hint?: string;
  placeholder?: string;
  maxLength?: number;
  /** Galat validasi; ditampilkan merah di bawah field. */
  error?: string | null;
}

export function TextField({
  label,
  value,
  onChange,
  required,
  type,
  className,
  disabled,
  hint,
  placeholder,
  maxLength,
  error,
}: TextFieldProps) {
  const id = useId();
  return (
    <div className={className}>
      <FormFieldLabel required={required} htmlFor={id}>
        {label}
      </FormFieldLabel>
      <Input
        id={id}
        type={type}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className={formInputClassName}
        disabled={disabled}
        placeholder={placeholder}
        maxLength={maxLength}
        aria-invalid={error ? true : undefined}
        aria-describedby={error ? `${id}-error` : undefined}
      />
      {error ? (
        <p id={`${id}-error`} role="alert" className="mt-1 text-xs text-red-600">
          {error}
        </p>
      ) : null}
      {hint ? <p className="mt-1 text-xs text-muted-foreground">{hint}</p> : null}
    </div>
  );
}
