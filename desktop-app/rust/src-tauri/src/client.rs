use serde_json::{json, Value};
use std::time::Duration;

use crate::models::{GroupEntry, MemberInfo, SharedState};

pub const BASE_URL: &str = "http://26.174.248.219:8080";
pub const REQUEST_TIMEOUT: u64 = 60;

pub fn is_ok_status(status: u16) -> bool {
    matches!(status, 200 | 201 | 202 | 204)
}

pub fn server_url() -> reqwest::Client {
    reqwest::Client::builder()
        .timeout(Duration::from_secs(REQUEST_TIMEOUT))
        .build()
        .expect("build client")
}

pub fn parse_body(text: &str) -> Value {
    serde_json::from_str::<Value>(text).unwrap_or(Value::Null)
}

pub fn parse_member(member: &Value) -> MemberInfo {
    MemberInfo {
        id: member.get("id").and_then(|v| v.as_str()).unwrap_or("").to_string(),
        name: member.get("name").and_then(|v| v.as_str()).unwrap_or("").to_string(),
    }
}

pub fn parse_group(group: &Value) -> GroupEntry {
    GroupEntry {
        id: group.get("group_id").and_then(|v| v.as_str()).unwrap_or("").to_string(),
        name: group.get("name").and_then(|v| v.as_str()).unwrap_or("").to_string(),
        owner_id: group.get("owner_id").and_then(|v| v.as_str()).unwrap_or("").to_string(),
        members: Vec::new(),
    }
}

pub fn parse_groups(member: &Value) -> Vec<GroupEntry> {
    member
        .get("groups")
        .and_then(|v| v.as_array())
        .map(|arr| arr.iter().map(parse_group).collect())
        .unwrap_or_default()
}

pub async fn upsert_group(state: &SharedState, group: GroupEntry) {
    let mut groups = state.groups.write().await;
    let entry = groups.entry(group.id.clone()).or_insert_with(|| GroupEntry {
        id: group.id.clone(),
        name: group.name.clone(),
        owner_id: group.owner_id.clone(),
        members: Vec::new(),
    });
    if !group.name.is_empty() {
        entry.name = group.name;
    }
    if !group.owner_id.is_empty() {
        entry.owner_id = group.owner_id;
    }
    let user = state.current_user.read().await;
    if !user.id.is_empty() && !entry.members.iter().any(|m| m.id == user.id) {
        entry.members.push(user.clone());
    }
    if !entry.owner_id.is_empty() && !entry.members.iter().any(|m| m.id == entry.owner_id) {
        entry.members.push(MemberInfo {
            id: entry.owner_id.clone(),
            name: "群主".to_string(),
        });
    }
}

pub async fn rebuild_groups(state: &SharedState, member: &Value) {
    let current = parse_member(member);
    *state.current_user.write().await = current.clone();
    let mut groups = state.groups.write().await;
    groups.clear();
    for group in parse_groups(member) {
        let mut g = group;
        if !g.members.iter().any(|m| m.id == current.id) {
            g.members.push(current.clone());
        }
        if !g.owner_id.is_empty() && !g.members.iter().any(|m| m.id == g.owner_id) {
            g.members.push(MemberInfo {
                id: g.owner_id.clone(),
                name: "群主".to_string(),
            });
        }
        groups.insert(g.id.clone(), g);
    }
}

pub async fn request_json(
    client: &reqwest::Client,
    method: &str,
    path: &str,
    token: Option<&str>,
    body: Option<Value>,
) -> Result<(u16, Value), String> {
    let url = format!("{BASE_URL}{path}");
    let mut req = match method {
        "POST" => client.post(&url),
        "PUT" => client.put(&url),
        "DELETE" => client.delete(&url),
        _ => client.get(&url),
    };
    if let Some(t) = token {
        req = req.header("Authorization", t);
    }
    if let Some(b) = body {
        req = req.json(&b);
    }
    let resp = req.send().await.map_err(|e| e.to_string())?;
    let status = resp.status().as_u16();
    let text = resp.text().await.map_err(|e| e.to_string())?;
    Ok((status, parse_body(&text)))
}

pub async fn health_check() -> Result<(bool, String), String> {
    let resp = server_url().get(format!("{BASE_URL}/health")).send().await.map_err(|_| "無法連線後端".to_string())?;
    let status = resp.status().as_u16();
    if status == 200 {
        Ok((true, "ok".to_string()))
    } else {
        Ok((false, format!("後端異常（HTTP {status}）")))
    }
}

pub async fn login_request(
    client: &reqwest::Client,
    email: &str,
    password: &str,
) -> Result<(Value, String), String> {
    let (status, body) = request_json(
        client,
        "POST",
        "/auth/login",
        None,
        Some(json!({"email": email, "password": password})),
    )
    .await?;
    if !matches!(status, 200 | 201) {
        return Ok((json!({}), String::new()));
    }
    let data = body.get("data").cloned().unwrap_or_else(|| json!({}));
    let token = data
        .get("token")
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .to_string();
    Ok((data, token))
}

pub async fn register_request(
    client: &reqwest::Client,
    name: &str,
    email: &str,
    password: &str,
) -> Result<(u16, Value, String), String> {
    let (status, body) = request_json(
        client,
        "POST",
        "/auth/register",
        None,
        Some(json!({"name": name, "email": email, "password": password})),
    )
    .await?;
    let data = body.get("data").cloned().unwrap_or_else(|| json!({}));
    let token = data
        .get("token")
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .to_string();
    Ok((status, data, token))
}
