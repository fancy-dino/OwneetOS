// SPDX-License-Identifier: GPL-3.0-or-later
// OwneetOS interface translations (roadmap step 3.4, docs/translating.md).
//
// Each language is one JSON file named after its language tag (en.json, it.json, pt-BR.json…)
// in one of the language folders; a new language needs no code change. English is the default
// language and the fallback for every missing message.

#pragma once

#include <QJsonObject>
#include <QObject>
#include <QStringList>
#include <QVariantList>
#include <QVariantMap>

#include <map>


namespace owneet {
class I18n : public QObject {
    Q_OBJECT
    Q_PROPERTY(QString language READ language WRITE setLanguage NOTIFY languageChanged)
    Q_PROPERTY(QVariantList languages READ languages CONSTANT)
    // Read by QML wrappers so that translated bindings update when the language changes
    Q_PROPERTY(int revision READ revision NOTIFY languageChanged)

public:
    static const QString DEFAULT_LANGUAGE; // "en"

    /// Loads every *.json file of `dirs`; a later folder overrides an earlier one for the same
    /// language. `preferred` is the saved choice: if it is not available, English is used.
    explicit I18n(const QStringList& dirs, const QString& preferred, QObject* parent = nullptr);

    /// Language folders: $OWNEET_I18N_DIR (colon-separated, for development) if set, otherwise
    /// /usr/share/owneet-frontend/i18n and then the user's <config dir>/i18n.
    static QStringList defaultDirs();

    /// The message `key` in the current language, else in English, else the key itself.
    /// {name} placeholders are replaced from `args`.
    Q_INVOKABLE QString tr(const QString& key, const QVariantMap& args = {}) const;
    /// Like tr(), for messages with plural forms; {n} is replaced by `n`.
    Q_INVOKABLE QString trn(const QString& key, int n, const QVariantMap& args = {}) const;

    const QString& language() const { return m_current; }
    void setLanguage(const QString& tag);
    QVariantList languages() const;
    int revision() const { return m_revision; }

    /// CLDR plural category ("one", "few", "many", "other") of `n` for a plural rule family.
    static QString pluralCategory(const QString& rule, int n);

signals:
    void languageChanged();

private:
    struct Language {
        QString name;   // native name, e.g. "Italiano"
        QString plural; // plural rule family, e.g. "one-other"
        QJsonObject messages;
    };
    std::map<QString, Language> m_languages;
    QString m_current;
    int m_revision = 0;

    void load_dir(const QString& dir);
    const Language* find(const QString& tag) const;
    QString format(QString text, const QVariantMap& args) const;
};
} // namespace owneet
