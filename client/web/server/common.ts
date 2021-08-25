export const entrypointHTML = async (appElementHTML?: string, appendToBody?: string): Promise<string> => {
    let html = '<html><head><title>STATIC</title></head><body><div id="root"></div></body></html>'
    if (appElementHTML) {
        html = html.replace('<div id="root"></div>', `<div id="root">${appElementHTML}</div>`)
    }
    if (appendToBody) {
        html = html.replace('</body>', `${appendToBody}</body>`)
    }
    return html
}
