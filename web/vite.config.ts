import {defineConfig} from 'vite'
import react from '@vitejs/plugin-react'
import {readFileSync} from 'node:fs'
const packageVersion=JSON.parse(readFileSync(new URL('./package.json',import.meta.url),'utf8')).version
const version=process.env.VIRSREE_VERSION||packageVersion
export default defineConfig({plugins:[react()],define:{__VIRSREE_VERSION__:JSON.stringify(version)},server:{port:5173,proxy:{'/api':'http://127.0.0.1:8080','/health':'http://127.0.0.1:8080','/p':'http://127.0.0.1:8080','/s3':'http://127.0.0.1:8080'}}})
