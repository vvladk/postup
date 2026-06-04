CREATE TABLE users (
    id                      INTEGER PRIMARY KEY AUTOINCREMENT,
    email                   TEXT    NOT NULL UNIQUE,
    password_hash           TEXT    NOT NULL,
    first_name              TEXT    NOT NULL DEFAULT '',
    last_name               TEXT    NOT NULL DEFAULT '',
    role                    TEXT    NOT NULL DEFAULT 'member' CHECK (role IN ('admin', 'member')),
    invite_token            TEXT,
    invite_used_at          TEXT,
    reset_token             TEXT,
    reset_token_expires_at  TEXT,
    created_at              TEXT    NOT NULL,
    updated_at              TEXT    NOT NULL
);

CREATE TABLE sessions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token       TEXT    NOT NULL UNIQUE,
    expires_at  TEXT    NOT NULL,
    created_at  TEXT    NOT NULL
);

CREATE TABLE teams (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL,
    created_at  TEXT    NOT NULL
);

CREATE TABLE team_members (
    team_id  INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (team_id, user_id)
);

CREATE TABLE templates (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL,
    status      TEXT    NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    created_at  TEXT    NOT NULL
);

CREATE TABLE template_columns (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    template_id  INTEGER NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    title        TEXT    NOT NULL,
    position     INTEGER NOT NULL,
    type         TEXT    NOT NULL DEFAULT 'regular' CHECK (type IN ('fixed_first', 'regular', 'fixed_last'))
);

CREATE TABLE retros (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    team_id      INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    template_id  INTEGER REFERENCES templates(id) ON DELETE SET NULL,
    date         TEXT    NOT NULL,
    vote_limit   INTEGER NOT NULL DEFAULT 10,
    status       TEXT    NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'finished')),
    created_at   TEXT    NOT NULL
);

CREATE TABLE retro_participants (
    retro_id  INTEGER NOT NULL REFERENCES retros(id) ON DELETE CASCADE,
    user_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (retro_id, user_id)
);

CREATE TABLE cards (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    retro_id    INTEGER NOT NULL REFERENCES retros(id) ON DELETE CASCADE,
    column_id   INTEGER NOT NULL REFERENCES template_columns(id) ON DELETE CASCADE,
    author_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content     TEXT    NOT NULL DEFAULT '',
    created_at  TEXT    NOT NULL,
    updated_at  TEXT    NOT NULL
);

CREATE TABLE votes (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    card_id  INTEGER NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    user_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    count    INTEGER NOT NULL DEFAULT 0,
    UNIQUE (card_id, user_id)
);

CREATE TABLE action_statuses (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL,
    position    INTEGER NOT NULL,
    is_default  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE action_items (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    retro_id     INTEGER NOT NULL REFERENCES retros(id) ON DELETE CASCADE,
    card_id      INTEGER REFERENCES cards(id) ON DELETE SET NULL,
    assignee_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    deadline     TEXT    NOT NULL,
    status_id    INTEGER NOT NULL REFERENCES action_statuses(id),
    created_at   TEXT    NOT NULL
);

-- Indexes
CREATE INDEX idx_sessions_token    ON sessions(token);
CREATE INDEX idx_sessions_user_id  ON sessions(user_id);
CREATE INDEX idx_cards_retro_id    ON cards(retro_id);
CREATE INDEX idx_votes_card_id     ON votes(card_id);
CREATE INDEX idx_action_items_retro_id ON action_items(retro_id);
