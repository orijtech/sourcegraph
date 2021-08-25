import { PassThrough } from 'stream'

import { handleRequest } from './handle'

const handle = async (pathname: string): Promise<string> => {
    const stream = new PassThrough()

    const done = handleRequest(stream, pathname, {}, { noEntrypointHTML: true })

    const chunks = []
    for await (const chunk of stream) {
        chunks.push(Buffer.from(chunk))
    }
    const buffer = Buffer.concat(chunks)
    const string = buffer.toString('utf-8')

    await done

    return string
}

describe('handle', () => {
    test('render', async () => {
        expect(await handle('/search')).toBe('<!--$!-->Loading app...<!--/$-->')
    })
})
