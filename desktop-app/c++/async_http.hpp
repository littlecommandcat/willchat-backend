#pragma once

// ==========================================================
// async_http.hpp —— 非同步 HTTP 客戶端（Boost.Asio 協程 / C++20）
// ==========================================================
//
// 設計目的：
//   用 C++20 協程 (coroutine) 包裝 Boost.Beast 的非同步 HTTP 請求，
//   讓「發請求 → 等回應」寫起來像同步程式，但實際上完全不阻塞
//   呼叫它的執行緒（例如 ImGui 主迴圈）。
//
// 用法（對外 API 只有三個）：
//   auto r = co_await http::post("127.0.0.1", "8080", "/auth/login", R"({"email":"a@b.c","password":"x"})");
//   取回 http::response { status, reason, body }，再自行解析 body（JSON）。
//
// 注意：
//   1. 只能被「C++20 協程」呼叫（呼叫者自己要是 awaitable）。
//   2. 網路錯誤以例外拋出，請用 try/catch 攔截，
//      否則 co_spawn 派的 handler 內例外無人接 → std::terminate。
// ==========================================================

// -------- Boost.Asio：協程相關 --------
#include <boost/asio/awaitable.hpp>    // asio::awaitable<T>：協程的回傳型別。T 是這支協程做完後 co_return 給別人的值
#include <boost/asio/co_spawn.hpp>     // asio::co_spawn：把一個協程丟進 io_context 排程執行
#include <boost/asio/use_awaitable.hpp>// asio::use_awaitable：CompletionToken，
                                       //   餵給任何 async_xxx() 讓它回傳 awaitable 可直接 co_await，免寫 callback
#include <boost/asio/this_coro.hpp>    // asio::this_coro::executor：在協程內取得「本協程所屬 executor」

// -------- Boost.Beast：HTTP 協定本身 --------
#include <boost/beast/core.hpp>        // Beast 核心：tcp_stream、flat_buffer、error_code 等基礎元件
#include <boost/beast/http.hpp>        // Beast HTTP：request、response、verb、field、async_write/read
#include <boost/beast/version.hpp>     // BOOST_BEAST_VERSION_STRING：版本字串，可當 User-Agent

// -------- C++ 標準函式庫 --------
#include <string>     // std::string：body、host 等字串
#include <optional>   // std::optional：body 可傳可不傳的包裝型別
#include <exception>  // std::exception：例外攔截用

// 自訂命名空間 http，避免與 std / boost 名字衝突

namespace asio  = boost::asio;  // 縮寫：asio 就等於 boost::asio
namespace beast = boost::beast; // 縮寫：beast 就等於 boost::beast
     

using tcp       = asio::ip::tcp; // 縮寫：tcp 直接可用

namespace http { 
// ----------------------------------------------------------
// response：統一的 HTTP 回應結構，所有公開 API 的回傳型別
// ----------------------------------------------------------
struct response {
    unsigned int status; // HTTP 狀態碼，例如 200(OK)、400、401、404、500
    std::string reason;  // 狀態說明文字，例如 "OK"、"Not Found"
    std::string body;    // 伺服器回傳的內容（通常就是 JSON 字串）
};

// ----------------------------------------------------------
// detail 命名空間：放「內部共用、不打算公開」的實作
// ----------------------------------------------------------
namespace detail {

    // ------------------------------------------------------
    // request_impl：真正執行一次性 HTTP 請求的底層協程工具
    //
    // 流程（每一步都非同步，co_await 交出執行權，未完成前不佔線）：
    //   ① DNS 解析主機名稱
    //   ② TCP 連線
    //   ③ 組 HTTP Request（method + target + header + body）
    //   ④ async_write 把請求送出
    //   ⑤ async_read 等伺服器回整個 HTTP 回應
    //   ⑥ 關閉 socket，把回應包成 response 回傳
    //
    // 參數：
    //   method       HTTP 動詞：get、post、delete_...
    //   host         主機 IP 或網域名稱，例如 "127.0.0.1"
    //   port         連接埠：字串形式，例如 "8080"
    //   target       路徑（URI），例如 "/auth/login" 或 "/?a=1"
    //   body         請求本文（optional，沒傳就是 default = 不存在）
    //   content_type 本文格式，預設 "application/json"
    // ------------------------------------------------------
    inline asio::awaitable<response> request_impl(
        beast::http::verb method,                                  // HTTP 動詞
        const std::string& host,                                   // 目標主機
        const std::string& port,                                   // 目標連接埠（字串）
        const std::string& target,                                 // 目標路徑
        const std::optional<std::string>& body = std::nullopt,     // 請求本文（可省略）
        const std::string& content_type = "application/json")      // 本文類型（可省略）
    {
        // 取回現在這個協程所屬的 executor（等同 io_context 的排程器），
        // 之後所有 async 物件都要用同一個 executor 才不會在不同執行緒錯亂
        auto executor = co_await asio::this_coro::executor;

        // 建立 DNS resolver：負責把 host 名稱解析成 IP 位址清單
        tcp::resolver resolver{executor};

        // 建立 TCP 串流：beast::tcp_stream 就是「能非同步讀寫的 TCP socket」，
        // 包裝 asio::ip::tcp::socket 並多了存活時間(timeout)等便利功能
        beast::tcp_stream stream{executor};

        // ---------- ① DNS 解析 ----------
        // async_resolve：把 host+port 解析成 IP 清單；非同步函式，
        // 傳 asio::use_awaitable 表示「我要用 co_await 等結果」。
        // 回傳的 results 是「端點清單」，代表解析出的所有候選 IP
        auto const results = co_await resolver.async_resolve(host, port, asio::use_awaitable);

        // ---------- ② TCP 連線 ----------
        // async_connect：從 results 依序試連，連到第一個成功為止。
        // 也是非同步：連線過程中不佔用呼叫者執行緒
        co_await stream.async_connect(results, asio::use_awaitable);
        // ---------- ③ 組 HTTP Request ----------
        // 建立 HTTP 請求物件；第三個參數 11 表示 HTTP/1.1。
        // body 型別用 string_body：本文就是一個 std::string
        beast::http::request<beast::http::string_body> req{method, target, 11};

        // Host header：伺服器依它判斷要接受哪個虛擬主機（HTTP/1.1 必填）
        req.set(beast::http::field::host, host);

        // User-Agent：把 beast 版本昭告給伺服器（可有可無）
        req.set(beast::http::field::user_agent, BOOST_BEAST_VERSION_STRING);

        // ---------- 處理 Body（POST/PUT 等帶本文的方法才需要） ----------
        if (body.has_value()) {                       // 如果有傳 body
            req.set(beast::http::field::content_type, content_type); // 告訴伺服器本文格式（JSON）
            req.body() = body.value();                // 把本文內容放進去
            req.prepare_payload();                    // 自動計算並填好 Content-Length 等必要 header；忘了這步會發不出去
        }

        // ---------- ④ 送出請求 ----------
        // async_write：把整個 HTTP 請求（header+body）非同步寫進 socket。
        // co_await 等到它寫完為止，之後就不用再碰 req
        co_await beast::http::async_write(stream, req, asio::use_awaitable);

        // ---------- ⑤ 接收回應 ----------
        // flat_buffer：Beast 用來暫存「讀進來但還沒組好」的原始資料
        beast::flat_buffer buffer;

        // 宣告接收回應的容器（型別與請求對稱，也是 string_body）
        beast::http::response<beast::http::string_body> res;

        // async_read：非同步讀取「完整的一個 HTTP 回應」。
        // 它會一直讀到 header 與 body 都收齊才 co_await 返回；
        // 伺服器若久久不回，這裡沒設 timeout 就會一直等（已知坑之一）
        co_await beast::http::async_read(stream, buffer, res, asio::use_awaitable);

        // ---------- ⑥ 關閉連線 ----------
        // 對 socket 發 shutdown：送 FIN 告訴伺服器「我方不再送資料」。
        // shutdown_both = 雙向都停止。
        // ec（error_code）必填：關閉一般不拋例外，用 ec 接住吞掉即可
        beast::error_code ec;
        stream.socket().shutdown(tcp::socket::shutdown_both, ec);

        // 打包回傳（response 是 aggregate，用大括號依序填三個欄位）
        co_return response{
            res.result_int(),       // 狀態碼：res.result_int() = 200、404...
            std::string(res.reason()), // 狀態說明："OK"、"Not Found"，從 char* 轉成 std::string
            std::move(res.body())   // body：用 move 直接搬走，避免把伺服器回的大字串再拷貝一份
        };
    }
} // namespace detail

// ----------------------------------------------------------
// 公開 API ①：HTTP GET
// 參數與 request_impl 相同；底層把 GET 動詞傳下去
// ----------------------------------------------------------
inline asio::awaitable<response> get(
    const std::string& host, const std::string& port, const std::string& target)
{
    // 委派給 detail::request_impl，動詞固定 beast::http::verb::get，
    // 不帶 body（GET 本來就不放本文）
    co_return co_await detail::request_impl(beast::http::verb::get, host, port, target);
}

// ----------------------------------------------------------
// 公開 API ②：HTTP POST（送 JSON）
// json_body = 要送的 JSON 字串；content_type 固定 application/json
// ----------------------------------------------------------
inline asio::awaitable<response> post(
    const std::string& host, const std::string& port, const std::string& target,
    const std::string& json_body)
{
    // 委派給 request_impl，並把 JSON 當 body、content_type 指定為 application/json
    co_return co_await detail::request_impl(beast::http::verb::post, host, port, target, json_body, "application/json");
}

// ----------------------------------------------------------
// 公開 API ③：HTTP DELETE
// 註：delete 是 C++ 關鍵字不能當函式名，故命名 del，底層映射到 beast 的 delete_
// ----------------------------------------------------------
inline asio::awaitable<response> del(
    const std::string& host, const std::string& port, const std::string& target)
{
    // 委派給 request_impl，動詞固定 delete_
    co_return co_await detail::request_impl(beast::http::verb::delete_, host, port, target);
}

inline asio::awaitable<response> put(
    const std::string& host, const std::string& port, const std::string& target,
    const std::string& json_body)
{
    // 委派給 request_impl，並把 JSON 當 body、content_type 指定為 application/json
    co_return co_await detail::request_impl(beast::http::verb::put, host, port, target, json_body, "application/json");
}

} 

