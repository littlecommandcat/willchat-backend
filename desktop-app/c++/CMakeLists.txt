cmake_minimum_required(VERSION 3.20)
project(my_opengl_app CXX)

if(POLICY CMP0167)
    cmake_policy(SET CMP0167 NEW)
endif()

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

if(MINGW)
    set(CMAKE_EXE_LINKER_FLAGS "${CMAKE_EXE_LINKER_FLAGS} -static -static-libgcc -static-libstdc++")
endif() 

find_package(OpenGL REQUIRED)
find_package(glfw3 CONFIG REQUIRED)
find_package(glad CONFIG REQUIRED)
find_package(imgui CONFIG REQUIRED)
find_package(Boost CONFIG REQUIRED COMPONENTS system json)
find_package(OpenSSL REQUIRED)
add_executable(${PROJECT_NAME} main.cpp
    async_http.hpp)

target_link_libraries(${PROJECT_NAME} PRIVATE
    OpenGL::GL
    glfw
    glad::glad
    imgui::imgui
    Boost::system
    Boost::boost
    Boost::json
    OpenSSL::SSL
    OpenSSL::Crypto
)
