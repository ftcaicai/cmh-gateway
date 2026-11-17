import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { ConfirmDialog } from "@/components/confirm-dialog";
import { CopyButton } from "@/components/copy-button";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { api, ApiError, type BrandDomain as Domain } from "~/lib/api";

const failures: Record<string, string> = {
  cname_missing: "No CNAME record was found for this domain yet.",
  cname_mismatch: "The CNAME record points somewhere else. Check the record value.",
  host_taken: "This domain is already served by another brand.",
};

/** Owner-only custom domain setup: save the domain, add the CNAME, wait for the check. */
export function BrandDomain({ siteID }: { siteID: string }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const key = ["oem-domain", siteID];
  const domain = useQuery({
    queryKey: key,
    queryFn: () => api.brandDomain(siteID),
    gcTime: 0,
    // The gateway checks pending domains every minute; show the result unprompted.
    refetchInterval: (query) =>
      query.state.data?.host && !query.state.data.verified ? 20_000 : false,
  });
  // null follows the saved value; a string is the owner's unsaved edit.
  const [draft, setDraft] = useState<string | null>(null);
  const [confirming, setConfirming] = useState(false);
  const save = useMutation({
    mutationFn: (host: string) => api.setBrandDomain(siteID, host),
    onSuccess: (saved: Domain) => {
      client.setQueryData(key, saved);
      setDraft(null);
      setConfirming(false);
    },
    onError: () => setConfirming(false),
  });
  const verify = useMutation({
    mutationFn: () => api.verifyBrandDomain(siteID),
    onSuccess: (checked: Domain) => client.setQueryData(key, checked),
  });

  if (domain.isPending) return <p>{t("Loading...")}</p>;
  if (domain.isError)
    return (
      <div role="alert">
        <p>{t("Unable to load the domain settings.")}</p>
        <Button variant="outline" onClick={() => void domain.refetch()}>
          {t("Retry")}
        </Button>
      </div>
    );
  const current = domain.data;
  if (!current.available) return <p>{t("Custom domains are not enabled on this gateway yet.")}</p>;

  const value = draft ?? current.host;
  const changed = value.trim() !== current.host;
  const busy = save.isPending || verify.isPending;
  const submit = () => {
    // Replacing or removing a saved domain takes it offline at once: confirm first.
    if (current.host) setConfirming(true);
    else save.mutate(value.trim());
  };
  const failure = failures[current.last_error];
  let saveLabel = t("Save domain");
  if (save.isPending) saveLabel = t("Saving...");
  else if (!value.trim() && current.host) saveLabel = t("Remove domain");

  return (
    <div>
      <p>
        {t(
          "Serve this brand on your own domain. Changing the domain takes the previous one offline immediately.",
        )}
      </p>
      <form
        noValidate
        onSubmit={(event) => {
          event.preventDefault();
          if (changed && !busy) submit();
        }}
      >
        <Field>
          <FieldLabel htmlFor="brand-domain">{t("Domain")}</FieldLabel>
          <Input
            id="brand-domain"
            value={value}
            placeholder="api.example.com"
            disabled={busy}
            onChange={(event) => setDraft(event.target.value)}
          />
        </Field>
        <div className="flex flex-wrap gap-3">
          <Button type="submit" disabled={!changed || busy}>
            {saveLabel}
          </Button>
          {draft !== null && (
            <Button type="button" variant="outline" disabled={busy} onClick={() => setDraft(null)}>
              {t("Cancel")}
            </Button>
          )}
        </div>
      </form>
      {save.isError && (
        <p role="alert">
          {t(
            save.error instanceof ApiError && save.error.status === 400
              ? "Enter a public domain name such as api.example.com. Platform domains cannot be used."
              : "Unable to save the domain. Try again later.",
          )}
        </p>
      )}
      {current.host && (
        <div className="mt-5">
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <strong>{current.host}</strong>
            <Badge variant={current.verified ? "default" : "secondary"}>
              {current.verified ? t("Live") : t("Waiting for DNS")}
            </Badge>
          </div>
          {current.verified ? (
            <p className="cmh-muted mt-2">
              {t(
                "Visitors of this domain now see this brand. HTTPS is provided by the gateway operator.",
              )}
            </p>
          ) : (
            <>
              <p className="mt-3">{t("Add this record at your DNS provider:")}</p>
              <dl>
                <dt>{t("Type")}</dt>
                <dd>CNAME</dd>
                <dt>{t("Name")}</dt>
                <dd>{current.host}</dd>
                <dt>{t("Value")}</dt>
                <dd className="flex items-center gap-1">
                  <span className="cmh-mono">{current.cname_target}</span>
                  <CopyButton value={current.cname_target} aria-label={t("Copy record value")} />
                </dd>
              </dl>
              <p className="cmh-muted mt-3">
                {t(
                  "The record is checked automatically every minute. DNS changes can take a while to spread.",
                )}
              </p>
              {failure && <p role="status">{t(failure)}</p>}
              <Button
                className="mt-3"
                variant="outline"
                disabled={busy || changed}
                onClick={() => verify.mutate()}
              >
                {verify.isPending ? t("Checking...") : t("Check now")}
              </Button>
              {verify.isError && (
                <p role="alert">{t("Unable to check the domain. Try again later.")}</p>
              )}
            </>
          )}
        </div>
      )}
      <ConfirmDialog
        open={confirming}
        onOpenChange={(open) => {
          if (!open && !save.isPending) setConfirming(false);
        }}
        title={value.trim() ? t("Change domain") : t("Remove domain")}
        desc={t(
          "The current domain stops serving this brand immediately and must be verified again.",
        )}
        confirmText={t("Continue")}
        isLoading={save.isPending}
        handleConfirm={() => save.mutate(value.trim())}
      />
    </div>
  );
}
