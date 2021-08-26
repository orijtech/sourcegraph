/// <reference types="react/experimental" />
/// <reference types="react-dom/experimental" />

import React from 'react'
import ReactDOMServer from 'react-dom/server'
import { StaticRouter } from 'react-router'

// TODO(sqs): separate into enterprise/oss
import { EnterpriseWebApp } from '@sourcegraph/web/src/enterprise/EnterpriseWebApp'

export interface RenderRequest {
    requestURI: string
    jscontext: object
}

export interface RenderResponse {
    html?: string
    redirectURL?: string
    error?: string
}

export const render = async ({ requestURI, jscontext }: RenderRequest): Promise<RenderResponse> => {
    const routerContext: { url?: string } = {}
    const e = (
        <React.StrictMode>
            <StaticRouter location={requestURI} context={routerContext}>
                <EnterpriseWebApp />
            </StaticRouter>
        </React.StrictMode>
    )
    // TODO(sqs): figure out how many times to iterate async
    ReactDOMServer.renderToString(e)
    await new Promise(resolve => setTimeout(resolve))
    const html = ReactDOMServer.renderToString(e)

    return {
        html,
        redirectURL: routerContext.url,
    }
}
