-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email VARCHAR(255) NOT NULL UNIQUE,
  password_hash VARCHAR(255) NOT NULL,
  full_name VARCHAR(255),
  -- If a user has created a listing - house for rent
  is_host BOOLEAN DEFAULT FALSE,
  last_login TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE profiles (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  avatar_url VARCHAR(255) DEFAULT NULL,
  profile_highlights VARCHAR[] DEFAULT NULL,
  about_me TEXT DEFAULT NULL,
  interests VARCHAR[] DEFAULT NULL,
  my_destinations VARCHAR[] DEFAULT NULL
);

-- +goose Down
DROP TABLE IF EXISTS profiles;
DROP TABLE IF EXISTS users;
DROP EXTENSION IF EXISTS pgcrypto;
