target_sources(pegasus-backend PRIVATE
    PowerCommands.h
    TerminalKbd.cpp
    TerminalKbd.h
)


if(PEGASUS_ON_ANDROID)
    target_sources(pegasus-backend PRIVATE
        AndroidAppIconProvider.cpp
        AndroidAppIconProvider.h
        AndroidHelpers.cpp
        AndroidHelpers.h
    )
endif()


# OwneetOS: tag the interface's windows for gamescope (X11 only)

if(PEGASUS_ON_X11)
    find_library(XCB_LIBRARY xcb REQUIRED)
    target_sources(pegasus-backend PRIVATE GamescopeTag.h GamescopeTag_x11.cpp)
    target_link_libraries(pegasus-backend PRIVATE "${XCB_LIBRARY}")
else()
    target_sources(pegasus-backend PRIVATE GamescopeTag.h GamescopeTag_unimpl.cpp)
endif()


# Select power manager implementation

if(PEGASUS_ON_X11 OR PEGASUS_ON_EGLFS)
    target_sources(pegasus-backend PRIVATE PowerCommands_linux.cpp)
    set(POWER_IMPL_NAME "Linux" CACHE INTERNAL "")
elseif(PEGASUS_ON_WINDOWS)
    target_sources(pegasus-backend PRIVATE PowerCommands_win.cpp)
    set(POWER_IMPL_NAME "Windows" CACHE INTERNAL "")
elseif(PEGASUS_ON_MACOS)
    target_sources(pegasus-backend PRIVATE PowerCommands_mac.cpp)
    set(POWER_IMPL_NAME "macOS" CACHE INTERNAL "")
else()
    target_sources(pegasus-backend PRIVATE PowerCommands_unimpl.cpp)
    set(POWER_IMPL_NAME "Disabled" CACHE INTERNAL "")
endif()
