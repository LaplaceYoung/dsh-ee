# 界面多语言（i18n）规范

本目录是前端多语言的唯一入口。**中文（`zh-CN`）是源语言**：先写中文，其它语言按中文的结构翻译，缺键由 TypeScript 直接报错。

- 运行时注册表：`langs.ts`；i18next 初始化与 `t()` 类型：`index.ts`。
- **加语言 = ① 新建语言目录 ② 在 `langs.ts` 里 import + 加一行 `LANGUAGES` + 加一项 `resources`**，组件与 e2e 都不用改（清单见 §7）。
- 当前状态：已注册 `zh-CN` / `en`；`es` / `pt` / `hi` 是目标语言，语言包上线前运行时按 fallback 回退英文。

## 1. 语言码与标签

| 语言码 | 标签（母语） | 状态 |
|---|---|---|
| `zh-CN` | 简体中文 | 源语言（结构源） |
| `en` | English | 已上线 |
| `es` | Español | 目标语言，待语言包 |
| `pt` | Português | 目标语言，待语言包 |
| `hi` | हिन्दी | 目标语言，待语言包 |

规则：

- 语言码用 BCP 47（主语言小写、地区大写，如 `zh-CN`）；界面上的语言标签一律用**母语**写（不用「西班牙语」这种转述）。
- 语言选择的 localStorage 键是 `dsh-ee:lang`；首屏 `web/index.html` 的内联脚本也读这个键，改键要一起改。
- `<html lang>` 与语言码一致：中文固定 `zh-CN`（繁中、港台也显示简体），其它语言用解析后的语言码。

## 2. 目录与命名空间

```
web/src/i18n/
  langs.ts      语言注册表：LANGUAGES / resources / detectLanguage / fallbackLng
  index.ts      i18next 初始化、supportedLngs、html lang 同步、isZh()
  types.ts      Messages<T>：把中文文案的结构放宽成「同样的键、任意字符串」
  zh/           源语言。一个文件 = 一组命名空间（common.ts → common.*、shell.*…）
  en/           英文，文件名与命名空间必须与 zh/ 一一对应
  es/ pt/ hi/   目标语言，结构同上（待建）
  README.md     本文件
```

- 一个页面/功能一个模块；模块 `default export` 里的一级对象就是命名空间，组件里用 `t("命名空间.key")`。
- key 用 camelCase；跨分组的公共词放 `common.*` / `shell.*`，不要为每个页面重复造「保存/取消」。
- `index.ts` 把各模块展开成一棵树，`en/index.ts` 整体 `satisfies Messages<typeof zh>`。
- 禁止用字符串拼 key（`t(prefix + "." + name)` 这类要把前缀补全成字面量或模板字符串，且 zh 里必须有对应 key）。

## 3. 类型约束（tsc 即 CI 门禁）

`zh` 是结构源；`en` / `es` / `pt` / `hi` 的**每个命名空间文件**都这样写：

```ts
import type zh from "../zh/xxx";
import type { Messages } from "../types";

export default {
  // 与 zh/xxx.ts 完全相同的 key 树，值换成译文
} satisfies Messages<typeof zh>;
```

再加 key 的顺序：**先 `zh/`，再所有已翻译语言**。tsc 只保证 key 存在，不保证译文正确——语义与术语仍要人工对照 §6。

## 4. fallback 规则

- **UI：请求语言 → `en` → `zh`。** 不支持的请求语言（`ja`、`fr`…）一律显示英文；`en` 的完备性由类型约束保证，正常情况下不会再到 `zh`；只有类型系统覆盖不到的动态 key、后端返回的多语言字段缺项时才回落到中文源文案，组件不要依赖这个兜底。
- 浏览器语言归一：`zh*`（`zh-TW`、`zh-HK`）→ `zh-CN`；已注册语言的区域变体按主语言归一（`en-US`/`en-GB` → `en`，将来 `es-MX` → `es`、`pt-BR` → `pt`、`hi-IN` → `hi`）；其它未注册语言 → `en`。
- 检测顺序：`localStorage["dsh-ee:lang"]` → `navigator.language`；切换语言会写回 localStorage，下次打开沿用。
- 嵌入页 `?lang=` 只覆盖**本次 iframe 渲染**，URL 值不写 localStorage（i18next 初始化仍会缓存浏览器检测结果，这是原有行为）；未注册语言同样回退英文。详见 `docs/embedding.md`。
- `<html lang>` 随切换即时同步；首屏由 `web/index.html` 内联脚本按同一规则先设一次（避免先闪中文）。

## 5. 翻译守则

1. **占位符原样保留**：`{{name}}`、`{{count}}`、`{{reset}}` 的名字、大小写、花括号一个字符都不能动；不要翻译占位符本身。
2. **保留标记与内部结构**：`<link>…</link>`、`<strong>…</strong>`、换行、列表层级照旧；只翻译标签之间的文字，标签与属性不动。
3. **品牌 / 技术词不译**（保留原文与大小写）：`dsh-ee`、`DSH`、`Harness`、`DeepSeek`、`npm`、`token`、`API`、`MCP`、`OIDC`、`LDAP`、`SSO`、`RBAC`、`URL`、`JSON`、`CSV`、`ID`、`CLI`、`SDK`、`GitHub`、`Markdown`、`AI`、`LLM`、`OAuth`、`Webhook`、`YAML`、`iframe`、`postMessage` 等；各语言 glossary 里另有约定的按 glossary。
4. **示例 / 演示数据保持原样**：如「星河科技」（Xinghe Tech）、人名、部门名、示例 slug、示例 URL、mock 数据标题；后端数据里自带的多语言字段用 `isZh()` 选择，不要翻译数据。
5. **按钮用短动词**：Save / Guardar / Salvar / सहेजें，不要写成一句话；同一动作用词全仓一致。
6. **中文标点不带到其它语言**：`，。；：！？「」（）` 不要出现；英文用 `, . ; : ! ? "…"`，西语用 `¿ ¡ «»`，葡语用 `«»` 或 `"…"`，印地语用 `।` 等标准标点。
7. **日期 / 数字 / 货币不手写**：统一走 `Intl` 工具（P2 提供 `formatDate` / `formatNumber`）；在工具就绪前，新增文案优先用占位符接收已格式化的值。
8. **大小写与空格**：默认跟随英文现状（sentence case）；标题可按各语言习惯。中文与拉丁/天城文之间保留一个空格。
9. **长度**：西/葡/印语通常比中英长 20–40%，按钮、页签、窄屏内的标签优先用短词；不要为了塞下而乱缩写。
10. **不要机翻直接提交**：术语按 §6，同一术语全仓一致；有疑问先改 glossary 再改语言包。

## 6. 术语表（glossary）

同一术语在所有语言、所有页面保持一致；西/葡/印语用标准译法。品牌/技术词（见 §5.3）不进表。

| 中文 | English | Español | Português | हिन्दी |
|---|---|---|---|---|
| 保存 | Save | Guardar | Salvar | सहेजें |
| 取消 | Cancel | Cancelar | Cancelar | रद्द करें |
| 删除 | Delete | Eliminar | Excluir | हटाएँ |
| 编辑 | Edit | Editar | Editar | संपादित करें |
| 创建 | Create | Crear | Criar | बनाएँ |
| 搜索 | Search | Buscar | Pesquisar | खोजें |
| 设置 | Settings | Configuración | Configurações | सेटिंग्स |
| 仪表板 | Dashboard | Panel de control | Painel | डैशबोर्ड |
| 工作台 | Workbench | Espacio de trabajo | Espaço de trabalho | कार्यक्षेत्र |
| 令牌 | Token | Token | Token | टोकन |
| 权限 | Permission | Permiso | Permissão | अनुमति |
| 角色 | Role | Rol | Função | भूमिका |
| 部门 | Department | Departamento | Departamento | विभाग |
| 员工 | Employee | Empleado | Funcionário | कर्मचारी |
| 企业 | Enterprise | Empresa | Empresa | उद्यम |
| 能力中心 | Capability hub | Centro de capacidades | Centro de capacidades | क्षमता केंद्र |
| 微应用 | Micro app | Microaplicación | Microaplicativo | माइक्रो ऐप |
| 应用 | Application | Aplicación | Aplicativo | ऐप |
| 连接 | Connection | Conexión | Conexão | कनेक्शन |
| 目录 | Directory | Directorio | Diretório | निर्देशिका |
| 知识库 | Knowledge base | Base de conocimiento | Base de conhecimento | ज्ञान आधार |
| 审核 | Review | Revisión | Revisão | समीक्षा |
| 额度 | Quota | Cuota | Cota | कोटा |
| 插件 | Plugin | Complemento | Plugin | प्लगइन |
| 扩展 | Extension | Extensión | Extensão | एक्सटेंशन |
| 签名 | Signature | Firma | Assinatura | हस्ताक्षर |
| 信任根 | Root of trust | Raíz de confianza | Raiz de confiança | ट्रस्ट रूट |
| 发布 | Publish | Publicar | Publicar | प्रकाशित करें |
| 安装 | Install | Instalar | Instalar | इंस्टॉल करें |
| 升级 | Upgrade | Actualizar | Atualizar | अपग्रेड करें |
| 回滚 | Rollback | Revertir | Reverter | रोलबैक |
| 嵌入 | Embed | Insertar | Incorporar | एम्बेड करें |
| 会话 | Session | Sesión | Sessão | सत्र |
| 审计 | Audit | Auditoría | Auditoria | ऑडिट |
| 速率限制 | Rate limit | Límite de velocidad | Limite de taxa | दर सीमा |
| 请求 ID | Request ID | ID de solicitud | ID da solicitação | अनुरोध आईडी |
| API 客户端 | API client | Cliente de API | Cliente de API | API क्लाइंट |
| 设备码 | Device code | Código de dispositivo | Código de dispositivo | डिवाइस कोड |
| 上游源 | Upstream source | Fuente upstream | Fonte upstream | अपस्ट्रीम स्रोत |
| 字段 | Field | Campo | Campo | फ़ील्ड |
| 版本 | Version | Versión | Versão | संस्करण |
| 工作流 | Workflow | Flujo de trabajo | Fluxo de trabalho | कार्यप्रवाह |
| 状态 | Status | Estado | Status | स्थिति |
| 名称 | Name | Nombre | Nome | नाम |
| 描述 | Description | Descripción | Descrição | विवरण |
| 用户名 | Username | Nombre de usuario | Nome de usuário | उपयोगकर्ता नाम |
| 密码 | Password | Contraseña | Senha | पासवर्ड |
| 登录 | Log in | Iniciar sesión | Entrar | साइन इन करें |
| 退出登录 | Log out | Cerrar sesión | Sair | साइन आउट करें |
| 邮箱 | Email | Correo electrónico | E-mail | ईमेल |
| 上传 | Upload | Subir | Enviar | अपलोड करें |
| 下载 | Download | Descargar | Baixar | डाउनलोड करें |
| 复制 | Copy | Copiar | Copiar | कॉपी करें |
| 提交 | Submit | Enviar | Submeter | सबमिट करें |
| 确认 | Confirm | Confirmar | Confirmar | पुष्टि करें |
| 全部 | All | Todos | Todos | सभी |
| 启用 | Enable | Habilitar | Ativar | सक्षम करें |
| 测试 | Test | Probar | Testar | परीक्षण |
| 部署 | Deploy | Desplegar | Implantar | तैनात करें |
| 文档 | Documentation | Documentación | Documentação | दस्तावेज़ |
| 示例 | Example | Ejemplo | Exemplo | उदाहरण |
| 通知 | Notification | Notificación | Notificação | सूचना |
| 请求 | Request | Solicitud | Solicitação | अनुरोध |

术语变更流程：先改本表 → 再改所有语言包 → 同一 PR 附 glossary diff；不要只改某一个页面。

## 7. 新增一门语言（清单）

以 `es`（Español）为例，按顺序做完即可：

1. **确认语言码**：`langs.ts` 的 `LANGUAGES` 用 BCP 47 码（`es`）；标签用母语（`Español`）。
2. **新建语言目录**：`web/src/i18n/es/`，文件名与 `zh/` 一一对应；从 `es/index.ts` 开始，各模块 `satisfies Messages<typeof zhXxx>`，先跑通 tsc 再补译文。
3. **注册语言**（唯一需要改代码的地方）：`langs.ts` 里
   - `import es from "./es";`
   - `LANGUAGES` 加一行 `{ code: "es", label: "Español" }`
   - `resources` 加一项 `es: { translation: es }`
   三处加完，`Lang` 类型、语言选择器、e2e 的语言列表自动生效。
4. **浏览器语言归一**：`es-MX` → `es` 这类区域变体由 `detectLanguage` 按注册表自动处理，一般不用改；只有主语言要特殊处理（如 `zh*` → `zh-CN`）时才加特例。
5. **首屏脚本**：`web/index.html` 的内联脚本目前只认 `zh`/`en`，**必须补**该语言的 `<html lang>` 分支与首屏 `<title>`（标题文案走产品/品牌确认）；否则首屏会闪中文标题。
6. **嵌入页语言码**：`web/src/lib/embed.ts` 的 `EMBED_LANGS` 已包含 `es`/`pt`/`hi`，注册后 `?lang=es` 自动生效；如新增六语目标外语言，两边一起加。
7. **e2e**：`06-i18n.mjs` 自动读注册表，一般不用改；在语言 PR 里把该语言加进页面扫描列表（现在只扫 `zh-CN`/`en`），并跑真浏览器 e2e。
8. **验收**：`cd web && node_modules/.bin/tsc -p tsconfig.json --noEmit`；`cd web && pnpm build`；`node ../scripts/docs/build.mjs --check`；e2e 全量（含新语言页面扫描）。
9. **PR 范围**：语言 PR 只加语言包 + 第 3 步注册 + 第 5 步首屏标题 + e2e 扫描，不混功能改动。

## 8. 维护规则

- **改 `zh` 文案必须同步改其它已翻译语言**：tsc 只保证 key 存在，不保证语义；漏同步在 review 里算 bug。
- **禁止在组件里写死用户可见文案**：包括 JSX 文本、`aria-label`、`title`、`placeholder`、`toast`、空状态、错误兜底、按钮。新增文案先查 `common.*` 能否复用。
- **动态 key 只能由字面量前缀拼**：确保 zh 里存在该 key；不要用运行时字符串生成新 key。
- **数据自带的多语言字段**（后端 spec 的 `title.zh`/`title.en`）用 `isZh()` 选取，不进 i18n 资源；新增这类字段时同时提供 zh/en（其余语言随语言包扩展）。
- **日期 / 数字 / 货币 / 相对时间**统一用 `Intl` 工具（P2），不要在文案里手写 `` `${n} 天前` ``。
- **术语统一**：改术语先改 §6，再全仓替换；同一 PR 内完成。
- **语言入口位置**（新增页面原则上复用）：侧栏底部、登录页、落地页顶栏（手机端 compact）、移动端顶栏、账号页「偏好设置」、开发文档列表/详情页头部；组件是 `components/lang-switch.tsx`，窄屏用 `compact`。
- **评审检查**：tsc 通过、其它已翻译语言已补齐、占位符完整、术语符合 glossary、标点正确、375px 宽下按钮/入口不溢出。

## 9. 相关文件

| 文件 | 作用 |
|---|---|
| `langs.ts` | 语言注册表（唯一事实源） |
| `index.ts` | i18next 初始化、`supportedLngs`、fallback、`<html lang>` 同步、`isZh()` |
| `types.ts` | `Messages<T>` 类型约束 |
| `components/lang-switch.tsx` | 语言切换器（`compact` 变体用于窄屏） |
| `web/index.html` | 首屏内联脚本：先落主题，再落 `<html lang>` 与标题 |
| `web/src/lib/embed.ts` / `docs/embedding.md` | 嵌入页 `?lang=` 参数与 `hello`/`ready` 的 `locale`、`supports: lang` |
| `e2e/tests/06-i18n.mjs` | 多语言 e2e：语言列表读注册表、未翻译 key 前缀读 zh 资源 |
