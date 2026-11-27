import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { Dialog } from "@/components/dialog";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { api, type PreviewPage } from "~/lib/api";

const pages: { id: PreviewPage; label: string }[] = [
  { id: "home", label: "Home" },
  { id: "docs", label: "Docs" },
  { id: "login", label: "Sign-in" },
];

/**
 * Shows the storefront with the chosen theme before it is saved. The page is
 * rendered by the gateway and shown in a frame with every sandbox permission
 * withheld: no scripts, no same-origin access, no navigation of this console.
 */
export function BrandPreview(props: { siteID: string; theme: string; disabled?: boolean }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [page, setPage] = useState<PreviewPage>("home");
  const preview = useQuery({
    queryKey: ["oem-preview", props.siteID, props.theme, page],
    queryFn: () => api.brandPreview(props.siteID, props.theme, page),
    enabled: open,
    gcTime: 0,
  });
  let body = <p>{t("Loading...")}</p>;
  if (preview.isError) {
    body = (
      <div role="alert">
        <p>{t("Unable to load the preview.")}</p>
        <Button variant="outline" onClick={() => void preview.refetch()}>
          {t("Retry")}
        </Button>
      </div>
    );
  } else if (preview.data) {
    body = (
      <iframe
        title={t("Storefront preview")}
        sandbox=""
        srcDoc={preview.data.html}
        className="h-[560px] w-full rounded-md border bg-white"
      />
    );
  }
  return (
    <Dialog
      open={open}
      onOpenChange={setOpen}
      title={t("Storefront preview")}
      description={t(
        "The selected theme with the saved title, logo and profile. Nothing is saved from here.",
      )}
      contentClassName="sm:max-w-5xl"
      trigger={
        <Button type="button" variant="outline" disabled={props.disabled}>
          {t("Preview")}
        </Button>
      }
    >
      <Tabs value={page} onValueChange={(value) => setPage(value as PreviewPage)}>
        <TabsList>
          {pages.map((item) => (
            <TabsTrigger key={item.id} value={item.id}>
              {t(item.label)}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>
      <div className="mt-3">{body}</div>
    </Dialog>
  );
}
