# Interface localization

English (`en`) is the source language and default for new or older settings files without a language. The repository, comments, diagnostics and Markdown documentation use English. The interface also supports Brazilian Portuguese (`pt-BR`). Select **Preferences → Language** and save to change it. The preference persists in `settings.json`; unsupported saved values fall back to English, and unsupported values submitted through the API are rejected.

## Catalog and rendering

`internal/i18n/pt-BR.json` maps English source text to Portuguese. Both the React interface and native Go tray/file-picker labels use this catalog. English text serves as the key and fallback, so no duplicate English catalog or additional package is required. The HTML language attribute follows the selected language.

`frontend/src/i18n.tsx` provides the language context and translator. Wrap interface copy in `t("English source text")`. Use named placeholders for complete sentences, such as `t("More actions for {name}", { name: connection.name })`. Keep placeholder names identical in translations. Known backend errors use their existing Go format placeholders (`%s`, `%d`, `%w`, `%v`), which the translators match to preserve dynamic details. Unknown messages fall back to English. Operating-system and third-party messages may use their own language.

Translate display fields explicitly: labels, hints, notices, known authentication questions and errors. Never translate configuration keys, provider IDs, account names, user-supplied labels, remote folder names, paths, example values or file content. Provider-owned library/site names stay as returned by the service. The installer and its helper progress use English; native system dialogs may follow Windows language settings.

Accessibility text follows the same catalog: the drive-list name, experimental indicators, control descriptions and drive-state announcements use English source keys. Announcements preserve the connection name as a placeholder, establish a silent initial baseline and report state changes rather than repeating unchanged polls or upload counters. Changing language alone does not announce existing drives again.

Use `aria-describedby` to associate instructions and consequences with their controls. Keep help and validation-error IDs together while an error is present; clearing an error must retain the help. Password visibility controls preserve that association. Keep explanatory text outside checkbox labels so it does not become part of their accessible name.

## Adding a language

1. Add an English-to-translation JSON catalog beside `pt-BR.json`, preserving named and Go format placeholders.
2. Register it in `internal/i18n/i18n.go` and `frontend/src/i18n.tsx`, extend the language types in the frontend translator and API, and add a Preferences option using the language's own name.
3. Add translation, fallback and settings-persistence tests. Check the connection wizard, errors, accessibility labels and native tray/file picker.
4. Update README and this guide with the supported language.

Run `npm --prefix frontend test`, `npm --prefix frontend run build` and `./.tools/go/bin/go.exe test ./...`. The native UI smoke test uses English selectors and an isolated profile.
