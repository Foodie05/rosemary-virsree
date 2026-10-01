import type {ReactNode} from 'react';
import {useEffect, useId, useRef, useState} from 'react';
import {AlertTriangle, Loader2, ShieldCheck} from 'lucide-react';
import {useVersionHold} from './VersionGuard';

export function RiskDialog({eyebrow='RISK CONFIRMATION',title,lead,children,busy=false,confirmDisabled=false,cancelLabel='取消',confirmLabel='确认',onCancel,onConfirm}:{eyebrow?:string;title:string;lead?:string;children:ReactNode;busy?:boolean;confirmDisabled?:boolean;cancelLabel?:string;confirmLabel?:string;onCancel:()=>void;onConfirm:()=>void}){
  useVersionHold();
  const titleId=useId();
  const [closing,setClosing]=useState(false);
  const closeTimer=useRef<ReturnType<typeof setTimeout>|undefined>(undefined);
  const cancelRef=useRef<HTMLButtonElement>(null);
  const sectionRef=useRef<HTMLElement>(null);
  const latest=useRef({onCancel,busy});latest.current={onCancel,busy};
  const cancel=()=>{if(latest.current.busy||closeTimer.current)return;setClosing(true);closeTimer.current=setTimeout(()=>latest.current.onCancel(),160)};
  useEffect(()=>{
    const previous=document.activeElement as HTMLElement|null;
    cancelRef.current?.focus();
    const onKey=(event:KeyboardEvent)=>{
      if(event.key==='Escape'&&!latest.current.busy){event.preventDefault();cancel()}
      if(event.key==='Tab'){
        const buttons=Array.from(sectionRef.current?.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), a[href], [tabindex="0"]')||[]);
        if(!buttons.length)return;
        if(event.shiftKey&&document.activeElement===buttons[0]){event.preventDefault();buttons[buttons.length-1].focus()}
        else if(!event.shiftKey&&document.activeElement===buttons[buttons.length-1]){event.preventDefault();buttons[0].focus()}
      }
    };
    window.addEventListener('keydown',onKey);
    return()=>{window.removeEventListener('keydown',onKey);if(closeTimer.current)clearTimeout(closeTimer.current);previous?.focus()};
  },[]);
  return <div className={`modal-bg risk-layer ${closing?'is-closing':''}`} role="presentation" onMouseDown={e=>{if(e.target===e.currentTarget)cancel()}}><section ref={sectionRef} className="risk-dialog" role="alertdialog" aria-modal="true" aria-labelledby={titleId}><div className="risk-dialog-head"><span className="risk-icon"><AlertTriangle/></span><div><p className="eyebrow">{eyebrow}</p><h3 id={titleId}>{title}</h3></div></div>{lead&&<p className="risk-lead">{lead}</p>}{children}<div className="risk-actions"><button ref={cancelRef} className="secondary" disabled={busy||closing} onClick={cancel}>{cancelLabel}</button><button className="danger-confirm" disabled={busy||confirmDisabled||closing} onClick={onConfirm}>{busy?<><Loader2 className="spin"/>正在处理…</>:<><ShieldCheck/>{confirmLabel}</>}</button></div></section></div>;
}
