// SPDX-License-Identifier: GPL-3.0-or-later
// Client of owneetd, the OwneetOS system daemon (roadmap 3.8, docs/daemon-design.md): HTTP + JSON
// over its Unix socket ($XDG_RUNTIME_DIR/owneetd.sock), and its event stream (/v1/events), which
// reconnects by itself. QML sees it as `owneetd`:
//   owneetd.get("/v1/apps", (status, data) => …)     also post(path, body, cb), put, remove
//   owneetd.event(type, data)                        every event of the stream
//   owneetd.connected                                the event stream is up

#pragma once

#include <QJSValue>
#include <QObject>
#include <QTimer>
#include <QVariant>

#include <functional>

class QJSEngine;
class QLocalSocket;


namespace owneet {
class Daemon : public QObject {
    Q_OBJECT
    Q_PROPERTY(bool connected READ connected NOTIFY connectedChanged)

public:
    using Callback = std::function<void(int status, const QVariant& data)>;

    explicit Daemon(QObject* parent = nullptr);

    /// $OWNEETD_SOCKET, or $XDG_RUNTIME_DIR/owneetd.sock
    static QString socketPath();
    /// The daemon's socket exists (owneetd runs in the console session, not in every test setup).
    bool available() const;
    bool connected() const { return m_connected; }
    /// The QML engine, to hand results to QML callbacks
    void setEngine(QJSEngine* engine) { m_engine = engine; }
    /// Starts following the event stream (reconnecting whenever it drops)
    void start();

    /// One request. `cb` gets the HTTP status (0 when owneetd cannot be reached) and the JSON
    /// body (errors: {"error": {"code", "message"}}).
    void request(const QString& method, const QString& path, const QVariant& body, Callback cb);

    Q_INVOKABLE void get(const QString& path, const QJSValue& callback = QJSValue());
    Q_INVOKABLE void post(const QString& path, const QVariant& body = QVariant(), const QJSValue& callback = QJSValue());
    Q_INVOKABLE void put(const QString& path, const QVariant& body, const QJSValue& callback = QJSValue());
    Q_INVOKABLE void remove(const QString& path, const QJSValue& callback = QJSValue());

signals:
    void connectedChanged();
    void event(const QString& type, const QVariant& data);

private:
    QJSEngine* m_engine = nullptr;
    QLocalSocket* m_events = nullptr;
    QByteArray m_buffer;
    bool m_headersDone = false;
    bool m_connected = false;
    QTimer m_retry;
    int m_retryMs = 1000;

    void connectEvents();
    void onEventsData();
    void onEventsGone();
    void setConnected(bool);
    Callback toQml(const QJSValue& callback);
};
} // namespace owneet
