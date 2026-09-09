import {useEffect, useState} from 'react';
import {Loader2, RefreshCw} from 'lucide-react';

declare const __VIRSREE_VERSION__: string;

export const APP_VERSION=__VIRSREE_VERSION__;
const reloadStateKey='virsree-version-reload';

function reloadFor(version:string){
  const now=Date.now();
  let attempts=1;
  try{
    const prior=JSON.parse(sessionStorage.getItem(reloadStateKey)||'null');
    if(prior?.version===version&&now-prior.at<5*60_000)attempts=Number(prior.attempts||0)+1;
    sessionStorage.setItem(reloadStateKey,JSON.stringify({version,attempts,at:now}));
  }catch{}
  if(attempts>3)return false;
  const url=new URL(location.href);
  url.searchParams.set('__virsree_version',version);
  url.searchParams.set('__virsree_refresh',String(now));
  setTimeout(()=>location.replace(url.toString()),260);
  return true;
}

export function VersionGuard(){
  const[updating,setUpdating]=useState(false);
  const[manualVersion,setManualVersion]=useState('');
  useEffect(()=>{
    let active=true;
    const check=async()=>{
      if(!active||updating)return;
      try{
        const response=await fetch(`/api/v1/version?client=${encodeURIComponent(APP_VERSION)}&t=${Date.now()}`,{cache:'no-store',headers:{'Cache-Control':'no-cache'}});
        if(!response.ok)return;
        const data=await response.json();
        if(!active)return;
        const serverVersion=String(data.version||'').replace(/^v/,'');
        const clientVersion=String(APP_VERSION||'').replace(/^v/,'');
        if(!serverVersion||serverVersion==='dev'||clientVersion==='dev')return;
        if(serverVersion===clientVersion){try{sessionStorage.removeItem(reloadStateKey)}catch{};return}
        setUpdating(true);
        if(!reloadFor(serverVersion)){setUpdating(false);setManualVersion(serverVersion)}
      }catch{}
    };
    const visible=()=>{if(document.visibilityState==='visible')void check()};
    void check();
    const timer=setInterval(check,60_000);
    window.addEventListener('focus',check);
    window.addEventListener('online',check);
    document.addEventListener('visibilitychange',visible);
    return()=>{active=false;clearInterval(timer);window.removeEventListener('focus',check);window.removeEventListener('online',check);document.removeEventListener('visibilitychange',visible)};
  },[updating]);
  if(!updating&&!manualVersion)return null;
  return <div className="version-update" role="status" aria-live="polite"><div className="version-update-mark">{updating?<Loader2 className="spin"/>:<RefreshCw/>}</div><div><strong>{updating?'VirSree 已更新':'需要刷新管理台'}</strong><p>{updating?`正在切换到 v${manualVersion||'最新版本'}…`:`服务器已升级到 v${manualVersion}，请重新载入最新界面。`}</p></div>{manualVersion&&<button onClick={()=>{try{sessionStorage.removeItem(reloadStateKey)}catch{};reloadFor(manualVersion)}}>立即刷新</button>}</div>;
}
