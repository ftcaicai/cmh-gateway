import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { test } from "node:test";
import i18next from "i18next";

const locales = ["en", "zh", "zh-TW", "fr", "ja", "ru", "vi"];

test("all console and official loading/error translation keys are complete in seven own locales", async () => {
  const resources = {};
  for (const locale of locales)
    resources[locale] = JSON.parse(
      await readFile(new URL(`../src/i18n/locales/${locale}.json`, import.meta.url), "utf8"),
    );
  const base = Object.keys(resources.en.translation).sort();
  for (const locale of locales) {
    assert.deepEqual(Object.keys(resources[locale].translation).sort(), base);
    for (const key of base) assert.ok(resources[locale].translation[key].length > 0);
  }
  for (const source of ["routes/__root.tsx", "routes/index.tsx", "routes/whoami.tsx"]) {
    const content = await readFile(new URL(`../src/${source}`, import.meta.url), "utf8");
    for (const match of content.matchAll(/\bt\(['"]([^'"]+)['"]\)/g))
      assert.ok(base.includes(match[1]), `missing key ${match[1]}`);
  }
  const instance = i18next.createInstance();
  await instance.init({ resources, lng: "en", fallbackLng: "en", keySeparator: false });
  assert.equal(instance.t("Current account"), "Current account");
  await instance.changeLanguage("zh-TW");
  assert.equal(instance.t("Current account"), "目前帳號");
  await instance.changeLanguage("unknown-language");
  assert.equal(instance.t("Current account"), "Current account");
});
