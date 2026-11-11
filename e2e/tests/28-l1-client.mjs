// 非 DSH 客户端 L1 冒烟：只拿一个统一令牌，走模型网关和 /mcp，不需要安装 DSH。
// 目标读者是 curl / 自研 Agent / 企业集成方；它证明 dsh-ee 的 L1 接入面不依赖 DSH 前端。
import http from "node:http";
import { ADMIN_PW, BASE, api, assert, assertEq, login, newPage, step, tag } from "../lib.mjs";

let rpcId = 0;

async function mcp(token, method, params) {
  const response = await fetch(`${BASE}/mcp`, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
      Accept: "application/json, text/event-stream",
    },
    body: JSON.stringify({ jsonrpc: "2.0", id: ++rpcId, method, params }),
  });
  assertEq(response.status, 200, `/mcp ${method}`);
  return response.json();
}

export default async function (t) {
  const upstream = http.createServer((req, res) => {
    if (req.headers.authorization !== "Bearer sk-l1-upstream") {
      res.statusCode = 401;
      return res.end("{}");
    }
    let body = "";
    req.on("data", (chunk) => (body += chunk));
    req.on("end", () => {
      res.setHeader("Content-Type", "application/json");
      if (req.url === "/v1/chat/completions") {
        const parsed = JSON.parse(body || "{}");
        return res.end(JSON.stringify({
          id: "l1-smoke",
          model: parsed.model,
          choices: [{ index: 0, message: { role: "assistant", content: "pong" }, finish_reason: "stop" }],
          usage: { prompt_tokens: 2, completion_tokens: 1, total_tokens: 3 },
        }));
      }
      res.statusCode = 404;
      res.end("{}");
    });
  });
  await new Promise((resolve) => upstream.listen(0, "127.0.0.1", resolve));
  const port = upstream.address().port;
  const admin = await newPage(t);
  let providerId = null;
  let tokenInfo = null;
  try {
    await login(admin, "admin", ADMIN_PW);
    assertEq((await api(admin, "GET", "/healthz")).status, 200, "healthz 不需要登录");

    const model = `l1-chat-${tag}`;
    const provider = await api(admin, "POST", "/api/admin/providers", {
      name: `L1 冒烟 ${tag}`,
      vendor: "custom",
      base_url: `http://127.0.0.1:${port}/v1`,
      api_key: "sk-l1-upstream",
      models: [model],
    });
    assertEq(provider.status, 201, "建 L1 上游");
    providerId = provider.data.id;

    const me = await api(admin, "GET", "/api/auth/me");
    assertEq(me.status, 200, "当前管理员");
    const issued = await api(admin, "POST", "/api/admin/tokens", {
      user_id: me.data.id,
      name: `l1-smoke-${tag}`,
      scopes: ["models", "agent", "connections"],
      expires_in_days: 1,
    });
    assertEq(issued.status, 201, "签发统一令牌");
    const token = issued.data.token;
    tokenInfo = issued.data.info;
    assert(token.startsWith("ee_at_"), "统一令牌前缀");

    const models = await fetch(`${BASE}/v1/models`, { headers: { Authorization: `Bearer ${token}` } });
    assertEq(models.status, 200, "/v1/models");
    const listed = await models.json();
    assert(listed.data.some((m) => m.id === model), "/v1/models 应包含刚建的模型：" + JSON.stringify(listed));
    step("curl 用统一令牌读取 /v1/models");

    const chat = await fetch(`${BASE}/v1/chat/completions`, {
      method: "POST",
      headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
      body: JSON.stringify({ model, messages: [{ role: "user", content: "ping" }] }),
    });
    assertEq(chat.status, 200, "/v1/chat/completions");
    const answer = await chat.json();
    assertEq(answer.choices?.[0]?.message?.content, "pong", "模型网关返回");
    step("curl 用同一个令牌调用 /v1/chat/completions");

    const init = await mcp(token, "initialize", {
      protocolVersion: "2025-06-18",
      capabilities: {},
      clientInfo: { name: "curl", version: "1.0" },
    });
    assert(init.result?.serverInfo?.name, "MCP initialize 返回 serverInfo：" + JSON.stringify(init));
    const tools = await mcp(token, "tools/list", {});
    const names = (tools.result?.tools ?? []).map((x) => x.name);
    assert(names.includes("ee_me"), "/mcp 应暴露员工侧工具：" + JSON.stringify(names));
    step("同一个令牌完成 /mcp initialize + tools/list（无需 DSH）");
  } finally {
    if (providerId) await api(admin, "DELETE", `/api/admin/providers/${providerId}`);
    if (tokenInfo?.id) await api(admin, "POST", "/api/admin/tokens/revoke", { ids: [tokenInfo.id] });
    upstream.close();
  }
}
