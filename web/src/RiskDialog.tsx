import type {ReactNode} from 'react';
import {AlertTriangle, Loader2, ShieldCheck} from 'lucide-react';

export function RiskDialog({eyebrow='RISK CONFIRMATION',title,lead,children,busy=false,cancelLabel='取消',confirmLabel='确认',onCancel,onConfirm}:{eyebrow?:string;title:string;lead?:string;children:ReactNode;busy?:boolean;cancelLabel?:string;confirmLabel?:string;onCancel:()=>void;onConfirm:()=>void}){
  return <div className="modal-bg risk-layer" role="presentation" onMouseDown={e=>{if(e.target===e.currentTarget&&!busy)onCancel()}}><section className="risk-dialog" role="alertdialog" aria-modal="true" aria-labelledby="risk-dialog-title"><div className="risk-dialog-head"><span className="risk-icon"><AlertTriangle/></span><div><p className="eyebrow">{eyebrow}</p><h3 id="risk-dialog-title">{title}</h3></div></div>{lead&&<p className="risk-lead">{lead}</p>}{children}<div className="risk-actions"><button className="secondary" disabled={busy} onClick={onCancel}>{cancelLabel}</button><button className="danger-confirm" disabled={busy} onClick={onConfirm}>{busy?<><Loader2 className="spin"/>正在处理…</>:<><ShieldCheck/>{confirmLabel}</>}</button></div></section></div>;
}
