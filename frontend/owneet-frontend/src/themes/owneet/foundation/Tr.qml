// SPDX-License-Identifier: GPL-3.0-or-later
// Translated interface text (docs/translating.md). Every visible string goes through here:
//   Tr.tr("games.empty")                      message in the current language
//   Tr.tr("games.launching", { title: t })    with a {title} placeholder
//   Tr.trn("games.count", n)                  plural forms, {n} = n
// Bindings that use it update when the language changes.
pragma Singleton
import QtQuick 2.15

QtObject {
    readonly property string language: i18n.language
    readonly property var languages: i18n.languages
    function setLanguage(tag) { i18n.language = tag; }

    function tr(key, args) {
        const revision = i18n.revision; // makes the calling binding depend on the language
        return i18n.tr(key, args || {});
    }
    function trn(key, n, args) {
        const revision = i18n.revision;
        return i18n.trn(key, n, args || {});
    }
    function languageName(tag) {
        for (let i = 0; i < languages.length; i++)
            if (languages[i].tag === tag)
                return languages[i].name;
        return tag;
    }
}
