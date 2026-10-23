# Interface localization

English (`en`) is the source language and default for new or older settings files without a language. The repository, comments, diagnostics and Markdown documentation use English. The interface also supports Brazilian Portuguese (`pt-BR`). Select **Preferences → Language** and save to change it. The preference persists in `settings.json`; unsupported saved values fall back to English, and unsupported values submitted through the API are rejected.

## Catalog and rendering

`internal/i18n/pt-BR.json` maps English source text to Portuguese. Both the React interface and native Go tray/file-picker labels use this catalog. English text serves as the key and fallback for static copy. The HTML language attribute follows the selected language. No additional package is required.

`frontend/src/i18n.tsx` provides the language context and translator. Wrap interface copy in `t("English source text")`. Use named placeholders for complete sentences, such as `t("More actions for {name}", { name: connection.name })`. Keep placeholder names identical in translations. Legacy backend errors retain their Go format placeholders (`%s`, `%d`, `%w`, `%v`); matching uses a deterministic order, with more literal text first and source text as the tie-breaker. Unknown messages fall back to English. Operating-system and third-party messages may use their own language.

## Structured dynamic messages

`internal/i18n/messages.json` shares stable message codes and named English/Portuguese templates between Go and TypeScript. Dynamic field validation, occupied drive letters and recovery-before-exit errors use `{code, params, message}`. `message` remains the original English error for callers and fallback. Field validation also retains `field`; snapshots carry optional `connection.errorMessage` beside the existing `connection.error` string. These additions do not change public service method signatures or persisted settings.

Use `i18n.NewMessage` or the core field-message helper at the error's source, preserve metadata through the Wails error cause, and pass the structured message to `t`. Native action errors use `i18n.Error`. Parameters are strings and are inserted once; braces, format tokens and Unicode inside user values remain opaque. Unknown codes, missing parameters and malformed frontend parameters display the original message. Structured errors do not require English-wording regex inference.

This is a limited migration. Engine-exit summaries, wrapped persistence errors, provider questions and other existing text continue using the static/legacy catalog. Do not introduce codes for every static label or infer a code by parsing already formatted English text. Extend structured messages when a concrete dynamic flow needs them, and add fallback and opaque-parameter tests.

Translate display fields explicitly: labels, hints, notices, known authentication questions and errors. Never translate configuration keys, provider IDs, account names, user-supplied labels, remote folder names, paths, example values or file content. Provider-owned library/site names stay as returned by the service. The installer and its helper progress use English; native system dialogs may follow Windows language settings.

Accessibility text follows the same catalog: the drive-list name, experimental indicators, control descriptions and drive-state announcements use English source keys. Announcements preserve the connection name as a placeholder, establish a silent initial baseline and report state changes rather than repeating unchanged polls or upload counters. Changing language alone does not announce existing drives again.

Use `aria-describedby` to associate instructions and consequences with their controls. Keep help and validation-error IDs together while an error is present; clearing an error must retain the help. Password visibility controls preserve that association. Keep explanatory text outside checkbox labels so it does not become part of their accessible name.

## Adding a language

1. Add an English-to-translation JSON catalog beside `pt-BR.json`, preserving named and Go format placeholders.
2. Register it in `internal/i18n/i18n.go` and `frontend/src/i18n.tsx`, extend the language types in the frontend translator and API, and add a Preferences option using the language's own name.
   Add the language to the structured message catalog and its Go/TypeScript formatter selection as well.
3. Add translation, fallback and settings-persistence tests. Check the connection wizard, errors, accessibility labels and native tray/file picker.
4. Update README and this guide with the supported language.

Run `npm --prefix frontend test`, `npm --prefix frontend run build` and `./.tools/go/bin/go.exe test ./...`. The native UI smoke test uses English selectors and an isolated profile.
