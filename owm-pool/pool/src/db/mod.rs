//! Database module.
//!
//! SQL migrations live in `./migrations/` and are embedded via `sqlx::migrate!`.
//! Future schema helpers (connection pool setup, query functions) will be added
//! as submodules here (e.g. `schema.rs`).
