use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::{Mutex, RwLock};

#[derive(Clone, Default, Serialize, Deserialize)]
pub struct MemberInfo {
    pub id: String,
    pub name: String,
}

#[derive(Clone, Default)]
pub struct GroupEntry {
    pub id: String,
    pub name: String,
    pub owner_id: String,
    pub members: Vec<MemberInfo>,
}

#[derive(Default)]
pub struct AppState {
    pub token: RwLock<String>,
    pub current_user: RwLock<MemberInfo>,
    pub groups: RwLock<HashMap<String, GroupEntry>>,
    pub messages: RwLock<HashMap<String, Vec<Value>>>,
    pub ws_busy: Mutex<bool>,
}

pub type SharedState = Arc<AppState>;
