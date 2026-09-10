export type Bucket={id:string;name:string;slug:string;visibility:string;quota_bytes:number;quota_unlimited:boolean;used_bytes:number;reserved_bytes:number;created_at:string}
export type Overview={bucket_count:number;object_count:number;active_key_count:number;used_bytes:number;reserved_bytes:number;allocated_quota:number;total_quota:number;backend_ready:boolean;primary_source_unlimited:boolean}
export type AccessKey={id:string;name:string;access_key:string;permissions:string[];revoked:boolean;created_at:string;bucket:string}
export type ObjectInfo={id:string;source_id:string;key:string;size:number;content_type:string;etag:string;status:string;generation:number;created_at:string;updated_at:string}
export type Session={authenticated:boolean;email:string;oidc_configured:boolean;setup_required:boolean}
export type StorageSource={id:string;name:string;kind:'s3'|'webdav';priority:number;capacity_bytes:number;capacity_unlimited:boolean;used_bytes:number;reserved_bytes:number;enabled:boolean;direct_transfer:boolean;cdn_enabled:boolean;created_at:string}
export type CDNMode='s3_sigv4'|'bitiful_token'
export type StorageSourceDetail=StorageSource&{endpoint:string;public_endpoint:string;region:string;bucket:string;cdn_endpoint:string;cdn_mode:CDNMode|'';path_style:boolean;access_key_configured:boolean;secret_key_configured:boolean;cdn_auth_key_configured:boolean;webdav_username_configured:boolean;webdav_password_configured:boolean}
export type StorageSourceInput={name:string;kind:'s3'|'webdav';priority:number;capacity_bytes:number;capacity_unlimited:boolean;endpoint:string;public_endpoint?:string;region?:string;bucket?:string;access_key?:string;secret_key?:string;path_style?:boolean;cdn_endpoint?:string;cdn_mode?:CDNMode;cdn_auth_key?:string;webdav_username?:string;webdav_password?:string;acknowledge_bucket_change?:boolean}
export type BucketDetail={bucket:Bucket;status:'ready'|'storage_unavailable';s3_endpoint:string;api_endpoint:string;region:string;can_set_unlimited:boolean;metrics:{object_count:number;active_key_count:number;active_public_link_count:number};allocations:{source_id:string;source_name:string;source_kind:string;object_count:number;used_bytes:number}[]}
export type PlatformFilesystem={initialized:boolean;source_id?:string;source_name?:string;source_kind?:string;prefix:string;encryption:string;backup_interval_seconds:number;retention:number;last_attempt_at?:string;last_success_at?:string;next_attempt_at?:string;last_error_code?:string}

export class API{
  constructor(public token=''){}
  async call<T>(path:string,init:RequestInit={}):Promise<T>{
    const language=navigator.language||'en'
    const headers:Record<string,string>={'Content-Type':'application/json','Accept-Language':language,...(init.headers as Record<string,string>||{})}
    if(this.token)headers.Authorization=`Bearer ${this.token}`
    const r=await fetch(path,{...init,credentials:'same-origin',headers})
    if(r.status===204)return undefined as T
    const data=await r.json().catch(()=>({}))
    if(r.status===401)window.dispatchEvent(new Event('rvs-unauthorized'))
    if(!r.ok){
      const zh=language.toLowerCase().startsWith('zh')
      const message=(zh?data.message_zh:data.message_en)||data.error||(zh?`请求失败 (${r.status})`:`Request failed (${r.status})`)
      const trace=data.trace_id?(zh?`（追踪编号：${data.trace_id}）`:` (Trace ID: ${data.trace_id})`):''
      throw new Error(message+trace)
    }
    return data
  }
  session(){return this.call<Session>('/api/v1/session')}
  logout(){return this.call<void>('/auth/logout',{method:'POST'})}
  overview(){return this.call<Overview>('/api/v1/overview')}
  buckets(){return this.call<Bucket[]>('/api/v1/buckets')}
  bucketDetail(slug:string){return this.call<BucketDetail>(`/api/v1/admin/buckets/${encodeURIComponent(slug)}`)}
  updateBucket(slug:string,v:object){return this.call<Bucket>(`/api/v1/admin/buckets/${encodeURIComponent(slug)}`,{method:'PUT',body:JSON.stringify(v)})}
  keys(){return this.call<AccessKey[]>('/api/v1/access-keys')}
  storageSources(){return this.call<StorageSource[]>('/api/v1/storage-sources')}
  platformFilesystem(){return this.call<PlatformFilesystem>('/api/v1/platform-filesystem')}
  storageSource(id:string){return this.call<StorageSourceDetail>(`/api/v1/storage-sources/${encodeURIComponent(id)}`)}
  addStorageSource(v:StorageSourceInput){return this.call<{source:StorageSource;verified:boolean}>('/api/v1/storage-sources',{method:'POST',body:JSON.stringify(v)})}
  updateStorageSource(id:string,v:StorageSourceInput){return this.call<{source:StorageSource;verified:boolean;bucket_changed:boolean}>(`/api/v1/storage-sources/${encodeURIComponent(id)}`,{method:'PUT',body:JSON.stringify(v)})}
  createBucket(v:object){return this.call<any>('/api/v1/buckets',{method:'POST',body:JSON.stringify(v)})}
  createKey(bucket:string,v:object){return this.call<any>(`/api/v1/buckets/${encodeURIComponent(bucket)}/access-keys`,{method:'POST',body:JSON.stringify(v)})}
  revokeKey(id:string){return this.call<void>(`/api/v1/access-keys/${encodeURIComponent(id)}`,{method:'DELETE'})}
  objects(bucket:string,prefix=''){return this.call<ObjectInfo[]>(`/api/v1/admin/buckets/${encodeURIComponent(bucket)}/objects?prefix=${encodeURIComponent(prefix)}`)}
  adminDownload(bucket:string,key:string,expires_in:number){return this.call<{url:string;direct:boolean}>(`/api/v1/admin/buckets/${encodeURIComponent(bucket)}/objects/download`,{method:'POST',body:JSON.stringify({key,expires_in})})}
  invalidateObject(bucket:string,key:string){return this.call<void>(`/api/v1/admin/buckets/${encodeURIComponent(bucket)}/objects/invalidate-links`,{method:'POST',body:JSON.stringify({key})})}
  deleteObject(bucket:string,key:string){return this.call<void>(`/api/v1/admin/buckets/${encodeURIComponent(bucket)}/objects/${key.split('/').map(encodeURIComponent).join('/')}`,{method:'DELETE'})}
  bootstrap(v:object){return this.call<any>('/api/v1/bootstrap-tokens',{method:'POST',body:JSON.stringify(v)})}
}
