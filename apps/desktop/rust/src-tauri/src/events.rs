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
            if let Some(m) = member {
                crate::client::rebuild_groups(state, &m).await;
            }
            let _ = app.emit("willchat:event", json!({"type": "ready"}));
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
                let author_id = msg.get("authorid").and_then(|v| v.as_str()).unwrap_or("").to_string();
                let author_name = resolve_member_name(state, &author_id).await;
                let ser = ser_message(&msg, &author_name);
                if !gid.is_empty() {
                    let mut messages = state.messages.write().await;
                    messages
                        .entry(gid.clone())
                        .or_insert_with(Vec::new)
                        .push(ser.clone());
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

