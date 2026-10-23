// Package models holds one file per stored record type. There is no ORM and
// no query builder -- each file is a small set of plain functions over the db
// store, which is enough for a bolt key/value file and keeps the storage
// layer readable end to end.
package models

import (
	strings "strings"
	time "time"

	db "{{MODULE_PATH}}/server/db"
	encryption "{{MODULE_PATH}}/server/encryption"
)

const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// User has no password field, and adding one would undo the entire design
// here: identity is proven only by redeeming a login link or presenting a
// session cookie. See references/architecture.md before changing that.
type User struct {
	ID          uint64     `json:"id"`
	DisplayName string     `json:"display_name"`
	Role        string     `json:"role"`
	CreatedAt   time.Time  `json:"created_at"`
	DisabledAt  *time.Time `json:"disabled_at,omitempty"`
	// PinHash is the optional PIN asked for when someone signs back in
	// through an invite link. It is not a password: nothing else checks it.
	// See pin.go.
	PinHash string `json:"pin_hash,omitempty"`
}

func ( user *User ) Disabled() ( result bool ) {
	result = user.DisabledAt != nil
	return
}

func ( user *User ) IsAdmin() ( result bool ) {
	result = user.Role == RoleAdmin
	return
}

func ValidRole( role string ) ( result bool ) {
	result = role == RoleAdmin || role == RoleUser
	return
}

// ValidDisplayName bounds the one piece of free text a user controls. The cap
// is about keeping the database and the UI sane; escaping for display is the
// frontend's job (see static/js/dom.js, which never uses innerHTML for
// user-supplied values).
func ValidDisplayName( display_name string ) ( result bool ) {
	trimmed := strings.TrimSpace( display_name )
	result = len( trimmed ) >= 1 && len( trimmed ) <= 80
	return
}

// CreateUser stores a new account. The first admin ever created -- by the
// first-run bootstrap or `manage bootstrap-admin` -- also becomes the owner,
// in the same transaction, so there is no moment where admins exist and no
// owner does. See owner.go.
//
// Ids come from the bucket's sequence and are never reused, so a deleted
// account's id can never come back as a different person.
func CreateUser( store *db.Store , display_name string , role string ) ( user *User , err error ) {
	err = store.Write( func( tx *db.Tx ) ( tx_err error ) {
		user , tx_err = createUserTx( tx , display_name , role )
		return
	} )
	if err != nil { user = nil }
	return
}

// createUserTx is CreateUser inside the caller's transaction, for an account
// that has to be created together with something else -- joining through an
// invite link creates the account, takes a place on the link, and starts the
// session all at once.
func createUserTx( tx *db.Tx , display_name string , role string ) ( user *User , err error ) {
	next_id , err := tx.NextSequence( db.BucketUsers )
	if err != nil { return }
	user = &User{
		ID:          next_id,
		DisplayName: strings.TrimSpace( display_name ),
		Role:        role,
		CreatedAt:   time.Now().UTC(),
	}
	if err = tx.Put( db.BucketUsers , encryption.Uint64ToBytes( user.ID ) , user ); err != nil { return }
	if role != RoleAdmin { return }
	owner_id , err := ownerIDTx( tx )
	if err != nil || owner_id != 0 { return }
	err = setOwnerTx( tx , user.ID )
	return
}

func GetUser( store *db.Store , user_id uint64 ) ( user *User , err error ) {
	user = &User{}
	err = store.Get( db.BucketUsers , encryption.Uint64ToBytes( user_id ) , user )
	if err != nil { user = nil }
	return
}

func ListUsers( store *db.Store ) ( users []*User , err error ) {
	users = []*User{}
	err = store.ForEach( db.BucketUsers ,
		func() any { return &User{} } ,
		func( key []byte , item any ) bool {
			users = append( users , item.( *User ) )
			return true
		} )
	return
}

func SaveUser( store *db.Store , user *User ) ( err error ) {
	err = store.Put( db.BucketUsers , encryption.Uint64ToBytes( user.ID ) , user )
	return
}

// RenameUser and SetUserDisabled both edit a record that already exists, so
// the read and the write land in one transaction rather than GetUser followed
// by SaveUser. That is what stops two concurrent admin requests from each
// reading the old row and the slower one silently discarding the other's
// change.
func RenameUser( store *db.Store , user_id uint64 , display_name string ) ( err error ) {
	err = store.UpdateValue( db.BucketUsers , encryption.Uint64ToBytes( user_id ) ,
		func() any { return &User{} } ,
		func( item any ) ( mutate_err error ) {
			item.( *User ).DisplayName = strings.TrimSpace( display_name )
			return
		} )
	return
}

// SetUserDisabled disables or re-enables an account. Disabling also deletes
// every session the account holds and every login link it has not used yet,
// in the same transaction as the change itself: a session or a link that
// survived would come back to life the moment the account was re-enabled.
// API keys are deliberately left alone -- see LoadAPIKey -- and use
// RevokeAllCredentials when they should die too.
//
// The owner cannot be disabled by any route, the CLI included: the owner is
// always an enabled admin (see owner.go). Transfer ownership first.
func SetUserDisabled( store *db.Store , user_id uint64 , disabled bool ) ( err error ) {
	err = store.Write( func( tx *db.Tx ) ( tx_err error ) {
		key := encryption.Uint64ToBytes( user_id )
		user := &User{}
		if tx_err = tx.Get( db.BucketUsers , key , user ); tx_err != nil { return }
		if disabled {
			if tx_err = refuseOwnerTx( tx , user_id ); tx_err != nil { return }
			now := time.Now().UTC()
			user.DisabledAt = &now
		} else {
			user.DisabledAt = nil
		}
		if tx_err = tx.Put( db.BucketUsers , key , user ); tx_err != nil { return }
		if disabled == false { return }
		if _ , tx_err = destroySessionsForUser( tx , user_id ); tx_err != nil { return }
		_ , tx_err = deletePendingLoginTokens( tx , user_id )
		return
	} )
	return
}

// AnyAdminExists decides whether the server prints a first-run login link on
// boot. Once one admin exists, that bootstrap path stays closed.
func AnyAdminExists( store *db.Store ) ( result bool , err error ) {
	err = store.ForEach( db.BucketUsers ,
		func() any { return &User{} } ,
		func( key []byte , item any ) bool {
			user := item.( *User )
			if user.IsAdmin() && user.Disabled() == false {
				result = true
				return false
			}
			return true
		} )
	return
}
