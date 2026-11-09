use serde_json::{json, Value};
use tauri::{AppHandle, Emitter};

use crate::client::parse_group;
use crate::models::{GroupEntry, MemberInfo, SharedState};

pub fn ser_group(group: &GroupEntry) -> Value {
    let members: Vec<Value> = group
        .members
        .iter()
        .map(|m| json!({"id": m.id, "name": m.name}))
        .collect();
    json!({
        "id": group.id,
        "name": group.name,
        "owner_id": group.owner_id,
        "members": members,
    })
}

pub fn ser_message(msg: &Value, author_name: &str) -> Value {
    let mut data = msg.clone();
    data["author_name"] = json!(author_name);
    if let Some(author) = msg.get("author").filter(|v| v.is_object()) {
        data["author_id"] = author.get("id").cloned().unwrap_or_default();
        if let Some(name) = author.get("name").and_then(Value::as_str).filter(|s| !s.is_empty()) {
            data["author_name"] = json!(name);
        }
    }
    if data.get("author_id").is_none() {
        if let Some(aid) = msg.get("authorid") {
            data["author_id"] = aid.clone();
        }
    }
    data
}

pub async fn resolve_member_name(state: &SharedState, member_id: &str) -> String {
    {
        let user = state.current_user.read().await;
        if user.id == member_id && !user.name.is_empty() {
            return user.name.clone();
        }
    }
    let groups = state.groups.read().await;
    for group in groups.values() {
        if let Some(m) = group.members.iter().find(|m| m.id == member_id) {
            return m.name.clone();
        }
    }
    "成員".to_string()
}

fn ready_member(payload: &Value) -> &Value {
    payload.get("member").filter(|m| m.is_object()).unwrap_or(payload)
}

pub async fn handle_ws_event(state: &SharedState, app: &AppHandle, event: &Value) {
    let event_type = event
        .get("event")
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .to_string();
    let payload = event.get("payload").cloned().unwrap_or_else(|| json!({}));
    let member = payload.get("member").cloned();
    let message = payload.get("message").cloned();
    let group = payload.get("group").cloned();

    match event_type.as_str() {
        "ready" => {
            let m = ready_member(&payload);
            crate::client::rebuild_groups(state, m).await;
            state.messages.write().await.clear();
            crate::ws::set_status(state, app, "connected").await;
            let online: Vec<Value> = payload.get("online").and_then(Value::as_array).map(|members| {
                members.iter().map(|m| m.get("id").unwrap_or(m).clone()).collect()
            }).unwrap_or_default();
            let _ = app.emit("willchat:event", json!({"type": "ready", "online": online}));
        }
        "message_send" => {
            if let Some(msg) = message {
                let gid = group
                    .as_ref()
                    .and_then(|g| g.get("group_id").and_then(|v| v.as_str()))
                    .map(|s| s.to_string())
                    .or_else(|| {
                        msg.get("group")
                            .and_then(|g| g.get("id").and_then(|v| v.as_str()))
                            .map(|s| s.to_string())
                    })
                    .unwrap_or_default();
                let author_id = msg.get("author").and_then(|v| v.get("id")).or_else(|| msg.get("authorid")).and_then(|v| v.as_str()).unwrap_or("").to_string();
                let author_name = resolve_member_name(state, &author_id).await;
                let ser = ser_message(&msg, &author_name);
                if !gid.is_empty() {
                    let mut messages = state.messages.write().await;
                    let list = messages.entry(gid.clone()).or_insert_with(Vec::new);
                    if !list.iter().any(|m| m.get("id") == ser.get("id")) {
                        list.push(ser.clone());
                    }
                }
                let _ = app.emit(
                    "willchat:event",
                    json!({"type": "message_send", "group_id": gid, "message": ser}),
                );
            }
        }
        "message_delete" => {
            if let Some(msg) = message {
                let gid = group
                    .as_ref()
                    .and_then(|g| g.get("group_id").and_then(|v| v.as_str()))
                    .map(|s| s.to_string())
                    .unwrap_or_default();
                let _ = app.emit(
                    "willchat:event",
                    json!({"type": "message_delete", "group_id": gid, "message": msg}),
                );
            }
        }
        "group_create" => {
            if let Some(g) = group {
                let entry = parse_group(&g);
                crate::client::upsert_group(state, entry.clone()).await;
                let _ = app.emit(
                    "willchat:event",
                    json!({"type": "group_create", "group": ser_group(&entry)}),
                );
            }
        }
        "group_delete" => {
            if let Some(g) = group {
                let gid = g.get("group_id").and_then(|v| v.as_str()).unwrap_or("").to_string();
                {
                    let mut groups = state.groups.write().await;
                    groups.remove(&gid);
                }
                {
                    let mut messages = state.messages.write().await;
                    messages.remove(&gid);
                }
                let _ = app.emit(
                    "willchat:event",
                    json!({"type": "group_delete", "group_id": gid}),
                );
            }
        }
        "group_join" => {
            if let Some(g) = group {
                let entry = parse_group(&g);
                crate::client::upsert_group(state, entry.clone()).await;
                let m = member.clone().unwrap_or_else(|| json!({}));
                {
                    let mut groups = state.groups.write().await;
                    if let Some(e) = groups.get_mut(&entry.id) {
                        let mid = m.get("id").and_then(|v| v.as_str()).unwrap_or("").to_string();
                        let mname = m.get("name").and_then(|v| v.as_str()).unwrap_or("").to_string();
                        if !mid.is_empty() && !e.members.iter().any(|x| x.id == mid) {
                            e.members.push(MemberInfo { id: mid, name: mname });
                        }
                    }
                }
                let _ = app.emit(
                    "willchat:event",
                    json!({"type": "group_join", "group_id": entry.id, "member": m}),
                );
            }
        }
        "group_leave" => {
            if let Some(g) = group {
                let gid = g.get("group_id").and_then(|v| v.as_str()).unwrap_or("").to_string();
                let m = member.clone().unwrap_or_else(|| json!({}));
                {
                    let mut groups = state.groups.write().await;
                    if let Some(e) = groups.get_mut(&gid) {
                        let mid = m.get("id").and_then(|v| v.as_str()).unwrap_or("").to_string();
                        e.members.retain(|x| x.id != mid);
                    }
                }
                let _ = app.emit(
                    "willchat:event",
                    json!({"type": "group_leave", "group_id": gid, "member": m}),
                );
            }
        }
        "member_update" => {
            if let Some(m) = member {
                let mid = m.get("id").and_then(|v| v.as_str()).unwrap_or("").to_string();
                let mname = m.get("name").and_then(|v| v.as_str()).unwrap_or("").to_string();
                {
                    let mut user = state.current_user.write().await;
                    if user.id == mid {
                        user.name = mname.clone();
                    }
                }
                {
                    let mut groups = state.groups.write().await;
                    for entry in groups.values_mut() {
                        if let Some(mm) = entry.members.iter_mut().find(|x| x.id == mid) {
                            mm.name = mname.clone();
                        }
                    }
                }
                let _ = app.emit(
                    "willchat:event",
                    json!({"type": "member_update", "member": m}),
                );
            }
        }
        "member_online" | "member_offline" => {
            let mid = member.as_ref().and_then(|m| m.get("id")).cloned().unwrap_or_default();
            let _ = app.emit("willchat:event", json!({"type": event_type, "member_id": mid}));
        }
        "error" | "disconnect" => {
            let message = member
                .map(|m| m.to_string())
                .unwrap_or_else(|| payload.to_string());
            let _ = app.emit(
                "willchat:event",
                json!({"type": "error", "message": message}),
            );
        }
        _ => {}
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Arc;

    #[tokio::test]
    async fn ready_rebuilds_groups_for_current_and_legacy_payloads() {
        let member = json!({
            "id": "alice", "name": "Alice",
            "groups": [{"group_id": "chat", "name": "Chat", "owner_id": "alice"}]
        });
        for payload in [member.clone(), json!({"member": member})] {
            let state = Arc::new(crate::models::AppState::default());
            crate::client::rebuild_groups(&state, ready_member(&payload)).await;
            assert_eq!(state.current_user.read().await.id, "alice");
            assert!(state.groups.read().await.contains_key("chat"));
        }
    }

    #[test]
    fn message_normalizes_backend_author_id() {
        let message = ser_message(&json!({"id": "message", "authorid": "alice"}), "Alice");
        assert_eq!(message["author_id"], "alice");
        assert_eq!(message["author_name"], "Alice");
    }

    #[test]
    fn message_uses_embedded_author_and_preserves_reply_author() {
        let message = ser_message(&json!({
            "id": "message", "author": {"id": "alice", "name": "Alice"},
            "reply_to": {"author": {"id": "bob", "name": "Bob"}, "content": "Hi"}
        }), "成員");
        assert_eq!(message["author_id"], "alice");
        assert_eq!(message["author_name"], "Alice");
        assert_eq!(message["reply_to"]["author"]["name"], "Bob");
    }

    #[test]
    fn group_parses_embedded_owner() {
        let group = parse_group(&json!({
            "group_id": "chat", "name": "Chat", "owner": {"id": "alice", "name": "Alice"}
        }));
        assert_eq!(group.owner_id, "alice");
        assert_eq!(group.members[0].name, "Alice");
    }
}

