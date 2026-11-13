import fs from "node:fs/promises";

const locales = ["en", "zh", "zh-TW", "fr", "ja", "ru", "vi"];
const base = JSON.parse(await fs.readFile("src/i18n/locales/en.json", "utf8")).translation;
for (const locale of locales) {
  const file = `src/i18n/locales/${locale}.json`;
  const json = JSON.parse(await fs.readFile(file, "utf8"));
  for (const key of Object.keys(base)) {
    if (typeof json.translation[key] !== "string" || !json.translation[key])
      throw new Error(`${locale}: missing ${key}`);
  }
  json.translation = Object.fromEntries(
    Object.entries(json.translation).sort(([a], [b]) => a.localeCompare(b)),
  );
  await fs.writeFile(file, JSON.stringify(json, null, 2) + "\n");
  console.log(`${locale}: ${Object.keys(json.translation).length} keys, complete`);
}
