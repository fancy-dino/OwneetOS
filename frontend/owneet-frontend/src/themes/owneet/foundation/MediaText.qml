// SPDX-License-Identifier: GPL-3.0-or-later
// Times and languages on the Videos and Music pages ([[step-5.6-local-videos-and-music]]):
//   MediaText.clock(214)        "3:34" (a song, where a video stopped; "1:02:05" over an hour)
//   MediaText.span(2465)        "41 min", "1 h 52 min" (a video's length, what is left)
//   MediaText.languages(["en", "it"])   "English, Italian" (the message files' names, else the
//                               language's own name)
//   MediaText.hue("Low Tide")   the hue GameArt draws a cover from (0…359): the player draws a
//                               song without a picture the same way
pragma Singleton
import QtQuick 2.15

QtObject {
    function two(n) { return (n < 10 ? "0" : "") + n; }
    function clock(seconds) {
        const s = Math.max(0, Math.floor(seconds || 0));
        const h = Math.floor(s / 3600), m = Math.floor(s / 60) % 60;
        return h > 0 ? h + ":" + two(m) + ":" + two(s % 60) : m + ":" + two(s % 60);
    }
    function span(seconds) {
        const m = Math.max(1, Math.round((seconds || 0) / 60));
        return m >= 60 ? Tr.tr("media.time.hours", { h: Math.floor(m / 60), m: m % 60 }) : Tr.tr("media.time.minutes", { m: m });
    }
    function language(code) {
        const key = "lang." + code;
        const name = Tr.tr(key);
        return name !== key ? name : media.nativeLanguageName(code);
    }
    function hue(seed) {             // GameArt's: FNV-1a over the characters, over the colour wheel
        let h = 2166136261;
        for (let i = 0; i < (seed || "").length; i++)
            h = Math.imul(h ^ seed.charCodeAt(i), 16777619) >>> 0;
        return h % 360;
    }
    // the known languages, each once
    function languages(codes) {
        const out = [];
        (codes || []).forEach(c => { if (c && out.indexOf(c) < 0) out.push(c); });
        return out.map(language).join(", ");
    }
}
