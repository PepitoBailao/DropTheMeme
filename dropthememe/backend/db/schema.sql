-- Schéma DropTheMeme V1
-- À exécuter une seule fois sur ta base PostgreSQL

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Utilisateurs (identifiés par leur SteamID)
CREATE TABLE IF NOT EXISTS users (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    steam_id    VARCHAR(20) UNIQUE NOT NULL,
    username    VARCHAR(64) NOT NULL,
    avatar_url  TEXT,
    created_at  TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    last_seen   TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Groupes
CREATE TABLE IF NOT EXISTS groups (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name        VARCHAR(64) NOT NULL,
    invite_code VARCHAR(8) UNIQUE NOT NULL,
    owner_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Membres des groupes
CREATE TABLE IF NOT EXISTS group_members (
    group_id    UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    joined_at   TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (group_id, user_id)
);

-- Messages
CREATE TABLE IF NOT EXISTS messages (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    group_id    UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type        VARCHAR(10) NOT NULL CHECK (type IN ('text','image','audio','video','gif')),
    content     TEXT NOT NULL,        -- texte brut OU url R2 pour les médias
    file_size   INTEGER,              -- taille en octets (null pour texte)
    duration    INTEGER,              -- durée en secondes (audio/vidéo uniquement)
    created_at  TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Index utiles
CREATE INDEX IF NOT EXISTS idx_messages_group_id ON messages(group_id);
CREATE INDEX IF NOT EXISTS idx_group_members_user_id ON group_members(user_id);
CREATE INDEX IF NOT EXISTS idx_users_steam_id ON users(steam_id);
