package models

import (
	errors "errors"
	strings "strings"
	time "time"

	db "{{MODULE_PATH}}/server/db"
	encryption "{{MODULE_PATH}}/server/encryption"
)

// APIKey is the non-browser way in. It exists so that a script, a cron job, a
// CI pipeline, or another service can call the same API a signed-in person
// calls, without inventing a second permission model for them.
//
// A key carries a role from the same set users do -- admin or user -- so
// whatever an admin may do in the UI an admin-scoped key may do over HTTP,
// and a user-scoped key is limited to exactly what a non-admin account is.
// There is no separate scope language to keep in step with the role checks.
//
// # A key can never outrank its owner
//
// The role here is a *request*, and it is bounded twice. IssueAPIKey refuses
// to mint an admin key for a non-admin account, and NarrowRole re-applies the
// ceiling on every request. The second check is what covers an account that
// is demoted after the key was minted: the stored role still says admin, and
// the effective role does not.
//
// # Same "<id>.<secret>" shape as sessions, and the same hash
//
// The id is the bolt key, so authenticating a key is one O(1) fetch and one
// hash comparison -- see references/architecture.md on why a salted hash
// cannot be looked up. sha256 rather than bcrypt because this runs on every
// request against a 256-bit random secret, exactly like a session; bcrypt
// here would be a latency tax that buys nothing.
type APIKey struct {
	ID         string     `json:"id"`
	UserID     uint64     `json:"user_id"`
	Name       string     `json:"name"`
	Role       string     `json:"role"`
	SecretHash string     `json:"secret_hash"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

var (
	ErrAPIKeyLimit    = errors.New( "this account already holds the maximum number of live API keys" )
	ErrAPIKeyRole     = errors.New( "an API key cannot be given a role its owner does not have" )
	ErrAPIKeyDisabled = errors.New( "cannot issue an API key for a disabled account" )
)

const (
	// APIKeyMaxPerUser bounds how many live keys one account can hold. Any
	// signed-in user can mint keys, so without a cap a single account can
	// grow the bucket without limit. 25 is far past what a person needs and
	// low enough to stay a bound; revoking one frees a slot.
	APIKeyMaxPerUser = 25

	// apiKeyTouchInterval is how stale LastUsedAt is allowed to get.
	//
	// "When was this key last used" is the question an operator asks before
	// revoking something, so it is worth recording -- but bolt has a single
	// writer, and a write per authenticated request would put every handler
	// that needs the database behind it. Writing at most once per interval
	// keeps the answer useful (to within 15 minutes) and the cost negligible.
	apiKeyTouchInterval = 15 * time.Minute

	// apiKeyRetention is how long a dead key's row is kept before the purge
	// loop drops it. A revoked key that vanished immediately would make the
	// listing lie about what existed last week, which is the wrong trade for
	// a record this small; keeping it forever is the other wrong trade.
	apiKeyRetention = 30 * 24 * time.Hour

	// APIKeyMaxDays bounds an explicit expiry. Zero still means "never
	// expires", asked for deliberately; this only stops a typo from producing
	// a date so far out that it is the same thing by accident.
	APIKeyMaxDays = 3650
)

// APIKeyTTLFromDays turns a requested lifetime in days into a duration, and
// lives here rather than in a handler so that the HTTP API, the control
// socket, and the CLI cannot end up disagreeing about what "0 days" means.
//
// nil means "use the configured default", 0 means "never expires", and a
// number means that many days. ok is false for a value that is out of range.
func APIKeyTTLFromDays( days *int , fallback time.Duration ) ( ttl time.Duration , ok bool ) {
	if days == nil {
		ttl , ok = fallback , true
		return
	}
	if *days < 0 || *days > APIKeyMaxDays { return }
	ok = true
	if *days > 0 {
		ttl = time.Duration( *days ) * 24 * time.Hour
	}
	return
}

func ( key *APIKey ) Revoked() ( result bool ) {
	result = key.RevokedAt != nil
	return
}

func ( key *APIKey ) Expired() ( result bool ) {
	result = key.ExpiresAt != nil && time.Now().UTC().After( *key.ExpiresAt )
	return
}

// Live means the key would authenticate right now, as far as the key itself
// is concerned. Whether its owner is still enabled is a separate question,
// asked in LoadAPIKey.
func ( key *APIKey ) Live() ( result bool ) {
	result = key.Revoked() == false && key.Expired() == false
	return
}

// ValidAPIKeyName bounds the label a key is given. It is free text a user
// controls, same as a display name, and it exists only so a list of keys is
// readable -- "deploy bot", "grafana", "my laptop".
func ValidAPIKeyName( name string ) ( result bool ) {
	trimmed := strings.TrimSpace( name )
	result = len( trimmed ) >= 1 && len( trimmed ) <= 80
	return
}

// NarrowRole is the permission ceiling, applied on every request that
// authenticates with a key: the effective role is the *lower* of the account's
// role and the key's.
//
// Storing the key's role and then re-deriving the effective one, rather than
// trusting what was stored, is what makes demotion work. If an admin mints an
// admin key and later becomes an ordinary user, the row still says admin --
// and every request it authenticates is treated as a user.
func NarrowRole( account_role string , key_role string ) ( result string ) {
	result = RoleUser
	if account_role == RoleAdmin && key_role == RoleAdmin {
		result = RoleAdmin
	}
	return
}

// IssueAPIKey returns the raw credential exactly once. It is never stored and
// cannot be recovered -- if it is lost, revoke the key and mint another.
//
// ttl of zero means the key never expires. That is a deliberate option rather
// than an oversight: a CI job nobody will remember to re-key is worse served
// by a credential that dies silently at 3am. It has to be asked for
// explicitly; the default comes from configuration.
func IssueAPIKey( store *db.Store , user_id uint64 , name string , role string , ttl time.Duration ) ( credential string , key *APIKey , err error ) {
	// Load the owner rather than taking the caller's word for the role. Every
	// route into this function -- the HTTP API, the control socket, the CLI --
	// then gets the same ceiling without each having to remember it.
	owner , err := GetUser( store , user_id )
	if err != nil { return }
	if owner.Disabled() {
		err = ErrAPIKeyDisabled
		return
	}
	if ValidRole( role ) == false {
		role = RoleUser
	}
	if role == RoleAdmin && owner.IsAdmin() == false {
		err = ErrAPIKeyRole
		return
	}

	live , err := CountLiveAPIKeysForUser( store , user_id )
	if err != nil { return }
	if live >= APIKeyMaxPerUser {
		err = ErrAPIKeyLimit
		return
	}

	key_id := encryption.GenerateURLToken( 12 )
	secret := encryption.GenerateURLToken( 32 )
	now := time.Now().UTC()
	key = &APIKey{
		ID:         key_id,
		UserID:     user_id,
		Name:       strings.TrimSpace( name ),
		Role:       role,
		SecretHash: encryption.Sha256Hex( secret ),
		CreatedAt:  now,
	}
	if ttl > 0 {
		expires := now.Add( ttl )
		key.ExpiresAt = &expires
	}

	if err = store.Put( db.BucketAPIKeys , []byte( key_id ) , key ); err != nil {
		key = nil
		return
	}
	credential = key_id + "." + secret
	return
}

// LoadAPIKey returns nil (with no error) for anything that is not a usable
// key -- wrong shape, unknown id, bad secret, expired, revoked, or belonging
// to a disabled account. Callers only ever need "is this request
// authenticated", and folding every failure into one answer avoids leaking
// which check failed.
//
// Note what is *not* here: a disabled account does not have its keys revoked,
// the way it has its sessions destroyed. A session is a browser artifact and
// destroying it costs nothing; a key is a configured integration, and tearing
// every integration's credential down because someone was disabled for an
// afternoon is a footgun with no security gain -- the check below already
// refuses the request either way.
func LoadAPIKey( store *db.Store , credential string ) ( key *APIKey , user *User ) {
	key_id , secret , ok := splitCredential( credential )
	if ok == false { return }

	found := &APIKey{}
	if err := store.Get( db.BucketAPIKeys , []byte( key_id ) , found ); err != nil { return }

	if encryption.ConstantTimeEqual( found.SecretHash , encryption.Sha256Hex( secret ) ) == false { return }
	if found.Live() == false { return }

	loaded_user , err := GetUser( store , found.UserID )
	if err != nil || loaded_user.Disabled() { return }

	touchAPIKey( store , found )
	key , user = found , loaded_user
	return
}

// touchAPIKey records use at most once per apiKeyTouchInterval. Failures are
// ignored on purpose: this is bookkeeping, and a request that authenticated
// correctly should not fail because a timestamp could not be written.
func touchAPIKey( store *db.Store , key *APIKey ) {
	now := time.Now().UTC()
	if key.LastUsedAt != nil && now.Sub( *key.LastUsedAt ) < apiKeyTouchInterval {
		return
	}
	store.UpdateValue( db.BucketAPIKeys , []byte( key.ID ) ,
		func() any { return &APIKey{} } ,
		func( item any ) ( mutate_err error ) {
			item.( *APIKey ).LastUsedAt = &now
			return
		} )
	key.LastUsedAt = &now
	return
}

func GetAPIKey( store *db.Store , key_id string ) ( key *APIKey , err error ) {
	key = &APIKey{}
	err = store.Get( db.BucketAPIKeys , []byte( key_id ) , key )
	if err != nil { key = nil }
	return
}

// ListAPIKeys returns every key in the database, newest last. Only the admin
// endpoint and the CLI use it; a user asking for "my keys" goes through
// ListAPIKeysForUser.
func ListAPIKeys( store *db.Store ) ( keys []*APIKey , err error ) {
	keys = []*APIKey{}
	err = store.ForEach( db.BucketAPIKeys ,
		func() any { return &APIKey{} } ,
		func( key []byte , item any ) bool {
			keys = append( keys , item.( *APIKey ) )
			return true
		} )
	return
}

func ListAPIKeysForUser( store *db.Store , user_id uint64 ) ( keys []*APIKey , err error ) {
	keys = []*APIKey{}
	err = store.ForEach( db.BucketAPIKeys ,
		func() any { return &APIKey{} } ,
		func( key []byte , item any ) bool {
			stored := item.( *APIKey )
			if stored.UserID == user_id {
				keys = append( keys , stored )
			}
			return true
		} )
	return
}

func CountLiveAPIKeysForUser( store *db.Store , user_id uint64 ) ( count int , err error ) {
	err = store.ForEach( db.BucketAPIKeys ,
		func() any { return &APIKey{} } ,
		func( key []byte , item any ) bool {
			stored := item.( *APIKey )
			if stored.UserID == user_id && stored.Live() {
				count += 1
			}
			return true
		} )
	return
}

// RevokeAPIKey marks a key dead. It is idempotent -- revoking twice keeps the
// first timestamp -- and it goes through UpdateValue so that two concurrent
// revocations cannot have one overwrite the other with a stale copy of the row.
//
// The row is marked rather than deleted so that a listing can still show the
// key existed and when it died. The purge loop removes it later.
func RevokeAPIKey( store *db.Store , key_id string ) ( err error ) {
	err = store.UpdateValue( db.BucketAPIKeys , []byte( key_id ) ,
		func() any { return &APIKey{} } ,
		func( item any ) ( mutate_err error ) {
			key := item.( *APIKey )
			if key.RevokedAt == nil {
				now := time.Now().UTC()
				key.RevokedAt = &now
			}
			return
		} )
	return
}

// PurgeDeadAPIKeys drops rows for keys that have been revoked or expired for
// longer than apiKeyRetention. Dead keys are already refused by LoadAPIKey, so
// this is only housekeeping -- one transaction, for the same reason as
// PurgeExpiredSessions.
func PurgeDeadAPIKeys( store *db.Store ) ( removed int , err error ) {
	cutoff := time.Now().UTC().Add( -apiKeyRetention )
	removed , err = store.DeleteWhere( db.BucketAPIKeys ,
		func() any { return &APIKey{} } ,
		func( key []byte , item any ) bool {
			stored := item.( *APIKey )
			if stored.RevokedAt != nil && stored.RevokedAt.Before( cutoff ) {
				return true
			}
			return stored.ExpiresAt != nil && stored.ExpiresAt.Before( cutoff )
		} )
	return
}
