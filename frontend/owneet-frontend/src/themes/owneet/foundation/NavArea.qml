// SPDX-License-Identifier: GPL-3.0-or-later
// A focus area with spatial navigation: the D-pad / left stick move the selection among the
// navigable items inside it (see Nav.find), A activates the selected item (its `activated()`
// signal, if any), B emits `cancelled`. Every other button goes on to the parent.
import QtQuick 2.15

FocusScope {
    id: root
    property Item current: null
    property string backFeedback: "back"   // sound of B ("" when closing makes its own)
    signal moved(Item item)
    signal accepted(Item item)
    signal cancelled()

    function focusItem(item) {
        if (!item)
            return;
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
            } else {
                Nav.feedback("edge");
            }
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
            return;
        }
        event.accepted = true;
    }
}
