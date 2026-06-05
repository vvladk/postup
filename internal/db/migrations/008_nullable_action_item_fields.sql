CREATE TABLE action_items_new (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    retro_id     INTEGER NOT NULL REFERENCES retros(id) ON DELETE CASCADE,
    card_id      INTEGER REFERENCES cards(id) ON DELETE SET NULL,
    assignee_id  INTEGER REFERENCES users(id) ON DELETE SET NULL,
    deadline     TEXT,
    status_id    INTEGER NOT NULL REFERENCES action_statuses(id),
    created_at   TEXT    NOT NULL,
    content      TEXT    NOT NULL DEFAULT '',
    column_id    INTEGER
);

INSERT INTO action_items_new SELECT * FROM action_items;

DROP TABLE action_items;

ALTER TABLE action_items_new RENAME TO action_items;

CREATE INDEX idx_action_items_retro_id ON action_items(retro_id);
