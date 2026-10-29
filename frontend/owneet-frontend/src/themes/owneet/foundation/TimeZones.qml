// SPDX-License-Identifier: GPL-3.0-or-later
// The cities of the time zone lists ([[step-6.1-first-boot-flow]], [[first-boot-setup]]): the
// first start and Settings → Language. A city is an id (its name is the message city.<id>) and a
// zone; each language has its own cities (one: it is suggested; several: a list), and "another
// city" lists them all. The zones' distance from UTC now comes from owneetd (GET /v1/time/zones):
// never guessed from the place.
pragma Singleton
import QtQuick 2.15

QtObject {
    readonly property var cityZones: ({
        rome: "Europe/Rome", berlin: "Europe/Berlin", paris: "Europe/Paris", brussels: "Europe/Brussels",
        geneva: "Europe/Zurich", montreal: "America/Toronto", madrid: "Europe/Madrid",
        mexico_city: "America/Mexico_City", bogota: "America/Bogota", lima: "America/Lima",
        buenos_aires: "America/Argentina/Buenos_Aires", santiago: "America/Santiago",
        london: "Europe/London", dublin: "Europe/Dublin", new_york: "America/New_York",
        toronto: "America/Toronto", chicago: "America/Chicago", denver: "America/Denver",
        los_angeles: "America/Los_Angeles", vancouver: "America/Vancouver", sydney: "Australia/Sydney",
        auckland: "Pacific/Auckland", johannesburg: "Africa/Johannesburg", singapore: "Asia/Singapore"
    })
    readonly property var languageCities: ({
        it: ["rome"], de: ["berlin"], fr: ["paris", "brussels", "geneva", "montreal"],
        es: ["madrid", "mexico_city", "bogota", "lima", "buenos_aires", "santiago"],
        en: ["london", "dublin", "new_york", "toronto", "chicago", "denver", "los_angeles", "vancouver",
             "sydney", "auckland", "johannesburg", "singapore"]
    })
    // zone -> seconds east of UTC now, from owneetd (GET /v1/time/zones), set by the theme
    property var offsets: ({})
    readonly property var allZones: {
        const out = [];
        for (const c in cityZones)
            if (out.indexOf(cityZones[c]) < 0) out.push(cityZones[c]);
        return out;
    }

    function citiesFor(language) { return languageCities[language] || languageCities.en; }
    // every city, by distance from UTC, then by name in the language shown
    function allCities() {
        const seen = {}, out = [];
        for (const tag in languageCities)
            languageCities[tag].forEach(c => { if (!seen[c]) { seen[c] = true; out.push(c); } });
        const off = offsets;
        const revision = Tr.language;     // the names' order follows the language
        out.sort((a, b) => ((off[cityZones[a]] || 0) - (off[cityZones[b]] || 0))
                           || Tr.tr("city." + a).localeCompare(Tr.tr("city." + b)));
        return out;
    }
    // the city to show for a zone: the one chosen (kept in the interface's memory), if it is in that
    // zone, else the first city of the language in it, else any city in it ("" for none)
    function cityOf(zone, chosen) {
        if (chosen && cityZones[chosen] === zone)
            return chosen;
        const own = citiesFor(Tr.language).filter(c => cityZones[c] === zone);
        if (own.length > 0)
            return own[0];
        for (const c in cityZones)
            if (cityZones[c] === zone)
                return c;
        return "";
    }
    function offsetOf(zone) { return offsets[zone] || 0; }
    // the time now in a zone, from the clock in UTC (whatever zone the interface started with)
    function clockAt(zone, nowMs) {
        const d = new Date(nowMs + offsetOf(zone) * 1000);
        const two = n => (n < 10 ? "0" : "") + n;
        return two(d.getUTCHours()) + ":" + two(d.getUTCMinutes());
    }
    // a date and time in one zone ("2026-10-29", "13:58", "UTC") as fields in another: [day, month,
    // year, hours, minutes]
    function fieldsIn(date, clockTime, from, to) {
        const d = String(date).split("-").map(Number), t = String(clockTime).split(":").map(Number);
        if (d.length !== 3 || t.length !== 2 || d.concat(t).some(isNaN))
            return [1, 1, 2026, 0, 0];
        const at = new Date(Date.UTC(d[0], d[1] - 1, d[2], t[0], t[1]) + (offsetOf(to) - offsetOf(from)) * 1000);
        return [at.getUTCDate(), at.getUTCMonth() + 1, at.getUTCFullYear(), at.getUTCHours(), at.getUTCMinutes()];
    }
    function utcLabel(zone) {
        const offset = offsetOf(zone);
        const h = Math.floor(Math.abs(offset) / 3600), m = Math.floor(Math.abs(offset) % 3600 / 60);
        return "UTC" + (offset < 0 ? "−" : "+") + h + (m > 0 ? ":" + (m < 10 ? "0" : "") + m : "");
    }
}
