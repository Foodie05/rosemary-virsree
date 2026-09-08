import{useEffect,useState}from'react';
import{AlertTriangle,ArrowRight,Check,Clipboard,Cloud,Code2,Download,ExternalLink,FileKey,KeyRound,Link2,LockKeyhole,Network,ShieldCheck,Terminal,Upload}from'lucide-react';

const DEFAULT_PROJECT='https://github.com/Foodie05/rosemary-virsree';
const DEFAULT_RELEASE=`${DEFAULT_PROJECT}/releases`;
const sections=[['start','先分清两个网站'],['install','下载 rvsctl'],['claim','兑换一次性授权'],['configure','配置应用'],['upload','上传：签名直传'],['download','下载与公开链接'],['permissions','权限和配额'],['verify','验收清单'],['errors','常见错误']];

const samples={
  curl:`# 第 1 步：只向 Rosemary 发送 JSON，申请真实 S3 上传地址
curl -X POST "$RVS_GATEWAY/api/v1/buckets/$RVS_BUCKET/objects/upload" \\
  -H "X-RVS-Access-Key: $AWS_ACCESS_KEY_ID" \\
  -H "X-RVS-Secret-Key: $AWS_SECRET_ACCESS_KEY" \\
  -H 'Content-Type: application/json' \\
  -d '{"key":"images/cover.webp","size":284190,"content_type":"image/webp","expires_in":600}'

# 第 2 步：把文件 PUT 到响应中的真实 S3 url，而不是 Rosemary
curl -X PUT "$SIGNED_REAL_S3_URL" \\
  -H 'Content-Type: image/webp' --data-binary @cover.webp

# 第 3 步：PUT 成功后调用响应中的 commit_url
curl -X POST "$COMMIT_URL" \\
  -H "X-RVS-Access-Key: $AWS_ACCESS_KEY_ID" \\
  -H "X-RVS-Secret-Key: $AWS_SECRET_ACCESS_KEY" \\
  -H 'Content-Type: application/json' \\
  -d '{"upload_id":"obj_...","key":"images/cover.webp"}'`,
  ts:`const signed = await fetch(
  \`${'${gateway}'}/api/v1/buckets/${'${bucket}'}/objects/upload\`, {
    method: 'POST',
    headers: authHeaders,
    body: JSON.stringify({
      key, size: file.size, content_type: file.type,
      expires_in: 600, // 由当前业务明确选择
    }),
  }).then(assertOK).then(r => r.json());

// 这一请求直达真实 S3；signed.url 不要写日志
await fetch(signed.url, {
  method: 'PUT',
  headers: signed.required_headers,
  body: file,
}).then(assertOK);

await fetch(signed.commit_url, {
  method: 'POST', headers: authHeaders,
  body: JSON.stringify({ upload_id: signed.upload_id, key }),
}).then(assertOK);`,
  py:`signed = requests.post(
    f"{gateway}/api/v1/buckets/{bucket}/objects/upload",
    headers=auth_headers,
    json={"key": key, "size": len(data),
          "content_type": content_type, "expires_in": 600},
).raise_for_status().json()

# 文件正文只发送到真实 S3
requests.put(signed["url"], data=data,
             headers=signed["required_headers"]).raise_for_status()

requests.post(signed["commit_url"], headers=auth_headers,
              json={"upload_id": signed["upload_id"],
                    "key": key}).raise_for_status()`
};

function Copy({text}:{text:string}){const[done,setDone]=useState(false);return <button className="docs-copy" onClick={()=>{navigator.clipboard.writeText(text);setDone(true);setTimeout(()=>setDone(false),1200)}}><Clipboard size={14}/>{done?'已复制':'复制'}</button>}
function Code({children}:{children:string}){return <div className="guide-code"><Copy text={children}/><pre>{children}</pre></div>}

export function FullDocsPage(){const[lang,setLang]=useState<keyof typeof samples>('curl');const[links,setLinks]=useState({project_url:DEFAULT_PROJECT,release_url:DEFAULT_RELEASE});const gateway=location.origin;useEffect(()=>{fetch('/api/v1/meta').then(r=>r.ok?r.json():Promise.reject()).then(v=>setLinks({project_url:v.project_url||DEFAULT_PROJECT,release_url:v.release_url||DEFAULT_RELEASE})).catch(()=>{})},[]);const project=links.project_url.replace(/\/$/,'');const releases=links.release_url.replace(/\/$/,'');const install=`# macOS / Linux：自动识别系统和 CPU\nOS=$(uname -s | tr '[:upper:]' '[:lower:]')\nARCH=$(uname -m)\n[ "$ARCH" = x86_64 ] && ARCH=amd64\n[ "$ARCH" = aarch64 ] && ARCH=arm64\n\ncurl -fsSLo rvsctl "${releases}/latest/download/rvsctl-$OS-$ARCH"\ncurl -fsSLo SHA256SUMS "${releases}/latest/download/SHA256SUMS"\nchmod 0755 rvsctl\nEXPECTED=$(awk -v f="rvsctl-$OS-$ARCH" '$2==f {print $1}' SHA256SUMS)\nprintf '%s  %s\\n' "$EXPECTED" rvsctl | shasum -a 256 -c -`;
return <div className="guide"><div className="guide-hero"><div><p className="eyebrow">COMPLETE INTEGRATION GUIDE</p><h2>从一台陌生应用开始接入</h2><p>先找到正确的实例，安全兑换虚拟凭据，再实现签名上传、下载和验收。每一步都给出可以执行的命令。</p></div><a href={`${gateway}/docs/integration.md`} target="_blank">查看机器可读版 <ExternalLink size={14}/></a></div>
<div className="address-map"><div><Network/><span>你的 Rosemary 实例<strong>{gateway}</strong><small>应用 API、虚拟 S3、健康检查都配置到这里</small></span></div><ArrowRight/><div><Code2/><span>开源项目<strong>{project.replace(/^https?:\/\//,'')}</strong><small>源码、Issue、Release；不能作为存储 endpoint</small></span></div></div>
<div className="guide-layout"><nav>{sections.map(([id,label],i)=><a key={id} href={`#${id}`}><b>{String(i+1).padStart(2,'0')}</b>{label}</a>)}</nav><main>
<section id="start"><div className="section-icon"><Network/></div><h3>先分清两个网站</h3><p>Rosemary VirSree 是开源软件，因此每个组织都可以部署自己的实例。应用必须连接管理员给你的实例地址，例如 <code>{gateway}</code>。开源仓库 <a href={project} target="_blank">{project}</a> 只负责源码、Issue 和通用说明；CLI 从该实例声明的 <a href={releases} target="_blank">Release 页面</a>下载。</p><div className="do-dont"><div><Check/><span><strong>应用配置</strong><code>AWS_ENDPOINT_URL={gateway}/s3</code></span></div><div className="dont"><AlertTriangle/><span><strong>不要这样配置</strong><code>AWS_ENDPOINT_URL={project}</code></span></div></div><p>开始前运行 <code>curl -fsS {gateway}/health</code>。只有返回 <code>backend_ready: true</code> 才能继续对象操作；也可查看 <code>{gateway}/api/v1/meta</code> 核对这三个地址。</p></section>
<section id="install"><div className="section-icon"><Download/></div><h3>下载并校验 rvsctl</h3><p><code>rvsctl</code> 只在首次接入时使用：它拿一次性 Token 创建虚拟桶，并把 AK/SK 写入受限 Secret 文件。请从本项目 GitHub Releases 下载与你的操作系统、CPU 匹配的版本。</p><Code>{install}</Code><p className="note"><ShieldCheck/>把输出哈希与 <code>SHA256SUMS</code> 中对应文件比较。平台也可配置自己的 Release 地址，Agent 提示词会使用该实例声明的地址。</p></section>
<section id="claim"><div className="section-icon"><FileKey/></div><h3>兑换一次性授权</h3><p>管理员在“Agent 接入”页面生成 Token。先选择桶名、配额和 Secret 位置；确认目标文件不存在并已被 Git 忽略，然后只执行一次：</p><Code>{`./rvsctl \\\n  -endpoint "${gateway}" \\\n  -token "$RVS_BOOTSTRAP_TOKEN" \\\n  -name "订单服务生产环境" \\\n  -bucket order-service-prod \\\n  -quota 10737418240 \\\n  -visibility private \\\n  -env-file /run/secrets/rosemary.env`}</Code><p><code>rvsctl</code> 不打印 AK/SK，也拒绝覆盖旧文件。成功后文件包含 endpoint、region、虚拟凭据、path-style 开关和桶名。</p></section>
<section id="configure"><div className="section-icon"><Cloud/></div><h3>配置应用，而不是 GitHub</h3><div className="config-grid">{[['AWS_ENDPOINT_URL',`${gateway}/s3`],['AWS_REGION','平台返回值，通常 us-east-1'],['AWS_ACCESS_KEY_ID','rvsctl 写入的虚拟 AK'],['AWS_SECRET_ACCESS_KEY','rvsctl 写入的虚拟 SK'],['AWS_S3_FORCE_PATH_STYLE','true'],['RVS_BUCKET','兑换时选择的虚拟桶名']].map(([k,v])=><div key={k}><code>{k}</code><span>{v}</span></div>)}</div><p>S3 SDK 可用于 ListObjectsV2、HeadObject、GetObject 和 DeleteObject。Rosemary 明确拒绝标准 PutObject，因为文件正文不能经过平台。</p></section>
<section id="upload"><div className="section-icon"><Upload/></div><h3>上传：签名 → 真实 S3 → commit</h3><p>上传永远是三步。第一步和第三步只传 JSON 到 Rosemary；第二步才传文件，而且目标必须是响应中的真实 S3 URL。</p><div className="mini-flow"><span>① 申请签名<small>JSON → Rosemary</small></span><ArrowRight/><span>② PUT 文件<small>bytes → Real S3</small></span><ArrowRight/><span>③ commit<small>JSON → Rosemary</small></span></div><div className="lang-tabs">{Object.keys(samples).map(k=><button className={lang===k?'active':''} key={k} onClick={()=>setLang(k as keyof typeof samples)}>{k==='curl'?'HTTP / cURL':k==='ts'?'TypeScript':'Python'}</button>)}</div><Code>{samples[lang]}</Code><div className="warning-box"><AlertTriangle/><div><strong>不要调用 <code>PutObject({gateway}/s3/...)</code></strong><p>它会返回 405。只有第二步允许 PUT，且目标必须是签名响应中的真实 S3 URL。</p></div></div></section>
<section id="download"><div className="section-icon"><Link2/></div><h3>下载与公开链接</h3><p>调用 <code>POST /api/v1/buckets/{'{bucket}'}/objects/download</code>，提交 <code>key</code> 和本次业务选择的 <code>expires_in</code>。响应 URL 直接指向真实私有 S3。</p><Code>{`POST ${gateway}/api/v1/buckets/media/objects/download\nX-RVS-Access-Key: <virtual AK>\nX-RVS-Secret-Key: <virtual SK>\nContent-Type: application/json\n\n{"key":"reports/q3.pdf","filename":"q3.pdf","expires_in":300}`}</Code><p>“公开链接”仍然基于签名：应用创建稳定的 <code>/p/...</code> 地址；访问时 Rosemary 重新签发并 307 跳转。保存创建响应的 <code>slug</code>，用 <code>DELETE .../public-links/{'{slug}'}</code> 撤销。底层桶始终 Private。</p></section>
<section id="permissions"><div className="section-icon"><LockKeyhole/></div><h3>权限和配额</h3><div className="permission-doc-grid">{[['read','列表、HEAD、下载签名'],['write','上传签名、commit'],['delete','删除对象'],['manage','公开链接、立即失效旧链']].map(([p,d])=><div><code>{p}</code><span>{d}</span></div>)}</div><p>权限互不包含，<code>manage</code> 不会获得读取能力。申请上传时先占用预留空间，commit 后转为已使用；桶配额之和受平台总配额约束。</p></section>
<section id="verify"><div className="section-icon"><Check/></div><h3>交付前逐项验收</h3><ol className="checklist"><li>健康检查显示真实后端 Ready。</li><li>凭据文件权限为 0600，日志和 Git diff 中没有 Token、AK/SK。</li><li>ListObjectsV2 与 HeadObject 正常。</li><li>直接向 Rosemary PutObject 返回 405。</li><li>上传签名 URL 的主机不是 Rosemary。</li><li>真实 S3 PUT 成功，commit 成功。</li><li>签名下载最终响应主机是真实 S3，内容一致。</li><li>没有 write 权限的 Key 申请上传返回 403。</li></ol></section>
<section id="errors"><div className="section-icon"><Terminal/></div><h3>遇到错误先看这里</h3><table className="error-table"><tbody>{[['backend_ready=false','真实 S3 尚未配置，请联系平台管理员'],['403 invalid credentials','虚拟凭据错误、解密主密钥变化或 Key 已撤销'],['403 permission denied','当前 Key 没有所需的独立权限'],['expires_in is required','应用必须为本次签名明确选择秒数'],['bucket quota exceeded','清理对象、等待过期预留回收或调整配额'],['commit 大小不匹配','申请时 size 与真实上传字节数不同'],['浏览器 CORS 失败','真实 S3 桶需要允许应用域名的 GET/HEAD/PUT'],['PutObject 返回 405','这是预期行为，请改用签名上传三步流程']].map(([e,d])=><tr key={e}><td><code>{e}</code></td><td>{d}</td></tr>)}</tbody></table><p>完整接口字段见 <a href={`${gateway}/docs/api.md`} target="_blank">API 文档</a>；仍无法解决时，在 <a href={`${project}/issues`} target="_blank">GitHub Issues</a> 提交脱敏后的复现。</p></section>
</main></div></div>}
