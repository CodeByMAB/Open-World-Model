#[derive(Debug, serde::Deserialize)]
pub struct Config {
    pub pool: PoolConfig,
    pub payout: PayoutConfig,
    pub lightning: LightningConfig,
    pub database: DatabaseConfig,
    pub observer: ObserverConfig,
}

#[derive(Debug, serde::Deserialize)]
pub struct PoolConfig {
    pub listen_addr: String,
    pub bitcoin_rpc_url: String,
    pub bitcoin_rpc_user: String,
    pub bitcoin_rpc_pass: String,
    pub bitcoin_zmq_addr: String,
    pub coordinator_internal_url: String,
    pub min_payout_sats: i64,
    pub payout_interval_seconds: u64,
}

#[derive(Debug, serde::Deserialize)]
pub struct PayoutConfig {
    pub min_payout_sats: i64,
    pub payout_interval_seconds: u64,
    pub treasury_ln_uri: String,
}

#[derive(Debug, serde::Deserialize)]
pub struct LightningConfig {
    pub lnd_host: String,
    pub payment_macaroon_path: String,
    pub lnd_tls_cert_path: String,
}

#[derive(Debug, serde::Deserialize)]
pub struct DatabaseConfig {
    pub dsn: String,
}

#[derive(Debug, serde::Deserialize)]
pub struct ObserverConfig {
    pub enabled: bool,
    pub api_endpoint: String,
    pub signing_key_path: String,
}

impl Config {
    pub fn load(path: &str) -> anyhow::Result<Self> {
        let contents = std::fs::read_to_string(path)?;
        let config = toml::from_str(&contents)?;
        Ok(config)
    }
}
