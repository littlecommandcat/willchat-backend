use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::HashMap;
use std::sync::Arc;
// use std::time::Duration;
use tokio::sync::{Mutex, RwLock};
// use smart_default::SmartDefault;

// #[derive(Clone, SmartDefault)]
// pub struct Network {
//     pub latency: i64,
//     #[default = 0]
//     pub tries: i64,
// }

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
    pub ws_status: RwLock<String>,
    pub token: RwLock<String>,
    pub current_user: RwLock<MemberInfo>,
    pub groups: RwLock<HashMap<String, GroupEntry>>,
    pub messages: RwLock<HashMap<String, Vec<Value>>>,
    pub ws_busy: Mutex<bool>,
    pub ws_task: Mutex<Option<tokio::task::JoinHandle<()>>>,
    // pub network: RwLock<Network>,
}

pub type SharedState = Arc<AppState>;
