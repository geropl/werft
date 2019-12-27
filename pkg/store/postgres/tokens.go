package postgres

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/32leaves/werft/pkg/store"
)

// TokenStore provides postgres backed token store
type TokenStore struct {
	DB *sql.DB
}

// NewTokenStore creates a new SQL token store
func NewTokenStore(db *sql.DB) (*TokenStore, error) {
	return &TokenStore{DB: db}, nil
}

// Store stores a user-token. If a token previously existed for the user,
// this is added additionally.
func (ngrp *TokenStore) Store(token, user string) error {
	_, err := ngrp.DB.Query(`
		INSERT
		INTO   user_tokens (token, user_name)
		VALUES             ($1   , $2       )`,
		token, user,
	)
	return err
}

// Get retrieves a user based on the token.
func (ngrp *TokenStore) Get(token string) (user string, err error) {
	err = ngrp.DB.QueryRow(`
		SELECT user_name
		FROM   user_tokens
		WHERE  token = $1`,
		token,
	).Scan(&user)
	if err == sql.ErrNoRows {
		return "", store.ErrNotFound
	}
	return
}

// Prune removes all tokens older than maxAge
func (ngrp *TokenStore) Prune(maxAge time.Duration) error {
	interval := fmt.Sprintf("%.0f seconds", maxAge.Seconds())
	_, err := ngrp.DB.Query(`
		DELETE 
		FROM  user_tokens
		WHERE created < NOW() - INTERVAL $1`,
		interval,
	)
	return err
}
