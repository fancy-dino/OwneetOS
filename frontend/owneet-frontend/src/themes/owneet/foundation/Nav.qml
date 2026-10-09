// SPDX-License-Identifier: GPL-3.0-or-later
// Input map and navigation, shared by every screen (PROJECT_RULES.md section 9.1):
//   action(event)  turns a key event into what it means here:
//     up, down, left, right   move the selection (D-pad, left stick; they repeat when held)
//     accept (A), back (B), secondary (X), page (Y), options (Menu)
//     section-prev / section-next (LB / RB), filter-prev / filter-next (LT / RT)
//     scroll-up / scroll-down (right stick, repeats: fast scrolling)
//   Guide never reaches the interface (owned by owneetd).
//   find(area, from, direction)  spatial navigation: the nearest navigable item that way.
//   feedback(kind)  what just happened, for the interface sounds (step 3.12): move, edge,
//     confirm, back, section, tab, toggle-on, toggle-off, sheet-open, sheet-close, launch. The sounds
//     listen to `played`: one per button press, the most specific one ("confirm" only when the
//     action that follows, e.g. opening a window, reports nothing of its own).
pragma Singleton
import QtQuick 2.15

QtObject {
    signal played(string kind)
    property real lastPageStep: 0
    property string lastPage: ""

    property bool confirmPending: false
    function feedback(kind) {
        if (kind === "confirm") {
            confirmPending = true;
            Qt.callLater(flushConfirm);
            return;
        }
        confirmPending = false;
        played(kind);
    }
    function flushConfirm() {
        if (confirmPending) {
            confirmPending = false;
            played("confirm");
        }
    }

    function action(event) {
        const k = api.keys;
        if (k.isUp(event)) return "up";
        if (k.isDown(event)) return "down";
        if (k.isLeft(event)) return "left";
        if (k.isRight(event)) return "right";
        if (k.isScrollUp(event)) return "scroll-up";
        if (k.isScrollDown(event)) return "scroll-down";
        // Sections and filters repeat while held, at half the speed of the directions (the
        // controller: after 720 ms every 360 ms; a keyboard's own repeat is held to 360 ms);
        // every other button acts once per press (no repeated launches or closes).
        const page = k.isPrevPage(event) ? "section-prev" : k.isNextPage(event) ? "section-next"
                   : k.isPageUp(event) ? "filter-prev" : k.isPageDown(event) ? "filter-next" : "";
        if (page !== "") {
            const now = Date.now();
            if (event.isAutoRepeat && now - lastPageStep < 350) {
                // the same event read again by a parent handler keeps its answer
                return page === lastPage && now - lastPageStep < 20 ? page : "";
            }
            lastPageStep = now;
            lastPage = page;
            return page;
        }
        if (event.isAutoRepeat)
            return "";
        if (k.isAccept(event)) return "accept";
        if (k.isCancel(event)) return "back";
        if (k.isDetails(event)) return "secondary";
        if (k.isFilters(event)) return "page";
        if (k.isMenu(event)) return "options";
        return "";
    }
    function isDirection(a) { return a === "up" || a === "down" || a === "left" || a === "right"; }

    // Items take part in navigation when they declare `property bool navigable: true` and are
    // visible and enabled.
    function navigables(root) {
        const out = [];
        const walk = item => {
            for (let i = 0; i < item.children.length; i++) {
                const c = item.children[i];
                if (!c.visible || !c.enabled)
                    continue;
                if (c.navigable === true)
                    out.push(c);
                walk(c);
            }
        };
        walk(root);
        return out;
    }

    // Spatial navigation. Without groups: the nearest navigable item in `direction` from `from`
    // (distance along the direction counts once, across it 2.5 times, as in demo 3.3).
    // With groups (NavGroup, approved in demo 3.6): the selection first moves inside its group,
    // only along its row or column; at the group's edge it enters the next group in that
    // direction (the one overlapping the selection most). Entering sideways returns to the item
    // selected there last time (the first item the first time); entering up or down goes to the
    // nearest item. Returns null at the edge.
    function find(root, from, direction) {
        if (!from)
            return navigables(root)[0] || null;
        const group = groupOf(root, from);
        const inside = nearest(root, navigables(group || root), from, direction, group !== null);
        if (inside || !group)
            return inside;

        const f = rect(root, from), g = rect(root, group);
        let target = null, targetOverlap = 0;
        for (const other of groups(root)) {
            if (other === group || navigables(other).length === 0)
                continue;
            const o = rect(root, other);
            const beyond = direction === "left" ? o.right <= g.left + 4 : direction === "right" ? o.left >= g.right - 4
                         : direction === "up" ? o.bottom <= g.top + 4 : o.top >= g.bottom - 4;
            if (!beyond)
                continue;
            const ov = direction === "left" || direction === "right"
                ? (overlap(f.top, f.bottom, o.top, o.bottom) || overlap(g.top, g.bottom, o.top, o.bottom))
                : overlap(f.left, f.right, o.left, o.right);
            if (ov > targetOverlap) { targetOverlap = ov; target = other; }
        }
        if (!target)
            return null;
        if (direction === "left" || direction === "right") {
            const r = target.remembered;
            return r && r.visible && navigables(target).indexOf(r) >= 0 ? r : navigables(target)[0];
        }
        return nearest(root, navigables(target), from, "", false);
    }

    function rect(root, item) {
        const p = item.mapToItem(root, 0, 0);
        return { left: p.x, top: p.y, right: p.x + item.width, bottom: p.y + item.height };
    }
    function overlap(a0, a1, b0, b1) { return Math.max(0, Math.min(a1, b1) - Math.max(a0, b0)); }
    function groupOf(root, item) {
        for (let p = item.parent; p && p !== root; p = p.parent)
            if (p.navGroup === true)
                return p;
        return null;
    }
    function groups(root) {
        const out = [];
        const walk = item => {
            for (let i = 0; i < item.children.length; i++) {
                const c = item.children[i];
                if (!c.visible)
                    continue;
                if (c.navGroup === true)
                    out.push(c);
                else
                    walk(c);
            }
        };
        walk(root);
        return out;
    }
    // `aligned`: only items on the same row (left/right) or column (up/down) as `from`;
    // `direction` "": any direction (entering a group)
    function nearest(root, items, from, direction, aligned) {
        const a = rect(root, from), ax = (a.left + a.right) / 2, ay = (a.top + a.bottom) / 2;
        // Up / down: the nearest row first, then the item closest to the selection in it, so a
        // row of buttons (e.g. text size S M L XL) is never skipped (demo 3.9)
        if (direction === "up" || direction === "down") {
            const below = direction === "down";
            const rows = [];
            for (const it of items) {
                if (it === from)
                    continue;
                const b = rect(root, it);
                if (below ? b.top < a.bottom - 4 : b.bottom > a.top + 4)
                    continue;
                if (aligned && overlap(a.left, a.right, b.left, b.right) < 4)
                    continue;
                rows.push({ it: it, edge: below ? b.top : -b.bottom, cx: (b.left + b.right) / 2 });
            }
            if (rows.length === 0)
                return null;
            const first = Math.min.apply(null, rows.map(r => r.edge));
            let best = null, bestDx = Infinity;
            for (const r of rows) {
                if (r.edge > first + 8)
                    continue;
                const dx = Math.abs(r.cx - ax);
                if (dx < bestDx) { bestDx = dx; best = r.it; }
            }
            return best;
        }
        const horizontal = direction === "" || direction === "left" || direction === "right";
        let best = null, bestScore = Infinity;
        for (const it of items) {
            if (it === from)
                continue;
            const b = rect(root, it), dx = (b.left + b.right) / 2 - ax, dy = (b.top + b.bottom) / 2 - ay;
            const ok = direction === "" || (direction === "left" ? dx < -4 : direction === "right" ? dx > 4
                     : direction === "up" ? dy < -4 : dy > 4);
            if (!ok)
                continue;
            if (aligned && (horizontal ? overlap(a.top, a.bottom, b.top, b.bottom) : overlap(a.left, a.right, b.left, b.right)) < 4)
                continue;
            const score = horizontal ? Math.abs(dx) + 2.5 * Math.abs(dy) : Math.abs(dy) + 2.5 * Math.abs(dx);
            if (score < bestScore) { bestScore = score; best = it; }
        }
        return best;
    }

    // Up/down and fast scrolling for a ListView or GridView; true if handled.
    function listKeys(view, action) {
        if (view.count === 0)
            return false;
        let target = view.currentIndex;
        const page = Math.max(1, Math.floor(view.height / Math.max(1, view.currentItem ? view.currentItem.height : 1)) - 1);
        const step = view instanceof GridView ? Math.max(1, Math.floor(view.width / view.cellWidth)) : 1;
        if (action === "up") target -= step;
        else if (action === "down") target += step;
        else if (action === "scroll-up") target = Math.max(0, target - page * step);
        else if (action === "scroll-down") target = Math.min(view.count - 1, target + page * step);
        else if (view instanceof GridView && action === "left" && view.currentIndex % step > 0) target -= 1;
        else if (view instanceof GridView && action === "right" && view.currentIndex % step < step - 1) target += 1;
        else return false;
        if (target < 0 || target >= view.count || target === view.currentIndex) {
            feedback("edge");
        } else {
            view.currentIndex = target;
            feedback("move");
        }
        return true;
    }
}
