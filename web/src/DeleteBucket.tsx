import {useEffect, useRef, useState} from 'react';
import {Check, Clock3, Loader2} from 'lucide-react';
import {API, BucketDetail} from './api';
import {RiskDialog} from './RiskDialog';

export function DeleteBucket({api,detail,close,done}:{api:API;detail:BucketDetail;close:()=>void;done:(v:BucketDetail)=>void}) {
  const [stage,setStage]=useState<'warning'|'name'>('warning');
  const [challenge,setChallenge]=useState('');
  const [remaining,setRemaining]=useState(5);
  const [name,setName]=useState('');
  const [error,setError]=useState('');
  const [busy,setBusy]=useState(false);
  const [attempt,setAttempt]=useState(0);
  const input=useRef<HTMLInputElement>(null);
  const deadline=useRef(Infinity);
  const slug=detail.bucket.slug;
  useEffect(()=>{
    let active=true;
    setChallenge('');setRemaining(5);setError('');deadline.current=Infinity;
    api.prepareBucketDeletion(slug).then(v=>{
      if(!active)return;
      setChallenge(v.confirmation_token);
      // Monotonic client clock avoids countdown jumps when system time changes.
      deadline.current=performance.now()+v.wait_seconds*1000;
    }).catch(e=>{if(active)setError(e.message)});
    const timer=setInterval(()=>{if(active&&Number.isFinite(deadline.current))setRemaining(Math.max(0,Math.ceil((deadline.current-performance.now())/1000)))},100);
    return()=>{active=false;clearInterval(timer)};
  },[api,slug,attempt]);
  useEffect(()=>{if(stage==='name')input.current?.focus()},[stage]);
  const next=()=>{if(challenge&&performance.now()>=deadline.current){setStage('name');setError('')}};
  const remove=async()=>{
    if(busy||name!==slug||!challenge||performance.now()<deadline.current)return;
    setBusy(true);setError('');
    try{await api.deleteBucket(slug,name,challenge);done({...detail,bucket:{...detail.bucket,deleting:true},status:'deleting',deletion:undefined,metrics:{...detail.metrics,active_key_count:0,active_public_link_count:0}})}
    catch(e:any){setError(e.message||'无法提交删除请求，请重试');setBusy(false)}
  };
  return <RiskDialog eyebrow={stage==='warning'?'DELETE BUCKET · 01 / 02':'DELETE BUCKET · 02 / 02'} title={stage==='warning'?'删除整个虚拟桶？':'输入完整桶名，确认永久删除'} lead={`${detail.bucket.name} · s3://${slug}`} busy={busy} cancelLabel="保留此桶" confirmLabel={stage==='warning'?(challenge?(remaining>0?`请阅读后果 · ${remaining}s`:'我已了解，继续'):'正在准备确认…'):'永久删除桶'} confirmDisabled={stage==='warning'?(!challenge||remaining>0):name!==slug} onCancel={close} onConfirm={stage==='warning'?next:remove}>
    <div className="delete-stage" key={stage}>
      {stage==='warning'?<>
        <div className="delete-summary"><strong>{detail.metrics.object_count}<small>个对象</small></strong><strong>{detail.metrics.active_key_count}<small>个活跃密钥</small></strong><strong>{detail.metrics.active_public_link_count}<small>个公开链接</small></strong></div>
        <div className="risk-impact"><strong>此操作不可撤销</strong><p>VirSree 将停止这个桶的访问，永久删除各存储源中的对象、暂存文件、映射、访问密钥和公开链接，并释放容量。接入此桶的应用将无法继续读写。</p><p>已有上传直链尚未到期时，桶会保持“清理中”，到期后再次清理。存储故障会自动重试，完成后桶才会从列表移除。</p><p>CDN 可能保留已缓存的文件，平台加密备份按原滚动周期保留；如需立刻移除缓存，请在 CDN 服务商处执行刷新。</p></div>
        <div className="delete-countdown"><Clock3 size={17}/><span>{!challenge?'正在建立删除确认…':remaining>0?'请花 5 秒确认删除范围和后果':'等待完成，可以进入下一步'}</span><span className="countdown-value" aria-hidden="true">{!challenge?<Loader2 className="spin" size={15}/>:remaining>0?remaining:<Check size={16}/>}</span></div>
      </>:<>
        <p className="delete-instruction">请输入以下 <strong>S3 桶名</strong>。必须完整匹配，包含每一个字符；不要输入显示名称。</p>
        <code className="delete-bucket-name">{slug}</code>
        <label className="delete-name-field" htmlFor="delete-bucket-name">完整桶名<input ref={input} id="delete-bucket-name" value={name} disabled={busy} autoComplete="off" autoCapitalize="none" spellCheck={false} placeholder="在此输入上方桶名" aria-describedby="delete-name-help" aria-invalid={name.length>0&&name!==slug} onChange={e=>setName(e.target.value)} onKeyDown={e=>{if(e.key==='Enter'&&name===slug){e.preventDefault();void remove()}}}/></label>
        <p className={`delete-match ${name===slug?'matched':''}`} id="delete-name-help">{name===slug?<><Check size={14}/>桶名完全匹配，确认后开始永久清理</>:name?'桶名尚未完全匹配，请检查字符和空格':'输入完整桶名后，删除按钮才会启用'}</p>
        <div className="risk-impact"><strong>确认后立即停止此桶的所有访问</strong><p>清理会在服务器继续执行。你可以离开页面，VirSree 会保留进度并自动重试。</p></div>
      </>}
      {error&&<div className="delete-error" role="alert"><p>{error}</p>{stage==='warning'&&<button className="ghost" onClick={()=>setAttempt(v=>v+1)}>重新准备确认</button>}{stage==='name'&&<button className="ghost" disabled={busy} onClick={()=>{setStage('warning');setAttempt(v=>v+1)}}>重新开始确认</button>}</div>}
    </div>
  </RiskDialog>;
}
