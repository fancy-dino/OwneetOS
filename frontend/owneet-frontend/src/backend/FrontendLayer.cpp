// Pegasus Frontend
// Copyright (C) 2017  Mátyás Mustoha
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <http://www.gnu.org/licenses/>.


#include "FrontendLayer.h"

#include "AppSettings.h"
#include "Paths.h"
#include "owneet/Daemon.h"
#include "owneet/I18n.h"
#include "owneet/Storage.h"
#include "owneet/SystemInfo.h"
#include "imggen/BlurhashProvider.h"
#include "platform/GamescopeTag.h"
#include "utils/DiskCachedNAM.h"

#ifdef Q_OS_ANDROID
#include "platform/AndroidAppIconProvider.h"
#endif

#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QQmlNetworkAccessManagerFactory>


namespace {

class DiskCachedNAMFactory : public QQmlNetworkAccessManagerFactory {
public:
    QNetworkAccessManager* create(QObject* parent) override;
};

QNetworkAccessManager* DiskCachedNAMFactory::create(QObject* parent)
{
    return utils::create_disc_cached_nam(parent);
}

} // namespace


FrontendLayer::FrontendLayer(QObject* const api_public, QObject* const api_private, QObject* parent)
    : QObject(parent)
    , m_api_public(api_public)
    , m_api_private(api_private)
    , m_engine(nullptr)
    , m_i18n(new owneet::I18n(owneet::I18n::defaultDirs(), AppSettings::general.locale, this))
    , m_storage(new owneet::Storage(this))
    , m_daemon(new owneet::Daemon(this))
    , m_system_info(new owneet::SystemInfo(this))
{
    // Note: the pointer to the Api is non-owning and constant during the runtime

    // OwneetOS: the chosen language is saved in the settings file (`general.locale`)
    connect(m_i18n, &owneet::I18n::languageChanged, this, [this] {
        AppSettings::general.locale = m_i18n->language();
        AppSettings::save_config();
    });
}

void FrontendLayer::rebuild()
{
    if (m_engine) // OwneetOS: already loaded (the interface is no longer unloaded during games)
        return;

    m_engine = new QQmlApplicationEngine(this);
    m_engine->addImportPath(QStringLiteral("lib/qml"));
    m_engine->addImportPath(QStringLiteral("qml"));
    m_engine->setNetworkAccessManagerFactory(new DiskCachedNAMFactory);

    m_engine->addImageProvider(QStringLiteral("blurhash"), new BlurhashProvider);
#ifdef Q_OS_ANDROID
    m_engine->addImageProvider(QStringLiteral("androidicons"), new AndroidAppIconProvider);
#endif

    m_engine->rootContext()->setContextProperty(QStringLiteral("api"), m_api_public);
    m_engine->rootContext()->setContextProperty(QStringLiteral("Api"), m_api_public);
    m_engine->rootContext()->setContextProperty(QStringLiteral("Internal"), m_api_private);
    m_engine->rootContext()->setContextProperty(QStringLiteral("i18n"), m_i18n); // OwneetOS
    m_engine->rootContext()->setContextProperty(QStringLiteral("storage"), m_storage); // OwneetOS
    m_engine->rootContext()->setContextProperty(QStringLiteral("owneetd"), m_daemon); // OwneetOS
    m_engine->rootContext()->setContextProperty(QStringLiteral("systemInfo"), m_system_info); // OwneetOS
    m_daemon->setEngine(m_engine);
    m_daemon->start();
    m_engine->load(QUrl(QStringLiteral("qrc:/frontend/main.qml")));
    platform::tag_windows_for_gamescope(m_engine->rootObjects()); // OwneetOS

    emit rebuildComplete();
}

void FrontendLayer::teardown()
{
    Q_ASSERT(m_engine);

    // signal forwarding
    connect(m_engine, &QQmlApplicationEngine::destroyed,
            this, &FrontendLayer::teardownComplete);

    m_engine->deleteLater();
    m_engine = nullptr;
}

void FrontendLayer::clearCache()
{
    Q_ASSERT(m_engine);
    m_engine->clearComponentCache();
}
