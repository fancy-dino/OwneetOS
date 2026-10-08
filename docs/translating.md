# Translating OwneetOS

The OwneetOS interface is translated with one **JSON file per language** in
[`frontend/i18n/`](../frontend/i18n/). Adding a language needs no code change: add its file, and
it appears in **Appearance → Language** (Settings → Language from step 3.9).

- **English** (`en.json`) is the default language and the reference: every message exists there
  first. When a message is missing in a language, the English one is shown.
- **Italian** (`it.json`) is kept complete too. Other languages may be partial.

## The file

The file name is the language tag (`de.json`, `fr.json`, `pt-BR.json`, `zh-Hans.json`, …).

```json
{
  "@meta": {
    "language": "Deutsch",
    "plural": "one-other"
  },
  "prompt.back": "Zurück",
  "games.count": { "one": "{n} Spiel", "other": "{n} Spiele" }
}
```

- `@meta.language`: the language's name **in that language** ("Deutsch", not "German"); this is
  what the language list shows.
- `@meta.plural`: how the language forms plurals (see below). Default: `one-other`.
- Every other entry is a message: its key (do not translate keys) and the text.
- Keep the `{placeholders}` exactly as in English; you may move them within the sentence.
  Numbers are written in the language's own style (e.g. `1.000` in Italian).
- Button names in prompts ("A", "LB") and brand names (Xbox, PlayStation, Nintendo, Steam) are not
  translated. Palette names (`palette.*`) may be translated freely or kept.
- Write the text for a TV seen from the sofa: short, plain words, no technical terms.
- Save the file as UTF-8.

## Plural forms

A message with a number can have one text per plural form:

| `plural` rule | Forms | Languages (examples) |
|---|---|---|
| `one-other` | `one` (1), `other` | English, Italian, German, Spanish, Portuguese, Dutch, Swedish, Greek… |
| `french` | `one` (0 and 1), `other` | French, Brazilian Portuguese |
| `none` | `other` | Japanese, Chinese, Korean, Vietnamese, Thai, Indonesian, Turkish… |
| `east-slavic` | `one` (1, 21, 31…), `few` (2–4, 22–24…), `many` | Russian, Ukrainian, Belarusian |
| `polish` | `one` (1), `few` (2–4, 22–24…), `many` | Polish |
| `czech` | `one` (1), `few` (2–4), `other` | Czech, Slovak |

A language whose plural rules are not listed needs a small code change (a new rule in
`frontend/owneet-frontend/src/backend/owneet/I18n.cpp` and `tools/i18n-check`): open an issue.

## Checking a translation

`tools/i18n-check` (also part of `tools/lint`) checks every file: valid JSON, known plural rule,
the same placeholders as English, no keys that English does not have; for English and Italian, no
missing message. It lists how many messages each other language still misses.

To see a translation on a running console without rebuilding anything, copy the file to
`~/.config/owneet-frontend/i18n/` of the `owneet` user and restart the interface: a file there
overrides the installed one of the same language.

## Current limits

- Right-to-left languages (Arabic, Hebrew) are not supported yet.
- The bundled fonts (Lexend, Bricolage Grotesque) cover Latin scripts (including Vietnamese).
  Greek, Cyrillic and Asian scripts need extra fonts in the image, which will be added together
  with the first such language.

## For developers

QML uses the `Tr` singleton of the theme (`src/themes/owneet/foundation/Tr.qml`), never a literal
string for visible text:

```qml
Text { text: Tr.tr("games.empty") }
Text { text: Tr.trn("games.count", games.count) }        // plural forms, {n}
Text { text: Tr.tr("games.launching", { title: name }) } // {title} placeholder
```

Add new messages to `en.json` and `it.json` in the same change. The language choice is saved in
the frontend's settings file (`general.locale`).
