import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { useForm, Controller } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field, FieldLabel, FieldError } from "@/components/ui/field";
import { CopyButton } from "@/components/copy-button";
import { LoadingState } from "@/components/loading-state";
import { BrandAppearance } from "~/features/brands/brand-appearance";
import { BrandDomain } from "~/features/brands/brand-domain";
import {
  api,
  ApiError,
  brandInput,
  type BrandInput,
  type BrandSite,
  officialLoginUrl,
  ownsBrand,
} from "~/lib/api";

export const Route = createFileRoute("/brands")({ component: Brands });
const auditLabels: Record<string, string> = {
  "site.created": "Profile created",
  "site.updated": "Profile updated",
  "site.domain_set": "Domain saved",
  "site.domain_verified": "Domain verified",
  "site.domain_cleared": "Domain removed",
  "site.appearance_updated": "Appearance updated",
  "site.logo_updated": "Logo updated",
  "site.logo_removed": "Logo removed",
};
const blank: BrandInput = { name: "", support_email: "", description: "" };

function errorKey(error: unknown): string {
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

function Brands() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const list = useQuery({ queryKey: ["oem-sites"], queryFn: api.brands, gcTime: 0 });
  const [selected, setSelected] = useState<BrandSite | null>(null);
  const [editing, setEditing] = useState(false);
  const schema = z.object({
    name: z
      .string()
      .refine((value) => value.trim() !== "" && value === value.trim() && validText(value, 100), {
        message: t("Check the profile fields and byte limits."),
      }),
    support_email: z.string().refine((value) => validText(value, 254), {
      message: t("Check the profile fields and byte limits."),
    }),
    description: z.string().refine((value) => validText(value, 1000), {
      message: t("Check the profile fields and byte limits."),
    }),
  });
  const form = useForm<BrandInput>({ resolver: zodResolver(schema), defaultValues: blank });
  const [notice, setNotice] = useState(false);
  const audits = useQuery({
    queryKey: ["oem-audits", selected?.id],
    queryFn: () => api.brandAudits(selected!.id),
    enabled: !!selected,
    gcTime: 0,
  });
  const save = useMutation({
    mutationFn: (input: BrandInput) =>
      selected ? api.updateBrand(selected.id, selected.version, input) : api.createBrand(input),
    onSuccess: (site) => {
      setSelected(site);
      form.reset(brandInput(site));
      setNotice(true);
      void client.invalidateQueries({ queryKey: ["oem-sites"] });
      void client.invalidateQueries({ queryKey: ["oem-audits", site.id] });
    },
  });
  const reload = useMutation({
    mutationFn: () => api.brand(selected!.id),
    onSuccess: (site) => {
      setSelected(site);
      form.reset(brandInput(site));
      save.reset();
      setNotice(false);
    },
  });
  const choose = (site: BrandSite | null) => {
    if (save.isPending || reload.isPending) return;
    setSelected(site);
    form.reset(site ? brandInput(site) : blank);
    setEditing(true);
    setNotice(false);
    save.reset();
    reload.reset();
  };
  const submit = form.handleSubmit((input) => {
    if (save.isPending || reload.isPending || (selected && !ownsBrand(selected))) return;
    setNotice(false);
    save.mutate(input);
  });
  if (list.isPending) return <LoadingState message={t("Loading brand profiles...")} />;
  if (list.isError)
    return (
      <section role="alert">
        <h2>{t("Brand profiles")}</h2>
        <p>{t(errorKey(list.error))}</p>
        <Button onClick={() => void list.refetch()}>{t("Retry")}</Button>
        {list.error instanceof ApiError && list.error.status === 401 && (
          <a href={officialLoginUrl()}>{t("Sign in")}</a>
        )}
      </section>
    );
  return (
    <section>
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2>{t("Brand profiles")}</h2>
          <p>{t("Manage owned profiles, not live domains or gateway accounts.")}</p>
        </div>
        <Button onClick={() => choose(null)} disabled={save.isPending || reload.isPending}>
          {t("Create profile")}
        </Button>
      </header>
      <p>{t("The latest 100 profiles and audit records are shown.")}</p>
      {list.data.length === 0 ? (
        <p>{t("No brand profiles yet. Create your first profile.")}</p>
      ) : (
        <ul>
          {list.data.map((site) => (
            <li key={site.id}>
              <div className="min-w-0">
                <strong>{site.name}</strong>
                <div className="cmh-muted">{site.support_email || t("No support email")}</div>
                <div className="flex items-center gap-1">
                  <span className="cmh-mono">
                    {t("Site ID")}: {site.id}
                  </span>
                  <CopyButton value={site.id} aria-label={t("Copy site ID")} />
                </div>
              </div>
              <Button
                variant="outline"
                disabled={save.isPending || reload.isPending}
                onClick={() => choose(site)}
              >
                {ownsBrand(site) ? t("Edit profile") : t("View shared profile")}
              </Button>
            </li>
          ))}
        </ul>
      )}
      {editing && (
        <section aria-label={t("Profile editor")}>
          <h3>{selected ? t("Edit profile") : t("Create profile")}</h3>
          {selected && (
            <p>
              {t("Version")}: {selected.version}
            </p>
          )}
          {selected && !ownsBrand(selected) ? (
            <section>
              <h3>{t("Shared read-only access")}</h3>
              <p>{selected.name}</p>
              <p>{selected.support_email || t("No support email")}</p>
              <p>{selected.description}</p>
              <Button
                variant="outline"
                onClick={() => {
                  setEditing(false);
                  setSelected(null);
                }}
              >
                {t("Close")}
              </Button>
            </section>
          ) : (
            <form onSubmit={submit} noValidate>
              {(
                [
                  ["name", "Profile name"],
                  ["support_email", "Support email"],
                  ["description", "Description"],
                ] as const
              ).map(([name, label]) => (
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
                        disabled={save.isPending || reload.isPending}
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
              <p className="cmh-muted">
                {t(
                  "UTF-8 byte limits: name 100, email 254, description 1000. Single-line text only.",
                )}
              </p>
              <div className="flex flex-wrap gap-3">
                <Button
                  type="submit"
                  disabled={
                    save.isPending ||
                    reload.isPending ||
                    (save.error instanceof ApiError && save.error.status === 409)
                  }
                >
                  {save.isPending ? t("Saving...") : t("Save profile")}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  disabled={save.isPending || reload.isPending}
                  onClick={() => {
                    setEditing(false);
                    setSelected(null);
                  }}
                >
                  {t("Close")}
                </Button>
              </div>
            </form>
          )}
          {notice && <p role="status">{t("Profile saved.")}</p>}
          {save.isError && (
            <div role="alert">
              <p>{t(errorKey(save.error))}</p>
              <p>{t("If the result is uncertain, check the list before creating again.")}</p>
              {selected && (
                <Button
                  variant="outline"
                  disabled={reload.isPending}
                  onClick={() => reload.mutate()}
                >
                  {t("Reload saved profile and discard local changes")}
                </Button>
              )}
            </div>
          )}
          {reload.isError && <p role="alert">{t(errorKey(reload.error))}</p>}
          {selected && ownsBrand(selected) && (
            <section aria-label={t("Appearance and SEO")}>
              <h3>{t("Appearance and SEO")}</h3>
              <BrandAppearance key={selected.id} siteID={selected.id} />
            </section>
          )}
          {selected && ownsBrand(selected) && (
            <section aria-label={t("Custom domain")}>
              <h3>{t("Custom domain")}</h3>
              <BrandDomain key={selected.id} siteID={selected.id} />
            </section>
          )}
          {selected && (
            <section>
              <h3>{t("Operation records")}</h3>
              {audits.isPending ? (
                <p>{t("Loading operation records...")}</p>
              ) : audits.isError ? (
                <div role="alert">
                  <p>{t(errorKey(audits.error))}</p>
                  <Button variant="outline" onClick={() => void audits.refetch()}>
                    {t("Retry")}
                  </Button>
                </div>
              ) : audits.data.length === 0 ? (
                <p>{t("No operation records.")}</p>
              ) : (
                <ul>
                  {audits.data.map((record) => (
                    <li key={record.id}>
                      {t(auditLabels[record.action] ?? "Profile updated")} · {t("Version")}{" "}
                      {record.version}
                    </li>
                  ))}
                </ul>
              )}
            </section>
          )}
        </section>
      )}
    </section>
  );
}
