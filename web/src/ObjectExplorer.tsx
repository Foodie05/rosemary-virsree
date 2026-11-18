import { msg, msgt } from "./locale";
import { Button, Input, Modal } from "./Forma";
import { useCallback, useEffect, useState } from "react";
import type { FormEvent } from "react";
import {
  ArrowLeft,
  ChevronRight,
  Copy,
  Download,
  File,
  Folder,
  KeyRound,
  Loader2,
  RefreshCw,
  Trash2,
} from "./icons";
import { API, ObjectBrowsePage, ObjectInfo } from "./api";
import { RiskDialog } from "./RiskDialog";
import { useVersionHold } from "./VersionGuard";
const PAGE_SIZE = 40;
const size = (n: number) =>
  n >= 1 << 30
    ? `${(n / (1 << 30)).toFixed(1)} GB`
    : n >= 1 << 20
      ? `${(n / (1 << 20)).toFixed(1)} MB`
      : n >= 1 << 10
        ? `${(n / (1 << 10)).toFixed(1)} KB`
        : `${n} B`;
export function ObjectExplorer({
  api,
  bucket,
  notify,
}: {
  api: API;
  bucket: string;
  notify: (message: string) => void;
}) {
  const [prefix, setPrefix] = useState("");
  const [draft, setDraft] = useState("");
  const [cursors, setCursors] = useState<string[]>([""]);
  const [page, setPage] = useState<ObjectBrowsePage>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [refresh, setRefresh] = useState(0);
  const [risk, setRisk] = useState<{
    kind: "invalidate" | "delete";
    object: ObjectInfo;
  } | null>(null);
  const [download, setDownload] = useState<ObjectInfo | null>(null);
  const [ttl, setTTL] = useState("900");
  const [signedURL, setSignedURL] = useState("");
  const [actionError, setActionError] = useState("");
  const [busy, setBusy] = useState(false);
  useVersionHold(Boolean(download));
  const cursor = cursors[cursors.length - 1];
  const load = useCallback(() => setRefresh((n) => n + 1), []);
  useEffect(() => {
    let active = true;
    setLoading(true);
    setError("");
    api
      .browseObjects(bucket, prefix, cursor, PAGE_SIZE)
      .then((result) => {
        if (active) setPage(result);
      })
      .catch((e) => {
        if (active) setError(e.message);
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [api, bucket, prefix, cursor, refresh]);
  const navigate = (path: string) => {
    setPrefix(path);
    setDraft(path);
    setCursors([""]);
    setPage(undefined);
  };
  const submit = (event: FormEvent) => {
    event.preventDefault();
    navigate(draft);
  };
  const closeRisk = () => {
    if (!busy) {
      setRisk(null);
      setActionError("");
    }
  };
  const confirmRisk = async () => {
    if (!risk) return;
    setBusy(true);
    setActionError("");
    try {
      if (risk.kind === "invalidate") {
        await api.invalidateObject(bucket, risk.object.key);
        notify(msg("对象物理键已轮换，旧直链已失效"));
      } else {
        await api.deleteObject(bucket, risk.object.key);
        notify(msg("对象已删除"));
      }
      setRisk(null);
      load();
    } catch (e: any) {
      setActionError(e.message || msg("操作失败，请重试"));
    } finally {
      setBusy(false);
    }
  };
  const confirmDownload = async () => {
    if (!download) return;
    const seconds = Number(ttl);
    if (!Number.isSafeInteger(seconds) || seconds < 1) {
      setActionError(msg("请输入大于 0 的整数秒数"));
      return;
    }
    setBusy(true);
    setActionError("");
    try {
      const result = await api.adminDownload(bucket, download.key, seconds);
      setSignedURL(result.url);
    } catch (e: any) {
      setActionError(e.message || msg("生成下载链接失败"));
    } finally {
      setBusy(false);
    }
  };
  const copyKey = async (key: string) => {
    try {
      await navigator.clipboard.writeText(key);
      notify(msg("对象键已复制"));
    } catch {
      notify(msg("复制失败，请检查浏览器剪贴板权限"));
    }
  };
  const segments = prefix.split("/").filter(Boolean);
  const crumbs = segments.map((segment, index) => ({
    label: segment,
    path: segments.slice(0, index + 1).join("/") + "/",
  }));
  return (
    <section className="object-explorer" aria-label={msg("对象管理")}>
      <div className="explorer-heading">
        <div>
          <p className="eyebrow">OBJECT EXPLORER</p>
          <h3>{msg("对象管理")}</h3>
          <p>
            {msg("按目录浏览，或输入完整对象键的前缀。每页最多 ")}
            {PAGE_SIZE}
            {msg(" 项。")}
          </p>
        </div>
        <Button
          className="explorer-refresh"
          onClick={load}
          disabled={loading}
          aria-label={msg("刷新对象列表")}
        >
          <RefreshCw size={17} />
          {msg("刷新")}
        </Button>
      </div>
      <nav className="explorer-crumbs" aria-label={msg("当前路径")}>
        <Button onClick={() => navigate("")}>{msg("桶根目录")}</Button>
        {crumbs.map((c) => (
          <span key={c.path}>
            <ChevronRight size={15} />
            <Button onClick={() => navigate(c.path)}>{c.label}</Button>
          </span>
        ))}
      </nav>
      <form className="explorer-search" onSubmit={submit}>
        <Input
          label={msg("对象键前缀")}
          value={draft}
          onValueChange={setDraft}
          placeholder={msg("例如 images/2026/ 或 reports/q3")}
          onKeyDown={(e) => {
            if (e.key === "Escape") {
              e.preventDefault();
              navigate("");
            }
          }}
        />
        <Button type="submit">{msg("定位")}</Button>
        {prefix && (
          <Button type="button" className="clear" onClick={() => navigate("")}>
            {msg("清除")}
          </Button>
        )}
      </form>
      {error ? (
        <div className="explorer-message error">
          <strong>{msg("对象列表加载失败")}</strong>
          <span>{error}</span>
          <Button onClick={load}>{msg("重试")}</Button>
        </div>
      ) : loading ? (
        <div className="explorer-message">
          <Loader2 className="spin" />
          <span>{msg("正在读取当前目录…")}</span>
        </div>
      ) : !page?.entries.length ? (
        <div className="explorer-message">
          <Folder size={25} />
          <strong>
            {prefix ? msg("当前路径没有对象") : msg("这个桶还没有对象")}
          </strong>
          <span>
            {prefix
              ? msg("检查前缀，或返回桶根目录。")
              : msg("应用完成签名上传和 commit 后，对象会出现在这里。")}
          </span>
          {prefix && (
            <Button onClick={() => navigate("")}>{msg("返回根目录")}</Button>
          )}
        </div>
      ) : (
        <div className="explorer-list" role="list">
          {page.entries.map((entry) => {
            const object = entry.object;
            const display = entry.key.slice(prefix.length).replace(/\/$/, "");
            return (
              <div className="explorer-row" role="listitem" key={entry.key}>
                <div
                  className={`explorer-file-icon ${entry.folder ? "folder" : ""}`}
                >
                  {entry.folder ? <Folder size={20} /> : <File size={20} />}
                </div>
                <div className="explorer-file-main">
                  {entry.folder ? (
                    <Button
                      className="explorer-name"
                      onClick={() => navigate(entry.key)}
                    >
                      {display}
                      <ChevronRight size={16} />
                    </Button>
                  ) : (
                    <strong className="explorer-name" title={entry.key}>
                      {display}
                    </strong>
                  )}
                  <small title={entry.key}>
                    {entry.folder
                      ? msg("文件夹")
                      : entry.key === display
                        ? object?.content_type || msg("文件")
                        : entry.key}
                  </small>
                </div>
                {!entry.folder && object && (
                  <>
                    <div className="explorer-file-meta">
                      <span>{size(object.size)}</span>
                      <small>
                        {new Date(object.updated_at).toLocaleString()}
                      </small>
                    </div>
                    <div className="explorer-actions">
                      <Button
                        title={msg("复制对象键")}
                        aria-label={msgt`复制 ${display} 的对象键`}
                        onClick={() => copyKey(object.key)}
                      >
                        <Copy size={16} />
                      </Button>
                      <Button
                        title={msg("下载")}
                        aria-label={msgt`下载 ${display}`}
                        onClick={() => {
                          setDownload(object);
                          setTTL("900");
                          setSignedURL("");
                          setActionError("");
                        }}
                      >
                        <Download size={16} />
                      </Button>
                      <Button
                        title={msg("失效旧链接")}
                        aria-label={msgt`失效 ${display} 的旧链接`}
                        onClick={() => {
                          setRisk({
                            kind: "invalidate",
                            object,
                          });
                          setActionError("");
                        }}
                      >
                        <KeyRound size={16} />
                      </Button>
                      <Button
                        className="danger"
                        title={msg("删除")}
                        aria-label={msgt`删除 ${display}`}
                        onClick={() => {
                          setRisk({
                            kind: "delete",
                            object,
                          });
                          setActionError("");
                        }}
                      >
                        <Trash2 size={16} />
                      </Button>
                    </div>
                  </>
                )}
              </div>
            );
          })}
        </div>
      )}
      {!loading && !error && page && (
        <div className="explorer-pagination">
          <span>
            {msg("第 ")}
            {cursors.length}
            {msg(" 页 · 当前 ")}
            {page.entries.length}
            {msg(" 项")}
            {page.next_cursor ? msg(" · 后面还有内容") : ""}
          </span>
          <div>
            <Button
              disabled={cursors.length === 1}
              onClick={() => setCursors((v) => v.slice(0, -1))}
            >
              <ArrowLeft size={15} />
              {msg("上一页")}
            </Button>
            <Button
              disabled={!page.next_cursor}
              onClick={() => setCursors((v) => [...v, page.next_cursor])}
            >
              {msg("下一页")}
              <ChevronRight size={15} />
            </Button>
          </div>
        </div>
      )}
      {download && (
        <Modal
          title={msg("生成下载链接")}
          busy={busy}
          close={() => {
            setDownload(null);
            setSignedURL("");
            setActionError("");
          }}
          className="explorer-download"
        >
          <p className="explorer-dialog-key">{download.key}</p>
          <div>
            <Input
              autoFocus
              type="number"
              min="1"
              step="1"
              onValueChange={(value) => {
                setTTL(value);
                setSignedURL("");
              }}
              value={String(ttl)}
              label={msg("链接有效期（秒）")}
            />
          </div>
          <p className="explorer-dialog-hint">
            {msg(
              "由你决定这次链接的有效期。S3 来源直达真实存储或 CDN；WebDAV 来源会使用中转能力链接。",
            )}
          </p>
          {actionError && (
            <p className="form-error" role="alert">
              {actionError}
            </p>
          )}
          <div className="actions">
            <Button
              className="ghost"
              disabled={busy}
              onClick={() => {
                setDownload(null);
                setSignedURL("");
                setActionError("");
              }}
            >
              {msg("取消")}
            </Button>
            {signedURL ? (
              <a
                className="primary"
                href={signedURL}
                target="_blank"
                rel="noopener noreferrer"
                onClick={() => setDownload(null)}
              >
                <Download size={16} />
                {msg("打开下载")}
              </a>
            ) : (
              <Button
                className="primary"
                disabled={busy}
                onClick={confirmDownload}
                loading={busy}
              >
                {<Download size={16} />}
                {msg("生成下载地址")}
              </Button>
            )}
          </div>
        </Modal>
      )}
      {risk && (
        <RiskDialog
          eyebrow={
            risk.kind === "delete" ? "PERMANENT DELETE" : "LINK INVALIDATION"
          }
          title={
            risk.kind === "delete"
              ? msg("确认永久删除对象")
              : msg("确认让旧直链失效")
          }
          lead={risk.object.key}
          busy={busy}
          cancelLabel={msg("返回检查")}
          confirmLabel={
            risk.kind === "delete" ? msg("永久删除") : msg("轮换物理键")
          }
          onCancel={closeRisk}
          onConfirm={confirmRisk}
        >
          <div className="risk-impact">
            <strong>
              {risk.kind === "delete"
                ? msg("此操作无法撤销")
                : msg("已签发的旧地址将立即失效")}
            </strong>
            <p>
              {risk.kind === "delete"
                ? msg("真实存储源中的文件和映射都会删除。")
                : msg("文件内容与逻辑对象键保持不变，旧物理键会被删除。")}
            </p>
            {actionError && (
              <p className="form-error" role="alert">
                {actionError}
              </p>
            )}
          </div>
        </RiskDialog>
      )}
    </section>
  );
}
