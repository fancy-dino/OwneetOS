// SPDX-License-Identifier: GPL-3.0-or-later
#include <QtTest/QtTest>

#include "owneet/I18n.h"

#include <QSignalSpy>
#include <QTemporaryDir>


class test_I18n : public QObject {
    Q_OBJECT

private:
    QTemporaryDir m_system;
    QTemporaryDir m_user;

    void write(const QTemporaryDir& dir, const QString& name, const QByteArray& json)
    {
        QFile file(dir.filePath(name));
        QVERIFY(file.open(QIODevice::WriteOnly));
        file.write(json);
    }

private slots:
    void initTestCase()
    {
        write(m_system, QStringLiteral("en.json"), R"({
            "@meta": { "language": "English", "plural": "one-other" },
            "hello": "Hello {name}",
            "only.en": "English only",
            "games": { "one": "{n} game", "other": "{n} games" }
        })");
        write(m_system, QStringLiteral("it.json"), R"({
            "@meta": { "language": "Italiano" },
            "hello": "Ciao {name}",
            "games": { "one": "{n} gioco", "other": "{n} giochi" }
        })");
        write(m_system, QStringLiteral("pl.json"), R"({
            "@meta": { "language": "Polski", "plural": "polish" },
            "games": { "one": "{n} gra", "few": "{n} gry", "many": "{n} gier" }
        })");
        write(m_system, QStringLiteral("broken.json"), "{ not json");
        // a user folder overrides a system language
        write(m_user, QStringLiteral("it.json"), R"({ "@meta": { "language": "Italiano" }, "hello": "Salve {name}" })");
    }

    void default_is_english()
    {
        owneet::I18n i18n({m_system.path()}, QString());
        QCOMPARE(i18n.language(), QStringLiteral("en"));
        QCOMPARE(i18n.tr(QStringLiteral("hello"), {{QStringLiteral("name"), QStringLiteral("Ada")}}),
                 QStringLiteral("Hello Ada"));
    }

    void unknown_preference_falls_back_to_english()
    {
        owneet::I18n i18n({m_system.path()}, QStringLiteral("xx"));
        QCOMPARE(i18n.language(), QStringLiteral("en"));
    }

    void fallback_to_english_then_key()
    {
        owneet::I18n i18n({m_system.path()}, QStringLiteral("it"));
        QCOMPARE(i18n.tr(QStringLiteral("hello"), {{QStringLiteral("name"), QStringLiteral("Ada")}}),
                 QStringLiteral("Ciao Ada"));
        QCOMPARE(i18n.tr(QStringLiteral("only.en")), QStringLiteral("English only"));
        QCOMPARE(i18n.tr(QStringLiteral("missing.key")), QStringLiteral("missing.key"));
    }

    void plurals()
    {
        owneet::I18n i18n({m_system.path()}, QStringLiteral("pl"));
        QCOMPARE(i18n.trn(QStringLiteral("games"), 1), QStringLiteral("1 gra"));
        QCOMPARE(i18n.trn(QStringLiteral("games"), 3), QStringLiteral("3 gry"));
        QCOMPARE(i18n.trn(QStringLiteral("games"), 12), QStringLiteral("12 gier"));
        QCOMPARE(i18n.trn(QStringLiteral("games"), 22), QStringLiteral("22 gry"));
        i18n.setLanguage(QStringLiteral("it"));
        QCOMPARE(i18n.trn(QStringLiteral("games"), 1), QStringLiteral("1 gioco"));
        QCOMPARE(i18n.trn(QStringLiteral("games"), 0), QStringLiteral("0 giochi"));
    }

    void plural_rules()
    {
        QCOMPARE(owneet::I18n::pluralCategory(QStringLiteral("french"), 0), QStringLiteral("one"));
        QCOMPARE(owneet::I18n::pluralCategory(QStringLiteral("east-slavic"), 21), QStringLiteral("one"));
        QCOMPARE(owneet::I18n::pluralCategory(QStringLiteral("east-slavic"), 11), QStringLiteral("many"));
        QCOMPARE(owneet::I18n::pluralCategory(QStringLiteral("czech"), 4), QStringLiteral("few"));
        QCOMPARE(owneet::I18n::pluralCategory(QStringLiteral("none"), 1), QStringLiteral("other"));
    }

    void languages_and_switching()
    {
        owneet::I18n i18n({m_system.path(), m_user.path()}, QString());
        const QVariantList langs = i18n.languages();
        QCOMPARE(langs.size(), 3); // broken.json is skipped
        QCOMPARE(langs.first().toMap().value(QStringLiteral("tag")).toString(), QStringLiteral("en"));

        QSignalSpy spy(&i18n, &owneet::I18n::languageChanged);
        i18n.setLanguage(QStringLiteral("xx")); // not installed: ignored
        QCOMPARE(spy.count(), 0);
        i18n.setLanguage(QStringLiteral("it"));
        QCOMPARE(spy.count(), 1);
        QCOMPARE(i18n.revision(), 1);
        QCOMPARE(i18n.tr(QStringLiteral("hello"), {{QStringLiteral("name"), QStringLiteral("Ada")}}),
                 QStringLiteral("Salve Ada"));
    }
};


QTEST_MAIN(test_I18n)
#include "test_I18n.moc"
