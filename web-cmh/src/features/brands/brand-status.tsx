import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { ConfirmDialog } from "@/components/confirm-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { formatTimestampToDate } from "@/lib/format";
import { api, type BrandPause } from "~/lib/api";

/** Owner-only switch that takes the whole storefront offline and back. */
export function BrandStatus({ siteID }: { siteID: string }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const key = ["oem-pause", siteID];
  const pause = useQuery({ queryKey: key, queryFn: () => api.brandPause(siteID), gcTime: 0 });
  const [confirming, setConfirming] = useState(false);
  const change = useMutation({
    mutationFn: (paused: boolean) => api.setBrandPause(siteID, paused),
    onSuccess: (next: BrandPause) => {
      client.setQueryData(key, next);
      setConfirming(false);
      void client.invalidateQueries({ queryKey: ["oem-audits", siteID] });
    },
    onError: () => setConfirming(false),
  });

  if (pause.isPending) return <p>{t("Loading...")}</p>;
  if (pause.isError)
    return (
      <div role="alert">
        <p>{t("Unable to load the storefront status.")}</p>
        <Button variant="outline" onClick={() => void pause.refetch()}>
          {t("Retry")}
        </Button>
      </div>
    );
  const paused = pause.data.paused;
  return (
    <div>
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <Badge variant={paused ? "secondary" : "default"}>
          {paused ? t("Paused") : t("Online")}
        </Badge>
        {paused && (
          <span className="cmh-muted">
            {t("Since")} {formatTimestampToDate(pause.data.paused_at)}
          </span>
        )}
      </div>
      <p className="cmh-muted mt-2">
        {paused
          ? t("Every page on this brand's domains shows a paused notice until you resume it.")
          : t(
              "Pausing shows a paused notice on every page and blocks sign-in on this brand's domains. Domain, customers and settings are kept.",
            )}
      </p>
      <Button
        className="mt-3"
        variant="outline"
        disabled={change.isPending}
        onClick={() => (paused ? change.mutate(false) : setConfirming(true))}
      >
        {paused ? t("Resume storefront") : t("Pause storefront")}
      </Button>
      {change.isError && (
        <p role="alert">{t("Unable to change the storefront status. Try again later.")}</p>
      )}
      <ConfirmDialog
        open={confirming}
        onOpenChange={(open) => {
          if (!open && !change.isPending) setConfirming(false);
        }}
        title={t("Pause storefront")}
        desc={t(
          "Customers can no longer use this brand's site until you resume it. Their accounts and sessions are not changed.",
        )}
        confirmText={t("Pause storefront")}
        isLoading={change.isPending}
        handleConfirm={() => change.mutate(true)}
      />
    </div>
  );
}
