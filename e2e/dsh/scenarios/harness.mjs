import assert from 'node:assert/strict'
import { execFileSync, spawn } from 'node:child_process'
import fs from 'node:fs'
import http from 'node:http'
import net from 'node:net'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { createDsh, waitFor } from '../lib.mjs'
import { startFakeLlm } from '../spikes/fake-llm.mjs'

export const scenarioDir = path.dirname(fileURLToPath(import.meta.url))
export const repo = path.resolve(scenarioDir, '../../..')
export const dsh = process.env.DSH_BIN ?? path.join(repo, '.tools/dsh-rc5/node_modules/.bin/dsh')
export const chrome = process.env.CHROME ?? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
export const work = process.env.SCENARIO_WORK ?? path.join(repo, '.tools/dsh-scenarios', Date.now().toString(36))
export const shots = path.join(repo, 'e2e/shots/phase-a')
const dsnRoot = process.env.DSH_EE_TEST_DSN ?? 'root:@tcp(127.0.0.1:4001)/'
export const adminPassword = 'scenario-admin-password-1'
export const employeePassword = 'scenario-employee-password-1'
export const employeeName = 'scenario.employee'
export const department = '研发效能部'
export const model = 'scenario-chat'
export const workspaceDir = 'dsh-workspace'
export const { command, startWeb, stopWeb, openPage, submitTask } = createDsh({ dsh, repo, chrome, workspaceLabel: process.env.SPIKE_WORKSPACE_LABEL ?? workspaceDir })
fs.mkdirSync(work, { recursive: true })
fs.mkdirSync(shots, { recursive: true })

/** 真实 DSH 版本写进每个场景结果，兼容性矩阵靠它而不是靠记忆。 */
export const dshVersion = (() => {
  try {
    return execFileSync(dsh, ['--version'], { encoding: 'utf8', timeout: 10_000 }).trim().split('\n')[0] || 'unknown'
  } catch {
    return 'unknown'
  }
})()
fs.writeFileSync(path.join(work, 'dsh-version.txt'), dshVersion + '\n')
console.log(`DSH: ${dshVersion} (${dsh})`)

export function save(name, result) {
  const saved = { ...result, dshVersion }
  fs.mkdirSync(path.join(work, name), { recursive: true })
  fs.writeFileSync(path.join(work, name, 'result.json'), JSON.stringify(saved, null, 2))
  console.log(`\n=== ${name.toUpperCase()} ===\n${JSON.stringify(saved, null, 2)}`)
}

export async function modelRequest(fake, task) {
  return waitFor(() => fake.requests.find((request) => request.body.messages?.some((message) => message.role === 'user' && String(message.content) === task)), `model request: ${task}`, 90_000)
}

export class Session {
  constructor(base) { this.base = base; this.cookie = '' }
  async request(method, url, body, expected) {
    const response = await fetch(this.base + url, {
      method,
      headers: { ...(this.cookie ? { cookie: this.cookie } : {}), ...(body !== undefined ? { 'content-type': 'application/json' } : {}) },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
    const setCookie = response.headers.get('set-cookie')
    if (setCookie) this.cookie = setCookie.split(';')[0]
    const text = await response.text()
    let data = null
    try { data = text ? JSON.parse(text) : null } catch { data = text }
    if (expected !== undefined) assert.equal(response.status, expected, `${method} ${url}: ${text}`)
    else assert.ok(response.status < 400, `${method} ${url}: ${response.status} ${text}`)
    return { status: response.status, data }
  }
  async login(username, password) {
    await this.request('POST', '/api/auth/login', { username, password }, 200)
  }
}

export async function freePort() {
  const server = net.createServer()
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve))
  const port = server.address().port
  await new Promise((resolve) => server.close(resolve))
  return port
}

export async function startServer(extraEnv = {}) {
  const port = await freePort()
  const proxyPort = await freePort()
  const target = `http://127.0.0.1:${port}`
  const base = `http://127.0.0.1:${proxyPort}`
  const dbName = `dsh_ee_scenario_${Date.now().toString(36)}`
  const log = fs.createWriteStream(path.join(work, 'server.log'))
  const env = {
    ...process.env,
    DSH_EE_ADDR: `127.0.0.1:${port}`,
    DSH_EE_DSN: `${dsnRoot}${dbName}`,
    DSH_EE_SECRET_KEY: 'MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=',
    DSH_EE_BOOTSTRAP_ADMIN_PASSWORD: adminPassword,
    DSH_EE_DATA_DIR: path.join(work, 'server-data'),
    DSH_EE_PUBLIC_URL: base,
    ...extraEnv,
  }
  const child = spawn(path.join(repo, 'dist/dsh-ee'), [], { env, cwd: repo })
  child.stdout.pipe(log)
  child.stderr.pipe(log)
  await waitFor(async () => {
    if (child.exitCode !== null) throw new Error(`dsh-ee exited ${child.exitCode}; see ${path.join(work, 'server.log')}`)
    return (await fetch(`${target}/healthz`).catch(() => null))?.ok
  }, 'dsh-ee health', 60_000)
  const requests = []
  const proxy = http.createServer((req, res) => {
    if (req.url === '/v1/chat/completions') requests.push({ method: req.method, url: req.url, headers: req.headers })
    const upstream = http.request({ hostname: '127.0.0.1', port, path: req.url, method: req.method, headers: req.headers }, (response) => {
      res.writeHead(response.statusCode ?? 502, response.headers)
      response.pipe(res)
    })
    upstream.on('error', (error) => { res.writeHead(502); res.end(error.message) })
    req.pipe(upstream)
  })
  await new Promise((resolve) => proxy.listen(proxyPort, '127.0.0.1', resolve))
  return { base, target, dbName, child, log, proxy, requests }
}

export async function stopProcess(child) {
  if (!child || child.exitCode !== null) return
  child.kill('SIGTERM')
  await waitFor(() => child.exitCode !== null, 'process shutdown', 10_000).catch(() => child.kill('SIGKILL'))
}

export function runProcess(file, args, env, onOutput) {
  return new Promise((resolve, reject) => {
    const child = spawn(file, args, { env, cwd: repo })
    let output = ''
    child.stdout.on('data', (chunk) => { output += chunk; onOutput?.(chunk.toString()) })
    child.stderr.on('data', (chunk) => { output += chunk; onOutput?.(chunk.toString()) })
    child.on('error', reject)
    child.on('close', (code) => code === 0 ? resolve(output) : reject(new Error(`${file} ${args.join(' ')} exited ${code}\n${output}`)))
  })
}

export function scenarioEnv(name, token) {
  const home = path.join(work, name, 'home')
  const dshHome = path.join(work, name, 'dsh-home')
  const fakeBin = path.join(work, 'fake-bin')
  fs.mkdirSync(home, { recursive: true })
  // Linux 上全新的 DSH Web 首页要求先选工作区，目录浏览器从（隔离后的）HOME 开始；
  // 在隔离 HOME 里放一个固定名字的工作区目录，供 chooseWorkspace 点选。
  fs.mkdirSync(path.join(home, workspaceDir), { recursive: true })
  fs.mkdirSync(dshHome, { recursive: true })
  fs.mkdirSync(fakeBin, { recursive: true })
  for (const command of ['open', 'xdg-open']) {
    const file = path.join(fakeBin, command)
    fs.writeFileSync(file, '#!/bin/sh\nexit 0\n', { mode: 0o755 })
    fs.chmodSync(file, 0o755)
  }
  const env = {
    ...process.env,
    HOME: home,
    DSH_HOME: dshHome,
    NPM_CONFIG_USERCONFIG: path.join(home, '.npmrc'),
    npm_config_userconfig: path.join(home, '.npmrc'),
    NPM_CONFIG_GLOBALCONFIG: path.join(home, '.npmrc-global'),
    npm_config_globalconfig: path.join(home, '.npmrc-global'),
    NPM_CONFIG_CACHE: path.join(work, 'npm-cache'),
    npm_config_cache: path.join(work, 'npm-cache'),
    npm_config_store_path: path.join(work, 'pnpm-store'),
    PATH: `${fakeBin}:${path.dirname(dsh)}:${process.env.PATH}`,
    DSH_PERMISSION_MODE: 'workspace-write',
    DSH_PROFILES: 'web headless',
    DSH_EE_SETUP_INTERACTIVE: '1',
    NO_PROXY: '127.0.0.1,localhost',
    no_proxy: '127.0.0.1,localhost',
    CI: '',
  }
  delete env.DSHEE_TOKEN
  if (token) env.DSHEE_TOKEN = token
  return env
}

export async function setupDevice(script, env, employee) {
  let code = ''
  let approval
  const output = await runProcess('sh', [script], env, (text) => {
    process.stdout.write(text)
    const match = text.match(/确认码：([BCDFGHJKLMNPQRSTVWXZ23456789]{4}-[BCDFGHJKLMNPQRSTVWXZ23456789]{4})/)
    if (match && !approval) {
      code = match[1]
      approval = employee.request('POST', `/api/device/${encodeURIComponent(code)}`, { approve: true }, 200)
    }
  })
  assert.ok(code, `setup 没输出确认码：${output}`)
  await approval
  return output
}

// 接入脚本与接入插件只写 DSH profile 目录的 .npmrc，不写用户级 ~/.npmrc
export function npmrcToken(env, profile = 'web') {
  const file = path.join(env.DSH_HOME, 'profiles', profile, '.npmrc')
  if (!fs.existsSync(file)) return ''
  return fs.readFileSync(file, 'utf8').match(/_authToken=(ee_at_\S+)/)?.[1] ?? ''
}

export async function prepare(base, fake) {
  const admin = new Session(base)
  await admin.login('admin', adminPassword)
  const settings = (await admin.request('GET', '/api/settings')).data
  settings.baseUrl = base
  settings.connect.enabled = true
  settings.connect.model = true
  settings.connect.defaultModel = model
  await admin.request('PUT', '/api/admin/settings', settings, 200)
  await admin.request('POST', '/api/admin/providers', { name: 'scenario-fake', base_url: fake.url, api_key: 'scenario-upstream', models: [model] }, 201)
  const employeeInfo = (await admin.request('POST', '/api/admin/users', {
    username: employeeName, password: employeePassword, display_name: '场景员工', department, monthly_token_limit: 1_000_000,
  }, 201)).data
  const botInfo = (await admin.request('POST', '/api/admin/users', {
    username: 'scenario.ci', display_name: '场景 CI', department, account_type: 'bot', owner_id: employeeInfo.id, monthly_token_limit: 1_000_000,
  }, 201)).data
  const employee = new Session(base)
  await employee.login(employeeName, employeePassword)
  await admin.request('POST', '/api/hub', {
    kind: 'skill', slug: 'scenario-skill', delivery: 'auto',
    spec: { title: '场景技能', description: '验证企业技能下发', when_to_use: '真实 DSH 场景', body: '场景技能正文。' },
  }, 201)
  const openapi = JSON.stringify({ openapi: '3.0.0', info: { title: 'Health' }, paths: { '/healthz': { get: { operationId: 'health', summary: '健康检查' } } } })
  const connection = (await admin.request('POST', '/api/hub', {
    kind: 'connection', slug: 'scenario-health', delivery: 'auto',
    spec: { title: '场景健康检查', type: 'openapi', url: base, openapi, auth: { type: 'none' } },
  }, 201)).data
  const published = (await admin.request('POST', '/api/admin/connect/publish', {}, 200)).data
  assert.ok(published.package?.endsWith('/dsh-connect'), '发布接入插件失败：' + JSON.stringify(published))
  return { admin, employee, employeeInfo, botInfo, published, connectionId: connection.asset.id }
}


// ---- 阶段 B / C 场景共用：按任务文本取模型请求、取工具结果、跑一轮对话并处理审批 ----

export const forTask = (fake, task) => fake.requests.filter((request) => request.body.messages?.some((message) => message.role === 'user' && String(message.content) === task))
export const text = (content) => (typeof content === 'string' ? content : (content ?? []).map((part) => part.text ?? '').join(''))
export const toolResults = (request) => request.body.messages.filter((message) => message.role === 'tool').map((message) => text(message.content))

/** 返回 turn(fake, page, task, script, { approve, requests, shot })：提交任务，遇到「Allow once」就点，直到收到足够的模型请求。 */
export function makeTurn(shotsDir) {
  fs.mkdirSync(shotsDir, { recursive: true })
  return async function turn(fake, page, task, script, { approve = true, requests, shot } = {}) {
    fake.script(task, script)
    await submitTask(page, task)
    let approvals = 0
    const allow = page.getByRole('button', { name: 'Allow once', exact: true })
    await waitFor(async () => {
      if (await allow.isVisible().catch(() => false)) {
        if (shot && approvals === 0) await page.screenshot({ path: path.join(shotsDir, `${shot}.png`), fullPage: true })
        if (approve === false) return true
        approvals += 1
        await allow.click()
      }
      return forTask(fake, task).length >= requests
    }, `${task}: ${requests} model requests`, 90_000)
    return { all: forTask(fake, task), approvals }
  }
}
