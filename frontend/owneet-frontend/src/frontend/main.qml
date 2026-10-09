// Pegasus Frontend
// Copyright (C) 2017-2018  Mátyás Mustoha
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


import "qrc:/qmlutils" as PegasusUtils
import QtQuick 2.8
import QtQuick.Window 2.2


Window {
    id: appWindow
    visible: true
    // OwneetOS: born at the screen's size (no 1280x720 frame before going full screen)
    width: Screen.width
    height: Screen.height
    title: "OwneetOS"
    color: "#0E1424"

    visibility: Internal.settings.fullscreen
                ? Window.FullScreen : Window.AutomaticVisibility

    onClosing: {
        theme.source = "";
        Internal.system.quit();
    }

    // OwneetOS fonts (SIL Open Font License, see assets/fonts/owneet): Lexend for interface
    // text, Bricolage Grotesque for titles. Pegasus's Roboto fonts are not bundled.
    FontLoader { id: sansFont; source: "/fonts/owneet/Lexend-Regular.ttf" }
    FontLoader { source: "/fonts/owneet/Lexend-Light.ttf" }
    FontLoader { id: sansBoldFont; source: "/fonts/owneet/Lexend-Medium.ttf" }
    FontLoader { source: "/fonts/owneet/Lexend-SemiBold.ttf" }
    FontLoader { id: condensedFont; source: "/fonts/owneet/BricolageGrotesque-SemiBold.ttf" }
    FontLoader { id: condensedBoldFont; source: "/fonts/owneet/BricolageGrotesque-Bold.ttf" }
    FontLoader { source: "/fonts/owneet/BricolageGrotesque-ExtraBold.ttf" }
    readonly property alias monoFont: sansFont // no monospace font in OwneetOS


    // a globally avalable utility object
    QtObject {
        id: global

        readonly property real winScale: Math.min(width / 1280.0, height / 720.0)

        property QtObject fonts: QtObject {
            readonly property string sans: sansFont.name
            readonly property string sansBold: sansBoldFont.name
            readonly property string condensed: condensedFont.name
            readonly property string condensedBold: condensedBoldFont.name
            readonly property string mono: monoFont.name
        }
    }


    // legacy global objects
    QtObject {
        id: globalFonts
        readonly property string sans: global.fonts.sans
        readonly property string condensed: global.fonts.condensed
    }
    function vpx(value) {
        return global.winScale * value;
    }


    // the main content
    FocusScope {
        id: content
        anchors.fill: parent
        enabled: focus

        Loader {
            id: theme
            anchors.fill: parent

            focus: true
            enabled: focus

            readonly property url apiThemePath: Internal.settings.themes.currentQmlPath

            // OwneetOS: the theme also handles "no games yet" (no Pegasus message screen).
            function getThemeFile() {
                if (Internal.scanner.running)
                    return "";

                return apiThemePath;
            }
            onApiThemePathChanged: source = Qt.binding(getThemeFile)

            // OwneetOS: Pegasus's main menu is not used (OwneetOS screens replace it).
            Keys.onPressed: {
                if (event.key === Qt.Key_F5) {
                    event.accepted = true;

                    theme.source = "";
                    Internal.meta.clearQMLCache();
                    theme.source = Qt.binding(getThemeFile);
                }
            }

            source: getThemeFile()
            asynchronous: true
            onStatusChanged: {
                if (status == Loader.Error)
                    source = "messages/ThemeError.qml";
            }
            onLoaded: item.focus = focus
            onFocusChanged: if (item) item.focus = focus
        }

        Loader {
            id: mainMenu
            anchors.fill: parent

            source: "MenuLayer.qml"
            asynchronous: true
            active: false // OwneetOS: Pegasus's main menu is not used

            onLoaded: item.focus = focus
            onFocusChanged: if (item) item.focus = focus
            enabled: focus
        }
        Connections {
            target: mainMenu.item

            function onClose() { theme.focus = true; }

            function onRequestShutdown() {
                powerDialog.source = "dialogs/ShutdownDialog.qml"
                powerDialog.focus = true;
            }
            function onRequestSuspend() {
                Internal.system.suspend()
            }                        
            function onRequestReboot() {
                powerDialog.source = "dialogs/RebootDialog.qml"
                powerDialog.focus = true;
            }
            function onRequestQuit() {
                theme.source = "";
                Internal.system.quit();
            }
        }
        PegasusUtils.HorizontalSwipeArea {
            id: menuSwipe
            enabled: false // OwneetOS: Pegasus's main menu is not used

            width: vpx(40)
            height: parent.height
            anchors.right: parent.right

            onSwipeLeft: {
                if (!mainMenu.focus)
                    mainMenu.focus = true;
            }
        }
    }


    Loader {
        id: powerDialog
        anchors.fill: parent
    }
    Connections {
        target: powerDialog.item
        function onCancel() { content.focus = true; }
    }


    Loader {
        id: multifileSelector
        anchors.fill: parent
    }
    Connections {
        target: multifileSelector.item
        function onCancel() { content.focus = true; }
    }


    Loader {
        id: genericMessage
        anchors.fill: parent
    }
    Connections {
        target: genericMessage.item
        function onClose() { content.focus = true; }
    }


    Connections {
        target: api
        function onEventSelectGameFile(game) {
            multifileSelector.setSource("dialogs/MultifileSelector.qml", {"game": game})
            multifileSelector.focus = true;
        }
        // OwneetOS: the theme shows launch errors itself, translated (no Pegasus dialog)
        function onEventLaunchError(msg) {}
    }
    Connections {
        target: Internal.scanner
        function onRunningChanged() {
            if (Internal.scanner.running)
                splashScreen.focus = true;
        }
    }


    SplashLayer {
        id: splashScreen
        focus: true
        enabled: false
        visible: focus

        readonly property bool dataLoading: Internal.scanner.running
        readonly property bool skinLoading: theme.status === Loader.Null || theme.status === Loader.Loading

        showDataProgressText: dataLoading
        progress: Internal.scanner.progress
        stage: Internal.scanner.stage

        function hideMaybe() {
            if (focus && !dataLoading && !skinLoading) {
                content.focus = true;
                Internal.scanner.reset();
            }
        }

        onSkinLoadingChanged: hideMaybe()
        onDataLoadingChanged: hideMaybe()
    }
}
