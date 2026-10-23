package models

import (
	errors "errors"
	sort "sort"
	strings "strings"
	time "time"

	db "{{MODULE_PATH}}/server/db"
	encryption "{{MODULE_PATH}}/server/encryption"
)

var (
	ErrInviteLinkInvalid = errors.New( "invite link is invalid, expired, or switched off" )
	ErrInviteLinkFull    = errors.New( "invite link has no places left" )
	ErrNotInviteAccount  = errors.New( "that account did not join through this invite link" )
	ErrBadInviteLink     = errors.New( "label must be 1-80 characters, uses 1-1000, days 1-365" )
	ErrInviteNotAdmin    = errors.New( "an invite link must be made by an enabled admin" )
)

const (
	InviteLinkMaxUses = 1000
	InviteLinkMaxDays = 365

	// inviteLinkRetention is how long a dead link's row is kept before the
	// purge loop drops it, so the admin list still shows last week's links.
	inviteLinkRetention = 30 * 24 * time.Hour
)

// InviteLink is one link for a whole group -- posted in a group chat, say.
// Each person who opens it types their name. A new name becomes a new
// account (always role user), up to MaxUses of them. A name close to one
// that already joined through this link is offered back ("did you
// mean...?"), which is how the same person signs in on their next device.
//
// Picking an existing name signs you in as that person, so anyone holding
// the link can be anyone who joined through it. Two limits keep that sane:
// only accounts this link created can be picked (never an admin, never
// someone who joined another way), and a person can set a PIN, which is
// then asked for. See pin.go.
//
// Same "<id>.<secret>" shape as the other credentials. The secret is a long
// random token, so sha256 is enough; bcrypt is for low-entropy secrets.
//
// Accounts is append-only. Deleting an account does not hand its place back
// -- the link was for that many people, and the id is never reused, so the
// stale entry simply never matches anything again.
type InviteLink struct {
	ID         string     `json:"id"`
	SecretHash string     `json:"secret_hash"`
	Label      string     `json:"label"`
	MaxUses    int        `json:"max_uses"`
	Accounts   []uint64   `json:"accounts"`
	CreatedBy  uint64     `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

func ( link *InviteLink ) Remaining() ( result int ) {
	result = link.MaxUses - len( link.Accounts )
	if result < 0 { result = 0 }
	return
}

func ( link *InviteLink ) Live() ( result bool ) {
	result = link.RevokedAt == nil && time.Now().UTC().Before( link.ExpiresAt )
	return
}

func ( link *InviteLink ) Owns( user_id uint64 ) ( result bool ) {
	for _ , account := range link.Accounts {
		if account == user_id {
			result = true
			return
		}
	}
	return
}

func ValidInviteLink( label string , max_uses int , days int ) ( result bool ) {
	trimmed := strings.TrimSpace( label )
	result = len( trimmed ) >= 1 && len( trimmed ) <= 80 &&
		max_uses >= 1 && max_uses <= InviteLinkMaxUses &&
		days >= 1 && days <= InviteLinkMaxDays
	return
}

// IssueInviteLink returns the raw credential exactly once, like a login
// link: only its hash is stored.
func IssueInviteLink( store *db.Store , label string , max_uses int , ttl time.Duration , created_by uint64 ) ( credential string , link *InviteLink , err error ) {
	id := encryption.GenerateURLToken( 12 )
	secret := encryption.GenerateURLToken( 24 )
	now := time.Now().UTC()
	link = &InviteLink{
		ID:         id,
		SecretHash: encryption.Sha256Hex( secret ),
		Label:      strings.TrimSpace( label ),
		MaxUses:    max_uses,
		Accounts:   []uint64{},
		CreatedBy:  created_by,
		CreatedAt:  now,
		ExpiresAt:  now.Add( ttl ),
	}
	if err = store.Put( db.BucketInviteLinks , []byte( id ) , link ); err != nil {
		link = nil
		return
	}
	credential = id + "." + secret
	return
}

// IssueInviteLinkAs is IssueInviteLink with the checks the CLI and the
// control socket need: the inputs are in range and the maker is an enabled
// admin, since a link made by anyone else would never load.
func IssueInviteLinkAs( store *db.Store , created_by uint64 , label string , max_uses int , days int ) ( credential string , link *InviteLink , err error ) {
	if ValidInviteLink( label , max_uses , days ) == false {
		err = ErrBadInviteLink
		return
	}
	creator , err := GetUser( store , created_by )
	if err != nil { return }
	if creator.Disabled() || creator.IsAdmin() == false {
		err = ErrInviteNotAdmin
		return
	}
	credential , link , err = IssueInviteLink( store , label , max_uses , time.Duration( days )*24*time.Hour , created_by )
	return
}

// LoadInviteLink checks a credential and returns its live link. Every way it
// can be wrong answers the same, like a login link.
//
// A link also dies with its maker's admin rights: once the admin who made it
// is disabled, deleted, or made a plain user, the links they made stop
// working too.
func LoadInviteLink( store *db.Store , credential string ) ( link *InviteLink , err error ) {
	id , secret , ok := splitInviteCredential( credential )
	link = &InviteLink{}
	creator := &User{}
	if ok == false ||
		store.Get( db.BucketInviteLinks , []byte( id ) , link ) != nil ||
		store.Get( db.BucketUsers , encryption.Uint64ToBytes( link.CreatedBy ) , creator ) != nil ||
		inviteLinkUsable( link , secret , creator ) == false {
		link , err = nil , ErrInviteLinkInvalid
	}
	return
}

// loadInviteLinkTx is LoadInviteLink inside the caller's write transaction,
// so the checks and whatever the link is used for land together.
func loadInviteLinkTx( tx *db.Tx , credential string ) ( link *InviteLink , err error ) {
	id , secret , ok := splitInviteCredential( credential )
	link = &InviteLink{}
	creator := &User{}
	if ok == false ||
		tx.Get( db.BucketInviteLinks , []byte( id ) , link ) != nil ||
		tx.Get( db.BucketUsers , encryption.Uint64ToBytes( link.CreatedBy ) , creator ) != nil ||
		inviteLinkUsable( link , secret , creator ) == false {
		link , err = nil , ErrInviteLinkInvalid
	}
	return
}

func splitInviteCredential( credential string ) ( id string , secret string , ok bool ) {
	id , secret , ok = splitCredential( strings.Trim( strings.TrimSpace( credential ) , "/" ) )
	return
}

func inviteLinkUsable( link *InviteLink , secret string , creator *User ) ( result bool ) {
	result = encryption.ConstantTimeEqual( link.SecretHash , encryption.Sha256Hex( secret ) ) &&
		link.Live() && creator.Disabled() == false && creator.IsAdmin()
	return
}

// JoinInviteLink creates a new account through the link and the session it
// signs in with. Checking the link, taking a place, creating the account and
// storing the session are one transaction, so two people joining at the same
// moment cannot both take the last place, and a link switched off mid-way
// either lands first or refuses.
func JoinInviteLink( store *db.Store , credential string , display_name string , session_ttl time.Duration ) ( session_credential string , user *User , err error ) {
	err = store.Write( func( tx *db.Tx ) ( tx_err error ) {
		link , tx_err := loadInviteLinkTx( tx , credential )
		if tx_err != nil { return }
		if link.Remaining() <= 0 {
			tx_err = ErrInviteLinkFull
			return
		}
		if user , tx_err = createUserTx( tx , display_name , RoleUser ); tx_err != nil { return }
		link.Accounts = append( link.Accounts , user.ID )
		if tx_err = tx.Put( db.BucketInviteLinks , []byte( link.ID ) , link ); tx_err != nil { return }
		session_id , new_credential , session := newSession( user.ID , session_ttl )
		if tx_err = tx.Put( db.BucketSessions , []byte( session_id ) , session ); tx_err != nil { return }
		session_credential = new_credential
		return
	} )
	if err != nil { session_credential , user = "" , nil }
	return
}

// ClaimInviteAccount signs in as an account this link created -- the same
// person on another device. The caller checks the PIN first (bcrypt stays
// outside the single writer); everything else is re-checked here, in the
// transaction that stores the session.
func ClaimInviteAccount( store *db.Store , credential string , user_id uint64 , session_ttl time.Duration ) ( session_credential string , user *User , err error ) {
	err = store.Write( func( tx *db.Tx ) ( tx_err error ) {
		link , tx_err := loadInviteLinkTx( tx , credential )
		if tx_err != nil { return }
		user = &User{}
		if link.Owns( user_id ) == false ||
			tx.Get( db.BucketUsers , encryption.Uint64ToBytes( user_id ) , user ) != nil ||
			user.Disabled() || user.IsAdmin() {
			tx_err = ErrNotInviteAccount
			return
		}
		session_id , new_credential , session := newSession( user.ID , session_ttl )
		if tx_err = tx.Put( db.BucketSessions , []byte( session_id ) , session ); tx_err != nil { return }
		session_credential = new_credential
		return
	} )
	if err != nil { session_credential , user = "" , nil }
	return
}

// InviteAccount returns an account the link may sign in as, or
// ErrNotInviteAccount: never an admin, a disabled account, or one that did
// not join through this link.
func InviteAccount( store *db.Store , link *InviteLink , user_id uint64 ) ( user *User , err error ) {
	user , get_err := GetUser( store , user_id )
	if get_err != nil || link.Owns( user_id ) == false || user.Disabled() || user.IsAdmin() {
		user , err = nil , ErrNotInviteAccount
	}
	return
}

// InviteMatches are this link's accounts whose names are close to typed,
// closest first. Disabled accounts and admins are never offered.
func InviteMatches( store *db.Store , link *InviteLink , typed string ) ( matches []*User , err error ) {
	type scored struct {
		user  *User
		score int
	}
	found := []scored{}
	for _ , user_id := range link.Accounts {
		user , get_err := InviteAccount( store , link , user_id )
		if get_err != nil { continue }
		if score := NameCloseness( typed , user.DisplayName ); score >= 0 {
			found = append( found , scored{ user , score } )
		}
	}
	sort.SliceStable( found , func( i int , j int ) bool { return found[ i ].score < found[ j ].score } )
	matches = []*User{}
	for _ , entry := range found {
		matches = append( matches , entry.user )
	}
	return
}

// ListInviteLinks is every link, newest first.
func ListInviteLinks( store *db.Store ) ( links []*InviteLink , err error ) {
	links = []*InviteLink{}
	err = store.ForEach( db.BucketInviteLinks ,
		func() any { return &InviteLink{} } ,
		func( key []byte , item any ) bool {
			links = append( links , item.( *InviteLink ) )
			return true
		} )
	sort.Slice( links , func( i int , j int ) bool { return links[ i ].CreatedAt.After( links[ j ].CreatedAt ) } )
	return
}

// GetInviteLink loads a link by its public id, without its secret: for
// managing a link, never for joining through one.
func GetInviteLink( store *db.Store , link_id string ) ( link *InviteLink , err error ) {
	link = &InviteLink{}
	if err = store.Get( db.BucketInviteLinks , []byte( link_id ) , link ); err != nil { link = nil }
	return
}

// RevokeInviteLink switches a link off. Nobody can join or sign back in
// through it afterwards; the accounts it made keep their sessions.
func RevokeInviteLink( store *db.Store , link_id string ) ( err error ) {
	err = store.UpdateValue( db.BucketInviteLinks , []byte( link_id ) ,
		func() any { return &InviteLink{} } ,
		func( item any ) ( mutate_err error ) {
			link := item.( *InviteLink )
			if link.RevokedAt != nil { return }
			now := time.Now().UTC()
			link.RevokedAt = &now
			return
		} )
	return
}

// DeleteInviteLink removes a link for good. The accounts it made stay; they
// just can no longer sign in on a new device through it.
func DeleteInviteLink( store *db.Store , link_id string ) ( err error ) {
	if _ , err = GetInviteLink( store , link_id ); err != nil { return }
	err = store.Delete( db.BucketInviteLinks , []byte( link_id ) )
	return
}

// PurgeDeadInviteLinks drops links that have been revoked or expired for
// longer than inviteLinkRetention. Dead links are already refused by
// LoadInviteLink, so this is only housekeeping.
func PurgeDeadInviteLinks( store *db.Store ) ( removed int , err error ) {
	cutoff := time.Now().UTC().Add( -inviteLinkRetention )
	removed , err = store.DeleteWhere( db.BucketInviteLinks ,
		func() any { return &InviteLink{} } ,
		func( key []byte , item any ) bool {
			link := item.( *InviteLink )
			if link.RevokedAt != nil && link.RevokedAt.Before( cutoff ) {
				return true
			}
			return link.ExpiresAt.Before( cutoff )
		} )
	return
}
