import {useCallback, useEffect, useRef, useState} from 'react';
import type {CSSProperties, FormEvent} from 'react';
import {ArrowLeft, ChevronRight, Copy, Download, File, Folder, KeyRound, Loader2, RefreshCw, Search, Trash2} from 'lucide-react';
import {API, ObjectBrowsePage, ObjectInfo} from './api';
import {RiskDialog} from './RiskDialog';
import {useVersionHold} from './VersionGuard';

const PAGE_SIZE=40;
const size=(n:number)=>n>=1<<30?`${(n/(1<<30)).toFixed(1)} GB`:n>=1<<20?`${(n/(1<<20)).toFixed(1)} MB`:n>=1<<10?`${(n/(1<<10)).toFixed(1)} KB`:`${n} B`;

export function ObjectExplorer({api,bucket,notify}:{api:API;bucket:string;notify:(message:string)=>void}){
  const [prefix,setPrefix]=useState('');
  const [draft,setDraft]=useState('');
  const [cursors,setCursors]=useState<string[]>(['']);
  const [page,setPage]=useState<ObjectBrowsePage>();
  const [loading,setLoading]=useState(false);
  const [error,setError]=useState('');
  const [refresh,setRefresh]=useState(0);
  const [risk,setRisk]=useState<{kind:'invalidate'|'delete';object:ObjectInfo}|null>(null);
  const [download,setDownload]=useState<ObjectInfo|null>(null);
  const [ttl,setTTL]=useState('900');
  const [signedURL,setSignedURL]=useState('');
  const [actionError,setActionError]=useState('');
  const [busy,setBusy]=useState(false);
  const ttlInput=useRef<HTMLInputElement>(null);
  useVersionHold(Boolean(download));
  useEffect(()=>{if(download)ttlInput.current?.focus()},[download]);
  const cursor=cursors[cursors.length-1];
  const load=useCallback(()=>setRefresh(n=>n+1),[]);
  useEffect(()=>{
    let active=true;
    setLoading(true);setError('');
    api.browseObjects(bucket,prefix,cursor,PAGE_SIZE).then(result=>{if(active)setPage(result)}).catch(e=>{if(active)setError(e.message)}).finally(()=>{if(active)setLoading(false)});
    return()=>{active=false};
  },[api,bucket,prefix,cursor,refresh]);
  const navigate=(path:string)=>{setPrefix(path);setDraft(path);setCursors(['']);setPage(undefined)};
  const submit=(event:FormEvent)=>{event.preventDefault();navigate(draft.trim())};
  const closeRisk=()=>{if(!busy){setRisk(null);setActionError('')}};
  const confirmRisk=async()=>{
    if(!risk)return;
    setBusy(true);setActionError('');
    try{
      if(risk.kind==='invalidate'){
        await api.invalidateObject(bucket,risk.object.key);
        notify('对象物理键已轮换，旧直链已失效');
      }else{
        await api.deleteObject(bucket,risk.object.key);
        notify('对象已删除');
      }
      setRisk(null);load();
    }catch(e:any){setActionError(e.message||'操作失败，请重试')}
    finally{setBusy(false)}
  };
  const confirmDownload=async()=>{
    if(!download)return;
    const seconds=Number(ttl);
    if(!Number.isSafeInteger(seconds)||seconds<1){setActionError('请输入大于 0 的整数秒数');return}
    setBusy(true);setActionError('');
    try{
      const result=await api.adminDownload(bucket,download.key,seconds);
      setSignedURL(result.url);
    }catch(e:any){setActionError(e.message||'生成下载链接失败')}
    finally{setBusy(false)}
  };
  const copyKey=async(key:string)=>{
    try{await navigator.clipboard.writeText(key);notify('对象键已复制')}
    catch{notify('复制失败，请检查浏览器剪贴板权限')}
  };
  const segments=prefix.split('/').filter(Boolean);
  const crumbs=segments.map((segment,index)=>({label:segment,path:segments.slice(0,index+1).join('/')+'/'}));
  return <section className="object-explorer" aria-label="对象管理">
    <div className="explorer-heading"><div><p className="eyebrow">OBJECT EXPLORER</p><h3>对象管理</h3><p>按目录浏览，或输入完整对象键的前缀。每页最多 {PAGE_SIZE} 项。</p></div><button className="explorer-refresh" onClick={load} disabled={loading} aria-label="刷新对象列表"><RefreshCw size={17}/>刷新</button></div>
    <nav className="explorer-crumbs" aria-label="当前路径"><button onClick={()=>navigate('')}>桶根目录</button>{crumbs.map(c=><span key={c.path}><ChevronRight size={15}/><button onClick={()=>navigate(c.path)}>{c.label}</button></span>)}</nav>
    <form className="explorer-search" onSubmit={submit}><Search size={18}/><input aria-label="对象键前缀" value={draft} onChange={e=>setDraft(e.target.value)} placeholder="输入前缀，例如 images/2026/ 或 reports/q3"/><button type="submit">定位</button>{prefix&&<button type="button" className="clear" onClick={()=>navigate('')}>清除</button>}</form>
    {error?<div className="explorer-message error"><strong>对象列表加载失败</strong><span>{error}</span><button onClick={load}>重试</button></div>:loading?<div className="explorer-message"><Loader2 className="spin"/><span>正在读取当前目录…</span></div>:!page?.entries.length?<div className="explorer-message"><Folder size={25}/><strong>{prefix?'当前路径没有对象':'这个桶还没有对象'}</strong><span>{prefix?'检查前缀，或返回桶根目录。':'应用完成签名上传和 commit 后，对象会出现在这里。'}</span>{prefix&&<button onClick={()=>navigate('')}>返回根目录</button>}</div>:<div className="explorer-list" role="list">{page.entries.map((entry,index)=>{
      const object=entry.object;
      const display=entry.key.slice(prefix.length).replace(/\/$/,'');
      return <div className="explorer-row" role="listitem" key={entry.key} style={{'--row-delay':`${Math.min(index,8)*25}ms`} as CSSProperties}>
        <div className={`explorer-file-icon ${entry.folder?'folder':''}`}>{entry.folder?<Folder size={20}/>:<File size={20}/>}</div>
        <div className="explorer-file-main">{entry.folder?<button className="explorer-name" onClick={()=>navigate(entry.key)}>{display}<ChevronRight size={16}/></button>:<strong className="explorer-name" title={entry.key}>{display}</strong>}<small title={entry.key}>{entry.folder?'文件夹':entry.key===display?(object?.content_type||'文件'):entry.key}</small></div>
        {!entry.folder&&object&&<><div className="explorer-file-meta"><span>{size(object.size)}</span><small>{new Date(object.updated_at).toLocaleString()}</small></div><div className="explorer-actions"><button title="复制对象键" aria-label={`复制 ${display} 的对象键`} onClick={()=>copyKey(object.key)}><Copy size={16}/></button><button title="下载" aria-label={`下载 ${display}`} onClick={()=>{setDownload(object);setTTL('900');setSignedURL('');setActionError('')}}><Download size={16}/></button><button title="失效旧链接" aria-label={`失效 ${display} 的旧链接`} onClick={()=>{setRisk({kind:'invalidate',object});setActionError('')}}><KeyRound size={16}/></button><button className="danger" title="删除" aria-label={`删除 ${display}`} onClick={()=>{setRisk({kind:'delete',object});setActionError('')}}><Trash2 size={16}/></button></div></>}
      </div>
    })}</div>}
    {!loading&&!error&&page&&<div className="explorer-pagination"><span>第 {cursors.length} 页 · 当前 {page.entries.length} 项{page.next_cursor?' · 后面还有内容':''}</span><div><button disabled={cursors.length===1} onClick={()=>setCursors(v=>v.slice(0,-1))}><ArrowLeft size={15}/>上一页</button><button disabled={!page.next_cursor} onClick={()=>setCursors(v=>[...v,page.next_cursor])}>下一页<ChevronRight size={15}/></button></div></div>}
    {download&&<div className="modal-bg" role="presentation"><section className="modal explorer-download" role="dialog" aria-modal="true" aria-labelledby="explorer-download-title"><div className="modal-head"><h3 id="explorer-download-title">生成下载链接</h3></div><p className="explorer-dialog-key">{download.key}</p><label>链接有效期（秒）<input ref={ttlInput} type="number" min="1" step="1" value={ttl} onChange={e=>{setTTL(e.target.value);setSignedURL('')}}/></label><p className="explorer-dialog-hint">由你决定这次链接的有效期。S3 来源直达真实存储或 CDN；WebDAV 来源会使用中转能力链接。</p>{actionError&&<p className="form-error" role="alert">{actionError}</p>}<div className="actions"><button className="ghost" disabled={busy} onClick={()=>{setDownload(null);setSignedURL('');setActionError('')}}>取消</button>{signedURL?<a className="primary" href={signedURL} target="_blank" rel="noopener noreferrer" onClick={()=>setDownload(null)}><Download size={16}/>打开下载</a>:<button className="primary" disabled={busy} onClick={confirmDownload}>{busy?<Loader2 className="spin" size={16}/>:<Download size={16}/>}生成下载地址</button>}</div></section></div>}
    {risk&&<RiskDialog eyebrow={risk.kind==='delete'?'PERMANENT DELETE':'LINK INVALIDATION'} title={risk.kind==='delete'?'确认永久删除对象':'确认让旧直链失效'} lead={risk.object.key} busy={busy} cancelLabel="返回检查" confirmLabel={risk.kind==='delete'?'永久删除':'轮换物理键'} onCancel={closeRisk} onConfirm={confirmRisk}><div className="risk-impact"><strong>{risk.kind==='delete'?'此操作无法撤销':'已签发的旧地址将立即失效'}</strong><p>{risk.kind==='delete'?'真实存储源中的文件和映射都会删除。':'文件内容与逻辑对象键保持不变，旧物理键会被删除。'}</p>{actionError&&<p className="form-error" role="alert">{actionError}</p>}</div></RiskDialog>}
  </section>;
}
