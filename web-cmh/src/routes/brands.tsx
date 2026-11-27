import { useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { Store } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { CopyButton } from "@/components/copy-button";
import { Dialog } from "@/components/dialog";
import { EmptyState } from "@/components/empty-state";
import { LoadingState } from "@/components/loading-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { BrandDetail } from "~/features/brands/brand-detail";
import { brandErrorKey, BrandProfileForm } from "~/features/brands/brand-profile-form";
import { api, ApiError, officialLoginUrl, ownsBrand } from "~/lib/api";

export const Route = createFileRoute("/brands")({
  component: Brands,
  // ?site=<id> opens that brand, so it can be linked to and left with Back.
  validateSearch: (search: Record<string, unknown>): { site?: string } =>
    typeof search.site === "string" && search.site ? { site: search.site } : {},
});

function Brands() {
  const { site } = Route.useSearch();
  const navigate = Route.useNavigate();
  const open = (id?: string) => void navigate({ search: id ? { site: id } : {} });
  if (site) return <BrandDetail key={site} siteID={site} onBack={() => open()} />;
  return <BrandList onOpen={open} />;
}

function BrandList(props: { onOpen: (id: string) => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const list = useQuery({ queryKey: ["oem-sites"], queryFn: api.brands, gcTime: 0 });
  const [creating, setCreating] = useState(false);
  if (list.isPending) return <LoadingState message={t("Loading brand profiles...")} />;
  if (list.isError)
    return (
      <section role="alert">
        <h2>{t("Brand profiles")}</h2>
        <p>{t(brandErrorKey(list.error))}</p>
        <Button onClick={() => void list.refetch()}>{t("Retry")}</Button>
        {list.error instanceof ApiError && list.error.status === 401 && (
          <a href={officialLoginUrl()}>{t("Sign in")}</a>
        )}
      </section>
    );
  const create = (
    <Dialog
      open={creating}
      onOpenChange={setCreating}
      title={t("Create profile")}
      trigger={<Button>{t("Create profile")}</Button>}
    >
      <BrandProfileForm
        onCancel={() => setCreating(false)}
        onSaved={(saved) => {
          setCreating(false);
          void client.invalidateQueries({ queryKey: ["oem-sites"] });
          props.onOpen(saved.id);
        }}
      />
    </Dialog>
  );
  return (
    <section>
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2>{t("Brand profiles")}</h2>
          <p>{t("Manage owned profiles, not live domains or gateway accounts.")}</p>
        </div>
        {list.data.length > 0 && create}
      </header>
      {list.data.length === 0 ? (
        <EmptyState
          bordered
          className="bg-card mt-5 rounded-xl"
          icon={Store}
          title={t("No brand profiles yet. Create your first profile.")}
          action={create}
        />
      ) : (
        <ul className="mt-5">
          {list.data.map((site) => (
            <li key={site.id}>
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <strong>{site.name}</strong>
                  {!ownsBrand(site) && (
                    <Badge variant="secondary">{t("Shared read-only access")}</Badge>
                  )}
                </div>
                <div className="cmh-muted">{site.support_email || t("No support email")}</div>
                <div className="flex items-center gap-1">
                  <span className="cmh-mono">
                    {t("Site ID")}: {site.id}
                  </span>
                  <CopyButton value={site.id} aria-label={t("Copy site ID")} />
                </div>
              </div>
              <Button variant="outline" onClick={() => props.onOpen(site.id)}>
                {ownsBrand(site) ? t("Manage") : t("View shared profile")}
              </Button>
            </li>
          ))}
        </ul>
      )}
      {list.data.length >= 100 && (
        <p className="cmh-muted mt-3">
          {t("The latest 100 profiles are shown; operation records load 50 at a time.")}
        </p>
      )}
    </section>
  );
}
