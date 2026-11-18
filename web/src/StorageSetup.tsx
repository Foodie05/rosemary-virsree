import { msg, msgt } from "./locale";
import {
  Button,
  Input,
  Checkbox,
  Modal,
  ThemeToggle,
  Skeleton,
  Tabs,
  Stepper,
  Progress,
} from "./Forma";
import { FormEvent, useEffect, useState, useRef } from "react";
import type { ReactNode } from "react";
import {
  ArrowRight,
  Check,
  ChevronRight,
  Cloud,
  Globe2,
  HardDrive,
  Pencil,
  Plus,
  ShieldCheck,
  Waypoints,
} from "./icons";
import { API } from "./api";
import type {
  CDNMode,
  PlatformFilesystem,
  StorageSource,
  StorageSourceDetail,
  StorageSourceInput,
} from "./api";
import { Brand } from "./Brand";
import { RiskDialog } from "./RiskDialog";
import { useVersionHold } from "./VersionGuard";
const gb = (n: number) =>
  n >= 2 ** 30
    ? `${(n / 2 ** 30).toFixed(1)} GB`
    : `${(n / 2 ** 20).toFixed(0)} MB`;
const normalizeEndpoint = (raw: string) => {
  let value = raw.trim().replace(/^\.+/, "");
  if (!value) return "";
  value = value
    .replace(/^https\/\//i, "https://")
    .replace(/^http\/\//i, "http://")
    .replace(/^https:\/(?!\/)/i, "https://")
    .replace(/^http:\/(?!\/)/i, "http://");
  if (value.startsWith("//")) value = "https:" + value;
  else if (!/^https?:\/\//i.test(value)) value = "https://" + value;
  return value.replace(/\/+$/, "");
};
const normalizeS3ServiceEndpoint = (raw: string, bucket: string) => {
  const value = normalizeEndpoint(raw);
  if (!value || !bucket.trim()) return value;
  try {
    const u = new URL(value),
      prefix = bucket.trim().toLowerCase() + ".",
      host = u.hostname.toLowerCase();
    if (host.startsWith(prefix) && host.slice(prefix.length).startsWith("s3."))
      u.hostname = u.hostname.slice(prefix.length);
    return u.toString().replace(/\/$/, "");
  } catch {
    return value;
  }
};
export function OOBE({
  email,
  onComplete,
}: {
  email: string;
  onComplete: () => void;
}) {
  const [step, setStep] = useState<"welcome" | "source" | "done">("welcome");
  const [busy, setBusy] = useState(false);
  useVersionHold(step === "source");
  return (
    <div className="oobe">
      <header>
        <Brand />
        <ThemeToggle />
        <div className="oobe-user">
          <span>{email}</span>
          <small>{msg("已通过 OIDC 验证")}</small>
        </div>
      </header>
      <main>
        <Stepper
          label={msg("首次配置进度")}
          current={step}
          items={[
            {
              id: "welcome",
              title: msg("欢迎使用"),
            },
            {
              id: "source",
              title: msg("连接存储源"),
            },
            {
              id: "done",
              title: msg("配置完成"),
            },
          ]}
        />
        {step === "welcome" && (
          <section className="welcome-card">
            <div className="welcome-visual">
              <div className="orbit one">
                <Cloud />
              </div>
              <div className="orbit two">
                <HardDrive />
              </div>
              <div className="core">
                <Waypoints />
              </div>
            </div>
            <p className="eyebrow">WELCOME TO ARCA</p>
            <h1>
              {msg("先种下第一处")}
              <br />
              <em>{msg("私有存储。")}</em>
            </h1>
            <p>
              {msg(
                "Arca 会把真实存储源虚拟成应用熟悉的 S3 边界。以后还可以继续添加存储源；新对象先写入优先级最高且空间充足的来源。",
              )}
            </p>
            <Button
              className="primary oobe-next"
              onClick={() => setStep("source")}
            >
              {msg("开始配置 ")}
              <ArrowRight size={18} />
            </Button>
            <div className="welcome-points">
              <span>
                <Check />
                {msg("凭据加密保存")}
              </span>
              <span>
                <Check />
                {msg("添加时读写验证")}
              </span>
              <span>
                <Check />
                {msg("统一容量调度")}
              </span>
            </div>
          </section>
        )}
        {step === "source" && (
          <section className="source-step">
            <div className="source-step-head">
              <Button disabled={busy} onClick={() => setStep("welcome")}>
                {msg("← 返回")}
              </Button>
              <p className="eyebrow">FIRST STORAGE SOURCE</p>
              <h1>{msg("连接你的第一个存储源")}</h1>
              <p>
                {msg(
                  "保存前会创建一个极小的探针对象，确认写入、读取和删除都可用。",
                )}
              </p>
            </div>
            <StorageSourceForm
              onBusyChange={setBusy}
              submitLabel={msg("验证并完成配置")}
              onSaved={() => setStep("done")}
            />
          </section>
        )}
        {step === "done" && (
          <section className="done-card">
            <div>
              <Check />
            </div>
            <p className="eyebrow">SOURCE VERIFIED</p>
            <h1>{msg("Arca 已经准备好了")}</h1>
            <p>
              {msg(
                "第一个存储源已通过读写验证。接下来可以创建虚拟桶、签发应用凭据，或随时添加更多存储源。",
              )}
            </p>
            <Button className="primary oobe-next" onClick={onComplete}>
              {msg("进入控制台 ")}
              <ChevronRight size={18} />
            </Button>
          </section>
        )}
      </main>
    </div>
  );
}
function StorageModal({
  eyebrow,
  title,
  close,
  children,
  busy = false,
  returnFocusRef,
}: {
  eyebrow: string;
  title: string;
  close: () => void;
  children: ReactNode;
  busy?: boolean;
  returnFocusRef?: {
    readonly current: HTMLElement | null;
  };
}) {
  useVersionHold();
  return (
    <Modal
      returnFocusRef={returnFocusRef}
      title={title}
      close={close}
      busy={busy}
      className="storage-modal"
    >
      <p className="eyebrow">{eyebrow}</p>
      {children}
    </Modal>
  );
}
function BucketRiskDialog({
  oldBucket,
  newBucket,
  used,
  busy,
  onCancel,
  onConfirm,
}: {
  oldBucket: string;
  newBucket: string;
  used: number;
  busy: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  return (
    <RiskDialog
      eyebrow="STORAGE AVAILABILITY RISK"
      title={msg("确认更换真实存储桶")}
      lead={msg("这个操作会改变 Arca 读取现有对象的位置。")}
      busy={busy}
      cancelLabel={msg("返回检查")}
      confirmLabel={msg("理解风险，重新验证")}
      onCancel={onCancel}
      onConfirm={onConfirm}
    >
      <div className="risk-comparison">
        <div>
          <small>{msg("当前桶")}</small>
          <code>{oldBucket}</code>
        </div>
        <ArrowRight />
        <div>
          <small>{msg("新桶")}</small>
          <code>{newBucket}</code>
        </div>
      </div>
      <div className="risk-impact">
        <strong>{msg("请先确认数据迁移计划")}</strong>
        <p>
          {msg("当前记录的占用空间为 ")}
          {gb(used)}
          {msg(
            "。保存后，Arca 会从新桶读取相同的物理对象键；若对象尚未迁移，现有文件的下载、删除和链接轮换会失败。Arca 不会自动搬迁数据。",
          )}
        </p>
      </div>
    </RiskDialog>
  );
}
export function StoragePage({ notify }: { notify: (s: string) => void }) {
  const api = new API();
  const [sources, setSources] = useState<StorageSource[]>([]);
  const [filesystem, setFilesystem] = useState<PlatformFilesystem>();
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<StorageSourceDetail | null>(null);
  const [editingID, setEditingID] = useState("");
  const editTrigger = useRef<HTMLButtonElement>(null);
  const [formBusy, setFormBusy] = useState(false);
  const [error, setError] = useState("");
  const load = () => {
    setLoading(true);
    setError("");
    Promise.allSettled([api.storageSources(), api.platformFilesystem()])
      .then(([sourceResult, filesystemResult]) => {
        const problems: string[] = [];
        if (sourceResult.status === "fulfilled")
          setSources(sourceResult.value || []);
        else problems.push(msg("存储源：") + sourceResult.reason.message);
        if (filesystemResult.status === "fulfilled")
          setFilesystem(filesystemResult.value);
        else
          problems.push(
            msg("平台文件系统：") + filesystemResult.reason.message,
          );
        setError(problems.join("；"));
      })
      .finally(() => setLoading(false));
  };
  useEffect(load, []);
  const beginEdit = async (id: string) => {
    setEditingID(id);
    setError("");
    try {
      setEditing(await api.storageSource(id));
    } catch (e: any) {
      setError(e.message);
    } finally {
      setEditingID("");
    }
  };
  return (
    <>
      <div className="section-head">
        <div>
          <p className="eyebrow">PHYSICAL STORAGE POOL</p>
          <h2>{msg("存储源")}</h2>
          <p>
            {msg(
              "按优先级分配新对象；最高优先级来源同时承载 Arca 平台文件系统。",
            )}
          </p>
        </div>
        <Button className="primary small" onClick={() => setOpen(true)}>
          <Plus size={16} />
          {msg("添加存储源")}
        </Button>
      </div>
      {error && (
        <div className="error" role="alert">
          <ShieldCheck />
          {error}
          <Button className="ghost" onClick={load}>
            {msg("重试")}
          </Button>
        </div>
      )}
      {!loading && filesystem && (
        <article
          className={`platform-fs ${filesystem.initialized ? "ready" : ""}`}
        >
          <div>
            <ShieldCheck />
            <span>
              <small>VIRSREE PLATFORM FILESYSTEM</small>
              <strong>
                {filesystem.last_error_code
                  ? msg("最近备份失败，等待重试")
                  : filesystem.initialized
                    ? msg("平台文件系统已建立")
                    : msg("正在建立平台文件系统")}
              </strong>
            </span>
          </div>
          <p>
            {msg("元数据、审计日志和对象键映射每")}{" "}
            {filesystem.backup_interval_seconds >= 3600
              ? msgt`${filesystem.backup_interval_seconds / 3600} 小时`
              : msgt`${Math.ceil(filesystem.backup_interval_seconds / 60)} 分钟`}
            {msg("备份至")}{" "}
            <code>
              {filesystem.source_name || msg("最高优先级存储源")}/
              {filesystem.prefix}
            </code>
            {msg("，保留 ")}
            {filesystem.retention}
            {msg(" 个滚动快照。")}
          </p>
          <span>
            {filesystem.last_success_at
              ? msgt`最近成功：${new Date(filesystem.last_success_at).toLocaleString()}`
              : filesystem.last_error_code
                ? msgt`等待重试 · ${filesystem.last_error_code}`
                : msg("等待首次备份")}
          </span>
        </article>
      )}
      {loading && !sources.length ? (
        <div className="skeletons">
          {[1, 2, 3].map((id) => (
            <Skeleton key={id} lines={4} label={msg("正在读取存储源")} />
          ))}
        </div>
      ) : (
        <div className="source-grid">
          {sources.map((s, index) => {
            const used = s.used_bytes + s.reserved_bytes,
              pct = s.capacity_unlimited
                ? 0
                : Math.min(100, (used / s.capacity_bytes) * 100);
            return (
              <article className="source-card" key={s.id}>
                <div className="source-rank">
                  <span>
                    {msg("优先级 ")}
                    {s.priority}
                  </span>
                  {index === 0 && <b>{msg("平台文件系统")}</b>}
                </div>
                <div className="source-title">
                  <div>{s.kind === "s3" ? <Cloud /> : <Globe2 />}</div>
                  <div>
                    <h3>{s.name}</h3>
                    <code>{s.kind.toUpperCase()}</code>
                  </div>
                  <span className="status">
                    <i />
                    {msg("已通过接入验证")}
                  </span>
                </div>
                <div className="source-capacity">
                  <div>
                    <span>
                      {gb(used)}
                      {msg(" 已占用")}
                    </span>
                    <span>
                      {s.capacity_unlimited
                        ? msg("∞ 无上限")
                        : gb(s.capacity_bytes)}
                    </span>
                  </div>
                  {s.capacity_unlimited ? (
                    <span className="capacity-unlimited">
                      {msg("容量不设上限")}
                    </span>
                  ) : (
                    <Progress
                      label={msgt`${s.name}已用与预留容量`}
                      value={pct}
                      flowing={false}
                    />
                  )}
                </div>
                <footer>
                  <span>
                    {s.direct_transfer
                      ? msg("文件直达存储")
                      : msg("经 Arca 安全中转")}
                  </span>
                  <div>
                    {s.cdn_enabled && <b>{msg("CDN 下载")}</b>}
                    <Button
                      className="source-edit"
                      aria-label={msgt`编辑${s.name}`}
                      disabled={editingID === s.id}
                      onClick={(event) => {
                        editTrigger.current = event.currentTarget;
                        void beginEdit(s.id);
                      }}
                      loading={editingID === s.id}
                    >
                      {<Pencil />}
                      {msg("编辑")}
                    </Button>
                  </div>
                </footer>
              </article>
            );
          })}
        </div>
      )}
      {open && (
        <StorageModal
          eyebrow="NEW STORAGE SOURCE"
          title={msg("添加存储源")}
          busy={formBusy}
          close={() => setOpen(false)}
        >
          <StorageSourceForm
            onBusyChange={setFormBusy}
            submitLabel={msg("验证并添加")}
            onSaved={() => {
              setOpen(false);
              load();
              notify(msg("存储源已验证并加入调度池"));
            }}
          />
        </StorageModal>
      )}
      {editing && (
        <StorageModal
          eyebrow="EDIT STORAGE SOURCE"
          title={msg("编辑存储源")}
          returnFocusRef={editTrigger}
          busy={formBusy}
          close={() => setEditing(null)}
        >
          <div className="edit-verification-note">
            <ShieldCheck />
            <span>
              <strong>{msg("更新前重新验证")}</strong>
              <small>
                {msg(
                  "新配置通过完整读、写、复制和删除验证后才会生效；验证失败时原配置继续运行。",
                )}
              </small>
            </span>
          </div>
          <StorageSourceForm
            initial={editing}
            onBusyChange={setFormBusy}
            submitLabel={msg("重新验证并保存")}
            onSaved={() => {
              setEditing(null);
              load();
              notify(msg("存储源已重新验证并更新"));
            }}
          />
        </StorageModal>
      )}
    </>
  );
}
function StorageSourceForm({
  submitLabel,
  onSaved,
  initial,
  onBusyChange,
}: {
  submitLabel: string;
  onSaved: () => void;
  initial?: StorageSourceDetail;
  onBusyChange?: (busy: boolean) => void;
}) {
  const editing = Boolean(initial);
  const [kind, setKind] = useState<"s3" | "webdav">(initial?.kind || "s3");
  const [name, setName] = useState(initial?.name || "");
  const [priority, setPriority] = useState(initial?.priority ?? 10);
  const [capacity, setCapacity] = useState(
    initial ? initial.capacity_bytes / 2 ** 30 : 100,
  );
  const [capacityUnlimited, setCapacityUnlimited] = useState(
    initial?.capacity_unlimited ?? false,
  );
  const [endpoint, setEndpoint] = useState(initial?.endpoint || "");
  const [publicEndpoint, setPublicEndpoint] = useState(
    initial?.public_endpoint || "",
  );
  const [region, setRegion] = useState(initial?.region || "us-east-1");
  const [bucket, setBucket] = useState(initial?.bucket || "");
  const [accessKey, setAccessKey] = useState("");
  const [secretKey, setSecretKey] = useState("");
  const [pathStyle, setPathStyle] = useState(initial?.path_style ?? false);
  const [cdn, setCDN] = useState(initial?.cdn_endpoint || "");
  const [cdnMode, setCDNMode] = useState<CDNMode>(
    initial?.cdn_mode || "s3_sigv4",
  );
  const [cdnAuthKey, setCDNAuthKey] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [riskOpen, setRiskOpen] = useState(false);
  const save = async (acknowledgeBucketChange = false) => {
    setBusy(true);
    onBusyChange?.(true);
    setError("");
    const cleanEndpoint =
      kind === "s3"
        ? normalizeS3ServiceEndpoint(endpoint, bucket)
        : normalizeEndpoint(endpoint);
    const cleanPublicEndpoint = normalizeS3ServiceEndpoint(
      publicEndpoint,
      bucket,
    );
    const cleanCDN = normalizeEndpoint(cdn);
    setEndpoint(cleanEndpoint);
    setPublicEndpoint(cleanPublicEndpoint);
    setCDN(cleanCDN);
    const payload: StorageSourceInput = {
      name,
      kind,
      priority,
      capacity_bytes: capacityUnlimited ? 0 : Math.round(capacity * 2 ** 30),
      capacity_unlimited: capacityUnlimited,
      endpoint: cleanEndpoint,
    };
    if (kind === "s3")
      Object.assign(payload, {
        public_endpoint: cleanPublicEndpoint,
        region,
        bucket: bucket.trim(),
        access_key: accessKey,
        secret_key: secretKey,
        path_style: pathStyle,
        cdn_endpoint: cleanCDN,
        cdn_mode: cdnMode,
        cdn_auth_key: cdnAuthKey,
        acknowledge_bucket_change: acknowledgeBucketChange,
      });
    else
      Object.assign(payload, {
        webdav_username: username,
        webdav_password: password,
      });
    try {
      if (initial) await new API().updateStorageSource(initial.id, payload);
      else await new API().addStorageSource(payload);
      onSaved();
    } catch (e: any) {
      setError(e.message);
    } finally {
      setBusy(false);
      onBusyChange?.(false);
    }
  };
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (initial?.kind === "s3" && bucket.trim() !== initial.bucket) {
      setRiskOpen(true);
      return;
    }
    void save();
  };
  const confirmRisk = () => {
    setRiskOpen(false);
    void save(true);
  };
  const secretHint = editing ? msg("留空则继续使用已加密保存的凭据") : "";
  const canReuseCDNKey = Boolean(
    editing &&
      initial?.cdn_mode === "bitiful_token" &&
      initial.cdn_auth_key_configured &&
      cdnMode === "bitiful_token" &&
      normalizeEndpoint(cdn) === initial.cdn_endpoint,
  );
  return (
    <>
      <form className="storage-form" onSubmit={submit}>
        <fieldset className="request-fields" disabled={busy}>
          <Tabs
            label={msg("存储协议")}
            value={kind}
            onValueChange={(value) => setKind(value as "s3" | "webdav")}
            items={[
              {
                value: "s3",
                label: msg("S3 兼容存储"),
                disabled: editing,
                content: null,
              },
              {
                value: "webdav",
                label: "WebDAV",
                disabled: editing,
                content: null,
              },
            ]}
          />
          <p className="field-explain">
            {kind === "s3"
              ? msg("连接 AWS S3、OSS、COS、R2、MinIO 等私有对象存储。")
              : msg("连接 NAS、Nextcloud 或标准 WebDAV 服务。")}
          </p>
          <div className="form-section">
            <div className="form-section-title">
              <span>01</span>
              <div>
                <strong>{msg("调度信息")}</strong>
                <small>
                  {msg("容量用于阻止超额分配，不会修改存储服务本身的配额。")}
                </small>
              </div>
            </div>
            <div className="field-grid three">
              <div>
                <Input
                  required
                  placeholder={
                    kind === "s3" ? msg("主对象存储") : msg("归档 WebDAV")
                  }
                  onValueChange={(value) => setName(value)}
                  value={String(name)}
                  label={msg("显示名称")}
                />
              </div>
              <div>
                <Input
                  required
                  type="number"
                  min="0"
                  onValueChange={(value) => setPriority(+value)}
                  value={String(priority)}
                  label={msg("优先级")}
                  description={<>{msg("数字越小越优先")}</>}
                />
              </div>
              <div>
                <Input
                  required={!capacityUnlimited}
                  disabled={capacityUnlimited}
                  type="number"
                  min="1"
                  step="0.1"
                  onValueChange={(value) => setCapacity(+value)}
                  value={String(capacity)}
                  label={msg("可用容量（GB）")}
                />
                <Checkbox
                  label={msg("设置为无限容量")}
                  checked={capacityUnlimited}
                  onCheckedChange={setCapacityUnlimited}
                />
              </div>
            </div>
          </div>
          {kind === "s3" ? (
            <>
              <div className="form-section">
                <div className="form-section-title">
                  <span>02</span>
                  <div>
                    <strong>{msg("S3 连接")}</strong>
                    <small>
                      {msg("使用具备对象读、写、复制和删除权限的真实桶凭据。")}
                    </small>
                  </div>
                </div>
                <div className="field-grid">
                  <div>
                    <Input
                      onBlur={() =>
                        setEndpoint(
                          normalizeS3ServiceEndpoint(endpoint, bucket),
                        )
                      }
                      placeholder={msg("s3.example.com（AWS 可留空）")}
                      onValueChange={(value) => setEndpoint(value)}
                      value={String(endpoint)}
                      label={msg("内部 API Endpoint")}
                      description={
                        <>
                          {msg(
                            "可以只填域名；Arca 会自动补全 HTTPS 并修正常见格式。",
                          )}
                        </>
                      }
                    />
                  </div>
                  <div>
                    <Input
                      onBlur={() =>
                        setPublicEndpoint(
                          normalizeS3ServiceEndpoint(publicEndpoint, bucket),
                        )
                      }
                      placeholder={msg("留空则与内部 Endpoint 相同")}
                      onValueChange={(value) => setPublicEndpoint(value)}
                      value={String(publicEndpoint)}
                      label={msg("应用直传 Endpoint")}
                    />
                  </div>
                  <div>
                    <Input
                      required
                      placeholder="us-east-1"
                      onValueChange={(value) => setRegion(value)}
                      value={String(region)}
                      label="Region"
                    />
                  </div>
                  <div>
                    <Input
                      required
                      placeholder="private-assets"
                      onValueChange={(value) => setBucket(value)}
                      value={String(bucket)}
                      label={msg("真实私有桶名称")}
                    />
                  </div>
                  <div>
                    <Input
                      required={!editing}
                      autoComplete="off"
                      placeholder={secretHint}
                      onValueChange={(value) => setAccessKey(value)}
                      value={String(accessKey)}
                      label="Access Key"
                      description={<>{secretHint}</>}
                    />
                  </div>
                  <div>
                    <Input
                      required={!editing}
                      type="password"
                      autoComplete="new-password"
                      placeholder={secretHint}
                      onValueChange={(value) => setSecretKey(value)}
                      value={String(secretKey)}
                      label="Secret Key"
                      description={<>{secretHint}</>}
                    />
                  </div>
                </div>
                <Checkbox
                  onCheckedChange={(checked) => setPathStyle(checked)}
                  checked={pathStyle}
                  label={
                    <>
                      <span>
                        <strong>{msg("使用 Path-style 地址")}</strong>
                        <small>
                          {msg(
                            "公有云 S3 兼容服务通常关闭；MinIO 和多数自建 S3 服务需要开启。",
                          )}
                        </small>
                      </span>
                    </>
                  }
                  className="check-row"
                />
              </div>
              <div className="form-section cdn-section">
                <div className="form-section-title">
                  <span>03</span>
                  <div>
                    <strong>{msg("下载 CDN（可选）")}</strong>
                    <small>
                      {msg(
                        "仅用于生成下载 URL；上传始终使用上面的直传 Endpoint。",
                      )}
                    </small>
                  </div>
                </div>
                <div>
                  <Input
                    onBlur={() => setCDN(normalizeEndpoint(cdn))}
                    placeholder="https://cdn.example.com"
                    onValueChange={(value) => setCDN(value)}
                    value={String(cdn)}
                    label={msg("CDN 加速域名")}
                    description={
                      <>
                        {msg(
                          "留空则从应用直传 Endpoint 下载。填写后请选择该 CDN 实际启用的鉴权协议。",
                        )}
                      </>
                    }
                  />
                </div>
                <fieldset className="cdn-mode">
                  <legend>{msg("下载鉴权方式")}</legend>
                  <div>
                    <Button
                      type="button"
                      aria-pressed={cdnMode === "bitiful_token"}
                      className={cdnMode === "bitiful_token" ? "active" : ""}
                      onClick={() => setCDNMode("bitiful_token")}
                    >
                      <strong>{msg("缤纷云高级鉴权")}</strong>
                      <small>
                        <code>_ts</code> + <code>_btf_tk</code>
                      </small>
                    </Button>
                    <Button
                      type="button"
                      aria-pressed={cdnMode === "s3_sigv4"}
                      className={cdnMode === "s3_sigv4" ? "active" : ""}
                      onClick={() => setCDNMode("s3_sigv4")}
                    >
                      <strong>{msg("S3 SigV4 兼容")}</strong>
                      <small>{msg("AWS 签名查询参数")}</small>
                    </Button>
                  </div>
                </fieldset>
                {cdnMode === "bitiful_token" && (
                  <div>
                    <Input
                      required={Boolean(cdn) && !canReuseCDNKey}
                      type="password"
                      autoComplete="new-password"
                      placeholder={
                        canReuseCDNKey
                          ? msg("留空则继续使用已加密保存的 Key")
                          : msg("从缤纷云 CDN 项目复制")
                      }
                      onValueChange={(value) => setCDNAuthKey(value)}
                      value={String(cdnAuthKey)}
                      label={msg("CDN 鉴权 Key")}
                      description={
                        <>
                          {msg(
                            "路径：缤纷云控制台 → CDN 项目 → 高级鉴权 → 鉴权 Key。Arca 加密保存，不会在接口、日志或页面中回显。",
                          )}
                        </>
                      }
                    />
                  </div>
                )}
                <div className="cdn-help">
                  <Waypoints />
                  <div>
                    <strong>
                      {cdnMode === "bitiful_token"
                        ? msg("适用于缤纷云 CDN 加速域名")
                        : msg("仅适用于真正接受 SigV4 的下载 Endpoint")}
                    </strong>
                    <p>
                      {cdnMode === "bitiful_token"
                        ? msg(
                            "Arca 会按应用每次提交的 expires_in 生成 _ts，并在服务器内计算 _btf_tk；文件由 CDN 直接返回。",
                          )
                        : msg(
                            "普通 CDN 自定义域名通常不接受 S3 SigV4。只有服务商明确说明兼容 AWS 预签名 GET 时才选择此项。",
                          )}
                    </p>
                  </div>
                </div>
              </div>
            </>
          ) : (
            <div className="form-section">
              <div className="form-section-title">
                <span>02</span>
                <div>
                  <strong>{msg("WebDAV 连接")}</strong>
                  <small>
                    {msg("Arca 会用以下账号创建、读取、复制和删除对象。")}
                  </small>
                </div>
              </div>
              <div className="field-grid">
                <div>
                  <Input
                    required
                    onBlur={() => setEndpoint(normalizeEndpoint(endpoint))}
                    placeholder="dav.example.com/remote.php/dav/files/user"
                    onValueChange={(value) => setEndpoint(value)}
                    value={String(endpoint)}
                    label="WebDAV Endpoint"
                    description={
                      <>{msg("可以只填域名或路径；Arca 会自动补全 HTTPS。")}</>
                    }
                  />
                </div>
                <div>
                  <Input
                    required={!editing}
                    placeholder={secretHint}
                    onValueChange={(value) => setUsername(value)}
                    value={String(username)}
                    label={msg("用户名")}
                    description={<>{secretHint}</>}
                  />
                </div>
                <div>
                  <Input
                    required={!editing}
                    type="password"
                    autoComplete="new-password"
                    placeholder={secretHint}
                    onValueChange={(value) => setPassword(value)}
                    value={String(password)}
                    label={msg("密码 / App Password")}
                    description={<>{secretHint}</>}
                  />
                </div>
              </div>
              <div className="protocol-note">
                <Waypoints />
                <div>
                  <strong>{msg("WebDAV 文件会经过 Arca")}</strong>
                  <p>
                    {msg(
                      "WebDAV 没有通用的预签名 URL 协议。平台会发放短期能力链接并中转文件字节；应用调用方式保持一致，响应里的",
                    )}{" "}
                    <code>direct</code>
                    {msg(" 为 ")}
                    <code>false</code>。
                  </p>
                </div>
              </div>
            </div>
          )}
          {error && (
            <div className="verify-error">
              <ShieldCheck />
              <div>
                <strong>{msg("验证没有通过")}</strong>
                <p>{error}</p>
              </div>
            </div>
          )}
          <Button className="primary verify-button" loading={busy}>
            <ShieldCheck />
            {submitLabel}
          </Button>
          {busy && (
            <p className="verification-status" role="status">
              {msg("正在验证连接与读写权限，请稍候…")}
            </p>
          )}
          <p className="verify-foot">
            {msg(
              "只有探针对象写入、HEAD 读取、下载、复制和删除全部成功后，配置才会保存。",
            )}
          </p>
        </fieldset>
      </form>
      {riskOpen && initial && (
        <BucketRiskDialog
          oldBucket={initial.bucket}
          newBucket={bucket.trim()}
          used={initial.used_bytes + initial.reserved_bytes}
          busy={busy}
          onCancel={() => setRiskOpen(false)}
          onConfirm={confirmRisk}
        />
      )}
    </>
  );
}
