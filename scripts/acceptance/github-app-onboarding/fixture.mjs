import { createServer } from 'node:https'
import { appendFileSync, existsSync, readFileSync } from 'node:fs'
import { createHash, generateKeyPairSync, randomBytes, verify } from 'node:crypto'

const [certFile, keyFile, host, port, logFile, controlDir] = process.argv.slice(2)
const pat = process.env.FIXTURE_PAT
const badPat = process.env.FIXTURE_BAD_PAT
const ssoPat = process.env.FIXTURE_SSO_PAT
const owner = { id: 5001, login: 'fixture-owner', type: 'User' }
const orgs = {
  'fixture-org': { account: { id: 7001, login: 'fixture-org', type: 'Organization' }, role: 'admin' },
  'member-org': { account: { id: 7002, login: 'member-org', type: 'Organization' }, role: 'member' },
}
const repo = (id, account, name, extra = {}) => ({
  id, node_id: `R_${id}`, name, full_name: `${account.login}/${name}`, private: true, owner: account,
  html_url: `https://github.com/${account.login}/${name}`, clone_url: `https://github.com/${account.login}/${name}.git`,
  default_branch: 'main', archived: false, permissions: { admin: false, maintain: false, push: true, pull: true }, ...extra,
})
const personalRepos = [repo(8101, owner, 'token-alpha'), repo(8102, owner, 'token-beta'), repo(8103, owner, 'token-archive', { archived: true })]
const appRepos = account => [repo(8201 + account.id % 100, account, 'app-alpha'), repo(8301 + account.id % 100, account, 'app-beta'), repo(8401 + account.id % 100, account, 'app-gamma')]

const manifests = new Map()
const apps = new Map()
const installations = new Map()
const oauthCodes = new Map()
const userTokens = new Map()
const installTokens = new Map()
const emptyTree = '4b825dc642cb6eb9a060e54bf8d69288fbee4904'
const refSHA = (owner, name, ref) => createHash('sha1').update(`ref:${owner}/${name}/${ref}`).digest('hex')
let sequence = 0
const id = () => ++sequence
const remember = value => { appendFileSync(`${controlDir}/secrets.txt`, value.split('\n').filter(line => line.length > 40 && !line.startsWith('-----')).concat(value.includes('\n') ? [] : [value]).join('\n') + '\n'); return value }
const secret = prefix => remember(`${prefix}${randomBytes(18).toString('hex')}`)

const log = entry => appendFileSync(logFile, JSON.stringify(entry) + '\n')
const escape = value => String(value).replace(/[&<>"']/g, c => `&#${c.charCodeAt(0)};`)
const page = (res, title, body) => {
  res.writeHead(200, { 'content-type': 'text/html; charset=utf-8', 'cache-control': 'no-store' })
  res.end(`<!doctype html><html lang="en"><head><meta charset="utf-8"><title>${escape(title)}</title></head><body><main><h1>${escape(title)}</h1>${body}</main></body></html>`)
}
const form = (action, fields, label) => `<form method="get" action="${escape(action)}">${Object.entries(fields).map(([k, v]) => `<input type="hidden" name="${escape(k)}" value="${escape(v)}">`).join('')}<button type="submit">${escape(label)}</button></form>`
const json = (res, status, body, headers = {}) => { res.writeHead(status, { 'content-type': 'application/json; charset=utf-8', ...headers }); res.end(JSON.stringify(body)) }
const notFound = res => json(res, 404, { message: 'Not Found', documentation_url: 'https://docs.github.com/rest' })
const unauthorized = res => json(res, 401, { message: 'Bad credentials', documentation_url: 'https://docs.github.com/rest' })

function jwtApp(header) {
  const token = header.replace(/^Bearer /, '')
  const [h, p, s] = token.split('.')
  if (!h || !p || !s) return null
  let claims
  try { claims = JSON.parse(Buffer.from(p, 'base64url').toString()) } catch { return null }
  const app = [...apps.values()].find(a => String(a.id) === String(claims.iss) || a.client_id === claims.iss)
  if (!app || JSON.parse(Buffer.from(h, 'base64url').toString()).alg !== 'RS256') return null
  if (!verify('RSA-SHA256', Buffer.from(`${h}.${p}`), app.publicKey, Buffer.from(s, 'base64url'))) return null
  const now = Math.floor(Date.now() / 1000)
  if (!(claims.iat <= now + 60 && claims.exp > now && claims.exp - claims.iat <= 660)) return null
  return app
}

function credential(req) {
  const header = req.headers.authorization ?? ''
  const token = header.replace(/^(Bearer|token) /i, '')
  if (!header) return { kind: 'none' }
  if (token === pat) return { kind: 'pat' }
  if (token === ssoPat) return { kind: existsSync(`${controlDir}/sso-authorized`) ? 'pat' : 'sso' }
  if (token === badPat) return { kind: 'bad' }
  if (userTokens.has(token)) return { kind: 'user', ...userTokens.get(token) }
  if (installTokens.has(token)) return { kind: 'installation', ...installTokens.get(token) }
  const app = token.split('.').length === 3 ? jwtApp(header) : null
  return app ? { kind: 'jwt', app } : { kind: 'invalid' }
}

const readBody = req => new Promise((resolve, reject) => {
  const chunks = []
  let size = 0
  req.on('data', chunk => { size += chunk.length; if (size > 1 << 20) req.destroy(); chunks.push(chunk) })
  req.on('end', () => resolve(Buffer.concat(chunks).toString()))
  req.on('error', reject)
})

function paginate(res, url, items, key) {
  const perPage = Math.min(Math.max(Number(url.searchParams.get('per_page') ?? 30), 1), 100)
  const pageNo = Math.max(Number(url.searchParams.get('page') ?? 1), 1)
  const slice = items.slice((pageNo - 1) * perPage, pageNo * perPage)
  const headers = {}
  if (pageNo * perPage < items.length) {
    const next = new URL(url)
    next.searchParams.set('page', String(pageNo + 1))
    headers.link = `<https://api.github.com${next.pathname}${next.search}>; rel="next"`
  }
  json(res, 200, key ? { total_count: items.length, [key]: slice } : slice, headers)
}

async function web(req, res, url) {
  const path = url.pathname
  if (path === '/favicon.ico') { res.writeHead(204); return res.end() }
  const creation = path.match(/^(?:\/organizations\/([A-Za-z0-9-]+))?\/settings\/apps\/new$/)
  if (creation && req.method === 'POST') {
    const params = new URLSearchParams(await readBody(req))
    let manifest
    try { manifest = JSON.parse(params.get('manifest') ?? '') } catch { return json(res, 422, { message: 'Invalid manifest' }) }
    const state = url.searchParams.get('state') ?? ''
    const target = creation[1] ? orgs[creation[1]]?.account : owner
    if (!target || !manifest.name || !manifest.url || !/^https?:\/\//.test(manifest.redirect_url ?? '') || !manifest.default_permissions) return json(res, 422, { message: 'Invalid manifest' })
    const code = secret('mc')
    manifests.set(code, { manifest, account: target, used: false })
    log({ event: 'manifest', owner: target.login, permissions: manifest.default_permissions, events: manifest.default_events, hook_active: manifest.hook_attributes?.active, request_oauth_on_install: manifest.request_oauth_on_install, setup_on_update: manifest.setup_on_update, public: manifest.public })
    return page(res, `Create GitHub App for ${target.login}`, `<p>${escape(manifest.name)}</p>${form(manifest.redirect_url, { code, state }, 'Create GitHub App')}`)
  }
  const install = path.match(/^\/apps\/([a-z0-9-]+)\/installations\/new$/)
  if (install && req.method === 'GET') {
    const app = [...apps.values()].find(a => a.slug === install[1])
    if (!app) return notFound(res)
    const state = url.searchParams.get('state') ?? ''
    let inst = [...installations.values()].find(i => i.app_id === app.id && i.account.id === app.owner.id)
    if (!inst) {
      inst = { id: 6100 + id(), app_id: app.id, app_slug: app.slug, account: app.owner, repository_selection: 'selected', suspended_at: null, target_type: app.owner.type }
      installations.set(inst.id, inst)
    }
    return page(res, `Install ${app.name}`, `<p>${escape(app.owner.login)}</p>${form(app.setup_url, { installation_id: inst.id, setup_action: 'install', state }, 'Install')}`)
  }
  if (path === '/login/oauth/authorize' && req.method === 'GET') {
    const q = url.searchParams
    const app = [...apps.values()].find(a => a.client_id === q.get('client_id'))
    if (!app || q.get('code_challenge_method') !== 'S256' || !/^[A-Za-z0-9_-]{43}$/.test(q.get('code_challenge') ?? '') || !app.callback_urls.includes(q.get('redirect_uri'))) {
      log({ event: 'authorize_rejected' })
      return page(res, 'Authorization failed', '<p>Invalid request.</p>')
    }
    const code = secret('oc')
    oauthCodes.set(code, { app, challenge: q.get('code_challenge'), redirect: q.get('redirect_uri'), used: false })
    return page(res, `Authorize ${app.name}`, form(q.get('redirect_uri'), { code, state: q.get('state') ?? '' }, 'Authorize'))
  }
  if (path === '/login/oauth/access_token' && req.method === 'POST') {
    const params = new URLSearchParams(await readBody(req))
    const grant = oauthCodes.get(params.get('code') ?? '')
    const verifier = params.get('code_verifier') ?? ''
    const ok = grant && !grant.used && grant.app.client_id === params.get('client_id') && grant.app.client_secret === params.get('client_secret') && grant.redirect === params.get('redirect_uri') && createHash('sha256').update(verifier).digest('base64url') === grant.challenge
    log({ event: 'token_exchange', ok: !!ok, pkce: !!grant && createHash('sha256').update(verifier).digest('base64url') === grant.challenge })
    if (!ok) return json(res, 200, { error: 'bad_verification_code', error_description: 'The code passed is incorrect or expired.' })
    grant.used = true
    const token = secret('ghu_')
    userTokens.set(token, { app: grant.app, user: owner })
    return json(res, 200, { access_token: token, token_type: 'bearer', scope: '' })
  }
  return notFound(res)
}

async function api(req, res, url, auth) {
  const path = url.pathname
  const conversion = path.match(/^\/app-manifests\/([A-Za-z0-9]+)\/conversions$/)
  if (conversion && req.method === 'POST') {
    const entry = manifests.get(conversion[1])
    if (!entry || entry.used) return notFound(res)
    entry.used = true
    const { publicKey, privateKey } = generateKeyPairSync('rsa', { modulusLength: 2048 })
    const n = id()
    const m = entry.manifest
    const app = {
      id: 9100 + n, slug: `${m.name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')}-${n}`, name: m.name, owner: entry.account,
      client_id: `Iv23li${randomBytes(7).toString('hex')}`, client_secret: remember(randomBytes(20).toString('hex')), webhook_secret: m.hook_attributes?.active === false ? null : remember(randomBytes(20).toString('hex')),
      publicKey, callback_urls: m.callback_urls ?? [], setup_url: m.setup_url, permissions: m.default_permissions, events: m.default_events ?? [],
    }
    apps.set(app.id, app)
    return json(res, 201, {
      id: app.id, slug: app.slug, node_id: `A_${app.id}`, owner: { ...app.owner }, name: app.name, description: '', external_url: m.url, html_url: `https://github.com/apps/${app.slug}`,
      created_at: new Date().toISOString(), updated_at: new Date().toISOString(), permissions: app.permissions, events: app.events,
      client_id: app.client_id, client_secret: app.client_secret, webhook_secret: app.webhook_secret, pem: remember(privateKey.export({ type: 'pkcs1', format: 'pem' })),
    })
  }
  if (path === '/meta' && req.method === 'GET') {
    if (!['pat', 'installation'].includes(auth.kind)) return unauthorized(res)
    return json(res, 200, { verifiable_password_authentication: false, hooks: ['192.30.252.0/22'] })
  }
  if (path === '/user' && req.method === 'GET') {
    if (auth.kind === 'sso') return json(res, 403, { message: 'Resource protected by organization SAML enforcement. You must grant your Personal Access token access to this organization.', documentation_url: 'https://docs.github.com/articles/authenticating-to-a-github-organization-with-saml-single-sign-on/' })
    if (auth.kind !== 'pat' && auth.kind !== 'user') return unauthorized(res)
    return json(res, 200, { ...owner, node_id: 'U_5001', html_url: 'https://github.com/fixture-owner' })
  }
  if (path === '/user/repos' && req.method === 'GET') {
    if (auth.kind !== 'pat') return unauthorized(res)
    return paginate(res, url, personalRepos)
  }
  if (path === '/user/installations' && req.method === 'GET') {
    if (auth.kind !== 'user') return unauthorized(res)
    const visible = [...installations.values()].filter(i => i.app_id === auth.app.id && (i.account.id === owner.id || Object.values(orgs).some(o => o.account.id === i.account.id)))
    return paginate(res, url, visible, 'installations')
  }
  const membership = path.match(/^\/user\/memberships\/orgs\/([A-Za-z0-9-]+)$/)
  if (membership && req.method === 'GET') {
    if (auth.kind !== 'user') return unauthorized(res)
    const org = orgs[membership[1]]
    if (!org) return notFound(res)
    return json(res, 200, { state: 'active', role: org.role, organization: org.account, user: owner })
  }
  if (path === '/app' && req.method === 'GET') {
    if (auth.kind !== 'jwt') return unauthorized(res)
    return json(res, 200, { id: auth.app.id, slug: auth.app.slug, name: auth.app.name, owner: auth.app.owner })
  }
  const inst = path.match(/^\/app\/installations\/(\d+)(\/access_tokens)?$/)
  if (inst) {
    if (auth.kind !== 'jwt') return unauthorized(res)
    const found = installations.get(Number(inst[1]))
    if (!found || found.app_id !== auth.app.id) return notFound(res)
    if (!inst[2] && req.method === 'GET') return json(res, 200, found)
    if (inst[2] && req.method === 'POST') {
      const token = secret('ghs_')
      installTokens.set(token, { installation: found })
      return json(res, 201, { token, expires_at: new Date(Date.now() + 3600_000).toISOString(), permissions: auth.app.permissions, repository_selection: 'selected' })
    }
  }
  const bot = decodeURIComponent(path).match(/^\/users\/([a-z0-9-]+)\[bot\]$/)
  if (bot && req.method === 'GET') {
    const app = [...apps.values()].find(a => a.slug === bot[1])
    if (!app || auth.kind !== 'installation') return auth.kind === 'installation' ? notFound(res) : unauthorized(res)
    return json(res, 200, { id: 4100 + app.id, login: `${app.slug}[bot]`, type: 'Bot' })
  }
  const single = path.match(/^\/repos\/([A-Za-z0-9-]+)\/([A-Za-z0-9._-]+)$/)
  if (single && req.method === 'GET') {
    const visible = auth.kind === 'pat' ? personalRepos : auth.kind === 'installation' ? appRepos(auth.installation.account) : null
    if (!visible) return unauthorized(res)
    const found = visible.find(r => r.full_name === `${single[1]}/${single[2]}`)
    return found ? json(res, 200, found) : notFound(res)
  }
  const ref = path.match(/^\/repos\/([A-Za-z0-9-]+)\/([A-Za-z0-9._-]+)\/git\/ref\/(.+)$/)
  if (ref && req.method === 'GET') {
    const visible = auth.kind === 'pat' ? personalRepos : auth.kind === 'installation' ? appRepos(auth.installation.account) : null
    if (!visible) return unauthorized(res)
    const found = visible.find(r => r.full_name === `${ref[1]}/${ref[2]}`)
    if (!found) return notFound(res)
    const sha = refSHA(ref[1], ref[2], ref[3])
    return json(res, 200, { ref: `refs/${ref[3]}`, node_id: `R_${sha}`, url: `https://api.github.com/repos/${ref[1]}/${ref[2]}/git/refs/${ref[3]}`, object: { sha, type: 'commit', url: `https://api.github.com/repos/${ref[1]}/${ref[2]}/git/commits/${sha}` } })
  }
  const commit = path.match(/^\/repos\/([A-Za-z0-9-]+)\/([A-Za-z0-9._-]+)\/git\/commits\/([0-9a-f]{40})$/)
  if (commit && req.method === 'GET') {
    if (!['pat', 'installation'].includes(auth.kind)) return unauthorized(res)
    return json(res, 200, { sha: commit[3], node_id: `C_${commit[3]}`, url: `https://api.github.com/repos/${commit[1]}/${commit[2]}/git/commits/${commit[3]}`, tree: { sha: emptyTree, url: `https://api.github.com/repos/${commit[1]}/${commit[2]}/git/trees/${emptyTree}` } })
  }
  const tree = path.match(/^\/repos\/([A-Za-z0-9-]+)\/([A-Za-z0-9._-]+)\/git\/trees\/([0-9a-f]{40})$/)
  if (tree && req.method === 'GET') {
    if (!['pat', 'installation'].includes(auth.kind)) return unauthorized(res)
    return json(res, 200, { sha: tree[3], url: `https://api.github.com/repos/${tree[1]}/${tree[2]}/git/trees/${tree[3]}`, tree: [], truncated: false })
  }
  const content = path.match(/^\/repos\/([A-Za-z0-9-]+)\/([A-Za-z0-9._-]+)\/contents\/(.+)$/)
  if (content && req.method === 'GET') {
    if (!['pat', 'installation'].includes(auth.kind)) return unauthorized(res)
    return notFound(res)
  }
  const checks = path.match(/^\/repos\/([A-Za-z0-9-]+)\/([A-Za-z0-9._-]+)\/commits\/([0-9a-f]{40})\/check-runs$/)
  if (checks && req.method === 'GET') {
    if (!['pat', 'installation'].includes(auth.kind)) return unauthorized(res)
    return json(res, 200, { total_count: 0, check_runs: [] })
  }
  const pulls = path.match(/^\/repos\/([A-Za-z0-9-]+)\/([A-Za-z0-9._-]+)\/pulls$/)
  if (pulls && req.method === 'GET') {
    if (!['pat', 'installation'].includes(auth.kind)) return unauthorized(res)
    return paginate(res, url, [])
  }
  if (path === '/installation/repositories' && req.method === 'GET') {
    if (auth.kind !== 'installation') return unauthorized(res)
    return paginate(res, url, appRepos(auth.installation.account), 'repositories')
  }
  return notFound(res)
}

createServer({ cert: readFileSync(certFile), key: readFileSync(keyFile) }, async (req, res) => {
  const url = new URL(req.url, `https://${req.headers.host}`)
  const auth = credential(req)
  const hostname = (req.headers.host ?? '').split(':')[0]
  const before = res.writableEnded
  res.on('finish', () => log({ host: hostname, method: req.method, path: url.pathname.replace(/^\/app-manifests\/[^/]+/, '/app-manifests/:code'), auth: auth.kind, status: res.statusCode }))
  try {
    if (hostname === 'github.com') await web(req, res, url)
    else if (hostname === 'api.github.com') await api(req, res, url, auth)
    else notFound(res)
  } catch (error) {
    if (!before && !res.headersSent) json(res, 500, { message: 'fixture error' })
    log({ event: 'fixture_error', message: String(error?.message ?? error).slice(0, 200) })
  }
}).listen(Number(port), host)
