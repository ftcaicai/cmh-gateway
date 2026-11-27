import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { formatTimestampToDate } from "@/lib/format";
import { api, type BrandAppearance, type LogoHistoryEntry } from "~/lib/api";

/** The last few replaced or removed logos, each restorable. Owner only. */
export function BrandLogoHistory(props: { siteID: string; logoUpdatedAt: number }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  // Any logo change moves an entry in or out, so key the list on it.
  const history = useQuery({
    queryKey: ["oem-logo-history", props.siteID, props.logoUpdatedAt],
    queryFn: () => api.logoHistory(props.siteID),
    gcTime: 0,
  });
  const restore = useMutation({
    mutationFn: (entry: string) => api.restoreLogo(props.siteID, entry),
    onSuccess: (next: BrandAppearance) => {
      client.setQueryData(["oem-appearance", props.siteID], next);
      void client.invalidateQueries({ queryKey: ["oem-audits", props.siteID] });
    },
  });
  if (history.isPending || history.isError || history.data.length === 0) return null;
  return (
    <div className="mt-4">
      <div className="text-sm font-medium">{t("Previous logos")}</div>
      <ul className="mt-2 flex flex-wrap gap-4">
        {history.data.map((entry) => (
          <HistoryItem
            key={entry.id}
            siteID={props.siteID}
            entry={entry}
            busy={restore.isPending}
            onRestore={() => restore.mutate(entry.id)}
          />
        ))}
      </ul>
      {restore.isError && <p role="alert">{t("Unable to restore the logo. Try again later.")}</p>}
    </div>
  );
}

function HistoryItem(props: {
  siteID: string;
  entry: LogoHistoryEntry;
  busy: boolean;
  onRestore: () => void;
}) {
  const { t } = useTranslation();
  const image = useQuery({
    queryKey: ["oem-logo-history-image", props.siteID, props.entry.id],
    queryFn: () => api.historicLogo(props.siteID, props.entry.id),
    gcTime: 0,
  });
  return (
    <li className="flex items-center gap-3">
      <div className="bg-muted/50 flex size-12 items-center justify-center overflow-hidden rounded-lg border">
        {image.data && (
          <img
            src={`data:${image.data.content_type};base64,${image.data.image}`}
            alt={t("Previous logo")}
            className="max-h-full max-w-full object-contain"
          />
        )}
      </div>
      <div className="text-sm">
        <div className="cmh-muted">{formatTimestampToDate(props.entry.replaced_at)}</div>
        <Button type="button" variant="outline" disabled={props.busy} onClick={props.onRestore}>
          {t("Restore")}
        </Button>
      </div>
    </li>
  );
}
