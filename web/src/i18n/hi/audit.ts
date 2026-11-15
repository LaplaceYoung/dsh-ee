// ऑडिट लॉग पेज और कार्रवाई के नाम। कार्रवाई के नाम सर्वर की action (जैसे auth.login) के अनुसार नेस्टेड हैं।
import type zhNs from "../zh/audit";
import type { Messages } from "../types";

export default {
  "auditLog": {
    "title": "ऑडिट लॉग",
    "desc": "साइन-इन, खाते और अनुमतियाँ, मॉडल कनेक्शन, सिस्टम सेटिंग्स, प्रकाशन और समीक्षा आदि कार्रवाइयाँ। रिकॉर्ड केवल जुड़ते हैं, बदले नहीं जाते।",
    "search": "कार्रवाई करने वाला या लक्ष्य खोजें…",
    "allCategories": "सभी श्रेणियाँ",
    "categories": {
      "auth": "साइन-इन",
      "user": "उपयोगकर्ता",
      "role": "भूमिकाएँ",
      "permission": "अनुमति बिंदु",
      "menu": "मेन्यू",
      "provider": "मॉडल अपस्ट्रीम",
      "model": "मॉडल",
      "files": "DeepSeek files",
      "key": "API कुंजी",
      "settings": "सिस्टम सेटिंग्स",
      "ldap": "LDAP",
      "oidc": "एंटरप्राइज़ एकल साइन-ऑन",
      "package": "क्षमता",
      "review": "समीक्षा",
      "upstream": "अपस्ट्रीम प्रीलोड",
      "npm_token": "npm टोकन",
      "token": "एकीकृत टोकन",
      "device": "डिवाइस साइन-इन",
      "asset": "क्षमता केंद्र",
      "connection": "एंटरप्राइज़ कनेक्शन",
      "extension": "एक्सटेंशन",
      "connect": "DSH कनेक्ट प्लगइन",
      "dsh_release": "DSH डेस्कटॉप वितरण",
      "embed": "क्रॉस-साइट एम्बेडिंग",
      "brand": "एंटरप्राइज़ ब्रांड",
      "kb": "ज्ञान आधार",
      "app_data": "ऐप डेटा",
      "data": "डेटा हब",
      "directory": "एंटरप्राइज़ निर्देशिका",
      "client": "API क्लाइंट"
    },
    "colTime": "समय",
    "colActor": "कार्रवाई करने वाला",
    "colAction": "कार्रवाई",
    "colTarget": "लक्ष्य",
    "colDetail": "विवरण",
    "colIp": "IP",
    "colSource": "स्रोत",
    "allSources": "सभी स्रोत",
    "sources": {
      "web": "वेब पेज",
      "token": "टोकन",
      "agent": "Agent",
      "system": "सिस्टम",
      "client_credentials": "क्लाइंट क्रेडेंशियल"
    },
    "system": "सिस्टम",
    "anonymous": "गुमनाम",
    "loadMore": "और लोड करें",
    "noMore": "और रिकॉर्ड नहीं हैं",
    "empty": "कोई मेल खाता रिकॉर्ड नहीं।",
    "actions": {
      "auth": {
        "login": "साइन इन",
        "login_failed": "साइन-इन विफल",
        "password_change": "पासवर्ड बदला"
      },
      "user": {
        "create": "उपयोगकर्ता बनाया",
        "update": "उपयोगकर्ता बदला",
        "roles": "भूमिकाएँ सौंपीं",
        "quota": "कोटा समायोजित किया",
        "create_bot": "बॉट बनाया"
      },
      "role": {
        "create": "भूमिका बनाई",
        "update": "भूमिका बदली",
        "delete": "भूमिका हटाई"
      },
      "permission": {
        "create": "अनुमति बिंदु बनाया",
        "delete": "अनुमति बिंदु हटाया"
      },
      "menu": {
        "change": "मेन्यू बदला"
      },
      "provider": {
        "create": "अपस्ट्रीम जोड़ा",
        "update": "अपस्ट्रीम बदला",
        "delete": "अपस्ट्रीम हटाया"
      },
      "model": {
        "change": "मॉडल बदला"
      },
      "key": {
        "create": "कुंजी बनाई",
        "revoke": "कुंजी निरस्त की"
      },
      "settings": {
        "update": "सेटिंग बदली"
      },
      "ldap": {
        "update": "LDAP कॉन्फ़िगरेशन बदला",
        "sync": "निर्देशिका सिंक"
      },
      "package": {
        "publish": "प्रकाशित किया",
        "status": "उपलब्धता बदली",
        "edit": "प्रदर्शन जानकारी बदली",
        "import_request": "समावेश का अनुरोध किया"
      },
      "review": {
        "approve": "समीक्षा स्वीकृत की",
        "reject": "अस्वीकृत किया"
      },
      "upstream": {
        "sync": "प्रीलोड किया",
        "import": "ऑफ़लाइन पैकेज आयात किया",
        "export": "ऑफ़लाइन पैकेज निर्यात किया"
      },
      "npm_token": {
        "create": "npm टोकन बनाया",
        "revoke": "npm टोकन निरस्त किया"
      },
      "token": {
        "create": "टोकन बनाया",
        "issue": "टोकन जारी किया",
        "revoke": "टोकन निरस्त किया",
        "revoke_bulk": "टोकन सामूहिक रूप से निरस्त किए",
        "elevate": "प्रशासन मोड शुरू किया",
        "elevate_end": "प्रशासन मोड समाप्त किया",
        "elevate_expire": "प्रशासन मोड समाप्त हो गया",
        "refresh": "टोकन रीफ़्रेश किया",
        "refresh_reuse": "रिफ़्रेश टोकन का पुनः उपयोग पाया गया (पूरा परिवार निरस्त)"
      },
      "device": {
        "approve": "डिवाइस अधिकृत किया",
        "deny": "डिवाइस अस्वीकृत किया"
      },
      "asset": {
        "create": "क्षमता बनाई",
        "version": "नया संस्करण सबमिट किया",
        "edit": "दायरा या उपलब्धता बदली",
        "delete": "क्षमता हटाई",
        "scope_request": "उपलब्ध दायरा बढ़ाने का अनुरोध किया"
      },
      "hub": {
        "category": "Changed a capability category",
        "block": "Changed a block policy"
      },
      "connection": {
        "tool": "कनेक्शन टूल सेट किया",
        "request": "एंटरप्राइज़ कनेक्शन के उपयोग का अनुरोध किया",
        "credential_set": "कनेक्शन क्रेडेंशियल भरा",
        "credential_delete": "कनेक्शन क्रेडेंशियल हटाया",
        "credential_revoke": "कर्मचारी का कनेक्शन क्रेडेंशियल वापस लिया",
        "ee_jwt_rotate": "ee_jwt हस्ताक्षर कुंजी रोटेट की",
        "shared_credential_set": "नामित साझा क्रेडेंशियल सहेजा",
        "shared_credential_delete": "नामित साझा क्रेडेंशियल हटाया"
      },
      "extension": {
        "install": "एक्सटेंशन इंस्टॉल या अपग्रेड किया",
        "rollback": "एक्सटेंशन संस्करण वापस लिया"
      },
      "connect": {
        "publish": "कनेक्ट प्लगइन प्रकाशित किया"
      },
      "oidc": {
        "update": "OIDC कॉन्फ़िगरेशन बदला",
        "bind": "एंटरप्राइज़ पहचान बाइंड की",
        "feishu_sync": "Feishu संगठन सिंक किया"
      },
      "access": {
        "request": "पहुँच का अनुरोध किया"
      },
      "app": {
        "export": "ऐप निर्यात किया",
        "import": "ऐप आयात किया"
      },
      "demo": {
        "clear": "डेमो डेटा साफ़ किया",
        "import": "डेमो डेटा आयात किया"
      },
      "feedback": {
        "vote": "प्रतिक्रिया को रेटिंग दी"
      },
      "dsh_release": {
        "check": "आधिकारिक संस्करण जाँचा",
        "mirror": "इंस्टॉलर डाउनलोड किया",
        "mirror_failed": "इंस्टॉलर सत्यापन विफल",
        "recommend": "अनुशंसित संस्करण सेट किया",
        "delete": "स्थानीय इंस्टॉलर हटाया"
      },
      "brand": {
        "update": "एंटरप्राइज़ ब्रांड अपडेट किया"
      },
      "data": {
        "upload": "डेटा तालिका अपलोड की",
        "drop": "डेटा तालिका हटाई",
        "dataset_create": "डेटासेट बनाया",
        "dataset_update": "डेटासेट बदला",
        "dataset_delete": "डेटासेट हटाया"
      },
      "app_data": {
        "insert": "ऐप डेटा जोड़ा",
        "update": "ऐप डेटा बदला",
        "delete": "ऐप डेटा हटाया",
        "export": "ऐप डेटा निर्यात किया"
      },
      "kb": {
        "create": "ज्ञान आधार बनाया",
        "edit": "ज्ञान आधार बदला",
        "delete": "ज्ञान आधार हटाया",
        "doc_create": "दस्तावेज़ जोड़ा",
        "doc_edit": "दस्तावेज़ बदला",
        "doc_delete": "दस्तावेज़ हटाया"
      },
      "embed": {
        "session_create": "एम्बेड साइन-इन कोड जारी किया",
        "revoke": "एम्बेड सत्र निरस्त किया",
        "deny": "एम्बेड अनुरोध अस्वीकृत किया"
      },
      "directory": {
        "apply": "एंटरप्राइज़ निर्देशिका पंजीकृत की"
      },
      "client": {
        "create": "API क्लाइंट बनाया",
        "revoke": "API क्लाइंट अक्षम किया"
      },
      "files": {
        "upload": "Uploaded file",
        "delete": "Deleted file",
        "list": "Listed files"
      }
    }
  }
} satisfies Messages<typeof zhNs>;
