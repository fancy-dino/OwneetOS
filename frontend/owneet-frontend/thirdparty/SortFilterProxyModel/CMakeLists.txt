find_package(Qt5 REQUIRED Qml)

set(CMAKE_AUTOMOC ON)
set(CMAKE_INCLUDE_CURRENT_DIR ON)

add_library(SortFilterProxyModel
    qqmlsortfilterproxymodel.cpp
    filters/filter.cpp
    filters/filtercontainer.cpp
    filters/rolefilter.cpp
    filters/valuefilter.cpp
    filters/indexfilter.cpp
    filters/regexpfilter.cpp
    filters/rangefilter.cpp
    filters/expressionfilter.cpp
    filters/filtercontainerfilter.cpp
    filters/anyoffilter.cpp
    filters/alloffilter.cpp
    filters/filtersqmltypes.cpp
    sorters/sorter.cpp
    sorters/sortercontainer.cpp
    sorters/rolesorter.cpp
    sorters/stringsorter.cpp
    sorters/expressionsorter.cpp
    sorters/sortersqmltypes.cpp
    proxyroles/proxyrole.cpp
    proxyroles/proxyrolecontainer.cpp
    proxyroles/joinrole.cpp
    proxyroles/switchrole.cpp
    proxyroles/expressionrole.cpp
    proxyroles/proxyrolesqmltypes.cpp
    proxyroles/singlerole.cpp
    proxyroles/regexprole.cpp
    sorters/filtersorter.cpp
    proxyroles/filterrole.cpp
)

target_link_libraries(SortFilterProxyModel PUBLIC Qt5::Qml)
target_include_directories(SortFilterProxyModel PUBLIC "${CMAKE_CURRENT_LIST_DIR}")
target_compile_features(SortFilterProxyModel PUBLIC cxx_std_11)
set_target_properties(SortFilterProxyModel PROPERTIES CXX_EXTENSIONS OFF)
