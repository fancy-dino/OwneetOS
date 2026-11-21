// SPDX-License-Identifier: GPL-3.0-or-later
// The order of the apps on the home screen and on the Apps page ([[step-5.7-apps-page]],
// [[home-and-library]], approved demo design/demos/5.7-apps-page.html). An app is
// { key, installed }: "videos", "music", "store" (built in, always installed) or "web:<id>" (a web
// app of owneetd's catalogue).
//  - The fixed order: the built-in apps first (Videos, Music, the store), then the web apps in the
//    catalogue's order.
//  - The Apps page: every app in the fixed order, the installed ones before the missing ones.
//  - The home's six places: with six apps or fewer every app, a missing one in its place; beyond,
//    five apps and "All apps": the first five of the Apps page's order while six or fewer are
//    installed, from the seventh installed app the five most used (the most used first; equal
//    counts in the fixed order).
// Uses: how many times each app was opened from the console ({ key: count }; the theme keeps
// them). Plain functions, so that the theme imports them and Node tests them
// (frontend/owneet-frontend/tests/theme/apps.test.js).

var HOME_PLACES = 6;
var ALL_APPS = "allapps";

// The fixed order: the built-in apps, then the catalogue's web apps ({ id, installed })
function fixedOrder(webApps) {
    var apps = [{ key: "videos", installed: true }, { key: "music", installed: true }, { key: "store", installed: true }];
    for (var i = 0; i < webApps.length; i++)
        apps.push({ key: "web:" + webApps[i].id, installed: webApps[i].installed === true });
    return apps;
}

// The Apps page: the installed apps before the missing ones, each in the fixed order (keys)
function pageOrder(apps) {
    var keys = [];
    apps.forEach(function (a) { if (a.installed) keys.push(a.key); });
    apps.forEach(function (a) { if (!a.installed) keys.push(a.key); });
    return keys;
}

// The home's places (keys; ALL_APPS for "All apps")
function homeOrder(apps, uses) {
    if (apps.length <= HOME_PLACES)
        return apps.map(function (a) { return a.key; });
    var installed = apps.filter(function (a) { return a.installed; });
    var five;
    if (installed.length <= HOME_PLACES) {
        five = pageOrder(apps).slice(0, HOME_PLACES - 1);
    } else {
        var count = function (a) { return (uses && uses[a.key]) || 0; };
        five = installed.map(function (a, i) { return { a: a, i: i }; })
            .sort(function (x, y) { return count(y.a) - count(x.a) || x.i - y.i; })
            .slice(0, HOME_PLACES - 1)
            .map(function (x) { return x.a.key; });
    }
    return five.concat([ALL_APPS]);
}

// The uses without the apps that are not installed (removing an app clears its count); null when
// nothing changes
function pruneUses(apps, uses) {
    var kept = {}, changed = false;
    Object.keys(uses || {}).forEach(function (key) {
        var app = apps.find(function (a) { return a.key === key; });
        if (app && app.installed) kept[key] = uses[key];
        else changed = true;
    });
    return changed ? kept : null;
}

if (typeof module !== "undefined")
    module.exports = { HOME_PLACES: HOME_PLACES, ALL_APPS: ALL_APPS, fixedOrder: fixedOrder, pageOrder: pageOrder,
                       homeOrder: homeOrder, pruneUses: pruneUses };
