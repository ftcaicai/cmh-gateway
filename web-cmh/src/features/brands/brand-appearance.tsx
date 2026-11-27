import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { api, ApiError, type BrandAppearance as Appearance } from "~/lib/api";

import { BrandLogoHistory } from "./brand-logo-history";
import { BrandPreview } from "./brand-preview";

const themes = [
  { id: "default", label: "Cloud-white business", swatch: "#285cf2", surface: "#f4f7fc" },
  { id: "technology", label: "Dark technology", swatch: "#6b96ff", surface: "#0b1120" },
  { id: "business", label: "Warm-white magazine", swatch: "#a84c16", surface: "#faf6ef" },
] as const;
const logoLimit = 1024 * 1024;

/** Owner-only storefront look: theme, logo and basic SEO. */
export function BrandAppearance({ siteID }: { siteID: string }) {
  const { t } = useTranslation();
  const appearance = useQuery({
    queryKey: ["oem-appearance", siteID],
    queryFn: () => api.brandAppearance(siteID),
    gcTime: 0,
  });
  if (appearance.isPending) return <p>{t("Loading...")}</p>;
  if (appearance.isError)
    return (
      <div role="alert">
        <p>{t("Unable to load the appearance settings.")}</p>
        <Button variant="outline" onClick={() => void appearance.refetch()}>
          {t("Retry")}
        </Button>
      </div>
    );
  // Remount on a new version so the form starts from what the server holds.
  return (
    <>
      <Logo siteID={siteID} current={appearance.data} />
      <Settings key={appearance.data.version} siteID={siteID} saved={appearance.data} />
    </>
  );
}

function Settings({ siteID, saved }: { siteID: string; saved: Appearance }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const key = ["oem-appearance", siteID];
  const [form, setForm] = useState({
    theme: saved.theme,
    seo_title: saved.seo_title,
    seo_description: saved.seo_description,
    seo_keywords: saved.seo_keywords,
    allow_indexing: saved.allow_indexing,
  });
  const [notice, setNotice] = useState(false);
  const save = useMutation({
    mutationFn: () => api.updateBrandAppearance(siteID, { ...form, version: saved.version }),
    onSuccess: (next: Appearance) => {
      client.setQueryData(key, next);
      setNotice(true);
    },
  });
  const conflict = save.error instanceof ApiError && save.error.status === 409;
  const set = (patch: Partial<typeof form>) => {
    setNotice(false);
    setForm((value) => ({ ...value, ...patch }));
  };
  let failure = "Unable to save the appearance. Try again later.";
  if (conflict) failure = "These settings changed elsewhere. Reload before saving again.";
  else if (save.error instanceof ApiError && save.error.status === 400)
    failure = "Check the length of the title, description and keywords.";

  return (
    <form
      noValidate
      onSubmit={(event) => {
        event.preventDefault();
        if (!save.isPending && !conflict) save.mutate();
      }}
    >
      <Field>
        <FieldLabel>{t("Site theme")}</FieldLabel>
        <div className="grid gap-3 sm:grid-cols-3">
          {themes.map((theme) => (
            <button
              key={theme.id}
              type="button"
              aria-pressed={form.theme === theme.id}
              disabled={save.isPending}
              onClick={() => set({ theme: theme.id })}
              className={`rounded-lg border p-3 text-left text-sm transition-colors ${
                form.theme === theme.id
                  ? "border-primary ring-primary/30 ring-2"
                  : "hover:border-primary/50"
              }`}
            >
              <span
                className="mb-2 flex h-12 items-end rounded-md border p-2"
                style={{ background: theme.surface }}
                aria-hidden="true"
              >
                <span className="h-2 w-10 rounded-full" style={{ background: theme.swatch }} />
              </span>
              {t(theme.label)}
            </button>
          ))}
        </div>
      </Field>
      <Field>
        <FieldLabel htmlFor="seo-title">{t("Site title")}</FieldLabel>
        <Input
          id="seo-title"
          value={form.seo_title}
          maxLength={120}
          disabled={save.isPending}
          placeholder={t("Defaults to the brand name")}
          onChange={(event) => set({ seo_title: event.target.value })}
        />
      </Field>
      <Field>
        <FieldLabel htmlFor="seo-description">{t("Site description")}</FieldLabel>
        <Textarea
          id="seo-description"
          value={form.seo_description}
          maxLength={300}
          rows={3}
          disabled={save.isPending}
          placeholder={t("Defaults to the brand description")}
          onChange={(event) =>
            set({ seo_description: event.target.value.replace(/[\r\n]+/g, " ") })
          }
        />
      </Field>
      <Field>
        <FieldLabel htmlFor="seo-keywords">{t("Keywords")}</FieldLabel>
        <Input
          id="seo-keywords"
          value={form.seo_keywords}
          maxLength={200}
          disabled={save.isPending}
          onChange={(event) => set({ seo_keywords: event.target.value })}
        />
      </Field>
      <label className="flex items-center gap-3 text-sm">
        <Switch
          checked={form.allow_indexing}
          disabled={save.isPending}
          onCheckedChange={(checked) => set({ allow_indexing: checked })}
        />
        {t("Allow search engines to index the home, pricing and docs pages")}
      </label>
      <p className="cmh-muted">
        {t(
          "Sign-in and account pages are never indexed. Limits: title 120, description 300, keywords 200 characters.",
        )}
      </p>
      <div className="flex flex-wrap gap-3">
        <Button type="submit" disabled={save.isPending || conflict}>
          {save.isPending ? t("Saving...") : t("Save appearance")}
        </Button>
        <BrandPreview siteID={siteID} theme={form.theme} disabled={save.isPending} />
      </div>
      {notice && <p role="status">{t("Appearance saved.")}</p>}
      {save.isError && (
        <div role="alert">
          <p>{t(failure)}</p>
          {conflict && (
            <Button
              type="button"
              variant="outline"
              onClick={() => void client.invalidateQueries({ queryKey: key })}
            >
              {t("Reload")}
            </Button>
          )}
        </div>
      )}
    </form>
  );
}

function Logo({ siteID, current }: { siteID: string; current: Appearance }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const input = useRef<HTMLInputElement>(null);
  const [problem, setProblem] = useState("");
  const logo = useQuery({
    queryKey: ["oem-logo", siteID, current.logo_updated_at],
    queryFn: () => api.brandLogo(siteID),
    enabled: current.has_logo,
    gcTime: 0,
  });
  const upload = useMutation({
    mutationFn: (image: string) => api.setBrandLogo(siteID, image),
    onSuccess: (next: Appearance) => {
      client.setQueryData(["oem-appearance", siteID], next);
      void client.invalidateQueries({ queryKey: ["oem-logo-history", siteID] });
    },
    onError: (error) =>
      setProblem(
        error instanceof ApiError && error.status === 400
          ? "The logo must be a PNG, JPEG or WebP image up to 1 MB and 2048 × 2048 pixels."
          : "Unable to save the logo. Try again later.",
      ),
  });
  const choose = (file: File | undefined) => {
    setProblem("");
    if (!file) return;
    if (file.size > logoLimit) {
      setProblem("The logo must be a PNG, JPEG or WebP image up to 1 MB and 2048 × 2048 pixels.");
      return;
    }
    const reader = new FileReader();
    reader.onload = () => {
      // A data URL is "data:<type>;base64,<payload>"; the server checks the bytes.
      upload.mutate(String(reader.result).slice(String(reader.result).indexOf(",") + 1));
    };
    reader.onerror = () => setProblem("Unable to save the logo. Try again later.");
    reader.readAsDataURL(file);
  };

  return (
    <div className="mb-6">
      <div className="text-sm font-medium">{t("Logo")}</div>
      <div className="mt-2 flex flex-wrap items-center gap-4">
        <div className="bg-muted/50 flex size-16 items-center justify-center overflow-hidden rounded-lg border">
          {current.has_logo && logo.data ? (
            <img
              src={`data:${logo.data.content_type};base64,${logo.data.image}`}
              alt={t("Current logo")}
              className="max-h-full max-w-full object-contain"
            />
          ) : (
            <span className="cmh-muted text-xs">{t("None")}</span>
          )}
        </div>
        <input
          ref={input}
          type="file"
          accept="image/png,image/jpeg,image/webp"
          className="sr-only"
          aria-label={t("Choose a logo file")}
          onChange={(event) => {
            choose(event.target.files?.[0]);
            event.target.value = "";
          }}
        />
        <Button
          type="button"
          variant="outline"
          disabled={upload.isPending}
          onClick={() => input.current?.click()}
        >
          {upload.isPending ? t("Saving...") : t("Upload logo")}
        </Button>
        {current.has_logo && (
          <Button
            type="button"
            variant="outline"
            disabled={upload.isPending}
            onClick={() => {
              setProblem("");
              upload.mutate("");
            }}
          >
            {t("Use default mark")}
          </Button>
        )}
      </div>
      <p className="cmh-muted mt-2">
        {t("Shown in the site header and as the browser icon. PNG, JPEG or WebP, up to 1 MB.")}
      </p>
      {problem && <p role="alert">{t(problem)}</p>}
      <BrandLogoHistory siteID={siteID} />
    </div>
  );
}
