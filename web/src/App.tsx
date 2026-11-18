import { msg, msgt } from "./locale";
import { useLanguage } from "./locale";
import {
  Button,
  Input,
  Select,
  Checkbox,
  Modal as FormaModal,
  clearCompletedOverlayMotion,
  ThemeToggle,
  IconButton,
  Loading as FoldLoading,
  Skeleton,
  Progress,
} from "./Forma";
import { useEffect, useMemo, useState } from "react";
import { Drawer } from "@rosemary-ui/react";
import {
  Activity,
  BookOpen,
  Box,
  ChevronRight,
  Cloud,
  Copy,
  Database,
  KeyRound,
  LayoutDashboard,
  Loader2,
  LogOut,
  Pencil,
  Plus,
  RefreshCw,
  ShieldCheck,
  Sparkles,
  Trash2,
  X,
} from "./icons";
import {
  API,
  APIError,
  AccessKey,
  Bucket,
  BucketDetail,
  Overview,
  Session,
} from "./api";
import { Brand } from "./Brand";
import { buildAgentPrompt } from "./agentPrompt";
import { FullDocsPage } from "./FullDocsPage";
import { OOBE, StoragePage } from "./StorageSetup";
import { RiskDialog } from "./RiskDialog";
import { DeleteBucket } from "./DeleteBucket";
import { UploadInvitations } from "./UploadInvitations";
import { ObjectExplorer } from "./ObjectExplorer";
import { APP_VERSION, hasVersionHold, useVersionHold } from "./VersionGuard";
type Page =
  | "overview"
  | "buckets"
  | "storage"
  | "access"
  | "agent"
  | "docs"
  | "settings";
const fmt = (n = 0) =>
  n >= 1 << 30
    ? `${(n / (1 << 30)).toFixed(1)} GB`
    : n >= 1 << 20
      ? `${(n / (1 << 20)).toFixed(1)} MB`
      : `${n} B`;
const nav = [
  ["overview", "总览", LayoutDashboard],
  ["buckets", "虚拟桶", Box],
  ["storage", "存储源", Database],
  ["access", "访问控制", KeyRound],
  ["agent", "Agent 接入", Sparkles],
  ["docs", "接入文档", BookOpen],
  ["settings", "平台设置", ShieldCheck],
] as const;
const safeDecode = (value: string) => {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
};
type Route = {
  page: Page;
  bucket?: string;
  section?: string;
};
function readRoute(): Route {
  const [page, bucket] = location.hash.replace(/^#\/?/, "").split("/");
  if (nav.some((item) => item[0] === page))
    return {
      page: page as Page,
      ...(page === "buckets"
        ? {
            bucket: bucket ? safeDecode(bucket) : undefined,
          }
        : {
            section: bucket,
          }),
    };
  if (
    [
      "start",
      "install",
      "claim",
      "configure",
      "upload",
      "download",
      "permissions",
      "verify",
      "errors",
    ].includes(page)
  )
    return {
      page: "docs",
      section: page,
    };
  return {
    page: "overview",
  };
}
function routeTo(page: Page, bucket?: string) {
  location.hash = `/${page}${bucket ? "/" + encodeURIComponent(bucket) : ""}`;
}
export default function App() {
  const language = useLanguage();
  const [session, setSession] = useState<Session>();
  const [sessionError, setSessionError] = useState("");
  const [route, setRoute] = useState<Route>(readRoute);
  const [drawer, setDrawer] = useState(false);
  const [notice, setNotice] = useState("");
  const page = route.page;
  useEffect(() => {
    if (!notice) return;
    const timer = setTimeout(() => setNotice(""), 6000);
    return () => clearTimeout(timer);
  }, [notice]);
  useEffect(() => {
    let acceptedHash = location.hash;
    const change = () => {
      if (hasVersionHold()) {
        history.replaceState(
          null,
          "",
          location.pathname + location.search + acceptedHash,
        );
        setNotice(msg("请先完成或关闭当前操作，再离开此页面"));
        return;
      }
      acceptedHash = location.hash;
      setRoute(readRoute());
      document.querySelector(".app-main")?.scrollTo({
        top: 0,
      });
    };
    window.addEventListener("hashchange", change);
    return () => window.removeEventListener("hashchange", change);
  }, []);
  useEffect(() => {
    document.title = `${msg(nav.find((n) => n[0] === page)?.[1] || "管理台")} · Arca`;
  }, [page, language]);
  const loadSession = () =>
    new API()
      .session()
      .then((value) => {
        setSession(value);
        setSessionError("");
      })
      .catch((e) => setSessionError(e.message || msg("无法连接管理服务")));
  useEffect(() => {
    loadSession();
    const unauthorized = () => loadSession();
    window.addEventListener("rvs-unauthorized", unauthorized);
    return () => window.removeEventListener("rvs-unauthorized", unauthorized);
  }, []);
  if (!session)
    return sessionError ? (
      <div className="boot-screen">
        <ThemeToggle />
        <Brand />
        <ErrorBox text={sessionError} retry={loadSession} />
      </div>
    ) : (
      <BootScreen />
    );
  if (!session.authenticated)
    return <Login oidcReady={session.oidc_configured} />;
  if (session.setup_required)
    return <OOBE email={session.email} onComplete={loadSession} />;
  const navigation = (
    <>
      <Brand />
      <p className="nav-caption">{msg("工作空间")}</p>
      <nav aria-label={msg("主导航")}>
        {nav.map(([id, label, Icon]) => (
          <a
            key={id}
            className={page === id ? "active" : ""}
            href={`#/${id}`}
            aria-current={page === id ? "page" : undefined}
            onClick={(event) => {
              if (hasVersionHold()) {
                event.preventDefault();
                setNotice(msg("请先完成或关闭当前操作，再切换页面"));
                return;
              }
              setDrawer(false);
            }}
          >
            <Icon />
            <span>{msg(label)}</span>
            {page === id && <ChevronRight className="chev" />}
          </a>
        ))}
      </nav>
      <div className="side-foot">
        <div className="identity">
          <span>{session.email}</span>
          <small>
            {msg("OIDC 管理员 · v")}
            {APP_VERSION}
          </small>
        </div>
        <Button
          type="button"
          onClick={async () => {
            if (hasVersionHold()) {
              setNotice(msg("请先安全保存当前内容，再退出"));
              return;
            }
            try {
              await new API().logout();
              setSession({
                ...session,
                authenticated: false,
              });
            } catch {
              setNotice(msg("退出失败，请检查网络后重试"));
            }
          }}
        >
          <LogOut />
          {msg("退出管理台")}
        </Button>
      </div>
    </>
  );
  return (
    <div className="shell">
      <a
        className="skip-link"
        href="#main-content"
        onClick={(e) => {
          e.preventDefault();
          document.getElementById("main-content")?.focus();
        }}
      >
        {msg("跳到主要内容")}
      </a>
      <aside className="app-sidebar">{navigation}</aside>
      <main className="app-main" id="main-content" tabIndex={-1}>
        <Header page={page} onMenu={() => setDrawer(true)} />
        <section className="content" key={page}>
          {page === "overview" && <OverviewPage token="" />}
          {page === "buckets" && (
            <BucketsPage
              token=""
              routeBucket={route.bucket}
              notify={setNotice}
            />
          )}{" "}
          {page === "storage" && <StoragePage notify={setNotice} />}{" "}
          {page === "access" && <AccessPage token="" />}{" "}
          {page === "agent" && <AgentPage token="" notify={setNotice} />}{" "}
          {page === "docs" && <FullDocsPage section={route.section} />}{" "}
          {page === "settings" && <SettingsPage token="" />}
        </section>
        <footer className="app-footer">
          Arca <span>by Rosemary · Forma 1.3</span>
          <a href="#/docs">
            {msg("接入指南 ")}
            <ChevronRight />
          </a>
        </footer>
      </main>
      <Drawer
        open={drawer}
        onOpenChange={setDrawer}
        title={msg("Arca 导航")}
        onAnimationComplete={(open) => {
          if (open) clearCompletedOverlayMotion();
        }}
        className="navigation-drawer"
      >
        {navigation}
      </Drawer>
      {notice && (
        <div className="toast" role="status" aria-live="polite">
          <ShieldCheck />
          {notice}
          <IconButton
            icon="close"
            label={msg("关闭通知")}
            variant="ghost"
            onClick={() => setNotice("")}
          />
        </div>
      )}
    </div>
  );
}
function BootScreen() {
  return (
    <div className="boot-screen">
      <ThemeToggle />
      <Brand />
      <FoldLoading label={msg("正在连接管理服务")} size="large" />
      <p>{msg("正在连接 Arca 管理服务…")}</p>
    </div>
  );
}
function Login({ oidcReady }: { oidcReady: boolean }) {
  const authError = new URLSearchParams(location.search).has("auth_error");
  return (
    <div className="login">
      <div className="login-art">
        <Brand />
        <div>
          <p className="eyebrow">{msg("ARCA / 对象存储网关")}</p>
          <h1>
            {msg("应用的存储空间，")}
            <br />
            {msg("在这里井然有序。")}
          </h1>
          <p>
            {msg(
              "把私有存储连接到应用。集中管理权限与容量，让文件直接抵达目的地。",
            )}
          </p>
        </div>
        <div className="route-line">
          <span>{msg("应用")}</span>
          <ChevronRight />
          <span>Arca</span>
          <ChevronRight />
          <span>{msg("私有存储")}</span>
        </div>
      </div>
      <section className="login-panel">
        <ThemeToggle />
        <div>
          <p className="eyebrow">{msg("管理控制台")}</p>
          <h2>{msg("欢迎使用 Arca")}</h2>
          <p>{msg("使用管理员白名单内的邮箱登录。")}</p>
        </div>
        {authError && (
          <div className="form-error" role="alert">
            {msg("登录未能完成。请检查是否使用了允许的邮箱，然后重试。")}
          </div>
        )}
        <a
          className={`primary oidc-button ${oidcReady ? "" : "disabled"}`}
          aria-disabled={!oidcReady}
          href={oidcReady ? "/auth/login" : undefined}
        >
          {msg("使用 OIDC 登录 ")}
          <ChevronRight />
        </a>
        <small>
          {oidcReady
            ? msg("由身份服务验证，Arca 不保存你的密码。")
            : msg("OIDC 尚未配置，请联系部署管理员。")}
        </small>
      </section>
    </div>
  );
}
function Header({ page, onMenu }: { page: Page; onMenu: () => void }) {
  const title = nav.find((n) => n[0] === page)?.[1];
  return (
    <header className="app-topbar">
      <div className="topbar-inner">
        <div className="topbar-location">
          <IconButton
            className="mobile-menu"
            icon="menu"
            label={msg("打开主导航")}
            variant="ghost"
            onClick={onMenu}
          />
          <span className="crumb">Arca</span>
          <ChevronRight />
          <h1>{msg(title || "")}</h1>
        </div>
        <div className="header-actions">
          <span className="gateway">
            <Activity />
            {msg("集中网关")}
          </span>
          <ThemeToggle />
        </div>
      </div>
    </header>
  );
}
function useData(token: string, kind: "overview" | "buckets" | "access") {
  const api = useMemo(() => new API(token), [token]);
  const [overview, setOverview] = useState<Overview>();
  const [buckets, setBuckets] = useState<Bucket[]>([]);
  const [keys, setKeys] = useState<AccessKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const reload = () => {
    setError("");
    const requests =
      kind === "overview"
        ? [api.overview()]
        : kind === "buckets"
          ? [api.overview(), api.buckets()]
          : [api.buckets(), api.keys()];
    Promise.all(requests)
      .then((values) => {
        if (kind === "overview") setOverview(values[0] as Overview);
        if (kind === "buckets") {
          setOverview(values[0] as Overview);
          setBuckets((values[1] as Bucket[]) || []);
        }
        if (kind === "access") {
          setBuckets((values[0] as Bucket[]) || []);
          setKeys((values[1] as AccessKey[]) || []);
        }
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  };
  useEffect(reload, [api, kind]);
  return {
    api,
    overview,
    buckets,
    keys,
    loading,
    error,
    reload,
  };
}
function Loading() {
  return (
    <div className="skeletons" aria-label={msg("正在读取平台数据")}>
      {[1, 2, 3, 4].map((id) => (
        <Skeleton key={id} lines={3} label={msg("正在加载内容")} />
      ))}
    </div>
  );
}
function ErrorBox({ text, retry }: { text: string; retry?: () => void }) {
  return (
    <div className="error" role="alert">
      <ShieldCheck />
      {msg("无法读取平台数据：")}
      {text}
      {retry && (
        <Button className="ghost" onClick={retry}>
          {msg("重试")}
        </Button>
      )}
    </div>
  );
}
function OverviewPage({ token }: { token: string }) {
  const d = useData(token, "overview");
  if (d.loading) return <Loading />;
  if (d.error) return <ErrorBox text={d.error} retry={d.reload} />;
  const o = d.overview!;
  const used = o.used_bytes + o.reserved_bytes;
  const pct = Math.min(100, o.total_quota ? (used / o.total_quota) * 100 : 0);
  return (
    <>
      <div className="hero">
        <div>
          <p className="eyebrow">CONTROL PLANE</p>
          <h2>{msg("存储，一目了然。")}</h2>
          <p>
            {msg(
              "集中管理应用的权限与空间。S3 文件通过签名链接直达私有存储，WebDAV 使用安全中转。",
            )}
          </p>
        </div>
        <Button
          className="icon-button"
          aria-label={msg("刷新平台数据")}
          onClick={d.reload}
        >
          <RefreshCw size={17} />
        </Button>
      </div>
      <div className="metrics">
        <Metric
          label={msg("虚拟桶")}
          value={o.bucket_count}
          note={msg("隔离命名空间")}
          icon={<Box />}
        />
        <Metric
          label={msg("已管理对象")}
          value={o.object_count}
          note={msg("映射关系有效")}
          icon={<Database />}
        />
        <Metric
          label={msg("活跃密钥")}
          value={o.active_key_count}
          note={msg("可独立撤销")}
          icon={<KeyRound />}
        />
        <Metric
          label={msg("后端状态")}
          value={o.backend_ready ? msg("可用") : msg("待配置")}
          note={msg("真实桶保持私有")}
          icon={<Cloud />}
        />
      </div>
      <div className="grid2">
        <article className="card capacity">
          <div className="card-title">
            <div>
              <span>{msg("平台总空间")}</span>
              <strong>
                {fmt(used)} <small>/ {fmt(o.total_quota)}</small>
              </strong>
            </div>
            <span>{pct.toFixed(1)}%</span>
          </div>
          <Progress
            label={msg("平台已用与预留容量")}
            value={pct}
            flowing={false}
          />
          <div className="legend">
            <span>
              <i className="used" />
              {msg("已使用 ")}
              {fmt(o.used_bytes)}
            </span>
            <span>
              <i className="reserved" />
              {msg("上传预留 ")}
              {fmt(o.reserved_bytes)}
            </span>
            <span>
              {msg("已分配 ")}
              {fmt(o.allocated_quota)}
            </span>
          </div>
        </article>
        <article className="card flow">
          <div className="card-title">
            <div>
              <span>{msg("请求路径")}</span>
              <strong>{msg("控制集中，数据直达")}</strong>
            </div>
          </div>
          <div className="flow-row">
            <div>
              {msg("应用程序")}
              <small>{msg("虚拟 AK/SK")}</small>
            </div>
            <i />
            <div className="accent">
              {msg("Arca 网关")}
              <small>{msg("鉴权 · 配额 · 签名")}</small>
            </div>
            <i className="dashed" />
            <div>
              {msg("私有 S3")}
              <small>{msg("文件字节直传")}</small>
            </div>
          </div>
        </article>
      </div>
    </>
  );
}
function Metric({
  label,
  value,
  note,
  icon,
}: {
  label: string;
  value: string | number;
  note: string;
  icon: any;
}) {
  return (
    <article className="metric">
      <div>{icon}</div>
      <span>{msg(label)}</span>
      <strong>{value}</strong>
      <small>{note}</small>
    </article>
  );
}
function BucketsPage({
  token,
  notify,
  routeBucket,
}: {
  token: string;
  routeBucket?: string;
  notify: (s: string) => void;
}) {
  const d = useData(token, "buckets");
  const [open, setOpen] = useState(false);
  const [secret, setSecret] = useState<any>();
  const selected = d.buckets.find((b) => b.slug === routeBucket);
  const setSelected = (b: Bucket | undefined) => routeTo("buckets", b?.slug);
  const hasDeleting = d.buckets.some((b) => b.deleting);
  useEffect(() => {
    if (!hasDeleting) return;
    const timer = setInterval(d.reload, 3000);
    return () => clearInterval(timer);
  }, [hasDeleting, d.api]);
  if (d.loading && !secret) return <Loading />;
  if (d.error && !secret) return <ErrorBox text={d.error} retry={d.reload} />;
  if (routeBucket && !selected)
    return (
      <>
        <Button onClick={() => routeTo("buckets")}>
          {msg("返回虚拟桶列表")}
        </Button>
        <ErrorBox
          text={msg("此虚拟桶不存在，或已经清理完成。")}
          retry={d.reload}
        />
      </>
    );
  if (selected)
    return (
      <BucketManager
        key={selected.slug}
        api={d.api}
        bucket={selected}
        close={() => {
          setSelected(undefined);
          d.reload();
        }}
        notify={notify}
      />
    );
  return (
    <>
      <div className="section-head">
        <div>
          <p className="eyebrow">VIRTUAL NAMESPACES</p>
          <h2>{msg("虚拟桶")}</h2>
          <p>{msg("每个桶拥有独立命名空间、容量限制、状态和访问凭据。")}</p>
        </div>
        <Button className="primary small" onClick={() => setOpen(true)}>
          <Plus size={16} />
          {msg("创建虚拟桶")}
        </Button>
      </div>
      {!d.buckets.length ? (
        <Empty
          title={msg("还没有虚拟桶")}
          text={msg("创建第一个隔离存储空间，真实文件仍落在受管的私有存储源。")}
        />
      ) : (
        <div className="bucket-grid">
          {d.buckets.map((b) => {
            const pct = b.quota_unlimited
              ? 0
              : Math.min(
                  100,
                  ((b.used_bytes + b.reserved_bytes) / b.quota_bytes) * 100,
                );
            return (
              <article className="bucket" key={b.id}>
                <div className="bucket-top">
                  <div className="bucket-icon">
                    <Box />
                  </div>
                  <span className={`pill ${b.visibility}`}>
                    {b.deleting
                      ? msg("清理中")
                      : b.visibility === "public"
                        ? msg("公开映射")
                        : msg("私有")}
                  </span>
                </div>
                <h3>{b.name}</h3>
                <code>s3://{b.slug}</code>
                <div className="bucket-usage">
                  <span>
                    {fmt(b.used_bytes + b.reserved_bytes)} /{" "}
                    {b.quota_unlimited ? msg("∞ 无上限") : fmt(b.quota_bytes)}
                  </span>
                  <span>
                    {b.quota_unlimited ? "UNLIMITED" : `${pct.toFixed(1)}%`}
                  </span>
                </div>
                {b.quota_unlimited ? (
                  <span className="capacity-unlimited">
                    {msg("容量不设上限")}
                  </span>
                ) : (
                  <Progress
                    label={msgt`${b.name}已用与预留容量`}
                    value={pct}
                    flowing={false}
                  />
                )}
                <footer>
                  <span>{new Date(b.created_at).toLocaleDateString()}</span>
                  <Button onClick={() => setSelected(b)}>
                    {msg("管理 ")}
                    <ChevronRight size={14} />
                  </Button>
                </footer>
              </article>
            );
          })}
        </div>
      )}
      {open && (
        <CreateBucket
          api={d.api}
          canUnlimited={Boolean(d.overview?.primary_source_unlimited)}
          close={() => setOpen(false)}
          done={(v) => {
            setSecret(v);
            setOpen(false);
            d.reload();
            notify(msg("虚拟桶已创建"));
          }}
        />
      )}
      {secret && (
        <OneTimeSecret data={secret} close={() => setSecret(undefined)} />
      )}
    </>
  );
}
function BucketManager({
  api,
  bucket,
  close,
  notify,
}: {
  api: API;
  bucket: Bucket;
  close: () => void;
  notify: (s: string) => void;
}) {
  const [detail, setDetail] = useState<BucketDetail>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [editing, setEditing] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const load = () => {
    setLoading(true);
    setError("");
    api
      .bucketDetail(bucket.slug)
      .then(setDetail)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  };
  useEffect(load, [api, bucket.slug]);
  const current = detail?.bucket || bucket;
  useEffect(() => {
    if (!current.deleting) return;
    let active = true;
    const timer = setInterval(() => {
      api
        .bucketDetail(bucket.slug)
        .then((v) => {
          if (active) {
            setDetail(v);
            setError("");
          }
        })
        .catch((e) => {
          if (!active) return;
          if (e instanceof APIError && e.status === 404) {
            active = false;
            notify(msg("虚拟桶及其关联文件已清理完成"));
            close();
          } else setError(e.message);
        });
    }, 3000);
    return () => {
      active = false;
      clearInterval(timer);
    };
  }, [api, bucket.slug, current.deleting, close, notify]);
  return (
    <>
      <div className="section-head">
        <div>
          <Button className="back" onClick={close}>
            {msg("← 返回虚拟桶")}
          </Button>
          <p className="eyebrow">BUCKET CONTROL PLANE</p>
          <h2>{current.name}</h2>
          <p>
            <code>s3://{current.slug}</code>
            {msg(" · 配置、接入信息与对象运维")}
          </p>
        </div>
        <div className="detail-actions">
          <Button
            className="ghost"
            disabled={!detail || current.deleting}
            onClick={() => setEditing(true)}
          >
            <Pencil size={15} />
            {msg("编辑桶")}
          </Button>
          {!current.deleting && (
            <Button
              className="bucket-delete-button"
              disabled={!detail}
              onClick={() => setDeleting(true)}
            >
              <Trash2 size={15} />
              {msg("删除桶")}
            </Button>
          )}
          <Button className="ghost" onClick={load}>
            <RefreshCw size={15} />
            {msg("刷新")}
          </Button>
        </div>
      </div>
      {loading && !detail ? (
        <Loading />
      ) : error ? (
        <ErrorBox text={error} retry={load} />
      ) : (
        detail && (
          <>
            <div className="bucket-detail-grid">
              <article>
                <span>{msg("当前状态")}</span>
                <strong className={`detail-status ${detail.status}`}>
                  {detail.status === "deleting"
                    ? msg("正在清理")
                    : detail.status === "ready"
                      ? msg("可用")
                      : msg("存储不可用")}
                </strong>
                <small>
                  {detail.metrics.object_count}
                  {msg(" 个对象 ·")} {detail.metrics.active_key_count}
                  {msg(" 个活跃密钥")}
                </small>
              </article>
              <article>
                <span>{msg("容量限制")}</span>
                <strong>
                  {current.quota_unlimited
                    ? msg("∞ 无上限")
                    : fmt(current.quota_bytes)}
                </strong>
                <small>
                  {msg("已用与预留 ")}
                  {fmt(current.used_bytes + current.reserved_bytes)}
                </small>
              </article>
              <article>
                <span>{msg("S3 接入点")}</span>
                <code>{detail.s3_endpoint}</code>
                <small>Region · {detail.region}</small>
              </article>
              <article>
                <span>{msg("控制 API")}</span>
                <code>{detail.api_endpoint}</code>
                <small>
                  {detail.metrics.active_public_link_count}
                  {msg(" 个活跃公开链接")}
                </small>
              </article>
            </div>
            {detail.allocations.length > 0 && (
              <article className="allocation-strip">
                <strong>{msg("对象分布")}</strong>
                {detail.allocations.map((a) => (
                  <span key={a.source_id || "legacy"}>
                    <i>{a.source_kind.toUpperCase()}</i>
                    {a.source_name}
                    <small>
                      {a.object_count}
                      {msg(" 个 · ")}
                      {fmt(a.used_bytes)}
                    </small>
                  </span>
                ))}
              </article>
            )}
          </>
        )
      )}
      {editing && detail && (
        <EditBucket
          api={api}
          detail={detail}
          close={() => setEditing(false)}
          done={() => {
            setEditing(false);
            load();
            notify(msg("虚拟桶配置已更新"));
          }}
        />
      )}
      {deleting && detail && (
        <DeleteBucket
          api={api}
          detail={detail}
          close={() => setDeleting(false)}
          done={(v) => {
            setDetail(v);
            setDeleting(false);
            notify(msg("删除已开始，服务器正在清理此桶"));
          }}
        />
      )}
      {current.deleting ? (
        <article className="bucket-cleanup" role="status">
          <span className="cleanup-icon">
            <Loader2 className="spin" />
          </span>
          <div>
            <h3>
              {detail?.deletion?.last_error_code
                ? msg("清理遇到存储故障，将自动重试")
                : msg("Arca 正在永久清理此桶")}
            </h3>
            <p>
              {msg(
                "访问已停止。对象、暂存文件、密钥和链接全部清理完成后，此桶会自动从列表消失。",
              )}
            </p>
            {detail?.deletion &&
              Date.parse(detail.deletion.cleanup_after) > Date.now() && (
                <p>
                  {msg("已有上传直链仍可能写入，最终清理将于")}{" "}
                  <strong>
                    {new Date(detail.deletion.cleanup_after).toLocaleString()}
                  </strong>{" "}
                  {msg("后执行。期间会持续清理暂存文件。")}
                </p>
              )}
            {detail?.deletion?.last_error_code && (
              <p>
                {msg(
                  "请检查存储源连接和列举、删除权限；启用 S3 版本控制时还需要列举与删除历史版本的权限。错误码：",
                )}
                <code>{detail.deletion.last_error_code}</code>
                {msg(" · 清理编号：")}
                <code>{current.id}</code>
              </p>
            )}
            <small>{msg("每 3 秒同步进度 · 可以安全离开页面")}</small>
          </div>
        </article>
      ) : (
        <>
          <UploadInvitations api={api} bucket={current} notify={notify} />
          <ObjectExplorer api={api} bucket={bucket.slug} notify={notify} />
        </>
      )}
    </>
  );
}
function BucketFormFields({
  name,
  setName,
  quota,
  setQuota,
  visibility,
  setVisibility,
  unlimited,
  setUnlimited,
  canUnlimited,
}: {
  name: string;
  setName: (v: string) => void;
  quota: number;
  setQuota: (v: number) => void;
  visibility: string;
  setVisibility: (v: string) => void;
  unlimited: boolean;
  setUnlimited: (v: boolean) => void;
  canUnlimited: boolean;
}) {
  return (
    <>
      <div>
        <Input
          required
          placeholder={msg("媒体资源")}
          onValueChange={(value) => setName(value)}
          value={String(name)}
          label={msg("显示名称")}
        />
      </div>
      <div className="split">
        <div>
          <Input
            type="number"
            min="1"
            disabled={unlimited}
            required={!unlimited}
            onValueChange={(value) => setQuota(+value)}
            value={String(quota)}
            label={msg("空间上限（GB）")}
            description={
              <>
                {canUnlimited
                  ? msg("仅在最高优先级存储源无限时可用")
                  : msg("最高优先级存储源为有限容量")}
              </>
            }
          />
          <Checkbox
            label={msg("无限容量")}
            checked={unlimited}
            disabled={!canUnlimited}
            onCheckedChange={setUnlimited}
          />
        </div>
        <Select
          value={visibility}
          onValueChange={(value) => setVisibility(value)}
          label={msg("访问模式")}
          options={[
            {
              value: "private",
              label: msg("私有"),
            },
            {
              value: "public",
              label: msg("公开映射"),
            },
          ]}
        />
      </div>
    </>
  );
}
function EditBucket({
  api,
  detail,
  close,
  done,
}: {
  api: API;
  detail: BucketDetail;
  close: () => void;
  done: () => void;
}) {
  const b = detail.bucket;
  const [name, setName] = useState(b.name);
  const [quota, setQuota] = useState(Math.max(1, b.quota_bytes / (1 << 30)));
  const [visibility, setVisibility] = useState(b.visibility);
  const [unlimited, setUnlimited] = useState(b.quota_unlimited);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [confirmPrivate, setConfirmPrivate] = useState(false);
  const save = async () => {
    setBusy(true);
    setError("");
    try {
      await api.updateBucket(b.slug, {
        name,
        visibility,
        quota_bytes: unlimited ? 0 : Math.round(quota * (1 << 30)),
        quota_unlimited: unlimited,
      });
      done();
    } catch (e: any) {
      setError(e.message || msg("保存失败"));
      setConfirmPrivate(false);
    } finally {
      setBusy(false);
    }
  };
  return (
    <>
      <Modal title={msg("编辑虚拟桶")} close={close} busy={busy}>
        <form
          className="form"
          onSubmit={(e) => {
            e.preventDefault();
            if (b.visibility === "public" && visibility === "private")
              setConfirmPrivate(true);
            else void save();
          }}
        >
          <fieldset className="request-fields" disabled={busy}>
            <div className="immutable-slug">
              <span>{msg("虚拟桶名称")}</span>
              <code>s3://{b.slug}</code>
              <small>{msg("接入名称保持不变，避免应用配置失效。")}</small>
            </div>
            <BucketFormFields
              {...{
                name,
                setName,
                quota,
                setQuota,
                visibility,
                setVisibility,
                unlimited,
                setUnlimited,
                canUnlimited: detail.can_set_unlimited,
              }}
            />
            <p className="field-explain">
              {msg(
                "私有模式允许按次签发下载链接；公开映射模式还允许创建稳定的 /p/ 链接。真实存储源始终保持私有。",
              )}
            </p>
            {error && (
              <p className="form-error" role="alert">
                {error}
              </p>
            )}
            <div className="actions">
              <Button
                type="button"
                className="ghost"
                disabled={busy}
                onClick={close}
              >
                {msg("取消")}
              </Button>
              <Button className="primary" disabled={busy} loading={busy}>
                {<ShieldCheck size={17} />}
                {msg("保存调整")}
              </Button>
            </div>
          </fieldset>
        </form>
      </Modal>
      {confirmPrivate && (
        <RiskDialog
          eyebrow="PUBLIC LINK AVAILABILITY"
          title={msg("将桶改为私有模式？")}
          lead={b.slug}
          busy={busy}
          cancelLabel={msg("继续检查")}
          confirmLabel={msg("停止公开映射并保存")}
          onCancel={() => setConfirmPrivate(false)}
          onConfirm={() => void save()}
        >
          <div className="risk-impact">
            <strong>{msg("现有 /p/ 公开别名将立即停止跳转")}</strong>
            <p>
              {msg(
                "应用仍可使用有 read 权限的虚拟凭据按次申请下载签名；已签发的真实 S3 URL 会按原有效期继续工作。",
              )}
            </p>
            {error && (
              <p className="form-error" role="alert">
                {error}
              </p>
            )}
          </div>
        </RiskDialog>
      )}
    </>
  );
}
function CreateBucket({
  api,
  canUnlimited,
  close,
  done,
}: {
  api: API;
  canUnlimited: boolean;
  close: () => void;
  done: (v: any) => void;
}) {
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [quota, setQuota] = useState(10);
  const [visibility, setVisibility] = useState("private");
  const [unlimited, setUnlimited] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  return (
    <Modal title={msg("创建虚拟桶")} close={close} busy={busy}>
      <form
        className="form"
        onSubmit={async (e) => {
          e.preventDefault();
          setBusy(true);
          setError("");
          try {
            done(
              await api.createBucket({
                name,
                slug,
                visibility,
                quota_bytes: unlimited ? 0 : quota * (1 << 30),
                quota_unlimited: unlimited,
              }),
            );
          } catch (e: any) {
            setError(e.message);
            setBusy(false);
          }
        }}
      >
        <fieldset className="request-fields" disabled={busy}>
          <div>
            <Input
              required
              placeholder="media-assets"
              onValueChange={(value) => setSlug(value)}
              value={String(slug)}
              label={msg("虚拟桶名称")}
              requirements={[
                {
                  id: "s3-name",
                  label: msg(
                    "3–63 个小写字母、数字或短横线，以字母或数字开头和结尾",
                  ),
                  check: (value) =>
                    /^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$/.test(value),
                },
              ]}
            />
          </div>
          <BucketFormFields
            {...{
              name,
              setName,
              quota,
              setQuota,
              visibility,
              setVisibility,
              unlimited,
              setUnlimited,
              canUnlimited,
            }}
          />
          <div className="hint">
            <ShieldCheck size={17} />
            <span>
              {msg(
                "真实存储始终保持私有。公开映射模式允许稳定 /p/ 链接；私有模式仅允许按次签发下载链接。",
              )}
            </span>
          </div>
          {error && <p className="form-error">{error}</p>}
          <div className="actions">
            <Button
              type="button"
              className="ghost"
              disabled={busy}
              onClick={close}
            >
              {msg("取消")}
            </Button>
            <Button className="primary" disabled={busy} loading={busy}>
              {<Plus size={17} />}
              {msg("创建并签发凭据")}
            </Button>
          </div>
        </fieldset>
      </form>
    </Modal>
  );
}
function OneTimeSecret({ data, close }: { data: any; close: () => void }) {
  return (
    <Modal title={msg("保存桶凭据")} close={close} dismissible={false}>
      <div className="once">
        <div className="warning">
          <KeyRound />
          <div>
            <strong>{msg("仅显示这一次")}</strong>
            <p>{msg("关闭后平台不会再次返回 Secret Key。")}</p>
          </div>
        </div>
        <Field label="Access Key" value={data.access_key} />
        <Field label="Secret Key" value={data.secret_key} />
        <p>
          {msg(
            "建议通过 rvsctl 直接写入部署环境的 Secret，而不是复制给 Agent。",
          )}
        </p>
        <Button className="primary full" onClick={close}>
          {msg("我已安全保存")}
        </Button>
      </div>
    </Modal>
  );
}
function Field({ label, value }: { label: string; value: string }) {
  return (
    <div className="secret-field">
      <span>{msg(label)}</span>
      <div>
        <code>{value}</code>
        <CopyButton text={value} />
      </div>
    </div>
  );
}
function AccessPage({ token }: { token: string }) {
  const d = useData(token, "access");
  const [open, setOpen] = useState(false);
  const [secret, setSecret] = useState<any>();
  const [busy, setBusy] = useState("");
  const [risk, setRisk] = useState<AccessKey | null>(null);
  const [riskError, setRiskError] = useState("");
  if (d.loading && !secret) return <Loading />;
  if (d.error && !secret) return <ErrorBox text={d.error} retry={d.reload} />;
  const confirmRevoke = async () => {
    if (!risk) return;
    setBusy(risk.id);
    try {
      await d.api.revokeKey(risk.id);
      setRisk(null);
      d.reload();
    } catch (e: any) {
      setRiskError(e.message || msg("撤销失败，请重试"));
    } finally {
      setBusy("");
    }
  };
  return (
    <>
      <div className="section-head">
        <div>
          <p className="eyebrow">IDENTITIES & GRANTS</p>
          <h2>{msg("访问控制")}</h2>
          <p>{msg("密钥按桶签发，可单独撤销并限制读、写、删除和管理权限。")}</p>
        </div>
        <Button
          className="primary small"
          disabled={!d.buckets.some((b) => !b.deleting)}
          onClick={() => setOpen(true)}
        >
          <Plus size={16} />
          {msg("签发访问密钥")}
        </Button>
      </div>
      <article className="card table-card">
        <table>
          <thead>
            <tr>
              <th>{msg("名称")}</th>
              <th>{msg("虚拟桶")}</th>
              <th>Access Key</th>
              <th>{msg("权限")}</th>
              <th>{msg("状态 / 操作")}</th>
            </tr>
          </thead>
          <tbody>
            {d.keys.map((k) => (
              <tr key={k.id}>
                <td>
                  <strong>{k.name}</strong>
                </td>
                <td>
                  <code>{k.bucket}</code>
                </td>
                <td>
                  <code>{k.access_key.slice(0, 10)}••••</code>
                </td>
                <td>
                  <div className="tags">
                    {k.permissions.map((p) => (
                      <span key={p}>{p}</span>
                    ))}
                  </div>
                </td>
                <td>
                  {k.revoked ? (
                    <span className="status off">
                      <i />
                      {msg("已撤销")}
                    </span>
                  ) : (
                    <Button
                      className="revoke"
                      disabled={busy === k.id}
                      onClick={() => {
                        setRisk(k);
                        setRiskError("");
                      }}
                      loading={busy === k.id}
                    >
                      {<X size={13} />}
                      {msg("撤销")}
                    </Button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {!d.keys.length && (
          <Empty
            title={msg("暂无访问密钥")}
            text={msg("创建虚拟桶时会自动签发 Owner 密钥。")}
          />
        )}
      </article>
      <div className="callout">
        <ShieldCheck />
        <div>
          <strong>{msg("关于已经签发的真实 S3 链接")}</strong>
          <p>
            {msg(
              "撤销 AK/SK 会阻止新的签名请求，不会撤销已生成的 URL。对单个对象执行“立即失效”时，平台会在 S3 内部复制到新物理键并删除旧键，使旧 URL 立即返回 404。",
            )}
          </p>
        </div>
      </div>
      {open && (
        <CreateAccessKey
          api={d.api}
          buckets={d.buckets.filter((b) => !b.deleting)}
          close={() => setOpen(false)}
          done={(v) => {
            setSecret(v);
            setOpen(false);
            d.reload();
          }}
        />
      )}
      {secret && (
        <OneTimeSecret data={secret} close={() => setSecret(undefined)} />
      )}{" "}
      {risk && (
        <RiskDialog
          eyebrow="CREDENTIAL REVOCATION"
          title={msg("确认撤销访问密钥")}
          lead={risk.name}
          busy={busy === risk.id}
          cancelLabel={msg("保留密钥")}
          confirmLabel={msg("撤销密钥")}
          onCancel={() => {
            setRisk(null);
            setRiskError("");
          }}
          onConfirm={confirmRevoke}
        >
          <div className="risk-impact">
            <strong>{msg("应用将无法再申请新链接")}</strong>
            <p>
              {msg(
                "此密钥的所有权限会立即失效。已经签发的真实 S3 URL 会继续工作到各自的到期时间。",
              )}
            </p>
            {riskError && (
              <p className="form-error" role="alert">
                {riskError}
              </p>
            )}
          </div>
        </RiskDialog>
      )}
    </>
  );
}
function CreateAccessKey({
  api,
  buckets,
  close,
  done,
}: {
  api: API;
  buckets: Bucket[];
  close: () => void;
  done: (v: any) => void;
}) {
  const [name, setName] = useState(msg("应用访问"));
  const [bucket, setBucket] = useState(buckets[0]?.slug || "");
  const [permissions, setPermissions] = useState(["read"]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const toggle = (p: string) =>
    setPermissions((v) =>
      v.includes(p) ? v.filter((x) => x !== p) : [...v, p],
    );
  return (
    <Modal title={msg("签发访问密钥")} close={close} busy={busy}>
      <form
        className="form"
        onSubmit={async (e) => {
          e.preventDefault();
          setBusy(true);
          setError("");
          try {
            done(
              await api.createKey(bucket, {
                name,
                permissions,
              }),
            );
          } catch (e: any) {
            setError(e.message);
            setBusy(false);
          }
        }}
      >
        <fieldset className="request-fields" disabled={busy}>
          <div>
            <Input
              required
              onValueChange={(value) => setName(value)}
              value={String(name)}
              label={msg("密钥名称")}
            />
          </div>
          <Select
            value={bucket}
            onValueChange={(value) => setBucket(value)}
            label={msg("虚拟桶")}
            options={buckets.map((b) => ({
              value: b.slug,
              label: `${b.name} · ${b.slug}`,
            }))}
          />
          <fieldset className="permission-field">
            <legend>{msg("细粒度权限")}</legend>
            <div className="permission-grid">
              {[
                ["read", msg("只读 / 下载")],
                ["write", msg("写入 / 上传")],
                ["delete", msg("删除对象")],
                ["manage", msg("管理链接")],
              ].map(([p, label]) => (
                <Button
                  type="button"
                  key={p}
                  aria-pressed={permissions.includes(p)}
                  className={permissions.includes(p) ? "active" : ""}
                  onClick={() => toggle(p)}
                >
                  <ShieldCheck size={15} />
                  <span>
                    {label}
                    <small>{p}</small>
                  </span>
                </Button>
              ))}
            </div>
          </fieldset>
          {error && <p className="form-error">{error}</p>}
          <div className="actions">
            <Button
              type="button"
              className="ghost"
              disabled={busy}
              onClick={close}
            >
              {msg("取消")}
            </Button>
            <Button
              className="primary"
              disabled={busy || !permissions.length}
              loading={busy}
            >
              {<KeyRound size={17} />}
              {msg("签发密钥")}
            </Button>
          </div>
        </fieldset>
      </form>
    </Modal>
  );
}
function AgentPage({
  token,
  notify,
}: {
  token: string;
  notify: (s: string) => void;
}) {
  const api = useMemo(() => new API(token), [token]);
  const [name, setName] = useState(msg("新应用接入"));
  const [quota, setQuota] = useState(10);
  const [minutes, setMinutes] = useState(30);
  const [result, setResult] = useState<any>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useVersionHold(Boolean(result));
  const prompt = result
    ? buildAgentPrompt({
        gateway: location.origin,
        token: result.token,
        quotaGB: quota,
        expiresAt: result.expires_at,
        projectURL: result.project_url,
        releaseURL: result.release_url,
      })
    : "";
  const generate = async () => {
    setBusy(true);
    setError("");
    try {
      const response = await api.bootstrap({
        name,
        max_quota_bytes: quota * (1 << 30),
        expires_in: minutes * 60,
      });
      setResult(response);
      notify(msg("一次性 Agent 提示词已生成"));
    } catch (e: any) {
      setError(e.message || msg("生成失败，请重试"));
    } finally {
      setBusy(false);
    }
  };
  return (
    <>
      <div className="section-head">
        <div>
          <p className="eyebrow">AGENT ONBOARDING</p>
          <h2>{msg("把接入说明交给 Agent")}</h2>
          <p>{msg("创建一次性临时令牌，自动组成可复制的接入提示词。")}</p>
        </div>
      </div>
      <div className="agent-layout">
        <article className="card form-card">
          <div className="step">
            <span>01</span>
            <div>
              <strong>{msg("临时授权范围")}</strong>
              <p>{msg("令牌只能使用一次，并受空间和时间限制。")}</p>
            </div>
          </div>
          <div className="form">
            <div>
              <Input
                disabled={busy || Boolean(result)}
                onValueChange={(value) => setName(value)}
                value={String(name)}
                label={msg("用途")}
              />
            </div>
            <div className="split">
              <div>
                <Input
                  disabled={busy || Boolean(result)}
                  type="number"
                  min="1"
                  onValueChange={(value) => setQuota(+value)}
                  value={String(quota)}
                  label={msg("最大空间（GB）")}
                />
              </div>
              <div>
                <Input
                  disabled={busy || Boolean(result)}
                  type="number"
                  min="1"
                  onValueChange={(value) => setMinutes(+value)}
                  value={String(minutes)}
                  label={msg("令牌有效期（分钟）")}
                />
              </div>
            </div>
            {error && (
              <p className="form-error" role="alert">
                {error}
              </p>
            )}
            <Button
              className="primary full"
              disabled={busy || Boolean(result) || quota < 1 || minutes < 1}
              onClick={generate}
              loading={busy}
            >
              {<Sparkles size={17} />}
              {msg("生成一次性接入提示词")}
            </Button>
            {result && (
              <Button
                className="ghost full"
                onClick={() => setResult(undefined)}
              >
                {msg("已安全保存，准备生成下一份")}
              </Button>
            )}
          </div>
        </article>
        <article className="prompt-card">
          <div className="prompt-head">
            <div>
              <span>AGENT PROMPT</span>
              <strong>
                {result ? msg("已包含一次性授权") : msg("等待生成")}
              </strong>
            </div>
            {result && <CopyButton text={prompt} label={msg("一键复制")} />}
          </div>
          {result ? (
            <pre>{prompt}</pre>
          ) : (
            <div className="prompt-empty">
              <Sparkles />
              <p>{msg("生成后，完整接入说明会出现在这里。")}</p>
              <small>
                {msg("Secret Key 将由 CLI 直接写入目标配置，不在终端回显。")}
              </small>
            </div>
          )}
        </article>
      </div>
      <div className="callout warm">
        <KeyRound />
        <div>
          <strong>{msg("“Agent 不接触明文”是一条部署边界")}</strong>
          <p>
            {msg(
              "rvsctl 可以避免凭据出现在对话和终端输出中；拥有目标机器完整文件读取权的 Agent 仍可能读取 Secret。生产环境建议让 rvsctl 直接写入 Kubernetes Secret、Vault 或云端 Secret Manager，并限制 Agent 对 Secret 的读权限。",
            )}
          </p>
        </div>
      </div>
    </>
  );
}
function SettingsPage({ token }: { token: string }) {
  const d = useData(token, "overview");
  if (d.loading) return <Loading />;
  if (d.error) return <ErrorBox text={d.error} retry={d.reload} />;
  return (
    <>
      <div className="section-head">
        <div>
          <p className="eyebrow">GATEWAY GOVERNANCE</p>
          <h2>{msg("平台设置")}</h2>
          <p>{msg("集中入口为后续策略、审计和流量治理保留统一边界。")}</p>
        </div>
      </div>
      <div className="settings-grid">
        <article className="card setting">
          <div className="setting-icon">
            <Cloud />
          </div>
          <div>
            <span>{msg("真实存储后端")}</span>
            <strong>
              {d.overview?.backend_ready
                ? msg("连接配置完整")
                : msg("等待配置")}
            </strong>
            <p>
              {msg(
                "对象由已验证的 S3 / WebDAV 存储源承载；S3 真实桶始终保持私有。",
              )}
            </p>
          </div>
          <span className={`status ${d.overview?.backend_ready ? "" : "off"}`}>
            <i />
            {d.overview?.backend_ready ? "Ready" : "Not ready"}
          </span>
        </article>
        <article className="card setting">
          <div className="setting-icon">
            <Activity />
          </div>
          <div>
            <span>{msg("集中网关")}</span>
            <strong>{location.origin}</strong>
            <p>
              <code>/api/v1</code>、<code>/s3</code>、<code>/p</code>
              {msg(" 共用入口。")}
            </p>
          </div>
          <span className="status">
            <i />
            Online
          </span>
        </article>
        <article className="card setting">
          <div className="setting-icon">
            <Database />
          </div>
          <div>
            <span>{msg("平台总空间")}</span>
            <strong>{fmt(d.overview?.total_quota)}</strong>
            <p>
              {msg(
                "有限容量时按平台和存储源配额约束；无限容量需最高优先级存储源支持。",
              )}
            </p>
          </div>
        </article>
        <article className="card setting">
          <div className="setting-icon">
            <ShieldCheck />
          </div>
          <div>
            <span>{msg("链接撤销策略")}</span>
            <strong>{msg("短期签名 + 物理键轮换")}</strong>
            <p>{msg("AK 轮换阻止新签名；对象键轮换使旧直链立即失效。")}</p>
          </div>
        </article>
      </div>
    </>
  );
}
function Empty({ title, text }: { title: string; text: string }) {
  return (
    <div className="empty">
      <div>
        <Box />
      </div>
      <h3>{title}</h3>
      <p>{text}</p>
    </div>
  );
}
function Modal({
  title,
  close,
  children,
  dismissible = true,
  busy = false,
}: {
  title: string;
  close: () => void;
  children: React.ReactNode;
  dismissible?: boolean;
  busy?: boolean;
}) {
  useVersionHold();
  return (
    <FormaModal
      {...{
        title,
        close,
        dismissible,
        busy,
      }}
    >
      {children}
    </FormaModal>
  );
}
function CopyButton({ text, label }: { text: string; label?: string }) {
  const [status, setStatus] = useState("");
  useEffect(() => {
    if (!status) return;
    const timer = setTimeout(() => setStatus(""), 1800);
    return () => clearTimeout(timer);
  }, [status]);
  return (
    <Button
      type="button"
      className="copy"
      aria-label={label || msg("复制内容")}
      onClick={async () => {
        try {
          await navigator.clipboard.writeText(text);
          setStatus(msg("已复制"));
        } catch {
          setStatus(msg("复制失败"));
        }
      }}
    >
      <Copy size={14} />
      <span aria-live="polite">{status || label || ""}</span>
    </Button>
  );
}
