/// <reference types="react/experimental" />
/// <reference types="react-dom/experimental" />

import { Writable } from 'stream'
import React from 'react'
import ReactDOMServer from 'react-dom/server'
import { StaticRouter } from 'react-router'

import { EnterpriseWebApp } from '@sourcegraph/web/src/enterprise/EnterpriseWebApp'

import { entrypointHTML } from './common'

export const handleRequest = async (
    response: Writable,
    pathname: string,
    jscontext: object,
    { noEntrypointHTML }: { noEntrypointHTML?: boolean }
): Promise<void> => {
    const e = (
        <React.StrictMode>
            <StaticRouter location={pathname}>
                <EnterpriseWebApp />
            </StaticRouter>
        </React.StrictMode>
    )
    // ReactDOMServer.renderToString(e)
    // await new Promise(resolve => setTimeout(resolve))
    const appElementHTML = ReactDOMServer.renderToString(e)

    if (!noEntrypointHTML) {
        // res.write('HTTP/1.1 200 OK\n')
        // res.write('Content-Type: text/html; charset=utf-8\n')
        // res.write('\n')
    }

    response.write(
        noEntrypointHTML
            ? appElementHTML
            : await entrypointHTML(appElementHTML, `<script>window.context = ${safeJSONStringify(jscontext)}</script>`)
    )
    response.end()
}

if (require.main === module) {
    handleRequest(process.stdout, process.argv[2], {}, {})
        .catch(error => console.error('Error:', error))
        .finally(() => process.exit(0))
}

// A utility function to safely escape JSON for embedding in a <script> tag; https://medium.com/node-security/the-most-common-xss-vulnerability-in-react-js-applications-2bdffbcc1fa0.
function safeJSONStringify(object: any): string {
    return JSON.stringify(object)
        .replace(/<\/(script)/gi, '<\\/$1')
        .replace(/<!--/g, '<\\!--')
        .replace(/\u2028/g, '\\u2028') // Only necessary if interpreting as JS, which we do
        .replace(/\u2029/g, '\\u2029') // Ditto
}
