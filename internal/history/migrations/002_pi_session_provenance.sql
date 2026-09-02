ALTER TABLE learning_attempts
ADD COLUMN pi_session_id TEXT
CHECK (
    pi_session_id IS NULL OR (
        length(CAST(pi_session_id AS BLOB)) BETWEEN 1 AND 128 AND
        length(pi_session_id) = length(CAST(pi_session_id AS BLOB)) AND
        pi_session_id NOT GLOB '*[^A-Za-z0-9._-]*' AND
        substr(pi_session_id, 1, 1) GLOB '[A-Za-z0-9]' AND
        substr(pi_session_id, -1, 1) GLOB '[A-Za-z0-9]'
    )
);
