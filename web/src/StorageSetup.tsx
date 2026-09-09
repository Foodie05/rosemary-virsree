import {FormEvent, useEffect, useState} from 'react';
import type {ReactNode} from 'react';
import {ArrowRight, Check, ChevronRight, Cloud, Globe2, HardDrive, Loader2, Pencil, Plus, ShieldCheck, Waypoints, X} from 'lucide-react';
import {API} from './api';
import type {CDNMode, StorageSource, StorageSourceDetail, StorageSourceInput} from './api';
import {Brand} from './Brand';
import {RiskDialog} from './RiskDialog';

const gb=(n:number)=>n>=2**30?`${(n/2**30).toFixed(1)} GB`:`${(n/2**20).toFixed(0)} MB`;
const normalizeEndpoint=(raw:string)=>{let value=raw.trim().replace(/^\.+/,'');if(!value)return '';value=value.replace(/^https\/\//i,'https://').replace(/^http\/\//i,'http://').replace(/^https:\/(?!\/)/i,'https://').replace(/^http:\/(?!\/)/i,'http://');if(value.startsWith('//'))value='https:'+value;else if(!/^https?:\/\//i.test(value))value='https://'+value;return value.replace(/\/+$/,'')};
const normalizeS3ServiceEndpoint=(raw:string,bucket:string)=>{const value=normalizeEndpoint(raw);if(!value||!bucket.trim())return value;try{const u=new URL(value),prefix=bucket.trim().toLowerCase()+'.',host=u.hostname.toLowerCase();if(host.startsWith(prefix)&&host.slice(prefix.length).startsWith('s3.'))u.hostname=u.hostname.slice(prefix.length);return u.toString().replace(/\/$/,'')}catch{return value}};

export function OOBE({email,onComplete}:{email:string;onComplete:()=>void}){
  const[step,setStep]=useState<'welcome'|'source'|'done'>('welcome');
  return <div className="oobe"><header><Brand/><div className="oobe-user"><span>{email}</span><small>已通过 OIDC 验证</small></div></header><main><div className="oobe-progress"><i className="active"/><i className={step!=='welcome'?'active':''}/><i className={step==='done'?'active':''}/><span>{step==='welcome'?'欢迎':step==='source'?'连接存储源':'配置完成'}</span></div>{step==='welcome'&&<section className="welcome-card"><div className="welcome-visual"><div className="orbit one"><Cloud/></div><div className="orbit two"><HardDrive/></div><div className="core"><Waypoints/></div></div><p className="eyebrow">WELCOME TO VIRSREE</p><h1>先种下第一处<br/><em>私有存储。</em></h1><p>VirSree 会把真实存储源虚拟成应用熟悉的 S3 边界。以后还可以继续添加存储源；新对象先写入优先级最高且空间充足的来源。</p><button className="primary oobe-next" onClick={()=>setStep('source')}>开始配置 <ArrowRight size={18}/></button><div className="welcome-points"><span><Check/>凭据加密保存</span><span><Check/>添加时读写验证</span><span><Check/>统一容量调度</span></div></section>}{step==='source'&&<section className="source-step"><div className="source-step-head"><button onClick={()=>setStep('welcome')}>← 返回</button><p className="eyebrow">FIRST STORAGE SOURCE</p><h1>连接你的第一个存储源</h1><p>保存前会创建一个极小的探针对象，确认写入、读取和删除都可用。</p></div><StorageSourceForm submitLabel="验证并完成配置" onSaved={()=>setStep('done')}/></section>}{step==='done'&&<section className="done-card"><div><Check/></div><p className="eyebrow">SOURCE VERIFIED</p><h1>VirSree 已经准备好了</h1><p>第一个存储源已通过读写验证。接下来可以创建虚拟桶、签发应用凭据，或随时添加更多存储源。</p><button className="primary oobe-next" onClick={onComplete}>进入控制台 <ChevronRight size={18}/></button></section>}</main></div>;
}

function StorageModal({eyebrow,title,close,children}:{eyebrow:string;title:string;close:()=>void;children:ReactNode}){
  return <div className="modal-bg" onMouseDown={e=>{if(e.target===e.currentTarget)close()}}><div className="storage-modal"><div className="modal-head"><div><p className="eyebrow">{eyebrow}</p><h3>{title}</h3></div><button aria-label="关闭" onClick={close}><X/></button></div>{children}</div></div>;
}

function BucketRiskDialog({oldBucket,newBucket,used,busy,onCancel,onConfirm}:{oldBucket:string;newBucket:string;used:number;busy:boolean;onCancel:()=>void;onConfirm:()=>void}){
  return <RiskDialog eyebrow="STORAGE AVAILABILITY RISK" title="确认更换真实存储桶" lead="这个操作会改变 VirSree 读取现有对象的位置。" busy={busy} cancelLabel="返回检查" confirmLabel="理解风险，重新验证" onCancel={onCancel} onConfirm={onConfirm}><div className="risk-comparison"><div><small>当前桶</small><code>{oldBucket}</code></div><ArrowRight/><div><small>新桶</small><code>{newBucket}</code></div></div><div className="risk-impact"><strong>请先确认数据迁移计划</strong><p>当前记录的占用空间为 {gb(used)}。保存后，VirSree 会从新桶读取相同的物理对象键；若对象尚未迁移，现有文件的下载、删除和链接轮换会失败。VirSree 不会自动搬迁数据。</p></div></RiskDialog>;
}

export function StoragePage({notify}:{notify:(s:string)=>void}){
  const api=new API();
  const[sources,setSources]=useState<StorageSource[]>([]);
  const[loading,setLoading]=useState(true);
  const[open,setOpen]=useState(false);
  const[editing,setEditing]=useState<StorageSourceDetail|null>(null);
  const[editingID,setEditingID]=useState('');
  const[error,setError]=useState('');
  const load=()=>{setLoading(true);setError('');api.storageSources().then(v=>setSources(v||[])).catch(e=>setError(e.message)).finally(()=>setLoading(false))};
  useEffect(load,[]);
  const beginEdit=async(id:string)=>{setEditingID(id);setError('');try{setEditing(await api.storageSource(id))}catch(e:any){setError(e.message)}finally{setEditingID('')}};
  return <><div className="section-head"><div><p className="eyebrow">PHYSICAL STORAGE POOL</p><h2>存储源</h2><p>按优先级分配新对象；当前来源空间不足时自动尝试下一处。</p></div><button className="primary small" onClick={()=>setOpen(true)}><Plus size={16}/>添加存储源</button></div>{error&&<div className="error"><ShieldCheck/>{error}</div>}{loading?<div className="skeletons"><i/><i/><i/><i/></div>:<div className="source-grid">{sources.map((s,index)=>{const used=s.used_bytes+s.reserved_bytes,pct=Math.min(100,used/s.capacity_bytes*100);return <article className="source-card" key={s.id}><div className="source-rank"><span>优先级 {s.priority}</span>{index===0&&<b>首选</b>}</div><div className="source-title"><div>{s.kind==='s3'?<Cloud/>:<Globe2/>}</div><div><h3>{s.name}</h3><code>{s.kind.toUpperCase()}</code></div><span className="status"><i/>已验证</span></div><div className="source-capacity"><div><span>{gb(used)} 已占用</span><span>{gb(s.capacity_bytes)}</span></div><div className="bar"><i style={{width:`${pct}%`}}/></div></div><footer><span>{s.direct_transfer?'文件直达存储':'经 VirSree 安全中转'}</span><div>{s.cdn_enabled&&<b>CDN 下载</b>}<button className="source-edit" aria-label={`编辑${s.name}`} disabled={editingID===s.id} onClick={()=>beginEdit(s.id)}>{editingID===s.id?<Loader2 className="spin"/>:<Pencil/>}编辑</button></div></footer></article>})}</div>}{open&&<StorageModal eyebrow="NEW STORAGE SOURCE" title="添加存储源" close={()=>setOpen(false)}><StorageSourceForm submitLabel="验证并添加" onSaved={()=>{setOpen(false);load();notify('存储源已验证并加入调度池')}}/></StorageModal>}{editing&&<StorageModal eyebrow="EDIT STORAGE SOURCE" title="编辑存储源" close={()=>setEditing(null)}><div className="edit-verification-note"><ShieldCheck/><span><strong>更新前重新验证</strong><small>新配置通过完整读、写、复制和删除验证后才会生效；验证失败时原配置继续运行。</small></span></div><StorageSourceForm initial={editing} submitLabel="重新验证并保存" onSaved={()=>{setEditing(null);load();notify('存储源已重新验证并更新')}}/></StorageModal>}</>;
}

function StorageSourceForm({submitLabel,onSaved,initial}:{submitLabel:string;onSaved:()=>void;initial?:StorageSourceDetail}){
  const editing=Boolean(initial);
  const[kind,setKind]=useState<'s3'|'webdav'>(initial?.kind||'s3');
  const[name,setName]=useState(initial?.name||'');
  const[priority,setPriority]=useState(initial?.priority??10);
  const[capacity,setCapacity]=useState(initial?initial.capacity_bytes/2**30:100);
  const[endpoint,setEndpoint]=useState(initial?.endpoint||'');
  const[publicEndpoint,setPublicEndpoint]=useState(initial?.public_endpoint||'');
  const[region,setRegion]=useState(initial?.region||'us-east-1');
  const[bucket,setBucket]=useState(initial?.bucket||'');
  const[accessKey,setAccessKey]=useState('');
  const[secretKey,setSecretKey]=useState('');
  const[pathStyle,setPathStyle]=useState(initial?.path_style??false);
  const[cdn,setCDN]=useState(initial?.cdn_endpoint||'');
  const[cdnMode,setCDNMode]=useState<CDNMode>(initial?.cdn_mode||'s3_sigv4');
  const[cdnAuthKey,setCDNAuthKey]=useState('');
  const[username,setUsername]=useState('');
  const[password,setPassword]=useState('');
  const[busy,setBusy]=useState(false);
  const[error,setError]=useState('');
  const[riskOpen,setRiskOpen]=useState(false);

  const save=async(acknowledgeBucketChange=false)=>{
    setBusy(true);setError('');
    const cleanEndpoint=kind==='s3'?normalizeS3ServiceEndpoint(endpoint,bucket):normalizeEndpoint(endpoint);
    const cleanPublicEndpoint=normalizeS3ServiceEndpoint(publicEndpoint,bucket);
    const cleanCDN=normalizeEndpoint(cdn);
    setEndpoint(cleanEndpoint);setPublicEndpoint(cleanPublicEndpoint);setCDN(cleanCDN);
    const payload:StorageSourceInput={name,kind,priority,capacity_bytes:Math.round(capacity*2**30),endpoint:cleanEndpoint};
    if(kind==='s3')Object.assign(payload,{public_endpoint:cleanPublicEndpoint,region,bucket:bucket.trim(),access_key:accessKey,secret_key:secretKey,path_style:pathStyle,cdn_endpoint:cleanCDN,cdn_mode:cdnMode,cdn_auth_key:cdnAuthKey,acknowledge_bucket_change:acknowledgeBucketChange});
    else Object.assign(payload,{webdav_username:username,webdav_password:password});
    try{
      if(initial)await new API().updateStorageSource(initial.id,payload);else await new API().addStorageSource(payload);
      onSaved();
    }catch(e:any){setError(e.message)}finally{setBusy(false)}
  };
  const submit=(e:FormEvent)=>{e.preventDefault();if(initial?.kind==='s3'&&bucket.trim()!==initial.bucket){setRiskOpen(true);return}void save()};
  const confirmRisk=()=>{setRiskOpen(false);void save(true)};
  const secretHint=editing?'留空则继续使用已加密保存的凭据':'';
  const canReuseCDNKey=Boolean(editing&&initial?.cdn_mode==='bitiful_token'&&initial.cdn_auth_key_configured&&cdnMode==='bitiful_token'&&normalizeEndpoint(cdn)===initial.cdn_endpoint);

  return <><form className="storage-form" onSubmit={submit}><div className={`kind-tabs ${editing?'locked':''}`}><button type="button" disabled={editing} className={kind==='s3'?'active':''} onClick={()=>setKind('s3')}><Cloud/><span><strong>S3 兼容存储</strong><small>AWS S3、OSS、COS、R2、MinIO 等</small></span></button><button type="button" disabled={editing} className={kind==='webdav'?'active':''} onClick={()=>setKind('webdav')}><Globe2/><span><strong>WebDAV</strong><small>NAS、Nextcloud 与标准 WebDAV 服务</small></span></button></div><div className="form-section"><div className="form-section-title"><span>01</span><div><strong>调度信息</strong><small>容量用于阻止超额分配，不会修改存储服务本身的配额。</small></div></div><div className="field-grid three"><label>显示名称<input required value={name} onChange={e=>setName(e.target.value)} placeholder={kind==='s3'?'主对象存储':'归档 WebDAV'}/></label><label>优先级<input required type="number" min="0" value={priority} onChange={e=>setPriority(+e.target.value)}/><small>数字越小越优先</small></label><label>可用容量（GB）<input required type="number" min="1" step="0.1" value={capacity} onChange={e=>setCapacity(+e.target.value)}/></label></div></div>{kind==='s3'?<><div className="form-section"><div className="form-section-title"><span>02</span><div><strong>S3 连接</strong><small>使用具备对象读、写、复制和删除权限的真实桶凭据。</small></div></div><div className="field-grid"><label>内部 API Endpoint<input value={endpoint} onChange={e=>setEndpoint(e.target.value)} onBlur={()=>setEndpoint(normalizeS3ServiceEndpoint(endpoint,bucket))} placeholder="s3.example.com（AWS 可留空）"/><small>可以只填域名；VirSree 会自动补全 HTTPS 并修正常见格式。</small></label><label>应用直传 Endpoint<input value={publicEndpoint} onChange={e=>setPublicEndpoint(e.target.value)} onBlur={()=>setPublicEndpoint(normalizeS3ServiceEndpoint(publicEndpoint,bucket))} placeholder="留空则与内部 Endpoint 相同"/></label><label>Region<input required value={region} onChange={e=>setRegion(e.target.value)} placeholder="us-east-1"/></label><label>真实私有桶名称<input required value={bucket} onChange={e=>setBucket(e.target.value)} placeholder="private-assets"/></label><label>Access Key<input required={!editing} autoComplete="off" value={accessKey} onChange={e=>setAccessKey(e.target.value)} placeholder={secretHint}/><small>{secretHint}</small></label><label>Secret Key<input required={!editing} type="password" autoComplete="new-password" value={secretKey} onChange={e=>setSecretKey(e.target.value)} placeholder={secretHint}/><small>{secretHint}</small></label></div><label className="check-row"><input type="checkbox" checked={pathStyle} onChange={e=>setPathStyle(e.target.checked)}/><span><strong>使用 Path-style 地址</strong><small>公有云 S3 兼容服务通常关闭；MinIO 和多数自建 S3 服务需要开启。</small></span></label></div><div className="form-section cdn-section"><div className="form-section-title"><span>03</span><div><strong>下载 CDN（可选）</strong><small>仅用于生成下载 URL；上传始终使用上面的直传 Endpoint。</small></div></div><label>CDN 加速域名<input value={cdn} onChange={e=>setCDN(e.target.value)} onBlur={()=>setCDN(normalizeEndpoint(cdn))} placeholder="https://cdn.example.com"/><small>留空则从应用直传 Endpoint 下载。填写后请选择该 CDN 实际启用的鉴权协议。</small></label><fieldset className="cdn-mode"><legend>下载鉴权方式</legend><div><button type="button" className={cdnMode==='bitiful_token'?'active':''} onClick={()=>setCDNMode('bitiful_token')}><strong>缤纷云高级鉴权</strong><small><code>_ts</code> + <code>_btf_tk</code></small></button><button type="button" className={cdnMode==='s3_sigv4'?'active':''} onClick={()=>setCDNMode('s3_sigv4')}><strong>S3 SigV4 兼容</strong><small>AWS 签名查询参数</small></button></div></fieldset>{cdnMode==='bitiful_token'&&<label>CDN 鉴权 Key<input required={Boolean(cdn)&&!canReuseCDNKey} type="password" autoComplete="new-password" value={cdnAuthKey} onChange={e=>setCDNAuthKey(e.target.value)} placeholder={canReuseCDNKey?'留空则继续使用已加密保存的 Key':'从缤纷云 CDN 项目复制'}/><small>路径：缤纷云控制台 → CDN 项目 → 高级鉴权 → 鉴权 Key。VirSree 加密保存，不会在接口、日志或页面中回显。</small></label>}<div className="cdn-help"><Waypoints/><div><strong>{cdnMode==='bitiful_token'?'适用于缤纷云 CDN 加速域名':'仅适用于真正接受 SigV4 的下载 Endpoint'}</strong><p>{cdnMode==='bitiful_token'?'VirSree 会按应用每次提交的 expires_in 生成 _ts，并在服务器内计算 _btf_tk；文件由 CDN 直接返回。':'普通 CDN 自定义域名通常不接受 S3 SigV4。只有服务商明确说明兼容 AWS 预签名 GET 时才选择此项。'}</p></div></div></div></>:<div className="form-section"><div className="form-section-title"><span>02</span><div><strong>WebDAV 连接</strong><small>VirSree 会用以下账号创建、读取、复制和删除对象。</small></div></div><div className="field-grid"><label>WebDAV Endpoint<input required value={endpoint} onChange={e=>setEndpoint(e.target.value)} onBlur={()=>setEndpoint(normalizeEndpoint(endpoint))} placeholder="dav.example.com/remote.php/dav/files/user"/><small>可以只填域名或路径；VirSree 会自动补全 HTTPS。</small></label><label>用户名<input required={!editing} value={username} onChange={e=>setUsername(e.target.value)} placeholder={secretHint}/><small>{secretHint}</small></label><label>密码 / App Password<input required={!editing} type="password" autoComplete="new-password" value={password} onChange={e=>setPassword(e.target.value)} placeholder={secretHint}/><small>{secretHint}</small></label></div><div className="protocol-note"><Waypoints/><div><strong>WebDAV 文件会经过 VirSree</strong><p>WebDAV 没有通用的预签名 URL 协议。平台会发放短期能力链接并中转文件字节；应用调用方式保持一致，响应里的 <code>direct</code> 为 <code>false</code>。</p></div></div></div>}{error&&<div className="verify-error"><ShieldCheck/><div><strong>验证没有通过</strong><p>{error}</p></div></div>}<button className="primary verify-button" disabled={busy}>{busy?<><Loader2 className="spin"/>正在连接并执行读写验证…</>:<><ShieldCheck/>{submitLabel}</>}</button><p className="verify-foot">只有探针对象写入、HEAD 读取、下载、复制和删除全部成功后，配置才会保存。</p></form>{riskOpen&&initial&&<BucketRiskDialog oldBucket={initial.bucket} newBucket={bucket.trim()} used={initial.used_bytes+initial.reserved_bytes} busy={busy} onCancel={()=>setRiskOpen(false)} onConfirm={confirmRisk}/>}</>;
}
