import { msg } from "./locale";
import { useCallback, useEffect, useRef, useState } from "react";
import { API, Bucket, UploadInvitation, UploadInvitationResult } from "./api";
import { Button, Checkbox, Input, Select, Modal, Loading } from "./Forma";
import { Copy, Plus, RefreshCw } from "./icons";
import { RiskDialog } from "./RiskDialog";
import { useVersionHold } from "./VersionGuard";
import { getLanguage } from "./locale";
export function invitationPrompt(result: UploadInvitationResult) {
  const i = result.invitation;
  const policy = JSON.stringify(
    {
      bucket: result.bucket,
      visibility: result.visibility,
      expires_at: i.expires_at,
      max_files: i.max_files,
      allow_overwrite: i.allow_overwrite,
    },
    null,
    2,
  );
  const gateway = new URL(result.upload_url).origin;
  const credentials = `UPLOAD_ENDPOINT=${result.upload_url}\nUPLOAD_TOKEN=${result.token}`;
  return getLanguage() === "en"
    ? `Upload the files I specify to this Arca bucket with the temporary upload-only invitation below. Do not request AK/SK or administer the bucket.

Gateway instance: ${gateway}
API reference: ${gateway}/docs/api.md?raw=1 (section: Temporary upload invitations)
Tools: use the project's HTTP library, or curl plus Python 3 (or jq) to construct and parse JSON. Check these tools are installed before starting. Do not install rvsctl or call /agent/claim for this flow. Keep secrets and signed URLs out of terminal output, logs, Git, and final replies; disable shell tracing.

Policy (max_files=0 means unlimited; completed and pending uploads consume slots):
${policy}

Temporary credentials, for this gateway only:
${credentials}

1. GET UPLOAD_ENDPOINT with Authorization: Bearer UPLOAD_TOKEN. Verify bucket, policy and expiry; ask which local files and object keys to use if unclear.
2. For each file POST JSON to UPLOAD_ENDPOINT with that Authorization header and Content-Type: application/json: {"key":"path/file.ext","size":123,"content_type":"application/octet-stream"}. Use the exact byte count and actual MIME type. Preserve the selected key; do not rename conflicts without permission.
3. PUT bytes only to the returned url, following method and required_headers. Never forward the invitation token to that URL, and never PUT to the gateway /s3 endpoint. Finish within expires_in. S3 is direct=true; WebDAV uses the returned relay URL.
4. Only after PUT succeeds, POST {"upload_id":"returned ID","key":"same key"} to the returned commit_url with the invitation Authorization and JSON content type. A repeated successful commit does not consume another slot; reuse its ID for retries. Do not claim success before commit succeeds.
5. Report committed keys and nonempty public_url only. Public aliases have no automatic expiry, but object deletion, link revocation or a private bucket disables them. Private buckets return no public URL.

On 409, distinguish upload_invitation_limit from upload_overwrite_denied; do not bypass limits or rename files silently. On 403 upload_invitation_inactive request a new invitation. For other failures retain the code and trace_id without credentials. This token grants no listing, reading, deletion, or administration.`
    : `请用以下 Arca 临时上传授权，把我指定的本地文件上传至指定桶。只做上传，不索取 AK/SK，不修改桶配置。

目标实例：${gateway}
先阅读 API 参考：${gateway}/docs/api.md?raw=1 中的 Temporary upload invitations。
所需工具：优先使用现有项目的 HTTP 库，也可使用 curl 与 Python 3（或 jq）构造和解析 JSON；开始前检查工具是否已安装。此流程不需要下载 rvsctl，也不能调用 /agent/claim。关闭 shell 调试输出，不要将临时令牌或签名 URL 打印到终端、日志、Git 或最终回复。

授权范围（max_files=0 表示不限；已提交和正在上传的文件都占用名额）：
${policy}

临时凭据（仅发送至上述平台实例）：
${credentials}

1. 携带 Authorization: Bearer UPLOAD_TOKEN，GET UPLOAD_ENDPOINT 核对桶、到期时间和覆盖规则。不清楚文件或对象键时先向我确认。
2. 每个文件先携带同一授权头和 Content-Type: application/json，POST 至 UPLOAD_ENDPOINT：{"key":"path/file.ext","size":123,"content_type":"application/octet-stream"}。size 必须是实际字节数，MIME 按实际类型填写。保留选定对象键，冲突时不擅自改名。
3. 只把原始文件字节 PUT 到响应 url，原样遵守 method、required_headers，并在 expires_in 内完成。绝不把 UPLOAD_TOKEN 转发给这个存储 URL，不向平台 /s3 直接 PUT。S3 的 direct=true，WebDAV 使用返回的中转地址。
4. 只有 PUT 成功后，才带同一授权头和 JSON 类型，向响应 commit_url POST：{"upload_id":"响应中的 ID","key":"同一对象键"}。commit 成功才算上传成功；重试复用 upload_id，成功的重复提交不会重复计数。
5. 最终仅报告成功提交的对象键及非空的 public_url。公开入口无自动到期时间，但对象删除、链接撤销、桶改为私有后失效；私有桶没有公开链接。

遇到 409，区分 upload_invitation_limit（数量已满）与 upload_overwrite_denied（禁止覆盖），不要绕过限制或擅自改名。遇到 403 upload_invitation_inactive，申请新授权。其他失败保留错误码和 trace_id，不附带凭据。此令牌没有列举、读取、删除或管理权限。`;
}
function Prompt({
  result,
  close,
}: {
  result: UploadInvitationResult;
  close: () => void;
}) {
  useVersionHold();
  const [copied, setCopied] = useState("");
  const text = invitationPrompt(result);
  useEffect(() => {
    if (!copied) return;
    const timer = setTimeout(() => setCopied(""), 2000);
    return () => clearTimeout(timer);
  }, [copied]);
  return (
    <Modal title={msg("保存临时上传提示词")} close={close} dismissible={false}>
      <div className="notice">
        <strong>{msg("仅显示这一次")}</strong>
        <p>
          {msg(
            "提示词包含临时上传凭据。请只交给受信任的 Agent；关闭后无法重新查看。",
          )}
        </p>
      </div>
      <div className="code-panel">
        <Button
          type="button"
          onClick={async () => {
            try {
              await navigator.clipboard.writeText(text);
              setCopied(msg("已复制"));
            } catch {
              setCopied(msg("复制失败，请手动选择文本"));
            }
          }}
        >
          <Copy size={16} />
          <span aria-live="polite">{copied || msg("一键复制提示词")}</span>
        </Button>
        <pre tabIndex={0} aria-label={msg("临时上传接入提示词")}>
          {text}
        </pre>
      </div>
      <div className="form-actions">
        <Button type="button" className="primary" onClick={close}>
          {msg("我已保存提示词")}
        </Button>
      </div>
    </Modal>
  );
}
function Create({
  api,
  bucket,
  close,
  done,
}: {
  api: API;
  bucket: string;
  close: () => void;
  done: (v: UploadInvitationResult) => void;
}) {
  useVersionHold();
  const [name, setName] = useState("Agent upload");
  const [max, setMax] = useState(10);
  const [unlimited, setUnlimited] = useState(false);
  const [expiry, setExpiry] = useState("3600");
  const [overwrite, setOverwrite] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  return (
    <Modal title={msg("生成临时上传授权")} close={close} busy={busy}>
      <form
        onSubmit={async (e) => {
          e.preventDefault();
          if (busy) return;
          setBusy(true);
          setError("");
          try {
            done(
              await api.createUploadInvitation(bucket, {
                name,
                max_files: unlimited ? 0 : max,
                expires_in: Number(expiry),
                allow_overwrite: overwrite,
              }),
            );
          } catch (e) {
            setError((e as Error).message);
          } finally {
            setBusy(false);
          }
        }}
      >
        <Input
          label={msg("授权名称")}
          required
          maxLength={160}
          value={name}
          disabled={busy}
          onValueChange={setName}
        />
        <Select
          label={msg("有效期")}
          value={expiry}
          disabled={busy}
          onValueChange={setExpiry}
          options={[
            {
              value: "900",
              label: msg("15 分钟"),
            },
            {
              value: "3600",
              label: msg("1 小时"),
            },
            {
              value: "86400",
              label: msg("24 小时"),
            },
            {
              value: "604800",
              label: msg("7 天"),
            },
          ]}
        />
        <Checkbox
          checked={unlimited}
          disabled={busy}
          onCheckedChange={setUnlimited}
          label={msg("不限文件数量")}
        />
        {!unlimited && (
          <Input
            label={msg("最多上传文件数")}
            type="number"
            required
            min={1}
            max={1000000}
            step={1}
            value={String(max)}
            disabled={busy}
            onValueChange={(v) => setMax(Number(v))}
          />
        )}
        <Checkbox
          checked={overwrite}
          disabled={busy}
          onCheckedChange={setOverwrite}
          label={msg("允许覆盖桶内同名文件")}
        />
        <p className="muted">
          {msg(
            "默认拒绝覆盖同名对象；已提交和正在上传的文件均占名额。数量不限仍受桶容量与平台限制。",
          )}
        </p>
        {error && (
          <div className="error-box" role="alert">
            {error}
          </div>
        )}
        <div className="form-actions">
          <Button type="button" disabled={busy} onClick={close}>
            {msg("取消")}
          </Button>
          <Button className="primary" loading={busy}>
            {msg("生成提示词")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
export function UploadInvitations({
  api,
  bucket,
  notify,
}: {
  api: API;
  bucket: Bucket;
  notify: (s: string) => void;
}) {
  const [list, setList] = useState<UploadInvitation[]>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [creating, setCreating] = useState(false);
  const [result, setResult] = useState<UploadInvitationResult>();
  const [revoking, setRevoking] = useState<UploadInvitation>();
  const [busy, setBusy] = useState(false);
  const generation = useRef(0);
  const load = useCallback(() => {
    const request = ++generation.current;
    setLoading(true);
    setError("");
    api
      .uploadInvitations(bucket.slug)
      .then((value) => {
        if (request === generation.current) setList(value);
      })
      .catch((error) => {
        if (request === generation.current) setError(error.message);
      })
      .finally(() => {
        if (request === generation.current) setLoading(false);
      });
  }, [api, bucket.slug]);
  useEffect(() => {
    setList(undefined);
    setError("");
    load();
    return () => {
      generation.current++;
    };
  }, [load]);
  return (
    <section className="temporary-uploads" aria-label={msg("临时 Agent 上传")}>
      <div className="card-heading">
        <div>
          <h3>{msg("临时 Agent 上传")}</h3>
          <p className="muted">
            {msg("只授权向此桶上传，按数量和有效期限制，随时可撤销。")}
          </p>
        </div>
        <div className="detail-actions">
          <Button
            type="button"
            disabled={loading}
            aria-label={msg("刷新上传授权")}
            onClick={load}
          >
            <RefreshCw size={16} />
          </Button>
          <Button
            type="button"
            className="secondary"
            onClick={() => setCreating(true)}
          >
            <Plus size={16} />
            {msg("生成上传提示词")}
          </Button>
        </div>
      </div>
      <p className="muted">
        {bucket.visibility === "public"
          ? msg(
              "上传成功后返回无自动到期时间的公开入口。删除对象、撤销链接或桶改为私有后失效。",
            )
          : msg("这是私有桶，临时授权没有读取权限，也不会产生公开链接。")}
      </p>
      {error && (
        <div className="error-box" role="alert">
          {error}
          <Button type="button" onClick={load}>
            {msg("重试")}
          </Button>
        </div>
      )}
      {loading && !list ? (
        <Loading label={msg("正在读取上传授权")} />
      ) : list?.length ? (
        <ul className="invitation-list">
          {list.map((i) => {
            const expired = Date.parse(i.expires_at) <= Date.now();
            return (
              <li key={i.id}>
                <div>
                  <strong>{i.name}</strong>
                  <p>
                    {msg("已完成 ")}
                    {i.used_files}
                    {msg(" · 上传中 ")}
                    {i.pending_files} /{" "}
                    {i.max_files === 0 ? msg("不限") : i.max_files}
                  </p>
                  <p>
                    {i.allow_overwrite ? msg("允许覆盖") : msg("禁止覆盖")}
                    {msg(" · 到期 ")}
                    {new Date(i.expires_at).toLocaleString(getLanguage())}
                  </p>
                </div>
                <div className="detail-actions">
                  <span className="pill">
                    {i.revoked
                      ? msg("已撤销")
                      : expired
                        ? msg("已过期")
                        : msg("有效")}
                  </span>
                  {!i.revoked && !expired && (
                    <Button
                      type="button"
                      onClick={() => {
                        setError("");
                        setRevoking(i);
                      }}
                    >
                      {msg("撤销授权")}
                    </Button>
                  )}
                </div>
              </li>
            );
          })}
        </ul>
      ) : (
        !loading &&
        !error && <p className="muted">{msg("还没有临时上传授权。")}</p>
      )}
      {creating && (
        <Create
          api={api}
          bucket={bucket.slug}
          close={() => setCreating(false)}
          done={(v) => {
            setCreating(false);
            setResult(v);
            load();
          }}
        />
      )}
      {result && <Prompt result={result} close={() => setResult(undefined)} />}
      {revoking && (
        <RiskDialog
          title={msg("撤销临时上传授权")}
          busy={busy}
          confirmLabel={msg("撤销授权")}
          onCancel={() => setRevoking(undefined)}
          onConfirm={async () => {
            if (busy) return;
            setBusy(true);
            setError("");
            try {
              await api.revokeUploadInvitation(bucket.slug, revoking.id);
              setRevoking(undefined);
              load();
              notify(msg("临时上传授权已撤销"));
            } catch (e) {
              setError((e as Error).message);
            } finally {
              setBusy(false);
            }
          }}
        >
          <p>
            {msg(
              "撤销后不能再申请或提交上传。已经提交的文件及其公开链接会保留。",
            )}
          </p>
          {error && <p role="alert">{error}</p>}
        </RiskDialog>
      )}
    </section>
  );
}
