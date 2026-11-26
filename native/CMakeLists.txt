cmake_minimum_required(VERSION 3.16)
project(jolt_c_api LANGUAGES CXX C)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

add_library(jolt_c_api SHARED
    jolt_c_api.h
    jolt_c_api.cpp
)

target_include_directories(jolt_c_api PUBLIC
    ${CMAKE_CURRENT_SOURCE_DIR}
    ${CMAKE_CURRENT_SOURCE_DIR}/../engine/spatial/thirdparty/joltc/include
)

if (WIN32)
    target_link_libraries(jolt_c_api PRIVATE
        ${CMAKE_CURRENT_SOURCE_DIR}/../engine/spatial/thirdparty/joltc/lib/windows/amd64/libjoltc.a
    )
else()
    target_link_libraries(jolt_c_api PRIVATE joltc)
endif()
