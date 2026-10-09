// SPDX-License-Identifier: GPL-3.0-or-later
#include "Daemon.h"

#include "Log.h"

#include <QFileInfo>
#include <QJSEngine>
#include <QJsonDocument>
#include <QLocalSocket>
#include <QVariantMap>


namespace owneet {

namespace {
// HTTP/1.0: owneetd answers without chunked encoding and closes the connection at the end, so a
// response is everything read until the socket closes (the event stream stays open).
QByteArray http_request(const QString& method, const QString& path, const QByteArray& body)
{
    QByteArray req = method.toLatin1() + ' ' + path.toUtf8() + " HTTP/1.0\r\nHost: owneetd\r\n";
    if (!body.isEmpty())
        req += "Content-Type: application/json\r\nContent-Length: " + QByteArray::number(body.size()) + "\r\n";
    return req + "\r\n" + body;
}

QVariantMap error_body(const QString& code, const QString& message)
{
    return {{QStringLiteral("error"), QVariantMap {{QStringLiteral("code"), code}, {QStringLiteral("message"), message}}}};
}

constexpr int REQUEST_TIMEOUT_MS = 75000; // Wi-Fi connections wait up to 60 s for the result
} // namespace


Daemon::Daemon(QObject* parent)
    : QObject(parent)
{
    m_retry.setSingleShot(true);
    connect(&m_retry, &QTimer::timeout, this, &Daemon::connectEvents);
}

QString Daemon::socketPath()
{
    const QString env = qEnvironmentVariable("OWNEETD_SOCKET");
    if (!env.isEmpty())
        return env;
    return qEnvironmentVariable("XDG_RUNTIME_DIR") + QStringLiteral("/owneetd.sock");
}

bool Daemon::available() const
{
    return QFileInfo::exists(socketPath());
}

void Daemon::start()
{
    connectEvents();
}

void Daemon::request(const QString& method, const QString& path, const QVariant& body, Callback cb)
{
    auto* socket = new QLocalSocket(this);
    auto* timer = new QTimer(socket);
    auto* data = new QByteArray;
    auto done = std::make_shared<bool>(false);

    auto finish = [=](int status, const QVariant& result) {
        if (*done)
            return;
        *done = true;
        if (cb)
            cb(status, result);
        delete data;
        socket->deleteLater();
    };

    connect(socket, &QLocalSocket::connected, socket, [=] {
        const QByteArray json = body.isValid() && !body.isNull() ? QJsonDocument::fromVariant(body).toJson(QJsonDocument::Compact) : QByteArray();
        socket->write(http_request(method, path, json));
    });
    connect(socket, &QLocalSocket::readyRead, socket, [=] { data->append(socket->readAll()); });
    connect(socket, &QLocalSocket::disconnected, socket, [=] {
        data->append(socket->readAll());
        const int split = data->indexOf("\r\n\r\n");
        const QList<QByteArray> status_line = data->left(data->indexOf("\r\n")).split(' ');
        const int status = status_line.size() > 1 ? status_line.at(1).toInt() : 0;
        if (split < 0 || status == 0) {
            finish(0, error_body(QStringLiteral("daemon.bad_response"), QStringLiteral("unreadable answer from owneetd")));
            return;
        }
        const QByteArray payload = data->mid(split + 4);
        finish(status, payload.isEmpty() ? QVariant() : QJsonDocument::fromJson(payload).toVariant());
    });
    connect(socket, &QLocalSocket::errorOccurred, socket, [=](QLocalSocket::LocalSocketError err) {
        if (err == QLocalSocket::PeerClosedError)
            return; // the normal end of a response: handled by disconnected
        finish(0, error_body(QStringLiteral("daemon.unavailable"), socket->errorString()));
    });
    connect(timer, &QTimer::timeout, socket, [=] {
        finish(0, error_body(QStringLiteral("daemon.timeout"), QStringLiteral("owneetd did not answer")));
    });
    timer->start(REQUEST_TIMEOUT_MS);
    socket->connectToServer(socketPath());
}

Daemon::Callback Daemon::toQml(const QJSValue& callback)
{
    if (!callback.isCallable())
        return nullptr;
    return [this, cb = QJSValue(callback)](int status, const QVariant& data) mutable {
        if (!m_engine)
            return;
        const QJSValue result = cb.call({QJSValue(status), m_engine->toScriptValue(data)});
        if (result.isError())
            Log::warning(LOGMSG("owneetd: error in a QML callback: %1").arg(result.toString()));
    };
}

void Daemon::get(const QString& path, const QJSValue& callback) { request(QStringLiteral("GET"), path, QVariant(), toQml(callback)); }
void Daemon::post(const QString& path, const QVariant& body, const QJSValue& callback) { request(QStringLiteral("POST"), path, body, toQml(callback)); }
void Daemon::put(const QString& path, const QVariant& body, const QJSValue& callback) { request(QStringLiteral("PUT"), path, body, toQml(callback)); }
void Daemon::remove(const QString& path, const QJSValue& callback) { request(QStringLiteral("DELETE"), path, QVariant(), toQml(callback)); }


// ---- Event stream (Server-Sent Events)

void Daemon::connectEvents()
{
    if (m_events)
        return;
    m_events = new QLocalSocket(this);
    m_buffer.clear();
    m_headersDone = false;
    connect(m_events, &QLocalSocket::connected, this, [this] {
        m_events->write(http_request(QStringLiteral("GET"), QStringLiteral("/v1/events"), QByteArray()));
    });
    connect(m_events, &QLocalSocket::readyRead, this, &Daemon::onEventsData);
    connect(m_events, &QLocalSocket::disconnected, this, &Daemon::onEventsGone);
    connect(m_events, &QLocalSocket::errorOccurred, this, [this](QLocalSocket::LocalSocketError) {
        if (m_events && m_events->state() == QLocalSocket::UnconnectedState)
            onEventsGone();
    });
    m_events->connectToServer(socketPath());
}

void Daemon::onEventsData()
{
    m_buffer.append(m_events->readAll());
    if (!m_headersDone) {
        const int split = m_buffer.indexOf("\r\n\r\n");
        if (split < 0)
            return;
        const bool ok = m_buffer.startsWith("HTTP/1.") && m_buffer.mid(9, 3) == "200";
        m_buffer.remove(0, split + 4);
        m_headersDone = true;
        if (!ok) {
            m_events->abort();
            return;
        }
        m_retryMs = 1000;
        setConnected(true);
    }
    // One event per block: "event: TYPE\ndata: {json}\n\n"
    int end;
    while ((end = m_buffer.indexOf("\n\n")) >= 0) {
        const QByteArray block = m_buffer.left(end);
        m_buffer.remove(0, end + 2);
        QString type;
        QByteArray json;
        for (const QByteArray& line : block.split('\n')) {
            if (line.startsWith("event: "))
                type = QString::fromUtf8(line.mid(7)).trimmed();
            else if (line.startsWith("data: "))
                json += line.mid(6);
        }
        if (type.isEmpty())
            continue;
        const QVariantMap ev = QJsonDocument::fromJson(json).toVariant().toMap();
        emit event(type, ev.value(QStringLiteral("data")));
    }
}

void Daemon::onEventsGone()
{
    if (!m_events)
        return;
    m_events->deleteLater();
    m_events = nullptr;
    setConnected(false);
    m_retry.start(m_retryMs);                   // try again: 1 s, 2 s, 4 s … up to 10 s
    m_retryMs = qMin(m_retryMs * 2, 10000);
}

void Daemon::setConnected(bool value)
{
    if (value == m_connected)
        return;
    m_connected = value;
    Log::info(LOGMSG("owneetd: %1").arg(value ? QStringLiteral("connected") : QStringLiteral("not connected")));
    emit connectedChanged();
}

} // namespace owneet
