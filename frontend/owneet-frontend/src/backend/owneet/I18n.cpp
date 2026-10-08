// SPDX-License-Identifier: GPL-3.0-or-later
#include "I18n.h"

#include "Log.h"
#include "Paths.h"

#include <QDir>
#include <QFile>
#include <QJsonDocument>
#include <QJsonParseError>
#include <QLocale>

#include <algorithm>


namespace owneet {

const QString I18n::DEFAULT_LANGUAGE = QStringLiteral("en");

namespace {
const QString META_KEY = QStringLiteral("@meta");
const QStringList PLURAL_RULES = {
    QStringLiteral("one-other"), QStringLiteral("french"), QStringLiteral("none"),
    QStringLiteral("east-slavic"), QStringLiteral("polish"), QStringLiteral("czech"),
};
} // namespace


I18n::I18n(const QStringList& dirs, const QString& preferred, QObject* parent)
    : QObject(parent)
{
    for (const QString& dir : dirs)
        load_dir(dir);

    if (m_languages.find(DEFAULT_LANGUAGE) == m_languages.end())
        Log::warning(LOGMSG("Translations: no English messages found; keys are shown instead"));

    m_current = m_languages.count(preferred) ? preferred : DEFAULT_LANGUAGE;
    QLocale::setDefault(QLocale(m_current));
    Log::info(LOGMSG("Translations: language set to `%1`").arg(m_current));
}

QStringList I18n::defaultDirs()
{
    const QString env = qEnvironmentVariable("OWNEET_I18N_DIR");
    if (!env.isEmpty())
        return env.split(QLatin1Char(':'), Qt::SkipEmptyParts);

    return {
        QStringLiteral("/usr/share/owneet-frontend/i18n"),
        paths::writableConfigDir() + QStringLiteral("/i18n"),
    };
}

void I18n::load_dir(const QString& dir)
{
    const QFileInfoList files = QDir(dir).entryInfoList({QStringLiteral("*.json")}, QDir::Files, QDir::Name);
    for (const QFileInfo& info : files) {
        QFile file(info.filePath());
        if (!file.open(QIODevice::ReadOnly)) {
            Log::warning(LOGMSG("Translations: cannot read `%1`").arg(info.filePath()));
            continue;
        }
        QJsonParseError error;
        const QJsonDocument doc = QJsonDocument::fromJson(file.readAll(), &error);
        if (!doc.isObject()) {
            Log::warning(LOGMSG("Translations: `%1` is not valid: %2")
                .arg(info.filePath(), error.errorString()));
            continue;
        }

        QJsonObject messages = doc.object();
        const QJsonObject meta = messages.take(META_KEY).toObject();
        Language lang;
        lang.messages = std::move(messages);
        lang.name = meta.value(QStringLiteral("language")).toString(info.baseName());
        lang.plural = meta.value(QStringLiteral("plural")).toString(QStringLiteral("one-other"));
        if (!PLURAL_RULES.contains(lang.plural)) {
            Log::warning(LOGMSG("Translations: `%1`: unknown plural rule `%2`, using `one-other`")
                .arg(info.filePath(), lang.plural));
            lang.plural = QStringLiteral("one-other");
        }
        m_languages[info.baseName()] = std::move(lang);
    }
}

const I18n::Language* I18n::find(const QString& tag) const
{
    const auto it = m_languages.find(tag);
    return it != m_languages.end() ? &it->second : nullptr;
}

QString I18n::format(QString text, const QVariantMap& args) const
{
    const QLocale locale(m_current);
    for (auto it = args.cbegin(); it != args.cend(); ++it) {
        const QVariant& value = it.value();
        QString shown = value.toString();
        if (value.type() == QVariant::Int || value.type() == QVariant::LongLong || value.type() == QVariant::Double) {
            const double d = value.toDouble();
            shown = d == static_cast<qlonglong>(d)
                ? locale.toString(static_cast<qlonglong>(d))  // digit grouping of the language
                : locale.toString(d);
        }
        text.replace(QLatin1Char('{') + it.key() + QLatin1Char('}'), shown);
    }
    return text;
}

QString I18n::tr(const QString& key, const QVariantMap& args) const
{
    for (const QString& tag : {m_current, DEFAULT_LANGUAGE}) {
        const Language* lang = find(tag);
        if (!lang)
            continue;
        const QJsonValue value = lang->messages.value(key);
        if (value.isString())
            return format(value.toString(), args);
        if (value.isObject()) // a plural message used without a number
            return format(value.toObject().value(QStringLiteral("other")).toString(key), args);
    }
    return key;
}

QString I18n::trn(const QString& key, int n, const QVariantMap& args) const
{
    QVariantMap all = args;
    if (!all.contains(QStringLiteral("n")))
        all.insert(QStringLiteral("n"), n);

    for (const QString& tag : {m_current, DEFAULT_LANGUAGE}) {
        const Language* lang = find(tag);
        if (!lang)
            continue;
        const QJsonValue value = lang->messages.value(key);
        if (value.isString())
            return format(value.toString(), all);
        if (value.isObject()) {
            const QJsonObject forms = value.toObject();
            const QJsonValue form = forms.value(pluralCategory(lang->plural, n));
            return format(form.isString() ? form.toString() : forms.value(QStringLiteral("other")).toString(key), all);
        }
    }
    return key;
}

void I18n::setLanguage(const QString& tag)
{
    if (tag == m_current || !find(tag))
        return;

    m_current = tag;
    m_revision++;
    QLocale::setDefault(QLocale(m_current));
    Log::info(LOGMSG("Translations: language set to `%1`").arg(m_current));
    emit languageChanged();
}

QVariantList I18n::languages() const
{
    // English first (the default), then the others by their native name
    std::vector<std::pair<QString, QString>> list;
    for (const auto& entry : m_languages)
        list.emplace_back(entry.first, entry.second.name);
    std::sort(list.begin(), list.end(), [](const auto& a, const auto& b) {
        if ((a.first == DEFAULT_LANGUAGE) != (b.first == DEFAULT_LANGUAGE))
            return a.first == DEFAULT_LANGUAGE;
        return QString::localeAwareCompare(a.second, b.second) < 0;
    });

    QVariantList out;
    for (const auto& entry : list)
        out.append(QVariantMap {{QStringLiteral("tag"), entry.first}, {QStringLiteral("name"), entry.second}});
    return out;
}

QString I18n::pluralCategory(const QString& rule, int n)
{
    const int abs = n < 0 ? -n : n;
    const int mod10 = abs % 10;
    const int mod100 = abs % 100;
    const bool few_ending = mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14);

    if (rule == QLatin1String("none"))
        return QStringLiteral("other");
    if (rule == QLatin1String("french"))
        return abs <= 1 ? QStringLiteral("one") : QStringLiteral("other");
    if (rule == QLatin1String("east-slavic")) {
        if (mod10 == 1 && mod100 != 11)
            return QStringLiteral("one");
        return few_ending ? QStringLiteral("few") : QStringLiteral("many");
    }
    if (rule == QLatin1String("polish")) {
        if (abs == 1)
            return QStringLiteral("one");
        return few_ending ? QStringLiteral("few") : QStringLiteral("many");
    }
    if (rule == QLatin1String("czech")) {
        if (abs == 1)
            return QStringLiteral("one");
        return abs >= 2 && abs <= 4 ? QStringLiteral("few") : QStringLiteral("other");
    }
    return abs == 1 ? QStringLiteral("one") : QStringLiteral("other"); // one-other
}

} // namespace owneet
