// SPDX-License-Identifier: GPL-3.0-or-later
// OwneetOS addition to the Pegasus Frontend source.

#include "CursorHider.h"

#include <QCursor>
#include <QEvent>
#include <QGuiApplication>


namespace platform {

CursorHider::CursorHider(QObject* parent)
    : QObject(parent)
    , m_hidden(false)
{
    hide();
}

void CursorHider::hide()
{
    if (m_hidden)
        return;
    QGuiApplication::setOverrideCursor(QCursor(Qt::BlankCursor));
    m_hidden = true;
}

void CursorHider::show()
{
    if (!m_hidden)
        return;
    QGuiApplication::restoreOverrideCursor();
    m_hidden = false;
}

bool CursorHider::eventFilter(QObject* watched, QEvent* event)
{
    switch (event->type()) {
        case QEvent::MouseMove:
        case QEvent::MouseButtonPress:
        case QEvent::Wheel:
            show();
            break;
        case QEvent::KeyPress:
            hide();
            break;
        case QEvent::Expose:
            // A new window does not take the cursor set before it existed: set it again.
            if (m_hidden)
                QGuiApplication::changeOverrideCursor(QCursor(Qt::BlankCursor));
            break;
        default:
            break;
    }
    return QObject::eventFilter(watched, event);
}

} // namespace platform
