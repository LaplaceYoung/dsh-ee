// क्षमता बाज़ार और प्लगइन विवरण।
import type zhNs from "../zh/market";
import type { Messages } from "../types";

export default {
  "market": {
    "title": "क्षमता बाज़ार",
    "desc": "{{count}} समीक्षित क्षमताएँ, जिनमें {{internal}} सहकर्मियों द्वारा साझा; सभी आंतरिक नेटवर्क में संग्रहीत, ऑफ़लाइन भी इंस्टॉल हो सकती हैं।",
    "setupGuide": "कनेक्शन गाइड",
    "downloadDsh": "DSH {{version}} डाउनलोड करें",
    "searchPlaceholder": "नाम, उपयोग या लेखक खोजें",
    "searchLabel": "क्षमता खोजें",
    "all": "सभी",
    "source": "स्रोत",
    "allSources": "सभी स्रोत",
    "internal": "कंपनी के भीतर",
    "community": "समुदाय से चयनित",
    "category": "श्रेणी",
    "allCategories": "सभी श्रेणियाँ",
    "sort": "क्रम",
    "sortBy": "{{label}} के अनुसार",
    "sortInstalls": "कंपनी में लोकप्रिय",
    "sortScore": "dshfind स्कोर",
    "sortRating": "कर्मचारी रेटिंग",
    "sortRecent": "हाल में अपडेट",
    "count": "{{count}}",
    "empty": "कोई मेल खाती क्षमता नहीं मिली। जो चाहिए वह यहाँ नहीं है? <link>समावेश का अनुरोध करें या स्वयं एक साझा करें</link>।"
  },
  "pluginDetail": {
    "notFound": "प्लगइन मौजूद नहीं या अभी समीक्षा से पास नहीं हुआ।",
    "backToMarket": "क्षमता केंद्र पर वापस जाएँ",
    "market": "क्षमता केंद्र",
    "internal": "आंतरिक · {{department}}",
    "upstream": "अपस्ट्रीम · dshfind संग्रहित",
    "featured": "अनुशंसित",
    "reviewed": "समीक्षित",
    "install": "इंस्टॉल करें",
    "runInDsh": "DSH में चलाएँ (एंटरप्राइज़ स्रोत जुड़ा होने पर)",
    "discoverHint": "DSH प्लगइन पेज → Discover में “{{title}}” खोजकर एक-क्लिक इंस्टॉल भी कर सकते हैं। एंटरप्राइज़ स्रोत अभी नहीं जुड़ा? <link>कनेक्शन गाइड</link> देखें।",
    "securityChecks": "सुरक्षा जाँच",
    "version": "संस्करण",
    "author": "लेखक",
    "installs": "कंपनी में इंस्टॉल",
    "installsValue": "{{count}} बार",
    "updated": "अपडेट",
    "deps": "निर्भरता-बंदी",
    "depsValue": "{{count}} पैकेज · {{size}}",
    "offline": "ऑफ़लाइन इंस्टॉल योग्य",
    "offlineNo": "नहीं (बाइनरी डाउनलोड के लिए इंटरनेट चाहिए)",
    "repo": "स्रोत रिपॉज़िटरी"
  }
} satisfies Messages<typeof zhNs>;
