CREATE TABLE IF NOT EXISTS user_tokens (
	token     varchar(64)  NOT NULL PRIMARY KEY,
	user_name varchar(255) NOT NULL,
    created   timestamp    NOT NULL DEFAULT NOW()
);
