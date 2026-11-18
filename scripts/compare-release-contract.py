#!/usr/bin/env python3
"""Compare invitation contracts using two same-platform server binaries.

Usage: python3 scripts/compare-release-contract.py /path/to/old-server /path/to/new-server
Uses temporary synthetic databases and a loopback S3 stub; never production storage.
"""
import os,subprocess,tempfile,socket,time,urllib.request,urllib.error,json,secrets,signal,shutil,threading,sqlite3,sys
from http.server import BaseHTTPRequestHandler,ThreadingHTTPServer
from urllib.parse import urlsplit,unquote
objects={}
class S3(BaseHTTPRequestHandler):
 def log_message(self,*args):pass
 def pathkey(self):return unquote(urlsplit(self.path).path)
 def answer(self,status,body=b''):
  self.send_response(status);self.send_header('Content-Length',str(len(body)));self.send_header('ETag','"fixture"');self.end_headers()
  if self.command!='HEAD':self.wfile.write(body)
 def do_PUT(self):
  key=self.pathkey();source=self.headers.get('x-amz-copy-source')
  if source:
   objects[key]=objects["/"+unquote(source).lstrip("/")];self.answer(200,b'<CopyObjectResult><ETag>"fixture"</ETag><LastModified>2026-11-18T00:00:00Z</LastModified></CopyObjectResult>')
  else:objects[key]=self.rfile.read(int(self.headers.get('Content-Length',0)));self.answer(200)
 def do_HEAD(self):
  key=self.pathkey()
  if key not in objects:self.answer(404);return
  self.answer(200,objects[key])
 def do_GET(self):
  key=self.pathkey();self.answer(200,objects[key]) if key in objects else self.answer(404)
 def do_DELETE(self):objects.pop(self.pathkey(),None);self.answer(204)
mock=ThreadingHTTPServer(('127.0.0.1',0),S3);threading.Thread(target=mock.serve_forever,daemon=True).start()
def run(binary):
 root=tempfile.mkdtemp(prefix='arca-contract-');os.chmod(root,0o700)
 sock=socket.socket();sock.bind(('127.0.0.1',0));port=sock.getsockname()[1];sock.close();base=f'http://127.0.0.1:{port}';admin=secrets.token_urlsafe(32)
 env={'PATH':'/usr/bin:/bin','RVS_ADMIN_TOKEN':admin,'RVS_MASTER_KEY':secrets.token_urlsafe(48),'RVS_LISTEN':f'127.0.0.1:{port}','RVS_PUBLIC_URL':base,'RVS_DATABASE':root+'/fixture.db','RVS_WEB_DIR':root+'/missing','RVS_DOCS_DIR':root+'/missing','RVS_S3_ENDPOINT':f'http://127.0.0.1:{mock.server_port}','RVS_S3_BUCKET':'fixture','RVS_S3_ACCESS_KEY':'fixture-ak','RVS_S3_SECRET_KEY':'fixture-secret','RVS_S3_REGION':'us-east-1'}
 def call(path,method='GET',token=admin,body=None):
  req=urllib.request.Request(base+path,method=method,data=json.dumps(body).encode() if body is not None else None,headers={'Authorization':'Bearer '+token,'Content-Type':'application/json'})
  try:
   with urllib.request.urlopen(req,timeout=15) as r:return r.status,json.load(r) if r.status!=204 else None
  except urllib.error.HTTPError as e:return e.code,json.load(e)
 log=open(root+'/server.log','w');p=subprocess.Popen([binary],env=env,stdout=log,stderr=log,cwd=root)
 try:
  for _ in range(60):
   try:call('/health');break
   except Exception:time.sleep(.1)
  out={}
  for visibility in ['public','private']:
   bucket='compat-'+visibility
   status,_=call('/api/v1/buckets','POST',body={'name':'Fixture','slug':bucket,'visibility':visibility,'quota_bytes':1024});assert status==201
   path='/api/v1/admin/buckets/'+bucket+'/upload-invitations'
   status,result=call(path,'POST',body={'name':'Fixture','max_files':2,'expires_in':3600,'allow_overwrite':False});assert status==201
   token=result['token'];out[visibility+'_policy_fields']=sorted(result['invitation'])
   status,inspection=call('/api/v1/agent/uploads',token=token);assert status==200;out[visibility+'_inspection_fields']=sorted(inspection)
   status,signed=call('/api/v1/agent/uploads','POST',token,{'key':'report.txt','size':4,'content_type':'text/plain'});assert status==201,"upload signing status differs"
   with urllib.request.urlopen(urllib.request.Request(signed['url'],data=b'test',method='PUT',headers=signed['required_headers']),timeout=10) as r:assert r.status==200
   commit={'upload_id':signed['upload_id'],'key':'report.txt'}
   status,receipt=call('/api/v1/agent/uploads/commit','POST',token,commit);assert status==200,"commit status differs"
   status,repeated=call('/api/v1/agent/uploads/commit','POST',token,commit);assert status==200 and repeated==receipt
   out[visibility+'_public_alias']=bool(receipt['public_url']);out[visibility+'_direct']=signed['direct'];out[visibility+'_signing_fields']=sorted(signed)
   status,denied=call('/api/v1/agent/uploads','POST',token,{'key':'report.txt','size':4});out[visibility+'_overwrite']=[status,denied.get('code')]
   status,pending=call('/api/v1/agent/uploads','POST',token,{'key':'pending.txt','size':4});assert status==201
   status,denied=call('/api/v1/agent/uploads','POST',token,{'key':'third.txt','size':4});out[visibility+'_limit']=[status,denied.get('code')]
   status,_=call(path+'/'+result['invitation']['id'],'DELETE');assert status==204
   status,denied=call('/api/v1/agent/uploads/commit','POST',token,{'upload_id':pending['upload_id'],'key':'pending.txt'});out[visibility+'_revoked_commit']=[status,denied.get('code')]
   if receipt['public_url']:
    with urllib.request.urlopen(receipt['public_url'],timeout=10) as r:assert r.read()==b'test'
  db=sqlite3.connect(root+'/fixture.db');out['alias_sign_ttls']=[x[0] for x in db.execute('SELECT sign_ttl_seconds FROM public_links')];db.close()
  return out
 finally:
  p.send_signal(signal.SIGTERM)
  try:p.wait(timeout=15)
  except subprocess.TimeoutExpired:p.kill();p.wait()
  log.close();shutil.rmtree(root)
try:
 old=run(sys.argv[1]);new=run(sys.argv[2]);print(json.dumps({'old':old,'new':new,'equivalent':old==new},indent=2));assert old==new
finally:mock.shutdown()
