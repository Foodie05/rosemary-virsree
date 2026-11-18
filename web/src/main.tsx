import React from "react";
import { createRoot } from "react-dom/client";
import "@rosemary-ui/react/css";
import "./forma.css";
const DocumentPage = React.lazy(() =>
  import("./DocumentPage").then((module) => ({ default: module.DocumentPage })),
);
import App from "./App";
import { VersionGuard } from "./VersionGuard";
import { AppearanceProvider } from "./Forma";
createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <AppearanceProvider>
      <VersionGuard />
      {location.pathname.startsWith("/docs/") ? (
        <React.Suspense
          fallback={
            <div className="boot-screen" role="status">
              Loading…
            </div>
          }
        >
          <DocumentPage />
        </React.Suspense>
      ) : (
        <App />
      )}
    </AppearanceProvider>
  </React.StrictMode>,
);
