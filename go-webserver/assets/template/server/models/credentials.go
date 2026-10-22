package models

import (
	time "time"

	db "{{MODULE_PATH}}/server/db"
	encryption "{{MODULE_PATH}}/server/encryption"
)

// Revoked counts what RevokeAllCredentials took away, or what DeleteUser
// removed along with the account -- the same three things either way.
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
//
// allow_owner is false for the web route, so no admin can cut the owner off,
// and true for the CLI and control socket: when it is the owner's own
// credentials that leaked, an operator with shell access still needs this.
func RevokeAllCredentials( store *db.Store , user_id uint64 , allow_owner bool ) ( revoked Revoked , err error ) {
	err = store.Write( func( tx *db.Tx ) ( tx_err error ) {
		if tx_err = tx.Get( db.BucketUsers , encryption.Uint64ToBytes( user_id ) , &User{} ); tx_err != nil { return }
		if allow_owner == false {
			if tx_err = refuseOwnerTx( tx , user_id ); tx_err != nil { return }
		}
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

// DeleteUser removes an account and everything it could sign in with -- every
// session, every login link (used or not), every API key row -- in one
// transaction. Unlike disabling, it cannot be undone: the id is never reused
// (see CreateUser), so the account cannot come back even by accident.
//
// actor_id is who is asking. Zero means the operator, through the CLI or the
// control socket, who may delete any account but the owner. Anyone else is
// an admin acting through the web, and the rules are:
//
//   - nobody deletes themselves (ErrDeleteSelf);
//   - nobody deletes the owner (ErrOwnerProtected) -- transfer it first;
//   - only the owner deletes another admin (ErrOwnerRequired).
//
// The checks run inside the transaction, so ownership moving between a check
// and the delete cannot let a delete through that the new state would refuse.
//
// Records an app adds that point at a user id are not touched here. When you
// add one, decide whether it should go with the account (delete it in this
// transaction) or outlive it (render a missing user as "#<id>").
func DeleteUser( store *db.Store , user_id uint64 , actor_id uint64 ) ( removed Revoked , err error ) {
	err = store.Write( func( tx *db.Tx ) ( tx_err error ) {
		target := &User{}
		if tx_err = tx.Get( db.BucketUsers , encryption.Uint64ToBytes( user_id ) , target ); tx_err != nil { return }
		if actor_id != 0 && actor_id == user_id {
			tx_err = ErrDeleteSelf
			return
		}
		owner_id , tx_err := ownerIDTx( tx )
		if tx_err != nil { return }
		if owner_id == user_id {
			tx_err = ErrOwnerProtected
			return
		}
		if actor_id != 0 && target.IsAdmin() && actor_id != owner_id {
			tx_err = ErrOwnerRequired
			return
		}

		if removed.Sessions , tx_err = destroySessionsForUser( tx , user_id ); tx_err != nil { return }
		if removed.LoginLinks , tx_err = deleteLoginTokensForUser( tx , user_id ); tx_err != nil { return }
		removed.APIKeys , tx_err = tx.DeleteWhere( db.BucketAPIKeys ,
			func() any { return &APIKey{} } ,
			func( key []byte , item any ) bool {
				return item.( *APIKey ).UserID == user_id
			} )
		if tx_err != nil { return }
		tx_err = tx.Delete( db.BucketUsers , encryption.Uint64ToBytes( user_id ) )
		return
	} )
	if err != nil { removed = Revoked{} }
	return
}
