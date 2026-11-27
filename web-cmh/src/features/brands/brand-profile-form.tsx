import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { Controller, useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Field, FieldError, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { api, ApiError, brandInput, type BrandInput, type BrandSite } from "~/lib/api";

const fields = [
  ["name", "Profile name"],
  ["support_email", "Support email"],
  ["description", "Description"],
] as const;

export function brandErrorKey(error: unknown): string {
  if (!(error instanceof ApiError)) return "Unable to load brand profiles.";
  if (error.status === 401) return "Your session expired. Sign in again.";
  if (error.status === 403) return "You do not have permission to manage brand profiles.";
  if (error.status === 409) return "This profile changed. Reload it before saving again.";
  if (error.status === 400) return "Check the profile fields and byte limits.";
  if (error.status === 404) return "This profile is no longer available.";
  return "The OEM service is unavailable. Try again later.";
}

function validText(value: string, limit: number): boolean {
  if (new TextEncoder().encode(value).length > limit) return false;
  for (const character of value) {
    const code = character.codePointAt(0)!;
    if (code <= 31 || (code >= 127 && code <= 159)) return false;
  }
  return true;
}

/** Creates a profile, or edits `site` when given. `onSaved` receives the stored profile. */
export function BrandProfileForm(props: {
  site?: BrandSite;
  onSaved: (site: BrandSite) => void;
  onCancel?: () => void;
}) {
  const { t } = useTranslation();
  const site = props.site;
  const invalid = t("Check the profile fields and byte limits.");
  const schema = z.object({
    name: z
      .string()
      .refine((value) => value.trim() !== "" && value === value.trim() && validText(value, 100), {
        message: invalid,
      }),
    support_email: z.string().refine((value) => validText(value, 254), { message: invalid }),
    description: z.string().refine((value) => validText(value, 1000), { message: invalid }),
  });
  const form = useForm<BrandInput>({
    resolver: zodResolver(schema),
    defaultValues: site ? brandInput(site) : { name: "", support_email: "", description: "" },
  });
  const [saved, setSaved] = useState(false);
  const save = useMutation({
    mutationFn: (input: BrandInput) =>
      site ? api.updateBrand(site.id, site.version, input) : api.createBrand(input),
    onSuccess: (next) => {
      form.reset(brandInput(next));
      setSaved(true);
      props.onSaved(next);
    },
  });
  const reload = useMutation({
    mutationFn: () => api.brand(site!.id),
    onSuccess: (next) => {
      form.reset(brandInput(next));
      save.reset();
      props.onSaved(next);
    },
  });
  const busy = save.isPending || reload.isPending;
  const conflict = save.error instanceof ApiError && save.error.status === 409;
  return (
    <form
      noValidate
      className="grid gap-3.5"
      onSubmit={form.handleSubmit((input) => {
        if (busy) return;
        setSaved(false);
        save.mutate(input);
      })}
    >
      {fields.map(([name, label]) => (
        <Controller
          key={name}
          name={name}
          control={form.control}
          render={({ field, fieldState }) => (
            <Field data-invalid={fieldState.invalid}>
              <FieldLabel htmlFor={`brand-${name}`}>{t(label)}</FieldLabel>
              <Input
                {...field}
                id={`brand-${name}`}
                disabled={busy}
                aria-invalid={fieldState.invalid}
                aria-describedby={fieldState.invalid ? `brand-${name}-error` : undefined}
              />
              {fieldState.invalid && (
                <FieldError id={`brand-${name}-error`} errors={[fieldState.error]} />
              )}
            </Field>
          )}
        />
      ))}
      {!form.formState.isValid && form.formState.isSubmitted && (
        <p className="cmh-muted">
          {t("UTF-8 byte limits: name 100, email 254, description 1000. Single-line text only.")}
        </p>
      )}
      <div className="flex flex-wrap items-center gap-3">
        <Button type="submit" disabled={busy || conflict}>
          {save.isPending ? t("Saving...") : t("Save profile")}
        </Button>
        {props.onCancel && (
          <Button type="button" variant="outline" disabled={busy} onClick={props.onCancel}>
            {t("Cancel")}
          </Button>
        )}
        {saved && !form.formState.isDirty && (
          <span role="status" className="cmh-muted text-sm">
            {t("Profile saved.")}
          </span>
        )}
      </div>
      {save.isError && (
        <div role="alert">
          <p>{t(brandErrorKey(save.error))}</p>
          <p>{t("If the result is uncertain, check the list before creating again.")}</p>
          {site && (
            <Button
              type="button"
              variant="outline"
              disabled={reload.isPending}
              onClick={() => reload.mutate()}
            >
              {t("Reload saved profile and discard local changes")}
            </Button>
          )}
        </div>
      )}
      {reload.isError && <p role="alert">{t(brandErrorKey(reload.error))}</p>}
    </form>
  );
}
