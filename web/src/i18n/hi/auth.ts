// साइन-इन और खाता।
import type zhNs from "../zh/auth";
import type { Messages } from "../types";

export default {
  "login": {
    "title": "वर्कबेंच में साइन इन करें",
    "subtitle": "व्यवस्थापक द्वारा आपके लिए बनाए गए खाते का उपयोग करें।",
    "username": "उपयोगकर्ता नाम",
    "password": "पासवर्ड",
    "submit": "साइन इन करें",
    "submitting": "साइन इन हो रहा है…",
    "forgot": "पासवर्ड भूल गए? कृपया व्यवस्थापक से रीसेट करवाएँ।",
    "ldapHint": "आप सीधे कंपनी के डोमेन खाते (कंप्यूटर के लॉगिन पासवर्ड जैसा) से साइन इन कर सकते हैं।",
    "ldapForgot": "डोमेन खाते का पासवर्ड कंपनी के खाता सिस्टम से पुनः प्राप्त करें; स्थानीय खाते का पासवर्ड भूल गए तो व्यवस्थापक से संपर्क करें।",
    "reauth": "आप अभी DSH से स्वतः साइन इन हैं और आपके पास केवल कर्मचारी अनुमतियाँ हैं। सभी सुविधाओं के लिए पासवर्ड से साइन इन करें।",
    "handoffExpired": "DSH से खोला गया साइन-इन लिंक समाप्त हो गया है (केवल एक बार, 1 मिनट तक वैध), कृपया साइन इन करके जारी रखें।",
    "handoffOtherSession": "This browser is already signed in with another account, so the sign-in link from DSH won't replace the existing session. Sign out of the current account first, then open the link from DSH again.",
    "demo": {
      "title": "यह एक सार्वजनिक डेमो साइट है",
      "account": "उपयोगकर्ता नाम {{username}} · पासवर्ड {{password}}",
      "reset": "डेटा {{reset}} रीसेट होता है, बेझिझक आज़माएँ।",
      "fill": "डेमो खाता भरें",
      "heroTitle": "“Xinghe Tech” के कर्मचारी के रूप में dsh-ee आज़माएँ",
      "heroSubtitle": "Xinghe Tech 200 कर्मचारियों की एक नमूना सॉफ़्टवेयर कंपनी है, जिसमें CRM, यात्रा व्यय, IT टिकट और 6 विभागों के ज्ञान आधार जुड़े हैं। कोई व्यक्ति चुनें और देखें वे रोज़ कौन से AI ऐप्स उपयोग करते हैं।",
      "uses": "सामान्य: {{app}}",
      "enterAs": "{{name}} के रूप में प्रवेश करें",
      "scenarios": {
        "pick": "Pick a demo scenario",
        "xinghe": {
          "tab": "Company · Xinghe Tech"
        },
        "novel": {
          "tab": "Small team · Shanhai Studio",
          "heroTitle": "Try dsh-ee as a member of Shanhai Studio",
          "heroSubtitle": "Shanhai Studio is a 4-person team serialising a novel, with no business systems to connect: the story bible and writing guide are knowledge bases, the chapter tracker is a table everyone can edit, and manuscript review goes to two experts. Pick a person to see the app they use every day."
        }
      },
      "adminDivider": "या व्यवस्थापक खाते से साइन इन करके देखें कि ऐप्स कैसे जुड़ते, समीक्षित होते, अधिकृत होते और ऑडिट होते हैं",
      "dsh": {
        "title": "DSH पहले से इंस्टॉल है? डेमो साइट से जोड़कर असली में पूछें",
        "intro": "अपने DSH के मॉडल से डेमो साइट के ऐप्स, ज्ञान आधार और नमूना व्यावसायिक सिस्टम कॉल करें।",
        "step1": "टर्मिनल में चलाएँ:",
        "unix": "macOS / Linux",
        "win": "Windows",
        "step2": "ब्राउज़र में पुष्टि पेज खुलने पर इस पेज पर कोई कर्मचारी पहचान चुनकर साइन इन करें, फिर “पुष्टि करें” पर क्लिक करें।",
        "step3": "DSH में लौटकर /customer-brief जैसा ऐप नाम टाइप करें और ऊपर के कार्ड में दिया प्रश्न पूछें।",
        "note": "डेमो साइट मॉडल प्रदान नहीं करती; अनुमान आपके DSH में पहले से कॉन्फ़िगर मॉडल से होता है। डेटा नियमित रूप से रीसेट होता है, रीसेट के बाद कनेक्शन कमांड दोबारा चलाएँ।"
      }
    }
  },
  "account": {
    "title": "खाता",
    "logout": "साइन आउट करें",
    "username": "उपयोगकर्ता नाम",
    "displayName": "प्रदर्शित नाम",
    "department": "विभाग",
    "roles": "भूमिकाएँ",
    "preferences": "Preferences",
    "preferencesHint": "Language and theme are saved on this device only.",
    "theme": "Theme",
    "changePassword": "पासवर्ड बदलें",
    "currentPassword": "वर्तमान पासवर्ड",
    "newPassword": "नया पासवर्ड",
    "newPasswordHint": "कम से कम 10 अक्षर। बदलने के बाद सभी डिवाइस पर दोबारा साइन इन करना होगा।",
    "confirmPassword": "नया पासवर्ड दोबारा दर्ज करें",
    "source": "खाता स्रोत",
    "sourceLocal": "स्थानीय खाता",
    "sourceLdap": "एंटरप्राइज़ निर्देशिका (LDAP)",
    "email": "ईमेल",
    "ldapManaged": "आपका खाता एंटरप्राइज़ निर्देशिका द्वारा प्रबंधित है, पासवर्ड कंपनी के खाता सिस्टम में बदलें; प्रदर्शित नाम और विभाग हर साइन-इन पर निर्देशिका से सिंक होते हैं।",
    "mismatch": "दोनों बार दर्ज किए गए नए पासवर्ड मेल नहीं खाते",
    "language": "Language",
    "languageHint": "Used for the web interface and messages such as Feishu notifications. Changes apply immediately and sync to your account.",
    "followOrg": "Follow organization default"
  }
} satisfies Messages<typeof zhNs>;
