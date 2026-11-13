import { createRouter, RouterProvider } from "@tanstack/react-router";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import "./i18n";
import { routeTree } from "./routeTree.gen";

// The app lives under /cmh/ (ext serves it there); basepath keeps the file
// routes written as "/" and "/whoami".
const router = createRouter({ routeTree, basepath: "/cmh" });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <RouterProvider router={router} />
  </StrictMode>,
);
