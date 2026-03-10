mod config;
mod db;

use config::Config;

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let _cfg: Config = Config::load("config.toml")?;
    println!("owm-pool starting...");
    Ok(())
}
