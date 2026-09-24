import { appendFileSync, readFileSync } from 'node:fs'
import { createServer } from 'node:https'

const [cert, key, host, port, log] = process.argv.slice(2)
const valid = `Bearer ${process.env.FIXTURE_API_KEY}`
const models = ['fixture-alpha', 'fixture-beta']

createServer({ cert: readFileSync(cert), key: readFileSync(key) }, (req, res) => {
  const auth = req.headers.authorization === valid ? 'valid' : req.headers.authorization ? 'invalid' : 'missing'
  appendFileSync(log, JSON.stringify({ method: req.method, path: req.url, auth }) + '\n')
  const send = (status, body) => { res.writeHead(status, { 'Content-Type': 'application/json' }); res.end(JSON.stringify(body)) }
  if (auth !== 'valid') return send(401, { error: { message: `invalid key ${req.headers.authorization ?? ''}` } })
  if (req.method === 'GET' && req.url === '/v1/models') return send(200, { object: 'list', data: models.map(id => ({ id, object: 'model' })) })
  const id = req.method === 'GET' && req.url.startsWith('/v1/models/') ? decodeURIComponent(req.url.slice(11)) : ''
  if (models.includes(id)) return send(200, { id, object: 'model' })
  send(404, { error: { message: 'not found' } })
}).listen(Number(port), host)
