import { useSyncExternalStore } from "react";
import { Button } from "@rosemary-ui/react";
import catalogue from "./translations.json";

export type Language = "zh-CN" | "en";
const translations: Record<string, string> = catalogue;
const normalize = (s: string) => s.replace(/\s+/g, " ").trim();
const listeners = new Set<() => void>();
let language: Language = (() => {
  try {
    const saved = localStorage.getItem("arca-language");
    if (saved === "en" || saved === "zh-CN") return saved;
  } catch {}
  return navigator.language.toLowerCase().startsWith("zh") ? "zh-CN" : "en";
})();
export const getLanguage = () => language;
export function setLanguage(next: Language) {
  language = next;
  try {
    localStorage.setItem("arca-language", next);
  } catch {}
  document.documentElement.lang = next;
  for (const listener of listeners) listener();
}
const subscribe = (listener: () => void) => {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
};
export function useLanguage() {
  return useSyncExternalStore(subscribe, getLanguage, getLanguage);
}
export function msg(value: string) {
  return language === "en" ? (translations[normalize(value)] ?? value) : value;
}
export function msgt(strings: TemplateStringsArray, ...values: unknown[]) {
  const key = strings
    .map((s, i) => s + (i < values.length ? `{${i}}` : ""))
    .join("");
  const translated =
    language === "en" ? translations[normalize(key)] : undefined;
  return translated
    ? translated.replace(/\{(\d+)\}/g, (_, i) => String(values[Number(i)]))
    : strings.reduce(
        (s, part, i) => s + part + (i < values.length ? String(values[i]) : ""),
        "",
      );
}
export function LanguageToggle() {
  const current = useLanguage();
  return (
    <Button
      type="button"
      className="language-toggle"
      variant="ghost"
      aria-label={current === "en" ? "切换为中文" : "EN — Switch to English"}
      onClick={() => setLanguage(current === "en" ? "zh-CN" : "en")}
    >
      {current === "en" ? "中文" : "EN"}
    </Button>
  );
}
document.documentElement.lang = language;
