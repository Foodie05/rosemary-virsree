import { msg } from "./locale";
import {
  Children,
  createElement,
  isValidElement,
  useEffect,
  useState,
} from "react";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Brand } from "./Brand";
import { Button, Loading, ThemeToggle } from "./Forma";
import { useLanguage } from "./locale";
import { Copy, Download, ExternalLink } from "./icons";
const documents = [
  ["integration.md", "从零接入指南"],
  ["api.md", "API 参考"],
  ["agent-prompt.md", "Agent 提示词"],
  ["architecture.md", "架构说明"],
  ["operator-setup.md", "管理员部署指南"],
];
function CodeBlock({ children }: { children?: React.ReactNode }) {
  const [status, setStatus] = useState("");
  const text =
    (
      children as React.ReactElement<{
        children?: string;
      }>
    )?.props?.children || "";
  useEffect(() => {
    if (!status) return;
    const id = setTimeout(() => setStatus(""), 1800);
    return () => clearTimeout(id);
  }, [status]);
  return (
    <div className="document-code">
      <Button
        type="button"
        aria-label={msg("复制代码")}
        onClick={async () => {
          try {
            await navigator.clipboard.writeText(String(text));
            setStatus(msg("已复制"));
          } catch {
            setStatus(msg("复制失败"));
          }
        }}
      >
        <Copy size={16} />
        <span aria-live="polite">{status || msg("复制代码")}</span>
      </Button>
      <pre tabIndex={0} aria-label={msg("文档代码")}>
        {children}
      </pre>
    </div>
  );
}
export function DocumentPage() {
  useLanguage();
  const [body, setBody] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [retry, setRetry] = useState(0);
  const path = location.pathname;
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError("");
    fetch(`${path}?raw=1`, {
      signal: controller.signal,
      headers: {
        Accept: "text/markdown",
      },
      cache: "no-store",
    })
      .then(async (r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        return r.text();
      })
      .then(setBody)
      .catch((e) => {
        if (!controller.signal.aborted) setError(e.message);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [path, retry]);
  const slug = (text: string) =>
    text
      .toLowerCase()
      .replace(/[`*_~]/g, "")
      .replace(/[^\p{L}\p{N} -]/gu, "")
      .trim()
      .replace(/\s+/g, "-");
  const counts: Record<string, number> = {};
  const nextID = (text: string, seen: Record<string, number>) => {
    const base = slug(text) || "section";
    const n = seen[base] || 0;
    seen[base] = n + 1;
    return base + (n ? `-${n}` : "");
  };
  let fenced = false;
  const headings = body.split("\n").flatMap((line) => {
    if (/^\s*(```|~~~)/.test(line)) {
      fenced = !fenced;
      return [];
    }
    if (fenced || !/^#{1,3} /.test(line)) return [];
    const text = line.replace(/^#+ /, "").replace(/[`*_~]/g, "");
    return [{ text, id: nextID(text, counts) }];
  });
  const rendered: Record<string, number> = {};
  const plain = (node: React.ReactNode): string =>
    Children.toArray(node)
      .map((n) =>
        isValidElement<{ children?: React.ReactNode }>(n)
          ? plain(n.props.children)
          : String(n),
      )
      .join("");
  const heading =
    (level: number) =>
    ({ children }: { children?: React.ReactNode }) =>
      createElement(
        `h${level}`,
        { id: nextID(plain(children), rendered) },
        children,
      );
  return (
    <div className="document-shell">
      <header>
        <a href="/" title={msg("返回控制台")}>
          <Brand />
        </a>
        <div className="detail-actions">
          <ThemeToggle />
          <a className="document-home" href="/">
            {msg("返回控制台")}
          </a>
        </div>
      </header>
      <div className="document-layout">
        <nav aria-label={msg("技术文档")}>
          <p className="eyebrow">{msg("技术文档")}</p>
          {documents.map(([name, label]) => (
            <a
              key={name}
              href={`/docs/${name}`}
              aria-current={path.endsWith(name) ? "page" : undefined}
            >
              {msg(label)}
            </a>
          ))}
        </nav>
        <main className="document-main">
          <div className="document-toolbar">
            <a href={`${path}?raw=1`}>
              <ExternalLink size={16} />
              {msg("原始 Markdown")}
            </a>
            <a href={`${path}?raw=1&download=1`}>
              <Download size={16} />
              {msg("下载文档")}
            </a>
          </div>
          {loading ? (
            <Loading label={msg("正在加载文档")} />
          ) : error ? (
            <div className="error-box" role="alert">
              <p>{error}</p>
              <Button type="button" onClick={() => setRetry((v) => v + 1)}>
                {msg("重试")}
              </Button>
            </div>
          ) : (
            <article className="markdown-body">
              <Markdown
                remarkPlugins={[remarkGfm]}
                components={{
                  pre: CodeBlock,
                  h1: heading(1),
                  h2: heading(2),
                  h3: heading(3),
                  table: ({ children }) => (
                    <table tabIndex={0}>{children}</table>
                  ),
                  a: ({ href, children, ...props }) => (
                    <a
                      {...props}
                      href={href}
                      {...(href?.startsWith("http")
                        ? {
                            target: "_blank",
                            rel: "noopener noreferrer",
                          }
                        : {})}
                    >
                      {children}
                    </a>
                  ),
                }}
              >
                {body}
              </Markdown>
            </article>
          )}
        </main>
        <aside aria-label={msg("本页内容")}>
          <p className="eyebrow">{msg("本页内容")}</p>
          {headings.map((h) => (
            <a key={h.id} href={`#${h.id}`}>
              {h.text}
            </a>
          ))}
        </aside>
      </div>
    </div>
  );
}
