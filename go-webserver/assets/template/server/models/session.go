package models

import (
	strings "strings"
	time "time"

	db "{{MODULE_PATH}}/server/db"
	encryption "{{MODULE_PATH}}/server/encryption"
)

// Session is the server side of a login. The cookie never carries the user id
// or the role -- only an opaque reference -- so revoking access is a matter of
// deleting this record, with no token denylist to maintain.
//
// The credential is shaped "<id>.<secret>": the id is the bolt key, so lookup
// is a single O(1) fetch, and only the secret's hash is stored. A stolen
// database therefore contains nothing that can be replayed as a cookie.
type Session struct {
	UserID     uint64    `json:"user_id"`
	SecretHash string    `json:"secret_hash"`
	CSRFToken  string    `json:"csrf_token"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

func splitCredential( raw string ) ( id string , secret string , ok bool ) {
	parts := strings.SplitN( raw , "." , 2 )
	if len( parts ) != 2 || parts[ 0 ] == "" || parts[ 1 ] == "" { return }
	id , secret , ok = parts[ 0 ] , parts[ 1 ] , true
	return
}

// newSession builds a session without storing it. The only caller that
// creates sessions is RedeemLoginToken, which stores it in the same
// transaction that consumes the login link and checks the account is enabled
// -- so there is deliberately no CreateSession that writes one on its own.
func newSession( user_id uint64 , ttl time.Duration ) ( session_id string , credential string , session *Session ) {
	session_id = encryption.GenerateURLToken( 16 )
	secret := encryption.GenerateURLToken( 32 )
	now := time.Now().UTC()
	session = &Session{
		UserID:     user_id,
		SecretHash: encryption.Sha256Hex( secret ),
		CSRFToken:  encryption.GenerateURLToken( 24 ),
		CreatedAt:  now,
		ExpiresAt:  now.Add( ttl ),
	}
	credential = session_id + "." + secret
	return
}

// LoadSession returns nil (with no error) for anything that is not a live
// session -- wrong shape, unknown id, bad secret, expired, or belonging to a
// disabled user. Callers only ever need to ask "am I logged in", and folding
// every failure into the same answer avoids leaking which check failed.
func LoadSession( store *db.Store , credential string ) ( session *Session , user *User ) {
	session_id , secret , ok := splitCredential( credential )
	if ok == false { return }

	found := &Session{}
	if err := store.Get( db.BucketSessions , []byte( session_id ) , found ); err != nil { return }

	if encryption.ConstantTimeEqual( found.SecretHash , encryption.Sha256Hex( secret ) ) == false { return }
	if time.Now().UTC().After( found.ExpiresAt ) {
		store.Delete( db.BucketSessions , []byte( session_id ) )
		return
	}

	loaded_user , err := GetUser( store , found.UserID )
	if err != nil || loaded_user.Disabled() { return }

	session , user = found , loaded_user
	return
}

func DestroySession( store *db.Store , credential string ) ( err error ) {
	session_id , _ , ok := splitCredential( credential )
	if ok == false { return }
	err = store.Delete( db.BucketSessions , []byte( session_id ) )
	return
}

// destroySessionsForUser deletes every session an account holds, inside the
// caller's transaction. It is not exported because deleting sessions is only
// ever half of a revocation: SetUserDisabled and RevokeAllCredentials run it
// together with the account change in one transaction, so a login link
// redeemed at the same moment either lands before (and its session is
// deleted here) or after (and sees the change).
func destroySessionsForUser( tx *db.Tx , user_id uint64 ) ( removed int , err error ) {
	removed , err = tx.DeleteWhere( db.BucketSessions ,
		func() any { return &Session{} } ,
		func( key []byte , item any ) bool {
			return item.( *Session ).UserID == user_id
		} )
	return
}

// PurgeExpiredSessions is called on a timer from main. Expired rows are
// already refused by LoadSession, so this is only housekeeping to stop the
// bolt file growing without bound -- but it still runs as one transaction,
// because bolt has a single writer and an hourly sweep that took one
// transaction per row would stall every request handler waiting to write.
func PurgeExpiredSessions( store *db.Store ) ( removed int , err error ) {
	now := time.Now().UTC()
	removed , err = store.DeleteWhere( db.BucketSessions ,
		func() any { return &Session{} } ,
		func( key []byte , item any ) bool {
			return now.After( item.( *Session ).ExpiresAt )
		} )
	return
}
