import i18n from "i18next";
import LanguageDetector from "i18next-browser-languagedetector";
import { initReactI18next } from "react-i18next";

import en from "./locales/en.json";
import zh from "./locales/zh.json";
import zhTW from "./locales/zh-TW.json";
import fr from "./locales/fr.json";
import ja from "./locales/ja.json";
import ru from "./locales/ru.json";
import vi from "./locales/vi.json";

void i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources: { en, zh, "zh-TW": zhTW, fr, ja, ru, vi },
    supportedLngs: ["en", "zh", "zh-TW", "fr", "ja", "ru", "vi"],
    fallbackLng: "en",
    keySeparator: false,
    interpolation: { escapeValue: false },
    detection: { order: ["navigator", "htmlTag"], caches: [] },
  });

export default i18n;
