# dsh-ee एक्सटेंशन लिखें: मैनिफ़ेस्ट, जाँच और कॉल

## चार रूप

dsh-ee एक्सटेंशन कंपनी के सिस्टमों को जोड़ता है। पहले रूप चुनें: पहले तीन मैनिफ़ेस्ट के माध्यम से डिलीवर होते हैं, चौथा dsh-ee द्वारा स्वयं लागू किया गया आधिकारिक इंटीग्रेशन है, जिसके लिए लेखक को मैनिफ़ेस्ट लिखने की ज़रूरत नहीं होती।

| रूप | मैनिफ़ेस्ट `kind` | कहाँ चलता है | डिलीवरेबल | dsh-ee इसे कैसे कॉल करता है |
| --- | --- | --- | --- | --- |
| डिक्लेरेटिव | `declarative` | dsh-ee प्रक्रिया के भीतर से HTTP | मैनिफ़ेस्ट + OpenAPI दस्तावेज़ (zip) | OpenAPI से टूल बनाकर सीधे स्रोत सिस्टम पर अनुरोध भेजता है |
| सर्विस | `service` | कंपनी द्वारा स्वयं तैनात HTTP सेवा | एक सेवा जो मैनिफ़ेस्ट और तय इंटरफ़ेस लागू करती है | `/.well-known/dsh-ee-extension.json` लेता है; टूल `POST /mcp` से |
| लोकल | `local` | कर्मचारी के कंप्यूटर पर npm पैकेज | npm पैकेज + मैनिफ़ेस्ट | एजेंट स्थानीय MCP से सीधे जुड़ता है; राइट ऑपरेशन से पहले dsh-ee से पुष्टि, कॉल के बाद सारांश रिपोर्ट |
| इनबिल्ट | (मैनिफ़ेस्ट नहीं) | dsh-ee प्रक्रिया के भीतर Go | dsh-ee के साथ अपग्रेड | सर्विस एक्सटेंशन के समान आंतरिक इंटरफ़ेस, नेटवर्क से नहीं गुज़रता (जैसे Feishu दस्तावेज़ ज्ञान स्रोत) |

मैनिफ़ेस्ट schema केवल `declarative` / `service` / `local` तीन `kind` ही स्वीकार करता है; “इनबिल्ट” dsh-ee स्वयं लागू करता है और मैनिफ़ेस्ट से डिलीवर नहीं हो सकता। सर्वर तृतीय-पक्ष कोड नहीं चलाता: डिक्लेरेटिव में dsh-ee अनुरोध भेजता है, सर्विस एक्सटेंशन कंपनी की अपनी सेवा में रहता है, लोकल कर्मचारी के कंप्यूटर पर।

## एक स्कैफ़ोल्ड बनाएँ

```bash
# 声明式：不写代码，有 OpenAPI 就能接
dsh-ee extension init ./my-ext --id acme/my-ext --kind declarative

# 服务式：要写逻辑，企业自己部署
dsh-ee extension init ./my-service --id acme/my-service --kind service
```

`init` में पहचान `--id लेखक/नाम` से देना अनिवार्य है; `--kind` में `declarative` (डिफ़ॉल्ट) या `service` चुन सकते हैं। लोकल एक्सटेंशन रिपॉज़िटरी के [`extensions/official/legacy-local`](https://github.com/hikariming/dsh-ee/blob/main/extensions/official/legacy-local) टेम्पलेट या उदाहरण से कॉपी करें। बनाई गई निर्देशिका में `AGENTS.md` होता है, एजेंट से खोलते ही अगला कदम पता चल जाता है।

| कमांड | उपयोग | इस मशीन पर क्या करता है |
| --- | --- | --- |
| `dsh-ee extension init` | उदाहरण से एक एक्सटेंशन कॉपी करना | फ़ाइलें लिखता है |
| `dsh-ee schema list` / `schema get manifest` | ऑफ़लाइन मैनिफ़ेस्ट, एरर कोड आदि विनिर्देश देखना | केवल पढ़ता है |
| `dsh-ee extension check` | मैनिफ़ेस्ट, इंटरफ़ेस दस्तावेज़, read-only मार्क और क्रेडेंशियल लेखन की स्टैटिक जाँच | केवल पढ़कर जाँच, कोड निष्पादित नहीं करता |
| `dsh-ee extension test` | सर्विस एक्सटेंशन चलाकर प्रोटोकॉल के अनुसार एक-एक करके परीक्षण | स्थानीय प्रक्रिया शुरू करता है |
| `dsh-ee extension dev` | एक नकली dsh-ee शुरू करना (डेटाबेस आवश्यक नहीं) | स्थानीय प्रक्रिया शुरू करता है |
| `dsh-ee extension call` | किसी कर्मचारी की पहचान से एक बार एक्सटेंशन टूल कॉल करना | केवल पढ़कर निदान |
| `dsh-ee extension pack` | **डिक्लेरेटिव** एक्सटेंशन को zip में पैक करके चेकसम निकालना | zip लिखता है |
| `dsh-ee extension report` | व्यवस्थापक के लिए डिलीवरी विवरण बनाना | फ़ाइल लिखता है |

`check` में त्रुटि होने पर निकास कोड 1 होता है, हर परिणाम में `code`, `severity`, `where`, `message`, `fix` होते हैं। उपरोक्त सभी कमांड लेखक की मशीन या लेखक रेपो CI में चलते हैं; dsh-ee सर्वर तृतीय-पक्ष कोड निष्पादित नहीं करता और वेब पेज में चेकर भी नहीं चलाता।

## मैनिफ़ेस्ट का न्यूनतम उदाहरण

नीचे दिया मैनिफ़ेस्ट `dsh-ee extension init --kind declarative` से बनने वाले आकार जैसा है; इसमें `servers` को कवर करने वाला एक `openapi.yaml` जोड़ने पर `dsh-ee extension check` का परिणाम 0 त्रुटि, 0 चेतावनी होता है। फ़ील्ड के लिए `dsh-ee schema get manifest` मानक है; मैनिफ़ेस्ट सख़्त schema है, विनिर्देश में न होने वाला एक अतिरिक्त फ़ील्ड भी त्रुटि देता है।

```json
{
  "id": "acme/oa",
  "version": "0.1.0",
  "name": "Acme OA",
  "description": "查询和提交 Acme OA 的审批单。",
  "kind": "declarative",
  "openapi": "openapi.yaml",
  "servers": ["https://oa.acme.internal"],
  "credential": { "mode": "ee_jwt" },
  "tools": [
    {
      "name": "list_items",
      "readOnly": true,
      "description": "列出当前员工的条目",
      "keep": ["items.id", "items.title", "total"]
    },
    {
      "name": "create_item",
      "description": "新建一个条目。这是写操作，调用前要员工确认。"
    }
  ]
}
```

कुछ बातें जिनमें गलती आसानी से होती है:

- `id` को “लेखक/नाम” रूप में लिखें, केवल छोटे अक्षर, अंक और हाइफ़न; `version` सख़्त सेमांटिक संस्करण है।
- डिक्लेरेटिव में `openapi` (एक्सटेंशन निर्देशिका के भीतर सापेक्ष पथ) और कम से कम एक `servers` होना अनिवार्य है; OpenAPI दस्तावेज़ का `servers[0].url` मैनिफ़ेस्ट के `servers` के भीतर होना चाहिए।
- `tools[].name` OpenAPI के हर इंटरफ़ेस के `operationId` से मेल खाता है; क्वेरी-प्रकार के टूल पर `readOnly` लगाएँ, राइट ऑपरेशन पर न लगाएँ।
- क्रेडेंशियल मोड (`credential.mode`) केवल कॉल के समय आवश्यक है: `shared` / `user_secret` / `ee_jwt` / `oauth`, सामने वाले पक्ष द्वारा स्वीकार्य तरीका चुनें।

## डिलीवरी से पहले पुष्टि करें

| जाँच मद | कहाँ पुष्टि करें |
| --- | --- |
| मैनिफ़ेस्ट फ़ील्ड और प्रोटोकॉल संस्करण | `dsh-ee schema get manifest`, `dsh-ee extension check --json` |
| इंटरफ़ेस दस्तावेज़ और टूल मैपिंग | `check` का `tools` परिणाम; राइट ऑपरेशन पर `readOnly` लगा है या नहीं |
| क्रेडेंशियल केवल dsh-ee से लें, मैनिफ़ेस्ट में न लिखें | मैनिफ़ेस्ट और कोड में स्पष्ट key न दिखे |
| सर्विस इंटरफ़ेस की प्रोटोकॉल संगति | `dsh-ee extension test` के केस |
| डिक्लेरेटिव पैकेजिंग | `dsh-ee extension pack` से zip बनता है; सर्विस एक्सटेंशन कोड रेपो या इमेज डिलीवर करे |
| कौन-कौन से टूल कौन उपयोग कर सकता है | व्यवस्थापक “कंपनी कनेक्शन” और “फ़ील्ड और एक्सेस नियम” में कॉन्फ़िगर करता है |

## DSH स्किल से संबंध

एजेंट को निष्पादन योग्य निर्देश देने के लिए dsh-ee ने एक्सटेंशन विकास प्रक्रिया को स्किल (`dsh-ee-extension-dev`) के रूप में लिखा है और “एक्सटेंशन विकसित करें” (`extensions.develop`) अनुमति के अनुसार DSH को देता है। मनुष्य और एजेंट को एक ही कदम दिखाई देते हैं; पेज मनुष्य के पढ़ने के लिए है, स्किल एजेंट के निष्पादन के लिए।

## रिपॉज़िटरी में पूरा मूल पाठ

साइट का यह पेज केवल परिचय है; प्रोटोकॉल का विवरण, इंस्टॉल समीक्षा और एरर कोड रिपॉज़िटरी के दस्तावेज़ों के अनुसार मान्य हैं (लिंक मर्ज होने के बाद के दस्तावेज़ों की ओर इशारा करते हैं):

- [एक्सटेंशन विकास (docs/dev/extensions.md, मर्ज होने के बाद उपलब्ध)](https://github.com/hikariming/dsh-ee/blob/main/docs/dev/extensions.md)
- [एक्सटेंशन प्रोटोकॉल v1 का अवलोकन (docs/extension-protocol/README.md, मर्ज होने के बाद उपलब्ध)](https://github.com/hikariming/dsh-ee/blob/main/docs/extension-protocol/README.md)
- [एक्सटेंशन प्रोटोकॉल का मूल पाठ (docs/extension-protocol.md)](https://github.com/hikariming/dsh-ee/blob/main/docs/extension-protocol.md)
- [कंपनी प्रकाशन, समीक्षा और अनुमतियाँ (docs/enterprise-publishing.md, मर्ज होने के बाद उपलब्ध)](https://github.com/hikariming/dsh-ee/blob/main/docs/enterprise-publishing.md)
- [एक्सटेंशन निर्देशिका और उदाहरण सूचकांक (extensions/README.md)](https://github.com/hikariming/dsh-ee/blob/main/extensions/README.md)
- [मैनिफ़ेस्ट JSON Schema (extensions/schemas/manifest.schema.json)](https://github.com/hikariming/dsh-ee/blob/main/extensions/schemas/manifest.schema.json)
- [डिज़ाइन विकल्प (docs/plugins.md, डिज़ाइन ड्राफ़्ट, उपयोग दस्तावेज़ नहीं)](https://github.com/hikariming/dsh-ee/blob/main/docs/plugins.md)
