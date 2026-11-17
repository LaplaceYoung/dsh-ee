# Anthropic Messages संगतता (POST /v1/messages)

dsh-ee, OpenAI-संगत गेटवे के अलावा, Anthropic Messages प्रारूप में मॉडल कॉल करने का प्रवेश भी देता है: Claude Code, Anthropic SDK
या Anthropic Messages प्रोटोकॉल के अनुसार लिखा गया कोई भी क्लाइंट **dsh-ee के एकीकृत एक्सेस टोकन** से कंपनी में जुड़े मॉडल को सीधे कॉल कर सकता है।
अनुरोध को एक OpenAI chat completions में बदलकर मौजूदा गेटवे को दिया जाता है, इसलिए मॉडल अनुमतियाँ, खाता/टोकन का मासिक कोटा, अपस्ट्रीम फ़ेलओवर,
उपयोग लेखांकन और ऑडिट OpenAI गेटवे के समान ही होते हैं।

**v0 में शामिल**: `POST /v1/messages` का नॉन-स्ट्रीमिंग और स्ट्रीमिंग (SSE) रूप, प्रमाणीकरण, दर सीमा और एरर एनवेलप।
**जो नहीं किया जाता**: `count_tokens`, Batches, Files, पूर्ण beta हेडर, सर्वर-साइड टूल (`web_search` आदि) का निष्पादन।

## 1. Claude Code से जोड़ें

dsh-ee को Anthropic API का base URL मानें, और मॉडल नाम के लिए dsh-ee का **बाहरी मॉडल नाम** उपयोग करें (`GET /v1/models` का लौटाया `id`,
अपस्ट्रीम विक्रेता का नाम नहीं):

```sh
export ANTHROPIC_BASE_URL='https://dsh-ee.example.com'
export ANTHROPIC_API_KEY='ee_at_…'            # 客户端发 x-api-key
# 或者：export ANTHROPIC_AUTH_TOKEN='ee_at_…'   # 客户端发 Authorization: Bearer

# 关键：Claude Code 默认的 claude-* 模型名在 dsh-ee 里不存在，必须显式指定对外模型名
export ANTHROPIC_MODEL='company-chat'
export ANTHROPIC_DEFAULT_HAIKU_MODEL='company-chat'
export ANTHROPIC_DEFAULT_SONNET_MODEL='company-chat'
export ANTHROPIC_DEFAULT_OPUS_MODEL='company-chat'
```

> dsh-ee `claude-*` नामों का मैपिंग **नहीं** करता: मॉडल नाम खाते/टोकन के उपलब्ध मॉडलों में होना चाहिए, अन्यथा 403।
> DeepSeek जैसे विक्रेताओं की Anthropic-संगत परत claude नामों को अपने मॉडलों पर स्वतः मैप कर देती है; dsh-ee के मॉडल व्यवस्थापक द्वारा जोड़े गए
> बहु-विक्रेता मॉडल हैं, जिनका कोई एकल मैपिंग लक्ष्य नहीं है, इसलिए स्पष्ट रूप से बाहरी मॉडल नाम उपयोग करने की आवश्यकता है।

base URL लिखने के दो समतुल्य तरीके हैं (एक ही handler; व्यवहार, प्रमाणीकरण और एरर एनवेलप पूरी तरह समान):

| `ANTHROPIC_BASE_URL` | क्लाइंट का वास्तविक अनुरोध | विवरण |
|---|---|---|
| `https://dsh-ee.example.com` | `POST /v1/messages` | मानक Anthropic base URL लेखन |
| `https://dsh-ee.example.com/anthropic` | `POST /anthropic/v1/messages` | DeepSeek की Anthropic base URL परंपरा (`<base>/anthropic`) से संगत |

curl से सीधे जाँच भी कर सकते हैं:

```sh
curl -fsS "$BASE/v1/messages" \
  -H "x-api-key: $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "company-chat",
    "max_tokens": 256,
    "messages": [{"role": "user", "content": [{"type": "text", "text": "ping"}]}]
  }'

# 同一个端点的别名（DeepSeek 风格 base URL：ANTHROPIC_BASE_URL=$BASE/anthropic）
curl -fsS "$BASE/anthropic/v1/messages" ...  # 参数与上面完全相同
```

## 2. प्रमाणीकरण और अनुमतियाँ

- क्रेडेंशियल: `x-api-key: <टोकन>` या `Authorization: Bearer <टोकन>`, दोनों समतुल्य; दोनों साथ भेजें तो `x-api-key` प्राथमिकता पाता है।
- टोकन: एकीकृत एक्सेस टोकन `ee_at_…` (पुराने `sk-ee-…` / `ee_npm_…` भी स्वीकार्य, व्यवहार OpenAI गेटवे जैसा)।
- अनुमतियाँ: टोकन में `models` scope होना चाहिए, खाते में “बड़े मॉडल को कॉल करें” (`llm.use`) अनुमति होनी चाहिए, और मॉडल खाते व टोकन की उपलब्ध सीमा में होना चाहिए।
- कोटा: खाता/टोकन का मासिक कोटा और कोटा समाप्ति OpenAI गेटवे के साथ साझा है; अस्वीकृत अनुरोध अपस्ट्रीम नहीं भेजे जाते और उनका लेखांकन नहीं होता।
- दर सीमा: `/v1/*` के साथ प्रति-टोकन निश्चित विंडो दर सीमा साझा है (डिफ़ॉल्ट 60 बार प्रति मिनट, `DSH_EE_RATE_LIMIT` से बदलें)।
  केवल `x-api-key` वाले अनुरोध भी टोकन-आधारित गिने जाते हैं, IP-आधारित गिनती पर नहीं लौटते; सीमा पार होने पर भी Anthropic एरर एनवेलप और `Retry-After` लौटता है।
- `anthropic-version` / `anthropic-beta`: DeepSeek नेटिव पासथ्रू मार्ग इन दोनों हेडर को अपस्ट्रीम तक **ज्यों का त्यों पास** करता है (समर्थन DeepSeek तय करता है, dsh-ee पार्स नहीं करता); अन्य provider के फ़ॉलबैक रूपांतरण मार्ग इन्हें अनदेखा करते हैं और त्रुटि नहीं देते।
- **Files API v0 के दायरे में नहीं है**: `anthropic-beta: files-api-2025-04-14` भेजने पर भी dsh-ee Files लागू नहीं करता; फ़ॉलबैक रूपांतरण मार्ग में `image.source.type=file` फ़ाइल संदर्भ 400 लौटाता है (छवियों के लिए केवल `base64` / `url` समर्थित), नेटिव पासथ्रू मार्ग में फ़ाइल/vision ब्लॉक ज्यों के त्यों DeepSeek को जाते हैं; Files / Batches एंडपॉइंट 404 लौटाते हैं।
- सेवा डिस्कवरी: `GET /.well-known/dsh-ee` का `models.anthropic_messages_endpoint` इस एंडपॉइंट का पता देता है,
  `GET /v1/openapi.json` में `POST /v1/messages` के अनुरोध/प्रतिक्रिया/त्रुटि विवरण भी हैं।

## 3. अपस्ट्रीम फ़ॉरवर्डिंग: DeepSeek नेटिव पासथ्रू / अन्य provider का फ़ॉलबैक रूपांतरण

dsh-ee बाहरी मॉडल के कैंडिडेट रूट के अनुसार तय करता है कि यह अनुरोध अपस्ट्रीम तक कैसे पहुँचे (कैंडिडेट क्रम ही `/admin/providers` की प्राथमिकता है):

| कैंडिडेट रूट में पहले की `provider.vendor` | फ़ॉरवर्डिंग तरीका | dsh-ee क्या करता है |
|---|---|---|
| `deepseek` | **नेटिव पासथ्रू**: `POST <provider.base>/anthropic/v1/messages` | केवल प्रमाणीकरण, मॉडल अनुमति, कोटा, दर सीमा और `model` का पुनर्लेखन (बाहरी नाम→अपस्ट्रीम नाम); `anthropic-version` / `anthropic-beta` ज्यों के त्यों पास; अनुरोध बॉडी के शेष फ़ील्ड (server tools, citations, safety, title, latest_reminder, file/vision ब्लॉक आदि) ज्यों के त्यों फ़ॉरवर्ड; सफल प्रतिक्रिया JSON/SSE **बाइट-दर-बाइट फ़ॉरवर्ड**; साइड में usage पार्स करके लेखांकन (usage न हो तो अनुमान); अपस्ट्रीम त्रुटि भी मूल स्थिति/body के साथ ज्यों की त्यों फ़ॉरवर्ड। |
| अन्य | **फ़ॉलबैक रूपांतरण**: Anthropic Messages → OpenAI chat completions | देखें §4 “अनुरोध रूपांतरण” और §5 “प्रतिक्रिया और स्ट्रीमिंग”; `anthropic-version` / `anthropic-beta` अनदेखे; असमर्थित ब्लॉक 400 लौटाते हैं या अनदेखे होते हैं। |

- नेटिव पासथ्रू में dsh-ee `max_tokens` / `temperature` जैसे जनरेशन पैरामीटर **सत्यापित नहीं करता** (DeepSeek को सौंपता है) और कंटेंट ब्लॉक भी पार्स नहीं करता; मासिक कोटा, प्रति-टोकन दर सीमा और मॉडल अनुमतियाँ dsh-ee ही लागू करता है।
- **फ़ेलओवर केवल एक ही प्रोटोकॉल के भीतर**: पहला रूट `deepseek` हो तो केवल अन्य `deepseek` रूटों के बीच स्विच होता है, अन्यथा केवल अन्य vendor के रूपांतरण रूटों के बीच स्विच होता है, **प्रोटोकॉल पार नहीं**; सभी विफल होने पर अंतिम रूट की त्रुटि लौटती है। एक ही बाहरी मॉडल पर कई रूट जुड़े हों तो पहला (उच्चतम प्राथमिकता) प्रोटोकॉल तय करता है।
- नेटिव पासथ्रू `/anthropic/v1/messages` उपनाम भी समर्थित करता है; `x-api-key` / Bearer प्रमाणीकरण रूपांतरण परत के समान ही है।
- डिस्कवरी दस्तावेज़ के `models.anthropic_messages_native_vendors` (डिफ़ॉल्ट `["deepseek"]`) और `models.anthropic_messages_conversion_fallback` (`openai-chat`) इस नीति की घोषणा करते हैं।

## 4. अनुरोध रूपांतरण (फ़ॉलबैक रूपांतरण मार्ग: Anthropic → OpenAI chat)

| Anthropic फ़ील्ड | प्रोसेसिंग |
|---|---|
| `model` | dsh-ee बाहरी मॉडल नाम से अनुमति जाँचता है, फ़ॉरवर्ड करते समय अपस्ट्रीम मॉडल नाम में बदलता है |
| `max_tokens` | अनिवार्य धनात्मक पूर्णांक, प्रति-अनुरोध ऊपरी सीमा OpenAI गेटवे जैसी (32768); सीमा पार होने पर 400 |
| `system` | स्ट्रिंग या टेक्स्ट ब्लॉक सरणी को मिलाकर एक OpenAI `system` संदेश बनता है |
| `messages[].content` | स्ट्रिंग ज्यों की त्यों; कंटेंट ब्लॉक नीचे की तालिका के अनुसार रूपांतरित |
| `temperature` / `top_p` | ज्यों के त्यों पास (`temperature` की सीमा अपस्ट्रीम तय करता है) |
| `top_k` | OpenAI chat में इसका संगत पैरामीटर नहीं है, इसलिए अनदेखा किया जाता है |
| `stop_sequences` | सरणी को OpenAI `stop` में बदला जाता है |
| `tools` | क्लाइंट टूल `{name, description, input_schema}` → `{type:"function", function:{name, description, parameters}}` |
| `tool_choice` | `auto`→`auto`, `any`→`required`, `none`→`none`, `tool`→निर्दिष्ट function; `disable_parallel_tool_use`→`parallel_tool_calls=false` |
| `metadata.user_id` | OpenAI के `user` में बदला जाता है, ताकि अपस्ट्रीम अंतिम उपयोगकर्ता के आधार पर दर सीमा/अलगाव कर सके |
| `thinking` | `enabled` / `adaptive` → OpenAI विस्तारित पैरामीटर `thinking:{"type":"enabled"}`; `disabled` → `thinking:{"type":"disabled"}`; `budget_tokens` अनदेखा |
| `reasoning_effort` / `output_config.effort` | OpenAI `reasoning_effort` में बदला जाता है; `none` → सोच बंद; `xhigh` / `max` / `ultra` → `high` पर सामान्यीकृत (मान `minimal/low/medium/high`) |
| `stream` | स्ट्रीमिंग फ़ॉरवर्ड; लेखांकन के लिए `stream_options.include_usage=true` स्वतः जोड़ा जाता है |
| `container` / `mcp_servers` / `service_tier` / `top_k` / `cache_control` | अनदेखा |

कंटेंट ब्लॉक:

| ब्लॉक प्रकार | स्थान | रूपांतरण |
|---|---|---|
| `text` | user / assistant | टेक्स्ट (एक ही संदेश के कई text ब्लॉक newline से जोड़े जाते हैं) |
| `image` | user | `source.type=base64` / `url` → OpenAI `image_url` कंटेंट सेगमेंट (अपस्ट्रीम में विज़न समर्थन चाहिए) |
| `tool_use` | assistant | → OpenAI `tool_calls` (`input` को `arguments` JSON स्ट्रिंग में क्रमबद्ध) |
| `tool_result` | user | → OpenAI `role:"tool"` संदेश (`tool_use_id` → `tool_call_id`; सामग्री को टेक्स्ट में समतल, छवियों के लिए `[image]` प्लेसहोल्डर) |
| `thinking` | assistant | → उस assistant संदेश का `reasoning_content` (DeepSeek जैसे अपस्ट्रीम tools के साथ अगली बारी में इसे वापस भेजने की अपेक्षा करते हैं) |
| `redacted_thinking` | assistant | टेक्स्ट में पुनर्स्थापित नहीं हो सकता, अनदेखा |
| `document` आदि अन्य ब्लॉक | — | 400 `invalid_request_error` (v0 में फ़ाइल/दस्तावेज़ ब्लॉक समर्थित नहीं) |

सर्वर-साइड टूल (`type` = `web_search_20250305`, `bash_20250124` आदि, जो Anthropic पक्ष पर निष्पादित होते हैं) **अनदेखे** किए जाते हैं
और पूरे अनुरोध को त्रुटि में नहीं बदला जाता — इससे Claude Code की अन्य सुविधाएँ काम करती रहती हैं, पर वेब सर्च/कोड निष्पादन जैसे टूल वास्तव में प्रभावी नहीं होते।

## 5. प्रतिक्रिया और स्ट्रीमिंग (फ़ॉलबैक रूपांतरण मार्ग)

नॉन-स्ट्रीमिंग प्रतिक्रिया मानक Anthropic `message` होती है: `id` (`msg_…`), `type:"message"`, `role:"assistant"`,
`model` (क्लाइंट द्वारा अनुरोधित बाहरी मॉडल नाम), `content` ब्लॉक सरणी, `stop_reason`, `stop_sequence`, `usage`।

- `content` क्रम: `thinking` (जब अपस्ट्रीम `reasoning_content` / `reasoning` लौटाता है) → `text` → `tool_use`।
  thinking ब्लॉक का `signature` dsh-ee द्वारा बनाया गया अपारदर्शी प्लेसहोल्डर मान है (सर्वर सिग्नेचर सत्यापित नहीं करता), क्लाइंट इसे ज्यों का त्यों वापस भेज सकता है।
- `stop_reason`: टूल कॉल हो तो → `tool_use`; `finish_reason=length` → `max_tokens`; `content_filter` → `refusal`;
  अन्य → `end_turn`। OpenAI `stop_sequences` के मिलने और सामान्य समाप्ति में अंतर नहीं कर सकता, इसलिए `stop_sequence` नहीं लौटाता।
- `usage`: `output_tokens` अपस्ट्रीम के `completion_tokens` से; `input_tokens = prompt_tokens - कैश हिट`,
  `cache_read_input_tokens` अपस्ट्रीम के `prompt_tokens_details.cached_tokens` से (यह फ़ील्ड न हो तो 0),
  `cache_creation_input_tokens` स्थायी रूप से 0। **लेखांकन** अभी भी अपस्ट्रीम के `prompt_tokens + completion_tokens` से मासिक कोटा में होता है।

स्ट्रीमिंग `Content-Type: text/event-stream`, इवेंट क्रम (Anthropic आधिकारिक के समान):

```text
event: message_start          # message 骨架，usage.input_tokens 是估算值
event: content_block_start    # 依次出现 thinking / text / tool_use 块
event: content_block_delta    # thinking_delta / text_delta / input_json_delta / signature_delta
event: content_block_stop
event: message_delta          # delta.stop_reason + usage（最终用量，以这里为准）
event: message_stop
```

- OpenAI का `data: [DONE]` नहीं भेजा जाता; कनेक्शन `message_stop` से पहले टूटे तो प्रतिक्रिया अपूर्ण मानी जाती है।
- `message_start.usage.input_tokens` अनुरोध बॉडी का अनुमानित मान है (स्ट्रीम शुरू होते समय अपस्ट्रीम usage ज्ञात नहीं होता);
  `message_delta.usage` के `input_tokens` / `output_tokens` अंतिम मान हैं।
- अपस्ट्रीम स्ट्रीमिंग usage न दे (जैसे क्लाइंट बीच में रुक जाए) तो dsh-ee अनुरोध और अब तक फ़ॉरवर्ड की गई सामग्री के आधार पर लेखांकन का अनुमान लगाता है, ताकि कोटा छूटे नहीं।

## 6. त्रुटियाँ

त्रुटि बॉडी Anthropic आकार की होती है, HTTP स्थिति कोड `error.type` से मेल खाता है:

```json
{"type":"error","error":{"type":"rate_limit_error","message":"请求过于频繁，请稍后重试"}}
```

| HTTP | `error.type` | ट्रिगर |
|---|---|---|
| 400 | `invalid_request_error` | अनुरोध बॉडी JSON नहीं है, `model` / `max_tokens` गायब, पैरामीटर सीमा से बाहर, असमर्थित ब्लॉक प्रकार |
| 401 | `authentication_error` | टोकन अनुपस्थित या अमान्य |
| 403 | `permission_error` | टोकन में `models` scope नहीं, खाते में `llm.use` नहीं, मॉडल दायरे से बाहर, कोटा समाप्त |
| 404 | `not_found_error` | बाहरी मॉडल के लिए कोई उपलब्ध अपस्ट्रीम नहीं; `count_tokens` आदि लागू न किए गए संगतता एंडपॉइंट |
| 413 | `request_too_large` | अनुरोध बॉडी 16 MiB से अधिक |
| 429 | `rate_limit_error` | खाता/टोकन का मासिक कोटा समाप्त, प्रति-टोकन अनुरोध दर सीमा, अपस्ट्रीम ने 429 लौटाया |
| 5xx | `api_error` / `overloaded_error` | सभी अपस्ट्रीम रूट अनुपलब्ध, अपस्ट्रीम 5xx, अपस्ट्रीम प्रतिक्रिया पार्स नहीं हो सकी |

`/v1/messages/count_tokens` पहले प्रमाणीकरण करता है, फिर 404 `not_found_error` लौटाता है (v0 में लागू नहीं),
`/v1/` के फ़ॉलबैक OpenAI एरर एनवेलप पर नहीं जाता।

DeepSeek नेटिव पासथ्रू मार्ग में अपस्ट्रीम द्वारा लौटाई गई त्रुटि (DeepSeek स्वयं Anthropic एरर एनवेलप देता है) **मूल स्थिति कोड और body** के साथ
फ़ॉरवर्ड होती है, दोबारा पैक नहीं की जाती; dsh-ee की अपनी त्रुटियाँ (प्रमाणीकरण/अनुमति/कोटा/दर सीमा/मॉडल के लिए अपस्ट्रीम नहीं/अपस्ट्रीम अगम्य) ऊपर की तालिका के अनुसार ही होती हैं।

## 7. ज्ञात सीमाएँ (v0)

- केवल `POST /v1/messages` (और उपनाम `POST /anthropic/v1/messages`) लागू है; `count_tokens` (उपनाम सहित),
  Batches, Files, `/v1/messages/batches` आदि 404 लौटाते हैं।
- सर्वर-साइड टूल निष्पादित नहीं होते (देखें §3); `container`, `mcp_servers`, `service_tier`, `top_k` अनदेखे होते हैं।
- `claude-*` मॉडल नामों का मैपिंग नहीं होता: क्लाइंट को मॉडल नाम `/v1/models` वाले बाहरी मॉडल नाम के रूप में ही कॉन्फ़िगर करना होगा।
- thinking सामग्री अपस्ट्रीम के `reasoning_content` / `reasoning` फ़ील्ड से पहचानी जाती है; अपस्ट्रीम सोच सामग्री न लौटाए तो
  प्रतिक्रिया में `thinking` ब्लॉक नहीं होता। `budget_tokens` प्रभावी नहीं होता, सोच की तीव्रता `reasoning_effort` / `output_config.effort` से नियंत्रित करें।
- छवियाँ OpenAI के `image_url` कंटेंट सेगमेंट में बदली जाती हैं, अपस्ट्रीम मॉडल छवियाँ समर्थित करता है या नहीं यह अपस्ट्रीम तय करता है; `document` आदि फ़ाइल ब्लॉक समर्थित नहीं।
- `stop_sequences` का मिलना और सामान्य समाप्ति, दोनों अपस्ट्रीम में `finish_reason=stop` होते हैं और अंतर नहीं किया जा सकता, इसलिए एकरूप से `end_turn` लौटता है।
- सिग्नेचर सत्यापित नहीं होते: thinking ब्लॉक का `signature` प्लेसहोल्डर मान है, सर्वर न सत्यापन करता है न उस पर निर्भर है।
- DeepSeek नेटिव पासथ्रू मार्ग में dsh-ee जनरेशन पैरामीटर सत्यापन और कंटेंट ब्लॉक पार्सिंग नहीं करता, `max_tokens` आदि की सीमा DeepSeek तय करता है;
  अपस्ट्रीम त्रुटियाँ ज्यों की त्यों फ़ॉरवर्ड होती हैं; नेटिव मार्ग की प्रतिक्रिया dsh-ee की Anthropic रूपांतरण परत से नहीं गुज़रती।
- फ़ेलओवर प्रोटोकॉल पार नहीं करता: उच्च प्राथमिकता वाला रूट प्रोटोकॉल तय करने के बाद, कम प्राथमिकता वाले दूसरे प्रोटोकॉल का रूट आज़माया नहीं जाता।

## 8. संबंधित

- OpenAI-संगत गेटवे, टोकन और scope: [Harness इंटीग्रेशन (docs/dev/harness.md)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/harness.md)
- तृतीय-पक्ष क्लाइंट का पूर्ण मैनुअल और त्रुटि तालिका: [l1-client.md](https://github.com/hikariming/dsh-ee/blob/main/docs/l1-client.md)
- मॉडल गेटवे एरर कोड सूचकांक: [errors.md](https://github.com/hikariming/dsh-ee/blob/main/docs/errors.md)
- कार्यान्वयन: `server/internal/api/anthropic.go`, `server/internal/api/anthropic_convert.go`
