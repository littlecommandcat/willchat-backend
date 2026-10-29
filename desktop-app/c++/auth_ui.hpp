#pragma once
#include "imgui.h"
#include "imgui_impl_glfw.h"
#include "imgui_impl_opengl3.h"
#include <GLFW/glfw3.h>

#include "async_http.hpp"

#include <boost/asio/io_context.hpp>
#include <boost/asio/co_spawn.hpp>
#include <boost/asio/detached.hpp>
#include <boost/asio/awaitable.hpp>

#include <boost/json.hpp>

#include <string>
#include <vector>

struct member_data{
    std::string id;
    std::string name;
    std::vector<std::string> owner_group;
};

struct LoginState {
    char name[128] = "";
    char email[128] = "";
    char password[128] = "",check_password[128] = "";
    bool in_flight = false;
    std::string err;
    bool logged_in = false;
    std::string token;
    std::string member_id;
    std::string member_name;
    std::string member_email;
    bool signup_page=false;
};

inline void drawLoginUI(asio::io_context& io, LoginState& st) {
    const int flags = ImGuiWindowFlags_NoMove | ImGuiWindowFlags_NoResize | ImGuiWindowFlags_NoTitleBar;
    ImVec2 size = ImGui::GetIO().DisplaySize;
    ImGui::SetNextWindowPos(ImVec2(size.x * 0.5f, size.y * 0.5f), ImGuiCond_Always, ImVec2(0.5f, 0.5f));
    
    if(!st.signup_page){
        ImGui::SetNextWindowSize(ImVec2(320.0f, 120.0f));
        ImGui::Begin("login/signup_page", nullptr, flags);
        ImGui::Text("Login to Willchat");
        ImGui::Text("Email:");
        ImGui::SameLine(80.0f);
        ImGui::InputText("##email", st.email, IM_ARRAYSIZE(st.email));
        ImGui::Text("Password:");
        ImGui::SameLine(80.0f);
        ImGui::InputText("##password", st.password, IM_ARRAYSIZE(st.password), ImGuiInputTextFlags_Password);

        if (ImGui::Button("Log in") && !st.in_flight) {
            // TODO check password and email
            st.in_flight = true;
            std::string email(st.email), password(st.password);
            asio::co_spawn(io,
                [&io, &st, email, password]() -> asio::awaitable<void> {
                    try {
                        auto r = co_await http::post("127.0.0.1", "8080", "/auth/login",
                            "{\"email\":\"" + email + "\",\"password\":\"" + password + "\"}");
                        if (r.status == 200) {
                            boost::json::value jv = boost::json::parse(r.body);
                            const boost::json::object& data = jv.at("data").as_object();
                            st.token = data.at("token").as_string().c_str();
                            const boost::json::object& member = data.at("member").as_object();
                            st.member_id = member.at("id").as_string().c_str();
                            st.member_name = member.at("name").as_string().c_str();
                            st.member_email = member.at("email").as_string().c_str();
                            st.logged_in = true;
                        } else if (r.status == 401 or r.status == 400) {
                            st.err = "Invalid username or password.";
                        } else {
                            st.err = "HTTP " + std::to_string(r.status) + ": " + r.reason;
                        }
                    } catch (const std::exception& e) {
                        st.err = e.what();
                    }
                    st.in_flight = false;
                },
                asio::detached);
        }
        ImGui::SameLine();
        if (ImGui::Button("Sign up")) {
            st.signup_page=true;
        }
    }else{
        ImGui::SetNextWindowSize(ImVec2(320.0f, 300.0f));
        ImGui::Begin("login/signup_page", nullptr, flags);
        ImGui::Text("Sign up Willchat");
        ImGui::Text("Name:");
        // ImGui::SameLine(120.0f);
        ImGui::InputText("##name", st.name, IM_ARRAYSIZE(st.name));
        ImGui::Text("Email:");
        // ImGui::SameLine(120.0f);
        ImGui::InputText("##email", st.email, IM_ARRAYSIZE(st.email));
        ImGui::Text("Password:");
        // ImGui::SameLine(120.0f);
        ImGui::InputText("##password", st.password, IM_ARRAYSIZE(st.password), ImGuiInputTextFlags_Password);
        ImGui::Text("Confirm Password:");
        // ImGui::SameLine(120.0f);
        ImGui::InputText("##check_password", st.check_password, IM_ARRAYSIZE(st.check_password), ImGuiInputTextFlags_Password);
 
        std::string name(st.name), email(st.email), password(st.password), check_password(st.check_password);
        if (ImGui::Button("Sign up") && !st.in_flight) {
            if (password != check_password) {
                st.err = "Passwords do not match.";
            } else {
                st.in_flight = true;
                asio::co_spawn(io,
                    [&io, &st, name, email, password]() -> asio::awaitable<void> {
                        try {
                            auto r = co_await http::post("127.0.0.1", "8080", "/auth/register",
                                "{\"name\":\"" + name + "\",\"email\":\"" + email + "\",\"password\":\"" + password + "\"}");
                            if (r.status == 201) {
                                boost::json::value jv = boost::json::parse(r.body);
                                const boost::json::object& data = jv.at("data").as_object();
                                st.token = data.at("token").as_string().c_str();
                                const boost::json::object& member = data.at("member").as_object();
                                st.member_id = member.at("id").as_string().c_str();
                                st.member_name = member.at("name").as_string().c_str();
                                st.member_email = member.at("email").as_string().c_str();
                                st.logged_in = true;
                            } else {
                                st.err = "HTTP " + std::to_string(r.status) + ": " + r.reason;
                            }
                        } catch (const std::exception& e) {
                            st.err = e.what();
                        }
                        st.in_flight = false;
                    },
                    asio::detached);
            }
        }
        ImGui::SameLine();
        if (ImGui::Button("Back to login")) {
            st.signup_page = false;
        }
    }
    if (!st.err.empty()) ImGui::Text(st.err.c_str());

    ImGui::End();
}
