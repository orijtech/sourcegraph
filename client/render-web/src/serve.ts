import './browserEnv'

import child_process from 'child_process'
import http from 'http'
import path from 'path'

import { handleRequest } from './handle'

const port = process.env.PORT || 3190

http.createServer(async (request, response) => {
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

    // TODO(sqs): handle when not HTTP 200 (support other HTTP status codes, including redirects)
    response.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' })
    try {
        await handleRequest(response, request.url!, require('./jscontext').JSCONTEXT, {})
    } catch (err) {
        console.error(`Error (${request.url}): ${err}`)
    }
}).listen(port)

console.error(`Ready at: http://localhost:${port}`)
