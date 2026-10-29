use std::sync::OnceLock;
use serde::Deserialize;

#[derive(Debug, Deserialize)]
pub struct Config {
    pub url: String,
    pub timeout: u64,
    pub ws_url: String,
}

fn env(key: &str, default: &str) -> String {
    std::env::var(key).unwrap_or_else(|_| default.to_string())
}

impl Config {
    fn load() -> Self {
        // Try loading .env from common locations (src-tauri/.env, package root)
        for path in [
            "src-tauri/.env".to_string(),
            ".env".to_string(),
            std::env::current_dir()
                .map(|d| d.join("src-tauri/.env"))
                .map(|p| p.to_string_lossy().into_owned())
                .unwrap_or_default(),
        ] {
            if !path.is_empty() {
                let _ = dotenvy::from_path(&path);
            }
        }

        let url = env("URL", "http://localhost:8080");
        let url = url.trim_end_matches('/').to_string();

        let timeout = env("TIMEOUT", "5")
            .parse::<u64>()
            .unwrap_or(5);

        let ws_url = env("WS_URL", "");
        let ws_url = if ws_url.is_empty() {
            url.replace("https://", "wss://").replace("http://", "ws://")
        } else {
            ws_url
        };

        Config { url, timeout, ws_url }
    }
}

pub fn get() -> &'static Config {
    static INSTANCE: OnceLock<Config> = OnceLock::new();
    INSTANCE.get_or_init(Config::load)
}
