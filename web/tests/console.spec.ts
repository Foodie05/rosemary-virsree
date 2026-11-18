import { test, expect } from "@playwright/test";
import type { Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { mkdir } from "node:fs/promises";
const GB = 2 ** 30;
const bucket = {
  id: "bucket_fixture",
  slug: "media-demo",
  name: "演示媒体库",
  deleting: false,
  visibility: "private",
  quota_bytes: 100 * GB,
  quota_unlimited: false,
  used_bytes: 5 * GB,
  reserved_bytes: 0,
  created_at: "2026-11-01T10:00:00Z",
};
const source = {
  id: "source_fixture",
  name: "主对象存储",
  kind: "s3",
  priority: 1,
  capacity_bytes: 1000 * GB,
  capacity_unlimited: false,
  used_bytes: 5 * GB,
  reserved_bytes: 0,
  enabled: true,
  direct_transfer: true,
  cdn_enabled: true,
  created_at: "2026-11-01T10:00:00Z",
};
const object = {
  id: "object_fixture",
  key: "说明书.pdf",
  size: 1048576,
  content_type: "application/pdf",
  updated_at: "2026-11-01T10:00:00Z",
};
async function fixture(
  page: Page,
  session = {
    authenticated: true,
    oidc_configured: true,
    setup_required: false,
    email: "admin@example.com",
  },
) {
  const requests: { path: string; method: string; body: any }[] = [];
  await page.route("**/api/**", async (route) => {
    const request = route.request(),
      url = new URL(request.url()),
      path = url.pathname,
      method = request.method();
    const body = request.postDataJSON();
    requests.push({ path, method, body });
    let data: any;
    let status = 200;
    if (path.endsWith("/upload-invitations") && method === "POST")
      data = {
        token: "rvs_au_fixture_NOT_REAL",
        bucket: bucket.slug,
        visibility: "private",
        shown_once: true,
        upload_url: "https://storage.example.com/api/v1/agent/uploads",
        invitation: {
          id: "inv_fixture",
          name: body.name,
          max_files: body.max_files,
          allow_overwrite: body.allow_overwrite,
          expires_at: "2099-01-01T00:00:00Z",
          used_files: 0,
          pending_files: 0,
          revoked: false,
        },
      };
    else if (path.endsWith("/upload-invitations"))
      data = [
        {
          id: "inv_fixture",
          name: "演示上传授权",
          max_files: 10,
          allow_overwrite: false,
          expires_at: "2099-01-01T00:00:00Z",
          used_files: 2,
          pending_files: 1,
          revoked: false,
        },
      ];
    else if (
      path.endsWith("/upload-invitations/inv_fixture") &&
      method === "DELETE"
    ) {
      data = {};
      status = 204;
    } else if (path.endsWith("/session")) data = session;
    else if (path.endsWith("/version")) data = { version: "dev" };
    else if (path.endsWith("/meta"))
      data = {
        project_url: "https://github.com/Foodie05/rosemary-virsree",
        release_url: "https://github.com/Foodie05/rosemary-virsree/releases",
      };
    else if (path.endsWith("/overview"))
      data = {
        bucket_count: 1,
        object_count: 2,
        active_key_count: 1,
        used_bytes: 5 * GB,
        reserved_bytes: 0,
        allocated_quota: 100 * GB,
        total_quota: 1000 * GB,
        backend_ready: true,
        primary_source_unlimited: false,
      };
    else if (path.endsWith("/deletion-confirmation"))
      data = {
        confirmation_token: "fixture-confirmation-not-a-secret",
        wait_seconds: 5,
      };
    else if (path === "/api/v1/admin/buckets/media-demo" && method === "DELETE")
      data = { accepted: true, status: "deleting" };
    else if (path === "/api/v1/admin/buckets/media-demo")
      data = {
        bucket,
        status: "ready",
        s3_endpoint: "https://storage.example.com/s3",
        api_endpoint: "https://storage.example.com/api/v1",
        region: "us-east-1",
        can_set_unlimited: false,
        metrics: {
          object_count: 2,
          active_key_count: 1,
          active_public_link_count: 0,
        },
        allocations: [
          {
            source_id: source.id,
            source_name: source.name,
            source_kind: "s3",
            object_count: 2,
            used_bytes: 5 * GB,
          },
        ],
      };
    else if (path.endsWith("/objects/download"))
      data = { url: "https://files.example.com/mock-direct-url", direct: true };
    else if (path.endsWith("/objects"))
      data = {
        entries: url.searchParams.get("prefix")
          ? [
              {
                key: url.searchParams.get("prefix") + "样例文件.png",
                folder: false,
                object: {
                  ...object,
                  key: url.searchParams.get("prefix") + "样例文件.png",
                },
              },
            ]
          : [
              { key: "images/", folder: true },
              { key: object.key, folder: false, object },
            ],
        next_cursor: "",
      };
    else if (path === "/api/v1/buckets") data = [bucket];
    else if (path === "/api/v1/access-keys")
      data = [
        {
          id: "key_fixture",
          name: "演示应用",
          bucket: bucket.slug,
          access_key: "RVS_DEMO_NOT_REAL",
          permissions: ["read", "write"],
          revoked: false,
        },
      ];
    else if (path === "/api/v1/storage-sources") data = [source];
    else if (path.endsWith("/source_fixture")) {
      if (method === "PUT") {
        await new Promise((resolve) => setTimeout(resolve, 400));
        data = { source, verified: true };
      } else
        data = {
          ...source,
          endpoint: "https://s3.example.com",
          public_endpoint: "",
          region: "us-east-1",
          bucket: "private-assets",
          cdn_endpoint: "https://cdn.example.com",
          cdn_mode: "bitiful_token",
          cdn_auth_key_configured: true,
          path_style: false,
        };
    } else if (path.endsWith("/platform-filesystem"))
      data = {
        initialized: true,
        source_name: source.name,
        prefix: "virsree-system/",
        backup_interval_seconds: 3600,
        retention: 24,
        last_success_at: "2026-11-01T10:00:00Z",
      };
    else {
      status = 404;
      data = { error: "Unsupported fixture endpoint" };
    }
    await route.fulfill({
      status,
      contentType: "application/json",
      body: status === 204 ? undefined : JSON.stringify(data),
    });
  });
  return requests;
}
async function noOverflow(page: Page) {
  expect(
    await page.evaluate(() => ({
      document: document.documentElement.scrollWidth <= innerWidth,
      main: [...document.querySelectorAll(".app-main,.content,.oobe")].every(
        (e) => e.scrollWidth <= e.clientWidth + 1,
      ),
    })),
  ).toEqual({ document: true, main: true });
}
async function accessible(page: Page) {
  await page.evaluate(() =>
    Promise.all(
      document
        .getAnimations()
        .filter((a) => a.effect?.getTiming().iterations !== Infinity)
        .map((a) => a.finished.catch(() => {})),
    ),
  );
  const results = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
    .analyze();
  expect(
    results.violations.map((v) => ({
      id: v.id,
      nodes: v.nodes.map((n) => n.target),
    })),
  ).toEqual([]);
}
test("desktop pages follow the official light/dark roles and remain accessible", async ({
  page,
}) => {
  await fixture(page);
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/");
  await expect(page.getByText("存储，一目了然。")).toBeVisible();
  expect(
    await page
      .locator(".app-sidebar .brand")
      .evaluate((e) => e.getBoundingClientRect().height),
  ).toBe(76);
  expect(
    await page
      .locator(".app-topbar")
      .evaluate((e) => e.getBoundingClientRect().height),
  ).toBe(76);
  for (const dark of [false, true]) {
    if (dark)
      await page.getByRole("button", { name: "切换为深色外观" }).click();
    for (const route of [
      "overview",
      "buckets",
      "buckets/media-demo",
      "storage",
      "access",
      "agent",
      "docs",
      "settings",
    ]) {
      await page.goto("/#/" + route);
      await expect(page.locator(".content")).toBeVisible();
      await expect(page.locator(".skeletons")).toHaveCount(0);
      await noOverflow(page);
      await accessible(page);
      if (["overview", "buckets/media-demo", "storage"].includes(route)) {
        await mkdir("../.local/forma-qa", { recursive: true });
        await page.screenshot({
          path: `../.local/forma-qa/${dark ? "dark" : "light"}-${route.replaceAll("/", "-")}.png`,
        });
      }
    }
  }
  expect(errors).toEqual([]);
  await page.reload();
  await expect(page.locator('[data-rm-theme="dark"]')).toBeVisible();
});
test("storage edit validates again and uses themed risk confirmation", async ({
  page,
}) => {
  const requests = await fixture(page);
  await page.goto("/#/storage");
  await page.getByRole("button", { name: "编辑主对象存储" }).click();
  const edit = page.getByRole("dialog", { name: "编辑存储源", exact: true });
  await expect(edit).toBeVisible();
  await expect(edit.getByLabel("可用容量（GB）")).toHaveCount(1);
  await expect(edit.getByLabel("设置为无限容量")).toHaveCount(1);
  await edit.getByLabel("真实私有桶名称").fill("changed-private-assets");
  await edit
    .getByRole("button", { name: "重新验证并保存", exact: true })
    .click();
  const risk = page.getByRole("dialog", {
    name: "确认更换真实存储桶",
    exact: true,
  });
  await expect(risk).toBeVisible();
  await accessible(page);
  await risk.getByRole("button", { name: "返回检查" }).click();
  await expect(risk).toHaveCount(0);
  await expect(edit.getByLabel("真实私有桶名称")).toHaveValue(
    "changed-private-assets",
  );
  await edit
    .getByRole("button", { name: "重新验证并保存", exact: true })
    .click();
  await page
    .getByRole("button", { name: "理解风险，重新验证", exact: true })
    .click();
  await expect(
    edit.getByRole("button", { name: "重新验证并保存", exact: true }),
  ).toHaveAttribute("aria-busy", "true");
  await page.keyboard.press("Escape");
  await expect(edit).toBeVisible();
  await expect(edit).toHaveCount(0);
  expect(
    requests.find((r) => r.method === "PUT")?.body.acknowledge_bucket_change,
  ).toBe(true);
  await expect(
    page.getByRole("button", { name: "编辑主对象存储" }),
  ).toBeFocused();
});
test("bucket deletion keeps the 5s gate and exact-name protection", async ({
  page,
}) => {
  const requests = await fixture(page);
  await page.goto("/#/buckets/media-demo");
  await page.getByRole("button", { name: "删除桶", exact: true }).click();
  const risk = page.getByRole("dialog");
  await expect(risk.getByRole("button", { name: /请阅读后果/ })).toBeDisabled();
  await expect(
    risk.getByRole("button", { name: "我已了解，继续" }),
  ).toBeEnabled({ timeout: 7000 });
  await risk.getByRole("button", { name: "我已了解，继续" }).click();
  await expect(risk.getByLabel("完整桶名", { exact: true })).toBeFocused();
  await risk.getByLabel("完整桶名", { exact: true }).fill("media-demo ");
  await expect(
    risk.getByRole("button", { name: "永久删除桶", exact: true }),
  ).toBeDisabled();
  await risk.getByLabel("完整桶名", { exact: true }).blur();
  await expect(risk.getByLabel("完整桶名", { exact: true })).toHaveAttribute(
    "aria-invalid",
    "true",
  );
  await risk.getByLabel("完整桶名", { exact: true }).fill("");
  await expect(
    risk.getByLabel("完整桶名", { exact: true }),
  ).not.toHaveAttribute("aria-invalid", "true");
  await risk.getByLabel("完整桶名", { exact: true }).fill("media-demo");
  await risk.getByRole("button", { name: "永久删除桶", exact: true }).click();
  expect(requests.find((r) => r.method === "DELETE")?.body.confirm_name).toBe(
    "media-demo",
  );
  await expect(page.getByText("Arca 正在永久清理此桶")).toBeVisible();
});
test("object browsing, direct download dialog and browser history work", async ({
  page,
}) => {
  const requests = await fixture(page);
  await page.goto("/#/buckets");
  await page.getByRole("button", { name: "管理", exact: true }).click();
  await expect(page).toHaveURL(/#\/buckets\/media-demo$/);
  await page.getByRole("button", { name: "images", exact: true }).click();
  await expect(page.getByText("样例文件.png", { exact: true })).toBeVisible();
  await page
    .getByRole("button", { name: "下载 样例文件.png", exact: true })
    .click();
  await accessible(page);
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page
    .getByRole("button", { name: "下载 样例文件.png", exact: true })
    .click();
  await page.getByLabel("链接有效期（秒）").fill("3600");
  await page.getByRole("button", { name: "生成下载地址", exact: true }).click();
  await expect(page.getByRole("link", { name: "打开下载" })).toHaveAttribute(
    "href",
    "https://files.example.com/mock-direct-url",
  );
  expect(
    requests.find((r) => r.path.endsWith("/objects/download"))?.body.expires_in,
  ).toBe(3600);
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.goBack();
  await expect(page.locator(".section-head h2")).toHaveText("虚拟桶");
});
for (const width of [320, 390, 768])
  test(`mobile ${width}px navigation, forms and reduced motion`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 844 });
    await page.emulateMedia({ reducedMotion: "reduce", colorScheme: "dark" });
    await fixture(page);
    await page.goto("/#/overview");
    await expect(
      page.getByRole("button", { name: "切换为浅色外观" }),
    ).toBeVisible();
    await page.getByRole("button", { name: "打开主导航" }).click();
    const drawer = page.getByRole("dialog", { name: "Arca 导航" });
    await expect(drawer).toBeVisible();
    expect(await drawer.evaluate((e) => e.getBoundingClientRect().top)).toBe(
      76,
    );
    await drawer.getByRole("link", { name: "存储源" }).click();
    await expect(drawer).not.toBeVisible();
    await expect(
      page.getByRole("heading", { name: "存储源", exact: true, level: 2 }),
    ).toBeVisible();
    await noOverflow(page);
    await accessible(page);
    await mkdir("../.local/forma-qa", { recursive: true });
    await page.screenshot({ path: `../.local/forma-qa/mobile-${width}.png` });
    await page.getByRole("button", { name: "编辑主对象存储" }).click();
    await noOverflow(page);
    await accessible(page);
    await page.keyboard.press("Escape");
    await page.goto("/#/buckets/media-demo");
    await expect(page.getByText("说明书.pdf", { exact: true })).toBeVisible();
    await noOverflow(page);
    await accessible(page);
    await page.goto("/#/docs/upload");
    await expect(
      page.getByRole("heading", { name: "上传：申请地址 → PUT → commit" }),
    ).toBeVisible();
    await noOverflow(page);
    expect(
      await page
        .locator(".content")
        .evaluate((e) => getComputedStyle(e).animationName),
    ).toBe("none");
  });
test("login and OOBE retain brand, theme and valid protocol controls", async ({
  page,
}) => {
  await fixture(page, {
    authenticated: false,
    oidc_configured: true,
    setup_required: false,
    email: "",
  });
  await page.goto("/?auth_error=unsafe-raw-provider-detail");
  await expect(page.getByText("unsafe-raw-provider-detail")).toHaveCount(0);
  await accessible(page);
  await page.unroute("**/api/**");
  await fixture(page, {
    authenticated: true,
    oidc_configured: true,
    setup_required: true,
    email: "admin@example.com",
  });
  await page.goto("/");
  await expect(page.getByText("WELCOME TO ARCA")).toBeVisible();
  await accessible(page);
  await page.getByRole("button", { name: "开始配置" }).click();
  await page.getByRole("tab", { name: "WebDAV", exact: true }).click();
  await expect(page.getByLabel("WebDAV Endpoint")).toBeVisible();
  await expect(page.getByLabel("设置为无限容量")).toHaveCount(1);
  await accessible(page);
});

test("live system appearance, forced colors and interrupted drawer preserve state", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.emulateMedia({ colorScheme: "light" });
  await fixture(page);
  await page.goto("/#/overview");
  await expect(page.locator(".app-main")).toHaveCSS(
    "background-color",
    "rgb(254, 247, 252)",
  );
  await page.emulateMedia({ colorScheme: "dark" });
  await expect(
    page.getByRole("button", { name: "切换为浅色外观" }),
  ).toBeVisible();
  await expect(page.locator(".app-main")).toHaveCSS(
    "background-color",
    "rgb(33, 31, 34)",
  );
  await page.getByRole("button", { name: "切换为浅色外观" }).click();
  await page.emulateMedia({ colorScheme: "dark" });
  await expect(page.locator('[data-rm-theme="light"]')).toBeVisible();
  const headerY = await page
    .locator(".app-topbar")
    .evaluate((e) => e.getBoundingClientRect().y);
  for (let i = 0; i < 3; i++) {
    await page.getByRole("button", { name: "打开主导航" }).click();
    await page.keyboard.press("Escape");
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(
      page.getByRole("button", { name: "打开主导航" }),
    ).toBeFocused();
    expect(
      await page
        .locator(".app-topbar")
        .evaluate((e) => e.getBoundingClientRect().y),
    ).toBe(headerY);
  }
  await page.emulateMedia({ forcedColors: "active", reducedMotion: "reduce" });
  await page.goto("/#/storage");
  await page.getByRole("button", { name: "编辑主对象存储" }).click();
  const check = page.getByLabel("设置为无限容量");
  await check.check();
  await expect(check).toBeChecked();
  await expect(
    page
      .locator(".rm-check")
      .filter({ has: check })
      .locator(".rm-check-visual"),
  ).toHaveCount(1);
  await expect(page.getByLabel("内部 API Endpoint")).toHaveCSS(
    "outline-style",
    "none",
  );
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
});

test("temporary upload prompts preserve policy, copy once, and revoke with the unified dialog", async ({
  page,
}) => {
  const requests = await fixture(page);
  await page.goto("/#/buckets/media-demo");
  await expect(
    page.getByRole("heading", { name: "临时 Agent 上传" }),
  ).toBeVisible();
  await expect(page.getByText("演示上传授权", { exact: true })).toBeVisible();
  await page
    .getByRole("button", { name: "生成上传提示词", exact: true })
    .click();
  let dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  await dialog
    .getByRole("textbox", { name: "授权名称" })
    .fill("Release assets");
  await dialog.getByRole("checkbox", { name: "不限文件数量" }).check();
  await dialog.getByRole("button", { name: "生成提示词", exact: true }).click();
  dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("仅显示这一次");
  await expect(dialog.locator("pre")).toContainText("rvs_au_fixture_NOT_REAL");
  await expect(dialog.locator("pre")).toContainText(
    "https://storage.example.com/docs/api.md?raw=1",
  );
  await expect(dialog.locator("pre")).toContainText('"max_files": 0');
  const create = requests.find(
    (r) => r.path.endsWith("/upload-invitations") && r.method === "POST",
  );
  expect(create?.body).toEqual({
    name: "Release assets",
    max_files: 0,
    expires_in: 3600,
    allow_overwrite: false,
  });
  await page.keyboard.press("Escape");
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "我已保存提示词" }).click();
  await expect(dialog).toHaveCount(0);
  await page.getByRole("button", { name: "撤销授权", exact: true }).click();
  dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("已经提交的文件");
  await dialog.getByRole("button", { name: "撤销授权", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect(
    requests.some(
      (r) =>
        r.path.endsWith("/upload-invitations/inv_fixture") &&
        r.method === "DELETE",
    ),
  ).toBe(true);
});

test("language switching preserves the current route and system preference", async ({
  page,
}) => {
  await fixture(page);
  await page.goto("/#/buckets/media-demo");
  await page.getByRole("button", { name: "EN — Switch to English" }).click();
  await expect(page.locator("html")).toHaveAttribute("lang", "en");
  await expect(
    page.getByRole("heading", { name: "Temporary Agent uploads" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Create upload prompt" }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Virtual buckets", exact: true }),
  ).toBeVisible();
  await expect(page).toHaveURL(/#\/buckets\/media-demo$/);
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("lang", "en");
  await page.getByRole("button", { name: "切换为中文" }).click();
  await expect(
    page.getByRole("heading", { name: "临时 Agent 上传" }),
  ).toBeVisible();
});

test("technical document reader displays GFM safely and offers raw source", async ({
  page,
}) => {
  await fixture(page);
  await page.route("**/docs/api.md?raw=1", (route) =>
    route.fulfill({
      contentType: "text/markdown",
      body: "# Gateway API\n\n## Upload invitations\n\n| Method | Route |\n| --- | --- |\n| POST | `/api/v1/agent/uploads` |\n\n```sh\ncurl https://storage.example.com/health\n```\n\n<script>window.readerXSS=true</script>",
    }),
  );
  await page.goto("/docs/api.md");
  await expect(
    page.getByRole("heading", { name: "Gateway API" }),
  ).toBeVisible();
  await expect(page.getByRole("table")).toContainText("/api/v1/agent/uploads");
  await expect(
    page.getByRole("link", { name: "原始 Markdown" }),
  ).toHaveAttribute("href", "/docs/api.md?raw=1");
  await expect(page.getByRole("link", { name: "下载文档" })).toHaveAttribute(
    "href",
    "/docs/api.md?raw=1&download=1",
  );
  expect(await page.evaluate(() => Boolean((window as any).readerXSS))).toBe(
    false,
  );
  await noOverflow(page);
  await page.setViewportSize({ width: 390, height: 844 });
  await noOverflow(page);
  const violations = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"])
    .analyze();
  expect(violations.violations).toEqual([]);
});

test("late invitation refreshes cannot display another bucket's authorizations", async ({
  page,
}) => {
  await fixture(page);
  const second = {
    ...bucket,
    id: "bucket_second",
    slug: "second-bucket",
    name: "第二个桶",
  };
  await page.route("**/api/v1/buckets", (route) =>
    route.fulfill({ json: [bucket, second] }),
  );
  await page.route("**/api/v1/admin/buckets/second-bucket", (route) =>
    route.fulfill({
      json: {
        bucket: second,
        status: "ready",
        s3_endpoint: "https://storage.example.com/s3",
        api_endpoint: "https://storage.example.com/api/v1",
        region: "us-east-1",
        can_set_unlimited: false,
        metrics: {
          object_count: 0,
          active_key_count: 0,
          active_public_link_count: 0,
        },
        allocations: [],
      },
    }),
  );
  let releaseA: () => void = () => {},
    releaseB: () => void = () => {};
  let countA = 0;
  await page.route("**/upload-invitations", async (route) => {
    const second = route.request().url().includes("second-bucket");
    if (second)
      await new Promise<void>((resolve) => {
        releaseB = resolve;
      });
    else if (++countA > 1)
      await new Promise<void>((resolve) => {
        releaseA = resolve;
      });
    await route.fulfill({
      json: [
        {
          id: second ? "second" : "first",
          name: second ? "B 的上传授权" : "A 的上传授权",
          max_files: 2,
          used_files: 0,
          pending_files: 0,
          revoked: false,
          expires_at: "2099-01-01T00:00:00Z",
          allow_overwrite: false,
        },
      ],
    });
  });
  await page.goto("/#/buckets/media-demo");
  await expect(page.getByText("A 的上传授权", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "刷新上传授权" }).click();
  await expect.poll(() => countA).toBe(2);
  await page.evaluate(() => {
    location.hash = "/buckets/second-bucket";
  });
  await expect(page.getByRole("heading", { name: "第二个桶" })).toBeVisible();
  await expect(page.getByText("A 的上传授权", { exact: true })).toHaveCount(0);
  releaseB();
  await expect(page.getByText("B 的上传授权", { exact: true })).toBeVisible();
  releaseA();
  await page.waitForTimeout(150);
  await expect(page.getByText("B 的上传授权", { exact: true })).toBeVisible();
  await expect(page.getByText("A 的上传授权", { exact: true })).toHaveCount(0);
});
