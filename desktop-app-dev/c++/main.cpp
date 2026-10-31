#include "glad/glad.h" 
#include "imgui.h"
#include "imgui_impl_glfw.h"
#include "imgui_impl_opengl3.h"
#include "async_http.hpp"
#include "auth_ui.hpp"
#include <GLFW/glfw3.h>

#include <boost/asio/io_context.hpp>
#include <boost/asio/co_spawn.hpp>
#include <boost/asio/detached.hpp>
#include <boost/asio/ip/tcp.hpp>
#include <boost/asio/ssl.hpp>
#include <boost/beast/core.hpp>
#include <boost/beast/http.hpp>
#include <boost/beast/ssl.hpp>
#include <boost/json.hpp>
#include <openssl/ssl.h>

#include <iostream>
#include <fstream>
#include <filesystem>
#include <cstdlib>
#include <string>
#include <vector>


namespace asio  = boost::asio;  
namespace beast = boost::beast; 





int main(){
    if(!glfwInit())return -1;
    glfwWindowHint(GLFW_CONTEXT_VERSION_MAJOR,3);
    glfwWindowHint(GLFW_CONTEXT_VERSION_MINOR,3);
    glfwWindowHint(GLFW_OPENGL_PROFILE,GLFW_OPENGL_CORE_PROFILE);
    glfwWindowHint(GLFW_MAXIMIZED,GLFW_TRUE);
    GLFWwindow* window=glfwCreateWindow(800,600,"helloworld",NULL,NULL);
    if(!window)return -1;
    glfwMakeContextCurrent(window);
    glfwSwapInterval(1);
    if (!gladLoadGLLoader((GLADloadproc)glfwGetProcAddress)){
        std::cout << "Glad 初始化失敗！" << std::endl;
        return -1;
    }

    IMGUI_CHECKVERSION();
    ImGui::CreateContext();
    ImGuiIO& io = ImGui::GetIO(); (void)io;
    ImGui::StyleColorsDark();
    const ImWchar* glyph_ranges = io.Fonts->GetGlyphRangesChineseFull();
    ImFont* font = io.Fonts->AddFontFromFileTTF("C:\\Windows\\Fonts\\msjh.ttc", 18.0f, nullptr, glyph_ranges);
    if (font == nullptr){
    std::cout << "找不到微軟正黑體，請檢查路徑！" << std::endl;
    }

    io.IniFilename = nullptr;
    ImGui_ImplGlfw_InitForOpenGL(window, true);
    ImGui_ImplOpenGL3_Init("#version 330");
    int window_basic_flags=ImGuiWindowFlags_NoMove|ImGuiWindowFlags_NoResize|ImGuiWindowFlags_NoTitleBar;
    bool channel_list_visable=true;
    bool member_list_visable=false;
    LoginState login;
    // network
    boost::asio::io_context net_io; 
    //
    while(!glfwWindowShouldClose(window)){
        glfwPollEvents();
        net_io.poll();
        ImGui_ImplOpenGL3_NewFrame();
        ImGui_ImplGlfw_NewFrame();
        ImGui::NewFrame();
        ImVec2 AppWindowSize = ImGui::GetIO().DisplaySize;
        float space_left = 0.0f, space_top = 0.0f, space_right = AppWindowSize.x ,space_bottom = AppWindowSize.y;
        if(!login.logged_in){
            drawLoginUI(net_io, login);
        }else{
            // channel_list
            if(channel_list_visable){
                ImGui::SetNextWindowPos(ImVec2(space_left,space_top),ImGuiCond_Always);
                ImGui::SetNextWindowSize(ImVec2(300.0f,space_bottom-space_top));
                space_left+=300.0f;
                
                ImGui::Begin("左側頻道",nullptr,window_basic_flags);

                ImGui::End();
            }
            
            // TODO : member list
            if(member_list_visable){
                ImGui::SetNextWindowPos(ImVec2(space_right-240.0f,space_top),ImGuiCond_Always);
                ImGui::SetNextWindowSize(ImVec2(240.0f,space_bottom-space_top));
                space_right-=240.0f;
                ImGui::Begin("右側成員名單/其餘訊息",nullptr,7);
                //...
                ImGui::End();
            }
            // message_list
            ImGui::SetNextWindowPos(ImVec2(space_left,space_top),ImGuiCond_Always);
            ImGui::SetNextWindowSize(ImVec2(space_right-space_left,space_bottom-space_top));
            ImGui::Begin("訊息列表",nullptr,window_basic_flags);

            ImGui::End();
        }
        ImGui::Render();
        
        glViewport(0, 0, (int)io.DisplaySize.x, (int)io.DisplaySize.y);
        glClearColor(0.1f, 0.12f, 0.15f, 1.0f);
        glClear(GL_COLOR_BUFFER_BIT);

        ImGui_ImplOpenGL3_RenderDrawData(ImGui::GetDrawData());
        glfwSwapBuffers(window);
    }

    ImGui_ImplOpenGL3_Shutdown();
    ImGui_ImplGlfw_Shutdown();
    ImGui::DestroyContext(nullptr);

    glfwDestroyWindow(window);
    glfwTerminate();
    return 0;
}
    
