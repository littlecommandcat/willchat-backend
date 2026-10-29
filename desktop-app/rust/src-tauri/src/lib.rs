use serde_json::{json, Value};
use std::sync::Arc;
use tauri::{AppHandle, State};

mod client;
mod events;
mod models;
mod ws;

use models::SharedState;

#[tauri::command]
async fn health() -> Value {
    let (ok, msg) = client::health_check().await.unwrap_or((false, "無法連線後端".to_string()));
    json!({"ok": ok, "error": msg})
}

#[tauri::command]
async fn login(
    state: State<'_, SharedState>,
    app: AppHandle,
    email: String,
    password: String,
) -> Result<Value, String> {
    let email = email.trim().to_string();
    let (ok, msg) = client::health_check().await.unwrap_or((false, "無法連線後端".to_string()));
    if !ok {
        return Ok(json!({"ok": false, "error": msg}));
    }
    let client = client::server_url();
    match client::login_request(&client, &email, &password).await {
        Ok((data, token)) => {
            if token.is_empty() {
                return Ok(json!({"ok": false, "error": "帳號或密碼錯誤"}));
            }
            *state.token.write().await = token;
            let member = data.get("member").cloned().unwrap_or_else(|| json!({}));
            let current = client::parse_member(&member);
            *state.current_user.write().await = current.clone();
            client::rebuild_groups(&state, &member).await;
            ws::start_ws(&state, &app).await;
            Ok(json!({"ok": true, "member": {"id": current.id, "name": current.name}}))
        }
        Err(_) => Ok(json!({"ok": false, "error": "無法連接伺服器"})),
    }
}

#[tauri::command]
async fn register(
    state: State<'_, SharedState>,
    app: AppHandle,
    username: String,
    email: String,
    password: String,
) -> Result<Value, String> {
    let username = username.trim().to_string();
    let email = email.trim().to_string();
    let (ok, msg) = client::health_check().await.unwrap_or((false, "無法連線後端".to_string()));
    if !ok {
        return Ok(json!({"ok": false, "error": msg}));
    }
    let client = client::server_url();
    match client::register_request(&client, &username, &email, &password).await {
        Ok((status, data, token)) => {
            if !matches!(status, 200 | 201) {
                return Ok(json!({"ok": false, "error": "註冊失敗"}));
            }
            if !token.is_empty() {
                *state.token.write().await = token;
                let member = data.get("member").cloned().unwrap_or_else(|| json!({}));
                let current = client::parse_member(&member);
                *state.current_user.write().await = current.clone();
                client::rebuild_groups(&state, &member).await;
                ws::start_ws(&state, &app).await;
            }
            Ok(json!({"ok": true}))
        }
        Err(_) => Ok(json!({"ok": false, "error": "無法連接伺服器"})),
    }
}
#[tauri::command]
async fn get_state(state: State<'_, SharedState>) -> Result<Value, String> {
    let user = state.current_user.read().await.clone();
    let groups = state.groups.read().await;
    let groups: Vec<Value> = groups.values().map(events::ser_group).collect();
    Ok(json!({"member": {"id": user.id, "name": user.name}, "groups": groups}))
}

#[tauri::command]
async fn get_messages(state: State<'_, SharedState>, group_id: String) -> Result<Value, String> {
    let client = client::server_url();
    let token = state.token.read().await.clone();
    let path = format!("/api/groups/{group_id}/messages?amount=50");
    match client::request_json(&client, "GET", &path, Some(&token), None).await {
        Ok((_, body)) => {
            let messages: Vec<Value> = {
                let arr = body
                    .get("data")
                    .and_then(|d| d.get("messages"))
                    .and_then(|v| v.as_array());
                let mut out = Vec::new();
                if let Some(arr) = arr {
                    for m in arr {
                        let author_id = m
                            .get("authorid")
                            .and_then(|v| v.as_str())
                            .unwrap_or("")
                            .to_string();
                        let state = state.clone();
                        let m = m.clone();
                        let name = events::resolve_member_name(&state, &author_id).await;
                        out.push(events::ser_message(&m, &name));
                    }
                }
                out
            };
            let mut data = messages;
            data.reverse();
            {
                let mut cache = state.messages.write().await;
                cache.insert(group_id.clone(), data.clone());
            }
            Ok(json!({"ok": true, "messages": data}))
        }
        Err(e) => Ok(json!({"ok": false, "error": e})),
    }
}

#[tauri::command]
async fn get_members(state: State<'_, SharedState>, group_id: String) -> Result<Value, String> {
    let groups = state.groups.read().await;
    let members = groups
        .get(&group_id)
        .map(|g| {
            g.members
                .iter()
                .map(|m| json!({"id": m.id, "name": m.name}))
                .collect::<Vec<_>>()
        })
        .unwrap_or_default();
    Ok(json!({"members": members}))
}

#[tauri::command]
async fn send_message(
    state: State<'_, SharedState>,
    group_id: String,
    content: String,
) -> Result<Value, String> {
    let content = content.trim().to_string();
    if content.is_empty() {
        return Ok(json!({"ok": false, "error": "訊息不能為空"}));
    }
    let client = client::server_url();
    let token = state.token.read().await.clone();
    let path = format!("/api/groups/{group_id}/messages");
    match client::request_json(&client, "POST", &path, Some(&token), Some(json!({"content": content}))).await {
        Ok((status, body)) => {
            if !client::is_ok_status(status) {
                let msg = body.get("message").and_then(|v| v.as_str()).unwrap_or("傳送失敗");
                return Ok(json!({"ok": false, "error": format!("傳送失敗：{msg}")}));
            }
            Ok(json!({"ok": true}))
        }
        Err(e) => Ok(json!({"ok": false, "error": format!("傳送失敗：{e}")})),
    }
}
#[tauri::command]
async fn create_group(state: State<'_, SharedState>, name: String) -> Result<Value, String> {
    let name = name.trim().to_string();
    if name.is_empty() {
        return Ok(json!({"ok": false, "error": "群組名稱不能為空"}));
    }
    let client = client::server_url();
    let token = state.token.read().await.clone();
    match client::request_json(&client, "POST", "/api/groups", Some(&token), Some(json!({"name": name}))).await {
        Ok((status, body)) => {
            if !client::is_ok_status(status) {
                let msg = body.get("message").and_then(|v| v.as_str()).unwrap_or("建立群組失敗");
                return Ok(json!({"ok": false, "error": format!("建立群組失敗：{msg}")}));
            }
            let group = body
                .get("data")
                .and_then(|d| d.get("group"))
                .cloned()
                .unwrap_or_else(|| json!({}));
            let entry = client::parse_group(&group);
            client::upsert_group(&state, entry.clone()).await;
            Ok(json!({"ok": true, "group": events::ser_group(&entry)}))
        }
        Err(e) => Ok(json!({"ok": false, "error": format!("建立群組失敗：{e}")})),
    }
}

#[tauri::command]
async fn delete_group(state: State<'_, SharedState>, group_id: String) -> Result<Value, String> {
    let client = client::server_url();
    let token = state.token.read().await.clone();
    let path = format!("/api/groups/{group_id}");
    match client::request_json(&client, "DELETE", &path, Some(&token), None).await {
        Ok((status, body)) => {
            if !client::is_ok_status(status) {
                let msg = body.get("message").and_then(|v| v.as_str()).unwrap_or("刪除群組失敗");
                return Ok(json!({"ok": false, "error": format!("刪除群組失敗：{msg}")}));
            }
            {
                let mut groups = state.groups.write().await;
                groups.remove(&group_id);
            }
            {
                let mut messages = state.messages.write().await;
                messages.remove(&group_id);
            }
            Ok(json!({"ok": true}))
        }
        Err(e) => Ok(json!({"ok": false, "error": format!("刪除群組失敗：{e}")})),
    }
}

#[tauri::command]
async fn join_group(state: State<'_, SharedState>, group_id: String) -> Result<Value, String> {
    let client = client::server_url();
    let token = state.token.read().await.clone();
    let path = format!("/api/groups/{group_id}/members");
    match client::request_json(&client, "POST", &path, Some(&token), Some(json!({}))).await {
        Ok((status, body)) => {
            if !client::is_ok_status(status) {
                let msg = body.get("message").and_then(|v| v.as_str()).unwrap_or("加入群組失敗");
                return Ok(json!({"ok": false, "error": format!("加入群組失敗：{msg}")}));
            }
            let group = body
                .get("data")
                .and_then(|d| d.get("group"))
                .cloned()
                .unwrap_or_else(|| json!({}));
            let entry = client::parse_group(&group);
            client::upsert_group(&state, entry.clone()).await;
            Ok(json!({"ok": true, "group": events::ser_group(&entry)}))
        }
        Err(e) => Ok(json!({"ok": false, "error": format!("加入群組失敗：{e}")})),
    }
}

#[tauri::command]
async fn leave_group(state: State<'_, SharedState>, group_id: String) -> Result<Value, String> {
    let client = client::server_url();
    let token = state.token.read().await.clone();
    let path = format!("/api/groups/{group_id}/members");
    match client::request_json(&client, "DELETE", &path, Some(&token), None).await {
        Ok((status, body)) => {
            if !client::is_ok_status(status) {
                let msg = body.get("message").and_then(|v| v.as_str()).unwrap_or("退出群組失敗");
                return Ok(json!({"ok": false, "error": format!("退出群組失敗：{msg}")}));
            }
            {
                let mut groups = state.groups.write().await;
                groups.remove(&group_id);
            }
            {
                let mut messages = state.messages.write().await;
                messages.remove(&group_id);
            }
            Ok(json!({"ok": true}))
        }
        Err(e) => Ok(json!({"ok": false, "error": format!("退出群組失敗：{e}")})),
    }
}

#[tauri::command]
async fn navigate(app: AppHandle, page: String) {
    if page == "/rick" {
        let _ = tauri_plugin_opener::open_url(
            "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
            None::<&str>,
        );
        let _ = &app;
    }
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_opener::init())
        .manage(Arc::new(models::AppState::default()))
        .invoke_handler(tauri::generate_handler![
            health,
            login,
            register,
            get_state,
            get_messages,
            get_members,
            send_message,
            create_group,
            delete_group,
            join_group,
            leave_group,
            navigate
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
