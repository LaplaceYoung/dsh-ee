// बिल्ट-इन npm स्रोत जुड़ने के बाद के टेक्स्ट: स्वतः जाँच, समीक्षा, क्षमता प्रबंधन, टोकन, प्रीलोड कार्य।
import type zhNs from "../zh/registry";
import type { Messages } from "../types";

export default {
  "registry": {
    "settingsSaveFailed": "सेटिंग्स सहेजी नहीं गईं: {{msg}}",
    "scan": {
      "none": "कोई स्वतः जाँच परिणाम नहीं।",
      "bundle": {
        "label": "DSH बंडल",
        "pass": "dsh.bundle घोषित है, बंडल मेनिफ़ेस्ट {{path}} पैकेज में है",
        "missing": "dsh.bundle घोषित नहीं: यह सामान्य npm पैकेज है, क्षमता बाज़ार और DSH इंडेक्स में नहीं दिखेगा",
        "no-patch": "dsh.bundle घोषित है, पर patch फ़ील्ड नहीं है",
        "unsafe": "dsh.bundle.patch सुरक्षित सापेक्ष पाथ नहीं है: {{path}}",
        "file-missing": "dsh.bundle.patch द्वारा इंगित {{path}} पैकेज में नहीं है (क्या build नहीं हुआ या files में छूट गया?)"
      },
      "patch": {
        "label": "बंडल मेनिफ़ेस्ट सामग्री",
        "pass": "{{path}} की संरचना सही है ({{rows}} पंक्तियाँ)",
        "warn": "{{path}}: {{reason}}",
        "fail": "{{path}}: {{reason}}",
        "reason": {
          "yaml": "मान्य YAML नहीं ({{error}})",
          "not-array": "शीर्ष स्तर सरणी होना चाहिए",
          "empty": "कोई patch पंक्ति नहीं",
          "row-not-object": "पंक्ति {{row}} ऑब्जेक्ट नहीं है",
          "insert-not-array": "पंक्ति {{row}} का insert सरणी होना चाहिए",
          "missing-id": "पंक्ति {{row}} में row id नहीं है",
          "duplicate-id": "row id {{id}} दोहराया गया (पंक्ति {{row}} और पंक्ति {{first}})",
          "missing-name": "पंक्ति {{row}} के इंसर्ट आइटम में name नहीं है",
          "name-mismatch": "किसी भी पंक्ति में इस पैकेज का नाम {{name}} संदर्भित नहीं है; DSH प्लगइन नहीं ढूँढ पाएगा",
          "config-not-map": "{{name}} का config मैपिंग नहीं है",
          "unreadable": "patch फ़ाइल बहुत बड़ी है या पढ़ी नहीं जा सकी"
        }
      },
      "skills": {
        "label": "पैकेज में स्किल",
        "pass": "पैकेज में कोई SKILL.md नहीं",
        "warn": "पैकेज में {{count}} स्किल हैं: {{files}}। ये प्लगइन रनटाइम से इंजेक्ट होते हैं, क्षमता केंद्र के स्किल टैब में नहीं दिखते, और अलग से अनुमति या अप्रकाशित नहीं किए जा सकते।"
      },
      "cordis": {
        "label": "Cordis डिपेंडेंसी",
        "warn": "पैकेज ने {{packages}} को dependencies में रखा है: DSH Cordis प्रदान करता है, दोहरा होने से सेवा इंजेक्शन और जीवनचक्र गड़बड़ा जाते हैं; इन्हें peerDependencies में ले जाएँ (केवल build के लिए devDependencies में रख सकते हैं)।"
      },
      "cordis-peer": {
        "label": "Cordis वर्ज़न",
        "warn": "peerDependencies की Cordis सीमा {{range}} में वह {{want}} शामिल नहीं जो वर्तमान DSH 0.2.2 उपयोग करता है; यदि पुराने होस्ट के साथ संगतता भी चाहिए तो सीमा जाँच लें।"
      },
      "install-scripts": {
        "label": "इंस्टॉल स्क्रिप्ट",
        "pass": "इंस्टॉल के समय कोई स्क्रिप्ट नहीं चलती",
        "warn": "इंस्टॉल के समय {{scripts}} चलेंगी: पुष्टि करें कि वे इंस्टॉल के दौरान बाहरी कोड डाउनलोड या निष्पादित नहीं करतीं"
      },
      "secrets": {
        "label": "कुंजी लीक",
        "pass": "कोई संदिग्ध कुंजी या संवेदनशील फ़ाइल नहीं मिली",
        "fail": "{{count}} संदिग्ध कुंजी या संवेदनशील फ़ाइल मिलीं: {{files}}"
      },
      "license": {
        "label": "License",
        "pass": "{{license}}",
        "warn": "license घोषित नहीं"
      },
      "engines": {
        "label": "DSH वर्ज़न",
        "pass": "engines.dsh: {{range}}",
        "warn": "engines.dsh घोषित नहीं, DSH peer डिपेंडेंसी से संगतता तय करेगा"
      },
      "dependencies": {
        "label": "समुदाय डिपेंडेंसी",
        "pass": "{{count}}: {{names}}"
      },
      "size": {
        "label": "आकार",
        "pass": "{{size}} (अनपैक {{unpacked}}, {{files}} फ़ाइलें)",
        "warn": "{{size}} (अनपैक {{unpacked}}, {{files}} फ़ाइलें), कुछ बड़ा है"
      },
      "manifest-mismatch": {
        "label": "मेनिफ़ेस्ट संगति",
        "fail": "tarball के अंदर package.json में {{inner}} लिखा है, जो मेटाडेटा से मेल नहीं खाता"
      }
    },
    "review": {
      "requested": "कर्मचारी अनुरोध",
      "publishedBy": "{{name}} · {{department}} द्वारा प्रकाशित",
      "requestedBy": "{{name}} ने लाने का अनुरोध किया",
      "previous": "वर्तमान प्रकाशित v{{version}}",
      "firstVersion": "पहला प्रकाशन",
      "dependencies": "dependencies",
      "peers": "peerDependencies",
      "engines": "engines",
      "scripts": "scripts",
      "downloadTarball": "tarball डाउनलोड करके देखें"
    },
    "plugins": {
      "empty": "अभी कोई क्षमता नहीं। कर्मचारी प्रकाशन, अपस्ट्रीम प्रीलोड या ऑफ़लाइन आयात के बाद यहाँ दिखेंगी।",
      "versions": "वर्ज़न",
      "pendingVersions": "{{count}} वर्ज़न समीक्षा हेतु लंबित",
      "yankAll": "{{name}} के सभी वर्ज़न अप्रकाशित करें, कर्मचारी फिर इंस्टॉल नहीं कर पाएँगे:",
      "yankVersion": "{{name}} अप्रकाशित करें:",
      "reasonPlaceholder": "अप्रकाशन का कारण (आवश्यक, वर्ज़न पर दर्ज होगा)",
      "publisher": "प्रकाशक / अनुरोधकर्ता",
      "created": "सबमिट समय",
      "note": "समीक्षा रिकॉर्ड",
      "reviewedBy": "{{name}} ने समीक्षा की"
    },
    "detail": {
      "deprecated": "अप्रचलित: {{msg}}",
      "deprecatedShort": "अप्रचलित",
      "npmInstall": "प्रोजेक्ट में pnpm / npm से इंस्टॉल करें",
      "engines": "DSH {{range}} आवश्यक; कंपनी के लिए अनुशंसित वर्ज़न {{version}}।",
      "readme": "विवरण",
      "versions": "प्रकाशित वर्ज़न",
      "size": "tarball आकार",
      "homepage": "होमपेज"
    },
    "setup": {
      "tokenPlaceholder": "<आपका टोकन>",
      "tokenCount": "{{count}}",
      "prefix": "प्रीफ़िक्स",
      "never": "कभी नहीं",
      "tokenNamePlaceholder": "जैसे “मेरा लैपटॉप”",
      "tokenOnce": "पूरा टोकन केवल इस बार दिखेगा, नीचे की कमांड पहले ही अपने आप भर दी गई है:",
      "revokeConfirm": "“{{name}}” रद्द करने के बाद, उसका उपयोग करने वाला कंप्यूटर तुरंत कंपनी स्रोत से इंस्टॉल नहीं कर पाएगा। पक्का है?",
      "scriptNpmrc": "registry, कंपनी स्कोप और आपका टोकन DSH प्रोफ़ाइल डायरेक्टरी के .npmrc में लिखें (केवल DSH पर लागू, ~/.npmrc नहीं बदलता; वास्तव में ग्लोबल लागू करना हो तो चलाने से पहले DSH_EE_GLOBAL_NPMRC=1 सेट करें)",
      "scriptIndex": "DSH प्लगइन पेज → Discover में इंडेक्स स्रोत जोड़ने का संकेत:",
      "caDesc": "जब dsh-ee कंपनी के स्व-हस्ताक्षरित प्रमाणपत्र का उपयोग करता है, तो एडमिन से CA प्रमाणपत्र लें (~/.dsh-ee-ca.pem के रूप में सहेजें), फिर चलाएँ:"
    },
    "publish": {
      "cliDesc": "पहले <link>कनेक्शन गाइड</link> में “प्रकाशन योग्य” टोकन बनाएँ और ~/.npmrc में लिखें, फिर प्लगइन डायरेक्टरी में चलाएँ:",
      "fileHint": "npm pack से बना .tgz; पैकेज नाम, वर्ज़न, प्रदर्शित नाम, श्रेणी सब package.json से पढ़े जाते हैं",
      "tag": "dist-tag",
      "tagHint": "डिफ़ॉल्ट latest; पूर्वावलोकन वर्ज़न के लिए beta",
      "uploading": "अपलोड हो रहा है…",
      "publishedNow": "{{name}} प्रकाशित और सूचीबद्ध हो गया",
      "resultPending": "{{name}} सबमिट हो गया, एडमिन समीक्षा स्वीकृति के बाद सहकर्मी इंस्टॉल कर सकेंगे।",
      "resultPublished": "{{name}} प्रकाशित हो गया।",
      "note": "समीक्षा टिप्पणी: {{note}}",
      "manifestTitle": "package.json में स्पष्ट लिखें",
      "manifestDesc": "dsh.bundle घोषित करता है कि यह DSH प्लगइन है; वैकल्पिक dsh-ee फ़ील्ड क्षमता बाज़ार में प्रदर्शित नाम, श्रेणी (tools, agent, memory…) और रूप (skill / connector / knowledge / plugin) तय करता है।",
      "exampleDesc": "एक वाक्य में बताएँ यह क्या कर सकता है",
      "exampleTitle": "मेरा प्लगइन"
    },
    "admin": {
      "checkOk": "अपस्ट्रीम {{upstream}} कनेक्शन सामान्य ({{ms}} ms)",
      "checkFail": "अपस्ट्रीम कनेक्शन विफल: {{msg}}",
      "upstreamPlugins": "इनमें {{count}} प्लगइन, बाकी डिपेंडेंसी",
      "versions": "{{count}} वर्ज़न",
      "downloads30d": "पिछले 30 दिन डाउनलोड",
      "pending": "{{count}} वर्ज़न समीक्षा हेतु लंबित",
      "indexUrl": "DSH प्लगइन इंडेक्स स्रोत",
      "tokensCount": "{{count}} वैध टोकन",
      "noTokens": "अभी किसी ने टोकन नहीं बनाया।",
      "revokeConfirm": "{{user}} का टोकन “{{name}}” रद्द करें? उसका उपयोग करने वाला कंप्यूटर तुरंत इंस्टॉल नहीं कर पाएगा।",
      "tokensHint": "टोकन व्यक्तिगत होते हैं: कर्मचारी <link>कनेक्शन गाइड</link> में खुद बनाते हैं; CI जैसी सेवाओं के लिए अलग समर्पित खाता बनाकर टोकन बनाएँ।"
    },
    "upstream": {
      "publishDirectly": "सिंक के बाद सीधे प्रकाशित करें",
      "publishDirectlyHint": "समीक्षा छोड़ें: प्लगइन तुरंत क्षमता बाज़ार और DSH इंडेक्स में दिखेगा।",
      "checkUpdates": "अभी नए वर्ज़न जाँचें",
      "noUpdates": "प्रीलोड किए गए प्लगइन सभी नवीनतम वर्ज़न हैं",
      "closureNote": "प्रीलोड के साथ डिपेंडेंसी क्लोज़र (dependencies, optionalDependencies और आवश्यक peerDependencies, जिसमें @deepseek-ai/dsh-* कोर पैकेज शामिल हैं) भी मिरर किया जाता है, ऑफ़लाइन भी इंस्टॉल हो सकता है।",
      "feedCount": "dshfind निर्देशिका {{count}} प्लगइन · {{at}} पर अपडेट",
      "refreshFeed": "निर्देशिका रीफ़्रेश करें",
      "feedDisabled": "ऑफ़लाइन मोड में हैं या सिस्टम सेटिंग्स में dshfind निर्देशिका बंद है: ऑफ़लाइन आयात उपयोग करें, या कर्मचारियों से निर्दिष्ट पैकेज लाने का अनुरोध करें।",
      "feedError": "dshfind निर्देशिका नहीं मिली: {{msg}}",
      "newer": "अपस्ट्रीम {{version}}",
      "importDesc": "इंटरनेट वाले dsh-ee (जैसे DMZ इंस्टेंस) से निर्यात किया गया ऑफ़लाइन पैकेज आयात करें। हर tarball का sha512 फिर से गणना होता है, मेनिफ़ेस्ट से न मेल खाने पर अस्वीकार; प्लगइन वर्ज़न समीक्षा हेतु लंबित में जाते हैं।",
      "importing": "आयात हो रहा है…",
      "importResult": "{{added}} नए वर्ज़न जोड़े गए ({{pending}} समीक्षा हेतु लंबित), {{existing}} पहले से मौजूद, {{skipped}} पैकेज छोड़े गए।",
      "exportAllHint": "खाली छोड़ने पर सभी प्रकाशित अपस्ट्रीम पैकेज निर्यात होंगे"
    },
    "jobs": {
      "kind": {
        "preload": "अपस्ट्रीम प्रीलोड",
        "request": "कर्मचारी लाने का अनुरोध",
        "update": "नया वर्ज़न सिंक",
        "import": "ऑफ़लाइन आयात"
      },
      "status": {
        "running": "चल रहा है",
        "succeeded": "पूरा",
        "partial": "आंशिक विफल",
        "failed": "विफल",
        "interrupted": "बाधित"
      },
      "colKind": "कार्य",
      "colCount": "सफल / कुल",
      "colStarted": "प्रारंभ समय",
      "viewLog": "लॉग देखें"
    }
  }
} satisfies Messages<typeof zhNs>;
