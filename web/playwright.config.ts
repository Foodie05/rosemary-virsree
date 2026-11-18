import { defineConfig } from "@playwright/test";
export default defineConfig({
  testDir: "./tests",
  fullyParallel: false,
  workers: 1,
  timeout: 30_000,
  use: {
    baseURL: "http://127.0.0.1:19112",
    viewport: { width: 1440, height: 1000 },
    locale: "zh-CN",
    launchOptions: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE
      ? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE }
      : {},
  },
  webServer: {
    command:
      "node node_modules/vite/bin/vite.js preview --host 127.0.0.1 --port 19112 --strictPort",
    url: "http://127.0.0.1:19112",
    reuseExistingServer: !process.env.CI,
  },
  reporter: [["list"]],
  outputDir: "test-results",
});
