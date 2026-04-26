-- SQL sample
CREATE TABLE IF NOT EXISTS users (
    id           SERIAL PRIMARY KEY,
    email        TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS users_email_idx ON users (LOWER(email));

WITH active_users AS (
    SELECT id, email, display_name
    FROM users
    WHERE created_at > NOW() - INTERVAL '30 days'
)
SELECT au.id,
       au.email,
       COUNT(p.id) AS post_count
FROM active_users AS au
LEFT JOIN posts AS p ON p.author_id = au.id
GROUP BY au.id, au.email
HAVING COUNT(p.id) >= 1
ORDER BY post_count DESC
LIMIT 50;

UPDATE users
SET updated_at = NOW()
WHERE id IN (SELECT id FROM active_users);
