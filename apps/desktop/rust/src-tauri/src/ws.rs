use futures_util::{SinkExt, StreamExt};
use serde_json::Value;
use std::time::Duration;
use tauri::{AppHandle, Emitter};
use tokio_tungstenite::tungstenite::Message;

use crate::events::handle_ws_event;
use crate::models::SharedState;
use crate::config;

pub async fn set_status(state: &SharedState, app: &AppHandle, status: &str) {
    *state.ws_status.write().await = status.to_string();
    let _ = app.emit("willchat:event", serde_json::json!({"type": "websocket_status", "status": status}));
}

pub async fn start_ws(state: &SharedState, app: &AppHandle) {
    let mut busy = state.ws_busy.lock().await;
    if *busy {
        return;
    }
    *busy = true;
    let token = state.token.read().await.clone();
    if token.is_empty() {
        *busy = false;
        return;
    }
    let owner = state.clone();
    let state = state.clone();
    let app = app.clone();
    let task = tokio::spawn(async move {
        let ws_base = config::get().ws_url.clone();
        loop {
            if state.token.read().await.is_empty() {
                break;
            }
            let full_url = format!("{ws_base}/ws?token={token}");
            set_status(&state, &app, "reconnecting").await;
            let stream = match tokio::time::timeout(Duration::from_secs(30), tokio_tungstenite::connect_async(&full_url)).await {
                Ok(Ok((stream, _))) => stream,
                _ => {
                    set_status(&state, &app, "reconnecting").await;
                    tokio::time::sleep(Duration::from_secs(3)).await;
                    continue;
                }
            };
            let (mut write, mut read) = stream.split();
            while let Ok(Some(Ok(msg))) = tokio::time::timeout(Duration::from_secs(45), read.next()).await {
                if let Message::Ping(payload) = msg {
                    if !matches!(tokio::time::timeout(Duration::from_secs(10), write.send(Message::Pong(payload))).await, Ok(Ok(()))) {
                        break;
                    }
                } else if let Message::Close(_) = msg {
                    break;
                } else if let Message::Text(text) = msg {
                    let event: Value = serde_json::from_str(&text).unwrap_or(Value::Null);
                    if event.is_object() {
                        handle_ws_event(&state, &app, &event).await;
                    }
                }
            }
            set_status(&state, &app, "reconnecting").await;
            tokio::time::sleep(Duration::from_secs(3)).await;
        }
        *state.ws_busy.lock().await = false;
        set_status(&state, &app, "closed").await;
    });
    *owner.ws_task.lock().await = Some(task);
}

pub async fn stop_ws(state: &SharedState, app: &AppHandle) {
    if let Some(task) = state.ws_task.lock().await.take() {
        task.abort();
        let _ = task.await;
    }
    *state.ws_busy.lock().await = false;
    set_status(state, app, "closed").await;
}
