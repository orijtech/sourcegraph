import child_process from 'child_process'
import http from 'http'
import path from 'path'

import { handleRequest } from './handle'

const webpackHost = 'sourcegraph.test'
const webpackPort = 3443

http.createServer(async (request, response) => {
    const PRERENDER = false
    if (PRERENDER && request.url!.startsWith('/flights')) {
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
        await handleRequest(response, request.url!, require('./jscontext').JSCONTEXT, {})
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
        // If esbuild returns "not found", send a custom 404 page
        if (proxyRes.statusCode === 404) {
            response.writeHead(404, { 'Content-Type': 'text/plain' })
            response.end()
            return
        }

        // Otherwise, forward the response to the client
        response.writeHead(proxyRes.statusCode!, proxyRes.headers)
        proxyRes.pipe(response, { end: true })
    })

    // Forward the body of the request to esbuild
    request.pipe(proxyRequest, { end: true })
}).listen(process.env.PORT || 5501)

console.error('Ready')
