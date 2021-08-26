import './browserEnv'

import child_process from 'child_process'
import http from 'http'
import path from 'path'

import { handleRequest } from './handle'

const webpackHost = 'sourcegraph.test'
const webpackPort = 3080

const port = process.env.PORT || 3079

http.createServer(async (request, response) => {
    const PRERENDER = true
    if (PRERENDER && (request.url! === '/' || request.url! === '/search')) {
        const SPAWN = false
        if (SPAWN) {
            const child = child_process.execFile('babel-node', [
                '-x',
                '.tsx,.ts',
                path.join(__dirname, 'handle.tsx'),
                request.url!,
            ])
            response.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' })
            child.stdout!.pipe(response)
            child.on('error', error => console.error(error))
            return
        }

        response.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' })
        try {
            await handleRequest(response, request.url!, require('./jscontext').JSCONTEXT, {})
        } catch (err) {
            console.error('ERROR', err)
        }
        return
    }

    const options: http.RequestOptions = {
        hostname: webpackHost,
        port: webpackPort,
        path: request.url!,
        method: request.method,
        headers: request.headers,
    }

    // Forward each incoming request
    const proxyRequest = http.request(options, async proxyRes => {
        response.writeHead(proxyRes.statusCode!, proxyRes.headers)
        proxyRes.pipe(response, { end: true })
    })

    // Forward the body of the request to esbuild
    request.pipe(proxyRequest, { end: true })
}).listen(port)

console.error(`Ready at http://localhost:${port}`)
