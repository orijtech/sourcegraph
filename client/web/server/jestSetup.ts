import sinon from 'sinon'

window.context = require('./jscontext').JSCONTEXT

window.matchMedia = sinon.spy(() => ({ matches: false }))
