// SPDX-License-Identifier: GPL-3.0-or-later
// A focus area with spatial navigation: the D-pad / left stick move the selection among the
// navigable items inside it (see Nav.find), A activates the selected item (its `activated()`
// signal, if any), B emits `cancelled`. Other buttons are offered to `otherAction` (set
// event.accepted to keep them), then go on to the parent.
import QtQuick 2.15

FocusScope {
    id: root
    property Item current: null
    property string backFeedback: "back"   // sound of B ("" when closing makes its own)
    // The page's scrolling content (PROJECT_RULES.md section 9.1: the right stick scrolls the
    // page); up / down scroll it too when there is nothing more to select that way.
    property Flickable scrollTarget: null
    function scrollBy(dy) {
        const f = scrollTarget;
        if (!f || f.contentHeight <= f.height) { Nav.feedback("edge"); return; }
        const top = f.originY - f.topMargin;
        const bottom = f.originY + f.contentHeight + f.bottomMargin - f.height;
        const next = Math.max(top, Math.min(bottom, f.contentY + dy));
        if (next === f.contentY) { Nav.feedback("edge"); return; }
        f.contentY = next;
        Nav.feedback("move");
    }
    signal moved(Item item)
    signal accepted(Item item)
    signal cancelled()
    signal otherAction(string action, var event)    // secondary, options, page, filters…

    function focusItem(item) {
        if (!item)
            return;
        const group = Nav.groupOf(root, item);
        if (group)
            group.remembered = item;    // NavGroup: come back here when entering sideways
        current = item;
        item.forceActiveFocus();
        moved(item);
    }
    function focusFirst() { focusItem(Nav.find(root, null, "down")); }

    Keys.onPressed: {
        const a = Nav.action(event);
        if (Nav.isDirection(a)) {
            const next = Nav.find(root, current, a);
            if (next) {
                Nav.feedback("move");
                focusItem(next);
            } else if (scrollTarget && (a === "up" || a === "down")) {
                scrollBy((a === "up" ? -1 : 1) * Theme.px(60));
            } else {
                Nav.feedback("edge");
            }
        } else if ((a === "scroll-up" || a === "scroll-down") && scrollTarget) {
            scrollBy((a === "scroll-up" ? -1 : 1) * scrollTarget.height * 0.4);
        } else if (a === "accept" && current) {
            Nav.feedback(current.feedbackKind || "confirm");
            if (typeof current.activated === "function")
                current.activated();
            accepted(current);
        } else if (a === "back") {
            if (backFeedback !== "")
                Nav.feedback(backFeedback);
            cancelled();
        } else {
            if (a !== "")
                otherAction(a, event);
            return;
        }
        event.accepted = true;
    }
}
