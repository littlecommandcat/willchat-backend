use futures_util::StreamExt;
use serde_json::Value;
use std::time::Duration;
use tauri::AppHandle;
use tokio_tungstenite::tungstenite::Message;

use crate::events::handle_ws_event;
use crate::models::SharedState;
use crate::config;

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
    let state = state.clone();
    let app = app.clone();
    tauri::async_runtime::spawn(async move {
        let ws_base = config::get().ws_url.clone();
        loop {
            let full_url = format!("{ws_base}/ws?token={token}");
            let stream = match tokio_tungstenite::connect_async(&full_url).await {
                Ok((stream, _)) => stream,
                Err(_) => {
                    tokio::time::sleep(Duration::from_secs(3)).await;
                    continue;
                }
            };
            let (mut write, mut read) = stream.split();
            let _ = &mut write;
            while let Some(Ok(msg)) = read.next().await {
                if let Message::Text(text) = msg {
                    let event: Value = serde_json::from_str(&text).unwrap_or(Value::Null);
                    if event.is_object() {
                        handle_ws_event(&state, &app, &event).await;
                    }
                }
            }
            tokio::time::sleep(Duration::from_secs(3)).await;
        }
    });
}
