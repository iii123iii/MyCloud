cmake_minimum_required(VERSION 3.25)
project(mycloud_backend VERSION 1.0.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 26)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

# Extra C++26 flags
add_compile_options(-Wall -Wextra -Wpedantic)

find_package(Drogon CONFIG REQUIRED)
find_package(OpenSSL REQUIRED)

# hiredis
find_library(HIREDIS_LIB hiredis REQUIRED)
find_path(HIREDIS_INCLUDE hiredis/hiredis.h REQUIRED)

file(GLOB_RECURSE SOURCES src/*.cpp)

add_executable(mycloud_backend ${SOURCES})

target_include_directories(mycloud_backend PRIVATE
    src
    ${HIREDIS_INCLUDE}
    /usr/local/include  # jwt-cpp
)

target_link_libraries(mycloud_backend PRIVATE
    Drogon::Drogon
    OpenSSL::SSL
    OpenSSL::Crypto
    ${HIREDIS_LIB}
)
