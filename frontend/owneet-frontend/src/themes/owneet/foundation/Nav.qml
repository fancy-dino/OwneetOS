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
//     confirm, back, section, tab, toggle-on, toggle-off, sheet-open, sheet-close. The sounds
//     listen to `played`: one per button press, the most specific one ("confirm" only when the
//     action that follows, e.g. opening a window, reports nothing of its own).
pragma Singleton
import QtQuick 2.15

QtObject {
    signal played(string kind)

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
        if (event.isAutoRepeat) // a held button acts once (no repeated launches or closes)
            return "";
        if (k.isAccept(event)) return "accept";
        if (k.isCancel(event)) return "back";
        if (k.isDetails(event)) return "secondary";
        if (k.isFilters(event)) return "page";
        if (k.isMenu(event)) return "options";
        if (k.isPrevPage(event)) return "section-prev";
        if (k.isNextPage(event)) return "section-next";
        if (k.isPageUp(event)) return "filter-prev";
        if (k.isPageDown(event)) return "filter-next";
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

    // The nearest navigable item in `direction` from `from`, or null at the edge. Distance along
    // the direction counts once, distance across it 2.5 times (as in the approved demo 3.3).
    function find(root, from, direction) {
        const items = navigables(root);
        if (!from)
            return items.length ? items[0] : null;
        const a = from.mapToItem(root, from.width / 2, from.height / 2);
        const horizontal = direction === "left" || direction === "right";
        let best = null, bestScore = Infinity;
        for (const it of items) {
            if (it === from)
                continue;
            const b = it.mapToItem(root, it.width / 2, it.height / 2);
            const dx = b.x - a.x, dy = b.y - a.y;
            const ok = direction === "left" ? dx < -4 : direction === "right" ? dx > 4
                     : direction === "up" ? dy < -4 : dy > 4;
            if (!ok)
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
