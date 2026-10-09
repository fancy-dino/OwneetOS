// SPDX-License-Identifier: GPL-3.0-or-later
#include <QtTest/QtTest>

#include "owneet/Daemon.h"

#include <QJSEngine>
#include <QLocalServer>
#include <QLocalSocket>
#include <QTemporaryDir>


// A tiny owneetd: answers one request with `reply` and keeps what it received.
class FakeDaemon : public QObject {
    Q_OBJECT
public:
    QLocalServer server;
    QByteArray received;
    QByteArray reply = "HTTP/1.0 204 No Content\r\n\r\n";

    bool listen(const QString& path)
    {
        connect(&server, &QLocalServer::newConnection, this, [this] {
            QLocalSocket* s = server.nextPendingConnection();
            connect(s, &QLocalSocket::readyRead, s, [this, s] {
                received += s->readAll();
                const int split = received.indexOf("\r\n\r\n");
                if (split < 0)
                    return;
                const QByteArray headers = received.left(split);
                const int at = headers.indexOf("Content-Length: ");
                const int length = at < 0 ? 0 : headers.mid(at + 16, headers.indexOf("\r\n", at) - at - 16).toInt();
                if (received.size() < split + 4 + length)
                    return;
                s->write(reply);
                s->flush();
                s->disconnectFromServer();
            });
        });
        return server.listen(path);
    }
};

class test_Daemon : public QObject {
    Q_OBJECT

private slots:
    void body_from_qml_is_sent_as_json()
    {
        QTemporaryDir dir;
        const QString path = dir.filePath(QStringLiteral("owneetd.sock"));
        FakeDaemon fake;
        QVERIFY(fake.listen(path));
        qputenv("OWNEETD_SOCKET", path.toUtf8());

        QJSEngine engine;
        owneet::Daemon daemon;
        daemon.setEngine(&engine);
        // What QML passes for { volume: 45 }: a QJSValue inside the QVariant
        const QJSValue body = engine.evaluate(QStringLiteral("({ volume: 45 })"));
        daemon.put(QStringLiteral("/v1/audio/volume"), QVariant::fromValue(body));

        QTRY_VERIFY(fake.received.contains("\r\n\r\n"));
        QTRY_VERIFY(fake.received.endsWith("{\"volume\":45}"));
        QVERIFY(fake.received.startsWith("PUT /v1/audio/volume HTTP/1.0\r\n"));
    }

    void status_and_error_reach_the_callback()
    {
        QTemporaryDir dir;
        const QString path = dir.filePath(QStringLiteral("owneetd.sock"));
        FakeDaemon fake;
        fake.reply = "HTTP/1.0 409 Conflict\r\nContent-Type: application/json\r\n\r\n"
                     "{\"error\":{\"code\":\"apps.game_running\",\"message\":\"one game at a time\"}}";
        QVERIFY(fake.listen(path));
        qputenv("OWNEETD_SOCKET", path.toUtf8());

        owneet::Daemon daemon;
        int status = -1;
        QString code;
        daemon.request(QStringLiteral("POST"), QStringLiteral("/v1/apps/launch"), QVariantMap {{QStringLiteral("id"), QStringLiteral("x")}},
                       [&](int s, const QVariant& data) {
                           status = s;
                           code = data.toMap().value(QStringLiteral("error")).toMap().value(QStringLiteral("code")).toString();
                       });
        QTRY_COMPARE(status, 409);
        QCOMPARE(code, QStringLiteral("apps.game_running"));
    }

    void unreachable_daemon_gives_status_0()
    {
        qputenv("OWNEETD_SOCKET", "/nonexistent/owneetd.sock");
        owneet::Daemon daemon;
        QVERIFY(!daemon.available());
        int status = -1;
        daemon.request(QStringLiteral("GET"), QStringLiteral("/v1/status"), QVariant(), [&](int s, const QVariant&) { status = s; });
        QTRY_COMPARE(status, 0);
    }
};


QTEST_MAIN(test_Daemon)
#include "test_Daemon.moc"
