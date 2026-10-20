package models

import (
	time "time"

	db "{{MODULE_PATH}}/server/db"
	encryption "{{MODULE_PATH}}/server/encryption"
)

// Revoked counts what RevokeAllCredentials took away.
type Revoked struct {
	Sessions   int `json:"sessions"`
	LoginLinks int `json:"login_links"`
	APIKeys    int `json:"api_keys"`
}

// RevokeAllCredentials is the "this account may be compromised" operation:
// every session is deleted, every unused login link is deleted, and every
// live API key is revoked, in one transaction. Afterwards nothing issued
// before this moment can act as the account; the way back in is a new login
// link from an admin.
//
// It is deliberately separate from the two operations it is easy to mistake
// it for. Issuing a new login link replaces the old link but leaves existing
// sessions alone (someone adding a second device should not be signed out of
// the first), and disabling leaves API keys alone (an account disabled for an
// afternoon should not need every integration re-keyed). Neither is a reset.
func RevokeAllCredentials( store *db.Store , user_id uint64 ) ( revoked Revoked , err error ) {
	err = store.Write( func( tx *db.Tx ) ( tx_err error ) {
		if tx_err = tx.Get( db.BucketUsers , encryption.Uint64ToBytes( user_id ) , &User{} ); tx_err != nil { return }
		if revoked.Sessions , tx_err = destroySessionsForUser( tx , user_id ); tx_err != nil { return }
		if revoked.LoginLinks , tx_err = deletePendingLoginTokens( tx , user_id ); tx_err != nil { return }

		live := []*APIKey{}
		tx_err = tx.ForEach( db.BucketAPIKeys ,
			func() any { return &APIKey{} } ,
			func( key []byte , item any ) bool {
				stored := item.( *APIKey )
				if stored.UserID == user_id && stored.Live() {
					live = append( live , stored )
				}
				return true
			} )
		if tx_err != nil { return }
		now := time.Now().UTC()
		for _ , key := range live {
			key.RevokedAt = &now
			if tx_err = tx.Put( db.BucketAPIKeys , []byte( key.ID ) , key ); tx_err != nil { return }
		}
		revoked.APIKeys = len( live )
		return
	} )
	if err != nil { revoked = Revoked{} }
	return
}
