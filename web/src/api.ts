export type Bucket={id:string;name:string;slug:string;visibility:string;quota_bytes:number;used_bytes:number;reserved_bytes:number;created_at:string}
export type Overview={bucket_count:number;object_count:number;active_key_count:number;used_bytes:number;reserved_bytes:number;allocated_quota:number;total_quota:number;backend_ready:boolean}
export type AccessKey={id:string;name:string;access_key:string;permissions:string[];revoked:boolean;created_at:string;bucket:string}
export type ObjectInfo={id:string;source_id:string;key:string;size:number;content_type:string;etag:string;status:string;generation:number;created_at:string;updated_at:string}
export type Session={authenticated:boolean;email:string;oidc_configured:boolean;setup_required:boolean}
export type StorageSource={id:string;name:string;kind:'s3'|'webdav';priority:number;capacity_bytes:number;used_bytes:number;reserved_bytes:number;enabled:boolean;direct_transfer:boolean;cdn_enabled:boolean;created_at:string}
export type StorageSourceInput={name:string;kind:'s3'|'webdav';priority:number;capacity_bytes:number;endpoint:string;public_endpoint?:string;region?:string;bucket?:string;access_key?:string;secret_key?:string;path_style?:boolean;cdn_endpoint?:string;webdav_username?:string;webdav_password?:string}

export class API{
  constructor(public token=''){}
  async call<T>(path:string,init:RequestInit={}):Promise<T>{
    const headers:Record<string,string>={'Content-Type':'application/json',...(init.headers as Record<string,string>||{})}
    if(this.token)headers.Authorization=`Bearer ${this.token}`
    const r=await fetch(path,{...init,credentials:'same-origin',headers})
    if(r.status===204)return undefined as T
    const data=await r.json().catch(()=>({}))
    if(r.status===401)window.dispatchEvent(new Event('rvs-unauthorized'))
    if(!r.ok)throw new Error(data.error||`请求失败 (${r.status})`)
    return data
  }
  session(){return this.call<Session>('/api/v1/session')}
  logout(){return this.call<void>('/auth/logout',{method:'POST'})}
  overview(){return this.call<Overview>('/api/v1/overview')}
  buckets(){return this.call<Bucket[]>('/api/v1/buckets')}
  keys(){return this.call<AccessKey[]>('/api/v1/access-keys')}
  storageSources(){return this.call<StorageSource[]>('/api/v1/storage-sources')}
  addStorageSource(v:StorageSourceInput){return this.call<{source:StorageSource;verified:boolean}>('/api/v1/storage-sources',{method:'POST',body:JSON.stringify(v)})}
  createBucket(v:object){return this.call<any>('/api/v1/buckets',{method:'POST',body:JSON.stringify(v)})}
  createKey(bucket:string,v:object){return this.call<any>(`/api/v1/buckets/${encodeURIComponent(bucket)}/access-keys`,{method:'POST',body:JSON.stringify(v)})}
  revokeKey(id:string){return this.call<void>(`/api/v1/access-keys/${encodeURIComponent(id)}`,{method:'DELETE'})}
  objects(bucket:string,prefix=''){return this.call<ObjectInfo[]>(`/api/v1/admin/buckets/${encodeURIComponent(bucket)}/objects?prefix=${encodeURIComponent(prefix)}`)}
  adminDownload(bucket:string,key:string,expires_in:number){return this.call<{url:string;direct:boolean}>(`/api/v1/admin/buckets/${encodeURIComponent(bucket)}/objects/download`,{method:'POST',body:JSON.stringify({key,expires_in})})}
  invalidateObject(bucket:string,key:string){return this.call<void>(`/api/v1/admin/buckets/${encodeURIComponent(bucket)}/objects/invalidate-links`,{method:'POST',body:JSON.stringify({key})})}
  deleteObject(bucket:string,key:string){return this.call<void>(`/api/v1/admin/buckets/${encodeURIComponent(bucket)}/objects/${key.split('/').map(encodeURIComponent).join('/')}`,{method:'DELETE'})}
  bootstrap(v:object){return this.call<any>('/api/v1/bootstrap-tokens',{method:'POST',body:JSON.stringify(v)})}
}
