-- Migration 004: User accounts, invite codes, and token ownership.
-- REF: SPR-017 (User Model + Auth Backend)

-- Users table for authentication and authorization.
CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    username      TEXT UNIQUE NOT NULL,
    email         TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'user',
    created_at    TEXT NOT NULL,
    last_login_at TEXT,
    is_active     INTEGER NOT NULL DEFAULT 1
);

-- Invite codes for controlled registration.
CREATE TABLE IF NOT EXISTS invite_codes (
    id          TEXT PRIMARY KEY,
    code        TEXT UNIQUE NOT NULL,
    created_by  TEXT NOT NULL REFERENCES users(id),
    used_by     TEXT REFERENCES users(id),
    used_at     TEXT,
    created_at  TEXT NOT NULL,
    is_active   INTEGER NOT NULL DEFAULT 1
);
