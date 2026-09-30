CREATE TABLE goals (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users (id),
    name TEXT NOT NULL,
    target_minor BIGINT NOT NULL,
    saved_minor BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT goals_name_nonempty CHECK (btrim(name) <> ''),
    CONSTRAINT goals_target_positive CHECK (target_minor > 0),
    CONSTRAINT goals_saved_nonnegative CHECK (saved_minor >= 0)
);

CREATE INDEX goals_user_created_id_idx ON goals (user_id, created_at DESC, id DESC);
