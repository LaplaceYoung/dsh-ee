package feishu

import (
	"sort"
	"strings"

	"github.com/hikariming/dsh-ee/server/internal/i18n"
)

// 飞书消息键：只覆盖真正发给飞书用户的文案——审批卡（标题/正文/按钮）、文本结果通知、
// 知识库待补提醒，以及点卡片按钮后回给操作人的 toast。JSON 卡片结构不变，只换文本值。
//
// 渲染时按「收件人用户语言 → 组织默认语言 → zh」选语言（见 i18n.Resolve）；群/机器人等
// 无法定位收件人的场景直接用组织默认语言。参数统一写 {{name}}，由 i18n.Render 替换。
//
// 不在这里的：飞书设置页/组织同步结果等管理端接口文案（属 P4 的 API 文案），以及
// internal/feishu 里只进日志和错误链的接口错误（不会发给飞书用户）。
const (
	MsgCardHubTitle            = "feishu.card.hub.title"
	MsgCardHubDetail           = "feishu.card.hub.detail"
	MsgCardScopeTitle          = "feishu.card.scope.title"
	MsgCardScopeDetail         = "feishu.card.scope.detail"
	MsgCardAccessTitle         = "feishu.card.access.title"
	MsgCardAccessDetail        = "feishu.card.access.detail"
	MsgCardNpmTitle            = "feishu.card.npm.title"
	MsgCardNpmDetail           = "feishu.card.npm.detail"
	MsgCardNpmDetailUnknownPub = "feishu.card.npm.detail_unknown_publisher"
	MsgCardButtonApprove       = "feishu.card.button.approve"
	MsgCardButtonReject        = "feishu.card.button.reject"

	MsgResultApproved = "feishu.result.approved"
	MsgResultRejected = "feishu.result.rejected"
	MsgResultOther    = "feishu.result.other"
	MsgKBGap          = "feishu.kb.gap"

	MsgToastCardInvalid            = "feishu.toast.card_invalid"
	MsgToastCardMismatch           = "feishu.toast.card_mismatch"
	MsgToastAlreadyHandled         = "feishu.toast.already_handled"
	MsgToastCardExpired            = "feishu.toast.card_expired"
	MsgToastDecisionApproved       = "feishu.toast.decision_approved"
	MsgToastDecisionRejected       = "feishu.toast.decision_rejected"
	MsgToastOperatorNotProvisioned = "feishu.toast.operator_not_provisioned"
	MsgToastOperatorDisabled       = "feishu.toast.operator_disabled"
	MsgToastNoReviewPermAsset      = "feishu.toast.no_review_perm_asset"
	MsgToastNoScopePerm            = "feishu.toast.no_scope_perm"
	MsgToastNotApprover            = "feishu.toast.not_approver"
	MsgToastVersionTaken           = "feishu.toast.version_taken"
	MsgToastScopeTaken             = "feishu.toast.scope_taken"
	MsgToastRequestTaken           = "feishu.toast.request_taken"
	MsgToastUnknownAction          = "feishu.toast.unknown_action"
	MsgToastNoReviewPermNpm        = "feishu.toast.no_review_perm_npm"
	MsgToastNpmVersionMissing      = "feishu.toast.npm_version_missing"
	MsgToastNpmTaken               = "feishu.toast.npm_taken"
)

// messages 是五语消息表：消息键 → 规范语言短码 → 模板。zh 是源语言（现有中文逐字保留），
// 其余四种语言按同一组 {{参数}} 翻译。缺语言时 Text 依次回退 en、zh，所以漏写某条译文
// 不会把内部键名发给用户；但 messages_test.go 的完整性测试会直接卡住漏译。
var messages = map[string]map[string]string{
	MsgCardHubTitle: {
		i18n.Zh: "{{title}} · 待审核",
		i18n.En: "{{title}} · Pending review",
		i18n.Es: "{{title}} · Pendiente de revisión",
		i18n.Pt: "{{title}} · Pendente de revisão",
		i18n.Hi: "{{title}} · समीक्षा लंबित",
	},
	MsgCardHubDetail: {
		i18n.Zh: "类型：{{kind}}\n版本：v{{version}}\n请核对改动后决定。网页：{{path}}",
		i18n.En: "Type: {{kind}}\nVersion: v{{version}}\nPlease review the changes and decide. Web: {{path}}",
		i18n.Es: "Tipo: {{kind}}\nVersión: v{{version}}\nRevisa los cambios y decide. Web: {{path}}",
		i18n.Pt: "Tipo: {{kind}}\nVersão: v{{version}}\nRevise as alterações e decida. Web: {{path}}",
		i18n.Hi: "प्रकार: {{kind}}\nसंस्करण: v{{version}}\nकृपया बदलाव जाँचें और निर्णय लें। वेब: {{path}}",
	},
	MsgCardScopeTitle: {
		i18n.Zh: "{{title}} · 范围分享申请",
		i18n.En: "{{title}} · Scope sharing request",
		i18n.Es: "{{title}} · Solicitud para compartir alcance",
		i18n.Pt: "{{title}} · Solicitação de compartilhamento de escopo",
		i18n.Hi: "{{title}} · स्कोप साझा करने का अनुरोध",
	},
	MsgCardScopeDetail: {
		i18n.Zh: "申请扩大「{{title}}」的可用范围。网页：{{path}}",
		i18n.En: "Request to expand the available scope of \"{{title}}\". Web: {{path}}",
		i18n.Es: "Solicitud para ampliar el alcance disponible de «{{title}}». Web: {{path}}",
		i18n.Pt: "Solicitação para ampliar o escopo disponível de “{{title}}”. Web: {{path}}",
		i18n.Hi: "“{{title}}” का उपलब्ध दायरा बढ़ाने का अनुरोध। वेब: {{path}}",
	},
	MsgCardAccessTitle: {
		i18n.Zh: "{{title}} · 权限申请",
		i18n.En: "{{title}} · Access request",
		i18n.Es: "{{title}} · Solicitud de permisos",
		i18n.Pt: "{{title}} · Solicitação de acesso",
		i18n.Hi: "{{title}} · अनुमति अनुरोध",
	},
	MsgCardAccessDetail: {
		i18n.Zh: "申请人：{{applicant}}\n用途：{{reason}}\n请确认是否开放「{{title}}」的必要权限。",
		i18n.En: "Requester: {{applicant}}\nPurpose: {{reason}}\nPlease confirm whether to grant the required access to \"{{title}}\".",
		i18n.Es: "Solicitante: {{applicant}}\nMotivo: {{reason}}\nConfirma si se deben conceder los permisos necesarios para «{{title}}».",
		i18n.Pt: "Solicitante: {{applicant}}\nFinalidade: {{reason}}\nConfirme se as permissões necessárias para “{{title}}” devem ser concedidas.",
		i18n.Hi: "अनुरोधकर्ता: {{applicant}}\nउद्देश्य: {{reason}}\nकृपया पुष्टि करें कि “{{title}}” के लिए आवश्यक अनुमतियाँ दी जानी चाहिए या नहीं।",
	},
	MsgCardNpmTitle: {
		i18n.Zh: "{{object}} · 插件待审核",
		i18n.En: "{{object}} · Plugin pending review",
		i18n.Es: "{{object}} · Plugin pendiente de revisión",
		i18n.Pt: "{{object}} · Plugin pendente de revisão",
		i18n.Hi: "{{object}} · प्लगइन समीक्षा लंबित",
	},
	MsgCardNpmDetail: {
		i18n.Zh: "包名：{{name}}\n版本：{{version}}\n发布者：{{publisher}}\n访问：{{url}}",
		i18n.En: "Package: {{name}}\nVersion: {{version}}\nPublisher: {{publisher}}\nDetails: {{url}}",
		i18n.Es: "Paquete: {{name}}\nVersión: {{version}}\nPublicador: {{publisher}}\nAcceso: {{url}}",
		i18n.Pt: "Pacote: {{name}}\nVersão: {{version}}\nPublicador: {{publisher}}\nAcesso: {{url}}",
		i18n.Hi: "पैकेज: {{name}}\nसंस्करण: {{version}}\nप्रकाशक: {{publisher}}\nएक्सेस: {{url}}",
	},
	MsgCardNpmDetailUnknownPub: {
		i18n.Zh: "包名：{{name}}\n版本：{{version}}\n发布者：未知发布者\n访问：{{url}}",
		i18n.En: "Package: {{name}}\nVersion: {{version}}\nPublisher: Unknown publisher\nDetails: {{url}}",
		i18n.Es: "Paquete: {{name}}\nVersión: {{version}}\nPublicador: Publicador desconocido\nAcceso: {{url}}",
		i18n.Pt: "Pacote: {{name}}\nVersão: {{version}}\nPublicador: Publicador desconhecido\nAcesso: {{url}}",
		i18n.Hi: "पैकेज: {{name}}\nसंस्करण: {{version}}\nप्रकाशक: अज्ञात प्रकाशक\nएक्सेस: {{url}}",
	},
	MsgCardButtonApprove: {
		i18n.Zh: "通过",
		i18n.En: "Approve",
		i18n.Es: "Aprobar",
		i18n.Pt: "Aprovar",
		i18n.Hi: "स्वीकृत करें",
	},
	MsgCardButtonReject: {
		i18n.Zh: "驳回",
		i18n.En: "Reject",
		i18n.Es: "Rechazar",
		i18n.Pt: "Rejeitar",
		i18n.Hi: "अस्वीकार करें",
	},

	MsgResultApproved: {
		i18n.Zh: "审批结果：{{title}} · 已通过",
		i18n.En: "Approval result: {{title}} · Approved",
		i18n.Es: "Resultado de la aprobación: {{title}} · Aprobado",
		i18n.Pt: "Resultado da aprovação: {{title}} · Aprovado",
		i18n.Hi: "अनुमोदन परिणाम: {{title}} · स्वीकृत",
	},
	MsgResultRejected: {
		i18n.Zh: "审批结果：{{title}} · 已驳回",
		i18n.En: "Approval result: {{title}} · Rejected",
		i18n.Es: "Resultado de la aprobación: {{title}} · Rechazado",
		i18n.Pt: "Resultado da aprovação: {{title}} · Rejeitado",
		i18n.Hi: "अनुमोदन परिणाम: {{title}} · अस्वीकृत",
	},
	MsgResultOther: {
		i18n.Zh: "审批结果：{{title}} · {{status}}",
		i18n.En: "Approval result: {{title}} · {{status}}",
		i18n.Es: "Resultado de la aprobación: {{title}} · {{status}}",
		i18n.Pt: "Resultado da aprovação: {{title}} · {{status}}",
		i18n.Hi: "अनुमोदन परिणाम: {{title}} · {{status}}",
	},
	MsgKBGap: {
		i18n.Zh: "知识库「{{title}}」有待补文档：员工检索「{{query}}」没有找到答案（{{count}} 次）。请到 /knowledge/{{id}} 处理。",
		i18n.En: "The knowledge base \"{{title}}\" has missing documents: employee searches for \"{{query}}\" found no answer ({{count}} times). Handle it at /knowledge/{{id}}.",
		i18n.Es: "La base de conocimiento «{{title}}» tiene documentos pendientes: las búsquedas de «{{query}}» no encontraron respuesta ({{count}} veces). Gestiónalo en /knowledge/{{id}}.",
		i18n.Pt: "A base de conhecimento “{{title}}” tem documentos pendentes: as pesquisas dos funcionários por “{{query}}” não encontraram resposta ({{count}} vezes). Trate em /knowledge/{{id}}.",
		i18n.Hi: "ज्ञान आधार “{{title}}” में लंबित दस्तावेज़ हैं: कर्मचारियों की “{{query}}” खोजों का कोई उत्तर नहीं मिला ({{count}} बार)। कृपया /knowledge/{{id}} पर निपटाएँ।",
	},

	MsgToastCardInvalid: {
		i18n.Zh: "这张卡片已经失效，请到 dsh-ee 网页上处理",
		i18n.En: "This card is no longer valid. Please handle it on the dsh-ee website.",
		i18n.Es: "Esta tarjeta ya no es válida. Gestiónalo en el sitio web de dsh-ee.",
		i18n.Pt: "Este cartão não é mais válido. Trate no site do dsh-ee.",
		i18n.Hi: "यह कार्ड अब मान्य नहीं है। कृपया dsh-ee वेबसाइट पर निपटाएँ।",
	},
	MsgToastCardMismatch: {
		i18n.Zh: "卡片和审批对不上，请到 dsh-ee 网页上处理",
		i18n.En: "The card does not match the approval. Please handle it on the dsh-ee website.",
		i18n.Es: "La tarjeta no coincide con la aprobación. Gestiónalo en el sitio web de dsh-ee.",
		i18n.Pt: "O cartão não corresponde à aprovação. Trate no site do dsh-ee.",
		i18n.Hi: "कार्ड और अनुमोदन मेल नहीं खाते। कृपया dsh-ee वेबसाइट पर निपटाएँ।",
	},
	MsgToastAlreadyHandled: {
		i18n.Zh: "这条审批已经处理过了",
		i18n.En: "This approval has already been handled.",
		i18n.Es: "Esta aprobación ya se ha gestionado.",
		i18n.Pt: "Esta aprovação já foi tratada.",
		i18n.Hi: "यह अनुमोदन पहले ही निपटाया जा चुका है।",
	},
	MsgToastCardExpired: {
		i18n.Zh: "这张卡片已经过期，请到 dsh-ee 网页上处理",
		i18n.En: "This card has expired. Please handle it on the dsh-ee website.",
		i18n.Es: "Esta tarjeta ha caducado. Gestiónalo en el sitio web de dsh-ee.",
		i18n.Pt: "Este cartão expirou. Trate no site do dsh-ee.",
		i18n.Hi: "यह कार्ड समाप्त हो चुका है। कृपया dsh-ee वेबसाइट पर निपटाएँ।",
	},
	MsgToastDecisionApproved: {
		i18n.Zh: "已通过：{{title}}",
		i18n.En: "Approved: {{title}}",
		i18n.Es: "Aprobado: {{title}}",
		i18n.Pt: "Aprovado: {{title}}",
		i18n.Hi: "स्वीकृत: {{title}}",
	},
	MsgToastDecisionRejected: {
		i18n.Zh: "已驳回：{{title}}",
		i18n.En: "Rejected: {{title}}",
		i18n.Es: "Rechazado: {{title}}",
		i18n.Pt: "Rejeitado: {{title}}",
		i18n.Hi: "अस्वीकृत: {{title}}",
	},
	MsgToastOperatorNotProvisioned: {
		i18n.Zh: "飞书回调操作人未在 dsh-ee 开通账号",
		i18n.En: "The Feishu callback operator has no account in dsh-ee.",
		i18n.Es: "El operador del callback de Feishu no tiene cuenta en dsh-ee.",
		i18n.Pt: "O operador do callback do Feishu não tem conta no dsh-ee.",
		i18n.Hi: "फ़ीशू कॉलबैक ऑपरेटर का dsh-ee में खाता नहीं है।",
	},
	MsgToastOperatorDisabled: {
		i18n.Zh: "飞书回调操作人已被禁用",
		i18n.En: "The Feishu callback operator has been disabled.",
		i18n.Es: "El operador del callback de Feishu está deshabilitado.",
		i18n.Pt: "O operador do callback do Feishu está desativado.",
		i18n.Hi: "फ़ीशू कॉलबैक ऑपरेटर अक्षम कर दिया गया है।",
	},
	MsgToastNoReviewPermAsset: {
		i18n.Zh: "没有权限：你没有审核这个版本的权限",
		i18n.En: "No permission: you cannot review this version.",
		i18n.Es: "Sin permiso: no puedes revisar esta versión.",
		i18n.Pt: "Sem permissão: você não pode revisar esta versão.",
		i18n.Hi: "अनुमति नहीं: आप इस संस्करण की समीक्षा नहीं कर सकते।",
	},
	MsgToastNoScopePerm: {
		i18n.Zh: "没有权限：你没有处理范围分享的权限",
		i18n.En: "No permission: you cannot handle scope sharing.",
		i18n.Es: "Sin permiso: no puedes gestionar el uso compartido de alcance.",
		i18n.Pt: "Sem permissão: você não pode tratar o compartilhamento de escopo.",
		i18n.Hi: "अनुमति नहीं: आप स्कोप साझा करना नहीं संभाल सकते।",
	},
	MsgToastNotApprover: {
		i18n.Zh: "没有权限：你不是这条申请的审批人",
		i18n.En: "No permission: you are not an approver of this request.",
		i18n.Es: "Sin permiso: no eres aprobador de esta solicitud.",
		i18n.Pt: "Sem permissão: você não é aprovador desta solicitação.",
		i18n.Hi: "अनुमति नहीं: आप इस अनुरोध के अनुमोदक नहीं हैं।",
	},
	MsgToastVersionTaken: {
		i18n.Zh: "这个版本已被其他人处理或不是待审核状态",
		i18n.En: "This version has already been handled by someone else or is not pending review.",
		i18n.Es: "Esta versión ya ha sido gestionada por otra persona o no está pendiente de revisión.",
		i18n.Pt: "Esta versão já foi tratada por outra pessoa ou não está pendente de revisão.",
		i18n.Hi: "यह संस्करण किसी और द्वारा निपटाया जा चुका है या समीक्षा के लिए लंबित नहीं है।",
	},
	MsgToastScopeTaken: {
		i18n.Zh: "范围分享已被其他人处理",
		i18n.En: "This scope sharing request has already been handled.",
		i18n.Es: "Esta solicitud de uso compartido de alcance ya ha sido gestionada.",
		i18n.Pt: "Esta solicitação de compartilhamento de escopo já foi tratada.",
		i18n.Hi: "यह स्कोप साझा अनुरोध पहले ही निपटाया जा चुका है।",
	},
	MsgToastRequestTaken: {
		i18n.Zh: "这条申请已被其他人处理",
		i18n.En: "This request has already been handled.",
		i18n.Es: "Esta solicitud ya ha sido gestionada.",
		i18n.Pt: "Esta solicitação já foi tratada.",
		i18n.Hi: "यह अनुरोध पहले ही निपटाया जा चुका है।",
	},
	MsgToastUnknownAction: {
		i18n.Zh: "未知的审批动作",
		i18n.En: "Unknown approval action.",
		i18n.Es: "Acción de aprobación desconocida.",
		i18n.Pt: "Ação de aprovação desconhecida.",
		i18n.Hi: "अज्ञात अनुमोदन क्रिया।",
	},
	MsgToastNoReviewPermNpm: {
		i18n.Zh: "没有权限：你没有审核 NPM 插件版本的权限",
		i18n.En: "No permission: you cannot review NPM plugin versions.",
		i18n.Es: "Sin permiso: no puedes revisar versiones de plugins NPM.",
		i18n.Pt: "Sem permissão: você não pode revisar versões de plugins NPM.",
		i18n.Hi: "अनुमति नहीं: आप NPM प्लगइन संस्करणों की समीक्षा नहीं कर सकते।",
	},
	MsgToastNpmVersionMissing: {
		i18n.Zh: "插件版本不存在",
		i18n.En: "The plugin version does not exist.",
		i18n.Es: "La versión del plugin no existe.",
		i18n.Pt: "A versão do plugin não existe.",
		i18n.Hi: "प्लगइन संस्करण मौजूद नहीं है।",
	},
	MsgToastNpmTaken: {
		i18n.Zh: "这个插件版本已被处理或不是待审核状态",
		i18n.En: "This plugin version has already been handled or is not pending review.",
		i18n.Es: "Esta versión del plugin ya ha sido gestionada o no está pendiente de revisión.",
		i18n.Pt: "Esta versão do plugin já foi tratada ou não está pendente de revisão.",
		i18n.Hi: "यह प्लगइन संस्करण पहले ही निपटाया जा चुका है या समीक्षा के लिए लंबित नहीं है।",
	},
}

// Text 按语言渲染一条飞书消息。语言先归一；该语言没写时回退英文、再回退中文；
// 连消息键都不存在时返回键名本身（测试会卡住未登记键，不会静默发空消息）。
func Text(lang, key string, params map[string]string) string {
	tpl := template(lang, key)
	if tpl == "" {
		return key
	}
	return i18n.Render(tpl, params)
}

func template(lang, key string) string {
	table := messages[key]
	if table == nil {
		return ""
	}
	if l := i18n.Normalize(lang); l != "" {
		if tpl := strings.TrimSpace(table[l]); tpl != "" {
			return tpl
		}
	}
	for _, fallback := range []string{i18n.En, i18n.Zh} {
		if tpl := strings.TrimSpace(table[fallback]); tpl != "" {
			return tpl
		}
	}
	return ""
}

// MessageKeys 返回全部消息键（排序）。文案完整性测试和审计脚本用它。
func MessageKeys() []string {
	keys := make([]string, 0, len(messages))
	for k := range messages {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// MessageLanguages 返回消息表要求的语言：与 i18n.Languages 保持一致。
func MessageLanguages() []string { return i18n.Languages() }
