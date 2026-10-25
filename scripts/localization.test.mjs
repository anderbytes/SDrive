import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("every installer language translates all custom strings and preserves runtime variables", async () => {
  const source = await readFile(new URL("../installer/sdrive.nsi", import.meta.url), "utf8");
  const strings = await readFile(new URL("../installer/languages.nsh", import.meta.url), "utf8");
  const languages = ["English", "PortugueseBR", "Spanish", "French", "German", "Italian", "SimpChinese"];
  for (const language of languages) assert(source.includes(`!insertmacro MUI_LANGUAGE "${language}"`));
  const entries = new Map();
  for (const [, name, language, value] of strings.matchAll(/^LangString (\w+) \$\{LANG_(\w+)\} "(.+)"\r?$/gm)) {
    if (!entries.has(name)) entries.set(name, new Map());
    assert(!entries.get(name).has(language), `Duplicate ${name}: ${language}`);
    entries.get(name).set(language, value);
  }
  const variables = (text) => (text.match(/\$\d|\$\\[rn]/g) || []).sort();
  for (const [name, translations] of entries) {
    assert.equal(translations.size, languages.length, name);
    const english = translations.get("ENGLISH");
    assert(english, `Missing English: ${name}`);
    for (const [language, value] of translations) {
      assert(value.trim(), `${name}: ${language}`);
      assert.deepEqual(variables(value), variables(english), `${name}: ${language}`);
    }
  }
  for (const [, name] of source.matchAll(/\$\((\w+)\)/g)) assert(entries.has(name), `Undefined installer text: ${name}`);
  assert(!/MessageBox[^\n]*"(?!\$\()[A-Za-z]/.test(source), "Untranslated custom MessageBox");
  assert(!/DetailPrint "(?!\$\()[A-Za-z]/.test(source), "Untranslated custom progress");
});
