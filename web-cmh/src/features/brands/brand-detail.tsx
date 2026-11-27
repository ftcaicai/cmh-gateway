import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft } from "lucide-react";
import { useTranslation } from "react-i18next";

import { CopyButton } from "@/components/copy-button";
import { LoadingState } from "@/components/loading-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { formatTimestampToDate } from "@/lib/format";
import { api, ownsBrand, type BrandSite } from "~/lib/api";
import { usePagedList } from "~/lib/use-paged-list";

import { BrandAppearance } from "./brand-appearance";
import { BrandDomain } from "./brand-domain";
import { brandErrorKey, BrandProfileForm } from "./brand-profile-form";
import { BrandStatus } from "./brand-status";

const auditLabels: Record<string, string> = {
  "site.created": "Profile created",
  "site.updated": "Profile updated",
  "site.domain_set": "Domain saved",
  "site.domain_verified": "Domain verified",
  "site.domain_lapsed": "Domain CNAME no longer matches",
  "site.paused": "Storefront paused",
  "site.resumed": "Storefront resumed",
  "site.logo_restored": "Logo restored",
  "site.domain_cleared": "Domain removed",
  "site.appearance_updated": "Appearance updated",
  "site.logo_updated": "Logo updated",
  "site.logo_removed": "Logo removed",
};

/** One brand: its profile, and for the owner its appearance, domain and status. */
export function BrandDetail(props: { siteID: string; onBack: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const key = ["oem-site", props.siteID];
  const site = useQuery({ queryKey: key, queryFn: () => api.brand(props.siteID), gcTime: 0 });
  const back = (
    <Button variant="ghost" size="sm" className="-ml-2" onClick={props.onBack}>
      <ArrowLeft aria-hidden="true" />
      {t("Brand profiles")}
    </Button>
  );
  if (site.isPending) return <LoadingState message={t("Loading brand profiles...")} />;
  if (site.isError)
    return (
      <section>
        {back}
        <div role="alert">
          <p>{t(brandErrorKey(site.error))}</p>
          <Button variant="outline" onClick={() => void site.refetch()}>
            {t("Retry")}
          </Button>
        </div>
      </section>
    );
  const current = site.data;
  const owner = ownsBrand(current);
  return (
    <section>
      {back}
      <header className="mt-2">
        <div className="flex flex-wrap items-center gap-2">
          <h2>{current.name}</h2>
          {!owner && <Badge variant="secondary">{t("Shared read-only access")}</Badge>}
          {owner && <StatusBadges siteID={current.id} />}
        </div>
        <div className="mt-1 flex items-center gap-1">
          <span className="cmh-mono">
            {t("Site ID")}: {current.id}
          </span>
          <CopyButton value={current.id} aria-label={t("Copy site ID")} />
        </div>
      </header>
      <Tabs defaultValue="profile" className="mt-5">
        <TabsList className="max-w-full overflow-x-auto">
          <TabsTrigger value="profile">{t("Profile")}</TabsTrigger>
          {owner && <TabsTrigger value="appearance">{t("Appearance and SEO")}</TabsTrigger>}
          {owner && <TabsTrigger value="domain">{t("Custom domain")}</TabsTrigger>}
          {owner && <TabsTrigger value="status">{t("Storefront status")}</TabsTrigger>}
          <TabsTrigger value="records">{t("Operation records")}</TabsTrigger>
        </TabsList>
        <TabsContent value="profile">
          <section aria-label={t("Profile")}>
            {owner ? (
              <div className="max-w-xl">
                <BrandProfileForm
                  site={current}
                  onSaved={(saved: BrandSite) => {
                    client.setQueryData(key, saved);
                    void client.invalidateQueries({ queryKey: ["oem-sites"] });
                    void client.invalidateQueries({ queryKey: ["oem-audits", saved.id] });
                  }}
                />
              </div>
            ) : (
              <dl className="mt-0 border-0 p-0">
                <dt>{t("Profile name")}</dt>
                <dd>{current.name}</dd>
                <dt>{t("Support email")}</dt>
                <dd>{current.support_email || t("No support email")}</dd>
                <dt>{t("Description")}</dt>
                <dd>{current.description || "-"}</dd>
              </dl>
            )}
          </section>
        </TabsContent>
        {/* Owner panels stay mounted so an unsaved edit survives a tab switch. */}
        {owner && (
          <TabsContent value="appearance" keepMounted>
            <section aria-label={t("Appearance and SEO")}>
              <BrandAppearance siteID={current.id} />
            </section>
          </TabsContent>
        )}
        {owner && (
          <TabsContent value="domain" keepMounted>
            <section aria-label={t("Custom domain")}>
              <BrandDomain siteID={current.id} />
            </section>
          </TabsContent>
        )}
        {owner && (
          <TabsContent value="status">
            <section aria-label={t("Storefront status")}>
              <BrandStatus siteID={current.id} />
            </section>
          </TabsContent>
        )}
        <TabsContent value="records">
          <section aria-label={t("Operation records")}>
            <Records siteID={current.id} />
          </section>
        </TabsContent>
      </Tabs>
    </section>
  );
}

/** Live storefront state next to the brand name; shares the panels' queries. */
function StatusBadges({ siteID }: { siteID: string }) {
  const { t } = useTranslation();
  const pause = useQuery({
    queryKey: ["oem-pause", siteID],
    queryFn: () => api.brandPause(siteID),
    gcTime: 0,
  });
  const domain = useQuery({
    queryKey: ["oem-domain", siteID],
    queryFn: () => api.brandDomain(siteID),
    gcTime: 0,
  });
  return (
    <>
      {pause.data && (
        <Badge variant={pause.data.paused ? "secondary" : "default"}>
          {pause.data.paused ? t("Paused") : t("Online")}
        </Badge>
      )}
      {domain.data?.host && (
        <Badge variant="outline">
          {domain.data.host} · {domain.data.verified ? t("Live") : t("Waiting for DNS")}
        </Badge>
      )}
    </>
  );
}

function Records({ siteID }: { siteID: string }) {
  const { t } = useTranslation();
  const audits = usePagedList(["oem-audits", siteID], (page) => api.brandAudits(siteID, page));
  if (audits.isPending) return <p>{t("Loading operation records...")}</p>;
  if (audits.isError)
    return (
      <div role="alert">
        <p>{t(brandErrorKey(audits.error))}</p>
        <Button variant="outline" onClick={() => void audits.refetch()}>
          {t("Retry")}
        </Button>
      </div>
    );
  if (audits.items?.length === 0) return <p>{t("No operation records.")}</p>;
  return (
    <>
      <ul className="mt-0">
        {audits.items?.map((record) => (
          <li key={record.id}>
            <span>{t(auditLabels[record.action] ?? "Profile updated")}</span>
            <span className="cmh-muted">{formatTimestampToDate(record.created_at)}</span>
          </li>
        ))}
      </ul>
      {audits.hasMore && (
        <Button
          className="mt-3"
          variant="outline"
          onClick={audits.loadMore}
          disabled={audits.isLoadingMore}
        >
          {t("Load more")}
        </Button>
      )}
    </>
  );
}
